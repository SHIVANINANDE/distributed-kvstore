package performance

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// MessageBatcher implements efficient message batching for network operations
type MessageBatcher struct {
	mu           sync.RWMutex
	logger       *log.Logger
	config       BatchingConfig
	batchers     map[string]*ConnectionBatcher
	metrics      *BatchingMetrics
	isRunning    bool
	stopCh       chan struct{}
}

// BatchingConfig contains configuration for message batching
type BatchingConfig struct {
	MaxBatchSize     int           `json:"max_batch_size"`     // Maximum messages per batch
	MaxBatchBytes    int           `json:"max_batch_bytes"`    // Maximum bytes per batch
	BatchTimeout     time.Duration `json:"batch_timeout"`     // Maximum time to wait for batch
	FlushInterval    time.Duration `json:"flush_interval"`    // Periodic flush interval
	BufferSize       int           `json:"buffer_size"`       // Buffer size for pending messages
	CompressionLevel int           `json:"compression_level"` // Compression level (0-9)
	EnableMetrics    bool          `json:"enable_metrics"`    // Enable detailed metrics
}

// ConnectionBatcher handles batching for a specific connection
type ConnectionBatcher struct {
	mu            sync.Mutex
	connID        string
	config        BatchingConfig
	pendingMsgs   []*BatchMessage
	pendingBytes  int
	lastFlush     time.Time
	flushCh       chan struct{}
	metrics       *ConnectionBatchMetrics
	isRunning     bool
	sender        BatchSender
}

// BatchMessage represents a message to be batched
type BatchMessage struct {
	ID        string        `json:"id"`
	Type      MessageType   `json:"type"`
	Priority  Priority      `json:"priority"`
	Data      []byte        `json:"data"`
	Timestamp time.Time     `json:"timestamp"`
	Callback  func(error)   `json:"-"` // Completion callback
	Deadline  time.Time     `json:"deadline"`
}

// MessageType defines types of messages that can be batched
type MessageType string

const (
	MessageTypeHeartbeat  MessageType = "heartbeat"
	MessageTypeAppendLog  MessageType = "append_log"
	MessageTypeVoteReq    MessageType = "vote_request"
	MessageTypeVoteResp   MessageType = "vote_response"
	MessageTypeClientReq  MessageType = "client_request"
	MessageTypeClientResp MessageType = "client_response"
	MessageTypeSnapshot   MessageType = "snapshot"
	MessageTypeControl    MessageType = "control"
)

// Priority defines message priorities for batching
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// BatchSender defines the interface for sending batched messages
type BatchSender interface {
	SendBatch(batch *MessageBatch) error
}

// MessageBatch represents a batch of messages ready for transmission
type MessageBatch struct {
	ID           string          `json:"id"`
	Messages     []*BatchMessage `json:"messages"`
	TotalBytes   int             `json:"total_bytes"`
	MessageCount int             `json:"message_count"`
	CreatedAt    time.Time       `json:"created_at"`
	Compressed   bool            `json:"compressed"`
	Checksum     uint32          `json:"checksum"`
}

// BatchingMetrics tracks batching performance
type BatchingMetrics struct {
	TotalMessages       uint64                            `json:"total_messages"`
	TotalBatches        uint64                            `json:"total_batches"`
	AverageBatchSize    float64                           `json:"average_batch_size"`
	AverageBatchBytes   float64                           `json:"average_batch_bytes"`
	CompressionRatio    float64                           `json:"compression_ratio"`
	TotalBytesOriginal  uint64                            `json:"total_bytes_original"`
	TotalBytesCompressed uint64                           `json:"total_bytes_compressed"`
	MessagesByType      map[MessageType]uint64            `json:"messages_by_type"`
	MessagesByPriority  map[Priority]uint64               `json:"messages_by_priority"`
	FlushReasons        map[string]uint64                 `json:"flush_reasons"`
	ConnectionMetrics   map[string]*ConnectionBatchMetrics `json:"connection_metrics"`
}

