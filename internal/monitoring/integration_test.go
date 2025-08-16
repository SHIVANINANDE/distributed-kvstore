package monitoring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testError struct {
	msg string
}

func (te *testError) Error() string {
	return te.msg
}

func TestNewMonitoringService(t *testing.T) {
	storage := &MockStorageEngine{}
	
	service := NewMonitoringService(storage)
	
	if service == nil {
		t.Fatal("Expected monitoring service to be created")
	}
	
	if service.HealthManager == nil {
		t.Error("Expected health manager to be initialized")
	}
	
	if service.MetricsRegistry == nil {
		t.Error("Expected metrics registry to be initialized")
	}
	
	if service.KVMetrics == nil {
		t.Error("Expected KV metrics to be initialized")
	}
	
	if service.Dashboard == nil {
		t.Error("Expected dashboard to be initialized")
	}
	
	if service.Collector == nil {
		t.Error("Expected collector to be initialized")
	}
	
	if service.PrometheusExporter == nil {
		t.Error("Expected Prometheus exporter to be initialized")
	}
}

func TestMonitoringService_StartStop(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	// Start service
	service.Start()
	
	// Give it a moment to start collecting
	time.Sleep(10 * time.Millisecond)
	
	// Stop service
	service.Stop()
	
	// Should not panic or hang
}

func TestMonitoringService_GetHealthHandler(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	handler := service.GetHealthHandler()
	if handler == nil {
		t.Fatal("Expected health handler to be created")
	}
	
	// Test healthy response
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	
	handler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}
	
	body := w.Body.String()
	if !strings.Contains(body, "status") {
		t.Error("Expected health response to contain status")
	}
}

func TestMonitoringService_GetHealthHandler_Unhealthy(t *testing.T) {
	storage := &MockStorageEngine{shouldError: true, err: &testError{msg: "storage unavailable"}}
	service := NewMonitoringService(storage)
	
	handler := service.GetHealthHandler()
	
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	
	handler(w, req)
	
	// Should return service unavailable for unhealthy storage
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestMonitoringService_GetMetricsHandler(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	handler := service.GetMetricsHandler()
	if handler == nil {
		t.Fatal("Expected metrics handler to be created")
	}
	
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	expectedContentType := "text/plain; charset=utf-8"
	if contentType != expectedContentType {
		t.Errorf("Expected content type %s, got %s", expectedContentType, contentType)
	}
	
	body := w.Body.String()
	if !strings.Contains(body, "kvstore_") {
		t.Error("Expected metrics output to contain kvstore metrics")
	}
}

func TestMonitoringService_GetDashboardHandler(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	handler := service.GetDashboardHandler()
	if handler == nil {
		t.Fatal("Expected dashboard handler to be created")
	}
	
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMonitoringService_RecordHTTPRequest(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	// Get initial values
	initialRequests := service.KVMetrics.HTTPRequests.Get()
	initialErrors := service.KVMetrics.RequestErrors.Get()
	
	// Record successful request
	service.RecordHTTPRequest("GET", "/api/test", 200, 100*time.Millisecond, 1024)
	
	if service.KVMetrics.HTTPRequests.Get() != initialRequests+1 {
		t.Error("Expected HTTP requests counter to increment")
	}
	
	// Record error request
	service.RecordHTTPRequest("POST", "/api/test", 500, 200*time.Millisecond, 0)
	
	if service.KVMetrics.RequestErrors.Get() != initialErrors+1 {
		t.Error("Expected request errors counter to increment for 5xx status")
	}
	
	// Record client error
	service.RecordHTTPRequest("PUT", "/api/test", 404, 50*time.Millisecond, 256)
	
	if service.KVMetrics.RequestErrors.Get() != initialErrors+2 {
		t.Error("Expected request errors counter to increment for 4xx status")
	}
}

func TestMonitoringService_RecordStorageOperation(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	// Get initial values
	initialOperations := service.KVMetrics.StorageOperations.Get()
	initialErrors := service.KVMetrics.StorageErrors.Get()
	
	// Record successful operation
	service.RecordStorageOperation("GET", true, 5*time.Millisecond)
	
	if service.KVMetrics.StorageOperations.Get() != initialOperations+1 {
		t.Error("Expected storage operations counter to increment")
	}
	
	// Record failed operation
	service.RecordStorageOperation("SET", false, 10*time.Millisecond)
	
	if service.KVMetrics.StorageOperations.Get() != initialOperations+2 {
		t.Error("Expected storage operations counter to increment for failed operation")
	}
	
	if service.KVMetrics.StorageErrors.Get() != initialErrors+1 {
		t.Error("Expected storage errors counter to increment for failed operation")
	}
}

func TestMonitoringService_UpdateStorageMetrics(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	stats := map[string]interface{}{
		"total_size": 2048.0,
		"entries":    150.0,
	}
	
	service.UpdateStorageMetrics(stats)
	
	if service.KVMetrics.StorageSize.Get() != 2048.0 {
		t.Errorf("Expected storage size 2048, got %f", service.KVMetrics.StorageSize.Get())
	}
	
	if service.KVMetrics.StorageEntries.Get() != 150.0 {
		t.Errorf("Expected storage entries 150, got %f", service.KVMetrics.StorageEntries.Get())
	}
}

func TestMonitoringService_UpdateStorageMetrics_InvalidTypes(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	// Get initial values
	initialSize := service.KVMetrics.StorageSize.Get()
	initialEntries := service.KVMetrics.StorageEntries.Get()
	
	// Try to update with invalid types
	stats := map[string]interface{}{
		"total_size": "not a number",
		"entries":    []string{"not", "a", "number"},
	}
	
	service.UpdateStorageMetrics(stats)
	
	// Values should remain unchanged
	if service.KVMetrics.StorageSize.Get() != initialSize {
		t.Error("Storage size should not change with invalid type")
	}
	
	if service.KVMetrics.StorageEntries.Get() != initialEntries {
		t.Error("Storage entries should not change with invalid type")
	}
}

func TestMonitoringService_MonitoringMiddleware(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})
	
	// Wrap with monitoring middleware
	wrappedHandler := service.MonitoringMiddleware(testHandler)
	
	// Get initial metrics
	initialRequests := service.KVMetrics.HTTPRequests.Get()
	
	// Make request through middleware
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	
	wrappedHandler.ServeHTTP(w, req)
	
	// Check that metrics were recorded
	if service.KVMetrics.HTTPRequests.Get() != initialRequests+1 {
		t.Error("Expected HTTP requests counter to increment through middleware")
	}
	
	// Check response is preserved
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	if w.Body.String() != "test response" {
		t.Errorf("Expected 'test response', got '%s'", w.Body.String())
	}
}

