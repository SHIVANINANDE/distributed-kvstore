package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"distributed-kvstore/internal/storage"
)

// MonitoringService integrates all monitoring components
type MonitoringService struct {
	HealthManager   *HealthManager
	MetricsRegistry *MetricsRegistry
	KVMetrics       *KVStoreMetrics
	Dashboard       *Dashboard
	Collector       *MetricsCollector
	PrometheusExporter *PrometheusExporter
}

// NewMonitoringService creates a new monitoring service
func NewMonitoringService(storageEngine storage.StorageEngine) *MonitoringService {
	// Initialize metrics
	kvMetrics := NewKVStoreMetrics()
	
	// Initialize health manager
	healthManager := NewHealthManager("1.0.0")
	
	// Register health checkers
	healthManager.RegisterChecker(NewStorageHealthChecker(storageEngine))
	healthManager.RegisterChecker(NewMemoryHealthChecker(1024)) // 1GB limit
	healthManager.RegisterChecker(NewGoroutineHealthChecker(10000)) // 10k goroutines limit
	
	// Initialize dashboard
	dashboard := NewDashboard(healthManager, kvMetrics.GetRegistry(), kvMetrics)
	
	// Initialize metrics collector
	collector := NewMetricsCollector(kvMetrics, 15*time.Second)
	
	// Initialize Prometheus exporter
	prometheusExporter := NewPrometheusExporter(kvMetrics.GetRegistry(), kvMetrics)
	
	return &MonitoringService{
		HealthManager:      healthManager,
		MetricsRegistry:    kvMetrics.GetRegistry(),
		KVMetrics:          kvMetrics,
		Dashboard:          dashboard,
		Collector:          collector,
		PrometheusExporter: prometheusExporter,
	}
}

// Start starts the monitoring service
func (ms *MonitoringService) Start() {
	ms.Collector.Start()
}

// Stop stops the monitoring service
func (ms *MonitoringService) Stop() {
	ms.Collector.Stop()
}

// GetHealthHandler returns HTTP handler for health checks
func (ms *MonitoringService) GetHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		
		health := ms.HealthManager.CheckHealth(ctx)
		
		// Set appropriate HTTP status code
		statusCode := http.StatusOK
		if health.Status == HealthStatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if health.Status == HealthStatusDegraded {
			statusCode = http.StatusPartialContent
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		
		if err := json.NewEncoder(w).Encode(health); err != nil {
			http.Error(w, "Failed to encode health response", http.StatusInternalServerError)
		}
	}
}

// GetMetricsHandler returns HTTP handler for Prometheus metrics
func (ms *MonitoringService) GetMetricsHandler() http.Handler {
	return ms.PrometheusExporter
}

// GetDashboardHandler returns HTTP handler for the monitoring dashboard
func (ms *MonitoringService) GetDashboardHandler() http.Handler {
	return ms.Dashboard
}

// RecordHTTPRequest records HTTP request metrics
func (ms *MonitoringService) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration, responseSize int64) {
	ms.KVMetrics.HTTPRequests.Inc()
	ms.KVMetrics.HTTPDuration.Observe(duration.Seconds())
	if responseSize > 0 {
		ms.KVMetrics.HTTPResponseSize.Observe(float64(responseSize))
	}
	
	// Record errors
	if statusCode >= 400 {
		ms.KVMetrics.RequestErrors.Inc()
	}
}

// RecordStorageOperation records storage operation metrics
func (ms *MonitoringService) RecordStorageOperation(operation string, success bool, duration time.Duration) {
	ms.KVMetrics.StorageOperations.Inc()
	
	if !success {
		ms.KVMetrics.StorageErrors.Inc()
	}
	
	// Update request metrics
	ms.KVMetrics.RequestDuration.Observe(duration.Seconds())
}

// UpdateStorageMetrics updates storage-related metrics
func (ms *MonitoringService) UpdateStorageMetrics(stats map[string]interface{}) {
	if totalSize, ok := stats["total_size"].(float64); ok {
		ms.KVMetrics.StorageSize.Set(totalSize)
	}
	if entries, ok := stats["entries"].(float64); ok {
		ms.KVMetrics.StorageEntries.Set(entries)
	}
}

