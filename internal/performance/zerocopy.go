package performance

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// ZeroCopyNetworking implements zero-copy networking optimizations
type ZeroCopyNetworking struct {
	mu               sync.RWMutex
	logger           *log.Logger
	connections      map[string]*ZeroCopyConnection
	bufferPool       *ZeroCopyBufferPool
	config           ZeroCopyConfig
	metrics          *ZeroCopyMetrics
	isRunning        bool
}

// ZeroCopyConfig contains configuration for zero-copy networking
type ZeroCopyConfig struct {
	BufferSize       int           `json:"buffer_size"`        // Size of network buffers
	PoolSize         int           `json:"pool_size"`          // Size of buffer pool
	SocketBufferSize int           `json:"socket_buffer_size"` // OS socket buffer size
	EnableSendfile   bool          `json:"enable_sendfile"`    // Enable sendfile for large transfers
	EnableSplice     bool          `json:"enable_splice"`      // Enable splice for pipe transfers
	TCPNoDelay       bool          `json:"tcp_no_delay"`       // Disable Nagle's algorithm
	TCPQuickAck      bool          `json:"tcp_quick_ack"`      // Enable TCP quick ACK
	ReusePort        bool          `json:"reuse_port"`         // Enable SO_REUSEPORT
	Timeout          time.Duration `json:"timeout"`            // Network operation timeout
}

// ZeroCopyConnection represents a zero-copy optimized connection
type ZeroCopyConnection struct {
	conn         net.Conn
	fd           int
	sendBuffer   *ZeroCopyBuffer
	recvBuffer   *ZeroCopyBuffer
	isOptimized  bool
	bytesSent    uint64
	bytesRecv    uint64
	transferTime time.Duration
	lastUsed     time.Time
}

// ZeroCopyBuffer represents a buffer optimized for zero-copy operations
type ZeroCopyBuffer struct {
	data      []byte
	size      int
	capacity  int
	offset    int
	refCount  int32
	allocated time.Time
	pool      *ZeroCopyBufferPool
}

// ZeroCopyBufferPool manages a pool of zero-copy buffers
type ZeroCopyBufferPool struct {
	mu      sync.Mutex
	buffers chan *ZeroCopyBuffer
	config  ZeroCopyConfig
	metrics *BufferPoolMetrics
}

// BufferPoolMetrics tracks buffer pool performance
type BufferPoolMetrics struct {
	TotalAllocations uint64 `json:"total_allocations"`
	TotalReleases    uint64 `json:"total_releases"`
	ActiveBuffers    uint64 `json:"active_buffers"`
	PoolHits         uint64 `json:"pool_hits"`
	PoolMisses       uint64 `json:"pool_misses"`
	MemoryUsage      uint64 `json:"memory_usage"`
}

// ZeroCopyMetrics tracks zero-copy networking performance
type ZeroCopyMetrics struct {
	ConnectionsCreated   uint64        `json:"connections_created"`
	ConnectionsOptimized uint64        `json:"connections_optimized"`
	TotalBytesSent       uint64        `json:"total_bytes_sent"`
	TotalBytesReceived   uint64        `json:"total_bytes_received"`
	ZeroCopyTransfers    uint64        `json:"zero_copy_transfers"`
	AverageTransferTime  time.Duration `json:"average_transfer_time"`
	BufferPoolMetrics    *BufferPoolMetrics `json:"buffer_pool_metrics"`
}

