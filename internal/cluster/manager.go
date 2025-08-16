package cluster

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"distributed-kvstore/internal/consensus"
	"distributed-kvstore/internal/config"
)

// Manager manages the cluster state and membership
type Manager struct {
	mu     sync.RWMutex
	config config.ClusterConfig
	
	// Raft consensus
	raftNode    *consensus.RaftNode
	networkMgr  *consensus.RaftNetworkManager
	
	// Cluster state
	nodes       map[string]*NodeInfo
	localNodeID string
	
	// Control
	running bool
	stopCh  chan struct{}
	
	logger *log.Logger
}

// NodeInfo contains information about a cluster node
type NodeInfo struct {
	ID       string
	Address  string
	RaftPort int32
	GrpcPort int32
	State    NodeState
	LastSeen time.Time
	Metadata map[string]string
}

// NodeState represents the state of a node in the cluster
type NodeState int

const (
	NodeStateActive NodeState = iota
	NodeStateInactive
	NodeStateFailed
	NodeStateLeaving
)

func (s NodeState) String() string {
	switch s {
	case NodeStateActive:
		return "active"
	case NodeStateInactive:
		return "inactive"
	case NodeStateFailed:
		return "failed"
	case NodeStateLeaving:
		return "leaving"
	default:
		return "unknown"
	}
}

// NewManager creates a new cluster manager
func NewManager(cfg config.ClusterConfig, logger *log.Logger) (*Manager, error) {
	if logger == nil {
		logger = log.New(log.Writer(), "[cluster] ", log.LstdFlags)
	}
	
	manager := &Manager{
		config:      cfg,
		nodes:       make(map[string]*NodeInfo),
		localNodeID: cfg.NodeID,
		stopCh:      make(chan struct{}),
		logger:      logger,
	}
	
	// Create local node info
	localNode := &NodeInfo{
		ID:       cfg.NodeID,
		Address:  "localhost", // This should come from server config
		RaftPort: int32(cfg.RaftPort),
		GrpcPort: 9090, // This should come from server config
		State:    NodeStateActive,
		LastSeen: time.Now(),
		Metadata: make(map[string]string),
	}
	manager.nodes[cfg.NodeID] = localNode
	
	return manager, nil
}

// Start starts the cluster manager
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.running {
		return fmt.Errorf("cluster manager already running")
	}
	
	m.logger.Printf("Starting cluster manager for node %s", m.localNodeID)
	
	// Initialize Raft node
	raftConfig := consensus.Config{
		NodeID:           m.localNodeID,
		Address:          m.nodes[m.localNodeID].Address,
		RaftPort:         m.nodes[m.localNodeID].RaftPort,
		GrpcPort:         m.nodes[m.localNodeID].GrpcPort,
		ElectionTimeout:  m.config.ElectionTick,
		HeartbeatTimeout: m.config.HeartbeatTick,
		Logger:           m.logger,
	}
	
	var err error
	m.raftNode, err = consensus.NewRaftNode(raftConfig)
	if err != nil {
		return fmt.Errorf("failed to create raft node: %w", err)
	}
	
	// Initialize network manager
	m.networkMgr = consensus.NewRaftNetworkManager(m.raftNode)
	if err := m.networkMgr.Start(); err != nil {
		return fmt.Errorf("failed to start network manager: %w", err)
	}
	
	// Start Raft node
	m.raftNode.Start()
	
	// Connect to initial peers
	for _, peerAddr := range m.config.Peers {
		if err := m.addInitialPeer(peerAddr); err != nil {
			m.logger.Printf("Failed to add initial peer %s: %v", peerAddr, err)
		}
	}
	
	m.running = true
	
	// Start background tasks
	go m.runHealthChecker()
	go m.runNodeDiscovery()
	
	m.logger.Printf("Cluster manager started successfully")
	return nil
}

// Stop stops the cluster manager
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return
	}
	
	m.logger.Printf("Stopping cluster manager")
	
	close(m.stopCh)
	
	if m.raftNode != nil {
		m.raftNode.Stop()
	}
	
	if m.networkMgr != nil {
		m.networkMgr.Stop()
	}
	
	m.running = false
	m.logger.Printf("Cluster manager stopped")
}

// GetNodes returns information about all known nodes
func (m *Manager) GetNodes() map[string]*NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	nodes := make(map[string]*NodeInfo)
	for id, node := range m.nodes {
		nodes[id] = &NodeInfo{
			ID:       node.ID,
			Address:  node.Address,
			RaftPort: node.RaftPort,
			GrpcPort: node.GrpcPort,
			State:    node.State,
			LastSeen: node.LastSeen,
			Metadata: make(map[string]string),
		}
		// Copy metadata
		for k, v := range node.Metadata {
			nodes[id].Metadata[k] = v
		}
	}
	
	return nodes
}

// GetLeader returns the current cluster leader
func (m *Manager) GetLeader() string {
	if m.raftNode == nil {
		return ""
	}
	return m.raftNode.GetLeader()
}

// GetLocalNode returns information about the local node
func (m *Manager) GetLocalNode() *NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	node := m.nodes[m.localNodeID]
	if node == nil {
		return nil
	}
	
	return &NodeInfo{
		ID:       node.ID,
		Address:  node.Address,
		RaftPort: node.RaftPort,
		GrpcPort: node.GrpcPort,
		State:    node.State,
		LastSeen: node.LastSeen,
		Metadata: make(map[string]string),
	}
}

// IsLeader returns true if the local node is the cluster leader
func (m *Manager) IsLeader() bool {
	if m.raftNode == nil {
		return false
	}
	_, _, isLeader := m.raftNode.GetState()
	return isLeader
}

