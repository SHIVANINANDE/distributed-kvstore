package consensus

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"distributed-kvstore/proto/cluster"
)

// TestRaftChaosElection tests leader election under chaotic conditions
func TestRaftChaosElection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_chaos_data"
	defer os.RemoveAll(tempDir)

	// Create multiple nodes
	nodeCount := 5
	nodes := make([]*RaftNode, nodeCount)
	
	for i := 0; i < nodeCount; i++ {
		stateMachine := NewKVStateMachine(log.New(os.Stdout, fmt.Sprintf("[SM-%d] ", i), log.LstdFlags))
		config := Config{
			NodeID:           fmt.Sprintf("chaos-node-%d", i),
			Address:          "localhost",
			RaftPort:         int32(7000 + i),
			GrpcPort:         int32(9000 + i),
			ElectionTimeout:  time.Duration(100+rand.Intn(100)) * time.Millisecond,
			HeartbeatTimeout: 50 * time.Millisecond,
			StateMachine:     stateMachine,
			Logger:           log.New(os.Stdout, fmt.Sprintf("[CHAOS-%d] ", i), log.LstdFlags),
		}

		node, err := NewRaftNode(config)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		nodes[i] = node
	}

	// Connect all nodes to each other
	for i, node := range nodes {
		for j, peer := range nodes {
			if i != j {
				node.AddPeer(peer.id, peer.address, peer.raftPort, peer.grpcPort)
			}
		}
	}

	// Start all nodes
	for _, node := range nodes {
		node.Start()
	}
	defer func() {
		for _, node := range nodes {
			node.Stop()
		}
	}()

	// Run chaos scenarios
	t.Run("RandomNodeFailures", func(t *testing.T) {
		testRandomNodeFailures(t, nodes)
	})

	t.Run("NetworkPartitions", func(t *testing.T) {
		testNetworkPartitions(t, nodes)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		testConcurrentOperations(t, nodes)
	})

	t.Run("SafetyProperties", func(t *testing.T) {
		testSafetyProperties(t, nodes)
	})
}

// testRandomNodeFailures simulates random node failures and recoveries
func testRandomNodeFailures(t *testing.T, nodes []*RaftNode) {
	t.Logf("Testing random node failures with %d nodes", len(nodes))
	
	// Track which nodes are currently running
	running := make([]bool, len(nodes))
	for i := range running {
		running[i] = true
	}

	// Simulate failures and recoveries for 10 seconds
	duration := 10 * time.Second
	endTime := time.Now().Add(duration)
	
	for time.Now().Before(endTime) {
		// Random action: start or stop a node
		nodeIndex := rand.Intn(len(nodes))
		
		if running[nodeIndex] {
			// Stop the node
			if countRunning(running) > 2 { // Keep at least 2 nodes running
				t.Logf("Stopping node %d", nodeIndex)
				nodes[nodeIndex].Stop()
				running[nodeIndex] = false
			}
		} else {
			// Restart the node
			t.Logf("Restarting node %d", nodeIndex)
			nodes[nodeIndex].Start()
			running[nodeIndex] = true
		}
		
		// Wait between actions
		time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)
	}
	
	// Restart all nodes
	for i, node := range nodes {
		if !running[i] {
			node.Start()
			running[i] = true
		}
	}
	
	// Wait for stabilization
	time.Sleep(2 * time.Second)
	
	// Verify that we have a leader
	leaderCount := 0
	for _, node := range nodes {
		_, _, isLeader := node.GetState()
		if isLeader {
			leaderCount++
		}
	}
	
	if leaderCount != 1 {
		t.Errorf("Expected exactly 1 leader after chaos, got %d", leaderCount)
	}
}

// testNetworkPartitions simulates network partitions
func testNetworkPartitions(t *testing.T, nodes []*RaftNode) {
	t.Logf("Testing network partitions with %d nodes", len(nodes))
	
	if len(nodes) < 3 {
		t.Skip("Need at least 3 nodes for partition testing")
	}

	// Create a majority partition and minority partition
	majoritySize := (len(nodes) / 2) + 1
	majorityNodes := nodes[:majoritySize]
	minorityNodes := nodes[majoritySize:]
	
	// Create partitions using the first node as partition manager
	pt := nodes[0].partitionTolerance
	
	// Partition 1: Majority
	majorityIDs := make([]string, len(majorityNodes))
	for i, node := range majorityNodes {
		majorityIDs[i] = node.id
	}
	pt.CreatePartition("majority", majorityIDs)
	
	// Partition 2: Minority
	minorityIDs := make([]string, len(minorityNodes))
	for i, node := range minorityNodes {
		minorityIDs[i] = node.id
	}
	pt.CreatePartition("minority", minorityIDs)
	
	t.Logf("Created majority partition with %d nodes, minority partition with %d nodes", 
		len(majorityIDs), len(minorityIDs))
	
	// Wait for partition effects
	time.Sleep(3 * time.Second)
	
	// The majority partition should elect a leader
	majorityLeaders := 0
	for _, node := range majorityNodes {
		_, _, isLeader := node.GetState()
		if isLeader {
			majorityLeaders++
		}
	}
	
	// The minority partition should not have a leader (or lose leadership)
	minorityLeaders := 0
	for _, node := range minorityNodes {
		_, _, isLeader := node.GetState()
		if isLeader {
			minorityLeaders++
		}
	}
	
	t.Logf("After partition: majority leaders=%d, minority leaders=%d", 
		majorityLeaders, minorityLeaders)
	
	// Heal the partition
	pt.RemovePartition("majority")
	pt.RemovePartition("minority")
	
	// Wait for healing
	time.Sleep(2 * time.Second)
	
	// Should have exactly one leader total
	totalLeaders := 0
	for _, node := range nodes {
		_, _, isLeader := node.GetState()
		if isLeader {
			totalLeaders++
		}
	}
	
	if totalLeaders != 1 {
		t.Errorf("Expected exactly 1 leader after partition healing, got %d", totalLeaders)
	}
}

