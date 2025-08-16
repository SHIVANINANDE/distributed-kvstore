package partitioning

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Rebalancer handles automatic rebalancing of data across nodes
type Rebalancer struct {
	mu                sync.RWMutex
	hashRing          *HashRing
	shardManager      *ShardManager
	operations        map[string]*RebalanceOperation
	config            RebalancerConfig
	logger            *log.Logger
	stopCh            chan struct{}
	operationCh       chan *RebalanceOperation
	isRunning         bool
}

// RebalancerConfig contains configuration for the rebalancer
type RebalancerConfig struct {
	// Rebalancing thresholds
	LoadThreshold        float64       `json:"load_threshold"`         // Trigger rebalancing if load imbalance > threshold
	DistributionThreshold float64      `json:"distribution_threshold"` // Trigger rebalancing if key distribution imbalance > threshold
	
	// Timing configuration
	CheckInterval        time.Duration `json:"check_interval"`         // How often to check for imbalance
	RebalanceTimeout     time.Duration `json:"rebalance_timeout"`      // Timeout for rebalance operations
	BatchSize           int           `json:"batch_size"`             // Number of keys to move in one batch
	
	// Safety limits
	MaxConcurrentOps    int           `json:"max_concurrent_ops"`     // Maximum concurrent rebalance operations
	MinNodes            int           `json:"min_nodes"`              // Minimum nodes required for rebalancing
	MaxMovePercentage   float64       `json:"max_move_percentage"`    // Maximum percentage of data to move at once
	
	// Feature flags
	EnableAutoRebalance bool          `json:"enable_auto_rebalance"`  // Enable automatic rebalancing
	EnableLoadBalancing bool          `json:"enable_load_balancing"`  // Enable load-based rebalancing
}

// NewRebalancer creates a new rebalancer
func NewRebalancer(hashRing *HashRing, shardManager *ShardManager, config RebalancerConfig, logger *log.Logger) *Rebalancer {
	if logger == nil {
		logger = log.New(log.Writer(), "[REBALANCER] ", log.LstdFlags)
	}

	// Set default configuration values
	if config.LoadThreshold == 0 {
		config.LoadThreshold = 20.0 // 20% load imbalance threshold
	}
	if config.DistributionThreshold == 0 {
		config.DistributionThreshold = 10.0 // 10% distribution imbalance threshold
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 30 * time.Second
	}
	if config.RebalanceTimeout == 0 {
		config.RebalanceTimeout = 5 * time.Minute
	}
	if config.BatchSize == 0 {
		config.BatchSize = 1000
	}
	if config.MaxConcurrentOps == 0 {
		config.MaxConcurrentOps = 3
	}
	if config.MinNodes == 0 {
		config.MinNodes = 2
	}
	if config.MaxMovePercentage == 0 {
		config.MaxMovePercentage = 25.0 // Don't move more than 25% of data at once
	}

	return &Rebalancer{
		hashRing:     hashRing,
		shardManager: shardManager,
		operations:   make(map[string]*RebalanceOperation),
		config:       config,
		logger:       logger,
		stopCh:       make(chan struct{}),
		operationCh:  make(chan *RebalanceOperation, 100),
	}
}

// Start starts the rebalancer
func (r *Rebalancer) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRunning {
		return fmt.Errorf("rebalancer is already running")
	}

	r.logger.Printf("Starting rebalancer with auto-rebalance: %v", r.config.EnableAutoRebalance)
	r.isRunning = true

	// Start monitoring goroutine
	go r.monitorLoop()

	// Start operation processing goroutine
	go r.processOperations()

	return nil
}

// Stop stops the rebalancer
func (r *Rebalancer) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRunning {
		return
	}

	r.logger.Printf("Stopping rebalancer")
	close(r.stopCh)
	r.isRunning = false
}

// monitorLoop continuously monitors for rebalancing needs
func (r *Rebalancer) monitorLoop() {
	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			if r.config.EnableAutoRebalance {
				r.checkAndTriggerRebalance()
			}
		}
	}
}

// checkAndTriggerRebalance checks if rebalancing is needed and triggers it
func (r *Rebalancer) checkAndTriggerRebalance() {
	r.mu.RLock()
	activeOps := len(r.operations)
	r.mu.RUnlock()

	// Don't start new rebalancing if we have too many concurrent operations
	if activeOps >= r.config.MaxConcurrentOps {
		return
	}

	// Check if we have enough nodes
	stats := r.hashRing.GetStats()
	if stats.ActiveNodes < r.config.MinNodes {
		return
	}

	// Check load distribution
	if r.config.EnableLoadBalancing && r.needsLoadRebalancing() {
		r.logger.Printf("Load imbalance detected, triggering rebalancing")
		r.triggerLoadRebalancing()
	}

	// Check key distribution
	if r.needsKeyRebalancing() {
		r.logger.Printf("Key distribution imbalance detected, triggering rebalancing")
		r.triggerKeyRebalancing()
	}
}

