package testutil

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTestStorageEngine(t *testing.T) {
	engine := TestStorageEngine(t)
	
	// Test basic functionality
	key := "test-key"
	value := "test-value"
	
	err := engine.Put([]byte(key), []byte(value))
	if err != nil {
		t.Fatalf("Failed to put data: %v", err)
	}
	
	retrievedValue, err := engine.Get([]byte(key))
	if err != nil {
		t.Fatalf("Failed to get data: %v", err)
	}
	
	if string(retrievedValue) != value {
		t.Errorf("Expected %s, got %s", value, string(retrievedValue))
	}
}

func TestTestConfig(t *testing.T) {
	cfg := TestConfig()
	
	if !cfg.Storage.InMemory {
		t.Error("Expected test config to use in-memory storage")
	}
	
	if cfg.Server.Port != 0 {
		t.Error("Expected test config to use port 0")
	}
}

func TestTestLogger(t *testing.T) {
	logger := TestLogger()
	
	if logger == nil {
		t.Fatal("Expected logger to be created")
	}
	
	// Test basic logging functionality
	logger.InfoContext(context.Background(), "test message")
}

func TestGenerateRandomString(t *testing.T) {
	length := 10
	str1 := GenerateRandomString(length)
	str2 := GenerateRandomString(length)
	
	if len(str1) != length {
		t.Errorf("Expected string length %d, got %d", length, len(str1))
	}
	
	if len(str2) != length {
		t.Errorf("Expected string length %d, got %d", length, len(str2))
	}
	
	// Strings should be different (with high probability)
	if str1 == str2 {
		t.Error("Generated strings should be different")
	}
}

func TestGenerateRandomKey(t *testing.T) {
	key1 := GenerateRandomKey()
	key2 := GenerateRandomKey()
	
	if !strings.HasPrefix(key1, "test-key-") {
		t.Errorf("Expected key to start with 'test-key-', got %s", key1)
	}
	
	if key1 == key2 {
		t.Error("Generated keys should be different")
	}
}

func TestGenerateRandomValue(t *testing.T) {
	value1 := GenerateRandomValue()
	value2 := GenerateRandomValue()
	
	if !strings.HasPrefix(value1, "test-value-") {
		t.Errorf("Expected value to start with 'test-value-', got %s", value1)
	}
	
	if value1 == value2 {
		t.Error("Generated values should be different")
	}
}

func TestPopulateTestData(t *testing.T) {
	engine := TestStorageEngine(t)
	count := 5
	
	data := PopulateTestData(t, engine, count)
	
	if len(data) != count {
		t.Errorf("Expected %d items, got %d", count, len(data))
	}
	
	// Verify data was actually stored
	for key, expectedValue := range data {
		value, err := engine.Get([]byte(key))
		if err != nil {
			t.Fatalf("Failed to get key %s: %v", key, err)
		}
		
		if string(value) != expectedValue {
			t.Errorf("Expected value %s for key %s, got %s", expectedValue, key, string(value))
		}
	}
}

func TestPopulateTestDataWithPrefix(t *testing.T) {
	engine := TestStorageEngine(t)
	prefix := "user"
	count := 3
	
	data := PopulateTestDataWithPrefix(t, engine, prefix, count)
	
	if len(data) != count {
		t.Errorf("Expected %d items, got %d", count, len(data))
	}
	
	// Verify all keys have the prefix
	for key := range data {
		if !strings.HasPrefix(key, prefix+"-key-") {
			t.Errorf("Expected key to have prefix %s, got %s", prefix, key)
		}
	}
}

func TestAssertKeyExists(t *testing.T) {
	engine := TestStorageEngine(t)
	key := "existing-key"
	value := "test-value"
	
	// Put the key first
	err := engine.Put([]byte(key), []byte(value))
	if err != nil {
		t.Fatalf("Failed to put key: %v", err)
	}
	
	// This should not fail
	AssertKeyExists(t, engine, key)
}

func TestAssertKeyNotExists(t *testing.T) {
	engine := TestStorageEngine(t)
	key := "non-existing-key"
	
	// This should not fail
	AssertKeyNotExists(t, engine, key)
}

func TestAssertKeyValue(t *testing.T) {
	engine := TestStorageEngine(t)
	key := "test-key"
	value := "test-value"
	
	// Put the key first
	err := engine.Put([]byte(key), []byte(value))
	if err != nil {
		t.Fatalf("Failed to put key: %v", err)
	}
	
	// This should not fail
	AssertKeyValue(t, engine, key, value)
}

