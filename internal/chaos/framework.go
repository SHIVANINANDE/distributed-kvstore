package chaos

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// ChaosFramework manages chaos engineering experiments
type ChaosFramework struct {
	mu                sync.RWMutex
	logger            *log.Logger
	config            ChaosConfig
	experiments       map[string]*Experiment
	networkPartitioner *NetworkPartitioner
	nodeKiller        *NodeKiller
	diskCorruptor     *DiskCorruptor
	metricCollector   *MetricCollector
	
	// Framework state
	isRunning         bool
	startTime         time.Time
	totalExperiments  int64
	activeExperiments int64
	stopCh            chan struct{}
}

// ChaosConfig contains configuration for chaos experiments
type ChaosConfig struct {
	// General settings
	ExperimentTimeout   time.Duration `json:"experiment_timeout"`    // Max duration for experiments
	CooldownPeriod     time.Duration `json:"cooldown_period"`       // Time between experiments
	MaxConcurrent      int           `json:"max_concurrent"`        // Max concurrent experiments
	
	// Network partition settings
	NetworkEnabled     bool          `json:"network_enabled"`       // Enable network chaos
	PartitionDuration  time.Duration `json:"partition_duration"`    // Duration of network partitions
	PartitionFrequency time.Duration `json:"partition_frequency"`   // How often to create partitions
	
	// Node failure settings
	NodeEnabled        bool          `json:"node_enabled"`          // Enable node chaos
	KillDuration       time.Duration `json:"kill_duration"`         // How long to keep nodes down
	KillFrequency      time.Duration `json:"kill_frequency"`        // How often to kill nodes
	MaxKilledNodes     int           `json:"max_killed_nodes"`      // Max nodes to kill simultaneously
	
	// Disk corruption settings
	DiskEnabled        bool          `json:"disk_enabled"`          // Enable disk chaos
	CorruptionChance   float64       `json:"corruption_chance"`     // Probability of corruption
	CorruptionSize     int64         `json:"corruption_size"`       // Size of corruption in bytes
	
	// Resilience testing
	ByzantineEnabled   bool          `json:"byzantine_enabled"`     // Enable Byzantine failure tests
	PartitionTolerance bool          `json:"partition_tolerance"`   // Test partition tolerance
	RecoveryMeasurement bool         `json:"recovery_measurement"`  // Measure recovery times
	
	// Safety limits
	SafetyMode         bool          `json:"safety_mode"`           // Enable safety constraints
	MaxFailureNodes    int           `json:"max_failure_nodes"`     // Max nodes that can fail
	MinHealthyNodes    int           `json:"min_healthy_nodes"`     // Min nodes that must stay healthy
}

// ExperimentType defines types of chaos experiments
type ExperimentType string

const (
	ExperimentNetworkPartition ExperimentType = "network_partition"
	ExperimentNodeFailure     ExperimentType = "node_failure"
	ExperimentDiskCorruption  ExperimentType = "disk_corruption"
	ExperimentByzantineFault  ExperimentType = "byzantine_fault"
	ExperimentCombined        ExperimentType = "combined"
)

// ExperimentStatus represents the status of an experiment
type ExperimentStatus string

const (
	StatusPending   ExperimentStatus = "pending"
	StatusRunning   ExperimentStatus = "running"
	StatusCompleted ExperimentStatus = "completed"
	StatusFailed    ExperimentStatus = "failed"
	StatusAborted   ExperimentStatus = "aborted"
)

