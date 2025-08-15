package client

import (
	"context"
	"net"
	"testing"
	"time"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/server"
	"distributed-kvstore/internal/storage"
	"distributed-kvstore/proto/kvstore"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func init() {
	lis = bufconn.Listen(bufSize)
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func setupTestServer(t *testing.T) (*server.GRPCServer, func()) {
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

	grpcServer := server.NewGRPCServer(cfg, storageEngine, logger)
	
	s := grpc.NewServer()
	kvstore.RegisterKVStoreServer(s, grpcServer)

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	return grpcServer, func() {
		s.Stop()
		storageEngine.Close()
	}
}

func setupTestClient(t *testing.T) (*Client, func()) {
	_, cleanup := setupTestServer(t)

	config := &Config{
		Addresses:         []string{"bufnet"},
		MaxConnections:    2,
		MaxIdleTime:       5 * time.Minute,
		ConnectionTimeout: 5 * time.Second,
		RequestTimeout:    10 * time.Second,
		MaxRetries:        2,
		RetryDelay:        50 * time.Millisecond,
		RetryBackoff:      1.5,
	}

	// Override connection creation to use bufconn
	pool := &ConnectionPool{
		config:      config,
		connections: make([]*Connection, 0),
		closeCh:     make(chan struct{}),
	}

	// Create a test connection using bufconn
	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}

	testConn := &Connection{
		conn:     conn,
		address:  "bufnet",
		lastUsed: time.Now(),
		created:  time.Now(),
		inUse:    false,
	}

	pool.connections = append(pool.connections, testConn)

	client := &Client{
		config: config,
		pool:   pool,
	}

	return client, func() {
		client.Close()
		cleanup()
	}
}

func TestClient_Put_Get(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	// Test Put
	err := client.Put(ctx, key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test Get
	result, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !result.Found {
		t.Error("Expected key to be found")
	}

	if string(result.Value) != string(value) {
		t.Errorf("Expected value %s, got %s", string(value), string(result.Value))
	}
}

func TestClient_Put_WithTTL(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()
	key := "ttl-test-key"
	value := []byte("ttl-test-value")
	ttl := int64(3600) // 1 hour

	err := client.Put(ctx, key, value, WithTTL(ttl))
	if err != nil {
		t.Fatalf("Put with TTL failed: %v", err)
	}

	result, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !result.Found {
		t.Error("Expected key to be found")
	}

	if string(result.Value) != string(value) {
		t.Errorf("Expected value %s, got %s", string(value), string(result.Value))
	}
}

func TestClient_Delete(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()
	key := "delete-test-key"
	value := []byte("delete-test-value")

	// Put a key first
	err := client.Put(ctx, key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Delete the key
	result, err := client.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if !result.Existed {
		t.Error("Expected key to have existed before deletion")
	}

	// Verify key is gone
	getResult, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}

	if getResult.Found {
		t.Error("Expected key to not be found after deletion")
	}
}

func TestClient_Exists(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()
	key := "exists-test-key"
	value := []byte("exists-test-value")

	// Check non-existent key
	exists, err := client.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if exists {
		t.Error("Expected key to not exist")
	}

	// Put a key
	err = client.Put(ctx, key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Check existing key
	exists, err = client.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if !exists {
		t.Error("Expected key to exist")
	}
}

func TestClient_List(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()
	prefix := "list-test:"

	// Put some test data
	testData := map[string]string{
		"list-test:key1": "value1",
		"list-test:key2": "value2",
		"list-test:key3": "value3",
		"other:key1":     "other1",
	}

	for key, value := range testData {
		err := client.Put(ctx, key, []byte(value))
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	// Test list with prefix
	result, err := client.List(ctx, prefix)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if result.Count() != 3 {
		t.Errorf("Expected 3 items, got %d", result.Count())
	}

	// Verify all items have the correct prefix
	for _, item := range result.Items {
		if len(item.Key) < len(prefix) || item.Key[:len(prefix)] != prefix {
			t.Errorf("Item key %s doesn't have prefix %s", item.Key, prefix)
		}
	}
}

func TestClient_ListKeys(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()
	prefix := "keys-test:"

	// Put some test data
	testKeys := []string{
		"keys-test:key1",
		"keys-test:key2",
		"keys-test:key3",
	}

	for _, key := range testKeys {
		err := client.Put(ctx, key, []byte("value"))
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	// Test list keys with prefix
	result, err := client.ListKeys(ctx, prefix)
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}

	if result.Count() != 3 {
		t.Errorf("Expected 3 keys, got %d", result.Count())
	}

	// Verify all keys have the correct prefix
	for _, key := range result.Keys {
		if len(key) < len(prefix) || key[:len(prefix)] != prefix {
			t.Errorf("Key %s doesn't have prefix %s", key, prefix)
		}
	}
}

func TestClient_BatchPut(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()

	items := []*PutItem{
		{Key: "batch:key1", Value: []byte("value1")},
		{Key: "batch:key2", Value: []byte("value2")},
		{Key: "batch:key3", Value: []byte("value3")},
	}

	result, err := client.BatchPut(ctx, items)
	if err != nil {
		t.Fatalf("BatchPut failed: %v", err)
	}

	if result.SuccessCount != 3 {
		t.Errorf("Expected 3 successful puts, got %d", result.SuccessCount)
	}

	if result.ErrorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", result.ErrorCount)
	}

	if !result.IsFullSuccess() {
		t.Error("Expected full success")
	}
}

func TestClient_BatchGet(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()

	// Put some test data
	testData := map[string]string{
		"bget:key1": "value1",
		"bget:key2": "value2",
		"bget:key3": "value3",
	}

	for key, value := range testData {
		err := client.Put(ctx, key, []byte(value))
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	keys := []string{"bget:key1", "bget:key2", "bget:key3", "nonexistent"}
	results, err := client.BatchGet(ctx, keys)
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}

	foundCount := 0
	for _, result := range results {
		if result.Found {
			foundCount++
		}
	}

	if foundCount != 3 {
		t.Errorf("Expected 3 found results, got %d", foundCount)
	}
}

func TestClient_BatchDelete(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()

	// Put some test data
	testKeys := []string{"bdel:key1", "bdel:key2", "bdel:key3"}
	for _, key := range testKeys {
		err := client.Put(ctx, key, []byte("value"))
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	result, err := client.BatchDelete(ctx, testKeys)
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	if result.SuccessCount != 3 {
		t.Errorf("Expected 3 successful deletes, got %d", result.SuccessCount)
	}

	if result.ErrorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", result.ErrorCount)
	}
}

func TestClient_Health(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()

	result, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if !result.IsHealthy() {
		t.Error("Expected server to be healthy")
	}

	if result.Status == "" {
		t.Error("Expected non-empty status")
	}
}

func TestClient_Stats(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()

	// Put some data first
	err := client.Put(ctx, "stats-test", []byte("stats-value"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	result, err := client.Stats(ctx, false)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if result.TotalSize < 0 {
		t.Error("Expected non-negative total size")
	}

	// Test with details
	resultWithDetails, err := client.Stats(ctx, true)
	if err != nil {
		t.Fatalf("Stats with details failed: %v", err)
	}

	if len(resultWithDetails.Details) == 0 {
		t.Error("Expected details to be populated")
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	client, cleanup := setupTestClient(t)
	defer cleanup()

	ctx := context.Background()

	// Test empty key
	err := client.Put(ctx, "", []byte("value"))
	if err == nil {
		t.Error("Expected error for empty key")
	}

	// Test get non-existent key
	result, err := client.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.Found {
		t.Error("Expected key to not be found")
	}
}

func TestGetResult_Methods(t *testing.T) {
	// Test GetResult methods
	result := &GetResult{
		Found:     true,
		Value:     []byte("test"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if result.String() != "test" {
		t.Errorf("Expected String() to return 'test', got %s", result.String())
	}

	if !result.HasTTL() {
		t.Error("Expected HasTTL() to return true")
	}

	if result.IsExpired() {
		t.Error("Expected IsExpired() to return false")
	}

	if result.TTL() <= 0 {
		t.Error("Expected positive TTL")
	}

	// Test not found result
	notFoundResult := &GetResult{Found: false}
	if notFoundResult.String() != "(nil)" {
		t.Errorf("Expected String() to return '(nil)' for not found, got %s", notFoundResult.String())
	}
}

func TestBatchResult_Methods(t *testing.T) {
	result := &BatchResult{
		SuccessCount: 8,
		ErrorCount:   2,
		Errors: []*BatchError{
			{Key: "key1", Error: "error1"},
			{Key: "key2", Error: "error2"},
		},
	}

	if !result.HasErrors() {
		t.Error("Expected HasErrors() to return true")
	}

	if result.IsFullSuccess() {
		t.Error("Expected IsFullSuccess() to return false")
	}

	if result.TotalOperations() != 10 {
		t.Errorf("Expected TotalOperations() to return 10, got %d", result.TotalOperations())
	}

	expectedSuccessRate := 80.0
	if result.SuccessRate() != expectedSuccessRate {
		t.Errorf("Expected SuccessRate() to return %.1f, got %.1f", expectedSuccessRate, result.SuccessRate())
	}

	errors := result.GetErrorsForKey("key1")
	if len(errors) != 1 || errors[0] != "error1" {
		t.Errorf("Expected one error 'error1' for key1, got %v", errors)
	}
}