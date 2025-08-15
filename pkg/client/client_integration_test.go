package client

import (
	"testing"
	"time"

	"distributed-kvstore/proto/kvstore"
)

// Simple integration tests that don't require a test server
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if len(config.Addresses) == 0 {
		t.Error("Expected at least one default address")
	}

	if config.MaxConnections <= 0 {
		t.Error("Expected positive max connections")
	}

	if config.RequestTimeout <= 0 {
		t.Error("Expected positive request timeout")
	}

	if config.MaxRetries < 0 {
		t.Error("Expected non-negative max retries")
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	// Test with empty addresses
	config := &Config{
		Addresses: []string{},
	}

	_, err := NewClient(config)
	if err == nil {
		t.Error("Expected error for empty addresses")
	}

	// Test with nil config (should use defaults)
	client, err := NewClient(nil)
	if err != nil {
		// This might fail because we can't connect, which is expected
		// in test environment without a server running
		if client != nil {
			client.Close()
		}
	}
}

func TestPutOptions(t *testing.T) {
	req := &kvstore.PutRequest{}
	
	// Test WithTTL option
	ttlOpt := WithTTL(3600)
	ttlOpt(req)

	if req.TtlSeconds != 3600 {
		t.Errorf("Expected TTL to be 3600, got %d", req.TtlSeconds)
	}
}

func TestListOptions(t *testing.T) {
	req := &kvstore.ListRequest{}
	
	// Test WithLimit option
	limitOpt := WithLimit(100)
	limitOpt(req)

	if req.Limit != 100 {
		t.Errorf("Expected limit to be 100, got %d", req.Limit)
	}

	// Test WithCursor option
	cursorOpt := WithCursor("test-cursor")
	cursorOpt(req)

	if req.Cursor != "test-cursor" {
		t.Errorf("Expected cursor to be 'test-cursor', got %s", req.Cursor)
	}
}

func TestGetResult_UtilityMethods(t *testing.T) {
	now := time.Now()
	
	// Test result with TTL
	resultWithTTL := &GetResult{
		Found:     true,
		Value:     []byte("test-value"),
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
	}

	if !resultWithTTL.HasTTL() {
		t.Error("Expected HasTTL() to return true")
	}

	if resultWithTTL.IsExpired() {
		t.Error("Expected IsExpired() to return false for future expiration")
	}

	if resultWithTTL.TTL() <= 0 {
		t.Error("Expected positive TTL")
	}

	if resultWithTTL.String() != "test-value" {
		t.Errorf("Expected String() to return 'test-value', got %s", resultWithTTL.String())
	}

	// Test result without TTL
	resultWithoutTTL := &GetResult{
		Found:     true,
		Value:     []byte("test-value"),
		CreatedAt: now,
		ExpiresAt: time.Time{}, // Zero time means no TTL
	}

	if resultWithoutTTL.HasTTL() {
		t.Error("Expected HasTTL() to return false for zero ExpiresAt")
	}

	if resultWithoutTTL.TTL() != 0 {
		t.Error("Expected TTL() to return 0 for no TTL")
	}

	// Test expired result
	expiredResult := &GetResult{
		Found:     true,
		Value:     []byte("expired-value"),
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-1 * time.Hour),
	}

	if !expiredResult.IsExpired() {
		t.Error("Expected IsExpired() to return true for past expiration")
	}

	// Test not found result
	notFoundResult := &GetResult{Found: false}
	if notFoundResult.String() != "(nil)" {
		t.Errorf("Expected String() to return '(nil)' for not found, got %s", notFoundResult.String())
	}
}

func TestListResult_UtilityMethods(t *testing.T) {
	now := time.Now()
	
	items := []*KeyValue{
		{Key: "key1", Value: []byte("value1"), CreatedAt: now},
		{Key: "key2", Value: []byte("value2"), CreatedAt: now},
		{Key: "key3", Value: []byte("value3"), CreatedAt: now},
	}

	result := &ListResult{
		Items:      items,
		NextCursor: "next-cursor",
		HasMore:    true,
	}

	if result.IsEmpty() {
		t.Error("Expected IsEmpty() to return false")
	}

	if result.Count() != 3 {
		t.Errorf("Expected Count() to return 3, got %d", result.Count())
	}

	keys := result.Keys()
	expectedKeys := []string{"key1", "key2", "key3"}
	if len(keys) != len(expectedKeys) {
		t.Errorf("Expected %d keys, got %d", len(expectedKeys), len(keys))
	}

	values := result.Values()
	if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	}

	// Test empty result
	emptyResult := &ListResult{Items: []*KeyValue{}}
	if !emptyResult.IsEmpty() {
		t.Error("Expected IsEmpty() to return true for empty result")
	}

	if emptyResult.Count() != 0 {
		t.Errorf("Expected Count() to return 0 for empty result, got %d", emptyResult.Count())
	}
}

