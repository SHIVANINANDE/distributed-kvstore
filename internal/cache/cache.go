package cache

import (
	"sync"
	"time"
)

// Cache defines the interface for caching operations
type Cache interface {
	Get(key string) ([]byte, bool)
	Put(key string, value []byte, ttl time.Duration) bool
	Delete(key string) bool
	Clear()
	Stats() CacheStats
	Close() error
}

// CacheStats provides statistics about cache operations
type CacheStats struct {
	Hits     int64
	Misses   int64
	Evictions int64
	Size     int
	Capacity int
	HitRatio float64
}

// LRUCache implements an LRU (Least Recently Used) cache with TTL support
type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*cacheItem
	head     *cacheItem
	tail     *cacheItem
	
	// Statistics
	hits      int64
	misses    int64
	evictions int64
}

type cacheItem struct {
	key       string
	value     []byte
	expiresAt *time.Time
	prev      *cacheItem
	next      *cacheItem
}

// NewLRUCache creates a new LRU cache with the specified capacity
func NewLRUCache(capacity int) *LRUCache {
	if capacity <= 0 {
		capacity = 1000 // Default capacity
	}

	cache := &LRUCache{
		capacity: capacity,
		items:    make(map[string]*cacheItem),
	}

	// Initialize doubly linked list with dummy head and tail
	cache.head = &cacheItem{}
	cache.tail = &cacheItem{}
	cache.head.next = cache.tail
	cache.tail.prev = cache.head

	return cache
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check if item has expired
	if item.expiresAt != nil && time.Now().After(*item.expiresAt) {
		c.removeItem(item)
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.moveToFront(item)
	c.hits++
	
	// Return copy of value to prevent external modifications
	valueCopy := make([]byte, len(item.value))
	copy(valueCopy, item.value)
	return valueCopy, true
}

// Put stores a value in the cache with optional TTL
func (c *LRUCache) Put(key string, value []byte, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if item already exists
	if existingItem, exists := c.items[key]; exists {
		// Update existing item
		existingItem.value = make([]byte, len(value))
		copy(existingItem.value, value)
		
		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			existingItem.expiresAt = &expiresAt
		} else {
			existingItem.expiresAt = nil
		}
		
		c.moveToFront(existingItem)
		return true
	}

	// Create new item
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	
	item := &cacheItem{
		key:   key,
		value: valueCopy,
	}

	if ttl > 0 {
		expiresAt := time.Now().Add(ttl)
		item.expiresAt = &expiresAt
	}

	// Add to front of list
	c.addToFront(item)
	c.items[key] = item

	// Check capacity and evict if necessary
	if len(c.items) > c.capacity {
		c.evictLRU()
	}

	return true
}

// Delete removes a value from the cache
func (c *LRUCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return false
	}

	c.removeItem(item)
	return true
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.head.next = c.tail
	c.tail.prev = c.head
	c.hits = 0
	c.misses = 0
	c.evictions = 0
}

// Stats returns cache statistics
func (c *LRUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalAccess := c.hits + c.misses
	hitRatio := 0.0
	if totalAccess > 0 {
		hitRatio = float64(c.hits) / float64(totalAccess)
	}

	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		Size:      len(c.items),
		Capacity:  c.capacity,
		HitRatio:  hitRatio,
	}
}

// Close cleans up the cache
func (c *LRUCache) Close() error {
	c.Clear()
	return nil
}

// moveToFront moves an item to the front of the list (most recently used)
func (c *LRUCache) moveToFront(item *cacheItem) {
	c.removeFromList(item)
	c.addToFront(item)
}

// addToFront adds an item to the front of the list
func (c *LRUCache) addToFront(item *cacheItem) {
	item.prev = c.head
	item.next = c.head.next
	c.head.next.prev = item
	c.head.next = item
}

// removeFromList removes an item from the doubly linked list
func (c *LRUCache) removeFromList(item *cacheItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

// removeItem removes an item from both the map and the list
func (c *LRUCache) removeItem(item *cacheItem) {
	delete(c.items, item.key)
	c.removeFromList(item)
}

// evictLRU removes the least recently used item
func (c *LRUCache) evictLRU() {
	if c.tail.prev == c.head {
		return // Empty cache
	}

	lru := c.tail.prev
	c.removeItem(lru)
	c.evictions++
}

// CleanupExpired removes expired items from the cache
func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, item := range c.items {
		if item.expiresAt != nil && now.After(*item.expiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if item, exists := c.items[key]; exists {
			c.removeItem(item)
		}
	}

	return len(expiredKeys)
}