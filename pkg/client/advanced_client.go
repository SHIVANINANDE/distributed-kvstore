package client

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// AdvancedClient represents the production-ready distributed key-value store client
type AdvancedClient struct {
	mu               sync.RWMutex
	config           AdvancedClientConfig
	connectionPool   *AdvancedConnectionPool
	loadBalancer     *LoadBalancer
	leaderDiscovery  *LeaderDiscovery
	circuitBreaker   *CircuitBreaker
	retryManager     *RetryManager
	consistencyMgr   *ConsistencyManager
	logger           *log.Logger
	
	// Client state
	isConnected      bool
	lastActivity     time.Time
	metrics          *ClientMetrics
	stopCh           chan struct{}
}

// AdvancedClientConfig contains configuration for the advanced client
type AdvancedClientConfig struct {
	// Connection settings
	InitialNodes     []string      `json:"initial_nodes"`     // Initial cluster nodes
	ConnectionTimeout time.Duration `json:"connection_timeout"` // Connection timeout
	RequestTimeout   time.Duration `json:"request_timeout"`    // Default request timeout
	
	// Load balancing
	LoadBalancingPolicy LoadBalancingPolicy `json:"load_balancing_policy"` // Load balancing strategy
	MaxConnections      int                 `json:"max_connections"`       // Max connections per node
	
	// Retry settings
	MaxRetries        int           `json:"max_retries"`         // Maximum retry attempts
	BaseRetryDelay    time.Duration `json:"base_retry_delay"`    // Base delay for exponential backoff
	MaxRetryDelay     time.Duration `json:"max_retry_delay"`     // Maximum retry delay
	RetryJitter       bool          `json:"retry_jitter"`        // Add jitter to retry delays
	
	// Circuit breaker
	CircuitBreakerEnabled    bool          `json:"circuit_breaker_enabled"`     // Enable circuit breaker
	FailureThreshold         int           `json:"failure_threshold"`           // Failures before opening circuit
	RecoveryTimeout          time.Duration `json:"recovery_timeout"`            // Time before attempting recovery
	HalfOpenMaxRequests      int           `json:"half_open_max_requests"`      // Max requests in half-open state
	
	// Consistency settings
	DefaultConsistency       ConsistencyLevel `json:"default_consistency"`        // Default consistency level
	ReadYourWritesEnabled    bool            `json:"read_your_writes_enabled"`   // Enable read-your-writes
	StaleReadThreshold       time.Duration   `json:"stale_read_threshold"`       // Threshold for stale reads
	
	// Health checking
	HealthCheckInterval      time.Duration `json:"health_check_interval"`       // Health check frequency
	NodeFailureDetectionTime time.Duration `json:"node_failure_detection_time"` // Time to detect node failure
	
	// Logging and metrics
	EnableMetrics           bool `json:"enable_metrics"`            // Enable client metrics
	MetricsCollectionInterval time.Duration `json:"metrics_interval"` // Metrics collection interval
}

// LoadBalancingPolicy defines the load balancing strategy
type LoadBalancingPolicy string

const (
	LoadBalancingRoundRobin  LoadBalancingPolicy = "round_robin"
	LoadBalancingRandom      LoadBalancingPolicy = "random"
	LoadBalancingLeastConn   LoadBalancingPolicy = "least_connections"
	LoadBalancingWeighted    LoadBalancingPolicy = "weighted"
	LoadBalancingConsistent  LoadBalancingPolicy = "consistent_hash"
)

// ConsistencyLevel defines the consistency level for operations
type ConsistencyLevel string

const (
	ConsistencyLinearizable  ConsistencyLevel = "linearizable"   // Strong consistency
	ConsistencyEventual      ConsistencyLevel = "eventual"       // Eventual consistency
	ConsistencyBounded       ConsistencyLevel = "bounded"        // Bounded staleness
	ConsistencySession       ConsistencyLevel = "session"        // Session consistency
	ConsistencyReadYourWrite ConsistencyLevel = "read_your_write" // Read-your-writes
)

// NewAdvancedClient creates a new advanced client instance
func NewAdvancedClient(config AdvancedClientConfig, logger *log.Logger) *AdvancedClient {
	if logger == nil {
		logger = log.New(log.Writer(), "[ADV_CLIENT] ", log.LstdFlags)
	}
	
	// Set default configuration values
	setDefaultConfig(&config)
	
	client := &AdvancedClient{
		config:         config,
		logger:         logger,
		lastActivity:   time.Now(),
		metrics:        NewClientMetrics(),
		stopCh:         make(chan struct{}),
	}
	
	// Initialize components
	client.connectionPool = NewAdvancedConnectionPool(config, logger)
	client.loadBalancer = NewLoadBalancer(config.LoadBalancingPolicy, logger)
	client.leaderDiscovery = NewLeaderDiscovery(config, logger)
	client.circuitBreaker = NewCircuitBreaker(config, logger)
	client.retryManager = NewRetryManager(config, logger)
	client.consistencyMgr = NewConsistencyManager(config, logger)
	
	return client
}

