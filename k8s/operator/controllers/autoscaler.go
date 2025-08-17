package controllers

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/go-logr/logr"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kvstoreiov1 "github.com/kvstore/operator/api/v1"
)

// AutoScaler handles automatic scaling of KVStore clusters
type AutoScaler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// ScalingDecision represents a scaling decision
type ScalingDecision struct {
	ShouldScale   bool
	TargetReplicas int32
	Reason        string
	ScaleDirection ScaleDirection
	Confidence    float64
}

// ScaleDirection represents the direction of scaling
type ScaleDirection string

const (
	ScaleUp   ScaleDirection = "up"
	ScaleDown ScaleDirection = "down"
	ScaleNone ScaleDirection = "none"
)

// MetricsData represents collected metrics for scaling decisions
type MetricsData struct {
	CPUUtilization    float64
	MemoryUtilization float64
	RequestRate       float64
	ResponseTime      float64
	QueueDepth        int64
	ActiveConnections int64
	DiskUtilization   float64
	NetworkIO         float64
	RaftHealth        bool
	NodeHealth        map[string]bool
	Timestamp         time.Time
}

// ScalingHistory tracks scaling decisions and outcomes
type ScalingHistory struct {
	Decisions []ScalingEvent
	mutex     sync.RWMutex
}

type ScalingEvent struct {
	Timestamp       time.Time
	FromReplicas    int32
	ToReplicas      int32
	Reason          string
	MetricsSnapshot MetricsData
	Success         bool
	Duration        time.Duration
}

// NewAutoScaler creates a new AutoScaler
func NewAutoScaler(client client.Client, log logr.Logger, scheme *runtime.Scheme, recorder record.EventRecorder) *AutoScaler {
	return &AutoScaler{
		Client:   client,
		Log:      log,
		Scheme:   scheme,
		Recorder: recorder,
	}
}

// EvaluateScaling evaluates whether a KVStore cluster should be scaled
func (a *AutoScaler) EvaluateScaling(ctx context.Context, kvstore *kvstoreiov1.KVStore) (*ScalingDecision, error) {
	log := a.Log.WithValues("kvstore", kvstore.Name, "namespace", kvstore.Namespace)

	// Check if scaling is enabled
	if !kvstore.Spec.Scaling.Enabled {
		return &ScalingDecision{
			ShouldScale:    false,
			TargetReplicas: kvstore.Spec.Cluster.Replicas,
			Reason:         "Scaling disabled",
			ScaleDirection: ScaleNone,
			Confidence:     0.0,
		}, nil
	}

	// Collect current metrics
	metrics, err := a.collectMetrics(ctx, kvstore)
	if err != nil {
		log.Error(err, "Failed to collect metrics for scaling evaluation")
		return nil, err
	}

	// Get current replica count
	currentReplicas := kvstore.Spec.Cluster.Replicas

	// Evaluate scaling based on multiple factors
	decision := a.makeScalingDecision(kvstore, metrics, currentReplicas)

	// Apply safety checks
	decision = a.applySafetyChecks(kvstore, decision, currentReplicas)

	// Check cooldown period
	if a.isInCooldownPeriod(kvstore, decision.ScaleDirection) {
		decision.ShouldScale = false
		decision.Reason = fmt.Sprintf("In cooldown period: %s", decision.Reason)
	}

	log.Info("Scaling evaluation completed",
		"shouldScale", decision.ShouldScale,
		"currentReplicas", currentReplicas,
		"targetReplicas", decision.TargetReplicas,
		"reason", decision.Reason,
		"confidence", decision.Confidence,
	)

	return decision, nil
}