// needsLoadRebalancing checks if load-based rebalancing is needed
func (r *Rebalancer) needsLoadRebalancing() bool {
	distribution := r.hashRing.GetLoadDistribution()
	if len(distribution) < 2 {
		return false
	}

	var min, max float64
	first := true
	for _, load := range distribution {
		if first {
			min, max = load, load
			first = false
		} else {
			if load < min {
				min = load
			}
			if load > max {
				max = load
			}
		}
	}

	imbalance := max - min
	return imbalance > r.config.LoadThreshold
}

// needsKeyRebalancing checks if key distribution rebalancing is needed
func (r *Rebalancer) needsKeyRebalancing() bool {
	return !r.hashRing.IsBalanced(r.config.DistributionThreshold)
}

// triggerLoadRebalancing creates operations to rebalance load
func (r *Rebalancer) triggerLoadRebalancing() {
	distribution := r.hashRing.GetLoadDistribution()
	nodes := r.hashRing.GetAllNodes()

	// Find the most loaded and least loaded nodes
	var maxLoadNode, minLoadNode string
	var maxLoad, minLoad float64
	first := true

	for nodeID, load := range distribution {
		if node, exists := nodes[nodeID]; exists && node.Status == NodeStatusActive {
			if first {
				maxLoadNode, minLoadNode = nodeID, nodeID
				maxLoad, minLoad = load, load
				first = false
			} else {
				if load > maxLoad {
					maxLoadNode = nodeID
					maxLoad = load
				}
				if load < minLoad {
					minLoadNode = nodeID
					minLoad = load
				}
			}
		}
	}

	// Create rebalance operation if imbalance is significant
	if maxLoad-minLoad > r.config.LoadThreshold && maxLoadNode != minLoadNode {
		operation := &RebalanceOperation{
			Type:       RebalanceTypeRepair,
			SourceNode: maxLoadNode,
			TargetNode: minLoadNode,
			Status:     "pending",
			Progress:   0.0,
		}

		r.scheduleOperation(operation)
	}
}

// triggerKeyRebalancing creates operations to rebalance key distribution
func (r *Rebalancer) triggerKeyRebalancing() {
	// For key distribution rebalancing, we need to analyze the hash ring
	// and determine if virtual nodes need to be redistributed
	// This is a simplified implementation
	
	distribution := r.hashRing.GetKeyDistribution()
	expectedPercentage := 100.0 / float64(len(distribution))

	for nodeID, percentage := range distribution {
		deviation := abs(percentage - expectedPercentage)
		if deviation > r.config.DistributionThreshold {
			r.logger.Printf("Node %s has %.2f%% deviation from expected %.2f%%", 
				nodeID, deviation, expectedPercentage)
			
			// In a production system, this would trigger virtual node redistribution
			// For now, we'll log the imbalance
		}
	}
}

// OnNodeAdded handles when a new node is added to the cluster
func (r *Rebalancer) OnNodeAdded(nodeID string) {
	r.logger.Printf("Node %s added, scheduling rebalancing", nodeID)

	// Create operation to move data to the new node
	operation := &RebalanceOperation{
		Type:       RebalanceTypeAdd,
		TargetNode: nodeID,
		Status:     "pending",
		Progress:   0.0,
	}

	r.scheduleOperation(operation)
}

// OnNodeRemoved handles when a node is removed from the cluster
func (r *Rebalancer) OnNodeRemoved(nodeID string) {
	r.logger.Printf("Node %s removed, scheduling rebalancing", nodeID)

	// Create operation to move data from the removed node
	operation := &RebalanceOperation{
		Type:       RebalanceTypeRemove,
		SourceNode: nodeID,
		Status:     "pending",
		Progress:   0.0,
	}

	r.scheduleOperation(operation)
}

// scheduleOperation schedules a rebalancing operation
func (r *Rebalancer) scheduleOperation(operation *RebalanceOperation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	operationID := fmt.Sprintf("%s_%s_%s_%d", 
		operation.Type, operation.SourceNode, operation.TargetNode, time.Now().Unix())
	
	r.operations[operationID] = operation

	select {
	case r.operationCh <- operation:
		r.logger.Printf("Scheduled rebalance operation: %s", operationID)
	default:
		r.logger.Printf("Operation queue full, dropping operation: %s", operationID)
		delete(r.operations, operationID)
	}
}

// processOperations processes rebalancing operations
func (r *Rebalancer) processOperations() {
	for {
		select {
		case <-r.stopCh:
			return
		case operation := <-r.operationCh:
			r.executeOperation(operation)
		}
	}
}

// executeOperation executes a single rebalancing operation
func (r *Rebalancer) executeOperation(operation *RebalanceOperation) {
	ctx, cancel := context.WithTimeout(context.Background(), r.config.RebalanceTimeout)
	defer cancel()

	r.logger.Printf("Executing rebalance operation: %s from %s to %s", 
		operation.Type, operation.SourceNode, operation.TargetNode)

	operation.Status = "running"

	switch operation.Type {
	case RebalanceTypeAdd:
		r.executeAddNodeRebalance(ctx, operation)
	case RebalanceTypeRemove:
		r.executeRemoveNodeRebalance(ctx, operation)
	case RebalanceTypeRepair:
		r.executeRepairRebalance(ctx, operation)
	}

	// Remove completed operation
	r.mu.Lock()
	for id, op := range r.operations {
		if op == operation {
			delete(r.operations, id)
			break
		}
	}
	r.mu.Unlock()
}

