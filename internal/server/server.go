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
	"distributed-kvstore/internal/storage"

	"github.com/sirupsen/logrus"
)

type Server struct {
	config      *config.Config
	logger      *logrus.Logger
	storage     storage.StorageEngine
	grpcServer  *GRPCServer
	httpServer  *HTTPServer
	startTime   time.Time
}

func NewServer(cfg *config.Config) (*Server, error) {
	logger := setupLogger(cfg)
	
	logger.WithFields(logrus.Fields{
		"node_id": cfg.Cluster.NodeID,
		"version": "1.0.0",
	}).Info("Initializing server")

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

	grpcServer := NewGRPCServer(cfg, storageEngine, logger)
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

	s.logger.WithFields(logrus.Fields{
		"http_port": s.config.Server.Port,
		"grpc_port": s.config.Server.GRPCPort,
		"node_id":   s.config.Cluster.NodeID,
	}).Info("Server started successfully")

	select {
	case err := <-errChan:
		s.logger.WithError(err).Error("Server encountered an error")
		return err
	case sig := <-sigChan:
		s.logger.WithField("signal", sig).Info("Received shutdown signal")
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
			s.logger.WithError(err).Error("Failed to stop HTTP server")
		}

		if err := s.storage.Close(); err != nil {
			s.logger.WithError(err).Error("Failed to close storage engine")
			done <- err
			return
		}

		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			s.logger.WithError(err).Error("Error during shutdown")
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

func setupLogger(cfg *config.Config) *logrus.Logger {
	logger := logrus.New()

	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	switch cfg.Logging.Format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	case "console":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "15:04:05",
			DisableColors:   false,
		})
	default:
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	}

	switch cfg.Logging.Output {
	case "stdout":
		logger.SetOutput(os.Stdout)
	case "stderr":
		logger.SetOutput(os.Stderr)
	default:
		if cfg.Logging.Output != "" {
			file, err := os.OpenFile(cfg.Logging.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				logger.SetOutput(file)
			} else {
				logger.SetOutput(os.Stdout)
				logger.WithError(err).WithField("output", cfg.Logging.Output).Warn("Failed to open log file, using stdout")
			}
		} else {
			logger.SetOutput(os.Stdout)
		}
	}

	return logger
}