# ADR-0008: Prometheus + Grafana Monitoring Stack

## Status
Accepted

Date: 2024-01-15

## Context

The distributed key-value store requires comprehensive monitoring and observability to ensure reliable operation in production environments. The monitoring system must provide:

### Operational Requirements
- **Performance Monitoring**: Latency, throughput, error rates
- **Resource Monitoring**: CPU, memory, disk, network utilization
- **Business Metrics**: Key count, storage usage, operation types
- **Cluster Health**: Node status, leader elections, consensus lag
- **Alerting**: Proactive notification of issues and anomalies

### Technical Requirements
- **Scalability**: Handle metrics from hundreds of nodes
- **Retention**: Store metrics for capacity planning and analysis
- **Query Performance**: Fast dashboard rendering and alerting
- **Integration**: Work with Kubernetes and cloud environments
- **Extensibility**: Support custom metrics and integrations

### Operational Requirements
- **Reliability**: Monitoring system must be highly available
- **Cost Efficiency**: Reasonable resource usage for monitoring overhead
- **Ease of Use**: Intuitive dashboards and alerting configuration
- **Security**: Secure metrics collection and access control

## Decision

We will implement a **Prometheus + Grafana monitoring stack** as our primary observability solution.

### Architecture Overview
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   KVStore       │    │   KVStore       │    │   KVStore       │
│   Node 1        │    │   Node 2        │    │   Node 3        │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │   Metrics   │ │    │ │   Metrics   │ │    │ │   Metrics   │ │
│ │  Endpoint   │ │    │ │  Endpoint   │ │    │ │  Endpoint   │ │
│ │ :2112/metrics│ │    │ │ :2112/metrics│ │    │ │ :2112/metrics│ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │     Prometheus          │
                    │   (Metrics Storage)     │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Alert Manager    │ │
                    │ │                    │ │
                    │ └─────────────────────┘ │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │      Grafana            │
                    │   (Visualization)       │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │    Dashboards      │ │
                    │ │                    │ │
                    │ └─────────────────────┘ │
                    └─────────────────────────┘
```

### Component Selection
1. **Prometheus**: Metrics collection, storage, and alerting
2. **Grafana**: Visualization, dashboards, and advanced alerting
3. **AlertManager**: Alert routing, grouping, and notification
4. **Node Exporter**: System-level metrics (CPU, memory, disk)
5. **Custom Exporters**: KVStore-specific metrics

## Consequences

### Positive
- **Industry Standard**: Well-established monitoring solution with large community
- **Pull-based Model**: Resilient to network issues and target failures
- **Powerful Query Language**: PromQL for complex metric analysis
- **Service Discovery**: Automatic discovery of monitoring targets
- **Rich Ecosystem**: Large library of exporters and integrations
- **Cost Effective**: Open source with reasonable resource requirements

### Negative
- **Learning Curve**: PromQL and Grafana configuration require expertise
- **Storage Requirements**: Metrics storage grows over time
- **High Cardinality Risk**: Improper metric design can cause performance issues
- **Single Point of Failure**: Prometheus server becomes critical dependency
- **Limited Long-term Storage**: Native retention is not infinite

### Resource Requirements
- **Prometheus**: 2-4 CPU cores, 4-8GB RAM per 100K active series
- **Grafana**: 1-2 CPU cores, 2-4GB RAM
- **Storage**: ~1-2 bytes per sample, configurable retention
- **Network**: ~1KB/sec per monitored target

## Implementation Details

### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'kvstore-prod'
    environment: 'production'

rule_files:
  - "rules/*.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093

scrape_configs:
  # KVStore application metrics
  - job_name: 'kvstore'
    static_configs:
      - targets: ['kvstore-1:2112', 'kvstore-2:2112', 'kvstore-3:2112']
    scrape_interval: 5s
    metrics_path: /metrics
    
  # System metrics
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['kvstore-1:9100', 'kvstore-2:9100', 'kvstore-3:9100']
    
  # Kubernetes service discovery (if applicable)
  - job_name: 'kubernetes-pods'
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
```

### KVStore Metrics Exposition