// collectMetrics collects metrics from the KVStore cluster
func (a *AutoScaler) collectMetrics(ctx context.Context, kvstore *kvstoreiov1.KVStore) (*MetricsData, error) {
	// In a real implementation, this would collect metrics from Prometheus or other monitoring systems
	// For now, we'll simulate metric collection

	metrics := &MetricsData{
		Timestamp: time.Now(),
	}

	// Collect CPU and Memory metrics from pods
	cpuUsage, memoryUsage, err := a.collectResourceMetrics(ctx, kvstore)
	if err != nil {
		return nil, fmt.Errorf("failed to collect resource metrics: %w", err)
	}
	metrics.CPUUtilization = cpuUsage
	metrics.MemoryUtilization = memoryUsage

	// Collect application-specific metrics
	appMetrics, err := a.collectApplicationMetrics(ctx, kvstore)
	if err != nil {
		return nil, fmt.Errorf("failed to collect application metrics: %w", err)
	}
	metrics.RequestRate = appMetrics.RequestRate
	metrics.ResponseTime = appMetrics.ResponseTime
	metrics.QueueDepth = appMetrics.QueueDepth
	metrics.ActiveConnections = appMetrics.ActiveConnections

	// Collect cluster health metrics
	healthMetrics, err := a.collectHealthMetrics(ctx, kvstore)
	if err != nil {
		return nil, fmt.Errorf("failed to collect health metrics: %w", err)
	}
	metrics.RaftHealth = healthMetrics.RaftHealth
	metrics.NodeHealth = healthMetrics.NodeHealth

	return metrics, nil
}

// collectResourceMetrics collects CPU and memory metrics
func (a *AutoScaler) collectResourceMetrics(ctx context.Context, kvstore *kvstoreiov1.KVStore) (float64, float64, error) {
	// Get pods for the KVStore
	podList := &corev1.PodList{}
	labels := map[string]string{
		"app":                "kvstore",
		"kvstore.io/cluster": kvstore.Name,
	}
	
	listOpts := []client.ListOption{
		client.InNamespace(kvstore.Namespace),
		client.MatchingLabels(labels),
	}
	
	if err := a.List(ctx, podList, listOpts...); err != nil {
		return 0, 0, err
	}

	if len(podList.Items) == 0 {
		return 0, 0, fmt.Errorf("no pods found for KVStore %s", kvstore.Name)
	}

	// In a real implementation, you would query the metrics server or Prometheus
	// For simulation, we'll return mock values
	totalCPU := 0.0
	totalMemory := 0.0
	podCount := float64(len(podList.Items))

	for _, pod := range podList.Items {
		// Simulate CPU usage (40-80%)
		totalCPU += 50.0 + (float64(time.Now().UnixNano()%30))
		// Simulate Memory usage (30-70%)
		totalMemory += 40.0 + (float64(time.Now().UnixNano()%30))
	}

	avgCPU := totalCPU / podCount
	avgMemory := totalMemory / podCount

	return avgCPU, avgMemory, nil
}

// ApplicationMetrics represents application-specific metrics
type ApplicationMetrics struct {
	RequestRate       float64
	ResponseTime      float64
	QueueDepth        int64
	ActiveConnections int64
}

// collectApplicationMetrics collects application-specific metrics
func (a *AutoScaler) collectApplicationMetrics(ctx context.Context, kvstore *kvstoreiov1.KVStore) (*ApplicationMetrics, error) {
	// In a real implementation, this would query Prometheus or custom metrics
	// For simulation, we'll return mock values

	metrics := &ApplicationMetrics{
		RequestRate:       100.0 + float64(time.Now().UnixNano()%200), // 100-300 req/s
		ResponseTime:      50.0 + float64(time.Now().UnixNano()%100),  // 50-150ms
		QueueDepth:        int64(time.Now().UnixNano() % 50),          // 0-50 queued requests
		ActiveConnections: int64(100 + time.Now().UnixNano()%400),     // 100-500 connections
	}

	return metrics, nil
}

// HealthMetrics represents cluster health metrics
type HealthMetrics struct {
	RaftHealth bool
	NodeHealth map[string]bool
}

