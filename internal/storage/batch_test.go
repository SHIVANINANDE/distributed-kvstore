package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestEngine_BatchOperations(t *testing.T) {
	config := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	
	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Test BatchPut
	items := []KeyValue{
		{Key: []byte("batch1"), Value: []byte("value1")},
		{Key: []byte("batch2"), Value: []byte("value2")},
		{Key: []byte("batch3"), Value: []byte("value3")},
	}

	err = engine.BatchPut(items)
	if err != nil {
		t.Fatalf("BatchPut failed: %v", err)
	}

	// Test BatchGet
	keys := [][]byte{
		[]byte("batch1"),
		[]byte("batch2"),
		[]byte("batch3"),
		[]byte("nonexistent"),
	}

	results, err := engine.BatchGet(keys)
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}

	// Check found items
	foundCount := 0
	for _, result := range results {
		if result.Found {
			foundCount++
		}
	}

	if foundCount != 3 {
		t.Errorf("Expected 3 found items, got %d", foundCount)
	}

	// Verify specific values
	if !results[0].Found || string(results[0].Value) != "value1" {
		t.Errorf("Expected batch1 = value1, got found=%v, value=%s", results[0].Found, string(results[0].Value))
	}

	if results[3].Found {
		t.Error("Expected nonexistent key to not be found")
	}

	// Test BatchDelete
	deleteKeys := [][]byte{
		[]byte("batch1"),
		[]byte("batch2"),
	}

	err = engine.BatchDelete(deleteKeys)
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	// Verify deletion
	results, err = engine.BatchGet(keys)
	if err != nil {
		t.Fatalf("BatchGet after delete failed: %v", err)
	}

	if results[0].Found || results[1].Found {
		t.Error("Expected deleted keys to not be found")
	}

	if !results[2].Found {
		t.Error("Expected non-deleted key to still be found")
	}
}

func TestCachedEngine_BatchOperations(t *testing.T) {
	baseConfig := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	baseEngine, err := NewEngine(baseConfig)
	if err != nil {
		t.Fatalf("Failed to create base engine: %v", err)
	}
	defer baseEngine.Close()

	cacheConfig := CachedStorageConfig{
		CacheEnabled:    true,
		CacheSize:       100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	
	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	defer cachedEngine.Close()

	// Test BatchPut
	items := []KeyValue{
		{Key: []byte("cached1"), Value: []byte("cvalue1")},
		{Key: []byte("cached2"), Value: []byte("cvalue2")},
		{Key: []byte("cached3"), Value: []byte("cvalue3")},
	}

	err = cachedEngine.BatchPut(items)
	if err != nil {
		t.Fatalf("Cached BatchPut failed: %v", err)
	}

	// Test BatchGet - should hit cache after first get
	keys := [][]byte{
		[]byte("cached1"),
		[]byte("cached2"),
		[]byte("cached3"),
	}

	// First get - will populate cache
	results, err := cachedEngine.BatchGet(keys)
	if err != nil {
		t.Fatalf("Cached BatchGet failed: %v", err)
	}

	foundCount := 0
	for _, result := range results {
		if result.Found {
			foundCount++
		}
	}

	if foundCount != 3 {
		t.Errorf("Expected 3 found items in cache test, got %d", foundCount)
	}

	// Check cache stats
	stats := cachedEngine.GetCacheStats()
	if stats == nil {
		t.Fatal("Expected cache stats to be available")
	}

	if stats.Size == 0 {
		t.Error("Expected cache to have items after BatchGet")
	}

	// Test BatchDelete
	err = cachedEngine.BatchDelete(keys)
	if err != nil {
		t.Fatalf("Cached BatchDelete failed: %v", err)
	}

	// Verify deletion from both storage and cache
	results, err = cachedEngine.BatchGet(keys)
	if err != nil {
		t.Fatalf("BatchGet after delete failed: %v", err)
	}

	for _, result := range results {
		if result.Found {
			t.Error("Expected all keys to be deleted")
		}
	}
}

func TestConcurrentBatchOperations(t *testing.T) {
	config := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	
	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	const numGoroutines = 10
	const itemsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent batch puts
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			items := make([]KeyValue, itemsPerGoroutine)
			for j := 0; j < itemsPerGoroutine; j++ {
				key := fmt.Sprintf("concurrent_%d_%d", goroutineID, j)
				value := fmt.Sprintf("value_%d_%d", goroutineID, j)
				items[j] = KeyValue{Key: []byte(key), Value: []byte(value)}
			}

			err := engine.BatchPut(items)
			if err != nil {
				t.Errorf("Concurrent BatchPut failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all data was written
	totalKeys := numGoroutines * itemsPerGoroutine
	allKeys := make([][]byte, totalKeys)
	idx := 0

	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < itemsPerGoroutine; j++ {
			key := fmt.Sprintf("concurrent_%d_%d", i, j)
			allKeys[idx] = []byte(key)
			idx++
		}
	}

	results, err := engine.BatchGet(allKeys)
	if err != nil {
		t.Fatalf("BatchGet verification failed: %v", err)
	}

	foundCount := 0
	for _, result := range results {
		if result.Found {
			foundCount++
		}
	}

	if foundCount != totalKeys {
		t.Errorf("Expected %d keys to be found, got %d", totalKeys, foundCount)
	}
}

func BenchmarkBatchOperations(b *testing.B) {
	config := Config{
		InMemory:   true,
		SyncWrites: false,
	}
	
	engine, err := NewEngine(config)
	if err != nil {
		b.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	const batchSize = 100

	b.Run("BatchPut", func(b *testing.B) {
		items := make([]KeyValue, batchSize)
		for i := 0; i < batchSize; i++ {
			key := fmt.Sprintf("bench_put_%d", i)
			value := fmt.Sprintf("bench_value_%d", i)
			items[i] = KeyValue{Key: []byte(key), Value: []byte(value)}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := engine.BatchPut(items)
			if err != nil {
				b.Fatalf("BatchPut failed: %v", err)
			}
		}
	})

	// Prepare data for get benchmark
	items := make([]KeyValue, batchSize)
	keys := make([][]byte, batchSize)
	for i := 0; i < batchSize; i++ {
		key := fmt.Sprintf("bench_get_%d", i)
		value := fmt.Sprintf("bench_value_%d", i)
		items[i] = KeyValue{Key: []byte(key), Value: []byte(value)}
		keys[i] = []byte(key)
	}
	
	err = engine.BatchPut(items)
	if err != nil {
		b.Fatalf("Setup BatchPut failed: %v", err)
	}

	b.Run("BatchGet", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := engine.BatchGet(keys)
			if err != nil {
				b.Fatalf("BatchGet failed: %v", err)
			}
		}
	})

	b.Run("BatchDelete", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			// Re-add data for each iteration
			err := engine.BatchPut(items)
			if err != nil {
				b.Fatalf("Setup BatchPut failed: %v", err)
			}
			b.StartTimer()

			err = engine.BatchDelete(keys)
			if err != nil {
				b.Fatalf("BatchDelete failed: %v", err)
			}
		}
	})
}