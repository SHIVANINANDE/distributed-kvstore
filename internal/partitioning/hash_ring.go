package partitioning

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"sort"
	"sync"
)

// HashRing implements consistent hashing with SHA-256
type HashRing struct {
	mu           sync.RWMutex
	ring         map[uint64]string  // Hash -> NodeID
	sortedHashes []uint64           // Sorted hash values for binary search
	nodes        map[string]*Node   // NodeID -> Node info
	virtualNodes int                // Number of virtual nodes per physical node
	logger       *log.Logger
}

// Node represents a physical node in the hash ring
type Node struct {
	ID           string            `json:"id"`
	Address      string            `json:"address"`
	Port         int32             `json:"port"`
	Weight       int               `json:"weight"`       // Weight for load balancing
	Capacity     int64             `json:"capacity"`     // Storage capacity
	Load         int64             `json:"load"`         // Current load
	Shards       []uint32          `json:"shards"`       // Shards assigned to this node
	Metadata     map[string]string `json:"metadata"`     // Additional node metadata
	Status       NodeStatus        `json:"status"`       // Node status
	LastSeen     int64             `json:"last_seen"`    // Unix timestamp
}

// NodeStatus represents the status of a node
type NodeStatus string

const (
	NodeStatusActive   NodeStatus = "active"
	NodeStatusInactive NodeStatus = "inactive"
	NodeStatusFailed   NodeStatus = "failed"
	NodeStatusDraining NodeStatus = "draining"
)

// VirtualNode represents a virtual node on the hash ring
type VirtualNode struct {
	Hash     uint64 `json:"hash"`
	NodeID   string `json:"node_id"`
	VirtualID int   `json:"virtual_id"`
}

// RebalanceOperation represents a data movement operation during rebalancing
type RebalanceOperation struct {
	Type       RebalanceType `json:"type"`
	SourceNode string        `json:"source_node"`
	TargetNode string        `json:"target_node"`
	Shard      uint32        `json:"shard"`
	KeyRange   KeyRange      `json:"key_range"`
	Status     string        `json:"status"`
	Progress   float64       `json:"progress"`
}

// RebalanceType represents the type of rebalancing operation
type RebalanceType string

const (
	RebalanceTypeAdd    RebalanceType = "add_node"
	RebalanceTypeRemove RebalanceType = "remove_node"
	RebalanceTypeRepair RebalanceType = "repair"
)

// KeyRange represents a range of keys
type KeyRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// NewHashRing creates a new consistent hash ring
func NewHashRing(virtualNodes int, logger *log.Logger) *HashRing {
	if logger == nil {
		logger = log.New(log.Writer(), "[HASHRING] ", log.LstdFlags)
	}
	if virtualNodes <= 0 {
		virtualNodes = 150 // Default virtual nodes per physical node
	}

	return &HashRing{
		ring:         make(map[uint64]string),
		sortedHashes: make([]uint64, 0),
		nodes:        make(map[string]*Node),
		virtualNodes: virtualNodes,
		logger:       logger,
	}
}

// Hash generates a SHA-256 hash for the given key
func (hr *HashRing) Hash(key string) uint64 {
	hash := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(hash[:8])
}

// AddNode adds a node to the hash ring
func (hr *HashRing) AddNode(node *Node) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, exists := hr.nodes[node.ID]; exists {
		return fmt.Errorf("node %s already exists", node.ID)
	}

	hr.logger.Printf("Adding node %s to hash ring", node.ID)

	// Store node information
	hr.nodes[node.ID] = node

	// Add virtual nodes to the ring
	for i := 0; i < hr.virtualNodes; i++ {
		virtualKey := fmt.Sprintf("%s:%d", node.ID, i)
		hash := hr.Hash(virtualKey)
		hr.ring[hash] = node.ID
		hr.sortedHashes = append(hr.sortedHashes, hash)
	}

	// Re-sort the hash values
	sort.Slice(hr.sortedHashes, func(i, j int) bool {
		return hr.sortedHashes[i] < hr.sortedHashes[j]
	})

	hr.logger.Printf("Node %s added with %d virtual nodes", node.ID, hr.virtualNodes)
	return nil
}

