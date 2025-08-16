package consensus

import (
	"fmt"
	"log"
	"time"

	"distributed-kvstore/proto/cluster"
)

// SafetyValidator ensures Raft safety properties are maintained
type SafetyValidator struct {
	logger *log.Logger
}

// NewSafetyValidator creates a new safety validator
func NewSafetyValidator(logger *log.Logger) *SafetyValidator {
	if logger == nil {
		logger = log.New(log.Writer(), "[SAFETY] ", log.LstdFlags)
	}
	return &SafetyValidator{
		logger: logger,
	}
}

// ValidateElectionSafety ensures election safety property:
// At most one leader can be elected in a given term
func (sv *SafetyValidator) ValidateElectionSafety(nodes []*RaftNode) error {
	termLeaders := make(map[int64][]string)
	
	for _, node := range nodes {
		state, term, isLeader := node.GetState()
		if isLeader {
			termLeaders[term] = append(termLeaders[term], node.id)
		}
		sv.logger.Printf("Node %s: State=%s, Term=%d, IsLeader=%t", 
			node.id, state, term, isLeader)
	}
	
	// Check that no term has more than one leader
	for term, leaders := range termLeaders {
		if len(leaders) > 1 {
			return fmt.Errorf("election safety violation: term %d has multiple leaders: %v", 
				term, leaders)
		}
	}
	
	sv.logger.Printf("Election safety validated: %d terms with leaders", len(termLeaders))
	return nil
}

// ValidateLogMatching ensures log matching property:
// If two logs contain an entry with the same index and term, 
// then the logs are identical in all entries up through the given index
func (sv *SafetyValidator) ValidateLogMatching(nodes []*RaftNode) error {
	// Collect all log entries from all nodes
	allEntries := make(map[string][]*cluster.LogEntry)
	
	for _, node := range nodes {
		node.mu.RLock()
		entries := make([]*cluster.LogEntry, len(node.log))
		copy(entries, node.log)
		allEntries[node.id] = entries
		node.mu.RUnlock()
	}
	
	// Check log matching property
	for nodeID1, log1 := range allEntries {
		for nodeID2, log2 := range allEntries {
			if nodeID1 >= nodeID2 {
				continue // Avoid duplicate comparisons
			}
			
			if err := sv.validateLogPairMatching(nodeID1, log1, nodeID2, log2); err != nil {
				return err
			}
		}
	}
	
	sv.logger.Printf("Log matching validated across %d nodes", len(nodes))
	return nil
}

// validateLogPairMatching validates log matching between two specific logs
func (sv *SafetyValidator) validateLogPairMatching(nodeID1 string, log1 []*cluster.LogEntry, 
	nodeID2 string, log2 []*cluster.LogEntry) error {
	
	minLen := len(log1)
	if len(log2) < minLen {
		minLen = len(log2)
	}
	
	for i := 0; i < minLen; i++ {
		entry1 := log1[i]
		entry2 := log2[i]
		
		// If entries have same index and term, all preceding entries must match
		if entry1.Index == entry2.Index && entry1.Term == entry2.Term {
			// Validate all entries up to this point match
			for j := 0; j <= i; j++ {
				if log1[j].Index != log2[j].Index || log1[j].Term != log2[j].Term {
					return fmt.Errorf("log matching violation between %s and %s: "+
						"entries at position %d have same index/term (%d/%d) but "+
						"preceding entry at position %d differs", 
						nodeID1, nodeID2, i, entry1.Index, entry1.Term, j)
				}
			}
		}
	}
	
	return nil
}

// ValidateLeaderCompleteness ensures leader completeness property:
// If a log entry is committed in a given term, then that entry will be present 
// in the logs of the leaders for all higher-numbered terms
func (sv *SafetyValidator) ValidateLeaderCompleteness(nodes []*RaftNode, 
	committedEntries map[int64]*cluster.LogEntry) error {
	
	// Find leaders and their terms
	leaders := make(map[int64]*RaftNode)
	for _, node := range nodes {
		state, term, isLeader := node.GetState()
		if isLeader {
			leaders[term] = node
		}
		_ = state // Suppress unused variable warning
	}
	
	// Check that each leader has all entries committed in lower terms
	for leaderTerm, leader := range leaders {
		leader.mu.RLock()
		leaderLog := make([]*cluster.LogEntry, len(leader.log))
		copy(leaderLog, leader.log)
		leader.mu.RUnlock()
		
		for commitTerm, committedEntry := range committedEntries {
			if commitTerm < leaderTerm {
				// Leader should have this committed entry
				found := false
				for _, entry := range leaderLog {
					if entry.Index == committedEntry.Index && 
					   entry.Term == committedEntry.Term {
						found = true
						break
					}
				}
				
				if !found {
					return fmt.Errorf("leader completeness violation: "+
						"leader %s (term %d) missing committed entry %d from term %d",
						leader.id, leaderTerm, committedEntry.Index, commitTerm)
				}
			}
		}
	}
	
	sv.logger.Printf("Leader completeness validated for %d leaders", len(leaders))
	return nil
}

