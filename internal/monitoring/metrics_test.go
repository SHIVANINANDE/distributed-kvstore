package monitoring

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestMetricsRegistry_NewCounter(t *testing.T) {
	registry := NewMetricsRegistry()

	counter := registry.NewCounter("test_counter", "Test counter", map[string]string{"type": "test"})
	if counter == nil {
		t.Fatal("Expected counter to be created")
	}

	if counter.name != "test_counter" {
		t.Errorf("Expected name 'test_counter', got '%s'", counter.name)
	}

	if counter.help != "Test counter" {
		t.Errorf("Expected help 'Test counter', got '%s'", counter.help)
	}
}

func TestCounter_Operations(t *testing.T) {
	registry := NewMetricsRegistry()
	counter := registry.NewCounter("test_counter", "Test counter", nil)

	// Test increment
	counter.Inc()
	if counter.Get() != 1 {
		t.Errorf("Expected counter value 1, got %f", counter.Get())
	}

	// Test add
	counter.Add(5)
	if counter.Get() != 6 {
		t.Errorf("Expected counter value 6, got %f", counter.Get())
	}

	// Test initial value is 0
	counter2 := registry.NewCounter("test_counter_2", "Test counter 2", nil)
	if counter2.Get() != 0 {
		t.Errorf("Expected initial counter value 0, got %f", counter2.Get())
	}
}

func TestCounter_Concurrency(t *testing.T) {
	registry := NewMetricsRegistry()
	counter := registry.NewCounter("concurrent_counter", "Concurrent counter", nil)
	
	goroutines := 10
	incrementsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				counter.Inc()
			}
		}()
	}

	wg.Wait()

	expected := float64(goroutines * incrementsPerGoroutine)
	if counter.Get() != expected {
		t.Errorf("Expected counter value %f, got %f", expected, counter.Get())
	}
}

func TestMetricsRegistry_NewGauge(t *testing.T) {
	registry := NewMetricsRegistry()

	gauge := registry.NewGauge("test_gauge", "Test gauge", map[string]string{"unit": "bytes"})
	if gauge == nil {
		t.Fatal("Expected gauge to be created")
	}

	if gauge.name != "test_gauge" {
		t.Errorf("Expected name 'test_gauge', got '%s'", gauge.name)
	}
}

func TestGauge_Operations(t *testing.T) {
	registry := NewMetricsRegistry()
	gauge := registry.NewGauge("test_gauge", "Test gauge", nil)

	// Test set value (using integer values since gauge uses int64 internally)
	gauge.Set(42)
	if gauge.Get() != 42 {
		t.Errorf("Expected gauge value 42, got %f", gauge.Get())
	}

	// Test add
	gauge.Add(8)
	if gauge.Get() != 50.0 {
		t.Errorf("Expected gauge value 50.0, got %f", gauge.Get())
	}

	// Test subtract
	gauge.Sub(10)
	if gauge.Get() != 40.0 {
		t.Errorf("Expected gauge value 40.0, got %f", gauge.Get())
	}

	// Test increment
	gauge.Inc()
	if gauge.Get() != 41.0 {
		t.Errorf("Expected gauge value 41.0, got %f", gauge.Get())
	}

	// Test decrement
	gauge.Dec()
	if gauge.Get() != 40.0 {
		t.Errorf("Expected gauge value 40.0, got %f", gauge.Get())
	}
}

func TestMetricsRegistry_NewHistogram(t *testing.T) {
	registry := NewMetricsRegistry()
	buckets := []float64{0.1, 0.5, 1.0, 2.5, 5.0}

	histogram := registry.NewHistogram("test_histogram", "Test histogram", buckets, nil)
	if histogram == nil {
		t.Fatal("Expected histogram to be created")
	}

	if histogram.name != "test_histogram" {
		t.Errorf("Expected name 'test_histogram', got '%s'", histogram.name)
	}

	if len(histogram.buckets) != len(buckets) {
		t.Errorf("Expected %d buckets, got %d", len(buckets), len(histogram.buckets))
	}
}

