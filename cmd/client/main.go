package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"distributed-kvstore/proto/kvstore"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var (
		addr    = flag.String("addr", "localhost:9090", "Server address")
		timeout = flag.Duration("timeout", 30*time.Second, "Request timeout")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		return
	}

	conn, err := grpc.Dial(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := kvstore.NewKVStoreClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	command := args[0]
	switch command {
	case "put":
		if len(args) < 3 {
			fmt.Println("Usage: put <key> <value> [ttl_seconds]")
			os.Exit(1)
		}
		handlePut(ctx, client, args[1:])
	case "get":
		if len(args) < 2 {
			fmt.Println("Usage: get <key>")
			os.Exit(1)
		}
		handleGet(ctx, client, args[1])
	case "delete":
		if len(args) < 2 {
			fmt.Println("Usage: delete <key>")
			os.Exit(1)
		}
		handleDelete(ctx, client, args[1])
	case "exists":
		if len(args) < 2 {
			fmt.Println("Usage: exists <key>")
			os.Exit(1)
		}
		handleExists(ctx, client, args[1])
	case "list":
		prefix := ""
		if len(args) > 1 {
			prefix = args[1]
		}
		handleList(ctx, client, prefix)
	case "health":
		handleHealth(ctx, client)
	case "stats":
		handleStats(ctx, client)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handlePut(ctx context.Context, client kvstore.KVStoreClient, args []string) {
	req := &kvstore.PutRequest{
		Key:   args[0],
		Value: []byte(args[1]),
	}

	if len(args) > 2 {
		var ttl int64
		if _, err := fmt.Sscanf(args[2], "%d", &ttl); err == nil {
			req.TtlSeconds = ttl
		}
	}

	resp, err := client.Put(ctx, req)
	if err != nil {
		log.Fatalf("Put failed: %v", err)
	}

	if resp.Success {
		fmt.Println("OK")
	} else {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}
}

func handleGet(ctx context.Context, client kvstore.KVStoreClient, key string) {
	resp, err := client.Get(ctx, &kvstore.GetRequest{Key: key})
	if err != nil {
		log.Fatalf("Get failed: %v", err)
	}

	if resp.Found {
		fmt.Printf("%s\n", string(resp.Value))
	} else if resp.Error != "" {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	} else {
		fmt.Println("(nil)")
		os.Exit(1)
	}
}

func handleDelete(ctx context.Context, client kvstore.KVStoreClient, key string) {
	resp, err := client.Delete(ctx, &kvstore.DeleteRequest{Key: key})
	if err != nil {
		log.Fatalf("Delete failed: %v", err)
	}

	if resp.Success {
		if resp.Existed {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
	} else {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}
}

func handleExists(ctx context.Context, client kvstore.KVStoreClient, key string) {
	resp, err := client.Exists(ctx, &kvstore.ExistsRequest{Key: key})
	if err != nil {
		log.Fatalf("Exists failed: %v", err)
	}

	if resp.Error != "" {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	if resp.Exists {
		fmt.Println("1")
	} else {
		fmt.Println("0")
	}
}

func handleList(ctx context.Context, client kvstore.KVStoreClient, prefix string) {
	resp, err := client.List(ctx, &kvstore.ListRequest{
		Prefix: prefix,
		Limit:  100,
	})
	if err != nil {
		log.Fatalf("List failed: %v", err)
	}

	if resp.Error != "" {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	if len(resp.Items) == 0 {
		fmt.Println("(empty list)")
		return
	}

	for i, item := range resp.Items {
		fmt.Printf("%d) \"%s\" => \"%s\"\n", i+1, item.Key, string(item.Value))
	}

	if resp.HasMore {
		fmt.Printf("... and more (use pagination)\n")
	}
}

func handleHealth(ctx context.Context, client kvstore.KVStoreClient) {
	resp, err := client.Health(ctx, &kvstore.HealthRequest{})
	if err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Printf("Healthy: %v\n", resp.Healthy)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Uptime: %d seconds\n", resp.UptimeSeconds)
	fmt.Printf("Version: %s\n", resp.Version)
}

func handleStats(ctx context.Context, client kvstore.KVStoreClient) {
	resp, err := client.Stats(ctx, &kvstore.StatsRequest{
		IncludeDetails: true,
	})
	if err != nil {
		log.Fatalf("Stats failed: %v", err)
	}

	if resp.Error != "" {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Printf("Total Keys: %d\n", resp.TotalKeys)
	fmt.Printf("Total Size: %d bytes\n", resp.TotalSize)
	fmt.Printf("LSM Size: %d bytes\n", resp.LsmSize)
	fmt.Printf("VLog Size: %d bytes\n", resp.VlogSize)

	if len(resp.Details) > 0 {
		fmt.Println("\nDetailed Stats:")
		for key, value := range resp.Details {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
}

func printUsage() {
	fmt.Printf(`Distributed Key-Value Store Client

Usage:
  %s [options] <command> [args...]

Options:
  -addr string
        Server address (default "localhost:9090")
  -timeout duration
        Request timeout (default "30s")

Commands:
  put <key> <value> [ttl_seconds]
        Store a key-value pair with optional TTL
  
  get <key>
        Retrieve a value by key
  
  delete <key>
        Delete a key-value pair
  
  exists <key>
        Check if a key exists
  
  list [prefix]
        List all keys (or keys with prefix)
  
  health
        Check server health
  
  stats
        Get server statistics

Examples:
  # Store a value
  %s put user:123 "John Doe"
  
  # Store with TTL (1 hour)
  %s put session:abc "token123" 3600
  
  # Retrieve a value
  %s get user:123
  
  # Check existence
  %s exists user:123
  
  # List all user keys
  %s list user:
  
  # Delete a key
  %s delete user:123
  
  # Check server health
  %s health
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}