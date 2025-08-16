package partitioning

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"distributed-kvstore/internal/consensus"
)

// Router handles data routing to appropriate shards
type Router struct {
	mu           sync.RWMutex
	shardManager *ShardManager
	hashRing     *HashRing
	nodeID       string
	config       RouterConfig
	logger       *log.Logger
	
	// Route caching
	routeCache   map[string]RouteInfo
	cacheTimeout time.Duration
}

// RouterConfig contains configuration for the router
type RouterConfig struct {
	EnableCaching    bool          `json:"enable_caching"`     // Enable route caching
	CacheTimeout     time.Duration `json:"cache_timeout"`      // Route cache timeout
	MaxCacheSize     int           `json:"max_cache_size"`     // Maximum cache entries
	RetryAttempts    int           `json:"retry_attempts"`     // Number of retry attempts for failed operations
	RetryDelay       time.Duration `json:"retry_delay"`        // Delay between retries
	RequestTimeout   time.Duration `json:"request_timeout"`    // Timeout for individual requests
	EnableMetrics    bool          `json:"enable_metrics"`     // Enable routing metrics
}

// RouteInfo contains information about where a key should be routed
type RouteInfo struct {
	Key         string    `json:"key"`
	ShardID     uint32    `json:"shard_id"`
	PrimaryNode string    `json:"primary_node"`
	Replicas    []string  `json:"replicas"`
	CachedAt    time.Time `json:"cached_at"`
	IsLocal     bool      `json:"is_local"`
}

// OperationContext contains context for a routing operation
type OperationContext struct {
	Operation   string            `json:"operation"`    // PUT, GET, DELETE, etc.
	Key         string            `json:"key"`
	Value       []byte            `json:"value,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	Timeout     time.Duration     `json:"timeout"`
	Consistency string            `json:"consistency"`  // strong, eventual, etc.
}

// RoutingResult contains the result of a routing operation
type RoutingResult struct {
	Success     bool              `json:"success"`
	Value       []byte            `json:"value,omitempty"`
	Error       string            `json:"error,omitempty"`
	Route       RouteInfo         `json:"route"`
	Latency     time.Duration     `json:"latency"`
	Attempts    int               `json:"attempts"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NewRouter creates a new data router
func NewRouter(nodeID string, shardManager *ShardManager, hashRing *HashRing, config RouterConfig, logger *log.Logger) *Router {
	if logger == nil {
		logger = log.New(log.Writer(), "[ROUTER] ", log.LstdFlags)
	}

	// Set default configuration values
	if config.CacheTimeout == 0 {
		config.CacheTimeout = 5 * time.Minute
	}
	if config.MaxCacheSize == 0 {
		config.MaxCacheSize = 10000
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 100 * time.Millisecond
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Second
	}

	return &Router{
		shardManager: shardManager,
		hashRing:     hashRing,
		nodeID:       nodeID,
		config:       config,
		logger:       logger,
		routeCache:   make(map[string]RouteInfo),
		cacheTimeout: config.CacheTimeout,
	}
}

// RouteOperation routes an operation to the appropriate shard
func (r *Router) RouteOperation(ctx context.Context, opCtx OperationContext) (*RoutingResult, error) {
	startTime := time.Now()
	
	result := &RoutingResult{
		Attempts: 0,
		Metadata: make(map[string]string),
	}

	// Apply timeout from operation context
	if opCtx.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opCtx.Timeout)
		defer cancel()
	}

	// Get route information
	route, err := r.GetRoute(opCtx.Key)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get route: %v", err)
		result.Latency = time.Since(startTime)
		return result, err
	}

	result.Route = route

	// Execute operation with retries
	for attempt := 1; attempt <= r.config.RetryAttempts; attempt++ {
		result.Attempts = attempt

		operationResult, err := r.executeOperation(ctx, opCtx, route)
		if err == nil {
			result.Success = true
			result.Value = operationResult.Value
			result.Latency = time.Since(startTime)
			if operationResult.Metadata != nil {
				for k, v := range operationResult.Metadata {
					result.Metadata[k] = v
				}
			}
			return result, nil
		}

		r.logger.Printf("Operation attempt %d failed for key %s: %v", attempt, opCtx.Key, err)

		// Check if we should retry
		if attempt < r.config.RetryAttempts && r.shouldRetry(err) {
			// Clear cache entry if it might be stale
			if r.config.EnableCaching {
				r.invalidateRoute(opCtx.Key)
			}

			// Wait before retry
			time.Sleep(r.config.RetryDelay)

			// Get fresh route information
			route, err = r.GetRoute(opCtx.Key)
			if err != nil {
				result.Error = fmt.Sprintf("failed to get route on retry: %v", err)
				break
			}
			result.Route = route
		} else {
			result.Error = err.Error()
			break
		}
	}

	result.Latency = time.Since(startTime)
	return result, fmt.Errorf("operation failed after %d attempts: %s", result.Attempts, result.Error)
}

