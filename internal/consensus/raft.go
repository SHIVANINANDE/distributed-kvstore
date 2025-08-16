package consensus

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"distributed-kvstore/proto/cluster"
)

// RaftState represents the state of a Raft node
type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
)

func (s RaftState) String() string {
	switch s {
	case Follower:
		return "follower"
	case Candidate:
		return "candidate"
	case Leader:
		return "leader"
	default:
		return "unknown"
	}
}

// RaftNode represents a single node in the Raft cluster
type RaftNode struct {
	// Node identification
	id       string
	address  string
	raftPort int32
	grpcPort int32

	// Raft state
	mu                sync.RWMutex
	state             RaftState
	currentTerm       int64
	votedFor          string
	log               []*cluster.LogEntry
	commitIndex       int64
	lastApplied       int64

	// Leader state (reinitialized after election)
	nextIndex  map[string]int64
	matchIndex map[string]int64

	// Cluster membership
	peers map[string]*PeerInfo

	// Timing and control
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration
	lastHeartbeat    time.Time
	electionTimer    *time.Timer
	heartbeatTimer   *time.Timer

	// Channels for communication
	voteRequestCh  chan *VoteRequest
	voteResponseCh chan *VoteResponse
	appendEntryCh  chan *AppendRequest
	applyEntryCh   chan *cluster.LogEntry
	shutdownCh     chan struct{}

	// State machine
	stateMachine StateMachine

	// Persistent state manager
	persistence *PersistentState
	logStorage  *LogStorage

	// Safety validation
	safetyValidator *SafetyValidator
	partitionTolerance *PartitionTolerance

	// Logger
	logger *log.Logger

	// Metrics and monitoring
	lastLogIndex int64
	lastLogTerm  int64
}

// PeerInfo contains information about a peer node
type PeerInfo struct {
	ID       string
	Address  string
	RaftPort int32
	GrpcPort int32
	Active   bool
	LastSeen time.Time
}

// VoteRequest represents a request for votes during leader election
type VoteRequest struct {
	Term         int64
	CandidateID  string
	LastLogIndex int64
	LastLogTerm  int64
	ResponseCh   chan *VoteResponse
}

// VoteResponse represents the response to a vote request
type VoteResponse struct {
	Term        int64
	VoteGranted bool
	NodeID      string
}

// AppendRequest represents an append entries request (heartbeat or log replication)
type AppendRequest struct {
	Term         int64
	LeaderID     string
	PrevLogIndex int64
	PrevLogTerm  int64
	Entries      []*cluster.LogEntry
	LeaderCommit int64
	ResponseCh   chan *AppendResponse
}

// AppendResponse represents the response to an append entries request
type AppendResponse struct {
	Term         int64
	Success      bool
	LastLogIndex int64
	NodeID       string
}

// StateMachine interface for applying committed log entries
type StateMachine interface {
	Apply(entry *cluster.LogEntry) interface{}
	Snapshot() ([]byte, error)
	Restore(data []byte) error
}

// Config holds the configuration for a Raft node
type Config struct {
	NodeID           string
	Address          string
	RaftPort         int32
	GrpcPort         int32
	ElectionTimeout  time.Duration
	HeartbeatTimeout time.Duration
	StateMachine     StateMachine
	Logger           *log.Logger
}

