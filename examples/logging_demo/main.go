package main

import (
	"context"
	"fmt"
	"time"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
)

func main() {
	fmt.Println("=== Distributed Key-Value Store - Structured Logging Demo ===")
	fmt.Println()

	// Demo different logging configurations
	demonstrateEnvironmentConfigs()
	fmt.Println()

	// Demo correlation IDs and context logging
	demonstrateCorrelationIDs()
	fmt.Println()

	// Demo specialized logging methods
	demonstrateSpecializedLogging()
	fmt.Println()

	// Demo request tracing simulation
	demonstrateRequestTracing()
}

func demonstrateEnvironmentConfigs() {
	fmt.Println("1. Environment-Specific Logging Configurations")
	fmt.Println("----------------------------------------------")

	environments := []struct {
		name   string
		config config.LoggingConfig
	}{
		{"Development", logging.DevelopmentLoggingConfig()},
		{"Production", logging.ProductionLoggingConfig()},
		{"High Volume", logging.HighVolumeLoggingConfig()},
	}

	for _, env := range environments {
		fmt.Printf("\n%s Environment:\n", env.name)
		fmt.Printf("- Level: %s\n", env.config.Level)
		fmt.Printf("- Format: %s\n", env.config.Format)
		fmt.Printf("- Request Tracing: %t\n", env.config.EnableRequestTracing)
		fmt.Printf("- Correlation IDs: %t\n", env.config.EnableCorrelationIDs)
		fmt.Printf("- Database Logging: %t\n", env.config.EnableDatabaseLogging)
		fmt.Printf("- Performance Logging: %t\n", env.config.EnablePerformanceLog)
		fmt.Printf("- Log Sampling: %d\n", env.config.LogSampling)

		logger := logging.NewLogger(&env.config)
		logger.Info("Sample log from "+env.name+" environment",
			"environment", env.name,
			"feature", "logging_demo",
		)
	}
}

func demonstrateCorrelationIDs() {
	fmt.Println("2. Correlation IDs and Context Logging")
	fmt.Println("-------------------------------------")

	config := logging.DevelopmentLoggingConfig()
	config.Format = "json" // Use JSON for clarity in demo
	logger := logging.NewLogger(&config)

	// Generate correlation and request IDs
	correlationID := logging.GenerateCorrelationID()
	requestID := logging.GenerateRequestID()

	fmt.Printf("Generated Correlation ID: %s\n", correlationID)
	fmt.Printf("Generated Request ID: %s\n", requestID)

	// Create context with IDs
	ctx := context.Background()
	ctx = logging.CreateContextWithIDs(ctx, correlationID, requestID)
	ctx = context.WithValue(ctx, logging.ServiceKey, "demo-service")

	// Log with context - notice how correlation/request IDs are automatically included
	logger.InfoContext(ctx, "User authentication started", "user_id", "user123")
	logger.DebugContext(ctx, "Database query executed", "query", "SELECT * FROM users", "duration_ms", 15)
	logger.InfoContext(ctx, "User authentication completed", "user_id", "user123", "success", true)
}

func demonstrateSpecializedLogging() {
	fmt.Println("\n3. Specialized Logging Methods")
	fmt.Println("-----------------------------")

	config := logging.ProductionLoggingConfig()
	logger := logging.NewLogger(&config)
	ctx := context.Background()
	ctx = logging.CreateContextWithIDs(ctx, logging.GenerateCorrelationID(), logging.GenerateRequestID())

	// Database operation logging
	fmt.Println("\nDatabase Operations:")
	logger.DatabaseOperation(ctx, "put", "user:123", 5*time.Millisecond, nil)
	logger.DatabaseOperation(ctx, "get", "user:456", 2*time.Millisecond, fmt.Errorf("key not found"))

	// Cluster event logging  
	fmt.Println("\nCluster Events:")
	logger.ClusterEvent(ctx, "node_joined", "node2", map[string]interface{}{
		"address":    "10.0.0.2:7000",
		"region":     "us-west-2",
		"datacenter": "west-2a",
	})

	logger.ClusterEvent(ctx, "leader_elected", "node1", map[string]interface{}{
		"term":      5,
		"log_index": 1250,
	})

	// Security event logging
	fmt.Println("\nSecurity Events:")
	logger.SecurityEvent(ctx, "authentication_failure", "192.168.1.100", "medium", map[string]interface{}{
		"username":      "admin",
		"failure_count": 2,
		"source":        "web_ui",
	})

	logger.SecurityEvent(ctx, "unauthorized_access", "192.168.1.100", "high", map[string]interface{}{
		"endpoint":   "/admin/users",
		"user_agent": "curl/7.68.0",
		"blocked":    true,
	})

	// Performance metrics logging
	fmt.Println("\nPerformance Metrics:")
	logger.Performance(ctx, "http_request_duration", 125.5, "milliseconds", map[string]string{
		"method":   "GET",
		"endpoint": "/api/v1/kv/user:123",
		"status":   "200",
	})

	logger.Performance(ctx, "database_connection_pool", 15, "active_connections", map[string]string{
		"pool":     "primary",
		"database": "badger",
	})
}

func demonstrateRequestTracing() {
	fmt.Println("\n4. Request Tracing Simulation")
	fmt.Println("----------------------------")

	config := logging.DevelopmentLoggingConfig()
	config.Format = "json"
	logger := logging.NewLogger(&config)

	// Simulate an HTTP request with correlation ID middleware
	simulateHTTPRequest(logger, "GET", "/api/v1/kv/user:123", "user123")
	fmt.Println()
	simulateHTTPRequest(logger, "POST", "/api/v1/kv/batch/put", "user456")
}

func simulateHTTPRequest(logger *logging.Logger, method, path, userID string) {
	// Create a new context for the request
	ctx := context.Background()
	
	// Generate correlation and request IDs (normally done by middleware)
	correlationID := logging.GenerateCorrelationID()
	requestID := logging.GenerateRequestID()
	
	ctx = logging.CreateContextWithIDs(ctx, correlationID, requestID)
	ctx = context.WithValue(ctx, logging.ServiceKey, "kvstore-api")
	ctx = context.WithValue(ctx, logging.UserIDKey, userID)

	start := time.Now()

	// Log request start
	logger.RequestStart(ctx, method, path, "kvstore-client/1.0.0")

	// Simulate some processing
	time.Sleep(10 * time.Millisecond)

	// Log database operations
	logger.DatabaseOperation(ctx, "get", "user:"+userID, 3*time.Millisecond, nil)
	
	if method == "POST" {
		logger.DatabaseOperation(ctx, "batch_put", "multiple_keys", 8*time.Millisecond, nil)
	}

	// Simulate response
	statusCode := 200
	if userID == "user456" && method == "POST" {
		statusCode = 201 // Created
	}

	duration := time.Since(start)
	logger.RequestEnd(ctx, method, path, statusCode, duration, 1024)

	fmt.Printf("Completed %s %s (correlation_id: %s, duration: %v)\n", 
		method, path, correlationID, duration)
}