func TestMonitoringService_MonitoringMiddleware_ErrorStatus(t *testing.T) {
	storage := &MockStorageEngine{}
	service := NewMonitoringService(storage)
	
	// Create a test handler that returns an error
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})
	
	// Wrap with monitoring middleware
	wrappedHandler := service.MonitoringMiddleware(testHandler)
	
	// Get initial metrics
	initialErrors := service.KVMetrics.RequestErrors.Get()
	
	// Make request through middleware
	req := httptest.NewRequest("POST", "/test", nil)
	w := httptest.NewRecorder()
	
	wrappedHandler.ServeHTTP(w, req)
	
	// Check that error was recorded
	if service.KVMetrics.RequestErrors.Get() != initialErrors+1 {
		t.Error("Expected request errors counter to increment for error status")
	}
}

func TestMonitoringResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	wrapped := &monitoringResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		size:          0,
	}
	
	// Test WriteHeader
	wrapped.WriteHeader(http.StatusCreated)
	if wrapped.statusCode != http.StatusCreated {
		t.Errorf("Expected status code 201, got %d", wrapped.statusCode)
	}
	
	// Test Write
	data := []byte("test data")
	n, err := wrapped.Write(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}
	
	if wrapped.size != int64(len(data)) {
		t.Errorf("Expected size %d, got %d", len(data), wrapped.size)
	}
	
	// Test multiple writes
	moreData := []byte(" more")
	wrapped.Write(moreData)
	
	expectedSize := int64(len(data) + len(moreData))
	if wrapped.size != expectedSize {
		t.Errorf("Expected total size %d, got %d", expectedSize, wrapped.size)
	}
}

func TestNewAlertManager(t *testing.T) {
	am := NewAlertManager()
	
	if am == nil {
		t.Fatal("Expected alert manager to be created")
	}
	
	if am.alerts == nil {
		t.Error("Expected alerts slice to be initialized")
	}
	
	if am.rules == nil {
		t.Error("Expected rules slice to be initialized")
	}
	
	if am.notifications == nil {
		t.Error("Expected notifications channel to be initialized")
	}
	
	// Should have default rules
	if len(am.rules) == 0 {
		t.Error("Expected default alert rules to be added")
	}
}

func TestAlertManager_DefaultRules(t *testing.T) {
	am := NewAlertManager()
	
	// Check that default rules are present
	expectedRuleNames := []string{"HighMemoryUsage", "HighErrorRate", "StorageUnhealthy"}
	
	if len(am.rules) < len(expectedRuleNames) {
		t.Errorf("Expected at least %d default rules, got %d", len(expectedRuleNames), len(am.rules))
	}
	
	// Check rule names
	ruleNames := make(map[string]bool)
	for _, rule := range am.rules {
		ruleNames[rule.Name] = true
	}
	
	for _, expectedName := range expectedRuleNames {
		if !ruleNames[expectedName] {
			t.Errorf("Expected default rule %s to be present", expectedName)
		}
	}
}

