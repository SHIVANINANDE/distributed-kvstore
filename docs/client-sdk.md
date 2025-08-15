# Client SDK Documentation

This document provides comprehensive documentation for the distributed key-value store Go client SDK.

## Overview

The client SDK provides a high-level, feature-rich interface for interacting with the distributed key-value store. It includes connection pooling, automatic retry logic, and comprehensive error handling.

## Installation

```bash
go get distributed-kvstore/pkg/client
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"

    "distributed-kvstore/pkg/client"
)

func main() {
    // Create client with default configuration
    kvClient, err := client.NewClient(nil)
    if err != nil {
        log.Fatal(err)
    }
    defer kvClient.Close()

    ctx := context.Background()

    // Store a value
    err = kvClient.Put(ctx, "user:123", []byte("John Doe"))
    if err != nil {
        log.Fatal(err)
    }

    // Retrieve a value
    result, err := kvClient.Get(ctx, "user:123")
    if err != nil {
        log.Fatal(err)
    }

    if result.Found {
        log.Printf("Found: %s", string(result.Value))
    } else {
        log.Println("Key not found")
    }
}
```

## Configuration

### Config Structure

```go
type Config struct {
    // Server addresses (multiple for load balancing)
    Addresses []string
    
    // Connection pool settings
    MaxConnections    int
    MaxIdleTime       time.Duration
    ConnectionTimeout time.Duration
    RequestTimeout    time.Duration
    
    // Retry settings
    MaxRetries     int
    RetryDelay     time.Duration
    RetryBackoff   float64
    RetryableErrors []codes.Code
    
    // TLS settings (not yet implemented)
    TLSEnabled bool
    CertFile   string
    KeyFile    string
    CAFile     string
}
```

### Default Configuration

```go
config := client.DefaultConfig()
// Addresses: ["localhost:9090"]
// MaxConnections: 10
// MaxIdleTime: 5 minutes
// ConnectionTimeout: 10 seconds
// RequestTimeout: 30 seconds
// MaxRetries: 3
// RetryDelay: 100ms
// RetryBackoff: 2.0
```

### Custom Configuration

```go
config := &client.Config{
    Addresses:         []string{"server1:9090", "server2:9090"},
    MaxConnections:    20,
    MaxIdleTime:       10 * time.Minute,
    ConnectionTimeout: 5 * time.Second,
    RequestTimeout:    60 * time.Second,
    MaxRetries:        5,
    RetryDelay:        200 * time.Millisecond,
    RetryBackoff:      1.5,
}

kvClient, err := client.NewClient(config)
```

## Basic Operations

### Put (Store)

Store a key-value pair:

```go
// Simple put
err := kvClient.Put(ctx, "key", []byte("value"))

// Put with TTL (Time To Live)
err := kvClient.Put(ctx, "session:abc", []byte("token"), 
    client.WithTTL(3600)) // 1 hour TTL
```

### Get (Retrieve)

Retrieve a value by key:

```go
result, err := kvClient.Get(ctx, "key")
if err != nil {
    log.Fatal(err)
}

if result.Found {
    fmt.Printf("Value: %s\n", string(result.Value))
    fmt.Printf("Created: %v\n", result.CreatedAt)
    
    if result.HasTTL() {
        fmt.Printf("Expires: %v\n", result.ExpiresAt)
        fmt.Printf("TTL: %v\n", result.TTL())
    }
} else {
    fmt.Println("Key not found")
}
```

### Delete

Remove a key-value pair:

```go
result, err := kvClient.Delete(ctx, "key")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Key existed: %v\n", result.Existed)
```

### Exists

Check if a key exists:

```go
exists, err := kvClient.Exists(ctx, "key")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Key exists: %v\n", exists)
```

## Advanced Operations

### List

List key-value pairs with optional prefix and pagination:

```go
// List all keys with prefix
result, err := kvClient.List(ctx, "user:")
if err != nil {
    log.Fatal(err)
}

for _, item := range result.Items {
    fmt.Printf("%s => %s\n", item.Key, string(item.Value))
}

// List with limit and pagination
result, err := kvClient.List(ctx, "user:", 
    client.WithLimit(10),
    client.WithCursor("cursor-token"))

if result.HasMore {
    fmt.Printf("More results available, cursor: %s\n", result.NextCursor)
}
```

### List Keys

List only keys (more efficient than List for key-only operations):

```go
result, err := kvClient.ListKeys(ctx, "user:", client.WithLimit(100))
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d keys\n", result.Count())
for _, key := range result.Keys {
    fmt.Println(key)
}
```

## Batch Operations

### Batch Put

Store multiple key-value pairs efficiently:

```go
items := []*client.PutItem{
    {Key: "user:1", Value: []byte("Alice")},
    {Key: "user:2", Value: []byte("Bob")},
    {Key: "user:3", Value: []byte("Charlie"), TTLSeconds: 3600},
}

result, err := kvClient.BatchPut(ctx, items)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Success: %d, Errors: %d\n", result.SuccessCount, result.ErrorCount)
fmt.Printf("Success rate: %.1f%%\n", result.SuccessRate())

if result.HasErrors() {
    for _, batchErr := range result.Errors {
        fmt.Printf("Error for %s: %s\n", batchErr.Key, batchErr.Error)
    }
}
```

### Batch Get

Retrieve multiple values efficiently:

```go
keys := []string{"user:1", "user:2", "user:3", "nonexistent"}
results, err := kvClient.BatchGet(ctx, keys)
if err != nil {
    log.Fatal(err)
}

for _, result := range results {
    if result.Found {
        fmt.Printf("%s => %s\n", result.Key, string(result.Value))
    } else {
        fmt.Printf("%s => (not found)\n", result.Key)
    }
}
```

### Batch Delete

Remove multiple keys efficiently:

```go
keys := []string{"user:1", "user:2", "user:3"}
result, err := kvClient.BatchDelete(ctx, keys)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Deleted %d keys successfully\n", result.SuccessCount)
```

## Monitoring and Health

### Health Check

Check server health:

```go
result, err := kvClient.Health(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Healthy: %v\n", result.IsHealthy())
fmt.Printf("Status: %s\n", result.Status)
fmt.Printf("Uptime: %v\n", result.UptimeDuration())
fmt.Printf("Version: %s\n", result.Version)
```

### Statistics

Get server statistics:

```go
// Basic stats
result, err := kvClient.Stats(ctx, false)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Total Keys: %d\n", result.TotalKeys)
fmt.Printf("Total Size: %.2f MB\n", result.TotalSizeMB())
fmt.Printf("LSM Size: %d bytes\n", result.LSMSizeBytes())
fmt.Printf("VLog Size: %d bytes\n", result.VLogSizeBytes())

// Detailed stats
detailedResult, err := kvClient.Stats(ctx, true)
if err != nil {
    log.Fatal(err)
}

if value, exists := detailedResult.GetDetail("custom_metric"); exists {
    fmt.Printf("Custom metric: %s\n", value)
}
```

## Connection Pooling

The client automatically manages a pool of gRPC connections for optimal performance:

### Features

- **Automatic pooling**: Maintains a pool of connections to configured servers
- **Load balancing**: Distributes requests across available servers
- **Health monitoring**: Automatically detects and handles unhealthy connections
- **Idle cleanup**: Removes idle connections after MaxIdleTime
- **Connection limits**: Prevents connection exhaustion with MaxConnections

### Pool Statistics

```go
// Access pool statistics (if needed for monitoring)
// Note: This is typically not needed in application code
stats := kvClient.pool.Stats()
fmt.Printf("Pool: %s\n", stats.String())
```

## Retry Logic

The client includes sophisticated retry logic for handling transient failures:

### Retryable Errors

By default, the following gRPC error codes trigger retries:
- `Unavailable`: Server temporarily unavailable
- `DeadlineExceeded`: Request timeout
- `ResourceExhausted`: Server overloaded

### Retry Configuration

```go
config := client.DefaultConfig()
config.MaxRetries = 5                    // Maximum retry attempts
config.RetryDelay = 100 * time.Millisecond // Initial delay
config.RetryBackoff = 2.0                // Exponential backoff multiplier

// Custom retryable errors
config.RetryableErrors = []codes.Code{
    codes.Unavailable,
    codes.DeadlineExceeded,
    codes.ResourceExhausted,
    codes.Internal, // Add custom retryable error
}
```

### Retry Behavior

1. **Exponential backoff**: Delay increases exponentially with each retry
2. **Jitter**: Random variation to prevent thundering herd
3. **Context respect**: Respects context cancellation and timeouts
4. **Connection rotation**: Uses different connections for retries when possible

## Error Handling

### Error Types

The client returns different types of errors:

```go
result, err := kvClient.Get(ctx, "key")
if err != nil {
    // Check for specific error types
    if status.Code(err) == codes.NotFound {
        // Handle not found differently
    } else if status.Code(err) == codes.Unavailable {
        // Handle server unavailable
    } else {
        // Handle other errors
    }
}

// For operations that can partially succeed
batchResult, err := kvClient.BatchPut(ctx, items)
if err != nil {
    log.Fatal(err) // Complete failure
}

if batchResult.HasErrors() {
    // Some operations failed
    for _, batchErr := range batchResult.Errors {
        log.Printf("Failed to put %s: %s", batchErr.Key, batchErr.Error)
    }
}
```

### Best Practices

1. **Always check errors**: Never ignore error returns
2. **Use context timeouts**: Set appropriate timeouts for operations
3. **Handle partial failures**: Check batch operation results for individual errors
4. **Implement circuit breakers**: For high-availability applications
5. **Log appropriately**: Log errors with sufficient context

## Context and Timeouts

### Request Timeouts

```go
// Per-request timeout
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

result, err := kvClient.Get(ctx, "key")

// Global timeout (configured in client)
config := client.DefaultConfig()
config.RequestTimeout = 30 * time.Second
```