// setDefaultConfig sets default values for the configuration
func setDefaultConfig(config *AdvancedClientConfig) {
	if config.ConnectionTimeout == 0 {
		config.ConnectionTimeout = 5 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 10 * time.Second
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = 10
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.BaseRetryDelay == 0 {
		config.BaseRetryDelay = 100 * time.Millisecond
	}
	if config.MaxRetryDelay == 0 {
		config.MaxRetryDelay = 30 * time.Second
	}
	if config.FailureThreshold == 0 {
		config.FailureThreshold = 5
	}
	if config.RecoveryTimeout == 0 {
		config.RecoveryTimeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests == 0 {
		config.HalfOpenMaxRequests = 3
	}
	if config.DefaultConsistency == "" {
		config.DefaultConsistency = ConsistencyLinearizable
	}
	if config.StaleReadThreshold == 0 {
		config.StaleReadThreshold = 5 * time.Second
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.NodeFailureDetectionTime == 0 {
		config.NodeFailureDetectionTime = 10 * time.Second
	}
	if config.MetricsCollectionInterval == 0 {
		config.MetricsCollectionInterval = 1 * time.Minute
	}
	if config.LoadBalancingPolicy == "" {
		config.LoadBalancingPolicy = LoadBalancingRoundRobin
	}
}

// Connect establishes connection to the cluster
func (c *AdvancedClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.isConnected {
		return nil
	}
	
	c.logger.Printf("Connecting to cluster with %d initial nodes", len(c.config.InitialNodes))
	
	// Initialize connection pool with initial nodes
	if err := c.connectionPool.Initialize(ctx, c.config.InitialNodes); err != nil {
		return fmt.Errorf("failed to initialize connection pool: %w", err)
	}
	
	// Start leader discovery
	if err := c.leaderDiscovery.Start(ctx, c.connectionPool); err != nil {
		return fmt.Errorf("failed to start leader discovery: %w", err)
	}
	
	// Start health checking if enabled
	if c.config.HealthCheckInterval > 0 {
		go c.healthCheckLoop(ctx)
	}
	
	// Start metrics collection if enabled
	if c.config.EnableMetrics {
		go c.metricsCollectionLoop(ctx)
	}
	
	c.isConnected = true
	c.lastActivity = time.Now()
	
	c.logger.Printf("Successfully connected to cluster")
	return nil
}

// Disconnect closes all connections to the cluster
func (c *AdvancedClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.isConnected {
		return nil
	}
	
	c.logger.Printf("Disconnecting from cluster")
	
	// Signal stop to all goroutines
	close(c.stopCh)
	
	// Stop leader discovery
	c.leaderDiscovery.Stop()
	
	// Close connection pool
	if err := c.connectionPool.Close(); err != nil {
		c.logger.Printf("Error closing connection pool: %v", err)
	}
	
	c.isConnected = false
	c.logger.Printf("Disconnected from cluster")
	
	return nil
}

// Get retrieves a value for the given key
func (c *AdvancedClient) Get(ctx context.Context, key string, opts ...OperationOption) (*AdvancedGetResult, error) {
	return c.GetWithConsistency(ctx, key, c.config.DefaultConsistency, opts...)
}

// GetWithConsistency retrieves a value with specified consistency level
func (c *AdvancedClient) GetWithConsistency(ctx context.Context, key string, consistency ConsistencyLevel, opts ...OperationOption) (*AdvancedGetResult, error) {
	c.mu.RLock()
	if !c.isConnected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()
	
	operation := &Operation{
		Type:        OperationTypeGet,
		Key:         key,
		Consistency: consistency,
		Timeout:     c.config.RequestTimeout,
		Metadata:    make(map[string]string),
		StartTime:   time.Now(),
	}
	
	// Apply operation options
	for _, opt := range opts {
		opt(operation)
	}
	
	// Execute operation with retry logic
	result, err := c.retryManager.ExecuteWithRetry(ctx, func() (interface{}, error) {
		return c.executeGetOperation(ctx, operation)
	})
	
	if err != nil {
		c.metrics.RecordError("get", err)
		return nil, err
	}
	
	getResult := result.(*AdvancedGetResult)
	c.metrics.RecordOperation("get", time.Since(operation.StartTime))
	c.lastActivity = time.Now()
	
	return getResult, nil
}

// Put stores a key-value pair
func (c *AdvancedClient) Put(ctx context.Context, key string, value []byte, opts ...OperationOption) (*PutResult, error) {
	return c.PutWithConsistency(ctx, key, value, c.config.DefaultConsistency, opts...)
}

// PutWithConsistency stores a key-value pair with specified consistency
func (c *AdvancedClient) PutWithConsistency(ctx context.Context, key string, value []byte, consistency ConsistencyLevel, opts ...OperationOption) (*PutResult, error) {
	c.mu.RLock()
	if !c.isConnected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()
	
	operation := &Operation{
		Type:        OperationTypePut,
		Key:         key,
		Value:       value,
		Consistency: consistency,
		Timeout:     c.config.RequestTimeout,
		Metadata:    make(map[string]string),
		StartTime:   time.Now(),
	}
	
	// Apply operation options
	for _, opt := range opts {
		opt(operation)
	}
	
	// Execute operation with retry logic
	result, err := c.retryManager.ExecuteWithRetry(ctx, func() (interface{}, error) {
		return c.executePutOperation(ctx, operation)
	})
	
	if err != nil {
		c.metrics.RecordError("put", err)
		return nil, err
	}
	
	putResult := result.(*PutResult)
	c.metrics.RecordOperation("put", time.Since(operation.StartTime))
	c.lastActivity = time.Now()
	
	// Update read-your-writes tracking if enabled
	if c.config.ReadYourWritesEnabled {
		c.consistencyMgr.TrackWrite(key, putResult.Version)
	}
	
	return putResult, nil
}

// Delete removes a key-value pair
func (c *AdvancedClient) Delete(ctx context.Context, key string, opts ...OperationOption) (*AdvancedDeleteResult, error) {
	c.mu.RLock()
	if !c.isConnected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()
	
	operation := &Operation{
		Type:        OperationTypeDelete,
		Key:         key,
		Consistency: c.config.DefaultConsistency,
		Timeout:     c.config.RequestTimeout,
		Metadata:    make(map[string]string),
		StartTime:   time.Now(),
	}
	
	// Apply operation options
	for _, opt := range opts {
		opt(operation)
	}
	
	// Execute operation with retry logic
	result, err := c.retryManager.ExecuteWithRetry(ctx, func() (interface{}, error) {
		return c.executeDeleteOperation(ctx, operation)
	})
	
	if err != nil {
		c.metrics.RecordError("delete", err)
		return nil, err
	}
	
	deleteResult := result.(*AdvancedDeleteResult)
	c.metrics.RecordOperation("delete", time.Since(operation.StartTime))
	c.lastActivity = time.Now()
	
	return deleteResult, nil
}

// executeGetOperation executes a GET operation
func (c *AdvancedClient) executeGetOperation(ctx context.Context, op *Operation) (*AdvancedGetResult, error) {
	// Apply circuit breaker
	if c.config.CircuitBreakerEnabled {
		if !c.circuitBreaker.CanExecute() {
			return nil, fmt.Errorf("circuit breaker is open")
		}
	}
	
	// Select appropriate nodes based on consistency level
	nodes, err := c.selectNodesForRead(op.Key, op.Consistency)
	if err != nil {
		return nil, fmt.Errorf("failed to select nodes: %w", err)
	}
	
	// Execute read operation
	result, err := c.consistencyMgr.ExecuteRead(ctx, op, nodes, c.connectionPool)
	
	// Update circuit breaker
	if c.config.CircuitBreakerEnabled {
		if err != nil {
			c.circuitBreaker.RecordFailure()
		} else {
			c.circuitBreaker.RecordSuccess()
		}
	}
	
	return result, err
}

// executePutOperation executes a PUT operation
func (c *AdvancedClient) executePutOperation(ctx context.Context, op *Operation) (*PutResult, error) {
	// Apply circuit breaker
	if c.config.CircuitBreakerEnabled {
		if !c.circuitBreaker.CanExecute() {
			return nil, fmt.Errorf("circuit breaker is open")
		}
	}
	
	// Select leader node for write operations
	leaderNode, err := c.leaderDiscovery.GetLeaderForKey(op.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to get leader: %w", err)
	}
	
	// Execute write operation
	result, err := c.consistencyMgr.ExecuteWrite(ctx, op, leaderNode, c.connectionPool)
	
	// Update circuit breaker
	if c.config.CircuitBreakerEnabled {
		if err != nil {
			c.circuitBreaker.RecordFailure()
		} else {
			c.circuitBreaker.RecordSuccess()
		}
	}
	
	return result, err
}

// executeDeleteOperation executes a DELETE operation
func (c *AdvancedClient) executeDeleteOperation(ctx context.Context, op *Operation) (*AdvancedDeleteResult, error) {
	// Apply circuit breaker
	if c.config.CircuitBreakerEnabled {
		if !c.circuitBreaker.CanExecute() {
			return nil, fmt.Errorf("circuit breaker is open")
		}
	}
	
	// Select leader node for write operations
	leaderNode, err := c.leaderDiscovery.GetLeaderForKey(op.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to get leader: %w", err)
	}
	
	// Execute delete operation
	result, err := c.consistencyMgr.ExecuteDelete(ctx, op, leaderNode, c.connectionPool)
	
	// Update circuit breaker
	if c.config.CircuitBreakerEnabled {
		if err != nil {
			c.circuitBreaker.RecordFailure()
		} else {
			c.circuitBreaker.RecordSuccess()
		}
	}
	
	return result, err
}

// selectNodesForRead selects appropriate nodes for read operations
func (c *AdvancedClient) selectNodesForRead(key string, consistency ConsistencyLevel) ([]*NodeInfo, error) {
	switch consistency {
	case ConsistencyLinearizable:
		// Strong consistency - read from leader only
		leader, err := c.leaderDiscovery.GetLeaderForKey(key)
		if err != nil {
			return nil, err
		}
		return []*NodeInfo{leader}, nil
		
	case ConsistencyEventual:
		// Eventual consistency - can read from any replica
		return c.loadBalancer.SelectNodes(key, 1)
		
	case ConsistencyBounded:
		// Bounded staleness - prefer recent replicas
		return c.loadBalancer.SelectRecentNodes(key, 1, c.config.StaleReadThreshold)
		
	case ConsistencySession, ConsistencyReadYourWrite:
		// Session consistency - check if we need to read from leader
		if c.config.ReadYourWritesEnabled && c.consistencyMgr.RequiresLeaderRead(key) {
			leader, err := c.leaderDiscovery.GetLeaderForKey(key)
			if err != nil {
				return nil, err
			}
			return []*NodeInfo{leader}, nil
		}
		return c.loadBalancer.SelectNodes(key, 1)
		
	default:
		return nil, fmt.Errorf("unsupported consistency level: %s", consistency)
	}
}

// healthCheckLoop periodically checks node health
func (c *AdvancedClient) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(c.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.performHealthCheck(ctx)
		}
	}
}

// performHealthCheck checks the health of all nodes
func (c *AdvancedClient) performHealthCheck(ctx context.Context) {
	nodes := c.connectionPool.GetAllNodes()
	
	for _, node := range nodes {
		go func(n *NodeInfo) {
			if err := c.connectionPool.CheckNodeHealth(ctx, n); err != nil {
				c.logger.Printf("Node %s health check failed: %v", n.ID, err)
				c.loadBalancer.MarkNodeUnhealthy(n.ID)
			} else {
				c.loadBalancer.MarkNodeHealthy(n.ID)
			}
		}(node)
	}
}

// metricsCollectionLoop periodically collects metrics
func (c *AdvancedClient) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(c.config.MetricsCollectionInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collectMetrics()
		}
	}
}

