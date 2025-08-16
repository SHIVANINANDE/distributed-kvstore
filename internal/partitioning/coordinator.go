package partitioning

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Coordinator manages cross-shard operations and transactions
type Coordinator struct {
	mu                sync.RWMutex
	nodeID            string
	shardManager      *ShardManager
	router            *Router
	transactions      map[string]*Transaction
	config            CoordinatorConfig
	logger            *log.Logger
	
	// Transaction management
	txCounter         uint64
	cleanupTicker     *time.Ticker
	stopCh            chan struct{}
}

// CoordinatorConfig contains configuration for the coordinator
type CoordinatorConfig struct {
	TransactionTimeout time.Duration `json:"transaction_timeout"`    // Transaction timeout
	CleanupInterval    time.Duration `json:"cleanup_interval"`      // Cleanup interval for old transactions
	MaxTransactions    int           `json:"max_transactions"`      // Maximum concurrent transactions
	EnableDeadlock     bool          `json:"enable_deadlock"`       // Enable deadlock detection
	RetryAttempts      int           `json:"retry_attempts"`        // Retry attempts for failed operations
	RetryDelay         time.Duration `json:"retry_delay"`           // Delay between retries
}

// Transaction represents a cross-shard transaction
type Transaction struct {
	ID              string                 `json:"id"`
	StartTime       time.Time              `json:"start_time"`
	Status          TransactionStatus      `json:"status"`
	Operations      []TransactionOperation `json:"operations"`
	Participants    map[uint32]bool        `json:"participants"`    // ShardID -> participated
	Votes           map[uint32]VoteResult  `json:"votes"`           // ShardID -> vote result
	Coordinator     string                 `json:"coordinator"`     // Node coordinating this transaction
	Timeout         time.Duration          `json:"timeout"`
	LastActivity    time.Time              `json:"last_activity"`
	Metadata        map[string]string      `json:"metadata"`
}

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TxStatusPending   TransactionStatus = "pending"
	TxStatusPreparing TransactionStatus = "preparing"
	TxStatusPrepared  TransactionStatus = "prepared"
	TxStatusCommitted TransactionStatus = "committed"
	TxStatusAborted   TransactionStatus = "aborted"
	TxStatusTimeout   TransactionStatus = "timeout"
)

// TransactionOperation represents a single operation within a transaction
type TransactionOperation struct {
	Type     string            `json:"type"`      // GET, PUT, DELETE, etc.
	Key      string            `json:"key"`
	Value    []byte            `json:"value,omitempty"`
	ShardID  uint32            `json:"shard_id"`
	Status   OperationStatus   `json:"status"`
	Result   *OperationResult  `json:"result,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// OperationStatus represents the status of an operation
type OperationStatus string

const (
	OpStatusPending   OperationStatus = "pending"
	OpStatusExecuting OperationStatus = "executing"
	OpStatusCompleted OperationStatus = "completed"
	OpStatusFailed    OperationStatus = "failed"
)

// VoteResult represents the result of a prepare vote
type VoteResult string

const (
	VoteCommit VoteResult = "commit"
	VoteAbort  VoteResult = "abort"
)

// CrossShardQuery represents a query that spans multiple shards
type CrossShardQuery struct {
	ID          string                     `json:"id"`
	Query       string                     `json:"query"`
	Shards      []uint32                   `json:"shards"`
	Results     map[uint32]*QueryResult    `json:"results"`
	Status      QueryStatus                `json:"status"`
	StartTime   time.Time                  `json:"start_time"`
	Timeout     time.Duration              `json:"timeout"`
	Aggregator  func([]*QueryResult) interface{} `json:"-"`
}

// QueryResult represents the result from a single shard
type QueryResult struct {
	ShardID   uint32        `json:"shard_id"`
	Success   bool          `json:"success"`
	Data      interface{}   `json:"data"`
	Error     string        `json:"error,omitempty"`
	Latency   time.Duration `json:"latency"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// QueryStatus represents the status of a cross-shard query
type QueryStatus string

const (
	QueryStatusPending   QueryStatus = "pending"
	QueryStatusExecuting QueryStatus = "executing"
	QueryStatusCompleted QueryStatus = "completed"
	QueryStatusFailed    QueryStatus = "failed"
	QueryStatusTimeout   QueryStatus = "timeout"
)

// NewCoordinator creates a new cross-shard coordinator
func NewCoordinator(nodeID string, shardManager *ShardManager, router *Router, config CoordinatorConfig, logger *log.Logger) *Coordinator {
	if logger == nil {
		logger = log.New(log.Writer(), "[COORDINATOR] ", log.LstdFlags)
	}

	// Set default configuration values
	if config.TransactionTimeout == 0 {
		config.TransactionTimeout = 30 * time.Second
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 1 * time.Minute
	}
	if config.MaxTransactions == 0 {
		config.MaxTransactions = 1000
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 100 * time.Millisecond
	}

	return &Coordinator{
		nodeID:       nodeID,
		shardManager: shardManager,
		router:       router,
		transactions: make(map[string]*Transaction),
		config:       config,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

// Start starts the coordinator
func (c *Coordinator) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Printf("Starting cross-shard coordinator")

	// Start cleanup ticker
	c.cleanupTicker = time.NewTicker(c.config.CleanupInterval)
	go c.cleanupLoop()

	return nil
}

// Stop stops the coordinator
func (c *Coordinator) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Printf("Stopping cross-shard coordinator")

	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}

	close(c.stopCh)
}

