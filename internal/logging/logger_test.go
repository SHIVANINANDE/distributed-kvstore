package logging

import (
	"context"
	"testing"
	"time"

	"distributed-kvstore/internal/config"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name   string
		config config.LoggingConfig
	}{
		{
			name: "development config",
			config: config.LoggingConfig{
				Level:                 "debug",
				Format:                "console",
				Output:                "stdout",
				EnableRequestTracing:  true,
				EnableCorrelationIDs:  true,
				EnableDatabaseLogging: true,
				EnablePerformanceLog:  true,
				LogSampling:           0,
			},
		},
		{
			name: "production config",
			config: config.LoggingConfig{
				Level:                 "info",
				Format:                "json",
				Output:                "stdout",
				EnableRequestTracing:  true,
				EnableCorrelationIDs:  true,
				EnableDatabaseLogging: false,
				EnablePerformanceLog:  false,
				LogSampling:           0,
			},
		},
		{
			name: "high volume config",
			config: config.LoggingConfig{
				Level:                 "warn",
				Format:                "json",
				Output:                "stdout",
				EnableRequestTracing:  true,
				EnableCorrelationIDs:  true,
				EnableDatabaseLogging: false,
				EnablePerformanceLog:  false,
				LogSampling:           10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(&tt.config)
			if logger == nil {
				t.Fatal("Expected logger to be created")
			}

			// Test basic logging
			logger.Info("Test log message", "test", true)
			logger.Debug("Debug message", "debug", true)
			logger.Warn("Warning message", "warning", true)
			logger.Error("Error message", "error", "test error")
		})
	}
}

func TestCorrelationID(t *testing.T) {
	// Test correlation ID generation
	id1 := GenerateCorrelationID()
	id2 := GenerateCorrelationID()

	if id1 == id2 {
		t.Error("Expected different correlation IDs")
	}

	if id1 == "" || id2 == "" {
		t.Error("Expected non-empty correlation IDs")
	}

	// Test context operations
	ctx := context.Background()
	ctx = CreateContextWithIDs(ctx, id1, "req123")

	extractedCorrelationID := ExtractCorrelationID(ctx)
	if extractedCorrelationID != id1 {
		t.Errorf("Expected correlation ID %s, got %s", id1, extractedCorrelationID)
	}

	extractedRequestID := ExtractRequestID(ctx)
	if extractedRequestID != "req123" {
		t.Errorf("Expected request ID req123, got %s", extractedRequestID)
	}
}

func TestLoggerWithContext(t *testing.T) {
	config := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	logger := NewLogger(&config)
	ctx := context.Background()

	// Add correlation ID to context
	correlationID := GenerateCorrelationID()
	ctx = context.WithValue(ctx, CorrelationIDKey, correlationID)

	// Test context logging
	logger.InfoContext(ctx, "Test with context", "test", "value")
	logger.DebugContext(ctx, "Debug with context", "debug", true)
	logger.WarnContext(ctx, "Warning with context", "warning", true)
	logger.ErrorContext(ctx, "Error with context", "error", "test error")
}

func TestLoggerFields(t *testing.T) {
	config := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	logger := NewLogger(&config)

	// Test WithField
	fieldLogger := logger.WithField("component", "test")
	fieldLogger.Info("Test with field")

	// Test WithFields
	fieldsLogger := logger.WithFields(map[string]interface{}{
		"component": "test",
		"version":   "1.0.0",
		"env":       "testing",
	})
	fieldsLogger.Info("Test with fields")

	// Test WithError
	testErr := &testError{message: "test error"}
	errorLogger := logger.WithError(testErr)
	errorLogger.Error("Test with error")
}

func TestSpecializedLogging(t *testing.T) {
	config := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	logger := NewLogger(&config)
	ctx := context.Background()
	ctx = CreateContextWithIDs(ctx, GenerateCorrelationID(), GenerateRequestID())

	// Test database operation logging
	logger.DatabaseOperation(ctx, "put", "test-key", 5*time.Millisecond, nil)
	logger.DatabaseOperation(ctx, "get", "test-key", 2*time.Millisecond, &testError{message: "key not found"})

	// Test cluster event logging
	logger.ClusterEvent(ctx, "node_joined", "node2", map[string]interface{}{
		"address": "127.0.0.1:7001",
		"region":  "us-west-1",
	})

	// Test security event logging
	logger.SecurityEvent(ctx, "failed_auth", "127.0.0.1", "high", map[string]interface{}{
		"attempts": 3,
		"user":     "unknown",
	})

	// Test performance logging
	logger.Performance(ctx, "request_duration", 125.5, "ms", map[string]string{
		"endpoint": "/api/v1/kv/test",
		"method":   "GET",
	})
}

func TestEnvironmentConfigs(t *testing.T) {
	tests := []struct {
		name   string
		config config.LoggingConfig
	}{
		{"development", DevelopmentLoggingConfig()},
		{"production", ProductionLoggingConfig()},
		{"test", TestLoggingConfig()},
		{"high-volume", HighVolumeLoggingConfig()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(&tt.config)
			if logger == nil {
				t.Fatal("Expected logger to be created")
			}

			// Test that logger works with the config
			logger.Info("Environment test", "environment", tt.name)
		})
	}
}

func TestCorrelationIDSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal ID",
			input:    "cor_abc123",
			expected: "cor_abc123",
		},
		{
			name:     "ID with newlines",
			input:    "cor_abc\n123",
			expected: "cor_abc123",
		},
		{
			name:     "ID with carriage returns",
			input:    "cor_abc\r123",
			expected: "cor_abc123",
		},
		{
			name:     "ID with tabs",
			input:    "cor_abc\t123",
			expected: "cor_abc123",
		},
		{
			name:     "very long ID",
			input:    "cor_" + string(make([]byte, 100)),
			expected: "cor_" + string(make([]byte, 60)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeCorrelationID(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// Helper types for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}