package consensus

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"
)

// TestCluster3Nodes tests a 3-node cluster
func TestCluster3Nodes(t *testing.T) {
	tempDir := "test_cluster_3_data"
	defer os.RemoveAll(tempDir)

	// Create cluster configuration
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7001, GrpcPort: 9001},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7002, GrpcPort: 9002},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7003, GrpcPort: 9003},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	// Create cluster manager
	logger := log.New(os.Stdout, "[TEST-3NODE] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	// Bootstrap the cluster
	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for leader election
	leader, err := manager.WaitForLeader(5 * time.Second)
	if err != nil {
		t.Fatalf("No leader elected: %v", err)
	}

	t.Logf("Leader elected: %s", leader)

	// Verify cluster health
	if !manager.IsHealthy() {
		t.Error("Cluster should be healthy")
	}

	// Check cluster status
	status := manager.GetClusterStatus()
	if len(status) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(status))
	}

	// Verify exactly one leader
	leaderCount := 0
	for _, nodeStatus := range status {
		if nodeStatus.IsLeader {
			leaderCount++
		}
	}
	if leaderCount != 1 {
		t.Errorf("Expected exactly 1 leader, got %d", leaderCount)
	}

	t.Logf("3-node cluster test completed successfully")
}

// TestCluster5Nodes tests a 5-node cluster
func TestCluster5Nodes(t *testing.T) {
	tempDir := "test_cluster_5_data"
	defer os.RemoveAll(tempDir)

	// Create cluster configuration
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7101, GrpcPort: 9101},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7102, GrpcPort: 9102},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7103, GrpcPort: 9103},
			{NodeID: "node-4", Address: "localhost", RaftPort: 7104, GrpcPort: 9104},
			{NodeID: "node-5", Address: "localhost", RaftPort: 7105, GrpcPort: 9105},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	// Create cluster manager
	logger := log.New(os.Stdout, "[TEST-5NODE] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	// Bootstrap the cluster
	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for leader election
	leader, err := manager.WaitForLeader(5 * time.Second)
	if err != nil {
		t.Fatalf("No leader elected: %v", err)
	}

	t.Logf("Leader elected: %s", leader)

	// Verify cluster health
	if !manager.IsHealthy() {
		t.Error("Cluster should be healthy")
	}

	// Check cluster status
	status := manager.GetClusterStatus()
	if len(status) != 5 {
		t.Errorf("Expected 5 nodes, got %d", len(status))
	}

	// Verify exactly one leader
	leaderCount := 0
	for _, nodeStatus := range status {
		if nodeStatus.IsLeader {
			leaderCount++
		}
	}
	if leaderCount != 1 {
		t.Errorf("Expected exactly 1 leader, got %d", leaderCount)
	}

	t.Logf("5-node cluster test completed successfully")
}

// TestDynamicMembership tests adding and removing nodes
func TestDynamicMembership(t *testing.T) {
	tempDir := "test_dynamic_data"
	defer os.RemoveAll(tempDir)

	// Start with 3 nodes
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7201, GrpcPort: 9201},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7202, GrpcPort: 9202},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7203, GrpcPort: 9203},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	logger := log.New(os.Stdout, "[TEST-DYNAMIC] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for initial leader
	leader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		t.Fatalf("No initial leader elected: %v", err)
	}
	t.Logf("Initial leader: %s", leader)

	// Test adding a node
	newNode := &NodeConfig{
		NodeID:   "node-4",
		Address:  "localhost", 
		RaftPort: 7204,
		GrpcPort: 9204,
	}

	if err := manager.AddNode(newNode); err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	// Wait a bit for the new node to integrate
	time.Sleep(1 * time.Second)

	// Check that we now have 4 nodes
	status := manager.GetClusterStatus()
	if len(status) != 4 {
		t.Errorf("Expected 4 nodes after addition, got %d", len(status))
	}

	// Test removing a node
	if err := manager.RemoveNode("node-4"); err != nil {
		t.Fatalf("Failed to remove node: %v", err)
	}

	// Wait a bit for the node to be removed
	time.Sleep(1 * time.Second)

	// Check that we're back to 3 nodes
	status = manager.GetClusterStatus()
	if len(status) != 3 {
		t.Errorf("Expected 3 nodes after removal, got %d", len(status))
	}

	t.Logf("Dynamic membership test completed successfully")
}

