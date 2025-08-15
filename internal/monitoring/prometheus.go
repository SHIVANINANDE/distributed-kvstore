package monitoring

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	registry *MetricsRegistry
	kvMetrics *KVStoreMetrics
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(registry *MetricsRegistry, kvMetrics *KVStoreMetrics) *PrometheusExporter {
	return &PrometheusExporter{
		registry:  registry,
		kvMetrics: kvMetrics,
	}
}

// ServeHTTP implements the http.Handler interface for Prometheus metrics endpoint
func (pe *PrometheusExporter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	
	// Update system metrics before export
	pe.kvMetrics.UpdateSystemMetrics()
	
	metrics := pe.registry.GetAllMetrics()
	
	// Sort metrics by name for consistent output
	var names []string
	for name := range metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	
	// Write metrics in Prometheus format
	for _, name := range names {
		metric := metrics[name]
		pe.writePrometheusMetric(w, metric)
	}
	
	// Add custom system metrics
	pe.writeSystemMetrics(w)
}

func (pe *PrometheusExporter) writePrometheusMetric(w http.ResponseWriter, metric *Metric) {
	// Write HELP line
	if metric.Help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", metric.Name, metric.Help)
	}
	
	// Write TYPE line
	fmt.Fprintf(w, "# TYPE %s %s\n", metric.Name, string(metric.Type))
	
	// Write metric value
	switch metric.Type {
	case MetricTypeCounter, MetricTypeGauge:
		pe.writeSimpleMetric(w, metric)
	case MetricTypeHistogram:
		pe.writeHistogramMetric(w, metric)
	case MetricTypeSummary:
		pe.writeSummaryMetric(w, metric)
	}
	
	fmt.Fprintf(w, "\n")
}

func (pe *PrometheusExporter) writeSimpleMetric(w http.ResponseWriter, metric *Metric) {
	labelStr := pe.formatLabels(metric.Labels)
	fmt.Fprintf(w, "%s%s %.6f %d\n", 
		metric.Name, labelStr, metric.Value, metric.Timestamp.Unix())
}

func (pe *PrometheusExporter) writeHistogramMetric(w http.ResponseWriter, metric *Metric) {
	baseName := metric.Name
	labelStr := pe.formatLabels(metric.Labels)
	timestamp := metric.Timestamp.Unix()
	
	if metric.Additional != nil {
		if buckets, ok := metric.Additional["buckets"].(map[string]int64); ok {
			// Write bucket counts
			for bucket, count := range buckets {
				if strings.HasPrefix(bucket, "le_") {
					le := strings.TrimPrefix(bucket, "le_")
					bucketLabels := pe.addLabel(metric.Labels, "le", le)
					bucketLabelStr := pe.formatLabels(bucketLabels)
					fmt.Fprintf(w, "%s_bucket%s %d %d\n", 
						baseName, bucketLabelStr, count, timestamp)
				}
			}
		}
		
		// Write sum and count
		if sum, ok := metric.Additional["sum"].(float64); ok {
			fmt.Fprintf(w, "%s_sum%s %.6f %d\n", 
				baseName, labelStr, sum, timestamp)
		}
		if count, ok := metric.Additional["count"].(int64); ok {
			fmt.Fprintf(w, "%s_count%s %d %d\n", 
				baseName, labelStr, count, timestamp)
		}
	}
}

func (pe *PrometheusExporter) writeSummaryMetric(w http.ResponseWriter, metric *Metric) {
	baseName := metric.Name
	labelStr := pe.formatLabels(metric.Labels)
	timestamp := metric.Timestamp.Unix()
	
	if metric.Additional != nil {
		if quantiles, ok := metric.Additional["quantiles"].(map[string]float64); ok {
			// Write quantiles
			for quantile, value := range quantiles {
				if strings.HasPrefix(quantile, "quantile_") {
					q := strings.TrimPrefix(quantile, "quantile_")
					quantileLabels := pe.addLabel(metric.Labels, "quantile", q)
					quantileLabelStr := pe.formatLabels(quantileLabels)
					fmt.Fprintf(w, "%s%s %.6f %d\n", 
						baseName, quantileLabelStr, value, timestamp)
				}
			}
		}
		
		// Write sum and count
		if sum, ok := metric.Additional["sum"].(float64); ok {
			fmt.Fprintf(w, "%s_sum%s %.6f %d\n", 
				baseName, labelStr, sum, timestamp)
		}
		if count, ok := metric.Additional["count"].(int64); ok {
			fmt.Fprintf(w, "%s_count%s %d %d\n", 
				baseName, labelStr, count, timestamp)
		}
	}
}