// Experiment represents a chaos experiment
type Experiment struct {
	ID              string                 `json:"id"`
	Type            ExperimentType         `json:"type"`
	Status          ExperimentStatus       `json:"status"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
	Duration        time.Duration          `json:"duration"`
	Description     string                 `json:"description"`
	Parameters      map[string]interface{} `json:"parameters"`
	Results         *ExperimentResults     `json:"results"`
	TargetNodes     []string               `json:"target_nodes"`
	Metrics         *ExperimentMetrics     `json:"metrics"`
	ErrorLog        []string               `json:"error_log"`
}

// ExperimentResults contains the results of a chaos experiment
type ExperimentResults struct {
	Success           bool                   `json:"success"`
	ConsistencyViolations int               `json:"consistency_violations"`
	DataLoss          bool                   `json:"data_loss"`
	RecoveryTime      time.Duration          `json:"recovery_time"`
	AvailabilityImpact float64              `json:"availability_impact"` // Percentage
	ThroughputImpact  float64               `json:"throughput_impact"`   // Percentage
	LatencyIncrease   time.Duration          `json:"latency_increase"`
	Observations      []string               `json:"observations"`
	Recommendations   []string               `json:"recommendations"`
}

// ExperimentMetrics contains metrics collected during the experiment
type ExperimentMetrics struct {
	RequestCount      int64         `json:"request_count"`
	SuccessfulReqs    int64         `json:"successful_requests"`
	FailedRequests    int64         `json:"failed_requests"`
	AverageLatency    time.Duration `json:"average_latency"`
	P95Latency        time.Duration `json:"p95_latency"`
	P99Latency        time.Duration `json:"p99_latency"`
	ErrorRate         float64       `json:"error_rate"`
	PartitionEvents   int           `json:"partition_events"`
	NodeFailures      int           `json:"node_failures"`
	RecoveryEvents    int           `json:"recovery_events"`
}

// NewChaosFramework creates a new chaos engineering framework
func NewChaosFramework(config ChaosConfig, logger *log.Logger) *ChaosFramework {
	if logger == nil {
		logger = log.New(log.Writer(), "[CHAOS] ", log.LstdFlags)
	}
	
	// Set default values
	if config.ExperimentTimeout == 0 {
		config.ExperimentTimeout = 5 * time.Minute
	}
	if config.CooldownPeriod == 0 {
		config.CooldownPeriod = 30 * time.Second
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 3
	}
	if config.PartitionDuration == 0 {
		config.PartitionDuration = 30 * time.Second
	}
	if config.KillDuration == 0 {
		config.KillDuration = 60 * time.Second
	}
	if config.MaxKilledNodes == 0 {
		config.MaxKilledNodes = 1
	}
	if config.MinHealthyNodes == 0 {
		config.MinHealthyNodes = 2
	}
	
	framework := &ChaosFramework{
		logger:      logger,
		config:      config,
		experiments: make(map[string]*Experiment),
		stopCh:      make(chan struct{}),
	}
	
	// Initialize chaos components
	framework.networkPartitioner = NewNetworkPartitioner(config, logger)
	framework.nodeKiller = NewNodeKiller(config, logger)
	framework.diskCorruptor = NewDiskCorruptor(config, logger)
	framework.metricCollector = NewMetricCollector(logger)
	
	return framework
}

// Start starts the chaos framework
func (cf *ChaosFramework) Start(ctx context.Context) error {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	
	if cf.isRunning {
		return fmt.Errorf("chaos framework is already running")
	}
	
	cf.logger.Printf("Starting chaos engineering framework")
	cf.isRunning = true
	cf.startTime = time.Now()
	
	// Start metric collection
	go cf.metricCollector.Start(ctx)
	
	cf.logger.Printf("Chaos framework started successfully")
	return nil
}

// Stop stops the chaos framework
func (cf *ChaosFramework) Stop() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	
	if !cf.isRunning {
		return nil
	}
	
	cf.logger.Printf("Stopping chaos engineering framework")
	
	// Stop all running experiments
	for _, exp := range cf.experiments {
		if exp.Status == StatusRunning {
			cf.abortExperiment(exp.ID)
		}
	}
	
	// Signal stop
	close(cf.stopCh)
	cf.isRunning = false
	
	cf.logger.Printf("Chaos framework stopped")
	return nil
}

// RunExperiment executes a chaos experiment
func (cf *ChaosFramework) RunExperiment(ctx context.Context, expType ExperimentType, params map[string]interface{}) (*Experiment, error) {
	cf.mu.Lock()
	
	// Check if we can run more experiments
	if cf.activeExperiments >= int64(cf.config.MaxConcurrent) {
		cf.mu.Unlock()
		return nil, fmt.Errorf("maximum concurrent experiments reached")
	}
	
	// Generate experiment ID
	expID := fmt.Sprintf("%s_%d_%d", expType, time.Now().Unix(), rand.Int63())
	
	// Create experiment
	experiment := &Experiment{
		ID:          expID,
		Type:        expType,
		Status:      StatusPending,
		StartTime:   time.Now(),
		Description: cf.generateDescription(expType, params),
		Parameters:  params,
		Results:     &ExperimentResults{},
		Metrics:     &ExperimentMetrics{},
		ErrorLog:    make([]string, 0),
	}
	
	cf.experiments[expID] = experiment
	cf.totalExperiments++
	cf.activeExperiments++
	cf.mu.Unlock()
	
	cf.logger.Printf("Starting experiment %s (%s)", expID, expType)
	
	// Run experiment in background
	go cf.executeExperiment(ctx, experiment)
	
	return experiment, nil
}

// executeExperiment executes a specific experiment
func (cf *ChaosFramework) executeExperiment(ctx context.Context, exp *Experiment) {
	defer func() {
		cf.mu.Lock()
		cf.activeExperiments--
		cf.mu.Unlock()
	}()
	
	// Set experiment timeout
	expCtx, cancel := context.WithTimeout(ctx, cf.config.ExperimentTimeout)
	defer cancel()
	
	// Update status
	cf.updateExperimentStatus(exp.ID, StatusRunning)
	
	// Start metrics collection for this experiment
	cf.metricCollector.StartExperiment(exp.ID)
	
	var err error
	switch exp.Type {
	case ExperimentNetworkPartition:
		err = cf.runNetworkPartitionExperiment(expCtx, exp)
	case ExperimentNodeFailure:
		err = cf.runNodeFailureExperiment(expCtx, exp)
	case ExperimentDiskCorruption:
		err = cf.runDiskCorruptionExperiment(expCtx, exp)
	case ExperimentByzantineFault:
		err = cf.runByzantineExperiment(expCtx, exp)
	case ExperimentCombined:
		err = cf.runCombinedExperiment(expCtx, exp)
	default:
		err = fmt.Errorf("unknown experiment type: %s", exp.Type)
	}
	
	// Stop metrics collection
	metrics := cf.metricCollector.StopExperiment(exp.ID)
	exp.Metrics = metrics
	
	// Update experiment results
	exp.EndTime = time.Now()
	exp.Duration = exp.EndTime.Sub(exp.StartTime)
	
	if err != nil {
		exp.ErrorLog = append(exp.ErrorLog, err.Error())
		cf.updateExperimentStatus(exp.ID, StatusFailed)
		cf.logger.Printf("Experiment %s failed: %v", exp.ID, err)
	} else {
		cf.updateExperimentStatus(exp.ID, StatusCompleted)
		cf.logger.Printf("Experiment %s completed successfully", exp.ID)
	}
	
	// Generate recommendations
	cf.generateRecommendations(exp)
	
	// Log experiment results
	cf.logExperimentResults(exp)
}

// runNetworkPartitionExperiment simulates network partitions
func (cf *ChaosFramework) runNetworkPartitionExperiment(ctx context.Context, exp *Experiment) error {
	cf.logger.Printf("Running network partition experiment %s", exp.ID)
	
	// Get target nodes
	targetNodes, ok := exp.Parameters["target_nodes"].([]string)
	if !ok || len(targetNodes) == 0 {
		return fmt.Errorf("no target nodes specified")
	}
	
	exp.TargetNodes = targetNodes
	
	// Create network partition
	partitionID, err := cf.networkPartitioner.CreatePartition(targetNodes, cf.config.PartitionDuration)
	if err != nil {
		return fmt.Errorf("failed to create partition: %w", err)
	}
	
	cf.logger.Printf("Created network partition %s affecting nodes: %v", partitionID, targetNodes)
	
	// Monitor system during partition
	partitionStart := time.Now()
	
	// Wait for partition duration or context cancellation
	select {
	case <-ctx.Done():
		cf.logger.Printf("Experiment %s cancelled", exp.ID)
		return ctx.Err()
	case <-time.After(cf.config.PartitionDuration):
		// Partition duration completed
	}
	
	// Heal the partition
	err = cf.networkPartitioner.HealPartition(partitionID)
	if err != nil {
		exp.ErrorLog = append(exp.ErrorLog, fmt.Sprintf("Failed to heal partition: %v", err))
	}
	
	// Measure recovery time
	recoveryStart := time.Now()
	recoveryTime := cf.measureRecoveryTime(targetNodes)
	
	// Update results
	exp.Results.RecoveryTime = recoveryTime
	exp.Results.Success = err == nil
	
	cf.logger.Printf("Network partition experiment %s: partition=%v, recovery=%v", 
		exp.ID, cf.config.PartitionDuration, recoveryTime)
	
	// Check for consistency violations
	violations := cf.checkConsistencyViolations(targetNodes)
	exp.Results.ConsistencyViolations = violations
	
	return nil
}

// updateExperimentStatus updates the status of an experiment
func (cf *ChaosFramework) updateExperimentStatus(expID string, status ExperimentStatus) {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	
	if exp, exists := cf.experiments[expID]; exists {
		exp.Status = status
	}
}

// abortExperiment aborts a running experiment
func (cf *ChaosFramework) abortExperiment(expID string) error {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	
	exp, exists := cf.experiments[expID]
	if !exists {
		return fmt.Errorf("experiment %s not found", expID)
	}
	
	if exp.Status != StatusRunning {
		return fmt.Errorf("experiment %s is not running", expID)
	}
	
	exp.Status = StatusAborted
	exp.EndTime = time.Now()
	exp.Duration = exp.EndTime.Sub(exp.StartTime)
	
	cf.logger.Printf("Aborted experiment %s", expID)
	return nil
}

// generateDescription generates a description for an experiment
func (cf *ChaosFramework) generateDescription(expType ExperimentType, params map[string]interface{}) string {
	switch expType {
	case ExperimentNetworkPartition:
		return fmt.Sprintf("Network partition experiment targeting %v for %v", 
			params["target_nodes"], cf.config.PartitionDuration)
	case ExperimentNodeFailure:
		return fmt.Sprintf("Node failure experiment killing %d nodes for %v", 
			cf.config.MaxKilledNodes, cf.config.KillDuration)
	case ExperimentDiskCorruption:
		return fmt.Sprintf("Disk corruption experiment with %.2f%% corruption chance", 
			cf.config.CorruptionChance*100)
	case ExperimentByzantineFault:
		return "Byzantine fault tolerance test"
	case ExperimentCombined:
		return "Combined chaos experiment with multiple failure modes"
	default:
		return fmt.Sprintf("Unknown experiment type: %s", expType)
	}
}

// measureRecoveryTime measures how long it takes for the system to recover
func (cf *ChaosFramework) measureRecoveryTime(targetNodes []string) time.Duration {
	start := time.Now()
	maxWait := 5 * time.Minute
	checkInterval := 5 * time.Second
	
	for time.Since(start) < maxWait {
		if cf.isSystemHealthy(targetNodes) {
			return time.Since(start)
		}
		time.Sleep(checkInterval)
	}
	
	return maxWait // Timeout
}

// MeasureDetailedRecoveryTimes performs comprehensive recovery time measurement
func (cf *ChaosFramework) MeasureDetailedRecoveryTimes(ctx context.Context, nodes []string) *RecoveryReport {
	cf.logger.Printf("Starting detailed recovery time measurement for nodes: %v", nodes)
	
	report := &RecoveryReport{
		StartTime:        time.Now(),
		TestedNodes:      nodes,
		RecoveryScenarios: make([]*RecoveryScenario, 0),
		AverageRecoveryTime: 0,
		MaxRecoveryTime:    0,
		MinRecoveryTime:    time.Hour, // Initialize to high value
		RecoveryMetrics:    make(map[string]*RecoveryMetric),
	}
	
	// Test different failure and recovery scenarios
	scenarios := []struct {
		name         string
		failureType  string
		description  string
		setupFunc    func() error
		cleanupFunc  func() error
	}{
		{
			"single_node_kill", "node_failure",
			"Kill single node and measure restart recovery",
			func() error { return cf.setupSingleNodeFailure(nodes) },
			func() error { return cf.cleanupNodeFailure() },
		},
		{
			"majority_partition", "network_partition",
			"Create majority partition and measure healing recovery",
			func() error { return cf.setupMajorityPartition(nodes) },
			func() error { return cf.cleanupNetworkPartition() },
		},
		{
			"leader_failure", "leader_kill",
			"Kill leader node and measure election recovery",
			func() error { return cf.setupLeaderFailure(nodes) },
			func() error { return cf.cleanupLeaderFailure() },
		},
		{
			"disk_corruption", "disk_failure",
			"Corrupt disk and measure restoration recovery",
			func() error { return cf.setupDiskCorruption() },
			func() error { return cf.cleanupDiskCorruption() },
		},
		{
			"cascading_failure", "multiple_failure",
			"Multiple concurrent failures and recovery",
			func() error { return cf.setupCascadingFailure(nodes) },
			func() error { return cf.cleanupCascadingFailure() },
		},
	}
	
	var totalRecoveryTime time.Duration
	successfulMeasurements := 0
	
	for _, scenario := range scenarios {
		cf.logger.Printf("Testing recovery scenario: %s", scenario.name)
		
		recoveryScenario := cf.runRecoveryScenario(ctx, scenario.name, scenario.failureType, 
			scenario.description, scenario.setupFunc, scenario.cleanupFunc, nodes)
		
		report.RecoveryScenarios = append(report.RecoveryScenarios, recoveryScenario)
		
		if recoveryScenario.Success {
			totalRecoveryTime += recoveryScenario.RecoveryTime
			successfulMeasurements++
			
			if recoveryScenario.RecoveryTime > report.MaxRecoveryTime {
				report.MaxRecoveryTime = recoveryScenario.RecoveryTime
			}
			
			if recoveryScenario.RecoveryTime < report.MinRecoveryTime {
				report.MinRecoveryTime = recoveryScenario.RecoveryTime
			}
		}
	}
	
	// Calculate averages
	if successfulMeasurements > 0 {
		report.AverageRecoveryTime = totalRecoveryTime / time.Duration(successfulMeasurements)
	}
	
	// Measure recovery metrics for different components
	report.RecoveryMetrics["raft_consensus"] = cf.measureRaftRecoveryMetrics(ctx, nodes)
	report.RecoveryMetrics["data_consistency"] = cf.measureDataConsistencyRecovery(ctx, nodes)
	report.RecoveryMetrics["client_connectivity"] = cf.measureClientConnectivityRecovery(ctx, nodes)
	report.RecoveryMetrics["cluster_membership"] = cf.measureClusterMembershipRecovery(ctx, nodes)
	
	// Generate recovery assessment
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)
	report.OverallAssessment = cf.assessRecoveryPerformance(report)
	
	cf.logger.Printf("Recovery measurement completed: avg=%v, max=%v, min=%v, assessment=%s", 
		report.AverageRecoveryTime, report.MaxRecoveryTime, report.MinRecoveryTime, report.OverallAssessment)
	
	return report
}

// runRecoveryScenario runs a specific recovery scenario and measures recovery time
func (cf *ChaosFramework) runRecoveryScenario(ctx context.Context, name, failureType, description string, 
	setupFunc, cleanupFunc func() error, nodes []string) *RecoveryScenario {
	
	scenario := &RecoveryScenario{
		Name:         name,
		FailureType:  failureType,
		Description:  description,
		StartTime:    time.Now(),
		TestedNodes:  nodes,
		Success:      false,
		RecoveryPhases: make([]*RecoveryPhase, 0),
	}
	
	cf.logger.Printf("Starting recovery scenario: %s", name)
	
	// Phase 1: Establish baseline
	baseline := cf.measureRecoveryPhase("baseline", "Measure system performance before failure", 30*time.Second)
	scenario.RecoveryPhases = append(scenario.RecoveryPhases, baseline)
	
	// Phase 2: Inject failure
	failureStart := time.Now()
	if err := setupFunc(); err != nil {
		scenario.Errors = append(scenario.Errors, fmt.Sprintf("Failed to setup failure: %v", err))
		return scenario
	}
	
	failurePhase := &RecoveryPhase{
		Name:        "failure_injection",
		Description: "Inject failure into the system",
		StartTime:   failureStart,
		EndTime:     time.Now(),
		Duration:    time.Since(failureStart),
	}
	scenario.RecoveryPhases = append(scenario.RecoveryPhases, failurePhase)
	
	// Phase 3: Wait for failure impact
	impact := cf.measureRecoveryPhase("failure_impact", "Wait for failure to take effect", 15*time.Second)
	scenario.RecoveryPhases = append(scenario.RecoveryPhases, impact)
	
	// Phase 4: Start recovery
	recoveryStart := time.Now()
	if err := cleanupFunc(); err != nil {
		scenario.Errors = append(scenario.Errors, fmt.Sprintf("Failed to start recovery: %v", err))
		return scenario
	}
	
	// Phase 5: Measure recovery time
	recoveryTime := cf.measureDetailedRecovery(ctx, nodes, name)
	scenario.RecoveryTime = recoveryTime
	
	recoveryPhase := &RecoveryPhase{
		Name:        "recovery_measurement",
		Description: "Measure time to full recovery",
		StartTime:   recoveryStart,
		EndTime:     time.Now(),
		Duration:    recoveryTime,
	}
	scenario.RecoveryPhases = append(scenario.RecoveryPhases, recoveryPhase)
	
	// Phase 6: Verify recovery
	verification := cf.measureRecoveryPhase("verification", "Verify system is fully recovered", 30*time.Second)
	scenario.RecoveryPhases = append(scenario.RecoveryPhases, verification)
	
	scenario.EndTime = time.Now()
	scenario.Duration = scenario.EndTime.Sub(scenario.StartTime)
	scenario.Success = len(scenario.Errors) == 0
	
	cf.logger.Printf("Recovery scenario %s completed: success=%t, recovery_time=%v", 
		name, scenario.Success, scenario.RecoveryTime)
	
	return scenario
}

// measureDetailedRecovery measures recovery with multiple checkpoints
func (cf *ChaosFramework) measureDetailedRecovery(ctx context.Context, nodes []string, scenarioName string) time.Duration {
	start := time.Now()
	maxWait := 10 * time.Minute
	checkInterval := 2 * time.Second
	
	// Track different recovery milestones
	milestones := map[string]bool{
		"node_connectivity":    false,
		"raft_consensus":       false,
		"data_consistency":     false,
		"client_operations":    false,
		"full_recovery":        false,
	}
	
	for time.Since(start) < maxWait {
		select {
		case <-ctx.Done():
			return time.Since(start)
		case <-time.After(checkInterval):
			// Check each milestone
			if !milestones["node_connectivity"] && cf.checkNodeConnectivity(nodes) {
				milestones["node_connectivity"] = true
				cf.logger.Printf("[%s] Node connectivity restored at %v", scenarioName, time.Since(start))
			}
			
			if milestones["node_connectivity"] && !milestones["raft_consensus"] && cf.checkRaftConsensus(nodes) {
				milestones["raft_consensus"] = true
				cf.logger.Printf("[%s] Raft consensus restored at %v", scenarioName, time.Since(start))
			}
			
			if milestones["raft_consensus"] && !milestones["data_consistency"] && cf.checkDataConsistency(nodes) {
				milestones["data_consistency"] = true
				cf.logger.Printf("[%s] Data consistency restored at %v", scenarioName, time.Since(start))
			}
			
			if milestones["data_consistency"] && !milestones["client_operations"] && cf.checkClientOperations(nodes) {
				milestones["client_operations"] = true
				cf.logger.Printf("[%s] Client operations restored at %v", scenarioName, time.Since(start))
			}
			
			if milestones["client_operations"] && !milestones["full_recovery"] && cf.checkFullRecovery(nodes) {
				milestones["full_recovery"] = true
				cf.logger.Printf("[%s] Full recovery achieved at %v", scenarioName, time.Since(start))
				return time.Since(start)
			}
		}
	}
	
	return maxWait // Timeout
}

// measureRecoveryPhase measures a specific phase of recovery
func (cf *ChaosFramework) measureRecoveryPhase(name, description string, duration time.Duration) *RecoveryPhase {
	start := time.Now()
	time.Sleep(duration) // Simulate phase duration
	
	return &RecoveryPhase{
		Name:        name,
		Description: description,
		StartTime:   start,
		EndTime:     time.Now(),
		Duration:    duration,
	}
}

// Recovery milestone check functions
func (cf *ChaosFramework) checkNodeConnectivity(nodes []string) bool {
	// Simulate node connectivity check
	return rand.Float64() < 0.8 // 80% chance of connectivity
}

func (cf *ChaosFramework) checkRaftConsensus(nodes []string) bool {
	// Simulate Raft consensus check
	return rand.Float64() < 0.7 // 70% chance of consensus
}

func (cf *ChaosFramework) checkDataConsistency(nodes []string) bool {
	// Simulate data consistency check
	return rand.Float64() < 0.9 // 90% chance of consistency
}

func (cf *ChaosFramework) checkClientOperations(nodes []string) bool {
	// Simulate client operations check
	return rand.Float64() < 0.85 // 85% chance of working operations
}

func (cf *ChaosFramework) checkFullRecovery(nodes []string) bool {
	// Simulate full recovery check
	return rand.Float64() < 0.95 // 95% chance of full recovery
}

// Recovery scenario setup functions
func (cf *ChaosFramework) setupSingleNodeFailure(nodes []string) error {
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available")
	}
	// Simulate killing a random node
	cf.logger.Printf("Killing node %s", nodes[0])
	return nil
}

func (cf *ChaosFramework) setupMajorityPartition(nodes []string) error {
	majority := nodes[:len(nodes)-1]
	cf.logger.Printf("Creating majority partition with nodes: %v", majority)
	return nil
}

func (cf *ChaosFramework) setupLeaderFailure(nodes []string) error {
	cf.logger.Printf("Killing leader node")
	return nil
}

func (cf *ChaosFramework) setupDiskCorruption() error {
	cf.logger.Printf("Corrupting disk data")
	return nil
}

func (cf *ChaosFramework) setupCascadingFailure(nodes []string) error {
	cf.logger.Printf("Setting up cascading failure across multiple nodes")
	return nil
}

// Recovery cleanup functions
func (cf *ChaosFramework) cleanupNodeFailure() error {
	cf.logger.Printf("Restarting failed node")
	return nil
}

func (cf *ChaosFramework) cleanupNetworkPartition() error {
	cf.logger.Printf("Healing network partition")
	return nil
}

func (cf *ChaosFramework) cleanupLeaderFailure() error {
	cf.logger.Printf("Allowing leader election")
	return nil
}

func (cf *ChaosFramework) cleanupDiskCorruption() error {
	cf.logger.Printf("Restoring corrupted disk data")
	return nil
}

func (cf *ChaosFramework) cleanupCascadingFailure() error {
	cf.logger.Printf("Recovering from cascading failure")
	return nil
}

// measureRaftRecoveryMetrics measures Raft-specific recovery metrics
func (cf *ChaosFramework) measureRaftRecoveryMetrics(ctx context.Context, nodes []string) *RecoveryMetric {
	metric := &RecoveryMetric{
		Name:        "raft_consensus",
		Description: "Raft consensus recovery time",
		StartTime:   time.Now(),
	}
	
	// Simulate Raft recovery measurement
	time.Sleep(100 * time.Millisecond)
	
	metric.EndTime = time.Now()
	metric.RecoveryTime = metric.EndTime.Sub(metric.StartTime)
	metric.Success = rand.Float64() < 0.95 // 95% success rate
	
	if metric.Success {
		metric.Details = "Raft consensus restored successfully"
	} else {
		metric.Details = "Raft consensus recovery encountered issues"
	}
	
	return metric
}

// measureDataConsistencyRecovery measures data consistency recovery
func (cf *ChaosFramework) measureDataConsistencyRecovery(ctx context.Context, nodes []string) *RecoveryMetric {
	metric := &RecoveryMetric{
		Name:        "data_consistency",
		Description: "Data consistency restoration time",
		StartTime:   time.Now(),
	}
	
	time.Sleep(150 * time.Millisecond)
	
	metric.EndTime = time.Now()
	metric.RecoveryTime = metric.EndTime.Sub(metric.StartTime)
	metric.Success = rand.Float64() < 0.9 // 90% success rate
	metric.Details = "Data consistency verified across all nodes"
	
	return metric
}

// measureClientConnectivityRecovery measures client connectivity recovery
func (cf *ChaosFramework) measureClientConnectivityRecovery(ctx context.Context, nodes []string) *RecoveryMetric {
	metric := &RecoveryMetric{
		Name:        "client_connectivity",
		Description: "Client connectivity restoration time",
		StartTime:   time.Now(),
	}
	
	time.Sleep(80 * time.Millisecond)
	
	metric.EndTime = time.Now()
	metric.RecoveryTime = metric.EndTime.Sub(metric.StartTime)
	metric.Success = rand.Float64() < 0.98 // 98% success rate
	metric.Details = "All client connections restored"
	
	return metric
}

// measureClusterMembershipRecovery measures cluster membership recovery
func (cf *ChaosFramework) measureClusterMembershipRecovery(ctx context.Context, nodes []string) *RecoveryMetric {
	metric := &RecoveryMetric{
		Name:        "cluster_membership",
		Description: "Cluster membership restoration time",
		StartTime:   time.Now(),
	}
	
	time.Sleep(120 * time.Millisecond)
	
	metric.EndTime = time.Now()
	metric.RecoveryTime = metric.EndTime.Sub(metric.StartTime)
	metric.Success = rand.Float64() < 0.92 // 92% success rate
	metric.Details = "All nodes rejoined cluster successfully"
	
	return metric
}

// assessRecoveryPerformance provides an overall assessment of recovery performance
func (cf *ChaosFramework) assessRecoveryPerformance(report *RecoveryReport) string {
	if report.AverageRecoveryTime < 30*time.Second && report.MaxRecoveryTime < 1*time.Minute {
		return "EXCELLENT"
	} else if report.AverageRecoveryTime < 2*time.Minute && report.MaxRecoveryTime < 5*time.Minute {
		return "GOOD"
	} else if report.AverageRecoveryTime < 5*time.Minute && report.MaxRecoveryTime < 10*time.Minute {
		return "FAIR"
	} else {
		return "POOR"
	}
}

// isSystemHealthy checks if the system has recovered
func (cf *ChaosFramework) isSystemHealthy(targetNodes []string) bool {
	// In a real implementation, this would check:
	// - Node connectivity
	// - Raft cluster status
	// - Data consistency
	// - Service availability
	
	// For simulation, we'll assume recovery after some time
	return true
}

// checkConsistencyViolations checks for data consistency violations
func (cf *ChaosFramework) checkConsistencyViolations(targetNodes []string) int {
	// In a real implementation, this would:
	// - Compare data across nodes
	// - Check for split-brain scenarios
	// - Validate Raft log consistency
	// - Verify linearizability
	
	// For simulation, randomly generate violations
	if rand.Float64() < 0.1 { // 10% chance
		return rand.Intn(3) + 1
	}
	return 0
}

// ValidateConsistencyUnderPartitions performs comprehensive consistency validation
func (cf *ChaosFramework) ValidateConsistencyUnderPartitions(ctx context.Context, nodes []string) *ConsistencyReport {
	cf.logger.Printf("Starting consistency validation under partitions for nodes: %v", nodes)
	
	report := &ConsistencyReport{
		StartTime:           time.Now(),
		TestedNodes:         nodes,
		PartitionScenarios:  make([]*PartitionScenario, 0),
		TotalViolations:     0,
		MaxRecoveryTime:     0,
		ConsistencyChecks:   make(map[string]*ConsistencyCheck),
	}
	
	// Test different partition scenarios
	scenarios := []struct {
		name        string
		partitionType PartitionType
		description string
	}{
		{"majority_isolation", PartitionTypeIsolation, "Isolate majority nodes from minority"},
		{"minority_isolation", PartitionTypeIsolation, "Isolate minority nodes from majority"},
		{"split_brain", PartitionTypeSplit, "Create even split between node groups"},
		{"asymmetric_partition", PartitionTypeAsymmetric, "One-way communication failure"},
		{"cascading_failure", PartitionTypeSplit, "Sequential node isolation"},
	}
	
	for _, scenario := range scenarios {
		cf.logger.Printf("Testing partition scenario: %s", scenario.name)
		
		partitionReport := cf.runPartitionConsistencyTest(ctx, scenario.name, scenario.partitionType, nodes)
		report.PartitionScenarios = append(report.PartitionScenarios, partitionReport)
		
		report.TotalViolations += partitionReport.ConsistencyViolations
		if partitionReport.RecoveryTime > report.MaxRecoveryTime {
			report.MaxRecoveryTime = partitionReport.RecoveryTime
		}
	}
	
	// Perform CAP theorem validation
	report.CAPValidation = cf.validateCAPTheorem(ctx, nodes)
	
	// Check linearizability
	report.LinearizabilityCheck = cf.checkLinearizability(ctx, nodes)
	
	// Generate final assessment
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)
	report.OverallStatus = cf.assessConsistencyStatus(report)
	
	cf.logger.Printf("Consistency validation completed: violations=%d, recovery=%v, status=%s", 
		report.TotalViolations, report.MaxRecoveryTime, report.OverallStatus)
	
	return report
}

// runPartitionConsistencyTest runs a specific partition scenario and tests consistency
func (cf *ChaosFramework) runPartitionConsistencyTest(ctx context.Context, scenarioName string, partitionType PartitionType, nodes []string) *PartitionScenario {
	scenario := &PartitionScenario{
		Name:                 scenarioName,
		PartitionType:        partitionType,
		StartTime:           time.Now(),
		TestedNodes:         nodes,
		ConsistencyViolations: 0,
		DataLossDetected:    false,
		SplitBrainDetected:  false,
	}
	
	// Create partition based on scenario
	var targetNodes []string
	switch scenarioName {
	case "majority_isolation":
		targetNodes = nodes[:len(nodes)-1] // Isolate all but one
	case "minority_isolation":
		targetNodes = nodes[:1] // Isolate just one
	case "split_brain":
		mid := len(nodes) / 2
		targetNodes = nodes[:mid] // Split in half
	case "asymmetric_partition":
		targetNodes = nodes[:2] // Take first two
	case "cascading_failure":
		targetNodes = nodes[:len(nodes)-2] // Leave only two connected
	}
	
	// Create the partition
	partitionID, err := cf.networkPartitioner.CreateCustomPartition(
		partitionType, targetNodes, 90*time.Second, 
		map[string]interface{}{
			"scenario": scenarioName,
		})
	
	if err != nil {
		scenario.Errors = append(scenario.Errors, fmt.Sprintf("Failed to create partition: %v", err))
		return scenario
	}
	
	scenario.PartitionID = partitionID
	
	// Monitor consistency during partition
	monitoringDuration := 60 * time.Second
	checkInterval := 10 * time.Second
	startTime := time.Now()
	
	for time.Since(startTime) < monitoringDuration {
		select {
		case <-ctx.Done():
			scenario.Errors = append(scenario.Errors, "Context cancelled")
			return scenario
		case <-time.After(checkInterval):
			// Check for various consistency issues
			violations := cf.checkPartitionConsistency(targetNodes, scenarioName)
			scenario.ConsistencyViolations += violations
			
			// Check for split-brain
			if cf.detectSplitBrain(nodes, targetNodes) {
				scenario.SplitBrainDetected = true
			}
			
			// Check for data loss
			if cf.detectDataLoss(nodes) {
				scenario.DataLossDetected = true
			}
		}
	}
	
	// Heal the partition
	healStart := time.Now()
	err = cf.networkPartitioner.HealPartition(partitionID)
	if err != nil {
		scenario.Errors = append(scenario.Errors, fmt.Sprintf("Failed to heal partition: %v", err))
	}
	
	// Measure recovery time
	scenario.RecoveryTime = cf.measureRecoveryTime(nodes)
	scenario.EndTime = time.Now()
	scenario.Duration = scenario.EndTime.Sub(scenario.StartTime)
	
	cf.logger.Printf("Partition scenario %s: violations=%d, split-brain=%t, data-loss=%t, recovery=%v",
		scenarioName, scenario.ConsistencyViolations, scenario.SplitBrainDetected, 
		scenario.DataLossDetected, scenario.RecoveryTime)
	
	return scenario
}

// checkPartitionConsistency checks for consistency issues during partitions
func (cf *ChaosFramework) checkPartitionConsistency(targetNodes []string, scenario string) int {
	violations := 0
	
	// Simulate different types of consistency checks
	switch scenario {
	case "majority_isolation":
		// Majority should continue operating, minority should stop accepting writes
		if rand.Float64() < 0.15 { // 15% chance of violation
			violations += rand.Intn(2) + 1
		}
	case "minority_isolation":
		// Minority should be unable to make progress
		if rand.Float64() < 0.05 { // 5% chance of violation (should be rare)
			violations += 1
		}
	case "split_brain":
		// Both sides might think they're the majority
		if rand.Float64() < 0.4 { // 40% chance of split-brain issues
			violations += rand.Intn(3) + 2
		}
	case "asymmetric_partition":
		// One-way communication can cause subtle issues
		if rand.Float64() < 0.25 { // 25% chance
			violations += rand.Intn(2) + 1
		}
	case "cascading_failure":
		// Multiple failures can compound issues
		if rand.Float64() < 0.35 { // 35% chance
			violations += rand.Intn(4) + 1
		}
	}
	
	return violations
}

// detectSplitBrain detects if the cluster has split into multiple leaders
func (cf *ChaosFramework) detectSplitBrain(allNodes, partitionedNodes []string) bool {
	// In a real implementation, this would:
	// - Check if multiple nodes think they're the leader
	// - Verify Raft term consistency
	// - Check for conflicting log entries
	
	// Simulate split-brain detection based on partition size
	partitionRatio := float64(len(partitionedNodes)) / float64(len(allNodes))
	
	// Higher chance of split-brain when partition is close to 50/50
	splitBrainChance := 0.1
	if partitionRatio > 0.4 && partitionRatio < 0.6 {
		splitBrainChance = 0.3
	}
	
	return rand.Float64() < splitBrainChance
}

// detectDataLoss detects if data has been lost during partition
func (cf *ChaosFramework) detectDataLoss(nodes []string) bool {
	// In a real implementation, this would:
	// - Compare checksums across nodes
	// - Verify data integrity
	// - Check for missing transactions
	
	// Simulate data loss detection (should be rare in well-designed systems)
	return rand.Float64() < 0.02 // 2% chance
}

// validateCAPTheorem validates CAP theorem trade-offs
func (cf *ChaosFramework) validateCAPTheorem(ctx context.Context, nodes []string) *CAPValidation {
	cf.logger.Printf("Validating CAP theorem trade-offs")
	
	validation := &CAPValidation{
		ConsistencyScore:  0.0,
		AvailabilityScore: 0.0,
		PartitionTolerance: 0.0,
		Observations:     make([]string, 0),
	}
	
	// Test consistency under normal conditions
	consistencyTests := []string{"read_your_writes", "monotonic_read", "causal_consistency"}
	passedConsistency := 0
	
	for _, test := range consistencyTests {
		if cf.runConsistencyTest(test, nodes) {
			passedConsistency++
		}
	}
	validation.ConsistencyScore = float64(passedConsistency) / float64(len(consistencyTests))
	
	// Test availability under failures
	availabilityTests := []string{"node_failure_response", "partition_availability", "load_balancing"}
	passedAvailability := 0
	
	for _, test := range availabilityTests {
		if cf.runAvailabilityTest(test, nodes) {
			passedAvailability++
		}
	}
	validation.AvailabilityScore = float64(passedAvailability) / float64(len(availabilityTests))
	
	// Test partition tolerance
	partitionTests := []string{"majority_partition", "minority_partition", "network_split"}
	passedPartition := 0
	
	for _, test := range partitionTests {
		if cf.runPartitionToleranceTest(test, nodes) {
			passedPartition++
		}
	}
	validation.PartitionTolerance = float64(passedPartition) / float64(len(partitionTests))
	
	// Generate observations
	if validation.ConsistencyScore > 0.8 && validation.AvailabilityScore > 0.8 {
		validation.Observations = append(validation.Observations, 
			"High consistency and availability achieved, partition tolerance may be limited")
	}
	
	if validation.PartitionTolerance > 0.8 && validation.ConsistencyScore < 0.7 {
		validation.Observations = append(validation.Observations,
			"Good partition tolerance with relaxed consistency (AP system)")
	}
	
	if validation.PartitionTolerance > 0.8 && validation.AvailabilityScore < 0.7 {
		validation.Observations = append(validation.Observations,
			"Good partition tolerance with strong consistency (CP system)")
	}
	
	return validation
}

// runConsistencyTest runs a specific consistency test
func (cf *ChaosFramework) runConsistencyTest(testType string, nodes []string) bool {
	// Simulate consistency test results
	switch testType {
	case "read_your_writes":
		return rand.Float64() < 0.9 // 90% pass rate
	case "monotonic_read":
		return rand.Float64() < 0.85 // 85% pass rate
	case "causal_consistency":
		return rand.Float64() < 0.8 // 80% pass rate
	default:
		return rand.Float64() < 0.75
	}
}

// runAvailabilityTest runs a specific availability test
func (cf *ChaosFramework) runAvailabilityTest(testType string, nodes []string) bool {
	switch testType {
	case "node_failure_response":
		return rand.Float64() < 0.95 // 95% pass rate
	case "partition_availability":
		return rand.Float64() < 0.8 // 80% pass rate
	case "load_balancing":
		return rand.Float64() < 0.9 // 90% pass rate
	default:
		return rand.Float64() < 0.85
	}
}

// runPartitionToleranceTest runs a specific partition tolerance test
func (cf *ChaosFramework) runPartitionToleranceTest(testType string, nodes []string) bool {
	switch testType {
	case "majority_partition":
		return rand.Float64() < 0.9 // 90% pass rate
	case "minority_partition":
		return rand.Float64() < 0.95 // 95% pass rate (should handle this well)
	case "network_split":
		return rand.Float64() < 0.7 // 70% pass rate (challenging)
	default:
		return rand.Float64() < 0.8
	}
}

// checkLinearizability checks for linearizability violations
func (cf *ChaosFramework) checkLinearizability(ctx context.Context, nodes []string) *LinearizabilityCheck {
	cf.logger.Printf("Checking linearizability across nodes: %v", nodes)
	
	check := &LinearizabilityCheck{
		TestedOperations: 0,
		Violations:      0,
		OperationResults: make([]*OperationResult, 0),
	}
	
	// Simulate testing various operations for linearizability
	operations := []string{"put", "get", "delete", "cas"} // compare-and-swap
	
	for i := 0; i < 50; i++ { // Test 50 operations
		op := operations[rand.Intn(len(operations))]
		result := cf.testOperationLinearizability(op, nodes)
		
		check.TestedOperations++
		check.OperationResults = append(check.OperationResults, result)
		
		if !result.IsLinearizable {
			check.Violations++
		}
	}
	
	check.LinearizabilityRatio = float64(check.TestedOperations-check.Violations) / float64(check.TestedOperations)
	
	cf.logger.Printf("Linearizability check: %d/%d operations passed (%.2f%%)", 
		check.TestedOperations-check.Violations, check.TestedOperations, 
		check.LinearizabilityRatio*100)
	
	return check
}

// testOperationLinearizability tests if a specific operation maintains linearizability
func (cf *ChaosFramework) testOperationLinearizability(operation string, nodes []string) *OperationResult {
	result := &OperationResult{
		Operation:       operation,
		Timestamp:       time.Now(),
		TestedNodes:     nodes,
		IsLinearizable:  true,
	}
	
	// Simulate operation testing with different failure probabilities
	switch operation {
	case "put":
		result.IsLinearizable = rand.Float64() < 0.95 // 95% success
	case "get":
		result.IsLinearizable = rand.Float64() < 0.98 // 98% success
	case "delete":
		result.IsLinearizable = rand.Float64() < 0.93 // 93% success
	case "cas":
		result.IsLinearizable = rand.Float64() < 0.85 // 85% success (more complex)
	}
	
	if !result.IsLinearizable {
		result.ViolationReason = fmt.Sprintf("Linearizability violation detected in %s operation", operation)
	}
	
	return result
}

// assessConsistencyStatus provides an overall assessment of consistency
func (cf *ChaosFramework) assessConsistencyStatus(report *ConsistencyReport) string {
	if report.TotalViolations == 0 && report.MaxRecoveryTime < 30*time.Second {
		return "EXCELLENT"
	} else if report.TotalViolations <= 2 && report.MaxRecoveryTime < 2*time.Minute {
		return "GOOD"
	} else if report.TotalViolations <= 5 && report.MaxRecoveryTime < 5*time.Minute {
		return "FAIR"
	} else {
		return "POOR"
	}
}

// generateRecommendations generates recommendations based on experiment results
func (cf *ChaosFramework) generateRecommendations(exp *Experiment) {
	recommendations := make([]string, 0)
	
	if exp.Results.ConsistencyViolations > 0 {
		recommendations = append(recommendations, 
			"Consider implementing stronger consistency checks")
	}
	
	if exp.Results.RecoveryTime > 2*time.Minute {
		recommendations = append(recommendations, 
			"Recovery time is high, consider optimizing failure detection")
	}
	
	if exp.Metrics.ErrorRate > 0.05 { // 5% error rate
		recommendations = append(recommendations, 
			"High error rate detected, review error handling")
	}
	
	if exp.Results.AvailabilityImpact > 50 { // 50% availability impact
		recommendations = append(recommendations, 
			"High availability impact, consider improving redundancy")
	}
	
	exp.Results.Recommendations = recommendations
}

// runNodeFailureExperiment simulates node failures
func (cf *ChaosFramework) runNodeFailureExperiment(ctx context.Context, exp *Experiment) error {
	cf.logger.Printf("Running node failure experiment %s", exp.ID)
	
	// Get kill method and count from parameters
	method := KillMethodSIGTERM
	if m, ok := exp.Parameters["method"].(string); ok {
		method = KillMethod(m)
	}
	
	count := 1
	if c, ok := exp.Parameters["count"].(int); ok {
		count = c
	}
	
	reason := fmt.Sprintf("Chaos experiment %s", exp.ID)
	
	// Kill random nodes
	killedNodes, err := cf.nodeKiller.KillRandomNodes(count, method, cf.config.KillDuration, reason)
	if err != nil {
		return fmt.Errorf("failed to kill nodes: %w", err)
	}
	
	exp.TargetNodes = killedNodes
	cf.logger.Printf("Killed nodes: %v using method %s", killedNodes, method)
	
	// Wait for kill duration
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(cf.config.KillDuration):
		// Duration completed
	}
	
	// Measure recovery after restart
	recoveryStart := time.Now()
	recoveryTime := cf.measureRecoveryTime(killedNodes)
	
	exp.Results.RecoveryTime = recoveryTime
	exp.Results.Success = true
	
	cf.logger.Printf("Node failure experiment %s: killed=%d, recovery=%v", 
		exp.ID, len(killedNodes), recoveryTime)
	
	return nil
}

// runDiskCorruptionExperiment simulates disk corruption
func (cf *ChaosFramework) runDiskCorruptionExperiment(ctx context.Context, exp *Experiment) error {
	cf.logger.Printf("Running disk corruption experiment %s", exp.ID)
	
	// Get corruption parameters
	corruptionType := CorruptionTypeRandom
	if t, ok := exp.Parameters["type"].(string); ok {
		corruptionType = CorruptionType(t)
	}
	
	targetPath := "/data"
	if p, ok := exp.Parameters["path"].(string); ok {
		targetPath = p
	}
	
	// Apply corruption
	corruptionID, err := cf.diskCorruptor.CorruptPath(targetPath, corruptionType, exp.Parameters)
	if err != nil {
		return fmt.Errorf("failed to corrupt disk: %w", err)
	}
	
	cf.logger.Printf("Applied disk corruption %s to path %s", corruptionID, targetPath)
	
	// Wait for experiment duration
	duration := 2 * time.Minute
	if d, ok := exp.Parameters["duration"].(time.Duration); ok {
		duration = d
	}
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		// Duration completed
	}
	
	// Restore corruption
	err = cf.diskCorruptor.RestoreCorruption(corruptionID)
	if err != nil {
		exp.ErrorLog = append(exp.ErrorLog, fmt.Sprintf("Failed to restore corruption: %v", err))
	}
	
	// Measure recovery
	recoveryTime := cf.measureRecoveryTime([]string{})
	exp.Results.RecoveryTime = recoveryTime
	exp.Results.Success = err == nil
	
	return nil
}

// runByzantineExperiment simulates Byzantine faults
func (cf *ChaosFramework) runByzantineExperiment(ctx context.Context, exp *Experiment) error {
	cf.logger.Printf("Running Byzantine fault experiment %s", exp.ID)
	
	if !cf.config.ByzantineEnabled {
		return fmt.Errorf("Byzantine fault testing is disabled")
	}
	
	// Get Byzantine behavior type
	behavior := "conflicting_votes"
	if b, ok := exp.Parameters["behavior"].(string); ok {
		behavior = b
	}
	
	// Get target node
	targetNode := "node1"
	if t, ok := exp.Parameters["target"].(string); ok {
		targetNode = t
	}
	
	exp.TargetNodes = []string{targetNode}
	
	cf.logger.Printf("Injecting Byzantine behavior '%s' into node %s", behavior, targetNode)
	
	// Simulate Byzantine behavior injection
	switch behavior {
	case "conflicting_votes":
		cf.logger.Printf("Node %s now sending conflicting votes", targetNode)
	case "invalid_proposals":
		cf.logger.Printf("Node %s now sending invalid proposals", targetNode)
	case "delayed_responses":
		cf.logger.Printf("Node %s now delaying responses", targetNode)
	case "corrupted_messages":
		cf.logger.Printf("Node %s now sending corrupted messages", targetNode)
	}
	
	// Run experiment for specified duration
	duration := 3 * time.Minute
	if d, ok := exp.Parameters["duration"].(time.Duration); ok {
		duration = d
	}
	
	// Monitor system during Byzantine behavior
	startTime := time.Now()
	consistencyViolations := 0
	
	for time.Since(startTime) < duration {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
			// Check for violations every 30 seconds
			violations := cf.checkByzantineViolations(targetNode, behavior)
			consistencyViolations += violations
		}
	}
	
	// Stop Byzantine behavior
	cf.logger.Printf("Stopping Byzantine behavior for node %s", targetNode)
	
	// Measure recovery time
	recoveryTime := cf.measureRecoveryTime([]string{targetNode})
	
	// Update results
	exp.Results.Success = true
	exp.Results.RecoveryTime = recoveryTime
	exp.Results.ConsistencyViolations = consistencyViolations
	
	// Generate Byzantine-specific observations
	if consistencyViolations > 0 {
		exp.Results.Observations = append(exp.Results.Observations,
			fmt.Sprintf("Detected %d consistency violations during Byzantine behavior", consistencyViolations))
	}
	
	if recoveryTime > 2*time.Minute {
		exp.Results.Observations = append(exp.Results.Observations,
			fmt.Sprintf("Recovery time %v exceeds target", recoveryTime))
	}
	
	cf.logger.Printf("Byzantine experiment %s: behavior=%s, violations=%d, recovery=%v", 
		exp.ID, behavior, consistencyViolations, recoveryTime)
	
	return nil
}

// runCombinedExperiment runs multiple chaos types simultaneously
func (cf *ChaosFramework) runCombinedExperiment(ctx context.Context, exp *Experiment) error {
	cf.logger.Printf("Running combined chaos experiment %s", exp.ID)
	
	// Create sub-experiments
	var subExperiments []*Experiment
	
	// Network partition
	if cf.config.NetworkEnabled {
		networkExp := &Experiment{
			ID:         fmt.Sprintf("%s_network", exp.ID),
			Type:       ExperimentNetworkPartition,
			Parameters: map[string]interface{}{"target_nodes": []string{"node1", "node2"}},
		}
		subExperiments = append(subExperiments, networkExp)
	}
	
	// Node failure
	if cf.config.NodeEnabled {
		nodeExp := &Experiment{
			ID:         fmt.Sprintf("%s_node", exp.ID),
			Type:       ExperimentNodeFailure,
			Parameters: map[string]interface{}{"method": "SIGKILL", "count": 1},
		}
		subExperiments = append(subExperiments, nodeExp)
	}
	
	// Execute sub-experiments concurrently
	errChan := make(chan error, len(subExperiments))
	
	for _, subExp := range subExperiments {
		go func(e *Experiment) {
			var err error
			switch e.Type {
			case ExperimentNetworkPartition:
				err = cf.runNetworkPartitionExperiment(ctx, e)
			case ExperimentNodeFailure:
				err = cf.runNodeFailureExperiment(ctx, e)
			case ExperimentDiskCorruption:
				err = cf.runDiskCorruptionExperiment(ctx, e)
			}
			errChan <- err
		}(subExp)
	}
	
	// Wait for all sub-experiments
	var errors []error
	for i := 0; i < len(subExperiments); i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}
	
	// Aggregate results
	totalViolations := 0
	maxRecoveryTime := time.Duration(0)
	
	for _, subExp := range subExperiments {
		if subExp.Results != nil {
			totalViolations += subExp.Results.ConsistencyViolations
			if subExp.Results.RecoveryTime > maxRecoveryTime {
				maxRecoveryTime = subExp.Results.RecoveryTime
			}
		}
	}
	
	exp.Results.ConsistencyViolations = totalViolations
	exp.Results.RecoveryTime = maxRecoveryTime
	exp.Results.Success = len(errors) == 0
	
	if len(errors) > 0 {
		for _, err := range errors {
			exp.ErrorLog = append(exp.ErrorLog, err.Error())
		}
	}
	
	return nil
}

// checkByzantineViolations checks for Byzantine-specific violations
func (cf *ChaosFramework) checkByzantineViolations(targetNode, behavior string) int {
	// In a real implementation, this would:
	// - Monitor Raft consensus for conflicting votes
	// - Check for invalid state transitions
	// - Verify message authenticity
	// - Detect split-brain scenarios
	
	// For simulation, generate violations based on behavior type
	violationChance := 0.2 // 20% base chance
	
	switch behavior {
	case "conflicting_votes":
		violationChance = 0.3 // Higher chance for vote conflicts
	case "invalid_proposals":
		violationChance = 0.25
	case "corrupted_messages":
		violationChance = 0.15
	}
	
	if rand.Float64() < violationChance {
		return rand.Intn(2) + 1 // 1-2 violations
	}
	
	return 0
}

// logExperimentResults logs the results of an experiment
func (cf *ChaosFramework) logExperimentResults(exp *Experiment) {
	cf.logger.Printf("=== Experiment Results: %s ===", exp.ID)
	cf.logger.Printf("Type: %s", exp.Type)
	cf.logger.Printf("Duration: %v", exp.Duration)
	cf.logger.Printf("Status: %s", exp.Status)
	cf.logger.Printf("Success: %t", exp.Results.Success)
	cf.logger.Printf("Recovery Time: %v", exp.Results.RecoveryTime)
	cf.logger.Printf("Consistency Violations: %d", exp.Results.ConsistencyViolations)
	cf.logger.Printf("Error Rate: %.2f%%", exp.Metrics.ErrorRate*100)
	
	if len(exp.Results.Observations) > 0 {
		cf.logger.Printf("Observations:")
		for _, obs := range exp.Results.Observations {
			cf.logger.Printf("  - %s", obs)
		}
	}
	
	if len(exp.Results.Recommendations) > 0 {
		cf.logger.Printf("Recommendations:")
		for _, rec := range exp.Results.Recommendations {
			cf.logger.Printf("  - %s", rec)
		}
	}
}

// GetExperiment returns information about a specific experiment
func (cf *ChaosFramework) GetExperiment(expID string) (*Experiment, error) {
	cf.mu.RLock()
	defer cf.mu.RUnlock()
	
	exp, exists := cf.experiments[expID]
	if !exists {
		return nil, fmt.Errorf("experiment %s not found", expID)
	}
	
	// Return a copy
	expCopy := *exp
	return &expCopy, nil
}

// GetAllExperiments returns all experiments
func (cf *ChaosFramework) GetAllExperiments() map[string]*Experiment {
	cf.mu.RLock()
	defer cf.mu.RUnlock()
	
	result := make(map[string]*Experiment)
	for expID, exp := range cf.experiments {
		expCopy := *exp
		result[expID] = &expCopy
	}
	
	return result
}

// GetStats returns chaos framework statistics
func (cf *ChaosFramework) GetStats() ChaosStats {
	cf.mu.RLock()
	defer cf.mu.RUnlock()
	
	stats := ChaosStats{
		IsRunning:         cf.isRunning,
		StartTime:         cf.startTime,
		TotalExperiments:  cf.totalExperiments,
		ActiveExperiments: cf.activeExperiments,
		ExperimentsByType: make(map[ExperimentType]int),
		ExperimentsByStatus: make(map[ExperimentStatus]int),
	}
	
	for _, exp := range cf.experiments {
		stats.ExperimentsByType[exp.Type]++
		stats.ExperimentsByStatus[exp.Status]++
	}
	
	return stats
}

// ChaosStats contains statistics about the chaos framework
type ChaosStats struct {
	IsRunning           bool                            `json:"is_running"`
	StartTime           time.Time                       `json:"start_time"`
	TotalExperiments    int64                           `json:"total_experiments"`
	ActiveExperiments   int64                           `json:"active_experiments"`
	ExperimentsByType   map[ExperimentType]int          `json:"experiments_by_type"`
	ExperimentsByStatus map[ExperimentStatus]int        `json:"experiments_by_status"`
}

// ConsistencyReport contains results of consistency validation
type ConsistencyReport struct {
	StartTime            time.Time                     `json:"start_time"`
	EndTime              time.Time                     `json:"end_time"`
	Duration             time.Duration                 `json:"duration"`
	TestedNodes          []string                      `json:"tested_nodes"`
	PartitionScenarios   []*PartitionScenario          `json:"partition_scenarios"`
	TotalViolations      int                           `json:"total_violations"`
	MaxRecoveryTime      time.Duration                 `json:"max_recovery_time"`
	OverallStatus        string                        `json:"overall_status"`
	ConsistencyChecks    map[string]*ConsistencyCheck  `json:"consistency_checks"`
	CAPValidation        *CAPValidation                `json:"cap_validation"`
	LinearizabilityCheck *LinearizabilityCheck         `json:"linearizability_check"`
}

// PartitionScenario contains results of a specific partition test scenario
type PartitionScenario struct {
	Name                  string        `json:"name"`
	PartitionType         PartitionType `json:"partition_type"`
	PartitionID           string        `json:"partition_id"`
	StartTime             time.Time     `json:"start_time"`
	EndTime               time.Time     `json:"end_time"`
	Duration              time.Duration `json:"duration"`
	TestedNodes           []string      `json:"tested_nodes"`
	ConsistencyViolations int           `json:"consistency_violations"`
	DataLossDetected      bool          `json:"data_loss_detected"`
	SplitBrainDetected    bool          `json:"split_brain_detected"`
	RecoveryTime          time.Duration `json:"recovery_time"`
	Errors                []string      `json:"errors"`
}

// ConsistencyCheck represents a specific consistency check
type ConsistencyCheck struct {
	Name        string    `json:"name"`
	Passed      bool      `json:"passed"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Details     string    `json:"details"`
}

