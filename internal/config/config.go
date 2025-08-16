package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Storage  StorageConfig  `yaml:"storage" json:"storage"`
	Cluster  ClusterConfig  `yaml:"cluster" json:"cluster"`
	Logging  LoggingConfig  `yaml:"logging" json:"logging"`
	Metrics  MetricsConfig  `yaml:"metrics" json:"metrics"`
	Security SecurityConfig `yaml:"security" json:"security"`
	Tracing  TracingConfig  `yaml:"tracing" json:"tracing"`
}

type ServerConfig struct {
	Host         string        `yaml:"host" json:"host"`
	Port         int           `yaml:"port" json:"port"`
	GRPCPort     int           `yaml:"grpc_port" json:"grpc_port"`
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxBodySize  int64         `yaml:"max_body_size" json:"max_body_size"`
}

type StorageConfig struct {
	Engine      string        `yaml:"engine" json:"engine"`
	DataPath    string        `yaml:"data_path" json:"data_path"`
	InMemory    bool          `yaml:"in_memory" json:"in_memory"`
	SyncWrites  bool          `yaml:"sync_writes" json:"sync_writes"`
	ValueLogGC  bool          `yaml:"value_log_gc" json:"value_log_gc"`
	GCInterval  time.Duration `yaml:"gc_interval" json:"gc_interval"`
	BackupPath  string        `yaml:"backup_path" json:"backup_path"`
	MaxFileSize int64         `yaml:"max_file_size" json:"max_file_size"`
	// WAL settings
	WAL WALConfig `yaml:"wal" json:"wal"`
	// Cache settings
	Cache CacheConfig `yaml:"cache" json:"cache"`
}

type CacheConfig struct {
	Enabled    bool          `yaml:"enabled" json:"enabled"`
	Size       int           `yaml:"size" json:"size"`           // Maximum number of items in cache
	TTL        time.Duration `yaml:"ttl" json:"ttl"`             // Default TTL for cached items
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"` // How often to clean expired items
}

type WALConfig struct {
	Enabled      bool  `yaml:"enabled" json:"enabled"`
	Threshold    int64 `yaml:"threshold" json:"threshold"`       // WAL file size threshold for rotation (bytes)
	MaxFiles     int   `yaml:"max_files" json:"max_files"`       // Maximum number of WAL files to keep
	FSyncThreshold int `yaml:"fsync_threshold" json:"fsync_threshold"` // Number of writes before fsync
}

type ClusterConfig struct {
	NodeID        string        `yaml:"node_id" json:"node_id"`
	Peers         []string      `yaml:"peers" json:"peers"`
	RaftPort      int           `yaml:"raft_port" json:"raft_port"`
	RaftDir       string        `yaml:"raft_dir" json:"raft_dir"`
	SnapshotCount int           `yaml:"snapshot_count" json:"snapshot_count"`
	HeartbeatTick time.Duration `yaml:"heartbeat_tick" json:"heartbeat_tick"`
	ElectionTick  time.Duration `yaml:"election_tick" json:"election_tick"`
	MaxSnapshots  int           `yaml:"max_snapshots" json:"max_snapshots"`
	JoinTimeout   time.Duration `yaml:"join_timeout" json:"join_timeout"`
}

type LoggingConfig struct {
	Level                 string `yaml:"level" json:"level"`
	Format                string `yaml:"format" json:"format"`
	Output                string `yaml:"output" json:"output"`
	EnableRequestTracing  bool   `yaml:"enable_request_tracing" json:"enable_request_tracing"`
	EnableCorrelationIDs  bool   `yaml:"enable_correlation_ids" json:"enable_correlation_ids"`
	EnableDatabaseLogging bool   `yaml:"enable_database_logging" json:"enable_database_logging"`
	EnablePerformanceLog  bool   `yaml:"enable_performance_log" json:"enable_performance_log"`
	LogSampling           int    `yaml:"log_sampling" json:"log_sampling"` // 0 = no sampling, N = sample every Nth log
}

type MetricsConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	Port    int  `yaml:"port" json:"port"`
	Path    string `yaml:"path" json:"path"`
}

type SecurityConfig struct {
	TLSEnabled  bool   `yaml:"tls_enabled" json:"tls_enabled"`
	CertFile    string `yaml:"cert_file" json:"cert_file"`
	KeyFile     string `yaml:"key_file" json:"key_file"`
	CAFile      string `yaml:"ca_file" json:"ca_file"`
	AuthEnabled bool   `yaml:"auth_enabled" json:"auth_enabled"`
	AuthToken   string `yaml:"auth_token" json:"auth_token"`
}

type TracingConfig struct {
	Enabled        bool              `yaml:"enabled" json:"enabled"`
	ServiceName    string            `yaml:"service_name" json:"service_name"`
	ServiceVersion string            `yaml:"service_version" json:"service_version"`
	Environment    string            `yaml:"environment" json:"environment"`
	ExporterType   string            `yaml:"exporter_type" json:"exporter_type"`
	JaegerEndpoint string            `yaml:"jaeger_endpoint" json:"jaeger_endpoint"`
	OTLPEndpoint   string            `yaml:"otlp_endpoint" json:"otlp_endpoint"`
	OTLPHeaders    map[string]string `yaml:"otlp_headers" json:"otlp_headers"`
	SamplingRatio  float64           `yaml:"sampling_ratio" json:"sampling_ratio"`
}

