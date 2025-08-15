# KVStore Monitoring Guide

## Overview

The KVStore includes comprehensive monitoring capabilities with health checks, metrics collection, and a web dashboard. This guide covers all monitoring features and their configuration.

## Features

### 1. Health Checks
- **Storage Health**: Tests read/write operations to ensure database functionality
- **Memory Health**: Monitors memory usage and alerts on high consumption
- **Goroutine Health**: Tracks goroutine count to detect potential leaks
- **HTTP Dependency Health**: Tests external service dependencies
- **Overall Health Status**: Aggregates all checks into system-wide health

### 2. Metrics Collection
- **Request Metrics**: HTTP/gRPC request rates, response times, error rates
- **Storage Metrics**: Database size, entry count, operation performance
- **System Metrics**: Memory usage, CPU usage, goroutine count, GC performance
- **Custom Metrics**: Application-specific counters, gauges, histograms, and summaries

### 3. Prometheus Integration
- **Standard Metrics Format**: Exports metrics in Prometheus format
- **Labels and Dimensions**: Rich metadata for metric filtering and aggregation
- **Histogram and Summary Support**: Detailed latency and size distributions

### 4. Web Dashboard
- **Real-time Monitoring**: Live view of system health and performance
- **Interactive Charts**: Visual representation of key metrics
- **Alert Management**: View and manage system alerts
- **System Information**: Hardware and runtime details

## Endpoints

### Health Check
```
GET /health
GET /api/v1/health
```

**Response Format:**
```json
{
  "status": "healthy|degraded|unhealthy",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "timestamp": "2025-01-15T10:30:00Z",
  "checks": {
    "storage": {
      "name": "storage",
      "status": "healthy",
      "message": "Storage is operational",
      "duration_ms": 12,
      "timestamp": "2025-01-15T10:30:00Z",
      "details": {
        "operation_time_ms": 5,
        "total_size": 1048576,
        "entries": 150
      },
      "critical": true
    }
  },
  "summary": {
    "total": 3,
    "healthy": 3,
    "degraded": 0,
    "unhealthy": 0,
    "critical": 0
  },
  "system_info": {
    "go_version": "go1.21.0",
    "os": "darwin",
    "arch": "arm64",
    "num_cpu": 8,
    "num_goroutine": 23,
    "memory_mb": 86,
    "start_time": "2025-01-15T10:00:00Z"
  }
}
```

### Prometheus Metrics
```
GET /api/v1/metrics
```

**Sample Output:**
```
# HELP kvstore_http_requests_total Total HTTP requests
# TYPE kvstore_http_requests_total counter
kvstore_http_requests_total{method="GET",path="/api/v1/kv/test",status="200"} 42

# HELP kvstore_request_duration_seconds Request duration in seconds
# TYPE kvstore_request_duration_seconds histogram
kvstore_request_duration_seconds_bucket{le="0.005"} 100
kvstore_request_duration_seconds_bucket{le="0.01"} 120
kvstore_request_duration_seconds_bucket{le="0.025"} 150
kvstore_request_duration_seconds_sum 12.5
kvstore_request_duration_seconds_count 150

# HELP kvstore_memory_usage_bytes Current memory usage in bytes
# TYPE kvstore_memory_usage_bytes gauge
kvstore_memory_usage_bytes 90177536
```

### Web Dashboard
```
GET /dashboard
```

Access the interactive web dashboard showing:
- Real-time health status
- System statistics
- Active alerts
- Service information
- Metric summaries

## Configuration

### Basic Configuration
```yaml
metrics:
  enabled: true
  port: 2112
  path: "/metrics"
  
logging:
  level: "info"
  format: "json"
  enable_performance_log: true
```

### Health Check Configuration
Health checks are automatically configured but can be customized:

```go
// Custom memory limit (1GB)
healthManager.RegisterChecker(NewMemoryHealthChecker(1024))

// Custom goroutine limit (5000)
healthManager.RegisterChecker(NewGoroutineHealthChecker(5000))

// External HTTP dependency
healthManager.RegisterChecker(NewHTTPDependencyChecker(
    "external-service",
    "https://api.example.com/health",
    5*time.Second,
    true, // critical
))
```

### Prometheus Configuration
For Prometheus scraping, add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'kvstore'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 15s
    metrics_path: /api/v1/metrics
    
  - job_name: 'kvstore-health'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 30s
    metrics_path: /api/v1/health
