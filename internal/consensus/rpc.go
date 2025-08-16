package consensus

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"distributed-kvstore/proto/cluster"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// RaftRPCServer implements the gRPC server for Raft operations
type RaftRPCServer struct {
	cluster.UnimplementedClusterServiceServer
	node   *RaftNode
	logger *log.Logger
}

// NewRaftRPCServer creates a new Raft RPC server
func NewRaftRPCServer(node *RaftNode) *RaftRPCServer {
	return &RaftRPCServer{
		node:   node,
		logger: node.logger,
	}
}

// RequestVote handles vote requests from other nodes
func (s *RaftRPCServer) RequestVote(ctx context.Context, req *cluster.VoteRequest) (*cluster.VoteResponse, error) {
	responseCh := make(chan *VoteResponse, 1)
	
	voteReq := &VoteRequest{
		Term:         req.Term,
		CandidateID:  req.CandidateId,
		LastLogIndex: req.LastLogIndex,
		LastLogTerm:  req.LastLogTerm,
		ResponseCh:   responseCh,
	}
	
	// Send to node's vote request channel
	select {
	case s.node.voteRequestCh <- voteReq:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout processing vote request")
	}
	
	// Wait for response
	select {
	case resp := <-responseCh:
		return &cluster.VoteResponse{
			Term:        resp.Term,
			VoteGranted: resp.VoteGranted,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for vote response")
	}
}

// AppendEntries handles append entries requests (heartbeats and log replication)
func (s *RaftRPCServer) AppendEntries(ctx context.Context, req *cluster.AppendRequest) (*cluster.AppendResponse, error) {
	responseCh := make(chan *AppendResponse, 1)
	
	appendReq := &AppendRequest{
		Term:         req.Term,
		LeaderID:     req.LeaderId,
		PrevLogIndex: req.PrevLogIndex,
		PrevLogTerm:  req.PrevLogTerm,
		Entries:      req.Entries,
		LeaderCommit: req.LeaderCommit,
		ResponseCh:   responseCh,
	}
	
	// Send to node's append entries channel
	select {
	case s.node.appendEntryCh <- appendReq:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout processing append entries request")
	}
	
	// Wait for response
	select {
	case resp := <-responseCh:
		return &cluster.AppendResponse{
			Term:         resp.Term,
			Success:      resp.Success,
			LastLogIndex: resp.LastLogIndex,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for append entries response")
	}
}

// Ping handles ping requests for connectivity testing
func (s *RaftRPCServer) Ping(ctx context.Context, req *cluster.PingRequest) (*cluster.PingResponse, error) {
	return &cluster.PingResponse{
		NodeId:    s.node.id,
		Timestamp: time.Now().Unix(),
		Status:    "ok",
	}, nil
}

// GetClusterStatus returns the current cluster status
func (s *RaftRPCServer) GetClusterStatus(ctx context.Context, req *cluster.ClusterStatusRequest) (*cluster.ClusterStatusResponse, error) {
	s.node.mu.RLock()
	defer s.node.mu.RUnlock()
	
	var nodes []*cluster.NodeInfo
	for id, peer := range s.node.peers {
		nodes = append(nodes, &cluster.NodeInfo{
			NodeId:   id,
			Address:  peer.Address,
			RaftPort: peer.RaftPort,
			GrpcPort: peer.GrpcPort,
			State:    "active", // Simplified for now
			Role:     "follower", // Would need to track actual roles
			LastSeen: peer.LastSeen.Unix(),
		})
	}
	
	// Add self
	role := s.node.state.String()
	nodes = append(nodes, &cluster.NodeInfo{
		NodeId:   s.node.id,
		Address:  s.node.address,
		RaftPort: s.node.raftPort,
		GrpcPort: s.node.grpcPort,
		State:    "active",
		Role:     role,
		LastSeen: time.Now().Unix(),
	})
	
	var leaderID string
	if s.node.state == Leader {
		leaderID = s.node.id
	} else {
		leaderID = s.node.GetLeader()
	}
	
	return &cluster.ClusterStatusResponse{
		LeaderId:    leaderID,
		TotalNodes:  int32(len(nodes)),
		ActiveNodes: int32(len(nodes)), // Simplified
		FailedNodes: 0,
		Nodes:       nodes,
		Health: &cluster.ClusterHealth{
			Status:         "healthy", // Simplified
			ConsensusRatio: 1.0,       // Simplified
			LastElection:   0,         // Would track actual elections
			Uptime:         int64(time.Since(time.Now()).Seconds()),
		},
	}, nil
}

// RaftRPCClient manages connections to other Raft nodes
type RaftRPCClient struct {
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn
	clients     map[string]cluster.ClusterServiceClient
	logger      *log.Logger
}

// NewRaftRPCClient creates a new Raft RPC client
func NewRaftRPCClient(logger *log.Logger) *RaftRPCClient {
	return &RaftRPCClient{
		connections: make(map[string]*grpc.ClientConn),
		clients:     make(map[string]cluster.ClusterServiceClient),
		logger:      logger,
	}
}

// Connect establishes a connection to a peer node
func (c *RaftRPCClient) Connect(nodeID, address string, port int32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	target := fmt.Sprintf("%s:%d", address, port)
	
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", target, err)
	}
	
	c.connections[nodeID] = conn
	c.clients[nodeID] = cluster.NewClusterServiceClient(conn)
	
	c.logger.Printf("Connected to peer %s at %s", nodeID, target)
	return nil
}

// Disconnect closes the connection to a peer node
func (c *RaftRPCClient) Disconnect(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if conn, exists := c.connections[nodeID]; exists {
		err := conn.Close()
		delete(c.connections, nodeID)
		delete(c.clients, nodeID)
		c.logger.Printf("Disconnected from peer %s", nodeID)
		return err
	}
	
	return nil
}

// RequestVote sends a vote request to a peer
func (c *RaftRPCClient) RequestVote(ctx context.Context, nodeID string, req *cluster.VoteRequest) (*cluster.VoteResponse, error) {
	c.mu.RLock()
	client, exists := c.clients[nodeID]
	c.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no connection to node %s", nodeID)
	}
	
	return client.RequestVote(ctx, req)
}

