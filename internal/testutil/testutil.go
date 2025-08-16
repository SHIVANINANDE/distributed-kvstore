package testutil

import (
	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestStorageEngine creates a test storage engine with in-memory configuration
func TestStorageEngine(t *testing.T) *storage.Engine {
	t.Helper()
	
	config := storage.Config{
		DataPath:   "",
		InMemory:   true,
		SyncWrites: false,
		ValueLogGC: false,
	}
	
	engine, err := storage.NewEngine(config)
	if err != nil {
		t.Fatalf("Failed to create test storage engine: %v", err)
	}
	
	t.Cleanup(func() {
		engine.Close()
	})
	
	return engine
}

// TestConfig creates a test configuration
func TestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Storage.InMemory = true
	cfg.Server.Port = 0 // Let the OS choose a free port for testing
	cfg.Metrics.Port = 0
	return cfg
}

// TestLogger creates a test logger with minimal configuration
func TestLogger() *logging.Logger {
	testLogConfig := logging.TestLoggingConfig()
	return logging.NewLogger(&testLogConfig)
}

// GenerateRandomString generates a random string of given length
func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

// GenerateRandomKey generates a random key for testing
func GenerateRandomKey() string {
	return fmt.Sprintf("test-key-%s", GenerateRandomString(8))
}

// GenerateRandomValue generates a random value for testing
func GenerateRandomValue() string {
	return fmt.Sprintf("test-value-%s", GenerateRandomString(16))
}

// PopulateTestData populates storage with test data and returns the data map
func PopulateTestData(t *testing.T, engine storage.StorageEngine, count int) map[string]string {
	t.Helper()
	
	data := make(map[string]string)
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		value := fmt.Sprintf("test-value-%d", i)
		
		err := engine.Put([]byte(key), []byte(value))
		if err != nil {
			t.Fatalf("Failed to put test data: %v", err)
		}
		
		data[key] = value
	}
	
	return data
}

// PopulateTestDataWithPrefix populates storage with test data using a prefix
func PopulateTestDataWithPrefix(t *testing.T, engine storage.StorageEngine, prefix string, count int) map[string]string {
	t.Helper()
	
	data := make(map[string]string)
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("%s-key-%d", prefix, i)
		value := fmt.Sprintf("%s-value-%d", prefix, i)
		
		err := engine.Put([]byte(key), []byte(value))
		if err != nil {
			t.Fatalf("Failed to put test data: %v", err)
		}
		
		data[key] = value
	}
	
	return data
}

// AssertKeyExists verifies that a key exists in storage
func AssertKeyExists(t *testing.T, engine storage.StorageEngine, key string) {
	t.Helper()
	
	exists, err := engine.Exists([]byte(key))
	if err != nil {
		t.Fatalf("Failed to check key existence: %v", err)
	}
	
	if !exists {
		t.Errorf("Expected key %s to exist, but it doesn't", key)
	}
}

// AssertKeyNotExists verifies that a key does not exist in storage
func AssertKeyNotExists(t *testing.T, engine storage.StorageEngine, key string) {
	t.Helper()
	
	exists, err := engine.Exists([]byte(key))
	if err != nil {
		t.Fatalf("Failed to check key existence: %v", err)
	}
	
	if exists {
		t.Errorf("Expected key %s to not exist, but it does", key)
	}
}

// AssertKeyValue verifies that a key has the expected value
func AssertKeyValue(t *testing.T, engine storage.StorageEngine, key, expectedValue string) {
	t.Helper()
	
	value, err := engine.Get([]byte(key))
	if err != nil {
		t.Fatalf("Failed to get key %s: %v", key, err)
	}
	
	if string(value) != expectedValue {
		t.Errorf("Expected key %s to have value %s, got %s", key, expectedValue, string(value))
	}
}

// HTTPTestRecorder creates a new httptest.ResponseRecorder for testing
func HTTPTestRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

// AssertHTTPStatus verifies that the HTTP response has the expected status code
func AssertHTTPStatus(t *testing.T, recorder *httptest.ResponseRecorder, expectedStatus int) {
	t.Helper()
	
	if recorder.Code != expectedStatus {
		t.Errorf("Expected HTTP status %d, got %d", expectedStatus, recorder.Code)
	}
}

// AssertHTTPHeader verifies that the HTTP response has the expected header value
func AssertHTTPHeader(t *testing.T, recorder *httptest.ResponseRecorder, header, expectedValue string) {
	t.Helper()
	
	actualValue := recorder.Header().Get(header)
	if actualValue != expectedValue {
		t.Errorf("Expected header %s to be %s, got %s", header, expectedValue, actualValue)
	}
}

