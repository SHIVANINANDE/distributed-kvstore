# ADR-0003: Dual Protocol API Design (gRPC + REST)

## Status
Accepted

Date: 2024-01-15

## Context

The distributed key-value store needs to provide APIs for client applications. Different use cases have different requirements:

### gRPC Requirements
- High-performance applications needing low latency
- Streaming operations for real-time updates
- Type-safe client-server communication
- Efficient binary serialization
- Built-in load balancing and connection management

### REST Requirements
- Web browsers and JavaScript applications
- Legacy systems without gRPC support
- Simple HTTP tooling and debugging
- Human-readable request/response formats
- Standard HTTP semantics and caching

### Performance Goals
- **gRPC**: P99 latency < 5ms, >100K ops/sec
- **REST**: P99 latency < 20ms, >50K ops/sec
- **Memory**: Minimal overhead for dual protocol support
- **Development**: Single codebase maintaining both protocols

## Decision

We will implement **dual protocol support** offering both gRPC and REST APIs with a unified backend implementation.

### Architecture Overview
```
┌─────────────────┐    ┌─────────────────┐
│   gRPC Clients  │    │   REST Clients  │
└─────────┬───────┘    └─────────┬───────┘
          │                      │
┌─────────▼───────┐    ┌─────────▼───────┐
│   gRPC Server   │    │   HTTP Server   │
└─────────┬───────┘    └─────────┬───────┘
          │                      │
          └──────┬─────────────┬─┘
                 │             │
        ┌────────▼─────────────▼────────┐
        │    Service Layer             │
        │  (Business Logic)            │
        └────────┬─────────────────────┘
                 │
        ┌────────▼─────────────────────┐
        │    Storage Engine            │
        │    (BadgerDB)                │
        └──────────────────────────────┘
```

### Protocol Mapping Strategy
1. **Unified Service Layer**: Single business logic implementation
2. **Protocol Adapters**: Thin translation layers for each protocol
3. **Shared Types**: Common internal types with protocol-specific serialization
4. **Error Handling**: Consistent error semantics across protocols
5. **Authentication**: Unified auth layer supporting both protocols

## Consequences

### Positive
- **Flexibility**: Supports diverse client ecosystems
- **Performance**: gRPC for high-performance, REST for compatibility
- **Developer Experience**: Choose best protocol for use case
- **Future Proofing**: Can add more protocols (GraphQL, WebSocket) easily
- **Ecosystem Integration**: Better integration with different tech stacks

### Negative
- **Complexity**: Maintaining two protocol implementations
- **Documentation**: Need comprehensive docs for both APIs
- **Testing**: Must test both protocol paths thoroughly
- **Resource Usage**: Slightly higher memory and CPU overhead
- **Deployment**: More ports and configuration to manage

### Trade-offs
- **Code Duplication**: Some protocol-specific code, mitigated by shared service layer
- **Consistency**: Need to ensure feature parity between protocols
- **Performance**: gRPC optimized for speed, REST optimized for compatibility
- **Versioning**: Need coordinated versioning strategy across protocols

## Implementation Details

### gRPC Protocol Implementation

#### Service Definition
```protobuf
service KVStore {
  // Core operations
  rpc Put(PutRequest) returns (PutResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  
  // Batch operations
  rpc BatchPut(BatchPutRequest) returns (BatchPutResponse);
  rpc BatchGet(BatchGetRequest) returns (BatchGetResponse);
  
  // Streaming operations
  rpc Watch(WatchRequest) returns (stream WatchResponse);
  
  // Administrative
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Status(StatusRequest) returns (StatusResponse);
}
```

#### Server Configuration
```go
type GRPCServerConfig struct {
    Port            int
    TLSConfig       *tls.Config
    MaxConnections  int
    KeepAlive       time.Duration
    MaxMessageSize  int
    Interceptors    []grpc.UnaryServerInterceptor
}
```

### REST Protocol Implementation