// NewRaftNode creates a new Raft node
func NewRaftNode(config Config) (*RaftNode, error) {
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = randomElectionTimeout()
	}
	if config.HeartbeatTimeout == 0 {
		config.HeartbeatTimeout = 50 * time.Millisecond
	}
	if config.Logger == nil {
		config.Logger = log.New(log.Writer(), fmt.Sprintf("[%s] ", config.NodeID), log.LstdFlags)
	}

	persistence, err := NewPersistentState(config.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistent state: %w", err)
	}

	logStorage, err := NewLogStorage(config.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to create log storage: %w", err)
	}

	node := &RaftNode{
		id:               config.NodeID,
		address:          config.Address,
		raftPort:         config.RaftPort,
		grpcPort:         config.GrpcPort,
		state:            Follower,
		currentTerm:      0,
		votedFor:         "",
		log:              make([]*cluster.LogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		nextIndex:        make(map[string]int64),
		matchIndex:       make(map[string]int64),
		peers:            make(map[string]*PeerInfo),
		electionTimeout:  config.ElectionTimeout,
		heartbeatTimeout: config.HeartbeatTimeout,
		lastHeartbeat:    time.Now(),
		voteRequestCh:    make(chan *VoteRequest, 100),
		voteResponseCh:   make(chan *VoteResponse, 100),
		appendEntryCh:    make(chan *AppendRequest, 100),
		applyEntryCh:     make(chan *cluster.LogEntry, 100),
		shutdownCh:       make(chan struct{}),
		stateMachine:     config.StateMachine,
		persistence:      persistence,
		logStorage:       logStorage,
		safetyValidator:  NewSafetyValidator(config.Logger),
		partitionTolerance: NewPartitionTolerance(config.Logger),
		logger:           config.Logger,
	}

	// Load persistent state
	if err := node.loadPersistentState(); err != nil {
		return nil, fmt.Errorf("failed to load persistent state: %w", err)
	}

	return node, nil
}

// Start starts the Raft node
func (rn *RaftNode) Start() {
	rn.logger.Printf("Starting Raft node %s in %s state", rn.id, rn.state)
	
	// Start election timer
	rn.resetElectionTimer()
	
	// Start main event loop
	go rn.run()
	
	// Start log application goroutine
	go rn.applyEntries()
}

// Stop stops the Raft node
func (rn *RaftNode) Stop() {
	rn.logger.Printf("Stopping Raft node %s", rn.id)
	close(rn.shutdownCh)
}

// GetState returns the current state of the node
func (rn *RaftNode) GetState() (RaftState, int64, bool) {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.state, rn.currentTerm, rn.state == Leader
}

// GetLeader returns the current leader ID
func (rn *RaftNode) GetLeader() string {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	
	if rn.state == Leader {
		return rn.id
	}
	
	// Find the leader from peers
	for id, peer := range rn.peers {
		if peer.Active && time.Since(peer.LastSeen) < rn.heartbeatTimeout*3 {
			// This is a heuristic - in a real implementation, we'd track the actual leader
			return id
		}
	}
	
	return ""
}

// AddPeer adds a peer to the cluster
func (rn *RaftNode) AddPeer(id, address string, raftPort, grpcPort int32) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	rn.peers[id] = &PeerInfo{
		ID:       id,
		Address:  address,
		RaftPort: raftPort,
		GrpcPort: grpcPort,
		Active:   true,
		LastSeen: time.Now(),
	}
	
	// Initialize leader state for new peer
	if rn.state == Leader {
		rn.nextIndex[id] = rn.getLastLogIndex() + 1
		rn.matchIndex[id] = 0
	}
	
	rn.logger.Printf("Added peer %s at %s:%d", id, address, raftPort)
}

// RemovePeer removes a peer from the cluster
func (rn *RaftNode) RemovePeer(id string) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	delete(rn.peers, id)
	delete(rn.nextIndex, id)
	delete(rn.matchIndex, id)
	
	rn.logger.Printf("Removed peer %s", id)
}

// run is the main event loop for the Raft node
func (rn *RaftNode) run() {
	for {
		select {
		case <-rn.shutdownCh:
			return
			
		case <-rn.electionTimer.C:
			rn.handleElectionTimeout()
			
		case <-func() <-chan time.Time {
			if rn.heartbeatTimer != nil {
				return rn.heartbeatTimer.C
			}
			return make(chan time.Time)
		}():
			if rn.state == Leader {
				rn.sendHeartbeats()
			}
			
		case voteReq := <-rn.voteRequestCh:
			rn.handleVoteRequest(voteReq)
			
		case voteResp := <-rn.voteResponseCh:
			rn.handleVoteResponse(voteResp)
			
		case appendReq := <-rn.appendEntryCh:
			rn.handleAppendEntries(appendReq)
		}
	}
}

