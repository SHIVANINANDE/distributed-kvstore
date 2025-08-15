package server

import (
	"context"
	"testing"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/storage"
	"distributed-kvstore/proto/kvstore"

	"github.com/sirupsen/logrus"
)

func setupTestGRPCServer(t *testing.T) (*GRPCServer, func()) {
	cfg := config.DefaultConfig()
	cfg.Storage.InMemory = true

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

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

	server := NewGRPCServer(cfg, storageEngine, logger)

	return server, func() {
		storageEngine.Close()
	}
}

func TestGRPCServer_Put(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	tests := []struct {
		name        string
		request     *kvstore.PutRequest
		expectError bool
	}{
		{
			name: "valid put request",
			request: &kvstore.PutRequest{
				Key:   "test-key",
				Value: []byte("test-value"),
			},
			expectError: false,
		},
		{
			name: "empty key",
			request: &kvstore.PutRequest{
				Key:   "",
				Value: []byte("test-value"),
			},
			expectError: true,
		},
		{
			name: "put with TTL",
			request: &kvstore.PutRequest{
				Key:        "ttl-key",
				Value:      []byte("ttl-value"),
				TtlSeconds: 3600,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			resp, err := server.Put(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if resp.Success {
					t.Error("Expected success to be false")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !resp.Success {
					t.Errorf("Expected success to be true, got false. Error: %s", resp.Error)
				}
			}
		})
	}
}

func TestGRPCServer_Get(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	key := "get-test-key"
	value := []byte("get-test-value")

	_, err := server.Put(ctx, &kvstore.PutRequest{
		Key:   key,
		Value: value,
	})
	if err != nil {
		t.Fatalf("Failed to put test data: %v", err)
	}

	tests := []struct {
		name        string
		request     *kvstore.GetRequest
		expectFound bool
		expectError bool
	}{
		{
			name: "get existing key",
			request: &kvstore.GetRequest{
				Key: key,
			},
			expectFound: true,
			expectError: false,
		},
		{
			name: "get non-existing key",
			request: &kvstore.GetRequest{
				Key: "non-existing-key",
			},
			expectFound: false,
			expectError: false,
		},
		{
			name: "empty key",
			request: &kvstore.GetRequest{
				Key: "",
			},
			expectFound: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.Get(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp.Found != tt.expectFound {
					t.Errorf("Expected found=%v, got found=%v", tt.expectFound, resp.Found)
				}
				if tt.expectFound && string(resp.Value) != string(value) {
					t.Errorf("Expected value=%s, got value=%s", string(value), string(resp.Value))
				}
			}
		})
	}
}

func TestGRPCServer_Delete(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	key := "delete-test-key"
	value := []byte("delete-test-value")

	_, err := server.Put(ctx, &kvstore.PutRequest{
		Key:   key,
		Value: value,
	})
	if err != nil {
		t.Fatalf("Failed to put test data: %v", err)
	}

	tests := []struct {
		name           string
		request        *kvstore.DeleteRequest
		expectSuccess  bool
		expectExisted  bool
		expectError    bool
	}{
		{
			name: "delete existing key",
			request: &kvstore.DeleteRequest{
				Key: key,
			},
			expectSuccess: true,
			expectExisted: true,
			expectError:   false,
		},
		{
			name: "delete non-existing key",
			request: &kvstore.DeleteRequest{
				Key: "non-existing-key",
			},
			expectSuccess: true,
			expectExisted: false,
			expectError:   false,
		},
		{
			name: "empty key",
			request: &kvstore.DeleteRequest{
				Key: "",
			},
			expectSuccess: false,
			expectExisted: false,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.Delete(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp.Success != tt.expectSuccess {
					t.Errorf("Expected success=%v, got success=%v", tt.expectSuccess, resp.Success)
				}
				if resp.Existed != tt.expectExisted {
					t.Errorf("Expected existed=%v, got existed=%v", tt.expectExisted, resp.Existed)
				}
			}
		})
	}
}

func TestGRPCServer_Exists(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	key := "exists-test-key"
	value := []byte("exists-test-value")

	_, err := server.Put(ctx, &kvstore.PutRequest{
		Key:   key,
		Value: value,
	})
	if err != nil {
		t.Fatalf("Failed to put test data: %v", err)
	}

	tests := []struct {
		name         string
		request      *kvstore.ExistsRequest
		expectExists bool
		expectError  bool
	}{
		{
			name: "existing key",
			request: &kvstore.ExistsRequest{
				Key: key,
			},
			expectExists: true,
			expectError:  false,
		},
		{
			name: "non-existing key",
			request: &kvstore.ExistsRequest{
				Key: "non-existing-key",
			},
			expectExists: false,
			expectError:  false,
		},
		{
			name: "empty key",
			request: &kvstore.ExistsRequest{
				Key: "",
			},
			expectExists: false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.Exists(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp.Exists != tt.expectExists {
					t.Errorf("Expected exists=%v, got exists=%v", tt.expectExists, resp.Exists)
				}
			}
		})
	}
}

