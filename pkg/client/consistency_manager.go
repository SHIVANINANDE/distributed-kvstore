package client

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// ConsistencyManager handles different consistency levels for operations
type ConsistencyManager struct {
	mu                    sync.RWMutex
	config                AdvancedClientConfig
	logger                *log.Logger
	
	// Read-your-writes tracking
	writeTimestamps       map[string]time.Time    // key -> last write timestamp
	writeVersions         map[string]uint64       // key -> last write version
	sessionToken          string                  // session token for session consistency
	
	// Statistics
	linearizableReads     int64
	eventualReads         int64
	boundedReads          int64
	sessionReads          int64
	readYourWriteReads    int64
	
	// Consistency violations detected
	staleReads            int64
	inconsistentReads     int64
}

// ConsistencyManagerStats contains statistics about consistency operations
type ConsistencyManagerStats struct {
	LinearizableReads     int64             `json:"linearizable_reads"`
	EventualReads         int64             `json:"eventual_reads"`
	BoundedReads          int64             `json:"bounded_reads"`
	SessionReads          int64             `json:"session_reads"`
	ReadYourWriteReads    int64             `json:"read_your_write_reads"`
	StaleReads            int64             `json:"stale_reads"`
	InconsistentReads     int64             `json:"inconsistent_reads"`
	TrackedKeys           int               `json:"tracked_keys"`
	SessionToken          string            `json:"session_token"`
}

// NewConsistencyManager creates a new consistency manager
func NewConsistencyManager(config AdvancedClientConfig, logger *log.Logger) *ConsistencyManager {
	if logger == nil {
		logger = log.New(log.Writer(), "[CONSISTENCY_MGR] ", log.LstdFlags)
	}
	
	return &ConsistencyManager{
		config:          config,
		logger:          logger,
		writeTimestamps: make(map[string]time.Time),
		writeVersions:   make(map[string]uint64),
		sessionToken:    generateSessionToken(),
	}
}

// ExecuteRead executes a read operation with the specified consistency level
func (cm *ConsistencyManager) ExecuteRead(ctx context.Context, op *Operation, nodes []*NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	switch op.Consistency {
	case ConsistencyLinearizable:
		return cm.executeLinearizableRead(ctx, op, nodes, pool)
	case ConsistencyEventual:
		return cm.executeEventualRead(ctx, op, nodes, pool)
	case ConsistencyBounded:
		return cm.executeBoundedRead(ctx, op, nodes, pool)
	case ConsistencySession:
		return cm.executeSessionRead(ctx, op, nodes, pool)
	case ConsistencyReadYourWrite:
		return cm.executeReadYourWriteRead(ctx, op, nodes, pool)
	default:
		return nil, fmt.Errorf("unsupported consistency level: %s", op.Consistency)
	}
}

// ExecuteWrite executes a write operation
func (cm *ConsistencyManager) ExecuteWrite(ctx context.Context, op *Operation, leader *NodeInfo, pool *AdvancedConnectionPool) (*PutResult, error) {
	// All writes go through the leader for strong consistency
	result, err := cm.executeWriteOperation(ctx, op, leader, pool)
	if err != nil {
		return nil, err
	}
	
	// Track write for read-your-writes consistency
	if cm.config.ReadYourWritesEnabled {
		cm.TrackWrite(op.Key, result.Version)
	}
	
	return result, nil
}

// ExecuteDelete executes a delete operation
func (cm *ConsistencyManager) ExecuteDelete(ctx context.Context, op *Operation, leader *NodeInfo, pool *AdvancedConnectionPool) (*DeleteResult, error) {
	// All deletes go through the leader for strong consistency
	return cm.executeDeleteOperation(ctx, op, leader, pool)
}

// executeLinearizableRead executes a linearizable (strong consistency) read
func (cm *ConsistencyManager) executeLinearizableRead(ctx context.Context, op *Operation, nodes []*NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	cm.mu.Lock()
	cm.linearizableReads++
	cm.mu.Unlock()
	
	// Linearizable reads must go through the leader
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available for linearizable read")
	}
	
	leader := nodes[0] // First node should be the leader
	result, err := cm.performRead(ctx, op, leader, pool)
	if err != nil {
		return nil, fmt.Errorf("linearizable read failed: %w", err)
	}
	
	cm.logger.Printf("Linearizable read executed for key %s", op.Key)
	return result, nil
}

