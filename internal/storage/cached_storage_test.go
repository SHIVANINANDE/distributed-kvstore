package storage

import (
	"testing"
	"time"
)

func TestCachedStorageEngine_BasicOperations(t *testing.T) {
	// Create base storage engine
	baseConfig := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	baseEngine, err := NewEngine(baseConfig)
	if err != nil {
		t.Fatalf("Failed to create base engine: %v", err)
	}
	defer baseEngine.Close()

	// Create cached storage engine
	cacheConfig := CachedStorageConfig{
		CacheEnabled:    true,
		CacheSize:       100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	
	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	defer cachedEngine.Close()

	// Test Put and Get
	key := []byte("test-key")
	value := []byte("test-value")

	err = cachedEngine.Put(key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// First get should miss cache but populate it
	retrievedValue, err := cachedEngine.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(retrievedValue) != string(value) {
		t.Errorf("Expected value %s, got %s", string(value), string(retrievedValue))
	}

	// Second get should hit cache
	retrievedValue2, err := cachedEngine.Get(key)
	if err != nil {
		t.Fatalf("Second get failed: %v", err)
	}

	if string(retrievedValue2) != string(value) {
		t.Errorf("Expected cached value %s, got %s", string(value), string(retrievedValue2))
	}

	// Check cache stats
	stats := cachedEngine.GetCacheStats()
	if stats == nil {
		t.Fatal("Expected cache stats to be available")
	}

	if stats.Hits == 0 {
		t.Error("Expected at least one cache hit")
	}

	if stats.Size == 0 {
		t.Error("Expected cache to have items")
	}
}

func TestCachedStorageEngine_CacheDisabled(t *testing.T) {
	// Create base storage engine
	baseConfig := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	baseEngine, err := NewEngine(baseConfig)
	if err != nil {
		t.Fatalf("Failed to create base engine: %v", err)
	}
	defer baseEngine.Close()

	// Create cached storage engine with cache disabled
	cacheConfig := CachedStorageConfig{
		CacheEnabled: false,
	}
	
	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	defer cachedEngine.Close()

	// Test operations still work
	key := []byte("test-key")
	value := []byte("test-value")

	err = cachedEngine.Put(key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	retrievedValue, err := cachedEngine.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(retrievedValue) != string(value) {
		t.Errorf("Expected value %s, got %s", string(value), string(retrievedValue))
	}

	// Cache stats should be nil
	stats := cachedEngine.GetCacheStats()
	if stats != nil {
		t.Error("Expected cache stats to be nil when cache is disabled")
	}
}

func TestCachedStorageEngine_Delete(t *testing.T) {
	// Create base storage engine
	baseConfig := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	baseEngine, err := NewEngine(baseConfig)
	if err != nil {
		t.Fatalf("Failed to create base engine: %v", err)
	}
	defer baseEngine.Close()

	// Create cached storage engine
	cacheConfig := CachedStorageConfig{
		CacheEnabled:    true,
		CacheSize:       100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	
	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	defer cachedEngine.Close()

	key := []byte("test-key")
	value := []byte("test-value")

	// Put and get to populate cache
	cachedEngine.Put(key, value)
	cachedEngine.Get(key)

	// Verify item is in cache
	stats := cachedEngine.GetCacheStats()
	if stats.Size == 0 {
		t.Error("Expected item to be in cache")
	}

	// Delete the item
	err = cachedEngine.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify item is gone from both storage and cache
	_, err = cachedEngine.Get(key)
	if err != ErrKeyNotFound {
		t.Errorf("Expected key not found error, got %v", err)
	}

	// Cache should be smaller now
	statsAfter := cachedEngine.GetCacheStats()
	if statsAfter.Size >= stats.Size {
		t.Error("Expected cache size to decrease after delete")
	}
}

func TestCachedStorageEngine_Stats(t *testing.T) {
	// Create base storage engine
	baseConfig := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	baseEngine, err := NewEngine(baseConfig)
	if err != nil {
		t.Fatalf("Failed to create base engine: %v", err)
	}
	defer baseEngine.Close()

	// Create cached storage engine
	cacheConfig := CachedStorageConfig{
		CacheEnabled:    true,
		CacheSize:       100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	
	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	defer cachedEngine.Close()

	// Get stats
	stats := cachedEngine.Stats()
	if stats == nil {
		t.Fatal("Expected stats to be available")
	}

	// Should have cache information
	cacheInfo, hasCacheInfo := stats["cache"]
	if !hasCacheInfo {
		t.Error("Expected cache information in stats")
	}

	cacheMap, ok := cacheInfo.(map[string]interface{})
	if !ok {
		t.Error("Expected cache info to be a map")
	}

	enabled, hasEnabled := cacheMap["enabled"]
	if !hasEnabled || enabled != true {
		t.Error("Expected cache to be enabled in stats")
	}
}

func TestCachedStorageEngine_ClearCache(t *testing.T) {
	// Create base storage engine
	baseConfig := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	baseEngine, err := NewEngine(baseConfig)
	if err != nil {
		t.Fatalf("Failed to create base engine: %v", err)
	}
	defer baseEngine.Close()

	// Create cached storage engine
	cacheConfig := CachedStorageConfig{
		CacheEnabled:    true,
		CacheSize:       100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	
	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	defer cachedEngine.Close()

	// Put some data and populate cache
	key := []byte("test-key")
	value := []byte("test-value")
	cachedEngine.Put(key, value)
	cachedEngine.Get(key)

	// Verify cache has items
	stats := cachedEngine.GetCacheStats()
	if stats.Size == 0 {
		t.Error("Expected cache to have items")
	}

	// Clear cache
	cachedEngine.ClearCache()

	// Verify cache is empty
	statsAfter := cachedEngine.GetCacheStats()
	if statsAfter.Size != 0 {
		t.Error("Expected cache to be empty after clear")
	}

	// Data should still be in storage
	retrievedValue, err := cachedEngine.Get(key)
	if err != nil {
		t.Fatalf("Get after cache clear failed: %v", err)
	}

	if string(retrievedValue) != string(value) {
		t.Errorf("Expected value %s after cache clear, got %s", string(value), string(retrievedValue))
	}
}

func TestStorageFactory(t *testing.T) {
	// Test creating storage engine with cache enabled
	config := Config{
		InMemory:     true,
		SyncWrites:   false,
		CacheEnabled: true,
		CacheSize:    100,
		CacheTTL:     5 * time.Minute,
		CacheCleanupInterval: 1 * time.Minute,
	}

	engine, err := NewStorageEngine(config)
	if err != nil {
		t.Fatalf("Failed to create storage engine: %v", err)
	}
	defer engine.Close()

	// Should be a cached engine
	cachedEngine, ok := engine.(*CachedStorageEngine)
	if !ok {
		t.Error("Expected cached storage engine when cache is enabled")
	}

	if cachedEngine.GetCacheStats() == nil {
		t.Error("Expected cache stats to be available")
	}

	// Test creating storage engine with cache disabled
	config.CacheEnabled = false
	engine2, err := NewStorageEngine(config)
	if err != nil {
		t.Fatalf("Failed to create storage engine: %v", err)
	}
	defer engine2.Close()

	// Should be a regular engine
	_, ok = engine2.(*Engine)
	if !ok {
		t.Error("Expected regular storage engine when cache is disabled")
	}
}