func TestAlertManager_CheckAlerts_HighMemoryUsage(t *testing.T) {
	am := NewAlertManager()
	
	// Create metrics with high memory usage
	metrics := map[string]*Metric{
		"kvstore_memory_usage_bytes": {
			Name:  "kvstore_memory_usage_bytes",
			Value: 900 * 1024 * 1024, // 900MB (above 800MB threshold)
		},
	}
	
	health := HealthResponse{
		Status: HealthStatusHealthy,
		Checks: make(map[string]HealthCheck),
	}
	
	initialAlertCount := len(am.alerts)
	
	am.CheckAlerts(metrics, health)
	
	// Should have generated high memory alert
	if len(am.alerts) <= initialAlertCount {
		t.Error("Expected high memory usage alert to be generated")
	}
	
	// Check alert details
	var memoryAlert *Alert
	for i := range am.alerts {
		if am.alerts[i].Title == "HighMemoryUsage" {
			memoryAlert = &am.alerts[i]
			break
		}
	}
	
	if memoryAlert == nil {
		t.Error("Expected to find HighMemoryUsage alert")
	} else {
		if memoryAlert.Level != "warning" {
			t.Errorf("Expected warning level, got %s", memoryAlert.Level)
		}
		if memoryAlert.Source != "alert_manager" {
			t.Errorf("Expected source alert_manager, got %s", memoryAlert.Source)
		}
	}
}

func TestAlertManager_CheckAlerts_HighErrorRate(t *testing.T) {
	am := NewAlertManager()
	
	// Create metrics with high error rate (10 errors out of 100 requests = 10%)
	metrics := map[string]*Metric{
		"kvstore_requests_total": {
			Name:  "kvstore_requests_total",
			Value: 100.0,
		},
		"kvstore_request_errors_total": {
			Name:  "kvstore_request_errors_total",
			Value: 10.0,
		},
	}
	
	health := HealthResponse{
		Status: HealthStatusHealthy,
		Checks: make(map[string]HealthCheck),
	}
	
	initialAlertCount := len(am.alerts)
	
	am.CheckAlerts(metrics, health)
	
	// Should have generated high error rate alert
	if len(am.alerts) <= initialAlertCount {
		t.Error("Expected high error rate alert to be generated")
	}
	
	// Check alert details
	var errorAlert *Alert
	for i := range am.alerts {
		if am.alerts[i].Title == "HighErrorRate" {
			errorAlert = &am.alerts[i]
			break
		}
	}
	
	if errorAlert == nil {
		t.Error("Expected to find HighErrorRate alert")
	} else {
		if errorAlert.Level != "critical" {
			t.Errorf("Expected critical level, got %s", errorAlert.Level)
		}
	}
}

func TestAlertManager_CheckAlerts_StorageUnhealthy(t *testing.T) {
	am := NewAlertManager()
	
	metrics := map[string]*Metric{}
	
	health := HealthResponse{
		Status: HealthStatusUnhealthy,
		Checks: map[string]HealthCheck{
			"storage": {
				Status:  HealthStatusUnhealthy,
				Message: "Storage connection failed",
			},
		},
	}
	
	initialAlertCount := len(am.alerts)
	
	am.CheckAlerts(metrics, health)
	
	// Should have generated storage unhealthy alert
	if len(am.alerts) <= initialAlertCount {
		t.Error("Expected storage unhealthy alert to be generated")
	}
	
	// Check alert details
	var storageAlert *Alert
	for i := range am.alerts {
		if am.alerts[i].Title == "StorageUnhealthy" {
			storageAlert = &am.alerts[i]
			break
		}
	}
	
	if storageAlert == nil {
		t.Error("Expected to find StorageUnhealthy alert")
	} else {
		if storageAlert.Level != "critical" {
			t.Errorf("Expected critical level, got %s", storageAlert.Level)
		}
	}
}

func TestAlertManager_CheckAlerts_NoAlerts(t *testing.T) {
	am := NewAlertManager()
	
	// Create metrics with normal values
	metrics := map[string]*Metric{
		"kvstore_memory_usage_bytes": {
			Name:  "kvstore_memory_usage_bytes",
			Value: 100 * 1024 * 1024, // 100MB (below threshold)
		},
		"kvstore_requests_total": {
			Name:  "kvstore_requests_total",
			Value: 100.0,
		},
		"kvstore_request_errors_total": {
			Name:  "kvstore_request_errors_total",
			Value: 2.0, // 2% error rate (below threshold)
		},
	}
	
	health := HealthResponse{
		Status: HealthStatusHealthy,
		Checks: map[string]HealthCheck{
			"storage": {
				Status:  HealthStatusHealthy,
				Message: "Storage OK",
			},
		},
	}
	
	initialAlertCount := len(am.alerts)
	
	am.CheckAlerts(metrics, health)
	
	// Should not have generated any alerts
	if len(am.alerts) != initialAlertCount {
		t.Error("Expected no alerts to be generated for normal conditions")
	}
}