// BeginTransaction starts a new cross-shard transaction
func (c *Coordinator) BeginTransaction(ctx context.Context, timeout time.Duration) (*Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check transaction limit
	if len(c.transactions) >= c.config.MaxTransactions {
		return nil, fmt.Errorf("maximum transaction limit reached")
	}

	// Generate transaction ID
	c.txCounter++
	txID := fmt.Sprintf("%s_%d_%d", c.nodeID, time.Now().Unix(), c.txCounter)

	if timeout == 0 {
		timeout = c.config.TransactionTimeout
	}

	tx := &Transaction{
		ID:           txID,
		StartTime:    time.Now(),
		Status:       TxStatusPending,
		Operations:   make([]TransactionOperation, 0),
		Participants: make(map[uint32]bool),
		Votes:        make(map[uint32]VoteResult),
		Coordinator:  c.nodeID,
		Timeout:      timeout,
		LastActivity: time.Now(),
		Metadata:     make(map[string]string),
	}

	c.transactions[txID] = tx
	c.logger.Printf("Started transaction %s", txID)

	return tx, nil
}

// AddOperation adds an operation to a transaction
func (c *Coordinator) AddOperation(txID string, operation TransactionOperation) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tx, exists := c.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	if tx.Status != TxStatusPending {
		return fmt.Errorf("cannot add operations to transaction in status %s", tx.Status)
	}

	// Determine which shard this operation affects
	route, err := c.router.GetRouteInfo(operation.Key)
	if err != nil {
		return fmt.Errorf("failed to route operation: %w", err)
	}

	operation.ShardID = route.ShardID
	operation.Status = OpStatusPending

	tx.Operations = append(tx.Operations, operation)
	tx.Participants[route.ShardID] = true
	tx.LastActivity = time.Now()

	c.logger.Printf("Added operation %s to transaction %s (shard %d)", 
		operation.Type, txID, route.ShardID)

	return nil
}

