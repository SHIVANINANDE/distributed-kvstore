package chaos

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// NetworkPartitioner simulates network partitions and failures
type NetworkPartitioner struct {
	mu         sync.RWMutex
	config     ChaosConfig
	logger     *log.Logger
	partitions map[string]*NetworkPartition
	rules      map[string]*IptablesRule
}

// NetworkPartition represents an active network partition
type NetworkPartition struct {
	ID          string                `json:"id"`
	Type        PartitionType         `json:"type"`
	StartTime   time.Time             `json:"start_time"`
	Duration    time.Duration         `json:"duration"`
	AffectedNodes []string            `json:"affected_nodes"`
	Rules       []*IptablesRule       `json:"rules"`
	Status      PartitionStatus       `json:"status"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// PartitionType defines types of network partitions
type PartitionType string

const (
	PartitionTypeSplit          PartitionType = "split"           // Split-brain partition
	PartitionTypeIsolation      PartitionType = "isolation"      // Isolate specific nodes
	PartitionTypeAsymmetric     PartitionType = "asymmetric"     // One-way communication failure
	PartitionTypeLatency        PartitionType = "latency"        // High latency simulation
	PartitionTypePacketLoss     PartitionType = "packet_loss"    // Packet loss simulation
	PartitionTypeBandwidthLimit PartitionType = "bandwidth"      // Bandwidth limitation
)

// PartitionStatus represents the status of a partition
type PartitionStatus string

const (
	PartitionStatusActive   PartitionStatus = "active"
	PartitionStatusHealing  PartitionStatus = "healing"
	PartitionStatusHealed   PartitionStatus = "healed"
	PartitionStatusFailed   PartitionStatus = "failed"
)

// IptablesRule represents an iptables rule for network manipulation
type IptablesRule struct {
	ID       string    `json:"id"`
	Rule     string    `json:"rule"`
	Applied  bool      `json:"applied"`
	AppliedAt time.Time `json:"applied_at"`
	Type     string    `json:"type"` // DROP, REJECT, DELAY, etc.
}

// NewNetworkPartitioner creates a new network partitioner
func NewNetworkPartitioner(config ChaosConfig, logger *log.Logger) *NetworkPartitioner {
	if logger == nil {
		logger = log.New(log.Writer(), "[NET_CHAOS] ", log.LstdFlags)
	}
	
	return &NetworkPartitioner{
		config:     config,
		logger:     logger,
		partitions: make(map[string]*NetworkPartition),
		rules:      make(map[string]*IptablesRule),
	}
}

// CreatePartition creates a network partition
func (np *NetworkPartitioner) CreatePartition(targetNodes []string, duration time.Duration) (string, error) {
	np.mu.Lock()
	defer np.mu.Unlock()
	
	if !np.config.NetworkEnabled {
		return "", fmt.Errorf("network chaos is disabled")
	}
	
	// Generate partition ID
	partitionID := fmt.Sprintf("partition_%d_%d", time.Now().Unix(), rand.Int63())
	
	np.logger.Printf("Creating network partition %s affecting nodes: %v", partitionID, targetNodes)
	
	// Create partition configuration
	partition := &NetworkPartition{
		ID:            partitionID,
		Type:          PartitionTypeSplit, // Default to split partition
		StartTime:     time.Now(),
		Duration:      duration,
		AffectedNodes: targetNodes,
		Status:        PartitionStatusActive,
		Parameters:    make(map[string]interface{}),
		Rules:         make([]*IptablesRule, 0),
	}
	
	// Apply network rules
	if err := np.applyPartitionRules(partition); err != nil {
		np.logger.Printf("Failed to apply partition rules: %v", err)
		return "", fmt.Errorf("failed to create partition: %w", err)
	}
	
	np.partitions[partitionID] = partition
	
	np.logger.Printf("Network partition %s created successfully", partitionID)
	return partitionID, nil
}

// CreateCustomPartition creates a partition with specific type and parameters
func (np *NetworkPartitioner) CreateCustomPartition(partitionType PartitionType, targetNodes []string, duration time.Duration, params map[string]interface{}) (string, error) {
	np.mu.Lock()
	defer np.mu.Unlock()
	
	partitionID := fmt.Sprintf("%s_%d_%d", partitionType, time.Now().Unix(), rand.Int63())
	
	partition := &NetworkPartition{
		ID:            partitionID,
		Type:          partitionType,
		StartTime:     time.Now(),
		Duration:      duration,
		AffectedNodes: targetNodes,
		Status:        PartitionStatusActive,
		Parameters:    params,
		Rules:         make([]*IptablesRule, 0),
	}
	
	// Apply type-specific rules
	if err := np.applyCustomPartitionRules(partition); err != nil {
		return "", fmt.Errorf("failed to create custom partition: %w", err)
	}
	
	np.partitions[partitionID] = partition
	
	np.logger.Printf("Custom partition %s (%s) created successfully", partitionID, partitionType)
	return partitionID, nil
}

// applyPartitionRules applies network rules for a basic partition
func (np *NetworkPartitioner) applyPartitionRules(partition *NetworkPartition) error {
	// Create rules to block communication between affected nodes
	for i, node1 := range partition.AffectedNodes {
		for j, node2 := range partition.AffectedNodes {
			if i != j {
				rule := np.createBlockRule(node1, node2)
				if err := np.applyRule(rule); err != nil {
					np.logger.Printf("Failed to apply rule %s: %v", rule.ID, err)
					// Continue with other rules
				} else {
					partition.Rules = append(partition.Rules, rule)
				}
			}
		}
	}
	
	return nil
}

// applyCustomPartitionRules applies rules for custom partition types
func (np *NetworkPartitioner) applyCustomPartitionRules(partition *NetworkPartition) error {
	switch partition.Type {
	case PartitionTypeIsolation:
		return np.applyIsolationRules(partition)
	case PartitionTypeAsymmetric:
		return np.applyAsymmetricRules(partition)
	case PartitionTypeLatency:
		return np.applyLatencyRules(partition)
	case PartitionTypePacketLoss:
		return np.applyPacketLossRules(partition)
	case PartitionTypeBandwidthLimit:
		return np.applyBandwidthRules(partition)
	default:
		return np.applyPartitionRules(partition)
	}
}

// applyIsolationRules isolates specific nodes from the rest of the cluster
func (np *NetworkPartitioner) applyIsolationRules(partition *NetworkPartition) error {
	np.logger.Printf("Applying isolation rules for partition %s", partition.ID)
	
	// Get all cluster nodes (simulated)
	allNodes := np.getAllClusterNodes()
	
	// Block communication from isolated nodes to all other nodes
	for _, isolatedNode := range partition.AffectedNodes {
		for _, otherNode := range allNodes {
			if !np.contains(partition.AffectedNodes, otherNode) {
				rule := np.createBlockRule(isolatedNode, otherNode)
				if err := np.applyRule(rule); err == nil {
					partition.Rules = append(partition.Rules, rule)
				}
			}
		}
	}
	
	return nil
}

// applyAsymmetricRules creates one-way communication failures
func (np *NetworkPartitioner) applyAsymmetricRules(partition *NetworkPartition) error {
	np.logger.Printf("Applying asymmetric rules for partition %s", partition.ID)
	
	if len(partition.AffectedNodes) < 2 {
		return fmt.Errorf("asymmetric partition requires at least 2 nodes")
	}
	
	// Create one-way block from first node to second node
	rule := np.createBlockRule(partition.AffectedNodes[0], partition.AffectedNodes[1])
	if err := np.applyRule(rule); err == nil {
		partition.Rules = append(partition.Rules, rule)
	}
	
	return nil
}

// applyLatencyRules adds artificial latency to network communication
func (np *NetworkPartitioner) applyLatencyRules(partition *NetworkPartition) error {
	np.logger.Printf("Applying latency rules for partition %s", partition.ID)
	
	// Get latency value from parameters
	latency, ok := partition.Parameters["latency"].(string)
	if !ok {
		latency = "100ms" // Default latency
	}
	
	for _, node := range partition.AffectedNodes {
		rule := np.createLatencyRule(node, latency)
		if err := np.applyRule(rule); err == nil {
			partition.Rules = append(partition.Rules, rule)
		}
	}
	
	return nil
}

// applyPacketLossRules simulates packet loss
func (np *NetworkPartitioner) applyPacketLossRules(partition *NetworkPartition) error {
	np.logger.Printf("Applying packet loss rules for partition %s", partition.ID)
	
	// Get packet loss percentage from parameters
	lossRate, ok := partition.Parameters["loss_rate"].(float64)
	if !ok {
		lossRate = 10.0 // Default 10% packet loss
	}
	
	for _, node := range partition.AffectedNodes {
		rule := np.createPacketLossRule(node, lossRate)
		if err := np.applyRule(rule); err == nil {
			partition.Rules = append(partition.Rules, rule)
		}
	}
	
	return nil
}

// applyBandwidthRules limits bandwidth
func (np *NetworkPartitioner) applyBandwidthRules(partition *NetworkPartition) error {
	np.logger.Printf("Applying bandwidth rules for partition %s", partition.ID)
	
	// Get bandwidth limit from parameters
	bandwidth, ok := partition.Parameters["bandwidth"].(string)
	if !ok {
		bandwidth = "1mbit" // Default bandwidth limit
	}
	
	for _, node := range partition.AffectedNodes {
		rule := np.createBandwidthRule(node, bandwidth)
		if err := np.applyRule(rule); err == nil {
			partition.Rules = append(partition.Rules, rule)
		}
	}
	
	return nil
}

// createBlockRule creates a rule to block communication between two nodes
func (np *NetworkPartitioner) createBlockRule(sourceNode, targetNode string) *IptablesRule {
	ruleID := fmt.Sprintf("block_%s_%s_%d", sourceNode, targetNode, time.Now().UnixNano())
	
	// Convert node names to IP addresses (simplified)
	sourceIP := np.nodeToIP(sourceNode)
	targetIP := np.nodeToIP(targetNode)
	
	rule := &IptablesRule{
		ID:   ruleID,
		Rule: fmt.Sprintf("iptables -A OUTPUT -s %s -d %s -j DROP", sourceIP, targetIP),
		Type: "DROP",
	}
	
	return rule
}

// createLatencyRule creates a rule to add network latency
func (np *NetworkPartitioner) createLatencyRule(node, latency string) *IptablesRule {
	ruleID := fmt.Sprintf("latency_%s_%d", node, time.Now().UnixNano())
	
	// Use tc (traffic control) for latency simulation
	rule := &IptablesRule{
		ID:   ruleID,
		Rule: fmt.Sprintf("tc qdisc add dev eth0 root netem delay %s", latency),
		Type: "DELAY",
	}
	
	return rule
}

// createPacketLossRule creates a rule to simulate packet loss
func (np *NetworkPartitioner) createPacketLossRule(node string, lossRate float64) *IptablesRule {
	ruleID := fmt.Sprintf("loss_%s_%d", node, time.Now().UnixNano())
	
	rule := &IptablesRule{
		ID:   ruleID,
		Rule: fmt.Sprintf("tc qdisc add dev eth0 root netem loss %.1f%%", lossRate),
		Type: "LOSS",
	}
	
	return rule
}

// createBandwidthRule creates a rule to limit bandwidth
func (np *NetworkPartitioner) createBandwidthRule(node, bandwidth string) *IptablesRule {
	ruleID := fmt.Sprintf("bandwidth_%s_%d", node, time.Now().UnixNano())
	
	rule := &IptablesRule{
		ID:   ruleID,
		Rule: fmt.Sprintf("tc qdisc add dev eth0 root handle 1: tbf rate %s buffer 1600 limit 3000", bandwidth),
		Type: "BANDWIDTH",
	}
	
	return rule
}

// applyRule applies an iptables/tc rule
func (np *NetworkPartitioner) applyRule(rule *IptablesRule) error {
	np.logger.Printf("Applying rule %s: %s", rule.ID, rule.Rule)
	
	// In a real implementation, this would execute the actual command
	// For simulation, we'll just log it
	if np.shouldSimulateRuleFailure() {
		return fmt.Errorf("simulated rule application failure")
	}
	
	// Parse and execute the rule
	parts := strings.Fields(rule.Rule)
	if len(parts) == 0 {
		return fmt.Errorf("empty rule")
	}
	
	// Simulate rule execution
	time.Sleep(10 * time.Millisecond)
	
	rule.Applied = true
	rule.AppliedAt = time.Now()
	np.rules[rule.ID] = rule
	
	np.logger.Printf("Rule %s applied successfully", rule.ID)
	return nil
}

// HealPartition removes a network partition
func (np *NetworkPartitioner) HealPartition(partitionID string) error {
	np.mu.Lock()
	defer np.mu.Unlock()
	
	partition, exists := np.partitions[partitionID]
	if !exists {
		return fmt.Errorf("partition %s not found", partitionID)
	}
	
	if partition.Status != PartitionStatusActive {
		return fmt.Errorf("partition %s is not active", partitionID)
	}
	
	np.logger.Printf("Healing partition %s", partitionID)
	partition.Status = PartitionStatusHealing
	
	// Remove all rules associated with this partition
	for _, rule := range partition.Rules {
		if err := np.removeRule(rule); err != nil {
			np.logger.Printf("Failed to remove rule %s: %v", rule.ID, err)
		}
	}
	
	partition.Status = PartitionStatusHealed
	
	np.logger.Printf("Partition %s healed successfully", partitionID)
	return nil
}

// removeRule removes an iptables/tc rule
func (np *NetworkPartitioner) removeRule(rule *IptablesRule) error {
	if !rule.Applied {
		return nil
	}
	
	np.logger.Printf("Removing rule %s", rule.ID)
	
	// Generate removal command based on rule type
	var removeCmd string
	switch rule.Type {
	case "DROP":
		// Convert iptables -A to iptables -D
		removeCmd = strings.Replace(rule.Rule, "-A ", "-D ", 1)
	case "DELAY", "LOSS", "BANDWIDTH":
		// Remove tc qdisc
		removeCmd = "tc qdisc del dev eth0 root"
	default:
		return fmt.Errorf("unknown rule type: %s", rule.Type)
	}
	
	// Simulate command execution
	time.Sleep(5 * time.Millisecond)
	
	rule.Applied = false
	delete(np.rules, rule.ID)
	
	np.logger.Printf("Rule %s removed successfully", rule.ID)
	return nil
}

// GetPartition returns information about a specific partition
func (np *NetworkPartitioner) GetPartition(partitionID string) (*NetworkPartition, error) {
	np.mu.RLock()
	defer np.mu.RUnlock()
	
	partition, exists := np.partitions[partitionID]
	if !exists {
		return nil, fmt.Errorf("partition %s not found", partitionID)
	}
	
	// Return a copy
	partitionCopy := *partition
	return &partitionCopy, nil
}

// GetActivePartitions returns all active partitions
func (np *NetworkPartitioner) GetActivePartitions() map[string]*NetworkPartition {
	np.mu.RLock()
	defer np.mu.RUnlock()
	
	result := make(map[string]*NetworkPartition)
	for partitionID, partition := range np.partitions {
		if partition.Status == PartitionStatusActive {
			partitionCopy := *partition
			result[partitionID] = &partitionCopy
		}
	}
	
	return result
}

// CleanupExpiredPartitions removes expired partitions
func (np *NetworkPartitioner) CleanupExpiredPartitions() {
	np.mu.Lock()
	defer np.mu.Unlock()
	
	now := time.Now()
	for partitionID, partition := range np.partitions {
		if partition.Status == PartitionStatusActive && 
		   now.Sub(partition.StartTime) > partition.Duration {
			np.logger.Printf("Cleaning up expired partition %s", partitionID)
			partition.Status = PartitionStatusHealing
			
			// Remove rules
			for _, rule := range partition.Rules {
				np.removeRule(rule)
			}
			
			partition.Status = PartitionStatusHealed
		}
	}
}

// Helper functions

// nodeToIP converts a node name to an IP address (simplified)
func (np *NetworkPartitioner) nodeToIP(node string) string {
	// In a real implementation, this would resolve actual node IPs
	// For simulation, generate predictable IPs
	hash := 0
	for _, c := range node {
		hash += int(c)
	}
	return fmt.Sprintf("192.168.1.%d", 100+(hash%50))
}

// getAllClusterNodes returns all nodes in the cluster (simulated)
func (np *NetworkPartitioner) getAllClusterNodes() []string {
	return []string{"node1", "node2", "node3", "node4", "node5"}
}

// contains checks if a slice contains a string
func (np *NetworkPartitioner) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// shouldSimulateRuleFailure randomly determines if a rule should fail
func (np *NetworkPartitioner) shouldSimulateRuleFailure() bool {
	return rand.Float64() < 0.05 // 5% chance of failure
}

// executeCommand executes a system command (for real implementations)
func (np *NetworkPartitioner) executeCommand(command string) error {
	np.logger.Printf("Executing command: %s", command)
	
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	
	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		np.logger.Printf("Command failed: %s, output: %s", err, string(output))
		return err
	}
	
	np.logger.Printf("Command executed successfully: %s", string(output))
	return nil
}

// GetNetworkStats returns network-related statistics
func (np *NetworkPartitioner) GetNetworkStats() NetworkStats {
	np.mu.RLock()
	defer np.mu.RUnlock()
	
	stats := NetworkStats{
		ActivePartitions: 0,
		TotalPartitions:  len(np.partitions),
		ActiveRules:      len(np.rules),
		PartitionsByType: make(map[PartitionType]int),
	}
	
	for _, partition := range np.partitions {
		stats.PartitionsByType[partition.Type]++
		if partition.Status == PartitionStatusActive {
			stats.ActivePartitions++
		}
	}
	
	return stats
}

// NetworkStats contains network chaos statistics
type NetworkStats struct {
	ActivePartitions int                       `json:"active_partitions"`
	TotalPartitions  int                       `json:"total_partitions"`
	ActiveRules      int                       `json:"active_rules"`
	PartitionsByType map[PartitionType]int     `json:"partitions_by_type"`
}