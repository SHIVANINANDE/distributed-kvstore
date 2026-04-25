# Distributed Key-Value Store

A **high-performance, distributed key-value store** built with Go, featuring BadgerDB storage, Raft consensus, LRU caching, and a modern React dashboard. Designed for production workloads with comprehensive monitoring, structured logging, and cloud-native deployment support.

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    React Dashboard (Vite)                     │
│              Dashboard · Key Explorer · Health                │
└────────────────────────┬─────────────────────────────────────┘
                         │  REST API
┌────────────────────────▼─────────────────────────────────────┐
│                   Nginx Load Balancer                         │
└────────────────────────┬─────────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────────┐
│                   KVStore Server Node                         │
│  ┌──────────┐  ┌──────────┐  ┌────────────┐  ┌───────────┐  │
│  │ REST API │  │ gRPC API │  │ Monitoring │  │ Structured│  │
│  │ (Mux)    │  │          │  │ Prometheus │  │  Logging  │  │
│  └────┬─────┘  └────┬─────┘  └────────────┘  └───────────┘  │
│       └──────┬───────┘                                       │
│        ┌─────▼──────┐                                        │
│        │ LRU Cache  │  ← In-memory, TTL, 10K entries         │
│        └─────┬──────┘                                        │
│        ┌─────▼──────┐                                        │
│        │ BadgerDB   │  ← LSM-tree, WAL, Value-log GC        │
│        └────────────┘                                        │
│  ┌──────────────────┐                                        │
│  │ Raft Consensus   │  ← Leader election, log replication    │
│  └──────────────────┘                                        │
└──────────────────────────────────────────────────────────────┘
         │
┌────────▼─────────────────────────────────────────────────────┐
│  Observability: Prometheus · Grafana · Jaeger                 │
└──────────────────────────────────────────────────────────────┘
```

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Storage** | BadgerDB v4 | LSM-tree based embedded KV store |
| **Caching** | Custom LRU | In-memory cache with TTL and eviction |
| **Consensus** | Raft (custom) | Leader election, log replication |
| **API** | gorilla/mux + gRPC | Dual protocol support |
| **Frontend** | React + Vite | Dashboard with real-time monitoring |
| **Monitoring** | Prometheus + Grafana | Metrics collection and visualization |
| **Tracing** | OpenTelemetry + Jaeger | Distributed tracing |
| **Logging** | Go slog (structured) | JSON-formatted structured logging |
| **Infra** | Docker, K8s, Terraform | Cloud-native deployment |

## Quick Start

### Prerequisites

- **Go 1.25+** — [install](https://go.dev/dl/)
- **Node.js 18+** — [install](https://nodejs.org/) (for frontend)
- **Docker** (optional) — for containerized deployment
- **protoc** (optional) — for regenerating protobuf code

### 1. Clone & Build

```bash
git clone https://github.com/SHIVANINANDE/distributed-kvstore.git
cd distributed-kvstore
make deps
make build
```

### 2. Run the Server

```bash
# Start with default config
./bin/kvstore-server -config config.yaml

# Or with environment overrides
KV_SERVER_HOST=0.0.0.0 KV_LOG_LEVEL=debug ./bin/kvstore-server
```

Server starts on:
- **HTTP API**: `http://localhost:8080`
- **gRPC API**: `localhost:9090`
- **Metrics**: `http://localhost:2112/metrics`

### 3. Run the Frontend

```bash
cd frontend
npm install
npm run dev
# Open http://localhost:3001
```

### 4. Docker (Full Stack)

```bash
docker-compose up -d
# KV Store:   http://localhost:8080
# Dashboard:  http://localhost:3001
# Grafana:    http://localhost:3000  (admin/admin)
# Prometheus: http://localhost:9091
# Jaeger:     http://localhost:16686
```

## API Reference

### Key-Value Operations

```bash
# Create / Update
curl -X PUT http://localhost:8080/api/v1/kv/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "hello world"}'
# Response: {"success": true}

# Read
curl http://localhost:8080/api/v1/kv/mykey
# Response: {"found": true, "value": "hello world"}

# Delete
curl -X DELETE http://localhost:8080/api/v1/kv/mykey
# Response: {"success": true, "existed": true}

# Check existence
curl -I http://localhost:8080/api/v1/kv/mykey

# List with prefix
curl "http://localhost:8080/api/v1/kv?prefix=user:&limit=50"
```

### Batch Operations

```bash
# Batch PUT
curl -X POST http://localhost:8080/api/v1/kv/batch/put \
  -H "Content-Type: application/json" \
  -d '{"items": [{"key":"a","value":"1"}, {"key":"b","value":"2"}]}'

# Batch GET
curl -X POST http://localhost:8080/api/v1/kv/batch/get \
  -H "Content-Type: application/json" \
  -d '{"keys": ["a", "b", "c"]}'

# Batch DELETE
curl -X POST http://localhost:8080/api/v1/kv/batch/delete \
  -H "Content-Type: application/json" \
  -d '{"keys": ["a", "b"]}'
```

### Health & Stats

```bash
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/stats?details=true
```

Full OpenAPI specification: [`docs/openapi.yaml`](docs/openapi.yaml)

## Performance

### Benchmark Results

*Measured on Apple M1, macOS — Go benchmarks with BadgerDB*

