package storage

// NewStorageEngine creates a storage engine, optionally with caching
func NewStorageEngine(config Config) (StorageEngine, error) {
	// Create base engine
	baseEngine, err := NewEngine(config)
	if err != nil {
		return nil, err
	}

	// If caching is disabled, return base engine
	if !config.CacheEnabled {
		return baseEngine, nil
	}

	// Create cached engine
	cacheConfig := CachedStorageConfig{
		CacheEnabled:    config.CacheEnabled,
		CacheSize:       config.CacheSize,
		DefaultTTL:      config.CacheTTL,
		CleanupInterval: config.CacheCleanupInterval,
	}

	cachedEngine := NewCachedStorageEngine(baseEngine, cacheConfig)
	return cachedEngine, nil
}