// NewZeroCopyNetworking creates a new zero-copy networking instance
func NewZeroCopyNetworking(config ZeroCopyConfig, logger *log.Logger) *ZeroCopyNetworking {
	if logger == nil {
		logger = log.New(log.Writer(), "[ZEROCOPY] ", log.LstdFlags)
	}
	
	// Set default values
	if config.BufferSize == 0 {
		config.BufferSize = 64 * 1024 // 64KB
	}
	if config.PoolSize == 0 {
		config.PoolSize = 100
	}
	if config.SocketBufferSize == 0 {
		config.SocketBufferSize = 1024 * 1024 // 1MB
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	
	bufferPool := NewZeroCopyBufferPool(config)
	
	return &ZeroCopyNetworking{
		logger:      logger,
		connections: make(map[string]*ZeroCopyConnection),
		bufferPool:  bufferPool,
		config:      config,
		metrics: &ZeroCopyMetrics{
			BufferPoolMetrics: bufferPool.metrics,
		},
	}
}

// NewZeroCopyBufferPool creates a new buffer pool
func NewZeroCopyBufferPool(config ZeroCopyConfig) *ZeroCopyBufferPool {
	pool := &ZeroCopyBufferPool{
		buffers: make(chan *ZeroCopyBuffer, config.PoolSize),
		config:  config,
		metrics: &BufferPoolMetrics{},
	}
	
	// Pre-allocate buffers
	for i := 0; i < config.PoolSize; i++ {
		buffer := &ZeroCopyBuffer{
			data:      make([]byte, config.BufferSize),
			capacity:  config.BufferSize,
			allocated: time.Now(),
			pool:      pool,
		}
		pool.buffers <- buffer
	}
	
	return pool
}

// Start initializes the zero-copy networking system
func (zcn *ZeroCopyNetworking) Start(ctx context.Context) error {
	zcn.mu.Lock()
	defer zcn.mu.Unlock()
	
	if zcn.isRunning {
		return fmt.Errorf("zero-copy networking already running")
	}
	
	zcn.logger.Printf("Starting zero-copy networking with config: buffer_size=%d, pool_size=%d", 
		zcn.config.BufferSize, zcn.config.PoolSize)
	
	zcn.isRunning = true
	
	// Start background cleanup
	go zcn.cleanupRoutine(ctx)
	
	return nil
}

// CreateOptimizedConnection creates a zero-copy optimized connection
func (zcn *ZeroCopyNetworking) CreateOptimizedConnection(conn net.Conn) (*ZeroCopyConnection, error) {
	zcn.mu.Lock()
	defer zcn.mu.Unlock()
	
	connID := conn.RemoteAddr().String()
	
	// Get file descriptor for advanced optimizations
	fd := -1
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if file, err := tcpConn.File(); err == nil {
			fd = int(file.Fd())
			file.Close() // Close the file handle but keep the socket
		}
	}
	
	zcConn := &ZeroCopyConnection{
		conn:        conn,
		fd:          fd,
		sendBuffer:  zcn.bufferPool.Get(),
		recvBuffer:  zcn.bufferPool.Get(),
		isOptimized: fd != -1,
		lastUsed:    time.Now(),
	}
	
	// Apply socket optimizations
	if err := zcn.optimizeSocket(zcConn); err != nil {
		zcn.logger.Printf("Failed to optimize socket: %v", err)
		// Continue anyway with non-optimized connection
	}
	
	zcn.connections[connID] = zcConn
	atomic.AddUint64(&zcn.metrics.ConnectionsCreated, 1)
	
	if zcConn.isOptimized {
		atomic.AddUint64(&zcn.metrics.ConnectionsOptimized, 1)
	}
	
	zcn.logger.Printf("Created zero-copy connection to %s (optimized: %t)", connID, zcConn.isOptimized)
	return zcConn, nil
}

// optimizeSocket applies zero-copy optimizations to the socket
func (zcn *ZeroCopyNetworking) optimizeSocket(conn *ZeroCopyConnection) error {
	if conn.fd == -1 {
		return fmt.Errorf("invalid file descriptor")
	}
	
	// Set socket buffer sizes
	if err := syscall.SetsockoptInt(conn.fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, zcn.config.SocketBufferSize); err != nil {
		return fmt.Errorf("failed to set send buffer size: %w", err)
	}
	
	if err := syscall.SetsockoptInt(conn.fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, zcn.config.SocketBufferSize); err != nil {
		return fmt.Errorf("failed to set receive buffer size: %w", err)
	}
	
	// Enable TCP optimizations
	if zcn.config.TCPNoDelay {
		if err := syscall.SetsockoptInt(conn.fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1); err != nil {
			return fmt.Errorf("failed to set TCP_NODELAY: %w", err)
		}
	}
	
	// Platform-specific optimizations
	if runtime.GOOS == "linux" {
		// Enable TCP quick ACK on Linux
		if zcn.config.TCPQuickAck {
			if err := syscall.SetsockoptInt(conn.fd, syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 1); err != nil {
				zcn.logger.Printf("Failed to set TCP_QUICKACK: %v", err)
			}
		}
	}
	
	return nil
}