### Cancellation

```go
ctx, cancel := context.WithCancel(context.Background())

go func() {
    time.Sleep(5 * time.Second)
    cancel() // Cancel after 5 seconds
}()

// This will be cancelled if it takes longer than 5 seconds
result, err := kvClient.Get(ctx, "key")
if err == context.Canceled {
    fmt.Println("Operation was cancelled")
}
```

## Performance Considerations

### Connection Reuse

- The client maintains a pool of persistent connections
- Reusing the same client instance across requests is optimal
- Avoid creating new clients for each request

### Batch Operations

- Use batch operations for multiple keys to reduce network overhead
- Batch operations are significantly more efficient than individual operations

### TTL Usage

- Use TTL for temporary data to prevent storage bloat
- Consider TTL for cache-like use cases

### Memory Management

```go
// Always close the client when done
defer kvClient.Close()

// For long-running applications, monitor connection pool
// The client will automatically clean up idle connections
```

## Testing

### Unit Testing

```go
func TestMyFunction(t *testing.T) {
    // Use the integration test helpers for testing
    config := client.DefaultConfig()
    config.Addresses = []string{"localhost:9090"}
    
    kvClient, err := client.NewClient(config)
    if err != nil {
        t.Skip("Server not available for testing")
    }
    defer kvClient.Close()
    
    // Your test code here
}
```

### Mock Testing

For unit tests that don't require a real server, test the utility methods and configuration:

```go
func TestResultMethods(t *testing.T) {
    result := &client.GetResult{
        Found: true,
        Value: []byte("test"),
    }
    
    if result.String() != "test" {
        t.Errorf("Expected 'test', got %s", result.String())
    }
}
```

## CLI Tool (kvtool)

The SDK includes a comprehensive CLI tool for testing and administration:

```bash
# Install
go install ./cmd/kvtool

# Basic operations
kvtool put user:123 "John Doe"
kvtool get user:123
kvtool delete user:123

# Batch operations
kvtool batch-put user:1 "Alice" user:2 "Bob" user:3 "Charlie"
kvtool batch-get user:1 user:2 user:3

# JSON output
kvtool -json get user:123
kvtool -json stats detailed

# Benchmarking
kvtool benchmark 10000

# Health monitoring
kvtool health
kvtool stats detailed
```

## Examples

### Cache Implementation

```go
type Cache struct {
    client *client.Client
    ttl    int64
}

func NewCache(addresses []string, ttl time.Duration) (*Cache, error) {
    config := client.DefaultConfig()
    config.Addresses = addresses
    
    kvClient, err := client.NewClient(config)
    if err != nil {
        return nil, err
    }
    
    return &Cache{
        client: kvClient,
        ttl:    int64(ttl.Seconds()),
    }, nil
}

func (c *Cache) Set(key string, value []byte) error {
    return c.client.Put(context.Background(), key, value, 
        client.WithTTL(c.ttl))
}

func (c *Cache) Get(key string) ([]byte, bool, error) {
    result, err := c.client.Get(context.Background(), key)
    if err != nil {
        return nil, false, err
    }
    return result.Value, result.Found, nil
}

func (c *Cache) Delete(key string) error {
    _, err := c.client.Delete(context.Background(), key)
    return err
}

func (c *Cache) Close() error {
    return c.client.Close()
}
```

### Session Store

```go
type SessionStore struct {
    client *client.Client
    prefix string
}

func NewSessionStore(client *client.Client) *SessionStore {
    return &SessionStore{
        client: client,
        prefix: "session:",
    }
}

func (s *SessionStore) CreateSession(sessionID string, data []byte, ttl time.Duration) error {
    key := s.prefix + sessionID
    return s.client.Put(context.Background(), key, data, 
        client.WithTTL(int64(ttl.Seconds())))
}

func (s *SessionStore) GetSession(sessionID string) ([]byte, bool, error) {
    key := s.prefix + sessionID
    result, err := s.client.Get(context.Background(), key)
    if err != nil {
        return nil, false, err
    }
    
    if result.Found && result.IsExpired() {
        // Clean up expired session
        s.client.Delete(context.Background(), key)
        return nil, false, nil
    }
    
    return result.Value, result.Found, nil
}

func (s *SessionStore) DeleteSession(sessionID string) error {
    key := s.prefix + sessionID
    _, err := s.client.Delete(context.Background(), key)
    return err
}

func (s *SessionStore) ListActiveSessions() ([]string, error) {
    result, err := s.client.ListKeys(context.Background(), s.prefix)
    if err != nil {
        return nil, err
    }
    
    // Remove prefix from session IDs
    sessions := make([]string, len(result.Keys))
    for i, key := range result.Keys {
        sessions[i] = key[len(s.prefix):]
    }
    
    return sessions, nil
}
```

This SDK provides a robust, production-ready client for the distributed key-value store with comprehensive features for building scalable applications.