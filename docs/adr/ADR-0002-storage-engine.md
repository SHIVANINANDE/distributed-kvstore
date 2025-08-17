# ADR-0002: BadgerDB as Storage Engine

## Status
Accepted

Date: 2024-01-15

## Context

The distributed key-value store requires a high-performance storage engine for persistent data storage. The storage layer must provide:

- High throughput for both reads and writes
- ACID transaction support
- Efficient range queries and prefix scans
- Crash recovery and data durability
- Configurable consistency levels
- Memory-efficient operation
- Support for TTL (time-to-live) expiration

### Performance Requirements
- **Write Throughput**: >50K writes/second per node
- **Read Throughput**: >200K reads/second per node
- **Latency**: P99 < 10ms for single operations
- **Storage**: Support for TB-scale datasets
- **Memory**: Efficient memory usage with configurable cache sizes

### Operational Requirements
- **Backup/Restore**: Support for consistent snapshots
- **Monitoring**: Detailed metrics and observability
- **Maintenance**: Online compaction and garbage collection
- **Recovery**: Fast startup and crash recovery

## Decision

We will use **BadgerDB** as the primary storage engine for our distributed key-value store.

### Key Features Utilized
1. **LSM-Tree Architecture**: Write-optimized with efficient compaction
2. **Value Log**: Separate storage for large values to reduce write amplification
3. **MVCC Support**: Multi-version concurrency control for transactions
4. **Configurable Caching**: Separate caches for blocks and indexes
5. **Built-in Compression**: Snappy and Zstandard support
6. **TTL Support**: Native time-to-live functionality

### Configuration Strategy
```yaml
storage:
  engine: "badger"
  data_path: "/data/badger"
  
  # Memory settings
  memtable_size: 64MB
  block_cache_size: 256MB
  index_cache_size: 128MB
  
  # Performance settings
  num_memtables: 5
  num_level_zero_tables: 5
  level_size_multiplier: 10
  value_threshold: 1024
  
  # Operational settings
  sync_writes: false
  compression: "snappy"
  num_compactors: 4
  detect_conflicts: true
```

## Consequences

### Positive
- **High Performance**: LSM-tree provides excellent write performance
- **Go Native**: Written in Go, eliminating CGO overhead and compatibility issues
- **Active Development**: Maintained by DGraph team with regular updates
- **Rich Feature Set**: MVCC, transactions, TTL, compression built-in
- **Memory Efficient**: Configurable cache sizes and memory-mapped files
- **Operational Maturity**: Used in production by multiple organizations

### Negative
- **Read Amplification**: LSM-tree structure can cause read amplification
- **Compaction Overhead**: Background compaction can impact performance
- **Memory Usage**: Requires significant memory for optimal performance
- **Limited SQL**: No SQL interface, key-value only operations
- **Write Amplification**: Multiple levels can cause write amplification during compaction

### Performance Characteristics
- **Write Performance**: Excellent due to LSM-tree append-only writes
- **Read Performance**: Good with caching, degraded during compaction
- **Space Amplification**: ~1.3x due to LSM levels and value log
- **Memory Requirements**: 10-50% of working set for optimal performance

## Alternatives Considered

### 1. RocksDB
**Pros:**
- Battle-tested in production (Facebook, Uber, Netflix)
- Extensive tuning options
- Very high performance
- Rich ecosystem and tooling

**Cons:**
- CGO overhead and complexity
- C++ dependencies complicate deployment
- More complex configuration
- Larger memory footprint

**Verdict:** Rejected due to CGO complexity and deployment overhead

### 2. BoltDB/bbolt
**Pros:**
- Simple, pure Go implementation
- ACID transactions
- B-tree structure good for range queries
- Minimal configuration required

**Cons:**
- Single writer limitation
- Poor write performance for high-throughput workloads
- Limited scalability
- No built-in compression

**Verdict:** Rejected due to write performance limitations

### 3. LevelDB (Go ports)
**Pros:**
- Simple LSM-tree implementation
- Good baseline performance
- Well-understood algorithm

**Cons:**
- Limited active development
- Missing advanced features (TTL, better compression)
- No transaction support
- Memory inefficient compared to BadgerDB

