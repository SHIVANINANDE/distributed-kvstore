package consensus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"distributed-kvstore/proto/cluster"
)

// LogStorage manages persistent storage of log entries
type LogStorage struct {
	mu         sync.RWMutex
	nodeID     string
	baseDir    string
	logFile    string
	entries    []*cluster.LogEntry
	lastIndex  int64
	compactTo  int64 // Index up to which log has been compacted
}

// LogMetadata contains metadata about the log
type LogMetadata struct {
	FirstIndex int64 `json:"first_index"`
	LastIndex  int64 `json:"last_index"`
	CompactTo  int64 `json:"compact_to"`
}

// NewLogStorage creates a new log storage manager
func NewLogStorage(nodeID string) (*LogStorage, error) {
	baseDir := filepath.Join("data", "raft", nodeID, "log")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	ls := &LogStorage{
		nodeID:  nodeID,
		baseDir: baseDir,
		logFile: filepath.Join(baseDir, "entries.json"),
		entries: make([]*cluster.LogEntry, 0),
	}

	// Load existing log entries
	if err := ls.loadLog(); err != nil {
		return nil, fmt.Errorf("failed to load log: %w", err)
	}

	return ls, nil
}

// AppendEntries appends log entries to persistent storage
func (ls *LogStorage) AppendEntries(entries []*cluster.LogEntry) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Append to in-memory log
	ls.entries = append(ls.entries, entries...)
	
	// Update last index
	if len(entries) > 0 {
		ls.lastIndex = entries[len(entries)-1].Index
	}

	// Persist to disk
	return ls.persist()
}

// GetEntries retrieves log entries from startIndex to endIndex (inclusive)
func (ls *LogStorage) GetEntries(startIndex, endIndex int64) ([]*cluster.LogEntry, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if startIndex < ls.compactTo {
		return nil, fmt.Errorf("requested entries before compaction point")
	}

	var result []*cluster.LogEntry
	for _, entry := range ls.entries {
		if entry.Index >= startIndex && entry.Index <= endIndex {
			result = append(result, entry)
		}
	}

	return result, nil
}

// GetLastIndex returns the index of the last log entry
func (ls *LogStorage) GetLastIndex() int64 {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.lastIndex
}

// GetLastTerm returns the term of the last log entry
func (ls *LogStorage) GetLastTerm() int64 {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if len(ls.entries) == 0 {
		return 0
	}
	
	return ls.entries[len(ls.entries)-1].Term
}

// TruncateFrom removes all log entries from the given index onwards
func (ls *LogStorage) TruncateFrom(index int64) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if index <= ls.compactTo {
		return fmt.Errorf("cannot truncate before compaction point")
	}

	// Find the position to truncate from
	var newEntries []*cluster.LogEntry
	for _, entry := range ls.entries {
		if entry.Index < index {
			newEntries = append(newEntries, entry)
		}
	}

	ls.entries = newEntries
	
	// Update last index
	if len(ls.entries) > 0 {
		ls.lastIndex = ls.entries[len(ls.entries)-1].Index
	} else {
		ls.lastIndex = ls.compactTo
	}

	return ls.persist()
}

// Compact removes log entries up to the given index
func (ls *LogStorage) Compact(upToIndex int64) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if upToIndex <= ls.compactTo {
		return nil // Already compacted
	}

	// Remove entries up to upToIndex
	var newEntries []*cluster.LogEntry
	for _, entry := range ls.entries {
		if entry.Index > upToIndex {
			newEntries = append(newEntries, entry)
		}
	}

	ls.entries = newEntries
	ls.compactTo = upToIndex

	return ls.persist()
}

// GetSnapshot returns a snapshot of the log state
func (ls *LogStorage) GetSnapshot() (*LogSnapshot, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	snapshot := &LogSnapshot{
		LastIndex:   ls.lastIndex,
		LastTerm:    ls.GetLastTerm(),
		CompactTo:   ls.compactTo,
		EntryCount:  int64(len(ls.entries)),
	}

	return snapshot, nil
}

// InstallSnapshot installs a snapshot and compacts the log
func (ls *LogStorage) InstallSnapshot(snapshot *LogSnapshot, entries []*cluster.LogEntry) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Replace all entries with snapshot data
	ls.entries = entries
	ls.lastIndex = snapshot.LastIndex
	ls.compactTo = snapshot.CompactTo

	return ls.persist()
}