// RemoveNode removes a node from the hash ring
func (hr *HashRing) RemoveNode(nodeID string) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, exists := hr.nodes[nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	hr.logger.Printf("Removing node %s from hash ring", nodeID)

	// Remove all virtual nodes for this physical node
	newSortedHashes := make([]uint64, 0, len(hr.sortedHashes))
	for _, hash := range hr.sortedHashes {
		if hr.ring[hash] != nodeID {
			newSortedHashes = append(newSortedHashes, hash)
		} else {
			delete(hr.ring, hash)
		}
	}
	hr.sortedHashes = newSortedHashes

	// Remove node information
	delete(hr.nodes, nodeID)

	hr.logger.Printf("Node %s removed from hash ring", nodeID)
	return nil
}

// GetNode returns the node responsible for a given key
func (hr *HashRing) GetNode(key string) (string, error) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.sortedHashes) == 0 {
		return "", fmt.Errorf("no nodes available")
	}

	hash := hr.Hash(key)
	
	// Find the first node with hash >= key hash
	idx := sort.Search(len(hr.sortedHashes), func(i int) bool {
		return hr.sortedHashes[i] >= hash
	})

	// If no node found, wrap around to the first node
	if idx == len(hr.sortedHashes) {
		idx = 0
	}

	nodeHash := hr.sortedHashes[idx]
	nodeID := hr.ring[nodeHash]

	return nodeID, nil
}

// GetNodes returns N nodes responsible for a given key (for replication)
func (hr *HashRing) GetNodes(key string, count int) ([]string, error) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.nodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}

	if count <= 0 {
		count = 1
	}

	hash := hr.Hash(key)
	
	// Find the starting position
	idx := sort.Search(len(hr.sortedHashes), func(i int) bool {
		return hr.sortedHashes[i] >= hash
	})

	if idx == len(hr.sortedHashes) {
		idx = 0
	}

	// Collect unique nodes
	uniqueNodes := make(map[string]bool)
	result := make([]string, 0, count)

	for len(result) < count && len(uniqueNodes) < len(hr.nodes) {
		nodeHash := hr.sortedHashes[idx]
		nodeID := hr.ring[nodeHash]

		if !uniqueNodes[nodeID] {
			uniqueNodes[nodeID] = true
			
			// Check if node is active
			if node, exists := hr.nodes[nodeID]; exists && node.Status == NodeStatusActive {
				result = append(result, nodeID)
			}
		}

		idx = (idx + 1) % len(hr.sortedHashes)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no active nodes available")
	}

	return result, nil
}

// GetNodeInfo returns information about a specific node
func (hr *HashRing) GetNodeInfo(nodeID string) (*Node, error) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	node, exists := hr.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	// Return a copy to prevent external modification
	nodeCopy := *node
	return &nodeCopy, nil
}

// GetAllNodes returns information about all nodes
func (hr *HashRing) GetAllNodes() map[string]*Node {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	result := make(map[string]*Node)
	for nodeID, node := range hr.nodes {
		nodeCopy := *node
		result[nodeID] = &nodeCopy
	}

	return result
}

// UpdateNodeStatus updates the status of a node
func (hr *HashRing) UpdateNodeStatus(nodeID string, status NodeStatus) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	node, exists := hr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	hr.logger.Printf("Updating node %s status from %s to %s", nodeID, node.Status, status)
	node.Status = status

	return nil
}

// UpdateNodeLoad updates the load information for a node
func (hr *HashRing) UpdateNodeLoad(nodeID string, load int64) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	node, exists := hr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.Load = load
	return nil
}

// GetLoadDistribution returns the load distribution across all nodes
func (hr *HashRing) GetLoadDistribution() map[string]float64 {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	distribution := make(map[string]float64)
	var totalLoad int64

	// Calculate total load
	for _, node := range hr.nodes {
		if node.Status == NodeStatusActive {
			totalLoad += node.Load
		}
	}

	// Calculate percentage for each node
	for nodeID, node := range hr.nodes {
		if node.Status == NodeStatusActive {
			if totalLoad > 0 {
				distribution[nodeID] = float64(node.Load) / float64(totalLoad) * 100
			} else {
				distribution[nodeID] = 0
			}
		}
	}

	return distribution
}

// GetKeyDistribution returns the estimated key distribution across nodes
func (hr *HashRing) GetKeyDistribution() map[string]float64 {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.nodes) == 0 {
		return make(map[string]float64)
	}

	// Count virtual nodes per physical node
	virtualNodeCount := make(map[string]int)
	for _, nodeID := range hr.ring {
		virtualNodeCount[nodeID]++
	}

	// Calculate distribution percentages
	distribution := make(map[string]float64)
	totalVirtualNodes := len(hr.ring)

	for nodeID, count := range virtualNodeCount {
		distribution[nodeID] = float64(count) / float64(totalVirtualNodes) * 100
	}

	return distribution
}