// GetClusterSize returns the number of nodes in the cluster
func (m *Manager) GetClusterSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}

// GetActiveNodes returns the number of active nodes
func (m *Manager) GetActiveNodes() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	count := 0
	for _, node := range m.nodes {
		if node.State == NodeStateActive {
			count++
		}
	}
	return count
}

// JoinCluster attempts to join an existing cluster
func (m *Manager) JoinCluster(leaderAddr string) error {
	m.logger.Printf("Attempting to join cluster via leader at %s", leaderAddr)
	
	// This would implement the actual cluster join protocol
	// For now, we'll just add the leader as a peer
	return m.addInitialPeer(leaderAddr)
}

// LeaveCluster gracefully leaves the cluster
func (m *Manager) LeaveCluster() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.logger.Printf("Leaving cluster")
	
	// Mark local node as leaving
	if localNode := m.nodes[m.localNodeID]; localNode != nil {
		localNode.State = NodeStateLeaving
	}
	
	// Notify other nodes (in a real implementation)
	// For now, we'll just stop
	m.Stop()
	
	return nil
}

// addInitialPeer adds a peer from the initial configuration
func (m *Manager) addInitialPeer(peerAddr string) error {
	// Parse peer address (format: "host:port" or just "host")
	// For simplicity, assuming format "nodeID@host:raftPort:grpcPort"
	// In a real implementation, this would be more sophisticated
	
	// For now, create a mock peer
	peerID := fmt.Sprintf("peer-%s", peerAddr)
	
	peer := &NodeInfo{
		ID:       peerID,
		Address:  peerAddr,
		RaftPort: int32(m.config.RaftPort), // Assume same port
		GrpcPort: 9090,                     // Assume standard gRPC port
		State:    NodeStateActive,
		LastSeen: time.Now(),
		Metadata: make(map[string]string),
	}
	
	m.mu.Lock()
	m.nodes[peerID] = peer
	m.mu.Unlock()
	
	// Add to Raft node
	if m.raftNode != nil {
		m.raftNode.AddPeer(peerID, peer.Address, peer.RaftPort, peer.GrpcPort)
	}
	
	// Connect via network manager
	if m.networkMgr != nil {
		if err := m.networkMgr.ConnectToPeer(peerID, peer.Address, peer.RaftPort); err != nil {
			m.logger.Printf("Failed to connect to peer %s: %v", peerID, err)
			return err
		}
	}
	
	m.logger.Printf("Added initial peer %s", peerID)
	return nil
}

// runHealthChecker periodically checks the health of cluster nodes
func (m *Manager) runHealthChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkNodeHealth()
		}
	}
}

// checkNodeHealth checks the health of all known nodes
func (m *Manager) checkNodeHealth() {
	m.mu.RLock()
	nodes := make(map[string]*NodeInfo)
	for id, node := range m.nodes {
		if id != m.localNodeID { // Don't ping ourselves
			nodes[id] = node
		}
	}
	m.mu.RUnlock()
	
	for nodeID, node := range nodes {
		go func(nodeID string, node *NodeInfo) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
			if m.networkMgr != nil {
				_, err := m.networkMgr.SendPing(ctx, nodeID)
				
				m.mu.Lock()
				if err != nil {
					// Node is unreachable
					if time.Since(node.LastSeen) > 60*time.Second {
						node.State = NodeStateFailed
						m.logger.Printf("Marked node %s as failed", nodeID)
					} else {
						node.State = NodeStateInactive
					}
				} else {
					// Node is reachable
					node.State = NodeStateActive
					node.LastSeen = time.Now()
				}
				m.mu.Unlock()
			}
		}(nodeID, node)
	}
}

// runNodeDiscovery periodically discovers new nodes (placeholder)
func (m *Manager) runNodeDiscovery() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			// In a real implementation, this would discover new nodes
			// through various mechanisms (DNS, service discovery, etc.)
			m.logger.Printf("Node discovery check (placeholder)")
		}
	}
}

// GetClusterStatus returns the current cluster status
func (m *Manager) GetClusterStatus() *ClusterStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	status := &ClusterStatus{
		Leader:      m.GetLeader(),
		TotalNodes:  len(m.nodes),
		ActiveNodes: m.GetActiveNodes(),
		Nodes:       make([]*NodeInfo, 0, len(m.nodes)),
	}
	
	for _, node := range m.nodes {
		nodeCopy := &NodeInfo{
			ID:       node.ID,
			Address:  node.Address,
			RaftPort: node.RaftPort,
			GrpcPort: node.GrpcPort,
			State:    node.State,
			LastSeen: node.LastSeen,
			Metadata: make(map[string]string),
		}
		for k, v := range node.Metadata {
			nodeCopy.Metadata[k] = v
		}
		status.Nodes = append(status.Nodes, nodeCopy)
	}
	
	status.FailedNodes = status.TotalNodes - status.ActiveNodes
	
	return status
}

// ClusterStatus represents the current status of the cluster
type ClusterStatus struct {
	Leader      string      `json:"leader"`
	TotalNodes  int         `json:"total_nodes"`
	ActiveNodes int         `json:"active_nodes"`
	FailedNodes int         `json:"failed_nodes"`
	Nodes       []*NodeInfo `json:"nodes"`
}

// Health returns the cluster health status
func (cs *ClusterStatus) Health() string {
	if cs.ActiveNodes == 0 {
		return "critical"
	}
	
	activeRatio := float64(cs.ActiveNodes) / float64(cs.TotalNodes)
	if activeRatio >= 0.8 {
		return "healthy"
	} else if activeRatio >= 0.5 {
		return "degraded"
	} else {
		return "unhealthy"
	}
}