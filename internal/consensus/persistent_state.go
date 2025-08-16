package consensus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PersistentStateData represents the data that must be persisted across restarts
type PersistentStateData struct {
	CurrentTerm int64  `json:"current_term"`
	VotedFor    string `json:"voted_for"`
	// Log entries would be persisted separately in a real implementation
}

// PersistentState manages the persistent state of a Raft node
type PersistentState struct {
	mu       sync.RWMutex
	nodeID   string
	filePath string
	data     *PersistentStateData
}

// NewPersistentState creates a new persistent state manager
func NewPersistentState(nodeID string) (*PersistentState, error) {
	// Create data directory if it doesn't exist
	dataDir := filepath.Join("data", "raft", nodeID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	ps := &PersistentState{
		nodeID:   nodeID,
		filePath: filepath.Join(dataDir, "state.json"),
		data: &PersistentStateData{
			CurrentTerm: 0,
			VotedFor:    "",
		},
	}

	// Try to load existing state
	if err := ps.load(); err != nil {
		// If file doesn't exist, that's OK - we'll start with default values
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load persistent state: %w", err)
		}
		
		// Save initial state
		if err := ps.save(); err != nil {
			return nil, fmt.Errorf("failed to save initial state: %w", err)
		}
	}

	return ps, nil
}

// Load returns the current persistent state
func (ps *PersistentState) Load() (*PersistentStateData, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// Return a copy to prevent external modification
	return &PersistentStateData{
		CurrentTerm: ps.data.CurrentTerm,
		VotedFor:    ps.data.VotedFor,
	}, nil
}

// Save saves the persistent state to disk
func (ps *PersistentState) Save(data *PersistentStateData) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.data = &PersistentStateData{
		CurrentTerm: data.CurrentTerm,
		VotedFor:    data.VotedFor,
	}

	return ps.save()
}

// load loads the state from disk (internal method, assumes lock is held)
func (ps *PersistentState) load() error {
	data, err := os.ReadFile(ps.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, ps.data)
}

// save saves the state to disk (internal method, assumes lock is held)
func (ps *PersistentState) save() error {
	// Write to temporary file first, then rename for atomic operation
	tempFile := ps.filePath + ".tmp"
	
	data, err := json.MarshalIndent(ps.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempFile, ps.filePath); err != nil {
		// Clean up temp file on failure
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// UpdateTerm updates the current term
func (ps *PersistentState) UpdateTerm(term int64) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if term > ps.data.CurrentTerm {
		ps.data.CurrentTerm = term
		ps.data.VotedFor = "" // Clear vote when term changes
		return ps.save()
	}

	return nil
}

// UpdateVote updates the voted for field
func (ps *PersistentState) UpdateVote(term int64, candidateID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if term == ps.data.CurrentTerm {
		ps.data.VotedFor = candidateID
		return ps.save()
	}

	return fmt.Errorf("cannot vote for term %d, current term is %d", term, ps.data.CurrentTerm)
}

// GetCurrentTerm returns the current term
func (ps *PersistentState) GetCurrentTerm() int64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.data.CurrentTerm
}

// GetVotedFor returns the candidate voted for in the current term
func (ps *PersistentState) GetVotedFor() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.data.VotedFor
}

// Reset resets the persistent state (for testing)
func (ps *PersistentState) Reset() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.data = &PersistentStateData{
		CurrentTerm: 0,
		VotedFor:    "",
	}

	return ps.save()
}