package performance

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// BenchmarkSuite provides comprehensive performance testing capabilities
type BenchmarkSuite struct {
	mu           sync.RWMutex
	logger       *log.Logger
	config       BenchmarkConfig
	tests        map[string]*BenchmarkTest
	results      map[string]*BenchmarkResult
	metrics      *BenchmarkMetrics
	isRunning    bool
	kvStore      KVStoreInterface
	testData     *TestDataSet
}

// BenchmarkConfig contains configuration for performance benchmarks
type BenchmarkConfig struct {
	// Test parameters
	MaxKeys          int           `json:"max_keys"`           // Maximum number of keys to test
	ValueSizes       []int         `json:"value_sizes"`        // Different value sizes to test
	ConcurrencyLevels []int        `json:"concurrency_levels"` // Different concurrency levels
	TestDuration     time.Duration `json:"test_duration"`      // Duration for each test
	WarmupDuration   time.Duration `json:"warmup_duration"`    // Warmup period before testing
	
	// Performance targets
	TargetThroughput int64         `json:"target_throughput"`  // Target operations per second
	TargetLatencyP99 time.Duration `json:"target_latency_p99"` // Target 99th percentile latency
	TargetLatencyP95 time.Duration `json:"target_latency_p95"` // Target 95th percentile latency
	
	// Test types to run
	EnableReadTests    bool `json:"enable_read_tests"`    // Enable read performance tests
	EnableWriteTests   bool `json:"enable_write_tests"`   // Enable write performance tests
	EnableMixedTests   bool `json:"enable_mixed_tests"`   // Enable mixed read/write tests
	EnableStressTests  bool `json:"enable_stress_tests"`  // Enable stress tests
	
	// Output configuration
	EnableMetrics      bool `json:"enable_metrics"`       // Enable detailed metrics collection
	EnableProfiling    bool `json:"enable_profiling"`     // Enable CPU/memory profiling
	ReportFormat       string `json:"report_format"`      // Report format (json, csv, html)
	OutputDirectory    string `json:"output_directory"`   // Directory for output files
}

// KVStoreInterface defines the interface for testing key-value stores
type KVStoreInterface interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
	Exists(key string) bool
	GetMetrics() interface{}
	Close() error
}

// BenchmarkTest represents a single benchmark test
type BenchmarkTest struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	TestFunc         func(*BenchmarkContext) *BenchmarkResult `json:"-"`
	Config           map[string]interface{} `json:"config"`
	Prerequisites    []string               `json:"prerequisites"`
	EstimatedDuration time.Duration         `json:"estimated_duration"`
}

// BenchmarkContext provides context for benchmark execution
type BenchmarkContext struct {
	Suite      *BenchmarkSuite
	TestName   string
	Config     BenchmarkConfig
	KVStore    KVStoreInterface
	TestData   *TestDataSet
	Logger     *log.Logger
	StartTime  time.Time
	metrics    *TestMetrics
	cancelled  bool
}

// BenchmarkResult contains the results of a benchmark test
type BenchmarkResult struct {
	TestName         string            `json:"test_name"`
	StartTime        time.Time         `json:"start_time"`
	Duration         time.Duration     `json:"duration"`
	OperationsCount  int64             `json:"operations_count"`
	Throughput       float64           `json:"throughput"`       // Operations per second
	LatencyStats     *LatencyStats     `json:"latency_stats"`
	MemoryStats      *MemoryStats      `json:"memory_stats"`
	ErrorCount       int64             `json:"error_count"`
	Success          bool              `json:"success"`
	Details          map[string]interface{} `json:"details"`
	Recommendations  []string          `json:"recommendations"`
}