// GetRoute determines the route for a given key
func (r *Router) GetRoute(key string) (RouteInfo, error) {
	// Check cache first
	if r.config.EnableCaching {
		if route, found := r.getCachedRoute(key); found {
			return route, nil
		}
	}

	// Find the shard for this key
	shard, err := r.shardManager.GetShardForKey(key)
	if err != nil {
		// If no shard exists, we might need to create one or use default routing
		r.logger.Printf("No shard found for key %s, using hash ring routing", key)
		return r.getHashRingRoute(key)
	}

	// Get current leader for the shard
	leader, err := r.shardManager.GetShardLeader(shard.ID)
	if err != nil {
		r.logger.Printf("Failed to get leader for shard %d: %v", shard.ID, err)
		// Fall back to first replica
		if len(shard.Replicas) > 0 {
			leader = shard.Replicas[0]
		}
	}

	route := RouteInfo{
		Key:         key,
		ShardID:     shard.ID,
		PrimaryNode: leader,
		Replicas:    shard.Replicas,
		CachedAt:    time.Now(),
		IsLocal:     r.isLocalNode(leader),
	}

	// Cache the route
	if r.config.EnableCaching {
		r.cacheRoute(key, route)
	}

	return route, nil
}

// getHashRingRoute gets route information using only the hash ring
func (r *Router) getHashRingRoute(key string) (RouteInfo, error) {
	// Get nodes from hash ring
	nodes, err := r.hashRing.GetNodes(key, 3) // Get 3 nodes for replication
	if err != nil {
		return RouteInfo{}, fmt.Errorf("failed to get nodes from hash ring: %w", err)
	}

	if len(nodes) == 0 {
		return RouteInfo{}, fmt.Errorf("no nodes available")
	}

	route := RouteInfo{
		Key:         key,
		ShardID:     0, // No specific shard
		PrimaryNode: nodes[0],
		Replicas:    nodes,
		CachedAt:    time.Now(),
		IsLocal:     r.isLocalNode(nodes[0]),
	}

	return route, nil
}

// executeOperation executes an operation on the target node/shard
func (r *Router) executeOperation(ctx context.Context, opCtx OperationContext, route RouteInfo) (*OperationResult, error) {
	// If the operation should be handled locally
	if route.IsLocal {
		return r.executeLocalOperation(ctx, opCtx, route)
	}

	// Forward to remote node
	return r.executeRemoteOperation(ctx, opCtx, route)
}

// executeLocalOperation executes an operation locally
func (r *Router) executeLocalOperation(ctx context.Context, opCtx OperationContext, route RouteInfo) (*OperationResult, error) {
	r.logger.Printf("Executing local operation %s for key %s on shard %d", 
		opCtx.Operation, opCtx.Key, route.ShardID)

	// Get the shard
	shard, err := r.shardManager.GetShard(route.ShardID)
	if err != nil {
		return nil, fmt.Errorf("shard %d not found: %w", route.ShardID, err)
	}

	// Execute operation based on type
	switch opCtx.Operation {
	case "GET":
		return r.executeGet(ctx, opCtx, shard)
	case "PUT":
		return r.executePut(ctx, opCtx, shard)
	case "DELETE":
		return r.executeDelete(ctx, opCtx, shard)
	default:
		return nil, fmt.Errorf("unsupported operation: %s", opCtx.Operation)
	}
}

// executeRemoteOperation forwards an operation to a remote node
func (r *Router) executeRemoteOperation(ctx context.Context, opCtx OperationContext, route RouteInfo) (*OperationResult, error) {
	r.logger.Printf("Forwarding operation %s for key %s to node %s", 
		opCtx.Operation, opCtx.Key, route.PrimaryNode)

	// In a real implementation, this would make an RPC call to the remote node
	// For now, we'll simulate it
	time.Sleep(10 * time.Millisecond) // Simulate network latency

	return &OperationResult{
		Success: true,
		Value:   []byte(fmt.Sprintf("remote_result_%s", opCtx.Key)),
		Metadata: map[string]string{
			"node":     route.PrimaryNode,
			"shard_id": fmt.Sprintf("%d", route.ShardID),
			"remote":   "true",
		},
	}, nil
}

// executeGet executes a GET operation
func (r *Router) executeGet(ctx context.Context, opCtx OperationContext, shard *Shard) (*OperationResult, error) {
	// In a real implementation, this would interact with the storage engine
	// For now, we'll simulate it
	result := &OperationResult{
		Success: true,
		Value:   []byte(fmt.Sprintf("value_for_%s", opCtx.Key)),
		Metadata: map[string]string{
			"operation": "GET",
			"shard_id":  fmt.Sprintf("%d", shard.ID),
			"leader":    shard.Leader,
		},
	}

	return result, nil
}

