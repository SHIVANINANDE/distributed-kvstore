package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"distributed-kvstore/internal/config"
)

type Logger struct {
	*slog.Logger
	config *config.LoggingConfig
}

type ContextKey string

const (
	CorrelationIDKey ContextKey = "correlation_id"
	RequestIDKey     ContextKey = "request_id"
	UserIDKey        ContextKey = "user_id"
	ServiceKey       ContextKey = "service"
)

// NewLogger creates a new structured logger using slog
func NewLogger(cfg *config.LoggingConfig) *Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var writer io.Writer
	switch cfg.Output {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		if cfg.Output != "" {
			file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				writer = file
			} else {
				writer = os.Stdout
				slog.Warn("Failed to open log file, using stdout", "error", err, "file", cfg.Output)
			}
		} else {
			writer = os.Stdout
		}
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
		AddSource: level == slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize timestamp format
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339))
			}
			return a
		},
	}

	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	case "text", "console":
		handler = slog.NewTextHandler(writer, opts)
	default:
		// Default to JSON for production
		handler = slog.NewJSONHandler(writer, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return &Logger{
		Logger: logger,
		config: cfg,
	}
}

// WithContext creates a new logger with context values
func (l *Logger) WithContext(ctx context.Context) *Logger {
	logger := l.Logger
	
	// Add correlation ID if present
	if correlationID := ctx.Value(CorrelationIDKey); correlationID != nil {
		logger = logger.With("correlation_id", correlationID)
	}
	
	// Add request ID if present
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		logger = logger.With("request_id", requestID)
	}
	
	// Add user ID if present
	if userID := ctx.Value(UserIDKey); userID != nil {
		logger = logger.With("user_id", userID)
	}
	
	// Add service name if present
	if service := ctx.Value(ServiceKey); service != nil {
		logger = logger.With("service", service)
	}

	return &Logger{
		Logger: logger,
		config: l.config,
	}
}

// WithFields creates a new logger with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	var args []interface{}
	for key, value := range fields {
		args = append(args, key, value)
	}
	
	return &Logger{
		Logger: l.Logger.With(args...),
		config: l.config,
	}
}

// WithField creates a new logger with a single additional field
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		Logger: l.Logger.With(key, value),
		config: l.config,
	}
}

// WithError creates a new logger with an error field
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		Logger: l.Logger.With("error", err.Error()),
		config: l.config,
	}
}

// DebugContext logs a debug message with context
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...interface{}) {
	l.WithContext(ctx).Debug(msg, args...)
}

// InfoContext logs an info message with context
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...interface{}) {
	l.WithContext(ctx).Info(msg, args...)
}

// WarnContext logs a warning message with context
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...interface{}) {
	l.WithContext(ctx).Warn(msg, args...)
}

// ErrorContext logs an error message with context
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...interface{}) {
	l.WithContext(ctx).Error(msg, args...)
}

// RequestStart logs the start of a request
func (l *Logger) RequestStart(ctx context.Context, method, path, userAgent string) {
	l.WithContext(ctx).Info("Request started",
		"method", method,
		"path", path,
		"user_agent", userAgent,
	)
}

// RequestEnd logs the end of a request
func (l *Logger) RequestEnd(ctx context.Context, method, path string, statusCode int, duration time.Duration, size int64) {
	level := slog.LevelInfo
	if statusCode >= 400 && statusCode < 500 {
		level = slog.LevelWarn
	} else if statusCode >= 500 {
		level = slog.LevelError
	}

	l.WithContext(ctx).Log(ctx, level, "Request completed",
		"method", method,
		"path", path,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
		"response_size", size,
	)
}

// DatabaseOperation logs database operations
func (l *Logger) DatabaseOperation(ctx context.Context, operation, key string, duration time.Duration, err error) {
	logger := l.WithContext(ctx).With(
		"operation", operation,
		"key", key,
		"duration_ms", duration.Milliseconds(),
	)

	if err != nil {
		logger.Error("Database operation failed", "error", err.Error())
	} else {
		logger.Debug("Database operation completed")
	}
}

// ClusterEvent logs cluster-related events
func (l *Logger) ClusterEvent(ctx context.Context, event, nodeID string, details map[string]interface{}) {
	args := []interface{}{
		"event", event,
		"node_id", nodeID,
	}
	
	for key, value := range details {
		args = append(args, key, value)
	}

	l.WithContext(ctx).Info("Cluster event", args...)
}

// SecurityEvent logs security-related events
func (l *Logger) SecurityEvent(ctx context.Context, event, source string, severity string, details map[string]interface{}) {
	args := []interface{}{
		"event", event,
		"source", source,
		"severity", severity,
	}
	
	for key, value := range details {
		args = append(args, key, value)
	}

	var level slog.Level
	switch severity {
	case "low":
		level = slog.LevelInfo
	case "medium":
		level = slog.LevelWarn
	case "high", "critical":
		level = slog.LevelError
	default:
		level = slog.LevelWarn
	}

	l.WithContext(ctx).Log(ctx, level, "Security event", args...)
}

// Performance logs performance metrics
func (l *Logger) Performance(ctx context.Context, metric string, value float64, unit string, tags map[string]string) {
	args := []interface{}{
		"metric", metric,
		"value", value,
		"unit", unit,
	}
	
	for key, value := range tags {
		args = append(args, "tag_"+key, value)
	}

	l.WithContext(ctx).Info("Performance metric", args...)
}