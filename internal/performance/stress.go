package performance

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// StressTester provides comprehensive stress testing capabilities
type StressTester struct {
	mu           sync.RWMutex
	logger       *log.Logger
	config       StressConfig
	kvStore      KVStoreInterface
	tests        map[string]*StressTest
	results      map[string]*StressResult
	metrics      *StressMetrics
	isRunning    bool
	stopCh       chan struct{}
	testData     *StressTestData
}

// StressConfig contains configuration for stress testing
type StressConfig struct {
	// Test duration and intensity
	TestDuration        time.Duration `json:"test_duration"`         // Duration for each stress test
	RampUpDuration      time.Duration `json:"ramp_up_duration"`      // Time to ramp up to full load
	RampDownDuration    time.Duration `json:"ramp_down_duration"`    // Time to ramp down from full load
	MaxConcurrency      int           `json:"max_concurrency"`       // Maximum concurrent operations
	MaxThroughput       int64         `json:"max_throughput"`        // Maximum operations per second
	
	// Data characteristics
	KeyRange            int           `json:"key_range"`             // Range of keys to use
	ValueSizeMin        int           `json:"value_size_min"`        // Minimum value size
	ValueSizeMax        int           `json:"value_size_max"`        // Maximum value size
	HotKeyRatio         float64       `json:"hot_key_ratio"`         // Ratio of hot keys (0.0-1.0)
	WriteRatio          float64       `json:"write_ratio"`           // Ratio of write operations
	
	// Failure scenarios
	EnableNetworkLatency bool          `json:"enable_network_latency"` // Simulate network latency
	NetworkLatencyMin   time.Duration `json:"network_latency_min"`    // Minimum network latency
	NetworkLatencyMax   time.Duration `json:"network_latency_max"`    // Maximum network latency
	EnablePacketLoss    bool          `json:"enable_packet_loss"`     // Simulate packet loss
	PacketLossRate      float64       `json:"packet_loss_rate"`       // Packet loss rate (0.0-1.0)
	EnableMemoryPressure bool         `json:"enable_memory_pressure"` // Simulate memory pressure
	MemoryPressureLevel  float64      `json:"memory_pressure_level"`  // Memory pressure level (0.0-1.0)
	
	// Monitoring
	SampleInterval      time.Duration `json:"sample_interval"`       // Metrics sampling interval
	EnableProfiling     bool          `json:"enable_profiling"`      // Enable CPU/memory profiling
	AlertThresholds     *AlertThresholds `json:"alert_thresholds"`   // Alert thresholds
}

// AlertThresholds defines thresholds for stress test alerts
type AlertThresholds struct {
	MaxLatencyP99      time.Duration `json:"max_latency_p99"`      // Maximum acceptable P99 latency
	MinThroughput      float64       `json:"min_throughput"`       // Minimum acceptable throughput
	MaxErrorRate       float64       `json:"max_error_rate"`       // Maximum acceptable error rate
	MaxMemoryUsage     uint64        `json:"max_memory_usage"`     // Maximum acceptable memory usage
	MaxCPUUsage        float64       `json:"max_cpu_usage"`        // Maximum acceptable CPU usage
}

// StressTest represents a single stress test scenario
type StressTest struct {
	Name             string                    `json:"name"`
	Description      string                    `json:"description"`
	TestFunc         func(*StressContext) *StressResult `json:"-"`
	Config           map[string]interface{}    `json:"config"`
	ExpectedFailures []string                  `json:"expected_failures"`
	RecoveryTime     time.Duration             `json:"recovery_time"`
}

// StressContext provides context for stress test execution
type StressContext struct {
	Tester       *StressTester
	TestName     string
	Config       StressConfig
	KVStore      KVStoreInterface
	TestData     *StressTestData
	Logger       *log.Logger
	StartTime    time.Time
	StopCh       chan struct{}
	metrics      *LiveStressMetrics
}

// StressResult contains the results of a stress test
type StressResult struct {
	TestName         string                 `json:"test_name"`
	StartTime        time.Time              `json:"start_time"`
	Duration         time.Duration          `json:"duration"`
	TotalOperations  int64                  `json:"total_operations"`
	SuccessfulOps    int64                  `json:"successful_ops"`
	FailedOps        int64                  `json:"failed_ops"`
	MaxThroughput    float64                `json:"max_throughput"`
	MinThroughput    float64                `json:"min_throughput"`
	AvgThroughput    float64                `json:"avg_throughput"`
	LatencyStats     *LatencyStats          `json:"latency_stats"`
	ResourceUsage    *ResourceUsageStats    `json:"resource_usage"`
	FailurePoints    []FailurePoint         `json:"failure_points"`
	RecoveryTime     time.Duration          `json:"recovery_time"`
	AlertsTriggered  []Alert                `json:"alerts_triggered"`
	Success          bool                   `json:"success"`
	Details          map[string]interface{} `json:"details"`
}

