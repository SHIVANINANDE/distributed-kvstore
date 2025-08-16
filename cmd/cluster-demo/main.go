package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"distributed-kvstore/internal/consensus"
)

func main() {
	fmt.Println("🚀 Multi-Node Cluster Demo")
	fmt.Println("===========================")
	
	// Clean up any existing data
	os.RemoveAll("data")
	defer os.RemoveAll("data")

	// Run all cluster scenarios
	runClusterInitialization()
	runDynamicMembership()
	runFailureRecovery()
	runNetworkPartitions()

	fmt.Println("\n🎉 All cluster scenarios completed successfully!")
}

func runClusterInitialization() {
	fmt.Println("\n📊 Scenario 1: Cluster Initialization")
	fmt.Println("=====================================")

	// Create 3-node cluster configuration
	config := &consensus.ClusterConfig{
		Nodes: []*consensus.NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7001, GrpcPort: 9001},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7002, GrpcPort: 9002},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7003, GrpcPort: 9003},
		},
		ElectionTimeout:  200 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         "data/cluster1",
	}

	logger := log.New(os.Stdout, "[INIT] ", log.LstdFlags)
	manager := consensus.NewClusterManager(config, logger)

	fmt.Printf("✅ Creating cluster with %d nodes\n", len(config.Nodes))
	
	// Bootstrap the cluster
	if err := manager.Bootstrap(); err != nil {
		fmt.Printf("❌ Failed to bootstrap cluster: %v\n", err)
		return
	}
	defer manager.Stop()

	// Wait for leader election
	fmt.Println("⏳ Waiting for leader election...")
	leader, err := manager.WaitForLeader(5 * time.Second)
	if err != nil {
		fmt.Printf("❌ No leader elected: %v\n", err)
		return
	}

	fmt.Printf("✅ Leader elected: %s\n", leader)

	// Display cluster status
	displayClusterStatus(manager)

	fmt.Println("✅ Cluster initialization completed successfully")
}

func runDynamicMembership() {
	fmt.Println("\n📊 Scenario 2: Dynamic Membership")
	fmt.Println("=================================")

	// Create initial 3-node cluster
	config := &consensus.ClusterConfig{
		Nodes: []*consensus.NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7101, GrpcPort: 9101},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7102, GrpcPort: 9102},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7103, GrpcPort: 9103},
		},
		ElectionTimeout:  200 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         "data/cluster2",
	}

	logger := log.New(os.Stdout, "[DYNAMIC] ", log.LstdFlags)
	manager := consensus.NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		fmt.Printf("❌ Failed to bootstrap cluster: %v\n", err)
		return
	}
	defer manager.Stop()

	// Wait for initial leader
	leader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		fmt.Printf("❌ No initial leader elected: %v\n", err)
		return
	}
	fmt.Printf("✅ Initial leader: %s\n", leader)

	fmt.Println("📈 Initial cluster state:")
	displayClusterStatus(manager)

	// Add a new node
	fmt.Println("\n➕ Adding new node to cluster...")
	newNode := &consensus.NodeConfig{
		NodeID:   "node-4",
		Address:  "localhost",
		RaftPort: 7104,
		GrpcPort: 9104,
	}

	if err := manager.AddNode(newNode); err != nil {
		fmt.Printf("❌ Failed to add node: %v\n", err)
		return
	}

	time.Sleep(1 * time.Second)
	fmt.Println("📈 Cluster state after adding node:")
	displayClusterStatus(manager)

	// Remove a node
	fmt.Println("\n➖ Removing node from cluster...")
	if err := manager.RemoveNode("node-4"); err != nil {
		fmt.Printf("❌ Failed to remove node: %v\n", err)
		return
	}

	time.Sleep(1 * time.Second)
	fmt.Println("📈 Cluster state after removing node:")
	displayClusterStatus(manager)

	fmt.Println("✅ Dynamic membership completed successfully")
}

func runFailureRecovery() {
	fmt.Println("\n📊 Scenario 3: Failure Recovery")
	fmt.Println("===============================")

	// Create 5-node cluster for better failure testing
	config := &consensus.ClusterConfig{
		Nodes: []*consensus.NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7201, GrpcPort: 9201},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7202, GrpcPort: 9202},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7203, GrpcPort: 9203},
			{NodeID: "node-4", Address: "localhost", RaftPort: 7204, GrpcPort: 9204},
			{NodeID: "node-5", Address: "localhost", RaftPort: 7205, GrpcPort: 9205},
		},
		ElectionTimeout:  200 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         "data/cluster3",
	}

	logger := log.New(os.Stdout, "[FAILURE] ", log.LstdFlags)
	manager := consensus.NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		fmt.Printf("❌ Failed to bootstrap cluster: %v\n", err)
		return
	}
	defer manager.Stop()

	// Wait for initial leader
	initialLeader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		fmt.Printf("❌ No initial leader elected: %v\n", err)
		return
	}
	fmt.Printf("✅ Initial leader: %s\n", initialLeader)

	fmt.Println("📈 Initial cluster state:")
	displayClusterStatus(manager)

	// Simulate leader failure
	fmt.Printf("\n⚠️  Simulating failure of leader %s...\n", initialLeader)
	if err := manager.RemoveNode(initialLeader); err != nil {
		fmt.Printf("❌ Failed to simulate leader failure: %v\n", err)
		return
	}

	// Wait for new leader election
	time.Sleep(2 * time.Second)
	newLeader := manager.GetLeader()
	if newLeader == "" {
		fmt.Println("❌ No new leader elected after failure")
	} else {
		fmt.Printf("✅ New leader elected: %s\n", newLeader)
	}

	fmt.Println("📈 Cluster state after leader failure:")
	displayClusterStatus(manager)

	// Simulate another node failure
	fmt.Println("\n⚠️  Simulating failure of another node...")
	remainingNodes := manager.GetNodes()
	var nodeToRemove string
	for nodeID := range remainingNodes {
		if nodeID != newLeader {
			nodeToRemove = nodeID
			break
		}
	}

	if nodeToRemove != "" {
		if err := manager.RemoveNode(nodeToRemove); err != nil {
			fmt.Printf("❌ Failed to simulate node failure: %v\n", err)
		} else {
			fmt.Printf("⚠️  Removed node: %s\n", nodeToRemove)
		}
	}

	time.Sleep(1 * time.Second)
	fmt.Println("📈 Cluster state after multiple failures:")
	displayClusterStatus(manager)

	// Check if cluster is still functional
	if manager.IsHealthy() {
		fmt.Println("✅ Cluster remained healthy despite failures")
	} else {
		fmt.Println("⚠️  Cluster health degraded but may still function")
	}

	fmt.Println("✅ Failure recovery scenario completed")
}