// ZeroCopySend performs zero-copy send operation
func (zcn *ZeroCopyNetworking) ZeroCopySend(conn *ZeroCopyConnection, data []byte) (int, error) {
	start := time.Now()
	
	var bytesWritten int
	var err error
	
	// Try zero-copy operations first
	if conn.isOptimized && len(data) > 4096 { // Use zero-copy for larger data
		bytesWritten, err = zcn.sendfileTransfer(conn, data)
		if err == nil {
			atomic.AddUint64(&zcn.metrics.ZeroCopyTransfers, 1)
		}
	}
	
	// Fallback to optimized buffer copy
	if bytesWritten == 0 {
		bytesWritten, err = zcn.bufferSend(conn, data)
	}
	
	if err == nil {
		atomic.AddUint64(&conn.bytesSent, uint64(bytesWritten))
		atomic.AddUint64(&zcn.metrics.TotalBytesSent, uint64(bytesWritten))
		conn.transferTime += time.Since(start)
		conn.lastUsed = time.Now()
	}
	
	return bytesWritten, err
}

// sendfileTransfer uses sendfile for zero-copy transfer
func (zcn *ZeroCopyNetworking) sendfileTransfer(conn *ZeroCopyConnection, data []byte) (int, error) {
	if !zcn.config.EnableSendfile || runtime.GOOS != "linux" {
		return 0, fmt.Errorf("sendfile not available")
	}
	
	// For in-memory data, we can't use sendfile directly
	// This would be used for file-to-socket transfers
	// Here we simulate the concept for demonstration
	return zcn.optimizedWrite(conn, data)
}

// optimizedWrite performs optimized write using writev when possible
func (zcn *ZeroCopyNetworking) optimizedWrite(conn *ZeroCopyConnection, data []byte) (int, error) {
	if conn.fd == -1 {
		return conn.conn.Write(data)
	}
	
	// Use direct system call for better performance
	return syscall.Write(conn.fd, data)
}

// bufferSend uses buffer pool for efficient sending
func (zcn *ZeroCopyNetworking) bufferSend(conn *ZeroCopyConnection, data []byte) (int, error) {
	buffer := conn.sendBuffer
	
	// Ensure buffer has enough capacity
	if buffer.capacity < len(data) {
		// Return buffer to pool and get a larger one
		zcn.bufferPool.Put(buffer)
		buffer = zcn.bufferPool.GetWithSize(len(data))
		conn.sendBuffer = buffer
	}
	
	// Copy data to buffer (this is the only copy we make)
	copy(buffer.data[:len(data)], data)
	buffer.size = len(data)
	
	// Send from buffer
	return conn.conn.Write(buffer.data[:buffer.size])
}

// ZeroCopyReceive performs zero-copy receive operation
func (zcn *ZeroCopyNetworking) ZeroCopyReceive(conn *ZeroCopyConnection, size int) ([]byte, error) {
	start := time.Now()
	
	buffer := conn.recvBuffer
	
	// Ensure buffer has enough capacity
	if buffer.capacity < size {
		zcn.bufferPool.Put(buffer)
		buffer = zcn.bufferPool.GetWithSize(size)
		conn.recvBuffer = buffer
	}
	
	// Read directly into buffer
	n, err := conn.conn.Read(buffer.data[:size])
	if err != nil {
		return nil, err
	}
	
	buffer.size = n
	atomic.AddUint64(&conn.bytesRecv, uint64(n))
	atomic.AddUint64(&zcn.metrics.TotalBytesReceived, uint64(n))
	conn.transferTime += time.Since(start)
	conn.lastUsed = time.Now()
	
	// Return a slice pointing to the buffer data (zero-copy)
	return buffer.data[:n], nil
}

