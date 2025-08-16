package client

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// RetryManager handles retry logic with exponential backoff
type RetryManager struct {
	mu           sync.RWMutex
	config       AdvancedClientConfig
	logger       *log.Logger
	
	// Retry statistics
	totalRetries      int64
	successfulRetries int64
	failedRetries     int64
	maxRetriesReached int64
}

// RetryStrategy defines the retry strategy
type RetryStrategy struct {
	MaxRetries     int           `json:"max_retries"`
	BaseDelay      time.Duration `json:"base_delay"`
	MaxDelay       time.Duration `json:"max_delay"`
	Multiplier     float64       `json:"multiplier"`
	Jitter         bool          `json:"jitter"`
	RetryableErrors []string     `json:"retryable_errors"`
}

// RetryStats contains statistics about retry operations
type RetryStats struct {
	TotalRetries      int64         `json:"total_retries"`
	SuccessfulRetries int64         `json:"successful_retries"`
	FailedRetries     int64         `json:"failed_retries"`
	MaxRetriesReached int64         `json:"max_retries_reached"`
	Strategy          RetryStrategy `json:"strategy"`
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation func() (interface{}, error)

// NewRetryManager creates a new retry manager
func NewRetryManager(config AdvancedClientConfig, logger *log.Logger) *RetryManager {
	if logger == nil {
		logger = log.New(log.Writer(), "[RETRY_MANAGER] ", log.LstdFlags)
	}
	
	return &RetryManager{
		config: config,
		logger: logger,
	}
}

// ExecuteWithRetry executes an operation with retry logic
func (rm *RetryManager) ExecuteWithRetry(ctx context.Context, operation RetryableOperation) (interface{}, error) {
	var lastErr error
	
	for attempt := 0; attempt <= rm.config.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// Execute the operation
		result, err := operation()
		if err == nil {
			if attempt > 0 {
				rm.recordSuccessfulRetry(attempt)
			}
			return result, nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !rm.isRetryableError(err) {
			rm.recordFailedRetry(attempt, "non-retryable error")
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}
		
		// Check if we've exhausted all retry attempts
		if attempt >= rm.config.MaxRetries {
			rm.recordMaxRetriesReached()
			rm.recordFailedRetry(attempt, "max retries reached")
			break
		}
		
		// Calculate delay for next attempt
		delay := rm.calculateDelay(attempt)
		
		rm.logger.Printf("Operation failed (attempt %d/%d), retrying in %v: %v", 
			attempt+1, rm.config.MaxRetries+1, delay, err)
		
		// Wait before next attempt
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		
		rm.recordRetryAttempt()
	}
	
	return nil, fmt.Errorf("operation failed after %d attempts: %w", rm.config.MaxRetries+1, lastErr)
}

// ExecuteWithCustomRetry executes an operation with custom retry strategy
func (rm *RetryManager) ExecuteWithCustomRetry(ctx context.Context, operation RetryableOperation, strategy RetryStrategy) (interface{}, error) {
	var lastErr error
	
	for attempt := 0; attempt <= strategy.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// Execute the operation
		result, err := operation()
		if err == nil {
			if attempt > 0 {
				rm.recordSuccessfulRetry(attempt)
			}
			return result, nil
		}
		
		lastErr = err
		
		// Check if error is retryable based on custom strategy
		if !rm.isRetryableErrorCustom(err, strategy.RetryableErrors) {
			rm.recordFailedRetry(attempt, "non-retryable error")
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}
		
		// Check if we've exhausted all retry attempts
		if attempt >= strategy.MaxRetries {
			rm.recordMaxRetriesReached()
			rm.recordFailedRetry(attempt, "max retries reached")
			break
		}
		
		// Calculate delay for next attempt using custom strategy
		delay := rm.calculateDelayCustom(attempt, strategy)
		
		rm.logger.Printf("Operation failed (attempt %d/%d), retrying in %v: %v", 
			attempt+1, strategy.MaxRetries+1, delay, err)
		
		// Wait before next attempt
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		
		rm.recordRetryAttempt()
	}
	
	return nil, fmt.Errorf("operation failed after %d attempts: %w", strategy.MaxRetries+1, lastErr)
}

// calculateDelay calculates the delay for the next retry attempt using exponential backoff
func (rm *RetryManager) calculateDelay(attempt int) time.Duration {
	// Base delay with exponential backoff
	multiplier := math.Pow(2.0, float64(attempt))
	delay := time.Duration(float64(rm.config.BaseRetryDelay) * multiplier)
	
	// Cap at maximum delay
	if delay > rm.config.MaxRetryDelay {
		delay = rm.config.MaxRetryDelay
	}
	
	// Add jitter if enabled
	if rm.config.RetryJitter {
		delay = rm.addJitter(delay)
	}
	
	return delay
}