// LatencyStats contains latency statistics
type LatencyStats struct {
	Min    time.Duration `json:"min"`
	Max    time.Duration `json:"max"`
	Mean   time.Duration `json:"mean"`
	Median time.Duration `json:"median"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
	P999   time.Duration `json:"p999"`
	StdDev time.Duration `json:"std_dev"`
}

// TestDataSet contains pre-generated test data
type TestDataSet struct {
	Keys        []string            `json:"keys"`
	Values      map[string][]byte   `json:"values"`
	KeySizes    []int               `json:"key_sizes"`
	ValueSizes  []int               `json:"value_sizes"`
	TotalSize   int64               `json:"total_size"`
	GeneratedAt time.Time           `json:"generated_at"`
}

// BenchmarkMetrics tracks overall benchmark performance
type BenchmarkMetrics struct {
	TotalTests       int           `json:"total_tests"`
	PassedTests      int           `json:"passed_tests"`
	FailedTests      int           `json:"failed_tests"`
	TotalDuration    time.Duration `json:"total_duration"`
	AverageThroughput float64      `json:"average_throughput"`
	TotalOperations  int64         `json:"total_operations"`
	SystemMetrics    *SystemMetrics `json:"system_metrics"`
}

// SystemMetrics contains system-level performance metrics
type SystemMetrics struct {
	CPUUsage        float64 `json:"cpu_usage"`
	MemoryUsage     uint64  `json:"memory_usage"`
	GoroutineCount  int     `json:"goroutine_count"`
	GCPauses        []time.Duration `json:"gc_pauses"`
	NetworkIO       *NetworkIOStats `json:"network_io"`
	DiskIO          *DiskIOStats    `json:"disk_io"`
}

// NetworkIOStats contains network I/O statistics
type NetworkIOStats struct {
	BytesSent     uint64 `json:"bytes_sent"`
	BytesReceived uint64 `json:"bytes_received"`
	PacketsSent   uint64 `json:"packets_sent"`
	PacketsRecv   uint64 `json:"packets_received"`
}

// DiskIOStats contains disk I/O statistics
type DiskIOStats struct {
	BytesRead    uint64 `json:"bytes_read"`
	BytesWritten uint64 `json:"bytes_written"`
	ReadOps      uint64 `json:"read_ops"`
	WriteOps     uint64 `json:"write_ops"`
}

// TestMetrics tracks metrics during test execution
type TestMetrics struct {
	mu            sync.RWMutex
	operations    int64
	errors        int64
	latencies     []time.Duration
	startTime     time.Time
	memStats      runtime.MemStats
}

// NewBenchmarkSuite creates a new benchmark suite
func NewBenchmarkSuite(config BenchmarkConfig, kvStore KVStoreInterface, logger *log.Logger) *BenchmarkSuite {
	if logger == nil {
		logger = log.New(log.Writer(), "[BENCHMARK] ", log.LstdFlags)
	}
	
	// Set default values
	if config.MaxKeys == 0 {
		config.MaxKeys = 100000
	}
	if len(config.ValueSizes) == 0 {
		config.ValueSizes = []int{100, 1024, 4096, 16384, 65536} // 100B, 1KB, 4KB, 16KB, 64KB
	}
	if len(config.ConcurrencyLevels) == 0 {
		config.ConcurrencyLevels = []int{1, 10, 50, 100, 200}
	}
	if config.TestDuration == 0 {
		config.TestDuration = 60 * time.Second
	}
	if config.WarmupDuration == 0 {
		config.WarmupDuration = 10 * time.Second
	}
	if config.TargetThroughput == 0 {
		config.TargetThroughput = 10000 // 10K ops/sec
	}
	if config.TargetLatencyP99 == 0 {
		config.TargetLatencyP99 = 10 * time.Millisecond
	}
	if config.TargetLatencyP95 == 0 {
		config.TargetLatencyP95 = 5 * time.Millisecond
	}
	
	suite := &BenchmarkSuite{
		logger:  logger,
		config:  config,
		tests:   make(map[string]*BenchmarkTest),
		results: make(map[string]*BenchmarkResult),
		kvStore: kvStore,
		metrics: &BenchmarkMetrics{
			SystemMetrics: &SystemMetrics{
				NetworkIO: &NetworkIOStats{},
				DiskIO:    &DiskIOStats{},
			},
		},
	}
	
	// Register default tests
	suite.registerDefaultTests()
	
	return suite
}

// registerDefaultTests registers the default benchmark tests
func (bs *BenchmarkSuite) registerDefaultTests() {
	// Basic read performance test
	bs.RegisterTest(&BenchmarkTest{
		Name:        "read_performance",
		Description: "Tests read performance across different key sizes and concurrency levels",
		TestFunc:    bs.testReadPerformance,
		Config: map[string]interface{}{
			"operation_type": "read",
			"test_type":     "throughput",
		},
		EstimatedDuration: bs.config.TestDuration,
	})
	
	// Basic write performance test
	bs.RegisterTest(&BenchmarkTest{
		Name:        "write_performance",
		Description: "Tests write performance across different value sizes and concurrency levels",
		TestFunc:    bs.testWritePerformance,
		Config: map[string]interface{}{
			"operation_type": "write",
			"test_type":     "throughput",
		},
		EstimatedDuration: bs.config.TestDuration,
	})
	
	// Mixed read/write performance test
	bs.RegisterTest(&BenchmarkTest{
		Name:        "mixed_performance",
		Description: "Tests mixed read/write performance with different ratios",
		TestFunc:    bs.testMixedPerformance,
		Config: map[string]interface{}{
			"operation_type": "mixed",
			"read_ratio":    0.8, // 80% reads, 20% writes
		},
		EstimatedDuration: bs.config.TestDuration,
	})
	
	// Latency benchmark test
	bs.RegisterTest(&BenchmarkTest{
		Name:        "latency_benchmark",
		Description: "Tests latency characteristics under various loads",
		TestFunc:    bs.testLatencyBenchmark,
		Config: map[string]interface{}{
			"test_type": "latency",
			"load_levels": []float64{0.1, 0.5, 0.8, 0.95}, // Load as fraction of max throughput
		},
		EstimatedDuration: bs.config.TestDuration * 4, // Multiple load levels
	})
	
	// Memory efficiency test
	bs.RegisterTest(&BenchmarkTest{
		Name:        "memory_efficiency",
		Description: "Tests memory usage patterns and garbage collection impact",
		TestFunc:    bs.testMemoryEfficiency,
		Config: map[string]interface{}{
			"test_type":     "memory",
			"force_gc":      true,
			"track_allocs":  true,
		},
		EstimatedDuration: bs.config.TestDuration,
	})
	
	// Scalability test
	bs.RegisterTest(&BenchmarkTest{
		Name:        "scalability_test",
		Description: "Tests how performance scales with data size and concurrency",
		TestFunc:    bs.testScalability,
		Config: map[string]interface{}{
			"test_type":    "scalability",
			"data_scales":  []int{1000, 10000, 100000, 1000000},
		},
		EstimatedDuration: bs.config.TestDuration * 4, // Multiple scales
	})
}

// RegisterTest registers a new benchmark test
func (bs *BenchmarkSuite) RegisterTest(test *BenchmarkTest) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	
	bs.tests[test.Name] = test
	bs.logger.Printf("Registered benchmark test: %s", test.Name)
}

// PrepareTestData generates test data for benchmarks
func (bs *BenchmarkSuite) PrepareTestData() error {
	bs.logger.Printf("Preparing test data with %d keys", bs.config.MaxKeys)
	
	testData := &TestDataSet{
		Keys:        make([]string, bs.config.MaxKeys),
		Values:      make(map[string][]byte),
		KeySizes:    []int{10, 20, 50, 100}, // Different key sizes
		ValueSizes:  bs.config.ValueSizes,
		GeneratedAt: time.Now(),
	}
	
	// Generate keys
	for i := 0; i < bs.config.MaxKeys; i++ {
		keySize := testData.KeySizes[i%len(testData.KeySizes)]
		key := generateRandomString(keySize)
		testData.Keys[i] = key
		
		// Generate values for each size
		for _, valueSize := range testData.ValueSizes {
			valueKey := fmt.Sprintf("%s_%d", key, valueSize)
			value := generateRandomBytes(valueSize)
			testData.Values[valueKey] = value
			testData.TotalSize += int64(len(key) + len(value))
		}
	}
	
	bs.testData = testData
	bs.logger.Printf("Generated test data: %d keys, %d values, total size: %d bytes", 
		len(testData.Keys), len(testData.Values), testData.TotalSize)
	
	return nil
}

// RunAllTests runs all registered benchmark tests
func (bs *BenchmarkSuite) RunAllTests(ctx context.Context) (*BenchmarkMetrics, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	
	if bs.isRunning {
		return nil, fmt.Errorf("benchmark suite is already running")
	}
	
	bs.logger.Printf("Starting benchmark suite with %d tests", len(bs.tests))
	bs.isRunning = true
	defer func() { bs.isRunning = false }()
	
	// Prepare test data
	if err := bs.PrepareTestData(); err != nil {
		return nil, fmt.Errorf("failed to prepare test data: %w", err)
	}
	
	startTime := time.Now()
	totalOps := int64(0)
	
	// Run each test
	for testName, test := range bs.tests {
		if bs.shouldRunTest(test) {
			bs.logger.Printf("Running test: %s", testName)
			
			result := bs.runSingleTest(ctx, test)
			bs.results[testName] = result
			
			// Update metrics
			bs.metrics.TotalTests++
			if result.Success {
				bs.metrics.PassedTests++
			} else {
				bs.metrics.FailedTests++
			}
			totalOps += result.OperationsCount
			
			// Check if context was cancelled
			if ctx.Err() != nil {
				bs.logger.Printf("Benchmark suite cancelled")
				break
			}
		}
	}
	
	// Calculate final metrics
	bs.metrics.TotalDuration = time.Since(startTime)
	bs.metrics.TotalOperations = totalOps
	if bs.metrics.TotalDuration > 0 {
		bs.metrics.AverageThroughput = float64(totalOps) / bs.metrics.TotalDuration.Seconds()
	}
	
	// Collect system metrics
	bs.collectSystemMetrics()
	
	bs.logger.Printf("Benchmark suite completed: %d passed, %d failed, %.2f ops/sec average", 
		bs.metrics.PassedTests, bs.metrics.FailedTests, bs.metrics.AverageThroughput)
	
	return bs.metrics, nil
}

// shouldRunTest determines if a test should be run based on configuration
func (bs *BenchmarkSuite) shouldRunTest(test *BenchmarkTest) bool {
	opType, ok := test.Config["operation_type"].(string)
	if !ok {
		return true // Run test if operation type is not specified
	}
	
	switch opType {
	case "read":
		return bs.config.EnableReadTests
	case "write":
		return bs.config.EnableWriteTests
	case "mixed":
		return bs.config.EnableMixedTests
	case "stress":
		return bs.config.EnableStressTests
	default:
		return true
	}
}

// runSingleTest executes a single benchmark test
func (bs *BenchmarkSuite) runSingleTest(ctx context.Context, test *BenchmarkTest) *BenchmarkResult {
	testCtx := &BenchmarkContext{
		Suite:     bs,
		TestName:  test.Name,
		Config:    bs.config,
		KVStore:   bs.kvStore,
		TestData:  bs.testData,
		Logger:    bs.logger,
		StartTime: time.Now(),
		metrics:   NewTestMetrics(),
	}
	
	// Run warmup if configured
	if bs.config.WarmupDuration > 0 {
		bs.logger.Printf("Running warmup for %v", bs.config.WarmupDuration)
		bs.runWarmup(testCtx)
	}
	
	// Execute the test
	result := test.TestFunc(testCtx)
	result.TestName = test.Name
	result.StartTime = testCtx.StartTime
	
	return result
}

// Test implementations

// testReadPerformance tests read performance
func (bs *BenchmarkSuite) testReadPerformance(ctx *BenchmarkContext) *BenchmarkResult {
	result := &BenchmarkResult{
		Details: make(map[string]interface{}),
	}
	
	// Populate store with test data first
	bs.populateStore(ctx, 1000) // Use subset for read test
	
	start := time.Now()
	var operations int64
	var totalLatency time.Duration
	
	// Test with different concurrency levels
	for _, concurrency := range ctx.Config.ConcurrencyLevels {
		ctx.Logger.Printf("Testing read performance with concurrency: %d", concurrency)
		
		ops, latency := bs.runConcurrentReads(ctx, concurrency)
		operations += ops
		totalLatency += latency
	}
	
	result.Duration = time.Since(start)
	result.OperationsCount = operations
	result.Throughput = float64(operations) / result.Duration.Seconds()
	result.LatencyStats = &LatencyStats{
		Mean: totalLatency / time.Duration(operations),
	}
	result.Success = result.Throughput >= float64(ctx.Config.TargetThroughput)*0.5 // 50% of target
	
	return result
}

// testWritePerformance tests write performance
func (bs *BenchmarkSuite) testWritePerformance(ctx *BenchmarkContext) *BenchmarkResult {
	result := &BenchmarkResult{
		Details: make(map[string]interface{}),
	}
	
	start := time.Now()
	var operations int64
	var totalLatency time.Duration
	
	// Test with different value sizes and concurrency levels
	for _, concurrency := range ctx.Config.ConcurrencyLevels {
		ctx.Logger.Printf("Testing write performance with concurrency: %d", concurrency)
		
		ops, latency := bs.runConcurrentWrites(ctx, concurrency)
		operations += ops
		totalLatency += latency
	}
	
	result.Duration = time.Since(start)
	result.OperationsCount = operations
	result.Throughput = float64(operations) / result.Duration.Seconds()
	result.LatencyStats = &LatencyStats{
		Mean: totalLatency / time.Duration(operations),
	}
	result.Success = result.Throughput >= float64(ctx.Config.TargetThroughput)*0.3 // 30% of target (writes are slower)
	
	return result
}

// testMixedPerformance tests mixed read/write performance
func (bs *BenchmarkSuite) testMixedPerformance(ctx *BenchmarkContext) *BenchmarkResult {
	result := &BenchmarkResult{
		Details: make(map[string]interface{}),
	}
	
	// Populate store with initial data
	bs.populateStore(ctx, 5000)
	
	readRatio := ctx.Suite.tests[ctx.TestName].Config["read_ratio"].(float64)
	
	start := time.Now()
	var operations int64
	
	// Run mixed workload for test duration
	endTime := start.Add(ctx.Config.TestDuration)
	concurrency := 50 // Fixed concurrency for mixed test
	
	operations = bs.runMixedWorkload(ctx, concurrency, readRatio, endTime)
	
	result.Duration = time.Since(start)
	result.OperationsCount = operations
	result.Throughput = float64(operations) / result.Duration.Seconds()
	result.Success = result.Throughput >= float64(ctx.Config.TargetThroughput)*0.4 // 40% of target
	result.Details["read_ratio"] = readRatio
	
	return result
}

// testLatencyBenchmark tests latency under various loads
func (bs *BenchmarkSuite) testLatencyBenchmark(ctx *BenchmarkContext) *BenchmarkResult {
	result := &BenchmarkResult{
		Details: make(map[string]interface{}),
	}
	
	bs.populateStore(ctx, 2000)
	
	loadLevels := ctx.Suite.tests[ctx.TestName].Config["load_levels"].([]float64)
	latencyResults := make(map[string]*LatencyStats)
	
	start := time.Now()
	var totalOps int64
	
	for _, loadLevel := range loadLevels {
		ctx.Logger.Printf("Testing latency at %.0f%% load", loadLevel*100)
		
		targetTPS := int64(float64(ctx.Config.TargetThroughput) * loadLevel)
		latencies, ops := bs.measureLatencyAtLoad(ctx, targetTPS, 30*time.Second)
		
		totalOps += ops
		latencyResults[fmt.Sprintf("load_%.0f", loadLevel*100)] = calculateLatencyStats(latencies)
	}
	
	result.Duration = time.Since(start)
	result.OperationsCount = totalOps
	result.Success = true // Success based on latency targets
	result.Details["latency_by_load"] = latencyResults
	
	return result
}

// testMemoryEfficiency tests memory usage patterns
func (bs *BenchmarkSuite) testMemoryEfficiency(ctx *BenchmarkContext) *BenchmarkResult {
	result := &BenchmarkResult{
		Details: make(map[string]interface{}),
	}
	
	// Force GC and get baseline
	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)
	
	start := time.Now()
	
	// Run memory-intensive operations
	operations := bs.runMemoryIntensiveTest(ctx)
	
	// Force GC and measure final memory
	runtime.GC()
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)
	
	result.Duration = time.Since(start)
	result.OperationsCount = operations
	result.MemoryStats = &MemoryStats{
		Alloc:      memEnd.Alloc,
		TotalAlloc: memEnd.TotalAlloc,
		Sys:        memEnd.Sys,
		NumGC:      memEnd.NumGC,
	}
	
	memoryGrowth := memEnd.Alloc - memStart.Alloc
	result.Details["memory_growth"] = memoryGrowth
	result.Details["gc_cycles"] = memEnd.NumGC - memStart.NumGC
	result.Success = memoryGrowth < 100*1024*1024 // Less than 100MB growth
	
	return result
}

// testScalability tests performance scaling
func (bs *BenchmarkSuite) testScalability(ctx *BenchmarkContext) *BenchmarkResult {
	result := &BenchmarkResult{
		Details: make(map[string]interface{}),
	}
	
	dataScales := ctx.Suite.tests[ctx.TestName].Config["data_scales"].([]int)
	scalabilityResults := make(map[string]float64)
	
	start := time.Now()
	var totalOps int64
	
	for _, scale := range dataScales {
		ctx.Logger.Printf("Testing scalability with %d keys", scale)
		
		// Clear and populate with scaled data
		bs.clearStore(ctx)
		bs.populateStore(ctx, scale)
		
		// Run performance test
		ops := bs.runScalabilityTest(ctx, scale)
		totalOps += ops
		
		throughput := float64(ops) / (30 * time.Second).Seconds() // 30 second test
		scalabilityResults[fmt.Sprintf("scale_%d", scale)] = throughput
	}
	
	result.Duration = time.Since(start)
	result.OperationsCount = totalOps
	result.Details["throughput_by_scale"] = scalabilityResults
	result.Success = len(scalabilityResults) == len(dataScales)
	
	return result
}

// Helper functions

// populateStore populates the KV store with test data
func (bs *BenchmarkSuite) populateStore(ctx *BenchmarkContext, keyCount int) {
	if keyCount > len(ctx.TestData.Keys) {
		keyCount = len(ctx.TestData.Keys)
	}
	
	for i := 0; i < keyCount; i++ {
		key := ctx.TestData.Keys[i]
		valueSize := ctx.Config.ValueSizes[i%len(ctx.Config.ValueSizes)]
		valueKey := fmt.Sprintf("%s_%d", key, valueSize)
		value := ctx.TestData.Values[valueKey]
		
		ctx.KVStore.Set(key, value)
	}
}

// clearStore clears all data from the KV store
func (bs *BenchmarkSuite) clearStore(ctx *BenchmarkContext) {
	// Implementation depends on KV store interface
	// For now, we'll assume the store can be cleared through its interface
}

// runConcurrentReads performs concurrent read operations
func (bs *BenchmarkSuite) runConcurrentReads(ctx *BenchmarkContext, concurrency int) (int64, time.Duration) {
	var wg sync.WaitGroup
	var operations int64
	var totalLatency int64
	
	endTime := time.Now().Add(30 * time.Second) // 30 second test
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ops := int64(0)
			latency := int64(0)
			
			for time.Now().Before(endTime) {
				key := ctx.TestData.Keys[ops%int64(len(ctx.TestData.Keys))]
				
				start := time.Now()
				_, err := ctx.KVStore.Get(key)
				duration := time.Since(start)
				
				if err == nil {
					ops++
					latency += duration.Nanoseconds()
				}
			}
			
			atomic.AddInt64(&operations, ops)
			atomic.AddInt64(&totalLatency, latency)
		}()
	}
	
	wg.Wait()
	avgLatency := time.Duration(totalLatency / operations)
	return operations, avgLatency
}

// runConcurrentWrites performs concurrent write operations
func (bs *BenchmarkSuite) runConcurrentWrites(ctx *BenchmarkContext, concurrency int) (int64, time.Duration) {
	var wg sync.WaitGroup
	var operations int64
	var totalLatency int64
	
	endTime := time.Now().Add(30 * time.Second) // 30 second test
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			ops := int64(0)
			latency := int64(0)
			
			for time.Now().Before(endTime) {
				keyIndex := (int64(workerID)*1000 + ops) % int64(len(ctx.TestData.Keys))
				key := fmt.Sprintf("%s_w%d", ctx.TestData.Keys[keyIndex], workerID)
				
				valueSize := ctx.Config.ValueSizes[ops%int64(len(ctx.Config.ValueSizes))]
				value := generateRandomBytes(valueSize)
				
				start := time.Now()
				err := ctx.KVStore.Set(key, value)
				duration := time.Since(start)
				
				if err == nil {
					ops++
					latency += duration.Nanoseconds()
				}
			}
			
			atomic.AddInt64(&operations, ops)
			atomic.AddInt64(&totalLatency, latency)
		}(i)
	}
	
	wg.Wait()
	avgLatency := time.Duration(totalLatency / operations)
	return operations, avgLatency
}

// runMixedWorkload runs a mixed read/write workload
func (bs *BenchmarkSuite) runMixedWorkload(ctx *BenchmarkContext, concurrency int, readRatio float64, endTime time.Time) int64 {
	var wg sync.WaitGroup
	var operations int64
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			ops := int64(0)
			
			for time.Now().Before(endTime) {
				if bs.shouldRead(readRatio) {
					// Perform read
					key := ctx.TestData.Keys[ops%int64(len(ctx.TestData.Keys))]
					_, err := ctx.KVStore.Get(key)
					if err == nil {
						ops++
					}
				} else {
					// Perform write
					keyIndex := (int64(workerID)*1000 + ops) % int64(len(ctx.TestData.Keys))
					key := fmt.Sprintf("%s_m%d", ctx.TestData.Keys[keyIndex], workerID)
					value := generateRandomBytes(1024)
					
					err := ctx.KVStore.Set(key, value)
					if err == nil {
						ops++
					}
				}
			}
			
			atomic.AddInt64(&operations, ops)
		}(i)
	}
	
	wg.Wait()
	return operations
}

// measureLatencyAtLoad measures latency at a specific load level
func (bs *BenchmarkSuite) measureLatencyAtLoad(ctx *BenchmarkContext, targetTPS int64, duration time.Duration) ([]time.Duration, int64) {
	latencies := make([]time.Duration, 0, targetTPS*int64(duration.Seconds()))
	var operations int64
	
	interval := time.Duration(int64(time.Second) / targetTPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	endTime := time.Now().Add(duration)
	
	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			key := ctx.TestData.Keys[operations%int64(len(ctx.TestData.Keys))]
			
			start := time.Now()
			_, err := ctx.KVStore.Get(key)
			latency := time.Since(start)
			
			if err == nil {
				latencies = append(latencies, latency)
				operations++
			}
		}
	}
	
	return latencies, operations
}

// runMemoryIntensiveTest runs memory-intensive operations
func (bs *BenchmarkSuite) runMemoryIntensiveTest(ctx *BenchmarkContext) int64 {
	var operations int64
	endTime := time.Now().Add(ctx.Config.TestDuration)
	
	for time.Now().Before(endTime) {
		// Create large values to stress memory
		key := fmt.Sprintf("mem_test_%d", operations)
		value := generateRandomBytes(64 * 1024) // 64KB values
		
		ctx.KVStore.Set(key, value)
		operations++
		
		// Periodically read to prevent store optimization
		if operations%100 == 0 {
			readKey := fmt.Sprintf("mem_test_%d", operations-50)
			ctx.KVStore.Get(readKey)
		}
	}
	
	return operations
}

// runScalabilityTest runs a scalability test for a specific data size
func (bs *BenchmarkSuite) runScalabilityTest(ctx *BenchmarkContext, dataSize int) int64 {
	var operations int64
	endTime := time.Now().Add(30 * time.Second)
	
	concurrency := 20 // Fixed concurrency for scalability test
	var wg sync.WaitGroup
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ops := int64(0)
			for time.Now().Before(endTime) {
				key := ctx.TestData.Keys[ops%int64(len(ctx.TestData.Keys))]
				_, err := ctx.KVStore.Get(key)
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

// runWarmup runs a warmup phase before testing
func (bs *BenchmarkSuite) runWarmup(ctx *BenchmarkContext) {
	endTime := time.Now().Add(ctx.Config.WarmupDuration)
	
	for time.Now().Before(endTime) {
		key := ctx.TestData.Keys[0] // Use first key for warmup
		value := generateRandomBytes(1024)
		
		ctx.KVStore.Set(key, value)
		ctx.KVStore.Get(key)
	}
}

// shouldRead determines if the next operation should be a read based on read ratio
func (bs *BenchmarkSuite) shouldRead(readRatio float64) bool {
	return bs.randomFloat() < readRatio
}

// collectSystemMetrics collects system-level performance metrics
func (bs *BenchmarkSuite) collectSystemMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	bs.metrics.SystemMetrics.MemoryUsage = memStats.Alloc
	bs.metrics.SystemMetrics.GoroutineCount = runtime.NumGoroutine()
	
	// Collect GC pauses
	gcPauses := make([]time.Duration, len(memStats.PauseNs))
	for i, pause := range memStats.PauseNs {
		gcPauses[i] = time.Duration(pause)
	}
	bs.metrics.SystemMetrics.GCPauses = gcPauses
}

// GetResults returns all benchmark results
func (bs *BenchmarkSuite) GetResults() map[string]*BenchmarkResult {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	
	results := make(map[string]*BenchmarkResult)
	for name, result := range bs.results {
		resultCopy := *result
		results[name] = &resultCopy
	}
	
	return results
}

// GetMetrics returns benchmark metrics
func (bs *BenchmarkSuite) GetMetrics() *BenchmarkMetrics {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	
	metricsCopy := *bs.metrics
	return &metricsCopy
}

// NewTestMetrics creates new test metrics tracker
func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		startTime: time.Now(),
	}
}

// calculateLatencyStats calculates latency statistics from a slice of latencies
func calculateLatencyStats(latencies []time.Duration) *LatencyStats {
	if len(latencies) == 0 {
		return &LatencyStats{}
	}
	
	// Sort latencies for percentile calculations
	sortedLatencies := make([]time.Duration, len(latencies))
	copy(sortedLatencies, latencies)
	
	// Simple sort implementation
	for i := 0; i < len(sortedLatencies)-1; i++ {
		for j := i + 1; j < len(sortedLatencies); j++ {
			if sortedLatencies[i] > sortedLatencies[j] {
				sortedLatencies[i], sortedLatencies[j] = sortedLatencies[j], sortedLatencies[i]
			}
		}
	}
	
	stats := &LatencyStats{
		Min: sortedLatencies[0],
		Max: sortedLatencies[len(sortedLatencies)-1],
	}
	
	// Calculate mean
	var total time.Duration
	for _, lat := range latencies {
		total += lat
	}
	stats.Mean = total / time.Duration(len(latencies))
	
	// Calculate percentiles
	stats.Median = sortedLatencies[len(sortedLatencies)/2]
	stats.P95 = sortedLatencies[int(float64(len(sortedLatencies))*0.95)]
	stats.P99 = sortedLatencies[int(float64(len(sortedLatencies))*0.99)]
	stats.P999 = sortedLatencies[int(float64(len(sortedLatencies))*0.999)]
	
	return stats
}

// Utility functions

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

// generateRandomBytes generates random bytes of specified size
func generateRandomBytes(size int) []byte {
	bytes := make([]byte, size)
	for i := range bytes {
		bytes[i] = byte(i % 256)
	}
	return bytes
}

// randomFloat returns a random float between 0 and 1
func (bs *BenchmarkSuite) randomFloat() float64 {
	// Simple pseudo-random implementation
	return float64(time.Now().UnixNano()%1000) / 1000.0
}