// collectHealthMetrics collects cluster health metrics
func (a *AutoScaler) collectHealthMetrics(ctx context.Context, kvstore *kvstoreiov1.KVStore) (*HealthMetrics, error) {
	// In a real implementation, this would check Raft status and node health
	// For simulation, we'll assume healthy cluster

	metrics := &HealthMetrics{
		RaftHealth: true,
		NodeHealth: make(map[string]bool),
	}

	// Simulate node health
	for i := int32(0); i < kvstore.Spec.Cluster.Replicas; i++ {
		nodeName := fmt.Sprintf("%s-%d", kvstore.Name, i)
		metrics.NodeHealth[nodeName] = true
	}

	return metrics, nil
}

// makeScalingDecision makes a scaling decision based on collected metrics
func (a *AutoScaler) makeScalingDecision(kvstore *kvstoreiov1.KVStore, metrics *MetricsData, currentReplicas int32) *ScalingDecision {
	decision := &ScalingDecision{
		ShouldScale:    false,
		TargetReplicas: currentReplicas,
		ScaleDirection: ScaleNone,
		Confidence:     0.0,
	}

	// Calculate scaling factors based on different metrics
	factors := a.calculateScalingFactors(kvstore, metrics)

	// Determine scaling direction and magnitude
	scaleUpScore := a.calculateScaleUpScore(factors)
	scaleDownScore := a.calculateScaleDownScore(factors)

	// Make decision based on scores
	if scaleUpScore > 0.7 && scaleUpScore > scaleDownScore {
		decision.ShouldScale = true
		decision.ScaleDirection = ScaleUp
		decision.TargetReplicas = a.calculateScaleUpTarget(kvstore, currentReplicas, scaleUpScore)
		decision.Reason = a.buildScaleUpReason(factors)
		decision.Confidence = scaleUpScore
	} else if scaleDownScore > 0.7 && scaleDownScore > scaleUpScore {
		decision.ShouldScale = true
		decision.ScaleDirection = ScaleDown
		decision.TargetReplicas = a.calculateScaleDownTarget(kvstore, currentReplicas, scaleDownScore)
		decision.Reason = a.buildScaleDownReason(factors)
		decision.Confidence = scaleDownScore
	} else {
		decision.Reason = "Metrics within acceptable range"
		decision.Confidence = math.Max(scaleUpScore, scaleDownScore)
	}

	return decision
}

// ScalingFactors represents various factors that influence scaling decisions
type ScalingFactors struct {
	CPUPressure     float64
	MemoryPressure  float64
	RequestPressure float64
	ResponsePressure float64
	QueuePressure   float64
	HealthPressure  float64
}

// calculateScalingFactors calculates various scaling factors
func (a *AutoScaler) calculateScalingFactors(kvstore *kvstoreiov1.KVStore, metrics *MetricsData) *ScalingFactors {
	factors := &ScalingFactors{}

	// CPU pressure (normalized 0-1)
	targetCPU := float64(kvstore.Spec.Scaling.TargetCPUUtilization)
	factors.CPUPressure = metrics.CPUUtilization / targetCPU

	// Memory pressure (normalized 0-1)
	targetMemory := float64(kvstore.Spec.Scaling.TargetMemoryUtilization)
	factors.MemoryPressure = metrics.MemoryUtilization / targetMemory

	// Request pressure (based on request rate trends)
	factors.RequestPressure = a.calculateRequestPressure(metrics.RequestRate)

	// Response pressure (based on response time)
	factors.ResponsePressure = a.calculateResponsePressure(metrics.ResponseTime)

	// Queue pressure (based on queue depth)
	factors.QueuePressure = a.calculateQueuePressure(metrics.QueueDepth)

	// Health pressure (based on cluster health)
	factors.HealthPressure = a.calculateHealthPressure(metrics.RaftHealth, metrics.NodeHealth)

	return factors
}

