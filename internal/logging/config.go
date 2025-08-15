package logging

import (
	"distributed-kvstore/internal/config"
)

// DevelopmentLoggingConfig returns logging configuration optimized for development
func DevelopmentLoggingConfig() config.LoggingConfig {
	return config.LoggingConfig{
		Level:                 "debug",
		Format:                "console", // Human-readable format for development
		Output:                "stdout",
		EnableRequestTracing:  true,
		EnableCorrelationIDs:  true,
		EnableDatabaseLogging: true,  // Enable detailed database logging in dev
		EnablePerformanceLog:  true,  // Enable performance logging in dev
		LogSampling:           0,     // No sampling in development
	}
}

// ProductionLoggingConfig returns logging configuration optimized for production
func ProductionLoggingConfig() config.LoggingConfig {
	return config.LoggingConfig{
		Level:                 "info",
		Format:                "json", // Machine-readable format for production
		Output:                "stdout",
		EnableRequestTracing:  true,
		EnableCorrelationIDs:  true,
		EnableDatabaseLogging: false, // Reduce noise in production
		EnablePerformanceLog:  false, // Reduce noise in production
		LogSampling:           0,     // No sampling by default, can be configured
	}
}

// TestLoggingConfig returns logging configuration optimized for testing
func TestLoggingConfig() config.LoggingConfig {
	return config.LoggingConfig{
		Level:                 "error", // Minimal logging during tests
		Format:                "json",
		Output:                "stderr",
		EnableRequestTracing:  false,
		EnableCorrelationIDs:  false,
		EnableDatabaseLogging: false,
		EnablePerformanceLog:  false,
		LogSampling:           0,
	}
}

// HighVolumeLoggingConfig returns logging configuration for high-volume environments
func HighVolumeLoggingConfig() config.LoggingConfig {
	return config.LoggingConfig{
		Level:                 "warn",
		Format:                "json",
		Output:                "stdout",
		EnableRequestTracing:  true,
		EnableCorrelationIDs:  true,
		EnableDatabaseLogging: false,
		EnablePerformanceLog:  false,
		LogSampling:           10, // Sample every 10th log to reduce volume
	}
}

// SetupEnvironmentLogging configures logging based on environment
func SetupEnvironmentLogging(cfg *config.Config, environment string) {
	switch environment {
	case "development", "dev":
		cfg.Logging = DevelopmentLoggingConfig()
	case "production", "prod":
		cfg.Logging = ProductionLoggingConfig()
	case "test", "testing":
		cfg.Logging = TestLoggingConfig()
	case "staging", "stage":
		prodConfig := ProductionLoggingConfig()
		prodConfig.Level = "debug" // More verbose logging in staging
		cfg.Logging = prodConfig
	case "high-volume":
		cfg.Logging = HighVolumeLoggingConfig()
	}
}