// ValidateStateMachineSafety ensures state machine safety property:
// If a server has applied a log entry at a given index to its state machine, 
// no other server will ever apply a different log entry for the same index
func (sv *SafetyValidator) ValidateStateMachineSafety(nodes []*RaftNode) error {
	appliedEntries := make(map[int64]*cluster.LogEntry) // index -> entry
	
	for _, node := range nodes {
		node.mu.RLock()
		lastApplied := node.lastApplied
		log := node.log
		nodeID := node.id
		node.mu.RUnlock()
		
		// Check all applied entries for this node
		for i := int64(1); i <= lastApplied && i <= int64(len(log)); i++ {
			entry := log[i-1]
			
			if existingEntry, exists := appliedEntries[entry.Index]; exists {
				// Another node has applied an entry at this index
				if existingEntry.Term != entry.Term || 
				   string(existingEntry.Data) != string(entry.Data) {
					return fmt.Errorf("state machine safety violation: "+
						"node %s applied different entry at index %d "+
						"(term %d vs %d)", nodeID, entry.Index, 
						entry.Term, existingEntry.Term)
				}
			} else {
				appliedEntries[entry.Index] = entry
			}
		}
	}
	
	sv.logger.Printf("State machine safety validated: %d applied entries", len(appliedEntries))
	return nil
}

// ValidateMonotonicTerms ensures terms are monotonically increasing
func (sv *SafetyValidator) ValidateMonotonicTerms(nodes []*RaftNode, 
	termHistory map[string][]int64) error {
	
	for _, node := range nodes {
		_, currentTerm, _ := node.GetState()
		nodeID := node.id
		
		// Add current term to history
		if termHistory[nodeID] == nil {
			termHistory[nodeID] = []int64{}
		}
		
		// Check if current term is greater than or equal to last recorded term
		history := termHistory[nodeID]
		if len(history) > 0 {
			lastTerm := history[len(history)-1]
			if currentTerm < lastTerm {
				return fmt.Errorf("monotonic term violation: "+
					"node %s term decreased from %d to %d", 
					nodeID, lastTerm, currentTerm)
			}
		}
		
		// Only add if term actually changed
		if len(history) == 0 || currentTerm != history[len(history)-1] {
			termHistory[nodeID] = append(history, currentTerm)
		}
	}
	
	return nil
}

// ValidateCommitSafety ensures commit safety:
// Once a log entry is committed, it will never be overwritten
func (sv *SafetyValidator) ValidateCommitSafety(nodes []*RaftNode, 
	committedEntries map[int64]*cluster.LogEntry) error {
	
	for _, node := range nodes {
		node.mu.RLock()
		commitIndex := node.commitIndex
		log := node.log
		nodeID := node.id
		node.mu.RUnlock()
		
		// Check all committed entries in this node
		for i := int64(1); i <= commitIndex && i <= int64(len(log)); i++ {
			entry := log[i-1]
			
			if committedEntry, exists := committedEntries[entry.Index]; exists {
				// This index was previously committed
				if committedEntry.Term != entry.Term || 
				   string(committedEntry.Data) != string(entry.Data) {
					return fmt.Errorf("commit safety violation: "+
						"node %s has different committed entry at index %d "+
						"(term %d vs %d)", nodeID, entry.Index, 
						entry.Term, committedEntry.Term)
				}
			} else {
				// Record this as a newly committed entry
				committedEntries[entry.Index] = entry
			}
		}
	}
	
	sv.logger.Printf("Commit safety validated: %d committed entries tracked", 
		len(committedEntries))
	return nil
}

// PartitionTolerance manages network partition scenarios
type PartitionTolerance struct {
	partitions map[string][]string // partition -> list of node IDs
	logger     *log.Logger
}

// NewPartitionTolerance creates a new partition tolerance manager
func NewPartitionTolerance(logger *log.Logger) *PartitionTolerance {
	if logger == nil {
		logger = log.New(log.Writer(), "[PARTITION] ", log.LstdFlags)
	}
	return &PartitionTolerance{
		partitions: make(map[string][]string),
		logger:     logger,
	}
}

// CreatePartition creates a network partition
func (pt *PartitionTolerance) CreatePartition(partitionID string, nodeIDs []string) {
	pt.partitions[partitionID] = nodeIDs
	pt.logger.Printf("Created partition %s with nodes: %v", partitionID, nodeIDs)
}

// RemovePartition removes a network partition
func (pt *PartitionTolerance) RemovePartition(partitionID string) {
	if nodes, exists := pt.partitions[partitionID]; exists {
		delete(pt.partitions, partitionID)
		pt.logger.Printf("Removed partition %s (had nodes: %v)", partitionID, nodes)
	}
}

// CanCommunicate checks if two nodes can communicate given current partitions
func (pt *PartitionTolerance) CanCommunicate(nodeID1, nodeID2 string) bool {
	// Nodes can communicate if they're in the same partition or no partitions exist
	if len(pt.partitions) == 0 {
		return true
	}
	
	for _, nodeList := range pt.partitions {
		hasNode1 := false
		hasNode2 := false
		
		for _, nodeID := range nodeList {
			if nodeID == nodeID1 {
				hasNode1 = true
			}
			if nodeID == nodeID2 {
				hasNode2 = true
			}
		}
		
		if hasNode1 && hasNode2 {
			return true
		}
	}
	
	return false
}

// SimulatePartition simulates a network partition for a specified duration
func (pt *PartitionTolerance) SimulatePartition(partitionID string, nodeIDs []string, 
	duration time.Duration) {
	pt.CreatePartition(partitionID, nodeIDs)
	
	time.AfterFunc(duration, func() {
		pt.RemovePartition(partitionID)
	})
}

// GetPartitionInfo returns information about current partitions
func (pt *PartitionTolerance) GetPartitionInfo() map[string][]string {
	result := make(map[string][]string)
	for k, v := range pt.partitions {
		result[k] = make([]string, len(v))
		copy(result[k], v)
	}
	return result
}