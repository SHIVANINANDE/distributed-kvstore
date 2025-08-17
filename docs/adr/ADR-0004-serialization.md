# ADR-0004: Protocol Buffers for Serialization

## Status
Accepted

Date: 2024-01-15

## Context

The distributed key-value store requires efficient serialization for:

1. **Inter-node Communication**: Raft consensus messages between cluster nodes
2. **Client-Server Communication**: gRPC API requests and responses
3. **Persistent Storage**: Log entries and metadata serialization
4. **Backup/Restore**: Data export and import formats

### Requirements
- **Performance**: Fast serialization/deserialization (< 1ms for typical messages)
- **Efficiency**: Compact wire format to minimize network overhead
- **Schema Evolution**: Forward and backward compatibility for rolling upgrades
- **Type Safety**: Strong typing to prevent serialization errors
- **Cross-Language**: Support for multiple client languages
- **Tooling**: Good development and debugging tools

### Constraints
- Must integrate well with gRPC
- Should support Go's native types efficiently
- Need versioning strategy for long-term evolution
- Must handle large values (up to 16MB) efficiently

## Decision

We will use **Protocol Buffers (protobuf) version 3** as the primary serialization format for all structured data in the system.

### Scope of Usage
1. **gRPC API**: All client-server communication
2. **Raft Messages**: Consensus protocol messages
3. **Log Entries**: Raft log entry format
4. **Configuration**: Cluster configuration and metadata
5. **Snapshots**: Consistent point-in-time exports

### Schema Organization
```
proto/
├── kvstore/           # Client API definitions
│   ├── kvstore.proto
│   └── admin.proto
├── raft/              # Internal Raft protocol
│   ├── raft.proto
│   └── log.proto
├── cluster/           # Cluster management
│   ├── membership.proto
│   └── config.proto
└── common/            # Shared types
    ├── types.proto
    └── errors.proto
```

## Consequences

### Positive
- **Performance**: Binary format with efficient encoding/decoding
- **Schema Evolution**: Built-in versioning and compatibility rules
- **Type Safety**: Compile-time type checking and validation
- **Code Generation**: Automatic client library generation
- **Tooling**: Excellent debugging and inspection tools
- **Ecosystem**: Wide language support and mature ecosystem

### Negative
- **Binary Format**: Not human-readable without tools
- **Schema Dependency**: Changes require coordinated deployment
- **Learning Curve**: Developers need to understand protobuf concepts
- **Build Complexity**: Additional code generation step in build process
- **Large Messages**: Less efficient for very large payloads compared to streaming

### Performance Characteristics
- **Serialization**: 2-10x faster than JSON for structured data
- **Wire Size**: 20-50% smaller than JSON
- **Memory Usage**: Lower GC pressure due to efficient decoding
- **Parsing**: Zero-copy deserialization for some operations

## Implementation Details

### Core Types Definition

```protobuf
// common/types.proto
syntax = "proto3";
package common;

message KeyValue {
  string key = 1;
  bytes value = 2;
  int64 created_at = 3;
  int64 expires_at = 4;
  int64 version = 5;
  map<string, string> metadata = 6;
}

message Operation {
  enum Type {
    PUT = 0;
    DELETE = 1;
    BATCH_PUT = 2;
    BATCH_DELETE = 3;
  }
  
  Type type = 1;
  string key = 2;
  bytes value = 3;
  int64 ttl_seconds = 4;
  map<string, string> metadata = 5;
}
```

### Raft Protocol Messages

```protobuf
// raft/raft.proto
syntax = "proto3";
package raft;

import "common/types.proto";

message LogEntry {
  uint64 index = 1;
  uint64 term = 2;
  int64 timestamp = 3;
  common.Operation operation = 4;
  bytes checksum = 5;
}

message AppendEntriesRequest {
  uint64 term = 1;
  string leader_id = 2;
  uint64 prev_log_index = 3;
  uint64 prev_log_term = 4;
  repeated LogEntry entries = 5;
  uint64 leader_commit = 6;
}
```

### Schema Evolution Strategy

#### Versioning Rules
1. **Field Numbers**: Never reuse field numbers
2. **Required Fields**: Avoid required fields (use validation instead)
3. **Default Values**: Always provide sensible defaults
4. **Backward Compatibility**: New fields must be optional
5. **Forward Compatibility**: Ignore unknown fields

#### Example Evolution
```protobuf
// Version 1
message PutRequest {
  string key = 1;
  bytes value = 2;
}

// Version 2 - Adding optional TTL
message PutRequest {
  string key = 1;
  bytes value = 2;
  int64 ttl_seconds = 3;  // Optional, default = 0 (no expiration)
}

// Version 3 - Adding metadata
message PutRequest {
  string key = 1;
  bytes value = 2;
  int64 ttl_seconds = 3;
  map<string, string> metadata = 4;  // Optional
}
```

### Code Generation Setup

```makefile
# Makefile
.PHONY: generate-proto
generate-proto:
	@echo "Generating Go code from proto files..."
	@find proto -name "*.proto" | xargs protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		--proto_path=proto
```

### Optimization Strategies

#### Message Size Optimization
```protobuf
message OptimizedLogEntry {
  // Use smaller integer types when possible
  uint32 term = 1;           // Instead of uint64 if range allows
  
  // Pack related fields
  uint64 index_and_flags = 2; // Combine index + flags in single field
  
  // Use oneof for polymorphic data
  oneof operation_type {
    PutOperation put_op = 3;
    DeleteOperation delete_op = 4;
  }
}
```