// calculateScaleUpScore calculates a score for scaling up
func (a *AutoScaler) calculateScaleUpScore(factors *ScalingFactors) float64 {
	// Weighted combination of factors that suggest scaling up
	weights := map[string]float64{
		"cpu":      0.3,
		"memory":   0.25,
		"request":  0.2,
		"response": 0.15,
		"queue":    0.1,
	}

	score := 0.0
	
	// CPU pressure contributes to scale up if > 1.0
	if factors.CPUPressure > 1.0 {
		score += weights["cpu"] * math.Min((factors.CPUPressure-1.0)*2, 1.0)
	}

	// Memory pressure contributes to scale up if > 1.0
	if factors.MemoryPressure > 1.0 {
		score += weights["memory"] * math.Min((factors.MemoryPressure-1.0)*2, 1.0)
	}

	// High request pressure suggests scale up
	if factors.RequestPressure > 0.8 {
		score += weights["request"] * (factors.RequestPressure - 0.8) * 5
	}

	// High response pressure suggests scale up
	if factors.ResponsePressure > 0.7 {
		score += weights["response"] * (factors.ResponsePressure - 0.7) * 3.33
	}

	// Queue pressure suggests scale up
	if factors.QueuePressure > 0.5 {
		score += weights["queue"] * (factors.QueuePressure - 0.5) * 2
	}

	// Health issues reduce scale up confidence
	score *= (1.0 - factors.HealthPressure*0.5)

	return math.Min(score, 1.0)
}

// calculateScaleDownScore calculates a score for scaling down
func (a *AutoScaler) calculateScaleDownScore(factors *ScalingFactors) float64 {
	// Weighted combination of factors that suggest scaling down
	weights := map[string]float64{
		"cpu":      0.4,
		"memory":   0.3,
		"request":  0.15,
		"response": 0.1,
		"queue":    0.05,
	}

	score := 0.0

	// Low CPU usage suggests scale down
	if factors.CPUPressure < 0.3 {
		score += weights["cpu"] * (0.3 - factors.CPUPressure) * 3.33
	}

	// Low memory usage suggests scale down
	if factors.MemoryPressure < 0.4 {
		score += weights["memory"] * (0.4 - factors.MemoryPressure) * 2.5
	}

	// Low request pressure suggests scale down
	if factors.RequestPressure < 0.3 {
		score += weights["request"] * (0.3 - factors.RequestPressure) * 3.33
	}

	// Good response times suggest room for scale down
	if factors.ResponsePressure < 0.3 {
		score += weights["response"] * (0.3 - factors.ResponsePressure) * 3.33
	}

	// No queue pressure suggests scale down potential
	if factors.QueuePressure < 0.1 {
		score += weights["queue"] * (0.1 - factors.QueuePressure) * 10
	}

	// Health issues reduce scale down confidence
	score *= (1.0 - factors.HealthPressure*0.8)

	return math.Min(score, 1.0)
}

// calculateScaleUpTarget calculates the target replica count for scaling up
func (a *AutoScaler) calculateScaleUpTarget(kvstore *kvstoreiov1.KVStore, currentReplicas int32, score float64) int32 {
	// Calculate increment based on pressure and constraints
	maxReplicas := kvstore.Spec.Scaling.MaxReplicas
	
	// Conservative scaling: increment by 1 or 2 based on score
	increment := int32(1)
	if score > 0.9 {
		increment = 2
	}

	target := currentReplicas + increment
	if target > maxReplicas {
		target = maxReplicas
	}

	// Ensure odd number for Raft consensus (recommended)
	if target%2 == 0 && target < maxReplicas {
		target++
	}

	return target
}

// calculateScaleDownTarget calculates the target replica count for scaling down
func (a *AutoScaler) calculateScaleDownTarget(kvstore *kvstoreiov1.KVStore, currentReplicas int32, score float64) int32 {
	// Calculate decrement based on low utilization
	minReplicas := kvstore.Spec.Scaling.MinReplicas
	
	// Conservative scaling: decrement by 1
	decrement := int32(1)
	if score > 0.9 && currentReplicas > minReplicas+2 {
		decrement = 2
	}

	target := currentReplicas - decrement
	if target < minReplicas {
		target = minReplicas
	}

	// Ensure odd number for Raft consensus (recommended)
	if target%2 == 0 && target > minReplicas {
		target--
	}

	return target
}

