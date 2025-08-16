# Redis Performance Benchmarks

This document describes how to run performance benchmarks comparing our distributed key-value store with Redis.

## Prerequisites

1. **Redis Server**: You need Redis running locally on `localhost:6379`
   ```bash
   # Install Redis (macOS)
   brew install redis
   redis-server
   
   # Install Redis (Ubuntu/Debian)
   sudo apt-get install redis-server
   sudo systemctl start redis-server
   
   # Install Redis (Docker)
   docker run -d -p 6379:6379 redis:latest
   ```

2. **Go Dependencies**: Ensure all Go dependencies are installed
   ```bash
   go mod tidy
   ```

## Running Benchmarks

### Basic Benchmark Comparison
Run comprehensive benchmarks comparing single operations:
```bash
go test ./benchmarks -bench=BenchmarkVsRedis_SingleOperations -v
```

### Batch Operations Comparison
Compare batch operation performance:
```bash
go test ./benchmarks -bench=BenchmarkVsRedis_BatchOperations -v
```

### Concurrent Load Testing
Test concurrent read/write performance:
```bash
go test ./benchmarks -bench=BenchmarkVsRedis_ConcurrentLoad -v
```

### All Redis Benchmarks
Run all Redis comparison benchmarks:
```bash
go test ./benchmarks -bench=BenchmarkVsRedis -v
```

### Performance Summary
Get a detailed performance summary with operation counts:
```bash
go test ./benchmarks -run TestPerformanceSummary -v
```

## Benchmark Categories

### 1. Single Operations
- **PUT**: Individual key-value insertions
- **GET**: Individual key lookups  
- **DELETE**: Individual key deletions

### 2. Batch Operations
- **BATCH_PUT**: Multiple key-value insertions in single transaction
- **BATCH_GET**: Multiple key lookups
- **BATCH_DELETE**: Multiple key deletions in single transaction

### 3. Concurrent Operations
- **ConcurrentReads**: Parallel read operations
- **ConcurrentWrites**: Parallel write operations
- **MixedReadWrite**: 70% reads, 30% writes mix

## Interpreting Results

### Benchmark Output Format
```
BenchmarkVsRedis_SingleOperations/PUT_Operations/OurEngine-8    50000    25432 ns/op
BenchmarkVsRedis_SingleOperations/PUT_Operations/Redis-8       30000    35123 ns/op
```

- **50000**: Number of iterations
- **25432 ns/op**: Nanoseconds per operation
- **-8**: Number of CPU cores used

### Performance Metrics
- **Operations per second**: Higher is better
- **Nanoseconds per operation**: Lower is better
- **Memory allocations**: Lower is better
- **Concurrent throughput**: Higher is better

## Expected Performance Characteristics

### Our Engine Advantages
- **Embedded Storage**: No network overhead
- **Optimized Caching**: LRU cache with configurable TTL
- **Batch Operations**: Optimized transaction batching
- **Memory Efficiency**: Direct memory access

### Redis Advantages
- **Network Protocol**: Mature Redis protocol
- **Advanced Data Structures**: Sets, hashes, lists, etc.
- **Persistence Options**: RDB, AOF
- **Clustering**: Built-in replication and clustering

## Configuration for Benchmarks

### Our Engine Configuration
```yaml
storage:
  cache:
    enabled: true
    size: 10000
    ttl: "30m"
  wal:
    enabled: true
    threshold: 67108864  # 64MB
```

### Redis Configuration
```
# redis.conf optimizations for benchmarks
save ""                    # Disable snapshotting
appendonly no             # Disable AOF
maxmemory-policy allkeys-lru
```

## Troubleshooting

### Redis Connection Issues
```bash
# Test Redis connection
redis-cli ping
# Should return PONG
```

### Benchmark Skipping
If Redis is not available, benchmarks will be skipped with a message like:
```
Redis not available for comparison: dial tcp 127.0.0.1:6379: connection refused
```

### Memory Issues
For large benchmarks, you may need to increase limits:
```bash
# Increase Go test timeout
go test ./benchmarks -bench=BenchmarkVsRedis -timeout 30m

# Run with memory profiling
go test ./benchmarks -bench=BenchmarkVsRedis -memprofile=mem.prof
```

## Sample Results

Example benchmark results on a typical development machine:

```
=== Performance Comparison Summary ===
PUT Operations (10000 ops):
  Our Engine: 45.2ms (221,238 ops/sec)
  Redis:      72.1ms (138,696 ops/sec)
  Ratio:      1.59x (our engine vs Redis)

GET Operations (10000 ops):
  Our Engine: 12.3ms (813,008 ops/sec)
  Redis:      28.7ms (348,432 ops/sec)
  Ratio:      2.33x (our engine vs Redis)
```

*Note: Actual results will vary based on hardware, configuration, and workload patterns.*