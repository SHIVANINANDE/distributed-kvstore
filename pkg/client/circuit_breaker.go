package client

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState string

const (
	CircuitBreakerClosed    CircuitBreakerState = "closed"     // Normal operation
	CircuitBreakerOpen      CircuitBreakerState = "open"       // Circuit is open, requests fail fast
	CircuitBreakerHalfOpen  CircuitBreakerState = "half_open"  // Testing if service has recovered
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu                  sync.RWMutex
	config              AdvancedClientConfig
	logger              *log.Logger
	
	// Circuit breaker state
	state               CircuitBreakerState
	failureCount        int
	successCount        int
	lastFailureTime     time.Time
	lastStateChange     time.Time
	halfOpenRequests    int
	
	// Configuration
	failureThreshold    int
	recoveryTimeout     time.Duration
	halfOpenMaxRequests int
	
	// Statistics
	totalRequests       int64
	totalFailures       int64
	totalSuccesses      int64
	stateChanges        int64
}

// CircuitBreakerStats contains statistics about the circuit breaker
type CircuitBreakerStats struct {
	State               CircuitBreakerState `json:"state"`
	FailureCount        int                 `json:"failure_count"`
	SuccessCount        int                 `json:"success_count"`
	LastFailureTime     time.Time           `json:"last_failure_time"`
	LastStateChange     time.Time           `json:"last_state_change"`
	HalfOpenRequests    int                 `json:"half_open_requests"`
	TotalRequests       int64               `json:"total_requests"`
	TotalFailures       int64               `json:"total_failures"`
	TotalSuccesses      int64               `json:"total_successes"`
	StateChanges        int64               `json:"state_changes"`
	FailureThreshold    int                 `json:"failure_threshold"`
	RecoveryTimeout     time.Duration       `json:"recovery_timeout"`
	HalfOpenMaxRequests int                 `json:"half_open_max_requests"`
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config AdvancedClientConfig, logger *log.Logger) *CircuitBreaker {
	if logger == nil {
		logger = log.New(log.Writer(), "[CIRCUIT_BREAKER] ", log.LstdFlags)
	}
	
	return &CircuitBreaker{
		config:              config,
		logger:              logger,
		state:               CircuitBreakerClosed,
		failureThreshold:    config.FailureThreshold,
		recoveryTimeout:     config.RecoveryTimeout,
		halfOpenMaxRequests: config.HalfOpenMaxRequests,
		lastStateChange:     time.Now(),
	}
}

// CanExecute checks if a request can be executed
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.totalRequests++
	
	switch cb.state {
	case CircuitBreakerClosed:
		return true
		
	case CircuitBreakerOpen:
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
			cb.transitionToHalfOpen()
			return true
		}
		return false
		
	case CircuitBreakerHalfOpen:
		// Allow limited requests to test if service has recovered
		if cb.halfOpenRequests < cb.halfOpenMaxRequests {
			cb.halfOpenRequests++
			return true
		}
		return false
		
	default:
		return false
	}
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.totalSuccesses++
	cb.successCount++
	
	switch cb.state {
	case CircuitBreakerHalfOpen:
		// If we've had enough successful requests in half-open state, close the circuit
		if cb.successCount >= cb.halfOpenMaxRequests {
			cb.transitionToClosed()
		}
		
	case CircuitBreakerClosed:
		// Reset failure count on successful request
		if cb.failureCount > 0 {
			cb.failureCount = 0
		}
	}
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.totalFailures++
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	
	switch cb.state {
	case CircuitBreakerClosed:
		// Check if we should open the circuit
		if cb.failureCount >= cb.failureThreshold {
			cb.transitionToOpen()
		}
		
	case CircuitBreakerHalfOpen:
		// Any failure in half-open state should open the circuit immediately
		cb.transitionToOpen()
	}
}

// transitionToClosed transitions the circuit breaker to closed state
func (cb *CircuitBreaker) transitionToClosed() {
	cb.logger.Printf("Circuit breaker transitioning to CLOSED state")
	cb.state = CircuitBreakerClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
	cb.stateChanges++
}

// transitionToOpen transitions the circuit breaker to open state
func (cb *CircuitBreaker) transitionToOpen() {
	cb.logger.Printf("Circuit breaker transitioning to OPEN state (failures: %d)", cb.failureCount)
	cb.state = CircuitBreakerOpen
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
	cb.stateChanges++
}

// transitionToHalfOpen transitions the circuit breaker to half-open state
func (cb *CircuitBreaker) transitionToHalfOpen() {
	cb.logger.Printf("Circuit breaker transitioning to HALF_OPEN state")
	cb.state = CircuitBreakerHalfOpen
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
	cb.stateChanges++
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to its initial state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.logger.Printf("Circuit breaker reset")
	cb.state = CircuitBreakerClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
	cb.stateChanges++
}

// UpdateConfig updates the circuit breaker configuration
func (cb *CircuitBreaker) UpdateConfig(config AdvancedClientConfig) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.config = config
	cb.failureThreshold = config.FailureThreshold
	cb.recoveryTimeout = config.RecoveryTimeout
	cb.halfOpenMaxRequests = config.HalfOpenMaxRequests
	
	cb.logger.Printf("Circuit breaker configuration updated")
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	return CircuitBreakerStats{
		State:               cb.state,
		FailureCount:        cb.failureCount,
		SuccessCount:        cb.successCount,
		LastFailureTime:     cb.lastFailureTime,
		LastStateChange:     cb.lastStateChange,
		HalfOpenRequests:    cb.halfOpenRequests,
		TotalRequests:       cb.totalRequests,
		TotalFailures:       cb.totalFailures,
		TotalSuccesses:      cb.totalSuccesses,
		StateChanges:        cb.stateChanges,
		FailureThreshold:    cb.failureThreshold,
		RecoveryTimeout:     cb.recoveryTimeout,
		HalfOpenMaxRequests: cb.halfOpenMaxRequests,
	}
}

// IsHealthy returns true if the circuit breaker is in a healthy state
func (cb *CircuitBreaker) IsHealthy() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerHalfOpen:
		return cb.successCount > 0
	case CircuitBreakerOpen:
		return false
	default:
		return false
	}
}

// GetFailureRate returns the current failure rate
func (cb *CircuitBreaker) GetFailureRate() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	if cb.totalRequests == 0 {
		return 0.0
	}
	
	return float64(cb.totalFailures) / float64(cb.totalRequests)
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.CanExecute() {
		return fmt.Errorf("circuit breaker is open")
	}
	
	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}
	
	cb.RecordSuccess()
	return nil
}

// ExecuteWithFallback executes a function with circuit breaker protection and fallback
func (cb *CircuitBreaker) ExecuteWithFallback(fn func() error, fallback func() error) error {
	if !cb.CanExecute() {
		if fallback != nil {
			return fallback()
		}
		return fmt.Errorf("circuit breaker is open and no fallback provided")
	}
	
	err := fn()
	if err != nil {
		cb.RecordFailure()
		if fallback != nil {
			return fallback()
		}
		return err
	}
	
	cb.RecordSuccess()
	return nil
}