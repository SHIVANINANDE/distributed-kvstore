package storage

import (
	"time"

	"distributed-kvstore/internal/cache"
)

// CachedStorageEngine wraps a storage engine with an LRU cache
type CachedStorageEngine struct {
	storage StorageEngine
	cache   cache.Cache
	config  CachedStorageConfig
	
	// Background cleanup
	stopCleanup chan struct{}
}

type CachedStorageConfig struct {
	CacheEnabled    bool
	CacheSize       int
	DefaultTTL      time.Duration
	CleanupInterval time.Duration
}

// NewCachedStorageEngine creates a new cached storage engine
func NewCachedStorageEngine(storageEngine StorageEngine, config CachedStorageConfig) *CachedStorageEngine {
	var cacheInstance cache.Cache
	if config.CacheEnabled {
		cacheInstance = cache.NewLRUCache(config.CacheSize)
	}

	cached := &CachedStorageEngine{
		storage:     storageEngine,
		cache:       cacheInstance,
		config:      config,
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup if cache is enabled
	if config.CacheEnabled && config.CleanupInterval > 0 {
		go cached.cleanupLoop()
	}

	return cached
}

// Put stores a key-value pair in both storage and cache
func (c *CachedStorageEngine) Put(key, value []byte) error {
	// Store in underlying storage first
	err := c.storage.Put(key, value)
	if err != nil {
		return err
	}

	// Update cache if enabled
	if c.cache != nil {
		c.cache.Put(string(key), value, c.config.DefaultTTL)
	}

	return nil
}

// Get retrieves a value, checking cache first
func (c *CachedStorageEngine) Get(key []byte) ([]byte, error) {
	keyStr := string(key)

	// Check cache first if enabled
	if c.cache != nil {
		if value, found := c.cache.Get(keyStr); found {
			return value, nil
		}
	}

	// Cache miss or cache disabled, get from storage
	value, err := c.storage.Get(key)
	if err != nil {
		return nil, err
	}

	// Store in cache if enabled and value was found
	if c.cache != nil && value != nil {
		c.cache.Put(keyStr, value, c.config.DefaultTTL)
	}

	return value, nil
}

// Delete removes a key from both storage and cache
func (c *CachedStorageEngine) Delete(key []byte) error {
	// Delete from underlying storage first
	err := c.storage.Delete(key)
	if err != nil {
		return err
	}

	// Remove from cache if enabled
	if c.cache != nil {
		c.cache.Delete(string(key))
	}

	return nil
}

// Exists checks if a key exists (cache-aware)
func (c *CachedStorageEngine) Exists(key []byte) (bool, error) {
	keyStr := string(key)

	// Check cache first if enabled
	if c.cache != nil {
		if _, found := c.cache.Get(keyStr); found {
			return true, nil
		}
	}

	// Check storage
	return c.storage.Exists(key)
}

// List delegates to the underlying storage engine
// Note: List operations are not cached as they can be complex to invalidate
func (c *CachedStorageEngine) List(prefix []byte) (map[string][]byte, error) {
	return c.storage.List(prefix)
}

// Close closes both cache and storage
func (c *CachedStorageEngine) Close() error {
	// Stop cleanup goroutine
	if c.stopCleanup != nil {
		close(c.stopCleanup)
	}

	// Close cache
	if c.cache != nil {
		c.cache.Close()
	}

	// Close underlying storage
	return c.storage.Close()
}

// Backup delegates to the underlying storage engine
func (c *CachedStorageEngine) Backup(path string) error {
	return c.storage.Backup(path)
}

// Restore delegates to the underlying storage engine and clears cache
func (c *CachedStorageEngine) Restore(path string) error {
	err := c.storage.Restore(path)
	if err != nil {
		return err
	}

	// Clear cache after restore since data has changed
	if c.cache != nil {
		c.cache.Clear()
	}

	return nil
}

// Stats returns combined storage and cache statistics
func (c *CachedStorageEngine) Stats() map[string]interface{} {
	storageStats := c.storage.Stats()

	// Add cache statistics if enabled
	if c.cache != nil {
		cacheStats := c.cache.Stats()
		storageStats["cache"] = map[string]interface{}{
			"enabled":    true,
			"hits":       cacheStats.Hits,
			"misses":     cacheStats.Misses,
			"evictions":  cacheStats.Evictions,
			"size":       cacheStats.Size,
			"capacity":   cacheStats.Capacity,
			"hit_ratio":  cacheStats.HitRatio,
		}
	} else {
		storageStats["cache"] = map[string]interface{}{
			"enabled": false,
		}
	}

	return storageStats
}

// cleanupLoop runs periodic cleanup of expired cache entries
func (c *CachedStorageEngine) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if lruCache, ok := c.cache.(*cache.LRUCache); ok {
				lruCache.CleanupExpired()
			}
		case <-c.stopCleanup:
			return
		}
	}
}