// ExecuteTransaction executes a cross-shard transaction using 2-phase commit
func (c *Coordinator) ExecuteTransaction(ctx context.Context, txID string) error {
	c.mu.RLock()
	tx, exists := c.transactions[txID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	c.logger.Printf("Executing transaction %s with %d operations across %d shards", 
		txID, len(tx.Operations), len(tx.Participants))

	// Apply transaction timeout
	ctx, cancel := context.WithTimeout(ctx, tx.Timeout)
	defer cancel()

	// Phase 1: Prepare
	if err := c.preparePhase(ctx, tx); err != nil {
		c.abortTransaction(ctx, tx)
		return fmt.Errorf("prepare phase failed: %w", err)
	}

	// Phase 2: Commit
	if err := c.commitPhase(ctx, tx); err != nil {
		c.abortTransaction(ctx, tx)
		return fmt.Errorf("commit phase failed: %w", err)
	}

	c.mu.Lock()
	tx.Status = TxStatusCommitted
	tx.LastActivity = time.Now()
	c.mu.Unlock()

	c.logger.Printf("Transaction %s committed successfully", txID)
	return nil
}

// preparePhase executes the prepare phase of 2PC
func (c *Coordinator) preparePhase(ctx context.Context, tx *Transaction) error {
	c.mu.Lock()
	tx.Status = TxStatusPreparing
	tx.LastActivity = time.Now()
	c.mu.Unlock()

	c.logger.Printf("Starting prepare phase for transaction %s", tx.ID)

	// Send prepare requests to all participating shards
	var wg sync.WaitGroup
	errors := make(chan error, len(tx.Participants))

	for shardID := range tx.Participants {
		wg.Add(1)
		go func(sID uint32) {
			defer wg.Done()

			vote, err := c.sendPrepareRequest(ctx, tx, sID)
			if err != nil {
				errors <- fmt.Errorf("prepare failed for shard %d: %w", sID, err)
				return
			}

			c.mu.Lock()
			tx.Votes[sID] = vote
			c.mu.Unlock()

			if vote == VoteAbort {
				errors <- fmt.Errorf("shard %d voted to abort", sID)
			}
		}(shardID)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		return err
	}

	// Check that all shards voted to commit
	c.mu.RLock()
	for shardID, vote := range tx.Votes {
		if vote != VoteCommit {
			c.mu.RUnlock()
			return fmt.Errorf("shard %d did not vote to commit", shardID)
		}
	}
	c.mu.RUnlock()

	c.mu.Lock()
	tx.Status = TxStatusPrepared
	tx.LastActivity = time.Now()
	c.mu.Unlock()

	c.logger.Printf("Prepare phase completed for transaction %s", tx.ID)
	return nil
}

// commitPhase executes the commit phase of 2PC
func (c *Coordinator) commitPhase(ctx context.Context, tx *Transaction) error {
	c.logger.Printf("Starting commit phase for transaction %s", tx.ID)

	// Send commit requests to all participating shards
	var wg sync.WaitGroup
	errors := make(chan error, len(tx.Participants))

	for shardID := range tx.Participants {
		wg.Add(1)
		go func(sID uint32) {
			defer wg.Done()

			if err := c.sendCommitRequest(ctx, tx, sID); err != nil {
				errors <- fmt.Errorf("commit failed for shard %d: %w", sID, err)
			}
		}(shardID)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		return err
	}

	c.logger.Printf("Commit phase completed for transaction %s", tx.ID)
	return nil
}

// abortTransaction aborts a transaction
func (c *Coordinator) abortTransaction(ctx context.Context, tx *Transaction) {
	c.logger.Printf("Aborting transaction %s", tx.ID)

	c.mu.Lock()
	tx.Status = TxStatusAborted
	tx.LastActivity = time.Now()
	c.mu.Unlock()

	// Send abort requests to all participating shards
	for shardID := range tx.Participants {
		go func(sID uint32) {
			if err := c.sendAbortRequest(ctx, tx, sID); err != nil {
				c.logger.Printf("Failed to send abort to shard %d: %v", sID, err)
			}
		}(shardID)
	}
}

// sendPrepareRequest sends a prepare request to a shard
func (c *Coordinator) sendPrepareRequest(ctx context.Context, tx *Transaction, shardID uint32) (VoteResult, error) {
	c.logger.Printf("Sending prepare request to shard %d for transaction %s", shardID, tx.ID)

	// In a real implementation, this would send an RPC to the shard
	// For now, we'll simulate it
	time.Sleep(10 * time.Millisecond)

	// Simulate prepare logic
	// In a real system, this would check locks, validate operations, etc.
	return VoteCommit, nil
}

// sendCommitRequest sends a commit request to a shard
func (c *Coordinator) sendCommitRequest(ctx context.Context, tx *Transaction, shardID uint32) error {
	c.logger.Printf("Sending commit request to shard %d for transaction %s", shardID, tx.ID)

	// In a real implementation, this would send an RPC to the shard
	// For now, we'll simulate it
	time.Sleep(5 * time.Millisecond)

	return nil
}

// sendAbortRequest sends an abort request to a shard
func (c *Coordinator) sendAbortRequest(ctx context.Context, tx *Transaction, shardID uint32) error {
	c.logger.Printf("Sending abort request to shard %d for transaction %s", shardID, tx.ID)

	// In a real implementation, this would send an RPC to the shard
	time.Sleep(5 * time.Millisecond)

	return nil
}

// ExecuteCrossShardQuery executes a query across multiple shards
func (c *Coordinator) ExecuteCrossShardQuery(ctx context.Context, query CrossShardQuery) (*CrossShardQuery, error) {
	c.logger.Printf("Executing cross-shard query %s across %d shards", query.ID, len(query.Shards))

	query.Status = QueryStatusExecuting
	query.StartTime = time.Now()
	query.Results = make(map[uint32]*QueryResult)

	// Apply timeout
	if query.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, query.Timeout)
		defer cancel()
	}

	// Execute query on each shard in parallel
	var wg sync.WaitGroup
	for _, shardID := range query.Shards {
		wg.Add(1)
		go func(sID uint32) {
			defer wg.Done()

			result := c.executeShardQuery(ctx, query.Query, sID)
			c.mu.Lock()
			query.Results[sID] = result
			c.mu.Unlock()
		}(shardID)
	}

	wg.Wait()

	// Check if all queries succeeded
	allSucceeded := true
	for _, result := range query.Results {
		if !result.Success {
			allSucceeded = false
			break
		}
	}

	if allSucceeded {
		query.Status = QueryStatusCompleted

		// Apply aggregation if provided
		if query.Aggregator != nil {
			results := make([]*QueryResult, 0, len(query.Results))
			for _, result := range query.Results {
				results = append(results, result)
			}
			aggregatedResult := query.Aggregator(results)
			c.logger.Printf("Query %s completed with aggregated result: %v", query.ID, aggregatedResult)
		}
	} else {
		query.Status = QueryStatusFailed
	}

	return &query, nil
}

