package consensus

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"distributed-kvstore/proto/cluster"
)

// ClusterManager manages the distributed cluster of Raft nodes
type ClusterManager struct {
	mu          sync.RWMutex
	nodes       map[string]*RaftNode
	config      *ClusterConfig
	logger      *log.Logger
	grpcServers map[string]*grpc.Server
	listeners   map[string]net.Listener
	shutdown    chan struct{}
}

// ClusterConfig contains configuration for the cluster
type ClusterConfig struct {
	Nodes          []*NodeConfig `json:"nodes"`
	ElectionTimeout time.Duration `json:"election_timeout"`
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout"`
	DataDir        string        `json:"data_dir"`
}

// NodeConfig contains configuration for a single node
type NodeConfig struct {
	NodeID   string `json:"node_id"`
	Address  string `json:"address"`
	RaftPort int32  `json:"raft_port"`
	GrpcPort int32  `json:"grpc_port"`
}

// ClusterService implements the gRPC service for cluster communication
type ClusterService struct {
	cluster.UnimplementedClusterServiceServer
	manager *ClusterManager
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(config *ClusterConfig, logger *log.Logger) *ClusterManager {
	if logger == nil {
		logger = log.New(log.Writer(), "[CLUSTER] ", log.LstdFlags)
	}

	return &ClusterManager{
		nodes:       make(map[string]*RaftNode),
		config:      config,
		logger:      logger,
		grpcServers: make(map[string]*grpc.Server),
		listeners:   make(map[string]net.Listener),
		shutdown:    make(chan struct{}),
	}
}

// Bootstrap initializes the cluster with the configured nodes
func (cm *ClusterManager) Bootstrap() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.logger.Printf("Bootstrapping cluster with %d nodes", len(cm.config.Nodes))

	// Create and start all nodes
	for _, nodeConfig := range cm.config.Nodes {
		if err := cm.createNode(nodeConfig); err != nil {
			return fmt.Errorf("failed to create node %s: %w", nodeConfig.NodeID, err)
		}
	}

	// Connect all nodes to each other
	for _, nodeConfig := range cm.config.Nodes {
		node := cm.nodes[nodeConfig.NodeID]
		for _, peerConfig := range cm.config.Nodes {
			if nodeConfig.NodeID != peerConfig.NodeID {
				node.AddPeer(peerConfig.NodeID, peerConfig.Address, peerConfig.RaftPort, peerConfig.GrpcPort)
			}
		}
	}

	// Start all nodes
	for _, node := range cm.nodes {
		node.Start()
	}

	cm.logger.Printf("Cluster bootstrap completed successfully")
	return nil
}

// createNode creates and configures a single Raft node
func (cm *ClusterManager) createNode(nodeConfig *NodeConfig) error {
	// Create state machine for the node
	stateMachine := NewKVStateMachine(log.New(log.Writer(), fmt.Sprintf("[SM-%s] ", nodeConfig.NodeID), log.LstdFlags))

	// Create Raft node configuration
	config := Config{
		NodeID:           nodeConfig.NodeID,
		Address:          nodeConfig.Address,
		RaftPort:         nodeConfig.RaftPort,
		GrpcPort:         nodeConfig.GrpcPort,
		ElectionTimeout:  cm.config.ElectionTimeout,
		HeartbeatTimeout: cm.config.HeartbeatTimeout,
		StateMachine:     stateMachine,
		Logger:           log.New(log.Writer(), fmt.Sprintf("[%s] ", nodeConfig.NodeID), log.LstdFlags),
	}

	// Create the Raft node
	node, err := NewRaftNode(config)
	if err != nil {
		return fmt.Errorf("failed to create Raft node: %w", err)
	}

	cm.nodes[nodeConfig.NodeID] = node

	// Start gRPC server for the node
	if err := cm.startGRPCServer(nodeConfig, node); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	cm.logger.Printf("Created node %s at %s:%d", nodeConfig.NodeID, nodeConfig.Address, nodeConfig.GrpcPort)
	return nil
}

