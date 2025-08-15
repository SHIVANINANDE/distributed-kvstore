package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"distributed-kvstore/pkg/client"
)

var (
	address     = flag.String("addr", "localhost:9090", "Server address")
	timeout     = flag.Duration("timeout", 30*time.Second, "Request timeout")
	maxRetries  = flag.Int("retries", 3, "Maximum number of retries")
	verbose     = flag.Bool("v", false, "Verbose output")
	jsonOutput  = flag.Bool("json", false, "Output in JSON format")
	connections = flag.Int("connections", 5, "Maximum connections in pool")
)

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		return
	}

	config := client.DefaultConfig()
	config.Addresses = []string{*address}
	config.RequestTimeout = *timeout
	config.MaxRetries = *maxRetries
	config.MaxConnections = *connections

	kvClient, err := client.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer kvClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	command := args[0]
	switch command {
	case "put":
		handlePut(ctx, kvClient, args[1:])
	case "get":
		handleGet(ctx, kvClient, args[1:])
	case "delete", "del":
		handleDelete(ctx, kvClient, args[1:])
	case "exists":
		handleExists(ctx, kvClient, args[1:])
	case "list":
		handleList(ctx, kvClient, args[1:])
	case "keys":
		handleKeys(ctx, kvClient, args[1:])
	case "batch-put":
		handleBatchPut(ctx, kvClient, args[1:])
	case "batch-get":
		handleBatchGet(ctx, kvClient, args[1:])
	case "batch-delete", "batch-del":
		handleBatchDelete(ctx, kvClient, args[1:])
	case "health":
		handleHealth(ctx, kvClient)
	case "stats":
		handleStats(ctx, kvClient, args[1:])
	case "benchmark":
		handleBenchmark(ctx, kvClient, args[1:])
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handlePut(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: put <key> <value> [ttl_seconds]")
		os.Exit(1)
	}

	key := args[0]
	value := []byte(args[1])
	
	var opts []client.PutOption
	if len(args) > 2 {
		ttl, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			fmt.Printf("Invalid TTL: %v\n", err)
			os.Exit(1)
		}
		opts = append(opts, client.WithTTL(ttl))
	}

	start := time.Now()
	err := kvClient.Put(ctx, key, value, opts...)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(map[string]interface{}{
			"success": true,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		if *verbose {
			fmt.Printf("OK (took %v)\n", duration)
		} else {
			fmt.Println("OK")
		}
	}
}

func handleGet(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: get <key>")
		os.Exit(1)
	}

	key := args[0]
	start := time.Now()
	result, err := kvClient.Get(ctx, key)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"found": false,
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		data := map[string]interface{}{
			"found": result.Found,
			"duration_ms": duration.Milliseconds(),
		}
		if result.Found {
			data["value"] = string(result.Value)
			data["created_at"] = result.CreatedAt.Unix()
			if result.HasTTL() {
				data["expires_at"] = result.ExpiresAt.Unix()
				data["ttl_seconds"] = int64(result.TTL().Seconds())
			}
		}
		outputJSON(data)
	} else {
		if result.Found {
			fmt.Printf("%s", string(result.Value))
			if *verbose {
				fmt.Printf(" (created: %v", result.CreatedAt.Format(time.RFC3339))
				if result.HasTTL() {
					fmt.Printf(", expires: %v, ttl: %v", result.ExpiresAt.Format(time.RFC3339), result.TTL())
				}
				fmt.Printf(", took %v)", duration)
			}
			fmt.Println()
		} else {
			fmt.Println("(nil)")
			os.Exit(1)
		}
	}
}

