package server

import (
	"context"
	"fmt"
	"net/http"

	"distributed-kvstore/internal/api"
	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/storage"

	"github.com/sirupsen/logrus"
)

// HTTPServer represents the HTTP REST API server
type HTTPServer struct {
	config     *config.Config
	storage    storage.StorageEngine
	logger     *logrus.Logger
	server     *http.Server
	restHandler *api.RESTHandler
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(cfg *config.Config, storageEngine storage.StorageEngine, logger *logrus.Logger) *HTTPServer {
	restHandler := api.NewRESTHandler(storageEngine, logger)
	
	return &HTTPServer{
		config:      cfg,
		storage:     storageEngine,
		logger:      logger,
		restHandler: restHandler,
	}
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	
	router := s.restHandler.SetupRoutes()
	
	s.server = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  s.config.Server.ReadTimeout,
		WriteTimeout: s.config.Server.WriteTimeout,
		IdleTimeout:  s.config.Server.IdleTimeout,
	}

	s.logger.WithFields(logrus.Fields{
		"address": addr,
		"service": "http",
	}).Info("Starting HTTP server")

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server gracefully
func (s *HTTPServer) Stop(ctx context.Context) error {
	if s.server != nil {
		s.logger.Info("Stopping HTTP server")
		return s.server.Shutdown(ctx)
	}
	return nil
}