func TestListKeysResult_UtilityMethods(t *testing.T) {
	result := &ListKeysResult{
		Keys:       []string{"key1", "key2", "key3"},
		NextCursor: "next-cursor",
		HasMore:    true,
	}

	if result.IsEmpty() {
		t.Error("Expected IsEmpty() to return false")
	}

	if result.Count() != 3 {
		t.Errorf("Expected Count() to return 3, got %d", result.Count())
	}

	// Test empty result
	emptyResult := &ListKeysResult{Keys: []string{}}
	if !emptyResult.IsEmpty() {
		t.Error("Expected IsEmpty() to return true for empty result")
	}
}

func TestBatchResult_UtilityMethods(t *testing.T) {
	result := &BatchResult{
		SuccessCount: 7,
		ErrorCount:   3,
		Errors: []*BatchError{
			{Key: "key1", Error: "error1"},
			{Key: "key2", Error: "error2"},
			{Key: "key1", Error: "another error for key1"},
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

	expectedSuccessRate := 70.0
	if result.SuccessRate() != expectedSuccessRate {
		t.Errorf("Expected SuccessRate() to return %.1f, got %.1f", expectedSuccessRate, result.SuccessRate())
	}

	// Test errors for specific key
	key1Errors := result.GetErrorsForKey("key1")
	if len(key1Errors) != 2 {
		t.Errorf("Expected 2 errors for key1, got %d", len(key1Errors))
	}

	key3Errors := result.GetErrorsForKey("key3")
	if len(key3Errors) != 0 {
		t.Errorf("Expected 0 errors for key3, got %d", len(key3Errors))
	}

	// Test successful result
	successResult := &BatchResult{
		SuccessCount: 5,
		ErrorCount:   0,
		Errors:       []*BatchError{},
	}

	if successResult.HasErrors() {
		t.Error("Expected HasErrors() to return false for successful result")
	}

	if !successResult.IsFullSuccess() {
		t.Error("Expected IsFullSuccess() to return true for successful result")
	}

	if successResult.SuccessRate() != 100.0 {
		t.Errorf("Expected SuccessRate() to return 100.0 for successful result, got %.1f", successResult.SuccessRate())
	}

	// Test empty result
	emptyResult := &BatchResult{SuccessCount: 0, ErrorCount: 0}
	if emptyResult.SuccessRate() != 0 {
		t.Errorf("Expected SuccessRate() to return 0 for empty result, got %.1f", emptyResult.SuccessRate())
	}
}

func TestHealthResult_UtilityMethods(t *testing.T) {
	result := &HealthResult{
		Healthy:       true,
		Status:        "healthy",
		UptimeSeconds: 3661, // 1 hour and 1 minute and 1 second
		Version:       "1.0.0",
	}

	if !result.IsHealthy() {
		t.Error("Expected IsHealthy() to return true")
	}

	expectedUptime := time.Duration(3661) * time.Second
	if result.UptimeDuration() != expectedUptime {
		t.Errorf("Expected UptimeDuration() to return %v, got %v", expectedUptime, result.UptimeDuration())
	}

	// Test unhealthy result
	unhealthyResult := &HealthResult{Healthy: false}
	if unhealthyResult.IsHealthy() {
		t.Error("Expected IsHealthy() to return false for unhealthy result")
	}
}

func TestStatsResult_UtilityMethods(t *testing.T) {
	result := &StatsResult{
		TotalKeys: 1000,
		TotalSize: 2097152, // 2 MB
		LSMSize:   1048576, // 1 MB
		VLogSize:  1048576, // 1 MB
		Details: map[string]string{
			"custom_detail": "custom_value",
			"another":       "value",
		},
	}

	if result.TotalSizeBytes() != 2097152 {
		t.Errorf("Expected TotalSizeBytes() to return 2097152, got %d", result.TotalSizeBytes())
	}

	expectedMB := 2.0
	if result.TotalSizeMB() != expectedMB {
		t.Errorf("Expected TotalSizeMB() to return %.1f, got %.1f", expectedMB, result.TotalSizeMB())
	}

	if result.LSMSizeBytes() != 1048576 {
		t.Errorf("Expected LSMSizeBytes() to return 1048576, got %d", result.LSMSizeBytes())
	}

	if result.VLogSizeBytes() != 1048576 {
		t.Errorf("Expected VLogSizeBytes() to return 1048576, got %d", result.VLogSizeBytes())
	}

	// Test detail retrieval
	value, exists := result.GetDetail("custom_detail")
	if !exists {
		t.Error("Expected GetDetail('custom_detail') to return true for exists")
	}
	if value != "custom_value" {
		t.Errorf("Expected GetDetail('custom_detail') to return 'custom_value', got %s", value)
	}

	_, exists = result.GetDetail("nonexistent")
	if exists {
		t.Error("Expected GetDetail('nonexistent') to return false for exists")
	}
}