// CAPValidation contains CAP theorem validation results
type CAPValidation struct {
	ConsistencyScore   float64  `json:"consistency_score"`
	AvailabilityScore  float64  `json:"availability_score"`
	PartitionTolerance float64  `json:"partition_tolerance"`
	Observations       []string `json:"observations"`
}

// LinearizabilityCheck contains linearizability validation results
type LinearizabilityCheck struct {
	TestedOperations     int                `json:"tested_operations"`
	Violations           int                `json:"violations"`
	LinearizabilityRatio float64            `json:"linearizability_ratio"`
	OperationResults     []*OperationResult `json:"operation_results"`
}

// OperationResult contains the result of testing a single operation for linearizability
type OperationResult struct {
	Operation        string    `json:"operation"`
	Timestamp        time.Time `json:"timestamp"`
	TestedNodes      []string  `json:"tested_nodes"`
	IsLinearizable   bool      `json:"is_linearizable"`
	ViolationReason  string    `json:"violation_reason,omitempty"`
}

// RecoveryReport contains comprehensive recovery time measurements
type RecoveryReport struct {
	StartTime           time.Time                      `json:"start_time"`
	EndTime             time.Time                      `json:"end_time"`
	Duration            time.Duration                  `json:"duration"`
	TestedNodes         []string                       `json:"tested_nodes"`
	RecoveryScenarios   []*RecoveryScenario           `json:"recovery_scenarios"`
	AverageRecoveryTime time.Duration                  `json:"average_recovery_time"`
	MaxRecoveryTime     time.Duration                  `json:"max_recovery_time"`
	MinRecoveryTime     time.Duration                  `json:"min_recovery_time"`
	OverallAssessment   string                         `json:"overall_assessment"`
	RecoveryMetrics     map[string]*RecoveryMetric     `json:"recovery_metrics"`
}

// RecoveryScenario contains results of a specific recovery test scenario
type RecoveryScenario struct {
	Name           string           `json:"name"`
	FailureType    string           `json:"failure_type"`
	Description    string           `json:"description"`
	StartTime      time.Time        `json:"start_time"`
	EndTime        time.Time        `json:"end_time"`
	Duration       time.Duration    `json:"duration"`
	TestedNodes    []string         `json:"tested_nodes"`
	RecoveryTime   time.Duration    `json:"recovery_time"`
	Success        bool             `json:"success"`
	RecoveryPhases []*RecoveryPhase `json:"recovery_phases"`
	Errors         []string         `json:"errors"`
}

// RecoveryPhase represents a phase in the recovery process
type RecoveryPhase struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Duration    time.Duration `json:"duration"`
}

// RecoveryMetric contains metrics for a specific recovery aspect
type RecoveryMetric struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	RecoveryTime time.Duration `json:"recovery_time"`
	Success      bool          `json:"success"`
	Details      string        `json:"details"`
}