// testConcurrentOperations tests concurrent operations under chaos
func testConcurrentOperations(t *testing.T, nodes []*RaftNode) {
	t.Logf("Testing concurrent operations with %d nodes", len(nodes))
	
	// Wait for leader election
	time.Sleep(1 * time.Second)
	
	// Find the leader
	var leader *RaftNode
	for _, node := range nodes {
		_, _, isLeader := node.GetState()
		if isLeader {
			leader = node
			break
		}
	}
	
	if leader == nil {
		t.Skip("No leader found for concurrent operations test")
	}
	
	// Perform concurrent operations
	operationCount := 50
	var wg sync.WaitGroup
	errors := make(chan error, operationCount)
	
	for i := 0; i < operationCount; i++ {
		wg.Add(1)
		go func(opNum int) {
			defer wg.Done()
			
			// Create operation data
			key := fmt.Sprintf("key-%d", opNum)
			value := fmt.Sprintf("value-%d", opNum)
			
			data, err := CreatePutOperation(key, value)
			if err != nil {
				errors <- fmt.Errorf("failed to create operation %d: %w", opNum, err)
				return
			}
			
			// Try to append entry
			_, err = leader.AppendEntry("PUT", data)
			if err != nil {
				errors <- fmt.Errorf("failed to append entry %d: %w", opNum, err)
				return
			}
		}(i)
	}
	
	// Wait for all operations to complete
	wg.Wait()
	close(errors)
	
	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Operation error: %v", err)
		errorCount++
	}
	
	if errorCount > operationCount/2 {
		t.Errorf("Too many operation failures: %d/%d", errorCount, operationCount)
	}
	
	// Wait for replication
	time.Sleep(2 * time.Second)
	
	t.Logf("Completed %d concurrent operations with %d errors", operationCount, errorCount)
}

// testSafetyProperties validates Raft safety properties under chaos
func testSafetyProperties(t *testing.T, nodes []*RaftNode) {
	t.Logf("Testing safety properties with %d nodes", len(nodes))
	
	// Initialize tracking for safety validation
	committedEntries := make(map[int64]*cluster.LogEntry)
	termHistory := make(map[string][]int64)
	
	// Get safety validator from first node
	validator := nodes[0].safetyValidator
	
	// Run validation multiple times during chaos
	for round := 0; round < 5; round++ {
		t.Logf("Safety validation round %d", round+1)
		
		// Validate election safety
		if err := validator.ValidateElectionSafety(nodes); err != nil {
			t.Errorf("Election safety violation in round %d: %v", round+1, err)
		}
		
		// Validate log matching
		if err := validator.ValidateLogMatching(nodes); err != nil {
			t.Errorf("Log matching violation in round %d: %v", round+1, err)
		}
		
		// Validate state machine safety
		if err := validator.ValidateStateMachineSafety(nodes); err != nil {
			t.Errorf("State machine safety violation in round %d: %v", round+1, err)
		}
		
		// Validate monotonic terms
		if err := validator.ValidateMonotonicTerms(nodes, termHistory); err != nil {
			t.Errorf("Monotonic terms violation in round %d: %v", round+1, err)
		}
		
		// Validate commit safety
		if err := validator.ValidateCommitSafety(nodes, committedEntries); err != nil {
			t.Errorf("Commit safety violation in round %d: %v", round+1, err)
		}
		
		// Introduce some chaos between validations
		if round < 4 {
			// Random delay
			time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)
			
			// Maybe create a temporary partition
			if rand.Float64() < 0.3 && len(nodes) >= 3 {
				pt := nodes[0].partitionTolerance
				splitPoint := len(nodes) / 2
				partition1 := make([]string, splitPoint)
				for i := 0; i < splitPoint; i++ {
					partition1[i] = nodes[i].id
				}
				
				pt.CreatePartition("temp", partition1)
				time.Sleep(500 * time.Millisecond)
				pt.RemovePartition("temp")
			}
		}
	}
	
	t.Logf("Safety properties validation completed successfully")
}

// countRunning counts how many nodes are currently running
func countRunning(running []bool) int {
	count := 0
	for _, isRunning := range running {
		if isRunning {
			count++
		}
	}
	return count
}