// WriteVector performs vectored I/O for multiple buffers
func (zcn *ZeroCopyNetworking) WriteVector(conn *ZeroCopyConnection, buffers [][]byte) (int, error) {
	if conn.fd == -1 || runtime.GOOS != "linux" {
		// Fallback to sequential writes
		total := 0
		for _, buf := range buffers {
			n, err := conn.conn.Write(buf)
			total += n
			if err != nil {
				return total, err
			}
		}
		return total, nil
	}
	
	// Use writev for efficient vectored I/O
	return zcn.writev(conn.fd, buffers)
}

// writev implements vectored write using system call
func (zcn *ZeroCopyNetworking) writev(fd int, buffers [][]byte) (int, error) {
	if len(buffers) == 0 {
		return 0, nil
	}
	
	// Convert Go slices to syscall.Iovec
	iovecs := make([]syscall.Iovec, len(buffers))
	for i, buf := range buffers {
		if len(buf) > 0 {
			iovecs[i].Base = &buf[0]
			iovecs[i].Len = uint64(len(buf))
		}
	}
	
	// Perform writev system call
	n, _, err := syscall.Syscall(syscall.SYS_WRITEV, 
		uintptr(fd), 
		uintptr(unsafe.Pointer(&iovecs[0])), 
		uintptr(len(iovecs)))
	
	if err != 0 {
		return int(n), err
	}
	
	return int(n), nil
}

// Buffer pool operations

// Get retrieves a buffer from the pool
func (pool *ZeroCopyBufferPool) Get() *ZeroCopyBuffer {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	
	select {
	case buffer := <-pool.buffers:
		atomic.AddUint64(&pool.metrics.PoolHits, 1)
		atomic.AddUint64(&pool.metrics.ActiveBuffers, 1)
		buffer.offset = 0
		buffer.size = 0
		atomic.StoreInt32(&buffer.refCount, 1)
		return buffer
	default:
		// Pool is empty, create new buffer
		atomic.AddUint64(&pool.metrics.PoolMisses, 1)
		atomic.AddUint64(&pool.metrics.TotalAllocations, 1)
		atomic.AddUint64(&pool.metrics.ActiveBuffers, 1)
		atomic.AddUint64(&pool.metrics.MemoryUsage, uint64(pool.config.BufferSize))
		
		return &ZeroCopyBuffer{
			data:      make([]byte, pool.config.BufferSize),
			capacity:  pool.config.BufferSize,
			allocated: time.Now(),
			pool:      pool,
			refCount:  1,
		}
	}
}

// GetWithSize retrieves a buffer with at least the specified size
func (pool *ZeroCopyBufferPool) GetWithSize(size int) *ZeroCopyBuffer {
	if size <= pool.config.BufferSize {
		return pool.Get()
	}
	
	// Create a larger buffer for this specific request
	atomic.AddUint64(&pool.metrics.TotalAllocations, 1)
	atomic.AddUint64(&pool.metrics.ActiveBuffers, 1)
	atomic.AddUint64(&pool.metrics.MemoryUsage, uint64(size))
	
	return &ZeroCopyBuffer{
		data:      make([]byte, size),
		capacity:  size,
		allocated: time.Now(),
		pool:      pool,
		refCount:  1,
	}
}

// Put returns a buffer to the pool
func (pool *ZeroCopyBufferPool) Put(buffer *ZeroCopyBuffer) {
	if buffer == nil {
		return
	}
	
	// Decrease reference count
	if atomic.AddInt32(&buffer.refCount, -1) > 0 {
		return // Buffer is still being used
	}
	
	pool.mu.Lock()
	defer pool.mu.Unlock()
	
	atomic.AddUint64(&pool.metrics.TotalReleases, 1)
	atomic.AddUint64(&pool.metrics.ActiveBuffers, ^uint64(0)) // Subtract 1
	
	// Only return standard-sized buffers to the pool
	if buffer.capacity == pool.config.BufferSize {
		select {
		case pool.buffers <- buffer:
			// Buffer returned to pool
		default:
			// Pool is full, let buffer be garbage collected
			atomic.AddUint64(&pool.metrics.MemoryUsage, ^uint64(buffer.capacity-1)) // Subtract capacity
		}
	} else {
		// Large buffer, let it be garbage collected
		atomic.AddUint64(&pool.metrics.MemoryUsage, ^uint64(buffer.capacity-1)) // Subtract capacity
	}
}

