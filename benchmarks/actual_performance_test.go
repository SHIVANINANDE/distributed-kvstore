package benchmarks

import (
	"distributed-kvstore/internal/testutil"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkResults stores actual performance metrics
type BenchmarkResults struct {
	OperationsPerSecond float64
	LatencyP50          time.Duration
	LatencyP95          time.Duration
	LatencyP99          time.Duration
	TotalOperations     int
	Duration            time.Duration
}

func BenchmarkRealPerformance(b *testing.B) {
	// Create a testing.T for testutil compatibility
	t := &testing.T{}
	engine := testutil.TestStorageEngine(t)
	defer engine.Close()

	b.Run("Sequential_PUT_1KB", func(b *testing.B) {
		value := make([]byte, 1024) // 1KB value
		for i := range value {
			value[i] = byte(i % 256)
		}

		b.ResetTimer()
		start := time.Now()

		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("bench-key-%d", i)
			err := engine.Put([]byte(key), value)
			if err != nil {
				b.Fatal(err)
			}
		}

		duration := time.Since(start)
		opsPerSec := float64(b.N) / duration.Seconds()

		b.ReportMetric(opsPerSec, "ops/sec")
		b.ReportMetric(float64(duration.Nanoseconds()/int64(b.N))/1e6, "ms/op")

		b.Logf("PUT Performance: %.0f ops/sec, %.2f ms/op", opsPerSec, float64(duration.Nanoseconds()/int64(b.N))/1e6)
	})

	b.Run("Sequential_GET_1KB", func(b *testing.B) {
		// Pre-populate data
		value := make([]byte, 1024)
		for i := 0; i < 10000; i++ {
			key := fmt.Sprintf("get-bench-key-%d", i)
			engine.Put([]byte(key), value)
		}

		b.ResetTimer()
		start := time.Now()

		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("get-bench-key-%d", i%10000)
			_, err := engine.Get([]byte(key))
			if err != nil {
				b.Fatal(err)
			}
		}

		duration := time.Since(start)
		opsPerSec := float64(b.N) / duration.Seconds()

		b.ReportMetric(opsPerSec, "ops/sec")
		b.ReportMetric(float64(duration.Nanoseconds()/int64(b.N))/1e6, "ms/op")

		b.Logf("GET Performance: %.0f ops/sec, %.2f ms/op", opsPerSec, float64(duration.Nanoseconds()/int64(b.N))/1e6)
	})

	b.Run("Concurrent_Mixed_Workload", func(b *testing.B) {
		const numWorkers = 10
		const readRatio = 0.8 // 80% reads, 20% writes

		// Pre-populate data
		value := make([]byte, 1024)
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("mixed-key-%d", i)
			engine.Put([]byte(key), value)
		}

		var totalOps int64
		var wg sync.WaitGroup

		b.ResetTimer()
		start := time.Now()

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				opsPerWorker := b.N / numWorkers
				for i := 0; i < opsPerWorker; i++ {
					if float64(i%100)/100.0 < readRatio {
						// Read operation
						key := fmt.Sprintf("mixed-key-%d", i%1000)
						_, err := engine.Get([]byte(key))
						if err != nil {
							b.Error(err)
							return
						}
					} else {
						// Write operation
						key := fmt.Sprintf("mixed-key-%d", i%1000)
						err := engine.Put([]byte(key), value)
						if err != nil {
							b.Error(err)
							return
						}
					}
					atomic.AddInt64(&totalOps, 1)
				}
			}(w)
		}

		wg.Wait()
		duration := time.Since(start)
		opsPerSec := float64(totalOps) / duration.Seconds()

		b.ReportMetric(opsPerSec, "ops/sec")
		b.ReportMetric(float64(duration.Nanoseconds()/totalOps)/1e6, "ms/op")

		b.Logf("Mixed Workload (80%% read): %.0f ops/sec with %d workers", opsPerSec, numWorkers)
	})
}