// collectMetrics collects and logs client metrics
func (c *AdvancedClient) collectMetrics() {
	stats := c.GetStats()
	c.logger.Printf("Advanced client metrics: %+v", stats)
}

// GetStats returns client statistics
func (c *AdvancedClient) GetStats() AdvancedClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return AdvancedClientStats{
		IsConnected:          c.isConnected,
		LastActivity:         c.lastActivity,
		ConnectionPoolStats:  c.connectionPool.GetStats(),
		LoadBalancerStats:    c.loadBalancer.GetStats(),
		CircuitBreakerStats:  c.circuitBreaker.GetStats(),
		ConsistencyStats:     c.consistencyMgr.GetStats(),
		OperationMetrics:     c.metrics.GetMetrics(),
	}
}

// AdvancedClientStats contains statistics about the advanced client
type AdvancedClientStats struct {
	IsConnected         bool                      `json:"is_connected"`
	LastActivity        time.Time                 `json:"last_activity"`
	ConnectionPoolStats ConnectionPoolStats       `json:"connection_pool_stats"`
	LoadBalancerStats   LoadBalancerStats        `json:"load_balancer_stats"`
	CircuitBreakerStats CircuitBreakerStats      `json:"circuit_breaker_stats"`
	ConsistencyStats    ConsistencyManagerStats  `json:"consistency_stats"`
	OperationMetrics    map[string]OperationMetrics `json:"operation_metrics"`
}