// randomElectionTimeout returns a random election timeout between 150-300ms
func randomElectionTimeout() time.Duration {
	min := 150
	max := 300
	return time.Duration(rand.Intn(max-min)+min) * time.Millisecond
}

// resetElectionTimer resets the election timer with a new random timeout
func (rn *RaftNode) resetElectionTimer() {
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	rn.electionTimeout = randomElectionTimeout()
	rn.electionTimer = time.NewTimer(rn.electionTimeout)
}

// resetHeartbeatTimer resets the heartbeat timer
func (rn *RaftNode) resetHeartbeatTimer() {
	if rn.heartbeatTimer != nil {
		rn.heartbeatTimer.Stop()
	}
	rn.heartbeatTimer = time.NewTimer(rn.heartbeatTimeout)
}

// getLastLogIndex returns the index of the last log entry
func (rn *RaftNode) getLastLogIndex() int64 {
	if len(rn.log) == 0 {
		return 0
	}
	return rn.log[len(rn.log)-1].Index
}

// getLastLogTerm returns the term of the last log entry
func (rn *RaftNode) getLastLogTerm() int64 {
	if len(rn.log) == 0 {
		return 0
	}
	return rn.log[len(rn.log)-1].Term
}

// getMajority returns the number of nodes needed for majority
func (rn *RaftNode) getMajority() int {
	return (len(rn.peers) + 2) / 2 // +1 for self, +1 for majority calculation
}

// handleElectionTimeout handles election timeout by starting a new election
func (rn *RaftNode) handleElectionTimeout() {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	// Only followers and candidates can start elections
	if rn.state == Leader {
		return
	}
	
	rn.logger.Printf("Election timeout, starting new election")
	rn.startElection()
}

// startElection starts a new leader election
func (rn *RaftNode) startElection() {
	// Increment current term and vote for self
	rn.currentTerm++
	rn.state = Candidate
	rn.votedFor = rn.id
	rn.resetElectionTimer()
	
	// Save persistent state
	rn.savePersistentState()
	
	rn.logger.Printf("Starting election for term %d", rn.currentTerm)
	
	// Send vote requests to all peers
	go rn.requestVotes()
}

// requestVotes sends vote requests to all peers
func (rn *RaftNode) requestVotes() {
	rn.mu.RLock()
	term := rn.currentTerm
	lastLogIndex := rn.getLastLogIndex()
	lastLogTerm := rn.getLastLogTerm()
	peers := make(map[string]*PeerInfo)
	for k, v := range rn.peers {
		peers[k] = v
	}
	rn.mu.RUnlock()
	
	votes := 1 // Vote for self
	majority := rn.getMajority()
	responseCh := make(chan *VoteResponse, len(peers))
	
	// Send vote requests to all peers
	for peerID := range peers {
		go func(peerID string) {
			// In a real implementation, this would be an RPC call
			// For now, we'll simulate it
			response := &VoteResponse{
				Term:        term,
				VoteGranted: rn.simulateVoteRequest(peerID, term, lastLogIndex, lastLogTerm),
				NodeID:      peerID,
			}
			
			select {
			case responseCh <- response:
			case <-time.After(rn.electionTimeout / 2):
				// Timeout
			}
		}(peerID)
	}
	
	// Collect votes
	for i := 0; i < len(peers); i++ {
		select {
		case response := <-responseCh:
			rn.mu.Lock()
			
			// Check if we're still a candidate and in the same term
			if rn.state != Candidate || rn.currentTerm != term {
				rn.mu.Unlock()
				return
			}
			
			// If response term is higher, step down
			if response.Term > rn.currentTerm {
				rn.currentTerm = response.Term
				rn.state = Follower
				rn.votedFor = ""
				rn.savePersistentState()
				rn.resetElectionTimer()
				rn.mu.Unlock()
				return
			}
			
			// Count the vote
			if response.VoteGranted {
				votes++
				rn.logger.Printf("Received vote from %s (%d/%d)", response.NodeID, votes, majority)
			}
			
			// Check if we have majority
			if votes >= majority {
				rn.becomeLeader()
				rn.mu.Unlock()
				return
			}
			
			rn.mu.Unlock()
			
		case <-time.After(rn.electionTimeout):
			// Election timeout, start new election
			return
		}
	}
}

