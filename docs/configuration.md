# Configuration Guide

This document explains how to configure the distributed key-value store.

## Configuration Sources

The application loads configuration from multiple sources in the following order of precedence:

1. **Default values** - Built-in sensible defaults
2. **Configuration file** - YAML file (specified via command line or environment)
3. **Environment variables** - Override any configuration value

## Configuration File

Create a YAML file with your configuration. Example:

```yaml
server:
  host: "localhost"
  port: 8080
  grpc_port: 9090
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  max_body_size: 1048576

storage:
  engine: "badger"
  data_path: "./data/badger"
  in_memory: false
  sync_writes: false
  value_log_gc: true
  gc_interval: "5m"
  backup_path: "./backups"
  max_file_size: 67108864

cluster:
  node_id: "node1"
  peers: []
  raft_port: 7000
  raft_dir: "./data/raft"
  snapshot_count: 10000
  heartbeat_tick: "1s"
  election_tick: "10s"
  max_snapshots: 5
  join_timeout: "10s"

logging:
  level: "info"
  format: "json"
  output: "stdout"

metrics:
  enabled: true
  port: 2112
  path: "/metrics"

security:
  tls_enabled: false
  cert_file: ""
  key_file: ""
  ca_file: ""
  auth_enabled: false
  auth_token: ""
```

## Environment Variables

You can override any configuration value using environment variables with the `KV_` prefix:

### Server Configuration
- `KV_SERVER_HOST` - Server host address
- `KV_SERVER_PORT` - HTTP server port
- `KV_SERVER_GRPC_PORT` - gRPC server port

### Storage Configuration
- `KV_STORAGE_ENGINE` - Storage engine type (currently only "badger")
- `KV_STORAGE_DATA_PATH` - Path to store data files
- `KV_STORAGE_IN_MEMORY` - Use in-memory storage (true/false)
- `KV_STORAGE_SYNC_WRITES` - Enable synchronous writes (true/false)

### Cluster Configuration
- `KV_CLUSTER_NODE_ID` - Unique node identifier
- `KV_CLUSTER_PEERS` - Comma-separated list of peer addresses
- `KV_CLUSTER_RAFT_PORT` - Raft consensus protocol port

### Logging Configuration
- `KV_LOG_LEVEL` - Log level (debug, info, warn, error, fatal)
- `KV_LOG_FORMAT` - Log format (json, text, console)

### Metrics Configuration
- `KV_METRICS_ENABLED` - Enable metrics collection (true/false)
- `KV_METRICS_PORT` - Metrics server port

### Security Configuration
- `KV_SECURITY_TLS_ENABLED` - Enable TLS (true/false)
- `KV_SECURITY_CERT_FILE` - TLS certificate file path
- `KV_SECURITY_KEY_FILE` - TLS private key file path
- `KV_SECURITY_AUTH_TOKEN` - Authentication token

## Configuration Sections

### Server
Controls HTTP and gRPC server settings:
- **host**: Bind address (default: "localhost")
- **port**: HTTP server port (default: 8080)
- **grpc_port**: gRPC server port (default: 9090)
- **read_timeout**: Maximum request read time
- **write_timeout**: Maximum response write time
- **idle_timeout**: Maximum idle connection time
- **max_body_size**: Maximum request body size in bytes

### Storage
Controls storage engine behavior:
- **engine**: Storage backend ("badger" only currently)
- **data_path**: Directory for data files
- **in_memory**: Use memory-only storage for testing
- **sync_writes**: Ensure writes are flushed to disk
- **value_log_gc**: Enable garbage collection
- **gc_interval**: How often to run garbage collection
- **backup_path**: Directory for backup files
- **max_file_size**: Maximum size of storage files

### Cluster
Controls distributed system behavior:
- **node_id**: Unique identifier for this node
- **peers**: List of other nodes in the cluster
- **raft_port**: Port for Raft consensus communication
- **raft_dir**: Directory for Raft state files
- **snapshot_count**: How many log entries before snapshot
- **heartbeat_tick**: Raft heartbeat interval
- **election_tick**: Raft election timeout
- **max_snapshots**: Maximum snapshots to retain
- **join_timeout**: Timeout for joining cluster

### Logging
Controls application logging:
- **level**: Minimum log level to output
- **format**: Log output format
- **output**: Where to send logs (stdout, stderr, or file path)

### Metrics
Controls metrics collection:
- **enabled**: Whether to collect metrics
- **port**: Port for metrics HTTP endpoint
- **path**: URL path for metrics endpoint

### Security
Controls security features:
- **tls_enabled**: Enable TLS encryption
- **cert_file**: TLS certificate file
- **key_file**: TLS private key file
- **ca_file**: Certificate authority file
- **auth_enabled**: Enable authentication
- **auth_token**: Authentication token for API access

## Validation

The configuration system validates all settings on startup:

- Port numbers must be valid (1-65535) and unique
- File paths must exist when required (TLS certificates)
- Timeouts must be positive durations
- Log levels and formats must be valid values
- Storage engine must be supported

## Examples

### Development Configuration
```yaml
server:
  host: "localhost"
  port: 8080
storage:
  in_memory: true
logging:
  level: "debug"
  format: "console"
```

### Production Configuration
```yaml
server:
  host: "0.0.0.0"
  port: 8080
storage:
  data_path: "/var/lib/kvstore"
  sync_writes: true
  value_log_gc: true
cluster:
  node_id: "prod-node-1"
  peers: ["prod-node-2:7000", "prod-node-3:7000"]
security:
  tls_enabled: true
  cert_file: "/etc/ssl/certs/kvstore.crt"
  key_file: "/etc/ssl/private/kvstore.key"
logging:
  level: "info"
  format: "json"
  output: "/var/log/kvstore.log"
```

### Environment Variables Example
```bash
export KV_SERVER_HOST="0.0.0.0"
export KV_SERVER_PORT="9000"
export KV_STORAGE_DATA_PATH="/data/kvstore"
export KV_CLUSTER_NODE_ID="node-$(hostname)"
export KV_LOG_LEVEL="info"
```