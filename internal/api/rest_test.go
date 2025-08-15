package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"

	"github.com/gorilla/mux"
)

func setupTestRESTHandler(t *testing.T) *RESTHandler {
	cfg := config.DefaultConfig()
	cfg.Storage.InMemory = true

	// Use test logging configuration
	testLogConfig := logging.TestLoggingConfig()
	logger := logging.NewLogger(&testLogConfig)

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

	return NewRESTHandler(storageEngine, logger, nil)
}

func TestRESTHandler_PutKey(t *testing.T) {
	handler := setupTestRESTHandler(t)

	tests := []struct {
		name           string
		key            string
		requestBody    PutRequest
		expectedStatus int
		expectedSuccess bool
	}{
		{
			name: "valid put request",
			key:  "test-key",
			requestBody: PutRequest{
				Value: "test-value",
			},
			expectedStatus: http.StatusOK,
			expectedSuccess: true,
		},
		{
			name: "put with TTL",
			key:  "ttl-key",
			requestBody: PutRequest{
				Value:      "ttl-value",
				TTLSeconds: int64Ptr(3600),
			},
			expectedStatus: http.StatusOK,
			expectedSuccess: true,
		},
		{
			name:           "empty key",
			key:            "",
			requestBody:    PutRequest{Value: "test-value"},
			expectedStatus: http.StatusBadRequest,
			expectedSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", tt.key), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"key": tt.key})

			w := httptest.NewRecorder()
			handler.PutKey(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response PutResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response.Success != tt.expectedSuccess {
					t.Errorf("Expected success %v, got %v", tt.expectedSuccess, response.Success)
				}
			}
		})
	}
}

func TestRESTHandler_GetKey(t *testing.T) {
	handler := setupTestRESTHandler(t)

	// First, put a test key
	testKey := "get-test-key"
	testValue := "get-test-value"
	
	putBody, _ := json.Marshal(PutRequest{Value: testValue})
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", testKey), bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = mux.SetURLVars(putReq, map[string]string{"key": testKey})
	
	w := httptest.NewRecorder()
	handler.PutKey(w, putReq)

	tests := []struct {
		name           string
		key            string
		expectedStatus int
		expectedFound  bool
		expectedValue  string
	}{
		{
			name:           "get existing key",
			key:            testKey,
			expectedStatus: http.StatusOK,
			expectedFound:  true,
			expectedValue:  testValue,
		},
		{
			name:           "get non-existing key",
			key:            "non-existing-key",
			expectedStatus: http.StatusNotFound,
			expectedFound:  false,
		},
		{
			name:           "empty key",
			key:            "",
			expectedStatus: http.StatusBadRequest,
			expectedFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", tt.key), nil)
			req = mux.SetURLVars(req, map[string]string{"key": tt.key})

			w := httptest.NewRecorder()
			handler.GetKey(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response GetResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Found != tt.expectedFound {
				t.Errorf("Expected found %v, got %v", tt.expectedFound, response.Found)
			}

			if tt.expectedFound && response.Value != tt.expectedValue {
				t.Errorf("Expected value %s, got %s", tt.expectedValue, response.Value)
			}
		})
	}
}

func TestRESTHandler_DeleteKey(t *testing.T) {
	handler := setupTestRESTHandler(t)

	// First, put a test key
	testKey := "delete-test-key"
	testValue := "delete-test-value"
	
	putBody, _ := json.Marshal(PutRequest{Value: testValue})
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", testKey), bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = mux.SetURLVars(putReq, map[string]string{"key": testKey})
	
	w := httptest.NewRecorder()
	handler.PutKey(w, putReq)

	tests := []struct {
		name           string
		key            string
		expectedStatus int
		expectedSuccess bool
		expectedExisted bool
	}{
		{
			name:            "delete existing key",
			key:             testKey,
			expectedStatus:  http.StatusOK,
			expectedSuccess: true,
			expectedExisted: true,
		},
		{
			name:            "delete non-existing key",
			key:             "non-existing-key",
			expectedStatus:  http.StatusOK,
			expectedSuccess: true,
			expectedExisted: false,
		},
		{
			name:            "empty key",
			key:             "",
			expectedStatus:  http.StatusBadRequest,
			expectedSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/kv/%s", tt.key), nil)
			req = mux.SetURLVars(req, map[string]string{"key": tt.key})

			w := httptest.NewRecorder()
			handler.DeleteKey(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response DeleteResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success != tt.expectedSuccess {
				t.Errorf("Expected success %v, got %v", tt.expectedSuccess, response.Success)
			}

			if tt.expectedSuccess && response.Existed != tt.expectedExisted {
				t.Errorf("Expected existed %v, got %v", tt.expectedExisted, response.Existed)
			}
		})
	}
}

