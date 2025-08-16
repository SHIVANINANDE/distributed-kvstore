package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"distributed-kvstore/internal/consensus"
)

var manager *consensus.ClusterManager

func main() {
	fmt.Println("🚀 Distributed KV Store - Multi-Node Cluster CLI")
	fmt.Println("===============================================")
	fmt.Println("Commands:")
	fmt.Println("  init <nodes>    - Initialize cluster with specified number of nodes (3 or 5)")
	fmt.Println("  status          - Show cluster status")
	fmt.Println("  add <nodeID>    - Add a new node to cluster")
	fmt.Println("  remove <nodeID> - Remove node from cluster")
	fmt.Println("  partition       - Simulate network partition")
	fmt.Println("  heal            - Heal network partition")
	fmt.Println("  leader          - Show current leader")
	fmt.Println("  stop            - Stop the cluster")
	fmt.Println("  help            - Show this help")
	fmt.Println("  exit            - Exit CLI")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	
	for {
		fmt.Print("cluster> ")
		if !scanner.Scan() {
			break
		}
		
		command := strings.TrimSpace(scanner.Text())
		if command == "" {
			continue
		}
		
		parts := strings.Fields(command)
		cmd := parts[0]
		
		switch cmd {
		case "init":
			handleInit(parts)
		case "status":
			handleStatus()
		case "add":
			handleAdd(parts)
		case "remove":
			handleRemove(parts)
		case "partition":
			handlePartition()
		case "heal":
			handleHeal()
		case "leader":
			handleLeader()
		case "stop":
			handleStop()
		case "help":
			showHelp()
		case "exit":
			if manager != nil {
				manager.Stop()
			}
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
		}
	}
}

func handleInit(parts []string) {
	if manager != nil {
		fmt.Println("❌ Cluster already initialized. Stop it first.")
		return
	}
	
	nodeCount := 3 // default
	if len(parts) > 1 {
		if count, err := strconv.Atoi(parts[1]); err == nil {
			if count == 3 || count == 5 {
				nodeCount = count
			} else {
				fmt.Println("❌ Node count must be 3 or 5")
				return
			}
		}
	}
	
	fmt.Printf("🚀 Initializing %d-node cluster...\n", nodeCount)
	
	// Clean up any existing data
	os.RemoveAll("data")
	
	var nodes []*consensus.NodeConfig
	for i := 1; i <= nodeCount; i++ {
		nodes = append(nodes, &consensus.NodeConfig{
			NodeID:   fmt.Sprintf("node-%d", i),
			Address:  "localhost",
			RaftPort: int32(7000 + i),
			GrpcPort: int32(9000 + i),
		})
	}
	
	config := &consensus.ClusterConfig{
		Nodes:            nodes,
		ElectionTimeout:  200 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		DataDir:         "data/cluster",
	}
	
	logger := log.New(os.Stdout, "[CLUSTER] ", log.LstdFlags)
	manager = consensus.NewClusterManager(config, logger)
	
	if err := manager.Bootstrap(); err != nil {
		fmt.Printf("❌ Failed to bootstrap cluster: %v\n", err)
		manager = nil
		return
	}
	
	// Wait for leader election
	leader, err := manager.WaitForLeader(5 * time.Second)
	if err != nil {
		fmt.Printf("⚠️  No leader elected within timeout\n")
	} else {
		fmt.Printf("✅ Cluster initialized successfully! Leader: %s\n", leader)
	}
}

func handleStatus() {
	if manager == nil {
		fmt.Println("❌ No cluster initialized. Use 'init' command first.")
		return
	}
	
	fmt.Println("📊 Cluster Status:")
	fmt.Println("=================")
	
	status := manager.GetClusterStatus()
	isHealthy := manager.IsHealthy()
	leader := manager.GetLeader()
	
	fmt.Printf("Health: %s\n", getHealthIndicator(isHealthy))
	fmt.Printf("Leader: %s\n", getLeaderDisplay(leader))
	fmt.Printf("Total Nodes: %d\n", len(status))
	fmt.Println()
	fmt.Println("Node Details:")
	
	for nodeID, nodeStatus := range status {
		leaderBadge := ""
		if nodeStatus.IsLeader {
			leaderBadge = " 👑"
		}
		
		fmt.Printf("  %s: %s%s (Term: %d, Peers: %d)\n", 
			nodeID, nodeStatus.State, leaderBadge, nodeStatus.Term, nodeStatus.Peers)
	}
}