// becomeLeader transitions the node to leader state
func (rn *RaftNode) becomeLeader() {
	rn.logger.Printf("Won election for term %d, becoming leader", rn.currentTerm)
	
	rn.state = Leader
	
	// Initialize leader state
	lastLogIndex := rn.getLastLogIndex()
	for peerID := range rn.peers {
		rn.nextIndex[peerID] = lastLogIndex + 1
		rn.matchIndex[peerID] = 0
	}
	
	// Stop election timer and start heartbeat timer
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	rn.resetHeartbeatTimer()
	
	// Send initial heartbeats
	go rn.sendHeartbeats()
}

// simulateVoteRequest simulates a vote request to a peer
// In a real implementation, this would be an actual RPC call
func (rn *RaftNode) simulateVoteRequest(peerID string, term, lastLogIndex, lastLogTerm int64) bool {
	// For simulation, randomly grant votes with 70% probability
	// In reality, this would follow Raft voting rules
	return rand.Float64() < 0.7
}

// handleVoteRequest handles an incoming vote request
func (rn *RaftNode) handleVoteRequest(req *VoteRequest) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	response := &VoteResponse{
		Term:        rn.currentTerm,
		VoteGranted: false,
		NodeID:      rn.id,
	}
	
	// If term is outdated, reject
	if req.Term < rn.currentTerm {
		req.ResponseCh <- response
		return
	}
	
	// If term is newer, update our term and step down
	if req.Term > rn.currentTerm {
		rn.currentTerm = req.Term
		rn.state = Follower
		rn.votedFor = ""
		rn.savePersistentState()
	}
	
	// Grant vote if we haven't voted or already voted for this candidate
	// and candidate's log is at least as up-to-date as ours
	if (rn.votedFor == "" || rn.votedFor == req.CandidateID) &&
		rn.isLogUpToDate(req.LastLogIndex, req.LastLogTerm) {
		
		rn.votedFor = req.CandidateID
		rn.savePersistentState()
		rn.resetElectionTimer()
		response.VoteGranted = true
		response.Term = rn.currentTerm
		
		rn.logger.Printf("Granted vote to %s for term %d", req.CandidateID, req.Term)
	}
	
	req.ResponseCh <- response
}

// isLogUpToDate checks if the candidate's log is at least as up-to-date as ours
func (rn *RaftNode) isLogUpToDate(lastLogIndex, lastLogTerm int64) bool {
	ourLastLogTerm := rn.getLastLogTerm()
	ourLastLogIndex := rn.getLastLogIndex()
	
	// If terms are different, the one with higher term is more up-to-date
	if lastLogTerm != ourLastLogTerm {
		return lastLogTerm >= ourLastLogTerm
	}
	
	// If terms are the same, the one with higher index is more up-to-date
	return lastLogIndex >= ourLastLogIndex
}

// handleVoteResponse handles a vote response
func (rn *RaftNode) handleVoteResponse(resp *VoteResponse) {
	// Vote responses are handled in the requestVotes goroutine
	select {
	case rn.voteResponseCh <- resp:
	default:
		// Channel full, drop response
	}
}

// sendHeartbeats sends heartbeat messages to all peers
func (rn *RaftNode) sendHeartbeats() {
	rn.mu.RLock()
	if rn.state != Leader {
		rn.mu.RUnlock()
		return
	}
	
	term := rn.currentTerm
	peers := make(map[string]*PeerInfo)
	for k, v := range rn.peers {
		peers[k] = v
	}
	rn.mu.RUnlock()
	
	// Send heartbeats to all peers
	for peerID := range peers {
		go func(peerID string) {
			// In a real implementation, this would be an RPC call
			rn.sendAppendEntries(peerID, term, nil)
		}(peerID)
	}
	
	rn.resetHeartbeatTimer()
}