// loadLog loads log entries from persistent storage
func (ls *LogStorage) loadLog() error {
	// Load metadata
	metadataFile := filepath.Join(ls.baseDir, "metadata.json")
	if metadataData, err := os.ReadFile(metadataFile); err == nil {
		var metadata LogMetadata
		if err := json.Unmarshal(metadataData, &metadata); err == nil {
			ls.lastIndex = metadata.LastIndex
			ls.compactTo = metadata.CompactTo
		}
	}

	// Load entries
	if logData, err := os.ReadFile(ls.logFile); err == nil {
		if err := json.Unmarshal(logData, &ls.entries); err != nil {
			return fmt.Errorf("failed to unmarshal log entries: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	return nil
}

// persist saves log entries to persistent storage
func (ls *LogStorage) persist() error {
	// Save metadata
	metadata := LogMetadata{
		FirstIndex: ls.compactTo + 1,
		LastIndex:  ls.lastIndex,
		CompactTo:  ls.compactTo,
	}

	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataFile := filepath.Join(ls.baseDir, "metadata.json")
	if err := os.WriteFile(metadataFile, metadataData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	// Save entries
	entriesData, err := json.MarshalIndent(ls.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal entries: %w", err)
	}

	tempFile := ls.logFile + ".tmp"
	if err := os.WriteFile(tempFile, entriesData, 0644); err != nil {
		return fmt.Errorf("failed to write temp log file: %w", err)
	}

	if err := os.Rename(tempFile, ls.logFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp log file: %w", err)
	}

	return nil
}

// GetStats returns statistics about the log storage
func (ls *LogStorage) GetStats() LogStats {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	return LogStats{
		EntryCount:      int64(len(ls.entries)),
		FirstIndex:      ls.compactTo + 1,
		LastIndex:       ls.lastIndex,
		CompactedUpTo:   ls.compactTo,
		StorageSizeKB:   ls.estimateStorageSize(),
	}
}

// estimateStorageSize estimates the storage size in KB
func (ls *LogStorage) estimateStorageSize() int64 {
	if info, err := os.Stat(ls.logFile); err == nil {
		return info.Size() / 1024
	}
	return 0
}

// LogSnapshot represents a snapshot of log state
type LogSnapshot struct {
	LastIndex   int64 `json:"last_index"`
	LastTerm    int64 `json:"last_term"`
	CompactTo   int64 `json:"compact_to"`
	EntryCount  int64 `json:"entry_count"`
}

// LogStats provides statistics about log storage
type LogStats struct {
	EntryCount      int64 `json:"entry_count"`
	FirstIndex      int64 `json:"first_index"`
	LastIndex       int64 `json:"last_index"`
	CompactedUpTo   int64 `json:"compacted_up_to"`
	StorageSizeKB   int64 `json:"storage_size_kb"`
}

// LogCompactor manages log compaction
type LogCompactor struct {
	storage           *LogStorage
	compactionTrigger int64 // Number of entries to trigger compaction
	retentionSize     int64 // Number of entries to retain after compaction
}

// NewLogCompactor creates a new log compactor
func NewLogCompactor(storage *LogStorage) *LogCompactor {
	return &LogCompactor{
		storage:           storage,
		compactionTrigger: 1000, // Trigger compaction after 1000 entries
		retentionSize:     100,  // Retain last 100 entries after compaction
	}
}

// ShouldCompact determines if log compaction should be triggered
func (lc *LogCompactor) ShouldCompact() bool {
	stats := lc.storage.GetStats()
	return stats.EntryCount >= lc.compactionTrigger
}

// Compact performs log compaction
func (lc *LogCompactor) Compact(upToIndex int64) error {
	// Ensure we retain some entries
	stats := lc.storage.GetStats()
	maxCompactIndex := stats.LastIndex - lc.retentionSize
	
	if upToIndex > maxCompactIndex {
		upToIndex = maxCompactIndex
	}

	if upToIndex <= 0 {
		return nil // Nothing to compact
	}

	return lc.storage.Compact(upToIndex)
}