```go
package monitoring

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Request metrics
    requestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kvstore_requests_total",
            Help: "Total number of requests processed",
        },
        []string{"method", "status"},
    )
    
    requestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kvstore_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )
    
    // Storage metrics
    storageOperations = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kvstore_storage_operations_total",
            Help: "Total storage operations",
        },
        []string{"operation", "status"},
    )
    
    storageSize = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "kvstore_storage_size_bytes",
            Help: "Current storage size in bytes",
        },
    )
    
    // Raft metrics
    raftTerm = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "kvstore_raft_term",
            Help: "Current Raft term",
        },
    )
    
    raftCommitIndex = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "kvstore_raft_commit_index",
            Help: "Current Raft commit index",
        },
    )
    
    leaderElections = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "kvstore_raft_leader_elections_total",
            Help: "Total number of leader elections",
        },
    )
)

// Middleware for HTTP request metrics
func MetricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        // Wrap ResponseWriter to capture status code
        wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
        
        next.ServeHTTP(wrapped, r)
        
        duration := time.Since(start)
        method := r.Method
        status := fmt.Sprintf("%d", wrapped.statusCode)
        
        requestsTotal.WithLabelValues(method, status).Inc()
        requestDuration.WithLabelValues(method).Observe(duration.Seconds())
    })
}
```

### Grafana Dashboard Configuration

```json
{
  "dashboard": {
    "title": "KVStore Overview",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(kvstore_requests_total[5m])) by (method)",
            "legendFormat": "{{method}}"
          }
        ]
      },
      {
        "title": "Request Latency",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(kvstore_request_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          },
          {
            "expr": "histogram_quantile(0.50, rate(kvstore_request_duration_seconds_bucket[5m]))",
            "legendFormat": "50th percentile"
          }
        ]
      },
      {
        "title": "Error Rate",
        "type": "singlestat",
        "targets": [
          {
            "expr": "sum(rate(kvstore_requests_total{status=~\"4..|5..\"}[5m])) / sum(rate(kvstore_requests_total[5m])) * 100"
          }
        ]
      }
    ]
  }
}
```

### Alerting Rules

```yaml
# rules/kvstore-alerts.yml
groups:
  - name: kvstore.rules
    rules:
      # High error rate
      - alert: KVStoreHighErrorRate
        expr: >
          (
            sum(rate(kvstore_requests_total{status=~"4..|5.."}[5m])) /
            sum(rate(kvstore_requests_total[5m]))
          ) * 100 > 5
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "KVStore high error rate"
          description: "Error rate is {{ $value }}% for the last 5 minutes"
      
      # High latency
      - alert: KVStoreHighLatency
        expr: >
          histogram_quantile(0.95, 
            rate(kvstore_request_duration_seconds_bucket[5m])
          ) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "KVStore high latency"
          description: "95th percentile latency is {{ $value }}s"
      
      # Node down
      - alert: KVStoreNodeDown
        expr: up{job="kvstore"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "KVStore node down"
          description: "Node {{ $labels.instance }} is down"
      
      # Leader election
      - alert: KVStoreLeaderElection
        expr: increase(kvstore_raft_leader_elections_total[5m]) > 0
        for: 0m
        labels:
          severity: warning
        annotations:
          summary: "KVStore leader election occurred"
          description: "Leader election detected in cluster"
```

### AlertManager Configuration

```yaml
# alertmanager.yml
global:
  smtp_smarthost: 'localhost:587'
  smtp_from: 'alerts@kvstore.example.com'

route:
  group_by: ['alertname', 'cluster']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'web.hook'
  routes:
    - match:
        severity: critical
      receiver: 'critical-alerts'
      repeat_interval: 5m

receivers:
  - name: 'web.hook'
    webhook_configs:
      - url: 'http://slack-webhook/alerts'
        
  - name: 'critical-alerts'
    email_configs:
      - to: 'oncall@example.com'
        subject: '[CRITICAL] KVStore Alert'
        body: |
          {{ range .Alerts }}
          Alert: {{ .Annotations.summary }}
          Description: {{ .Annotations.description }}
          {{ end }}
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/...'
        channel: '#alerts'
        text: 'CRITICAL: {{ .GroupLabels.alertname }}'
```

## Alternatives Considered

### 1. DataDog
**Pros:**
- Managed service with no operational overhead
- Rich visualization and alerting features
- Good integration ecosystem
- Advanced APM capabilities

**Cons:**
- High cost for large-scale deployments
- Vendor lock-in
- Data leaves your infrastructure
- Limited customization for specific needs

**Verdict:** Rejected due to cost and vendor lock-in concerns