// calculateDelayCustom calculates delay using custom strategy
func (rm *RetryManager) calculateDelayCustom(attempt int, strategy RetryStrategy) time.Duration {
	// Use custom multiplier or default to 2.0
	multiplier := strategy.Multiplier
	if multiplier <= 1.0 {
		multiplier = 2.0
	}
	
	// Calculate delay with custom parameters
	power := math.Pow(multiplier, float64(attempt))
	delay := time.Duration(float64(strategy.BaseDelay) * power)
	
	// Cap at maximum delay
	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}
	
	// Add jitter if enabled
	if strategy.Jitter {
		delay = rm.addJitter(delay)
	}
	
	return delay
}

// addJitter adds random jitter to the delay to avoid thundering herd
func (rm *RetryManager) addJitter(delay time.Duration) time.Duration {
	// Add random jitter of ±25% of the delay
	jitterRange := float64(delay) * 0.25
	jitter := (rand.Float64() - 0.5) * 2 * jitterRange
	
	finalDelay := time.Duration(float64(delay) + jitter)
	
	// Ensure delay is not negative
	if finalDelay < 0 {
		finalDelay = delay / 2
	}
	
	return finalDelay
}

// isRetryableError checks if an error is retryable
func (rm *RetryManager) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	// Define retryable error patterns
	retryablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"service unavailable",
		"circuit breaker is open",
		"leader not available",
		"node unavailable",
		"network error",
		"context deadline exceeded",
	}
	
	errStr := err.Error()
	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

// isRetryableErrorCustom checks if an error is retryable based on custom patterns
func (rm *RetryManager) isRetryableErrorCustom(err error, patterns []string) bool {
	if err == nil {
		return false
	}
	
	if len(patterns) == 0 {
		return rm.isRetryableError(err)
	}
	
	errStr := err.Error()
	for _, pattern := range patterns {
		if contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

// findSubstring finds if substring exists in string
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// recordRetryAttempt records a retry attempt
func (rm *RetryManager) recordRetryAttempt() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.totalRetries++
}

// recordSuccessfulRetry records a successful retry
func (rm *RetryManager) recordSuccessfulRetry(attempts int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.successfulRetries++
	rm.logger.Printf("Operation succeeded after %d retry attempts", attempts)
}

// recordFailedRetry records a failed retry
func (rm *RetryManager) recordFailedRetry(attempts int, reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.failedRetries++
	rm.logger.Printf("Operation failed after %d attempts, reason: %s", attempts+1, reason)
}

// recordMaxRetriesReached records when max retries are reached
func (rm *RetryManager) recordMaxRetriesReached() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.maxRetriesReached++
}

// UpdateConfig updates the retry manager configuration
func (rm *RetryManager) UpdateConfig(config AdvancedClientConfig) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.config = config
	rm.logger.Printf("Retry manager configuration updated")
}

// GetStats returns retry statistics
func (rm *RetryManager) GetStats() RetryStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	return RetryStats{
		TotalRetries:      rm.totalRetries,
		SuccessfulRetries: rm.successfulRetries,
		FailedRetries:     rm.failedRetries,
		MaxRetriesReached: rm.maxRetriesReached,
		Strategy: RetryStrategy{
			MaxRetries: rm.config.MaxRetries,
			BaseDelay:  rm.config.BaseRetryDelay,
			MaxDelay:   rm.config.MaxRetryDelay,
			Multiplier: 2.0, // Default exponential backoff multiplier
			Jitter:     rm.config.RetryJitter,
		},
	}
}

// Reset resets retry statistics
func (rm *RetryManager) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.totalRetries = 0
	rm.successfulRetries = 0
	rm.failedRetries = 0
	rm.maxRetriesReached = 0
	
	rm.logger.Printf("Retry manager statistics reset")
}

// GetSuccessRate returns the success rate of retry operations
func (rm *RetryManager) GetSuccessRate() float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	total := rm.successfulRetries + rm.failedRetries
	if total == 0 {
		return 0.0
	}
	
	return float64(rm.successfulRetries) / float64(total)
}

// CreateCustomStrategy creates a custom retry strategy
func CreateCustomStrategy(maxRetries int, baseDelay, maxDelay time.Duration, multiplier float64, jitter bool, retryableErrors []string) RetryStrategy {
	return RetryStrategy{
		MaxRetries:      maxRetries,
		BaseDelay:       baseDelay,
		MaxDelay:        maxDelay,
		Multiplier:      multiplier,
		Jitter:          jitter,
		RetryableErrors: retryableErrors,
	}
}

// GetDefaultStrategy returns the default retry strategy
func (rm *RetryManager) GetDefaultStrategy() RetryStrategy {
	return RetryStrategy{
		MaxRetries: rm.config.MaxRetries,
		BaseDelay:  rm.config.BaseRetryDelay,
		MaxDelay:   rm.config.MaxRetryDelay,
		Multiplier: 2.0,
		Jitter:     rm.config.RetryJitter,
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"temporary failure",
			"service unavailable",
			"circuit breaker is open",
		},
	}
}