package server

import (
	"context"
	"fmt"
	"net/http"

	"distributed-kvstore/internal/api"
	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/monitoring"
	"distributed-kvstore/internal/storage"
)

// HTTPServer represents the HTTP REST API server
type HTTPServer struct {
	config      *config.Config
	storage     storage.StorageEngine
	logger      *logging.Logger
	monitoring  *monitoring.MonitoringService
	server      *http.Server
	restHandler *api.RESTHandler
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(cfg *config.Config, storageEngine storage.StorageEngine, logger *logging.Logger) *HTTPServer {
	// Initialize monitoring service
	monitoringService := monitoring.NewMonitoringService(storageEngine)
	
	// Create REST handler with monitoring
	restHandler := api.NewRESTHandler(storageEngine, logger, monitoringService)
	
	return &HTTPServer{
		config:      cfg,
		storage:     storageEngine,
		logger:      logger,
		monitoring:  monitoringService,
		restHandler: restHandler,
	}
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	
	// Start monitoring service
	if s.monitoring != nil {
		s.monitoring.Start()
	}
	
	router := s.restHandler.SetupRoutes()
	
	s.server = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  s.config.Server.ReadTimeout,
		WriteTimeout: s.config.Server.WriteTimeout,
		IdleTimeout:  s.config.Server.IdleTimeout,
	}

	s.logger.Info("Starting HTTP server",
		"address", addr,
		"service", "http",
	)

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server gracefully
func (s *HTTPServer) Stop(ctx context.Context) error {
	// Stop monitoring service
	if s.monitoring != nil {
		s.monitoring.Stop()
	}
	
	if s.server != nil {
		s.logger.Info("Stopping HTTP server")
		return s.server.Shutdown(ctx)
	}
	return nil
}