package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"distributed-kvstore/internal/api"
	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
)

// TestAPIServer represents a test API server instance using httptest
type TestAPIServer struct {
	handler    *api.RESTHandler
	router     http.Handler
	storage    storage.StorageEngine
}

// SetupTestAPIServer creates a test API server using httptest
func SetupTestAPIServer(t *testing.T) *TestAPIServer {
	// Create test config
	cfg := config.DefaultConfig()
	cfg.Storage.InMemory = true

	// Use test logging configuration
	testLogConfig := logging.TestLoggingConfig()
	logger := logging.NewLogger(&testLogConfig)

	// Create in-memory storage
	storageConfig := storage.Config{
		DataPath:   "",
		InMemory:   true,
		SyncWrites: false,
		ValueLogGC: false,
	}

	storageEngine, err := storage.NewEngine(storageConfig)
	if err != nil {
		t.Fatalf("Failed to create storage engine: %v", err)
	}

	t.Cleanup(func() {
		storageEngine.Close()
	})

	// Create REST handler
	handler := api.NewRESTHandler(storageEngine, logger, nil)
	router := handler.SetupRoutes()

	return &TestAPIServer{
		handler: handler,
		router:  router,
		storage: storageEngine,
	}
}

// Helper methods for making API requests using httptest

func (ts *TestAPIServer) PUT(key string, value string) *httptest.ResponseRecorder {
	requestBody := api.PutRequest{Value: value}
	body, _ := json.Marshal(requestBody)
	
	req := httptest.NewRequest(http.MethodPut, "/api/v1/kv/"+key, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) GET(key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kv/"+key, nil)
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) DELETE(key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/kv/"+key, nil)
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) HEAD(key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodHead, "/api/v1/kv/"+key, nil)
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) LIST(queryParams string) *httptest.ResponseRecorder {
	url := "/api/v1/kv"
	if queryParams != "" {
		url += "?" + queryParams
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) BatchPUT(items []api.BatchPutItem) *httptest.ResponseRecorder {
	requestBody := api.BatchPutRequest{Items: items}
	body, _ := json.Marshal(requestBody)
	
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/put", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) BatchGET(keys []string) *httptest.ResponseRecorder {
	requestBody := api.BatchGetRequest{Keys: keys}
	body, _ := json.Marshal(requestBody)
	
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/get", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) Health() *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

func (ts *TestAPIServer) Stats(includeDetails bool) *httptest.ResponseRecorder {
	url := "/api/v1/stats"
	if includeDetails {
		url += "?details=true"
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

// Integration Tests

func TestIntegration_BasicCRUDOperations(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	// Test PUT operation
	w := testServer.PUT("test-key", "test-value")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var putResp api.PutResponse
	if err := json.NewDecoder(w.Body).Decode(&putResp); err != nil {
		t.Fatalf("Failed to decode PUT response: %v", err)
	}

	if !putResp.Success {
		t.Errorf("Expected success=true, got success=%v, error=%s", putResp.Success, putResp.Error)
	}

	// Test GET operation
	w = testServer.GET("test-key")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var getResp api.GetResponse
	if err := json.NewDecoder(w.Body).Decode(&getResp); err != nil {
		t.Fatalf("Failed to decode GET response: %v", err)
	}

	if !getResp.Found {
		t.Error("Expected found=true")
	}

	if getResp.Value != "test-value" {
		t.Errorf("Expected value 'test-value', got '%s'", getResp.Value)
	}

	// Test HEAD operation (EXISTS)
	w = testServer.HEAD("test-key")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for existing key, got %d", w.Code)
	}

	// Test DELETE operation
	w = testServer.DELETE("test-key")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var deleteResp api.DeleteResponse
	if err := json.NewDecoder(w.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("Failed to decode DELETE response: %v", err)
	}

	if !deleteResp.Success {
		t.Errorf("Expected success=true, got success=%v", deleteResp.Success)
	}

	if !deleteResp.Existed {
		t.Error("Expected existed=true")
	}

	// Verify key is deleted
	w = testServer.GET("test-key")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for deleted key, got %d", w.Code)
	}
}

func TestIntegration_BatchOperations(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	// Test batch PUT
	batchItems := []api.BatchPutItem{
		{Key: "batch1", Value: "value1"},
		{Key: "batch2", Value: "value2"},
		{Key: "batch3", Value: "value3"},
	}

	w := testServer.BatchPUT(batchItems)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var batchResp api.BatchResponse
	if err := json.NewDecoder(w.Body).Decode(&batchResp); err != nil {
		t.Fatalf("Failed to decode batch PUT response: %v", err)
	}

	if batchResp.SuccessCount != 3 {
		t.Errorf("Expected success count 3, got %d", batchResp.SuccessCount)
	}

	if batchResp.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", batchResp.ErrorCount)
	}

	// Test batch GET
	keys := []string{"batch1", "batch2", "batch3", "nonexistent"}

	w = testServer.BatchGET(keys)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if err := json.NewDecoder(w.Body).Decode(&batchResp); err != nil {
		t.Fatalf("Failed to decode batch GET response: %v", err)
	}

	if len(batchResp.Results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(batchResp.Results))
	}

	foundCount := 0
	for _, result := range batchResp.Results {
		if result.Found {
			foundCount++
		}
	}

	if foundCount != 3 {
		t.Errorf("Expected 3 found keys, got %d", foundCount)
	}
}

