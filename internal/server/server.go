package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
)

type Server struct {
	config      *config.Config
	logger      *logging.Logger
	storage     storage.StorageEngine
	grpcServer  *SimpleGRPCServer
	httpServer  *HTTPServer
	startTime   time.Time
}

func NewServer(cfg *config.Config) (*Server, error) {
	logger := logging.NewLogger(&cfg.Logging)
	
	logger.Info("Initializing server",
		"node_id", cfg.Cluster.NodeID,
		"version", "1.0.0",
	)

	storageConfig := storage.Config{
		DataPath:   cfg.Storage.DataPath,
		InMemory:   cfg.Storage.InMemory,
		SyncWrites: cfg.Storage.SyncWrites,
		ValueLogGC: cfg.Storage.ValueLogGC,
		GCInterval: cfg.Storage.GCInterval,
	}

	storageEngine, err := storage.NewEngine(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage engine: %w", err)
	}

	grpcServer := NewSimpleGRPCServer(cfg, storageEngine, logger)
	httpServer := NewHTTPServer(cfg, storageEngine, logger)

	return &Server{
		config:     cfg,
		logger:     logger,
		storage:    storageEngine,
		grpcServer: grpcServer,
		httpServer: httpServer,
		startTime:  time.Now(),
	}, nil
}

func (s *Server) Start() error {
	s.logger.Info("Starting distributed key-value store server")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)

	go func() {
		if err := s.grpcServer.Start(); err != nil {
			errChan <- fmt.Errorf("gRPC server failed: %w", err)
		}
	}()

	go func() {
		if err := s.httpServer.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server failed: %w", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	s.logger.Info("Server started successfully",
		"http_port", s.config.Server.Port,
		"grpc_port", s.config.Server.GRPCPort,
		"node_id", s.config.Cluster.NodeID,
	)

	select {
	case err := <-errChan:
		s.logger.Error("Server encountered an error", "error", err.Error())
		return err
	case sig := <-sigChan:
		s.logger.Info("Received shutdown signal", "signal", sig)
		return s.Shutdown(ctx)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		s.grpcServer.Stop()
		
		if err := s.httpServer.Stop(shutdownCtx); err != nil {
			s.logger.Error("Failed to stop HTTP server", "error", err.Error())
		}

		if err := s.storage.Close(); err != nil {
			s.logger.Error("Failed to close storage engine", "error", err.Error())
			done <- err
			return
		}

		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			s.logger.Error("Error during shutdown", "error", err.Error())
			return err
		}
		s.logger.Info("Server shutdown completed")
		return nil
	case <-shutdownCtx.Done():
		s.logger.Error("Shutdown timeout exceeded")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

func (s *Server) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

// Note: Logger setup is now handled by the logging package