// executeEventualRead executes an eventually consistent read
func (cm *ConsistencyManager) executeEventualRead(ctx context.Context, op *Operation, nodes []*NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	cm.mu.Lock()
	cm.eventualReads++
	cm.mu.Unlock()
	
	// Eventual consistency can read from any replica
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available for eventual read")
	}
	
	// Try nodes in order until one succeeds
	var lastErr error
	for _, node := range nodes {
		result, err := cm.performRead(ctx, op, node, pool)
		if err == nil {
			cm.logger.Printf("Eventual read executed for key %s from node %s", op.Key, node.ID)
			return result, nil
		}
		lastErr = err
	}
	
	return nil, fmt.Errorf("eventual read failed from all nodes: %w", lastErr)
}

// executeBoundedRead executes a bounded staleness read
func (cm *ConsistencyManager) executeBoundedRead(ctx context.Context, op *Operation, nodes []*NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	cm.mu.Lock()
	cm.boundedReads++
	cm.mu.Unlock()
	
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available for bounded read")
	}
	
	// Try to read from recent replicas first, fall back to leader if too stale
	now := time.Now()
	staleThreshold := cm.config.StaleReadThreshold
	
	for _, node := range nodes {
		if now.Sub(node.LastSeen) <= staleThreshold {
			result, err := cm.performRead(ctx, op, node, pool)
			if err == nil {
				// Check if the result is within staleness bounds
				if cm.isResultWithinBounds(result, staleThreshold) {
					cm.logger.Printf("Bounded read executed for key %s from node %s", op.Key, node.ID)
					return result, nil
				} else {
					cm.recordStaleRead()
				}
			}
		}
	}
	
	// Fall back to linearizable read if all replicas are too stale
	return cm.executeLinearizableRead(ctx, op, nodes, pool)
}

// executeSessionRead executes a session consistent read
func (cm *ConsistencyManager) executeSessionRead(ctx context.Context, op *Operation, nodes []*NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	cm.mu.Lock()
	cm.sessionReads++
	cm.mu.Unlock()
	
	// Session consistency ensures reads see all writes from the same session
	// This can be implemented by including session token in read requests
	op.Metadata["session_token"] = cm.sessionToken
	
	// For simplicity, route to any available replica
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available for session read")
	}
	
	result, err := cm.performRead(ctx, op, nodes[0], pool)
	if err != nil {
		return nil, fmt.Errorf("session read failed: %w", err)
	}
	
	cm.logger.Printf("Session read executed for key %s", op.Key)
	return result, nil
}

// executeReadYourWriteRead executes a read-your-writes consistent read
func (cm *ConsistencyManager) executeReadYourWriteRead(ctx context.Context, op *Operation, nodes []*NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	cm.mu.Lock()
	cm.readYourWriteReads++
	cm.mu.Unlock()
	
	// Check if we need to read from leader to see our own writes
	if cm.RequiresLeaderRead(op.Key) {
		// Must read from leader to see our own writes
		if len(nodes) == 0 {
			return nil, fmt.Errorf("no leader available for read-your-writes")
		}
		
		result, err := cm.performRead(ctx, op, nodes[0], pool) // First node should be leader
		if err != nil {
			return nil, fmt.Errorf("read-your-writes failed: %w", err)
		}
		
		cm.logger.Printf("Read-your-writes read executed for key %s from leader", op.Key)
		return result, nil
	}
	
	// Can read from any replica if we haven't written to this key recently
	return cm.executeEventualRead(ctx, op, nodes, pool)
}

// performRead performs the actual read operation
func (cm *ConsistencyManager) performRead(ctx context.Context, op *Operation, node *NodeInfo, pool *AdvancedConnectionPool) (*AdvancedGetResult, error) {
	// In a real implementation, this would make an RPC call to the node
	// For now, we'll simulate it
	
	time.Sleep(5 * time.Millisecond) // Simulate network latency
	
	// Simulate read result
	baseResult := &GetResult{
		Key:       op.Key,
		Found:     true,
		Value:     []byte(fmt.Sprintf("value_for_%s_from_%s", op.Key, node.ID)),
		CreatedAt: time.Now().Add(-time.Hour), // Simulate creation time
		ExpiresAt: time.Time{}, // No expiration
	}
	
	result := &AdvancedGetResult{
		GetResult: baseResult,
		Version:   uint64(time.Now().UnixNano()),
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"node_id":     node.ID,
			"consistency": string(op.Consistency),
		},
	}
	
	return result, nil
}

// executeWriteOperation performs the actual write operation
func (cm *ConsistencyManager) executeWriteOperation(ctx context.Context, op *Operation, leader *NodeInfo, pool *AdvancedConnectionPool) (*PutResult, error) {
	// In a real implementation, this would make an RPC call to the leader
	// For now, we'll simulate it
	
	time.Sleep(10 * time.Millisecond) // Simulate write latency
	
	version := uint64(time.Now().UnixNano())
	result := &PutResult{
		Success:   true,
		Version:   version,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"leader_id": leader.ID,
		},
	}
	
	return result, nil
}