**Verdict:** Rejected due to missing features and limited development

### 4. Embedded SQLite
**Pros:**
- Extremely stable and well-tested
- Full SQL support
- ACID transactions
- WAL mode for better concurrency

**Cons:**
- SQL overhead for simple key-value operations
- Single writer in most configurations
- Not optimized for key-value workloads
- CGO dependency

**Verdict:** Rejected due to SQL overhead and single-writer limitation

### 5. Custom Storage Engine
**Pros:**
- Full control over implementation
- Optimized for exact use case
- No external dependencies

**Cons:**
- Enormous development effort
- High risk of bugs and edge cases
- Need to reimplement crash recovery, compaction, etc.
- Delayed time to market

**Verdict:** Rejected due to development complexity and risk

### 6. In-Memory with Persistence (Redis-style)
**Pros:**
- Extremely fast access
- Simple implementation
- Good for caching workloads

**Cons:**
- Limited by memory size
- Expensive for large datasets
- Complex persistence mechanisms
- Cold start performance issues

**Verdict:** Rejected due to memory limitations for large datasets

## Implementation Details

### Storage Interface
```go
type StorageEngine interface {
    Get(key []byte) ([]byte, error)
    Put(key, value []byte) error
    Delete(key []byte) error
    
    // Batch operations
    BatchPut(items []BatchItem) error
    BatchGet(keys [][]byte) ([][]byte, error)
    
    // Range operations
    Scan(prefix []byte, limit int) (Iterator, error)
    
    // Transactions
    Begin() (Transaction, error)
    
    // Management
    Backup(path string) error
    Restore(path string) error
    Stats() StorageStats
    Close() error
}
```

### BadgerDB Integration
1. **Wrapper Layer**: Implement storage interface over BadgerDB
2. **Transaction Support**: Map our transaction interface to BadgerDB transactions
3. **Metrics Collection**: Export BadgerDB metrics to Prometheus
4. **Configuration Management**: Dynamic configuration updates where possible

### Optimization Strategy
1. **Tuning**: Optimize LSM-tree levels and compaction for our workload
2. **Monitoring**: Detailed metrics on compaction, cache hit rates, and performance
3. **Memory Management**: Careful tuning of cache sizes and memtable configuration
4. **Backup Strategy**: Implement consistent snapshot mechanism

## Validation Criteria

The storage engine selection will be validated through:

### Performance Testing
- **Throughput**: Achieve >50K writes/sec and >200K reads/sec
- **Latency**: P99 latency under 10ms for single operations
- **Scalability**: Linear performance scaling with data size up to 1TB
- **Memory Efficiency**: Stable memory usage under load

### Operational Testing
- **Crash Recovery**: Fast recovery times (< 30 seconds for 100GB database)
- **Backup/Restore**: Consistent snapshots without service interruption
- **Monitoring**: Complete observability of storage metrics
- **Maintenance**: Online compaction without significant performance impact

### Long-term Validation
- **Stability**: 99.9%+ uptime in production environments
- **Data Integrity**: Zero data corruption incidents
- **Performance Regression**: Consistent performance over time
- **Operational Simplicity**: Minimal manual intervention required

## Migration Strategy

If BadgerDB proves insufficient, we have a clear migration path:

1. **Interface Abstraction**: Storage interface allows engine swapping
2. **Data Export**: Implement export to universal format (JSON, Protocol Buffers)
3. **Parallel Testing**: Run multiple engines side-by-side for validation
4. **Gradual Migration**: Per-node replacement with data replication
5. **Rollback Plan**: Ability to revert to previous engine if needed

## References

- [BadgerDB Documentation](https://dgraph.io/docs/badger/) - Official documentation
- [BadgerDB Paper](https://blog.dgraph.io/post/badger/) - Design rationale and benchmarks
- [LSM-Tree Paper](http://www.cs.umb.edu/~poneil/lsmtree.pdf) - "The Log-Structured Merge-Tree"
- [RocksDB Tuning Guide](https://github.com/facebook/rocksdb/wiki/RocksDB-Tuning-Guide) - Performance optimization reference
- [YCSB Benchmarks](https://github.com/brianfrankcooper/YCSB) - Standard database benchmarking suite