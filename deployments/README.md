# Deployment Guide for Distributed KV Store

This directory contains all the configuration files and deployment manifests for running the distributed key-value store in various environments.

## Quick Start

### Development Environment

```bash
# Start all services
./scripts/docker-compose-dev.sh up

# View logs
./scripts/docker-compose-dev.sh logs

# Stop services
./scripts/docker-compose-dev.sh down
```

### Production Environment

```bash
# Build production image
./scripts/docker-build.sh

# Start production environment
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## Services

### Core Services

1. **KV Store** - Main application
   - HTTP API: `:8080`
   - gRPC API: `:9090`
   - Metrics: `:2112`
   - Raft: `:7000`

2. **Prometheus** - Metrics collection
   - UI: `:9090`
   - Scrapes metrics from KV Store every 10s

3. **Grafana** - Metrics visualization
   - UI: `:3000`
   - Default credentials: admin/admin (dev), admin/dev (development)

4. **Jaeger** - Distributed tracing
   - UI: `:16686`
   - Collector: `:14268`

### Supporting Services

5. **Nginx** - Load balancer and reverse proxy
   - HTTP: `:80`
   - Routes traffic to KV Store instances

6. **Redis** - For benchmarking and comparison
   - Port: `:6379`

7. **Health Monitor** - Continuous health checking
   - Monitors all services every 60 seconds
   - Outputs health status to logs

## Health Checks

All services include comprehensive health checks:

- **KV Store**: HTTP GET `/health` every 30s
- **Prometheus**: HTTP GET `/-/healthy` every 30s  
- **Grafana**: HTTP GET `/api/health` every 30s
- **Jaeger**: HTTP GET `/` every 30s
- **Redis**: `redis-cli ping` every 30s

## Configuration Files

- `prometheus.yml` - Prometheus scraping configuration
- `nginx.conf` - Nginx load balancer configuration
- `grafana/datasources/` - Grafana data source definitions
- `grafana/dashboards/` - Grafana dashboard provisioning

## Docker Images

### Optimizations

- Multi-stage build reduces final image size
- Non-root user for security
- Alpine base for minimal attack surface
- Static compilation for no external dependencies

### Build Arguments

- `BUILD_DATE` - When the image was built
- `GIT_COMMIT` - Git commit hash
- `VERSION` - Application version

## Environment Variables

### KV Store

- `KV_SERVER_HOST` - Server bind address (default: localhost)
- `KV_LOG_LEVEL` - Logging level (debug, info, warn, error)
- `KV_LOG_FORMAT` - Log format (json, console)
- `KV_METRICS_ENABLED` - Enable metrics collection
- `KVSTORE_ENV` - Environment (development, production)

### Grafana

- `GF_SECURITY_ADMIN_PASSWORD` - Admin password
- `GF_SECURITY_SECRET_KEY` - Secret key for signing
- `GF_USERS_ALLOW_SIGN_UP` - Allow user registration

## Volumes

- `kvstore_data` - Persistent storage for KV Store data
- `prometheus_data` - Prometheus metrics storage
- `grafana_data` - Grafana configuration and dashboards
- `redis_data` - Redis persistence

## Networks

- `kvstore-network` - Bridge network (172.20.0.0/16)
- All services communicate over this isolated network

## Monitoring and Observability

### Metrics Available

- HTTP request metrics (rate, duration, errors)
- Storage metrics (size, entries, operations)
- System metrics (memory, goroutines, GC)
- Custom business metrics

### Traces Available

- HTTP request traces
- Storage operation traces
- gRPC call traces
- Cross-service distributed traces

### Dashboards

- System overview
- API performance
- Storage performance
- Error rates and alerting

## Scaling

### Horizontal Scaling

To run multiple KV Store instances:

```yaml
services:
  kvstore:
    deploy:
      replicas: 3
```

### Resource Limits

Production configuration includes:
- Memory limits (512MB per service)
- CPU limits (0.5 cores per service)
- Restart policies
- Health check configuration

## Security

- All services run as non-root users
- Network isolation via Docker networks
- No sensitive data in environment variables
- TLS termination at load balancer level

## Troubleshooting

### Common Issues

1. **Service won't start**: Check logs with `docker-compose logs <service>`
2. **Cannot connect**: Verify ports and firewall settings
3. **High memory usage**: Check resource limits in docker-compose files
4. **Data loss**: Ensure volumes are properly mounted

### Debug Commands

```bash
# Check service status
docker-compose ps

# View service logs
docker-compose logs -f kvstore

# Execute commands in container
docker-compose exec kvstore sh

# Check resource usage
docker stats

# Inspect network
docker network inspect distributed-kvstore_kvstore-network
```