func TestHistogram_Observe(t *testing.T) {
	registry := NewMetricsRegistry()
	buckets := []float64{0.1, 0.5, 1.0, 2.5, 5.0}
	histogram := registry.NewHistogram("test_histogram", "Test histogram", buckets, nil)

	// Test observations
	values := []float64{0.05, 0.3, 0.8, 1.5, 3.0, 7.0}
	for _, value := range values {
		histogram.Observe(value)
	}

	// Get histogram data
	data := histogram.Get()
	
	// Check count
	if count, ok := data["count"].(int64); !ok || count != int64(len(values)) {
		t.Errorf("Expected count %d, got %v", len(values), data["count"])
	}

	// Check sum (values are stored in milliseconds internally, but Get() returns seconds)
	expectedSum := 0.05 + 0.3 + 0.8 + 1.5 + 3.0 + 7.0
	if sum, ok := data["sum"].(float64); !ok || math.Abs(sum-expectedSum) > 0.001 {
		t.Errorf("Expected sum %f, got %v", expectedSum, data["sum"])
	}

	// Check buckets exist
	if buckets, ok := data["buckets"].(map[string]int64); !ok {
		t.Error("Expected buckets in histogram data")
	} else {
		// Check that we have the right number of buckets
		expectedBucketCount := len(histogram.buckets) + 1 // +1 for +Inf
		if len(buckets) != expectedBucketCount {
			t.Errorf("Expected %d bucket entries, got %d", expectedBucketCount, len(buckets))
		}
	}
}

func TestMetricsRegistry_NewSummary(t *testing.T) {
	registry := NewMetricsRegistry()
	quantiles := []float64{0.5, 0.9, 0.99}

	summary := registry.NewSummary("test_summary", "Test summary", quantiles, nil)
	if summary == nil {
		t.Fatal("Expected summary to be created")
	}

	if summary.name != "test_summary" {
		t.Errorf("Expected name 'test_summary', got '%s'", summary.name)
	}

	if len(summary.quantiles) != len(quantiles) {
		t.Errorf("Expected %d quantiles, got %d", len(quantiles), len(summary.quantiles))
	}
}

func TestSummary_Observe(t *testing.T) {
	registry := NewMetricsRegistry()
	quantiles := []float64{0.5, 0.9, 0.99}
	summary := registry.NewSummary("test_summary", "Test summary", quantiles, nil)

	// Observe some values
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
	for _, value := range values {
		summary.Observe(value)
	}

	// Get summary data
	data := summary.Get()

	// Check count
	if count, ok := data["count"].(int64); !ok || count != int64(len(values)) {
		t.Errorf("Expected count %d, got %v", len(values), data["count"])
	}

	// Check sum
	expectedSum := 55.0 // sum of 1-10
	if sum, ok := data["sum"].(float64); !ok || sum != expectedSum {
		t.Errorf("Expected sum %f, got %v", expectedSum, data["sum"])
	}

	// Check quantiles exist
	if quantiles, ok := data["quantiles"].(map[string]float64); !ok {
		t.Error("Expected quantiles in summary data")
	} else {
		if len(quantiles) != len(summary.quantiles) {
			t.Errorf("Expected %d quantiles, got %d", len(summary.quantiles), len(quantiles))
		}
	}
}

func TestMetricsRegistry_GetAllMetrics(t *testing.T) {
	registry := NewMetricsRegistry()

	// Create various metrics
	counter := registry.NewCounter("test_counter", "Test counter", nil)
	gauge := registry.NewGauge("test_gauge", "Test gauge", nil)
	histogram := registry.NewHistogram("test_histogram", "Test histogram", nil, nil)
	summary := registry.NewSummary("test_summary", "Test summary", nil, nil)

	// Set some values
	counter.Inc()
	gauge.Set(42)
	histogram.Observe(0.5)
	summary.Observe(1.0)

	// Get all metrics
	allMetrics := registry.GetAllMetrics()

	expectedNames := []string{"test_counter", "test_gauge", "test_histogram", "test_summary"}
	if len(allMetrics) != len(expectedNames) {
		t.Errorf("Expected %d metrics, got %d", len(expectedNames), len(allMetrics))
	}

	for _, name := range expectedNames {
		if metric, exists := allMetrics[name]; !exists {
			t.Errorf("Expected metric %s to exist", name)
		} else {
			if metric.Name != name {
				t.Errorf("Expected metric name %s, got %s", name, metric.Name)
			}
		}
	}
}

