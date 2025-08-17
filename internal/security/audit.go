package security

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditLogger provides comprehensive audit logging capabilities
type AuditLogger struct {
	mu              sync.RWMutex
	logger          *log.Logger
	config          AuditConfig
	writers         []AuditWriter
	filters         []AuditFilter
	enrichers       []AuditEnricher
	buffer          []*AuditEvent
	bufferSize      int
	metrics         *AuditMetrics
	isRunning       bool
	flushTicker     *time.Ticker
	stopCh          chan struct{}
	correlationMap  map[string]*CorrelationContext
}

// AuditConfig contains configuration for audit logging
type AuditConfig struct {
	// Output configuration
	EnableConsole     bool     `json:"enable_console"`     // Enable console output
	EnableFile        bool     `json:"enable_file"`        // Enable file output
	EnableSyslog      bool     `json:"enable_syslog"`      // Enable syslog output
	EnableRemote      bool     `json:"enable_remote"`      // Enable remote logging
	
	// File configuration
	LogDirectory      string   `json:"log_directory"`      // Log file directory
	LogFileName       string   `json:"log_file_name"`      // Log file name pattern
	MaxFileSize       int64    `json:"max_file_size"`      // Maximum file size in bytes
	MaxBackups        int      `json:"max_backups"`        // Maximum backup files
	MaxAge            int      `json:"max_age"`            // Maximum age in days
	CompressBackups   bool     `json:"compress_backups"`   // Compress backup files
	
	// Remote configuration
	RemoteEndpoint    string   `json:"remote_endpoint"`    // Remote logging endpoint
	RemoteAPIKey      string   `json:"remote_api_key"`     // Remote API key
	RemoteTimeout     time.Duration `json:"remote_timeout"` // Remote timeout
	
	// Buffering configuration
	BufferSize        int      `json:"buffer_size"`        // Buffer size for batching
	FlushInterval     time.Duration `json:"flush_interval"` // Flush interval
	FlushOnCritical   bool     `json:"flush_on_critical"`  // Immediate flush for critical events
	
	// Event filtering
	LogLevel          LogLevel `json:"log_level"`          // Minimum log level
	IncludeCategories []string `json:"include_categories"` // Categories to include
	ExcludeCategories []string `json:"exclude_categories"` // Categories to exclude
	IncludeActions    []string `json:"include_actions"`    // Actions to include
	ExcludeActions    []string `json:"exclude_actions"`    // Actions to exclude
	
	// Event enrichment
	IncludeStackTrace bool     `json:"include_stack_trace"` // Include stack traces
	IncludeEnvironment bool    `json:"include_environment"` // Include environment info
	IncludeRequestID  bool     `json:"include_request_id"`  // Include request ID
	AnonymizeData     bool     `json:"anonymize_data"`      // Anonymize sensitive data
	
	// Security configuration
	EnableSigning     bool     `json:"enable_signing"`     // Enable event signing
	SigningKey        string   `json:"signing_key"`        // Signing key
	EnableEncryption  bool     `json:"enable_encryption"`  // Enable event encryption
	EncryptionKey     string   `json:"encryption_key"`     // Encryption key
	
	// Compliance configuration
	RetentionPeriod   time.Duration `json:"retention_period"` // Log retention period
	ComplianceMode    string        `json:"compliance_mode"`  // Compliance standard
	EnableIntegrity   bool          `json:"enable_integrity"` // Enable integrity checking
	TamperDetection   bool          `json:"tamper_detection"` // Enable tamper detection
}

