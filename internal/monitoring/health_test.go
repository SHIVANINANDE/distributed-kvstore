package monitoring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestHealthManager_RegisterChecker(t *testing.T) {
	hm := NewHealthManager("test-version")

	checker := &MockHealthChecker{
		name:     "test",
		critical: false,
		result:   HealthCheck{Status: HealthStatusHealthy, Message: "OK"},
	}

	hm.RegisterChecker(checker)

	if len(hm.checkers) != 1 {
		t.Errorf("Expected 1 checker, got %d", len(hm.checkers))
	}
}

func TestHealthManager_CheckHealth(t *testing.T) {
	ctx := context.Background()
	hm := NewHealthManager("test-version")

	// Register healthy checker
	healthyChecker := &MockHealthChecker{
		name:     "healthy",
		critical: true,
		result:   HealthCheck{Status: HealthStatusHealthy, Message: "Healthy"},
	}
	hm.RegisterChecker(healthyChecker)

	// Register degraded checker
	degradedChecker := &MockHealthChecker{
		name:     "degraded",
		critical: false,
		result:   HealthCheck{Status: HealthStatusDegraded, Message: "Degraded"},
	}
	hm.RegisterChecker(degradedChecker)

	result := hm.CheckHealth(ctx)

	// With one critical healthy and one degraded, overall should be degraded
	if result.Status != HealthStatusDegraded {
		t.Errorf("Expected status degraded, got %s", result.Status)
	}

	if len(result.Checks) != 2 {
		t.Errorf("Expected 2 checks, got %d", len(result.Checks))
	}

	if result.Summary.Total != 2 {
		t.Errorf("Expected total 2, got %d", result.Summary.Total)
	}

	if result.Summary.Healthy != 1 {
		t.Errorf("Expected 1 healthy, got %d", result.Summary.Healthy)
	}

	if result.Summary.Degraded != 1 {
		t.Errorf("Expected 1 degraded, got %d", result.Summary.Degraded)
	}
}

func TestHealthManager_CheckHealth_AllUnhealthy(t *testing.T) {
	ctx := context.Background()
	hm := NewHealthManager("test-version")

	// Register unhealthy critical checker
	unhealthyChecker := &MockHealthChecker{
		name:     "unhealthy",
		critical: true,
		result:   HealthCheck{Status: HealthStatusUnhealthy, Message: "Unhealthy"},
	}
	hm.RegisterChecker(unhealthyChecker)

	result := hm.CheckHealth(ctx)

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Expected status unhealthy, got %s", result.Status)
	}

	if result.Summary.Unhealthy != 1 {
		t.Errorf("Expected 1 unhealthy, got %d", result.Summary.Unhealthy)
	}
}

func TestStorageHealthChecker_Name(t *testing.T) {
	mockStorage := &MockStorageEngine{}
	checker := NewStorageHealthChecker(mockStorage)

	if checker.Name() != "storage" {
		t.Errorf("Expected name 'storage', got '%s'", checker.Name())
	}
}

func TestStorageHealthChecker_IsCritical(t *testing.T) {
	mockStorage := &MockStorageEngine{}
	checker := NewStorageHealthChecker(mockStorage)

	if !checker.IsCritical() {
		t.Error("Expected storage check to be critical")
	}
}

func TestStorageHealthChecker_Check_Success(t *testing.T) {
	ctx := context.Background()
	mockStorage := &MockStorageEngine{
		stats: map[string]interface{}{
			"total_size": int64(1024),
			"entries":    int64(10),
		},
	}
	checker := NewStorageHealthChecker(mockStorage)

	result := checker.Check(ctx)

	if result.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s", result.Status)
	}

	if result.Message != "Storage is operational" {
		t.Errorf("Expected 'Storage is operational', got '%s'", result.Message)
	}

	// Check details
	if result.Details == nil {
		t.Fatal("Expected details to be present")
	}

	details := result.Details
	if details["total_size"] != int64(1024) {
		t.Errorf("Expected total_size 1024, got %v", details["total_size"])
	}
}

func TestStorageHealthChecker_Check_Error(t *testing.T) {
	ctx := context.Background()
	mockStorage := &MockStorageEngine{
		shouldError: true,
		err:         errors.New("storage error"),
	}
	checker := NewStorageHealthChecker(mockStorage)

	result := checker.Check(ctx)

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Expected unhealthy status, got %s", result.Status)
	}

	if result.Message != "Storage write failed: storage error" {
		t.Errorf("Expected storage error message, got '%s'", result.Message)
	}
}

