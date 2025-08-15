# Protocol Buffers Documentation

This document explains the Protocol Buffer definitions used in the distributed key-value store for gRPC communication.

## Overview

The project uses Protocol Buffers (protobuf) to define:
- **Service interfaces** for client-server and internal communication
- **Message types** for request/response data structures
- **Type-safe APIs** with automatic code generation

## Service Definitions

### KVStore Service (`proto/kvstore/kvstore.proto`)

The main client-facing service for key-value operations.

#### Basic Operations
- `Put(PutRequest) → PutResponse` - Store a key-value pair
- `Get(GetRequest) → GetResponse` - Retrieve a value by key
- `Delete(DeleteRequest) → DeleteResponse` - Remove a key-value pair
- `Exists(ExistsRequest) → ExistsResponse` - Check if a key exists

#### Advanced Operations
- `List(ListRequest) → ListResponse` - List key-value pairs with prefix
- `ListKeys(ListKeysRequest) → ListKeysResponse` - List keys only with prefix

#### Batch Operations
- `BatchPut(BatchPutRequest) → BatchPutResponse` - Store multiple key-value pairs
- `BatchGet(BatchGetRequest) → BatchGetResponse` - Retrieve multiple values
- `BatchDelete(BatchDeleteRequest) → BatchDeleteResponse` - Remove multiple keys

#### Storage Management
- `Backup(BackupRequest) → BackupResponse` - Create a backup
- `Restore(RestoreRequest) → RestoreResponse` - Restore from backup
- `Stats(StatsRequest) → StatsResponse` - Get storage statistics

#### Health and Status
- `Health(HealthRequest) → HealthResponse` - Check service health
- `Status(StatusRequest) → StatusResponse` - Get cluster status

### ClusterService (`proto/cluster/cluster.proto`)

Internal service for node-to-node communication in the distributed cluster.

#### Node Management
- `JoinCluster(JoinRequest) → JoinResponse` - Add a node to the cluster
- `LeaveCluster(LeaveRequest) → LeaveResponse` - Remove a node from the cluster
- `GetNodes(GetNodesRequest) → GetNodesResponse` - List cluster nodes

#### Raft Consensus
- `RequestVote(VoteRequest) → VoteResponse` - Raft leader election
- `AppendEntries(AppendRequest) → AppendResponse` - Raft log replication
- `InstallSnapshot(SnapshotRequest) → SnapshotResponse` - Raft snapshot transfer

#### Data Replication
- `ReplicateData(ReplicationRequest) → ReplicationResponse` - Replicate operations
- `SyncData(SyncRequest) → SyncResponse` - Synchronize data between nodes

#### Health and Monitoring
- `Ping(PingRequest) → PingResponse` - Node health check
- `GetClusterStatus(ClusterStatusRequest) → ClusterStatusResponse` - Cluster health

## Key Message Types

### KVStore Messages

#### PutRequest/PutResponse
```protobuf
message PutRequest {
  string key = 1;
  bytes value = 2;
  int64 ttl_seconds = 3; // Time to live (0 = no expiration)
}

message PutResponse {
  bool success = 1;
  string error = 2;
}
```

#### GetRequest/GetResponse
```protobuf
message GetRequest {
  string key = 1;
}

message GetResponse {
  bool found = 1;
  bytes value = 2;
  string error = 3;
  int64 created_at = 4; // Unix timestamp
  int64 expires_at = 5; // Unix timestamp (0 = no expiration)
}
```

#### ListRequest/ListResponse
```protobuf
message ListRequest {
  string prefix = 1;
  int32 limit = 2;   // Maximum items to return (0 = no limit)
  string cursor = 3; // Pagination cursor
}

message ListResponse {
  repeated KeyValue items = 1;
  string next_cursor = 2; // Cursor for next page
  bool has_more = 3;      // Whether there are more results
  string error = 4;
}
```

### Cluster Messages

#### JoinRequest/JoinResponse
```protobuf
message JoinRequest {
  string node_id = 1;
  string address = 2;   // Node's network address
  int32 raft_port = 3;  // Node's Raft port
  int32 grpc_port = 4;  // Node's gRPC port
  map<string, string> metadata = 5; // Additional node metadata
}

message JoinResponse {
  bool success = 1;
  string leader_id = 2;
  repeated NodeInfo nodes = 3; // Current cluster nodes
  string error = 4;
}
```

