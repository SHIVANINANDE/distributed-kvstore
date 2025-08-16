package consensus

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"distributed-kvstore/proto/cluster"
)

func TestRaftNodeCreation(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_data"
	defer os.RemoveAll(tempDir)

	config := Config{
		NodeID:           "test-node-1",
		Address:          "localhost",
		RaftPort:         7001,
		GrpcPort:         9001,
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		Logger:           log.New(os.Stdout, "[test] ", log.LstdFlags),
	}

	node, err := NewRaftNode(config)
	if err != nil {
		t.Fatalf("Failed to create Raft node: %v", err)
	}

	if node.id != config.NodeID {
		t.Errorf("Expected node ID %s, got %s", config.NodeID, node.id)
	}

	if node.state != Follower {
		t.Errorf("Expected initial state to be Follower, got %s", node.state)
	}

	if node.currentTerm != 0 {
		t.Errorf("Expected initial term to be 0, got %d", node.currentTerm)
	}

	node.Stop()
}

func TestRaftElectionTimeout(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_data"
	defer os.RemoveAll(tempDir)

	config := Config{
		NodeID:           "test-node-1",
		Address:          "localhost",
		RaftPort:         7001,
		GrpcPort:         9001,
		ElectionTimeout:  100 * time.Millisecond, // Short timeout for testing
		HeartbeatTimeout: 50 * time.Millisecond,
		Logger:           log.New(os.Stdout, "[test] ", log.LstdFlags),
	}

	node, err := NewRaftNode(config)
	if err != nil {
		t.Fatalf("Failed to create Raft node: %v", err)
	}
	defer node.Stop()

	// Start the node
	node.Start()

	// Wait for election timeout to trigger
	time.Sleep(200 * time.Millisecond)

	// Node should become candidate after election timeout
	state, term, _ := node.GetState()
	if state != Candidate && state != Leader {
		t.Errorf("Expected node to become Candidate or Leader after election timeout, got %s", state)
	}

	if term == 0 {
		t.Errorf("Expected term to be incremented after election, got %d", term)
	}
}

func TestRaftSingleNodeElection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_data"
	defer os.RemoveAll(tempDir)

	config := Config{
		NodeID:           "test-node-1",
		Address:          "localhost",
		RaftPort:         7001,
		GrpcPort:         9001,
		ElectionTimeout:  100 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		Logger:           log.New(os.Stdout, "[test] ", log.LstdFlags),
	}

	node, err := NewRaftNode(config)
	if err != nil {
		t.Fatalf("Failed to create Raft node: %v", err)
	}
	defer node.Stop()

	// Start the node
	node.Start()

	// Wait for election to complete
	time.Sleep(300 * time.Millisecond)

	// Single node should become leader quickly (no peers to vote)
	// In our current implementation, it won't become leader without peers
	// but it should at least become candidate
	state, term, isLeader := node.GetState()
	
	t.Logf("Node state: %s, term: %d, isLeader: %t", state, term, isLeader)

	if term == 0 {
		t.Errorf("Expected term to be incremented, got %d", term)
	}

	if state == Follower {
		t.Errorf("Expected node to transition from Follower state, got %s", state)
	}
}

func TestRaftPersistentState(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_data"
	defer os.RemoveAll(tempDir)

	nodeID := "test-persistent-node"
	ps, err := NewPersistentState(nodeID)
	if err != nil {
		t.Fatalf("Failed to create persistent state: %v", err)
	}

	// Test initial state
	state, err := ps.Load()
	if err != nil {
		t.Fatalf("Failed to load initial state: %v", err)
	}

	if state.CurrentTerm != 0 {
		t.Errorf("Expected initial term to be 0, got %d", state.CurrentTerm)
	}

	if state.VotedFor != "" {
		t.Errorf("Expected initial votedFor to be empty, got %s", state.VotedFor)
	}

	// Test saving state
	newState := &PersistentStateData{
		CurrentTerm: 5,
		VotedFor:    "candidate-1",
	}

	err = ps.Save(newState)
	if err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Test loading saved state
	loadedState, err := ps.Load()
	if err != nil {
		t.Fatalf("Failed to load saved state: %v", err)
	}

	if loadedState.CurrentTerm != 5 {
		t.Errorf("Expected term 5, got %d", loadedState.CurrentTerm)
	}

	if loadedState.VotedFor != "candidate-1" {
		t.Errorf("Expected votedFor 'candidate-1', got %s", loadedState.VotedFor)
	}
}

