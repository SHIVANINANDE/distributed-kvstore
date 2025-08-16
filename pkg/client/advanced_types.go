package client

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Operation types
const (
	OperationTypeGet    = "GET"
	OperationTypePut    = "PUT"
	OperationTypeDelete = "DELETE"
)

// Operation represents a client operation
type Operation struct {
	Type        string                 `json:"type"`
	Key         string                 `json:"key"`
	Value       []byte                 `json:"value,omitempty"`
	Consistency ConsistencyLevel       `json:"consistency"`
	Timeout     time.Duration          `json:"timeout"`
	Metadata    map[string]string      `json:"metadata"`
	StartTime   time.Time              `json:"start_time"`
}

// OperationOption allows customizing operations
type OperationOption func(*Operation)

// WithTimeout sets the timeout for an operation
func WithTimeout(timeout time.Duration) OperationOption {
	return func(op *Operation) {
		op.Timeout = timeout
	}
}

// WithConsistency sets the consistency level for an operation
func WithConsistency(consistency ConsistencyLevel) OperationOption {
	return func(op *Operation) {
		op.Consistency = consistency
	}
}

// WithMetadata adds metadata to an operation
func WithMetadata(key, value string) OperationOption {
	return func(op *Operation) {
		if op.Metadata == nil {
			op.Metadata = make(map[string]string)
		}
		op.Metadata[key] = value
	}
}