// executeDeleteOperation performs the actual delete operation
func (cm *ConsistencyManager) executeDeleteOperation(ctx context.Context, op *Operation, leader *NodeInfo, pool *AdvancedConnectionPool) (*AdvancedDeleteResult, error) {
	// In a real implementation, this would make an RPC call to the leader
	// For now, we'll simulate it
	
	time.Sleep(8 * time.Millisecond) // Simulate delete latency
	
	baseResult := &DeleteResult{
		Existed: true,
	}
	
	result := &AdvancedDeleteResult{
		DeleteResult: baseResult,
		Success:      true,
		Timestamp:    time.Now(),
		Metadata: map[string]string{
			"leader_id": leader.ID,
		},
	}
	
	return result, nil
}

// TrackWrite tracks a write operation for read-your-writes consistency
func (cm *ConsistencyManager) TrackWrite(key string, version uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.writeTimestamps[key] = time.Now()
	cm.writeVersions[key] = version
	
	cm.logger.Printf("Tracking write for key %s, version %d", key, version)
}

// RequiresLeaderRead checks if a key requires reading from leader
func (cm *ConsistencyManager) RequiresLeaderRead(key string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	writeTime, exists := cm.writeTimestamps[key]
	if !exists {
		return false
	}
	
	// Require leader read if we've written to this key recently
	timeSinceWrite := time.Since(writeTime)
	threshold := cm.config.StaleReadThreshold
	if threshold == 0 {
		threshold = 5 * time.Second // Default threshold
	}
	
	return timeSinceWrite <= threshold
}

// isResultWithinBounds checks if a read result is within staleness bounds
func (cm *ConsistencyManager) isResultWithinBounds(result *AdvancedGetResult, threshold time.Duration) bool {
	// Check if the result timestamp is within the staleness threshold
	timeSinceResult := time.Since(result.Timestamp)
	return timeSinceResult <= threshold
}

// recordStaleRead records when a stale read is detected
func (cm *ConsistencyManager) recordStaleRead() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.staleReads++
}

// recordInconsistentRead records when an inconsistent read is detected
func (cm *ConsistencyManager) recordInconsistentRead() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.inconsistentReads++
}

// UpdateConfig updates the consistency manager configuration
func (cm *ConsistencyManager) UpdateConfig(config AdvancedClientConfig) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.config = config
	cm.logger.Printf("Consistency manager configuration updated")
}

// GetStats returns consistency manager statistics
func (cm *ConsistencyManager) GetStats() ConsistencyManagerStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return ConsistencyManagerStats{
		LinearizableReads:  cm.linearizableReads,
		EventualReads:      cm.eventualReads,
		BoundedReads:       cm.boundedReads,
		SessionReads:       cm.sessionReads,
		ReadYourWriteReads: cm.readYourWriteReads,
		StaleReads:         cm.staleReads,
		InconsistentReads:  cm.inconsistentReads,
		TrackedKeys:        len(cm.writeTimestamps),
		SessionToken:       cm.sessionToken,
	}
}

// Reset resets consistency manager statistics
func (cm *ConsistencyManager) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.linearizableReads = 0
	cm.eventualReads = 0
	cm.boundedReads = 0
	cm.sessionReads = 0
	cm.readYourWriteReads = 0
	cm.staleReads = 0
	cm.inconsistentReads = 0
	
	// Clear write tracking
	cm.writeTimestamps = make(map[string]time.Time)
	cm.writeVersions = make(map[string]uint64)
	
	cm.logger.Printf("Consistency manager statistics reset")
}

// ClearWriteTracking clears write tracking for read-your-writes
func (cm *ConsistencyManager) ClearWriteTracking() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.writeTimestamps = make(map[string]time.Time)
	cm.writeVersions = make(map[string]uint64)
	
	cm.logger.Printf("Write tracking cleared")
}

// generateSessionToken generates a unique session token
func generateSessionToken() string {
	return fmt.Sprintf("session_%d_%d", time.Now().UnixNano(), rand.Int63())
}

// GetSessionToken returns the current session token
func (cm *ConsistencyManager) GetSessionToken() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.sessionToken
}

// RenewSession generates a new session token
func (cm *ConsistencyManager) RenewSession() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.sessionToken = generateSessionToken()
	cm.logger.Printf("Session renewed with token: %s", cm.sessionToken)
}