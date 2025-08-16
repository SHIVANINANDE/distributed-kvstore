package chaos

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// NodeKiller handles node failure injection
type NodeKiller struct {
	mu           sync.RWMutex
	config       ChaosConfig
	logger       *log.Logger
	killedNodes  map[string]*KilledNode
	nodeRegistry map[string]*NodeInfo
}

// KilledNode represents a node that has been killed
type KilledNode struct {
	NodeID      string           `json:"node_id"`
	KillTime    time.Time        `json:"kill_time"`
	KillMethod  KillMethod       `json:"kill_method"`
	Duration    time.Duration    `json:"duration"`
	Status      NodeKillStatus   `json:"status"`
	ProcessID   int              `json:"process_id"`
	RestartTime *time.Time       `json:"restart_time,omitempty"`
	KillReason  string           `json:"kill_reason"`
	Metadata    map[string]string `json:"metadata"`
}

// NodeInfo represents information about a cluster node
type NodeInfo struct {
	ID          string            `json:"id"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	ProcessID   int               `json:"process_id"`
	IsHealthy   bool              `json:"is_healthy"`
	Role        string            `json:"role"` // leader, follower, candidate
	StartTime   time.Time         `json:"start_time"`
	LastSeen    time.Time         `json:"last_seen"`
	Metadata    map[string]string `json:"metadata"`
}

// KillMethod defines different ways to kill a node
type KillMethod string

const (
	KillMethodSIGTERM    KillMethod = "SIGTERM"    // Graceful shutdown
	KillMethodSIGKILL    KillMethod = "SIGKILL"    // Force kill
	KillMethodSIGSTOP    KillMethod = "SIGSTOP"    // Suspend process
	KillMethodNetworkCut KillMethod = "NETWORK"    // Network isolation
	KillMethodResource   KillMethod = "RESOURCE"   // Resource exhaustion
	KillMethodCorruption KillMethod = "CORRUPTION" // Process corruption
)

// NodeKillStatus represents the status of a killed node
type NodeKillStatus string

const (
	StatusKilled     NodeKillStatus = "killed"
	StatusSuspended  NodeKillStatus = "suspended"
	StatusRestarting NodeKillStatus = "restarting"
	StatusRecovered  NodeKillStatus = "recovered"
	StatusFailed     NodeKillStatus = "failed"
)

// NewNodeKiller creates a new node killer
func NewNodeKiller(config ChaosConfig, logger *log.Logger) *NodeKiller {
	if logger == nil {
		logger = log.New(log.Writer(), "[NODE_KILLER] ", log.LstdFlags)
	}
	
	return &NodeKiller{
		config:       config,
		logger:       logger,
		killedNodes:  make(map[string]*KilledNode),
		nodeRegistry: make(map[string]*NodeInfo),
	}
}

// RegisterNode registers a node in the killer's registry
func (nk *NodeKiller) RegisterNode(node *NodeInfo) {
	nk.mu.Lock()
	defer nk.mu.Unlock()
	
	nk.nodeRegistry[node.ID] = node
	nk.logger.Printf("Registered node %s (PID: %d)", node.ID, node.ProcessID)
}

// UnregisterNode removes a node from the registry
func (nk *NodeKiller) UnregisterNode(nodeID string) {
	nk.mu.Lock()
	defer nk.mu.Unlock()
	
	delete(nk.nodeRegistry, nodeID)
	nk.logger.Printf("Unregistered node %s", nodeID)
}

// KillNode kills a specific node
func (nk *NodeKiller) KillNode(nodeID string, method KillMethod, duration time.Duration, reason string) error {
	nk.mu.Lock()
	defer nk.mu.Unlock()
	
	if !nk.config.NodeEnabled {
		return fmt.Errorf("node chaos is disabled")
	}
	
	// Check if node exists
	node, exists := nk.nodeRegistry[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in registry", nodeID)
	}
	
	// Check if node is already killed
	if _, killed := nk.killedNodes[nodeID]; killed {
		return fmt.Errorf("node %s is already killed", nodeID)
	}
	
	// Check safety constraints
	if err := nk.checkSafetyConstraints(nodeID); err != nil {
		return fmt.Errorf("safety constraint violation: %w", err)
	}
	
	nk.logger.Printf("Killing node %s using method %s for duration %v (reason: %s)", 
		nodeID, method, duration, reason)
	
	// Create killed node record
	killedNode := &KilledNode{
		NodeID:     nodeID,
		KillTime:   time.Now(),
		KillMethod: method,
		Duration:   duration,
		ProcessID:  node.ProcessID,
		KillReason: reason,
		Metadata:   make(map[string]string),
	}
	
	// Execute the kill
	if err := nk.executeKill(killedNode, node); err != nil {
		return fmt.Errorf("failed to kill node %s: %w", nodeID, err)
	}
	
	nk.killedNodes[nodeID] = killedNode
	
	// Schedule restart if duration is specified
	if duration > 0 {
		go nk.scheduleRestart(nodeID, duration)
	}
	
	return nil
}

// KillRandomNodes kills a random selection of nodes
func (nk *NodeKiller) KillRandomNodes(count int, method KillMethod, duration time.Duration, reason string) ([]string, error) {
	nk.mu.RLock()
	
	// Get available nodes
	var availableNodes []string
	for nodeID := range nk.nodeRegistry {
		if _, killed := nk.killedNodes[nodeID]; !killed {
			availableNodes = append(availableNodes, nodeID)
		}
	}
	nk.mu.RUnlock()
	
	if len(availableNodes) == 0 {
		return nil, fmt.Errorf("no available nodes to kill")
	}
	
	if count > len(availableNodes) {
		count = len(availableNodes)
	}
	
	// Check safety constraints
	if len(availableNodes)-count < nk.config.MinHealthyNodes {
		return nil, fmt.Errorf("killing %d nodes would violate minimum healthy nodes constraint", count)
	}
	
	// Randomly select nodes
	rand.Shuffle(len(availableNodes), func(i, j int) {
		availableNodes[i], availableNodes[j] = availableNodes[j], availableNodes[i]
	})
	
	var killedNodes []string
	for i := 0; i < count; i++ {
		nodeID := availableNodes[i]
		if err := nk.KillNode(nodeID, method, duration, reason); err != nil {
			nk.logger.Printf("Failed to kill node %s: %v", nodeID, err)
		} else {
			killedNodes = append(killedNodes, nodeID)
		}
	}
	
	return killedNodes, nil
}

// executeKill executes the actual node killing based on method
func (nk *NodeKiller) executeKill(killedNode *KilledNode, node *NodeInfo) error {
	switch killedNode.KillMethod {
	case KillMethodSIGTERM:
		return nk.killWithSignal(killedNode, node, syscall.SIGTERM)
	case KillMethodSIGKILL:
		return nk.killWithSignal(killedNode, node, syscall.SIGKILL)
	case KillMethodSIGSTOP:
		return nk.suspendProcess(killedNode, node)
	case KillMethodNetworkCut:
		return nk.isolateNetworkly(killedNode, node)
	case KillMethodResource:
		return nk.exhaustResources(killedNode, node)
	case KillMethodCorruption:
		return nk.corruptProcess(killedNode, node)
	default:
		return fmt.Errorf("unknown kill method: %s", killedNode.KillMethod)
	}
}

// killWithSignal kills a process using a specific signal
func (nk *NodeKiller) killWithSignal(killedNode *KilledNode, node *NodeInfo, signal syscall.Signal) error {
	nk.logger.Printf("Sending signal %v to process %d (node %s)", signal, node.ProcessID, node.ID)
	
	// In a real implementation, this would send the actual signal
	// For simulation, we'll just mark it as killed
	if nk.shouldSimulateFailure() {
		return fmt.Errorf("simulated kill failure")
	}
	
	// Simulate signal sending
	time.Sleep(10 * time.Millisecond)
	
	if signal == syscall.SIGSTOP {
		killedNode.Status = StatusSuspended
	} else {
		killedNode.Status = StatusKilled
	}
	
	// Update node status
	node.IsHealthy = false
	
	nk.logger.Printf("Node %s killed successfully with signal %v", node.ID, signal)
	return nil
}

// suspendProcess suspends a process using SIGSTOP
func (nk *NodeKiller) suspendProcess(killedNode *KilledNode, node *NodeInfo) error {
	nk.logger.Printf("Suspending process %d (node %s)", node.ProcessID, node.ID)
	
	// Use SIGSTOP to suspend the process
	if err := nk.killWithSignal(killedNode, node, syscall.SIGSTOP); err != nil {
		return err
	}
	
	killedNode.Status = StatusSuspended
	killedNode.Metadata["suspended"] = "true"
	
	return nil
}

// isolateNetworkly isolates a node by cutting network access
func (nk *NodeKiller) isolateNetworkly(killedNode *KilledNode, node *NodeInfo) error {
	nk.logger.Printf("Isolating node %s network access", node.ID)
	
	// Create network isolation rules
	rules := []string{
		fmt.Sprintf("iptables -A INPUT -s %s -j DROP", node.Address),
		fmt.Sprintf("iptables -A OUTPUT -d %s -j DROP", node.Address),
	}
	
	for _, rule := range rules {
		if err := nk.executeCommand(rule); err != nil {
			nk.logger.Printf("Failed to apply network rule: %v", err)
		}
	}
	
	killedNode.Status = StatusKilled
	killedNode.Metadata["isolation_type"] = "network"
	
	return nil
}

// exhaustResources causes resource exhaustion
func (nk *NodeKiller) exhaustResources(killedNode *KilledNode, node *NodeInfo) error {
	nk.logger.Printf("Exhausting resources for node %s", node.ID)
	
	// Simulate resource exhaustion
	// In a real implementation, this might:
	// - Consume all available memory
	// - Max out CPU usage
	// - Fill up disk space
	// - Exhaust file descriptors
	
	killedNode.Status = StatusKilled
	killedNode.Metadata["kill_type"] = "resource_exhaustion"
	
	return nil
}

// corruptProcess corrupts the process memory or state
func (nk *NodeKiller) corruptProcess(killedNode *KilledNode, node *NodeInfo) error {
	nk.logger.Printf("Corrupting process for node %s", node.ID)
	
	// Simulate process corruption
	// In a real implementation, this might:
	// - Corrupt process memory
	// - Modify process state
	// - Inject random signals
	// - Corrupt configuration files
	
	killedNode.Status = StatusKilled
	killedNode.Metadata["kill_type"] = "corruption"
	
	return nil
}

// RestartNode restarts a killed node
func (nk *NodeKiller) RestartNode(nodeID string) error {
	nk.mu.Lock()
	defer nk.mu.Unlock()
	
	killedNode, exists := nk.killedNodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s is not killed", nodeID)
	}
	
	if killedNode.Status == StatusRecovered {
		return fmt.Errorf("node %s is already recovered", nodeID)
	}
	
	nk.logger.Printf("Restarting node %s", nodeID)
	killedNode.Status = StatusRestarting
	
	// Execute restart based on kill method
	if err := nk.executeRestart(killedNode); err != nil {
		killedNode.Status = StatusFailed
		return fmt.Errorf("failed to restart node %s: %w", nodeID, err)
	}
	
	// Update status
	now := time.Now()
	killedNode.Status = StatusRecovered
	killedNode.RestartTime = &now
	
	// Update node registry
	if node, exists := nk.nodeRegistry[nodeID]; exists {
		node.IsHealthy = true
		node.LastSeen = now
	}
	
	// Remove from killed nodes
	delete(nk.killedNodes, nodeID)
	
	nk.logger.Printf("Node %s restarted successfully", nodeID)
	return nil
}

// executeRestart executes the restart process based on kill method
func (nk *NodeKiller) executeRestart(killedNode *KilledNode) error {
	switch killedNode.KillMethod {
	case KillMethodSIGSTOP:
		return nk.resumeProcess(killedNode)
	case KillMethodNetworkCut:
		return nk.restoreNetwork(killedNode)
	case KillMethodSIGTERM, KillMethodSIGKILL:
		return nk.restartProcess(killedNode)
	case KillMethodResource:
		return nk.restoreResources(killedNode)
	case KillMethodCorruption:
		return nk.restartProcess(killedNode) // Usually requires full restart
	default:
		return fmt.Errorf("unknown kill method for restart: %s", killedNode.KillMethod)
	}
}

// resumeProcess resumes a suspended process using SIGCONT
func (nk *NodeKiller) resumeProcess(killedNode *KilledNode) error {
	nk.logger.Printf("Resuming process %d", killedNode.ProcessID)
	
	// Send SIGCONT to resume the process
	// In a real implementation: syscall.Kill(killedNode.ProcessID, syscall.SIGCONT)
	time.Sleep(10 * time.Millisecond) // Simulate resume
	
	return nil
}

// restoreNetwork restores network access for an isolated node
func (nk *NodeKiller) restoreNetwork(killedNode *KilledNode) error {
	nk.logger.Printf("Restoring network access for node %s", killedNode.NodeID)
	
	// Remove network isolation rules
	node := nk.nodeRegistry[killedNode.NodeID]
	rules := []string{
		fmt.Sprintf("iptables -D INPUT -s %s -j DROP", node.Address),
		fmt.Sprintf("iptables -D OUTPUT -d %s -j DROP", node.Address),
	}
	
	for _, rule := range rules {
		if err := nk.executeCommand(rule); err != nil {
			nk.logger.Printf("Failed to remove network rule: %v", err)
		}
	}
	
	return nil
}

// restartProcess fully restarts a killed process
func (nk *NodeKiller) restartProcess(killedNode *KilledNode) error {
	nk.logger.Printf("Restarting process for node %s", killedNode.NodeID)
	
	// In a real implementation, this would:
	// 1. Execute the node's start command
	// 2. Wait for the process to become healthy
	// 3. Update the process ID in the registry
	
	// Simulate process restart
	time.Sleep(2 * time.Second)
	
	// Update process ID (simulated)
	if node, exists := nk.nodeRegistry[killedNode.NodeID]; exists {
		node.ProcessID = rand.Intn(65536) + 1000 // Simulate new PID
		node.StartTime = time.Now()
	}
	
	return nil
}

// restoreResources restores system resources
func (nk *NodeKiller) restoreResources(killedNode *KilledNode) error {
	nk.logger.Printf("Restoring resources for node %s", killedNode.NodeID)
	
	// In a real implementation, this would:
	// - Free up consumed memory
	// - Reduce CPU usage
	// - Clean up disk space
	// - Release file descriptors
	
	return nil
}

// scheduleRestart schedules a node restart after a specified duration
func (nk *NodeKiller) scheduleRestart(nodeID string, duration time.Duration) {
	nk.logger.Printf("Scheduling restart for node %s in %v", nodeID, duration)
	
	time.Sleep(duration)
	
	if err := nk.RestartNode(nodeID); err != nil {
		nk.logger.Printf("Scheduled restart failed for node %s: %v", nodeID, err)
	}
}

// checkSafetyConstraints checks if killing a node violates safety constraints
func (nk *NodeKiller) checkSafetyConstraints(nodeID string) error {
	if !nk.config.SafetyMode {
		return nil // Safety checks disabled
	}
	
	// Count currently healthy nodes
	healthyNodes := 0
	for _, node := range nk.nodeRegistry {
		if node.IsHealthy {
			healthyNodes++
		}
	}
	
	// Check if killing this node would violate minimum healthy nodes
	if healthyNodes-1 < nk.config.MinHealthyNodes {
		return fmt.Errorf("killing node would leave only %d healthy nodes (minimum: %d)", 
			healthyNodes-1, nk.config.MinHealthyNodes)
	}
	
	// Check maximum killed nodes
	if len(nk.killedNodes) >= nk.config.MaxKilledNodes {
		return fmt.Errorf("maximum killed nodes limit reached (%d)", nk.config.MaxKilledNodes)
	}
	
	return nil
}

// GetKilledNode returns information about a killed node
func (nk *NodeKiller) GetKilledNode(nodeID string) (*KilledNode, error) {
	nk.mu.RLock()
	defer nk.mu.RUnlock()
	
	killedNode, exists := nk.killedNodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s is not killed", nodeID)
	}
	
	// Return a copy
	killedNodeCopy := *killedNode
	return &killedNodeCopy, nil
}

// GetAllKilledNodes returns all currently killed nodes
func (nk *NodeKiller) GetAllKilledNodes() map[string]*KilledNode {
	nk.mu.RLock()
	defer nk.mu.RUnlock()
	
	result := make(map[string]*KilledNode)
	for nodeID, killedNode := range nk.killedNodes {
		killedNodeCopy := *killedNode
		result[nodeID] = &killedNodeCopy
	}
	
	return result
}

// GetNodeStats returns node killer statistics
func (nk *NodeKiller) GetNodeStats() NodeKillerStats {
	nk.mu.RLock()
	defer nk.mu.RUnlock()
	
	stats := NodeKillerStats{
		TotalNodes:       len(nk.nodeRegistry),
		KilledNodes:      len(nk.killedNodes),
		HealthyNodes:     0,
		KillsByMethod:    make(map[KillMethod]int),
		NodesByStatus:    make(map[NodeKillStatus]int),
	}
	
	// Count healthy nodes
	for _, node := range nk.nodeRegistry {
		if node.IsHealthy {
			stats.HealthyNodes++
		}
	}
	
	// Count kills by method and status
	for _, killedNode := range nk.killedNodes {
		stats.KillsByMethod[killedNode.KillMethod]++
		stats.NodesByStatus[killedNode.Status]++
	}
	
	return stats
}

// NodeKillerStats contains node killer statistics
type NodeKillerStats struct {
	TotalNodes    int                        `json:"total_nodes"`
	KilledNodes   int                        `json:"killed_nodes"`
	HealthyNodes  int                        `json:"healthy_nodes"`
	KillsByMethod map[KillMethod]int         `json:"kills_by_method"`
	NodesByStatus map[NodeKillStatus]int     `json:"nodes_by_status"`
}

// Helper functions

// shouldSimulateFailure randomly determines if an operation should fail
func (nk *NodeKiller) shouldSimulateFailure() bool {
	return rand.Float64() < 0.02 // 2% chance of failure
}

// executeCommand executes a system command
func (nk *NodeKiller) executeCommand(command string) error {
	nk.logger.Printf("Executing command: %s", command)
	
	// In a real implementation, this would execute the actual command
	// For simulation, we'll just log it and simulate success/failure
	if nk.shouldSimulateFailure() {
		return fmt.Errorf("simulated command failure")
	}
	
	time.Sleep(10 * time.Millisecond) // Simulate execution time
	return nil
}

// CleanupRecoveredNodes removes recovered nodes from the killed nodes map
func (nk *NodeKiller) CleanupRecoveredNodes() {
	nk.mu.Lock()
	defer nk.mu.Unlock()
	
	for nodeID, killedNode := range nk.killedNodes {
		if killedNode.Status == StatusRecovered {
			delete(nk.killedNodes, nodeID)
			nk.logger.Printf("Cleaned up recovered node %s", nodeID)
		}
	}
}