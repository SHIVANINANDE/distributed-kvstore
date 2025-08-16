package client

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// LoadBalancer handles client-side load balancing
type LoadBalancer struct {
	mu             sync.RWMutex
	policy         LoadBalancingPolicy
	nodes          map[string]*NodeInfo
	healthyNodes   map[string]bool
	roundRobinIdx  int
	weights        map[string]int
	connections    map[string]int  // Track connection count per node
	logger         *log.Logger
	
	// Consistent hashing ring for consistent policy
	hashRing       []HashPoint
	virtualNodes   int
}

// NodeInfo represents information about a cluster node
type NodeInfo struct {
	ID           string            `json:"id"`
	Address      string            `json:"address"`
	Port         int               `json:"port"`
	IsHealthy    bool              `json:"is_healthy"`
	IsLeader     bool              `json:"is_leader"`
	LastSeen     time.Time         `json:"last_seen"`
	Latency      time.Duration     `json:"latency"`
	Weight       int               `json:"weight"`
	Metadata     map[string]string `json:"metadata"`
	Connections  int               `json:"connections"`
}

// HashPoint represents a point on the consistent hash ring
type HashPoint struct {
	Hash   uint64
	NodeID string
}

// LoadBalancerStats contains statistics about the load balancer
type LoadBalancerStats struct {
	Policy         LoadBalancingPolicy    `json:"policy"`
	TotalNodes     int                    `json:"total_nodes"`
	HealthyNodes   int                    `json:"healthy_nodes"`
	NodeStats      map[string]*NodeInfo   `json:"node_stats"`
	RoundRobinIdx  int                    `json:"round_robin_idx"`
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(policy LoadBalancingPolicy, logger *log.Logger) *LoadBalancer {
	if logger == nil {
		logger = log.New(log.Writer(), "[LOAD_BALANCER] ", log.LstdFlags)
	}
	
	return &LoadBalancer{
		policy:       policy,
		nodes:        make(map[string]*NodeInfo),
		healthyNodes: make(map[string]bool),
		weights:      make(map[string]int),
		connections:  make(map[string]int),
		logger:       logger,
		virtualNodes: 150, // Virtual nodes for consistent hashing
	}
}

// AddNode adds a node to the load balancer
func (lb *LoadBalancer) AddNode(node *NodeInfo) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.nodes[node.ID] = node
	lb.healthyNodes[node.ID] = node.IsHealthy
	lb.weights[node.ID] = node.Weight
	if lb.weights[node.ID] == 0 {
		lb.weights[node.ID] = 1 // Default weight
	}
	
	// Update consistent hash ring if using consistent hashing
	if lb.policy == LoadBalancingConsistent {
		lb.rebuildHashRing()
	}
	
	lb.logger.Printf("Added node %s to load balancer", node.ID)
}

// RemoveNode removes a node from the load balancer
func (lb *LoadBalancer) RemoveNode(nodeID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	delete(lb.nodes, nodeID)
	delete(lb.healthyNodes, nodeID)
	delete(lb.weights, nodeID)
	delete(lb.connections, nodeID)
	
	// Update consistent hash ring if using consistent hashing
	if lb.policy == LoadBalancingConsistent {
		lb.rebuildHashRing()
	}
	
	lb.logger.Printf("Removed node %s from load balancer", nodeID)
}

// UpdatePolicy updates the load balancing policy
func (lb *LoadBalancer) UpdatePolicy(policy LoadBalancingPolicy) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.policy = policy
	
	// Rebuild hash ring if switching to consistent hashing
	if policy == LoadBalancingConsistent {
		lb.rebuildHashRing()
	}
	
	lb.logger.Printf("Load balancing policy updated to %s", policy)
}