func runNetworkPartitions() {
	fmt.Println("\n📊 Scenario 4: Network Partitions")
	fmt.Println("=================================")

	// Create 5-node cluster for partition testing
	config := &consensus.ClusterConfig{
		Nodes: []*consensus.NodeConfig{
			{NodeID: "node-1", Address: "localhost", RaftPort: 7301, GrpcPort: 9301},
			{NodeID: "node-2", Address: "localhost", RaftPort: 7302, GrpcPort: 9302},
			{NodeID: "node-3", Address: "localhost", RaftPort: 7303, GrpcPort: 9303},
			{NodeID: "node-4", Address: "localhost", RaftPort: 7304, GrpcPort: 9304},
			{NodeID: "node-5", Address: "localhost", RaftPort: 7305, GrpcPort: 9305},
		},
		ElectionTimeout:  200 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         "data/cluster4",
	}

	logger := log.New(os.Stdout, "[PARTITION] ", log.LstdFlags)
	manager := consensus.NewClusterManager(config, logger)

	if err := manager.Bootstrap(); err != nil {
		fmt.Printf("❌ Failed to bootstrap cluster: %v\n", err)
		return
	}
	defer manager.Stop()

	// Wait for initial leader
	initialLeader, err := manager.WaitForLeader(3 * time.Second)
	if err != nil {
		fmt.Printf("❌ No initial leader elected: %v\n", err)
		return
	}
	fmt.Printf("✅ Initial leader: %s\n", initialLeader)

	fmt.Println("📈 Initial cluster state:")
	displayClusterStatus(manager)

	// Create network partition
	majority := []string{"node-1", "node-2", "node-3"}
	minority := []string{"node-4", "node-5"}

	fmt.Printf("\n🔀 Creating network partition:\n")
	fmt.Printf("   Majority partition: %v\n", majority)
	fmt.Printf("   Minority partition: %v\n", minority)

	if err := manager.SimulatePartition(majority, minority); err != nil {
		fmt.Printf("❌ Failed to create partition: %v\n", err)
		return
	}

	// Wait for partition effects
	time.Sleep(2 * time.Second)

	currentLeader := manager.GetLeader()
	if currentLeader == "" {
		fmt.Println("⚠️  No leader detected during partition")
	} else {
		fmt.Printf("✅ Leader during partition: %s\n", currentLeader)
	}

	fmt.Println("📈 Cluster state during partition:")
	displayClusterStatus(manager)

	// Heal the partition
	fmt.Println("\n🔗 Healing network partition...")
	if err := manager.HealPartition(); err != nil {
		fmt.Printf("❌ Failed to heal partition: %v\n", err)
		return
	}

	// Wait for healing effects
	time.Sleep(2 * time.Second)

	finalLeader := manager.GetLeader()
	if finalLeader == "" {
		fmt.Println("❌ No leader after partition healing")
	} else {
		fmt.Printf("✅ Leader after healing: %s\n", finalLeader)
	}

	fmt.Println("📈 Final cluster state after healing:")
	displayClusterStatus(manager)

	fmt.Println("✅ Network partition scenario completed")
}

func displayClusterStatus(manager *consensus.ClusterManager) {
	status := manager.GetClusterStatus()
	
	fmt.Printf("   Cluster Health: %s\n", getHealthStatus(manager.IsHealthy()))
	fmt.Printf("   Total Nodes: %d\n", len(status))
	fmt.Printf("   Current Leader: %s\n", getLeaderOrNone(manager.GetLeader()))
	
	fmt.Println("   Node Details:")
	for nodeID, nodeStatus := range status {
		leaderIndicator := ""
		if nodeStatus.IsLeader {
			leaderIndicator = " [LEADER]"
		}
		fmt.Printf("     - %s: %s%s (Term: %d, Peers: %d)\n", 
			nodeID, nodeStatus.State, leaderIndicator, nodeStatus.Term, nodeStatus.Peers)
	}
}

func getHealthStatus(healthy bool) string {
	if healthy {
		return "✅ Healthy"
	}
	return "⚠️  Degraded"
}

func getLeaderOrNone(leader string) string {
	if leader == "" {
		return "None"
	}
	return leader
}