func TestMemoryHealthChecker_Name(t *testing.T) {
	checker := NewMemoryHealthChecker(1024)

	if checker.Name() != "memory" {
		t.Errorf("Expected name 'memory', got '%s'", checker.Name())
	}
}

func TestMemoryHealthChecker_IsCritical(t *testing.T) {
	checker := NewMemoryHealthChecker(1024)

	if checker.IsCritical() {
		t.Error("Expected memory check to not be critical")
	}
}

func TestMemoryHealthChecker_Check_Healthy(t *testing.T) {
	ctx := context.Background()
	// Set a very high limit to ensure healthy status
	checker := NewMemoryHealthChecker(10240) // 10GB limit

	result := checker.Check(ctx)

	if result.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s", result.Status)
	}

	if result.Message != "Memory usage is normal" {
		t.Errorf("Expected normal memory message, got '%s'", result.Message)
	}

	// Check details
	if result.Details == nil {
		t.Fatal("Expected details to be present")
	}

	details := result.Details
	if _, exists := details["alloc_mb"]; !exists {
		t.Error("Expected alloc_mb in details")
	}

	if _, exists := details["sys_mb"]; !exists {
		t.Error("Expected sys_mb in details")
	}

	if _, exists := details["num_gc"]; !exists {
		t.Error("Expected num_gc in details")
	}
}

func TestMemoryHealthChecker_Check_Degraded(t *testing.T) {
	ctx := context.Background()
	
	// Get current memory usage first
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	currentMB := m.Alloc / 1024 / 1024
	
	// Set limit to 80% of current usage to trigger degraded status
	limit := currentMB + 1 // Just above current usage
	degradedThreshold := limit * 80 / 100
	
	// Make sure we exceed the 80% threshold
	if currentMB <= degradedThreshold {
		t.Skip("Current memory usage too low to test degraded state")
	}
	
	checker := NewMemoryHealthChecker(limit)
	result := checker.Check(ctx)

	// Should be degraded since current usage is > 80% of limit
	if result.Status != HealthStatusDegraded {
		t.Errorf("Expected degraded status, got %s (current: %dMB, limit: %dMB, threshold: %dMB)", 
			result.Status, currentMB, limit, degradedThreshold)
	}

	expectedMsg := "Memory usage is high"
	if result.Message[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Expected message to start with '%s', got '%s'", expectedMsg, result.Message)
	}
}

func TestGoroutineHealthChecker_Name(t *testing.T) {
	checker := NewGoroutineHealthChecker(1000)

	if checker.Name() != "goroutines" {
		t.Errorf("Expected name 'goroutines', got '%s'", checker.Name())
	}
}

func TestGoroutineHealthChecker_IsCritical(t *testing.T) {
	checker := NewGoroutineHealthChecker(1000)

	if checker.IsCritical() {
		t.Error("Expected goroutine check to not be critical")
	}
}

func TestGoroutineHealthChecker_Check_Healthy(t *testing.T) {
	ctx := context.Background()
	// Set a high limit to ensure healthy status
	checker := NewGoroutineHealthChecker(10000)

	result := checker.Check(ctx)

	if result.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s", result.Status)
	}

	if result.Message != "Goroutine count is normal" {
		t.Errorf("Expected normal goroutine message, got '%s'", result.Message)
	}

	// Check details
	if result.Details == nil {
		t.Fatal("Expected details to be present")
	}

	details := result.Details
	if _, exists := details["count"]; !exists {
		t.Error("Expected count in details")
	}

	if _, exists := details["limit"]; !exists {
		t.Error("Expected limit in details")
	}
}

func TestGoroutineHealthChecker_Check_Degraded(t *testing.T) {
	ctx := context.Background()
	// Set limit high enough that current usage is in degraded range (80-100% of limit)
	currentGoroutines := runtime.NumGoroutine()
	
	// Calculate a limit where current usage falls in 80-100% range
	limit := (currentGoroutines * 100) / 85 // This makes current usage ~85% of limit
	
	checker := NewGoroutineHealthChecker(limit)
	result := checker.Check(ctx)

	if result.Status != HealthStatusDegraded {
		t.Errorf("Expected degraded status, got %s (current: %d, limit: %d)", 
			result.Status, currentGoroutines, limit)
	}

	expectedMsg := "High goroutine count"
	if result.Message[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Expected message to start with '%s', got '%s'", expectedMsg, result.Message)
	}
}

func TestHTTPDependencyHealthChecker_Name(t *testing.T) {
	checker := NewHTTPDependencyChecker("test-service", "http://example.com", 5*time.Second, false)

	if checker.Name() != "test-service" {
		t.Errorf("Expected name 'test-service', got '%s'", checker.Name())
	}
}

