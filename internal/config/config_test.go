package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	
	if config.Server.Host != "localhost" {
		t.Errorf("Expected default host to be localhost, got %s", config.Server.Host)
	}
	
	if config.Server.Port != 8080 {
		t.Errorf("Expected default port to be 8080, got %d", config.Server.Port)
	}
	
	if config.Storage.Engine != "badger" {
		t.Errorf("Expected default storage engine to be badger, got %s", config.Storage.Engine)
	}
	
	if config.Cluster.NodeID != "node1" {
		t.Errorf("Expected default node ID to be node1, got %s", config.Cluster.NodeID)
	}
}

func TestLoadFromFile(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.yaml")
	
	configContent := `
server:
  host: "0.0.0.0"
  port: 9000
  grpc_port: 9001

storage:
  engine: "badger"
  data_path: "/tmp/test-data"
  in_memory: true

cluster:
  node_id: "test-node"
  raft_port: 8000

logging:
  level: "debug"
  format: "text"

metrics:
  enabled: false
  port: 3000
`
	
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}
	
	config, err := Load(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	if config.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host to be 0.0.0.0, got %s", config.Server.Host)
	}
	
	if config.Server.Port != 9000 {
		t.Errorf("Expected port to be 9000, got %d", config.Server.Port)
	}
	
	if config.Storage.InMemory != true {
		t.Errorf("Expected in_memory to be true, got %v", config.Storage.InMemory)
	}
	
	if config.Cluster.NodeID != "test-node" {
		t.Errorf("Expected node ID to be test-node, got %s", config.Cluster.NodeID)
	}
	
	if config.Logging.Level != "debug" {
		t.Errorf("Expected log level to be debug, got %s", config.Logging.Level)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("KV_SERVER_HOST", "test-host")
	os.Setenv("KV_SERVER_PORT", "6000")
	os.Setenv("KV_CLUSTER_RAFT_PORT", "6001")
	os.Setenv("KV_STORAGE_ENGINE", "test-engine")
	os.Setenv("KV_CLUSTER_NODE_ID", "env-node")
	os.Setenv("KV_LOG_LEVEL", "error")
	
	defer func() {
		os.Unsetenv("KV_SERVER_HOST")
		os.Unsetenv("KV_SERVER_PORT")
		os.Unsetenv("KV_CLUSTER_RAFT_PORT")
		os.Unsetenv("KV_STORAGE_ENGINE")
		os.Unsetenv("KV_CLUSTER_NODE_ID")
		os.Unsetenv("KV_LOG_LEVEL")
	}()
	
	config, err := Load("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	if config.Server.Host != "test-host" {
		t.Errorf("Expected host to be test-host, got %s", config.Server.Host)
	}
	
	if config.Server.Port != 6000 {
		t.Errorf("Expected port to be 6000, got %d", config.Server.Port)
	}
	
	if config.Storage.Engine != "test-engine" {
		t.Errorf("Expected storage engine to be test-engine, got %s", config.Storage.Engine)
	}
	
	if config.Cluster.NodeID != "env-node" {
		t.Errorf("Expected node ID to be env-node, got %s", config.Cluster.NodeID)
	}
	
	if config.Logging.Level != "error" {
		t.Errorf("Expected log level to be error, got %s", config.Logging.Level)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		configFunc  func() *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			configFunc: func() *Config {
				return DefaultConfig()
			},
			expectError: false,
		},
		{
			name: "invalid server port",
			configFunc: func() *Config {
				config := DefaultConfig()
				config.Server.Port = 0
				return config
			},
			expectError: true,
			errorMsg:    "invalid server port",
		},
		{
			name: "ports conflict",
			configFunc: func() *Config {
				config := DefaultConfig()
				config.Server.GRPCPort = config.Server.Port
				return config
			},
			expectError: true,
			errorMsg:    "server port and gRPC port cannot be the same",
		},
		{
			name: "empty storage engine",
			configFunc: func() *Config {
				config := DefaultConfig()
				config.Storage.Engine = ""
				return config
			},
			expectError: true,
			errorMsg:    "storage engine cannot be empty",
		},
		{
			name: "empty node ID",
			configFunc: func() *Config {
				config := DefaultConfig()
				config.Cluster.NodeID = ""
				return config
			},
			expectError: true,
			errorMsg:    "node ID cannot be empty",
		},
		{
			name: "invalid log level",
			configFunc: func() *Config {
				config := DefaultConfig()
				config.Logging.Level = "invalid"
				return config
			},
			expectError: true,
			errorMsg:    "invalid log level",
		},
		{
			name: "election tick <= heartbeat tick",
			configFunc: func() *Config {
				config := DefaultConfig()
				config.Cluster.ElectionTick = config.Cluster.HeartbeatTick
				return config
			},
			expectError: true,
			errorMsg:    "election tick must be greater than heartbeat tick",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.configFunc()
			err := config.Validate()
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected validation error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no validation error but got: %v", err)
				}
			}
		})
	}
}

func TestGetStorageConfig(t *testing.T) {
	config := DefaultConfig()
	config.Storage.DataPath = "/test/path"
	config.Storage.InMemory = true
	config.Storage.SyncWrites = true
	config.Storage.ValueLogGC = false
	config.Storage.GCInterval = 10 * time.Minute
	
	storageConfig := config.GetStorageConfig()
	
	if storageConfig["data_path"] != "/test/path" {
		t.Errorf("Expected data_path to be /test/path, got %v", storageConfig["data_path"])
	}
	
	if storageConfig["in_memory"] != true {
		t.Errorf("Expected in_memory to be true, got %v", storageConfig["in_memory"])
	}
	
	if storageConfig["sync_writes"] != true {
		t.Errorf("Expected sync_writes to be true, got %v", storageConfig["sync_writes"])
	}
	
	if storageConfig["value_log_gc"] != false {
		t.Errorf("Expected value_log_gc to be false, got %v", storageConfig["value_log_gc"])
	}
	
	if storageConfig["gc_interval"] != 10*time.Minute {
		t.Errorf("Expected gc_interval to be 10m, got %v", storageConfig["gc_interval"])
	}
}

func TestConfigString(t *testing.T) {
	config := DefaultConfig()
	configStr := config.String()
	
	if configStr == "" {
		t.Error("Config string should not be empty")
	}
	
	// Should contain YAML content
	if !contains(configStr, "server:") {
		t.Error("Config string should contain server section")
	}
	
	if !contains(configStr, "storage:") {
		t.Error("Config string should contain storage section")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   (len(s) > len(substr) && contains(s[1:], substr))
}