func TestKVStoreMetrics_Creation(t *testing.T) {
	metrics := NewKVStoreMetrics()

	if metrics == nil {
		t.Fatal("Expected KVStoreMetrics to be created")
	}

	if metrics.registry == nil {
		t.Fatal("Expected registry to be initialized")
	}

	// Check that all metrics are created
	if metrics.RequestsTotal == nil {
		t.Error("RequestsTotal should not be nil")
	}
	if metrics.RequestDuration == nil {
		t.Error("RequestDuration should not be nil")
	}
	if metrics.MemoryUsage == nil {
		t.Error("MemoryUsage should not be nil")
	}
	if metrics.GoroutineCount == nil {
		t.Error("GoroutineCount should not be nil")
	}
	if metrics.StorageSize == nil {
		t.Error("StorageSize should not be nil")
	}
	if metrics.HTTPRequests == nil {
		t.Error("HTTPRequests should not be nil")
	}
}

func TestKVStoreMetrics_UpdateSystemMetrics(t *testing.T) {
	metrics := NewKVStoreMetrics()

	// Get initial values
	initialMemory := metrics.MemoryUsage.Get()
	initialGoroutines := metrics.GoroutineCount.Get()

	// Update system metrics
	metrics.UpdateSystemMetrics()

	// Check that values were updated (should be > 0)
	if metrics.MemoryUsage.Get() <= 0 {
		t.Error("Expected memory usage to be updated to > 0")
	}

	if metrics.GoroutineCount.Get() <= 0 {
		t.Error("Expected goroutine count to be updated to > 0")
	}

	// Values should have changed (or at least been set if they were 0)
	if initialMemory == 0 && metrics.MemoryUsage.Get() == 0 {
		t.Error("Memory usage should be updated from 0")
	}

	if initialGoroutines == 0 && metrics.GoroutineCount.Get() == 0 {
		t.Error("Goroutine count should be updated from 0")
	}
}

func TestKVStoreMetrics_RequestMetrics(t *testing.T) {
	metrics := NewKVStoreMetrics()

	// Test request metrics
	metrics.RequestsTotal.Inc()
	if metrics.RequestsTotal.Get() != 1 {
		t.Errorf("Expected requests total 1, got %f", metrics.RequestsTotal.Get())
	}

	// Test request duration
	duration := 0.250 // 250ms
	metrics.RequestDuration.Observe(duration)

	data := metrics.RequestDuration.Get()
	if count, ok := data["count"].(int64); !ok || count != 1 {
		t.Errorf("Expected request duration count 1, got %v", data["count"])
	}

	// Test request errors
	metrics.RequestErrors.Inc()
	if metrics.RequestErrors.Get() != 1 {
		t.Errorf("Expected request errors 1, got %f", metrics.RequestErrors.Get())
	}
}

func TestKVStoreMetrics_StorageMetrics(t *testing.T) {
	metrics := NewKVStoreMetrics()

	// Test storage operations
	metrics.StorageOperations.Inc()
	if metrics.StorageOperations.Get() != 1 {
		t.Errorf("Expected storage operations 1, got %f", metrics.StorageOperations.Get())
	}

	// Test storage size
	storageSize := float64(2048)
	metrics.StorageSize.Set(storageSize)
	if metrics.StorageSize.Get() != storageSize {
		t.Errorf("Expected storage size %f, got %f", storageSize, metrics.StorageSize.Get())
	}

	// Test storage entries
	entryCount := float64(150)
	metrics.StorageEntries.Set(entryCount)
	if metrics.StorageEntries.Get() != entryCount {
		t.Errorf("Expected storage entries %f, got %f", entryCount, metrics.StorageEntries.Get())
	}

	// Test storage errors
	metrics.StorageErrors.Inc()
	if metrics.StorageErrors.Get() != 1 {
		t.Errorf("Expected storage errors 1, got %f", metrics.StorageErrors.Get())
	}
}