func TestHTTPTestRecorder(t *testing.T) {
	recorder := HTTPTestRecorder()
	
	if recorder == nil {
		t.Fatal("Expected recorder to be created")
	}
	
	recorder.WriteHeader(http.StatusOK)
	recorder.Write([]byte("test response"))
	
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
	}
	
	if recorder.Body.String() != "test response" {
		t.Errorf("Expected 'test response', got %s", recorder.Body.String())
	}
}

func TestAssertHTTPStatus(t *testing.T) {
	recorder := HTTPTestRecorder()
	recorder.WriteHeader(http.StatusOK)
	
	// This should not fail
	AssertHTTPStatus(t, recorder, http.StatusOK)
}

func TestAssertHTTPHeader(t *testing.T) {
	recorder := HTTPTestRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	
	// This should not fail
	AssertHTTPHeader(t, recorder, "Content-Type", "application/json")
}

func TestAssertContains(t *testing.T) {
	str := "Hello, World!"
	substr := "World"
	
	// This should not fail
	AssertContains(t, str, substr)
}

func TestAssertNotContains(t *testing.T) {
	str := "Hello, World!"
	substr := "Universe"
	
	// This should not fail
	AssertNotContains(t, str, substr)
}

func TestWithTimeout(t *testing.T) {
	// Test successful completion within timeout
	WithTimeout(t, time.Second, func() {
		time.Sleep(100 * time.Millisecond)
	})
}

func TestTempDir(t *testing.T) {
	dir := TempDir(t, "test-prefix")
	
	if dir == "" {
		t.Fatal("Expected temp directory to be created")
	}
	
	if !strings.Contains(dir, "test-prefix") {
		t.Errorf("Expected directory name to contain prefix, got %s", dir)
	}
}

func TestSetupTestEnvironment(t *testing.T) {
	engine, logger, cfg := SetupTestEnvironment(t)
	
	if engine == nil {
		t.Fatal("Expected storage engine to be created")
	}
	
	if logger == nil {
		t.Fatal("Expected logger to be created")
	}
	
	if cfg == nil {
		t.Fatal("Expected config to be created")
	}
}

func TestMockHTTPRequest(t *testing.T) {
	req := MockHTTPRequest("GET", "/test", "")
	
	if req.Method != "GET" {
		t.Errorf("Expected method GET, got %s", req.Method)
	}
	
	if req.URL.Path != "/test" {
		t.Errorf("Expected path /test, got %s", req.URL.Path)
	}
	
	// Test with body
	reqWithBody := MockHTTPRequest("POST", "/test", "test body")
	if reqWithBody.Method != "POST" {
		t.Errorf("Expected method POST, got %s", reqWithBody.Method)
	}
}

func TestWaitForCondition(t *testing.T) {
	start := time.Now()
	counter := 0
	
	WaitForCondition(t, func() bool {
		counter++
		return counter >= 3
	}, time.Second, 10*time.Millisecond)
	
	if counter < 3 {
		t.Errorf("Expected counter to be at least 3, got %d", counter)
	}
	
	if time.Since(start) > time.Second {
		t.Error("Wait took too long")
	}
}

func TestConcurrentTest(t *testing.T) {
	concurrency := 5
	results := make([]int, concurrency)
	
	ConcurrentTest(t, concurrency, func(index int) {
		results[index] = index * 2
		time.Sleep(10 * time.Millisecond) // Simulate some work
	})
	
	// Verify all goroutines executed
	for i := 0; i < concurrency; i++ {
		if results[i] != i*2 {
			t.Errorf("Expected result[%d] to be %d, got %d", i, i*2, results[i])
		}
	}
}

func TestBenchmarkHelper(t *testing.T) {
	bh := NewBenchmarkHelper()
	
	if bh.StartTime.IsZero() {
		t.Error("Expected start time to be set")
	}
	
	time.Sleep(10 * time.Millisecond)
	bh.End()
	
	if bh.EndTime.IsZero() {
		t.Error("Expected end time to be set")
	}
	
	duration := bh.Duration()
	if duration <= 0 {
		t.Error("Expected positive duration")
	}
	
	ops := bh.OperationsPerSecond(100)
	if ops <= 0 {
		t.Error("Expected positive operations per second")
	}
}

func TestTestDataGenerator(t *testing.T) {
	tdg := NewTestDataGenerator(123) // Fixed seed for reproducible tests
	
	// Test key-value pair generation
	data := tdg.GenerateKeyValuePairs(5)
	if len(data) != 5 {
		t.Errorf("Expected 5 key-value pairs, got %d", len(data))
	}
	
	// Test prefix key generation
	keys := tdg.GenerateKeysWithPrefix("user", 3)
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}
	
	for _, key := range keys {
		if !strings.HasPrefix(key, "user-") {
			t.Errorf("Expected key to start with 'user-', got %s", key)
		}
	}
}