// StressTestData contains data for stress testing
type StressTestData struct {
	Keys            []string          `json:"keys"`
	HotKeys         []string          `json:"hot_keys"`
	Values          map[string][]byte `json:"values"`
	KeyDistribution *KeyDistribution  `json:"key_distribution"`
	GeneratedAt     time.Time         `json:"generated_at"`
}

// KeyDistribution describes the distribution of key access
type KeyDistribution struct {
	HotKeyRatio    float64   `json:"hot_key_ratio"`
	AccessPattern  string    `json:"access_pattern"`  // "uniform", "zipfian", "latest"
	ZipfianSkew    float64   `json:"zipfian_skew"`
	HotKeyAccesses uint64    `json:"hot_key_accesses"`
	ColdKeyAccesses uint64   `json:"cold_key_accesses"`
}

// ResourceUsageStats tracks resource usage during stress tests
type ResourceUsageStats struct {
	MaxMemoryUsage    uint64            `json:"max_memory_usage"`
	AvgMemoryUsage    uint64            `json:"avg_memory_usage"`
	MaxCPUUsage       float64           `json:"max_cpu_usage"`
	AvgCPUUsage       float64           `json:"avg_cpu_usage"`
	GCPauses          []time.Duration   `json:"gc_pauses"`
	GoroutineCount    int               `json:"goroutine_count"`
	NetworkIO         *NetworkIOStats   `json:"network_io"`
	DiskIO            *DiskIOStats      `json:"disk_io"`
	FileDescriptors   int               `json:"file_descriptors"`
}

// FailurePoint represents a point where the system showed signs of stress
type FailurePoint struct {
	Timestamp     time.Time         `json:"timestamp"`
	FailureType   string            `json:"failure_type"`
	Description   string            `json:"description"`
	Metrics       map[string]interface{} `json:"metrics"`
	Severity      string            `json:"severity"`
	Recovered     bool              `json:"recovered"`
	RecoveryTime  time.Duration     `json:"recovery_time"`
}

// Alert represents a triggered alert during stress testing
type Alert struct {
	Timestamp   time.Time         `json:"timestamp"`
	AlertType   string            `json:"alert_type"`
	Threshold   interface{}       `json:"threshold"`
	ActualValue interface{}       `json:"actual_value"`
	Severity    string            `json:"severity"`
	Message     string            `json:"message"`
}

// LiveStressMetrics tracks metrics during stress test execution
type LiveStressMetrics struct {
	mu                sync.RWMutex
	operations        int64
	successes         int64
	failures          int64
	latencies         []time.Duration
	throughputSamples []float64
	resourceSamples   []*ResourceSample
	startTime         time.Time
	lastSampleTime    time.Time
}

// ResourceSample represents a resource usage sample
type ResourceSample struct {
	Timestamp     time.Time `json:"timestamp"`
	MemoryUsage   uint64    `json:"memory_usage"`
	CPUUsage      float64   `json:"cpu_usage"`
	GoroutineCount int      `json:"goroutine_count"`
	Throughput    float64   `json:"throughput"`
}

// StressMetrics tracks overall stress testing metrics
type StressMetrics struct {
	TotalTests       int           `json:"total_tests"`
	PassedTests      int           `json:"passed_tests"`
	FailedTests      int           `json:"failed_tests"`
	TotalDuration    time.Duration `json:"total_duration"`
	TotalOperations  int64         `json:"total_operations"`
	AlertsTriggered  int           `json:"alerts_triggered"`
	FailuresDetected int           `json:"failures_detected"`
}

