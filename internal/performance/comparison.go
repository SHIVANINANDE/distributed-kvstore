package performance

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// ComparisonBenchmark provides benchmarking against Redis and etcd
type ComparisonBenchmark struct {
	mu              sync.RWMutex
	logger          *log.Logger
	config          ComparisonConfig
	ourStore        KVStoreInterface
	redisClient     RedisInterface
	etcdClient      EtcdInterface
	results         map[string]*ComparisonResult
	isRunning       bool
	testData        *TestDataSet
}

// ComparisonConfig contains configuration for comparison benchmarks
type ComparisonConfig struct {
	// Test parameters
	TestDuration      time.Duration `json:"test_duration"`      // Duration for each test
	WarmupDuration    time.Duration `json:"warmup_duration"`    // Warmup period
	ConcurrencyLevels []int         `json:"concurrency_levels"` // Concurrency levels to test
	KeyCount          int           `json:"key_count"`          // Number of keys to use
	ValueSizes        []int         `json:"value_sizes"`        // Value sizes to test
	
	// Store configurations
	RedisEndpoint     string `json:"redis_endpoint"`     // Redis connection endpoint
	EtcdEndpoints     []string `json:"etcd_endpoints"`   // etcd cluster endpoints
	
	// Test scenarios
	EnableThroughputTests bool `json:"enable_throughput_tests"` // Enable throughput comparison
	EnableLatencyTests    bool `json:"enable_latency_tests"`    // Enable latency comparison
	EnableScalabilityTests bool `json:"enable_scalability_tests"` // Enable scalability comparison
	EnableConsistencyTests bool `json:"enable_consistency_tests"` // Enable consistency comparison
	
	// Network configuration
	NetworkLatency    time.Duration `json:"network_latency"`    // Simulated network latency
	NetworkBandwidth  int64         `json:"network_bandwidth"`  // Network bandwidth limit (bytes/sec)
	PacketLossRate    float64       `json:"packet_loss_rate"`   // Packet loss rate (0.0-1.0)
}

// RedisInterface defines the interface for Redis operations
type RedisInterface interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
	Ping() error
	Close() error
	GetStats() *RedisStats
}

// EtcdInterface defines the interface for etcd operations
type EtcdInterface interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(key string) error
	Close() error
	GetStats() *EtcdStats
}

// RedisStats contains Redis performance statistics
type RedisStats struct {
	ConnectionsReceived uint64        `json:"connections_received"`
	TotalCommands       uint64        `json:"total_commands"`
	KeyspaceHits        uint64        `json:"keyspace_hits"`
	KeyspaceMisses      uint64        `json:"keyspace_misses"`
	UsedMemory          uint64        `json:"used_memory"`
	AvgLatency          time.Duration `json:"avg_latency"`
}

// EtcdStats contains etcd performance statistics
type EtcdStats struct {
	RaftIndex       uint64        `json:"raft_index"`
	RaftTerm        uint64        `json:"raft_term"`
	LeaderChanges   uint64        `json:"leader_changes"`
	ProposalsFailed uint64        `json:"proposals_failed"`
	AvgLatency      time.Duration `json:"avg_latency"`
	DBSize          int64         `json:"db_size"`
}

// ComparisonResult contains the results of a comparison benchmark
type ComparisonResult struct {
	TestName        string                      `json:"test_name"`
	StartTime       time.Time                   `json:"start_time"`
	Duration        time.Duration               `json:"duration"`
	Stores          map[string]*StoreResult     `json:"stores"`
	Winner          string                      `json:"winner"`
	WinningMetric   string                      `json:"winning_metric"`
	PerformanceGap  float64                     `json:"performance_gap"`
	Recommendations []string                    `json:"recommendations"`
	Details         map[string]interface{}      `json:"details"`
}