// startGRPCServer starts the gRPC server for cluster communication
func (cm *ClusterManager) startGRPCServer(nodeConfig *NodeConfig, node *RaftNode) error {
	address := fmt.Sprintf("%s:%d", nodeConfig.Address, nodeConfig.GrpcPort)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	server := grpc.NewServer()
	service := &ClusterService{manager: cm}
	cluster.RegisterClusterServiceServer(server, service)

	cm.listeners[nodeConfig.NodeID] = listener
	cm.grpcServers[nodeConfig.NodeID] = server

	// Start server in goroutine
	go func() {
		cm.logger.Printf("Starting gRPC server for node %s on %s", nodeConfig.NodeID, address)
		if err := server.Serve(listener); err != nil {
			cm.logger.Printf("gRPC server for node %s stopped: %v", nodeConfig.NodeID, err)
		}
	}()

	return nil
}

// AddNode dynamically adds a new node to the cluster
func (cm *ClusterManager) AddNode(nodeConfig *NodeConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.nodes[nodeConfig.NodeID]; exists {
		return fmt.Errorf("node %s already exists in cluster", nodeConfig.NodeID)
	}

	cm.logger.Printf("Adding new node %s to cluster", nodeConfig.NodeID)

	// Create the new node
	if err := cm.createNode(nodeConfig); err != nil {
		return fmt.Errorf("failed to create new node: %w", err)
	}

	newNode := cm.nodes[nodeConfig.NodeID]

	// Connect new node to existing nodes
	for _, existingNodeConfig := range cm.config.Nodes {
		if existingNodeConfig.NodeID != nodeConfig.NodeID {
			newNode.AddPeer(existingNodeConfig.NodeID, existingNodeConfig.Address, 
				existingNodeConfig.RaftPort, existingNodeConfig.GrpcPort)
		}
	}

	// Connect existing nodes to new node
	for _, existingNode := range cm.nodes {
		if existingNode.id != nodeConfig.NodeID {
			existingNode.AddPeer(nodeConfig.NodeID, nodeConfig.Address, 
				nodeConfig.RaftPort, nodeConfig.GrpcPort)
		}
	}

	// Add to configuration
	cm.config.Nodes = append(cm.config.Nodes, nodeConfig)

	// Start the new node
	newNode.Start()

	cm.logger.Printf("Successfully added node %s to cluster", nodeConfig.NodeID)
	return nil
}

// RemoveNode dynamically removes a node from the cluster
func (cm *ClusterManager) RemoveNode(nodeID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	node, exists := cm.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in cluster", nodeID)
	}

	cm.logger.Printf("Removing node %s from cluster", nodeID)

	// Stop the node
	node.Stop()

	// Stop gRPC server
	if server, exists := cm.grpcServers[nodeID]; exists {
		server.GracefulStop()
		delete(cm.grpcServers, nodeID)
	}

	// Close listener
	if listener, exists := cm.listeners[nodeID]; exists {
		listener.Close()
		delete(cm.listeners, nodeID)
	}

	// Remove from existing nodes
	for _, existingNode := range cm.nodes {
		if existingNode.id != nodeID {
			existingNode.RemovePeer(nodeID)
		}
	}

	// Remove from cluster
	delete(cm.nodes, nodeID)

	// Remove from configuration
	newNodes := make([]*NodeConfig, 0, len(cm.config.Nodes)-1)
	for _, nodeConfig := range cm.config.Nodes {
		if nodeConfig.NodeID != nodeID {
			newNodes = append(newNodes, nodeConfig)
		}
	}
	cm.config.Nodes = newNodes

	cm.logger.Printf("Successfully removed node %s from cluster", nodeID)
	return nil
}