// applySafetyChecks applies safety checks to scaling decisions
func (a *AutoScaler) applySafetyChecks(kvstore *kvstoreiov1.KVStore, decision *ScalingDecision, currentReplicas int32) *ScalingDecision {
	// Don't scale if cluster is unhealthy
	if !a.isClusterHealthy(kvstore) {
		decision.ShouldScale = false
		decision.Reason = "Cluster unhealthy - scaling disabled"
		return decision
	}

	// Don't scale beyond limits
	if decision.TargetReplicas > kvstore.Spec.Scaling.MaxReplicas {
		decision.TargetReplicas = kvstore.Spec.Scaling.MaxReplicas
		if decision.TargetReplicas == currentReplicas {
			decision.ShouldScale = false
			decision.Reason = "Already at maximum replicas"
		}
	}

	if decision.TargetReplicas < kvstore.Spec.Scaling.MinReplicas {
		decision.TargetReplicas = kvstore.Spec.Scaling.MinReplicas
		if decision.TargetReplicas == currentReplicas {
			decision.ShouldScale = false
			decision.Reason = "Already at minimum replicas"
		}
	}

	// Require minimum confidence for scaling
	if decision.Confidence < 0.6 {
		decision.ShouldScale = false
		decision.Reason = fmt.Sprintf("Confidence too low (%.2f < 0.6): %s", decision.Confidence, decision.Reason)
	}

	return decision
}

// Helper methods for calculating pressure values
func (a *AutoScaler) calculateRequestPressure(requestRate float64) float64 {
	// Normalize request rate pressure (example thresholds)
	if requestRate > 1000 {
		return 1.0
	} else if requestRate > 500 {
		return (requestRate - 500) / 500
	}
	return 0.0
}

func (a *AutoScaler) calculateResponsePressure(responseTime float64) float64 {
	// Normalize response time pressure (example: >200ms is high)
	if responseTime > 200 {
		return math.Min((responseTime-200)/200, 1.0)
	}
	return 0.0
}

func (a *AutoScaler) calculateQueuePressure(queueDepth int64) float64 {
	// Normalize queue pressure (example: >20 items is high)
	if queueDepth > 20 {
		return math.Min(float64(queueDepth-20)/30, 1.0)
	}
	return 0.0
}

func (a *AutoScaler) calculateHealthPressure(raftHealth bool, nodeHealth map[string]bool) float64 {
	pressure := 0.0
	
	if !raftHealth {
		pressure += 0.5
	}
	
	unhealthyNodes := 0
	for _, healthy := range nodeHealth {
		if !healthy {
			unhealthyNodes++
		}
	}
	
	if len(nodeHealth) > 0 {
		pressure += float64(unhealthyNodes) / float64(len(nodeHealth)) * 0.5
	}
	
	return math.Min(pressure, 1.0)
}

// buildScaleUpReason builds a human-readable reason for scaling up
func (a *AutoScaler) buildScaleUpReason(factors *ScalingFactors) string {
	reasons := []string{}
	
	if factors.CPUPressure > 1.0 {
		reasons = append(reasons, fmt.Sprintf("CPU usage high (%.1f%%)", factors.CPUPressure*100))
	}
	if factors.MemoryPressure > 1.0 {
		reasons = append(reasons, fmt.Sprintf("Memory usage high (%.1f%%)", factors.MemoryPressure*100))
	}
	if factors.RequestPressure > 0.8 {
		reasons = append(reasons, "High request rate")
	}
	if factors.ResponsePressure > 0.7 {
		reasons = append(reasons, "High response time")
	}
	if factors.QueuePressure > 0.5 {
		reasons = append(reasons, "Queue backlog detected")
	}
	
	if len(reasons) == 0 {
		return "Resource pressure detected"
	}
	
	return "Scale up needed: " + joinStrings(reasons, ", ")
}

// buildScaleDownReason builds a human-readable reason for scaling down
func (a *AutoScaler) buildScaleDownReason(factors *ScalingFactors) string {
	reasons := []string{}
	
	if factors.CPUPressure < 0.3 {
		reasons = append(reasons, fmt.Sprintf("CPU usage low (%.1f%%)", factors.CPUPressure*100))
	}
	if factors.MemoryPressure < 0.4 {
		reasons = append(reasons, fmt.Sprintf("Memory usage low (%.1f%%)", factors.MemoryPressure*100))
	}
	if factors.RequestPressure < 0.3 {
		reasons = append(reasons, "Low request rate")
	}
	
	if len(reasons) == 0 {
		return "Low resource utilization"
	}
	
	return "Scale down opportunity: " + joinStrings(reasons, ", ")
}

