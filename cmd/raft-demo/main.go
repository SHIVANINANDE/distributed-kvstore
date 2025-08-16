package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"distributed-kvstore/internal/consensus"
)

func main() {
	fmt.Println("Raft Consensus Demo")
	fmt.Println("===================")

	// Create a simple Raft node for demonstration
	config := consensus.Config{
		NodeID:           "demo-node-1",
		Address:          "localhost",
		RaftPort:         7001,
		GrpcPort:         9001,
		ElectionTimeout:  200 * time.Millisecond,
		HeartbeatTimeout: 50 * time.Millisecond,
		Logger:           log.New(os.Stdout, "[DEMO] ", log.LstdFlags),
	}

	fmt.Printf("Creating Raft node: %s\n", config.NodeID)
	node, err := consensus.NewRaftNode(config)
	if err != nil {
		log.Fatalf("Failed to create Raft node: %v", err)
	}

	fmt.Printf("Starting Raft node...\n")
	node.Start()

	// Monitor the node state for a few seconds
	fmt.Println("\nMonitoring node state changes:")
	for i := 0; i < 20; i++ {
		state, term, isLeader := node.GetState()
		fmt.Printf("Time %2ds: State=%s, Term=%d, IsLeader=%t\n", 
			i, state, term, isLeader)
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\nStopping Raft node...")
	node.Stop()
	fmt.Println("Demo completed!")
}