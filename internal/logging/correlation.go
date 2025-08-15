package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	CorrelationIDHeader = "X-Correlation-ID"
	RequestIDHeader     = "X-Request-ID"
)

// GenerateCorrelationID generates a new correlation ID
func GenerateCorrelationID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random fails
		return fmt.Sprintf("cor_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("cor_%s", hex.EncodeToString(bytes))
}

// GenerateRequestID generates a new request ID
func GenerateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random fails
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("req_%s", hex.EncodeToString(bytes))
}

// CorrelationIDMiddleware is HTTP middleware that adds correlation ID to requests
func CorrelationIDMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			
			// Get or generate correlation ID
			correlationID := r.Header.Get(CorrelationIDHeader)
			if correlationID == "" {
				correlationID = GenerateCorrelationID()
			}
			
			// Get or generate request ID
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				requestID = GenerateRequestID()
			}
			
			// Add IDs to context
			ctx = context.WithValue(ctx, CorrelationIDKey, correlationID)
			ctx = context.WithValue(ctx, RequestIDKey, requestID)
			ctx = context.WithValue(ctx, ServiceKey, "kvstore-http")
			
			// Add IDs to response headers
			w.Header().Set(CorrelationIDHeader, correlationID)
			w.Header().Set(RequestIDHeader, requestID)
			
			// Create new request with updated context
			r = r.WithContext(ctx)
			
			// Log request start
			logger.RequestStart(ctx, r.Method, r.URL.Path, r.UserAgent())
			
			// Continue with the request
			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Wrap ResponseWriter to capture status code and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     200,
				size:          0,
			}
			
			// Process request
			next.ServeHTTP(wrapped, r)
			
			// Log request completion
			duration := time.Since(start)
			logger.RequestEnd(r.Context(), r.Method, r.URL.Path, wrapped.statusCode, duration, wrapped.size)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture response details
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(data)
	rw.size += int64(size)
	return size, err
}

// ExtractCorrelationID extracts correlation ID from context
func ExtractCorrelationID(ctx context.Context) string {
	if id := ctx.Value(CorrelationIDKey); id != nil {
		if str, ok := id.(string); ok {
			return str
		}
	}
	return ""
}

// ExtractRequestID extracts request ID from context
func ExtractRequestID(ctx context.Context) string {
	if id := ctx.Value(RequestIDKey); id != nil {
		if str, ok := id.(string); ok {
			return str
		}
	}
	return ""
}

// PropagateCorrelationID propagates correlation ID to outgoing HTTP requests
func PropagateCorrelationID(ctx context.Context, req *http.Request) {
	if correlationID := ExtractCorrelationID(ctx); correlationID != "" {
		req.Header.Set(CorrelationIDHeader, correlationID)
	}
	if requestID := ExtractRequestID(ctx); requestID != "" {
		req.Header.Set(RequestIDHeader, requestID)
	}
}

// GRPCCorrelationIDFromContext extracts correlation ID for gRPC metadata
func GRPCCorrelationIDFromContext(ctx context.Context) map[string]string {
	metadata := make(map[string]string)
	
	if correlationID := ExtractCorrelationID(ctx); correlationID != "" {
		metadata["correlation-id"] = correlationID
	}
	if requestID := ExtractRequestID(ctx); requestID != "" {
		metadata["request-id"] = requestID
	}
	
	return metadata
}

// CreateContextWithIDs creates a context with correlation and request IDs
func CreateContextWithIDs(ctx context.Context, correlationID, requestID string) context.Context {
	if correlationID != "" {
		ctx = context.WithValue(ctx, CorrelationIDKey, correlationID)
	}
	if requestID != "" {
		ctx = context.WithValue(ctx, RequestIDKey, requestID)
	}
	return ctx
}

// SanitizeCorrelationID sanitizes correlation ID to prevent log injection
func SanitizeCorrelationID(id string) string {
	// Remove any characters that could be used for log injection
	id = strings.ReplaceAll(id, "\n", "")
	id = strings.ReplaceAll(id, "\r", "")
	id = strings.ReplaceAll(id, "\t", "")
	
	// Limit length to prevent extremely long IDs
	if len(id) > 64 {
		id = id[:64]
	}
	
	return id
}