func TestIntegration_ListOperations(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	// Put test data with different prefixes
	testData := map[string]string{
		"list:item1": "value1",
		"list:item2": "value2",
		"list:item3": "value3",
		"other:item1": "other1",
		"other:item2": "other2",
	}

	for key, value := range testData {
		w := testServer.PUT(key, value)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT failed for key %s: status %d", key, w.Code)
		}
	}

	// Test list with prefix
	w := testServer.LIST("prefix=list:")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var listResp api.ListResponse
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode LIST response: %v", err)
	}

	if listResp.Count != 3 {
		t.Errorf("Expected count 3 for prefix 'list:', got %d", listResp.Count)
	}

	// Test list keys only
	w = testServer.LIST("prefix=list:&keys_only=true")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var listKeysResp api.ListKeysResponse
	if err := json.NewDecoder(w.Body).Decode(&listKeysResp); err != nil {
		t.Fatalf("Failed to decode LIST keys response: %v", err)
	}

	if listKeysResp.Count != 3 {
		t.Errorf("Expected count 3 for keys only, got %d", listKeysResp.Count)
	}
}

func TestIntegration_HealthEndpoint(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	w := testServer.Health()

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check response headers
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content-type application/json, got %s", contentType)
	}

	// Check response body structure
	var health api.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	if !health.Healthy {
		t.Error("Expected healthy to be true")
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status to be 'healthy', got '%s'", health.Status)
	}

	if health.Version == "" {
		t.Error("Expected version to be non-empty")
	}
}

func TestIntegration_StatsEndpoint(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	// Put some data first
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("stats-test-%d", i)
		value := fmt.Sprintf("value-%d", i)
		w := testServer.PUT(key, value)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT failed for key %s: status %d", key, w.Code)
		}
	}

	// Test stats without details
	w := testServer.Stats(false)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var statsResp api.StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&statsResp); err != nil {
		t.Fatalf("Failed to decode stats response: %v", err)
	}

	if statsResp.TotalSize < 0 {
		t.Error("Expected non-negative total size")
	}

	if len(statsResp.Details) > 0 {
		t.Error("Expected no details when not requested")
	}

	// Test stats with details
	w = testServer.Stats(true)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if err := json.NewDecoder(w.Body).Decode(&statsResp); err != nil {
		t.Fatalf("Failed to decode stats with details response: %v", err)
	}

	if len(statsResp.Details) == 0 {
		t.Error("Expected details when requested")
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	// Test GET non-existent key
	w := testServer.GET("nonexistent-key")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent key, got %d", w.Code)
	}

	// Test malformed JSON
	req := httptest.NewRequest(http.MethodPut, "/api/v1/kv/test", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	testServer.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}
}

