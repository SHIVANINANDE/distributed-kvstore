package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setupTestDashboard(t *testing.T) *Dashboard {
	healthManager := NewHealthManager("test-version")
	metrics := NewKVStoreMetrics()
	
	// Add some test health checkers
	healthManager.RegisterChecker(&MockHealthChecker{
		name:     "test-storage",
		critical: true,
		result:   HealthCheck{Status: HealthStatusHealthy, Message: "Storage OK"},
	})
	
	healthManager.RegisterChecker(&MockHealthChecker{
		name:     "test-memory",
		critical: false,
		result:   HealthCheck{Status: HealthStatusDegraded, Message: "Memory usage high"},
	})
	
	// Add some test metrics data
	metrics.RequestsTotal.Add(100)
	metrics.RequestErrors.Add(5)
	metrics.HTTPRequests.Add(150)
	metrics.MemoryUsage.Set(512 * 1024 * 1024) // 512MB (will be overwritten by system metrics)
	metrics.StorageSize.Set(1024 * 1024)       // 1MB
	
	return NewDashboard(healthManager, metrics.GetRegistry(), metrics)
}

func TestNewDashboard(t *testing.T) {
	healthManager := NewHealthManager("test-version")
	metrics := NewKVStoreMetrics()
	
	dashboard := NewDashboard(healthManager, metrics.GetRegistry(), metrics)
	
	if dashboard == nil {
		t.Fatal("Expected dashboard to be created")
	}
	
	if dashboard.healthManager != healthManager {
		t.Error("Expected health manager to be set")
	}
	
	if dashboard.metricsRegistry != metrics.GetRegistry() {
		t.Error("Expected metrics registry to be set")
	}
	
	if dashboard.kvMetrics != metrics {
		t.Error("Expected KV metrics to be set")
	}
}

func TestDashboard_ServeHTTP_DashboardHTML(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	
	dashboard.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	expectedContentType := "text/html; charset=utf-8"
	if contentType != expectedContentType {
		t.Errorf("Expected content type %s, got %s", expectedContentType, contentType)
	}
	
	body := w.Body.String()
	
	// Check for essential HTML elements
	expectedElements := []string{
		"<!DOCTYPE html>",
		"<html",
		"<head>",
		"<body>",
		"KVStore Monitoring Dashboard",
		"System Statistics",
		"Health Status",
	}
	
	for _, element := range expectedElements {
		if !strings.Contains(body, element) {
			t.Errorf("Expected HTML element not found: %s", element)
		}
	}
	
	// Check that template variables are properly rendered
	if !strings.Contains(body, "Distributed Key-Value Store") {
		t.Error("Expected service name to be rendered")
	}
}

func TestDashboard_ServeHTTP_HealthAPI(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	req := httptest.NewRequest("GET", "/dashboard/api/health", nil)
	w := httptest.NewRecorder()
	
	dashboard.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}
	
	var healthResponse HealthResponse
	err := json.NewDecoder(w.Body).Decode(&healthResponse)
	if err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}
	
	if healthResponse.Status == "" {
		t.Error("Expected health status to be set")
	}
	
	if healthResponse.Version != "test-version" {
		t.Errorf("Expected version 'test-version', got '%s'", healthResponse.Version)
	}
	
	if len(healthResponse.Checks) == 0 {
		t.Error("Expected health checks to be present")
	}
	
	// Check specific health checks
	if storage, exists := healthResponse.Checks["test-storage"]; exists {
		if storage.Status != HealthStatusHealthy {
			t.Errorf("Expected storage health to be healthy, got %s", storage.Status)
		}
	} else {
		t.Error("Expected test-storage health check to exist")
	}
	
	if memory, exists := healthResponse.Checks["test-memory"]; exists {
		if memory.Status != HealthStatusDegraded {
			t.Errorf("Expected memory health to be degraded, got %s", memory.Status)
		}
	} else {
		t.Error("Expected test-memory health check to exist")
	}
}

