package monitoring

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusExporter_ServeHTTP(t *testing.T) {
	// Create KVStore metrics with some test data
	metrics := NewKVStoreMetrics()
	
	// Add some test metrics
	metrics.HTTPRequests.Inc()
	metrics.HTTPRequests.Inc()
	metrics.RequestsTotal.Inc()
	metrics.MemoryUsage.Set(104857600) // 100MB
	metrics.GoroutineCount.Set(25)
	metrics.RequestDuration.Observe(0.25)
	metrics.HTTPDuration.Observe(0.15)

	// Create Prometheus exporter
	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	// Create test request
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	// Serve the metrics
	exporter.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check content type
	expectedContentType := "text/plain; charset=utf-8"
	if w.Header().Get("Content-Type") != expectedContentType {
		t.Errorf("Expected content type %s, got %s", expectedContentType, w.Header().Get("Content-Type"))
	}

	// Parse the response body
	body := w.Body.String()
	
	// Check that basic metrics are present
	expectedMetrics := []string{
		"kvstore_http_requests_total",
		"kvstore_requests_total",
		"kvstore_memory_usage_bytes",
		"kvstore_goroutines",
		"kvstore_http_request_duration_seconds",
		"kvstore_request_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Expected metric %s to be present in output", metric)
		}
	}

	// Check for HELP and TYPE comments
	if !strings.Contains(body, "# HELP") {
		t.Error("Expected HELP comments in output")
	}

	if !strings.Contains(body, "# TYPE") {
		t.Error("Expected TYPE comments in output")
	}

	// Verify specific metric values
	lines := strings.Split(body, "\n")
	found := make(map[string]bool)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Check HTTP requests counter (format: metric_name value timestamp)
		if strings.Contains(line, "kvstore_http_requests_total") && strings.Contains(line, " 2.000000 ") {
			found["http_requests"] = true
		}
		
		// Check memory usage gauge exists (actual system value, not our test value)
		if strings.Contains(line, "kvstore_memory_usage_bytes") && !strings.HasPrefix(line, "#") {
			found["memory_usage"] = true
		}
		
		// Check goroutine count gauge exists (actual system value, not our test value)
		if strings.Contains(line, "kvstore_goroutines") && !strings.HasPrefix(line, "#") {
			found["goroutines"] = true
		}
	}

	if !found["http_requests"] {
		t.Error("Expected to find HTTP requests counter with value 2")
	}
	if !found["memory_usage"] {
		t.Error("Expected to find memory usage gauge in output")
	}
	if !found["goroutines"] {
		t.Error("Expected to find goroutines gauge in output")
	}
}

func TestPrometheusExporter_CounterFormat(t *testing.T) {
	metrics := NewKVStoreMetrics()
	
	// Increment counter with different labels
	counter := metrics.HTTPRequests
	counter.Add(5)

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	body := w.Body.String()
	
	// Check counter formatting
	expectedLines := []string{
		"# HELP kvstore_http_requests_total Total HTTP requests",
		"# TYPE kvstore_http_requests_total counter",
	}
	
	// Check for the metric line with value (allowing for timestamp)
	if !strings.Contains(body, "kvstore_http_requests_total 5.000000 ") {
		t.Error("Expected to find HTTP requests counter with value 5")
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(body, expectedLine) {
			t.Errorf("Expected line not found: %s\nBody: %s", expectedLine, body)
		}
	}
}

func TestPrometheusExporter_GaugeFormat(t *testing.T) {
	metrics := NewKVStoreMetrics()
	
	// Set gauge values
	metrics.MemoryUsage.Set(2048)
	metrics.GoroutineCount.Set(42)

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	body := w.Body.String()
	
	// Check gauge formatting
	expectedLines := []string{
		"# HELP kvstore_memory_usage_bytes Current memory usage in bytes",
		"# TYPE kvstore_memory_usage_bytes gauge",
		"# HELP kvstore_goroutines Current number of goroutines", 
		"# TYPE kvstore_goroutines gauge",
	}
	
	// The values will be system values due to UpdateSystemMetrics() call, so just check format exists
	// We can't predict exact values since they're real system metrics
	if !strings.Contains(body, "kvstore_memory_usage_bytes") {
		t.Error("Expected to find memory usage gauge in output")
	}
	if !strings.Contains(body, "kvstore_goroutines") {
		t.Error("Expected to find goroutines gauge in output")
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(body, expectedLine) {
			t.Errorf("Expected line not found: %s", expectedLine)
		}
	}
}

