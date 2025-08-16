package consensus

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"distributed-kvstore/proto/cluster"
)

// KVStateMachine implements a key-value store state machine for Raft
type KVStateMachine struct {
	mu       sync.RWMutex
	data     map[string]string
	metadata map[string]EntryMetadata
	logger   *log.Logger
}

// EntryMetadata contains metadata about a key-value entry
type EntryMetadata struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"`
}

// Operation represents a state machine operation
type Operation struct {
	Type      string `json:"type"`      // PUT, DELETE, NOOP
	Key       string `json:"key"`
	Value     string `json:"value"`
	Timestamp int64  `json:"timestamp"`
}

// OperationResult represents the result of applying an operation
type OperationResult struct {
	Success   bool        `json:"success"`
	Value     string      `json:"value,omitempty"`
	PrevValue string      `json:"prev_value,omitempty"`
	Error     string      `json:"error,omitempty"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

// StateMachineSnapshot represents a snapshot of the state machine
type StateMachineSnapshot struct {
	Data     map[string]string            `json:"data"`
	Metadata map[string]EntryMetadata     `json:"metadata"`
	Version  int64                        `json:"version"`
}

// NewKVStateMachine creates a new key-value state machine
func NewKVStateMachine(logger *log.Logger) *KVStateMachine {
	if logger == nil {
		logger = log.New(log.Writer(), "[SM] ", log.LstdFlags)
	}

	return &KVStateMachine{
		data:     make(map[string]string),
		metadata: make(map[string]EntryMetadata),
		logger:   logger,
	}
}

// Apply applies a log entry to the state machine
func (sm *KVStateMachine) Apply(entry *cluster.LogEntry) interface{} {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Parse the operation from the log entry data
	var op Operation
	if err := json.Unmarshal(entry.Data, &op); err != nil {
		sm.logger.Printf("Failed to unmarshal operation: %v", err)
		return &OperationResult{
			Success: false,
			Error:   fmt.Sprintf("invalid operation format: %v", err),
		}
	}

	// Apply the operation based on type
	switch op.Type {
	case "PUT":
		return sm.applyPut(op)
	case "DELETE":
		return sm.applyDelete(op)
	case "NOOP":
		return sm.applyNoop(op)
	default:
		sm.logger.Printf("Unknown operation type: %s", op.Type)
		return &OperationResult{
			Success: false,
			Error:   fmt.Sprintf("unknown operation type: %s", op.Type),
		}
	}
}

// applyPut applies a PUT operation
func (sm *KVStateMachine) applyPut(op Operation) *OperationResult {
	prevValue, existed := sm.data[op.Key]
	
	sm.data[op.Key] = op.Value
	
	now := time.Now()
	if metadata, exists := sm.metadata[op.Key]; exists {
		metadata.UpdatedAt = now
		metadata.Version++
		sm.metadata[op.Key] = metadata
	} else {
		sm.metadata[op.Key] = EntryMetadata{
			CreatedAt: now,
			UpdatedAt: now,
			Version:   1,
		}
	}

	sm.logger.Printf("Applied PUT: %s = %s", op.Key, op.Value)

	result := &OperationResult{
		Success:  true,
		Value:    op.Value,
		Metadata: sm.metadata[op.Key],
	}

	if existed {
		result.PrevValue = prevValue
	}

	return result
}

// applyDelete applies a DELETE operation
func (sm *KVStateMachine) applyDelete(op Operation) *OperationResult {
	prevValue, existed := sm.data[op.Key]
	
	if existed {
		delete(sm.data, op.Key)
		delete(sm.metadata, op.Key)
		sm.logger.Printf("Applied DELETE: %s (was: %s)", op.Key, prevValue)
	} else {
		sm.logger.Printf("Applied DELETE: %s (key not found)", op.Key)
	}

	return &OperationResult{
		Success:   true,
		PrevValue: prevValue,
		Metadata: map[string]interface{}{
			"existed": existed,
		},
	}
}

// applyNoop applies a NOOP operation (used for heartbeats)
func (sm *KVStateMachine) applyNoop(op Operation) *OperationResult {
	sm.logger.Printf("Applied NOOP operation")
	return &OperationResult{
		Success: true,
		Metadata: map[string]interface{}{
			"timestamp": op.Timestamp,
			"type":      "noop",
		},
	}
}

// Get retrieves a value from the state machine (read-only)
func (sm *KVStateMachine) Get(key string) (string, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	value, exists := sm.data[key]
	return value, exists
}

// GetWithMetadata retrieves a value and its metadata
func (sm *KVStateMachine) GetWithMetadata(key string) (string, EntryMetadata, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	value, exists := sm.data[key]
	metadata := sm.metadata[key]
	return value, metadata, exists
}

// List returns all key-value pairs (read-only)
func (sm *KVStateMachine) List() map[string]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	result := make(map[string]string)
	for k, v := range sm.data {
		result[k] = v
	}
	return result
}

// Size returns the number of entries in the state machine
func (sm *KVStateMachine) Size() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.data)
}

// Snapshot creates a snapshot of the current state machine state
func (sm *KVStateMachine) Snapshot() ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshot := StateMachineSnapshot{
		Data:     make(map[string]string),
		Metadata: make(map[string]EntryMetadata),
		Version:  time.Now().Unix(),
	}

	// Copy data
	for k, v := range sm.data {
		snapshot.Data[k] = v
	}

	// Copy metadata
	for k, v := range sm.metadata {
		snapshot.Metadata[k] = v
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	sm.logger.Printf("Created snapshot with %d entries", len(snapshot.Data))
	return data, nil
}

// Restore restores the state machine from a snapshot
func (sm *KVStateMachine) Restore(data []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var snapshot StateMachineSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	// Replace current state with snapshot data
	sm.data = make(map[string]string)
	sm.metadata = make(map[string]EntryMetadata)

	for k, v := range snapshot.Data {
		sm.data[k] = v
	}

	for k, v := range snapshot.Metadata {
		sm.metadata[k] = v
	}

	sm.logger.Printf("Restored state machine from snapshot with %d entries", len(sm.data))
	return nil
}

// CreatePutOperation creates a PUT operation for the given key-value pair
func CreatePutOperation(key, value string) ([]byte, error) {
	op := Operation{
		Type:      "PUT",
		Key:       key,
		Value:     value,
		Timestamp: time.Now().Unix(),
	}

	return json.Marshal(op)
}

// CreateDeleteOperation creates a DELETE operation for the given key
func CreateDeleteOperation(key string) ([]byte, error) {
	op := Operation{
		Type:      "DELETE",
		Key:       key,
		Timestamp: time.Now().Unix(),
	}

	return json.Marshal(op)
}

// CreateNoopOperation creates a NOOP operation
func CreateNoopOperation() ([]byte, error) {
	op := Operation{
		Type:      "NOOP",
		Timestamp: time.Now().Unix(),
	}

	return json.Marshal(op)
}

// ValidateOperation validates an operation before applying it
func ValidateOperation(data []byte) error {
	var op Operation
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("invalid operation format: %w", err)
	}

	switch op.Type {
	case "PUT":
		if op.Key == "" {
			return fmt.Errorf("PUT operation requires a key")
		}
	case "DELETE":
		if op.Key == "" {
			return fmt.Errorf("DELETE operation requires a key")
		}
	case "NOOP":
		// NOOP operations don't require validation
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}

	return nil
}

// GetStats returns statistics about the state machine
func (sm *KVStateMachine) GetStats() StateMachineStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := StateMachineStats{
		EntryCount: int64(len(sm.data)),
		DataSize:   0,
	}

	// Calculate approximate data size
	for k, v := range sm.data {
		stats.DataSize += int64(len(k) + len(v))
	}

	return stats
}

// StateMachineStats provides statistics about the state machine
type StateMachineStats struct {
	EntryCount int64 `json:"entry_count"`
	DataSize   int64 `json:"data_size_bytes"`
}