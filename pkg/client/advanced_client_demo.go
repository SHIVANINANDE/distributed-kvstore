package client

import (
	"context"
	"fmt"
	"log"
	"time"
)

// DemoAdvancedClient demonstrates the advanced client features
func DemoAdvancedClient() {
	// Create advanced client configuration
	config := AdvancedClientConfig{
		InitialNodes:     []string{"localhost:9090", "localhost:9091", "localhost:9092"},
		ConnectionTimeout: 5 * time.Second,
		RequestTimeout:   10 * time.Second,
		
		// Load balancing
		LoadBalancingPolicy: LoadBalancingRoundRobin,
		MaxConnections:      10,
		
		// Retry configuration
		MaxRetries:     3,
		BaseRetryDelay: 100 * time.Millisecond,
		MaxRetryDelay:  30 * time.Second,
		RetryJitter:    true,
		
		// Circuit breaker
		CircuitBreakerEnabled: true,
		FailureThreshold:      5,
		RecoveryTimeout:       30 * time.Second,
		HalfOpenMaxRequests:   3,
		
		// Consistency settings
		DefaultConsistency:    ConsistencyLinearizable,
		ReadYourWritesEnabled: true,
		StaleReadThreshold:    5 * time.Second,
		
		// Health checking
		HealthCheckInterval:      30 * time.Second,
		NodeFailureDetectionTime: 10 * time.Second,
		
		// Metrics
		EnableMetrics:             true,
		MetricsCollectionInterval: 1 * time.Minute,
	}
	
	// Create logger
	logger := log.New(log.Writer(), "[DEMO] ", log.LstdFlags)
	
	// Create advanced client
	client := NewAdvancedClient(config, logger)
	
	ctx := context.Background()
	
	// Connect to cluster
	fmt.Println("=== Connecting to Cluster ===")
	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer client.Disconnect()
	
	// Demonstrate different consistency levels
	demonstrateConsistencyLevels(ctx, client)
	
	// Demonstrate retry and circuit breaker
	demonstrateResilienceFeatures(ctx, client)
	
	// Show client statistics
	showClientStatistics(client)
}

// demonstrateConsistencyLevels shows different consistency levels in action
func demonstrateConsistencyLevels(ctx context.Context, client *AdvancedClient) {
	fmt.Println("\n=== Demonstrating Consistency Levels ===")
	
	key := "demo_key"
	value := []byte("demo_value")
	
	// 1. Strong consistency write
	fmt.Println("1. Performing linearizable write...")
	putResult, err := client.PutWithConsistency(ctx, key, value, ConsistencyLinearizable)
	if err != nil {
		fmt.Printf("Write failed: %v\n", err)
		return
	}
	fmt.Printf("Write successful, version: %d\n", putResult.Version)
	
	// 2. Linearizable read (strong consistency)
	fmt.Println("2. Performing linearizable read...")
	getResult, err := client.GetWithConsistency(ctx, key, ConsistencyLinearizable)
	if err != nil {
		fmt.Printf("Linearizable read failed: %v\n", err)
		return
	}
	fmt.Printf("Linearizable read: found=%t, value=%s\n", getResult.Found, string(getResult.Value))
	
	// 3. Eventual consistency read
	fmt.Println("3. Performing eventual consistency read...")
	getResult, err = client.GetWithConsistency(ctx, key, ConsistencyEventual)
	if err != nil {
		fmt.Printf("Eventual read failed: %v\n", err)
		return
	}
	fmt.Printf("Eventual read: found=%t, value=%s\n", getResult.Found, string(getResult.Value))
	
	// 4. Bounded staleness read
	fmt.Println("4. Performing bounded staleness read...")
	getResult, err = client.GetWithConsistency(ctx, key, ConsistencyBounded)
	if err != nil {
		fmt.Printf("Bounded read failed: %v\n", err)
		return
	}
	fmt.Printf("Bounded read: found=%t, value=%s\n", getResult.Found, string(getResult.Value))
	
	// 5. Read-your-writes consistency
	fmt.Println("5. Performing read-your-writes read...")
	getResult, err = client.GetWithConsistency(ctx, key, ConsistencyReadYourWrite)
	if err != nil {
		fmt.Printf("Read-your-writes read failed: %v\n", err)
		return
	}
	fmt.Printf("Read-your-writes read: found=%t, value=%s\n", getResult.Found, string(getResult.Value))
	
	// 6. Session consistency
	fmt.Println("6. Performing session consistency read...")
	getResult, err = client.GetWithConsistency(ctx, key, ConsistencySession)
	if err != nil {
		fmt.Printf("Session read failed: %v\n", err)
		return
	}
	fmt.Printf("Session read: found=%t, value=%s\n", getResult.Found, string(getResult.Value))
}