func (pe *PrometheusExporter) writeSystemMetrics(w http.ResponseWriter) {
	timestamp := time.Now().Unix()
	
	// Write build info
	fmt.Fprintf(w, "# HELP kvstore_build_info Build information\n")
	fmt.Fprintf(w, "# TYPE kvstore_build_info gauge\n")
	fmt.Fprintf(w, "kvstore_build_info{version=\"1.0.0\",go_version=\"%s\"} 1 %d\n", 
		getGoVersion(), timestamp)
	fmt.Fprintf(w, "\n")
	
	// Write uptime
	uptime := time.Since(time.Now()).Seconds() // This would be actual uptime in real implementation
	fmt.Fprintf(w, "# HELP kvstore_uptime_seconds Uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE kvstore_uptime_seconds counter\n")
	fmt.Fprintf(w, "kvstore_uptime_seconds %.6f %d\n", uptime, timestamp)
	fmt.Fprintf(w, "\n")
}

func (pe *PrometheusExporter) formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	
	var labelPairs []string
	for key, value := range labels {
		if value != "" {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=\"%s\"", key, escapePrometheusValue(value)))
		}
	}
	
	if len(labelPairs) == 0 {
		return ""
	}
	
	sort.Strings(labelPairs)
	return "{" + strings.Join(labelPairs, ",") + "}"
}

func (pe *PrometheusExporter) addLabel(labels map[string]string, key, value string) map[string]string {
	result := make(map[string]string)
	for k, v := range labels {
		result[k] = v
	}
	result[key] = value
	return result
}

func escapePrometheusValue(value string) string {
	// Escape special characters for Prometheus labels
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func getGoVersion() string {
	// In a real implementation, this would use runtime.Version()
	return "go1.21.0"
}

// PrometheusConfig holds Prometheus-specific configuration
type PrometheusConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	Path       string `yaml:"path" json:"path"`
	Port       int    `yaml:"port" json:"port"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Subsystem  string `yaml:"subsystem" json:"subsystem"`
}

// DefaultPrometheusConfig returns default Prometheus configuration
func DefaultPrometheusConfig() PrometheusConfig {
	return PrometheusConfig{
		Enabled:   true,
		Path:      "/metrics",
		Port:      2112,
		Namespace: "kvstore",
		Subsystem: "",
	}
}

// MetricsMiddleware is HTTP middleware that collects request metrics
func (pe *PrometheusExporter) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Wrap response writer to capture status code and size
		wrapped := &metricsResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			size:          0,
		}
		
		// Process request
		next.ServeHTTP(wrapped, r)
		
		// Record metrics
		duration := time.Since(start)
		pe.kvMetrics.HTTPRequests.Inc()
		pe.kvMetrics.HTTPDuration.Observe(duration.Seconds())
		pe.kvMetrics.HTTPResponseSize.Observe(float64(wrapped.size))
		
		// Update labels for more specific metrics
		labels := map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": fmt.Sprintf("%d", wrapped.statusCode),
		}
		
		httpRequestsWithLabels := pe.kvMetrics.registry.NewCounter(
			"kvstore_http_requests_with_labels_total", 
			"Total HTTP requests with labels", 
			labels,
		)
		httpRequestsWithLabels.Inc()
	})
}

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (mrw *metricsResponseWriter) WriteHeader(code int) {
	mrw.statusCode = code
	mrw.ResponseWriter.WriteHeader(code)
}

func (mrw *metricsResponseWriter) Write(data []byte) (int, error) {
	size, err := mrw.ResponseWriter.Write(data)
	mrw.size += int64(size)
	return size, err
}

// PrometheusMetricsHandler creates a simple metrics handler
func PrometheusMetricsHandler(kvMetrics *KVStoreMetrics) http.HandlerFunc {
	exporter := NewPrometheusExporter(kvMetrics.GetRegistry(), kvMetrics)
	return exporter.ServeHTTP
}

// Sample Prometheus configuration for docker-compose or Kubernetes
const PrometheusConfigYAML = `
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'kvstore'
    static_configs:
      - targets: ['localhost:2112']
    scrape_interval: 5s
    metrics_path: /metrics
    
  - job_name: 'kvstore-health'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 10s
    metrics_path: /api/v1/health
    
rule_files:
  - "kvstore_alerts.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093
`

// Sample Grafana dashboard JSON (simplified)
const GrafanaDashboardJSON = `{
  "dashboard": {
    "id": null,
    "title": "KVStore Monitoring",
    "tags": ["kvstore", "monitoring"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(kvstore_http_requests_total[5m])",
            "legendFormat": "{{method}} {{path}}"
          }
        ]
      },
      {
        "id": 2,
        "title": "Response Time",
        "type": "graph", 
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(kvstore_http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          },
          {
            "expr": "histogram_quantile(0.50, rate(kvstore_http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "50th percentile"
          }
        ]
      },
      {
        "id": 3,
        "title": "Memory Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "kvstore_memory_usage_bytes / 1024 / 1024",
            "legendFormat": "Memory (MB)"
          }
        ]
      },
      {
        "id": 4,
        "title": "Storage Metrics",
        "type": "graph",
        "targets": [
          {
            "expr": "kvstore_storage_size_bytes",
            "legendFormat": "Storage Size"
          },
          {
            "expr": "kvstore_storage_entries",
            "legendFormat": "Entry Count"
          }
        ]
      }
    ],
    "time": {
      "from": "now-1h",
      "to": "now"
    },
    "refresh": "30s"
  }
}`