// GetLeader returns the current leader node ID
func (cm *ClusterManager) GetLeader() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, node := range cm.nodes {
		if _, _, isLeader := node.GetState(); isLeader {
			return node.id
		}
	}
	return ""
}

// GetClusterStatus returns the status of all nodes in the cluster
func (cm *ClusterManager) GetClusterStatus() map[string]ClusterNodeStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	status := make(map[string]ClusterNodeStatus)
	for nodeID, node := range cm.nodes {
		state, term, isLeader := node.GetState()
		status[nodeID] = ClusterNodeStatus{
			NodeID:    nodeID,
			State:     state.String(),
			Term:      term,
			IsLeader:  isLeader,
			Peers:     len(node.peers),
			LastSeen:  time.Now(),
		}
	}
	return status
}

// ClusterNodeStatus represents the status of a single node
type ClusterNodeStatus struct {
	NodeID    string    `json:"node_id"`
	State     string    `json:"state"`
	Term      int64     `json:"term"`
	IsLeader  bool      `json:"is_leader"`
	Peers     int       `json:"peers"`
	LastSeen  time.Time `json:"last_seen"`
}

// Stop gracefully stops the entire cluster
func (cm *ClusterManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.logger.Printf("Stopping cluster with %d nodes", len(cm.nodes))

	// Signal shutdown
	close(cm.shutdown)

	// Stop all nodes
	for nodeID, node := range cm.nodes {
		cm.logger.Printf("Stopping node %s", nodeID)
		node.Stop()
	}

	// Stop all gRPC servers
	for nodeID, server := range cm.grpcServers {
		cm.logger.Printf("Stopping gRPC server for node %s", nodeID)
		server.GracefulStop()
	}

	// Close all listeners
	for nodeID, listener := range cm.listeners {
		cm.logger.Printf("Closing listener for node %s", nodeID)
		listener.Close()
	}

	cm.logger.Printf("Cluster stopped successfully")
}

// WaitForLeader waits for a leader to be elected
func (cm *ClusterManager) WaitForLeader(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		if leader := cm.GetLeader(); leader != "" {
			return leader, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	return "", fmt.Errorf("no leader elected within %v", timeout)
}

// GetNode returns a specific node by ID
func (cm *ClusterManager) GetNode(nodeID string) (*RaftNode, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	node, exists := cm.nodes[nodeID]
	return node, exists
}

// GetNodes returns all nodes in the cluster
func (cm *ClusterManager) GetNodes() map[string]*RaftNode {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	nodes := make(map[string]*RaftNode)
	for nodeID, node := range cm.nodes {
		nodes[nodeID] = node
	}
	return nodes
}

// IsHealthy checks if the cluster is healthy (has a leader and majority of nodes are running)
func (cm *ClusterManager) IsHealthy() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check if we have a leader
	leader := ""
	runningNodes := 0
	
	for _, node := range cm.nodes {
		if _, _, isLeader := node.GetState(); isLeader {
			leader = node.id
		}
		runningNodes++
	}

	// Cluster is healthy if:
	// 1. We have a leader
	// 2. We have at least a majority of configured nodes running
	totalNodes := len(cm.config.Nodes)
	majorityNodes := (totalNodes / 2) + 1

	return leader != "" && runningNodes >= majorityNodes
}

// SimulatePartition creates a network partition for testing
func (cm *ClusterManager) SimulatePartition(partition1, partition2 []string) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cm.logger.Printf("Simulating network partition: %v | %v", partition1, partition2)

	// Get partition tolerance manager from first node
	var pt *PartitionTolerance
	for _, node := range cm.nodes {
		pt = node.partitionTolerance
		break
	}

	if pt == nil {
		return fmt.Errorf("no partition tolerance manager available")
	}

	// Create partitions
	pt.CreatePartition("partition1", partition1)
	pt.CreatePartition("partition2", partition2)

	return nil
}