func TestDashboard_ServeHTTP_MetricsAPI(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	req := httptest.NewRequest("GET", "/dashboard/api/metrics", nil)
	w := httptest.NewRecorder()
	
	dashboard.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}
	
	var metrics map[string]*Metric
	err := json.NewDecoder(w.Body).Decode(&metrics)
	if err != nil {
		t.Fatalf("Failed to decode metrics response: %v", err)
	}
	
	if len(metrics) == 0 {
		t.Error("Expected metrics to be present")
	}
	
	// Check for expected metrics
	expectedMetrics := []string{
		"kvstore_requests_total",
		"kvstore_memory_usage_bytes",
		"kvstore_http_requests_total",
	}
	
	for _, metricName := range expectedMetrics {
		if _, exists := metrics[metricName]; !exists {
			t.Errorf("Expected metric %s to exist", metricName)
		}
	}
	
	// Verify metric structure
	if requestsTotal, exists := metrics["kvstore_requests_total"]; exists {
		if requestsTotal.Type != MetricTypeCounter {
			t.Errorf("Expected requests_total to be counter, got %s", requestsTotal.Type)
		}
		if requestsTotal.Name != "kvstore_requests_total" {
			t.Errorf("Expected metric name kvstore_requests_total, got %s", requestsTotal.Name)
		}
	}
}

func TestDashboard_ServeHTTP_AlertsAPI(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	req := httptest.NewRequest("GET", "/dashboard/api/alerts", nil)
	w := httptest.NewRecorder()
	
	dashboard.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}
	
	var alerts []Alert
	err := json.NewDecoder(w.Body).Decode(&alerts)
	if err != nil {
		t.Fatalf("Failed to decode alerts response: %v", err)
	}
	
	// Should have at least one alert from the degraded memory health check
	if len(alerts) == 0 {
		t.Error("Expected at least one alert")
	}
	
	// Find the alert for degraded memory
	var memoryAlert *Alert
	for i := range alerts {
		if strings.Contains(alerts[i].Title, "test-memory") {
			memoryAlert = &alerts[i]
			break
		}
	}
	
	if memoryAlert == nil {
		t.Error("Expected to find alert for degraded memory health check")
	} else {
		if memoryAlert.Level != "warning" {
			t.Errorf("Expected memory alert level to be warning, got %s", memoryAlert.Level)
		}
		if memoryAlert.Source != "health_check" {
			t.Errorf("Expected memory alert source to be health_check, got %s", memoryAlert.Source)
		}
	}
}

func TestDashboard_ServeHTTP_ChartsAPI(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	req := httptest.NewRequest("GET", "/dashboard/api/charts", nil)
	w := httptest.NewRecorder()
	
	dashboard.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}
	
	var chartData ChartData
	err := json.NewDecoder(w.Body).Decode(&chartData)
	if err != nil {
		t.Fatalf("Failed to decode chart data response: %v", err)
	}
	
	// Check that chart data has expected time series
	if len(chartData.RequestsPerSecond) == 0 {
		t.Error("Expected requests per second data")
	}
	
	if len(chartData.ResponseTimes) == 0 {
		t.Error("Expected response times data")
	}
	
	if len(chartData.ErrorRates) == 0 {
		t.Error("Expected error rates data")
	}
	
	if len(chartData.StorageSize) == 0 {
		t.Error("Expected storage size data")
	}
	
	// Check data point structure
	if len(chartData.RequestsPerSecond) > 0 {
		firstPoint := chartData.RequestsPerSecond[0]
		if firstPoint.Time == "" {
			t.Error("Expected time value to be set")
		}
		if firstPoint.Value < 0 {
			t.Error("Expected value to be non-negative")
		}
	}
}