// StoreResult contains results for a specific store
type StoreResult struct {
	StoreName       string            `json:"store_name"`
	Throughput      float64           `json:"throughput"`      // Operations per second
	LatencyStats    *LatencyStats     `json:"latency_stats"`
	ErrorRate       float64           `json:"error_rate"`
	MemoryUsage     uint64            `json:"memory_usage"`
	NetworkIO       *NetworkIOStats   `json:"network_io"`
	CPUUsage        float64           `json:"cpu_usage"`
	OperationsCount int64             `json:"operations_count"`
	ErrorCount      int64             `json:"error_count"`
	Features        *StoreFeatures    `json:"features"`
}

// StoreFeatures describes the features of each store
type StoreFeatures struct {
	Persistence         bool     `json:"persistence"`
	Replication         bool     `json:"replication"`
	Clustering          bool     `json:"clustering"`
	AtomicOperations    bool     `json:"atomic_operations"`
	Transactions        bool     `json:"transactions"`
	ConsistencyLevel    string   `json:"consistency_level"`
	DataTypes           []string `json:"data_types"`
	BuiltInDataStructures bool   `json:"built_in_data_structures"`
}

// NewComparisonBenchmark creates a new comparison benchmark
func NewComparisonBenchmark(config ComparisonConfig, ourStore KVStoreInterface, logger *log.Logger) *ComparisonBenchmark {
	if logger == nil {
		logger = log.New(log.Writer(), "[COMPARISON] ", log.LstdFlags)
	}
	
	// Set default values
	if config.TestDuration == 0 {
		config.TestDuration = 60 * time.Second
	}
	if config.WarmupDuration == 0 {
		config.WarmupDuration = 10 * time.Second
	}
	if len(config.ConcurrencyLevels) == 0 {
		config.ConcurrencyLevels = []int{1, 10, 50, 100}
	}
	if config.KeyCount == 0 {
		config.KeyCount = 10000
	}
	if len(config.ValueSizes) == 0 {
		config.ValueSizes = []int{100, 1024, 4096, 16384}
	}
	if config.RedisEndpoint == "" {
		config.RedisEndpoint = "localhost:6379"
	}
	if len(config.EtcdEndpoints) == 0 {
		config.EtcdEndpoints = []string{"localhost:2379"}
	}
	
	return &ComparisonBenchmark{
		logger:   logger,
		config:   config,
		ourStore: ourStore,
		results:  make(map[string]*ComparisonResult),
	}
}

// ConnectToStores establishes connections to Redis and etcd
func (cb *ComparisonBenchmark) ConnectToStores() error {
	cb.logger.Printf("Connecting to external stores...")
	
	// Connect to Redis
	redisClient, err := cb.connectToRedis()
	if err != nil {
		cb.logger.Printf("Failed to connect to Redis: %v", err)
		// Continue without Redis
	} else {
		cb.redisClient = redisClient
		cb.logger.Printf("Connected to Redis at %s", cb.config.RedisEndpoint)
	}
	
	// Connect to etcd
	etcdClient, err := cb.connectToEtcd()
	if err != nil {
		cb.logger.Printf("Failed to connect to etcd: %v", err)
		// Continue without etcd
	} else {
		cb.etcdClient = etcdClient
		cb.logger.Printf("Connected to etcd at %v", cb.config.EtcdEndpoints)
	}
	
	if cb.redisClient == nil && cb.etcdClient == nil {
		return fmt.Errorf("failed to connect to any external stores")
	}
	
	return nil
}

// connectToRedis creates a connection to Redis
func (cb *ComparisonBenchmark) connectToRedis() (RedisInterface, error) {
	// In a real implementation, this would use a Redis client library
	// For demonstration, we'll use a mock implementation
	return &MockRedisClient{
		endpoint: cb.config.RedisEndpoint,
		logger:   cb.logger,
		stats:    &RedisStats{},
	}, nil
}