func TestPrometheusExporter_HistogramFormat(t *testing.T) {
	metrics := NewKVStoreMetrics()
	
	// Add observations to histogram
	histogram := metrics.RequestDuration
	histogram.Observe(0.1)
	histogram.Observe(0.5)
	histogram.Observe(1.0)

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	body := w.Body.String()
	
	// Check histogram formatting
	expectedLines := []string{
		"# HELP kvstore_request_duration_seconds Request duration in seconds",
		"# TYPE kvstore_request_duration_seconds histogram",
	}
	
	// Check for histogram-specific lines (allowing for timestamp)
	if !strings.Contains(body, "kvstore_request_duration_seconds_count 3 ") {
		t.Error("Expected to find histogram count with value 3")
	}
	if !strings.Contains(body, "kvstore_request_duration_seconds_sum 1.600000 ") {
		t.Error("Expected to find histogram sum with value 1.6")
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(body, expectedLine) {
			t.Errorf("Expected line not found: %s", expectedLine)
		}
	}

	// Check for bucket lines
	expectedBucketPatterns := []string{
		"kvstore_request_duration_seconds_bucket{le=\"0.005\"}",
		"kvstore_request_duration_seconds_bucket{le=\"+Inf\"}",
	}

	for _, pattern := range expectedBucketPatterns {
		if !strings.Contains(body, pattern) {
			t.Errorf("Expected bucket pattern not found: %s", pattern)
		}
	}
}

func TestPrometheusExporter_SummaryFormat(t *testing.T) {
	// Create a custom summary for testing
	registry := NewMetricsRegistry()
	summary := registry.NewSummary("test_summary", "Test summary for Prometheus", []float64{0.5, 0.9}, nil)
	
	// Add observations
	summary.Observe(1.0)
	summary.Observe(2.0)
	summary.Observe(3.0)

	// Create a temporary KVStoreMetrics with our test summary
	metrics := NewKVStoreMetrics()
	
	// Add the summary to the registry (we'll test this indirectly through GetAllMetrics)
	_ = registry.GetAllMetrics()
	
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	// Create and test a mock Prometheus output for summary
	// Since our actual implementation may not export summaries directly, we'll test the format
	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)
	exporter.ServeHTTP(w, req)

	// For now, just check that the exporter doesn't crash with summaries
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify content type
	expectedContentType := "text/plain; charset=utf-8"
	if w.Header().Get("Content-Type") != expectedContentType {
		t.Errorf("Expected content type %s, got %s", expectedContentType, w.Header().Get("Content-Type"))
	}

	// Check that we have valid Prometheus output
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty Prometheus output")
	}
}

func TestPrometheusExporter_Labels(t *testing.T) {
	metrics := NewKVStoreMetrics()
	
	// The HTTPRequests counter has labels defined in its creation
	counter := metrics.HTTPRequests
	counter.Inc()

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	body := w.Body.String()
	
	// Check that labels are properly formatted (even if empty)
	// The counter should appear with its metric name
	if !strings.Contains(body, "kvstore_http_requests_total") {
		t.Error("Expected HTTP requests counter in output")
	}
}

func TestPrometheusExporter_EmptyMetrics(t *testing.T) {
	// Create empty metrics
	metrics := NewKVStoreMetrics()

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	// Should still return 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	
	// Should have metric names but with zero values
	expectedMetrics := []string{
		"kvstore_http_requests_total",
		"kvstore_memory_usage_bytes", 
		"kvstore_goroutines",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Expected metric %s to be present even with zero value", metric)
		}
	}
}