// HealPartition removes all network partitions
func (cm *ClusterManager) HealPartition() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cm.logger.Printf("Healing network partitions")

	// Get partition tolerance manager from first node
	var pt *PartitionTolerance
	for _, node := range cm.nodes {
		pt = node.partitionTolerance
		break
	}

	if pt == nil {
		return fmt.Errorf("no partition tolerance manager available")
	}

	// Remove all partitions
	partitions := pt.GetPartitionInfo()
	for partitionID := range partitions {
		pt.RemovePartition(partitionID)
	}

	return nil
}

// gRPC service implementations

// RequestVote handles vote requests between nodes
func (cs *ClusterService) RequestVote(ctx context.Context, req *cluster.VoteRequest) (*cluster.VoteResponse, error) {
	// This would implement actual RPC vote handling
	// For now, we'll use the existing simulation logic
	return &cluster.VoteResponse{
		Term:        req.Term,
		VoteGranted: true, // Simplified for demo
	}, nil
}

// AppendEntries handles append entries requests between nodes
func (cs *ClusterService) AppendEntries(ctx context.Context, req *cluster.AppendRequest) (*cluster.AppendResponse, error) {
	// This would implement actual RPC append entries handling
	// For now, we'll use the existing simulation logic
	return &cluster.AppendResponse{
		Term:         req.Term,
		Success:      true, // Simplified for demo
		LastLogIndex: req.PrevLogIndex + int64(len(req.Entries)),
	}, nil
}

// JoinCluster handles requests from nodes wanting to join the cluster
func (cs *ClusterService) JoinCluster(ctx context.Context, req *cluster.JoinRequest) (*cluster.JoinResponse, error) {
	nodeConfig := &NodeConfig{
		NodeID:   req.NodeId,
		Address:  req.Address,
		RaftPort: req.RaftPort,
		GrpcPort: req.GrpcPort,
	}

	if err := cs.manager.AddNode(nodeConfig); err != nil {
		return &cluster.JoinResponse{
			Success: false,
			Error: fmt.Sprintf("Failed to join cluster: %v", err),
		}, nil
	}

	return &cluster.JoinResponse{
		Success: true,
		LeaderId: cs.manager.GetLeader(),
	}, nil
}

// LeaveCluster handles requests from nodes wanting to leave the cluster
func (cs *ClusterService) LeaveCluster(ctx context.Context, req *cluster.LeaveRequest) (*cluster.LeaveResponse, error) {
	if err := cs.manager.RemoveNode(req.NodeId); err != nil {
		return &cluster.LeaveResponse{
			Success: false,
			Error: fmt.Sprintf("Failed to leave cluster: %v", err),
		}, nil
	}

	return &cluster.LeaveResponse{
		Success: true,
	}, nil
}

// GetClusterStatus returns status information about the cluster
func (cs *ClusterService) GetClusterStatus(ctx context.Context, req *cluster.ClusterStatusRequest) (*cluster.ClusterStatusResponse, error) {
	status := cs.manager.GetClusterStatus()
	
	var nodes []*cluster.NodeInfo
	activeNodes := int32(0)
	for _, nodeStatus := range status {
		nodes = append(nodes, &cluster.NodeInfo{
			NodeId:   nodeStatus.NodeID,
			State:    nodeStatus.State,
			Role:     nodeStatus.State, // Using state as role for simplicity
		})
		activeNodes++
	}

	health := &cluster.ClusterHealth{
		Status: "healthy",
		ConsensusRatio: 1.0,
		LastElection: time.Now().Unix(),
		Uptime: 0,
	}

	if !cs.manager.IsHealthy() {
		health.Status = "unhealthy"
		health.ConsensusRatio = 0.5
	}

	return &cluster.ClusterStatusResponse{
		LeaderId: cs.manager.GetLeader(),
		TotalNodes: int32(len(nodes)),
		ActiveNodes: activeNodes,
		FailedNodes: 0,
		Nodes: nodes,
		Health: health,
	}, nil
}