// NewStressTester creates a new stress tester
func NewStressTester(config StressConfig, kvStore KVStoreInterface, logger *log.Logger) *StressTester {
	if logger == nil {
		logger = log.New(log.Writer(), "[STRESS] ", log.LstdFlags)
	}
	
	// Set default values
	if config.TestDuration == 0 {
		config.TestDuration = 300 * time.Second // 5 minutes
	}
	if config.RampUpDuration == 0 {
		config.RampUpDuration = 30 * time.Second
	}
	if config.RampDownDuration == 0 {
		config.RampDownDuration = 30 * time.Second
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 1000
	}
	if config.MaxThroughput == 0 {
		config.MaxThroughput = 50000 // 50K ops/sec
	}
	if config.KeyRange == 0 {
		config.KeyRange = 1000000 // 1M keys
	}
	if config.ValueSizeMin == 0 {
		config.ValueSizeMin = 100
	}
	if config.ValueSizeMax == 0 {
		config.ValueSizeMax = 4096
	}
	if config.HotKeyRatio == 0 {
		config.HotKeyRatio = 0.1 // 10% hot keys
	}
	if config.WriteRatio == 0 {
		config.WriteRatio = 0.3 // 30% writes
	}
	if config.SampleInterval == 0 {
		config.SampleInterval = time.Second
	}
	
	// Set default alert thresholds
	if config.AlertThresholds == nil {
		config.AlertThresholds = &AlertThresholds{
			MaxLatencyP99:  50 * time.Millisecond,
			MinThroughput:  1000, // 1K ops/sec minimum
			MaxErrorRate:   0.05, // 5% max error rate
			MaxMemoryUsage: 2 * 1024 * 1024 * 1024, // 2GB
			MaxCPUUsage:    80.0, // 80% CPU
		}
	}
	
	st := &StressTester{
		logger:  logger,
		config:  config,
		kvStore: kvStore,
		tests:   make(map[string]*StressTest),
		results: make(map[string]*StressResult),
		metrics: &StressMetrics{},
		stopCh:  make(chan struct{}),
	}
	
	// Register default stress tests
	st.registerDefaultStressTests()
	
	return st
}

// registerDefaultStressTests registers the default stress test scenarios
func (st *StressTester) registerDefaultStressTests() {
	// High concurrency stress test
	st.RegisterTest(&StressTest{
		Name:        "high_concurrency",
		Description: "Tests performance under high concurrency load",
		TestFunc:    st.testHighConcurrency,
		Config: map[string]interface{}{
			"concurrency_multiplier": 2.0, // 2x normal concurrency
			"duration_multiplier":    1.5, // 1.5x normal duration
		},
		RecoveryTime: 30 * time.Second,
	})
	
	// High throughput stress test
	st.RegisterTest(&StressTest{
		Name:        "high_throughput",
		Description: "Tests performance under sustained high throughput",
		TestFunc:    st.testHighThroughput,
		Config: map[string]interface{}{
			"throughput_multiplier": 3.0, // 3x target throughput
			"sustained_duration":    10 * time.Minute,
		},
		RecoveryTime: 45 * time.Second,
	})
	
	// Memory pressure stress test
	st.RegisterTest(&StressTest{
		Name:        "memory_pressure",
		Description: "Tests performance under memory pressure",
		TestFunc:    st.testMemoryPressure,
		Config: map[string]interface{}{
			"large_value_ratio": 0.5,   // 50% large values
			"value_size_multiplier": 10, // 10x larger values
			"force_gc_interval": 5 * time.Second,
		},
		ExpectedFailures: []string{"high_latency", "memory_allocation_failures"},
		RecoveryTime:     60 * time.Second,
	})
	
	// Hot key stress test
	st.RegisterTest(&StressTest{
		Name:        "hot_key_contention",
		Description: "Tests performance with hot key contention",
		TestFunc:    st.testHotKeyContention,
		Config: map[string]interface{}{
			"hot_key_ratio":     0.01, // 1% hot keys
			"hot_key_access":    0.90, // 90% of accesses
			"write_heavy_ratio": 0.80, // 80% writes on hot keys
		},
		ExpectedFailures: []string{"lock_contention", "high_latency"},
		RecoveryTime:     20 * time.Second,
	})
	
	// Network instability stress test
	st.RegisterTest(&StressTest{
		Name:        "network_instability",
		Description: "Tests resilience under network instability",
		TestFunc:    st.testNetworkInstability,
		Config: map[string]interface{}{
			"latency_variation": 0.5,  // 50% latency variation
			"packet_loss_rate":  0.02, // 2% packet loss
			"connection_drops":  5,    // Drop connections 5 times
		},
		ExpectedFailures: []string{"network_timeouts", "connection_failures"},
		RecoveryTime:     15 * time.Second,
	})
	
	// Spike load stress test
	st.RegisterTest(&StressTest{
		Name:        "spike_load",
		Description: "Tests response to sudden load spikes",
		TestFunc:    st.testSpikeLoad,
		Config: map[string]interface{}{
			"spike_multiplier": 10.0, // 10x normal load
			"spike_duration":   30 * time.Second,
			"spike_count":      3, // 3 spikes during test
		},
		RecoveryTime: 30 * time.Second,
	})
	
	// Resource exhaustion stress test
	st.RegisterTest(&StressTest{
		Name:        "resource_exhaustion",
		Description: "Tests behavior when approaching resource limits",
		TestFunc:    st.testResourceExhaustion,
		Config: map[string]interface{}{
			"max_memory_target": 0.9, // Use 90% of available memory
			"max_cpu_target":    0.95, // Use 95% of CPU
			"max_connections":   10000, // High connection count
		},
		ExpectedFailures: []string{"resource_exhaustion", "system_overload"},
		RecoveryTime:     120 * time.Second,
	})
}