// AdvancedGetResult extends GetResult with additional fields for advanced client
type AdvancedGetResult struct {
	*GetResult
	Version   uint64            `json:"version"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// PutResult represents the result of a PUT operation
type PutResult struct {
	Success   bool              `json:"success"`
	Version   uint64            `json:"version"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AdvancedDeleteResult extends DeleteResult with additional fields for advanced client
type AdvancedDeleteResult struct {
	*DeleteResult
	Success   bool              `json:"success"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// OperationMetrics contains metrics for operations
type OperationMetrics struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessfulReqs  int64         `json:"successful_requests"`
	FailedRequests  int64         `json:"failed_requests"`
	AverageLatency  time.Duration `json:"average_latency"`
	MinLatency      time.Duration `json:"min_latency"`
	MaxLatency      time.Duration `json:"max_latency"`
	P95Latency      time.Duration `json:"p95_latency"`
	P99Latency      time.Duration `json:"p99_latency"`
	ErrorRate       float64       `json:"error_rate"`
}

// ClientMetrics tracks client-wide metrics
type ClientMetrics struct {
	mu                 sync.RWMutex
	operationMetrics   map[string]*OperationStats
	errorCounts        map[string]int64
	totalOperations    int64
	startTime          time.Time
}

// OperationStats tracks statistics for a specific operation type
type OperationStats struct {
	count     int64
	errors    int64
	latencies []time.Duration
	lastOp    time.Time
}

// NewClientMetrics creates a new client metrics instance
func NewClientMetrics() *ClientMetrics {
	return &ClientMetrics{
		operationMetrics: make(map[string]*OperationStats),
		errorCounts:      make(map[string]int64),
		startTime:        time.Now(),
	}
}

// RecordOperation records a successful operation
func (cm *ClientMetrics) RecordOperation(opType string, latency time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if _, exists := cm.operationMetrics[opType]; !exists {
		cm.operationMetrics[opType] = &OperationStats{
			latencies: make([]time.Duration, 0),
		}
	}
	
	stats := cm.operationMetrics[opType]
	stats.count++
	stats.latencies = append(stats.latencies, latency)
	stats.lastOp = time.Now()
	cm.totalOperations++
}

// RecordError records an error for an operation
func (cm *ClientMetrics) RecordError(opType string, err error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if _, exists := cm.operationMetrics[opType]; !exists {
		cm.operationMetrics[opType] = &OperationStats{
			latencies: make([]time.Duration, 0),
		}
	}
	
	stats := cm.operationMetrics[opType]
	stats.errors++
	cm.errorCounts[err.Error()]++
	cm.totalOperations++
}

// GetMetrics returns operation metrics
func (cm *ClientMetrics) GetMetrics() map[string]OperationMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	result := make(map[string]OperationMetrics)
	
	for opType, stats := range cm.operationMetrics {
		metrics := OperationMetrics{
			TotalRequests:  stats.count + stats.errors,
			SuccessfulReqs: stats.count,
			FailedRequests: stats.errors,
		}
		
		if len(stats.latencies) > 0 {
			total := time.Duration(0)
			min := stats.latencies[0]
			max := stats.latencies[0]
			
			for _, latency := range stats.latencies {
				total += latency
				if latency < min {
					min = latency
				}
				if latency > max {
					max = latency
				}
			}
			
			metrics.AverageLatency = total / time.Duration(len(stats.latencies))
			metrics.MinLatency = min
			metrics.MaxLatency = max
			
			// Calculate percentiles (simplified)
			if len(stats.latencies) >= 20 {
				p95Idx := int(float64(len(stats.latencies)) * 0.95)
				p99Idx := int(float64(len(stats.latencies)) * 0.99)
				metrics.P95Latency = stats.latencies[p95Idx]
				metrics.P99Latency = stats.latencies[p99Idx]
			}
		}
		
		if metrics.TotalRequests > 0 {
			metrics.ErrorRate = float64(metrics.FailedRequests) / float64(metrics.TotalRequests)
		}
		
		result[opType] = metrics
	}
	
	return result
}

// Reset resets all metrics
func (cm *ClientMetrics) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.operationMetrics = make(map[string]*OperationStats)
	cm.errorCounts = make(map[string]int64)
	cm.totalOperations = 0
	cm.startTime = time.Now()
}

// Connection represents a connection to a node
type Connection struct {
	NodeID     string
	Address    string
	Port       int
	IsHealthy  bool
	LastUsed   time.Time
	CreatedAt  time.Time
}

// ConnectionPoolStats contains statistics about the connection pool
type ConnectionPoolStats struct {
	TotalConnections    int               `json:"total_connections"`
	ActiveConnections   int               `json:"active_connections"`
	IdleConnections     int               `json:"idle_connections"`
	FailedConnections   int               `json:"failed_connections"`
	ConnectionsByNode   map[string]int    `json:"connections_by_node"`
	AverageLatency      time.Duration     `json:"average_latency"`
	ConnectionErrors    int64             `json:"connection_errors"`
}

// Advanced Connection Pool (placeholder for now)
type AdvancedConnectionPool struct {
	mu          sync.RWMutex
	connections map[string][]*Connection
	config      AdvancedClientConfig
	logger      *log.Logger
	nodes       map[string]*NodeInfo
}

// NewAdvancedConnectionPool creates a new advanced connection pool
func NewAdvancedConnectionPool(config AdvancedClientConfig, logger *log.Logger) *AdvancedConnectionPool {
	return &AdvancedConnectionPool{
		connections: make(map[string][]*Connection),
		config:      config,
		logger:      logger,
		nodes:       make(map[string]*NodeInfo),
	}
}

// Initialize initializes the connection pool with initial nodes
func (pool *AdvancedConnectionPool) Initialize(ctx context.Context, nodes []string) error {
	// Implementation placeholder
	pool.logger.Printf("Initializing connection pool with %d nodes", len(nodes))
	
	for i, nodeAddr := range nodes {
		nodeID := fmt.Sprintf("node_%d", i)
		node := &NodeInfo{
			ID:        nodeID,
			Address:   nodeAddr,
			Port:      9090, // Default port
			IsHealthy: true,
			LastSeen:  time.Now(),
			Weight:    1,
			Metadata:  make(map[string]string),
		}
		pool.nodes[nodeID] = node
	}
	
	return nil
}

// GetAllNodes returns all nodes in the pool
func (pool *AdvancedConnectionPool) GetAllNodes() []*NodeInfo {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	
	var nodes []*NodeInfo
	for _, node := range pool.nodes {
		nodeCopy := *node
		nodes = append(nodes, &nodeCopy)
	}
	
	return nodes
}

// CheckNodeHealth checks the health of a specific node
func (pool *AdvancedConnectionPool) CheckNodeHealth(ctx context.Context, node *NodeInfo) error {
	// Implementation placeholder
	time.Sleep(5 * time.Millisecond) // Simulate health check
	return nil
}

// Close closes the connection pool
func (pool *AdvancedConnectionPool) Close() error {
	pool.logger.Printf("Closing connection pool")
	return nil
}

// GetStats returns connection pool statistics
func (pool *AdvancedConnectionPool) GetStats() ConnectionPoolStats {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	
	connectionsByNode := make(map[string]int)
	totalConnections := 0
	
	for nodeID, conns := range pool.connections {
		connectionsByNode[nodeID] = len(conns)
		totalConnections += len(conns)
	}
	
	return ConnectionPoolStats{
		TotalConnections:  totalConnections,
		ActiveConnections: totalConnections, // Simplified
		ConnectionsByNode: connectionsByNode,
	}
}