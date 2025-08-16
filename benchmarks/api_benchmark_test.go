package benchmarks

import (
	"bytes"
	"distributed-kvstore/internal/api"
	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
	"distributed-kvstore/internal/testutil"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// API Benchmarks

func BenchmarkAPI_PUT(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-generate test data
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("api-put-key-%d", i)
		value := fmt.Sprintf("api-put-value-%d", i)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"key": key})
		
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.PutKey(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("PUT failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_GET(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("api-get-key-%d", i)
		value := fmt.Sprintf("api-get-value-%d", i)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}
	
	// Pre-generate GET requests
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("api-get-key-%d", i%numKeys)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", key), nil)
		req = mux.SetURLVars(req, map[string]string{"key": key})
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.GetKey(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("GET failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_DELETE(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data and create DELETE requests
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("api-delete-key-%d", i)
		value := fmt.Sprintf("api-delete-value-%d", i)
		
		// PUT first
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
		
		// Create DELETE request
		deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/kv/%s", key), nil)
		deleteReq = mux.SetURLVars(deleteReq, map[string]string{"key": key})
		requests[i] = deleteReq
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.DeleteKey(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("DELETE failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_EXISTS(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("api-exists-key-%d", i)
		value := fmt.Sprintf("api-exists-value-%d", i)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}
	
	// Pre-generate HEAD requests
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("api-exists-key-%d", i%numKeys)
		req := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/v1/kv/%s", key), nil)
		req = mux.SetURLVars(req, map[string]string{"key": key})
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ExistsKey(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("EXISTS failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_LIST(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data
	prefix := "api-list-benchmark"
	numKeys := 1000
	
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("%s-key-%d", prefix, i)
		value := fmt.Sprintf("api-list-value-%d", i)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}
	
	// Create LIST request
	listURL := fmt.Sprintf("/api/v1/kv?prefix=%s", prefix)
	req := httptest.NewRequest(http.MethodGet, listURL, nil)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ListKeys(w, req)
		
		if w.Code != http.StatusOK {
			b.Fatalf("LIST failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_BatchPUT(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	batchSize := 100
	
	// Pre-generate batch requests
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		items := make([]api.BatchPutItem, batchSize)
		for j := 0; j < batchSize; j++ {
			items[j] = api.BatchPutItem{
				Key:   fmt.Sprintf("batch-put-key-%d-%d", i, j),
				Value: fmt.Sprintf("batch-put-value-%d-%d", i, j),
			}
		}
		
		batchReq := api.BatchPutRequest{Items: items}
		body, _ := json.Marshal(batchReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/put", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.BatchPut(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("Batch PUT failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_BatchGET(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data
	batchSize := 100
	numBatches := 100
	
	for i := 0; i < numBatches; i++ {
		for j := 0; j < batchSize; j++ {
			key := fmt.Sprintf("batch-get-key-%d-%d", i, j)
			value := fmt.Sprintf("batch-get-value-%d-%d", i, j)
			
			putBody, _ := json.Marshal(api.PutRequest{Value: value})
			putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
			
			w := httptest.NewRecorder()
			handler.PutKey(w, putReq)
		}
	}
	
	// Pre-generate batch GET requests
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		keys := make([]string, batchSize)
		batchIndex := i % numBatches
		for j := 0; j < batchSize; j++ {
			keys[j] = fmt.Sprintf("batch-get-key-%d-%d", batchIndex, j)
		}
		
		batchReq := api.BatchGetRequest{Keys: keys}
		body, _ := json.Marshal(batchReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/get", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.BatchGet(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("Batch GET failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_Health(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.Health(w, req)
		
		if w.Code != http.StatusOK {
			b.Fatalf("Health failed with status: %d", w.Code)
		}
	}
}

func BenchmarkAPI_Stats(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.Stats(w, req)
		
		if w.Code != http.StatusOK {
			b.Fatalf("Stats failed with status: %d", w.Code)
		}
	}
}

// Different value sizes for API operations
func BenchmarkAPI_PUT_SmallValues(b *testing.B) {
	benchmarkAPIPutWithValueSize(b, 10)
}

func BenchmarkAPI_PUT_MediumValues(b *testing.B) {
	benchmarkAPIPutWithValueSize(b, 1000)
}

func BenchmarkAPI_PUT_LargeValues(b *testing.B) {
	benchmarkAPIPutWithValueSize(b, 10000)
}

func BenchmarkAPI_GET_SmallValues(b *testing.B) {
	benchmarkAPIGetWithValueSize(b, 10)
}

func BenchmarkAPI_GET_MediumValues(b *testing.B) {
	benchmarkAPIGetWithValueSize(b, 1000)
}

func BenchmarkAPI_GET_LargeValues(b *testing.B) {
	benchmarkAPIGetWithValueSize(b, 10000)
}

// Concurrent API operations
func BenchmarkAPI_PUT_Concurrent(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent-api-put-key-%d", i)
			value := fmt.Sprintf("concurrent-api-put-value-%d", i)
			
			putBody, _ := json.Marshal(api.PutRequest{Value: value})
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"key": key})
			
			w := httptest.NewRecorder()
			handler.PutKey(w, req)
			
			if w.Code != http.StatusOK {
				b.Fatalf("Concurrent PUT failed with status: %d", w.Code)
			}
			i++
		}
	})
}

func BenchmarkAPI_GET_Concurrent(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("concurrent-api-get-key-%d", i)
		value := fmt.Sprintf("concurrent-api-get-value-%d", i)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent-api-get-key-%d", i%numKeys)
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			req = mux.SetURLVars(req, map[string]string{"key": key})
			
			w := httptest.NewRecorder()
			handler.GetKey(w, req)
			
			if w.Code != http.StatusOK {
				b.Fatalf("Concurrent GET failed with status: %d", w.Code)
			}
			i++
		}
	})
}

