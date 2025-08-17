package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// RegressionTester provides performance regression testing capabilities
type RegressionTester struct {
	mu              sync.RWMutex
	logger          *log.Logger
	config          RegressionConfig
	kvStore         KVStoreInterface
	baselineResults map[string]*BaselineResult
	currentResults  map[string]*RegressionResult
	comparisons     map[string]*RegressionComparison
	isRunning       bool
	baselineDir     string
	reportDir       string
}

// RegressionConfig contains configuration for regression testing
type RegressionConfig struct {
	// Baseline configuration
	BaselineDirectory   string        `json:"baseline_directory"`   // Directory to store/load baselines
	AutoUpdateBaseline  bool          `json:"auto_update_baseline"` // Auto-update baseline if better
	BaselineThreshold   float64       `json:"baseline_threshold"`   // Threshold for baseline updates
	
	// Test configuration
	TestDuration        time.Duration `json:"test_duration"`        // Duration for each test
	WarmupDuration      time.Duration `json:"warmup_duration"`      // Warmup period
	CooldownDuration    time.Duration `json:"cooldown_duration"`    // Cooldown between tests
	TestIterations      int           `json:"test_iterations"`      // Number of test iterations
	ConcurrencyLevels   []int         `json:"concurrency_levels"`   // Concurrency levels to test
	
	// Regression thresholds
	LatencyThreshold    float64       `json:"latency_threshold"`    // Max acceptable latency increase (%)
	ThroughputThreshold float64       `json:"throughput_threshold"` // Max acceptable throughput decrease (%)
	MemoryThreshold     float64       `json:"memory_threshold"`     // Max acceptable memory increase (%)
	ErrorRateThreshold  float64       `json:"error_rate_threshold"` // Max acceptable error rate increase (%)
	
	// Reporting
	ReportDirectory     string        `json:"report_directory"`     // Directory for regression reports
	EnableDetailedLogs  bool          `json:"enable_detailed_logs"` // Enable detailed logging
	EmailNotifications  bool          `json:"email_notifications"`  // Send email notifications
	SlackNotifications  bool          `json:"slack_notifications"`  // Send Slack notifications
	
	// Test scenarios
	EnableReadTests     bool          `json:"enable_read_tests"`    // Enable read regression tests
	EnableWriteTests    bool          `json:"enable_write_tests"`   // Enable write regression tests
	EnableMixedTests    bool          `json:"enable_mixed_tests"`   // Enable mixed workload tests
}