#### Endpoint Mapping
| HTTP Method | Endpoint | gRPC Equivalent |
|-------------|----------|-----------------|
| `PUT /v1/keys/{key}` | Create/Update | `Put` |
| `GET /v1/keys/{key}` | Retrieve | `Get` |
| `DELETE /v1/keys/{key}` | Delete | `Delete` |
| `POST /v1/batch/put` | Batch Create | `BatchPut` |
| `POST /v1/batch/get` | Batch Retrieve | `BatchGet` |
| `GET /v1/watch/{prefix}` | Server-Sent Events | `Watch` stream |
| `GET /v1/health` | Health Check | `Health` |
| `GET /v1/status` | Status | `Status` |

#### Server Configuration
```go
type HTTPServerConfig struct {
    Port           int
    TLSConfig      *tls.Config
    ReadTimeout    time.Duration
    WriteTimeout   time.Duration
    MaxHeaderBytes int
    CORS           CORSConfig
    RateLimit      RateLimitConfig
}
```

### Unified Service Layer

```go
type KVStoreService interface {
    Put(ctx context.Context, req *PutRequest) (*PutResponse, error)
    Get(ctx context.Context, req *GetRequest) (*GetResponse, error)
    Delete(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error)
    BatchPut(ctx context.Context, req *BatchPutRequest) (*BatchPutResponse, error)
    BatchGet(ctx context.Context, req *BatchGetRequest) (*BatchGetResponse, error)
    Watch(ctx context.Context, req *WatchRequest) (<-chan *WatchEvent, error)
    Health(ctx context.Context) (*HealthResponse, error)
    Status(ctx context.Context) (*StatusResponse, error)
}

type service struct {
    storage     StorageEngine
    raft        ConsensusEngine
    auth        AuthManager
    metrics     MetricsCollector
    logger      Logger
}
```

### Protocol Translation

#### gRPC to Service Layer
```go
func (s *grpcServer) Put(ctx context.Context, req *pb.PutRequest) (*pb.PutResponse, error) {
    // Input validation
    if err := validatePutRequest(req); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }
    
    // Convert to internal format
    internalReq := &PutRequest{
        Key:        req.Key,
        Value:      req.Value,
        TTLSeconds: req.TtlSeconds,
    }
    
    // Call service layer
    internalResp, err := s.service.Put(ctx, internalReq)
    if err != nil {
        return nil, convertError(err)
    }
    
    // Convert response
    return &pb.PutResponse{
        Success: internalResp.Success,
    }, nil
}
```

