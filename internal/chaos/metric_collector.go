package chaos

import (
	"context"
	"log"
	"sync"
	"time"
)

// MetricCollector collects metrics during chaos experiments
type MetricCollector struct {
	mu                sync.RWMutex
	logger            *log.Logger
	isRunning         bool
	experimentMetrics map[string]*ExperimentMetrics
	globalMetrics     *GlobalMetrics
	collectors        map[string]chan struct{} // experiment -> stop channel
}

// GlobalMetrics contains system-wide metrics
type GlobalMetrics struct {
	StartTime         time.Time         `json:"start_time"`
	RequestCount      int64             `json:"request_count"`
	SuccessfulReqs    int64             `json:"successful_requests"`
	FailedRequests    int64             `json:"failed_requests"`
	AverageLatency    time.Duration     `json:"average_latency"`
	ErrorRate         float64           `json:"error_rate"`
	ActiveNodes       int               `json:"active_nodes"`
	TotalPartitions   int               `json:"total_partitions"`
	TotalFailures     int               `json:"total_failures"`
}

// NewMetricCollector creates a new metric collector
func NewMetricCollector(logger *log.Logger) *MetricCollector {
	if logger == nil {
		logger = log.New(log.Writer(), "[METRICS] ", log.LstdFlags)
	}
	
	return &MetricCollector{
		logger:            logger,
		experimentMetrics: make(map[string]*ExperimentMetrics),
		globalMetrics: &GlobalMetrics{
			StartTime: time.Now(),
		},
		collectors: make(map[string]chan struct{}),
	}
}

// Start starts global metric collection
func (mc *MetricCollector) Start(ctx context.Context) {
	mc.mu.Lock()
	if mc.isRunning {
		mc.mu.Unlock()
		return
	}
	mc.isRunning = true
	mc.mu.Unlock()
	
	mc.logger.Printf("Starting global metric collection")
	
	// Start global metrics collection
	go mc.collectGlobalMetrics(ctx)
}

// StartExperiment starts collecting metrics for a specific experiment
func (mc *MetricCollector) StartExperiment(experimentID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	metrics := &ExperimentMetrics{
		RequestCount:    0,
		SuccessfulReqs:  0,
		FailedRequests:  0,
		PartitionEvents: 0,
		NodeFailures:    0,
		RecoveryEvents:  0,
	}
	
	mc.experimentMetrics[experimentID] = metrics
	
	// Start experiment-specific collection
	stopCh := make(chan struct{})
	mc.collectors[experimentID] = stopCh
	
	go mc.collectExperimentMetrics(experimentID, stopCh)
	
	mc.logger.Printf("Started metric collection for experiment %s", experimentID)
}

// StopExperiment stops collecting metrics for an experiment and returns final metrics
func (mc *MetricCollector) StopExperiment(experimentID string) *ExperimentMetrics {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// Stop collection
	if stopCh, exists := mc.collectors[experimentID]; exists {
		close(stopCh)
		delete(mc.collectors, experimentID)
	}
	
	// Get final metrics
	metrics, exists := mc.experimentMetrics[experimentID]
	if !exists {
		return &ExperimentMetrics{}
	}
	
	// Calculate final values
	if metrics.RequestCount > 0 {
		metrics.ErrorRate = float64(metrics.FailedRequests) / float64(metrics.RequestCount)
	}
	
	mc.logger.Printf("Stopped metric collection for experiment %s", experimentID)
	
	// Return copy
	metricsCopy := *metrics
	return &metricsCopy
}

// collectGlobalMetrics collects system-wide metrics
func (mc *MetricCollector) collectGlobalMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mc.updateGlobalMetrics()
		}
	}
}

// collectExperimentMetrics collects metrics for a specific experiment
func (mc *MetricCollector) collectExperimentMetrics(experimentID string, stopCh chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			mc.updateExperimentMetrics(experimentID)
		}
	}
}

// updateGlobalMetrics updates global system metrics
func (mc *MetricCollector) updateGlobalMetrics() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// In a real implementation, this would collect actual system metrics
	// For simulation, we'll generate realistic values
	
	// Simulate increasing request counts
	mc.globalMetrics.RequestCount += int64(10 + (time.Now().UnixNano() % 20))
	mc.globalMetrics.SuccessfulReqs += int64(8 + (time.Now().UnixNano() % 15))
	mc.globalMetrics.FailedRequests = mc.globalMetrics.RequestCount - mc.globalMetrics.SuccessfulReqs
	
	// Calculate error rate
	if mc.globalMetrics.RequestCount > 0 {
		mc.globalMetrics.ErrorRate = float64(mc.globalMetrics.FailedRequests) / float64(mc.globalMetrics.RequestCount)
	}
	
	// Simulate latency (10-100ms)
	mc.globalMetrics.AverageLatency = time.Duration(10+(time.Now().UnixNano()%90)) * time.Millisecond
}

// updateExperimentMetrics updates metrics for a specific experiment
func (mc *MetricCollector) updateExperimentMetrics(experimentID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	metrics, exists := mc.experimentMetrics[experimentID]
	if !exists {
		return
	}
	
	// Simulate metric updates during experiment
	requestIncrease := int64(5 + (time.Now().UnixNano() % 10))
	metrics.RequestCount += requestIncrease
	
	// Simulate some failures during chaos
	if time.Now().UnixNano()%100 < 15 { // 15% chance of failures
		metrics.FailedRequests += 1
	} else {
		metrics.SuccessfulReqs += requestIncrease
	}
	
	// Simulate latency variations
	baseLatency := 20 * time.Millisecond
	variation := time.Duration(time.Now().UnixNano()%50) * time.Millisecond
	metrics.AverageLatency = baseLatency + variation
	
	// Simulate percentiles (simplified)
	metrics.P95Latency = metrics.AverageLatency + 30*time.Millisecond
	metrics.P99Latency = metrics.AverageLatency + 50*time.Millisecond
}

// RecordPartitionEvent records a network partition event
func (mc *MetricCollector) RecordPartitionEvent(experimentID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if metrics, exists := mc.experimentMetrics[experimentID]; exists {
		metrics.PartitionEvents++
	}
	
	mc.globalMetrics.TotalPartitions++
}

// RecordNodeFailure records a node failure event
func (mc *MetricCollector) RecordNodeFailure(experimentID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if metrics, exists := mc.experimentMetrics[experimentID]; exists {
		metrics.NodeFailures++
	}
	
	mc.globalMetrics.TotalFailures++
}

// RecordRecoveryEvent records a recovery event
func (mc *MetricCollector) RecordRecoveryEvent(experimentID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if metrics, exists := mc.experimentMetrics[experimentID]; exists {
		metrics.RecoveryEvents++
	}
}

// GetGlobalMetrics returns current global metrics
func (mc *MetricCollector) GetGlobalMetrics() *GlobalMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	// Return copy
	metricsCopy := *mc.globalMetrics
	return &metricsCopy
}

// GetExperimentMetrics returns metrics for a specific experiment
func (mc *MetricCollector) GetExperimentMetrics(experimentID string) *ExperimentMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	metrics, exists := mc.experimentMetrics[experimentID]
	if !exists {
		return nil
	}
	
	// Return copy
	metricsCopy := *metrics
	return &metricsCopy
}