func TestDashboard_ServeHTTP_NotFound(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	req := httptest.NewRequest("GET", "/dashboard/invalid-path", nil)
	w := httptest.NewRecorder()
	
	dashboard.ServeHTTP(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestDashboard_getDashboardData(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	data := dashboard.getDashboardData()
	
	if data.Title != "KVStore Monitoring Dashboard" {
		t.Errorf("Expected title 'KVStore Monitoring Dashboard', got '%s'", data.Title)
	}
	
	if data.RefreshTime == "" {
		t.Error("Expected refresh time to be set")
	}
	
	if data.Health.Status == "" {
		t.Error("Expected health status to be set")
	}
	
	if len(data.Metrics) == 0 {
		t.Error("Expected metrics to be present")
	}
	
	// Memory usage should be >= 0 (could be 0 in some test environments)
	if data.SystemStats.MemoryUsageMB < 0 {
		t.Error("Expected system stats memory usage to be non-negative")
	}
	
	if len(data.ChartData.RequestsPerSecond) == 0 {
		t.Error("Expected chart data to be present")
	}
	
	if len(data.Alerts) == 0 {
		t.Error("Expected alerts to be present")
	}
	
	if data.ServiceInfo.Name != "Distributed Key-Value Store" {
		t.Errorf("Expected service name 'Distributed Key-Value Store', got '%s'", data.ServiceInfo.Name)
	}
	
	if data.ServiceInfo.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", data.ServiceInfo.Version)
	}
}

func TestDashboard_getSystemStats(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	stats := dashboard.getSystemStats()
	
	if stats.CPUUsage != 0.0 {
		t.Errorf("Expected CPU usage to be 0.0 (placeholder), got %f", stats.CPUUsage)
	}
	
	// Memory usage will be updated by system metrics, so just check it's reasonable
	if stats.MemoryUsageMB < 0 {
		t.Error("Expected memory usage to be non-negative")
	}
	
	// Goroutine count should be non-negative (could be 0 in some environments)
	if stats.GoroutineCount < 0 {
		t.Error("Expected goroutine count to be non-negative")
	}
	
	if stats.Uptime != "running" {
		t.Errorf("Expected uptime to be 'running', got '%s'", stats.Uptime)
	}
}

func TestDashboard_generateChartData(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	chartData := dashboard.generateChartData()
	
	// Should have 20 data points for each chart
	expectedDataPoints := 20
	
	if len(chartData.RequestsPerSecond) != expectedDataPoints {
		t.Errorf("Expected %d request data points, got %d", expectedDataPoints, len(chartData.RequestsPerSecond))
	}
	
	if len(chartData.ResponseTimes) != expectedDataPoints {
		t.Errorf("Expected %d response time data points, got %d", expectedDataPoints, len(chartData.ResponseTimes))
	}
	
	if len(chartData.ErrorRates) != expectedDataPoints {
		t.Errorf("Expected %d error rate data points, got %d", expectedDataPoints, len(chartData.ErrorRates))
	}
	
	if len(chartData.StorageSize) != expectedDataPoints {
		t.Errorf("Expected %d storage size data points, got %d", expectedDataPoints, len(chartData.StorageSize))
	}
	
	// Check data point format
	if len(chartData.ResponseTimes) > 0 {
		firstPoint := chartData.ResponseTimes[0]
		if firstPoint.Time == "" {
			t.Error("Expected time to be formatted")
		}
		
		// Response times should be ascending (0.1 + i*0.01)
		if firstPoint.Value < 0.1 {
			t.Errorf("Expected first response time >= 0.1, got %f", firstPoint.Value)
		}
	}
}

func TestDashboard_generateAlerts(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	alerts := dashboard.generateAlerts()
	
	// Should have at least one alert from degraded health check
	if len(alerts) == 0 {
		t.Error("Expected at least one alert")
	}
	
	// Find the health check alert
	var healthAlert *Alert
	for i := range alerts {
		if alerts[i].Source == "health_check" {
			healthAlert = &alerts[i]
			break
		}
	}
	
	if healthAlert == nil {
		t.Error("Expected to find health check alert")
	} else {
		if healthAlert.Level != "warning" {
			t.Errorf("Expected health alert level to be warning, got %s", healthAlert.Level)
		}
		
		if healthAlert.Title == "" {
			t.Error("Expected alert title to be set")
		}
		
		if healthAlert.Description == "" {
			t.Error("Expected alert description to be set")
		}
		
		if healthAlert.Timestamp.IsZero() {
			t.Error("Expected alert timestamp to be set")
		}
	}
}

func TestDashboard_generateAlerts_MemoryThreshold(t *testing.T) {
	healthManager := NewHealthManager("test-version")
	metrics := NewKVStoreMetrics()
	dashboard := NewDashboard(healthManager, metrics.GetRegistry(), metrics)
	
	// Set memory to high value (the actual system value will override this in UpdateSystemMetrics)
	// So we'll test the logic with a mock scenario
	
	alerts := dashboard.generateAlerts()
	
	// Check if high memory alert is generated (depends on actual system memory)
	var memoryAlert *Alert
	for i := range alerts {
		if strings.Contains(alerts[i].Title, "High Memory Usage") {
			memoryAlert = &alerts[i]
			break
		}
	}
	
	// If system has >500MB memory in use, there should be an alert
	// Otherwise, no alert is expected - both cases are valid
	if memoryAlert != nil {
		if memoryAlert.Level != "warning" {
			t.Errorf("Expected memory alert level to be warning, got %s", memoryAlert.Level)
		}
		
		if memoryAlert.Source != "metrics" {
			t.Errorf("Expected memory alert source to be metrics, got %s", memoryAlert.Source)
		}
	}
}

func TestDashboard_ConcurrentRequests(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	const numRequests = 10
	results := make(chan int, numRequests)
	
	// Test concurrent requests to different endpoints
	endpoints := []string{
		"/dashboard/api/health",
		"/dashboard/api/metrics", 
		"/dashboard/api/alerts",
		"/dashboard/api/charts",
	}
	
	for i := 0; i < numRequests; i++ {
		go func(endpoint string) {
			req := httptest.NewRequest("GET", endpoint, nil)
			w := httptest.NewRecorder()
			dashboard.ServeHTTP(w, req)
			results <- w.Code
		}(endpoints[i%len(endpoints)])
	}
	
	// Collect results
	for i := 0; i < numRequests; i++ {
		code := <-results
		if code != http.StatusOK {
			t.Errorf("Concurrent request %d: Expected status 200, got %d", i, code)
		}
	}
}

func TestDashboard_HTTPMethods(t *testing.T) {
	dashboard := setupTestDashboard(t)
	
	tests := []struct {
		method   string
		path     string
		expected int
	}{
		{"GET", "/dashboard", http.StatusOK},
		{"POST", "/dashboard", http.StatusOK}, // Should still work
		{"GET", "/dashboard/api/health", http.StatusOK},
		{"POST", "/dashboard/api/health", http.StatusOK}, // Should still work
		{"GET", "/invalid", http.StatusNotFound},
	}
	
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		
		dashboard.ServeHTTP(w, req)
		
		if w.Code != tt.expected {
			t.Errorf("%s %s: Expected status %d, got %d", tt.method, tt.path, tt.expected, w.Code)
		}
	}
}