// TestRealLatencyMeasurement measures actual latency percentiles
func TestRealLatencyMeasurement(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	defer engine.Close()

	const numOps = 10000
	latencies := make([]time.Duration, numOps)
	value := make([]byte, 1024)

	t.Log("Measuring PUT latencies...")

	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("latency-key-%d", i)

		start := time.Now()
		err := engine.Put([]byte(key), value)
		latencies[i] = time.Since(start)

		if err != nil {
			t.Fatal(err)
		}
	}

	// Calculate percentiles
	results := calculateLatencyPercentiles(latencies)

	t.Logf("PUT Latency Results (%d operations):", numOps)
	t.Logf("  P50: %v", results.LatencyP50)
	t.Logf("  P95: %v", results.LatencyP95)
	t.Logf("  P99: %v", results.LatencyP99)
	t.Logf("  Ops/sec: %.0f", results.OperationsPerSecond)

	// Test GET latencies
	getLatencies := make([]time.Duration, numOps)

	t.Log("Measuring GET latencies...")

	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("latency-key-%d", i%1000) // Reuse keys for cache effects

		start := time.Now()
		_, err := engine.Get([]byte(key))
		getLatencies[i] = time.Since(start)

		if err != nil {
			t.Fatal(err)
		}
	}

	getResults := calculateLatencyPercentiles(getLatencies)

	t.Logf("GET Latency Results (%d operations):", numOps)
	t.Logf("  P50: %v", getResults.LatencyP50)
	t.Logf("  P95: %v", getResults.LatencyP95)
	t.Logf("  P99: %v", getResults.LatencyP99)
	t.Logf("  Ops/sec: %.0f", getResults.OperationsPerSecond)
}

// TestRealThroughputScaling tests performance under different load conditions
func TestRealThroughputScaling(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	defer engine.Close()

	workerCounts := []int{1, 2, 4, 8, 16}
	const opsPerWorker = 1000
	value := make([]byte, 1024)

	t.Log("Testing throughput scaling with concurrent workers:")

	for _, workers := range workerCounts {
		var wg sync.WaitGroup
		var totalOps int64

		start := time.Now()

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for i := 0; i < opsPerWorker; i++ {
					key := fmt.Sprintf("scale-key-%d-%d", workerID, i)
					err := engine.Put([]byte(key), value)
					if err != nil {
						t.Error(err)
						return
					}
					atomic.AddInt64(&totalOps, 1)
				}
			}(w)
		}

		wg.Wait()
		duration := time.Since(start)
		opsPerSec := float64(totalOps) / duration.Seconds()

		t.Logf("  %d workers: %.0f ops/sec (%.2f ms/op avg)",
			workers, opsPerSec, float64(duration.Nanoseconds()/totalOps)/1e6)
	}
}

// TestRealMemoryUsage measures memory consumption during operations
func TestRealMemoryUsage(t *testing.T) {
	engine := testutil.TestStorageEngine(t)
	defer engine.Close()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	const numOps = 50000
	value := make([]byte, 1024)

	start := time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("memory-key-%d", i)
		err := engine.Put([]byte(key), value)
		if err != nil {
			t.Fatal(err)
		}
	}
	duration := time.Since(start)

	runtime.GC()
	runtime.ReadMemStats(&m2)

	opsPerSec := float64(numOps) / duration.Seconds()
	memUsed := m2.Alloc - m1.Alloc
	memPerOp := float64(memUsed) / float64(numOps)

	t.Logf("Memory Usage Test (%d operations):", numOps)
	t.Logf("  Performance: %.0f ops/sec", opsPerSec)
	t.Logf("  Memory used: %d bytes (%.2f KB per op)", memUsed, memPerOp/1024)
	t.Logf("  Total heap: %d bytes", m2.Alloc)
}

func calculateLatencyPercentiles(latencies []time.Duration) BenchmarkResults {
	if len(latencies) == 0 {
		return BenchmarkResults{}
	}

	// Simple sort for percentile calculation
	for i := 0; i < len(latencies); i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	totalDuration := time.Duration(0)
	for _, lat := range latencies {
		totalDuration += lat
	}

	return BenchmarkResults{
		OperationsPerSecond: float64(len(latencies)) / totalDuration.Seconds(),
		LatencyP50:          latencies[len(latencies)*50/100],
		LatencyP95:          latencies[len(latencies)*95/100],
		LatencyP99:          latencies[len(latencies)*99/100],
		TotalOperations:     len(latencies),
		Duration:            totalDuration,
	}
}