// GetNodes returns information about all nodes in the cluster
func (cs *ClusterService) GetNodes(ctx context.Context, req *cluster.GetNodesRequest) (*cluster.GetNodesResponse, error) {
	status := cs.manager.GetClusterStatus()
	
	var nodes []*cluster.NodeInfo
	for _, nodeStatus := range status {
		nodes = append(nodes, &cluster.NodeInfo{
			NodeId:   nodeStatus.NodeID,
			State:    nodeStatus.State,
			Role:     nodeStatus.State,
		})
	}

	return &cluster.GetNodesResponse{
		Nodes: nodes,
	}, nil
}

// InstallSnapshot handles snapshot installation requests
func (cs *ClusterService) InstallSnapshot(ctx context.Context, req *cluster.SnapshotRequest) (*cluster.SnapshotResponse, error) {
	// This would implement actual snapshot installation
	// For now, we'll use simplified logic
	return &cluster.SnapshotResponse{
		Term:    req.Term,
		Success: true,
	}, nil
}

// ReplicateData handles data replication requests
func (cs *ClusterService) ReplicateData(ctx context.Context, req *cluster.ReplicationRequest) (*cluster.ReplicationResponse, error) {
	// This would implement actual data replication
	// For now, we'll use simplified logic
	return &cluster.ReplicationResponse{
		Success:     true,
		LastApplied: req.LogIndex,
	}, nil
}

// SyncData handles data synchronization requests
func (cs *ClusterService) SyncData(ctx context.Context, req *cluster.SyncRequest) (*cluster.SyncResponse, error) {
	// This would implement actual data synchronization
	// For now, we'll use simplified logic
	return &cluster.SyncResponse{
		CurrentIndex: req.LastIndex + 1,
		HasMore:      false,
	}, nil
}

// Ping handles ping requests for health checking
func (cs *ClusterService) Ping(ctx context.Context, req *cluster.PingRequest) (*cluster.PingResponse, error) {
	return &cluster.PingResponse{
		NodeId:    req.NodeId,
		Timestamp: time.Now().Unix(),
		Status:    "healthy",
	}, nil
}

// ClusterJoiner helps new nodes join an existing cluster
type ClusterJoiner struct {
	logger *log.Logger
}

// NewClusterJoiner creates a new cluster joiner
func NewClusterJoiner(logger *log.Logger) *ClusterJoiner {
	if logger == nil {
		logger = log.New(log.Writer(), "[JOINER] ", log.LstdFlags)
	}
	return &ClusterJoiner{logger: logger}
}

// JoinCluster attempts to join an existing cluster
func (cj *ClusterJoiner) JoinCluster(nodeConfig *NodeConfig, existingNodes []string) error {
	cj.logger.Printf("Attempting to join cluster with node %s", nodeConfig.NodeID)

	for _, existingNode := range existingNodes {
		cj.logger.Printf("Trying to contact existing node at %s", existingNode)
		
		conn, err := grpc.Dial(existingNode, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			cj.logger.Printf("Failed to connect to %s: %v", existingNode, err)
			continue
		}
		defer conn.Close()

		client := cluster.NewClusterServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req := &cluster.JoinRequest{
			NodeId:   nodeConfig.NodeID,
			Address:  nodeConfig.Address,
			RaftPort: nodeConfig.RaftPort,
			GrpcPort: nodeConfig.GrpcPort,
		}

		resp, err := client.JoinCluster(ctx, req)
		if err != nil {
			cj.logger.Printf("Join request to %s failed: %v", existingNode, err)
			continue
		}

		if resp.Success {
			cj.logger.Printf("Successfully joined cluster via %s", existingNode)
			return nil
		} else {
			cj.logger.Printf("Join request rejected by %s: %s", existingNode, resp.Error)
		}
	}

	return fmt.Errorf("failed to join cluster through any existing node")
}