func Load(configPath string) (*Config, error) {
	config := DefaultConfig()
	
	if configPath != "" {
		if err := loadFromFile(config, configPath); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}
	
	loadFromEnvironment(config)
	
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	
	return config, nil
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "localhost",
			Port:         8080,
			GRPCPort:     9090,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
			MaxBodySize:  1024 * 1024, // 1MB
		},
		Storage: StorageConfig{
			Engine:      "badger",
			DataPath:    "./data/badger",
			InMemory:    false,
			SyncWrites:  false,
			ValueLogGC:  true,
			GCInterval:  5 * time.Minute,
			BackupPath:  "./backups",
			MaxFileSize: 64 * 1024 * 1024, // 64MB
			WAL: WALConfig{
				Enabled:        true,
				Threshold:      64 * 1024 * 1024, // 64MB per WAL file
				MaxFiles:       5,                // Keep 5 WAL files
				FSyncThreshold: 1000,             // Sync every 1000 writes
			},
			Cache: CacheConfig{
				Enabled:         true,
				Size:            10000,           // 10k items max
				TTL:             30 * time.Minute, // 30 minute default TTL
				CleanupInterval: 5 * time.Minute,  // Clean expired items every 5 minutes
			},
		},
		Cluster: ClusterConfig{
			NodeID:        "node1",
			Peers:         []string{},
			RaftPort:      7000,
			RaftDir:       "./data/raft",
			SnapshotCount: 10000,
			HeartbeatTick: 1 * time.Second,
			ElectionTick:  10 * time.Second,
			MaxSnapshots:  5,
			JoinTimeout:   10 * time.Second,
		},
		Logging: LoggingConfig{
			Level:                 "info",
			Format:                "json",
			Output:                "stdout",
			EnableRequestTracing:  true,
			EnableCorrelationIDs:  true,
			EnableDatabaseLogging: false,
			EnablePerformanceLog:  false,
			LogSampling:           0,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    2112,
			Path:    "/metrics",
		},
		Security: SecurityConfig{
			TLSEnabled:  false,
			CertFile:    "",
			KeyFile:     "",
			CAFile:      "",
			AuthEnabled: false,
			AuthToken:   "",
		},
		Tracing: TracingConfig{
			Enabled:        false,
			ServiceName:    "kvstore",
			ServiceVersion: "1.0.0",
			Environment:    "development",
			ExporterType:   "console",
			JaegerEndpoint: "http://localhost:14268/api/traces",
			OTLPEndpoint:   "http://localhost:4318",
			OTLPHeaders:    make(map[string]string),
			SamplingRatio:  1.0,
		},
	}
}

func loadFromFile(config *Config, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	
	ext := strings.ToLower(filepath.Ext(configPath))
	
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, config); err != nil {
			return fmt.Errorf("failed to unmarshal YAML config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config file format: %s", ext)
	}
	
	return nil
}

func loadFromEnvironment(config *Config) {
	// Server configuration
	if host := os.Getenv("KV_SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("KV_SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}
	if grpcPort := os.Getenv("KV_SERVER_GRPC_PORT"); grpcPort != "" {
		if p, err := strconv.Atoi(grpcPort); err == nil {
			config.Server.GRPCPort = p
		}
	}
	
	// Storage configuration
	if engine := os.Getenv("KV_STORAGE_ENGINE"); engine != "" {
		config.Storage.Engine = engine
	}
	if dataPath := os.Getenv("KV_STORAGE_DATA_PATH"); dataPath != "" {
		config.Storage.DataPath = dataPath
	}
	if inMemory := os.Getenv("KV_STORAGE_IN_MEMORY"); inMemory != "" {
		if b, err := strconv.ParseBool(inMemory); err == nil {
			config.Storage.InMemory = b
		}
	}
	if syncWrites := os.Getenv("KV_STORAGE_SYNC_WRITES"); syncWrites != "" {
		if b, err := strconv.ParseBool(syncWrites); err == nil {
			config.Storage.SyncWrites = b
		}
	}
	
	// Cluster configuration
	if nodeID := os.Getenv("KV_CLUSTER_NODE_ID"); nodeID != "" {
		config.Cluster.NodeID = nodeID
	}
	if peers := os.Getenv("KV_CLUSTER_PEERS"); peers != "" {
		config.Cluster.Peers = strings.Split(peers, ",")
	}
	if raftPort := os.Getenv("KV_CLUSTER_RAFT_PORT"); raftPort != "" {
		if p, err := strconv.Atoi(raftPort); err == nil {
			config.Cluster.RaftPort = p
		}
	}
	
	// Logging configuration
	if level := os.Getenv("KV_LOG_LEVEL"); level != "" {
		config.Logging.Level = level
	}
	if format := os.Getenv("KV_LOG_FORMAT"); format != "" {
		config.Logging.Format = format
	}
	
	// Metrics configuration
	if enabled := os.Getenv("KV_METRICS_ENABLED"); enabled != "" {
		if b, err := strconv.ParseBool(enabled); err == nil {
			config.Metrics.Enabled = b
		}
	}
	if port := os.Getenv("KV_METRICS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Metrics.Port = p
		}
	}
	
	// Security configuration
	if tlsEnabled := os.Getenv("KV_SECURITY_TLS_ENABLED"); tlsEnabled != "" {
		if b, err := strconv.ParseBool(tlsEnabled); err == nil {
			config.Security.TLSEnabled = b
		}
	}
	if certFile := os.Getenv("KV_SECURITY_CERT_FILE"); certFile != "" {
		config.Security.CertFile = certFile
	}
	if keyFile := os.Getenv("KV_SECURITY_KEY_FILE"); keyFile != "" {
		config.Security.KeyFile = keyFile
	}
	if authToken := os.Getenv("KV_SECURITY_AUTH_TOKEN"); authToken != "" {
		config.Security.AuthToken = authToken
	}
}