func TestIntegration_ConcurrentOperations(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	const numGoroutines = 5
	const operationsPerGoroutine = 3

	// Channel to collect errors
	errChan := make(chan error, numGoroutines*operationsPerGoroutine)
	doneChan := make(chan bool, numGoroutines)

	// Start concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { doneChan <- true }()

			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("concurrent-%d-%d", goroutineID, j)
				value := fmt.Sprintf("value-%d-%d", goroutineID, j)

				// PUT
				w := testServer.PUT(key, value)
				if w.Code != http.StatusOK {
					errChan <- fmt.Errorf("PUT returned status %d for %s", w.Code, key)
					return
				}

				// GET
				w = testServer.GET(key)
				if w.Code != http.StatusOK {
					errChan <- fmt.Errorf("GET returned status %d for %s", w.Code, key)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-doneChan
	}

	// Check for errors
	close(errChan)
	for err := range errChan {
		t.Error(err)
	}
}

func TestIntegration_WorkflowEndToEnd(t *testing.T) {
	testServer := SetupTestAPIServer(t)

	// Simulate a complete workflow
	
	// 1. Create some initial data
	initialData := map[string]string{
		"user:1":    "Alice",
		"user:2":    "Bob", 
		"user:3":    "Charlie",
		"config:db": "localhost:5432",
		"config:cache": "redis:6379",
	}

	for key, value := range initialData {
		w := testServer.PUT(key, value)
		if w.Code != http.StatusOK {
			t.Fatalf("Failed to PUT %s: status %d", key, w.Code)
		}
	}

	// 2. Batch get all users
	userKeys := []string{"user:1", "user:2", "user:3"}
	w := testServer.BatchGET(userKeys)
	
	if w.Code != http.StatusOK {
		t.Errorf("Batch GET failed: status %d", w.Code)
	}

	var batchResp api.BatchResponse
	if err := json.NewDecoder(w.Body).Decode(&batchResp); err != nil {
		t.Fatalf("Failed to decode batch response: %v", err)
	}

	if len(batchResp.Results) != 3 {
		t.Errorf("Expected 3 batch results, got %d", len(batchResp.Results))
	}

	// 3. List all config entries
	w = testServer.LIST("prefix=config:")
	
	if w.Code != http.StatusOK {
		t.Errorf("List config failed: status %d", w.Code)
	}

	var listResp api.ListResponse
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode list response: %v", err)
	}

	if listResp.Count != 2 {
		t.Errorf("Expected 2 config entries, got %d", listResp.Count)
	}

	// 4. Update a user
	w = testServer.PUT("user:2", "Robert")
	if w.Code != http.StatusOK {
		t.Errorf("Update user failed: status %d", w.Code)
	}

	// 5. Verify the update
	w = testServer.GET("user:2")
	if w.Code != http.StatusOK {
		t.Errorf("Get updated user failed: status %d", w.Code)
	}

	var getResp api.GetResponse
	if err := json.NewDecoder(w.Body).Decode(&getResp); err != nil {
		t.Fatalf("Failed to decode get response: %v", err)
	}

	if getResp.Value != "Robert" {
		t.Errorf("Expected updated value 'Robert', got '%s'", getResp.Value)
	}

	// 6. Delete a config entry
	w = testServer.DELETE("config:cache")
	if w.Code != http.StatusOK {
		t.Errorf("Delete config failed: status %d", w.Code)
	}

	// 7. Verify config was deleted
	w = testServer.LIST("prefix=config:")
	if w.Code != http.StatusOK {
		t.Errorf("List config after delete failed: status %d", w.Code)
	}

	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode list response after delete: %v", err)
	}

	if listResp.Count != 1 {
		t.Errorf("Expected 1 config entry after delete, got %d", listResp.Count)
	}

	// 8. Check health
	w = testServer.Health()
	if w.Code != http.StatusOK {
		t.Errorf("Health check failed: status %d", w.Code)
	}
}