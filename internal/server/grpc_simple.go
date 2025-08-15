package server

import (
	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
)

// SimpleGRPCServer represents a simplified gRPC server for demo purposes
type SimpleGRPCServer struct {
	config  *config.Config
	storage storage.StorageEngine
	logger  *logging.Logger
}

// NewSimpleGRPCServer creates a new simple gRPC server
func NewSimpleGRPCServer(cfg *config.Config, storageEngine storage.StorageEngine, logger *logging.Logger) *SimpleGRPCServer {
	return &SimpleGRPCServer{
		config:  cfg,
		storage: storageEngine,
		logger:  logger,
	}
}

// Start starts the simple gRPC server
func (s *SimpleGRPCServer) Start() error {
	s.logger.Info("Simple gRPC server would start here",
		"address", s.config.Server.Host+":"+string(rune(s.config.Server.GRPCPort)),
		"service", "grpc",
	)
	// For demo purposes, we'll just log that it would start
	return nil
}

// Stop stops the simple gRPC server
func (s *SimpleGRPCServer) Stop() {
	s.logger.Info("Simple gRPC server stopped")
}