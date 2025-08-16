# Multi-stage build for optimized production image
FROM golang:1.21-alpine AS builder

# Install git and other build dependencies
RUN apk add --no-cache git protobuf-dev protoc

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate protobuf files
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    export PATH=$PATH:$(go env GOPATH)/bin && \
    ./scripts/generate-proto.sh

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o kvstore-server \
    ./cmd/server

# Build additional tools
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o kvstore-client \
    ./cmd/client

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o kvtool \
    ./cmd/kvtool

# Production stage - minimal image
FROM alpine:3.18

# Install ca-certificates for HTTPS requests and create user
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1001 -S kvstore && \
    adduser -u 1001 -S kvstore -G kvstore

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/kvstore-server /app/kvstore-client /app/kvtool ./
COPY --from=builder /app/config.yaml ./

# Create necessary directories with proper permissions
RUN mkdir -p /app/data /app/logs && \
    chown -R kvstore:kvstore /app

# Switch to non-root user
USER kvstore

# Expose ports
EXPOSE 8080 9090 2112 7000

# Add health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set default command
CMD ["./kvstore-server", "-config", "config.yaml"]

# Add labels for better image management
LABEL maintainer="distributed-kvstore"
LABEL version="1.0.0"
LABEL description="Distributed Key-Value Store with monitoring and observability"
LABEL org.opencontainers.image.source="https://github.com/user/distributed-kvstore"
LABEL org.opencontainers.image.title="Distributed KV Store"
LABEL org.opencontainers.image.description="Production-ready distributed key-value store"
LABEL org.opencontainers.image.vendor="KVStore Team"