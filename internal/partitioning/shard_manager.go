package partitioning

import (
	"fmt"
	"log"
	"sync"
	"time"

	"distributed-kvstore/internal/consensus"
)

// ShardManager manages multiple Raft groups (shards)
type ShardManager struct {
	mu           sync.RWMutex
	shards       map[uint32]*Shard          // ShardID -> Shard
	hashRing     *HashRing                  // Consistent hash ring for routing
	config       ShardManagerConfig         // Configuration
	logger       *log.Logger                // Logger
	nodeID       string                     // This node's ID
	rebalancer   *Rebalancer               // Rebalancer for automatic data movement
}

// Shard represents a single Raft group managing a portion of the data
type Shard struct {
	ID           uint32                    `json:"id"`
	RaftGroup    *consensus.ClusterManager `json:"-"`        // Raft cluster for this shard
	Replicas     []string                  `json:"replicas"` // Node IDs that host this shard
	Leader       string                    `json:"leader"`   // Current leader of this shard
	Status       ShardStatus               `json:"status"`   // Shard status
	KeyRange     KeyRange                  `json:"key_range"` // Key range this shard is responsible for
	CreatedAt    time.Time                 `json:"created_at"`
	UpdatedAt    time.Time                 `json:"updated_at"`
	Metadata     map[string]string         `json:"metadata"`
}

// ShardStatus represents the status of a shard
type ShardStatus string

const (
	ShardStatusActive      ShardStatus = "active"
	ShardStatusInitializing ShardStatus = "initializing"
	ShardStatusRebalancing ShardStatus = "rebalancing"
	ShardStatusDraining    ShardStatus = "draining"
	ShardStatusFailed      ShardStatus = "failed"
)

// ShardManagerConfig contains configuration for the shard manager
type ShardManagerConfig struct {
	DefaultReplicas     int           `json:"default_replicas"`      // Default number of replicas per shard
	MaxShardsPerNode    int           `json:"max_shards_per_node"`   // Maximum shards per node
	ShardSplitThreshold int64         `json:"shard_split_threshold"` // Size threshold for shard splitting
	ElectionTimeout     time.Duration `json:"election_timeout"`      // Raft election timeout
	HeartbeatTimeout    time.Duration `json:"heartbeat_timeout"`     // Raft heartbeat timeout
	EnableAutoSplit     bool          `json:"enable_auto_split"`     // Enable automatic shard splitting
	EnableAutoMerge     bool          `json:"enable_auto_merge"`     // Enable automatic shard merging
}

// NewShardManager creates a new shard manager
func NewShardManager(nodeID string, hashRing *HashRing, config ShardManagerConfig, logger *log.Logger) *ShardManager {
	if logger == nil {
		logger = log.New(log.Writer(), "[SHARD_MGR] ", log.LstdFlags)
	}

	// Set default values
	if config.DefaultReplicas == 0 {
		config.DefaultReplicas = 3
	}
	if config.MaxShardsPerNode == 0 {
		config.MaxShardsPerNode = 10
	}
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = 200 * time.Millisecond
	}
	if config.HeartbeatTimeout == 0 {
		config.HeartbeatTimeout = 50 * time.Millisecond
	}

	sm := &ShardManager{
		shards:   make(map[uint32]*Shard),
		hashRing: hashRing,
		config:   config,
		logger:   logger,
		nodeID:   nodeID,
	}

	return sm
}

// SetRebalancer sets the rebalancer for this shard manager
func (sm *ShardManager) SetRebalancer(rebalancer *Rebalancer) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.rebalancer = rebalancer
}

