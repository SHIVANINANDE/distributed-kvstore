package cache

import (
	"testing"
	"time"
)

func TestLRUCache_BasicOperations(t *testing.T) {
	cache := NewLRUCache(3)
	defer cache.Close()

	// Test Put and Get
	cache.Put("key1", []byte("value1"), 0)
	cache.Put("key2", []byte("value2"), 0)
	cache.Put("key3", []byte("value3"), 0)

	value, found := cache.Get("key1")
	if !found || string(value) != "value1" {
		t.Errorf("Expected to find key1 with value1, got found=%v, value=%s", found, string(value))
	}

	value, found = cache.Get("key2")
	if !found || string(value) != "value2" {
		t.Errorf("Expected to find key2 with value2, got found=%v, value=%s", found, string(value))
	}

	// Test non-existent key
	_, found = cache.Get("nonexistent")
	if found {
		t.Error("Expected not to find nonexistent key")
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	cache := NewLRUCache(2)
	defer cache.Close()

	// Fill cache to capacity
	cache.Put("key1", []byte("value1"), 0)
	cache.Put("key2", []byte("value2"), 0)

	// Add one more item, should evict least recently used (key1)
	cache.Put("key3", []byte("value3"), 0)

	// key1 should be evicted
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be evicted")
	}

	// key2 and key3 should still exist
	_, found = cache.Get("key2")
	if !found {
		t.Error("Expected key2 to still exist")
	}

	_, found = cache.Get("key3")
	if !found {
		t.Error("Expected key3 to still exist")
	}
}

func TestLRUCache_LRUOrder(t *testing.T) {
	cache := NewLRUCache(3)
	defer cache.Close()

	// Add items
	cache.Put("key1", []byte("value1"), 0)
	cache.Put("key2", []byte("value2"), 0)
	cache.Put("key3", []byte("value3"), 0)

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add key4, should evict key2 (least recently used)
	cache.Put("key4", []byte("value4"), 0)

	// key2 should be evicted
	_, found := cache.Get("key2")
	if found {
		t.Error("Expected key2 to be evicted")
	}

	// key1, key3, key4 should still exist
	_, found = cache.Get("key1")
	if !found {
		t.Error("Expected key1 to still exist")
	}

	_, found = cache.Get("key3")
	if !found {
		t.Error("Expected key3 to still exist")
	}

	_, found = cache.Get("key4")
	if !found {
		t.Error("Expected key4 to still exist")
	}
}

func TestLRUCache_Update(t *testing.T) {
	cache := NewLRUCache(2)
	defer cache.Close()

	// Add item
	cache.Put("key1", []byte("value1"), 0)

	// Update same key
	cache.Put("key1", []byte("updated_value1"), 0)

	value, found := cache.Get("key1")
	if !found || string(value) != "updated_value1" {
		t.Errorf("Expected updated value, got found=%v, value=%s", found, string(value))
	}

	// Cache should still have size 1
	stats := cache.Stats()
	if stats.Size != 1 {
		t.Errorf("Expected cache size 1, got %d", stats.Size)
	}
}

func TestLRUCache_Delete(t *testing.T) {
	cache := NewLRUCache(3)
	defer cache.Close()

	cache.Put("key1", []byte("value1"), 0)
	cache.Put("key2", []byte("value2"), 0)

	// Delete existing key
	deleted := cache.Delete("key1")
	if !deleted {
		t.Error("Expected successful deletion")
	}

	// Verify key is gone
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be deleted")
	}

	// Delete non-existent key
	deleted = cache.Delete("nonexistent")
	if deleted {
		t.Error("Expected deletion of non-existent key to return false")
	}
}