func TestRESTHandler_ExistsKey(t *testing.T) {
	handler := setupTestRESTHandler(t)

	// First, put a test key
	testKey := "exists-test-key"
	testValue := "exists-test-value"
	
	putBody, _ := json.Marshal(PutRequest{Value: testValue})
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", testKey), bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = mux.SetURLVars(putReq, map[string]string{"key": testKey})
	
	w := httptest.NewRecorder()
	handler.PutKey(w, putReq)

	tests := []struct {
		name           string
		key            string
		expectedStatus int
		expectedExists bool
	}{
		{
			name:           "existing key",
			key:            testKey,
			expectedStatus: http.StatusOK,
			expectedExists: true,
		},
		{
			name:           "non-existing key",
			key:            "non-existing-key",
			expectedStatus: http.StatusNotFound,
			expectedExists: false,
		},
		{
			name:           "empty key",
			key:            "",
			expectedStatus: http.StatusBadRequest,
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/v1/kv/%s", tt.key), nil)
			req = mux.SetURLVars(req, map[string]string{"key": tt.key})

			w := httptest.NewRecorder()
			handler.ExistsKey(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK || tt.expectedStatus == http.StatusNotFound {
				var response ExistsResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response.Exists != tt.expectedExists {
					t.Errorf("Expected exists %v, got %v", tt.expectedExists, response.Exists)
				}
			}
		})
	}
}

func TestRESTHandler_ListKeys(t *testing.T) {
	handler := setupTestRESTHandler(t)

	// Put some test data
	testData := map[string]string{
		"list:key1": "value1",
		"list:key2": "value2",
		"list:key3": "value3",
		"other:key1": "other1",
	}

	for key, value := range testData {
		putBody, _ := json.Marshal(PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}

	tests := []struct {
		name          string
		queryParams   string
		expectedCount int
		expectedStatus int
		keysOnly      bool
	}{
		{
			name:           "list with prefix",
			queryParams:    "prefix=list:",
			expectedCount:  3,
			expectedStatus: http.StatusOK,
			keysOnly:       false,
		},
		{
			name:           "list keys only",
			queryParams:    "prefix=list:&keys_only=true",
			expectedCount:  3,
			expectedStatus: http.StatusOK,
			keysOnly:       true,
		},
		{
			name:           "list with limit",
			queryParams:    "prefix=list:&limit=2",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
			keysOnly:       false,
		},
		{
			name:           "list all",
			queryParams:    "",
			expectedCount:  4,
			expectedStatus: http.StatusOK,
			keysOnly:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/kv"
			if tt.queryParams != "" {
				url += "?" + tt.queryParams
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			handler.ListKeys(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.keysOnly {
				var response ListKeysResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response.Count != tt.expectedCount {
					t.Errorf("Expected count %d, got %d", tt.expectedCount, response.Count)
				}
			} else {
				var response ListResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response.Count != tt.expectedCount {
					t.Errorf("Expected count %d, got %d", tt.expectedCount, response.Count)
				}
			}
		})
	}
}

func TestRESTHandler_BatchPut(t *testing.T) {
	handler := setupTestRESTHandler(t)

	tests := []struct {
		name              string
		request           BatchPutRequest
		expectedStatus    int
		expectedSuccessCount int
		expectedErrorCount   int
	}{
		{
			name: "valid batch put",
			request: BatchPutRequest{
				Items: []BatchPutItem{
					{Key: "batch1", Value: "value1"},
					{Key: "batch2", Value: "value2"},
					{Key: "batch3", Value: "value3"},
				},
			},
			expectedStatus:       http.StatusOK,
			expectedSuccessCount: 3,
			expectedErrorCount:   0,
		},
		{
			name: "batch put with empty key",
			request: BatchPutRequest{
				Items: []BatchPutItem{
					{Key: "batch4", Value: "value4"},
					{Key: "", Value: "invalid"},
					{Key: "batch5", Value: "value5"},
				},
			},
			expectedStatus:       http.StatusPartialContent,
			expectedSuccessCount: 2,
			expectedErrorCount:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/put", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handler.BatchPut(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response BatchResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.SuccessCount != tt.expectedSuccessCount {
				t.Errorf("Expected success count %d, got %d", tt.expectedSuccessCount, response.SuccessCount)
			}

			if response.ErrorCount != tt.expectedErrorCount {
				t.Errorf("Expected error count %d, got %d", tt.expectedErrorCount, response.ErrorCount)
			}
		})
	}
}