// GetCacheStats returns cache statistics if cache is enabled
func (c *CachedStorageEngine) GetCacheStats() *cache.CacheStats {
	if c.cache == nil {
		return nil
	}

	stats := c.cache.Stats()
	return &stats
}

// ClearCache clears the cache if enabled
func (c *CachedStorageEngine) ClearCache() {
	if c.cache != nil {
		c.cache.Clear()
	}
}

// WarmCache pre-loads frequently accessed keys into cache
func (c *CachedStorageEngine) WarmCache(keys [][]byte) error {
	if c.cache == nil {
		return nil // Cache disabled
	}

	for _, key := range keys {
		value, err := c.storage.Get(key)
		if err != nil {
			continue // Skip keys that don't exist or have errors
		}

		c.cache.Put(string(key), value, c.config.DefaultTTL)
	}

	return nil
}

// BatchPut performs batch put operations
func (c *CachedStorageEngine) BatchPut(items []KeyValue) error {
	// Store in underlying storage first
	err := c.storage.BatchPut(items)
	if err != nil {
		return err
	}

	// Update cache if enabled
	if c.cache != nil {
		for _, item := range items {
			c.cache.Put(string(item.Key), item.Value, c.config.DefaultTTL)
		}
	}

	return nil
}

// BatchGet performs batch get operations
func (c *CachedStorageEngine) BatchGet(keys [][]byte) ([]KeyValue, error) {
	results := make([]KeyValue, len(keys))
	cacheMisses := make([]int, 0, len(keys))
	missKeys := make([][]byte, 0, len(keys))

	// Check cache first if enabled
	if c.cache != nil {
		for i, key := range keys {
			keyStr := string(key)
			if value, found := c.cache.Get(keyStr); found {
				results[i] = KeyValue{Key: key, Value: value, Found: true}
			} else {
				cacheMisses = append(cacheMisses, i)
				missKeys = append(missKeys, key)
			}
		}
	} else {
		// Cache disabled, get all from storage
		for i, key := range keys {
			cacheMisses = append(cacheMisses, i)
			missKeys = append(missKeys, key)
		}
	}

	// Get cache misses from storage
	if len(missKeys) > 0 {
		storageResults, err := c.storage.BatchGet(missKeys)
		if err != nil {
			return nil, err
		}

		// Update results and cache
		for j, storageResult := range storageResults {
			i := cacheMisses[j]
			results[i] = storageResult

			// Cache the result if found and cache is enabled
			if c.cache != nil && storageResult.Found {
				c.cache.Put(string(storageResult.Key), storageResult.Value, c.config.DefaultTTL)
			}
		}
	}

	return results, nil
}

// BatchDelete performs batch delete operations
func (c *CachedStorageEngine) BatchDelete(keys [][]byte) error {
	// Delete from underlying storage first
	err := c.storage.BatchDelete(keys)
	if err != nil {
		return err
	}

	// Remove from cache if enabled
	if c.cache != nil {
		for _, key := range keys {
			c.cache.Delete(string(key))
		}
	}

	return nil
}