// executeAddNodeRebalance handles rebalancing when a node is added
func (r *Rebalancer) executeAddNodeRebalance(ctx context.Context, operation *RebalanceOperation) {
	r.logger.Printf("Executing add node rebalance for %s", operation.TargetNode)

	// Simulate data movement by updating progress
	for progress := 0.0; progress <= 100.0; progress += 10.0 {
		select {
		case <-ctx.Done():
			operation.Status = "timeout"
			return
		default:
			operation.Progress = progress
			time.Sleep(100 * time.Millisecond) // Simulate work
		}
	}

	operation.Status = "completed"
	operation.Progress = 100.0
	r.logger.Printf("Add node rebalance completed for %s", operation.TargetNode)
}

// executeRemoveNodeRebalance handles rebalancing when a node is removed
func (r *Rebalancer) executeRemoveNodeRebalance(ctx context.Context, operation *RebalanceOperation) {
	r.logger.Printf("Executing remove node rebalance for %s", operation.SourceNode)

	// Find target nodes to move data to
	nodes := r.hashRing.GetAllNodes()
	var activeNodes []string
	for nodeID, node := range nodes {
		if node.Status == NodeStatusActive && nodeID != operation.SourceNode {
			activeNodes = append(activeNodes, nodeID)
		}
	}

	if len(activeNodes) == 0 {
		operation.Status = "failed"
		r.logger.Printf("No active nodes available to move data from %s", operation.SourceNode)
		return
	}

	// Simulate data movement
	for progress := 0.0; progress <= 100.0; progress += 10.0 {
		select {
		case <-ctx.Done():
			operation.Status = "timeout"
			return
		default:
			operation.Progress = progress
			time.Sleep(100 * time.Millisecond) // Simulate work
		}
	}

	operation.Status = "completed"
	operation.Progress = 100.0
	r.logger.Printf("Remove node rebalance completed for %s", operation.SourceNode)
}

// executeRepairRebalance handles load balancing between nodes
func (r *Rebalancer) executeRepairRebalance(ctx context.Context, operation *RebalanceOperation) {
	r.logger.Printf("Executing repair rebalance from %s to %s", 
		operation.SourceNode, operation.TargetNode)

	// Simulate selective data movement
	for progress := 0.0; progress <= 100.0; progress += 20.0 {
		select {
		case <-ctx.Done():
			operation.Status = "timeout"
			return
		default:
			operation.Progress = progress
			time.Sleep(50 * time.Millisecond) // Simulate work
		}
	}

	operation.Status = "completed"
	operation.Progress = 100.0
	r.logger.Printf("Repair rebalance completed from %s to %s", 
		operation.SourceNode, operation.TargetNode)
}

// GetActiveOperations returns all currently active rebalancing operations
func (r *Rebalancer) GetActiveOperations() map[string]*RebalanceOperation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*RebalanceOperation)
	for id, operation := range r.operations {
		operationCopy := *operation
		result[id] = &operationCopy
	}

	return result
}

// GetStats returns statistics about the rebalancer
func (r *Rebalancer) GetStats() RebalancerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RebalancerStats{
		IsRunning:        r.isRunning,
		ActiveOperations: len(r.operations),
		Configuration:    r.config,
	}

	// Count operations by status
	stats.OperationsByStatus = make(map[string]int)
	for _, operation := range r.operations {
		stats.OperationsByStatus[operation.Status]++
	}

	return stats
}

// RebalancerStats contains statistics about the rebalancer
type RebalancerStats struct {
	IsRunning           bool                   `json:"is_running"`
	ActiveOperations    int                    `json:"active_operations"`
	OperationsByStatus  map[string]int         `json:"operations_by_status"`
	Configuration       RebalancerConfig       `json:"configuration"`
}

// ForceRebalance manually triggers a rebalancing operation
func (r *Rebalancer) ForceRebalance(sourceNode, targetNode string) error {
	if sourceNode == "" && targetNode == "" {
		return fmt.Errorf("at least one of source or target node must be specified")
	}

	operationType := RebalanceTypeRepair
	if sourceNode == "" {
		operationType = RebalanceTypeAdd
	} else if targetNode == "" {
		operationType = RebalanceTypeRemove
	}

	operation := &RebalanceOperation{
		Type:       operationType,
		SourceNode: sourceNode,
		TargetNode: targetNode,
		Status:     "pending",
		Progress:   0.0,
	}

	r.scheduleOperation(operation)
	return nil
}

// SetConfig updates the rebalancer configuration
func (r *Rebalancer) SetConfig(config RebalancerConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = config
	r.logger.Printf("Rebalancer configuration updated")
}