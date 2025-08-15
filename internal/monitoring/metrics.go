package monitoring

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType represents different types of metrics
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric represents a single metric
type Metric struct {
	Name        string                 `json:"name"`
	Type        MetricType             `json:"type"`
	Value       float64                `json:"value"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Help        string                 `json:"help"`
	Timestamp   time.Time              `json:"timestamp"`
	Unit        string                 `json:"unit,omitempty"`
	Additional  map[string]interface{} `json:"additional,omitempty"`
}

// MetricsRegistry manages all metrics
type MetricsRegistry struct {
	mu      sync.RWMutex
	metrics map[string]*Metric
	counters map[string]*Counter
	gauges   map[string]*Gauge
	histograms map[string]*Histogram
	summaries  map[string]*Summary
}

// NewMetricsRegistry creates a new metrics registry
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		metrics:    make(map[string]*Metric),
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		summaries:  make(map[string]*Summary),
	}
}

// Counter represents a monotonically increasing counter
type Counter struct {
	name   string
	help   string
	value  int64
	labels map[string]string
}

func (mr *MetricsRegistry) NewCounter(name, help string, labels map[string]string) *Counter {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	counter := &Counter{
		name:   name,
		help:   help,
		labels: labels,
		value:  0,
	}
	
	mr.counters[name] = counter
	return counter
}

func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

func (c *Counter) Add(delta float64) {
	atomic.AddInt64(&c.value, int64(delta))
}

func (c *Counter) Get() float64 {
	return float64(atomic.LoadInt64(&c.value))
}

// Gauge represents a value that can go up and down
type Gauge struct {
	name   string
	help   string
	value  int64
	labels map[string]string
}

func (mr *MetricsRegistry) NewGauge(name, help string, labels map[string]string) *Gauge {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	gauge := &Gauge{
		name:   name,
		help:   help,
		labels: labels,
	}
	
	mr.gauges[name] = gauge
	return gauge
}

func (g *Gauge) Set(value float64) {
	atomic.StoreInt64(&g.value, int64(value))
}

func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

func (g *Gauge) Add(delta float64) {
	atomic.AddInt64(&g.value, int64(delta))
}

func (g *Gauge) Sub(delta float64) {
	atomic.AddInt64(&g.value, -int64(delta))
}

func (g *Gauge) Get() float64 {
	return float64(atomic.LoadInt64(&g.value))
}

// Histogram tracks the distribution of values
type Histogram struct {
	name    string
	help    string
	buckets []float64
	counts  []int64
	sum     int64
	count   int64
	labels  map[string]string
	mu      sync.RWMutex
}

func (mr *MetricsRegistry) NewHistogram(name, help string, buckets []float64, labels map[string]string) *Histogram {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	if buckets == nil {
		buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	}
	
	histogram := &Histogram{
		name:    name,
		help:    help,
		buckets: buckets,
		counts:  make([]int64, len(buckets)+1), // +1 for +Inf bucket
		labels:  labels,
	}
	
	mr.histograms[name] = histogram
	return histogram
}

func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	atomic.AddInt64(&h.count, 1)
	atomic.AddInt64(&h.sum, int64(value*1000)) // Store in milliseconds for precision
	
	for i, bucket := range h.buckets {
		if value <= bucket {
			atomic.AddInt64(&h.counts[i], 1)
			return
		}
	}
	// +Inf bucket
	atomic.AddInt64(&h.counts[len(h.buckets)], 1)
}

func (h *Histogram) Get() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	bucketCounts := make(map[string]int64)
	for i, bucket := range h.buckets {
		bucketCounts[fmt.Sprintf("le_%g", bucket)] = atomic.LoadInt64(&h.counts[i])
	}
	bucketCounts["le_+Inf"] = atomic.LoadInt64(&h.counts[len(h.buckets)])
	
	return map[string]interface{}{
		"buckets": bucketCounts,
		"sum":     float64(atomic.LoadInt64(&h.sum)) / 1000.0,
		"count":   atomic.LoadInt64(&h.count),
	}
}

// Summary tracks quantiles of observations
type Summary struct {
	name      string
	help      string
	count     int64
	sum       int64
	quantiles []float64
	labels    map[string]string
	values    []float64
	mu        sync.RWMutex
}

func (mr *MetricsRegistry) NewSummary(name, help string, quantiles []float64, labels map[string]string) *Summary {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	if quantiles == nil {
		quantiles = []float64{0.5, 0.9, 0.95, 0.99}
	}
	
	summary := &Summary{
		name:      name,
		help:      help,
		quantiles: quantiles,
		labels:    labels,
		values:    make([]float64, 0, 1000), // Buffer for recent values
	}
	
	mr.summaries[name] = summary
	return summary
}

func (s *Summary) Observe(value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	atomic.AddInt64(&s.count, 1)
	atomic.AddInt64(&s.sum, int64(value*1000))
	
	// Keep only recent values for quantile calculation
	s.values = append(s.values, value)
	if len(s.values) > 1000 {
		s.values = s.values[len(s.values)-1000:]
	}
}

func (s *Summary) Get() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Calculate quantiles (simplified implementation)
	quantileValues := make(map[string]float64)
	if len(s.values) > 0 {
		for _, q := range s.quantiles {
			idx := int(float64(len(s.values)-1) * q)
			if idx < len(s.values) {
				quantileValues[fmt.Sprintf("quantile_%g", q)] = s.values[idx]
			}
		}
	}
	
	return map[string]interface{}{
		"quantiles": quantileValues,
		"sum":       float64(atomic.LoadInt64(&s.sum)) / 1000.0,
		"count":     atomic.LoadInt64(&s.count),
	}
}

// GetAllMetrics returns all metrics as a snapshot
func (mr *MetricsRegistry) GetAllMetrics() map[string]*Metric {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	
	result := make(map[string]*Metric)
	now := time.Now()
	
	// Add counters
	for name, counter := range mr.counters {
		result[name] = &Metric{
			Name:      name,
			Type:      MetricTypeCounter,
			Value:     counter.Get(),
			Labels:    counter.labels,
			Help:      counter.help,
			Timestamp: now,
			Unit:      "total",
		}
	}
	
	// Add gauges
	for name, gauge := range mr.gauges {
		result[name] = &Metric{
			Name:      name,
			Type:      MetricTypeGauge,
			Value:     gauge.Get(),
			Labels:    gauge.labels,
			Help:      gauge.help,
			Timestamp: now,
		}
	}
	
	// Add histograms
	for name, histogram := range mr.histograms {
		result[name] = &Metric{
			Name:       name,
			Type:       MetricTypeHistogram,
			Labels:     histogram.labels,
			Help:       histogram.help,
			Timestamp:  now,
			Unit:       "seconds",
			Additional: histogram.Get(),
		}
	}
	
	// Add summaries
	for name, summary := range mr.summaries {
		result[name] = &Metric{
			Name:       name,
			Type:       MetricTypeSummary,
			Labels:     summary.labels,
			Help:       summary.help,
			Timestamp:  now,
			Unit:       "seconds",
			Additional: summary.Get(),
		}
	}
	
	return result
}

// KVStoreMetrics contains application-specific metrics
type KVStoreMetrics struct {
	registry *MetricsRegistry
	
	// Request metrics
	RequestsTotal     *Counter
	RequestDuration   *Histogram
	RequestErrors     *Counter
	
	// Storage metrics
	StorageOperations *Counter
	StorageErrors     *Counter
	StorageSize       *Gauge
	StorageEntries    *Gauge
	
	// System metrics
	MemoryUsage       *Gauge
	GoroutineCount    *Gauge
	GCDuration        *Histogram
	
	// HTTP metrics
	HTTPRequests      *Counter
	HTTPDuration      *Histogram
	HTTPResponseSize  *Histogram
	
	// gRPC metrics (when implemented)
	GRPCRequests      *Counter
	GRPCDuration      *Histogram
	GRPCErrors        *Counter
}

// NewKVStoreMetrics creates application-specific metrics
func NewKVStoreMetrics() *KVStoreMetrics {
	registry := NewMetricsRegistry()
	
	return &KVStoreMetrics{
		registry: registry,
		
		// Request metrics
		RequestsTotal: registry.NewCounter("kvstore_requests_total", "Total number of requests", map[string]string{"method": ""}),
		RequestDuration: registry.NewHistogram("kvstore_request_duration_seconds", "Request duration in seconds", nil, nil),
		RequestErrors: registry.NewCounter("kvstore_request_errors_total", "Total number of request errors", map[string]string{"type": ""}),
		
		// Storage metrics
		StorageOperations: registry.NewCounter("kvstore_storage_operations_total", "Total storage operations", map[string]string{"operation": ""}),
		StorageErrors: registry.NewCounter("kvstore_storage_errors_total", "Total storage errors", map[string]string{"operation": ""}),
		StorageSize: registry.NewGauge("kvstore_storage_size_bytes", "Current storage size in bytes", nil),
		StorageEntries: registry.NewGauge("kvstore_storage_entries", "Current number of stored entries", nil),
		
		// System metrics
		MemoryUsage: registry.NewGauge("kvstore_memory_usage_bytes", "Current memory usage in bytes", nil),
		GoroutineCount: registry.NewGauge("kvstore_goroutines", "Current number of goroutines", nil),
		GCDuration: registry.NewHistogram("kvstore_gc_duration_seconds", "Garbage collection duration", nil, nil),
		
		// HTTP metrics
		HTTPRequests: registry.NewCounter("kvstore_http_requests_total", "Total HTTP requests", map[string]string{"method": "", "path": "", "status": ""}),
		HTTPDuration: registry.NewHistogram("kvstore_http_request_duration_seconds", "HTTP request duration", nil, nil),
		HTTPResponseSize: registry.NewHistogram("kvstore_http_response_size_bytes", "HTTP response size in bytes", nil, nil),
		
		// gRPC metrics
		GRPCRequests: registry.NewCounter("kvstore_grpc_requests_total", "Total gRPC requests", map[string]string{"method": "", "status": ""}),
		GRPCDuration: registry.NewHistogram("kvstore_grpc_request_duration_seconds", "gRPC request duration", nil, nil),
		GRPCErrors: registry.NewCounter("kvstore_grpc_errors_total", "Total gRPC errors", map[string]string{"method": "", "code": ""}),
	}
}

// UpdateSystemMetrics updates system-level metrics
func (m *KVStoreMetrics) UpdateSystemMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	m.MemoryUsage.Set(float64(memStats.Alloc))
	m.GoroutineCount.Set(float64(runtime.NumGoroutine()))
	
	// GC metrics
	if memStats.NumGC > 0 {
		gcPause := float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1e9
		m.GCDuration.Observe(gcPause)
	}
}

// GetRegistry returns the metrics registry
func (m *KVStoreMetrics) GetRegistry() *MetricsRegistry {
	return m.registry
}

// MetricsCollector periodically updates system metrics
type MetricsCollector struct {
	metrics  *KVStoreMetrics
	interval time.Duration
	stopChan chan struct{}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(metrics *KVStoreMetrics, interval time.Duration) *MetricsCollector {
	return &MetricsCollector{
		metrics:  metrics,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins collecting metrics
func (mc *MetricsCollector) Start() {
	ticker := time.NewTicker(mc.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mc.metrics.UpdateSystemMetrics()
			case <-mc.stopChan:
				return
			}
		}
	}()
}

// Stop stops collecting metrics
func (mc *MetricsCollector) Stop() {
	close(mc.stopChan)
}