// isInCooldownPeriod checks if the cluster is in a cooldown period
func (a *AutoScaler) isInCooldownPeriod(kvstore *kvstoreiov1.KVStore, direction ScaleDirection) bool {
	// Check the last scaling event from annotations or status
	lastScaleUpStr := kvstore.Annotations["kvstore.io/last-scale-up"]
	lastScaleDownStr := kvstore.Annotations["kvstore.io/last-scale-down"]
	
	cooldownDuration := 10 * time.Minute // Default cooldown
	
	switch direction {
	case ScaleUp:
		if lastScaleUpStr != "" {
			if lastScaleUp, err := time.Parse(time.RFC3339, lastScaleUpStr); err == nil {
				return time.Since(lastScaleUp) < cooldownDuration
			}
		}
	case ScaleDown:
		if lastScaleDownStr != "" {
			if lastScaleDown, err := time.Parse(time.RFC3339, lastScaleDownStr); err == nil {
				return time.Since(lastScaleDown) < cooldownDuration*2 // Longer cooldown for scale down
			}
		}
	}
	
	return false
}

// isClusterHealthy checks if the cluster is in a healthy state for scaling
func (a *AutoScaler) isClusterHealthy(kvstore *kvstoreiov1.KVStore) bool {
	// Check cluster conditions
	for _, condition := range kvstore.Status.Conditions {
		if condition.Type == string(KVStoreConditionReady) && condition.Status != metav1.ConditionTrue {
			return false
		}
		if condition.Type == string(KVStoreConditionDegraded) && condition.Status == metav1.ConditionTrue {
			return false
		}
	}
	
	// Check if all desired replicas are ready
	if kvstore.Status.Replicas.Ready < kvstore.Status.Replicas.Desired {
		return false
	}
	
	return true
}

// ExecuteScaling executes the scaling decision
func (a *AutoScaler) ExecuteScaling(ctx context.Context, kvstore *kvstoreiov1.KVStore, decision *ScalingDecision) error {
	log := a.Log.WithValues("kvstore", kvstore.Name, "namespace", kvstore.Namespace)
	
	if !decision.ShouldScale {
		return nil
	}
	
	log.Info("Executing scaling decision",
		"from", kvstore.Spec.Cluster.Replicas,
		"to", decision.TargetReplicas,
		"reason", decision.Reason,
	)
	
	// Update the replica count
	kvstore.Spec.Cluster.Replicas = decision.TargetReplicas
	
	// Update annotations with scaling timestamp
	if kvstore.Annotations == nil {
		kvstore.Annotations = make(map[string]string)
	}
	
	now := time.Now().Format(time.RFC3339)
	if decision.ScaleDirection == ScaleUp {
		kvstore.Annotations["kvstore.io/last-scale-up"] = now
	} else {
		kvstore.Annotations["kvstore.io/last-scale-down"] = now
	}
	
	kvstore.Annotations["kvstore.io/last-scaling-reason"] = decision.Reason
	
	// Update the resource
	if err := a.Update(ctx, kvstore); err != nil {
		a.Recorder.Event(kvstore, corev1.EventTypeWarning, "ScalingFailed", 
			fmt.Sprintf("Failed to scale from %d to %d replicas: %v", 
				kvstore.Status.Replicas.Desired, decision.TargetReplicas, err))
		return err
	}
	
	a.Recorder.Event(kvstore, corev1.EventTypeNormal, "ScalingExecuted",
		fmt.Sprintf("Scaled from %d to %d replicas. Reason: %s", 
			kvstore.Status.Replicas.Desired, decision.TargetReplicas, decision.Reason))
	
	return nil
}

// Helper function to join strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}