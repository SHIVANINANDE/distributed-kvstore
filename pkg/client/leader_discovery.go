package client

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// LeaderDiscovery handles automatic leader discovery and failover
type LeaderDiscovery struct {
	mu              sync.RWMutex
	config          AdvancedClientConfig
	logger          *log.Logger
	
	// Leader tracking
	leaders         map[string]*NodeInfo  // shard/partition -> leader node
	shardMapping    map[string]string     // key -> shard mapping
	lastUpdate      time.Time
	
	// Discovery state
	isRunning       bool
	stopCh          chan struct{}
	updateInterval  time.Duration
	
	// Connection pool reference
	connectionPool  *AdvancedConnectionPool
}

// LeaderDiscoveryStats contains statistics about leader discovery
type LeaderDiscoveryStats struct {
	IsRunning      bool                   `json:"is_running"`
	LastUpdate     time.Time              `json:"last_update"`
	LeaderCount    int                    `json:"leader_count"`
	Leaders        map[string]*NodeInfo   `json:"leaders"`
	UpdateInterval time.Duration          `json:"update_interval"`
}

// NewLeaderDiscovery creates a new leader discovery instance
func NewLeaderDiscovery(config AdvancedClientConfig, logger *log.Logger) *LeaderDiscovery {
	if logger == nil {
		logger = log.New(log.Writer(), "[LEADER_DISCOVERY] ", log.LstdFlags)
	}
	
	return &LeaderDiscovery{
		config:         config,
		logger:         logger,
		leaders:        make(map[string]*NodeInfo),
		shardMapping:   make(map[string]string),
		stopCh:         make(chan struct{}),
		updateInterval: 30 * time.Second, // Default update interval
	}
}

// Start starts the leader discovery process
func (ld *LeaderDiscovery) Start(ctx context.Context, connectionPool *AdvancedConnectionPool) error {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	
	if ld.isRunning {
		return fmt.Errorf("leader discovery is already running")
	}
	
	ld.connectionPool = connectionPool
	ld.isRunning = true
	
	// Initial leader discovery
	if err := ld.discoverLeaders(ctx); err != nil {
		ld.logger.Printf("Initial leader discovery failed: %v", err)
	}
	
	// Start periodic discovery
	go ld.discoveryLoop(ctx)
	
	ld.logger.Printf("Leader discovery started")
	return nil
}

// Stop stops the leader discovery process
func (ld *LeaderDiscovery) Stop() {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	
	if !ld.isRunning {
		return
	}
	
	close(ld.stopCh)
	ld.isRunning = false
	
	ld.logger.Printf("Leader discovery stopped")
}

// GetLeaderForKey returns the leader node for a given key
func (ld *LeaderDiscovery) GetLeaderForKey(key string) (*NodeInfo, error) {
	ld.mu.RLock()
	defer ld.mu.RUnlock()
	
	// Determine which shard/partition the key belongs to
	shardID := ld.getShardForKey(key)
	
	// Look up the leader for that shard
	if leader, exists := ld.leaders[shardID]; exists {
		if leader.IsHealthy {
			return leader, nil
		}
	}
	
	// If no leader found or leader is unhealthy, try to discover
	ld.mu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := ld.discoverLeaders(ctx); err != nil {
		ld.mu.RLock()
		return nil, fmt.Errorf("failed to discover leader for key %s: %w", key, err)
	}
	
	ld.mu.RLock()
	if leader, exists := ld.leaders[shardID]; exists && leader.IsHealthy {
		return leader, nil
	}
	
	return nil, fmt.Errorf("no healthy leader found for key %s", key)
}

// GetAllLeaders returns all known leaders
func (ld *LeaderDiscovery) GetAllLeaders() map[string]*NodeInfo {
	ld.mu.RLock()
	defer ld.mu.RUnlock()
	
	result := make(map[string]*NodeInfo)
	for shardID, leader := range ld.leaders {
		if leader.IsHealthy {
			// Create a copy
			leaderCopy := *leader
			result[shardID] = &leaderCopy
		}
	}
	
	return result
}

// UpdateLeader manually updates the leader for a shard
func (ld *LeaderDiscovery) UpdateLeader(shardID string, leader *NodeInfo) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	
	ld.leaders[shardID] = leader
	ld.lastUpdate = time.Now()
	
	ld.logger.Printf("Updated leader for shard %s: %s", shardID, leader.ID)
}

// MarkLeaderUnhealthy marks a leader as unhealthy and triggers rediscovery
func (ld *LeaderDiscovery) MarkLeaderUnhealthy(shardID string) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	
	if leader, exists := ld.leaders[shardID]; exists {
		leader.IsHealthy = false
		ld.logger.Printf("Marked leader for shard %s as unhealthy: %s", shardID, leader.ID)
		
		// Trigger immediate rediscovery in background
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ld.discoverLeaders(ctx)
		}()
	}
}

// discoveryLoop runs the periodic leader discovery
func (ld *LeaderDiscovery) discoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(ld.updateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ld.stopCh:
			return
		case <-ticker.C:
			if err := ld.discoverLeaders(ctx); err != nil {
				ld.logger.Printf("Leader discovery failed: %v", err)
			}
		}
	}
}

