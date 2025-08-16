package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/logging"
	"distributed-kvstore/internal/storage"
	"distributed-kvstore/proto/kvstore"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServer implements the KVStore gRPC service
type GRPCServer struct {
	kvstore.UnimplementedKVStoreServer
	
	config   *config.Config
	storage  storage.StorageEngine
	logger   *logging.Logger
	server   *grpc.Server
	listener net.Listener
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(cfg *config.Config, storageEngine storage.StorageEngine, logger *logging.Logger) *GRPCServer {
	return &GRPCServer{
		config:  cfg,
		storage: storageEngine,
		logger:  logger,
	}
}

// Start starts the gRPC server
func (s *GRPCServer) Start() error {
	address := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.GRPCPort)
	
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}
	s.listener = listener
	
	// Create gRPC server with options
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(s.loggingInterceptor),
	}
	
	s.server = grpc.NewServer(opts...)
	kvstore.RegisterKVStoreServer(s.server, s)
	
	s.logger.Info("Starting gRPC server", "address", address)
	
	go func() {
		if err := s.server.Serve(listener); err != nil {
			s.logger.Error("gRPC server failed", "error", err)
		}
	}()
	
	return nil
}

// Stop stops the gRPC server
func (s *GRPCServer) Stop() {
	s.logger.Info("Stopping gRPC server")
	
	if s.server != nil {
		s.server.GracefulStop()
	}
	
	if s.listener != nil {
		s.listener.Close()
	}
}

