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

lint: ## Run golangci-lint with strict rules
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

lint-fix: ## Run golangci-lint with auto-fix
	@echo "Running golangci-lint with auto-fix..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --fix; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
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
	@go test ./internal/api ./internal/config ./internal/logging ./internal/monitoring ./internal/storage ./tests -cover

test-coverage-report: ## Generate comprehensive coverage report
	@echo "Generating comprehensive coverage report..."
	@go test ./internal/api ./internal/config ./internal/logging ./internal/monitoring ./internal/storage ./tests -coverprofile=coverage.out
	@go tool cover -func=coverage.out
	@echo "Function coverage report completed!"

test-coverage-html: ## Generate HTML coverage report
	@echo "Generating HTML coverage report..."
	@go test ./internal/api ./internal/config ./internal/logging ./internal/monitoring ./internal/storage ./tests -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-coverage-detailed: ## Generate detailed coverage with per-package breakdown
	@echo "Generating detailed coverage report..."
	@echo "=== API Package Coverage ===" > coverage-detailed.txt
	@go test ./internal/api -cover -coverprofile=api.out >> coverage-detailed.txt 2>&1
	@echo "" >> coverage-detailed.txt
	@echo "=== Config Package Coverage ===" >> coverage-detailed.txt
	@go test ./internal/config -cover -coverprofile=config.out >> coverage-detailed.txt 2>&1
	@echo "" >> coverage-detailed.txt
	@echo "=== Logging Package Coverage ===" >> coverage-detailed.txt
	@go test ./internal/logging -cover -coverprofile=logging.out >> coverage-detailed.txt 2>&1
	@echo "" >> coverage-detailed.txt
	@echo "=== Monitoring Package Coverage ===" >> coverage-detailed.txt
	@go test ./internal/monitoring -cover -coverprofile=monitoring.out >> coverage-detailed.txt 2>&1
	@echo "" >> coverage-detailed.txt
	@echo "=== Storage Package Coverage ===" >> coverage-detailed.txt
	@go test ./internal/storage -cover -coverprofile=storage.out >> coverage-detailed.txt 2>&1
	@echo "" >> coverage-detailed.txt
	@echo "=== Integration Tests Coverage ===" >> coverage-detailed.txt
	@go test ./tests -cover -coverprofile=tests.out >> coverage-detailed.txt 2>&1
	@echo "Detailed coverage report generated: coverage-detailed.txt"

test-coverage-threshold: ## Check if coverage meets minimum threshold (75%)
	@echo "Checking coverage threshold..."
	@go test ./internal/api ./internal/config ./internal/logging ./internal/monitoring ./internal/storage ./tests -coverprofile=coverage.out > /dev/null 2>&1; \
	COVERAGE=$$(go tool cover -func=coverage.out | grep "total:" | awk '{print $$3}' | sed 's/%//'); \
	if [ -n "$$COVERAGE" ]; then \
		echo "Overall coverage: $$COVERAGE%"; \
		if [ $$(echo "$$COVERAGE >= 75" | bc -l 2>/dev/null || echo "0") -eq 1 ]; then \
			echo "✅ Coverage threshold met (75%)"; \
		else \
			echo "❌ Coverage below threshold (75%)"; \
			exit 1; \
		fi; \
	else \
		echo "❌ Could not determine coverage"; \
		exit 1; \
	fi

test-coverage-ci: ## Coverage target for CI/CD (with JSON output)
	@echo "Running coverage for CI/CD..."
	@go test ./internal/api ./internal/config ./internal/logging ./internal/monitoring ./internal/storage ./tests -coverprofile=coverage.out -json > test-results.json
	@go tool cover -func=coverage.out > coverage-summary.txt
	@echo "CI coverage reports generated: test-results.json, coverage-summary.txt"

test-property: ## Run property-based tests with gopter
	@echo "Running property-based tests..."
	@go test ./internal/storage -run TestStorageEngineProperty -v
	@go test ./internal/api -run TestAPIProperties -v

# Benchmarking
benchmark: ## Run all benchmarks
	@echo "Running all benchmarks..."
	@go test ./benchmarks -bench=. -benchmem

benchmark-storage: ## Run storage engine benchmarks
	@echo "Running storage benchmarks..."
	@go test ./benchmarks -bench=BenchmarkStorageEngine -benchmem

benchmark-api: ## Run API benchmarks
	@echo "Running API benchmarks..."
	@go test ./benchmarks -bench=BenchmarkAPI -benchmem

benchmark-short: ## Run quick benchmarks (1 second each)
	@echo "Running quick benchmarks..."
	@go test ./benchmarks -bench=. -benchtime=1s

benchmark-performance: ## Run performance tests
	@echo "Running performance tests..."
	@go test ./benchmarks -run TestPerformance -v

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
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Development environment setup completed!"

setup-hooks: ## Setup pre-commit hooks
	@echo "Setting up pre-commit hooks..."
	@chmod +x scripts/setup-hooks.sh
	@./scripts/setup-hooks.sh

pre-commit: ## Run pre-commit hooks manually
	@echo "Running pre-commit hooks..."
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit run --all-files; \
	else \
		echo "pre-commit not found. Run 'make setup-hooks' first"; \
	fi

# Release targets
release: clean deps proto test build ## Build release binaries
	@echo "Building release..."
	@mkdir -p release
	@cp bin/* release/
	@echo "Release built in release/ directory"

# Check targets
check: fmt vet lint test ## Run all checks (format, vet, lint, test)
	@echo "All checks passed!"

# CI targets
ci: deps proto fmt vet lint test-coverage-threshold ## Run CI pipeline
	@echo "CI pipeline completed!"

ci-full: deps proto fmt vet lint test-coverage-ci ## Run full CI pipeline with reports
	@echo "Full CI pipeline completed!"