// ConnectionBatchMetrics tracks per-connection batching metrics
type ConnectionBatchMetrics struct {
	MessagesQueued   uint64        `json:"messages_queued"`
	BatchesSent      uint64        `json:"batches_sent"`
	BytesSent        uint64        `json:"bytes_sent"`
	AverageLatency   time.Duration `json:"average_latency"`
	LastBatchTime    time.Time     `json:"last_batch_time"`
	QueueDepth       int           `json:"queue_depth"`
	FlushTimeouts    uint64        `json:"flush_timeouts"`
	FlushSizeLimit   uint64        `json:"flush_size_limit"`
	FlushByteLimit   uint64        `json:"flush_byte_limit"`
}

// NewMessageBatcher creates a new message batcher
func NewMessageBatcher(config BatchingConfig, logger *log.Logger) *MessageBatcher {
	if logger == nil {
		logger = log.New(log.Writer(), "[BATCHER] ", log.LstdFlags)
	}
	
	// Set default values
	if config.MaxBatchSize == 0 {
		config.MaxBatchSize = 100
	}
	if config.MaxBatchBytes == 0 {
		config.MaxBatchBytes = 64 * 1024 // 64KB
	}
	if config.BatchTimeout == 0 {
		config.BatchTimeout = 10 * time.Millisecond
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 100 * time.Millisecond
	}
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	
	return &MessageBatcher{
		logger:   logger,
		config:   config,
		batchers: make(map[string]*ConnectionBatcher),
		metrics: &BatchingMetrics{
			MessagesByType:     make(map[MessageType]uint64),
			MessagesByPriority: make(map[Priority]uint64),
			FlushReasons:      make(map[string]uint64),
			ConnectionMetrics: make(map[string]*ConnectionBatchMetrics),
		},
		stopCh: make(chan struct{}),
	}
}

// Start initializes the message batching system
func (mb *MessageBatcher) Start(ctx context.Context) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	
	if mb.isRunning {
		return fmt.Errorf("message batcher already running")
	}
	
	mb.logger.Printf("Starting message batcher with config: max_batch_size=%d, max_batch_bytes=%d, timeout=%v", 
		mb.config.MaxBatchSize, mb.config.MaxBatchBytes, mb.config.BatchTimeout)
	
	mb.isRunning = true
	
	// Start periodic flush routine
	go mb.periodicFlushRoutine(ctx)
	
	return nil
}

// CreateConnectionBatcher creates a batcher for a specific connection
func (mb *MessageBatcher) CreateConnectionBatcher(connID string, sender BatchSender) *ConnectionBatcher {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	
	if batcher, exists := mb.batchers[connID]; exists {
		return batcher
	}
	
	batcher := &ConnectionBatcher{
		connID:      connID,
		config:      mb.config,
		pendingMsgs: make([]*BatchMessage, 0, mb.config.MaxBatchSize),
		lastFlush:   time.Now(),
		flushCh:     make(chan struct{}, 1),
		isRunning:   true,
		sender:      sender,
		metrics: &ConnectionBatchMetrics{
			LastBatchTime: time.Now(),
		},
	}
	
	mb.batchers[connID] = batcher
	mb.metrics.ConnectionMetrics[connID] = batcher.metrics
	
	// Start batcher routine for this connection
	go batcher.runBatcher()
	
	mb.logger.Printf("Created connection batcher for %s", connID)
	return batcher
}

// QueueMessage queues a message for batching
func (mb *MessageBatcher) QueueMessage(connID string, msg *BatchMessage) error {
	mb.mu.RLock()
	batcher, exists := mb.batchers[connID]
	mb.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("no batcher found for connection %s", connID)
	}
	
	return batcher.QueueMessage(msg)
}

// QueueMessage queues a message in the connection batcher
func (cb *ConnectionBatcher) QueueMessage(msg *BatchMessage) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if !cb.isRunning {
		return fmt.Errorf("connection batcher is not running")
	}
	
	// Check if message has expired
	if !msg.Deadline.IsZero() && time.Now().After(msg.Deadline) {
		if msg.Callback != nil {
			msg.Callback(fmt.Errorf("message expired"))
		}
		return fmt.Errorf("message expired")
	}
	
	// Add message to pending queue
	cb.pendingMsgs = append(cb.pendingMsgs, msg)
	cb.pendingBytes += len(msg.Data)
	atomic.AddUint64(&cb.metrics.MessagesQueued, 1)
	cb.metrics.QueueDepth = len(cb.pendingMsgs)
	
	// Check if we should flush immediately
	shouldFlush := cb.shouldFlush()
	
	if shouldFlush {
		// Signal flush
		select {
		case cb.flushCh <- struct{}{}:
		default:
			// Channel is full, flush is already scheduled
		}
	}
	
	return nil
}

