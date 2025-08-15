package api

import (
	"net/http"

	"distributed-kvstore/internal/logging"

	"github.com/gorilla/mux"
)

// SetupRoutes configures all REST API routes
func (h *RESTHandler) SetupRoutes() *mux.Router {
	router := mux.NewRouter()

	// Apply middleware
	router.Use(logging.CorrelationIDMiddleware(h.logger))
	router.Use(logging.LoggingMiddleware(h.logger))
	if h.monitoring != nil {
		router.Use(h.monitoring.MonitoringMiddleware)
	}
	router.Use(h.CORSMiddleware)

	// API version 1
	v1 := router.PathPrefix("/api/v1").Subrouter()

	// Key-value operations
	v1.HandleFunc("/kv/{key}", h.PutKey).Methods(http.MethodPut)
	v1.HandleFunc("/kv/{key}", h.GetKey).Methods(http.MethodGet)
	v1.HandleFunc("/kv/{key}", h.DeleteKey).Methods(http.MethodDelete)
	v1.HandleFunc("/kv/{key}", h.ExistsKey).Methods(http.MethodHead)

	// List operations
	v1.HandleFunc("/kv", h.ListKeys).Methods(http.MethodGet)

	// Batch operations
	v1.HandleFunc("/kv/batch/put", h.BatchPut).Methods(http.MethodPost)
	v1.HandleFunc("/kv/batch/get", h.BatchGet).Methods(http.MethodPost)
	v1.HandleFunc("/kv/batch/delete", h.BatchDelete).Methods(http.MethodPost)

	// Health and stats (enhanced)
	if h.monitoring != nil {
		v1.HandleFunc("/health", h.monitoring.GetHealthHandler()).Methods(http.MethodGet)
	} else {
		v1.HandleFunc("/health", h.Health).Methods(http.MethodGet)
	}
	v1.HandleFunc("/stats", h.Stats).Methods(http.MethodGet)
	
	// Monitoring endpoints
	if h.monitoring != nil {
		v1.Handle("/metrics", h.monitoring.GetMetricsHandler()).Methods(http.MethodGet)
		router.Handle("/dashboard", h.monitoring.GetDashboardHandler()).Methods(http.MethodGet)
		router.Handle("/dashboard/", h.monitoring.GetDashboardHandler()).Methods(http.MethodGet)
	}

	// Handle OPTIONS for all routes (CORS preflight)
	v1.Methods(http.MethodOptions).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Root endpoints
	if h.monitoring != nil {
		router.HandleFunc("/health", h.monitoring.GetHealthHandler()).Methods(http.MethodGet)
	} else {
		router.HandleFunc("/health", h.Health).Methods(http.MethodGet)
	}
	router.HandleFunc("/", h.RootHandler).Methods(http.MethodGet)

	return router
}

// RootHandler handles requests to the root path
func (h *RESTHandler) RootHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service": "Distributed Key-Value Store",
		"version": "1.0.0",
		"api_version": "v1",
		"endpoints": map[string]interface{}{
			"health": "/health or /api/v1/health",
			"stats":  "/api/v1/stats",
			"kv_operations": map[string]string{
				"put":    "PUT /api/v1/kv/{key}",
				"get":    "GET /api/v1/kv/{key}",
				"delete": "DELETE /api/v1/kv/{key}",
				"exists": "HEAD /api/v1/kv/{key}",
				"list":   "GET /api/v1/kv?prefix={prefix}&limit={limit}&keys_only={true|false}",
			},
			"batch_operations": map[string]string{
				"batch_put":    "POST /api/v1/kv/batch/put",
				"batch_get":    "POST /api/v1/kv/batch/get",
				"batch_delete": "POST /api/v1/kv/batch/delete",
			},
		},
		"documentation": "/api/v1/docs",
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}