// MonitoringMiddleware provides HTTP middleware for automatic metrics collection
func (ms *MonitoringService) MonitoringMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Wrap response writer to capture metrics
		wrapped := &monitoringResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			size:          0,
		}
		
		// Process request
		next.ServeHTTP(wrapped, r)
		
		// Record metrics
		duration := time.Since(start)
		ms.RecordHTTPRequest(r.Method, r.URL.Path, wrapped.statusCode, duration, wrapped.size)
	})
}

// monitoringResponseWriter wraps http.ResponseWriter to capture metrics
type monitoringResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (mrw *monitoringResponseWriter) WriteHeader(code int) {
	mrw.statusCode = code
	mrw.ResponseWriter.WriteHeader(code)
}

func (mrw *monitoringResponseWriter) Write(data []byte) (int, error) {
	size, err := mrw.ResponseWriter.Write(data)
	mrw.size += int64(size)
	return size, err
}

// Enhanced health check with storage operation test
type EnhancedHealthResponse struct {
	HealthResponse
	LastCheck    time.Time `json:"last_check"`
	CheckHistory []HealthCheckHistory `json:"check_history,omitempty"`
}

type HealthCheckHistory struct {
	Timestamp time.Time    `json:"timestamp"`
	Status    HealthStatus `json:"status"`
	Duration  time.Duration `json:"duration"`
}

// AlertManager handles monitoring alerts
type AlertManager struct {
	alerts        []Alert
	rules         []AlertRule
	notifications chan Alert
}

type AlertRule struct {
	Name        string
	Condition   func(metrics map[string]*Metric, health HealthResponse) bool
	Severity    string
	Description string
}

// NewAlertManager creates a new alert manager
func NewAlertManager() *AlertManager {
	am := &AlertManager{
		alerts:        make([]Alert, 0),
		rules:         make([]AlertRule, 0),
		notifications: make(chan Alert, 100),
	}
	
	// Add default alert rules
	am.AddDefaultRules()
	
	return am
}

// AddDefaultRules adds default monitoring alert rules
func (am *AlertManager) AddDefaultRules() {
	// High memory usage
	am.rules = append(am.rules, AlertRule{
		Name:        "HighMemoryUsage",
		Severity:    "warning",
		Description: "Memory usage is above 80%",
		Condition: func(metrics map[string]*Metric, health HealthResponse) bool {
			if memMetric, exists := metrics["kvstore_memory_usage_bytes"]; exists {
				memUsageMB := memMetric.Value / 1024 / 1024
				return memUsageMB > 800 // 800MB threshold
			}
			return false
		},
	})
	
	// High error rate
	am.rules = append(am.rules, AlertRule{
		Name:        "HighErrorRate",
		Severity:    "critical",
		Description: "Error rate is above 5%",
		Condition: func(metrics map[string]*Metric, health HealthResponse) bool {
			var totalRequests, totalErrors float64
			if requestsMetric, exists := metrics["kvstore_requests_total"]; exists {
				totalRequests = requestsMetric.Value
			}
			if errorsMetric, exists := metrics["kvstore_request_errors_total"]; exists {
				totalErrors = errorsMetric.Value
			}
			
			if totalRequests > 0 {
				errorRate := (totalErrors / totalRequests) * 100
				return errorRate > 5.0
			}
			return false
		},
	})
	
	// Storage health failure
	am.rules = append(am.rules, AlertRule{
		Name:        "StorageUnhealthy",
		Severity:    "critical",
		Description: "Storage health check is failing",
		Condition: func(metrics map[string]*Metric, health HealthResponse) bool {
			if storageCheck, exists := health.Checks["storage"]; exists {
				return storageCheck.Status == HealthStatusUnhealthy
			}
			return false
		},
	})
}

// CheckAlerts evaluates alert rules and generates alerts
func (am *AlertManager) CheckAlerts(metrics map[string]*Metric, health HealthResponse) {
	for _, rule := range am.rules {
		if rule.Condition(metrics, health) {
			alert := Alert{
				Level:       rule.Severity,
				Title:       rule.Name,
				Description: rule.Description,
				Timestamp:   time.Now(),
				Source:      "alert_manager",
			}
			
			// Send notification
			select {
			case am.notifications <- alert:
			default:
				// Channel full, skip notification
			}
			
			am.alerts = append(am.alerts, alert)
		}
	}
}

// GetAlerts returns current alerts
func (am *AlertManager) GetAlerts() []Alert {
	return am.alerts
}

// GetNotifications returns the alerts notification channel
func (am *AlertManager) GetNotifications() <-chan Alert {
	return am.notifications
}