func TestKVStoreMetrics_HTTPMetrics(t *testing.T) {
	metrics := NewKVStoreMetrics()

	// Test HTTP requests
	metrics.HTTPRequests.Inc()
	if metrics.HTTPRequests.Get() != 1 {
		t.Errorf("Expected HTTP requests 1, got %f", metrics.HTTPRequests.Get())
	}

	// Test HTTP duration
	duration := 0.150 // 150ms
	metrics.HTTPDuration.Observe(duration)

	data := metrics.HTTPDuration.Get()
	if count, ok := data["count"].(int64); !ok || count != 1 {
		t.Errorf("Expected HTTP duration count 1, got %v", data["count"])
	}

	// Test HTTP response size
	responseSize := 1024.0 // 1KB
	metrics.HTTPResponseSize.Observe(responseSize)

	sizeData := metrics.HTTPResponseSize.Get()
	if count, ok := sizeData["count"].(int64); !ok || count != 1 {
		t.Errorf("Expected HTTP response size count 1, got %v", sizeData["count"])
	}
}

func TestMetricsCollector(t *testing.T) {
	metrics := NewKVStoreMetrics()

	// Create collector with short interval for testing
	collector := NewMetricsCollector(metrics, 10*time.Millisecond)

	// Start collection
	collector.Start()

	// Wait a bit for collection to happen
	time.Sleep(50 * time.Millisecond)

	// Stop collection
	collector.Stop()

	// Check that metrics were updated (should be > 0)
	if metrics.MemoryUsage.Get() <= 0 {
		t.Error("Expected memory usage to be updated")
	}

	if metrics.GoroutineCount.Get() <= 0 {
		t.Error("Expected goroutine count to be updated")
	}
}

func TestMetricsCollector_Stop(t *testing.T) {
	metrics := NewKVStoreMetrics()
	collector := NewMetricsCollector(metrics, 10*time.Millisecond)

	// Start and immediately stop
	collector.Start()
	collector.Stop()

	// Should not panic or hang
	time.Sleep(20 * time.Millisecond)
}

func TestHistogram_DefaultBuckets(t *testing.T) {
	registry := NewMetricsRegistry()
	
	// Create histogram with default buckets (nil)
	histogram := registry.NewHistogram("test_histogram", "Test histogram", nil, nil)

	// Should have default buckets
	expectedBuckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	if len(histogram.buckets) != len(expectedBuckets) {
		t.Errorf("Expected %d default buckets, got %d", len(expectedBuckets), len(histogram.buckets))
	}

	for i, expected := range expectedBuckets {
		if histogram.buckets[i] != expected {
			t.Errorf("Expected bucket %d to be %f, got %f", i, expected, histogram.buckets[i])
		}
	}
}

func TestSummary_DefaultQuantiles(t *testing.T) {
	registry := NewMetricsRegistry()
	
	// Create summary with default quantiles (nil)
	summary := registry.NewSummary("test_summary", "Test summary", nil, nil)

	// Should have default quantiles
	expectedQuantiles := []float64{0.5, 0.9, 0.95, 0.99}
	if len(summary.quantiles) != len(expectedQuantiles) {
		t.Errorf("Expected %d default quantiles, got %d", len(expectedQuantiles), len(summary.quantiles))
	}

	for i, expected := range expectedQuantiles {
		if summary.quantiles[i] != expected {
			t.Errorf("Expected quantile %d to be %f, got %f", i, expected, summary.quantiles[i])
		}
	}
}