// CreateShard creates a new shard with the specified replicas
func (sm *ShardManager) CreateShard(shardID uint32, replicas []string, keyRange KeyRange) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.shards[shardID]; exists {
		return fmt.Errorf("shard %d already exists", shardID)
	}

	sm.logger.Printf("Creating shard %d with replicas: %v", shardID, replicas)

	// Validate replicas
	if len(replicas) == 0 {
		return fmt.Errorf("at least one replica must be specified")
	}

	// Create the shard
	shard := &Shard{
		ID:        shardID,
		Replicas:  replicas,
		Status:    ShardStatusInitializing,
		KeyRange:  keyRange,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]string),
	}

	// Create Raft cluster configuration for this shard
	var nodeConfigs []*consensus.NodeConfig
	for i, replicaID := range replicas {
		nodeConfigs = append(nodeConfigs, &consensus.NodeConfig{
			NodeID:   replicaID,
			Address:  "localhost", // In production, this would be looked up
			RaftPort: int32(8000 + int(shardID)*100 + i),
			GrpcPort: int32(9000 + int(shardID)*100 + i),
		})
	}

	clusterConfig := &consensus.ClusterConfig{
		Nodes:            nodeConfigs,
		ElectionTimeout:  sm.config.ElectionTimeout,
		HeartbeatTimeout: sm.config.HeartbeatTimeout,
		DataDir:          fmt.Sprintf("data/shard_%d", shardID),
	}

	// Create Raft cluster manager for this shard
	raftCluster := consensus.NewClusterManager(clusterConfig, 
		log.New(log.Writer(), fmt.Sprintf("[SHARD_%d] ", shardID), log.LstdFlags))

	// Bootstrap the Raft cluster if this node is in the replica set
	if sm.isNodeInReplicas(sm.nodeID, replicas) {
		if err := raftCluster.Bootstrap(); err != nil {
			return fmt.Errorf("failed to bootstrap shard %d: %w", shardID, err)
		}

		// Wait for leader election
		leader, err := raftCluster.WaitForLeader(5 * time.Second)
		if err == nil {
			shard.Leader = leader
		}
	}

	shard.RaftGroup = raftCluster
	shard.Status = ShardStatusActive
	shard.UpdatedAt = time.Now()

	sm.shards[shardID] = shard
	sm.logger.Printf("Shard %d created successfully with leader: %s", shardID, shard.Leader)

	return nil
}

// GetShard returns information about a specific shard
func (sm *ShardManager) GetShard(shardID uint32) (*Shard, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %d not found", shardID)
	}

	// Return a copy to prevent external modification
	shardCopy := *shard
	return &shardCopy, nil
}

// GetShardForKey returns the shard responsible for a given key
func (sm *ShardManager) GetShardForKey(key string) (*Shard, error) {
	// Use hash ring to determine which node should handle this key
	nodeID, err := sm.hashRing.GetNode(key)
	if err != nil {
		return nil, fmt.Errorf("failed to find node for key: %w", err)
	}

	// Find the shard on that node
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, shard := range sm.shards {
		if sm.isNodeInReplicas(nodeID, shard.Replicas) {
			// Check if key falls within this shard's range
			keyHash := sm.hashRing.Hash(key)
			if keyHash >= shard.KeyRange.Start && keyHash <= shard.KeyRange.End {
				shardCopy := *shard
				return &shardCopy, nil
			}
		}
	}

	// If no existing shard found, we might need to create one
	return nil, fmt.Errorf("no shard found for key %s (node: %s)", key, nodeID)
}

// GetAllShards returns information about all shards
func (sm *ShardManager) GetAllShards() map[uint32]*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[uint32]*Shard)
	for shardID, shard := range sm.shards {
		shardCopy := *shard
		result[shardID] = &shardCopy
	}

	return result
}

// GetLocalShards returns shards that this node participates in
func (sm *ShardManager) GetLocalShards() map[uint32]*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[uint32]*Shard)
	for shardID, shard := range sm.shards {
		if sm.isNodeInReplicas(sm.nodeID, shard.Replicas) {
			shardCopy := *shard
			result[shardID] = &shardCopy
		}
	}

	return result
}