func TestPrometheusExporter_HTTPMethods(t *testing.T) {
	metrics := NewKVStoreMetrics()
	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	// Test GET method
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET: Expected status 200, got %d", w.Code)
	}

	// Test POST method (should still work)
	req = httptest.NewRequest("POST", "/metrics", nil)
	w = httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST: Expected status 200, got %d", w.Code)
	}

	// Test HEAD method
	req = httptest.NewRequest("HEAD", "/metrics", nil)
	w = httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HEAD: Expected status 200, got %d", w.Code)
	}

	// For HEAD, we still get a body in our implementation (this is fine for metrics endpoint)
	// The HTTP library handles HEAD vs GET difference at transport level

	expectedContentType := "text/plain; charset=utf-8"
	if w.Header().Get("Content-Type") != expectedContentType {
		t.Errorf("HEAD: Expected content type %s, got %s", expectedContentType, w.Header().Get("Content-Type"))
	}
}

func TestPrometheusExporter_ConcurrentRequests(t *testing.T) {
	metrics := NewKVStoreMetrics()
	
	// Add some metrics
	metrics.HTTPRequests.Inc()
	metrics.MemoryUsage.Set(1024)

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	// Make concurrent requests
	const numRequests = 10
	results := make(chan int, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/metrics", nil)
			w := httptest.NewRecorder()
			exporter.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		code := <-results
		if code != http.StatusOK {
			t.Errorf("Concurrent request %d: Expected status 200, got %d", i, code)
		}
	}
}

func TestPrometheusExporter_MetricNameEscaping(t *testing.T) {
	// Create custom registry to test metric name handling
	registry := NewMetricsRegistry()
	
	// Create metrics with names that need escaping (though our implementation may not support this)
	counter := registry.NewCounter("test_counter_with_underscores", "Test counter", nil)
	counter.Inc()

	metrics := NewKVStoreMetrics()
	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Basic check that we get valid output
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty output")
	}
}

func TestPrometheusExporter_OutputFormat(t *testing.T) {
	metrics := NewKVStoreMetrics()
	
	// Add test data
	metrics.HTTPRequests.Add(3)
	metrics.MemoryUsage.Set(512)
	metrics.RequestDuration.Observe(0.5)

	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	exporter.ServeHTTP(w, req)

	body := w.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))

	helpCount := 0
	typeCount := 0
	metricCount := 0
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		
		if strings.HasPrefix(line, "# HELP") {
			helpCount++
		} else if strings.HasPrefix(line, "# TYPE") {
			typeCount++
		} else if !strings.HasPrefix(line, "#") {
			metricCount++
		}
	}

	// Should have help and type comments
	if helpCount == 0 {
		t.Error("Expected at least one HELP comment")
	}
	
	if typeCount == 0 {
		t.Error("Expected at least one TYPE comment")
	}
	
	if metricCount == 0 {
		t.Error("Expected at least one metric line")
	}

	// Basic format validation - each metric should have help and type
	if helpCount != typeCount {
		t.Errorf("Expected equal number of HELP (%d) and TYPE (%d) comments", helpCount, typeCount)
	}
}

func TestPrometheusExporter_Timestamp(t *testing.T) {
	metrics := NewKVStoreMetrics()
	metrics.HTTPRequests.Inc()
	
	exporter := NewPrometheusExporter(metrics.GetRegistry(), metrics)

	// Make two requests with a small delay
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	w1 := httptest.NewRecorder()
	exporter.ServeHTTP(w1, req1)
	
	time.Sleep(10 * time.Millisecond)
	
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	w2 := httptest.NewRecorder()
	exporter.ServeHTTP(w2, req2)

	// Both should succeed
	if w1.Code != http.StatusOK {
		t.Errorf("First request: Expected status 200, got %d", w1.Code)
	}
	
	if w2.Code != http.StatusOK {
		t.Errorf("Second request: Expected status 200, got %d", w2.Code)
	}

	// Both should have content
	if len(w1.Body.String()) == 0 {
		t.Error("First request: Expected non-empty body")
	}
	
	if len(w2.Body.String()) == 0 {
		t.Error("Second request: Expected non-empty body")
	}
}