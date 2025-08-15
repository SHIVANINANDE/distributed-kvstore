package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/monitoring"
	"distributed-kvstore/internal/storage"

	"github.com/gorilla/mux"
)

// RESTHandler handles HTTP REST API requests
type RESTHandler struct {
	storage    storage.StorageEngine
	logger     *logging.Logger
	monitoring *monitoring.MonitoringService
}

// NewRESTHandler creates a new REST API handler
func NewRESTHandler(storageEngine storage.StorageEngine, logger *logging.Logger, monitoringService *monitoring.MonitoringService) *RESTHandler {
	return &RESTHandler{
		storage:    storageEngine,
		logger:     logger,
		monitoring: monitoringService,
	}
}

// Request/Response types for JSON handling

// PutRequest represents a PUT request
type PutRequest struct {
	Value      string `json:"value"`
	TTLSeconds *int64 `json:"ttl_seconds,omitempty"`
}

// PutResponse represents a PUT response
type PutResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// GetResponse represents a GET response
type GetResponse struct {
	Found     bool   `json:"found"`
	Value     string `json:"value,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DeleteResponse represents a DELETE response
type DeleteResponse struct {
	Success bool   `json:"success"`
	Existed bool   `json:"existed"`
	Error   string `json:"error,omitempty"`
}

// ExistsResponse represents an EXISTS response
type ExistsResponse struct {
	Exists bool   `json:"exists"`
	Error  string `json:"error,omitempty"`
}

// KeyValue represents a key-value pair in responses
type KeyValue struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// ListResponse represents a LIST response
type ListResponse struct {
	Items      []KeyValue `json:"items"`
	Count      int        `json:"count"`
	HasMore    bool       `json:"has_more"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// ListKeysResponse represents a LIST KEYS response
type ListKeysResponse struct {
	Keys       []string `json:"keys"`
	Count      int      `json:"count"`
	HasMore    bool     `json:"has_more"`
	NextCursor string   `json:"next_cursor,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// BatchPutRequest represents a batch PUT request
type BatchPutRequest struct {
	Items []BatchPutItem `json:"items"`
}

// BatchPutItem represents a single item in batch PUT
type BatchPutItem struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds *int64 `json:"ttl_seconds,omitempty"`
}

// BatchGetRequest represents a batch GET request
type BatchGetRequest struct {
	Keys []string `json:"keys"`
}

// BatchDeleteRequest represents a batch DELETE request
type BatchDeleteRequest struct {
	Keys []string `json:"keys"`
}

// BatchResponse represents a batch operation response
type BatchResponse struct {
	SuccessCount int                `json:"success_count"`
	ErrorCount   int                `json:"error_count"`
	TotalCount   int                `json:"total_count"`
	SuccessRate  float64            `json:"success_rate"`
	Results      []BatchItemResult  `json:"results,omitempty"`
	Errors       []BatchItemError   `json:"errors,omitempty"`
}

// BatchItemResult represents a single result in batch GET
type BatchItemResult struct {
	Key       string `json:"key"`
	Found     bool   `json:"found"`
	Value     string `json:"value,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// BatchItemError represents an error in batch operations
type BatchItemError struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Healthy       bool   `json:"healthy"`
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Version       string `json:"version"`
	Timestamp     int64  `json:"timestamp"`
}

// StatsResponse represents a stats response
type StatsResponse struct {
	TotalKeys int64             `json:"total_keys"`
	TotalSize int64             `json:"total_size"`
	LSMSize   int64             `json:"lsm_size"`
	VLogSize  int64             `json:"vlog_size"`
	Details   map[string]string `json:"details,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// ErrorResponse represents a generic error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PUT /api/v1/kv/{key}
func (h *RESTHandler) PutKey(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		h.logger.WarnContext(ctx, "PUT request with empty key")
		h.writeErrorResponse(w, http.StatusBadRequest, "Key cannot be empty")
		return
	}

	var req PutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WarnContext(ctx, "PUT request with invalid JSON", "error", err.Error())
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	h.logger.DebugContext(ctx, "Processing PUT request",
		"key", key,
		"value_length", len(req.Value),
		"ttl_seconds", req.TTLSeconds,
	)

	err := h.storage.Put([]byte(key), []byte(req.Value))
	duration := time.Since(start)
	
	if err != nil {
		h.logger.DatabaseOperation(ctx, "put", key, duration, err)
		h.writeJSONResponse(w, http.StatusInternalServerError, PutResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.logger.DatabaseOperation(ctx, "put", key, duration, nil)
	h.writeJSONResponse(w, http.StatusOK, PutResponse{
		Success: true,
	})
}

// GET /api/v1/kv/{key}
func (h *RESTHandler) GetKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Key cannot be empty")
		return
	}

	h.logger.DebugContext(r.Context(), "Processing GET request",
		"method", "GET",
		"key", key,
	)

	value, err := h.storage.Get([]byte(key))
	if err != nil {
		if err == storage.ErrKeyNotFound {
			h.writeJSONResponse(w, http.StatusNotFound, GetResponse{
				Found: false,
			})
			return
		}

		h.logger.WithError(err).WithField("key", key).Error("Failed to get key")
		h.writeJSONResponse(w, http.StatusInternalServerError, GetResponse{
			Found: false,
			Error: err.Error(),
		})
		return
	}

	h.writeJSONResponse(w, http.StatusOK, GetResponse{
		Found:     true,
		Value:     string(value),
		CreatedAt: time.Now().Unix(),
	})
}

// DELETE /api/v1/kv/{key}
func (h *RESTHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Key cannot be empty")
		return
	}

	h.logger.DebugContext(r.Context(), "Processing DELETE request",
		"method", "DELETE",
		"key", key,
	)

	existed, err := h.storage.Exists([]byte(key))
	if err != nil {
		h.logger.WithError(err).WithField("key", key).Error("Failed to check key existence")
		h.writeJSONResponse(w, http.StatusInternalServerError, DeleteResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	err = h.storage.Delete([]byte(key))
	if err != nil {
		h.logger.WithError(err).WithField("key", key).Error("Failed to delete key")
		h.writeJSONResponse(w, http.StatusInternalServerError, DeleteResponse{
			Success: false,
			Existed: existed,
			Error:   err.Error(),
		})
		return
	}

	h.writeJSONResponse(w, http.StatusOK, DeleteResponse{
		Success: true,
		Existed: existed,
	})
}

// HEAD /api/v1/kv/{key}
func (h *RESTHandler) ExistsKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Key cannot be empty")
		return
	}

	h.logger.DebugContext(r.Context(), "Processing EXISTS request",
		"method", "HEAD",
		"key", key,
	)

	exists, err := h.storage.Exists([]byte(key))
	if err != nil {
		h.logger.WithError(err).WithField("key", key).Error("Failed to check key existence")
		h.writeJSONResponse(w, http.StatusInternalServerError, ExistsResponse{
			Exists: false,
			Error:  err.Error(),
		})
		return
	}

	if exists {
		h.writeJSONResponse(w, http.StatusOK, ExistsResponse{
			Exists: true,
		})
	} else {
		h.writeJSONResponse(w, http.StatusNotFound, ExistsResponse{
			Exists: false,
		})
	}
}

// GET /api/v1/kv?prefix={prefix}&limit={limit}&cursor={cursor}
func (h *RESTHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	limitStr := query.Get("limit")
	cursor := query.Get("cursor")
	keysOnly := query.Get("keys_only") == "true"

	limit := 100 // default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	h.logger.DebugContext(r.Context(), "Processing LIST request",
		"method", "GET",
		"prefix", prefix,
		"limit", limit,
		"cursor", cursor,
		"keys_only", keysOnly,
	)

	data, err := h.storage.List([]byte(prefix))
	if err != nil {
		h.logger.WithError(err).WithField("prefix", prefix).Error("Failed to list keys")
		if keysOnly {
			h.writeJSONResponse(w, http.StatusInternalServerError, ListKeysResponse{
				Error: err.Error(),
			})
		} else {
			h.writeJSONResponse(w, http.StatusInternalServerError, ListResponse{
				Error: err.Error(),
			})
		}
		return
	}

	if keysOnly {
		h.handleListKeysResponse(w, data, limit)
	} else {
		h.handleListResponse(w, data, limit)
	}
}

// POST /api/v1/kv/batch/put
func (h *RESTHandler) BatchPut(w http.ResponseWriter, r *http.Request) {
	var req BatchPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	h.logger.DebugContext(r.Context(), "Processing batch PUT request",
		"method", "BATCH_PUT",
		"count", len(req.Items),
	)

	successCount := 0
	errorCount := 0
	var errors []BatchItemError

	for _, item := range req.Items {
		if item.Key == "" {
			errorCount++
			errors = append(errors, BatchItemError{
				Key:   item.Key,
				Error: "key cannot be empty",
			})
			continue
		}

		err := h.storage.Put([]byte(item.Key), []byte(item.Value))
		if err != nil {
			errorCount++
			errors = append(errors, BatchItemError{
				Key:   item.Key,
				Error: err.Error(),
			})
			h.logger.WithError(err).WithField("key", item.Key).Error("Failed to put key in batch")
		} else {
			successCount++
		}
	}

	totalCount := successCount + errorCount
	successRate := 0.0
	if totalCount > 0 {
		successRate = float64(successCount) / float64(totalCount) * 100
	}

	statusCode := http.StatusOK
	if errorCount > 0 {
		if successCount == 0 {
			statusCode = http.StatusBadRequest
		} else {
			statusCode = http.StatusPartialContent
		}
	}

	h.writeJSONResponse(w, statusCode, BatchResponse{
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		TotalCount:   totalCount,
		SuccessRate:  successRate,
		Errors:       errors,
	})
}

// POST /api/v1/kv/batch/get
func (h *RESTHandler) BatchGet(w http.ResponseWriter, r *http.Request) {
	var req BatchGetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	h.logger.DebugContext(r.Context(), "Processing batch GET request",
		"method", "BATCH_GET",
		"count", len(req.Keys),
	)

	var results []BatchItemResult

	for _, key := range req.Keys {
		if key == "" {
			results = append(results, BatchItemResult{
				Key:   key,
				Found: false,
			})
			continue
		}

		value, err := h.storage.Get([]byte(key))
		if err != nil {
			if err == storage.ErrKeyNotFound {
				results = append(results, BatchItemResult{
					Key:   key,
					Found: false,
				})
			} else {
				h.logger.WithError(err).WithField("key", key).Error("Failed to get key in batch")
				h.writeJSONResponse(w, http.StatusInternalServerError, BatchResponse{
					Errors: []BatchItemError{{Key: key, Error: err.Error()}},
				})
				return
			}
		} else {
			results = append(results, BatchItemResult{
				Key:       key,
				Found:     true,
				Value:     string(value),
				CreatedAt: time.Now().Unix(),
			})
		}
	}

	h.writeJSONResponse(w, http.StatusOK, BatchResponse{
		SuccessCount: len(req.Keys),
		ErrorCount:   0,
		TotalCount:   len(req.Keys),
		SuccessRate:  100.0,
		Results:      results,
	})
}

// POST /api/v1/kv/batch/delete
func (h *RESTHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	var req BatchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	h.logger.DebugContext(r.Context(), "Processing batch DELETE request",
		"method", "BATCH_DELETE",
		"count", len(req.Keys),
	)

	successCount := 0
	errorCount := 0
	var errors []BatchItemError

	for _, key := range req.Keys {
		if key == "" {
			errorCount++
			errors = append(errors, BatchItemError{
				Key:   key,
				Error: "key cannot be empty",
			})
			continue
		}

		err := h.storage.Delete([]byte(key))
		if err != nil {
			errorCount++
			errors = append(errors, BatchItemError{
				Key:   key,
				Error: err.Error(),
			})
			h.logger.WithError(err).WithField("key", key).Error("Failed to delete key in batch")
		} else {
			successCount++
		}
	}

	totalCount := successCount + errorCount
	successRate := 0.0
	if totalCount > 0 {
		successRate = float64(successCount) / float64(totalCount) * 100
	}

	statusCode := http.StatusOK
	if errorCount > 0 {
		if successCount == 0 {
			statusCode = http.StatusBadRequest
		} else {
			statusCode = http.StatusPartialContent
		}
	}

	h.writeJSONResponse(w, statusCode, BatchResponse{
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		TotalCount:   totalCount,
		SuccessRate:  successRate,
		Errors:       errors,
	})
}

// GET /api/v1/health
func (h *RESTHandler) Health(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("Processing health check request")

	h.writeJSONResponse(w, http.StatusOK, HealthResponse{
		Healthy:       true,
		Status:        "healthy",
		UptimeSeconds: int64(time.Since(time.Now()).Seconds()),
		Version:       "1.0.0",
		Timestamp:     time.Now().Unix(),
	})
}

// GET /api/v1/stats
func (h *RESTHandler) Stats(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	includeDetails := query.Get("details") == "true"

	h.logger.WithField("include_details", includeDetails).Debug("Processing stats request")

	stats := h.storage.Stats()

	response := StatsResponse{
		TotalSize: int64(stats["total_size"].(int64)),
		LSMSize:   int64(stats["lsm_size"].(int64)),
		VLogSize:  int64(stats["vlog_size"].(int64)),
	}

	if includeDetails {
		details := make(map[string]string)
		for key, value := range stats {
			details[key] = fmt.Sprintf("%v", value)
		}
		response.Details = details
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods

func (h *RESTHandler) handleListResponse(w http.ResponseWriter, data map[string][]byte, limit int) {
	var items []KeyValue
	count := 0

	for key, value := range data {
		if limit > 0 && count >= limit {
			break
		}

		items = append(items, KeyValue{
			Key:       key,
			Value:     string(value),
			CreatedAt: time.Now().Unix(),
		})
		count++
	}

	h.writeJSONResponse(w, http.StatusOK, ListResponse{
		Items:   items,
		Count:   len(items),
		HasMore: limit > 0 && len(data) > limit,
	})
}

func (h *RESTHandler) handleListKeysResponse(w http.ResponseWriter, data map[string][]byte, limit int) {
	var keys []string
	count := 0

	for key := range data {
		if limit > 0 && count >= limit {
			break
		}
		keys = append(keys, key)
		count++
	}

	h.writeJSONResponse(w, http.StatusOK, ListKeysResponse{
		Keys:    keys,
		Count:   len(keys),
		HasMore: limit > 0 && len(data) > limit,
	})
}

func (h *RESTHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

func (h *RESTHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	h.writeJSONResponse(w, statusCode, ErrorResponse{
		Error:   message,
		Code:    statusCode,
		Message: http.StatusText(statusCode),
	})
}

// Note: Logging middleware is now handled by the logging package

// CORS middleware
func (h *RESTHandler) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}