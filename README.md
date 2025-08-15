# Distributed Key-Value Store

A high-performance, distributed key-value store built with Go, featuring BadgerDB storage engine and Raft consensus for data consistency across multiple nodes.

## Features

- **Fast Storage**: BadgerDB-based storage engine with LSM-tree architecture
- **Distributed**: Raft consensus algorithm for data consistency
- **Configurable**: Flexible configuration via files and environment variables
- **Secure**: TLS support and authentication options
- **Observable**: Built-in metrics and structured logging
- **Scalable**: Horizontal scaling with cluster management

## Quick Start

### Prerequisites

- Go 1.21 or later
- Make (optional, for build automation)

### Installation

```bash
# Clone the repository
git clone <repository-url>
cd distributed-kvstore

# Download dependencies
go mod download

# Build the server
go build -o bin/kvstore-server cmd/server/main.go

# Build the client
go build -o bin/kvstore-client cmd/client/main.go
```

### Running the Server

```bash
# Start with default configuration
./bin/kvstore-server

# Start with custom config file
./bin/kvstore-server -config config.yaml

# Start with environment variables
KV_SERVER_PORT=9000 KV_STORAGE_IN_MEMORY=true ./bin/kvstore-server
```

### Basic Usage

```bash
# Set a key-value pair
./bin/kvstore-client set mykey "Hello World"

# Get a value
./bin/kvstore-client get mykey

# Delete a key
./bin/kvstore-client delete mykey

# List keys with prefix
./bin/kvstore-client list user:
```

## Project Structure

```
distributed-kvstore/
├── cmd/                    # Application entry points
│   ├── server/            # Main server application
│   └── client/            # Client CLI tool
├── internal/              # Private application code
│   ├── consensus/         # Raft implementation
│   ├── storage/           # Storage engine
│   ├── network/           # Networking layer
│   ├── api/               # REST/gRPC APIs
│   ├── cluster/           # Cluster management
│   └── config/            # Configuration management
├── pkg/                   # Public packages
├── web/                   # Frontend applications
├── scripts/               # Build and deployment scripts
├── deployments/           # Kubernetes manifests
├── docs/                  # Documentation
├── tests/                 # Integration tests
└── benchmarks/           # Performance tests
```

## Configuration

The application supports multiple configuration methods:

1. **Default values** - Sensible defaults for development
2. **Configuration files** - YAML configuration files
3. **Environment variables** - Runtime configuration overrides

See [Configuration Guide](docs/configuration.md) for detailed configuration options.

### Example Configuration

```yaml
server:
  host: "localhost"
  port: 8080
  grpc_port: 9090

storage:
  engine: "badger"
  data_path: "./data/badger"
  in_memory: false

cluster:
  node_id: "node1"
  peers: []
  raft_port: 7000

logging:
  level: "info"
  format: "json"

metrics:
  enabled: true
  port: 2112
```

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/storage -v
go test ./internal/config -v
```

### Code Structure

- **Storage Engine**: BadgerDB-based with interface for flexibility
- **Configuration**: Type-safe configuration with validation
- **Testing**: Comprehensive unit tests for all components

### Current Implementation Status

✅ **Completed Components:**
- Storage engine with BadgerDB
- Configuration system with validation
- Comprehensive test coverage

🚧 **In Progress:**
- Raft consensus implementation
- gRPC API layer
- Cluster management

📋 **Planned:**
- REST API
- Client library
- Web interface
- Deployment automation

## Documentation

- [Configuration Guide](docs/configuration.md) - Complete configuration reference
- [API Documentation](docs/api.md) - REST and gRPC API reference
- [Architecture](docs/architecture.md) - System design and architecture
- [Deployment](docs/deployment.md) - Deployment guides and best practices

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.