```

## Available Metrics

### HTTP Metrics
- `kvstore_http_requests_total`: Total HTTP requests (counter)
- `kvstore_http_request_duration_seconds`: HTTP request duration (histogram)  
- `kvstore_http_response_size_bytes`: HTTP response size (histogram)

### Storage Metrics
- `kvstore_storage_operations_total`: Total storage operations (counter)
- `kvstore_storage_errors_total`: Storage operation errors (counter)
- `kvstore_storage_size_bytes`: Current storage size (gauge)
- `kvstore_storage_entries`: Number of stored entries (gauge)

### System Metrics
- `kvstore_memory_usage_bytes`: Memory usage (gauge)
- `kvstore_goroutines`: Goroutine count (gauge)
- `kvstore_gc_duration_seconds`: Garbage collection duration (histogram)

### Application Metrics
- `kvstore_requests_total`: Total application requests (counter)
- `kvstore_request_duration_seconds`: Request duration (histogram)
- `kvstore_request_errors_total`: Request errors (counter)

## Alerting Rules

### Prometheus Alerting Rules
Create `kvstore_alerts.yml`:

```yaml
groups:
- name: kvstore
  rules:
  - alert: KVStoreDown
    expr: up{job="kvstore"} == 0
    for: 30s
    labels:
      severity: critical
    annotations:
      summary: "KVStore instance is down"
      
  - alert: HighMemoryUsage
    expr: kvstore_memory_usage_bytes / 1024 / 1024 > 500
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "KVStore memory usage is high"
      description: "Memory usage is {{ $value }}MB"
      
  - alert: HighErrorRate
    expr: rate(kvstore_request_errors_total[5m]) / rate(kvstore_requests_total[5m]) > 0.05
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "KVStore error rate is high"
      description: "Error rate is {{ $value | humanizePercentage }}"
      
  - alert: StorageUnhealthy
    expr: kvstore_health_check{check="storage",status="unhealthy"} == 1
    for: 30s
    labels:
      severity: critical
    annotations:
      summary: "KVStore storage health check failing"
```

### Built-in Alerts
The system includes built-in alert rules:
- **HighMemoryUsage**: Triggers when memory usage exceeds 80%
- **HighErrorRate**: Triggers when error rate exceeds 5%
- **StorageUnhealthy**: Triggers when storage health check fails
- **HighGoroutineCount**: Triggers when goroutines exceed configured limit

## Grafana Dashboard

### Import Dashboard
Use the provided Grafana dashboard JSON or create custom dashboards with these queries:

**Request Rate:**
```promql
rate(kvstore_http_requests_total[5m])
```

**Response Time Percentiles:**
```promql
histogram_quantile(0.95, rate(kvstore_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.50, rate(kvstore_http_request_duration_seconds_bucket[5m]))
```

**Memory Usage:**
```promql
kvstore_memory_usage_bytes / 1024 / 1024
```

**Storage Growth:**
```promql
kvstore_storage_size_bytes
kvstore_storage_entries
```

### Dashboard Features
- Request rate and error rate graphs
- Response time percentile charts
- Memory and CPU usage monitoring
- Storage size and entry count tracking
- Health check status indicators
- System information panels

## Troubleshooting

### Health Check Issues
1. **Storage health failing**: Check database connectivity and disk space
2. **Memory health degraded**: Monitor for memory leaks, consider increasing limits
3. **High goroutine count**: Look for goroutine leaks in application code

### Metrics Issues
1. **Missing metrics**: Verify metrics endpoint is accessible
2. **Prometheus scraping failing**: Check network connectivity and endpoint URLs
3. **Dashboard not updating**: Verify Prometheus is successfully scraping metrics

### Performance Impact
- Health checks run every request with ~1ms overhead
- Metrics collection has minimal CPU impact (<1%)
- Storage overhead is negligible for typical workloads

## Integration Examples

### Docker Compose
```yaml
version: '3.8'
services:
  kvstore:
    image: kvstore:latest
    ports:
      - "8080:8080"
    environment:
      - KVSTORE_METRICS_ENABLED=true
      
  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      
  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
```

### Kubernetes
```yaml
apiVersion: v1
kind: Service
metadata:
  name: kvstore
  labels:
    app: kvstore
spec:
  ports:
  - port: 8080
    name: http
  selector:
    app: kvstore
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kvstore
spec:
  selector:
    matchLabels:
      app: kvstore
  endpoints:
  - port: http
    path: /api/v1/metrics
```

## Best Practices

1. **Monitor critical metrics**: Focus on request latency, error rates, and resource usage
2. **Set appropriate alert thresholds**: Balance between noise and missing issues
3. **Use structured logging**: Leverage correlation IDs for request tracing
4. **Regular health checks**: Monitor health endpoints in load balancers
5. **Capacity planning**: Use storage and memory trends for scaling decisions
6. **Performance baselines**: Establish normal operating ranges for alerts