#### Raft Messages
```protobuf
message VoteRequest {
  int64 term = 1;
  string candidate_id = 2;
  int64 last_log_index = 3;
  int64 last_log_term = 4;
}

message AppendRequest {
  int64 term = 1;
  string leader_id = 2;
  int64 prev_log_index = 3;
  int64 prev_log_term = 4;
  repeated LogEntry entries = 5;
  int64 leader_commit = 6;
}
```

## Features

### TTL Support
Keys can have time-to-live (TTL) values for automatic expiration:
```protobuf
message PutRequest {
  string key = 1;
  bytes value = 2;
  int64 ttl_seconds = 3; // 0 = no expiration
}
```

### Pagination
List operations support pagination for large datasets:
```protobuf
message ListRequest {
  string prefix = 1;
  int32 limit = 2;   // Page size
  string cursor = 3; // Page token
}
```

### Batch Operations
Efficient batch processing for multiple operations:
```protobuf
message BatchPutRequest {
  repeated PutItem items = 1;
}

message BatchPutResponse {
  int32 success_count = 1;
  int32 error_count = 2;
  repeated BatchError errors = 3; // Individual errors
}
```

### Metadata Support
Extensible metadata for nodes and operations:
```protobuf
message NodeInfo {
  string node_id = 1;
  // ... other fields
  map<string, string> metadata = 8; // Flexible metadata
}
```

## Code Generation

### Prerequisites
```bash
# Install Protocol Buffers compiler
brew install protobuf  # macOS
sudo apt-get install protobuf-compiler  # Ubuntu

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Generation Commands
```bash
# Using the script
./scripts/generate-proto.sh

# Using Makefile
make proto

# Manual generation
protoc \
  --go_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_out=. \
  --go-grpc_opt=paths=source_relative \
  proto/kvstore/kvstore.proto

protoc \
  --go_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_out=. \
  --go-grpc_opt=paths=source_relative \
  proto/cluster/cluster.proto
```

### Generated Files
- `proto/kvstore/kvstore.pb.go` - Message types for KVStore
- `proto/kvstore/kvstore_grpc.pb.go` - gRPC service interfaces for KVStore
- `proto/cluster/cluster.pb.go` - Message types for Cluster
- `proto/cluster/cluster_grpc.pb.go` - gRPC service interfaces for Cluster

## Usage Examples

### Client Usage
```go
import (
    "distributed-kvstore/proto/kvstore"
    "google.golang.org/grpc"
)

// Connect to server
conn, err := grpc.Dial("localhost:9090", grpc.WithInsecure())
client := kvstore.NewKVStoreClient(conn)

// Put a value
resp, err := client.Put(ctx, &kvstore.PutRequest{
    Key:   "user:123",
    Value: []byte("john_doe"),
    TtlSeconds: 3600, // 1 hour TTL
})

// Get a value
resp, err := client.Get(ctx, &kvstore.GetRequest{
    Key: "user:123",
})

// List with prefix
resp, err := client.List(ctx, &kvstore.ListRequest{
    Prefix: "user:",
    Limit:  10,
})
```

### Server Implementation
```go
import (
    "distributed-kvstore/proto/kvstore"
)

type KVStoreServer struct {
    kvstore.UnimplementedKVStoreServer
    storage storage.Engine
}

func (s *KVStoreServer) Put(ctx context.Context, req *kvstore.PutRequest) (*kvstore.PutResponse, error) {
    err := s.storage.Put([]byte(req.Key), req.Value)
    return &kvstore.PutResponse{
        Success: err == nil,
        Error:   errorString(err),
    }, nil
}
```

## Best Practices

### Message Design
- Use `bytes` for binary data, `string` for text
- Include error fields in all response messages
- Use appropriate field numbers (1-15 for frequent fields)
- Add optional metadata fields for extensibility

### Versioning
- Never change field numbers
- Add new fields with new numbers
- Use `reserved` for deprecated fields
- Consider message versioning for breaking changes

### Performance
- Use appropriate message sizes
- Implement pagination for large responses
- Consider streaming for large data transfers
- Use batch operations for efficiency

### Error Handling
- Include detailed error messages
- Use appropriate gRPC status codes
- Implement proper error propagation
- Log errors for debugging