func handleAdd(parts []string) {
	if manager == nil {
		fmt.Println("❌ No cluster initialized. Use 'init' command first.")
		return
	}
	
	if len(parts) < 2 {
		fmt.Println("❌ Usage: add <nodeID>")
		return
	}
	
	nodeID := parts[1]
	
	// Find next available port
	status := manager.GetClusterStatus()
	nextPort := 7000 + len(status) + 1
	nextGrpcPort := 9000 + len(status) + 1
	
	newNode := &consensus.NodeConfig{
		NodeID:   nodeID,
		Address:  "localhost",
		RaftPort: int32(nextPort),
		GrpcPort: int32(nextGrpcPort),
	}
	
	fmt.Printf("➕ Adding node %s to cluster...\n", nodeID)
	
	if err := manager.AddNode(newNode); err != nil {
		fmt.Printf("❌ Failed to add node: %v\n", err)
		return
	}
	
	fmt.Printf("✅ Node %s added successfully!\n", nodeID)
}

func handleRemove(parts []string) {
	if manager == nil {
		fmt.Println("❌ No cluster initialized. Use 'init' command first.")
		return
	}
	
	if len(parts) < 2 {
		fmt.Println("❌ Usage: remove <nodeID>")
		return
	}
	
	nodeID := parts[1]
	
	fmt.Printf("➖ Removing node %s from cluster...\n", nodeID)
	
	if err := manager.RemoveNode(nodeID); err != nil {
		fmt.Printf("❌ Failed to remove node: %v\n", err)
		return
	}
	
	fmt.Printf("✅ Node %s removed successfully!\n", nodeID)
}

func handlePartition() {
	if manager == nil {
		fmt.Println("❌ No cluster initialized. Use 'init' command first.")
		return
	}
	
	status := manager.GetClusterStatus()
	if len(status) < 3 {
		fmt.Println("❌ Need at least 3 nodes for partition simulation")
		return
	}
	
	// Create majority and minority partitions
	var nodeIDs []string
	for nodeID := range status {
		nodeIDs = append(nodeIDs, nodeID)
	}
	
	splitPoint := len(nodeIDs) / 2 + 1
	majority := nodeIDs[:splitPoint]
	minority := nodeIDs[splitPoint:]
	
	fmt.Printf("🔀 Creating network partition:\n")
	fmt.Printf("   Majority: %v\n", majority)
	fmt.Printf("   Minority: %v\n", minority)
	
	if err := manager.SimulatePartition(majority, minority); err != nil {
		fmt.Printf("❌ Failed to create partition: %v\n", err)
		return
	}
	
	fmt.Printf("✅ Network partition created successfully!\n")
}

func handleHeal() {
	if manager == nil {
		fmt.Println("❌ No cluster initialized. Use 'init' command first.")
		return
	}
	
	fmt.Println("🔗 Healing network partition...")
	
	if err := manager.HealPartition(); err != nil {
		fmt.Printf("❌ Failed to heal partition: %v\n", err)
		return
	}
	
	fmt.Println("✅ Network partition healed successfully!")
}

func handleLeader() {
	if manager == nil {
		fmt.Println("❌ No cluster initialized. Use 'init' command first.")
		return
	}
	
	leader := manager.GetLeader()
	if leader == "" {
		fmt.Println("⚠️  No leader currently elected")
	} else {
		fmt.Printf("👑 Current leader: %s\n", leader)
	}
}

func handleStop() {
	if manager == nil {
		fmt.Println("❌ No cluster to stop.")
		return
	}
	
	fmt.Println("🛑 Stopping cluster...")
	manager.Stop()
	manager = nil
	
	// Clean up data
	os.RemoveAll("data")
	
	fmt.Println("✅ Cluster stopped successfully!")
}

func showHelp() {
	fmt.Println("Available Commands:")
	fmt.Println("==================")
	fmt.Println("  init <nodes>    - Initialize cluster with 3 or 5 nodes")
	fmt.Println("  status          - Show detailed cluster status")
	fmt.Println("  add <nodeID>    - Dynamically add a new node")
	fmt.Println("  remove <nodeID> - Remove a node from cluster")
	fmt.Println("  partition       - Simulate network partition")
	fmt.Println("  heal            - Heal network partition")
	fmt.Println("  leader          - Show current leader")
	fmt.Println("  stop            - Stop and cleanup cluster")
	fmt.Println("  help            - Show this help")
	fmt.Println("  exit            - Exit CLI")
	fmt.Println()
	fmt.Println("Example usage:")
	fmt.Println("  cluster> init 5")
	fmt.Println("  cluster> status")
	fmt.Println("  cluster> add node-6")
	fmt.Println("  cluster> partition")
	fmt.Println("  cluster> heal")
	fmt.Println("  cluster> stop")
}

func getHealthIndicator(healthy bool) string {
	if healthy {
		return "✅ Healthy"
	}
	return "⚠️  Degraded"
}

func getLeaderDisplay(leader string) string {
	if leader == "" {
		return "None"
	}
	return fmt.Sprintf("👑 %s", leader)
}