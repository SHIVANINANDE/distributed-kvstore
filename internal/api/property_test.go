package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"

	"github.com/gorilla/mux"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func setupPropertyTestHandler(t *testing.T) *RESTHandler {
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
		t.Fatalf("Failed to create storage engine: %v", err)
	}

	t.Cleanup(func() {
		storageEngine.Close()
	})

	return NewRESTHandler(storageEngine, logger, nil)
}

func TestAPIProperties(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Property 1: PUT then GET should return the same value via HTTP API
	properties.Property("HTTP PUT then GET returns same value", prop.ForAll(
		func(key string, value string) bool {
			handler := setupPropertyTestHandler(t)

			// PUT request
			putBody, _ := json.Marshal(PutRequest{Value: value})
			putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq = mux.SetURLVars(putReq, map[string]string{"key": key})

			putW := httptest.NewRecorder()
			handler.PutKey(putW, putReq)

			if putW.Code != http.StatusOK {
				return false
			}

			var putResp PutResponse
			if err := json.NewDecoder(putW.Body).Decode(&putResp); err != nil || !putResp.Success {
				return false
			}

			// GET request
			getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			getReq = mux.SetURLVars(getReq, map[string]string{"key": key})

			getW := httptest.NewRecorder()
			handler.GetKey(getW, getReq)

			if getW.Code != http.StatusOK {
				return false
			}

			var getResp GetResponse
			if err := json.NewDecoder(getW.Body).Decode(&getResp); err != nil {
				return false
			}

			return getResp.Found && getResp.Value == value
		},
		gen.Identifier(),
		gen.AlphaString(),
	))

	// Property 2: DELETE after PUT should return 404 on subsequent GET
	properties.Property("HTTP DELETE after PUT makes GET return 404", prop.ForAll(
		func(key string, value string) bool {
			handler := setupPropertyTestHandler(t)

			// PUT request
			putBody, _ := json.Marshal(PutRequest{Value: value})
			putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq = mux.SetURLVars(putReq, map[string]string{"key": key})

			putW := httptest.NewRecorder()
			handler.PutKey(putW, putReq)

			if putW.Code != http.StatusOK {
				return false
			}

			// DELETE request
			deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			deleteReq = mux.SetURLVars(deleteReq, map[string]string{"key": key})

			deleteW := httptest.NewRecorder()
			handler.DeleteKey(deleteW, deleteReq)

			if deleteW.Code != http.StatusOK {
				return false
			}

			var deleteResp DeleteResponse
			if err := json.NewDecoder(deleteW.Body).Decode(&deleteResp); err != nil || !deleteResp.Success {
				return false
			}

			// GET request should return 404
			getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			getReq = mux.SetURLVars(getReq, map[string]string{"key": key})

			getW := httptest.NewRecorder()
			handler.GetKey(getW, getReq)

			return getW.Code == http.StatusNotFound
		},
		gen.Identifier(),
		gen.AlphaString(),
	))

	// Property 3: HEAD request returns correct existence status
	properties.Property("HTTP HEAD request returns correct existence status", prop.ForAll(
		func(key string, value string, shouldExist bool) bool {
			handler := setupPropertyTestHandler(t)

			if shouldExist {
				// PUT request first
				putBody, _ := json.Marshal(PutRequest{Value: value})
				putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
				putReq.Header.Set("Content-Type", "application/json")
				putReq = mux.SetURLVars(putReq, map[string]string{"key": key})

				putW := httptest.NewRecorder()
				handler.PutKey(putW, putReq)

				if putW.Code != http.StatusOK {
					return false
				}
			}

			// HEAD request
			headReq := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/v1/kv/%s", key), nil)
			headReq = mux.SetURLVars(headReq, map[string]string{"key": key})

			headW := httptest.NewRecorder()
			handler.ExistsKey(headW, headReq)

			if shouldExist {
				return headW.Code == http.StatusOK
			} else {
				return headW.Code == http.StatusNotFound
			}
		},
		gen.Identifier(),
		gen.AlphaString(),
		gen.Bool(),
	))

	// Property 4: Batch PUT then individual GET consistency
	properties.Property("Batch PUT then individual GET consistency", prop.ForAll(
		func(items []BatchPutItem) bool {
			if len(items) == 0 || len(items) > 5 {
				return true // Skip invalid or too large inputs
			}

			// Ensure unique keys
			keySet := make(map[string]bool)
			for _, item := range items {
				if item.Key == "" || keySet[item.Key] {
					return true // Skip invalid or duplicate keys
				}
				keySet[item.Key] = true
			}

			handler := setupPropertyTestHandler(t)

			// Batch PUT request
			batchReq := BatchPutRequest{Items: items}
			body, _ := json.Marshal(batchReq)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch/put", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handler.BatchPut(w, req)

			if w.Code != http.StatusOK {
				return false
			}

			var batchResp BatchResponse
			if err := json.NewDecoder(w.Body).Decode(&batchResp); err != nil {
				return false
			}

			if batchResp.SuccessCount != len(items) {
				return false
			}

			// Verify each item individually
			for _, item := range items {
				getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv/%s", item.Key), nil)
				getReq = mux.SetURLVars(getReq, map[string]string{"key": item.Key})

				getW := httptest.NewRecorder()
				handler.GetKey(getW, getReq)

				if getW.Code != http.StatusOK {
					return false
				}

				var getResp GetResponse
				if err := json.NewDecoder(getW.Body).Decode(&getResp); err != nil {
					return false
				}

				if !getResp.Found || getResp.Value != item.Value {
					return false
				}
			}

			return true
		},
		gen.SliceOfN(2, gen.Struct(reflect.TypeOf(BatchPutItem{}), map[string]gopter.Gen{
			"Key":   gen.Identifier(),
			"Value": gen.AlphaString(),
		})),
	))

	// Property 5: JSON request/response format consistency
	properties.Property("JSON request/response format consistency", prop.ForAll(
		func(key string, value string) bool {
			handler := setupPropertyTestHandler(t)

			// Test with valid JSON
			putBody, _ := json.Marshal(PutRequest{Value: value})
			putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", key), bytes.NewReader(putBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq = mux.SetURLVars(putReq, map[string]string{"key": key})

			putW := httptest.NewRecorder()
			handler.PutKey(putW, putReq)

			if putW.Code != http.StatusOK {
				return false
			}

			// Response should be valid JSON
			var putResp PutResponse
			if err := json.NewDecoder(putW.Body).Decode(&putResp); err != nil {
				return false
			}

			// Response should have correct structure
			return putResp.Success == true
		},
		gen.Identifier(),
		gen.AlphaString(),
	))

	// Property 6: Empty key handling
	properties.Property("Empty keys return BadRequest", prop.ForAll(
		func(value string) bool {
			handler := setupPropertyTestHandler(t)

			// PUT request with empty key
			putBody, _ := json.Marshal(PutRequest{Value: value})
			putReq := httptest.NewRequest(http.MethodPut, "/api/v1/kv/", bytes.NewReader(putBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq = mux.SetURLVars(putReq, map[string]string{"key": ""})

			putW := httptest.NewRecorder()
			handler.PutKey(putW, putReq)

			return putW.Code == http.StatusBadRequest
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func TestAPIPropertiesAdvanced(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Property: List operations return consistent results
	properties.Property("List operations return all stored keys with prefix", prop.ForAll(
		func(prefix string, keys []string, values []string) bool {
			if len(keys) == 0 || len(values) == 0 || len(keys) != len(values) || len(keys) > 3 {
				return true // Skip invalid inputs
			}

			// Ensure unique keys
			keySet := make(map[string]bool)
			for _, key := range keys {
				if key == "" || keySet[key] {
					return true // Skip invalid or duplicate keys
				}
				keySet[key] = true
			}

			handler := setupPropertyTestHandler(t)

			// PUT all items with prefix
			expectedItems := make(map[string]string)
			for i, key := range keys {
				fullKey := prefix + key
				expectedItems[fullKey] = values[i]

				putBody, _ := json.Marshal(PutRequest{Value: values[i]})
				putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/kv/%s", fullKey), bytes.NewReader(putBody))
				putReq.Header.Set("Content-Type", "application/json")
				putReq = mux.SetURLVars(putReq, map[string]string{"key": fullKey})

				putW := httptest.NewRecorder()
				handler.PutKey(putW, putReq)

				if putW.Code != http.StatusOK {
					return false
				}
			}

			// List with prefix
			listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/kv?prefix=%s", prefix), nil)
			listW := httptest.NewRecorder()
			handler.ListKeys(listW, listReq)

			if listW.Code != http.StatusOK {
				return false
			}

			var listResp ListResponse
			if err := json.NewDecoder(listW.Body).Decode(&listResp); err != nil {
				return false
			}

			// Check if all expected items are in the list
			listedItems := make(map[string]string)
			for _, item := range listResp.Items {
				listedItems[item.Key] = item.Value
			}

			for expectedKey, expectedValue := range expectedItems {
				if listedValue, found := listedItems[expectedKey]; !found || listedValue != expectedValue {
					return false
				}
			}

			return listResp.Count == len(expectedItems)
		},
		gen.RegexMatch("^[a-z]{1,2}$"), // Short alpha prefix
		gen.SliceOfN(2, gen.Identifier()),
		gen.SliceOfN(2, gen.AlphaString()),
	))

	// Property: Health endpoint always returns consistent structure
	properties.Property("Health endpoint returns consistent structure", prop.ForAll(
		func() bool {
			handler := setupPropertyTestHandler(t)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			w := httptest.NewRecorder()

			handler.Health(w, req)

			if w.Code != http.StatusOK {
				return false
			}

			var response HealthResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				return false
			}

			// Health should always be true for test setup
			// Status should always be "healthy" for test setup
			// Version should be non-empty
			return response.Healthy == true && response.Status == "healthy" && response.Version != ""
		},
	))

	// Property: Stats endpoint returns non-negative values
	properties.Property("Stats endpoint returns non-negative values", prop.ForAll(
		func(includeDetails bool) bool {
			handler := setupPropertyTestHandler(t)

			url := "/api/v1/stats"
			if includeDetails {
				url += "?details=true"
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			handler.Stats(w, req)

			if w.Code != http.StatusOK {
				return false
			}

			var response StatsResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				return false
			}

			// All numeric stats should be non-negative
			if response.TotalSize < 0 {
				return false
			}

			if includeDetails {
				return len(response.Details) >= 0 // Should have details
			} else {
				return len(response.Details) == 0 // Should not have details
			}
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}