// connectToEtcd creates a connection to etcd
func (cb *ComparisonBenchmark) connectToEtcd() (EtcdInterface, error) {
	// In a real implementation, this would use the etcd client library
	// For demonstration, we'll use a mock implementation
	return &MockEtcdClient{
		endpoints: cb.config.EtcdEndpoints,
		logger:    cb.logger,
		stats:     &EtcdStats{},
	}, nil
}

// PrepareTestData prepares test data for comparison benchmarks
func (cb *ComparisonBenchmark) PrepareTestData() error {
	cb.logger.Printf("Preparing test data for comparison benchmarks")
	
	testData := &TestDataSet{
		Keys:        make([]string, cb.config.KeyCount),
		Values:      make(map[string][]byte),
		ValueSizes:  cb.config.ValueSizes,
		GeneratedAt: time.Now(),
	}
	
	// Generate test keys and values
	for i := 0; i < cb.config.KeyCount; i++ {
		key := fmt.Sprintf("bench_key_%06d", i)
		testData.Keys[i] = key
		
		// Generate values for each size
		for _, valueSize := range cb.config.ValueSizes {
			valueKey := fmt.Sprintf("%s_%d", key, valueSize)
			value := generateRandomBytes(valueSize)
			testData.Values[valueKey] = value
			testData.TotalSize += int64(len(key) + len(value))
		}
	}
	
	cb.testData = testData
	cb.logger.Printf("Generated test data: %d keys, total size: %d bytes", 
		len(testData.Keys), testData.TotalSize)
	
	return nil
}

// RunAllComparisons runs all comparison benchmarks
func (cb *ComparisonBenchmark) RunAllComparisons(ctx context.Context) (map[string]*ComparisonResult, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if cb.isRunning {
		return nil, fmt.Errorf("comparison benchmark is already running")
	}
	
	cb.logger.Printf("Starting comparison benchmarks")
	cb.isRunning = true
	defer func() { cb.isRunning = false }()
	
	// Prepare test data
	if err := cb.PrepareTestData(); err != nil {
		return nil, fmt.Errorf("failed to prepare test data: %w", err)
	}
	
	// Run different types of comparisons
	if cb.config.EnableThroughputTests {
		cb.runThroughputComparison(ctx)
	}
	
	if cb.config.EnableLatencyTests {
		cb.runLatencyComparison(ctx)
	}
	
	if cb.config.EnableScalabilityTests {
		cb.runScalabilityComparison(ctx)
	}
	
	if cb.config.EnableConsistencyTests {
		cb.runConsistencyComparison(ctx)
	}
	
	// Generate recommendations
	cb.generateRecommendations()
	
	cb.logger.Printf("Comparison benchmarks completed: %d tests", len(cb.results))
	return cb.results, nil
}

// runThroughputComparison compares throughput across stores
func (cb *ComparisonBenchmark) runThroughputComparison(ctx context.Context) {
	testName := "throughput_comparison"
	cb.logger.Printf("Running throughput comparison")
	
	result := &ComparisonResult{
		TestName:  testName,
		StartTime: time.Now(),
		Stores:    make(map[string]*StoreResult),
		Details:   make(map[string]interface{}),
	}
	
	// Test each store
	stores := cb.getAvailableStores()
	
	for storeName, store := range stores {
		cb.logger.Printf("Testing throughput for %s", storeName)
		
		storeResult := cb.runThroughputTest(store, storeName)
		result.Stores[storeName] = storeResult
	}
	
	// Determine winner
	cb.determineWinner(result, "throughput")
	
	result.Duration = time.Since(result.StartTime)
	cb.results[testName] = result
}

// runLatencyComparison compares latency across stores
func (cb *ComparisonBenchmark) runLatencyComparison(ctx context.Context) {
	testName := "latency_comparison"
	cb.logger.Printf("Running latency comparison")
	
	result := &ComparisonResult{
		TestName:  testName,
		StartTime: time.Now(),
		Stores:    make(map[string]*StoreResult),
		Details:   make(map[string]interface{}),
	}
	
	stores := cb.getAvailableStores()
	
	for storeName, store := range stores {
		cb.logger.Printf("Testing latency for %s", storeName)
		
		storeResult := cb.runLatencyTest(store, storeName)
		result.Stores[storeName] = storeResult
	}
	
	cb.determineWinner(result, "latency")
	
	result.Duration = time.Since(result.StartTime)
	cb.results[testName] = result
}