// RegisterTest registers a new stress test
func (st *StressTester) RegisterTest(test *StressTest) {
	st.mu.Lock()
	defer st.mu.Unlock()
	
	st.tests[test.Name] = test
	st.logger.Printf("Registered stress test: %s", test.Name)
}

// PrepareTestData prepares test data for stress testing
func (st *StressTester) PrepareTestData() error {
	st.logger.Printf("Preparing stress test data with %d keys", st.config.KeyRange)
	
	testData := &StressTestData{
		Keys:     make([]string, st.config.KeyRange),
		Values:   make(map[string][]byte),
		GeneratedAt: time.Now(),
	}
	
	// Generate keys
	for i := 0; i < st.config.KeyRange; i++ {
		key := fmt.Sprintf("stress_key_%08d", i)
		testData.Keys[i] = key
	}
	
	// Determine hot keys
	hotKeyCount := int(float64(st.config.KeyRange) * st.config.HotKeyRatio)
	testData.HotKeys = make([]string, hotKeyCount)
	for i := 0; i < hotKeyCount; i++ {
		testData.HotKeys[i] = testData.Keys[i]
	}
	
	// Generate values with various sizes
	valueCount := st.config.KeyRange / 10 // Generate fewer unique values
	for i := 0; i < valueCount; i++ {
		valueSize := st.config.ValueSizeMin + 
			rand.Intn(st.config.ValueSizeMax-st.config.ValueSizeMin)
		value := generateRandomBytes(valueSize)
		valueKey := fmt.Sprintf("value_%d_%d", i, valueSize)
		testData.Values[valueKey] = value
	}
	
	// Set up key distribution
	testData.KeyDistribution = &KeyDistribution{
		HotKeyRatio:   st.config.HotKeyRatio,
		AccessPattern: "zipfian",
		ZipfianSkew:   1.0,
	}
	
	st.testData = testData
	st.logger.Printf("Generated stress test data: %d keys, %d hot keys, %d value patterns", 
		len(testData.Keys), len(testData.HotKeys), len(testData.Values))
	
	return nil
}

// RunAllStressTests runs all registered stress tests
func (st *StressTester) RunAllStressTests(ctx context.Context) (*StressMetrics, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	
	if st.isRunning {
		return nil, fmt.Errorf("stress tester is already running")
	}
	
	st.logger.Printf("Starting stress testing suite with %d tests", len(st.tests))
	st.isRunning = true
	defer func() { st.isRunning = false }()
	
	// Prepare test data
	if err := st.PrepareTestData(); err != nil {
		return nil, fmt.Errorf("failed to prepare test data: %w", err)
	}
	
	startTime := time.Now()
	totalOps := int64(0)
	
	// Run each stress test
	for testName, test := range st.tests {
		st.logger.Printf("Running stress test: %s", testName)
		
		result := st.runSingleStressTest(ctx, test)
		st.results[testName] = result
		
		// Update metrics
		st.metrics.TotalTests++
		if result.Success {
			st.metrics.PassedTests++
		} else {
			st.metrics.FailedTests++
		}
		totalOps += result.TotalOperations
		st.metrics.AlertsTriggered += len(result.AlertsTriggered)
		st.metrics.FailuresDetected += len(result.FailurePoints)
		
		// Recovery period between tests
		if test.RecoveryTime > 0 {
			st.logger.Printf("Recovery period: %v", test.RecoveryTime)
			time.Sleep(test.RecoveryTime)
		}
		
		// Check if context was cancelled
		if ctx.Err() != nil {
			st.logger.Printf("Stress testing cancelled")
			break
		}
	}
	
	// Calculate final metrics
	st.metrics.TotalDuration = time.Since(startTime)
	st.metrics.TotalOperations = totalOps
	
	st.logger.Printf("Stress testing completed: %d passed, %d failed, %d alerts, %d failures", 
		st.metrics.PassedTests, st.metrics.FailedTests, 
		st.metrics.AlertsTriggered, st.metrics.FailuresDetected)
	
	return st.metrics, nil
}