func TestGRPCServer_List(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	testData := map[string][]byte{
		"prefix:key1": []byte("value1"),
		"prefix:key2": []byte("value2"),
		"prefix:key3": []byte("value3"),
		"other:key1":  []byte("other1"),
	}

	for key, value := range testData {
		_, err := server.Put(ctx, &kvstore.PutRequest{
			Key:   key,
			Value: value,
		})
		if err != nil {
			t.Fatalf("Failed to put test data: %v", err)
		}
	}

	tests := []struct {
		name          string
		request       *kvstore.ListRequest
		expectedCount int
		expectError   bool
	}{
		{
			name: "list with prefix",
			request: &kvstore.ListRequest{
				Prefix: "prefix:",
			},
			expectedCount: 3,
			expectError:   false,
		},
		{
			name: "list with limit",
			request: &kvstore.ListRequest{
				Prefix: "prefix:",
				Limit:  2,
			},
			expectedCount: 2,
			expectError:   false,
		},
		{
			name: "list non-existing prefix",
			request: &kvstore.ListRequest{
				Prefix: "nonexistent:",
			},
			expectedCount: 0,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.List(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(resp.Items) != tt.expectedCount {
					t.Errorf("Expected %d items, got %d", tt.expectedCount, len(resp.Items))
				}
			}
		})
	}
}

func TestGRPCServer_BatchPut(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name                string
		request             *kvstore.BatchPutRequest
		expectedSuccessCount int32
		expectedErrorCount   int32
	}{
		{
			name: "valid batch put",
			request: &kvstore.BatchPutRequest{
				Items: []*kvstore.PutItem{
					{Key: "batch1", Value: []byte("value1")},
					{Key: "batch2", Value: []byte("value2")},
					{Key: "batch3", Value: []byte("value3")},
				},
			},
			expectedSuccessCount: 3,
			expectedErrorCount:   0,
		},
		{
			name: "batch put with empty key",
			request: &kvstore.BatchPutRequest{
				Items: []*kvstore.PutItem{
					{Key: "batch4", Value: []byte("value4")},
					{Key: "", Value: []byte("invalid")},
					{Key: "batch5", Value: []byte("value5")},
				},
			},
			expectedSuccessCount: 2,
			expectedErrorCount:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.BatchPut(ctx, tt.request)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if resp.SuccessCount != tt.expectedSuccessCount {
				t.Errorf("Expected success count=%d, got %d", tt.expectedSuccessCount, resp.SuccessCount)
			}
			if resp.ErrorCount != tt.expectedErrorCount {
				t.Errorf("Expected error count=%d, got %d", tt.expectedErrorCount, resp.ErrorCount)
			}
		})
	}
}

func TestGRPCServer_BatchGet(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	testKeys := []string{"bget1", "bget2", "bget3"}
	for _, key := range testKeys {
		_, err := server.Put(ctx, &kvstore.PutRequest{
			Key:   key,
			Value: []byte("value-" + key),
		})
		if err != nil {
			t.Fatalf("Failed to put test data: %v", err)
		}
	}

	tests := []struct {
		name         string
		request      *kvstore.BatchGetRequest
		expectedFound int
	}{
		{
			name: "batch get existing keys",
			request: &kvstore.BatchGetRequest{
				Keys: testKeys,
			},
			expectedFound: 3,
		},
		{
			name: "batch get mixed keys",
			request: &kvstore.BatchGetRequest{
				Keys: []string{"bget1", "nonexistent", "bget2"},
			},
			expectedFound: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.BatchGet(ctx, tt.request)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			foundCount := 0
			for _, result := range resp.Results {
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

func TestGRPCServer_Health(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	resp, err := server.Health(ctx, &kvstore.HealthRequest{})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !resp.Healthy {
		t.Error("Expected healthy to be true")
	}

	if resp.Status != "healthy" {
		t.Errorf("Expected status to be 'healthy', got '%s'", resp.Status)
	}

	if resp.Version == "" {
		t.Error("Expected version to be non-empty")
	}
}

func TestGRPCServer_Stats(t *testing.T) {
	server, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	_, err := server.Put(ctx, &kvstore.PutRequest{
		Key:   "stats-test",
		Value: []byte("stats-value"),
	})
	if err != nil {
		t.Fatalf("Failed to put test data: %v", err)
	}

	tests := []struct {
		name           string
		request        *kvstore.StatsRequest
		expectDetails  bool
	}{
		{
			name: "stats without details",
			request: &kvstore.StatsRequest{
				IncludeDetails: false,
			},
			expectDetails: false,
		},
		{
			name: "stats with details",
			request: &kvstore.StatsRequest{
				IncludeDetails: true,
			},
			expectDetails: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.Stats(ctx, tt.request)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if resp.TotalSize < 0 {
				t.Error("Expected total size to be non-negative")
			}

			if tt.expectDetails {
				if len(resp.Details) == 0 {
					t.Error("Expected details to be non-empty when requested")
				}
			} else {
				if len(resp.Details) > 0 {
					t.Error("Expected no details when not requested")
				}
			}
		})
	}
}