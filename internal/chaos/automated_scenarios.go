package chaos

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// AutomatedScenarios handles automated chaos testing scenarios
type AutomatedScenarios struct {
	mu           sync.RWMutex
	framework    *ChaosFramework
	logger       *log.Logger
	scenarios    map[string]*Scenario
	isRunning    bool
	stopCh       chan struct{}
	config       AutomatedConfig
}

// AutomatedConfig contains configuration for automated scenarios
type AutomatedConfig struct {
	Enabled              bool          `json:"enabled"`
	ScenarioInterval     time.Duration `json:"scenario_interval"`     // Time between scenarios
	RandomizeTiming      bool          `json:"randomize_timing"`      // Add random delays
	MaxConcurrentScenarios int         `json:"max_concurrent"`        // Max concurrent scenarios
	SafetyMode           bool          `json:"safety_mode"`           // Enable safety checks
	ContinuousMode       bool          `json:"continuous_mode"`       // Run scenarios continuously
}

// Scenario represents an automated chaos scenario
type Scenario struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Type            ScenarioType           `json:"type"`
	Status          ScenarioStatus         `json:"status"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         *time.Time             `json:"end_time,omitempty"`
	Duration        time.Duration          `json:"duration"`
	Steps           []*ScenarioStep        `json:"steps"`
	Results         *ScenarioResults       `json:"results"`
	Parameters      map[string]interface{} `json:"parameters"`
	Experiments     []string               `json:"experiments"` // List of experiment IDs
}

// ScenarioType defines types of automated scenarios
type ScenarioType string

const (
	ScenarioTypeBasic          ScenarioType = "basic_chaos"
	ScenarioTypeNetworkChaos   ScenarioType = "network_chaos"
	ScenarioTypeNodeChaos      ScenarioType = "node_chaos"
	ScenarioTypeDiskChaos      ScenarioType = "disk_chaos"
	ScenarioTypeByzantine      ScenarioType = "byzantine_faults"
	ScenarioTypePartitionTest  ScenarioType = "partition_tolerance"
	ScenarioTypeRecoveryTest   ScenarioType = "recovery_test"
	ScenarioTypeCombined       ScenarioType = "combined_chaos"
	ScenarioTypeStressTest     ScenarioType = "stress_test"
)

// ScenarioStatus represents the status of a scenario
type ScenarioStatus string

const (
	ScenarioStatusPending   ScenarioStatus = "pending"
	ScenarioStatusRunning   ScenarioStatus = "running"
	ScenarioStatusCompleted ScenarioStatus = "completed"
	ScenarioStatusFailed    ScenarioStatus = "failed"
	ScenarioStatusAborted   ScenarioStatus = "aborted"
)

// ScenarioStep represents a step in a chaos scenario
type ScenarioStep struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // partition, kill_node, corrupt_disk, etc.
	Parameters  map[string]interface{} `json:"parameters"`
	Duration    time.Duration          `json:"duration"`
	StartTime   time.Time              `json:"start_time"`
	Status      string                 `json:"status"`
	Error       string                 `json:"error,omitempty"`
}

// ScenarioResults contains the results of a scenario execution
type ScenarioResults struct {
	Success                bool                   `json:"success"`
	TotalSteps            int                    `json:"total_steps"`
	CompletedSteps        int                    `json:"completed_steps"`
	FailedSteps           int                    `json:"failed_steps"`
	TotalRecoveryTime     time.Duration          `json:"total_recovery_time"`
	MaxRecoveryTime       time.Duration          `json:"max_recovery_time"`
	ConsistencyViolations int                    `json:"consistency_violations"`
	DataLossDetected      bool                   `json:"data_loss_detected"`
	AvailabilityImpact    float64                `json:"availability_impact"`
	PerformanceImpact     float64                `json:"performance_impact"`
	Observations          []string               `json:"observations"`
	Recommendations       []string               `json:"recommendations"`
}

// NewAutomatedScenarios creates a new automated scenarios manager
func NewAutomatedScenarios(framework *ChaosFramework, config AutomatedConfig, logger *log.Logger) *AutomatedScenarios {
	if logger == nil {
		logger = log.New(log.Writer(), "[AUTO_CHAOS] ", log.LstdFlags)
	}
	
	// Set default values
	if config.ScenarioInterval == 0 {
		config.ScenarioInterval = 10 * time.Minute
	}
	if config.MaxConcurrentScenarios == 0 {
		config.MaxConcurrentScenarios = 2
	}
	
	return &AutomatedScenarios{
		framework: framework,
		logger:    logger,
		scenarios: make(map[string]*Scenario),
		stopCh:    make(chan struct{}),
		config:    config,
	}
}

// Start starts the automated chaos testing
func (as *AutomatedScenarios) Start(ctx context.Context) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	if as.isRunning {
		return fmt.Errorf("automated scenarios are already running")
	}
	
	if !as.config.Enabled {
		return fmt.Errorf("automated scenarios are disabled")
	}
	
	as.logger.Printf("Starting automated chaos scenarios")
	as.isRunning = true
	
	// Start scenario execution loop
	go as.scenarioLoop(ctx)
	
	return nil
}

// Stop stops the automated chaos testing
func (as *AutomatedScenarios) Stop() {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	if !as.isRunning {
		return
	}
	
	as.logger.Printf("Stopping automated chaos scenarios")
	close(as.stopCh)
	as.isRunning = false
}

// scenarioLoop runs scenarios continuously or at intervals
func (as *AutomatedScenarios) scenarioLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-as.stopCh:
			return
		default:
			// Check if we can run more scenarios
			if as.getRunningScenarioCount() < as.config.MaxConcurrentScenarios {
				scenario := as.selectNextScenario()
				if scenario != nil {
					go as.executeScenario(ctx, scenario)
				}
			}
			
			// Wait before next iteration
			interval := as.config.ScenarioInterval
			if as.config.RandomizeTiming {
				// Add randomness (±25% of interval)
				variance := time.Duration(float64(interval) * 0.25)
				randomDelay := time.Duration(rand.Int63n(int64(variance*2))) - variance
				interval += randomDelay
			}
			
			select {
			case <-time.After(interval):
			case <-as.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// selectNextScenario selects the next scenario to run
func (as *AutomatedScenarios) selectNextScenario() *Scenario {
	// Define available scenario types
	scenarioTypes := []ScenarioType{
		ScenarioTypeBasic,
		ScenarioTypeNetworkChaos,
		ScenarioTypeNodeChaos,
		ScenarioTypeDiskChaos,
		ScenarioTypeByzantine,
		ScenarioTypePartitionTest,
		ScenarioTypeRecoveryTest,
		ScenarioTypeCombined,
	}
	
	// Randomly select a scenario type
	selectedType := scenarioTypes[rand.Intn(len(scenarioTypes))]
	
	// Create scenario based on type
	return as.createScenario(selectedType)
}

// createScenario creates a scenario of the specified type
func (as *AutomatedScenarios) createScenario(scenarioType ScenarioType) *Scenario {
	scenarioID := fmt.Sprintf("%s_%d", scenarioType, time.Now().UnixNano())
	
	scenario := &Scenario{
		ID:          scenarioID,
		Type:        scenarioType,
		Status:      ScenarioStatusPending,
		Parameters:  make(map[string]interface{}),
		Results:     &ScenarioResults{},
		Experiments: make([]string, 0),
	}
	
	switch scenarioType {
	case ScenarioTypeBasic:
		scenario.Name = "Basic Chaos Test"
		scenario.Description = "Basic chaos test with single failure injection"
		scenario.Duration = 2 * time.Minute
		scenario.Steps = as.createBasicChaosSteps()
		
	case ScenarioTypeNetworkChaos:
		scenario.Name = "Network Chaos Test"
		scenario.Description = "Network partition and connectivity issues"
		scenario.Duration = 5 * time.Minute
		scenario.Steps = as.createNetworkChaosSteps()
		
	case ScenarioTypeNodeChaos:
		scenario.Name = "Node Failure Test"
		scenario.Description = "Node killing and recovery testing"
		scenario.Duration = 3 * time.Minute
		scenario.Steps = as.createNodeChaosSteps()
		
	case ScenarioTypeDiskChaos:
		scenario.Name = "Disk Corruption Test"
		scenario.Description = "Disk corruption and I/O failure simulation"
		scenario.Duration = 4 * time.Minute
		scenario.Steps = as.createDiskChaosSteps()
		
	case ScenarioTypeByzantine:
		scenario.Name = "Byzantine Fault Test"
		scenario.Description = "Byzantine failure tolerance testing"
		scenario.Duration = 6 * time.Minute
		scenario.Steps = as.createByzantineSteps()
		
	case ScenarioTypePartitionTest:
		scenario.Name = "Partition Tolerance Test"
		scenario.Description = "CAP theorem partition tolerance validation"
		scenario.Duration = 8 * time.Minute
		scenario.Steps = as.createPartitionToleranceSteps()
		
	case ScenarioTypeRecoveryTest:
		scenario.Name = "Recovery Time Test"
		scenario.Description = "System recovery time measurement"
		scenario.Duration = 5 * time.Minute
		scenario.Steps = as.createRecoveryTestSteps()
		
	case ScenarioTypeCombined:
		scenario.Name = "Combined Chaos Test"
		scenario.Description = "Multiple concurrent failure modes"
		scenario.Duration = 10 * time.Minute
		scenario.Steps = as.createCombinedChaosSteps()
	}
	
	return scenario
}

// createBasicChaosSteps creates steps for basic chaos testing
func (as *AutomatedScenarios) createBasicChaosSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Parameters: map[string]interface{}{
				"duration": "30s",
			},
			Duration: 30 * time.Second,
		},
		{
			ID:   "single_partition",
			Name: "Create Network Partition",
			Type: "network_partition",
			Parameters: map[string]interface{}{
				"target_nodes": []string{"node1", "node2"},
				"duration":     "60s",
			},
			Duration: 60 * time.Second,
		},
		{
			ID:   "recovery",
			Name: "Measure Recovery",
			Type: "recovery",
			Parameters: map[string]interface{}{
				"timeout": "60s",
			},
			Duration: 60 * time.Second,
		},
	}
}

// createNetworkChaosSteps creates steps for network chaos testing
func (as *AutomatedScenarios) createNetworkChaosSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 30 * time.Second,
		},
		{
			ID:   "split_brain",
			Name: "Create Split-Brain Partition",
			Type: "network_partition",
			Parameters: map[string]interface{}{
				"type": "split",
				"groups": [][]string{
					{"node1", "node2"},
					{"node3", "node4", "node5"},
				},
			},
			Duration: 90 * time.Second,
		},
		{
			ID:   "isolation",
			Name: "Isolate Minority Nodes",
			Type: "network_partition",
			Parameters: map[string]interface{}{
				"type": "isolation",
				"target_nodes": []string{"node1"},
			},
			Duration: 60 * time.Second,
		},
		{
			ID:   "packet_loss",
			Name: "Simulate Packet Loss",
			Type: "network_chaos",
			Parameters: map[string]interface{}{
				"type": "packet_loss",
				"loss_rate": 10.0,
			},
			Duration: 45 * time.Second,
		},
		{
			ID:   "full_recovery",
			Name: "Full Network Recovery",
			Type: "recovery",
			Duration: 60 * time.Second,
		},
	}
}

// createNodeChaosSteps creates steps for node chaos testing
func (as *AutomatedScenarios) createNodeChaosSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 30 * time.Second,
		},
		{
			ID:   "kill_follower",
			Name: "Kill Follower Node",
			Type: "kill_node",
			Parameters: map[string]interface{}{
				"target":  "follower",
				"method":  "SIGKILL",
				"duration": "90s",
			},
			Duration: 90 * time.Second,
		},
		{
			ID:   "kill_leader",
			Name: "Kill Leader Node",
			Type: "kill_node",
			Parameters: map[string]interface{}{
				"target":  "leader",
				"method":  "SIGTERM",
				"duration": "60s",
			},
			Duration: 60 * time.Second,
		},
		{
			ID:   "recovery_measurement",
			Name: "Measure Recovery Times",
			Type: "recovery",
			Duration: 60 * time.Second,
		},
	}
}

// createDiskChaosSteps creates steps for disk chaos testing
func (as *AutomatedScenarios) createDiskChaosSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 30 * time.Second,
		},
		{
			ID:   "corrupt_raft_log",
			Name: "Corrupt Raft Log",
			Type: "disk_corruption",
			Parameters: map[string]interface{}{
				"type": "random",
				"target": "raft_log",
				"size": 1024,
			},
			Duration: 45 * time.Second,
		},
		{
			ID:   "disk_full",
			Name: "Simulate Disk Full",
			Type: "disk_corruption",
			Parameters: map[string]interface{}{
				"type": "disk_full",
				"size": 100 * 1024 * 1024, // 100MB
			},
			Duration: 60 * time.Second,
		},
		{
			ID:   "slow_io",
			Name: "Simulate Slow I/O",
			Type: "disk_corruption",
			Parameters: map[string]interface{}{
				"type": "slow_io",
				"latency": "100ms",
			},
			Duration: 90 * time.Second,
		},
		{
			ID:   "disk_recovery",
			Name: "Disk Recovery",
			Type: "recovery",
			Duration: 60 * time.Second,
		},
	}
}

// createByzantineSteps creates steps for Byzantine fault testing
func (as *AutomatedScenarios) createByzantineSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 45 * time.Second,
		},
		{
			ID:   "byzantine_node",
			Name: "Create Byzantine Node",
			Type: "byzantine_fault",
			Parameters: map[string]interface{}{
				"behavior": "conflicting_votes",
				"target":   "node2",
			},
			Duration: 120 * time.Second,
		},
		{
			ID:   "byzantine_leader",
			Name: "Byzantine Leader",
			Type: "byzantine_fault",
			Parameters: map[string]interface{}{
				"behavior": "invalid_proposals",
				"target":   "leader",
			},
			Duration: 90 * time.Second,
		},
		{
			ID:   "consistency_check",
			Name: "Check Consistency",
			Type: "consistency_validation",
			Duration: 60 * time.Second,
		},
		{
			ID:   "byzantine_recovery",
			Name: "Byzantine Recovery",
			Type: "recovery",
			Duration: 90 * time.Second,
		},
	}
}

// createPartitionToleranceSteps creates steps for partition tolerance testing
func (as *AutomatedScenarios) createPartitionToleranceSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 60 * time.Second,
		},
		{
			ID:   "majority_partition",
			Name: "Majority Partition",
			Type: "network_partition",
			Parameters: map[string]interface{}{
				"type": "majority",
				"groups": [][]string{
					{"node1", "node2", "node3"},
					{"node4", "node5"},
				},
			},
			Duration: 180 * time.Second,
		},
		{
			ID:   "minority_partition",
			Name: "Minority Partition",
			Type: "network_partition",
			Parameters: map[string]interface{}{
				"type": "minority",
				"groups": [][]string{
					{"node1", "node2"},
					{"node3", "node4", "node5"},
				},
			},
			Duration: 120 * time.Second,
		},
		{
			ID:   "availability_test",
			Name: "Test Availability",
			Type: "availability_test",
			Duration: 90 * time.Second,
		},
		{
			ID:   "consistency_test",
			Name: "Test Consistency",
			Type: "consistency_validation",
			Duration: 90 * time.Second,
		},
	}
}

// createRecoveryTestSteps creates steps for recovery time testing
func (as *AutomatedScenarios) createRecoveryTestSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 30 * time.Second,
		},
		{
			ID:   "rapid_failures",
			Name: "Rapid Failure Injection",
			Type: "combined_chaos",
			Parameters: map[string]interface{}{
				"failures": []string{"network", "node", "disk"},
				"interval": "10s",
			},
			Duration: 120 * time.Second,
		},
		{
			ID:   "recovery_measurement",
			Name: "Detailed Recovery Measurement",
			Type: "recovery",
			Parameters: map[string]interface{}{
				"measure_all": true,
				"timeout":     "180s",
			},
			Duration: 180 * time.Second,
		},
	}
}

// createCombinedChaosSteps creates steps for combined chaos testing
func (as *AutomatedScenarios) createCombinedChaosSteps() []*ScenarioStep {
	return []*ScenarioStep{
		{
			ID:   "baseline",
			Name: "Establish Baseline",
			Type: "baseline",
			Duration: 60 * time.Second,
		},
		{
			ID:   "network_and_node",
			Name: "Network Partition + Node Failure",
			Type: "combined_chaos",
			Parameters: map[string]interface{}{
				"network_partition": map[string]interface{}{
					"target_nodes": []string{"node1", "node2"},
				},
				"kill_node": map[string]interface{}{
					"target": "node3",
					"method": "SIGKILL",
				},
			},
			Duration: 180 * time.Second,
		},
		{
			ID:   "full_chaos",
			Name: "All Failure Modes",
			Type: "combined_chaos",
			Parameters: map[string]interface{}{
				"enable_all": true,
				"intensity":  "high",
			},
			Duration: 240 * time.Second,
		},
		{
			ID:   "gradual_recovery",
			Name: "Gradual Recovery",
			Type: "recovery",
			Parameters: map[string]interface{}{
				"method": "gradual",
				"steps":  3,
			},
			Duration: 180 * time.Second,
		},
	}
}

// executeScenario executes a specific scenario
func (as *AutomatedScenarios) executeScenario(ctx context.Context, scenario *Scenario) {
	as.mu.Lock()
	as.scenarios[scenario.ID] = scenario
	as.mu.Unlock()
	
	as.logger.Printf("Executing scenario %s: %s", scenario.ID, scenario.Name)
	
	scenario.Status = ScenarioStatusRunning
	scenario.StartTime = time.Now()
	
	// Execute each step
	for _, step := range scenario.Steps {
		if err := as.executeStep(ctx, scenario, step); err != nil {
			as.logger.Printf("Step %s failed: %v", step.ID, err)
			step.Status = "failed"
			step.Error = err.Error()
			scenario.Results.FailedSteps++
			
			if !as.config.SafetyMode {
				// Continue with next step in non-safety mode
				continue
			} else {
				// Abort scenario in safety mode
				scenario.Status = ScenarioStatusFailed
				break
			}
		} else {
			step.Status = "completed"
			scenario.Results.CompletedSteps++
		}
	}
	
	// Finalize scenario
	now := time.Now()
	scenario.EndTime = &now
	scenario.Duration = now.Sub(scenario.StartTime)
	
	if scenario.Status == ScenarioStatusRunning {
		scenario.Status = ScenarioStatusCompleted
		scenario.Results.Success = true
	}
	
	scenario.Results.TotalSteps = len(scenario.Steps)
	
	// Generate observations and recommendations
	as.analyzeScenarioResults(scenario)
	
	as.logger.Printf("Scenario %s completed: %s", scenario.ID, scenario.Status)
}

// executeStep executes a single scenario step
func (as *AutomatedScenarios) executeStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	as.logger.Printf("Executing step %s: %s", step.ID, step.Name)
	
	step.StartTime = time.Now()
	step.Status = "running"
	
	stepCtx, cancel := context.WithTimeout(ctx, step.Duration)
	defer cancel()
	
	switch step.Type {
	case "baseline":
		return as.establishBaseline(stepCtx, step)
	case "network_partition":
		return as.executeNetworkStep(stepCtx, scenario, step)
	case "kill_node":
		return as.executeNodeStep(stepCtx, scenario, step)
	case "disk_corruption":
		return as.executeDiskStep(stepCtx, scenario, step)
	case "byzantine_fault":
		return as.executeByzantineStep(stepCtx, scenario, step)
	case "recovery":
		return as.executeRecoveryStep(stepCtx, scenario, step)
	case "consistency_validation":
		return as.executeConsistencyStep(stepCtx, scenario, step)
	case "combined_chaos":
		return as.executeCombinedStep(stepCtx, scenario, step)
	default:
		return fmt.Errorf("unknown step type: %s", step.Type)
	}
}

// establishBaseline establishes a performance baseline
func (as *AutomatedScenarios) establishBaseline(ctx context.Context, step *ScenarioStep) error {
	as.logger.Printf("Establishing baseline for %v", step.Duration)
	
	// In a real implementation, this would:
	// - Measure normal system performance
	// - Record baseline metrics
	// - Verify system health
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(step.Duration):
		return nil
	}
}

// executeNetworkStep executes network-related chaos steps
func (as *AutomatedScenarios) executeNetworkStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	targetNodes := []string{"node1", "node2"} // Default
	if nodes, ok := step.Parameters["target_nodes"].([]string); ok {
		targetNodes = nodes
	}
	
	duration := step.Duration
	if d, ok := step.Parameters["duration"].(string); ok {
		if parsed, err := time.ParseDuration(d); err == nil {
			duration = parsed
		}
	}
	
	// Create network partition experiment
	expParams := map[string]interface{}{
		"target_nodes": targetNodes,
	}
	
	exp, err := as.framework.RunExperiment(ctx, ExperimentNetworkPartition, expParams)
	if err != nil {
		return fmt.Errorf("failed to start network experiment: %w", err)
	}
	
	scenario.Experiments = append(scenario.Experiments, exp.ID)
	
	// Wait for duration
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

// executeNodeStep executes node failure steps
func (as *AutomatedScenarios) executeNodeStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	// Similar implementation for node chaos
	exp, err := as.framework.RunExperiment(ctx, ExperimentNodeFailure, step.Parameters)
	if err != nil {
		return fmt.Errorf("failed to start node experiment: %w", err)
	}
	
	scenario.Experiments = append(scenario.Experiments, exp.ID)
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(step.Duration):
		return nil
	}
}

// executeDiskStep executes disk corruption steps
func (as *AutomatedScenarios) executeDiskStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	exp, err := as.framework.RunExperiment(ctx, ExperimentDiskCorruption, step.Parameters)
	if err != nil {
		return fmt.Errorf("failed to start disk experiment: %w", err)
	}
	
	scenario.Experiments = append(scenario.Experiments, exp.ID)
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(step.Duration):
		return nil
	}
}

// executeByzantineStep executes Byzantine fault steps
func (as *AutomatedScenarios) executeByzantineStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	exp, err := as.framework.RunExperiment(ctx, ExperimentByzantineFault, step.Parameters)
	if err != nil {
		return fmt.Errorf("failed to start Byzantine experiment: %w", err)
	}
	
	scenario.Experiments = append(scenario.Experiments, exp.ID)
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(step.Duration):
		return nil
	}
}

// executeRecoveryStep executes recovery measurement steps
func (as *AutomatedScenarios) executeRecoveryStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	as.logger.Printf("Measuring recovery for scenario %s", scenario.ID)
	
	// Measure recovery time
	startTime := time.Now()
	
	// Wait for system to recover
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(step.Duration):
		recoveryTime := time.Since(startTime)
		scenario.Results.TotalRecoveryTime += recoveryTime
		if recoveryTime > scenario.Results.MaxRecoveryTime {
			scenario.Results.MaxRecoveryTime = recoveryTime
		}
		return nil
	}
}

// executeConsistencyStep executes consistency validation steps
func (as *AutomatedScenarios) executeConsistencyStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	as.logger.Printf("Validating consistency for scenario %s", scenario.ID)
	
	// In a real implementation, this would:
	// - Check data consistency across nodes
	// - Validate linearizability
	// - Check for split-brain scenarios
	
	// Simulate consistency check
	time.Sleep(step.Duration)
	
	// Simulate finding some violations (10% chance)
	if rand.Float64() < 0.1 {
		scenario.Results.ConsistencyViolations++
	}
	
	return nil
}

// executeCombinedStep executes combined chaos steps
func (as *AutomatedScenarios) executeCombinedStep(ctx context.Context, scenario *Scenario, step *ScenarioStep) error {
	exp, err := as.framework.RunExperiment(ctx, ExperimentCombined, step.Parameters)
	if err != nil {
		return fmt.Errorf("failed to start combined experiment: %w", err)
	}
	
	scenario.Experiments = append(scenario.Experiments, exp.ID)
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(step.Duration):
		return nil
	}
}

// analyzeScenarioResults analyzes scenario results and generates recommendations
func (as *AutomatedScenarios) analyzeScenarioResults(scenario *Scenario) {
	// Generate observations
	if scenario.Results.ConsistencyViolations > 0 {
		scenario.Results.Observations = append(scenario.Results.Observations,
			fmt.Sprintf("Detected %d consistency violations", scenario.Results.ConsistencyViolations))
	}
	
	if scenario.Results.MaxRecoveryTime > 2*time.Minute {
		scenario.Results.Observations = append(scenario.Results.Observations,
			fmt.Sprintf("High recovery time detected: %v", scenario.Results.MaxRecoveryTime))
	}
	
	// Generate recommendations
	if scenario.Results.FailedSteps > 0 {
		scenario.Results.Recommendations = append(scenario.Results.Recommendations,
			"Review failed steps and improve error handling")
	}
	
	if scenario.Results.ConsistencyViolations > 0 {
		scenario.Results.Recommendations = append(scenario.Results.Recommendations,
			"Strengthen consistency mechanisms and validation")
	}
	
	if scenario.Results.MaxRecoveryTime > 2*time.Minute {
		scenario.Results.Recommendations = append(scenario.Results.Recommendations,
			"Optimize failure detection and recovery procedures")
	}
}

// getRunningScenarioCount returns the number of currently running scenarios
func (as *AutomatedScenarios) getRunningScenarioCount() int {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	count := 0
	for _, scenario := range as.scenarios {
		if scenario.Status == ScenarioStatusRunning {
			count++
		}
	}
	
	return count
}

// GetScenario returns information about a specific scenario
func (as *AutomatedScenarios) GetScenario(scenarioID string) (*Scenario, error) {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	scenario, exists := as.scenarios[scenarioID]
	if !exists {
		return nil, fmt.Errorf("scenario %s not found", scenarioID)
	}
	
	// Return a copy
	scenarioCopy := *scenario
	return &scenarioCopy, nil
}

// GetAllScenarios returns all scenarios
func (as *AutomatedScenarios) GetAllScenarios() map[string]*Scenario {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	result := make(map[string]*Scenario)
	for scenarioID, scenario := range as.scenarios {
		scenarioCopy := *scenario
		result[scenarioID] = &scenarioCopy
	}
	
	return result
}

// GetScenarioStats returns scenario statistics
func (as *AutomatedScenarios) GetScenarioStats() ScenarioStats {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	stats := ScenarioStats{
		IsRunning:         as.isRunning,
		TotalScenarios:    len(as.scenarios),
		RunningScenarios:  0,
		ScenariosByType:   make(map[ScenarioType]int),
		ScenariosByStatus: make(map[ScenarioStatus]int),
	}
	
	for _, scenario := range as.scenarios {
		stats.ScenariosByType[scenario.Type]++
		stats.ScenariosByStatus[scenario.Status]++
		
		if scenario.Status == ScenarioStatusRunning {
			stats.RunningScenarios++
		}
	}
	
	return stats
}

// ScenarioStats contains scenario statistics
type ScenarioStats struct {
	IsRunning         bool                           `json:"is_running"`
	TotalScenarios    int                            `json:"total_scenarios"`
	RunningScenarios  int                            `json:"running_scenarios"`
	ScenariosByType   map[ScenarioType]int           `json:"scenarios_by_type"`
	ScenariosByStatus map[ScenarioStatus]int         `json:"scenarios_by_status"`
}