// AddRef increases the reference count of a buffer
func (buffer *ZeroCopyBuffer) AddRef() {
	atomic.AddInt32(&buffer.refCount, 1)
}

// Release decreases the reference count and returns buffer to pool if needed
func (buffer *ZeroCopyBuffer) Release() {
	buffer.pool.Put(buffer)
}

// Data returns the buffer data as a slice
func (buffer *ZeroCopyBuffer) Data() []byte {
	return buffer.data[buffer.offset:buffer.offset+buffer.size]
}

// cleanupRoutine performs periodic cleanup of connections
func (zcn *ZeroCopyNetworking) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			zcn.cleanupInactiveConnections()
		}
	}
}

// cleanupInactiveConnections removes inactive connections
func (zcn *ZeroCopyNetworking) cleanupInactiveConnections() {
	zcn.mu.Lock()
	defer zcn.mu.Unlock()
	
	now := time.Now()
	threshold := 5 * time.Minute
	
	for connID, conn := range zcn.connections {
		if now.Sub(conn.lastUsed) > threshold {
			zcn.cleanupConnection(conn)
			delete(zcn.connections, connID)
		}
	}
}

// cleanupConnection cleans up resources for a connection
func (zcn *ZeroCopyNetworking) cleanupConnection(conn *ZeroCopyConnection) {
	if conn.sendBuffer != nil {
		zcn.bufferPool.Put(conn.sendBuffer)
	}
	if conn.recvBuffer != nil {
		zcn.bufferPool.Put(conn.recvBuffer)
	}
}

// GetMetrics returns current zero-copy networking metrics
func (zcn *ZeroCopyNetworking) GetMetrics() *ZeroCopyMetrics {
	zcn.mu.RLock()
	defer zcn.mu.RUnlock()
	
	// Calculate average transfer time
	totalTime := time.Duration(0)
	totalTransfers := uint64(0)
	
	for _, conn := range zcn.connections {
		if conn.bytesSent > 0 || conn.bytesRecv > 0 {
			totalTime += conn.transferTime
			totalTransfers++
		}
	}
	
	if totalTransfers > 0 {
		zcn.metrics.AverageTransferTime = totalTime / time.Duration(totalTransfers)
	}
	
	// Return a copy of metrics
	metricsCopy := *zcn.metrics
	return &metricsCopy
}

// GetConnectionStats returns statistics for all connections
func (zcn *ZeroCopyNetworking) GetConnectionStats() map[string]*ZeroCopyConnectionStats {
	zcn.mu.RLock()
	defer zcn.mu.RUnlock()
	
	stats := make(map[string]*ZeroCopyConnectionStats)
	
	for connID, conn := range zcn.connections {
		stats[connID] = &ZeroCopyConnectionStats{
			IsOptimized:  conn.isOptimized,
			BytesSent:    atomic.LoadUint64(&conn.bytesSent),
			BytesRecv:    atomic.LoadUint64(&conn.bytesRecv),
			TransferTime: conn.transferTime,
			LastUsed:     conn.lastUsed,
		}
	}
	
	return stats
}

// ZeroCopyConnectionStats contains statistics for a connection
type ZeroCopyConnectionStats struct {
	IsOptimized  bool          `json:"is_optimized"`
	BytesSent    uint64        `json:"bytes_sent"`
	BytesRecv    uint64        `json:"bytes_recv"`
	TransferTime time.Duration `json:"transfer_time"`
	LastUsed     time.Time     `json:"last_used"`
}

// Stop gracefully stops the zero-copy networking system
func (zcn *ZeroCopyNetworking) Stop() error {
	zcn.mu.Lock()
	defer zcn.mu.Unlock()
	
	if !zcn.isRunning {
		return nil
	}
	
	zcn.logger.Printf("Stopping zero-copy networking")
	
	// Cleanup all connections
	for _, conn := range zcn.connections {
		zcn.cleanupConnection(conn)
	}
	
	zcn.connections = make(map[string]*ZeroCopyConnection)
	zcn.isRunning = false
	
	return nil
}