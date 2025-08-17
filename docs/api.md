# KVStore API Documentation

## Overview

The KVStore API provides both gRPC and REST interfaces for interacting with the distributed key-value store. This document covers both protocols, authentication, error handling, and provides practical examples.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Authentication](#authentication)
3. [gRPC API](#grpc-api)
4. [REST API](#rest-api)
5. [Error Handling](#error-handling)
6. [Rate Limiting](#rate-limiting)
7. [Client Libraries](#client-libraries)
8. [Examples](#examples)

## Quick Start

### gRPC Client (Go)

```go
package main

import (
    "context"
    "log"
    
    "google.golang.org/grpc"
    pb "distributed-kvstore/proto/kvstore"
)

func main() {
    conn, err := grpc.Dial("localhost:9090", grpc.WithInsecure())
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    client := pb.NewKVStoreClient(conn)
    
    // Put a key-value pair
    _, err = client.Put(context.Background(), &pb.PutRequest{
        Key:   "hello",
        Value: []byte("world"),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Get the value
    resp, err := client.Get(context.Background(), &pb.GetRequest{
        Key: "hello",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Value: %s", resp.Value)
}
```

### REST Client (curl)

```bash
# Set a key-value pair
curl -X PUT "http://localhost:8080/v1/keys/hello" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{"value": "world"}'

# Get a value
curl -X GET "http://localhost:8080/v1/keys/hello" \
  -H "X-API-Key: your-api-key"

# Delete a key
curl -X DELETE "http://localhost:8080/v1/keys/hello" \
  -H "X-API-Key: your-api-key"
```

## Authentication

KVStore supports multiple authentication methods:

### 1. API Key Authentication

Include the API key in the `X-API-Key` header:

```bash
curl -H "X-API-Key: your-api-key-here" \
  "http://localhost:8080/v1/keys/mykey"
```

### 2. JWT Token Authentication

Include the JWT token in the `Authorization` header:

```bash
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  "http://localhost:8080/v1/keys/mykey"
```

### 3. Mutual TLS (mTLS)

For gRPC clients with client certificates:

```go
creds, err := credentials.LoadTLSCredentials(
    "client-cert.pem", 
    "client-key.pem", 
    "ca-cert.pem",
)
conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(creds))
```

## gRPC API

### Service Definition

```protobuf
service KVStore {
  // Basic CRUD operations
  rpc Put(PutRequest) returns (PutResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Exists(ExistsRequest) returns (ExistsResponse);
  
  // Advanced operations
  rpc List(ListRequest) returns (ListResponse);
  rpc ListKeys(ListKeysRequest) returns (ListKeysResponse);
  
  // Batch operations
  rpc BatchPut(BatchPutRequest) returns (BatchPutResponse);
  rpc BatchGet(BatchGetRequest) returns (BatchGetResponse);
  rpc BatchDelete(BatchDeleteRequest) returns (BatchDeleteResponse);
  
  // Storage management
  rpc Backup(BackupRequest) returns (BackupResponse);
  rpc Restore(RestoreRequest) returns (RestoreResponse);
  rpc Stats(StatsRequest) returns (StatsResponse);
  
  // Health and status
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Status(StatusRequest) returns (StatusResponse);
}
```

### Key Operations

#### Put Operation

```go
request := &pb.PutRequest{
    Key:        "user:123:profile",
    Value:      []byte(`{"name": "John Doe", "email": "john@example.com"}`),
    TtlSeconds: 3600, // 1 hour expiration
}

response, err := client.Put(ctx, request)
if err != nil {
    log.Fatal(err)
}
```

#### Get Operation

```go
request := &pb.GetRequest{
    Key: "user:123:profile",
}

response, err := client.Get(ctx, request)
if err != nil {
    log.Fatal(err)
}

if response.Found {
    fmt.Printf("Value: %s\n", response.Value)
    fmt.Printf("Created: %d\n", response.CreatedAt)
    fmt.Printf("Expires: %d\n", response.ExpiresAt)
}
```

#### List Operation

```go
request := &pb.ListRequest{
    Prefix: "user:",
    Limit:  100,
}

response, err := client.List(ctx, request)
if err != nil {
    log.Fatal(err)
}

for _, item := range response.Items {
    fmt.Printf("Key: %s, Value: %s\n", item.Key, item.Value)
}
```

#### Batch Operations

```go
// Batch Put
batchPutReq := &pb.BatchPutRequest{
    Items: []*pb.PutItem{
        {Key: "user:1", Value: []byte("John")},
        {Key: "user:2", Value: []byte("Jane")},
        {Key: "user:3", Value: []byte("Bob")},
    },
}

batchPutResp, err := client.BatchPut(ctx, batchPutReq)

// Batch Get
batchGetReq := &pb.BatchGetRequest{
    Keys: []string{"user:1", "user:2", "user:3"},
}

batchGetResp, err := client.BatchGet(ctx, batchGetReq)
```

## REST API

### Endpoints Overview

| Method | Endpoint | Description |
|--------|----------|-------------|
| `PUT` | `/v1/keys/{key}` | Create or update a key |
| `GET` | `/v1/keys/{key}` | Retrieve a key's value |
| `DELETE` | `/v1/keys/{key}` | Delete a key |
| `HEAD` | `/v1/keys/{key}` | Check if key exists |
| `GET` | `/v1/keys` | List keys with optional prefix |
| `POST` | `/v1/batch/put` | Batch create/update |
| `POST` | `/v1/batch/get` | Batch retrieve |
| `POST` | `/v1/batch/delete` | Batch delete |
| `POST` | `/v1/transaction` | Execute transaction |
| `GET` | `/v1/watch/{prefix}` | Stream key changes |

### Key Operations

#### Set a Key

```bash
curl -X PUT "http://localhost:8080/v1/keys/user:123" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "value": "John Doe",
    "ttl_seconds": 3600,
    "metadata": {
      "content_type": "text/plain",
      "source": "user_service"
    }
  }'
```

Response:
```json
{
  "success": true,
  "version": 1
}
```

#### Get a Key

```bash
curl -X GET "http://localhost:8080/v1/keys/user:123?include_metadata=true" \
  -H "X-API-Key: your-api-key"
```

Response:
```json
{
  "found": true,
  "value": "John Doe",
  "metadata": {
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z",
    "expires_at": "2024-01-15T11:30:00Z",
    "version": 1,
    "tags": {
      "content_type": "text/plain",
      "source": "user_service"
    }
  }
}
```

#### List Keys

```bash
curl -X GET "http://localhost:8080/v1/keys?prefix=user:&limit=10&include_values=true" \
  -H "X-API-Key: your-api-key"
```

Response:
```json
{
  "items": [
    {
      "key": "user:123",
      "value": "John Doe",
      "metadata": {
        "created_at": "2024-01-15T10:30:00Z",
        "version": 1
      }
    },
    {
      "key": "user:456",
      "value": "Jane Smith",
      "metadata": {
        "created_at": "2024-01-15T11:15:00Z",
        "version": 1
      }
    }
  ],
  "has_more": false,
  "next_cursor": null
}
```

#### Batch Operations

```bash
# Batch Put
curl -X POST "http://localhost:8080/v1/batch/put" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "items": [
      {
        "key": "user:1",
        "value": "Alice",
        "ttl_seconds": 3600
      },
      {
        "key": "user:2",
        "value": "Bob"
      },
      {
        "key": "user:3",
        "value": "Charlie",
        "metadata": {
          "source": "batch_import"
        }
      }
    ]
  }'

# Batch Get
curl -X POST "http://localhost:8080/v1/batch/get" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "keys": ["user:1", "user:2", "user:3"],
    "include_metadata": true
  }'
```

#### Transactions

```bash
curl -X POST "http://localhost:8080/v1/transaction" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "operations": [
      {
        "type": "put",
        "key": "user:123:profile",
        "value": "Updated profile",
        "conditions": [
          {
            "type": "version_match",
            "value": "5"
          }
        ]
      },
      {
        "type": "delete",
        "key": "user:123:temp_data"
      },
      {
        "type": "put",
        "key": "user:123:last_login",
        "value": "2024-01-15T10:30:00Z",
        "ttl_seconds": 86400
      }
    ]
  }'
```

#### Streaming (Server-Sent Events)

```bash
curl -N -H "X-API-Key: your-api-key" \
  "http://localhost:8080/v1/watch/user:?events=put,delete&include_values=true"
```

Response stream:
```
event: put
data: {"key": "user:123", "value": "John Doe", "version": 2}

event: delete
data: {"key": "user:456", "version": 3}

event: put
data: {"key": "user:789", "value": "New User", "version": 1}
```

## Error Handling

### HTTP Status Codes

| Code | Meaning | Description |
|------|---------|-------------|
| `200` | OK | Request successful |
| `201` | Created | Key created successfully |
| `400` | Bad Request | Invalid request format |
| `401` | Unauthorized | Missing or invalid authentication |
| `403` | Forbidden | Insufficient permissions |
| `404` | Not Found | Key not found |
| `409` | Conflict | Version mismatch or condition failed |
| `412` | Precondition Failed | If-None-Match or If-Match failed |
| `413` | Payload Too Large | Value exceeds size limit |
| `429` | Too Many Requests | Rate limit exceeded |
| `500` | Internal Server Error | Server error |
| `503` | Service Unavailable | Service temporarily unavailable |

### Error Response Format

```json
{
  "error": "Version mismatch",
  "details": "Expected version 5, got 3",
  "code": "VERSION_CONFLICT",
  "timestamp": "2024-01-15T10:30:00Z",
  "trace_id": "abc123def456"
}
```

### gRPC Error Codes

gRPC uses standard status codes:

- `OK` (0): Success
- `INVALID_ARGUMENT` (3): Invalid request
- `NOT_FOUND` (5): Key not found
- `ALREADY_EXISTS` (6): Key already exists (with If-None-Match)
- `PERMISSION_DENIED` (7): Insufficient permissions
- `FAILED_PRECONDITION` (9): Condition failed
- `ABORTED` (10): Transaction conflict
- `OUT_OF_RANGE` (11): Value too large
- `UNIMPLEMENTED` (12): Feature not implemented
- `INTERNAL` (13): Internal server error
- `UNAVAILABLE` (14): Service unavailable

## Rate Limiting

### Default Limits

- **Per API Key**: 1,000 requests per minute
- **Burst Limit**: 100 requests in 10 seconds
- **Batch Operations**: Count as number of items × complexity factor

### Rate Limit Headers

```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 950
X-RateLimit-Reset: 1642234567
Retry-After: 60
```

### Handling Rate Limits

```javascript
async function putWithRetry(key, value, maxRetries = 3) {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch(`/v1/keys/${key}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-API-Key': 'your-api-key'
        },
        body: JSON.stringify({ value })
      });
      
      if (response.status === 429) {
        const retryAfter = response.headers.get('Retry-After');
        await new Promise(resolve => 
          setTimeout(resolve, (retryAfter || 60) * 1000)
        );
        continue;
      }
      
      return response;
    } catch (error) {
      if (attempt === maxRetries) throw error;
      await new Promise(resolve => 
        setTimeout(resolve, Math.pow(2, attempt) * 1000)
      );
    }
  }
}
```

## Client Libraries

### Go Client

```go
import "distributed-kvstore/pkg/client"

client, err := client.NewAdvancedClient(client.Config{
    Servers: []string{
        "kvstore-1.example.com:9090",
        "kvstore-2.example.com:9090",
        "kvstore-3.example.com:9090",
    },
    APIKey: "your-api-key",
    TLS: &client.TLSConfig{
        CertFile: "client.crt",
        KeyFile:  "client.key",
        CAFile:   "ca.crt",
    },
    RetryPolicy: &client.RetryPolicy{
        MaxRetries:    3,
        InitialDelay:  100 * time.Millisecond,
        MaxDelay:      10 * time.Second,
        BackoffFactor: 2.0,
    },
})

// Use client with automatic retries and load balancing
err = client.Put(ctx, "mykey", "myvalue", client.WithTTL(1*time.Hour))
value, err := client.Get(ctx, "mykey")
```

### Python Client

```python
from kvstore import KVStoreClient

client = KVStoreClient(
    servers=[
        "kvstore-1.example.com:9090",
        "kvstore-2.example.com:9090", 
        "kvstore-3.example.com:9090"
    ],
    api_key="your-api-key",
    tls_config={
        "cert_file": "client.crt",
        "key_file": "client.key", 
        "ca_file": "ca.crt"
    }
)

# Basic operations
client.put("mykey", "myvalue", ttl_seconds=3600)
value = client.get("mykey")
client.delete("mykey")

# Batch operations
results = client.batch_get(["key1", "key2", "key3"])
client.batch_put([
    {"key": "key1", "value": "value1"},
    {"key": "key2", "value": "value2"}
])

# Transactions
with client.transaction() as tx:
    tx.put("key1", "new_value1")
    tx.delete("key2")
    tx.put("key3", "new_value3")
    # Automatically committed when exiting context
```

### JavaScript/Node.js Client

```javascript
const { KVStoreClient } = require('@kvstore/client');

const client = new KVStoreClient({
  servers: [
    'kvstore-1.example.com:9090',
    'kvstore-2.example.com:9090',
    'kvstore-3.example.com:9090'
  ],
  apiKey: 'your-api-key',
  tls: {
    certFile: 'client.crt',
    keyFile: 'client.key',
    caFile: 'ca.crt'
  }
});

// Async/await syntax
async function example() {
  await client.put('mykey', 'myvalue', { ttlSeconds: 3600 });
  const value = await client.get('mykey');
  await client.delete('mykey');
  
  // Batch operations
  const results = await client.batchGet(['key1', 'key2', 'key3']);
  await client.batchPut([
    { key: 'key1', value: 'value1' },
    { key: 'key2', value: 'value2' }
  ]);
  
  // Watch for changes
  const watcher = client.watch('user:');
  watcher.on('change', (event) => {
    console.log(`Key ${event.key} ${event.type}: ${event.value}`);
  });
}
```

## Examples

### E-commerce Session Management

```go
// Store user session
session := SessionData{
    UserID:    "user123",
    CartItems: []string{"item1", "item2"},
    LoginTime: time.Now(),
}

sessionJSON, _ := json.Marshal(session)
err := client.Put(ctx, "session:"+sessionID, string(sessionJSON), 
    client.WithTTL(30*time.Minute))

// Retrieve and extend session
sessionData, err := client.Get(ctx, "session:"+sessionID)
if err == nil {
    // Extend session by updating TTL
    err = client.Put(ctx, "session:"+sessionID, sessionData, 
        client.WithTTL(30*time.Minute))
}
```

### Caching with Fallback

```python
def get_user_profile(user_id):
    cache_key = f"profile:{user_id}"
    
    # Try cache first
    try:
        cached_data = client.get(cache_key)
        if cached_data:
            return json.loads(cached_data)
    except KeyError:
        pass  # Cache miss
    
    # Fallback to database
    profile = database.get_user_profile(user_id)
    
    # Cache the result
    client.put(
        cache_key, 
        json.dumps(profile), 
        ttl_seconds=300  # 5 minutes
    )
    
    return profile
```

### Distributed Counters

```javascript
async function incrementCounter(counterId, amount = 1) {
  const maxRetries = 10;
  
  for (let i = 0; i < maxRetries; i++) {
    try {
      // Get current value and version
      const current = await client.get(counterId, { includeMetadata: true });
      const currentValue = current.found ? parseInt(current.value) : 0;
      const newValue = currentValue + amount;
      
      // Conditional update using version
      await client.put(counterId, newValue.toString(), {
        ifMatch: current.metadata?.version
      });
      
      return newValue;
    } catch (error) {
      if (error.code === 'VERSION_CONFLICT') {
        // Retry on conflict
        await new Promise(resolve => 
          setTimeout(resolve, Math.random() * 100)
        );
        continue;
      }
      throw error;
    }
  }
  
  throw new Error('Failed to increment counter after retries');
}
```

### Configuration Management

```go
type AppConfig struct {
    DatabaseURL    string            `json:"database_url"`
    FeatureFlags   map[string]bool   `json:"feature_flags"`
    RateLimits     map[string]int    `json:"rate_limits"`
    LastUpdated    time.Time         `json:"last_updated"`
}

// Watch for configuration changes
func watchConfig(ctx context.Context, client *kvstore.Client, configKey string) {
    watcher := client.Watch(ctx, configKey)
    
    for {
        select {
        case event := <-watcher.Events():
            if event.Type == kvstore.EventTypePut {
                var config AppConfig
                if err := json.Unmarshal(event.Value, &config); err == nil {
                    updateApplicationConfig(config)
                    log.Printf("Configuration updated: %+v", config)
                }
            }
        case err := <-watcher.Errors():
            log.Printf("Configuration watch error: %v", err)
            return
        case <-ctx.Done():
            return
        }
    }
}
```

### Multi-Region Replication

```python
class MultiRegionClient:
    def __init__(self, regions):
        self.clients = {
            region: KVStoreClient(servers=servers)
            for region, servers in regions.items()
        }
        self.primary_region = list(regions.keys())[0]
    
    def put(self, key, value, **kwargs):
        # Write to primary region first
        primary_client = self.clients[self.primary_region]
        result = primary_client.put(key, value, **kwargs)
        
        # Async replication to other regions
        for region, client in self.clients.items():
            if region != self.primary_region:
                asyncio.create_task(
                    client.put(key, value, **kwargs)
                )
        
        return result
    
    def get(self, key, prefer_local=True):
        if prefer_local and 'local' in self.clients:
            try:
                return self.clients['local'].get(key)
            except KeyError:
                pass
        
        # Fallback to primary region
        return self.clients[self.primary_region].get(key)
```

This API documentation provides comprehensive coverage of both gRPC and REST interfaces, with practical examples for common use cases. The OpenAPI specification (`docs/openapi.yaml`) provides machine-readable API definitions for code generation and testing tools.