// AppendEntries sends an append entries request to a peer
func (c *RaftRPCClient) AppendEntries(ctx context.Context, nodeID string, req *cluster.AppendRequest) (*cluster.AppendResponse, error) {
	c.mu.RLock()
	client, exists := c.clients[nodeID]
	c.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no connection to node %s", nodeID)
	}
	
	return client.AppendEntries(ctx, req)
}

// Ping sends a ping request to a peer
func (c *RaftRPCClient) Ping(ctx context.Context, nodeID string) (*cluster.PingResponse, error) {
	c.mu.RLock()
	client, exists := c.clients[nodeID]
	c.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no connection to node %s", nodeID)
	}
	
	req := &cluster.PingRequest{
		NodeId:    nodeID,
		Timestamp: time.Now().Unix(),
	}
	
	return client.Ping(ctx, req)
}

// Close closes all connections
func (c *RaftRPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	var lastErr error
	for nodeID, conn := range c.connections {
		if err := conn.Close(); err != nil {
			lastErr = err
		}
		delete(c.connections, nodeID)
		delete(c.clients, nodeID)
	}
	
	return lastErr
}

// RaftNetworkManager manages the network layer for Raft operations
type RaftNetworkManager struct {
	node   *RaftNode
	server *grpc.Server
	client *RaftRPCClient
	logger *log.Logger
}

// NewRaftNetworkManager creates a new network manager
func NewRaftNetworkManager(node *RaftNode) *RaftNetworkManager {
	return &RaftNetworkManager{
		node:   node,
		client: NewRaftRPCClient(node.logger),
		logger: node.logger,
	}
}

// Start starts the network manager (gRPC server)
func (nm *RaftNetworkManager) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", nm.node.raftPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", nm.node.raftPort, err)
	}
	
	nm.server = grpc.NewServer()
	rpcServer := NewRaftRPCServer(nm.node)
	cluster.RegisterClusterServiceServer(nm.server, rpcServer)
	
	nm.logger.Printf("Starting Raft gRPC server on port %d", nm.node.raftPort)
	
	go func() {
		if err := nm.server.Serve(listener); err != nil {
			nm.logger.Printf("gRPC server error: %v", err)
		}
	}()
	
	return nil
}

// Stop stops the network manager
func (nm *RaftNetworkManager) Stop() {
	if nm.server != nil {
		nm.server.GracefulStop()
	}
	nm.client.Close()
}

// ConnectToPeer connects to a peer node
func (nm *RaftNetworkManager) ConnectToPeer(nodeID, address string, port int32) error {
	return nm.client.Connect(nodeID, address, port)
}

// DisconnectFromPeer disconnects from a peer node
func (nm *RaftNetworkManager) DisconnectFromPeer(nodeID string) error {
	return nm.client.Disconnect(nodeID)
}

// SendVoteRequest sends a vote request to a peer
func (nm *RaftNetworkManager) SendVoteRequest(ctx context.Context, nodeID string, req *cluster.VoteRequest) (*cluster.VoteResponse, error) {
	return nm.client.RequestVote(ctx, nodeID, req)
}

// SendAppendEntries sends an append entries request to a peer
func (nm *RaftNetworkManager) SendAppendEntries(ctx context.Context, nodeID string, req *cluster.AppendRequest) (*cluster.AppendResponse, error) {
	return nm.client.AppendEntries(ctx, nodeID, req)
}

// SendPing sends a ping to a peer
func (nm *RaftNetworkManager) SendPing(ctx context.Context, nodeID string) (*cluster.PingResponse, error) {
	return nm.client.Ping(ctx, nodeID)
}