// runSingleStressTest executes a single stress test
func (st *StressTester) runSingleStressTest(ctx context.Context, test *StressTest) *StressResult {
	testCtx := &StressContext{
		Tester:    st,
		TestName:  test.Name,
		Config:    st.config,
		KVStore:   st.kvStore,
		TestData:  st.testData,
		Logger:    st.logger,
		StartTime: time.Now(),
		StopCh:    make(chan struct{}),
		metrics:   NewLiveStressMetrics(),
	}
	
	// Start monitoring
	go st.monitorStressTest(testCtx)
	
	// Execute the test
	result := test.TestFunc(testCtx)
	result.TestName = test.Name
	result.StartTime = testCtx.StartTime
	
	// Stop monitoring
	close(testCtx.StopCh)
	
	return result
}

// Stress test implementations

// testHighConcurrency tests performance under high concurrency
func (st *StressTester) testHighConcurrency(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	multiplier := ctx.Tester.tests[ctx.TestName].Config["concurrency_multiplier"].(float64)
	maxConcurrency := int(float64(ctx.Config.MaxConcurrency) * multiplier)
	
	ctx.Logger.Printf("Testing high concurrency with %d concurrent operations", maxConcurrency)
	
	start := time.Now()
	operations := st.runConcurrencyStressTest(ctx, maxConcurrency)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	result.AvgThroughput = float64(operations) / result.Duration.Seconds()
	result.Success = result.FailedOps < result.TotalOperations/10 // Less than 10% failures
	result.Details["max_concurrency"] = maxConcurrency
	
	return result
}

// testHighThroughput tests sustained high throughput
func (st *StressTester) testHighThroughput(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	multiplier := ctx.Tester.tests[ctx.TestName].Config["throughput_multiplier"].(float64)
	targetTPS := int64(float64(ctx.Config.MaxThroughput) * multiplier)
	
	ctx.Logger.Printf("Testing high throughput with target %d ops/sec", targetTPS)
	
	start := time.Now()
	operations := st.runThroughputStressTest(ctx, targetTPS)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	result.AvgThroughput = float64(operations) / result.Duration.Seconds()
	result.Success = result.AvgThroughput >= float64(targetTPS)*0.8 // Achieve 80% of target
	result.Details["target_throughput"] = targetTPS
	
	return result
}

// testMemoryPressure tests performance under memory pressure
func (st *StressTester) testMemoryPressure(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	ctx.Logger.Printf("Testing memory pressure")
	
	// Force initial GC to get baseline
	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)
	
	start := time.Now()
	operations := st.runMemoryPressureTest(ctx)
	
	// Get final memory stats
	runtime.GC()
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	result.ResourceUsage = &ResourceUsageStats{
		MaxMemoryUsage: memEnd.Sys,
		AvgMemoryUsage: (memStart.Alloc + memEnd.Alloc) / 2,
	}
	result.Success = memEnd.Alloc < ctx.Config.AlertThresholds.MaxMemoryUsage
	result.Details["memory_growth"] = memEnd.Alloc - memStart.Alloc
	
	return result
}

// testHotKeyContention tests hot key contention scenarios
func (st *StressTester) testHotKeyContention(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	ctx.Logger.Printf("Testing hot key contention")
	
	start := time.Now()
	operations := st.runHotKeyContentionTest(ctx)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	result.AvgThroughput = float64(operations) / result.Duration.Seconds()
	
	// Calculate latency stats from collected latencies
	ctx.metrics.mu.RLock()
	latencies := make([]time.Duration, len(ctx.metrics.latencies))
	copy(latencies, ctx.metrics.latencies)
	ctx.metrics.mu.RUnlock()
	
	result.LatencyStats = calculateLatencyStats(latencies)
	result.Success = result.LatencyStats.P99 < ctx.Config.AlertThresholds.MaxLatencyP99
	result.Details["hot_key_ratio"] = ctx.Config.HotKeyRatio
	
	return result
}

// testNetworkInstability tests resilience under network issues
func (st *StressTester) testNetworkInstability(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	ctx.Logger.Printf("Testing network instability")
	
	start := time.Now()
	operations := st.runNetworkInstabilityTest(ctx)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	result.AvgThroughput = float64(operations) / result.Duration.Seconds()
	
	errorRate := float64(result.FailedOps) / float64(result.TotalOperations)
	result.Success = errorRate < ctx.Config.AlertThresholds.MaxErrorRate*2 // Allow 2x normal error rate
	result.Details["error_rate"] = errorRate
	
	return result
}