// TestLeaderFailure tests leader failure and recovery
func TestLeaderFailure(t *testing.T) {
	tempDir := "test_leader_failure_data"
	defer os.RemoveAll(tempDir)

	// Create 3-node cluster
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7301, GrpcPort: 9301},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7302, GrpcPort: 9302},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7303, GrpcPort: 9303},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	logger := log.New(os.Stdout, "[TEST-FAILURE] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for initial leader
	initialLeader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		t.Fatalf("No initial leader elected: %v", err)
	}
	t.Logf("Initial leader: %s", initialLeader)

	// Simulate leader failure by removing it
	t.Logf("Simulating failure of leader %s", initialLeader)
	if err := manager.RemoveNode(initialLeader); err != nil {
		t.Fatalf("Failed to remove leader: %v", err)
	}

	// Wait for new leader election
	time.Sleep(2 * time.Second)
	
	newLeader := manager.GetLeader()
	if newLeader == "" {
		t.Error("No new leader elected after failure")
	} else if newLeader == initialLeader {
		t.Error("Same leader detected after failure - leader should have changed")
	} else {
		t.Logf("New leader elected: %s", newLeader)
	}

	// Verify cluster is still functional with remaining nodes
	status := manager.GetClusterStatus()
	if len(status) != 2 {
		t.Errorf("Expected 2 nodes after leader removal, got %d", len(status))
	}

	t.Logf("Leader failure test completed successfully")
}

// TestNetworkPartition tests network partition handling
func TestNetworkPartition(t *testing.T) {
	tempDir := "test_partition_data"
	defer os.RemoveAll(tempDir)

	// Create 5-node cluster for partition testing
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7401, GrpcPort: 9401},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7402, GrpcPort: 9402},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7403, GrpcPort: 9403},
			{NodeID: "node-4", Address: "localhost", RaftPort: 7404, GrpcPort: 9404},
			{NodeID: "node-5", Address: "localhost", RaftPort: 7405, GrpcPort: 9405},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	logger := log.New(os.Stdout, "[TEST-PARTITION] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for initial leader
	initialLeader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		t.Fatalf("No initial leader elected: %v", err)
	}
	t.Logf("Initial leader: %s", initialLeader)

	// Create a partition: majority (3 nodes) vs minority (2 nodes)
	majority := []string{"node-1", "node-2", "node-3"}
	minority := []string{"node-4", "node-5"}

	t.Logf("Creating network partition: majority=%v, minority=%v", majority, minority)
	if err := manager.SimulatePartition(majority, minority); err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	// Wait for partition effects
	time.Sleep(2 * time.Second)

	// The majority partition should still have a leader
	currentLeader := manager.GetLeader()
	if currentLeader == "" {
		t.Error("No leader after partition (majority should maintain leadership)")
	}

	// Heal the partition
	t.Logf("Healing network partition")
	if err := manager.HealPartition(); err != nil {
		t.Fatalf("Failed to heal partition: %v", err)
	}

	// Wait for healing effects
	time.Sleep(2 * time.Second)

	// Should still have exactly one leader
	finalLeader := manager.GetLeader()
	if finalLeader == "" {
		t.Error("No leader after partition healing")
	}

	// All nodes should be accessible again
	status := manager.GetClusterStatus()
	if len(status) != 5 {
		t.Errorf("Expected 5 nodes after healing, got %d", len(status))
	}

	t.Logf("Network partition test completed successfully")
}