// demonstrateResilienceFeatures shows retry and circuit breaker features
func demonstrateResilienceFeatures(ctx context.Context, client *AdvancedClient) {
	fmt.Println("\n=== Demonstrating Resilience Features ===")
	
	// Operations with custom timeouts
	fmt.Println("1. Operation with custom timeout...")
	getResult, err := client.Get(ctx, "test_key", WithTimeout(2*time.Second))
	if err != nil {
		fmt.Printf("Operation with timeout: %v\n", err)
	} else {
		fmt.Printf("Operation successful: found=%t\n", getResult.Found)
	}
	
	// Operations with custom consistency
	fmt.Println("2. Operation with custom consistency...")
	getResult, err = client.Get(ctx, "test_key", WithConsistency(ConsistencyEventual))
	if err != nil {
		fmt.Printf("Operation with custom consistency: %v\n", err)
	} else {
		fmt.Printf("Operation successful: found=%t\n", getResult.Found)
	}
	
	// Operations with metadata
	fmt.Println("3. Operation with metadata...")
	putResult, err := client.Put(ctx, "metadata_key", []byte("metadata_value"), 
		WithMetadata("source", "demo"),
		WithMetadata("timestamp", fmt.Sprintf("%d", time.Now().Unix())))
	if err != nil {
		fmt.Printf("Operation with metadata: %v\n", err)
	} else {
		fmt.Printf("Operation successful: version=%d\n", putResult.Version)
	}
}

// showClientStatistics displays client statistics
func showClientStatistics(client *AdvancedClient) {
	fmt.Println("\n=== Client Statistics ===")
	
	stats := client.GetStats()
	
	fmt.Printf("Connected: %t\n", stats.IsConnected)
	fmt.Printf("Last Activity: %v\n", stats.LastActivity)
	
	// Connection pool stats
	fmt.Printf("\nConnection Pool:\n")
	fmt.Printf("  Total Connections: %d\n", stats.ConnectionPoolStats.TotalConnections)
	fmt.Printf("  Active Connections: %d\n", stats.ConnectionPoolStats.ActiveConnections)
	
	// Load balancer stats
	fmt.Printf("\nLoad Balancer:\n")
	fmt.Printf("  Policy: %s\n", stats.LoadBalancerStats.Policy)
	fmt.Printf("  Total Nodes: %d\n", stats.LoadBalancerStats.TotalNodes)
	fmt.Printf("  Healthy Nodes: %d\n", stats.LoadBalancerStats.HealthyNodes)
	
	// Circuit breaker stats
	fmt.Printf("\nCircuit Breaker:\n")
	fmt.Printf("  State: %s\n", stats.CircuitBreakerStats.State)
	fmt.Printf("  Failure Count: %d\n", stats.CircuitBreakerStats.FailureCount)
	fmt.Printf("  Total Requests: %d\n", stats.CircuitBreakerStats.TotalRequests)
	fmt.Printf("  Total Failures: %d\n", stats.CircuitBreakerStats.TotalFailures)
	
	// Consistency stats
	fmt.Printf("\nConsistency Manager:\n")
	fmt.Printf("  Linearizable Reads: %d\n", stats.ConsistencyStats.LinearizableReads)
	fmt.Printf("  Eventual Reads: %d\n", stats.ConsistencyStats.EventualReads)
	fmt.Printf("  Bounded Reads: %d\n", stats.ConsistencyStats.BoundedReads)
	fmt.Printf("  Session Reads: %d\n", stats.ConsistencyStats.SessionReads)
	fmt.Printf("  Read-Your-Write Reads: %d\n", stats.ConsistencyStats.ReadYourWriteReads)
	fmt.Printf("  Tracked Keys: %d\n", stats.ConsistencyStats.TrackedKeys)
	
	// Operation metrics
	fmt.Printf("\nOperation Metrics:\n")
	for opType, metrics := range stats.OperationMetrics {
		fmt.Printf("  %s:\n", opType)
		fmt.Printf("    Total Requests: %d\n", metrics.TotalRequests)
		fmt.Printf("    Successful Requests: %d\n", metrics.SuccessfulReqs)
		fmt.Printf("    Failed Requests: %d\n", metrics.FailedRequests)
		fmt.Printf("    Error Rate: %.2f%%\n", metrics.ErrorRate*100)
		if metrics.AverageLatency > 0 {
			fmt.Printf("    Average Latency: %v\n", metrics.AverageLatency)
			fmt.Printf("    Min Latency: %v\n", metrics.MinLatency)
			fmt.Printf("    Max Latency: %v\n", metrics.MaxLatency)
		}
	}
}