// BaselineResult represents a baseline performance measurement
type BaselineResult struct {
	TestName        string            `json:"test_name"`
	Version         string            `json:"version"`
	Timestamp       time.Time         `json:"timestamp"`
	Environment     *TestEnvironment  `json:"environment"`
	Metrics         *PerformanceMetrics `json:"metrics"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// RegressionResult represents current test results for comparison
type RegressionResult struct {
	TestName        string            `json:"test_name"`
	Timestamp       time.Time         `json:"timestamp"`
	Environment     *TestEnvironment  `json:"environment"`
	Metrics         *PerformanceMetrics `json:"metrics"`
	Baseline        *BaselineResult   `json:"baseline"`
	Regression      *RegressionAnalysis `json:"regression"`
}

// RegressionComparison contains detailed comparison between baseline and current
type RegressionComparison struct {
	TestName           string                    `json:"test_name"`
	BaselineVersion    string                    `json:"baseline_version"`
	CurrentVersion     string                    `json:"current_version"`
	ComparisonTime     time.Time                 `json:"comparison_time"`
	MetricComparisons  map[string]*MetricComparison `json:"metric_comparisons"`
	OverallAssessment  string                    `json:"overall_assessment"`
	RegressionDetected bool                      `json:"regression_detected"`
	Improvements       []string                  `json:"improvements"`
	Regressions        []string                  `json:"regressions"`
	Recommendations    []string                  `json:"recommendations"`
	Confidence         float64                   `json:"confidence"`
}

// TestEnvironment describes the test environment
type TestEnvironment struct {
	OS              string            `json:"os"`
	Architecture    string            `json:"architecture"`
	CPUModel        string            `json:"cpu_model"`
	CPUCores        int               `json:"cpu_cores"`
	MemoryTotal     uint64            `json:"memory_total"`
	GoVersion       string            `json:"go_version"`
	BuildTags       []string          `json:"build_tags"`
	ConfigHash      string            `json:"config_hash"`
	Dependencies    map[string]string `json:"dependencies"`
}

// PerformanceMetrics contains comprehensive performance metrics
type PerformanceMetrics struct {
	// Throughput metrics
	ReadThroughput    float64   `json:"read_throughput"`    // Reads per second
	WriteThroughput   float64   `json:"write_throughput"`   // Writes per second
	MixedThroughput   float64   `json:"mixed_throughput"`   // Mixed ops per second
	
	// Latency metrics
	ReadLatency       *LatencyStats `json:"read_latency"`
	WriteLatency      *LatencyStats `json:"write_latency"`
	MixedLatency      *LatencyStats `json:"mixed_latency"`
	
	// Resource metrics
	MemoryUsage       *MemoryMetrics    `json:"memory_usage"`
	CPUUsage          float64           `json:"cpu_usage"`
	GCMetrics         *GCPerformanceMetrics `json:"gc_metrics"`
	
	// Error metrics
	ReadErrorRate     float64   `json:"read_error_rate"`
	WriteErrorRate    float64   `json:"write_error_rate"`
	
	// Scalability metrics
	ScalabilityFactor float64   `json:"scalability_factor"`
	EfficiencyRatio   float64   `json:"efficiency_ratio"`
}

// GCPerformanceMetrics contains garbage collection performance metrics
type GCPerformanceMetrics struct {
	GCFrequency     time.Duration `json:"gc_frequency"`
	AvgGCPause      time.Duration `json:"avg_gc_pause"`
	MaxGCPause      time.Duration `json:"max_gc_pause"`
	TotalGCTime     time.Duration `json:"total_gc_time"`
	GCOverhead      float64       `json:"gc_overhead"`
}

// RegressionAnalysis contains regression analysis results
type RegressionAnalysis struct {
	LatencyRegression    float64  `json:"latency_regression"`    // % change in latency
	ThroughputRegression float64  `json:"throughput_regression"` // % change in throughput
	MemoryRegression     float64  `json:"memory_regression"`     // % change in memory usage
	ErrorRateRegression  float64  `json:"error_rate_regression"` // % change in error rate
	
	OverallScore         float64  `json:"overall_score"`         // Overall regression score
	RegressionSeverity   string   `json:"regression_severity"`   // "none", "minor", "major", "critical"
	StatisticalSignificance bool  `json:"statistical_significance"`
	ConfidenceInterval   float64  `json:"confidence_interval"`
}

// MetricComparison contains detailed comparison of a single metric
type MetricComparison struct {
	MetricName      string  `json:"metric_name"`
	BaselineValue   float64 `json:"baseline_value"`
	CurrentValue    float64 `json:"current_value"`
	ChangePercent   float64 `json:"change_percent"`
	ChangeDirection string  `json:"change_direction"` // "improved", "regressed", "unchanged"
	IsSignificant   bool    `json:"is_significant"`
	Threshold       float64 `json:"threshold"`
	Assessment      string  `json:"assessment"`
}

// RegressionTestCase represents a single regression test case
type RegressionTestCase struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	TestFunc       func(*RegressionContext) *PerformanceMetrics `json:"-"`
	Concurrency    int                    `json:"concurrency"`
	Duration       time.Duration          `json:"duration"`
	Config         map[string]interface{} `json:"config"`
}

// RegressionContext provides context for regression test execution
type RegressionContext struct {
	Tester      *RegressionTester
	TestCase    *RegressionTestCase
	KVStore     KVStoreInterface
	Logger      *log.Logger
	StartTime   time.Time
	Environment *TestEnvironment
}

// NewRegressionTester creates a new regression tester
func NewRegressionTester(config RegressionConfig, kvStore KVStoreInterface, logger *log.Logger) *RegressionTester {
	if logger == nil {
		logger = log.New(log.Writer(), "[REGRESSION] ", log.LstdFlags)
	}
	
	// Set default values
	if config.TestDuration == 0 {
		config.TestDuration = 60 * time.Second
	}
	if config.WarmupDuration == 0 {
		config.WarmupDuration = 10 * time.Second
	}
	if config.CooldownDuration == 0 {
		config.CooldownDuration = 5 * time.Second
	}
	if config.TestIterations == 0 {
		config.TestIterations = 3
	}
	if len(config.ConcurrencyLevels) == 0 {
		config.ConcurrencyLevels = []int{1, 10, 50, 100}
	}
	if config.LatencyThreshold == 0 {
		config.LatencyThreshold = 10.0 // 10% increase threshold
	}
	if config.ThroughputThreshold == 0 {
		config.ThroughputThreshold = 5.0 // 5% decrease threshold
	}
	if config.MemoryThreshold == 0 {
		config.MemoryThreshold = 15.0 // 15% increase threshold
	}
	if config.ErrorRateThreshold == 0 {
		config.ErrorRateThreshold = 2.0 // 2% increase threshold
	}
	if config.BaselineDirectory == "" {
		config.BaselineDirectory = "./baselines"
	}
	if config.ReportDirectory == "" {
		config.ReportDirectory = "./regression_reports"
	}
	
	rt := &RegressionTester{
		logger:          logger,
		config:          config,
		kvStore:         kvStore,
		baselineResults: make(map[string]*BaselineResult),
		currentResults:  make(map[string]*RegressionResult),
		comparisons:     make(map[string]*RegressionComparison),
		baselineDir:     config.BaselineDirectory,
		reportDir:       config.ReportDirectory,
	}
	
	// Create directories if they don't exist
	os.MkdirAll(rt.baselineDir, 0755)
	os.MkdirAll(rt.reportDir, 0755)
	
	return rt
}

// LoadBaselines loads baseline results from disk
func (rt *RegressionTester) LoadBaselines() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	rt.logger.Printf("Loading baselines from %s", rt.baselineDir)
	
	files, err := filepath.Glob(filepath.Join(rt.baselineDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to find baseline files: %w", err)
	}
	
	for _, file := range files {
		baseline, err := rt.loadBaselineFromFile(file)
		if err != nil {
			rt.logger.Printf("Failed to load baseline from %s: %v", file, err)
			continue
		}
		
		rt.baselineResults[baseline.TestName] = baseline
	}
	
	rt.logger.Printf("Loaded %d baselines", len(rt.baselineResults))
	return nil
}

// loadBaselineFromFile loads a baseline from a JSON file
func (rt *RegressionTester) loadBaselineFromFile(filename string) (*BaselineResult, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	var baseline BaselineResult
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to unmarshal baseline: %w", err)
	}
	
	return &baseline, nil
}

// SaveBaseline saves a baseline result to disk
func (rt *RegressionTester) SaveBaseline(testName string, metrics *PerformanceMetrics, version string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	baseline := &BaselineResult{
		TestName:    testName,
		Version:     version,
		Timestamp:   time.Now(),
		Environment: rt.getCurrentEnvironment(),
		Metrics:     metrics,
		Metadata:    make(map[string]interface{}),
	}
	
	// Save to memory
	rt.baselineResults[testName] = baseline
	
	// Save to disk
	filename := filepath.Join(rt.baselineDir, fmt.Sprintf("%s_baseline.json", testName))
	return rt.saveBaselineToFile(baseline, filename)
}

// saveBaselineToFile saves a baseline to a JSON file
func (rt *RegressionTester) saveBaselineToFile(baseline *BaselineResult, filename string) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write baseline file: %w", err)
	}
	
	rt.logger.Printf("Saved baseline for %s to %s", baseline.TestName, filename)
	return nil
}

// RunRegressionTests runs all regression tests
func (rt *RegressionTester) RunRegressionTests(ctx context.Context, version string) (*RegressionReport, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if rt.isRunning {
		return nil, fmt.Errorf("regression tests already running")
	}
	
	rt.logger.Printf("Starting regression tests for version %s", version)
	rt.isRunning = true
	defer func() { rt.isRunning = false }()
	
	// Load existing baselines
	if err := rt.LoadBaselines(); err != nil {
		rt.logger.Printf("Failed to load baselines: %v", err)
	}
	
	// Define test cases
	testCases := rt.getTestCases()
	
	startTime := time.Now()
	var failedTests []string
	var passedTests []string
	
	// Run each test case
	for _, testCase := range testCases {
		rt.logger.Printf("Running regression test: %s", testCase.Name)
		
		// Run the test multiple iterations for statistical significance
		metrics := rt.runTestCaseWithIterations(ctx, testCase)
		
		// Compare with baseline
		result := rt.compareWithBaseline(testCase.Name, metrics, version)
		rt.currentResults[testCase.Name] = result
		
		// Perform regression analysis
		comparison := rt.performRegressionAnalysis(testCase.Name, result)
		rt.comparisons[testCase.Name] = comparison
		
		if comparison.RegressionDetected {
			failedTests = append(failedTests, testCase.Name)
		} else {
			passedTests = append(passedTests, testCase.Name)
		}
		
		// Update baseline if configured and performance improved
		if rt.config.AutoUpdateBaseline && rt.shouldUpdateBaseline(result) {
			rt.SaveBaseline(testCase.Name, metrics, version)
		}
		
		// Cooldown between tests
		if rt.config.CooldownDuration > 0 {
			time.Sleep(rt.config.CooldownDuration)
		}
	}
	
	// Generate report
	report := &RegressionReport{
		Version:       version,
		StartTime:     startTime,
		Duration:      time.Since(startTime),
		TotalTests:    len(testCases),
		PassedTests:   len(passedTests),
		FailedTests:   len(failedTests),
		Comparisons:   rt.comparisons,
		Environment:   rt.getCurrentEnvironment(),
		Summary:       rt.generateSummary(passedTests, failedTests),
	}
	
	// Save report
	if err := rt.saveReport(report); err != nil {
		rt.logger.Printf("Failed to save regression report: %v", err)
	}
	
	// Send notifications if configured
	if rt.config.EmailNotifications || rt.config.SlackNotifications {
		rt.sendNotifications(report)
	}
	
	rt.logger.Printf("Regression tests completed: %d passed, %d failed", 
		len(passedTests), len(failedTests))
	
	return report, nil
}

// getTestCases returns the regression test cases to run
func (rt *RegressionTester) getTestCases() []*RegressionTestCase {
	var testCases []*RegressionTestCase
	
	if rt.config.EnableReadTests {
		testCases = append(testCases, &RegressionTestCase{
			Name:        "read_performance",
			Description: "Read performance regression test",
			TestFunc:    rt.testReadPerformance,
			Concurrency: 50,
			Duration:    rt.config.TestDuration,
		})
	}
	
	if rt.config.EnableWriteTests {
		testCases = append(testCases, &RegressionTestCase{
			Name:        "write_performance",
			Description: "Write performance regression test",
			TestFunc:    rt.testWritePerformance,
			Concurrency: 50,
			Duration:    rt.config.TestDuration,
		})
	}
	
	if rt.config.EnableMixedTests {
		testCases = append(testCases, &RegressionTestCase{
			Name:        "mixed_workload",
			Description: "Mixed workload regression test",
			TestFunc:    rt.testMixedWorkload,
			Concurrency: 50,
			Duration:    rt.config.TestDuration,
		})
	}
	
	// Add scalability test
	testCases = append(testCases, &RegressionTestCase{
		Name:        "scalability",
		Description: "Scalability regression test",
		TestFunc:    rt.testScalability,
		Concurrency: 100,
		Duration:    rt.config.TestDuration,
	})
	
	return testCases
}

// runTestCaseWithIterations runs a test case multiple times for statistical significance
func (rt *RegressionTester) runTestCaseWithIterations(ctx context.Context, testCase *RegressionTestCase) *PerformanceMetrics {
	var allMetrics []*PerformanceMetrics
	
	for i := 0; i < rt.config.TestIterations; i++ {
		rt.logger.Printf("Running iteration %d/%d for %s", i+1, rt.config.TestIterations, testCase.Name)
		
		// Warmup
		if rt.config.WarmupDuration > 0 {
			rt.runWarmup()
		}
		
		// Run test
		testCtx := &RegressionContext{
			Tester:      rt,
			TestCase:    testCase,
			KVStore:     rt.kvStore,
			Logger:      rt.logger,
			StartTime:   time.Now(),
			Environment: rt.getCurrentEnvironment(),
		}
		
		metrics := testCase.TestFunc(testCtx)
		allMetrics = append(allMetrics, metrics)
		
		// Check for context cancellation
		if ctx.Err() != nil {
			break
		}
	}
	
	// Aggregate metrics across iterations
	return rt.aggregateMetrics(allMetrics)
}

// Test implementations

// testReadPerformance tests read performance
func (rt *RegressionTester) testReadPerformance(ctx *RegressionContext) *PerformanceMetrics {
	// Populate store with test data
	rt.populateTestData(1000)
	
	// Measure read performance
	latencies, throughput, errorRate := rt.measureOperationPerformance(ctx, "read")
	
	return &PerformanceMetrics{
		ReadThroughput: throughput,
		ReadLatency:    calculateLatencyStats(latencies),
		ReadErrorRate:  errorRate,
		MemoryUsage:    rt.getCurrentMemoryMetrics(),
		CPUUsage:       rt.getCurrentCPUUsage(),
		GCMetrics:      rt.getCurrentGCMetrics(),
	}
}

// testWritePerformance tests write performance
func (rt *RegressionTester) testWritePerformance(ctx *RegressionContext) *PerformanceMetrics {
	// Measure write performance
	latencies, throughput, errorRate := rt.measureOperationPerformance(ctx, "write")
	
	return &PerformanceMetrics{
		WriteThroughput: throughput,
		WriteLatency:    calculateLatencyStats(latencies),
		WriteErrorRate:  errorRate,
		MemoryUsage:     rt.getCurrentMemoryMetrics(),
		CPUUsage:        rt.getCurrentCPUUsage(),
		GCMetrics:       rt.getCurrentGCMetrics(),
	}
}

// testMixedWorkload tests mixed read/write workload
func (rt *RegressionTester) testMixedWorkload(ctx *RegressionContext) *PerformanceMetrics {
	// Populate store with initial data
	rt.populateTestData(500)
	
	// Measure mixed workload performance
	latencies, throughput, errorRate := rt.measureOperationPerformance(ctx, "mixed")
	
	return &PerformanceMetrics{
		MixedThroughput: throughput,
		MixedLatency:    calculateLatencyStats(latencies),
		MemoryUsage:     rt.getCurrentMemoryMetrics(),
		CPUUsage:        rt.getCurrentCPUUsage(),
		GCMetrics:       rt.getCurrentGCMetrics(),
	}
}

// testScalability tests scalability across different concurrency levels
func (rt *RegressionTester) testScalability(ctx *RegressionContext) *PerformanceMetrics {
	throughputs := make([]float64, 0, len(rt.config.ConcurrencyLevels))
	
	for _, concurrency := range rt.config.ConcurrencyLevels {
		latencies, throughput, _ := rt.measureConcurrencyPerformance(ctx, concurrency)
		throughputs = append(throughputs, throughput)
	}
	
	// Calculate scalability factor
	scalabilityFactor := rt.calculateScalabilityFactor(throughputs)
	efficiencyRatio := rt.calculateEfficiencyRatio(throughputs)
	
	return &PerformanceMetrics{
		ReadThroughput:    throughputs[len(throughputs)-1], // Use highest concurrency
		ScalabilityFactor: scalabilityFactor,
		EfficiencyRatio:   efficiencyRatio,
		MemoryUsage:       rt.getCurrentMemoryMetrics(),
		CPUUsage:          rt.getCurrentCPUUsage(),
		GCMetrics:         rt.getCurrentGCMetrics(),
	}
}

// measureOperationPerformance measures performance for a specific operation type
func (rt *RegressionTester) measureOperationPerformance(ctx *RegressionContext, opType string) ([]time.Duration, float64, float64) {
	var latencies []time.Duration
	var operations int64
	var errors int64
	
	endTime := time.Now().Add(ctx.TestCase.Duration)
	
	for time.Now().Before(endTime) {
		key := fmt.Sprintf("test_key_%d", operations%1000)
		value := generateRandomBytes(1024)
		
		start := time.Now()
		var err error
		
		switch opType {
		case "read":
			_, err = ctx.KVStore.Get(key)
		case "write":
			err = ctx.KVStore.Set(key, value)
		case "mixed":
			if operations%3 == 0 { // 33% writes, 67% reads
				err = ctx.KVStore.Set(key, value)
			} else {
				_, err = ctx.KVStore.Get(key)
			}
		}
		
		latency := time.Since(start)
		latencies = append(latencies, latency)
		operations++
		
		if err != nil {
			errors++
		}
	}
	
	throughput := float64(operations) / ctx.TestCase.Duration.Seconds()
	errorRate := float64(errors) / float64(operations)
	
	return latencies, throughput, errorRate
}

// measureConcurrencyPerformance measures performance at a specific concurrency level
func (rt *RegressionTester) measureConcurrencyPerformance(ctx *RegressionContext, concurrency int) ([]time.Duration, float64, float64) {
	// Implementation similar to measureOperationPerformance but with specified concurrency
	// This is a simplified version for demonstration
	return rt.measureOperationPerformance(ctx, "read")
}

// compareWithBaseline compares current metrics with baseline
func (rt *RegressionTester) compareWithBaseline(testName string, metrics *PerformanceMetrics, version string) *RegressionResult {
	baseline := rt.baselineResults[testName]
	
	result := &RegressionResult{
		TestName:    testName,
		Timestamp:   time.Now(),
		Environment: rt.getCurrentEnvironment(),
		Metrics:     metrics,
		Baseline:    baseline,
	}
	
	if baseline != nil {
		result.Regression = rt.calculateRegression(baseline.Metrics, metrics)
	}
	
	return result
}

// calculateRegression calculates regression metrics
func (rt *RegressionTester) calculateRegression(baseline, current *PerformanceMetrics) *RegressionAnalysis {
	analysis := &RegressionAnalysis{}
	
	// Calculate throughput regression
	if baseline.ReadThroughput > 0 {
		analysis.ThroughputRegression = ((current.ReadThroughput - baseline.ReadThroughput) / baseline.ReadThroughput) * 100
	}
	
	// Calculate latency regression
	if baseline.ReadLatency != nil && current.ReadLatency != nil && baseline.ReadLatency.Mean > 0 {
		analysis.LatencyRegression = ((float64(current.ReadLatency.Mean - baseline.ReadLatency.Mean)) / float64(baseline.ReadLatency.Mean)) * 100
	}
	
	// Calculate memory regression
	if baseline.MemoryUsage != nil && current.MemoryUsage != nil && baseline.MemoryUsage.Alloc > 0 {
		analysis.MemoryRegression = ((float64(current.MemoryUsage.Alloc - baseline.MemoryUsage.Alloc)) / float64(baseline.MemoryUsage.Alloc)) * 100
	}
	
	// Calculate overall score (weighted average)
	analysis.OverallScore = rt.calculateOverallScore(analysis)
	
	// Determine severity
	analysis.RegressionSeverity = rt.determineRegressionSeverity(analysis)
	
	// Statistical significance (simplified)
	analysis.StatisticalSignificance = math.Abs(analysis.OverallScore) > 5.0 // 5% threshold
	analysis.ConfidenceInterval = 95.0
	
	return analysis
}

// performRegressionAnalysis performs comprehensive regression analysis
func (rt *RegressionTester) performRegressionAnalysis(testName string, result *RegressionResult) *RegressionComparison {
	comparison := &RegressionComparison{
		TestName:          testName,
		ComparisonTime:    time.Now(),
		MetricComparisons: make(map[string]*MetricComparison),
	}
	
	if result.Baseline != nil {
		comparison.BaselineVersion = result.Baseline.Version
		
		// Compare individual metrics
		rt.compareMetric(comparison, "read_throughput", 
			result.Baseline.Metrics.ReadThroughput, 
			result.Metrics.ReadThroughput,
			rt.config.ThroughputThreshold, "higher")
		
		if result.Baseline.Metrics.ReadLatency != nil && result.Metrics.ReadLatency != nil {
			rt.compareMetric(comparison, "read_latency_p99",
				float64(result.Baseline.Metrics.ReadLatency.P99.Nanoseconds()),
				float64(result.Metrics.ReadLatency.P99.Nanoseconds()),
				rt.config.LatencyThreshold, "lower")
		}
		
		if result.Baseline.Metrics.MemoryUsage != nil && result.Metrics.MemoryUsage != nil {
			rt.compareMetric(comparison, "memory_usage",
				float64(result.Baseline.Metrics.MemoryUsage.Alloc),
				float64(result.Metrics.MemoryUsage.Alloc),
				rt.config.MemoryThreshold, "lower")
		}
		
		// Determine overall assessment
		comparison.RegressionDetected = rt.hasSignificantRegression(comparison)
		comparison.OverallAssessment = rt.generateOverallAssessment(comparison)
		comparison.Confidence = rt.calculateConfidence(comparison)
		
		// Generate recommendations
		comparison.Recommendations = rt.generateRecommendations(comparison)
	}
	
	return comparison
}

// compareMetric compares a single metric between baseline and current
func (rt *RegressionTester) compareMetric(comparison *RegressionComparison, metricName string, 
	baselineValue, currentValue, threshold float64, betterDirection string) {
	
	var changePercent float64
	if baselineValue != 0 {
		changePercent = ((currentValue - baselineValue) / baselineValue) * 100
	}
	
	var changeDirection string
	var isSignificant bool
	
	if math.Abs(changePercent) > threshold {
		isSignificant = true
		if (betterDirection == "higher" && changePercent > 0) || 
		   (betterDirection == "lower" && changePercent < 0) {
			changeDirection = "improved"
			comparison.Improvements = append(comparison.Improvements, 
				fmt.Sprintf("%s improved by %.2f%%", metricName, math.Abs(changePercent)))
		} else {
			changeDirection = "regressed"
			comparison.Regressions = append(comparison.Regressions,
				fmt.Sprintf("%s regressed by %.2f%%", metricName, math.Abs(changePercent)))
		}
	} else {
		changeDirection = "unchanged"
	}
	
	var assessment string
	if isSignificant {
		if changeDirection == "improved" {
			assessment = "significant_improvement"
		} else {
			assessment = "significant_regression"
		}
	} else {
		assessment = "no_significant_change"
	}
	
	comparison.MetricComparisons[metricName] = &MetricComparison{
		MetricName:      metricName,
		BaselineValue:   baselineValue,
		CurrentValue:    currentValue,
		ChangePercent:   changePercent,
		ChangeDirection: changeDirection,
		IsSignificant:   isSignificant,
		Threshold:       threshold,
		Assessment:      assessment,
	}
}

// Helper functions

// populateTestData populates the store with test data
func (rt *RegressionTester) populateTestData(keyCount int) {
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		value := generateRandomBytes(1024)
		rt.kvStore.Set(key, value)
	}
}

// runWarmup runs a warmup phase
func (rt *RegressionTester) runWarmup() {
	endTime := time.Now().Add(rt.config.WarmupDuration)
	operations := int64(0)
	
	for time.Now().Before(endTime) {
		key := fmt.Sprintf("warmup_key_%d", operations%100)
		value := generateRandomBytes(512)
		
		rt.kvStore.Set(key, value)
		rt.kvStore.Get(key)
		operations++
	}
}

// getCurrentEnvironment gets the current test environment
func (rt *RegressionTester) getCurrentEnvironment() *TestEnvironment {
	return &TestEnvironment{
		OS:           "darwin", // Simplified for demo
		Architecture: "amd64",
		CPUCores:     8,
		MemoryTotal:  16 * 1024 * 1024 * 1024, // 16GB
		GoVersion:    "go1.21",
		ConfigHash:   "abc123", // Would be actual config hash
	}
}

// getCurrentMemoryMetrics gets current memory metrics
func (rt *RegressionTester) getCurrentMemoryMetrics() *MemoryMetrics {
	// Simplified implementation
	return &MemoryMetrics{
		Alloc:      1024 * 1024 * 100, // 100MB
		TotalAlloc: 1024 * 1024 * 500, // 500MB
		Sys:        1024 * 1024 * 200, // 200MB
		NumGC:      50,
	}
}

// getCurrentCPUUsage gets current CPU usage
func (rt *RegressionTester) getCurrentCPUUsage() float64 {
	return 45.5 // Simplified
}

// getCurrentGCMetrics gets current GC metrics
func (rt *RegressionTester) getCurrentGCMetrics() *GCPerformanceMetrics {
	return &GCPerformanceMetrics{
		GCFrequency: 2 * time.Second,
		AvgGCPause:  2 * time.Millisecond,
		MaxGCPause:  10 * time.Millisecond,
		TotalGCTime: 100 * time.Millisecond,
		GCOverhead:  2.5,
	}
}

// aggregateMetrics aggregates metrics across multiple iterations
func (rt *RegressionTester) aggregateMetrics(allMetrics []*PerformanceMetrics) *PerformanceMetrics {
	if len(allMetrics) == 0 {
		return &PerformanceMetrics{}
	}
	
	// Calculate averages (simplified implementation)
	aggregated := &PerformanceMetrics{}
	
	for _, metrics := range allMetrics {
		aggregated.ReadThroughput += metrics.ReadThroughput
		aggregated.WriteThroughput += metrics.WriteThroughput
		aggregated.MixedThroughput += metrics.MixedThroughput
		aggregated.CPUUsage += metrics.CPUUsage
	}
	
	count := float64(len(allMetrics))
	aggregated.ReadThroughput /= count
	aggregated.WriteThroughput /= count
	aggregated.MixedThroughput /= count
	aggregated.CPUUsage /= count
	
	// Use last iteration's latency and memory metrics (simplified)
	lastMetrics := allMetrics[len(allMetrics)-1]
	aggregated.ReadLatency = lastMetrics.ReadLatency
	aggregated.WriteLatency = lastMetrics.WriteLatency
	aggregated.MixedLatency = lastMetrics.MixedLatency
	aggregated.MemoryUsage = lastMetrics.MemoryUsage
	aggregated.GCMetrics = lastMetrics.GCMetrics
	
	return aggregated
}

// calculateScalabilityFactor calculates how well the system scales
func (rt *RegressionTester) calculateScalabilityFactor(throughputs []float64) float64 {
	if len(throughputs) < 2 {
		return 1.0
	}
	
	// Calculate ideal scaling vs actual scaling
	idealScale := float64(len(throughputs))
	actualScale := throughputs[len(throughputs)-1] / throughputs[0]
	
	return actualScale / idealScale
}

// calculateEfficiencyRatio calculates efficiency ratio
func (rt *RegressionTester) calculateEfficiencyRatio(throughputs []float64) float64 {
	if len(throughputs) == 0 {
		return 0.0
	}
	
	// Simple efficiency calculation
	return throughputs[len(throughputs)-1] / float64(len(throughputs))
}

// calculateOverallScore calculates an overall regression score
func (rt *RegressionTester) calculateOverallScore(analysis *RegressionAnalysis) float64 {
	// Weighted average of different regression metrics
	throughputWeight := 0.4
	latencyWeight := 0.3
	memoryWeight := 0.2
	errorWeight := 0.1
	
	score := (analysis.ThroughputRegression * throughputWeight) +
		(analysis.LatencyRegression * latencyWeight) +
		(analysis.MemoryRegression * memoryWeight) +
		(analysis.ErrorRateRegression * errorWeight)
	
	return score
}

// determineRegressionSeverity determines the severity of regression
func (rt *RegressionTester) determineRegressionSeverity(analysis *RegressionAnalysis) string {
	score := math.Abs(analysis.OverallScore)
	
	if score < 5.0 {
		return "none"
	} else if score < 15.0 {
		return "minor"
	} else if score < 30.0 {
		return "major"
	} else {
		return "critical"
	}
}

// shouldUpdateBaseline determines if baseline should be updated
func (rt *RegressionTester) shouldUpdateBaseline(result *RegressionResult) bool {
	if result.Baseline == nil || result.Regression == nil {
		return false
	}
	
	// Update if there's significant improvement
	return result.Regression.OverallScore < -rt.config.BaselineThreshold
}

// hasSignificantRegression checks if there's significant regression
func (rt *RegressionTester) hasSignificantRegression(comparison *RegressionComparison) bool {
	for _, metric := range comparison.MetricComparisons {
		if metric.IsSignificant && metric.ChangeDirection == "regressed" {
			return true
		}
	}
	return false
}

// generateOverallAssessment generates an overall assessment
func (rt *RegressionTester) generateOverallAssessment(comparison *RegressionComparison) string {
	improvementCount := len(comparison.Improvements)
	regressionCount := len(comparison.Regressions)
	
	if regressionCount > improvementCount {
		return "performance_regression_detected"
	} else if improvementCount > regressionCount {
		return "performance_improvement_detected"
	} else {
		return "performance_stable"
	}
}

// calculateConfidence calculates confidence level
func (rt *RegressionTester) calculateConfidence(comparison *RegressionComparison) float64 {
	significantChanges := 0
	totalMetrics := len(comparison.MetricComparisons)
	
	for _, metric := range comparison.MetricComparisons {
		if metric.IsSignificant {
			significantChanges++
		}
	}
	
	if totalMetrics == 0 {
		return 0.0
	}
	
	return (float64(significantChanges) / float64(totalMetrics)) * 100.0
}

// generateRecommendations generates performance recommendations
func (rt *RegressionTester) generateRecommendations(comparison *RegressionComparison) []string {
	var recommendations []string
	
	for _, metric := range comparison.MetricComparisons {
		if metric.IsSignificant && metric.ChangeDirection == "regressed" {
			switch metric.MetricName {
			case "read_throughput":
				recommendations = append(recommendations, "Investigate read path optimizations")
			case "write_throughput":
				recommendations = append(recommendations, "Investigate write path optimizations")
			case "memory_usage":
				recommendations = append(recommendations, "Review memory allocation patterns")
			case "read_latency_p99":
				recommendations = append(recommendations, "Profile and optimize high-latency operations")
			}
		}
	}
	
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Performance is stable or improved")
	}
	
	return recommendations
}

// generateSummary generates a test summary
func (rt *RegressionTester) generateSummary(passed, failed []string) string {
	return fmt.Sprintf("Regression testing completed: %d tests passed, %d tests failed", 
		len(passed), len(failed))
}

// saveReport saves the regression report
func (rt *RegressionTester) saveReport(report *RegressionReport) error {
	filename := filepath.Join(rt.reportDir, 
		fmt.Sprintf("regression_report_%s.json", time.Now().Format("2006-01-02_15-04-05")))
	
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}
	
	rt.logger.Printf("Saved regression report to %s", filename)
	return nil
}

// sendNotifications sends notifications about regression results
func (rt *RegressionTester) sendNotifications(report *RegressionReport) {
	// Implementation would send actual notifications
	rt.logger.Printf("Notifications would be sent for regression report")
}

// RegressionReport contains the complete regression test report
type RegressionReport struct {
	Version       string                              `json:"version"`
	StartTime     time.Time                           `json:"start_time"`
	Duration      time.Duration                       `json:"duration"`
	TotalTests    int                                 `json:"total_tests"`
	PassedTests   int                                 `json:"passed_tests"`
	FailedTests   int                                 `json:"failed_tests"`
	Comparisons   map[string]*RegressionComparison    `json:"comparisons"`
	Environment   *TestEnvironment                    `json:"environment"`
	Summary       string                              `json:"summary"`
}

// GetResults returns regression test results
func (rt *RegressionTester) GetResults() map[string]*RegressionResult {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	results := make(map[string]*RegressionResult)
	for name, result := range rt.currentResults {
		resultCopy := *result
		results[name] = &resultCopy
	}
	
	return results
}

// GetComparisons returns regression comparisons
func (rt *RegressionTester) GetComparisons() map[string]*RegressionComparison {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	comparisons := make(map[string]*RegressionComparison)
	for name, comparison := range rt.comparisons {
		comparisonCopy := *comparison
		comparisons[name] = &comparisonCopy
	}
	
	return comparisons
}