// AuditEvent represents a single audit event
type AuditEvent struct {
	// Core fields
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	Level         LogLevel               `json:"level"`
	Category      EventCategory          `json:"category"`
	Action        string                 `json:"action"`
	Status        EventStatus            `json:"status"`
	Message       string                 `json:"message"`
	
	// Actor information
	Actor         *Actor                 `json:"actor"`
	
	// Target information
	Target        *Target                `json:"target"`
	
	// Request context
	RequestContext *RequestContext       `json:"request_context"`
	
	// Event details
	Details       map[string]interface{} `json:"details"`
	Metadata      map[string]interface{} `json:"metadata"`
	
	// Security fields
	Signature     string                 `json:"signature,omitempty"`
	Checksum      string                 `json:"checksum"`
	
	// Correlation
	CorrelationID string                 `json:"correlation_id,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	
	// Compliance fields
	Severity      Severity               `json:"severity"`
	Risk          RiskLevel              `json:"risk"`
	Compliance    []string               `json:"compliance,omitempty"`
	
	// Processing metadata
	ProcessedAt   time.Time              `json:"processed_at"`
	Source        string                 `json:"source"`
	Version       string                 `json:"version"`
}

// Actor represents the entity performing an action
type Actor struct {
	Type        ActorType              `json:"type"`
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Email       string                 `json:"email,omitempty"`
	Roles       []string               `json:"roles,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Location    *Location              `json:"location,omitempty"`
	Device      *DeviceInfo            `json:"device,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}

// Target represents the target of an action
type Target struct {
	Type        string                 `json:"type"`
	ID          string                 `json:"id"`
	Name        string                 `json:"name,omitempty"`
	Resource    string                 `json:"resource"`
	Path        string                 `json:"path,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}

// RequestContext contains request-specific information
type RequestContext struct {
	RequestID     string                 `json:"request_id"`
	Method        string                 `json:"method"`
	URL           string                 `json:"url"`
	Headers       map[string]string      `json:"headers,omitempty"`
	QueryParams   map[string]string      `json:"query_params,omitempty"`
	ContentType   string                 `json:"content_type,omitempty"`
	ContentLength int64                  `json:"content_length,omitempty"`
	ResponseCode  int                    `json:"response_code,omitempty"`
	Duration      time.Duration          `json:"duration,omitempty"`
	BytesIn       int64                  `json:"bytes_in,omitempty"`
	BytesOut      int64                  `json:"bytes_out,omitempty"`
}

// CorrelationContext tracks related events
type CorrelationContext struct {
	ID          string                 `json:"id"`
	StartTime   time.Time              `json:"start_time"`
	EventCount  int                    `json:"event_count"`
	LastEvent   time.Time              `json:"last_event"`
	Events      []string               `json:"events"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AuditWriter interface for different output destinations
type AuditWriter interface {
	Write(event *AuditEvent) error
	WriteBatch(events []*AuditEvent) error
	Close() error
}

// AuditFilter interface for filtering events
type AuditFilter interface {
	ShouldLog(event *AuditEvent) bool
}

// AuditEnricher interface for enriching events
type AuditEnricher interface {
	Enrich(event *AuditEvent) error
}

// AuditMetrics tracks audit logging metrics
type AuditMetrics struct {
	TotalEvents        int64             `json:"total_events"`
	EventsByLevel      map[LogLevel]int64 `json:"events_by_level"`
	EventsByCategory   map[EventCategory]int64 `json:"events_by_category"`
	EventsByStatus     map[EventStatus]int64 `json:"events_by_status"`
	FailedWrites       int64             `json:"failed_writes"`
	BufferOverflows    int64             `json:"buffer_overflows"`
	ProcessingTime     time.Duration     `json:"processing_time"`
	LastFlush          time.Time         `json:"last_flush"`
	WriterMetrics      map[string]interface{} `json:"writer_metrics"`
}

// Enums
type LogLevel string
const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
)

type EventCategory string
const (
	CategoryAuthentication EventCategory = "authentication"
	CategoryAuthorization  EventCategory = "authorization"
	CategoryDataAccess     EventCategory = "data_access"
	CategoryDataChange     EventCategory = "data_change"
	CategorySystem         EventCategory = "system"
	CategorySecurity       EventCategory = "security"
	CategoryCompliance     EventCategory = "compliance"
	CategoryNetwork        EventCategory = "network"
	CategoryError          EventCategory = "error"
)

type EventStatus string
const (
	StatusSuccess EventStatus = "success"
	StatusFailure EventStatus = "failure"
	StatusError   EventStatus = "error"
	StatusDenied  EventStatus = "denied"
	StatusPartial EventStatus = "partial"
)

type ActorType string
const (
	ActorTypeUser    ActorType = "user"
	ActorTypeService ActorType = "service"
	ActorTypeSystem  ActorType = "system"
	ActorTypeAPI     ActorType = "api"
	ActorTypeUnknown ActorType = "unknown"
)

type Severity string
const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type RiskLevel string
const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config AuditConfig, logger *log.Logger) (*AuditLogger, error) {
	if logger == nil {
		logger = log.New(log.Writer(), "[AUDIT] ", log.LstdFlags)
	}
	
	// Set default values
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 30 * time.Second
	}
	if config.LogDirectory == "" {
		config.LogDirectory = "./logs/audit"
	}
	if config.LogFileName == "" {
		config.LogFileName = "audit-%Y%m%d.log"
	}
	if config.MaxFileSize == 0 {
		config.MaxFileSize = 100 * 1024 * 1024 // 100MB
	}
	if config.MaxBackups == 0 {
		config.MaxBackups = 10
	}
	if config.MaxAge == 0 {
		config.MaxAge = 30 // 30 days
	}
	if config.RetentionPeriod == 0 {
		config.RetentionPeriod = 90 * 24 * time.Hour // 90 days
	}
	if config.RemoteTimeout == 0 {
		config.RemoteTimeout = 10 * time.Second
	}
	
	al := &AuditLogger{
		logger:         logger,
		config:         config,
		writers:        make([]AuditWriter, 0),
		filters:        make([]AuditFilter, 0),
		enrichers:      make([]AuditEnricher, 0),
		buffer:         make([]*AuditEvent, 0, config.BufferSize),
		bufferSize:     config.BufferSize,
		correlationMap: make(map[string]*CorrelationContext),
		stopCh:         make(chan struct{}),
		metrics: &AuditMetrics{
			EventsByLevel:    make(map[LogLevel]int64),
			EventsByCategory: make(map[EventCategory]int64),
			EventsByStatus:   make(map[EventStatus]int64),
			WriterMetrics:    make(map[string]interface{}),
		},
	}
	
	// Initialize writers
	if err := al.initializeWriters(); err != nil {
		return nil, fmt.Errorf("failed to initialize writers: %w", err)
	}
	
	// Initialize filters
	al.initializeFilters()
	
	// Initialize enrichers
	al.initializeEnrichers()
	
	// Start processing
	if err := al.Start(); err != nil {
		return nil, fmt.Errorf("failed to start audit logger: %w", err)
	}
	
	logger.Printf("Audit logger initialized with %d writers", len(al.writers))
	return al, nil
}

// initializeWriters initializes audit writers based on configuration
func (al *AuditLogger) initializeWriters() error {
	// Console writer
	if al.config.EnableConsole {
		writer := &ConsoleWriter{logger: al.logger}
		al.writers = append(al.writers, writer)
	}
	
	// File writer
	if al.config.EnableFile {
		writer, err := NewFileWriter(FileWriterConfig{
			Directory:       al.config.LogDirectory,
			FileNamePattern: al.config.LogFileName,
			MaxSize:         al.config.MaxFileSize,
			MaxBackups:      al.config.MaxBackups,
			MaxAge:          al.config.MaxAge,
			Compress:        al.config.CompressBackups,
		})
		if err != nil {
			return fmt.Errorf("failed to create file writer: %w", err)
		}
		al.writers = append(al.writers, writer)
	}
	
	// Syslog writer
	if al.config.EnableSyslog {
		writer, err := NewSyslogWriter()
		if err != nil {
			return fmt.Errorf("failed to create syslog writer: %w", err)
		}
		al.writers = append(al.writers, writer)
	}
	
	// Remote writer
	if al.config.EnableRemote {
		writer := &RemoteWriter{
			endpoint: al.config.RemoteEndpoint,
			apiKey:   al.config.RemoteAPIKey,
			timeout:  al.config.RemoteTimeout,
		}
		al.writers = append(al.writers, writer)
	}
	
	return nil
}

// initializeFilters initializes audit filters
func (al *AuditLogger) initializeFilters() {
	// Level filter
	al.filters = append(al.filters, &LevelFilter{
		minLevel: al.config.LogLevel,
	})
	
	// Category filter
	if len(al.config.IncludeCategories) > 0 || len(al.config.ExcludeCategories) > 0 {
		al.filters = append(al.filters, &CategoryFilter{
			include: al.config.IncludeCategories,
			exclude: al.config.ExcludeCategories,
		})
	}
	
	// Action filter
	if len(al.config.IncludeActions) > 0 || len(al.config.ExcludeActions) > 0 {
		al.filters = append(al.filters, &ActionFilter{
			include: al.config.IncludeActions,
			exclude: al.config.ExcludeActions,
		})
	}
}

// initializeEnrichers initializes audit enrichers
func (al *AuditLogger) initializeEnrichers() {
	// Environment enricher
	if al.config.IncludeEnvironment {
		al.enrichers = append(al.enrichers, &EnvironmentEnricher{})
	}
	
	// Request ID enricher
	if al.config.IncludeRequestID {
		al.enrichers = append(al.enrichers, &RequestIDEnricher{})
	}
	
	// Anonymization enricher
	if al.config.AnonymizeData {
		al.enrichers = append(al.enrichers, &AnonymizationEnricher{})
	}
}

// Start starts the audit logger
func (al *AuditLogger) Start() error {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	if al.isRunning {
		return fmt.Errorf("audit logger is already running")
	}
	
	// Create log directory if it doesn't exist
	if al.config.EnableFile {
		if err := os.MkdirAll(al.config.LogDirectory, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	
	// Start flush ticker
	al.flushTicker = time.NewTicker(al.config.FlushInterval)
	
	// Start processing goroutine
	go al.processEvents()
	
	al.isRunning = true
	al.logger.Printf("Audit logger started")
	
	return nil
}

// LogAccess logs access events (implements AuditLogger interface from RBAC)
func (al *AuditLogger) LogAccess(request *AccessRequest, result *AccessResult) error {
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  CategoryAuthorization,
		Action:    "access_check",
		Message:   fmt.Sprintf("Access %s for %s on %s", al.statusFromResult(result), request.Username, request.Resource),
		Actor: &Actor{
			Type:      ActorTypeUser,
			ID:        request.UserID,
			Name:      request.Username,
			IPAddress: al.getIPFromContext(request.Context),
			UserAgent: al.getUserAgentFromContext(request.Context),
		},
		Target: &Target{
			Type:     "resource",
			Resource: request.Resource,
			ID:       request.Resource,
		},
		RequestContext: al.buildRequestContext(request.Context),
		Details: map[string]interface{}{
			"action":       request.Action,
			"granted":      result.Granted,
			"reason":       result.Reason,
			"permissions":  result.Permissions,
			"matched_rules": result.MatchedRules,
		},
		Severity: al.getSeverityFromResult(result),
		Risk:     al.getRiskFromResult(result),
	}
	
	if result.Granted {
		event.Status = StatusSuccess
	} else {
		event.Status = StatusDenied
		event.Level = LogLevelWarn
	}
	
	return al.LogEvent(event)
}

// LogRoleChange logs role changes (implements AuditLogger interface from RBAC)
func (al *AuditLogger) LogRoleChange(userID, role, action string, metadata map[string]interface{}) error {
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  CategoryAuthorization,
		Action:    fmt.Sprintf("role_%s", action),
		Status:    StatusSuccess,
		Message:   fmt.Sprintf("Role %s %s for user %s", role, action, userID),
		Actor: &Actor{
			Type: ActorTypeSystem,
			ID:   "system",
			Name: "System",
		},
		Target: &Target{
			Type: "user",
			ID:   userID,
			Name: userID,
		},
		Details: map[string]interface{}{
			"role":   role,
			"action": action,
		},
		Metadata:  metadata,
		Severity:  SeverityMedium,
		Risk:      RiskMedium,
	}
	
	return al.LogEvent(event)
}

// LogPermissionChange logs permission changes (implements AuditLogger interface from RBAC)
func (al *AuditLogger) LogPermissionChange(permissionID, action string, metadata map[string]interface{}) error {
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  CategoryAuthorization,
		Action:    fmt.Sprintf("permission_%s", action),
		Status:    StatusSuccess,
		Message:   fmt.Sprintf("Permission %s %s", permissionID, action),
		Actor: &Actor{
			Type: ActorTypeSystem,
			ID:   "system",
			Name: "System",
		},
		Target: &Target{
			Type: "permission",
			ID:   permissionID,
			Name: permissionID,
		},
		Details: map[string]interface{}{
			"permission_id": permissionID,
			"action":        action,
		},
		Metadata:  metadata,
		Severity:  SeverityMedium,
		Risk:      RiskMedium,
	}
	
	return al.LogEvent(event)
}

// LogPolicyChange logs policy changes (implements AuditLogger interface from RBAC)
func (al *AuditLogger) LogPolicyChange(policyID, action string, metadata map[string]interface{}) error {
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  CategoryAuthorization,
		Action:    fmt.Sprintf("policy_%s", action),
		Status:    StatusSuccess,
		Message:   fmt.Sprintf("Policy %s %s", policyID, action),
		Actor: &Actor{
			Type: ActorTypeSystem,
			ID:   "system",
			Name: "System",
		},
		Target: &Target{
			Type: "policy",
			ID:   policyID,
			Name: policyID,
		},
		Details: map[string]interface{}{
			"policy_id": policyID,
			"action":    action,
		},
		Metadata:  metadata,
		Severity:  SeverityHigh,
		Risk:      RiskHigh,
	}
	
	return al.LogEvent(event)
}

// LogEvent logs a general audit event
func (al *AuditLogger) LogEvent(event *AuditEvent) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	// Set event metadata
	if event.ID == "" {
		event.ID = al.generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.ProcessedAt = time.Now()
	event.Source = "kvstore-audit"
	event.Version = "1.0"
	
	// Apply filters
	for _, filter := range al.filters {
		if !filter.ShouldLog(event) {
			return nil // Event filtered out
		}
	}
	
	// Apply enrichers
	for _, enricher := range al.enrichers {
		if err := enricher.Enrich(event); err != nil {
			al.logger.Printf("Failed to enrich event: %v", err)
		}
	}
	
	// Calculate checksum
	event.Checksum = al.calculateChecksum(event)
	
	// Sign event if enabled
	if al.config.EnableSigning {
		event.Signature = al.signEvent(event)
	}
	
	// Update metrics
	al.metrics.TotalEvents++
	al.metrics.EventsByLevel[event.Level]++
	al.metrics.EventsByCategory[event.Category]++
	al.metrics.EventsByStatus[event.Status]++
	
	// Handle correlation
	if event.CorrelationID != "" {
		al.updateCorrelation(event)
	}
	
	// Add to buffer
	if len(al.buffer) >= al.bufferSize {
		// Buffer overflow - flush immediately
		al.metrics.BufferOverflows++
		if err := al.flushBuffer(); err != nil {
			al.logger.Printf("Failed to flush buffer on overflow: %v", err)
		}
	}
	
	al.buffer = append(al.buffer, event)
	
	// Immediate flush for critical events
	if al.config.FlushOnCritical && (event.Level == LogLevelError || event.Level == LogLevelFatal || event.Severity == SeverityCritical) {
		if err := al.flushBuffer(); err != nil {
			al.logger.Printf("Failed to flush critical event: %v", err)
		}
	}
	
	return nil
}

// LogAuthentication logs authentication events
func (al *AuditLogger) LogAuthentication(userID, username, action, result string, metadata map[string]interface{}) error {
	status := StatusSuccess
	level := LogLevelInfo
	severity := SeverityLow
	
	if result != "success" {
		status = StatusFailure
		level = LogLevelWarn
		severity = SeverityMedium
	}
	
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  CategoryAuthentication,
		Action:    action,
		Status:    status,
		Message:   fmt.Sprintf("Authentication %s for user %s: %s", action, username, result),
		Actor: &Actor{
			Type: ActorTypeUser,
			ID:   userID,
			Name: username,
		},
		Details: map[string]interface{}{
			"result": result,
		},
		Metadata: metadata,
		Severity: severity,
		Risk:     RiskLow,
	}
	
	return al.LogEvent(event)
}

// LogDataAccess logs data access events
func (al *AuditLogger) LogDataAccess(userID, username, resource, action string, metadata map[string]interface{}) error {
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  CategoryDataAccess,
		Action:    action,
		Status:    StatusSuccess,
		Message:   fmt.Sprintf("Data %s on %s by %s", action, resource, username),
		Actor: &Actor{
			Type: ActorTypeUser,
			ID:   userID,
			Name: username,
		},
		Target: &Target{
			Type:     "data",
			Resource: resource,
			ID:       resource,
		},
		Details:  metadata,
		Severity: al.getDataAccessSeverity(action),
		Risk:     al.getDataAccessRisk(action),
	}
	
	return al.LogEvent(event)
}

// LogSecurityEvent logs security-related events
func (al *AuditLogger) LogSecurityEvent(eventType, description string, severity Severity, metadata map[string]interface{}) error {
	level := LogLevelInfo
	if severity == SeverityHigh || severity == SeverityCritical {
		level = LogLevelError
	}
	
	event := &AuditEvent{
		ID:        al.generateEventID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  CategorySecurity,
		Action:    eventType,
		Status:    StatusSuccess,
		Message:   description,
		Actor: &Actor{
			Type: ActorTypeSystem,
			ID:   "security_system",
			Name: "Security System",
		},
		Details:  metadata,
		Severity: severity,
		Risk:     al.getRiskFromSeverity(severity),
	}
	
	return al.LogEvent(event)
}

// processEvents processes events from buffer
func (al *AuditLogger) processEvents() {
	for {
		select {
		case <-al.flushTicker.C:
			al.mu.Lock()
			if err := al.flushBuffer(); err != nil {
				al.logger.Printf("Failed to flush buffer: %v", err)
			}
			al.mu.Unlock()
		case <-al.stopCh:
			// Final flush
			al.mu.Lock()
			al.flushBuffer()
			al.mu.Unlock()
			return
		}
	}
}

// flushBuffer flushes the event buffer to all writers
func (al *AuditLogger) flushBuffer() error {
	if len(al.buffer) == 0 {
		return nil
	}
	
	start := time.Now()
	
	// Write to all writers
	for _, writer := range al.writers {
		if err := writer.WriteBatch(al.buffer); err != nil {
			al.metrics.FailedWrites++
			al.logger.Printf("Failed to write to audit writer: %v", err)
		}
	}
	
	// Clear buffer
	al.buffer = al.buffer[:0]
	
	// Update metrics
	al.metrics.ProcessingTime = time.Since(start)
	al.metrics.LastFlush = time.Now()
	
	return nil
}

// Helper methods

// generateEventID generates a unique event ID
func (al *AuditLogger) generateEventID() string {
	return fmt.Sprintf("audit_%d_%s", time.Now().UnixNano(), al.generateRandomString(8))
}

// generateRandomString generates a random string
func (al *AuditLogger) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// calculateChecksum calculates event checksum
func (al *AuditLogger) calculateChecksum(event *AuditEvent) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s", 
		event.ID, event.Timestamp.String(), event.Category, event.Action, event.Message)
	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// signEvent signs an event
func (al *AuditLogger) signEvent(event *AuditEvent) string {
	// Simplified signing - in production, use proper cryptographic signing
	data := event.Checksum + al.config.SigningKey
	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// updateCorrelation updates correlation context
func (al *AuditLogger) updateCorrelation(event *AuditEvent) {
	ctx, exists := al.correlationMap[event.CorrelationID]
	if !exists {
		ctx = &CorrelationContext{
			ID:        event.CorrelationID,
			StartTime: event.Timestamp,
			Events:    make([]string, 0),
			Metadata:  make(map[string]interface{}),
		}
		al.correlationMap[event.CorrelationID] = ctx
	}
	
	ctx.EventCount++
	ctx.LastEvent = event.Timestamp
	ctx.Events = append(ctx.Events, event.ID)
}

// Helper functions for extracting data from contexts

func (al *AuditLogger) statusFromResult(result *AccessResult) string {
	if result.Granted {
		return "granted"
	}
	return "denied"
}

func (al *AuditLogger) getIPFromContext(ctx *AccessContext) string {
	if ctx != nil {
		return ctx.IPAddress
	}
	return ""
}

func (al *AuditLogger) getUserAgentFromContext(ctx *AccessContext) string {
	if ctx != nil {
		return ctx.UserAgent
	}
	return ""
}

func (al *AuditLogger) buildRequestContext(ctx *AccessContext) *RequestContext {
	if ctx == nil {
		return nil
	}
	
	return &RequestContext{
		RequestID: ctx.RequestID,
	}
}

func (al *AuditLogger) getSeverityFromResult(result *AccessResult) Severity {
	if result.Granted {
		return SeverityLow
	}
	return SeverityMedium
}

func (al *AuditLogger) getRiskFromResult(result *AccessResult) RiskLevel {
	if result.Granted {
		return RiskLow
	}
	return RiskMedium
}

func (al *AuditLogger) getDataAccessSeverity(action string) Severity {
	switch strings.ToLower(action) {
	case "delete":
		return SeverityHigh
	case "write", "update":
		return SeverityMedium
	default:
		return SeverityLow
	}
}

func (al *AuditLogger) getDataAccessRisk(action string) RiskLevel {
	switch strings.ToLower(action) {
	case "delete":
		return RiskHigh
	case "write", "update":
		return RiskMedium
	default:
		return RiskLow
	}
}

func (al *AuditLogger) getRiskFromSeverity(severity Severity) RiskLevel {
	switch severity {
	case SeverityCritical, SeverityHigh:
		return RiskHigh
	case SeverityMedium:
		return RiskMedium
	default:
		return RiskLow
	}
}

// GetMetrics returns audit metrics
func (al *AuditLogger) GetMetrics() *AuditMetrics {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	metricsCopy := *al.metrics
	return &metricsCopy
}

// Stop stops the audit logger
func (al *AuditLogger) Stop() error {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	if !al.isRunning {
		return nil
	}
	
	// Stop processing
	close(al.stopCh)
	
	// Stop ticker
	if al.flushTicker != nil {
		al.flushTicker.Stop()
	}
	
	// Close all writers
	for _, writer := range al.writers {
		writer.Close()
	}
	
	al.isRunning = false
	al.logger.Printf("Audit logger stopped")
	
	return nil
}

// Writer implementations

// ConsoleWriter writes events to console
type ConsoleWriter struct {
	logger *log.Logger
}

func (cw *ConsoleWriter) Write(event *AuditEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	cw.logger.Printf("AUDIT: %s", string(data))
	return nil
}

func (cw *ConsoleWriter) WriteBatch(events []*AuditEvent) error {
	for _, event := range events {
		if err := cw.Write(event); err != nil {
			return err
		}
	}
	return nil
}

func (cw *ConsoleWriter) Close() error {
	return nil
}

// FileWriter writes events to files
type FileWriter struct {
	config   FileWriterConfig
	file     *os.File
	size     int64
	mu       sync.Mutex
}

type FileWriterConfig struct {
	Directory       string
	FileNamePattern string
	MaxSize         int64
	MaxBackups      int
	MaxAge          int
	Compress        bool
}

func NewFileWriter(config FileWriterConfig) (*FileWriter, error) {
	fw := &FileWriter{
		config: config,
	}
	
	if err := fw.openFile(); err != nil {
		return nil, err
	}
	
	return fw, nil
}

func (fw *FileWriter) openFile() error {
	// Generate filename
	filename := fw.generateFilename()
	filepath := filepath.Join(fw.config.Directory, filename)
	
	// Create directory if needed
	if err := os.MkdirAll(fw.config.Directory, 0755); err != nil {
		return err
	}
	
	// Open file
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	
	// Get current size
	if info, err := file.Stat(); err == nil {
		fw.size = info.Size()
	}
	
	fw.file = file
	return nil
}

func (fw *FileWriter) generateFilename() string {
	now := time.Now()
	return now.Format("audit-20060102.log")
}

func (fw *FileWriter) Write(event *AuditEvent) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	
	data = append(data, '\n')
	
	// Check if we need to rotate
	if fw.size+int64(len(data)) > fw.config.MaxSize {
		if err := fw.rotate(); err != nil {
			return err
		}
	}
	
	n, err := fw.file.Write(data)
	if err != nil {
		return err
	}
	
	fw.size += int64(n)
	return nil
}

func (fw *FileWriter) WriteBatch(events []*AuditEvent) error {
	for _, event := range events {
		if err := fw.Write(event); err != nil {
			return err
		}
	}
	return nil
}

func (fw *FileWriter) rotate() error {
	// Close current file
	if fw.file != nil {
		fw.file.Close()
	}
	
	// Open new file
	return fw.openFile()
}

func (fw *FileWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	if fw.file != nil {
		return fw.file.Close()
	}
	return nil
}

// SyslogWriter writes events to syslog
type SyslogWriter struct {
	// Simplified implementation
}

func NewSyslogWriter() (*SyslogWriter, error) {
	return &SyslogWriter{}, nil
}

func (sw *SyslogWriter) Write(event *AuditEvent) error {
	// Implementation would use syslog package
	return nil
}

func (sw *SyslogWriter) WriteBatch(events []*AuditEvent) error {
	for _, event := range events {
		if err := sw.Write(event); err != nil {
			return err
		}
	}
	return nil
}

func (sw *SyslogWriter) Close() error {
	return nil
}

// RemoteWriter writes events to remote endpoint
type RemoteWriter struct {
	endpoint string
	apiKey   string
	timeout  time.Duration
}

func (rw *RemoteWriter) Write(event *AuditEvent) error {
	// Implementation would send HTTP POST to remote endpoint
	return nil
}

func (rw *RemoteWriter) WriteBatch(events []*AuditEvent) error {
	// Implementation would send batch to remote endpoint
	return nil
}

func (rw *RemoteWriter) Close() error {
	return nil
}

// Filter implementations

// LevelFilter filters events by log level
type LevelFilter struct {
	minLevel LogLevel
}

func (lf *LevelFilter) ShouldLog(event *AuditEvent) bool {
	return al.compareLevels(event.Level, lf.minLevel) >= 0
}

// CategoryFilter filters events by category
type CategoryFilter struct {
	include []string
	exclude []string
}

func (cf *CategoryFilter) ShouldLog(event *AuditEvent) bool {
	category := string(event.Category)
	
	// Check exclude list first
	for _, exclude := range cf.exclude {
		if exclude == category {
			return false
		}
	}
	
	// Check include list
	if len(cf.include) > 0 {
		for _, include := range cf.include {
			if include == category {
				return true
			}
		}
		return false
	}
	
	return true
}

// ActionFilter filters events by action
type ActionFilter struct {
	include []string
	exclude []string
}

func (af *ActionFilter) ShouldLog(event *AuditEvent) bool {
	action := event.Action
	
	// Check exclude list first
	for _, exclude := range af.exclude {
		if exclude == action {
			return false
		}
	}
	
	// Check include list
	if len(af.include) > 0 {
		for _, include := range af.include {
			if include == action {
				return true
			}
		}
		return false
	}
	
	return true
}

// Enricher implementations

// EnvironmentEnricher adds environment information
type EnvironmentEnricher struct{}

func (ee *EnvironmentEnricher) Enrich(event *AuditEvent) error {
	if event.Metadata == nil {
		event.Metadata = make(map[string]interface{})
	}
	
	event.Metadata["hostname"], _ = os.Hostname()
	event.Metadata["pid"] = os.Getpid()
	
	return nil
}

// RequestIDEnricher adds request ID
type RequestIDEnricher struct{}

func (re *RequestIDEnricher) Enrich(event *AuditEvent) error {
	if event.RequestContext == nil {
		event.RequestContext = &RequestContext{}
	}
	
	if event.RequestContext.RequestID == "" {
		event.RequestContext.RequestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	
	return nil
}

// AnonymizationEnricher anonymizes sensitive data
type AnonymizationEnricher struct{}

func (ae *AnonymizationEnricher) Enrich(event *AuditEvent) error {
	// Anonymize actor email
	if event.Actor != nil && event.Actor.Email != "" {
		event.Actor.Email = ae.anonymizeEmail(event.Actor.Email)
	}
	
	// Anonymize IP addresses
	if event.Actor != nil && event.Actor.IPAddress != "" {
		event.Actor.IPAddress = ae.anonymizeIP(event.Actor.IPAddress)
	}
	
	return nil
}

func (ae *AnonymizationEnricher) anonymizeEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return "***@" + parts[1]
	}
	return "***"
}

func (ae *AnonymizationEnricher) anonymizeIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.*.*", parts[0], parts[1])
	}
	return "***"
}

// compareLevels compares log levels
func (al *AuditLogger) compareLevels(a, b LogLevel) int {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
		LogLevelFatal: 4,
	}
	
	return levels[a] - levels[b]
}