// AddReplica adds a new replica to a shard
func (sm *ShardManager) AddReplica(shardID uint32, nodeID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return fmt.Errorf("shard %d not found", shardID)
	}

	// Check if node is already a replica
	if sm.isNodeInReplicas(nodeID, shard.Replicas) {
		return fmt.Errorf("node %s is already a replica of shard %d", nodeID, shardID)
	}

	sm.logger.Printf("Adding replica %s to shard %d", nodeID, shardID)

	// Add to replica list
	shard.Replicas = append(shard.Replicas, nodeID)
	shard.UpdatedAt = time.Now()

	// Update the Raft cluster configuration
	// In a production system, this would use Raft's membership change protocol
	nodeConfig := &consensus.NodeConfig{
		NodeID:   nodeID,
		Address:  "localhost",
		RaftPort: int32(8000 + int(shardID)*100 + len(shard.Replicas)),
		GrpcPort: int32(9000 + int(shardID)*100 + len(shard.Replicas)),
	}

	if shard.RaftGroup != nil {
		if err := shard.RaftGroup.AddNode(nodeConfig); err != nil {
			// Rollback the replica addition
			shard.Replicas = shard.Replicas[:len(shard.Replicas)-1]
			return fmt.Errorf("failed to add node to Raft cluster: %w", err)
		}
	}

	sm.logger.Printf("Successfully added replica %s to shard %d", nodeID, shardID)
	return nil
}

// RemoveReplica removes a replica from a shard
func (sm *ShardManager) RemoveReplica(shardID uint32, nodeID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return fmt.Errorf("shard %d not found", shardID)
	}

	// Check if node is a replica
	if !sm.isNodeInReplicas(nodeID, shard.Replicas) {
		return fmt.Errorf("node %s is not a replica of shard %d", nodeID, shardID)
	}

	// Ensure we don't remove the last replica
	if len(shard.Replicas) <= 1 {
		return fmt.Errorf("cannot remove the last replica from shard %d", shardID)
	}

	sm.logger.Printf("Removing replica %s from shard %d", nodeID, shardID)

	// Remove from replica list
	newReplicas := make([]string, 0, len(shard.Replicas)-1)
	for _, replica := range shard.Replicas {
		if replica != nodeID {
			newReplicas = append(newReplicas, replica)
		}
	}
	shard.Replicas = newReplicas
	shard.UpdatedAt = time.Now()

	// Update the Raft cluster configuration
	if shard.RaftGroup != nil {
		if err := shard.RaftGroup.RemoveNode(nodeID); err != nil {
			sm.logger.Printf("Warning: failed to remove node from Raft cluster: %v", err)
		}
	}

	sm.logger.Printf("Successfully removed replica %s from shard %d", nodeID, shardID)
	return nil
}

// SplitShard splits a shard into two shards
func (sm *ShardManager) SplitShard(shardID uint32, splitKey string) (uint32, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return 0, fmt.Errorf("shard %d not found", shardID)
	}

	if shard.Status != ShardStatusActive {
		return 0, fmt.Errorf("cannot split shard %d: status is %s", shardID, shard.Status)
	}

	sm.logger.Printf("Splitting shard %d at key %s", shardID, splitKey)

	// Calculate split hash
	splitHash := sm.hashRing.Hash(splitKey)
	if splitHash <= shard.KeyRange.Start || splitHash >= shard.KeyRange.End {
		return 0, fmt.Errorf("split key %s is outside shard %d range", splitKey, shardID)
	}

	// Generate new shard ID
	newShardID := sm.generateShardID()

	// Create new key ranges
	originalRange := shard.KeyRange
	shard.KeyRange = KeyRange{Start: originalRange.Start, End: splitHash}
	newRange := KeyRange{Start: splitHash + 1, End: originalRange.End}

	// Create new shard with the same replicas
	newShard := &Shard{
		ID:        newShardID,
		Replicas:  make([]string, len(shard.Replicas)),
		Status:    ShardStatusInitializing,
		KeyRange:  newRange,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]string),
	}
	copy(newShard.Replicas, shard.Replicas)

	shard.UpdatedAt = time.Now()
	sm.shards[newShardID] = newShard

	sm.logger.Printf("Shard %d split into %d and %d", shardID, shardID, newShardID)
	return newShardID, nil
}