// shouldFlush determines if the batch should be flushed
func (cb *ConnectionBatcher) shouldFlush() bool {
	// Check size limits
	if len(cb.pendingMsgs) >= cb.config.MaxBatchSize {
		return true
	}
	
	if cb.pendingBytes >= cb.config.MaxBatchBytes {
		return true
	}
	
	// Check for high priority messages
	for _, msg := range cb.pendingMsgs {
		if msg.Priority >= PriorityHigh {
			return true
		}
	}
	
	// Check timeout
	if time.Since(cb.lastFlush) >= cb.config.BatchTimeout {
		return true
	}
	
	return false
}

// runBatcher runs the batching loop for a connection
func (cb *ConnectionBatcher) runBatcher() {
	flushTimer := time.NewTimer(cb.config.BatchTimeout)
	defer flushTimer.Stop()
	
	for cb.isRunning {
		select {
		case <-cb.flushCh:
			// Immediate flush requested
			cb.flush("manual_trigger")
			if !flushTimer.Stop() {
				<-flushTimer.C
			}
			flushTimer.Reset(cb.config.BatchTimeout)
			
		case <-flushTimer.C:
			// Timeout flush
			cb.flush("timeout")
			flushTimer.Reset(cb.config.BatchTimeout)
		}
	}
}

// flush creates and sends a batch from pending messages
func (cb *ConnectionBatcher) flush(reason string) {
	cb.mu.Lock()
	
	if len(cb.pendingMsgs) == 0 {
		cb.mu.Unlock()
		return
	}
	
	// Create batch
	batch := &MessageBatch{
		ID:           fmt.Sprintf("batch_%s_%d", cb.connID, time.Now().UnixNano()),
		Messages:     make([]*BatchMessage, len(cb.pendingMsgs)),
		TotalBytes:   cb.pendingBytes,
		MessageCount: len(cb.pendingMsgs),
		CreatedAt:    time.Now(),
	}
	
	// Copy messages (shallow copy)
	copy(batch.Messages, cb.pendingMsgs)
	
	// Clear pending queue
	cb.pendingMsgs = cb.pendingMsgs[:0]
	cb.pendingBytes = 0
	cb.lastFlush = time.Now()
	
	cb.mu.Unlock()
	
	// Optimize batch
	cb.optimizeBatch(batch)
	
	// Send batch
	start := time.Now()
	err := cb.sender.SendBatch(batch)
	latency := time.Since(start)
	
	// Update metrics
	atomic.AddUint64(&cb.metrics.BatchesSent, 1)
	atomic.AddUint64(&cb.metrics.BytesSent, uint64(batch.TotalBytes))
	cb.metrics.AverageLatency = (cb.metrics.AverageLatency + latency) / 2
	cb.metrics.LastBatchTime = time.Now()
	cb.metrics.QueueDepth = len(cb.pendingMsgs)
	
	// Update flush reason metrics
	switch reason {
	case "timeout":
		atomic.AddUint64(&cb.metrics.FlushTimeouts, 1)
	case "size_limit":
		atomic.AddUint64(&cb.metrics.FlushSizeLimit, 1)
	case "byte_limit":
		atomic.AddUint64(&cb.metrics.FlushByteLimit, 1)
	}
	
	// Call callbacks
	for _, msg := range batch.Messages {
		if msg.Callback != nil {
			msg.Callback(err)
		}
	}
}

// optimizeBatch applies optimizations to the batch
func (cb *ConnectionBatcher) optimizeBatch(batch *MessageBatch) {
	// Sort messages by priority (highest first)
	cb.sortMessagesByPriority(batch.Messages)
	
	// Apply compression if configured
	if cb.config.CompressionLevel > 0 {
		cb.compressBatch(batch)
	}
	
	// Calculate checksum
	batch.Checksum = cb.calculateChecksum(batch)
}