func TestHTTPDependencyHealthChecker_IsCritical(t *testing.T) {
	checker := NewHTTPDependencyChecker("test-service", "http://example.com", 5*time.Second, true)

	if !checker.IsCritical() {
		t.Error("Expected critical to be true")
	}

	nonCriticalChecker := NewHTTPDependencyChecker("test-service", "http://example.com", 5*time.Second, false)
	if nonCriticalChecker.IsCritical() {
		t.Error("Expected critical to be false")
	}
}

func TestHTTPDependencyHealthChecker_Check_Success(t *testing.T) {
	ctx := context.Background()
	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	checker := NewHTTPDependencyChecker("test-service", server.URL, 5*time.Second, true)

	result := checker.Check(ctx)

	if result.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s", result.Status)
	}

	if result.Message != "HTTP 200" {
		t.Errorf("Expected 'HTTP 200', got '%s'", result.Message)
	}
}

func TestHTTPDependencyHealthChecker_Check_Failure(t *testing.T) {
	ctx := context.Background()
	// Use a non-existent URL
	checker := NewHTTPDependencyChecker("test-service", "http://non-existent-url.invalid", 1*time.Second, false)

	result := checker.Check(ctx)

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Expected unhealthy status, got %s", result.Status)
	}

	// Message should contain error information
	if result.Message == "HTTP dependency is reachable" {
		t.Error("Expected error message, got success message")
	}
}

func TestHTTPDependencyHealthChecker_Check_BadStatusCode(t *testing.T) {
	ctx := context.Background()
	// Create a test server that returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	checker := NewHTTPDependencyChecker("test-service", server.URL, 5*time.Second, false)

	result := checker.Check(ctx)

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Expected unhealthy status, got %s", result.Status)
	}

	// Should contain status code in message
	if result.Message != "HTTP 500" {
		t.Errorf("Expected 'HTTP 500', got '%s'", result.Message)
	}
}

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected string
	}{
		{HealthStatusHealthy, "healthy"},
		{HealthStatusDegraded, "degraded"},
		{HealthStatusUnhealthy, "unhealthy"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
		}
	}
}

func TestHealthCheck_Duration(t *testing.T) {
	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	end := time.Now()

	result := HealthCheck{
		Timestamp: start,
		Duration:  end.Sub(start),
	}

	if result.Duration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms, got %v", result.Duration)
	}
}

// Mock implementations for testing

type MockHealthChecker struct {
	name     string
	critical bool
	result   HealthCheck
}

func (m *MockHealthChecker) Name() string {
	return m.name
}

func (m *MockHealthChecker) IsCritical() bool {
	return m.critical
}

func (m *MockHealthChecker) Check(ctx context.Context) HealthCheck {
	m.result.Name = m.name
	m.result.Timestamp = time.Now()
	m.result.Duration = time.Millisecond
	return m.result
}

type MockStorageEngine struct {
	shouldError bool
	err         error
	stats       map[string]interface{}
}

func (m *MockStorageEngine) Put(key, value []byte) error {
	if m.shouldError {
		return m.err
	}
	return nil
}

func (m *MockStorageEngine) Get(key []byte) ([]byte, error) {
	if m.shouldError {
		return nil, m.err
	}
	// Return the same value that was "put" - the health checker puts "ok"
	if string(key) == "__health_check__" {
		return []byte("ok"), nil
	}
	return []byte("test-value"), nil
}

func (m *MockStorageEngine) Delete(key []byte) error {
	if m.shouldError {
		return m.err
	}
	return nil
}

func (m *MockStorageEngine) Exists(key []byte) (bool, error) {
	if m.shouldError {
		return false, m.err
	}
	return true, nil
}

func (m *MockStorageEngine) List(prefix []byte) (map[string][]byte, error) {
	if m.shouldError {
		return nil, m.err
	}
	return map[string][]byte{
		"test-key": []byte("test-value"),
	}, nil
}

func (m *MockStorageEngine) Stats() map[string]interface{} {
	if m.stats == nil {
		return map[string]interface{}{
			"total_size": int64(0),
			"entries":    int64(0),
		}
	}
	return m.stats
}

func (m *MockStorageEngine) Close() error {
	return nil
}

func (m *MockStorageEngine) Backup(path string) error {
	if m.shouldError {
		return m.err
	}
	return nil
}

func (m *MockStorageEngine) Restore(path string) error {
	if m.shouldError {
		return m.err
	}
	return nil
}