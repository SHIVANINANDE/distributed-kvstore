package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"distributed-kvstore/proto/kvstore"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client represents a distributed key-value store client
type Client struct {
	config  *Config
	pool    *ConnectionPool
	mu      sync.RWMutex
	closed  bool
}

// Config holds client configuration
type Config struct {
	// Server addresses
	Addresses []string
	
	// Connection settings
	MaxConnections    int
	MaxIdleTime       time.Duration
	ConnectionTimeout time.Duration
	RequestTimeout    time.Duration
	
	// Retry settings
	MaxRetries     int
	RetryDelay     time.Duration
	RetryBackoff   float64
	RetryableErrors []codes.Code
	
	// TLS settings
	TLSEnabled bool
	CertFile   string
	KeyFile    string
	CAFile     string
}

// DefaultConfig returns a default client configuration
func DefaultConfig() *Config {
	return &Config{
		Addresses:         []string{"localhost:9090"},
		MaxConnections:    10,
		MaxIdleTime:       5 * time.Minute,
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		MaxRetries:        3,
		RetryDelay:        100 * time.Millisecond,
		RetryBackoff:      2.0,
		RetryableErrors: []codes.Code{
			codes.Unavailable,
			codes.DeadlineExceeded,
			codes.ResourceExhausted,
		},
		TLSEnabled: false,
	}
}

// NewClient creates a new distributed key-value store client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}
	
	if len(config.Addresses) == 0 {
		return nil, fmt.Errorf("at least one server address must be provided")
	}
	
	pool, err := NewConnectionPool(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}
	
	return &Client{
		config: config,
		pool:   pool,
	}, nil
}

// Close closes the client and all its connections
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return nil
	}
	
	c.closed = true
	return c.pool.Close()
}

// Put stores a key-value pair
func (c *Client) Put(ctx context.Context, key string, value []byte, opts ...PutOption) error {
	req := &kvstore.PutRequest{
		Key:   key,
		Value: value,
	}
	
	for _, opt := range opts {
		opt(req)
	}
	
	return c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.Put(ctx, req)
		if err != nil {
			return err
		}
		if !resp.Success {
			return fmt.Errorf("put failed: %s", resp.Error)
		}
		return nil
	})
}

// Get retrieves a value by key
func (c *Client) Get(ctx context.Context, key string) (*GetResult, error) {
	req := &kvstore.GetRequest{Key: key}
	
	var result *GetResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.Get(ctx, req)
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("get failed: %s", resp.Error)
		}
		
		result = &GetResult{
			Found:     resp.Found,
			Value:     resp.Value,
			CreatedAt: time.Unix(resp.CreatedAt, 0),
			ExpiresAt: time.Unix(resp.ExpiresAt, 0),
		}
		return nil
	})
	
	return result, err
}

// Delete removes a key-value pair
func (c *Client) Delete(ctx context.Context, key string) (*DeleteResult, error) {
	req := &kvstore.DeleteRequest{Key: key}
	
	var result *DeleteResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.Delete(ctx, req)
		if err != nil {
			return err
		}
		if !resp.Success {
			return fmt.Errorf("delete failed: %s", resp.Error)
		}
		
		result = &DeleteResult{
			Existed: resp.Existed,
		}
		return nil
	})
	
	return result, err
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	req := &kvstore.ExistsRequest{Key: key}
	
	var exists bool
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.Exists(ctx, req)
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("exists failed: %s", resp.Error)
		}
		
		exists = resp.Exists
		return nil
	})
	
	return exists, err
}

// List retrieves key-value pairs with a prefix
func (c *Client) List(ctx context.Context, prefix string, opts ...ListOption) (*ListResult, error) {
	req := &kvstore.ListRequest{Prefix: prefix}
	
	for _, opt := range opts {
		opt(req)
	}
	
	var result *ListResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.List(ctx, req)
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("list failed: %s", resp.Error)
		}
		
		items := make([]*KeyValue, len(resp.Items))
		for i, item := range resp.Items {
			items[i] = &KeyValue{
				Key:       item.Key,
				Value:     item.Value,
				CreatedAt: time.Unix(item.CreatedAt, 0),
				ExpiresAt: time.Unix(item.ExpiresAt, 0),
			}
		}
		
		result = &ListResult{
			Items:      items,
			NextCursor: resp.NextCursor,
			HasMore:    resp.HasMore,
		}
		return nil
	})
	
	return result, err
}

// ListKeys retrieves keys with a prefix
func (c *Client) ListKeys(ctx context.Context, prefix string, opts ...ListOption) (*ListKeysResult, error) {
	req := &kvstore.ListKeysRequest{Prefix: prefix}
	
	for _, opt := range opts {
		if opt != nil {
			// Convert ListOption to appropriate fields for ListKeysRequest
			tempReq := &kvstore.ListRequest{}
			opt(tempReq)
			req.Limit = tempReq.Limit
			req.Cursor = tempReq.Cursor
		}
	}
	
	var result *ListKeysResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.ListKeys(ctx, req)
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("list keys failed: %s", resp.Error)
		}
		
		result = &ListKeysResult{
			Keys:       resp.Keys,
			NextCursor: resp.NextCursor,
			HasMore:    resp.HasMore,
		}
		return nil
	})
	
	return result, err
}