// testSpikeLoad tests response to sudden load spikes
func (st *StressTester) testSpikeLoad(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	ctx.Logger.Printf("Testing spike load")
	
	start := time.Now()
	operations := st.runSpikeLoadTest(ctx)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	
	// Calculate throughput statistics
	ctx.metrics.mu.RLock()
	throughputSamples := make([]float64, len(ctx.metrics.throughputSamples))
	copy(throughputSamples, ctx.metrics.throughputSamples)
	ctx.metrics.mu.RUnlock()
	
	if len(throughputSamples) > 0 {
		result.MaxThroughput = st.calculateMax(throughputSamples)
		result.MinThroughput = st.calculateMin(throughputSamples)
		result.AvgThroughput = st.calculateAverage(throughputSamples)
	}
	
	result.Success = result.MinThroughput >= ctx.Config.AlertThresholds.MinThroughput*0.5 // 50% of minimum
	result.Details["throughput_variation"] = result.MaxThroughput - result.MinThroughput
	
	return result
}

// testResourceExhaustion tests behavior near resource limits
func (st *StressTester) testResourceExhaustion(ctx *StressContext) *StressResult {
	result := &StressResult{
		Details: make(map[string]interface{}),
	}
	
	ctx.Logger.Printf("Testing resource exhaustion")
	
	start := time.Now()
	operations := st.runResourceExhaustionTest(ctx)
	
	result.Duration = time.Since(start)
	result.TotalOperations = operations
	result.SuccessfulOps = atomic.LoadInt64(&ctx.metrics.successes)
	result.FailedOps = atomic.LoadInt64(&ctx.metrics.failures)
	result.AvgThroughput = float64(operations) / result.Duration.Seconds()
	
	// System should gracefully degrade, not crash
	result.Success = result.TotalOperations > 0 && result.FailedOps < result.TotalOperations
	result.Details["degradation_factor"] = float64(result.FailedOps) / float64(result.TotalOperations)
	
	return result
}

// Test execution helpers

// runConcurrencyStressTest runs a high concurrency stress test
func (st *StressTester) runConcurrencyStressTest(ctx *StressContext, maxConcurrency int) int64 {
	var wg sync.WaitGroup
	var operations int64
	
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			ops := int64(0)
			for time.Now().Before(endTime) {
				key := st.selectKey(ctx, ops)
				value := st.selectValue(ctx)
				
				start := time.Now()
				var err error
				
				if st.shouldWrite(ctx.Config.WriteRatio) {
					err = ctx.KVStore.Set(key, value)
				} else {
					_, err = ctx.KVStore.Get(key)
				}
				
				latency := time.Since(start)
				st.recordOperation(ctx, err, latency)
				
				if err == nil {
					ops++
				}
			}
			
			atomic.AddInt64(&operations, ops)
		}(i)
	}
	
	wg.Wait()
	return operations
}

// runThroughputStressTest runs a sustained throughput stress test
func (st *StressTester) runThroughputStressTest(ctx *StressContext, targetTPS int64) int64 {
	var operations int64
	interval := time.Duration(int64(time.Second) / targetTPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			key := st.selectKey(ctx, operations)
			value := st.selectValue(ctx)
			
			start := time.Now()
			var err error
			
			if st.shouldWrite(ctx.Config.WriteRatio) {
				err = ctx.KVStore.Set(key, value)
			} else {
				_, err = ctx.KVStore.Get(key)
			}
			
			latency := time.Since(start)
			st.recordOperation(ctx, err, latency)
			
			if err == nil {
				operations++
			}
		}
	}
	
	return operations
}

// runMemoryPressureTest runs a memory pressure test
func (st *StressTester) runMemoryPressureTest(ctx *StressContext) int64 {
	var operations int64
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	// Create memory pressure by using large values
	largeValueSize := ctx.Config.ValueSizeMax * 10
	
	for time.Now().Before(endTime) {
		key := st.selectKey(ctx, operations)
		value := generateRandomBytes(largeValueSize)
		
		start := time.Now()
		err := ctx.KVStore.Set(key, value)
		latency := time.Since(start)
		
		st.recordOperation(ctx, err, latency)
		
		if err == nil {
			operations++
		}
		
		// Periodic GC to simulate pressure
		if operations%100 == 0 {
			runtime.GC()
		}
	}
	
	return operations
}

