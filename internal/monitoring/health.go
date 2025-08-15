package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"distributed-kvstore/internal/storage"
)

// HealthStatus represents the overall health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a single health check
type HealthCheck struct {
	Name        string                 `json:"name"`
	Status      HealthStatus           `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Duration    time.Duration          `json:"duration_ms"`
	Timestamp   time.Time              `json:"timestamp"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Critical    bool                   `json:"critical"`
	LastFailure *time.Time             `json:"last_failure,omitempty"`
}

// HealthResponse represents the complete health check response
type HealthResponse struct {
	Status       HealthStatus             `json:"status"`
	Version      string                   `json:"version"`
	Uptime       time.Duration            `json:"uptime_seconds"`
	Timestamp    time.Time                `json:"timestamp"`
	Checks       map[string]HealthCheck   `json:"checks"`
	Summary      HealthSummary            `json:"summary"`
	SystemInfo   SystemInfo               `json:"system_info"`
	Dependencies map[string]HealthCheck   `json:"dependencies,omitempty"`
}

// HealthSummary provides overall health metrics
type HealthSummary struct {
	Total      int `json:"total"`
	Healthy    int `json:"healthy"`
	Degraded   int `json:"degraded"`
	Unhealthy  int `json:"unhealthy"`
	Critical   int `json:"critical"`
}

// SystemInfo provides system-level information
type SystemInfo struct {
	GoVersion    string    `json:"go_version"`
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
	NumCPU       int       `json:"num_cpu"`
	NumGoroutine int       `json:"num_goroutine"`
	MemoryMB     uint64    `json:"memory_mb"`
	StartTime    time.Time `json:"start_time"`
}

// HealthChecker interface for implementing health checks
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) HealthCheck
	IsCritical() bool
}

// HealthManager manages all health checks
type HealthManager struct {
	checkers     []HealthChecker
	startTime    time.Time
	version      string
	lastResults  map[string]HealthCheck
}

// NewHealthManager creates a new health manager
func NewHealthManager(version string) *HealthManager {
	return &HealthManager{
		checkers:    make([]HealthChecker, 0),
		startTime:   time.Now(),
		version:     version,
		lastResults: make(map[string]HealthCheck),
	}
}

// RegisterChecker adds a health checker
func (hm *HealthManager) RegisterChecker(checker HealthChecker) {
	hm.checkers = append(hm.checkers, checker)
}

// CheckHealth performs all health checks
func (hm *HealthManager) CheckHealth(ctx context.Context) HealthResponse {
	checks := make(map[string]HealthCheck)
	summary := HealthSummary{}
	overallStatus := HealthStatusHealthy

	// Execute all health checks
	for _, checker := range hm.checkers {
		start := time.Now()
		check := checker.Check(ctx)
		check.Duration = time.Since(start)
		check.Timestamp = time.Now()
		check.Critical = checker.IsCritical()
		
		checks[checker.Name()] = check
		hm.lastResults[checker.Name()] = check

		// Update summary
		summary.Total++
		switch check.Status {
		case HealthStatusHealthy:
			summary.Healthy++
		case HealthStatusDegraded:
			summary.Degraded++
			if overallStatus == HealthStatusHealthy {
				overallStatus = HealthStatusDegraded
			}
		case HealthStatusUnhealthy:
			summary.Unhealthy++
			overallStatus = HealthStatusUnhealthy
			if check.Critical {
				summary.Critical++
			}
		}
	}

	// If any critical check fails, overall status is unhealthy
	if summary.Critical > 0 {
		overallStatus = HealthStatusUnhealthy
	}

	return HealthResponse{
		Status:     overallStatus,
		Version:    hm.version,
		Uptime:     time.Since(hm.startTime),
		Timestamp:  time.Now(),
		Checks:     checks,
		Summary:    summary,
		SystemInfo: hm.getSystemInfo(),
	}
}

// GetLastResults returns the last health check results
func (hm *HealthManager) GetLastResults() map[string]HealthCheck {
	return hm.lastResults
}

// getSystemInfo collects system information
func (hm *HealthManager) getSystemInfo() SystemInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemInfo{
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
		MemoryMB:     m.Alloc / 1024 / 1024,
		StartTime:    hm.startTime,
	}
}

// Storage Health Checker
type StorageHealthChecker struct {
	storage storage.StorageEngine
}

func NewStorageHealthChecker(storage storage.StorageEngine) *StorageHealthChecker {
	return &StorageHealthChecker{storage: storage}
}

func (s *StorageHealthChecker) Name() string {
	return "storage"
}

func (s *StorageHealthChecker) IsCritical() bool {
	return true
}