// runScalabilityComparison compares scalability across stores
func (cb *ComparisonBenchmark) runScalabilityComparison(ctx context.Context) {
	testName := "scalability_comparison"
	cb.logger.Printf("Running scalability comparison")
	
	result := &ComparisonResult{
		TestName:  testName,
		StartTime: time.Now(),
		Stores:    make(map[string]*StoreResult),
		Details:   make(map[string]interface{}),
	}
	
	stores := cb.getAvailableStores()
	
	// Test scalability with different data sizes
	dataSizes := []int{1000, 10000, 50000, 100000}
	scalabilityResults := make(map[string]map[string]float64)
	
	for storeName, store := range stores {
		cb.logger.Printf("Testing scalability for %s", storeName)
		
		storeScalability := make(map[string]float64)
		
		for _, dataSize := range dataSizes {
			throughput := cb.runScalabilityTest(store, storeName, dataSize)
			storeScalability[fmt.Sprintf("size_%d", dataSize)] = throughput
		}
		
		scalabilityResults[storeName] = storeScalability
		
		// Create store result with average throughput
		avgThroughput := cb.calculateAverageMapValue(storeScalability)
		storeResult := &StoreResult{
			StoreName:  storeName,
			Throughput: avgThroughput,
		}
		result.Stores[storeName] = storeResult
	}
	
	result.Details["scalability_by_size"] = scalabilityResults
	cb.determineWinner(result, "scalability")
	
	result.Duration = time.Since(result.StartTime)
	cb.results[testName] = result
}

// runConsistencyComparison compares consistency guarantees
func (cb *ComparisonBenchmark) runConsistencyComparison(ctx context.Context) {
	testName := "consistency_comparison"
	cb.logger.Printf("Running consistency comparison")
	
	result := &ComparisonResult{
		TestName:  testName,
		StartTime: time.Now(),
		Stores:    make(map[string]*StoreResult),
		Details:   make(map[string]interface{}),
	}
	
	stores := cb.getAvailableStores()
	consistencyResults := make(map[string]*ConsistencyTestResult)
	
	for storeName, store := range stores {
		cb.logger.Printf("Testing consistency for %s", storeName)
		
		consistencyResult := cb.runConsistencyTest(store, storeName)
		consistencyResults[storeName] = consistencyResult
		
		// Convert to store result
		storeResult := &StoreResult{
			StoreName:       storeName,
			OperationsCount: consistencyResult.TotalOperations,
			ErrorCount:      consistencyResult.InconsistentReads,
		}
		result.Stores[storeName] = storeResult
	}
	
	result.Details["consistency_results"] = consistencyResults
	cb.determineConsistencyWinner(result)
	
	result.Duration = time.Since(result.StartTime)
	cb.results[testName] = result
}

// getAvailableStores returns a map of available stores for testing
func (cb *ComparisonBenchmark) getAvailableStores() map[string]interface{} {
	stores := make(map[string]interface{})
	
	// Always include our store
	stores["our_kvstore"] = cb.ourStore
	
	// Include Redis if available
	if cb.redisClient != nil {
		stores["redis"] = cb.redisClient
	}
	
	// Include etcd if available
	if cb.etcdClient != nil {
		stores["etcd"] = cb.etcdClient
	}
	
	return stores
}