// runHotKeyContentionTest runs a hot key contention test
func (st *StressTester) runHotKeyContentionTest(ctx *StressContext) int64 {
	var wg sync.WaitGroup
	var operations int64
	
	concurrency := ctx.Config.MaxConcurrency / 2 // Moderate concurrency
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ops := int64(0)
			for time.Now().Before(endTime) {
				var key string
				
				// 90% of operations on hot keys
				if rand.Float64() < 0.9 {
					key = ctx.TestData.HotKeys[rand.Intn(len(ctx.TestData.HotKeys))]
				} else {
					key = st.selectKey(ctx, ops)
				}
				
				value := st.selectValue(ctx)
				
				start := time.Now()
				var err error
				
				// High write ratio on hot keys
				if st.shouldWrite(0.8) {
					err = ctx.KVStore.Set(key, value)
				} else {
					_, err = ctx.KVStore.Get(key)
				}
				
				latency := time.Since(start)
				st.recordOperation(ctx, err, latency)
				
				if err == nil {
					ops++
				}
			}
			
			atomic.AddInt64(&operations, ops)
		}()
	}
	
	wg.Wait()
	return operations
}

// runNetworkInstabilityTest simulates network instability
func (st *StressTester) runNetworkInstabilityTest(ctx *StressContext) int64 {
	var operations int64
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	for time.Now().Before(endTime) {
		key := st.selectKey(ctx, operations)
		value := st.selectValue(ctx)
		
		// Simulate network latency
		if ctx.Config.EnableNetworkLatency {
			latency := ctx.Config.NetworkLatencyMin + 
				time.Duration(rand.Float64()*float64(ctx.Config.NetworkLatencyMax-ctx.Config.NetworkLatencyMin))
			time.Sleep(latency)
		}
		
		// Simulate packet loss
		if ctx.Config.EnablePacketLoss && rand.Float64() < ctx.Config.PacketLossRate {
			st.recordOperation(ctx, fmt.Errorf("simulated packet loss"), 0)
			continue
		}
		
		start := time.Now()
		var err error
		
		if st.shouldWrite(ctx.Config.WriteRatio) {
			err = ctx.KVStore.Set(key, value)
		} else {
			_, err = ctx.KVStore.Get(key)
		}
		
		latency := time.Since(start)
		st.recordOperation(ctx, err, latency)
		
		if err == nil {
			operations++
		}
	}
	
	return operations
}

// runSpikeLoadTest simulates sudden load spikes
func (st *StressTester) runSpikeLoadTest(ctx *StressContext) int64 {
	var operations int64
	testConfig := ctx.Tester.tests[ctx.TestName].Config
	
	spikeMultiplier := testConfig["spike_multiplier"].(float64)
	spikeDuration := testConfig["spike_duration"].(time.Duration)
	spikeCount := testConfig["spike_count"].(int)
	
	normalConcurrency := 10
	spikeConcurrency := int(float64(normalConcurrency) * spikeMultiplier)
	
	totalDuration := ctx.Config.TestDuration
	spikeInterval := totalDuration / time.Duration(spikeCount+1)
	
	for i := 0; i < spikeCount; i++ {
		// Normal load period
		ops := st.runLoadPeriod(ctx, normalConcurrency, spikeInterval-spikeDuration)
		operations += ops
		
		// Spike load period
		ctx.Logger.Printf("Starting load spike %d/%d", i+1, spikeCount)
		ops = st.runLoadPeriod(ctx, spikeConcurrency, spikeDuration)
		operations += ops
	}
	
	// Final normal period
	ops := st.runLoadPeriod(ctx, normalConcurrency, spikeInterval)
	operations += ops
	
	return operations
}

// runResourceExhaustionTest tests resource exhaustion scenarios
func (st *StressTester) runResourceExhaustionTest(ctx *StressContext) int64 {
	var operations int64
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	// Gradually increase load to exhaust resources
	maxConcurrency := ctx.Config.MaxConcurrency * 2 // Exceed normal capacity
	currentConcurrency := 1
	
	for time.Now().Before(endTime) && currentConcurrency <= maxConcurrency {
		periodEnd := time.Now().Add(30 * time.Second)
		
		ops := st.runLoadPeriod(ctx, currentConcurrency, 30*time.Second)
		operations += ops
		
		currentConcurrency *= 2 // Double concurrency each period
		
		if time.Now().After(periodEnd) {
			break
		}
	}
	
	return operations
}

// runLoadPeriod runs load for a specific period with given concurrency
func (st *StressTester) runLoadPeriod(ctx *StressContext, concurrency int, duration time.Duration) int64 {
	var wg sync.WaitGroup
	var operations int64
	
	endTime := time.Now().Add(duration)
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ops := int64(0)
			for time.Now().Before(endTime) {
				key := st.selectKey(ctx, ops)
				value := st.selectValue(ctx)
				
				start := time.Now()
				var err error
				
				if st.shouldWrite(ctx.Config.WriteRatio) {
					err = ctx.KVStore.Set(key, value)
				} else {
					_, err = ctx.KVStore.Get(key)
				}
				
				latency := time.Since(start)
				st.recordOperation(ctx, err, latency)
				
				if err == nil {
					ops++
				}
			}
			
			atomic.AddInt64(&operations, ops)
		}()
	}
	
	wg.Wait()
	return operations
}