#### REST to Service Layer
```go
func (s *httpServer) putHandler(w http.ResponseWriter, r *http.Request) {
    // Extract key from URL
    key := mux.Vars(r)["key"]
    
    // Parse JSON request
    var req struct {
        Value      string            `json:"value"`
        TTLSeconds int64             `json:"ttl_seconds,omitempty"`
        Metadata   map[string]string `json:"metadata,omitempty"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }
    
    // Convert to internal format
    internalReq := &PutRequest{
        Key:        key,
        Value:      []byte(req.Value),
        TTLSeconds: req.TTLSeconds,
        Metadata:   req.Metadata,
    }
    
    // Call service layer
    internalResp, err := s.service.Put(r.Context(), internalReq)
    if err != nil {
        httpError(w, err)
        return
    }
    
    // Send JSON response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": internalResp.Success,
        "version": internalResp.Version,
    })
}
```

## Alternatives Considered

### 1. gRPC Only
**Pros:**
- Simpler codebase
- Better performance
- Type safety
- Built-in streaming

**Cons:**
- Limited client ecosystem
- Not web browser compatible
- Harder debugging without tools
- Learning curve for HTTP developers

**Verdict:** Rejected due to compatibility requirements

### 2. REST Only
**Pros:**
- Universal compatibility
- Easy debugging
- Familiar to all developers
- Standard HTTP semantics

**Cons:**
- Lower performance than gRPC
- No native streaming
- Text-based overhead
- Manual client generation

**Verdict:** Rejected due to performance requirements

### 3. GraphQL
**Pros:**
- Flexible query language
- Single endpoint
- Strong type system
- Good tooling ecosystem

**Cons:**
- Overhead for simple key-value operations
- Not well-suited for streaming
- Additional complexity
- Caching challenges

**Verdict:** Rejected as not optimal for key-value operations

### 4. Protocol Translation Layer (grpc-gateway)
**Pros:**
- Single gRPC implementation
- Automatic REST API generation
- Consistent behavior
- Less code duplication

**Cons:**
- Less control over REST API design
- Additional translation overhead
- Limited REST customization
- Dependency on third-party tool

**Verdict:** Considered but rejected for flexibility requirements

### 5. WebSocket API
**Pros:**
- Real-time communication
- Lower overhead than HTTP
- Bidirectional communication
- Good for streaming

**Cons:**
- Not RESTful
- More complex client implementation
- Connection management overhead
- Limited HTTP tooling support

**Verdict:** Rejected in favor of Server-Sent Events for streaming

## Protocol-Specific Features

### gRPC Enhancements
- **Connection Pooling**: Built-in client-side load balancing
- **Streaming**: Bidirectional streaming for watch operations
- **Compression**: Built-in gzip compression
- **Deadlines**: Automatic timeout propagation
- **Metadata**: Rich context passing

### REST Enhancements
- **CORS Support**: Cross-origin resource sharing for web apps
- **Server-Sent Events**: Streaming via HTTP for watch operations
- **ETags**: HTTP caching with conditional requests
- **Content Negotiation**: Support JSON and MessagePack
- **OpenAPI**: Machine-readable API documentation

## Monitoring and Observability

### Metrics Collection
```go
type ProtocolMetrics struct {
    RequestCount    *prometheus.CounterVec   // by protocol, method, status
    RequestDuration *prometheus.HistogramVec // by protocol, method
    ActiveConnections *prometheus.GaugeVec   // by protocol
    ErrorRate       *prometheus.CounterVec   // by protocol, error_type
}
```

### Health Checks
- **gRPC**: Standard gRPC health checking protocol
- **REST**: HTTP health endpoint with JSON response
- **Unified**: Both protocols report same underlying health

### Logging
```go
type RequestLog struct {
    Protocol    string        `json:"protocol"`
    Method      string        `json:"method"`
    Path        string        `json:"path,omitempty"`
    UserID      string        `json:"user_id,omitempty"`
    Duration    time.Duration `json:"duration"`
    StatusCode  int           `json:"status_code"`
    Error       string        `json:"error,omitempty"`
    TraceID     string        `json:"trace_id"`
}
```

## Testing Strategy

### Protocol-Specific Tests
- **gRPC**: Use gRPC test clients and mocking
- **REST**: HTTP integration tests with standard tools
- **Load Testing**: Protocol-specific load testing

### Unified Tests
- **Service Layer**: Business logic tests independent of protocol
- **Integration**: End-to-end tests via both protocols
- **Compatibility**: Ensure feature parity between protocols

### Performance Tests
- **Latency**: Measure per-protocol latency characteristics
- **Throughput**: Compare throughput under different loads
- **Resource Usage**: Memory and CPU overhead comparison

## Future Considerations

### Additional Protocols
The architecture supports adding more protocols:
- **GraphQL**: For complex query scenarios
- **WebSocket**: For real-time applications requiring bidirectional communication
- **MessagePack**: For performance-critical applications needing compact serialization

### Protocol Evolution
- **gRPC**: Plan for Protocol Buffers schema evolution
- **REST**: API versioning strategy with backward compatibility
- **Features**: Ensure new features work across all protocols

## References

- [gRPC Documentation](https://grpc.io/docs/) - Official gRPC guide
- [OpenAPI Specification](https://swagger.io/specification/) - REST API documentation standard
- [Protocol Buffers](https://developers.google.com/protocol-buffers) - Serialization format
- [REST API Design Best Practices](https://restfulapi.net/) - REST design guidelines
- [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) - HTTP streaming specification