### 2. Elastic Stack (ELK)
**Pros:**
- Powerful log aggregation and search
- Good visualization with Kibana
- Full-text search capabilities
- Strong community and ecosystem

**Cons:**
- Resource intensive
- Complex to operate at scale
- Not optimized for time-series metrics
- Higher operational overhead

**Verdict:** Rejected in favor of specialized time-series solution

### 3. InfluxDB + Grafana
**Pros:**
- Purpose-built time-series database
- Good compression and performance
- SQL-like query language
- Native Grafana integration

**Cons:**
- Additional database to operate
- Clustering complexity (InfluxDB Enterprise)
- Less mature ecosystem than Prometheus
- Push-based model complexity

**Verdict:** Rejected due to operational complexity

### 4. Custom Metrics Solution
**Pros:**
- Optimized for exact use case
- Full control over features and performance
- No external dependencies
- Minimal resource usage

**Cons:**
- Significant development effort
- Limited visualization options
- No ecosystem benefits
- Maintenance burden

**Verdict:** Rejected due to development overhead

### 5. CloudWatch (AWS) / Cloud Monitoring (GCP)
**Pros:**
- Native cloud integration
- Managed service
- Pay-per-use pricing
- Built-in alerting

**Cons:**
- Cloud vendor lock-in
- Limited customization
- Higher latency for metrics
- Cost scaling issues

**Verdict:** Rejected to maintain cloud-agnostic approach

## Monitoring Best Practices

### Metric Design Principles
1. **Label Cardinality**: Keep label combinations under 100K per metric
2. **Naming Convention**: Use consistent prefixes (kvstore_*)
3. **Metric Types**: Choose appropriate types (counter, gauge, histogram)
4. **Units**: Include units in metric names (bytes, seconds, total)

### Dashboard Organization
```
Dashboards/
├── Overview/
│   ├── KVStore Cluster Overview
│   └── Node Health Summary
├── Performance/
│   ├── Request Performance
│   ├── Storage Performance
│   └── Raft Performance
├── Troubleshooting/
│   ├── Error Analysis
│   ├── Latency Breakdown
│   └── Resource Utilization
└── Capacity Planning/
    ├── Growth Trends
    ├── Resource Forecasting
    └── Scaling Recommendations
```

### Alert Design
1. **Symptom-based**: Alert on customer-visible issues first
2. **Actionable**: Every alert should have a clear response procedure
3. **Meaningful**: Avoid alert fatigue with proper thresholds
4. **Escalation**: Different severity levels with appropriate routing

## Operational Procedures

### Deployment
```bash
# Deploy monitoring stack
kubectl apply -f monitoring/prometheus/
kubectl apply -f monitoring/grafana/
kubectl apply -f monitoring/alertmanager/

# Verify metrics collection
curl http://prometheus:9090/api/v1/query?query=up
```

### Maintenance
```bash
# Backup Prometheus data
kubectl exec prometheus-0 -- tar czf /tmp/prometheus-backup.tar.gz /prometheus

# Update alerting rules
kubectl apply -f monitoring/prometheus/rules/
curl -X POST http://prometheus:9090/-/reload
```

### Troubleshooting
```bash
# Check metric ingestion
kubectl logs -f deployment/prometheus

# Validate PromQL queries
promtool query instant 'kvstore_requests_total'

# Test alerting rules
promtool test rules rules/test.yml
```

## Security Considerations

### Access Control
```yaml
# Grafana RBAC
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-config
data:
  grafana.ini: |
    [auth]
    disable_login_form = false
    
    [auth.ldap]
    enabled = true
    config_file = /etc/grafana/ldap.toml
```

### Network Security
- TLS encryption for all monitoring endpoints
- Network policies to restrict access
- Authentication for Grafana and AlertManager
- Secure credential management

### Data Privacy
- Avoid including sensitive data in metric labels
- Implement metric filtering for multi-tenant scenarios
- Regular security audits of monitoring access

## References

- [Prometheus Documentation](https://prometheus.io/docs/) - Official Prometheus guide
- [Grafana Documentation](https://grafana.com/docs/) - Grafana setup and configuration
- [PromQL Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/) - Query language reference
- [Monitoring Best Practices](https://landing.google.com/sre/sre-book/chapters/monitoring-distributed-systems/) - Google SRE monitoring guide
- [AlertManager Documentation](https://prometheus.io/docs/alerting/latest/alertmanager/) - Alert routing and management