// DemoLoadBalancingPolicies demonstrates different load balancing policies
func DemoLoadBalancingPolicies() {
	fmt.Println("\n=== Demonstrating Load Balancing Policies ===")
	
	// Create load balancer with different policies
	policies := []LoadBalancingPolicy{
		LoadBalancingRoundRobin,
		LoadBalancingRandom,
		LoadBalancingLeastConn,
		LoadBalancingWeighted,
		LoadBalancingConsistent,
	}
	
	logger := log.New(log.Writer(), "[LB_DEMO] ", log.LstdFlags)
	
	for _, policy := range policies {
		fmt.Printf("\n--- Testing %s Policy ---\n", policy)
		
		lb := NewLoadBalancer(policy, logger)
		
		// Add some nodes
		nodes := []*NodeInfo{
			{ID: "node1", Address: "localhost", Port: 9090, IsHealthy: true, Weight: 1},
			{ID: "node2", Address: "localhost", Port: 9091, IsHealthy: true, Weight: 2},
			{ID: "node3", Address: "localhost", Port: 9092, IsHealthy: true, Weight: 3},
		}
		
		for _, node := range nodes {
			lb.AddNode(node)
		}
		
		// Test node selection
		fmt.Printf("Selecting nodes for different keys:\n")
		testKeys := []string{"key1", "key2", "key3", "key1", "key2"}
		
		for _, key := range testKeys {
			selectedNodes, err := lb.SelectNodes(key, 1)
			if err != nil {
				fmt.Printf("  %s: Error - %v\n", key, err)
			} else if len(selectedNodes) > 0 {
				fmt.Printf("  %s: %s\n", key, selectedNodes[0].ID)
			}
		}
		
		// Show load balancer stats
		stats := lb.GetStats()
		fmt.Printf("Stats: Total=%d, Healthy=%d\n", stats.TotalNodes, stats.HealthyNodes)
	}
}

// DemoCircuitBreakerStates demonstrates circuit breaker state transitions
func DemoCircuitBreakerStates() {
	fmt.Println("\n=== Demonstrating Circuit Breaker States ===")
	
	config := AdvancedClientConfig{
		CircuitBreakerEnabled: true,
		FailureThreshold:      3,
		RecoveryTimeout:       2 * time.Second,
		HalfOpenMaxRequests:   2,
	}
	
	logger := log.New(log.Writer(), "[CB_DEMO] ", log.LstdFlags)
	cb := NewCircuitBreaker(config, logger)
	
	fmt.Printf("Initial state: %s\n", cb.GetState())
	
	// Simulate failures to trigger circuit breaker
	fmt.Println("\nSimulating failures...")
	for i := 1; i <= 5; i++ {
		canExecute := cb.CanExecute()
		fmt.Printf("Attempt %d: CanExecute=%t, State=%s\n", i, canExecute, cb.GetState())
		
		if canExecute {
			cb.RecordFailure()
		}
	}
	
	// Wait for recovery timeout
	fmt.Println("\nWaiting for recovery timeout...")
	time.Sleep(3 * time.Second)
	
	// Test half-open state
	fmt.Println("Testing half-open state...")
	for i := 1; i <= 3; i++ {
		canExecute := cb.CanExecute()
		fmt.Printf("Recovery attempt %d: CanExecute=%t, State=%s\n", i, canExecute, cb.GetState())
		
		if canExecute {
			if i <= 2 {
				cb.RecordSuccess() // First two succeed
			} else {
				cb.RecordFailure() // Third fails
			}
		}
	}
	
	// Show final stats
	stats := cb.GetStats()
	fmt.Printf("\nFinal Stats:\n")
	fmt.Printf("  State: %s\n", stats.State)
	fmt.Printf("  Total Requests: %d\n", stats.TotalRequests)
	fmt.Printf("  Total Failures: %d\n", stats.TotalFailures)
	fmt.Printf("  Total Successes: %d\n", stats.TotalSuccesses)
	fmt.Printf("  State Changes: %d\n", stats.StateChanges)
}