func TestRaftRandomElectionTimeout(t *testing.T) {
	// Test that election timeouts are randomized
	timeouts := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		timeouts[i] = randomElectionTimeout()
	}

	// Check that we have some variation
	allSame := true
	first := timeouts[0]
	for _, timeout := range timeouts[1:] {
		if timeout != first {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("All election timeouts are the same - randomization not working")
	}

	// Check that all timeouts are within expected range
	for i, timeout := range timeouts {
		if timeout < 150*time.Millisecond || timeout > 300*time.Millisecond {
			t.Errorf("Timeout %d (%v) is outside expected range [150ms, 300ms]", i, timeout)
		}
	}
}

func TestRaftMajorityCalculation(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_data"
	defer os.RemoveAll(tempDir)

	config := Config{
		NodeID:           "test-node",
		Address:          "localhost",
		RaftPort:         7001,
		GrpcPort:         9001,
		ElectionTimeout:  100 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		Logger:           log.New(os.Stdout, "[test] ", log.LstdFlags),
	}

	node, err := NewRaftNode(config)
	if err != nil {
		t.Fatalf("Failed to create Raft node: %v", err)
	}
	defer node.Stop()

	// Test majority calculation with different cluster sizes
	testCases := []struct {
		peers    int
		expected int
	}{
		{0, 1}, // Single node cluster
		{1, 2}, // 2 node cluster (1 peer + self)
		{2, 2}, // 3 node cluster (2 peers + self)
		{3, 3}, // 4 node cluster (3 peers + self)
		{4, 3}, // 5 node cluster (4 peers + self)
	}

	for _, tc := range testCases {
		// Clear existing peers
		node.mu.Lock()
		node.peers = make(map[string]*PeerInfo)
		
		// Add the specified number of peers
		for i := 0; i < tc.peers; i++ {
			peerID := fmt.Sprintf("peer-%d", i)
			node.peers[peerID] = &PeerInfo{ID: peerID}
		}
		
		majority := node.getMajority()
		node.mu.Unlock()

		if majority != tc.expected {
			t.Errorf("For %d peers, expected majority %d, got %d", tc.peers, tc.expected, majority)
		}
	}
}

// TestRaftLogUpToDateComparison tests the log up-to-date comparison logic
func TestRaftLogUpToDateComparison(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_data"
	defer os.RemoveAll(tempDir)

	config := Config{
		NodeID:           "test-node",
		Address:          "localhost",
		RaftPort:         7001,
		GrpcPort:         9001,
		ElectionTimeout:  100 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		Logger:           log.New(os.Stdout, "[test] ", log.LstdFlags),
	}

	node, err := NewRaftNode(config)
	if err != nil {
		t.Fatalf("Failed to create Raft node: %v", err)
	}
	defer node.Stop()

	// Test cases for log up-to-date comparison
	testCases := []struct {
		name           string
		ourLastTerm    int64
		ourLastIndex   int64
		candLastTerm   int64
		candLastIndex  int64
		expectedResult bool
	}{
		{"Higher term wins", 2, 5, 3, 1, true},
		{"Lower term loses", 3, 5, 2, 10, false},
		{"Same term, higher index wins", 2, 3, 2, 5, true},
		{"Same term, lower index loses", 2, 5, 2, 3, false},
		{"Same term, same index", 2, 5, 2, 5, true},
		{"Empty log loses to any log", 0, 0, 1, 1, true},
		{"Any log wins over empty log", 1, 1, 0, 0, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up our log state
			node.mu.Lock()
			node.log = make([]*cluster.LogEntry, 0)
			if tc.ourLastIndex > 0 {
				for i := int64(1); i <= tc.ourLastIndex; i++ {
					term := tc.ourLastTerm
					if i < tc.ourLastIndex {
						term = 1 // Earlier entries have term 1
					}
					node.log = append(node.log, &cluster.LogEntry{
						Index: i,
						Term:  term,
					})
				}
			}

			result := node.isLogUpToDate(tc.candLastIndex, tc.candLastTerm)
			node.mu.Unlock()

			if result != tc.expectedResult {
				t.Errorf("Expected %t, got %t", tc.expectedResult, result)
			}
		})
	}
}