// executePut executes a PUT operation
func (r *Router) executePut(ctx context.Context, opCtx OperationContext, shard *Shard) (*OperationResult, error) {
	// In a real implementation, this would:
	// 1. Submit the operation to the Raft group
	// 2. Wait for consensus
	// 3. Apply to storage engine

	if shard.RaftGroup != nil && shard.Leader == r.nodeID {
		// Create operation data
		data, err := consensus.CreatePutOperation(opCtx.Key, string(opCtx.Value))
		if err != nil {
			return nil, fmt.Errorf("failed to create operation: %w", err)
		}

		// Submit to Raft
		leaderNode, exists := shard.RaftGroup.GetNode(shard.Leader)
		if exists {
			_, err = leaderNode.AppendEntry("PUT", data)
			if err != nil {
				return nil, fmt.Errorf("failed to append entry: %w", err)
			}
		}
	}

	result := &OperationResult{
		Success: true,
		Metadata: map[string]string{
			"operation": "PUT",
			"shard_id":  fmt.Sprintf("%d", shard.ID),
			"leader":    shard.Leader,
		},
	}

	return result, nil
}

// executeDelete executes a DELETE operation
func (r *Router) executeDelete(ctx context.Context, opCtx OperationContext, shard *Shard) (*OperationResult, error) {
	// Similar to PUT, but for deletion
	if shard.RaftGroup != nil && shard.Leader == r.nodeID {
		data, err := consensus.CreateDeleteOperation(opCtx.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to create delete operation: %w", err)
		}

		leaderNode, exists := shard.RaftGroup.GetNode(shard.Leader)
		if exists {
			_, err = leaderNode.AppendEntry("DELETE", data)
			if err != nil {
				return nil, fmt.Errorf("failed to append delete entry: %w", err)
			}
		}
	}

	result := &OperationResult{
		Success: true,
		Metadata: map[string]string{
			"operation": "DELETE",
			"shard_id":  fmt.Sprintf("%d", shard.ID),
			"leader":    shard.Leader,
		},
	}

	return result, nil
}

// OperationResult represents the result of an operation
type OperationResult struct {
	Success  bool              `json:"success"`
	Value    []byte            `json:"value,omitempty"`
	Error    string            `json:"error,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Cache management functions

// getCachedRoute retrieves a route from the cache
func (r *Router) getCachedRoute(key string) (RouteInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	route, exists := r.routeCache[key]
	if !exists {
		return RouteInfo{}, false
	}

	// Check if cache entry is still valid
	if time.Since(route.CachedAt) > r.cacheTimeout {
		delete(r.routeCache, key)
		return RouteInfo{}, false
	}

	return route, true
}

// cacheRoute stores a route in the cache
func (r *Router) cacheRoute(key string, route RouteInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Implement simple LRU eviction if cache is full
	if len(r.routeCache) >= r.config.MaxCacheSize {
		r.evictOldestEntry()
	}

	r.routeCache[key] = route
}

// invalidateRoute removes a route from the cache
func (r *Router) invalidateRoute(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.routeCache, key)
}

// evictOldestEntry removes the oldest cache entry
func (r *Router) evictOldestEntry() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, route := range r.routeCache {
		if first || route.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = route.CachedAt
			first = false
		}
	}

	if oldestKey != "" {
		delete(r.routeCache, oldestKey)
	}
}

// Helper functions

// isLocalNode checks if a node ID represents the local node
func (r *Router) isLocalNode(nodeID string) bool {
	return nodeID == r.nodeID
}

// shouldRetry determines if an operation should be retried
func (r *Router) shouldRetry(err error) bool {
	// In a real implementation, this would check error types
	// For now, we'll retry on most errors
	errorStr := err.Error()
	
	// Don't retry on certain errors
	if contains(errorStr, "not found") || contains(errorStr, "invalid") {
		return false
	}

	return true
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || (len(s) > len(substr) && 
		   findSubstring(s, substr)))
}

// findSubstring checks if substring exists in string
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetRouteInfo returns route information for a key without executing an operation
func (r *Router) GetRouteInfo(key string) (RouteInfo, error) {
	return r.GetRoute(key)
}

// InvalidateCache clears the entire route cache
func (r *Router) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routeCache = make(map[string]RouteInfo)
	r.logger.Printf("Route cache invalidated")
}

// GetCacheStats returns statistics about the route cache
func (r *Router) GetCacheStats() CacheStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := CacheStats{
		Size:        len(r.routeCache),
		MaxSize:     r.config.MaxCacheSize,
		Timeout:     r.cacheTimeout,
		HitRate:     0.0, // Would need to track hits/misses
	}

	return stats
}

// CacheStats contains statistics about the route cache
type CacheStats struct {
	Size     int           `json:"size"`
	MaxSize  int           `json:"max_size"`
	Timeout  time.Duration `json:"timeout"`
	HitRate  float64       `json:"hit_rate"`
}

// GetStats returns routing statistics
func (r *Router) GetStats() RouterStats {
	return RouterStats{
		CacheStats: r.GetCacheStats(),
		Config:     r.config,
	}
}

// RouterStats contains router statistics
type RouterStats struct {
	CacheStats CacheStats    `json:"cache_stats"`
	Config     RouterConfig  `json:"config"`
}