// TestLogReplicationConsistency tests log replication across cluster
func TestLogReplicationConsistency(t *testing.T) {
	tempDir := "test_replication_data"
	defer os.RemoveAll(tempDir)

	// Create 3-node cluster
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7501, GrpcPort: 9501},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7502, GrpcPort: 9502},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7503, GrpcPort: 9503},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	logger := log.New(os.Stdout, "[TEST-REPLICATION] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for leader
	leaderID, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		t.Fatalf("No leader elected: %v", err)
	}

	// Get the leader node
	leader, exists := manager.GetNode(leaderID)
	if !exists {
		t.Fatalf("Leader node %s not found", leaderID)
	}

	// Append some entries to test replication
	for i := 0; i < 5; i++ {
		data, err := CreatePutOperation(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
		if err != nil {
			t.Fatalf("Failed to create operation: %v", err)
		}

		_, err = leader.AppendEntry("PUT", data)
		if err != nil {
			t.Fatalf("Failed to append entry: %v", err)
		}
	}

	// Wait for replication
	time.Sleep(2 * time.Second)

	// Check that all nodes have the same log length
	nodes := manager.GetNodes()
	var logLengths []int
	
	for nodeID, node := range nodes {
		node.mu.RLock()
		logLen := len(node.log)
		node.mu.RUnlock()
		
		logLengths = append(logLengths, logLen)
		t.Logf("Node %s has %d log entries", nodeID, logLen)
	}

	// Verify all nodes have the same log length
	firstLength := logLengths[0]
	for i, length := range logLengths {
		if length != firstLength {
			t.Errorf("Log length mismatch: node %d has %d entries, expected %d", 
				i, length, firstLength)
		}
	}

	t.Logf("Log replication consistency test completed successfully")
}

// TestClusterRecovery tests cluster recovery after multiple failures
func TestClusterRecovery(t *testing.T) {
	tempDir := "test_recovery_data"
	defer os.RemoveAll(tempDir)

	// Create 5-node cluster
	config := &ClusterConfig{
		Nodes: []*NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7601, GrpcPort: 9601},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7602, GrpcPort: 9602},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7603, GrpcPort: 9603},
			{NodeID: "node-4", Address: "localhost", RaftPort: 7604, GrpcPort: 9604},
			{NodeID: "node-5", Address: "localhost", RaftPort: 7605, GrpcPort: 9605},
		},
		ElectionTimeout:  150 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         tempDir,
	}

	logger := log.New(os.Stdout, "[TEST-RECOVERY] ", log.LstdFlags)
	manager := NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap cluster: %v", err)
	}
	defer manager.Stop()

	// Wait for initial leader
	initialLeader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		t.Fatalf("No initial leader elected: %v", err)
	}
	t.Logf("Initial leader: %s", initialLeader)

	// Simulate failure of 2 nodes (but maintain majority)
	t.Logf("Simulating failure of 2 nodes")
	if err := manager.RemoveNode("node-4"); err != nil {
		t.Fatalf("Failed to remove node-4: %v", err)
	}
	if err := manager.RemoveNode("node-5"); err != nil {
		t.Fatalf("Failed to remove node-5: %v", err)
	}

	// Wait and verify cluster still functions
	time.Sleep(2 * time.Second)
	
	// Should still have a leader (3 nodes remaining = majority)
	currentLeader := manager.GetLeader()
	if currentLeader == "" {
		t.Error("No leader after 2 node failures")
	}

	// Check remaining nodes
	status := manager.GetClusterStatus()
	if len(status) != 3 {
		t.Errorf("Expected 3 nodes after failures, got %d", len(status))
	}

	// Add nodes back to recover
	t.Logf("Recovering cluster by adding nodes back")
	
	node4 := &NodeConfig{
		NodeID: "node-4", Address: "localhost", RaftPort: 7604, GrpcPort: 9604,
	}
	if err := manager.AddNode(node4); err != nil {
		t.Fatalf("Failed to re-add node-4: %v", err)
	}

	node5 := &NodeConfig{
		NodeID: "node-5", Address: "localhost", RaftPort: 7605, GrpcPort: 9605,
	}
	if err := manager.AddNode(node5); err != nil {
		t.Fatalf("Failed to re-add node-5: %v", err)
	}

	// Wait for recovery
	time.Sleep(2 * time.Second)

	// Verify full recovery
	finalStatus := manager.GetClusterStatus()
	if len(finalStatus) != 5 {
		t.Errorf("Expected 5 nodes after recovery, got %d", len(finalStatus))
	}

	if !manager.IsHealthy() {
		t.Error("Cluster should be healthy after recovery")
	}

	t.Logf("Cluster recovery test completed successfully")
}