func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Server.GRPCPort)
	}
	if c.Server.Port == c.Server.GRPCPort {
		return fmt.Errorf("server port and gRPC port cannot be the same: %d", c.Server.Port)
	}
	if c.Server.ReadTimeout <= 0 {
		return fmt.Errorf("read timeout must be positive")
	}
	if c.Server.WriteTimeout <= 0 {
		return fmt.Errorf("write timeout must be positive")
	}
	if c.Server.MaxBodySize <= 0 {
		return fmt.Errorf("max body size must be positive")
	}
	
	// Storage validation
	if c.Storage.Engine == "" {
		return fmt.Errorf("storage engine cannot be empty")
	}
	if !c.Storage.InMemory && c.Storage.DataPath == "" {
		return fmt.Errorf("data path cannot be empty when not using in-memory storage")
	}
	if c.Storage.GCInterval <= 0 {
		return fmt.Errorf("GC interval must be positive")
	}
	if c.Storage.MaxFileSize <= 0 {
		return fmt.Errorf("max file size must be positive")
	}
	
	// Cluster validation
	if c.Cluster.NodeID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}
	if c.Cluster.RaftPort <= 0 || c.Cluster.RaftPort > 65535 {
		return fmt.Errorf("invalid raft port: %d", c.Cluster.RaftPort)
	}
	if c.Cluster.RaftPort == c.Server.Port || c.Cluster.RaftPort == c.Server.GRPCPort {
		return fmt.Errorf("raft port conflicts with server ports")
	}
	if c.Cluster.SnapshotCount <= 0 {
		return fmt.Errorf("snapshot count must be positive")
	}
	if c.Cluster.HeartbeatTick <= 0 {
		return fmt.Errorf("heartbeat tick must be positive")
	}
	if c.Cluster.ElectionTick <= c.Cluster.HeartbeatTick {
		return fmt.Errorf("election tick must be greater than heartbeat tick")
	}
	if c.Cluster.MaxSnapshots <= 0 {
		return fmt.Errorf("max snapshots must be positive")
	}
	
	// Logging validation
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}
	validFormats := map[string]bool{
		"json": true, "text": true, "console": true,
	}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}
	
	// Metrics validation
	if c.Metrics.Enabled {
		if c.Metrics.Port <= 0 || c.Metrics.Port > 65535 {
			return fmt.Errorf("invalid metrics port: %d", c.Metrics.Port)
		}
		if c.Metrics.Port == c.Server.Port || c.Metrics.Port == c.Server.GRPCPort || c.Metrics.Port == c.Cluster.RaftPort {
			return fmt.Errorf("metrics port conflicts with other ports")
		}
		if c.Metrics.Path == "" {
			return fmt.Errorf("metrics path cannot be empty when metrics are enabled")
		}
	}
	
	// Security validation
	if c.Security.TLSEnabled {
		if c.Security.CertFile == "" {
			return fmt.Errorf("cert file cannot be empty when TLS is enabled")
		}
		if c.Security.KeyFile == "" {
			return fmt.Errorf("key file cannot be empty when TLS is enabled")
		}
		if _, err := os.Stat(c.Security.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("cert file does not exist: %s", c.Security.CertFile)
		}
		if _, err := os.Stat(c.Security.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("key file does not exist: %s", c.Security.KeyFile)
		}
	}
	
	return nil
}

func (c *Config) GetStorageConfig() map[string]interface{} {
	return map[string]interface{}{
		"data_path":   c.Storage.DataPath,
		"in_memory":   c.Storage.InMemory,
		"sync_writes": c.Storage.SyncWrites,
		"value_log_gc": c.Storage.ValueLogGC,
		"gc_interval": c.Storage.GCInterval,
	}
}

func (c *Config) String() string {
	data, _ := yaml.Marshal(c)
	return string(data)
}