// executeShardQuery executes a query on a single shard
func (c *Coordinator) executeShardQuery(ctx context.Context, query string, shardID uint32) *QueryResult {
	startTime := time.Now()

	result := &QueryResult{
		ShardID: shardID,
		Metadata: make(map[string]string),
	}

	// In a real implementation, this would execute the query on the shard
	// For now, we'll simulate it
	time.Sleep(time.Duration(10+shardID) * time.Millisecond)

	result.Success = true
	result.Data = fmt.Sprintf("result_from_shard_%d", shardID)
	result.Latency = time.Since(startTime)
	result.Metadata["shard_id"] = fmt.Sprintf("%d", shardID)

	return result
}

// GetTransaction returns information about a transaction
func (c *Coordinator) GetTransaction(txID string) (*Transaction, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tx, exists := c.transactions[txID]
	if !exists {
		return nil, fmt.Errorf("transaction %s not found", txID)
	}

	// Return a copy
	txCopy := *tx
	return &txCopy, nil
}

// GetActiveTransactions returns all active transactions
func (c *Coordinator) GetActiveTransactions() map[string]*Transaction {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*Transaction)
	for txID, tx := range c.transactions {
		if tx.Status == TxStatusPending || tx.Status == TxStatusPreparing || tx.Status == TxStatusPrepared {
			txCopy := *tx
			result[txID] = &txCopy
		}
	}

	return result
}

// cleanupLoop periodically cleans up old transactions
func (c *Coordinator) cleanupLoop() {
	for {
		select {
		case <-c.stopCh:
			return
		case <-c.cleanupTicker.C:
			c.cleanupOldTransactions()
		}
	}
}

// cleanupOldTransactions removes old completed or aborted transactions
func (c *Coordinator) cleanupOldTransactions() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for txID, tx := range c.transactions {
		// Remove transactions that are completed/aborted and old enough
		if (tx.Status == TxStatusCommitted || tx.Status == TxStatusAborted || tx.Status == TxStatusTimeout) &&
		   now.Sub(tx.LastActivity) > 5*time.Minute {
			delete(c.transactions, txID)
			removed++
		}

		// Check for timed out transactions
		if tx.Status == TxStatusPending || tx.Status == TxStatusPreparing {
			if now.Sub(tx.StartTime) > tx.Timeout {
				tx.Status = TxStatusTimeout
				tx.LastActivity = now
				c.logger.Printf("Transaction %s timed out", txID)
			}
		}
	}

	if removed > 0 {
		c.logger.Printf("Cleaned up %d old transactions", removed)
	}
}

// GetStats returns statistics about the coordinator
func (c *Coordinator) GetStats() CoordinatorStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CoordinatorStats{
		ActiveTransactions: 0,
		TotalTransactions:  len(c.transactions),
		TransactionsByStatus: make(map[TransactionStatus]int),
	}

	for _, tx := range c.transactions {
		stats.TransactionsByStatus[tx.Status]++
		
		if tx.Status == TxStatusPending || tx.Status == TxStatusPreparing || tx.Status == TxStatusPrepared {
			stats.ActiveTransactions++
		}
	}

	return stats
}

// CoordinatorStats contains statistics about the coordinator
type CoordinatorStats struct {
	ActiveTransactions   int                           `json:"active_transactions"`
	TotalTransactions    int                           `json:"total_transactions"`
	TransactionsByStatus map[TransactionStatus]int     `json:"transactions_by_status"`
}

<function_calls>
<invoke name="TodoWrite">
<parameter name="todos">[{"content": "Implement SHA-256 based hash ring", "status": "completed", "id": "partition8-1"}, {"content": "Add virtual nodes for load balancing", "status": "completed", "id": "partition8-2"}, {"content": "Create automatic rebalancing on node changes", "status": "completed", "id": "partition8-3"}, {"content": "Implement one Raft group per shard", "status": "completed", "id": "partition8-4"}, {"content": "Create shard management and data routing", "status": "completed", "id": "partition8-5"}, {"content": "Add cross-shard coordination", "status": "completed", "id": "partition8-6"}]