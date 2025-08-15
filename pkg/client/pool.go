package client

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Connection represents a gRPC connection with metadata
type Connection struct {
	conn       *grpc.ClientConn
	address    string
	lastUsed   time.Time
	inUse      bool
	created    time.Time
}

// ConnectionPool manages a pool of gRPC connections
type ConnectionPool struct {
	config      *Config
	connections []*Connection
	mu          sync.RWMutex
	closed      bool
	closeCh     chan struct{}
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config *Config) (*ConnectionPool, error) {
	pool := &ConnectionPool{
		config:      config,
		connections: make([]*Connection, 0),
		closeCh:     make(chan struct{}),
	}

	// Create initial connections
	for i := 0; i < min(config.MaxConnections, len(config.Addresses)); i++ {
		address := config.Addresses[i%len(config.Addresses)]
		conn, err := pool.createConnection(address)
		if err != nil {
			// Log error but don't fail pool creation
			continue
		}
		pool.connections = append(pool.connections, conn)
	}

	if len(pool.connections) == 0 {
		return nil, fmt.Errorf("failed to create any connections")
	}

	// Start background cleanup routine
	go pool.cleanup()

	return pool, nil
}

// GetConnection retrieves a connection from the pool
func (p *ConnectionPool) GetConnection(ctx context.Context) (*Connection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("connection pool is closed")
	}

	// Try to find an available connection
	for _, conn := range p.connections {
		if !conn.inUse && p.isConnectionHealthy(conn) {
			conn.inUse = true
			conn.lastUsed = time.Now()
			return conn, nil
		}
	}

	// If no available connection and we haven't reached max, create a new one
	if len(p.connections) < p.config.MaxConnections {
		address := p.selectAddress()
		conn, err := p.createConnection(address)
		if err != nil {
			return nil, fmt.Errorf("failed to create new connection: %w", err)
		}
		conn.inUse = true
		conn.lastUsed = time.Now()
		p.connections = append(p.connections, conn)
		return conn, nil
	}

	// Wait for a connection to become available
	return p.waitForConnection(ctx)
}

// PutConnection returns a connection to the pool
func (p *ConnectionPool) PutConnection(conn *Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		p.closeConnection(conn)
		return
	}

	conn.inUse = false
	conn.lastUsed = time.Now()
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.closeCh)

	var lastErr error
	for _, conn := range p.connections {
		if err := p.closeConnection(conn); err != nil {
			lastErr = err
		}
	}

	p.connections = nil
	return lastErr
}

// createConnection creates a new gRPC connection
func (p *ConnectionPool) createConnection(address string) (*Connection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectionTimeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}

	if p.config.TLSEnabled {
		// TODO: Add TLS credentials
		return nil, fmt.Errorf("TLS support not implemented yet")
	}

	grpcConn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", address, err)
	}

	return &Connection{
		conn:     grpcConn,
		address:  address,
		lastUsed: time.Now(),
		created:  time.Now(),
		inUse:    false,
	}, nil
}

// closeConnection closes a single connection
func (p *ConnectionPool) closeConnection(conn *Connection) error {
	if conn.conn != nil {
		return conn.conn.Close()
	}
	return nil
}

// isConnectionHealthy checks if a connection is healthy
func (p *ConnectionPool) isConnectionHealthy(conn *Connection) bool {
	if conn.conn == nil {
		return false
	}

	state := conn.conn.GetState()
	switch state {
	case connectivity.Ready, connectivity.Idle:
		return true
	case connectivity.Connecting:
		// Give connecting connections a chance
		return time.Since(conn.created) < p.config.ConnectionTimeout
	default:
		return false
	}
}

// waitForConnection waits for a connection to become available
func (p *ConnectionPool) waitForConnection(ctx context.Context) (*Connection, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.closeCh:
			return nil, fmt.Errorf("connection pool is closed")
		case <-ticker.C:
			// Check again for available connections
			for _, conn := range p.connections {
				if !conn.inUse && p.isConnectionHealthy(conn) {
					conn.inUse = true
					conn.lastUsed = time.Now()
					return conn, nil
				}
			}
		}
	}
}

// selectAddress selects an address using round-robin with randomization
func (p *ConnectionPool) selectAddress() string {
	if len(p.config.Addresses) == 1 {
		return p.config.Addresses[0]
	}

	// Simple load balancing: random selection
	index := rand.Intn(len(p.config.Addresses))
	return p.config.Addresses[index]
}

// cleanup runs background cleanup tasks
func (p *ConnectionPool) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.closeCh:
			return
		case <-ticker.C:
			p.cleanupIdleConnections()
		}
	}
}

// cleanupIdleConnections removes idle connections that exceed MaxIdleTime
func (p *ConnectionPool) cleanupIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	var activeConnections []*Connection
	now := time.Now()

	for _, conn := range p.connections {
		if conn.inUse {
			activeConnections = append(activeConnections, conn)
			continue
		}

		if now.Sub(conn.lastUsed) > p.config.MaxIdleTime {
			// Close idle connection
			p.closeConnection(conn)
		} else {
			activeConnections = append(activeConnections, conn)
		}
	}

	p.connections = activeConnections
}

// Stats returns connection pool statistics
func (p *ConnectionPool) Stats() *PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := &PoolStats{
		TotalConnections: len(p.connections),
		MaxConnections:   p.config.MaxConnections,
	}

	for _, conn := range p.connections {
		if conn.inUse {
			stats.ActiveConnections++
		} else {
			stats.IdleConnections++
		}

		if p.isConnectionHealthy(conn) {
			stats.HealthyConnections++
		}
	}

	return stats
}

// PoolStats represents connection pool statistics
type PoolStats struct {
	TotalConnections   int
	ActiveConnections  int
	IdleConnections    int
	HealthyConnections int
	MaxConnections     int
}

// String returns a string representation of the pool stats
func (s *PoolStats) String() string {
	return fmt.Sprintf("Pool Stats: Total=%d, Active=%d, Idle=%d, Healthy=%d, Max=%d",
		s.TotalConnections, s.ActiveConnections, s.IdleConnections,
		s.HealthyConnections, s.MaxConnections)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}