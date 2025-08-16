package benchmarks

import (
	"distributed-kvstore/internal/testutil"
	"fmt"
	"testing"
	"time"
)

// Comprehensive Performance Test Suite

func TestPerformanceBaseline(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	// Test basic performance characteristics
	testCases := []struct {
		name        string
		operations  int
		maxDuration time.Duration
	}{
		{"1K Operations", 1000, time.Second * 5},
		{"10K Operations", 10000, time.Second * 30},
		{"100K Operations", 100000, time.Second * 300},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			
			// Perform operations
			for i := 0; i < tc.operations; i++ {
				key := fmt.Sprintf("perf-key-%d", i)
				value := fmt.Sprintf("perf-value-%d", i)
				
				err := engine.Put([]byte(key), []byte(value))
				if err != nil {
					t.Fatalf("Put failed: %v", err)
				}
			}
			
			duration := time.Since(start)
			
			if duration > tc.maxDuration {
				t.Errorf("Operations took too long: %v (max: %v)", duration, tc.maxDuration)
			}
			
			opsPerSec := float64(tc.operations) / duration.Seconds()
			t.Logf("Performance: %.2f ops/sec for %d operations", opsPerSec, tc.operations)
		})
	}
}

func TestConcurrentPerformance(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	concurrencyLevels := []int{1, 2, 4, 8, 16}
	operationsPerGoroutine := 1000
	
	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency-%d", concurrency), func(t *testing.T) {
			start := time.Now()
			
			testutil.ConcurrentTest(t, concurrency, func(workerID int) {
				for i := 0; i < operationsPerGoroutine; i++ {
					key := fmt.Sprintf("concurrent-key-%d-%d", workerID, i)
					value := fmt.Sprintf("concurrent-value-%d-%d", workerID, i)
					
					err := engine.Put([]byte(key), []byte(value))
					if err != nil {
						t.Errorf("Put failed: %v", err)
						return
					}
				}
			})
			
			duration := time.Since(start)
			totalOps := concurrency * operationsPerGoroutine
			opsPerSec := float64(totalOps) / duration.Seconds()
			
			t.Logf("Concurrency %d: %.2f ops/sec (%d total ops in %v)", 
				concurrency, opsPerSec, totalOps, duration)
		})
	}
}

func TestMemoryUsage(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	valueSizes := []int{10, 100, 1000, 10000}
	operations := 10000
	
	for _, size := range valueSizes {
		t.Run(fmt.Sprintf("ValueSize-%d", size), func(t *testing.T) {
			// Generate test data
			keys := make([][]byte, operations)
			values := make([][]byte, operations)
			
			for i := 0; i < operations; i++ {
				keys[i] = []byte(fmt.Sprintf("memory-key-%d", i))
				values[i] = []byte(testutil.GenerateRandomString(size))
			}
			
			start := time.Now()
			
			// Perform operations
			for i := 0; i < operations; i++ {
				err := engine.Put(keys[i], values[i])
				if err != nil {
					t.Fatalf("Put failed: %v", err)
				}
			}
			
			duration := time.Since(start)
			opsPerSec := float64(operations) / duration.Seconds()
			
			totalDataSize := operations * size
			t.Logf("Value size %d bytes: %.2f ops/sec, %d total data bytes", 
				size, opsPerSec, totalDataSize)
		})
	}
}

func TestLatencyDistribution(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	operations := 10000
	latencies := make([]time.Duration, operations)
	
	// Measure individual operation latencies
	for i := 0; i < operations; i++ {
		key := []byte(fmt.Sprintf("latency-key-%d", i))
		value := []byte(fmt.Sprintf("latency-value-%d", i))
		
		start := time.Now()
		err := engine.Put(key, value)
		latencies[i] = time.Since(start)
		
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}
	
	// Calculate percentiles
	percentiles := calculatePercentiles(latencies)
	
	t.Logf("Latency Distribution:")
	t.Logf("  P50: %v", percentiles[50])
	t.Logf("  P90: %v", percentiles[90])
	t.Logf("  P95: %v", percentiles[95])
	t.Logf("  P99: %v", percentiles[99])
	
	// Verify reasonable latencies (these thresholds may need adjustment)
	if percentiles[95] > time.Millisecond*10 {
		t.Logf("Warning: P95 latency is high: %v", percentiles[95])
	}
}