// SelectNodes selects nodes based on the configured policy
func (lb *LoadBalancer) SelectNodes(key string, count int) ([]*NodeInfo, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	// Get healthy nodes
	healthyNodes := lb.getHealthyNodes()
	if len(healthyNodes) == 0 {
		return nil, fmt.Errorf("no healthy nodes available")
	}
	
	if count <= 0 {
		count = 1
	}
	
	switch lb.policy {
	case LoadBalancingRoundRobin:
		return lb.selectRoundRobin(healthyNodes, count), nil
	case LoadBalancingRandom:
		return lb.selectRandom(healthyNodes, count), nil
	case LoadBalancingLeastConn:
		return lb.selectLeastConnections(healthyNodes, count), nil
	case LoadBalancingWeighted:
		return lb.selectWeighted(healthyNodes, count), nil
	case LoadBalancingConsistent:
		return lb.selectConsistentHash(key, healthyNodes, count), nil
	default:
		return lb.selectRoundRobin(healthyNodes, count), nil
	}
}

// SelectRecentNodes selects nodes that have been recently updated
func (lb *LoadBalancer) SelectRecentNodes(key string, count int, staleThreshold time.Duration) ([]*NodeInfo, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	now := time.Now()
	var recentNodes []*NodeInfo
	
	for _, node := range lb.nodes {
		if node.IsHealthy && now.Sub(node.LastSeen) <= staleThreshold {
			recentNodes = append(recentNodes, node)
		}
	}
	
	if len(recentNodes) == 0 {
		// Fall back to any healthy nodes if no recent nodes available
		return lb.SelectNodes(key, count)
	}
	
	if count > len(recentNodes) {
		count = len(recentNodes)
	}
	
	// Use random selection among recent nodes
	rand.Shuffle(len(recentNodes), func(i, j int) {
		recentNodes[i], recentNodes[j] = recentNodes[j], recentNodes[i]
	})
	
	return recentNodes[:count], nil
}

// MarkNodeHealthy marks a node as healthy
func (lb *LoadBalancer) MarkNodeHealthy(nodeID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	if node, exists := lb.nodes[nodeID]; exists {
		node.IsHealthy = true
		node.LastSeen = time.Now()
		lb.healthyNodes[nodeID] = true
		lb.logger.Printf("Node %s marked as healthy", nodeID)
	}
}

// MarkNodeUnhealthy marks a node as unhealthy
func (lb *LoadBalancer) MarkNodeUnhealthy(nodeID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	if node, exists := lb.nodes[nodeID]; exists {
		node.IsHealthy = false
		lb.healthyNodes[nodeID] = false
		lb.logger.Printf("Node %s marked as unhealthy", nodeID)
	}
}

// UpdateNodeConnections updates the connection count for a node
func (lb *LoadBalancer) UpdateNodeConnections(nodeID string, connections int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	if node, exists := lb.nodes[nodeID]; exists {
		node.Connections = connections
		lb.connections[nodeID] = connections
	}
}

// selectRoundRobin selects nodes using round-robin algorithm
func (lb *LoadBalancer) selectRoundRobin(nodes []*NodeInfo, count int) []*NodeInfo {
	if len(nodes) == 0 {
		return nil
	}
	
	var selected []*NodeInfo
	nodeCount := len(nodes)
	
	for i := 0; i < count; i++ {
		idx := (lb.roundRobinIdx + i) % nodeCount
		selected = append(selected, nodes[idx])
	}
	
	lb.roundRobinIdx = (lb.roundRobinIdx + count) % nodeCount
	return selected
}

// selectRandom selects nodes randomly
func (lb *LoadBalancer) selectRandom(nodes []*NodeInfo, count int) []*NodeInfo {
	if len(nodes) == 0 {
		return nil
	}
	
	if count >= len(nodes) {
		return nodes
	}
	
	// Create a copy and shuffle
	nodesCopy := make([]*NodeInfo, len(nodes))
	copy(nodesCopy, nodes)
	
	rand.Shuffle(len(nodesCopy), func(i, j int) {
		nodesCopy[i], nodesCopy[j] = nodesCopy[j], nodesCopy[i]
	})
	
	return nodesCopy[:count]
}

// selectLeastConnections selects nodes with the least connections
func (lb *LoadBalancer) selectLeastConnections(nodes []*NodeInfo, count int) []*NodeInfo {
	if len(nodes) == 0 {
		return nil
	}
	
	// Sort nodes by connection count
	nodesCopy := make([]*NodeInfo, len(nodes))
	copy(nodesCopy, nodes)
	
	sort.Slice(nodesCopy, func(i, j int) bool {
		return nodesCopy[i].Connections < nodesCopy[j].Connections
	})
	
	if count > len(nodesCopy) {
		count = len(nodesCopy)
	}
	
	return nodesCopy[:count]
}