// Helper functions

// selectKey selects a key based on distribution pattern
func (st *StressTester) selectKey(ctx *StressContext, operationCount int64) string {
	if len(ctx.TestData.HotKeys) > 0 && rand.Float64() < ctx.Config.HotKeyRatio*5 { // 5x more likely for hot keys
		return ctx.TestData.HotKeys[rand.Intn(len(ctx.TestData.HotKeys))]
	}
	
	index := operationCount % int64(len(ctx.TestData.Keys))
	return ctx.TestData.Keys[index]
}

// selectValue selects a value from the test data
func (st *StressTester) selectValue(ctx *StressContext) []byte {
	// Select a random value from the generated values
	valueKeys := make([]string, 0, len(ctx.TestData.Values))
	for k := range ctx.TestData.Values {
		valueKeys = append(valueKeys, k)
	}
	
	if len(valueKeys) == 0 {
		return generateRandomBytes(1024) // Fallback
	}
	
	randomKey := valueKeys[rand.Intn(len(valueKeys))]
	return ctx.TestData.Values[randomKey]
}

// shouldWrite determines if the operation should be a write
func (st *StressTester) shouldWrite(writeRatio float64) bool {
	return rand.Float64() < writeRatio
}

// recordOperation records an operation's result and metrics
func (st *StressTester) recordOperation(ctx *StressContext, err error, latency time.Duration) {
	ctx.metrics.mu.Lock()
	defer ctx.metrics.mu.Unlock()
	
	atomic.AddInt64(&ctx.metrics.operations, 1)
	
	if err == nil {
		atomic.AddInt64(&ctx.metrics.successes, 1)
	} else {
		atomic.AddInt64(&ctx.metrics.failures, 1)
	}
	
	if latency > 0 {
		ctx.metrics.latencies = append(ctx.metrics.latencies, latency)
	}
}

// monitorStressTest monitors a stress test execution
func (st *StressTester) monitorStressTest(ctx *StressContext) {
	ticker := time.NewTicker(ctx.Config.SampleInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.StopCh:
			return
		case <-ticker.C:
			st.collectStressMetrics(ctx)
		}
	}
}

// collectStressMetrics collects metrics during stress test
func (st *StressTester) collectStressMetrics(ctx *StressContext) {
	ctx.metrics.mu.Lock()
	defer ctx.metrics.mu.Unlock()
	
	now := time.Now()
	operations := atomic.LoadInt64(&ctx.metrics.operations)
	
	// Calculate throughput since last sample
	var throughput float64
	if !ctx.metrics.lastSampleTime.IsZero() {
		timeDiff := now.Sub(ctx.metrics.lastSampleTime).Seconds()
		if timeDiff > 0 {
			opsDiff := operations - int64(len(ctx.metrics.throughputSamples))
			throughput = float64(opsDiff) / timeDiff
		}
	}
	
	ctx.metrics.throughputSamples = append(ctx.metrics.throughputSamples, throughput)
	ctx.metrics.lastSampleTime = now
	
	// Collect resource sample
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	sample := &ResourceSample{
		Timestamp:     now,
		MemoryUsage:   memStats.Alloc,
		GoroutineCount: runtime.NumGoroutine(),
		Throughput:    throughput,
	}
	
	ctx.metrics.resourceSamples = append(ctx.metrics.resourceSamples, sample)
}

// calculateMax calculates maximum value from a slice
func (st *StressTester) calculateMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// calculateMin calculates minimum value from a slice
func (st *StressTester) calculateMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

// calculateAverage calculates average value from a slice
func (st *StressTester) calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// NewLiveStressMetrics creates new live stress metrics tracker
func NewLiveStressMetrics() *LiveStressMetrics {
	return &LiveStressMetrics{
		startTime: time.Now(),
	}
}

// GetResults returns all stress test results
func (st *StressTester) GetResults() map[string]*StressResult {
	st.mu.RLock()
	defer st.mu.RUnlock()
	
	results := make(map[string]*StressResult)
	for name, result := range st.results {
		resultCopy := *result
		results[name] = &resultCopy
	}
	
	return results
}

// GetMetrics returns stress testing metrics
func (st *StressTester) GetMetrics() *StressMetrics {
	st.mu.RLock()
	defer st.mu.RUnlock()
	
	metricsCopy := *st.metrics
	return &metricsCopy
}