// sortMessagesByPriority sorts messages by priority
func (cb *ConnectionBatcher) sortMessagesByPriority(messages []*BatchMessage) {
	// Simple bubble sort by priority (for demonstration)
	n := len(messages)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if messages[j].Priority < messages[j+1].Priority {
				messages[j], messages[j+1] = messages[j+1], messages[j]
			}
		}
	}
}

// compressBatch applies compression to the batch
func (cb *ConnectionBatcher) compressBatch(batch *MessageBatch) {
	// In a real implementation, this would use a compression library
	// like gzip, snappy, or lz4. For simulation, we just mark as compressed
	batch.Compressed = true
	
	// Simulate compression (assume 30% reduction)
	originalSize := batch.TotalBytes
	batch.TotalBytes = int(float64(originalSize) * 0.7)
}

// calculateChecksum calculates a checksum for the batch
func (cb *ConnectionBatcher) calculateChecksum(batch *MessageBatch) uint32 {
	// Simple checksum calculation (in practice, use CRC32 or similar)
	var checksum uint32
	for _, msg := range batch.Messages {
		for _, b := range msg.Data {
			checksum = checksum<<1 + uint32(b)
		}
	}
	return checksum
}

// periodicFlushRoutine performs periodic flushes
func (mb *MessageBatcher) periodicFlushRoutine(ctx context.Context) {
	ticker := time.NewTicker(mb.config.FlushInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mb.stopCh:
			return
		case <-ticker.C:
			mb.flushAllConnections("periodic")
		}
	}
}

// flushAllConnections flushes all connection batchers
func (mb *MessageBatcher) flushAllConnections(reason string) {
	mb.mu.RLock()
	batchers := make([]*ConnectionBatcher, 0, len(mb.batchers))
	for _, batcher := range mb.batchers {
		batchers = append(batchers, batcher)
	}
	mb.mu.RUnlock()
	
	for _, batcher := range batchers {
		select {
		case batcher.flushCh <- struct{}{}:
		default:
			// Channel is full, skip this flush
		}
	}
}

// GetMetrics returns current batching metrics
func (mb *MessageBatcher) GetMetrics() *BatchingMetrics {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	
	// Calculate aggregated metrics
	totalMessages := uint64(0)
	totalBatches := uint64(0)
	totalBytesOriginal := uint64(0)
	totalBytesCompressed := uint64(0)
	
	for _, connMetrics := range mb.metrics.ConnectionMetrics {
		totalMessages += connMetrics.MessagesQueued
		totalBatches += connMetrics.BatchesSent
		totalBytesOriginal += connMetrics.BytesSent
		// In a real implementation, track compressed bytes separately
		totalBytesCompressed += uint64(float64(connMetrics.BytesSent) * 0.7)
	}
	
	metrics := *mb.metrics
	metrics.TotalMessages = totalMessages
	metrics.TotalBatches = totalBatches
	metrics.TotalBytesOriginal = totalBytesOriginal
	metrics.TotalBytesCompressed = totalBytesCompressed
	
	if totalBatches > 0 {
		metrics.AverageBatchSize = float64(totalMessages) / float64(totalBatches)
		metrics.AverageBatchBytes = float64(totalBytesOriginal) / float64(totalBatches)
	}
	
	if totalBytesOriginal > 0 {
		metrics.CompressionRatio = float64(totalBytesCompressed) / float64(totalBytesOriginal)
	}
	
	return &metrics
}

// CreateMessage creates a new batch message with the specified parameters
func CreateMessage(msgType MessageType, priority Priority, data []byte, deadline time.Time, callback func(error)) *BatchMessage {
	return &BatchMessage{
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Type:      msgType,
		Priority:  priority,
		Data:      data,
		Timestamp: time.Now(),
		Callback:  callback,
		Deadline:  deadline,
	}
}

// CreateHeartbeatMessage creates a heartbeat message
func CreateHeartbeatMessage(data []byte) *BatchMessage {
	return CreateMessage(MessageTypeHeartbeat, PriorityLow, data, time.Time{}, nil)
}