func TestAlertManager_GetAlerts(t *testing.T) {
	am := NewAlertManager()
	
	// Manually add an alert
	testAlert := Alert{
		Level:       "info",
		Title:       "Test Alert",
		Description: "This is a test",
		Timestamp:   time.Now(),
		Source:      "test",
	}
	
	am.alerts = append(am.alerts, testAlert)
	
	alerts := am.GetAlerts()
	
	if len(alerts) == 0 {
		t.Error("Expected to get alerts")
	}
	
	found := false
	for _, alert := range alerts {
		if alert.Title == "Test Alert" {
			found = true
			break
		}
	}
	
	if !found {
		t.Error("Expected to find test alert in returned alerts")
	}
}

func TestAlertManager_GetNotifications(t *testing.T) {
	am := NewAlertManager()
	
	notifications := am.GetNotifications()
	if notifications == nil {
		t.Error("Expected notifications channel to be returned")
	}
	
	// Test that notifications are sent when alerts are triggered
	metrics := map[string]*Metric{
		"kvstore_memory_usage_bytes": {
			Name:  "kvstore_memory_usage_bytes",
			Value: 900 * 1024 * 1024, // High memory
		},
	}
	
	health := HealthResponse{
		Status: HealthStatusHealthy,
		Checks: make(map[string]HealthCheck),
	}
	
	// Check alerts (should trigger notification)
	go am.CheckAlerts(metrics, health)
	
	// Check for notification (with timeout)
	select {
	case alert := <-notifications:
		if alert.Title != "HighMemoryUsage" {
			t.Errorf("Expected HighMemoryUsage notification, got %s", alert.Title)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive notification within timeout")
	}
}

func TestAlertManager_NotificationChannelFull(t *testing.T) {
	// Create alert manager and fill notification channel
	am := NewAlertManager()
	
	// Fill the channel to capacity (100 alerts)
	for i := 0; i < 100; i++ {
		am.notifications <- Alert{
			Title:     "Test Alert",
			Timestamp: time.Now(),
		}
	}
	
	// Now trigger another alert - should not block
	metrics := map[string]*Metric{
		"kvstore_memory_usage_bytes": {
			Name:  "kvstore_memory_usage_bytes",
			Value: 900 * 1024 * 1024,
		},
	}
	
	health := HealthResponse{
		Status: HealthStatusHealthy,
		Checks: make(map[string]HealthCheck),
	}
	
	// This should not block even though channel is full
	done := make(chan bool, 1)
	go func() {
		am.CheckAlerts(metrics, health)
		done <- true
	}()
	
	select {
	case <-done:
		// Good, didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("CheckAlerts should not block when notification channel is full")
	}
}

func TestHealthCheckHistory(t *testing.T) {
	history := HealthCheckHistory{
		Timestamp: time.Now(),
		Status:    HealthStatusHealthy,
		Duration:  50 * time.Millisecond,
	}
	
	if history.Status != HealthStatusHealthy {
		t.Errorf("Expected status healthy, got %s", history.Status)
	}
	
	if history.Duration != 50*time.Millisecond {
		t.Errorf("Expected duration 50ms, got %v", history.Duration)
	}
}

func TestEnhancedHealthResponse(t *testing.T) {
	baseResponse := HealthResponse{
		Status:    HealthStatusHealthy,
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Checks:    make(map[string]HealthCheck),
	}
	
	enhanced := EnhancedHealthResponse{
		HealthResponse: baseResponse,
		LastCheck:      time.Now(),
		CheckHistory:   make([]HealthCheckHistory, 0),
	}
	
	if enhanced.Status != HealthStatusHealthy {
		t.Errorf("Expected status healthy, got %s", enhanced.Status)
	}
	
	if enhanced.CheckHistory == nil {
		t.Error("Expected check history to be initialized")
	}
}

func TestAlertRule_Structure(t *testing.T) {
	rule := AlertRule{
		Name:        "TestRule",
		Condition:   func(metrics map[string]*Metric, health HealthResponse) bool { return true },
		Severity:    "warning",
		Description: "Test rule description",
	}
	
	if rule.Name != "TestRule" {
		t.Errorf("Expected name TestRule, got %s", rule.Name)
	}
	
	if rule.Severity != "warning" {
		t.Errorf("Expected severity warning, got %s", rule.Severity)
	}
	
	if rule.Description != "Test rule description" {
		t.Errorf("Expected description 'Test rule description', got %s", rule.Description)
	}
	
	// Test condition function
	if !rule.Condition(nil, HealthResponse{}) {
		t.Error("Expected condition function to return true")
	}
}