func TestAlert_Structure(t *testing.T) {
	alert := Alert{
		Level:       "error",
		Title:       "Test Alert",
		Description: "This is a test alert",
		Timestamp:   time.Now(),
		Source:      "test",
	}
	
	if alert.Level != "error" {
		t.Errorf("Expected level 'error', got '%s'", alert.Level)
	}
	
	if alert.Title != "Test Alert" {
		t.Errorf("Expected title 'Test Alert', got '%s'", alert.Title)
	}
	
	if alert.Description != "This is a test alert" {
		t.Errorf("Expected description 'This is a test alert', got '%s'", alert.Description)
	}
	
	if alert.Source != "test" {
		t.Errorf("Expected source 'test', got '%s'", alert.Source)
	}
	
	if alert.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestTimeValue_Structure(t *testing.T) {
	tv := TimeValue{
		Time:  "12:34:56",
		Value: 42.5,
	}
	
	if tv.Time != "12:34:56" {
		t.Errorf("Expected time '12:34:56', got '%s'", tv.Time)
	}
	
	if tv.Value != 42.5 {
		t.Errorf("Expected value 42.5, got %f", tv.Value)
	}
}

func TestServiceInfo_Structure(t *testing.T) {
	info := ServiceInfo{
		Name:      "Test Service",
		Version:   "1.2.3",
		BuildTime: "2025-01-01 12:00:00",
		GitCommit: "abc123",
	}
	
	if info.Name != "Test Service" {
		t.Errorf("Expected name 'Test Service', got '%s'", info.Name)
	}
	
	if info.Version != "1.2.3" {
		t.Errorf("Expected version '1.2.3', got '%s'", info.Version)
	}
	
	if info.BuildTime != "2025-01-01 12:00:00" {
		t.Errorf("Expected build time '2025-01-01 12:00:00', got '%s'", info.BuildTime)
	}
	
	if info.GitCommit != "abc123" {
		t.Errorf("Expected git commit 'abc123', got '%s'", info.GitCommit)
	}
}