// CreateLogMessage creates a log append message
func CreateLogMessage(data []byte, callback func(error)) *BatchMessage {
	deadline := time.Now().Add(5 * time.Second) // 5 second deadline
	return CreateMessage(MessageTypeAppendLog, PriorityNormal, data, deadline, callback)
}

// CreateVoteMessage creates a vote request/response message
func CreateVoteMessage(msgType MessageType, data []byte, callback func(error)) *BatchMessage {
	deadline := time.Now().Add(1 * time.Second) // 1 second deadline for votes
	return CreateMessage(msgType, PriorityHigh, data, deadline, callback)
}

// CreateClientMessage creates a client request/response message
func CreateClientMessage(msgType MessageType, data []byte, callback func(error)) *BatchMessage {
	deadline := time.Now().Add(10 * time.Second) // 10 second deadline
	return CreateMessage(msgType, PriorityNormal, data, deadline, callback)
}

// CreateCriticalMessage creates a critical control message
func CreateCriticalMessage(data []byte, callback func(error)) *BatchMessage {
	deadline := time.Now().Add(500 * time.Millisecond) // 500ms deadline
	return CreateMessage(MessageTypeControl, PriorityCritical, data, deadline, callback)
}

// Stop gracefully stops the message batcher
func (mb *MessageBatcher) Stop() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	
	if !mb.isRunning {
		return nil
	}
	
	mb.logger.Printf("Stopping message batcher")
	
	// Signal stop
	close(mb.stopCh)
	
	// Flush all pending messages
	mb.flushAllConnections("shutdown")
	
	// Stop all connection batchers
	for _, batcher := range mb.batchers {
		batcher.isRunning = false
	}
	
	mb.isRunning = false
	return nil
}

// GetConnectionStats returns statistics for all connections
func (mb *MessageBatcher) GetConnectionStats() map[string]*ConnectionBatchMetrics {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	
	stats := make(map[string]*ConnectionBatchMetrics)
	for connID, metrics := range mb.metrics.ConnectionMetrics {
		// Return a copy
		statsCopy := *metrics
		stats[connID] = &statsCopy
	}
	
	return stats
}

// EstimateBatchEfficiency estimates the efficiency gain from batching
func (mb *MessageBatcher) EstimateBatchEfficiency() *BatchEfficiencyStats {
	metrics := mb.GetMetrics()
	
	// Calculate efficiency metrics
	efficiency := &BatchEfficiencyStats{
		TotalMessages:      metrics.TotalMessages,
		TotalBatches:       metrics.TotalBatches,
		AverageBatchSize:   metrics.AverageBatchSize,
		CompressionRatio:   metrics.CompressionRatio,
		NetworkCallsSaved:  0,
		BandwidthSaved:     0,
		EstimatedSpeedup:   1.0,
	}
	
	// Calculate network calls saved
	if metrics.TotalMessages > metrics.TotalBatches {
		efficiency.NetworkCallsSaved = metrics.TotalMessages - metrics.TotalBatches
	}
	
	// Calculate bandwidth saved from compression
	if metrics.TotalBytesOriginal > metrics.TotalBytesCompressed {
		efficiency.BandwidthSaved = metrics.TotalBytesOriginal - metrics.TotalBytesCompressed
	}
	
	// Estimate speedup (simplified calculation)
	if metrics.AverageBatchSize > 1 {
		efficiency.EstimatedSpeedup = float64(metrics.AverageBatchSize) * 0.8 // Assume 80% efficiency
	}
	
	return efficiency
}

// BatchEfficiencyStats contains efficiency statistics
type BatchEfficiencyStats struct {
	TotalMessages     uint64  `json:"total_messages"`
	TotalBatches      uint64  `json:"total_batches"`
	AverageBatchSize  float64 `json:"average_batch_size"`
	CompressionRatio  float64 `json:"compression_ratio"`
	NetworkCallsSaved uint64  `json:"network_calls_saved"`
	BandwidthSaved    uint64  `json:"bandwidth_saved"`
	EstimatedSpeedup  float64 `json:"estimated_speedup"`
}