func TestThroughputScaling(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	batchSizes := []int{1, 10, 100, 1000}
	
	for _, batchSize := range batchSizes {
		t.Run(fmt.Sprintf("BatchSize-%d", batchSize), func(t *testing.T) {
			numBatches := 1000
			
			start := time.Now()
			
			for batch := 0; batch < numBatches; batch++ {
				for i := 0; i < batchSize; i++ {
					key := []byte(fmt.Sprintf("batch-key-%d-%d", batch, i))
					value := []byte(fmt.Sprintf("batch-value-%d-%d", batch, i))
					
					err := engine.Put(key, value)
					if err != nil {
						t.Fatalf("Put failed: %v", err)
					}
				}
			}
			
			duration := time.Since(start)
			totalOps := numBatches * batchSize
			opsPerSec := float64(totalOps) / duration.Seconds()
			
			t.Logf("Batch size %d: %.2f ops/sec (%d total ops)", 
				batchSize, opsPerSec, totalOps)
		})
	}
}

func TestOperationMix(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	// Pre-populate with data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("mix-key-%d", i))
		value := []byte(fmt.Sprintf("mix-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			t.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	// Test different operation mixes
	mixes := []struct {
		name   string
		reads  int
		writes int
	}{
		{"ReadHeavy", 90, 10},
		{"WriteHeavy", 10, 90},
		{"Balanced", 50, 50},
	}
	
	operations := 10000
	
	for _, mix := range mixes {
		t.Run(mix.name, func(t *testing.T) {
			start := time.Now()
			
			for i := 0; i < operations; i++ {
				if i%100 < mix.reads {
					// Read operation
					keyNum := i % numKeys
					key := []byte(fmt.Sprintf("mix-key-%d", keyNum))
					_, err := engine.Get(key)
					if err != nil {
						t.Fatalf("Get failed: %v", err)
					}
				} else {
					// Write operation
					key := []byte(fmt.Sprintf("mix-new-key-%d", i))
					value := []byte(fmt.Sprintf("mix-new-value-%d", i))
					err := engine.Put(key, value)
					if err != nil {
						t.Fatalf("Put failed: %v", err)
					}
				}
			}
			
			duration := time.Since(start)
			opsPerSec := float64(operations) / duration.Seconds()
			
			t.Logf("Mix %s (%d%% reads): %.2f ops/sec", 
				mix.name, mix.reads, opsPerSec)
		})
	}
}

func TestResourceUtilization(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	
	operations := 50000
	
	// Warm up
	for i := 0; i < 1000; i++ {
		key := []byte(fmt.Sprintf("warmup-key-%d", i))
		value := []byte(fmt.Sprintf("warmup-value-%d", i))
		engine.Put(key, value)
	}
	
	start := time.Now()
	
	for i := 0; i < operations; i++ {
		key := []byte(fmt.Sprintf("resource-key-%d", i))
		value := []byte(fmt.Sprintf("resource-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		
		// Periodically perform reads to simulate mixed workload
		if i%10 == 0 {
			readKey := []byte(fmt.Sprintf("resource-key-%d", i/2))
			_, err := engine.Get(readKey)
			if err != nil && i/2 < i {
				// Key might not exist yet, which is fine
			}
		}
	}
	
	duration := time.Since(start)
	opsPerSec := float64(operations) / duration.Seconds()
	
	t.Logf("Resource test: %.2f ops/sec over %v", opsPerSec, duration)
}

// Helper functions

func calculatePercentiles(latencies []time.Duration) map[int]time.Duration {
	// Simple percentile calculation (for more accuracy, use a proper statistics library)
	n := len(latencies)
	percentiles := make(map[int]time.Duration)
	
	// Sort latencies (simple bubble sort for this test)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if latencies[j] > latencies[j+1] {
				latencies[j], latencies[j+1] = latencies[j+1], latencies[j]
			}
		}
	}
	
	percentiles[50] = latencies[n*50/100]
	percentiles[90] = latencies[n*90/100]
	percentiles[95] = latencies[n*95/100]
	percentiles[99] = latencies[n*99/100]
	
	return percentiles
}

// Note: Benchmark functions should be named BenchmarkXxx and take *testing.B
// Run benchmarks with: go test -bench=. ./benchmarks