// runThroughputTest runs a throughput test for a specific store
func (cb *ComparisonBenchmark) runThroughputTest(store interface{}, storeName string) *StoreResult {
	var totalOps int64
	var totalErrors int64
	var totalLatency time.Duration
	
	// Test with different concurrency levels
	for _, concurrency := range cb.config.ConcurrencyLevels {
		ops, errors, latency := cb.runConcurrentOperations(store, storeName, concurrency, "read")
		totalOps += ops
		totalErrors += errors
		totalLatency += latency
	}
	
	duration := cb.config.TestDuration * time.Duration(len(cb.config.ConcurrencyLevels))
	throughput := float64(totalOps) / duration.Seconds()
	errorRate := float64(totalErrors) / float64(totalOps+totalErrors)
	
	return &StoreResult{
		StoreName:       storeName,
		Throughput:      throughput,
		ErrorRate:       errorRate,
		OperationsCount: totalOps,
		ErrorCount:      totalErrors,
		LatencyStats: &LatencyStats{
			Mean: totalLatency / time.Duration(totalOps),
		},
		Features: cb.getStoreFeatures(storeName),
	}
}

// runLatencyTest runs a latency test for a specific store
func (cb *ComparisonBenchmark) runLatencyTest(store interface{}, storeName string) *StoreResult {
	latencies := make([]time.Duration, 0, 1000)
	var operations int64
	var errors int64
	
	// Run single-threaded latency test
	endTime := time.Now().Add(cb.config.TestDuration)
	
	for time.Now().Before(endTime) {
		key := cb.testData.Keys[operations%int64(len(cb.testData.Keys))]
		
		start := time.Now()
		err := cb.performGet(store, key)
		latency := time.Since(start)
		
		if err == nil {
			latencies = append(latencies, latency)
			operations++
		} else {
			errors++
		}
	}
	
	latencyStats := calculateLatencyStats(latencies)
	errorRate := float64(errors) / float64(operations+errors)
	
	return &StoreResult{
		StoreName:       storeName,
		LatencyStats:    latencyStats,
		ErrorRate:       errorRate,
		OperationsCount: operations,
		ErrorCount:      errors,
		Features:        cb.getStoreFeatures(storeName),
	}
}

// runScalabilityTest runs a scalability test for a specific data size
func (cb *ComparisonBenchmark) runScalabilityTest(store interface{}, storeName string, dataSize int) float64 {
	// Populate store with data
	cb.populateStoreForScale(store, dataSize)
	
	// Run throughput test
	ops, _, _ := cb.runConcurrentOperations(store, storeName, 50, "read") // Fixed concurrency
	duration := cb.config.TestDuration
	
	return float64(ops) / duration.Seconds()
}

// runConsistencyTest runs a consistency test
func (cb *ComparisonBenchmark) runConsistencyTest(store interface{}, storeName string) *ConsistencyTestResult {
	result := &ConsistencyTestResult{
		StoreName: storeName,
	}
	
	// For this test, we write a value and immediately read it back
	// This tests read-after-write consistency
	testKey := "consistency_test_key"
	testValue := []byte("consistency_test_value")
	
	var totalOps int64
	var inconsistentReads int64
	
	endTime := time.Now().Add(cb.config.TestDuration)
	
	for time.Now().Before(endTime) {
		// Write value
		writeErr := cb.performSet(store, testKey, testValue)
		if writeErr != nil {
			continue
		}
		
		// Immediately read back
		readValue, readErr := cb.performGetBytes(store, testKey)
		if readErr != nil {
			continue
		}
		
		totalOps++
		
		// Check consistency
		if string(readValue) != string(testValue) {
			inconsistentReads++
		}
	}
	
	result.TotalOperations = totalOps
	result.InconsistentReads = inconsistentReads
	result.ConsistencyRate = float64(totalOps-inconsistentReads) / float64(totalOps)
	
	return result
}

// ConsistencyTestResult contains results of consistency testing
type ConsistencyTestResult struct {
	StoreName         string  `json:"store_name"`
	TotalOperations   int64   `json:"total_operations"`
	InconsistentReads int64   `json:"inconsistent_reads"`
	ConsistencyRate   float64 `json:"consistency_rate"`
}