// IsBalanced checks if the hash ring is reasonably balanced
func (hr *HashRing) IsBalanced(threshold float64) bool {
	distribution := hr.GetKeyDistribution()
	
	if len(distribution) == 0 {
		return true
	}

	expectedPercentage := 100.0 / float64(len(distribution))
	
	for _, percentage := range distribution {
		deviation := abs(percentage - expectedPercentage)
		if deviation > threshold {
			return false
		}
	}

	return true
}

// GetRebalanceOperations calculates the operations needed to rebalance after node changes
func (hr *HashRing) GetRebalanceOperations(oldRing *HashRing) []RebalanceOperation {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	var operations []RebalanceOperation

	// This is a simplified implementation
	// In a production system, this would be more sophisticated
	if oldRing == nil {
		return operations
	}

	// Find nodes that have been added or removed
	for nodeID := range hr.nodes {
		if _, exists := oldRing.nodes[nodeID]; !exists {
			// New node added - need to move some data to it
			operations = append(operations, RebalanceOperation{
				Type:       RebalanceTypeAdd,
				TargetNode: nodeID,
				Status:     "pending",
				Progress:   0.0,
			})
		}
	}

	for nodeID := range oldRing.nodes {
		if _, exists := hr.nodes[nodeID]; !exists {
			// Node removed - need to move data from it
			operations = append(operations, RebalanceOperation{
				Type:       RebalanceTypeRemove,
				SourceNode: nodeID,
				Status:     "pending",
				Progress:   0.0,
			})
		}
	}

	return operations
}

// GetStats returns statistics about the hash ring
func (hr *HashRing) GetStats() HashRingStats {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	stats := HashRingStats{
		TotalNodes:       len(hr.nodes),
		ActiveNodes:      0,
		VirtualNodes:     len(hr.ring),
		LoadDistribution: hr.GetLoadDistribution(),
		KeyDistribution:  hr.GetKeyDistribution(),
	}

	for _, node := range hr.nodes {
		if node.Status == NodeStatusActive {
			stats.ActiveNodes++
		}
	}

	return stats
}

// HashRingStats contains statistics about the hash ring
type HashRingStats struct {
	TotalNodes       int                `json:"total_nodes"`
	ActiveNodes      int                `json:"active_nodes"`
	VirtualNodes     int                `json:"virtual_nodes"`
	LoadDistribution map[string]float64 `json:"load_distribution"`
	KeyDistribution  map[string]float64 `json:"key_distribution"`
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Clone creates a deep copy of the hash ring
func (hr *HashRing) Clone() *HashRing {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	newRing := NewHashRing(hr.virtualNodes, hr.logger)

	// Copy all nodes
	for nodeID, node := range hr.nodes {
		nodeCopy := *node
		newRing.nodes[nodeID] = &nodeCopy
	}

	// Rebuild the ring
	for _, node := range newRing.nodes {
		for i := 0; i < hr.virtualNodes; i++ {
			virtualKey := fmt.Sprintf("%s:%d", node.ID, i)
			hash := newRing.Hash(virtualKey)
			newRing.ring[hash] = node.ID
			newRing.sortedHashes = append(newRing.sortedHashes, hash)
		}
	}

	// Sort the hashes
	sort.Slice(newRing.sortedHashes, func(i, j int) bool {
		return newRing.sortedHashes[i] < newRing.sortedHashes[j]
	})

	return newRing
}

// GetHashRange returns the hash range that a node is responsible for
func (hr *HashRing) GetHashRange(nodeID string) ([]KeyRange, error) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if _, exists := hr.nodes[nodeID]; !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	var ranges []KeyRange
	var prevHash uint64

	for i, hash := range hr.sortedHashes {
		if hr.ring[hash] == nodeID {
			var start uint64
			if i == 0 {
				// First node in ring - responsible from last hash to this hash
				if len(hr.sortedHashes) > 1 {
					start = hr.sortedHashes[len(hr.sortedHashes)-1] + 1
				} else {
					start = 0
				}
			} else {
				start = prevHash + 1
			}

			ranges = append(ranges, KeyRange{
				Start: start,
				End:   hash,
			})
		}
		prevHash = hash
	}

	return ranges, nil
}