// Put implements the Put RPC
func (s *GRPCServer) Put(ctx context.Context, req *kvstore.PutRequest) (*kvstore.PutResponse, error) {
	s.logger.DebugContext(ctx, "Put request", "key", req.Key)
	
	if req.Key == "" {
		return &kvstore.PutResponse{
			Success: false,
			Error:   "key cannot be empty",
		}, nil
	}
	
	err := s.storage.Put([]byte(req.Key), req.Value)
	if err != nil {
		s.logger.ErrorContext(ctx, "Put failed", "key", req.Key, "error", err)
		return &kvstore.PutResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	
	return &kvstore.PutResponse{
		Success: true,
	}, nil
}

// Get implements the Get RPC
func (s *GRPCServer) Get(ctx context.Context, req *kvstore.GetRequest) (*kvstore.GetResponse, error) {
	s.logger.DebugContext(ctx, "Get request", "key", req.Key)
	
	if req.Key == "" {
		return nil, status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	
	value, err := s.storage.Get([]byte(req.Key))
	if err != nil {
		// Check if it's a "key not found" error
		if err.Error() == "Key not found" { // This should match your storage engine's error
			return &kvstore.GetResponse{
				Found: false,
			}, nil
		}
		
		s.logger.ErrorContext(ctx, "Get failed", "key", req.Key, "error", err)
		return &kvstore.GetResponse{
			Found: false,
			Error: err.Error(),
		}, nil
	}
	
	return &kvstore.GetResponse{
		Found:     true,
		Value:     value,
		CreatedAt: time.Now().Unix(), // In a real implementation, this would come from storage
	}, nil
}

// Delete implements the Delete RPC
func (s *GRPCServer) Delete(ctx context.Context, req *kvstore.DeleteRequest) (*kvstore.DeleteResponse, error) {
	s.logger.DebugContext(ctx, "Delete request", "key", req.Key)
	
	if req.Key == "" {
		return nil, status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	
	// Check if key exists before deletion
	existed, err := s.storage.Exists([]byte(req.Key))
	if err != nil {
		s.logger.ErrorContext(ctx, "Exists check failed", "key", req.Key, "error", err)
		return &kvstore.DeleteResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	
	err = s.storage.Delete([]byte(req.Key))
	if err != nil {
		s.logger.ErrorContext(ctx, "Delete failed", "key", req.Key, "error", err)
		return &kvstore.DeleteResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	
	return &kvstore.DeleteResponse{
		Success: true,
		Existed: existed,
	}, nil
}

// Exists implements the Exists RPC
func (s *GRPCServer) Exists(ctx context.Context, req *kvstore.ExistsRequest) (*kvstore.ExistsResponse, error) {
	s.logger.DebugContext(ctx, "Exists request", "key", req.Key)
	
	if req.Key == "" {
		return nil, status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	
	exists, err := s.storage.Exists([]byte(req.Key))
	if err != nil {
		s.logger.ErrorContext(ctx, "Exists failed", "key", req.Key, "error", err)
		return &kvstore.ExistsResponse{
			Exists: false,
			Error:  err.Error(),
		}, nil
	}
	
	return &kvstore.ExistsResponse{
		Exists: exists,
	}, nil
}

// List implements the List RPC
func (s *GRPCServer) List(ctx context.Context, req *kvstore.ListRequest) (*kvstore.ListResponse, error) {
	s.logger.DebugContext(ctx, "List request", "prefix", req.Prefix, "limit", req.Limit)
	
	items, err := s.storage.List([]byte(req.Prefix))
	if err != nil {
		s.logger.ErrorContext(ctx, "List failed", "prefix", req.Prefix, "error", err)
		return &kvstore.ListResponse{
			Error: err.Error(),
		}, nil
	}
	
	// Convert map to protobuf KeyValue slice
	var kvItems []*kvstore.KeyValue
	count := 0
	limit := int(req.Limit)
	
	for key, value := range items {
		if limit > 0 && count >= limit {
			break
		}
		
		kvItems = append(kvItems, &kvstore.KeyValue{
			Key:       key,
			Value:     value,
			CreatedAt: time.Now().Unix(), // In a real implementation, this would come from storage
		})
		count++
	}
	
	return &kvstore.ListResponse{
		Items:   kvItems,
		HasMore: limit > 0 && len(items) > limit,
	}, nil
}

// ListKeys implements the ListKeys RPC
func (s *GRPCServer) ListKeys(ctx context.Context, req *kvstore.ListKeysRequest) (*kvstore.ListKeysResponse, error) {
	s.logger.DebugContext(ctx, "ListKeys request", "prefix", req.Prefix, "limit", req.Limit)
	
	items, err := s.storage.List([]byte(req.Prefix))
	if err != nil {
		s.logger.ErrorContext(ctx, "ListKeys failed", "prefix", req.Prefix, "error", err)
		return &kvstore.ListKeysResponse{
			Error: err.Error(),
		}, nil
	}
	
	// Extract keys only
	var keys []string
	count := 0
	limit := int(req.Limit)
	
	for key := range items {
		if limit > 0 && count >= limit {
			break
		}
		keys = append(keys, key)
		count++
	}
	
	return &kvstore.ListKeysResponse{
		Keys:    keys,
		HasMore: limit > 0 && len(items) > limit,
	}, nil
}

// BatchPut implements the BatchPut RPC
func (s *GRPCServer) BatchPut(ctx context.Context, req *kvstore.BatchPutRequest) (*kvstore.BatchPutResponse, error) {
	s.logger.DebugContext(ctx, "BatchPut request", "items", len(req.Items))
	
	var successCount int32
	var errors []*kvstore.BatchError
	
	for _, item := range req.Items {
		if item.Key == "" {
			errors = append(errors, &kvstore.BatchError{
				Key:   item.Key,
				Error: "key cannot be empty",
			})
			continue
		}
		
		err := s.storage.Put([]byte(item.Key), item.Value)
		if err != nil {
			errors = append(errors, &kvstore.BatchError{
				Key:   item.Key,
				Error: err.Error(),
			})
		} else {
			successCount++
		}
	}
	
	return &kvstore.BatchPutResponse{
		SuccessCount: successCount,
		ErrorCount:   int32(len(errors)),
		Errors:       errors,
	}, nil
}

// BatchGet implements the BatchGet RPC
func (s *GRPCServer) BatchGet(ctx context.Context, req *kvstore.BatchGetRequest) (*kvstore.BatchGetResponse, error) {
	s.logger.DebugContext(ctx, "BatchGet request", "keys", len(req.Keys))
	
	var results []*kvstore.GetResult
	
	for _, key := range req.Keys {
		if key == "" {
			results = append(results, &kvstore.GetResult{
				Key:   key,
				Found: false,
			})
			continue
		}
		
		value, err := s.storage.Get([]byte(key))
		if err != nil {
			results = append(results, &kvstore.GetResult{
				Key:   key,
				Found: false,
			})
		} else {
			results = append(results, &kvstore.GetResult{
				Key:       key,
				Found:     true,
				Value:     value,
				CreatedAt: time.Now().Unix(),
			})
		}
	}
	
	return &kvstore.BatchGetResponse{
		Results: results,
	}, nil
}

// BatchDelete implements the BatchDelete RPC
func (s *GRPCServer) BatchDelete(ctx context.Context, req *kvstore.BatchDeleteRequest) (*kvstore.BatchDeleteResponse, error) {
	s.logger.DebugContext(ctx, "BatchDelete request", "keys", len(req.Keys))
	
	var successCount int32
	var errors []*kvstore.BatchError
	
	for _, key := range req.Keys {
		if key == "" {
			errors = append(errors, &kvstore.BatchError{
				Key:   key,
				Error: "key cannot be empty",
			})
			continue
		}
		
		err := s.storage.Delete([]byte(key))
		if err != nil {
			errors = append(errors, &kvstore.BatchError{
				Key:   key,
				Error: err.Error(),
			})
		} else {
			successCount++
		}
	}
	
	return &kvstore.BatchDeleteResponse{
		SuccessCount: successCount,
		ErrorCount:   int32(len(errors)),
		Errors:       errors,
	}, nil
}

// Health implements the Health RPC
func (s *GRPCServer) Health(ctx context.Context, req *kvstore.HealthRequest) (*kvstore.HealthResponse, error) {
	return &kvstore.HealthResponse{
		Healthy:       true,
		Status:        "healthy",
		UptimeSeconds: int64(time.Since(time.Now()).Seconds()), // This should track actual uptime
		Version:       "1.0.0",
	}, nil
}

// Stats implements the Stats RPC
func (s *GRPCServer) Stats(ctx context.Context, req *kvstore.StatsRequest) (*kvstore.StatsResponse, error) {
	stats := s.storage.Stats()
	
	// Convert storage stats to protobuf format
	details := make(map[string]string)
	if req.IncludeDetails {
		for key, value := range stats {
			details[key] = fmt.Sprintf("%v", value)
		}
	}
	
	return &kvstore.StatsResponse{
		TotalKeys: 0,    // This should be extracted from stats
		TotalSize: 0,    // This should be extracted from stats  
		LsmSize:   0,    // This should be extracted from stats
		VlogSize:  0,    // This should be extracted from stats
		Details:   details,
	}, nil
}

// Backup implements the Backup RPC
func (s *GRPCServer) Backup(ctx context.Context, req *kvstore.BackupRequest) (*kvstore.BackupResponse, error) {
	s.logger.InfoContext(ctx, "Backup request", "path", req.Path)
	
	err := s.storage.Backup(req.Path)
	if err != nil {
		s.logger.ErrorContext(ctx, "Backup failed", "path", req.Path, "error", err)
		return &kvstore.BackupResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	
	return &kvstore.BackupResponse{
		Success: true,
		Path:    req.Path,
	}, nil
}

// Restore implements the Restore RPC
func (s *GRPCServer) Restore(ctx context.Context, req *kvstore.RestoreRequest) (*kvstore.RestoreResponse, error) {
	s.logger.InfoContext(ctx, "Restore request", "path", req.Path)
	
	err := s.storage.Restore(req.Path)
	if err != nil {
		s.logger.ErrorContext(ctx, "Restore failed", "path", req.Path, "error", err)
		return &kvstore.RestoreResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	
	return &kvstore.RestoreResponse{
		Success: true,
	}, nil
}

// Status implements the Status RPC (placeholder for cluster functionality)
func (s *GRPCServer) Status(ctx context.Context, req *kvstore.StatusRequest) (*kvstore.StatusResponse, error) {
	return &kvstore.StatusResponse{
		NodeId: s.config.Cluster.NodeID,
		Role:   "standalone", // In a real cluster implementation, this would be dynamic
		State:  "running",
	}, nil
}

// loggingInterceptor is a gRPC unary interceptor for logging
func (s *GRPCServer) loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	
	resp, err := handler(ctx, req)
	
	duration := time.Since(start)
	
	if err != nil {
		s.logger.ErrorContext(ctx, "gRPC request failed",
			"method", info.FullMethod,
			"duration", duration,
			"error", err,
		)
	} else {
		s.logger.DebugContext(ctx, "gRPC request completed",
			"method", info.FullMethod,
			"duration", duration,
		)
	}
	
	return resp, err
}