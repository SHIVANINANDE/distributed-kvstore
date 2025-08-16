package server

import (
	"context"
	"net"
	"strings"
	"testing"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
	"distributed-kvstore/proto/kvstore"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func setupTestGRPCServer(t *testing.T) (*GRPCServer, *grpc.ClientConn) {
	t.Helper()

	// Create test config
	cfg := config.DefaultConfig()
	cfg.Storage.InMemory = true

	// Create test logger
	testLogConfig := logging.TestLoggingConfig()
	logger := logging.NewLogger(&testLogConfig)

	// Create test storage
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

	// Create gRPC server with in-memory listener
	server := NewGRPCServer(cfg, storageEngine, logger)

	// Use bufconn for testing
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	kvstore.RegisterKVStoreServer(grpcServer, server)

	go func() {
		grpcServer.Serve(lis)
	}()

	t.Cleanup(func() {
		grpcServer.Stop()
	})

	// Create client connection
	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}

	t.Cleanup(func() {
		conn.Close()
	})

	return server, conn
}

func TestGRPCServer_PutGet(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// Test PUT
	putResp, err := client.Put(ctx, &kvstore.PutRequest{
		Key:   "test-key",
		Value: []byte("test-value"),
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if !putResp.Success {
		t.Errorf("Expected Put success=true, got success=%v, error=%s", putResp.Success, putResp.Error)
	}

	// Test GET
	getResp, err := client.Get(ctx, &kvstore.GetRequest{
		Key: "test-key",
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !getResp.Found {
		t.Error("Expected Get found=true")
	}

	if string(getResp.Value) != "test-value" {
		t.Errorf("Expected value 'test-value', got '%s'", string(getResp.Value))
	}
}

func TestGRPCServer_Delete(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// First PUT
	_, err := client.Put(ctx, &kvstore.PutRequest{
		Key:   "delete-test",
		Value: []byte("delete-value"),
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test DELETE
	deleteResp, err := client.Delete(ctx, &kvstore.DeleteRequest{
		Key: "delete-test",
	})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if !deleteResp.Success {
		t.Errorf("Expected Delete success=true, got success=%v", deleteResp.Success)
	}

	if !deleteResp.Existed {
		t.Error("Expected Delete existed=true")
	}

	// Verify key is deleted
	getResp, err := client.Get(ctx, &kvstore.GetRequest{
		Key: "delete-test",
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if getResp.Found {
		t.Error("Expected Get found=false after delete")
	}
}

func TestGRPCServer_Exists(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// Test non-existent key
	existsResp, err := client.Exists(ctx, &kvstore.ExistsRequest{
		Key: "non-existent",
	})
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if existsResp.Exists {
		t.Error("Expected Exists=false for non-existent key")
	}

	// PUT a key
	_, err = client.Put(ctx, &kvstore.PutRequest{
		Key:   "exists-test",
		Value: []byte("exists-value"),
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test existing key
	existsResp, err = client.Exists(ctx, &kvstore.ExistsRequest{
		Key: "exists-test",
	})
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if !existsResp.Exists {
		t.Error("Expected Exists=true for existing key")
	}
}

func TestGRPCServer_List(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// PUT test data with prefix
	testData := map[string]string{
		"list:item1": "value1",
		"list:item2": "value2",
		"list:item3": "value3",
		"other:item": "other",
	}

	for key, value := range testData {
		_, err := client.Put(ctx, &kvstore.PutRequest{
			Key:   key,
			Value: []byte(value),
		})
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	// Test List with prefix
	listResp, err := client.List(ctx, &kvstore.ListRequest{
		Prefix: "list:",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listResp.Items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(listResp.Items))
	}

	// Verify items have correct prefix
	for _, item := range listResp.Items {
		if !strings.HasPrefix(item.Key, "list:") {
			t.Errorf("Item key %s doesn't have expected prefix", item.Key)
		}
	}
}

func TestGRPCServer_BatchPut(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// Test batch PUT
	batchResp, err := client.BatchPut(ctx, &kvstore.BatchPutRequest{
		Items: []*kvstore.PutItem{
			{Key: "batch1", Value: []byte("value1")},
			{Key: "batch2", Value: []byte("value2")},
			{Key: "batch3", Value: []byte("value3")},
		},
	})
	if err != nil {
		t.Fatalf("BatchPut failed: %v", err)
	}

	if batchResp.SuccessCount != 3 {
		t.Errorf("Expected success count 3, got %d", batchResp.SuccessCount)
	}

	if batchResp.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", batchResp.ErrorCount)
	}
}

func TestGRPCServer_BatchGet(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// PUT test data
	testKeys := []string{"bget1", "bget2", "bget3"}
	for _, key := range testKeys {
		_, err := client.Put(ctx, &kvstore.PutRequest{
			Key:   key,
			Value: []byte("value-" + key),
		})
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	// Test batch GET
	batchResp, err := client.BatchGet(ctx, &kvstore.BatchGetRequest{
		Keys: append(testKeys, "non-existent"),
	})
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
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

func TestGRPCServer_Health(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	healthResp, err := client.Health(ctx, &kvstore.HealthRequest{})
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}

	if !healthResp.Healthy {
		t.Error("Expected healthy=true")
	}

	if healthResp.Status != "healthy" {
		t.Errorf("Expected status='healthy', got '%s'", healthResp.Status)
	}
}

func TestGRPCServer_Stats(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// Test without details
	statsResp, err := client.Stats(ctx, &kvstore.StatsRequest{
		IncludeDetails: false,
	})
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if len(statsResp.Details) > 0 {
		t.Error("Expected no details when IncludeDetails=false")
	}

	// Test with details
	statsResp, err = client.Stats(ctx, &kvstore.StatsRequest{
		IncludeDetails: true,
	})
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	// Details should be present when requested
	// (actual content depends on storage implementation)
}

func TestGRPCServer_ErrorHandling(t *testing.T) {
	_, conn := setupTestGRPCServer(t)
	client := kvstore.NewKVStoreClient(conn)

	ctx := context.Background()

	// Test PUT with empty key
	putResp, err := client.Put(ctx, &kvstore.PutRequest{
		Key:   "",
		Value: []byte("test-value"),
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if putResp.Success {
		t.Error("Expected Put success=false for empty key")
	}

	if putResp.Error == "" {
		t.Error("Expected error message for empty key")
	}
}