func TestRESTHandler_BatchGet(t *testing.T) {
	handler := setupTestRESTHandler(t)

	// Put some test data
	testKeys := []string{"bget1", "bget2", "bget3"}
	for _, key := range testKeys {
		putBody, _ := json.Marshal(PutRequest{Value: "value-" + key})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}

	tests := []struct {
		name           string
		request        BatchGetRequest
		expectedStatus int
		expectedFound  int
	}{
		{
			name: "batch get existing keys",
			request: BatchGetRequest{
				Keys: testKeys,
			},
			expectedStatus: http.StatusOK,
			expectedFound:  3,
		},
		{
			name: "batch get mixed keys",
			request: BatchGetRequest{
				Keys: []string{"bget1", "nonexistent", "bget2"},
			},
			expectedStatus: http.StatusOK,
			expectedFound:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/get", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handler.BatchGet(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response BatchResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			foundCount := 0
			for _, result := range response.Results {
				if result.Found {
					foundCount++
				}
			}

			if foundCount != tt.expectedFound {
				t.Errorf("Expected %d found keys, got %d", tt.expectedFound, foundCount)
			}
		})
	}
}

func TestRESTHandler_Health(t *testing.T) {
	handler := setupTestRESTHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Healthy {
		t.Error("Expected healthy to be true")
	}

	if response.Status != "healthy" {
		t.Errorf("Expected status to be 'healthy', got '%s'", response.Status)
	}

	if response.Version == "" {
		t.Error("Expected version to be non-empty")
	}
}

func TestRESTHandler_Stats(t *testing.T) {
	handler := setupTestRESTHandler(t)

	// Put some test data
	putBody, _ := json.Marshal(PutRequest{Value: "stats-value"})
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/kv/stats-test", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = mux.SetURLVars(putReq, map[string]string{"key": "stats-test"})
	
	w := httptest.NewRecorder()
	handler.PutKey(w, putReq)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectDetails  bool
	}{
		{
			name:           "stats without details",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectDetails:  false,
		},
		{
			name:           "stats with details",
			queryParams:    "details=true",
			expectedStatus: http.StatusOK,
			expectDetails:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/stats"
			if tt.queryParams != "" {
				url += "?" + tt.queryParams
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			handler.Stats(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response StatsResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.TotalSize < 0 {
				t.Error("Expected non-negative total size")
			}

			if tt.expectDetails {
				if len(response.Details) == 0 {
					t.Error("Expected details to be populated when requested")
				}
			} else {
				if len(response.Details) > 0 {
					t.Error("Expected no details when not requested")
				}
			}
		})
	}
}

func TestRESTHandler_Middleware(t *testing.T) {
	handler := setupTestRESTHandler(t)
	router := handler.SetupRoutes()

	// Test CORS middleware
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/kv/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for OPTIONS request, got %d", http.StatusOK, w.Code)
	}

	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != "*" {
		t.Errorf("Expected CORS header '*', got '%s'", corsHeader)
	}
}

func TestRESTHandler_RootHandler(t *testing.T) {
	handler := setupTestRESTHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.RootHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["service"] == nil {
		t.Error("Expected service field in root response")
	}

	if response["endpoints"] == nil {
		t.Error("Expected endpoints field in root response")
	}
}

// Helper function
func int64Ptr(i int64) *int64 {
	return &i
}