// discoverLeaders discovers current leaders from the cluster
func (ld *LeaderDiscovery) discoverLeaders(ctx context.Context) error {
	// Get all available nodes from connection pool
	nodes := ld.connectionPool.GetAllNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available for leader discovery")
	}
	
	// Try to query cluster topology from each node until successful
	var lastErr error
	for _, node := range nodes {
		if !node.IsHealthy {
			continue
		}
		
		topology, err := ld.queryClusterTopology(ctx, node)
		if err != nil {
			lastErr = err
			continue
		}
		
		// Update leader mapping
		ld.updateLeaderMapping(topology)
		return nil
	}
	
	return fmt.Errorf("failed to discover leaders from any node: %v", lastErr)
}

// queryClusterTopology queries cluster topology from a specific node
func (ld *LeaderDiscovery) queryClusterTopology(ctx context.Context, node *NodeInfo) (*ClusterTopology, error) {
	// In a real implementation, this would make an RPC call to get cluster topology
	// For now, we'll simulate it
	
	ld.logger.Printf("Querying cluster topology from node %s", node.ID)
	
	// Simulate network delay
	time.Sleep(10 * time.Millisecond)
	
	// Create a simulated topology
	topology := &ClusterTopology{
		Shards: make(map[string]*ShardInfo),
		Nodes:  make(map[string]*NodeInfo),
	}
	
	// Simulate multiple shards with leaders
	shardCount := 3 // Simulate 3 shards
	for i := 0; i < shardCount; i++ {
		shardID := fmt.Sprintf("shard_%d", i)
		
		// Create shard info with simulated leader
		leaderNode := &NodeInfo{
			ID:        fmt.Sprintf("node_%d", i%len(ld.connectionPool.GetAllNodes())),
			Address:   node.Address, // Use same address for simulation
			Port:      node.Port + i,
			IsHealthy: true,
			IsLeader:  true,
			LastSeen:  time.Now(),
			Weight:    1,
			Metadata:  make(map[string]string),
		}
		
		topology.Shards[shardID] = &ShardInfo{
			ID:       shardID,
			Leader:   leaderNode,
			Replicas: []*NodeInfo{leaderNode},
			KeyRange: KeyRange{
				Start: uint64(i) * 1000,
				End:   uint64(i+1)*1000 - 1,
			},
		}
		
		topology.Nodes[leaderNode.ID] = leaderNode
	}
	
	return topology, nil
}

// updateLeaderMapping updates the internal leader mapping
func (ld *LeaderDiscovery) updateLeaderMapping(topology *ClusterTopology) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	
	// Update leaders from topology
	for shardID, shardInfo := range topology.Shards {
		if shardInfo.Leader != nil {
			ld.leaders[shardID] = shardInfo.Leader
		}
	}
	
	ld.lastUpdate = time.Now()
	ld.logger.Printf("Updated leader mapping with %d shards", len(topology.Shards))
}

// getShardForKey determines which shard a key belongs to
func (ld *LeaderDiscovery) getShardForKey(key string) string {
	// Simple hash-based sharding for demonstration
	// In a real implementation, this would use the same logic as the server
	hash := ld.hashKey(key)
	shardCount := 3 // Should match server configuration
	shardIdx := hash % uint64(shardCount)
	return fmt.Sprintf("shard_%d", shardIdx)
}

// hashKey generates a hash for the given key
func (ld *LeaderDiscovery) hashKey(key string) uint64 {
	hash := uint64(0)
	for _, b := range []byte(key) {
		hash = hash*31 + uint64(b)
	}
	return hash
}

// GetStats returns leader discovery statistics
func (ld *LeaderDiscovery) GetStats() LeaderDiscoveryStats {
	ld.mu.RLock()
	defer ld.mu.RUnlock()
	
	leaders := make(map[string]*NodeInfo)
	for shardID, leader := range ld.leaders {
		leaderCopy := *leader
		leaders[shardID] = &leaderCopy
	}
	
	return LeaderDiscoveryStats{
		IsRunning:      ld.isRunning,
		LastUpdate:     ld.lastUpdate,
		LeaderCount:    len(ld.leaders),
		Leaders:        leaders,
		UpdateInterval: ld.updateInterval,
	}
}

// ClusterTopology represents the topology of the cluster
type ClusterTopology struct {
	Shards map[string]*ShardInfo `json:"shards"`
	Nodes  map[string]*NodeInfo  `json:"nodes"`
}

// ShardInfo represents information about a shard
type ShardInfo struct {
	ID       string      `json:"id"`
	Leader   *NodeInfo   `json:"leader"`
	Replicas []*NodeInfo `json:"replicas"`
	KeyRange KeyRange    `json:"key_range"`
}

// KeyRange represents a range of keys
type KeyRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// SetUpdateInterval sets the leader discovery update interval
func (ld *LeaderDiscovery) SetUpdateInterval(interval time.Duration) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	
	ld.updateInterval = interval
	ld.logger.Printf("Leader discovery update interval set to %v", interval)
}

// ForceDiscovery forces an immediate leader discovery
func (ld *LeaderDiscovery) ForceDiscovery(ctx context.Context) error {
	return ld.discoverLeaders(ctx)
}