// AssertContains verifies that a string contains a substring
func AssertContains(t *testing.T, str, substr string) {
	t.Helper()
	
	if !strings.Contains(str, substr) {
		t.Errorf("Expected string to contain %s, but it doesn't: %s", substr, str)
	}
}

// AssertNotContains verifies that a string does not contain a substring
func AssertNotContains(t *testing.T, str, substr string) {
	t.Helper()
	
	if strings.Contains(str, substr) {
		t.Errorf("Expected string to not contain %s, but it does: %s", substr, str)
	}
}

// WithTimeout runs a test function with a timeout
func WithTimeout(t *testing.T, timeout time.Duration, fn func()) {
	t.Helper()
	
	done := make(chan bool, 1)
	
	go func() {
		fn()
		done <- true
	}()
	
	select {
	case <-done:
		// Test completed within timeout
	case <-time.After(timeout):
		t.Fatalf("Test timed out after %v", timeout)
	}
}

// TempDir creates a temporary directory for testing
func TempDir(t *testing.T, prefix string) string {
	t.Helper()
	
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	
	return dir
}

// SetupTestEnvironment sets up a complete test environment
func SetupTestEnvironment(t *testing.T) (*storage.Engine, *logging.Logger, *config.Config) {
	t.Helper()
	
	cfg := TestConfig()
	logger := TestLogger()
	engine := TestStorageEngine(t)
	
	return engine, logger, cfg
}

// MockHTTPRequest creates a mock HTTP request for testing
func MockHTTPRequest(method, url string, body string) *http.Request {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
		return httptest.NewRequest(method, url, bodyReader)
	}
	return httptest.NewRequest(method, url, nil)
}

// WaitForCondition waits for a condition to become true with timeout
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, checkInterval time.Duration) {
	t.Helper()
	
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(checkInterval)
	}
	
	t.Fatalf("Condition not met within timeout %v", timeout)
}

// ConcurrentTest runs multiple test functions concurrently
func ConcurrentTest(t *testing.T, concurrency int, testFunc func(int)) {
	t.Helper()
	
	done := make(chan bool, concurrency)
	errors := make(chan error, concurrency)
	
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("goroutine %d panicked: %v", index, r)
				}
				done <- true
			}()
			
			testFunc(index)
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}
	
	// Check for errors
	select {
	case err := <-errors:
		t.Fatalf("Concurrent test failed: %v", err)
	default:
		// No errors
	}
}

// MemoryUsage returns current memory usage stats for testing
func MemoryUsage() (heapAlloc, heapSys, numGC uint64) {
	// Implementation would use runtime.ReadMemStats but keeping simple for now
	return 0, 0, 0
}

// BenchmarkHelper provides utilities for benchmark tests
type BenchmarkHelper struct {
	StartTime time.Time
	EndTime   time.Time
}

// NewBenchmarkHelper creates a new benchmark helper
func NewBenchmarkHelper() *BenchmarkHelper {
	return &BenchmarkHelper{
		StartTime: time.Now(),
	}
}

// End marks the end of the benchmark
func (bh *BenchmarkHelper) End() {
	bh.EndTime = time.Now()
}

// Duration returns the duration of the benchmark
func (bh *BenchmarkHelper) Duration() time.Duration {
	if bh.EndTime.IsZero() {
		return time.Since(bh.StartTime)
	}
	return bh.EndTime.Sub(bh.StartTime)
}

// OperationsPerSecond calculates operations per second for given operation count
func (bh *BenchmarkHelper) OperationsPerSecond(operations int) float64 {
	duration := bh.Duration()
	if duration == 0 {
		return 0
	}
	return float64(operations) / duration.Seconds()
}

// TestDataGenerator generates test data for various scenarios
type TestDataGenerator struct {
	rand *rand.Rand
}

// NewTestDataGenerator creates a new test data generator
func NewTestDataGenerator(seed int64) *TestDataGenerator {
	return &TestDataGenerator{
		rand: rand.New(rand.NewSource(seed)),
	}
}

// GenerateKeyValuePairs generates n key-value pairs
func (tdg *TestDataGenerator) GenerateKeyValuePairs(n int) map[string]string {
	data := make(map[string]string)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key-%d-%s", i, tdg.randomString(8))
		value := fmt.Sprintf("value-%d-%s", i, tdg.randomString(16))
		data[key] = value
	}
	return data
}

// GenerateKeysWithPrefix generates n keys with the given prefix
func (tdg *TestDataGenerator) GenerateKeysWithPrefix(prefix string, n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = fmt.Sprintf("%s-%d-%s", prefix, i, tdg.randomString(8))
	}
	return keys
}

// randomString generates a random string of given length
func (tdg *TestDataGenerator) randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[tdg.rand.Intn(len(charset))]
	}
	return string(result)
}