// BatchPut stores multiple key-value pairs
func (c *Client) BatchPut(ctx context.Context, items []*PutItem) (*BatchResult, error) {
	reqItems := make([]*kvstore.PutItem, len(items))
	for i, item := range items {
		reqItems[i] = &kvstore.PutItem{
			Key:        item.Key,
			Value:      item.Value,
			TtlSeconds: item.TTLSeconds,
		}
	}
	
	req := &kvstore.BatchPutRequest{Items: reqItems}
	
	var result *BatchResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.BatchPut(ctx, req)
		if err != nil {
			return err
		}
		
		errors := make([]*BatchError, len(resp.Errors))
		for i, batchErr := range resp.Errors {
			errors[i] = &BatchError{
				Key:   batchErr.Key,
				Error: batchErr.Error,
			}
		}
		
		result = &BatchResult{
			SuccessCount: int(resp.SuccessCount),
			ErrorCount:   int(resp.ErrorCount),
			Errors:       errors,
		}
		return nil
	})
	
	return result, err
}

// BatchGet retrieves multiple values
func (c *Client) BatchGet(ctx context.Context, keys []string) ([]*GetResult, error) {
	req := &kvstore.BatchGetRequest{Keys: keys}
	
	var results []*GetResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.BatchGet(ctx, req)
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("batch get failed: %s", resp.Error)
		}
		
		results = make([]*GetResult, len(resp.Results))
		for i, result := range resp.Results {
			results[i] = &GetResult{
				Key:       result.Key,
				Found:     result.Found,
				Value:     result.Value,
				CreatedAt: time.Unix(result.CreatedAt, 0),
				ExpiresAt: time.Unix(result.ExpiresAt, 0),
			}
		}
		return nil
	})
	
	return results, err
}

// BatchDelete removes multiple keys
func (c *Client) BatchDelete(ctx context.Context, keys []string) (*BatchResult, error) {
	req := &kvstore.BatchDeleteRequest{Keys: keys}
	
	var result *BatchResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.BatchDelete(ctx, req)
		if err != nil {
			return err
		}
		
		errors := make([]*BatchError, len(resp.Errors))
		for i, batchErr := range resp.Errors {
			errors[i] = &BatchError{
				Key:   batchErr.Key,
				Error: batchErr.Error,
			}
		}
		
		result = &BatchResult{
			SuccessCount: int(resp.SuccessCount),
			ErrorCount:   int(resp.ErrorCount),
			Errors:       errors,
		}
		return nil
	})
	
	return result, err
}

// Health checks server health
func (c *Client) Health(ctx context.Context) (*HealthResult, error) {
	req := &kvstore.HealthRequest{}
	
	var result *HealthResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.Health(ctx, req)
		if err != nil {
			return err
		}
		
		result = &HealthResult{
			Healthy:       resp.Healthy,
			Status:        resp.Status,
			UptimeSeconds: int(resp.UptimeSeconds),
			Version:       resp.Version,
		}
		return nil
	})
	
	return result, err
}

// Stats retrieves server statistics
func (c *Client) Stats(ctx context.Context, includeDetails bool) (*StatsResult, error) {
	req := &kvstore.StatsRequest{IncludeDetails: includeDetails}
	
	var result *StatsResult
	err := c.executeWithRetry(ctx, func(client kvstore.KVStoreClient) error {
		resp, err := client.Stats(ctx, req)
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("stats failed: %s", resp.Error)
		}
		
		result = &StatsResult{
			TotalKeys: int(resp.TotalKeys),
			TotalSize: int(resp.TotalSize),
			LSMSize:   int(resp.LsmSize),
			VLogSize:  int(resp.VlogSize),
			Details:   resp.Details,
		}
		return nil
	})
	
	return result, err
}

// executeWithRetry executes a function with retry logic
func (c *Client) executeWithRetry(ctx context.Context, fn func(kvstore.KVStoreClient) error) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()
	
	var lastErr error
	
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoffMultiplier := 1 << (attempt - 1)
			delay := time.Duration(float64(c.config.RetryDelay) * float64(backoffMultiplier))
			if c.config.RetryBackoff > 1 {
				delay = time.Duration(float64(delay) * c.config.RetryBackoff)
			}
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		
		conn, err := c.pool.GetConnection(ctx)
		if err != nil {
			lastErr = fmt.Errorf("failed to get connection: %w", err)
			if c.shouldRetry(err) && attempt < c.config.MaxRetries {
				continue
			}
			return lastErr
		}
		
		client := kvstore.NewKVStoreClient(conn.conn)
		err = fn(client)
		
		c.pool.PutConnection(conn)
		
		if err == nil {
			return nil
		}
		
		lastErr = err
		if !c.shouldRetry(err) || attempt >= c.config.MaxRetries {
			break
		}
	}
	
	return lastErr
}

// shouldRetry determines if an error should trigger a retry
func (c *Client) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	
	for _, code := range c.config.RetryableErrors {
		if st.Code() == code {
			return true
		}
	}
	
	return false
}