| Operation | Throughput | P50 Latency | P95 Latency | P99 Latency |
|-----------|-----------|-------------|-------------|-------------|
| **PUT (1KB)** | ~200K ops/sec | 5.3 μs | 15.2 μs | 23.4 μs |
| **GET (cache hit)** | ~500K+ ops/sec | 0.5 μs | 1.5 μs | 3.0 μs |
| **GET (cache miss)** | ~500K ops/sec | 1.1 μs | 3.0 μs | 5.5 μs |
| **Batch PUT (100)** | ~2M items/sec | 50 μs/batch | — | — |

**HTTP API Latency** (includes serialization + network):

| Operation | P50 | P95 | P99 |
|-----------|-----|-----|-----|
| PUT | ~0.5 ms | ~2 ms | ~5 ms |
| GET | ~0.3 ms | ~1 ms | ~3 ms |
| Batch (100 items) | ~5 ms | ~15 ms | ~30 ms |

### Run Benchmarks

```bash
# Go benchmarks (storage layer)
make benchmark

# Performance tests
make benchmark-performance

# k6 load test (requires k6: brew install k6)
# Start the server first, then:
k6 run tests/k6_load_test.js

# k6 with custom VUs and duration
k6 run --vus 50 --duration 60s tests/k6_load_test.js
```

## Testing

```bash
# Unit tests
make test

# All tests with coverage
make test-coverage

# HTML coverage report
make test-coverage-html
# Open coverage.html in browser

# Property-based tests (gopter)
make test-property

# Coverage threshold check (75% minimum)
make test-coverage-threshold
```

### Test Types

| Type | Command | Description |
|------|---------|-------------|
| Unit | `make test` | Storage, config, API handler tests |
| Integration | `go test ./tests/...` | End-to-end API testing |
| Property | `make test-property` | Randomized property-based tests |
| Benchmark | `make benchmark` | Performance measurement |
| Load | `k6 run tests/k6_load_test.js` | Concurrent load testing |

## Configuration

Configuration is loaded from YAML and can be overridden with environment variables:

| Environment Variable | Config Key | Default |
|---------------------|-----------|---------|
| `KV_SERVER_HOST` | `server.host` | `localhost` |
| `KV_SERVER_PORT` | `server.port` | `8080` |
| `KV_SERVER_GRPC_PORT` | `server.grpc_port` | `9090` |
| `KV_STORAGE_DATA_PATH` | `storage.data_path` | `./data/badger` |
| `KV_STORAGE_IN_MEMORY` | `storage.in_memory` | `false` |
| `KV_LOG_LEVEL` | `logging.level` | `info` |
| `KV_LOG_FORMAT` | `logging.format` | `json` |
| `KV_METRICS_ENABLED` | `metrics.enabled` | `true` |

See [`docs/configuration.md`](docs/configuration.md) for full reference.

## Deployment

### Kubernetes

```bash
kubectl apply -f k8s/deploy.yaml
```

Helm chart and operator available in [`k8s/`](k8s/).

### Terraform (AWS)

```bash
cd terraform/environments/production
terraform init && terraform apply
```

## Monitoring

The system includes pre-configured monitoring:

- **Prometheus** — Metrics collection at `/metrics`
- **Grafana** — Pre-built dashboards for cluster health, performance, and capacity
- **Jaeger** — Distributed tracing with OpenTelemetry
- **Health checks** — Storage, memory, goroutine monitoring

Key metrics exposed:
```
kvstore_requests_total
kvstore_request_duration_seconds
kvstore_storage_size_bytes
kvstore_storage_operations_total
kvstore_memory_usage_bytes
kvstore_goroutines
```

## Project Structure

```
├── cmd/
│   ├── server/          # Server entry point
│   ├── client/          # CLI client
│   └── kvtool/          # Admin tool
├── internal/
│   ├── api/             # REST handlers, routing
│   ├── cache/           # LRU cache with TTL
│   ├── cluster/         # Cluster management
│   ├── config/          # Configuration loading & validation
│   ├── consensus/       # Raft consensus implementation
│   ├── logging/         # Structured logging (slog)
│   ├── monitoring/      # Metrics, health checks, dashboard
│   ├── security/        # TLS, RBAC, rate limiting
│   ├── server/          # HTTP + gRPC server wiring
│   └── storage/         # BadgerDB engine, cached storage
├── frontend/            # React dashboard (Vite)
├── proto/               # Protocol Buffer definitions
├── tests/               # Integration & load tests
├── benchmarks/          # Go benchmarks
├── deployments/         # Prometheus, Grafana, Nginx configs
├── k8s/                 # Kubernetes manifests
├── terraform/           # Infrastructure as Code
├── monitoring/          # Alertmanager, Grafana dashboards
├── docs/                # API docs, system design, ADRs
└── .github/workflows/   # CI/CD pipelines
```

## Future Roadmap

- [ ] **TTL support at API level** — Per-key expiration via PUT request
- [ ] **Watch/Subscribe API** — Server-Sent Events for key change notifications
- [ ] **Multi-region replication** — Cross-datacenter data sync
- [ ] **Admin UI improvements** — Config editor, cluster topology visualization
- [ ] **Rate limiting middleware** — Token bucket with configurable limits
- [ ] **Client SDK** — Published Go, Python, and JavaScript client libraries

## Author

**Shivani Nande**
- GitHub: [@SHIVANINANDE](https://github.com/SHIVANINANDE)
- Email: shivaninandee@gmail.com

## License

MIT License — see [LICENSE](LICENSE) for details.