// runConcurrentOperations runs concurrent operations against a store
func (cb *ComparisonBenchmark) runConcurrentOperations(store interface{}, storeName string, concurrency int, opType string) (int64, int64, time.Duration) {
	var wg sync.WaitGroup
	var totalOps int64
	var totalErrors int64
	var totalLatency int64
	
	endTime := time.Now().Add(cb.config.TestDuration)
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			var ops int64
			var errors int64
			var latency int64
			
			for time.Now().Before(endTime) {
				key := cb.testData.Keys[(int64(workerID)*1000+ops)%int64(len(cb.testData.Keys))]
				
				start := time.Now()
				var err error
				
				switch opType {
				case "read":
					err = cb.performGet(store, key)
				case "write":
					value := generateRandomBytes(1024)
					err = cb.performSet(store, key, value)
				}
				
				duration := time.Since(start)
				latency += duration.Nanoseconds()
				
				if err == nil {
					ops++
				} else {
					errors++
				}
			}
			
			atomic.AddInt64(&totalOps, ops)
			atomic.AddInt64(&totalErrors, errors)
			atomic.AddInt64(&totalLatency, latency)
		}(i)
	}
	
	wg.Wait()
	
	avgLatency := time.Duration(totalLatency / totalOps)
	return totalOps, totalErrors, avgLatency
}

// populateStoreForScale populates a store with data for scalability testing
func (cb *ComparisonBenchmark) populateStoreForScale(store interface{}, dataSize int) {
	for i := 0; i < dataSize && i < len(cb.testData.Keys); i++ {
		key := cb.testData.Keys[i]
		value := generateRandomBytes(1024) // 1KB values
		cb.performSet(store, key, value)
	}
}

// performGet performs a get operation on any store type
func (cb *ComparisonBenchmark) performGet(store interface{}, key string) error {
	switch s := store.(type) {
	case KVStoreInterface:
		_, err := s.Get(key)
		return err
	case RedisInterface:
		_, err := s.Get(key)
		return err
	case EtcdInterface:
		_, err := s.Get(key)
		return err
	default:
		return fmt.Errorf("unsupported store type")
	}
}

// performGetBytes performs a get operation and returns bytes
func (cb *ComparisonBenchmark) performGetBytes(store interface{}, key string) ([]byte, error) {
	switch s := store.(type) {
	case KVStoreInterface:
		return s.Get(key)
	case RedisInterface:
		return s.Get(key)
	case EtcdInterface:
		return s.Get(key)
	default:
		return nil, fmt.Errorf("unsupported store type")
	}
}

// performSet performs a set operation on any store type
func (cb *ComparisonBenchmark) performSet(store interface{}, key string, value []byte) error {
	switch s := store.(type) {
	case KVStoreInterface:
		return s.Set(key, value)
	case RedisInterface:
		return s.Set(key, value)
	case EtcdInterface:
		return s.Put(key, value)
	default:
		return fmt.Errorf("unsupported store type")
	}
}

// getStoreFeatures returns feature information for a store
func (cb *ComparisonBenchmark) getStoreFeatures(storeName string) *StoreFeatures {
	switch storeName {
	case "our_kvstore":
		return &StoreFeatures{
			Persistence:           true,
			Replication:           true,
			Clustering:            true,
			AtomicOperations:      true,
			Transactions:          false,
			ConsistencyLevel:      "strong",
			DataTypes:            []string{"bytes"},
			BuiltInDataStructures: false,
		}
	case "redis":
		return &StoreFeatures{
			Persistence:           true,
			Replication:           true,
			Clustering:            true,
			AtomicOperations:      true,
			Transactions:          true,
			ConsistencyLevel:      "eventual",
			DataTypes:            []string{"string", "hash", "list", "set", "zset", "stream"},
			BuiltInDataStructures: true,
		}
	case "etcd":
		return &StoreFeatures{
			Persistence:           true,
			Replication:           true,
			Clustering:            true,
			AtomicOperations:      true,
			Transactions:          true,
			ConsistencyLevel:      "strong",
			DataTypes:            []string{"bytes"},
			BuiltInDataStructures: false,
		}
	default:
		return &StoreFeatures{}
	}
}