// TestRaftLogReplication tests log replication under various conditions
func TestRaftLogReplication(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_replication_data"
	defer os.RemoveAll(tempDir)

	// Create 3 nodes for replication testing
	nodes := make([]*RaftNode, 3)
	
	for i := 0; i < 3; i++ {
		stateMachine := NewKVStateMachine(log.New(os.Stdout, fmt.Sprintf("[SM-%d] ", i), log.LstdFlags))
		config := Config{
			NodeID:           fmt.Sprintf("repl-node-%d", i),
			Address:          "localhost",
			RaftPort:         int32(7100 + i),
			GrpcPort:         int32(9100 + i),
			ElectionTimeout:  150 * time.Millisecond,
			HeartbeatTimeout: 50 * time.Millisecond,
			StateMachine:     stateMachine,
			Logger:           log.New(os.Stdout, fmt.Sprintf("[REPL-%d] ", i), log.LstdFlags),
		}

		node, err := NewRaftNode(config)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		nodes[i] = node
	}

	// Connect nodes
	for i, node := range nodes {
		for j, peer := range nodes {
			if i != j {
				node.AddPeer(peer.id, peer.address, peer.raftPort, peer.grpcPort)
			}
		}
	}

	// Start nodes
	for _, node := range nodes {
		node.Start()
	}
	defer func() {
		for _, node := range nodes {
			node.Stop()
		}
	}()

	// Wait for leader election
	time.Sleep(1 * time.Second)

	// Find leader
	var leader *RaftNode
	for _, node := range nodes {
		_, _, isLeader := node.GetState()
		if isLeader {
			leader = node
			break
		}
	}

	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Test log replication
	t.Run("BasicReplication", func(t *testing.T) {
		// Append entries and verify replication
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("test-key-%d", i)
			value := fmt.Sprintf("test-value-%d", i)
			
			data, err := CreatePutOperation(key, value)
			if err != nil {
				t.Fatalf("Failed to create operation: %v", err)
			}
			
			index, err := leader.AppendEntry("PUT", data)
			if err != nil {
				t.Fatalf("Failed to append entry: %v", err)
			}
			
			t.Logf("Appended entry %d", index)
		}
		
		// Wait for replication
		time.Sleep(2 * time.Second)
		
		// Verify all nodes have the same log length
		leaderLogLen := 0
		leader.mu.RLock()
		leaderLogLen = len(leader.log)
		leader.mu.RUnlock()
		
		for _, node := range nodes {
			node.mu.RLock()
			nodeLogLen := len(node.log)
			node.mu.RUnlock()
			
			if nodeLogLen != leaderLogLen {
				t.Errorf("Log length mismatch: leader has %d, node %s has %d", 
					leaderLogLen, node.id, nodeLogLen)
			}
		}
	})
}

// TestRaftCompaction tests log compaction functionality
func TestRaftCompaction(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "test_compaction_data"
	defer os.RemoveAll(tempDir)

	// Create log storage
	nodeID := "compaction-test-node"
	logStorage, err := NewLogStorage(nodeID)
	if err != nil {
		t.Fatalf("Failed to create log storage: %v", err)
	}

	// Create many log entries
	var entries []*cluster.LogEntry
	for i := int64(1); i <= 1000; i++ {
		entry := &cluster.LogEntry{
			Index:     i,
			Term:      1,
			Type:      "PUT",
			Data:      []byte(fmt.Sprintf("data-%d", i)),
			Timestamp: time.Now().Unix(),
		}
		entries = append(entries, entry)
	}

	// Append entries
	if err := logStorage.AppendEntries(entries); err != nil {
		t.Fatalf("Failed to append entries: %v", err)
	}

	// Verify entries exist
	retrieved, err := logStorage.GetEntries(1, 1000)
	if err != nil {
		t.Fatalf("Failed to retrieve entries: %v", err)
	}
	
	if len(retrieved) != 1000 {
		t.Fatalf("Expected 1000 entries, got %d", len(retrieved))
	}

	// Test compaction
	compactIndex := int64(500)
	if err := logStorage.Compact(compactIndex); err != nil {
		t.Fatalf("Failed to compact log: %v", err)
	}

	// Verify compaction
	stats := logStorage.GetStats()
	if stats.CompactedUpTo != compactIndex {
		t.Errorf("Expected compacted up to %d, got %d", compactIndex, stats.CompactedUpTo)
	}

	// Verify we can't retrieve compacted entries
	_, err = logStorage.GetEntries(1, compactIndex)
	if err == nil {
		t.Error("Expected error when retrieving compacted entries")
	}

	// Verify we can still retrieve non-compacted entries
	remaining, err := logStorage.GetEntries(compactIndex+1, 1000)
	if err != nil {
		t.Fatalf("Failed to retrieve remaining entries: %v", err)
	}
	
	expectedRemaining := 1000 - compactIndex
	if int64(len(remaining)) != expectedRemaining {
		t.Errorf("Expected %d remaining entries, got %d", expectedRemaining, len(remaining))
	}

	t.Logf("Compaction test completed successfully")
}