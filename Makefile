# Distributed Key-Value Store Makefile

.PHONY: help build test clean proto deps lint fmt vet install

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: proto ## Build all binaries
	@echo "Building server..."
	@go build -o bin/kvstore-server cmd/server/main.go
	@echo "Building client..."
	@go build -o bin/kvstore-client cmd/client/main.go
	@echo "Building kvtool..."
	@go build -o bin/kvtool cmd/kvtool/main.go
	@echo "Build completed!"

install: ## Install binaries to GOPATH/bin
	@echo "Installing server..."
	@go install ./cmd/server
	@echo "Installing client..."
	@go install ./cmd/client
	@echo "Installation completed!"

# Development targets
deps: ## Download and tidy dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies updated!"

proto: ## Generate Go code from Protocol Buffer definitions
	@echo "Generating protobuf code..."
	@chmod +x scripts/generate-proto.sh
	@export PATH=$$PATH:$$(go env GOPATH)/bin && ./scripts/generate-proto.sh

# Code quality targets
fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

lint: ## Run golint (requires golint to be installed)
	@echo "Running golint..."
	@if command -v golint >/dev/null 2>&1; then \
		golint ./...; \
	else \
		echo "golint not found. Install with: go install golang.org/x/lint/golint@latest"; \
	fi

# Testing targets
test: ## Run all tests
	@echo "Running tests..."
	@go test ./internal/storage ./internal/config -v

test-all: ## Run all tests including empty packages
	@echo "Running all tests..."
	@go test ./... -v

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test ./internal/storage ./internal/config -cover

test-coverage-html: ## Generate HTML coverage report
	@echo "Generating coverage report..."
	@go test ./internal/storage ./internal/config -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Benchmarking
benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	@go test ./benchmarks -bench=. -benchmem

# Development server
dev-server: proto ## Run development server
	@echo "Starting development server..."
	@go run cmd/server/main.go -config config.yaml

dev-client: ## Run development client
	@echo "Starting development client..."
	@go run cmd/client/main.go

# Cleanup targets
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "Cleanup completed!"

clean-proto: ## Clean generated protobuf files
	@echo "Cleaning generated protobuf files..."
	@find proto -name "*.pb.go" -type f -delete
	@echo "Protobuf files cleaned!"

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t distributed-kvstore .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	@docker run -p 8080:8080 -p 9090:9090 distributed-kvstore

# Development environment
dev-setup: deps proto ## Setup development environment
	@echo "Setting up development environment..."
	@go install golang.org/x/lint/golint@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Development environment setup completed!"

# Release targets
release: clean deps proto test build ## Build release binaries
	@echo "Building release..."
	@mkdir -p release
	@cp bin/* release/
	@echo "Release built in release/ directory"

# Check targets
check: fmt vet test ## Run all checks (format, vet, test)
	@echo "All checks passed!"

# CI targets
ci: deps proto fmt vet test-coverage ## Run CI pipeline
	@echo "CI pipeline completed!"