// determineWinner determines the winner based on a specific metric
func (cb *ComparisonBenchmark) determineWinner(result *ComparisonResult, metric string) {
	var bestStore string
	var bestValue float64
	var higherIsBetter bool
	
	switch metric {
	case "throughput":
		higherIsBetter = true
		for storeName, storeResult := range result.Stores {
			if storeResult.Throughput > bestValue {
				bestValue = storeResult.Throughput
				bestStore = storeName
			}
		}
	case "latency":
		higherIsBetter = false
		bestValue = float64(time.Hour) // Start with very high value
		for storeName, storeResult := range result.Stores {
			if storeResult.LatencyStats != nil {
				latencyValue := float64(storeResult.LatencyStats.Mean.Nanoseconds())
				if latencyValue < bestValue {
					bestValue = latencyValue
					bestStore = storeName
				}
			}
		}
	case "scalability":
		higherIsBetter = true
		for storeName, storeResult := range result.Stores {
			if storeResult.Throughput > bestValue {
				bestValue = storeResult.Throughput
				bestStore = storeName
			}
		}
	}
	
	result.Winner = bestStore
	result.WinningMetric = metric
	
	// Calculate performance gap
	if len(result.Stores) > 1 {
		cb.calculatePerformanceGap(result, metric, bestValue, higherIsBetter)
	}
}

// determineConsistencyWinner determines winner for consistency tests
func (cb *ComparisonBenchmark) determineConsistencyWinner(result *ComparisonResult) {
	var bestStore string
	var bestConsistency float64
	
	consistencyResults := result.Details["consistency_results"].(map[string]*ConsistencyTestResult)
	
	for storeName, consistencyResult := range consistencyResults {
		if consistencyResult.ConsistencyRate > bestConsistency {
			bestConsistency = consistencyResult.ConsistencyRate
			bestStore = storeName
		}
	}
	
	result.Winner = bestStore
	result.WinningMetric = "consistency"
	result.PerformanceGap = bestConsistency
}

// calculatePerformanceGap calculates the performance gap between winner and others
func (cb *ComparisonBenchmark) calculatePerformanceGap(result *ComparisonResult, metric string, bestValue float64, higherIsBetter bool) {
	var secondBest float64
	
	for storeName, storeResult := range result.Stores {
		if storeName == result.Winner {
			continue
		}
		
		var value float64
		switch metric {
		case "throughput", "scalability":
			value = storeResult.Throughput
		case "latency":
			if storeResult.LatencyStats != nil {
				value = float64(storeResult.LatencyStats.Mean.Nanoseconds())
			}
		}
		
		if higherIsBetter {
			if value > secondBest {
				secondBest = value
			}
		} else {
			if secondBest == 0 || value < secondBest {
				secondBest = value
			}
		}
	}
	
	if secondBest > 0 {
		if higherIsBetter {
			result.PerformanceGap = ((bestValue - secondBest) / secondBest) * 100
		} else {
			result.PerformanceGap = ((secondBest - bestValue) / bestValue) * 100
		}
	}
}

// generateRecommendations generates performance recommendations
func (cb *ComparisonBenchmark) generateRecommendations() {
	for _, result := range cb.results {
		recommendations := make([]string, 0)
		
		// Analyze results and generate recommendations
		ourStoreResult := result.Stores["our_kvstore"]
		if ourStoreResult == nil {
			continue
		}
		
		// Throughput recommendations
		if result.WinningMetric == "throughput" && result.Winner != "our_kvstore" {
			recommendations = append(recommendations, 
				fmt.Sprintf("Consider optimizing throughput - %s achieved %.2f%% better performance", 
					result.Winner, result.PerformanceGap))
		}
		
		// Latency recommendations
		if result.WinningMetric == "latency" && result.Winner != "our_kvstore" {
			recommendations = append(recommendations, 
				fmt.Sprintf("Consider optimizing latency - %s achieved %.2f%% better latency", 
					result.Winner, result.PerformanceGap))
		}
		
		// Error rate recommendations
		if ourStoreResult.ErrorRate > 0.01 { // More than 1% error rate
			recommendations = append(recommendations, 
				fmt.Sprintf("High error rate detected: %.2f%% - investigate error handling", 
					ourStoreResult.ErrorRate*100))
		}
		
		result.Recommendations = recommendations
	}
}