// selectWeighted selects nodes based on weights
func (lb *LoadBalancer) selectWeighted(nodes []*NodeInfo, count int) []*NodeInfo {
	if len(nodes) == 0 {
		return nil
	}
	
	// Calculate total weight
	totalWeight := 0
	for _, node := range nodes {
		totalWeight += node.Weight
	}
	
	if totalWeight == 0 {
		// If no weights set, fall back to random selection
		return lb.selectRandom(nodes, count)
	}
	
	var selected []*NodeInfo
	for i := 0; i < count && i < len(nodes); i++ {
		// Select based on weight
		target := rand.Intn(totalWeight)
		currentWeight := 0
		
		for _, node := range nodes {
			currentWeight += node.Weight
			if currentWeight > target {
				selected = append(selected, node)
				break
			}
		}
	}
	
	return selected
}

// selectConsistentHash selects nodes using consistent hashing
func (lb *LoadBalancer) selectConsistentHash(key string, nodes []*NodeInfo, count int) []*NodeInfo {
	if len(lb.hashRing) == 0 {
		return lb.selectRandom(nodes, count)
	}
	
	hash := lb.hash(key)
	
	// Find the first node in the ring with hash >= key hash
	idx := sort.Search(len(lb.hashRing), func(i int) bool {
		return lb.hashRing[i].Hash >= hash
	})
	
	// If no node found, wrap around to the first node
	if idx == len(lb.hashRing) {
		idx = 0
	}
	
	// Collect unique nodes starting from the selected position
	var selected []*NodeInfo
	seen := make(map[string]bool)
	
	for i := 0; i < len(lb.hashRing) && len(selected) < count; i++ {
		ringIdx := (idx + i) % len(lb.hashRing)
		nodeID := lb.hashRing[ringIdx].NodeID
		
		if !seen[nodeID] {
			if node, exists := lb.nodes[nodeID]; exists && node.IsHealthy {
				selected = append(selected, node)
				seen[nodeID] = true
			}
		}
	}
	
	return selected
}

// getHealthyNodes returns a list of healthy nodes
func (lb *LoadBalancer) getHealthyNodes() []*NodeInfo {
	var healthy []*NodeInfo
	for _, node := range lb.nodes {
		if node.IsHealthy {
			healthy = append(healthy, node)
		}
	}
	return healthy
}

// rebuildHashRing rebuilds the consistent hash ring
func (lb *LoadBalancer) rebuildHashRing() {
	lb.hashRing = nil
	
	for nodeID := range lb.nodes {
		for i := 0; i < lb.virtualNodes; i++ {
			virtualKey := fmt.Sprintf("%s:%d", nodeID, i)
			hash := lb.hash(virtualKey)
			lb.hashRing = append(lb.hashRing, HashPoint{
				Hash:   hash,
				NodeID: nodeID,
			})
		}
	}
	
	// Sort the ring by hash value
	sort.Slice(lb.hashRing, func(i, j int) bool {
		return lb.hashRing[i].Hash < lb.hashRing[j].Hash
	})
}

// hash generates a SHA-256 hash for the given key
func (lb *LoadBalancer) hash(key string) uint64 {
	hash := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(hash[:8])
}

// GetStats returns load balancer statistics
func (lb *LoadBalancer) GetStats() LoadBalancerStats {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	healthyCount := 0
	nodeStats := make(map[string]*NodeInfo)
	
	for nodeID, node := range lb.nodes {
		if node.IsHealthy {
			healthyCount++
		}
		// Create a copy for stats
		nodeCopy := *node
		nodeStats[nodeID] = &nodeCopy
	}
	
	return LoadBalancerStats{
		Policy:        lb.policy,
		TotalNodes:    len(lb.nodes),
		HealthyNodes:  healthyCount,
		NodeStats:     nodeStats,
		RoundRobinIdx: lb.roundRobinIdx,
	}
}