// Mixed API operations
func BenchmarkAPI_Mixed_Operations(b *testing.B) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with some data
	numInitialKeys := 5000
	for i := 0; i < numInitialKeys; i++ {
		key := fmt.Sprintf("api-mixed-initial-key-%d", i)
		value := fmt.Sprintf("api-mixed-initial-value-%d", i)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		operation := i % 3
		keyNum := i % numInitialKeys
		
		switch operation {
		case 0: // PUT
			key := fmt.Sprintf("api-mixed-put-key-%d", i)
			value := fmt.Sprintf("api-mixed-put-value-%d", i)
			
			putBody, _ := json.Marshal(api.PutRequest{Value: value})
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"key": key})
			
			w := httptest.NewRecorder()
			handler.PutKey(w, req)
			
			if w.Code != http.StatusOK {
				b.Fatalf("Mixed PUT failed with status: %d", w.Code)
			}
			
		case 1: // GET
			key := fmt.Sprintf("api-mixed-initial-key-%d", keyNum)
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			req = mux.SetURLVars(req, map[string]string{"key": key})
			
			w := httptest.NewRecorder()
			handler.GetKey(w, req)
			
			if w.Code != http.StatusOK {
				b.Fatalf("Mixed GET failed with status: %d", w.Code)
			}
			
		case 2: // EXISTS
			key := fmt.Sprintf("api-mixed-initial-key-%d", keyNum)
			req := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			req = mux.SetURLVars(req, map[string]string{"key": key})
			
			w := httptest.NewRecorder()
			handler.ExistsKey(w, req)
			
			if w.Code != http.StatusOK {
				b.Fatalf("Mixed EXISTS failed with status: %d", w.Code)
			}
		}
	}
}

// Helper functions

func setupBenchmarkAPI(b *testing.B) *api.RESTHandler {
	b.Helper()
	
	cfg := config.DefaultConfig()
	cfg.Storage.InMemory = true
	
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
		b.Fatalf("Failed to create benchmark storage engine: %v", err)
	}
	
	b.Cleanup(func() {
		storageEngine.Close()
	})
	
	return api.NewRESTHandler(storageEngine, logger, nil)
}

func benchmarkAPIPutWithValueSize(b *testing.B, valueSize int) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-generate test data
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("api-size-put-key-%d", i)
		value := testutil.GenerateRandomString(valueSize)
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"key": key})
		
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.PutKey(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("PUT failed with status: %d", w.Code)
		}
	}
}

func benchmarkAPIGetWithValueSize(b *testing.B, valueSize int) {
	handler := setupBenchmarkAPI(b)
	
	// Pre-populate with test data
	numKeys := 10000
	keys := make([]string, numKeys)
	
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("api-size-get-key-%d", i)
		value := testutil.GenerateRandomString(valueSize)
		keys[i] = key
		
		putBody, _ := json.Marshal(api.PutRequest{Value: value})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq = mux.SetURLVars(putReq, map[string]string{"key": key})
		
		w := httptest.NewRecorder()
		handler.PutKey(w, putReq)
	}
	
	// Pre-generate GET requests
	requests := make([]*http.Request, b.N)
	for i := 0; i < b.N; i++ {
		key := keys[i%numKeys]
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", key), nil)
		req = mux.SetURLVars(req, map[string]string{"key": key})
		requests[i] = req
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.GetKey(w, requests[i])
		
		if w.Code != http.StatusOK {
			b.Fatalf("GET failed with status: %d", w.Code)
		}
	}
}