// sendAppendEntries sends an append entries RPC to a peer
func (rn *RaftNode) sendAppendEntries(peerID string, term int64, entries []*cluster.LogEntry) {
	// Check if we can communicate with this peer (network partition simulation)
	if !rn.partitionTolerance.CanCommunicate(rn.id, peerID) {
		rn.logger.Printf("Cannot send heartbeat to %s: network partition", peerID)
		return
	}

	rn.mu.RLock()
	prevLogIndex := rn.nextIndex[peerID] - 1
	var prevLogTerm int64
	if prevLogIndex > 0 && prevLogIndex <= int64(len(rn.log)) {
		prevLogTerm = rn.log[prevLogIndex-1].Term
	}
	leaderCommit := rn.commitIndex
	rn.mu.RUnlock()
	
	// For simulation purposes, we'll just log the heartbeat
	// In a real implementation, this would be an actual RPC call with prevLogTerm and leaderCommit
	rn.logger.Printf("Sending heartbeat to %s (term: %d, prevLogIndex: %d, prevLogTerm: %d, leaderCommit: %d)", 
		peerID, term, prevLogIndex, prevLogTerm, leaderCommit)
}

// handleAppendEntries handles an incoming append entries request
func (rn *RaftNode) handleAppendEntries(req *AppendRequest) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	response := &AppendResponse{
		Term:         rn.currentTerm,
		Success:      false,
		LastLogIndex: rn.getLastLogIndex(),
		NodeID:       rn.id,
	}
	
	// Reply false if term < currentTerm (§5.1)
	if req.Term < rn.currentTerm {
		req.ResponseCh <- response
		return
	}
	
	// If term is newer or equal, update our term and step down to follower
	if req.Term >= rn.currentTerm {
		rn.currentTerm = req.Term
		rn.state = Follower
		rn.votedFor = ""
		rn.savePersistentState()
		rn.resetElectionTimer()
		rn.lastHeartbeat = time.Now()
		response.Term = rn.currentTerm
	}
	
	// Reply false if log doesn't contain an entry at prevLogIndex
	// whose term matches prevLogTerm (§5.3)
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex > rn.getLastLogIndex() {
			// We don't have the previous log entry
			rn.logger.Printf("AppendEntries failed: missing log entry at index %d", req.PrevLogIndex)
			req.ResponseCh <- response
			return
		}
		
		if req.PrevLogIndex <= int64(len(rn.log)) {
			prevEntry := rn.log[req.PrevLogIndex-1]
			if prevEntry.Term != req.PrevLogTerm {
				// Previous log entry term doesn't match
				rn.logger.Printf("AppendEntries failed: term mismatch at index %d (our: %d, leader: %d)", 
					req.PrevLogIndex, prevEntry.Term, req.PrevLogTerm)
				req.ResponseCh <- response
				return
			}
		}
	}
	
	// If an existing entry conflicts with a new one (same index
	// but different terms), delete the existing entry and all that
	// follow it (§5.3)
	if req.Entries != nil && len(req.Entries) > 0 {
		for i, entry := range req.Entries {
			entryIndex := req.PrevLogIndex + int64(i) + 1
			
			if entryIndex <= int64(len(rn.log)) {
				// Check for conflict
				existingEntry := rn.log[entryIndex-1]
				if existingEntry.Term != entry.Term {
					// Conflict detected - truncate log from this point
					rn.logger.Printf("Log conflict at index %d, truncating", entryIndex)
					rn.log = rn.log[:entryIndex-1]
					break
				}
			}
		}
		
		// Append any new entries not already in the log
		for i, entry := range req.Entries {
			entryIndex := req.PrevLogIndex + int64(i) + 1
			
			if entryIndex > int64(len(rn.log)) {
				// This is a new entry, append it
				rn.log = append(rn.log, entry)
				rn.logger.Printf("Appended entry at index %d (term: %d, type: %s)", 
					entry.Index, entry.Term, entry.Type)
			}
		}
		
		// Persist the updated log
		if err := rn.logStorage.AppendEntries(req.Entries); err != nil {
			rn.logger.Printf("Failed to persist log entries: %v", err)
		}
	}
	
	// If leaderCommit > commitIndex, set commitIndex =
	// min(leaderCommit, index of last new entry)
	if req.LeaderCommit > rn.commitIndex {
		newCommitIndex := req.LeaderCommit
		lastNewEntryIndex := rn.getLastLogIndex()
		
		if newCommitIndex > lastNewEntryIndex {
			newCommitIndex = lastNewEntryIndex
		}
		
		rn.commitIndex = newCommitIndex
		rn.logger.Printf("Updated commitIndex to %d", rn.commitIndex)
		
		// Trigger application of newly committed entries
		go rn.triggerApplyEntries()
	}
	
	response.Success = true
	response.LastLogIndex = rn.getLastLogIndex()
	
	if len(req.Entries) > 0 {
		rn.logger.Printf("Successfully processed AppendEntries with %d entries from leader %s", 
			len(req.Entries), req.LeaderID)
	}
	
	req.ResponseCh <- response
}