func handleDelete(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: delete <key>")
		os.Exit(1)
	}

	key := args[0]
	start := time.Now()
	result, err := kvClient.Delete(ctx, key)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"success": false,
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(map[string]interface{}{
			"success": true,
			"existed": result.Existed,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		if result.Existed {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
		if *verbose {
			fmt.Printf("(took %v)\n", duration)
		}
	}
}

func handleExists(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: exists <key>")
		os.Exit(1)
	}

	key := args[0]
	start := time.Now()
	exists, err := kvClient.Exists(ctx, key)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(map[string]interface{}{
			"exists": exists,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		if exists {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
		if *verbose {
			fmt.Printf("(took %v)\n", duration)
		}
	}
}

func handleList(ctx context.Context, kvClient *client.Client, args []string) {
	prefix := ""
	limit := int32(100)
	
	if len(args) > 0 {
		prefix = args[0]
	}
	if len(args) > 1 {
		l, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Printf("Invalid limit: %v\n", err)
			os.Exit(1)
		}
		limit = int32(l)
	}

	start := time.Now()
	result, err := kvClient.List(ctx, prefix, client.WithLimit(limit))
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		items := make([]map[string]interface{}, len(result.Items))
		for i, item := range result.Items {
			itemData := map[string]interface{}{
				"key":   item.Key,
				"value": string(item.Value),
				"created_at": item.CreatedAt.Unix(),
			}
			if item.ExpiresAt.Unix() > 0 {
				itemData["expires_at"] = item.ExpiresAt.Unix()
			}
			items[i] = itemData
		}
		
		outputJSON(map[string]interface{}{
			"items": items,
			"count": result.Count(),
			"has_more": result.HasMore,
			"next_cursor": result.NextCursor,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		if result.IsEmpty() {
			fmt.Println("(empty list)")
			return
		}

		for i, item := range result.Items {
			fmt.Printf("%d) \"%s\" => \"%s\"", i+1, item.Key, string(item.Value))
			if *verbose {
				fmt.Printf(" (created: %v", item.CreatedAt.Format(time.RFC3339))
				if item.ExpiresAt.Unix() > 0 {
					fmt.Printf(", expires: %v", item.ExpiresAt.Format(time.RFC3339))
				}
				fmt.Printf(")")
			}
			fmt.Println()
		}

		if result.HasMore {
			fmt.Printf("... and more (next cursor: %s)\n", result.NextCursor)
		}
		
		if *verbose {
			fmt.Printf("(returned %d items, took %v)\n", result.Count(), duration)
		}
	}
}

func handleKeys(ctx context.Context, kvClient *client.Client, args []string) {
	prefix := ""
	limit := int32(100)
	
	if len(args) > 0 {
		prefix = args[0]
	}
	if len(args) > 1 {
		l, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Printf("Invalid limit: %v\n", err)
			os.Exit(1)
		}
		limit = int32(l)
	}

	start := time.Now()
	result, err := kvClient.ListKeys(ctx, prefix, client.WithLimit(limit))
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(map[string]interface{}{
			"keys": result.Keys,
			"count": result.Count(),
			"has_more": result.HasMore,
			"next_cursor": result.NextCursor,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		if result.IsEmpty() {
			fmt.Println("(empty list)")
			return
		}

		for i, key := range result.Keys {
			fmt.Printf("%d) %s\n", i+1, key)
		}

		if result.HasMore {
			fmt.Printf("... and more (next cursor: %s)\n", result.NextCursor)
		}
		
		if *verbose {
			fmt.Printf("(returned %d keys, took %v)\n", result.Count(), duration)
		}
	}
}

func handleBatchPut(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 2 || len(args)%2 != 0 {
		fmt.Println("Usage: batch-put <key1> <value1> [key2] [value2] ...")
		os.Exit(1)
	}

	var items []*client.PutItem
	for i := 0; i < len(args); i += 2 {
		items = append(items, &client.PutItem{
			Key:   args[i],
			Value: []byte(args[i+1]),
		})
	}

	start := time.Now()
	result, err := kvClient.BatchPut(ctx, items)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		errors := make([]map[string]string, len(result.Errors))
		for i, batchErr := range result.Errors {
			errors[i] = map[string]string{
				"key":   batchErr.Key,
				"error": batchErr.Error,
			}
		}
		
		outputJSON(map[string]interface{}{
			"success_count": result.SuccessCount,
			"error_count": result.ErrorCount,
			"total": result.TotalOperations(),
			"success_rate": result.SuccessRate(),
			"errors": errors,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		fmt.Printf("Success: %d, Errors: %d, Total: %d\n", 
			result.SuccessCount, result.ErrorCount, result.TotalOperations())
		
		if result.HasErrors() {
			fmt.Println("Errors:")
			for _, batchErr := range result.Errors {
				fmt.Printf("  %s: %s\n", batchErr.Key, batchErr.Error)
			}
		}
		
		if *verbose {
			fmt.Printf("(success rate: %.1f%%, took %v)\n", result.SuccessRate(), duration)
		}
	}
}

func handleBatchGet(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: batch-get <key1> [key2] [key3] ...")
		os.Exit(1)
	}

	start := time.Now()
	results, err := kvClient.BatchGet(ctx, args)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		items := make([]map[string]interface{}, len(results))
		for i, result := range results {
			itemData := map[string]interface{}{
				"key":   result.Key,
				"found": result.Found,
			}
			if result.Found {
				itemData["value"] = string(result.Value)
				itemData["created_at"] = result.CreatedAt.Unix()
				if result.HasTTL() {
					itemData["expires_at"] = result.ExpiresAt.Unix()
				}
			}
			items[i] = itemData
		}
		
		outputJSON(map[string]interface{}{
			"results": items,
			"total": len(results),
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		foundCount := 0
		for i, result := range results {
			fmt.Printf("%d) %s => ", i+1, result.Key)
			if result.Found {
				fmt.Printf("\"%s\"", string(result.Value))
				foundCount++
				if *verbose && result.HasTTL() {
					fmt.Printf(" (ttl: %v)", result.TTL())
				}
			} else {
				fmt.Printf("(nil)")
			}
			fmt.Println()
		}
		
		if *verbose {
			fmt.Printf("(found %d/%d keys, took %v)\n", foundCount, len(results), duration)
		}
	}
}

func handleBatchDelete(ctx context.Context, kvClient *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: batch-delete <key1> [key2] [key3] ...")
		os.Exit(1)
	}

	start := time.Now()
	result, err := kvClient.BatchDelete(ctx, args)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		errors := make([]map[string]string, len(result.Errors))
		for i, batchErr := range result.Errors {
			errors[i] = map[string]string{
				"key":   batchErr.Key,
				"error": batchErr.Error,
			}
		}
		
		outputJSON(map[string]interface{}{
			"success_count": result.SuccessCount,
			"error_count": result.ErrorCount,
			"total": result.TotalOperations(),
			"success_rate": result.SuccessRate(),
			"errors": errors,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		fmt.Printf("Success: %d, Errors: %d, Total: %d\n", 
			result.SuccessCount, result.ErrorCount, result.TotalOperations())
		
		if result.HasErrors() {
			fmt.Println("Errors:")
			for _, batchErr := range result.Errors {
				fmt.Printf("  %s: %s\n", batchErr.Key, batchErr.Error)
			}
		}
		
		if *verbose {
			fmt.Printf("(success rate: %.1f%%, took %v)\n", result.SuccessRate(), duration)
		}
	}
}

func handleHealth(ctx context.Context, kvClient *client.Client) {
	start := time.Now()
	result, err := kvClient.Health(ctx)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(map[string]interface{}{
			"healthy": result.Healthy,
			"status": result.Status,
			"uptime_seconds": result.UptimeSeconds,
			"version": result.Version,
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		fmt.Printf("Healthy: %v\n", result.Healthy)
		fmt.Printf("Status: %s\n", result.Status)
		fmt.Printf("Uptime: %v\n", result.UptimeDuration())
		fmt.Printf("Version: %s\n", result.Version)
		
		if *verbose {
			fmt.Printf("(took %v)\n", duration)
		}
	}
}

func handleStats(ctx context.Context, kvClient *client.Client, args []string) {
	includeDetails := len(args) > 0 && (args[0] == "detailed" || args[0] == "details")
	
	start := time.Now()
	result, err := kvClient.Stats(ctx, includeDetails)
	duration := time.Since(start)

	if err != nil {
		if *jsonOutput {
			outputJSON(map[string]interface{}{
				"error": err.Error(),
				"duration_ms": duration.Milliseconds(),
			})
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	if *jsonOutput {
		data := map[string]interface{}{
			"total_keys": result.TotalKeys,
			"total_size": result.TotalSize,
			"lsm_size": result.LSMSize,
			"vlog_size": result.VLogSize,
			"duration_ms": duration.Milliseconds(),
		}
		if includeDetails && len(result.Details) > 0 {
			data["details"] = result.Details
		}
		outputJSON(data)
	} else {
		fmt.Printf("Total Keys: %d\n", result.TotalKeys)
		fmt.Printf("Total Size: %d bytes (%.2f MB)\n", result.TotalSize, result.TotalSizeMB())
		fmt.Printf("LSM Size: %d bytes\n", result.LSMSize)
		fmt.Printf("VLog Size: %d bytes\n", result.VLogSize)

		if includeDetails && len(result.Details) > 0 {
			fmt.Println("\nDetailed Stats:")
			for key, value := range result.Details {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}
		
		if *verbose {
			fmt.Printf("(took %v)\n", duration)
		}
	}
}

func handleBenchmark(ctx context.Context, kvClient *client.Client, args []string) {
	operations := 1000
	keySize := 10
	valueSize := 100
	
	if len(args) > 0 {
		ops, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Printf("Invalid operations count: %v\n", err)
			os.Exit(1)
		}
		operations = ops
	}

	fmt.Printf("Running benchmark: %d operations\n", operations)
	
	// Generate test data
	var items []*client.PutItem
	for i := 0; i < operations; i++ {
		key := fmt.Sprintf("bench:key:%0*d", keySize-10, i)
		value := strings.Repeat("x", valueSize)
		items = append(items, &client.PutItem{
			Key:   key,
			Value: []byte(value),
		})
	}

	// Benchmark PUT operations
	fmt.Println("Benchmarking PUT operations...")
	start := time.Now()
	result, err := kvClient.BatchPut(ctx, items)
	putDuration := time.Since(start)

	if err != nil {
		fmt.Printf("PUT benchmark failed: %v\n", err)
		os.Exit(1)
	}

	putOpsPerSec := float64(operations) / putDuration.Seconds()
	fmt.Printf("PUT: %d ops in %v (%.0f ops/sec, %.1f%% success)\n",
		operations, putDuration, putOpsPerSec, result.SuccessRate())

	// Benchmark GET operations
	fmt.Println("Benchmarking GET operations...")
	keys := make([]string, operations)
	for i, item := range items {
		keys[i] = item.Key
	}

	start = time.Now()
	getResults, err := kvClient.BatchGet(ctx, keys)
	getDuration := time.Since(start)

	if err != nil {
		fmt.Printf("GET benchmark failed: %v\n", err)
		os.Exit(1)
	}

	foundCount := 0
	for _, getResult := range getResults {
		if getResult.Found {
			foundCount++
		}
	}

	getOpsPerSec := float64(operations) / getDuration.Seconds()
	successRate := float64(foundCount) / float64(operations) * 100
	fmt.Printf("GET: %d ops in %v (%.0f ops/sec, %.1f%% found)\n",
		operations, getDuration, getOpsPerSec, successRate)

	// Summary
	totalDuration := putDuration + getDuration
	totalOpsPerSec := float64(operations*2) / totalDuration.Seconds()
	fmt.Printf("\nTotal: %d ops in %v (%.0f ops/sec)\n",
		operations*2, totalDuration, totalOpsPerSec)
}

func outputJSON(data interface{}) {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}

func printUsage() {
	fmt.Printf(`KV Tool - Distributed Key-Value Store Testing CLI

Usage:
  %s [options] <command> [args...]

Options:
  -addr string
        Server address (default "localhost:9090")
  -timeout duration
        Request timeout (default "30s")
  -retries int
        Maximum number of retries (default "3")
  -connections int
        Maximum connections in pool (default "5")
  -v    Verbose output
  -json Output in JSON format

Commands:
  put <key> <value> [ttl_seconds]
        Store a key-value pair with optional TTL
  
  get <key>
        Retrieve a value by key
  
  delete <key>
        Delete a key-value pair
  
  exists <key>
        Check if a key exists
  
  list [prefix] [limit]
        List key-value pairs (optionally with prefix and limit)
  
  keys [prefix] [limit]
        List keys only (optionally with prefix and limit)
  
  batch-put <key1> <value1> [key2] [value2] ...
        Store multiple key-value pairs
  
  batch-get <key1> [key2] [key3] ...
        Retrieve multiple values
  
  batch-delete <key1> [key2] [key3] ...
        Delete multiple keys
  
  health
        Check server health
  
  stats [detailed]
        Get server statistics (optionally detailed)
  
  benchmark [operations]
        Run performance benchmark (default 1000 operations)

Examples:
  # Basic operations
  %s put user:123 "John Doe"
  %s get user:123
  %s delete user:123
  
  # With TTL (1 hour)
  %s put session:abc "token123" 3600
  
  # Batch operations
  %s batch-put user:1 "Alice" user:2 "Bob" user:3 "Charlie"
  %s batch-get user:1 user:2 user:3
  
  # List operations
  %s list user: 10
  %s keys session: 5
  
  # JSON output
  %s -json get user:123
  %s -json stats detailed
  
  # Benchmark
  %s benchmark 5000
  
  # Verbose output
  %s -v put test:key "test value"
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}