#### Performance Optimization
```go
// Pool protobuf messages to reduce allocations
type MessagePool struct {
    putRequests   sync.Pool
    getResponses  sync.Pool
    logEntries    sync.Pool
}

func (p *MessagePool) GetPutRequest() *pb.PutRequest {
    if req := p.putRequests.Get(); req != nil {
        return req.(*pb.PutRequest)
    }
    return &pb.PutRequest{}
}

func (p *MessagePool) PutPutRequest(req *pb.PutRequest) {
    req.Reset()
    p.putRequests.Put(req)
}
```

## Alternatives Considered

### 1. JSON
**Pros:**
- Human-readable format
- Native browser support
- Simple debugging
- No code generation required

**Cons:**
- Larger wire size (2-3x)
- Slower parsing performance
- No schema validation
- Verbose for binary data

**Verdict:** Rejected for performance-critical paths, used for REST API

### 2. MessagePack
**Pros:**
- Compact binary format
- Faster than JSON
- Self-describing format
- Good language support

**Cons:**
- No schema validation
- No code generation
- Limited tooling
- No built-in versioning

**Verdict:** Rejected due to lack of schema evolution support

### 3. Apache Avro
**Pros:**
- Schema evolution support
- Compact binary format
- Schema registry concept
- Good for data pipelines

**Cons:**
- Complex schema resolution
- Limited gRPC integration
- Java-centric ecosystem
- Schema registry dependency

**Verdict:** Rejected due to complexity and gRPC integration challenges

### 4. FlatBuffers
**Pros:**
- Zero-copy deserialization
- Very fast access
- Cross-platform support
- Memory efficient

**Cons:**
- More complex to use
- Limited schema evolution
- Larger code generation
- Overkill for most use cases

**Verdict:** Rejected due to complexity for marginal benefits

### 5. Custom Binary Format
**Pros:**
- Optimal for specific use case
- Full control over format
- Minimal overhead
- No dependencies

**Cons:**
- High development effort
- Error-prone implementation
- No tooling support
- Maintenance burden

**Verdict:** Rejected due to development overhead and risk

### 6. CBOR (Concise Binary Object Representation)
**Pros:**
- Standardized format (RFC 7049)
- Self-describing
- Good space efficiency
- Growing ecosystem

**Cons:**
- No schema validation
- Limited code generation
- Less mature tooling
- No built-in versioning

**Verdict:** Rejected due to lack of schema evolution features

## Validation and Testing

### Schema Validation
```go
// Compile-time validation
func validatePutRequest(req *pb.PutRequest) error {
    if req.Key == "" {
        return errors.New("key cannot be empty")
    }
    if len(req.Key) > 1024 {
        return errors.New("key too long")
    }
    if len(req.Value) > 16*1024*1024 {
        return errors.New("value too large")
    }
    return nil
}
```

### Compatibility Testing
```go
func TestSchemaCompatibility(t *testing.T) {
    // Test that new client can read old messages
    oldMessage := &pb.PutRequestV1{
        Key:   "test",
        Value: []byte("value"),
    }
    
    // Serialize with old format
    oldData, err := proto.Marshal(oldMessage)
    require.NoError(t, err)
    
    // Deserialize with new format
    newMessage := &pb.PutRequest{}
    err = proto.Unmarshal(oldData, newMessage)
    require.NoError(t, err)
    
    assert.Equal(t, "test", newMessage.Key)
    assert.Equal(t, []byte("value"), newMessage.Value)
    assert.Equal(t, int64(0), newMessage.TtlSeconds) // Default value
}
```

### Performance Benchmarks
```go
func BenchmarkProtobufSerialization(b *testing.B) {
    req := &pb.PutRequest{
        Key:   "benchmark-key",
        Value: make([]byte, 1024),
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        data, _ := proto.Marshal(req)
        newReq := &pb.PutRequest{}
        proto.Unmarshal(data, newReq)
    }
}
```

## Monitoring and Debugging

### Protobuf Metrics
```go
type ProtobufMetrics struct {
    SerializationDuration *prometheus.HistogramVec // by message_type
    MessageSize          *prometheus.HistogramVec // by message_type
    SerializationErrors  *prometheus.CounterVec   // by message_type, error_type
}
```

### Debugging Tools
1. **protoc**: Schema compilation and validation
2. **protoreflect**: Runtime schema inspection
3. **buf**: Modern protobuf tooling and linting
4. **grpcurl**: gRPC client for testing
5. **Custom tools**: Message inspection utilities

### Wire Format Analysis
```bash
# Analyze protobuf wire format
protoc --decode=kvstore.PutRequest proto/kvstore/kvstore.proto < message.bin

# Validate schema compatibility
buf breaking --against .git#HEAD
```

## Migration Strategy

### Gradual Adoption
1. **Phase 1**: Implement for new gRPC APIs
2. **Phase 2**: Convert Raft protocol messages
3. **Phase 3**: Migrate configuration and metadata
4. **Phase 4**: Update backup/restore formats

### Backward Compatibility
- Maintain JSON support for REST API
- Version protobuf schemas carefully
- Provide conversion utilities between formats
- Support mixed deployments during transition

## References

- [Protocol Buffers Documentation](https://developers.google.com/protocol-buffers) - Official protobuf guide
- [gRPC Protocol Buffer Guide](https://grpc.io/docs/guides/concepts/) - gRPC-specific protobuf usage
- [Buf](https://buf.build/) - Modern protobuf tooling
- [Proto3 Language Guide](https://developers.google.com/protocol-buffers/docs/proto3) - Proto3 syntax reference
- [Schema Evolution Best Practices](https://docs.confluent.io/platform/current/schema-registry/fundamentals/schema-evolution.html) - Schema evolution patterns