// loadPersistentState loads the persistent state from storage
func (rn *RaftNode) loadPersistentState() error {
	state, err := rn.persistence.Load()
	if err != nil {
		return err
	}
	
	rn.currentTerm = state.CurrentTerm
	rn.votedFor = state.VotedFor
	// Log loading would be implemented here in a real system
	
	return nil
}

// savePersistentState saves the persistent state to storage
func (rn *RaftNode) savePersistentState() {
	state := &PersistentStateData{
		CurrentTerm: rn.currentTerm,
		VotedFor:    rn.votedFor,
		// Log would be saved here in a real system
	}
	
	if err := rn.persistence.Save(state); err != nil {
		rn.logger.Printf("Failed to save persistent state: %v", err)
	}
}

// applyEntries applies committed log entries to the state machine
func (rn *RaftNode) applyEntries() {
	for {
		select {
		case <-rn.shutdownCh:
			return
		case entry := <-rn.applyEntryCh:
			if rn.stateMachine != nil {
				result := rn.stateMachine.Apply(entry)
				rn.logger.Printf("Applied entry %d to state machine: %v", entry.Index, result)
			}
		default:
			// Check if we have entries to apply
			rn.mu.Lock()
			for rn.lastApplied < rn.commitIndex {
				rn.lastApplied++
				if rn.lastApplied <= int64(len(rn.log)) {
					entry := rn.log[rn.lastApplied-1]
					if rn.stateMachine != nil {
						rn.mu.Unlock()
						result := rn.stateMachine.Apply(entry)
						rn.mu.Lock()
						rn.logger.Printf("Applied entry %d to state machine: %v", entry.Index, result)
					}
				}
			}
			rn.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// triggerApplyEntries triggers application of committed entries
func (rn *RaftNode) triggerApplyEntries() {
	// This method is called when commitIndex is updated
	// The applyEntries goroutine will handle the actual application
}

// AppendEntry adds a new entry to the log (called by leader)
func (rn *RaftNode) AppendEntry(entryType string, data []byte) (int64, error) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state != Leader {
		return 0, fmt.Errorf("not the leader")
	}

	// Create new log entry
	index := rn.getLastLogIndex() + 1
	entry := &cluster.LogEntry{
		Index:     index,
		Term:      rn.currentTerm,
		Type:      entryType,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	// Append to local log
	rn.log = append(rn.log, entry)
	
	// Persist the entry
	if err := rn.logStorage.AppendEntries([]*cluster.LogEntry{entry}); err != nil {
		return 0, fmt.Errorf("failed to persist entry: %w", err)
	}

	rn.logger.Printf("Leader appended entry %d (term: %d, type: %s)", index, rn.currentTerm, entryType)

	// Immediately try to replicate to followers
	go rn.replicateToFollowers()

	return index, nil
}

// replicateToFollowers sends entries to all followers
func (rn *RaftNode) replicateToFollowers() {
	rn.mu.RLock()
	if rn.state != Leader {
		rn.mu.RUnlock()
		return
	}

	peers := make(map[string]*PeerInfo)
	for k, v := range rn.peers {
		peers[k] = v
	}
	rn.mu.RUnlock()

	for peerID := range peers {
		go rn.replicateToFollower(peerID)
	}
}

// replicateToFollower sends entries to a specific follower
func (rn *RaftNode) replicateToFollower(peerID string) {
	rn.mu.Lock()
	if rn.state != Leader {
		rn.mu.Unlock()
		return
	}

	nextIndex := rn.nextIndex[peerID]
	if nextIndex == 0 {
		nextIndex = 1
	}

	prevLogIndex := nextIndex - 1
	var prevLogTerm int64
	if prevLogIndex > 0 && prevLogIndex <= int64(len(rn.log)) {
		prevLogTerm = rn.log[prevLogIndex-1].Term
	}

	// Collect entries to send
	var entries []*cluster.LogEntry
	if nextIndex <= int64(len(rn.log)) {
		entries = rn.log[nextIndex-1:]
	}

	term := rn.currentTerm
	leaderCommit := rn.commitIndex
	rn.mu.Unlock()

	// Create append entries request
	req := &AppendRequest{
		Term:         term,
		LeaderID:     rn.id,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: leaderCommit,
		ResponseCh:   make(chan *AppendResponse, 1),
	}

	// Send to follower (this would be an RPC call in real implementation)
	// For now, we'll simulate it
	go func() {
		// Simulate network delay
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
		
		// Simulate response (70% success rate)
		success := rand.Float64() < 0.7
		resp := &AppendResponse{
			Term:         term,
			Success:      success,
			LastLogIndex: prevLogIndex + int64(len(entries)),
			NodeID:       peerID,
		}
		
		select {
		case req.ResponseCh <- resp:
		case <-time.After(1 * time.Second):
		}
	}()

	// Wait for response
	select {
	case resp := <-req.ResponseCh:
		rn.handleAppendEntriesResponse(peerID, req, resp)
	case <-time.After(1 * time.Second):
		rn.logger.Printf("Append entries to %s timed out", peerID)
	}
}

// handleAppendEntriesResponse handles the response from append entries
func (rn *RaftNode) handleAppendEntriesResponse(peerID string, req *AppendRequest, resp *AppendResponse) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state != Leader || resp.Term != rn.currentTerm {
		return
	}

	if resp.Term > rn.currentTerm {
		// Step down
		rn.currentTerm = resp.Term
		rn.state = Follower
		rn.votedFor = ""
		rn.savePersistentState()
		rn.resetElectionTimer()
		return
	}

	if resp.Success {
		// Update nextIndex and matchIndex for follower
		if len(req.Entries) > 0 {
			rn.matchIndex[peerID] = req.PrevLogIndex + int64(len(req.Entries))
			rn.nextIndex[peerID] = rn.matchIndex[peerID] + 1
			
			rn.logger.Printf("Successfully replicated %d entries to %s", len(req.Entries), peerID)
			
			// Check if we can advance commit index
			rn.updateCommitIndex()
		}
	} else {
		// Decrement nextIndex and retry
		if rn.nextIndex[peerID] > 1 {
			rn.nextIndex[peerID]--
			rn.logger.Printf("Append entries failed for %s, decremented nextIndex to %d", peerID, rn.nextIndex[peerID])
			
			// Retry immediately
			go rn.replicateToFollower(peerID)
		}
	}
}

// updateCommitIndex updates the commit index based on majority replication
func (rn *RaftNode) updateCommitIndex() {
	if rn.state != Leader {
		return
	}

	// Find the highest index that is replicated on a majority of servers
	for n := rn.getLastLogIndex(); n > rn.commitIndex; n-- {
		if n <= int64(len(rn.log)) && rn.log[n-1].Term == rn.currentTerm {
			count := 1 // Count self
			
			for _, matchIndex := range rn.matchIndex {
				if matchIndex >= n {
					count++
				}
			}
			
			if count >= rn.getMajority() {
				rn.commitIndex = n
				rn.logger.Printf("Leader advanced commitIndex to %d", n)
				
				// Trigger application of newly committed entries
				go rn.triggerApplyEntries()
				return
			}
		}
	}
}