// MergeShards merges two adjacent shards
func (sm *ShardManager) MergeShards(shard1ID, shard2ID uint32) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard1, exists1 := sm.shards[shard1ID]
	shard2, exists2 := sm.shards[shard2ID]

	if !exists1 || !exists2 {
		return fmt.Errorf("one or both shards not found")
	}

	if shard1.Status != ShardStatusActive || shard2.Status != ShardStatusActive {
		return fmt.Errorf("cannot merge non-active shards")
	}

	sm.logger.Printf("Merging shards %d and %d", shard1ID, shard2ID)

	// Ensure shards are adjacent
	if shard1.KeyRange.End+1 != shard2.KeyRange.Start {
		return fmt.Errorf("shards %d and %d are not adjacent", shard1ID, shard2ID)
	}

	// Merge key ranges
	shard1.KeyRange.End = shard2.KeyRange.End
	shard1.UpdatedAt = time.Now()

	// Remove the second shard
	delete(sm.shards, shard2ID)

	sm.logger.Printf("Shards %d and %d merged into %d", shard1ID, shard2ID, shard1ID)
	return nil
}

// Stop stops all Raft groups managed by this shard manager
func (sm *ShardManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.logger.Printf("Stopping shard manager")

	for shardID, shard := range sm.shards {
		if shard.RaftGroup != nil {
			sm.logger.Printf("Stopping Raft group for shard %d", shardID)
			shard.RaftGroup.Stop()
		}
	}
}

// GetStats returns statistics about the shard manager
func (sm *ShardManager) GetStats() ShardManagerStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := ShardManagerStats{
		TotalShards:      len(sm.shards),
		LocalShards:      0,
		ShardsByStatus:   make(map[ShardStatus]int),
		ReplicationStats: make(map[int]int), // replication factor -> count
	}

	for _, shard := range sm.shards {
		// Count shards by status
		stats.ShardsByStatus[shard.Status]++

		// Count replication factors
		replicas := len(shard.Replicas)
		stats.ReplicationStats[replicas]++

		// Count local shards
		if sm.isNodeInReplicas(sm.nodeID, shard.Replicas) {
			stats.LocalShards++
		}
	}

	return stats
}

// ShardManagerStats contains statistics about the shard manager
type ShardManagerStats struct {
	TotalShards      int                    `json:"total_shards"`
	LocalShards      int                    `json:"local_shards"`
	ShardsByStatus   map[ShardStatus]int    `json:"shards_by_status"`
	ReplicationStats map[int]int            `json:"replication_stats"`
}

// Helper functions

// isNodeInReplicas checks if a node is in the replica list
func (sm *ShardManager) isNodeInReplicas(nodeID string, replicas []string) bool {
	for _, replica := range replicas {
		if replica == nodeID {
			return true
		}
	}
	return false
}

// generateShardID generates a new unique shard ID
func (sm *ShardManager) generateShardID() uint32 {
	maxID := uint32(0)
	for shardID := range sm.shards {
		if shardID > maxID {
			maxID = shardID
		}
	}
	return maxID + 1
}

// UpdateShardLeader updates the leader information for a shard
func (sm *ShardManager) UpdateShardLeader(shardID uint32, leaderID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return fmt.Errorf("shard %d not found", shardID)
	}

	if shard.Leader != leaderID {
		sm.logger.Printf("Shard %d leader changed from %s to %s", shardID, shard.Leader, leaderID)
		shard.Leader = leaderID
		shard.UpdatedAt = time.Now()
	}

	return nil
}

// GetShardLeader returns the current leader of a shard
func (sm *ShardManager) GetShardLeader(shardID uint32) (string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return "", fmt.Errorf("shard %d not found", shardID)
	}

	// Get current leader from Raft cluster
	if shard.RaftGroup != nil {
		leader := shard.RaftGroup.GetLeader()
		if leader != "" {
			return leader, nil
		}
	}

	return shard.Leader, nil
}