func (s *StorageHealthChecker) Check(ctx context.Context) HealthCheck {
	start := time.Now()
	
	// Test basic operations
	testKey := []byte("__health_check__")
	testValue := []byte("ok")

	// Test write
	if err := s.storage.Put(testKey, testValue); err != nil {
		return HealthCheck{
			Name:    s.Name(),
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("Storage write failed: %v", err),
			Details: map[string]interface{}{
				"operation": "put",
				"error":     err.Error(),
			},
		}
	}

	// Test read
	value, err := s.storage.Get(testKey)
	if err != nil {
		return HealthCheck{
			Name:    s.Name(),
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("Storage read failed: %v", err),
			Details: map[string]interface{}{
				"operation": "get",
				"error":     err.Error(),
			},
		}
	}

	if string(value) != string(testValue) {
		return HealthCheck{
			Name:    s.Name(),
			Status:  HealthStatusUnhealthy,
			Message: "Storage data integrity check failed",
			Details: map[string]interface{}{
				"expected": string(testValue),
				"actual":   string(value),
			},
		}
	}

	// Test delete
	if err := s.storage.Delete(testKey); err != nil {
		return HealthCheck{
			Name:    s.Name(),
			Status:  HealthStatusDegraded,
			Message: fmt.Sprintf("Storage delete failed: %v", err),
			Details: map[string]interface{}{
				"operation": "delete",
				"error":     err.Error(),
			},
		}
	}

	// Get storage stats
	stats := s.storage.Stats()
	duration := time.Since(start)

	status := HealthStatusHealthy
	if duration > 100*time.Millisecond {
		status = HealthStatusDegraded
	}

	// Extract stats - stats is already map[string]interface{}
	totalSize := stats["total_size"]
	entries := stats["entries"]

	return HealthCheck{
		Name:    s.Name(),
		Status:  status,
		Message: "Storage is operational",
		Details: map[string]interface{}{
			"operation_time_ms": duration.Milliseconds(),
			"total_size":        totalSize,
			"entries":           entries,
		},
	}
}

// Memory Health Checker
type MemoryHealthChecker struct {
	maxMemoryMB uint64
}

func NewMemoryHealthChecker(maxMemoryMB uint64) *MemoryHealthChecker {
	return &MemoryHealthChecker{maxMemoryMB: maxMemoryMB}
}

func (m *MemoryHealthChecker) Name() string {
	return "memory"
}

func (m *MemoryHealthChecker) IsCritical() bool {
	return false
}

func (m *MemoryHealthChecker) Check(ctx context.Context) HealthCheck {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	allocMB := memStats.Alloc / 1024 / 1024
	sysMB := memStats.Sys / 1024 / 1024

	status := HealthStatusHealthy
	message := "Memory usage is normal"

	if m.maxMemoryMB > 0 {
		if allocMB > m.maxMemoryMB {
			status = HealthStatusUnhealthy
			message = fmt.Sprintf("Memory usage exceeds limit (%dMB > %dMB)", allocMB, m.maxMemoryMB)
		} else if allocMB > m.maxMemoryMB*80/100 {
			status = HealthStatusDegraded
			message = fmt.Sprintf("Memory usage is high (%dMB)", allocMB)
		}
	}

	return HealthCheck{
		Name:    m.Name(),
		Status:  status,
		Message: message,
		Details: map[string]interface{}{
			"alloc_mb":      allocMB,
			"sys_mb":        sysMB,
			"num_gc":        memStats.NumGC,
			"gc_pause_ns":   memStats.PauseNs[(memStats.NumGC+255)%256],
			"num_goroutine": runtime.NumGoroutine(),
		},
	}
}

// Goroutine Health Checker
type GoroutineHealthChecker struct {
	maxGoroutines int
}

func NewGoroutineHealthChecker(maxGoroutines int) *GoroutineHealthChecker {
	return &GoroutineHealthChecker{maxGoroutines: maxGoroutines}
}

func (g *GoroutineHealthChecker) Name() string {
	return "goroutines"
}

func (g *GoroutineHealthChecker) IsCritical() bool {
	return false
}

func (g *GoroutineHealthChecker) Check(ctx context.Context) HealthCheck {
	numGoroutines := runtime.NumGoroutine()
	
	status := HealthStatusHealthy
	message := "Goroutine count is normal"

	if g.maxGoroutines > 0 {
		if numGoroutines > g.maxGoroutines {
			status = HealthStatusUnhealthy
			message = fmt.Sprintf("Too many goroutines (%d > %d)", numGoroutines, g.maxGoroutines)
		} else if numGoroutines > g.maxGoroutines*80/100 {
			status = HealthStatusDegraded
			message = fmt.Sprintf("High goroutine count (%d)", numGoroutines)
		}
	}

	return HealthCheck{
		Name:    g.Name(),
		Status:  status,
		Message: message,
		Details: map[string]interface{}{
			"count": numGoroutines,
			"limit": g.maxGoroutines,
		},
	}
}

// HTTP Dependency Health Checker
type HTTPDependencyChecker struct {
	name     string
	url      string
	timeout  time.Duration
	critical bool
	client   *http.Client
}

func NewHTTPDependencyChecker(name, url string, timeout time.Duration, critical bool) *HTTPDependencyChecker {
	return &HTTPDependencyChecker{
		name:     name,
		url:      url,
		timeout:  timeout,
		critical: critical,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (h *HTTPDependencyChecker) Name() string {
	return h.name
}

func (h *HTTPDependencyChecker) IsCritical() bool {
	return h.critical
}

func (h *HTTPDependencyChecker) Check(ctx context.Context) HealthCheck {
	start := time.Now()
	
	req, err := http.NewRequestWithContext(ctx, "GET", h.url, nil)
	if err != nil {
		return HealthCheck{
			Name:    h.Name(),
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("Failed to create request: %v", err),
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return HealthCheck{
			Name:    h.Name(),
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("HTTP request failed: %v", err),
			Details: map[string]interface{}{
				"url":   h.url,
				"error": err.Error(),
			},
		}
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	status := HealthStatusHealthy

	if resp.StatusCode >= 500 {
		status = HealthStatusUnhealthy
	} else if resp.StatusCode >= 400 || duration > h.timeout/2 {
		status = HealthStatusDegraded
	}

	return HealthCheck{
		Name:    h.Name(),
		Status:  status,
		Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		Details: map[string]interface{}{
			"url":              h.url,
			"status_code":      resp.StatusCode,
			"response_time_ms": duration.Milliseconds(),
		},
	}
}