// calculateAverageMapValue calculates the average value from a map of float64
func (cb *ComparisonBenchmark) calculateAverageMapValue(values map[string]float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	total := 0.0
	for _, value := range values {
		total += value
	}
	
	return total / float64(len(values))
}

// GetResults returns all comparison results
func (cb *ComparisonBenchmark) GetResults() map[string]*ComparisonResult {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	results := make(map[string]*ComparisonResult)
	for name, result := range cb.results {
		resultCopy := *result
		results[name] = &resultCopy
	}
	
	return results
}

// Close closes connections to external stores
func (cb *ComparisonBenchmark) Close() error {
	if cb.redisClient != nil {
		cb.redisClient.Close()
	}
	
	if cb.etcdClient != nil {
		cb.etcdClient.Close()
	}
	
	return nil
}

// Mock implementations for demonstration

// MockRedisClient provides a mock Redis client for testing
type MockRedisClient struct {
	endpoint string
	logger   *log.Logger
	stats    *RedisStats
	data     sync.Map
}

func (m *MockRedisClient) Get(key string) ([]byte, error) {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&m.stats.TotalCommands, 1)
		// Simulate Redis latency
		time.Sleep(100 * time.Microsecond)
	}()
	
	if value, ok := m.data.Load(key); ok {
		atomic.AddUint64(&m.stats.KeyspaceHits, 1)
		return value.([]byte), nil
	}
	
	atomic.AddUint64(&m.stats.KeyspaceMisses, 1)
	return nil, fmt.Errorf("key not found")
}

func (m *MockRedisClient) Set(key string, value []byte) error {
	atomic.AddUint64(&m.stats.TotalCommands, 1)
	m.data.Store(key, value)
	// Simulate Redis latency
	time.Sleep(50 * time.Microsecond)
	return nil
}

func (m *MockRedisClient) Delete(key string) error {
	atomic.AddUint64(&m.stats.TotalCommands, 1)
	m.data.Delete(key)
	return nil
}

func (m *MockRedisClient) Ping() error {
	return nil
}

func (m *MockRedisClient) Close() error {
	return nil
}

func (m *MockRedisClient) GetStats() *RedisStats {
	return m.stats
}

// MockEtcdClient provides a mock etcd client for testing
type MockEtcdClient struct {
	endpoints []string
	logger    *log.Logger
	stats     *EtcdStats
	data      sync.Map
}

func (m *MockEtcdClient) Get(key string) ([]byte, error) {
	if value, ok := m.data.Load(key); ok {
		// Simulate etcd latency (typically higher than Redis)
		time.Sleep(500 * time.Microsecond)
		return value.([]byte), nil
	}
	return nil, fmt.Errorf("key not found")
}

func (m *MockEtcdClient) Put(key string, value []byte) error {
	m.data.Store(key, value)
	atomic.AddUint64(&m.stats.RaftIndex, 1)
	// Simulate etcd write latency
	time.Sleep(1 * time.Millisecond)
	return nil
}

func (m *MockEtcdClient) Delete(key string) error {
	m.data.Delete(key)
	atomic.AddUint64(&m.stats.RaftIndex, 1)
	return nil
}

func (m *MockEtcdClient) Close() error {
	return nil
}

func (m *MockEtcdClient) GetStats() *EtcdStats {
	return m.stats
}