func TestLRUCache_TTL(t *testing.T) {
	cache := NewLRUCache(3)
	defer cache.Close()

	// Add item with short TTL
	cache.Put("key1", []byte("value1"), 50*time.Millisecond)

	// Should be accessible immediately
	value, found := cache.Get("key1")
	if !found || string(value) != "value1" {
		t.Errorf("Expected to find key1 immediately, got found=%v, value=%s", found, string(value))
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired now
	_, found = cache.Get("key1")
	if found {
		t.Error("Expected key1 to be expired")
	}
}

func TestLRUCache_Stats(t *testing.T) {
	cache := NewLRUCache(2)
	defer cache.Close()

	// Add items
	cache.Put("key1", []byte("value1"), 0)
	cache.Put("key2", []byte("value2"), 0)

	// Generate hits and misses
	cache.Get("key1")    // hit
	cache.Get("key1")    // hit
	cache.Get("key2")    // hit
	cache.Get("missing") // miss

	// Add item to trigger eviction
	cache.Put("key3", []byte("value3"), 0) // should evict key1

	stats := cache.Stats()

	if stats.Hits != 3 {
		t.Errorf("Expected 3 hits, got %d", stats.Hits)
	}

	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	if stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", stats.Evictions)
	}

	if stats.Size != 2 {
		t.Errorf("Expected size 2, got %d", stats.Size)
	}

	if stats.Capacity != 2 {
		t.Errorf("Expected capacity 2, got %d", stats.Capacity)
	}

	expectedHitRatio := 3.0 / 4.0 // 3 hits out of 4 total accesses
	if stats.HitRatio != expectedHitRatio {
		t.Errorf("Expected hit ratio %.2f, got %.2f", expectedHitRatio, stats.HitRatio)
	}
}

func TestLRUCache_Clear(t *testing.T) {
	cache := NewLRUCache(3)
	defer cache.Close()

	// Add items
	cache.Put("key1", []byte("value1"), 0)
	cache.Put("key2", []byte("value2"), 0)

	// Clear cache
	cache.Clear()

	// Verify cache is empty
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected cache to be empty after clear")
	}

	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected size 0 after clear, got %d", stats.Size)
	}

	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Error("Expected all stats to be reset after clear")
	}
}

func TestLRUCache_CleanupExpired(t *testing.T) {
	cache := NewLRUCache(5)
	defer cache.Close()

	// Add items with different TTLs
	cache.Put("key1", []byte("value1"), 50*time.Millisecond)
	cache.Put("key2", []byte("value2"), 0) // no expiration
	cache.Put("key3", []byte("value3"), 50*time.Millisecond)

	// Wait for some to expire
	time.Sleep(100 * time.Millisecond)

	// Cleanup expired items
	expired := cache.CleanupExpired()

	if expired != 2 {
		t.Errorf("Expected 2 expired items, got %d", expired)
	}

	// Verify expired items are gone
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be cleaned up")
	}

	_, found = cache.Get("key3")
	if found {
		t.Error("Expected key3 to be cleaned up")
	}

	// Verify non-expired item still exists
	_, found = cache.Get("key2")
	if !found {
		t.Error("Expected key2 to still exist")
	}
}

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewLRUCache(100)
	defer cache.Close()

	// Run concurrent operations
	done := make(chan bool, 3)

	// Writer goroutine
	go func() {
		for i := 0; i < 50; i++ {
			cache.Put("key"+string(rune(i)), []byte("value"+string(rune(i))), 0)
		}
		done <- true
	}()

	// Reader goroutine 1
	go func() {
		for i := 0; i < 30; i++ {
			cache.Get("key" + string(rune(i)))
		}
		done <- true
	}()

	// Reader goroutine 2
	go func() {
		for i := 0; i < 30; i++ {
			cache.Get("key" + string(rune(i+10)))
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Test should complete without deadlocks or race conditions
	stats := cache.Stats()
	if stats.Size < 0 || stats.Size > 100 {
		t.Errorf("Invalid cache size after concurrent access: %d", stats.Size)
	}
}

func TestLRUCache_DataIntegrity(t *testing.T) {
	cache := NewLRUCache(3)
	defer cache.Close()

	originalData := []byte("original data")
	cache.Put("key1", originalData, 0)

	// Modify original data
	originalData[0] = 'X'

	// Retrieved data should not be affected
	retrievedData, found := cache.Get("key1")
	if !found {
		t.Fatal("Expected to find key1")
	}

	if retrievedData[0] == 'X' {
		t.Error("Cache data was modified by external change to original data")
	}

	// Modify retrieved data
	retrievedData[1] = 'Y'

	// Get data again, should not be affected by modification to retrieved data
	retrievedData2, found := cache.Get("key1")
	if !found {
		t.Fatal("Expected to find key1 on second retrieval")
	}

	if retrievedData2[1] == 'Y' {
		t.Error("Cache data was modified by external change to retrieved data")
	}
}