package client

import (
	"time"

	"distributed-kvstore/proto/kvstore"
)

// GetResult represents the result of a Get operation
type GetResult struct {
	Key       string
	Found     bool
	Value     []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

// DeleteResult represents the result of a Delete operation
type DeleteResult struct {
	Existed bool
}

// KeyValue represents a key-value pair with metadata
type KeyValue struct {
	Key       string
	Value     []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ListResult represents the result of a List operation
type ListResult struct {
	Items      []*KeyValue
	NextCursor string
	HasMore    bool
}

// ListKeysResult represents the result of a ListKeys operation
type ListKeysResult struct {
	Keys       []string
	NextCursor string
	HasMore    bool
}

// PutItem represents an item for batch put operations
type PutItem struct {
	Key        string
	Value      []byte
	TTLSeconds int64
}

// BatchError represents an error in batch operations
type BatchError struct {
	Key   string
	Error string
}

// BatchResult represents the result of batch operations
type BatchResult struct {
	SuccessCount int
	ErrorCount   int
	Errors       []*BatchError
}

// HealthResult represents the result of a health check
type HealthResult struct {
	Healthy       bool
	Status        string
	UptimeSeconds int
	Version       string
}

// StatsResult represents the result of a stats operation
type StatsResult struct {
	TotalKeys int
	TotalSize int
	LSMSize   int
	VLogSize  int
	Details   map[string]string
}

// PutOption is a function that modifies a PutRequest
type PutOption func(*kvstore.PutRequest)

// WithTTL sets the TTL for a put operation
func WithTTL(seconds int64) PutOption {
	return func(req *kvstore.PutRequest) {
		req.TtlSeconds = seconds
	}
}

// ListOption is a function that modifies a ListRequest
type ListOption func(*kvstore.ListRequest)

// WithLimit sets the limit for list operations
func WithLimit(limit int32) ListOption {
	return func(req *kvstore.ListRequest) {
		req.Limit = limit
	}
}

// WithCursor sets the cursor for list operations (pagination)
func WithCursor(cursor string) ListOption {
	return func(req *kvstore.ListRequest) {
		req.Cursor = cursor
	}
}

// String returns a string representation of the GetResult
func (r *GetResult) String() string {
	if !r.Found {
		return "(nil)"
	}
	return string(r.Value)
}

// IsExpired checks if the key has expired
func (r *GetResult) IsExpired() bool {
	if r.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(r.ExpiresAt)
}

// HasTTL checks if the key has a TTL set
func (r *GetResult) HasTTL() bool {
	return !r.ExpiresAt.IsZero()
}

// TTL returns the time until expiration
func (r *GetResult) TTL() time.Duration {
	if r.ExpiresAt.IsZero() {
		return 0
	}
	return time.Until(r.ExpiresAt)
}

// IsEmpty checks if the list result is empty
func (r *ListResult) IsEmpty() bool {
	return len(r.Items) == 0
}

// Count returns the number of items in the list result
func (r *ListResult) Count() int {
	return len(r.Items)
}

// Keys returns all keys from the list result
func (r *ListResult) Keys() []string {
	keys := make([]string, len(r.Items))
	for i, item := range r.Items {
		keys[i] = item.Key
	}
	return keys
}

// Values returns all values from the list result
func (r *ListResult) Values() [][]byte {
	values := make([][]byte, len(r.Items))
	for i, item := range r.Items {
		values[i] = item.Value
	}
	return values
}

// IsEmpty checks if the list keys result is empty
func (r *ListKeysResult) IsEmpty() bool {
	return len(r.Keys) == 0
}

// Count returns the number of keys in the result
func (r *ListKeysResult) Count() int {
	return len(r.Keys)
}

// HasErrors checks if the batch result has any errors
func (r *BatchResult) HasErrors() bool {
	return r.ErrorCount > 0
}

// IsFullSuccess checks if all operations in the batch succeeded
func (r *BatchResult) IsFullSuccess() bool {
	return r.ErrorCount == 0
}

// TotalOperations returns the total number of operations
func (r *BatchResult) TotalOperations() int {
	return r.SuccessCount + r.ErrorCount
}

// SuccessRate returns the success rate as a percentage
func (r *BatchResult) SuccessRate() float64 {
	total := r.TotalOperations()
	if total == 0 {
		return 0
	}
	return float64(r.SuccessCount) / float64(total) * 100
}

// GetErrorsForKey returns all errors for a specific key
func (r *BatchResult) GetErrorsForKey(key string) []string {
	var errors []string
	for _, err := range r.Errors {
		if err.Key == key {
			errors = append(errors, err.Error)
		}
	}
	return errors
}

// IsHealthy checks if the server is healthy
func (r *HealthResult) IsHealthy() bool {
	return r.Healthy
}

// UptimeDuration returns the uptime as a time.Duration
func (r *HealthResult) UptimeDuration() time.Duration {
	return time.Duration(r.UptimeSeconds) * time.Second
}

// TotalSizeBytes returns the total size in bytes
func (r *StatsResult) TotalSizeBytes() int {
	return r.TotalSize
}

// TotalSizeMB returns the total size in megabytes
func (r *StatsResult) TotalSizeMB() float64 {
	return float64(r.TotalSize) / (1024 * 1024)
}

// LSMSizeBytes returns the LSM size in bytes
func (r *StatsResult) LSMSizeBytes() int {
	return r.LSMSize
}

// VLogSizeBytes returns the value log size in bytes
func (r *StatsResult) VLogSizeBytes() int {
	return r.VLogSize
}

// GetDetail returns a specific detail value
func (r *StatsResult) GetDetail(key string) (string, bool) {
	value, exists := r.Details[key]
	return value, exists
}