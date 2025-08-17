package security

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// RateLimiter manages API rate limiting
type RateLimiter struct {
	mu          sync.RWMutex
	logger      *log.Logger
	config      RateLimitConfig
	limiters    map[string]*ClientLimiter
	globalStats *GlobalRateStats
	rules       map[string]*RateLimitRule
	blacklist   map[string]*BlacklistEntry
	whitelist   map[string]*WhitelistEntry
	metrics     *RateLimitMetrics
	cleanupStop chan struct{}
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	// Global limits
	GlobalRateLimit    int64         `json:"global_rate_limit"`    // Global requests per second
	GlobalBurstLimit   int64         `json:"global_burst_limit"`   // Global burst limit
	
	// Default limits
	DefaultRateLimit   int64         `json:"default_rate_limit"`   // Default requests per second per client
	DefaultBurstLimit  int64         `json:"default_burst_limit"`  // Default burst limit per client
	
	// Window configuration
	WindowSize         time.Duration `json:"window_size"`          // Rate limit window size
	SlidingWindow      bool          `json:"sliding_window"`       // Use sliding window vs fixed window
	
	// Client identification
	IdentifyBy         []string      `json:"identify_by"`          // Client identification methods
	HeaderName         string        `json:"header_name"`          // Custom header for client ID
	TrustForwardedFor  bool          `json:"trust_forwarded_for"`  // Trust X-Forwarded-For header
	
	// Behavior configuration
	EnableBlacklist    bool          `json:"enable_blacklist"`     // Enable IP blacklisting
	EnableWhitelist    bool          `json:"enable_whitelist"`     // Enable IP whitelisting
	BlockDuration      time.Duration `json:"block_duration"`       // Duration to block violators
	
	// Response configuration
	ReturnHeaders      bool          `json:"return_headers"`       // Return rate limit headers
	CustomResponse     *CustomResponse `json:"custom_response"`    // Custom rate limit response
	
	// Storage configuration
	StorageType        string        `json:"storage_type"`         // memory, redis, etc.
	RedisConfig        *RedisConfig  `json:"redis_config"`         // Redis configuration
	
	// Cleanup configuration
	CleanupInterval    time.Duration `json:"cleanup_interval"`     // Cleanup interval
	MaxIdleTime        time.Duration `json:"max_idle_time"`        // Max idle time before cleanup
	
	// Advanced features
	EnableAdaptive     bool          `json:"enable_adaptive"`      // Enable adaptive rate limiting
	EnableBurst        bool          `json:"enable_burst"`         // Enable burst handling
	StrictMode         bool          `json:"strict_mode"`          // Strict enforcement mode
}

// CustomResponse defines custom rate limit response
type CustomResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Format     string            `json:"format"` // json, xml, text
}

// RedisConfig contains Redis configuration for distributed rate limiting
type RedisConfig struct {
	Endpoints []string      `json:"endpoints"`
	Password  string        `json:"password"`
	Database  int           `json:"database"`
	KeyPrefix string        `json:"key_prefix"`
	Timeout   time.Duration `json:"timeout"`
}

// ClientLimiter manages rate limiting for a specific client
type ClientLimiter struct {
	mu             sync.RWMutex
	clientID       string
	rateLimit      int64
	burstLimit     int64
	windowSize     time.Duration
	tokens         int64
	lastRefill     time.Time
	requests       []RequestInfo
	blocked        bool
	blockedUntil   time.Time
	stats          *ClientStats
	violations     []ViolationRecord
}

// RequestInfo contains information about a request
type RequestInfo struct {
	Timestamp time.Time         `json:"timestamp"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	UserAgent string            `json:"user_agent"`
	Size      int64             `json:"size"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ClientStats tracks statistics for a client
type ClientStats struct {
	TotalRequests     int64     `json:"total_requests"`
	AllowedRequests   int64     `json:"allowed_requests"`
	BlockedRequests   int64     `json:"blocked_requests"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
	ViolationCount    int       `json:"violation_count"`
	CurrentRate       float64   `json:"current_rate"`
	AverageRate       float64   `json:"average_rate"`
	PeakRate          float64   `json:"peak_rate"`
}

// ViolationRecord records a rate limit violation
type ViolationRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	RequestRate float64   `json:"request_rate"`
	Limit       int64     `json:"limit"`
	Severity    string    `json:"severity"`
	Action      string    `json:"action"`
}

// RateLimitRule defines custom rate limiting rules
type RateLimitRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Pattern     string            `json:"pattern"`      // URL pattern
	Method      string            `json:"method"`       // HTTP method
	RateLimit   int64             `json:"rate_limit"`   // Requests per second
	BurstLimit  int64             `json:"burst_limit"`  // Burst limit
	WindowSize  time.Duration     `json:"window_size"`  // Time window
	Priority    int               `json:"priority"`     // Rule priority
	Conditions  []RuleCondition   `json:"conditions"`   // Additional conditions
	Actions     []RuleAction      `json:"actions"`      // Actions to take
	IsActive    bool              `json:"is_active"`    // Rule is active
	Metadata    map[string]interface{} `json:"metadata"`
}

// RuleCondition defines conditions for rate limit rules
type RuleCondition struct {
	Type      string      `json:"type"`      // header, query, body, etc.
	Field     string      `json:"field"`     // Field name
	Operator  string      `json:"operator"`  // eq, ne, contains, etc.
	Value     interface{} `json:"value"`     // Expected value
}

// RuleAction defines actions for rate limit violations
type RuleAction struct {
	Type       string            `json:"type"`       // block, throttle, alert, etc.
	Duration   time.Duration     `json:"duration"`   // Action duration
	Severity   string            `json:"severity"`   // Action severity
	Parameters map[string]interface{} `json:"parameters"` // Action parameters
}

// BlacklistEntry represents a blacklisted client
type BlacklistEntry struct {
	ClientID    string            `json:"client_id"`
	Reason      string            `json:"reason"`
	BlockedAt   time.Time         `json:"blocked_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	ViolationCount int            `json:"violation_count"`
	IsPermanent bool              `json:"is_permanent"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// WhitelistEntry represents a whitelisted client
type WhitelistEntry struct {
	ClientID    string            `json:"client_id"`
	Reason      string            `json:"reason"`
	AddedAt     time.Time         `json:"added_at"`
	ExpiresAt   *time.Time        `json:"expires_at"`
	IsPermanent bool              `json:"is_permanent"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	Allowed        bool              `json:"allowed"`
	RemainingTokens int64            `json:"remaining_tokens"`
	ResetTime      time.Time         `json:"reset_time"`
	RetryAfter     time.Duration     `json:"retry_after"`
	RateLimit      int64             `json:"rate_limit"`
	WindowSize     time.Duration     `json:"window_size"`
	Violation      *ViolationRecord  `json:"violation,omitempty"`
	Headers        map[string]string `json:"headers"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// RateLimitRequest contains information about a request to be rate limited
type RateLimitRequest struct {
	ClientID    string            `json:"client_id"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	IPAddress   string            `json:"ip_address"`
	UserAgent   string            `json:"user_agent"`
	Headers     map[string]string `json:"headers"`
	Size        int64             `json:"size"`
	Timestamp   time.Time         `json:"timestamp"`
	UserID      string            `json:"user_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// GlobalRateStats tracks global rate limiting statistics
type GlobalRateStats struct {
	mu                  sync.RWMutex
	TotalRequests       int64     `json:"total_requests"`
	AllowedRequests     int64     `json:"allowed_requests"`
	BlockedRequests     int64     `json:"blocked_requests"`
	CurrentRPS          float64   `json:"current_rps"`
	PeakRPS             float64   `json:"peak_rps"`
	ActiveClients       int       `json:"active_clients"`
	BlacklistedClients  int       `json:"blacklisted_clients"`
	WhitelistedClients  int       `json:"whitelisted_clients"`
	LastResetTime       time.Time `json:"last_reset_time"`
}

// RateLimitMetrics tracks detailed rate limiting metrics
type RateLimitMetrics struct {
	RequestsByClient    map[string]int64  `json:"requests_by_client"`
	ViolationsByClient  map[string]int64  `json:"violations_by_client"`
	BlocksByReason      map[string]int64  `json:"blocks_by_reason"`
	RequestsByEndpoint  map[string]int64  `json:"requests_by_endpoint"`
	RequestsByMethod    map[string]int64  `json:"requests_by_method"`
	ResponseTimes       []time.Duration   `json:"response_times"`
	TopViolators        []string          `json:"top_violators"`
	HourlyStats         map[string]int64  `json:"hourly_stats"`
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig, logger *log.Logger) *RateLimiter {
	if logger == nil {
		logger = log.New(log.Writer(), "[RATELIMIT] ", log.LstdFlags)
	}
	
	// Set default values
	if config.DefaultRateLimit == 0 {
		config.DefaultRateLimit = 100 // 100 requests per second
	}
	if config.DefaultBurstLimit == 0 {
		config.DefaultBurstLimit = 200 // 200 burst limit
	}
	if config.WindowSize == 0 {
		config.WindowSize = time.Second
	}
	if config.BlockDuration == 0 {
		config.BlockDuration = 5 * time.Minute
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 10 * time.Minute
	}
	if config.MaxIdleTime == 0 {
		config.MaxIdleTime = time.Hour
	}
	if len(config.IdentifyBy) == 0 {
		config.IdentifyBy = []string{"ip", "user_id"}
	}
	if config.HeaderName == "" {
		config.HeaderName = "X-Client-ID"
	}
	
	rl := &RateLimiter{
		logger:      logger,
		config:      config,
		limiters:    make(map[string]*ClientLimiter),
		globalStats: &GlobalRateStats{LastResetTime: time.Now()},
		rules:       make(map[string]*RateLimitRule),
		blacklist:   make(map[string]*BlacklistEntry),
		whitelist:   make(map[string]*WhitelistEntry),
		metrics: &RateLimitMetrics{
			RequestsByClient:   make(map[string]int64),
			ViolationsByClient: make(map[string]int64),
			BlocksByReason:     make(map[string]int64),
			RequestsByEndpoint: make(map[string]int64),
			RequestsByMethod:   make(map[string]int64),
			HourlyStats:        make(map[string]int64),
		},
		cleanupStop: make(chan struct{}),
	}
	
	// Initialize default rules
	rl.initializeDefaultRules()
	
	// Start cleanup routine
	go rl.cleanupRoutine()
	
	logger.Printf("Rate limiter initialized with default limit: %d req/s, burst: %d", 
		config.DefaultRateLimit, config.DefaultBurstLimit)
	
	return rl
}

// initializeDefaultRules creates default rate limiting rules
func (rl *RateLimiter) initializeDefaultRules() {
	// Default API rate limit rule
	rl.rules["default_api"] = &RateLimitRule{
		ID:         "default_api",
		Name:       "Default API Rate Limit",
		Pattern:    "/api/*",
		Method:     "*",
		RateLimit:  rl.config.DefaultRateLimit,
		BurstLimit: rl.config.DefaultBurstLimit,
		WindowSize: rl.config.WindowSize,
		Priority:   1000,
		IsActive:   true,
	}
	
	// Strict rate limit for authentication endpoints
	rl.rules["auth_strict"] = &RateLimitRule{
		ID:         "auth_strict",
		Name:       "Authentication Strict Limit",
		Pattern:    "/api/auth/*",
		Method:     "POST",
		RateLimit:  5, // 5 requests per second
		BurstLimit: 10,
		WindowSize: time.Second,
		Priority:   100, // Higher priority
		IsActive:   true,
		Actions: []RuleAction{
			{
				Type:     "block",
				Duration: 15 * time.Minute,
				Severity: "high",
			},
		},
	}
	
	// Lenient rate limit for read operations
	rl.rules["read_lenient"] = &RateLimitRule{
		ID:         "read_lenient",
		Name:       "Read Operations Lenient Limit",
		Pattern:    "/api/data/*",
		Method:     "GET",
		RateLimit:  500, // 500 requests per second
		BurstLimit: 1000,
		WindowSize: time.Second,
		Priority:   500,
		IsActive:   true,
	}
	
	// Strict rate limit for write operations
	rl.rules["write_strict"] = &RateLimitRule{
		ID:         "write_strict",
		Name:       "Write Operations Strict Limit",
		Pattern:    "/api/data/*",
		Method:     "POST,PUT,DELETE",
		RateLimit:  50, // 50 requests per second
		BurstLimit: 100,
		WindowSize: time.Second,
		Priority:   200,
		IsActive:   true,
	}
}

// CheckRateLimit checks if a request should be rate limited
func (rl *RateLimiter) CheckRateLimit(ctx context.Context, req *RateLimitRequest) (*RateLimitResult, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// Set request timestamp if not provided
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}
	
	// Extract client ID
	clientID := rl.extractClientID(req)
	req.ClientID = clientID
	
	// Update global stats
	rl.globalStats.mu.Lock()
	rl.globalStats.TotalRequests++
	rl.globalStats.mu.Unlock()
	
	// Update metrics
	rl.metrics.RequestsByClient[clientID]++
	rl.metrics.RequestsByEndpoint[req.Path]++
	rl.metrics.RequestsByMethod[req.Method]++
	
	// Track hourly stats
	hourKey := req.Timestamp.Format("2006-01-02-15")
	rl.metrics.HourlyStats[hourKey]++
	
	// Check whitelist
	if rl.config.EnableWhitelist && rl.isWhitelisted(clientID) {
		return &RateLimitResult{
			Allowed:         true,
			RemainingTokens: rl.config.DefaultBurstLimit,
			ResetTime:       time.Now().Add(rl.config.WindowSize),
			RateLimit:       rl.config.DefaultRateLimit,
			WindowSize:      rl.config.WindowSize,
			Headers:         rl.generateHeaders(rl.config.DefaultRateLimit, rl.config.DefaultBurstLimit, time.Now()),
			Metadata:        map[string]interface{}{"whitelisted": true},
		}, nil
	}
	
	// Check blacklist
	if rl.config.EnableBlacklist && rl.isBlacklisted(clientID) {
		violation := &ViolationRecord{
			Timestamp: req.Timestamp,
			Severity:  "critical",
			Action:    "blocked_blacklisted",
		}
		
		rl.globalStats.mu.Lock()
		rl.globalStats.BlockedRequests++
		rl.globalStats.mu.Unlock()
		
		rl.metrics.ViolationsByClient[clientID]++
		rl.metrics.BlocksByReason["blacklisted"]++
		
		return &RateLimitResult{
			Allowed:    false,
			RetryAfter: time.Hour, // Long retry time for blacklisted clients
			Violation:  violation,
			Headers:    rl.generateErrorHeaders(),
			Metadata:   map[string]interface{}{"blacklisted": true},
		}, nil
	}
	
	// Check global rate limit
	if rl.config.GlobalRateLimit > 0 {
		if !rl.checkGlobalRateLimit() {
			violation := &ViolationRecord{
				Timestamp: req.Timestamp,
				Severity:  "high",
				Action:    "global_rate_limit_exceeded",
			}
			
			rl.globalStats.mu.Lock()
			rl.globalStats.BlockedRequests++
			rl.globalStats.mu.Unlock()
			
			rl.metrics.BlocksByReason["global_limit"]++
			
			return &RateLimitResult{
				Allowed:    false,
				RetryAfter: time.Second,
				Violation:  violation,
				Headers:    rl.generateErrorHeaders(),
				Metadata:   map[string]interface{}{"global_limit_exceeded": true},
			}, nil
		}
	}
	
	// Get or create client limiter
	limiter := rl.getOrCreateLimiter(clientID, req)
	
	// Apply rate limiting rules
	rule := rl.findMatchingRule(req)
	if rule != nil {
		result := rl.applyRuleLimit(limiter, req, rule)
		
		// Update global stats
		rl.globalStats.mu.Lock()
		if result.Allowed {
			rl.globalStats.AllowedRequests++
		} else {
			rl.globalStats.BlockedRequests++
		}
		rl.globalStats.mu.Unlock()
		
		return result, nil
	}
	
	// Apply default rate limit
	result := rl.applyDefaultLimit(limiter, req)
	
	// Update global stats
	rl.globalStats.mu.Lock()
	if result.Allowed {
		rl.globalStats.AllowedRequests++
	} else {
		rl.globalStats.BlockedRequests++
	}
	rl.globalStats.mu.Unlock()
	
	return result, nil
}

// extractClientID extracts client identifier from request
func (rl *RateLimiter) extractClientID(req *RateLimitRequest) string {
	var identifiers []string
	
	for _, method := range rl.config.IdentifyBy {
		switch method {
		case "ip":
			if req.IPAddress != "" {
				identifiers = append(identifiers, "ip:"+req.IPAddress)
			}
		case "user_id":
			if req.UserID != "" {
				identifiers = append(identifiers, "user:"+req.UserID)
			}
		case "header":
			if headerValue, exists := req.Headers[rl.config.HeaderName]; exists {
				identifiers = append(identifiers, "header:"+headerValue)
			}
		case "user_agent":
			if req.UserAgent != "" {
				identifiers = append(identifiers, "ua:"+req.UserAgent)
			}
		}
	}
	
	if len(identifiers) == 0 {
		return "unknown"
	}
	
	return strings.Join(identifiers, "|")
}

// isWhitelisted checks if a client is whitelisted
func (rl *RateLimiter) isWhitelisted(clientID string) bool {
	entry, exists := rl.whitelist[clientID]
	if !exists {
		return false
	}
	
	// Check expiration
	if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
		delete(rl.whitelist, clientID)
		return false
	}
	
	return true
}

// isBlacklisted checks if a client is blacklisted
func (rl *RateLimiter) isBlacklisted(clientID string) bool {
	entry, exists := rl.blacklist[clientID]
	if !exists {
		return false
	}
	
	// Check expiration
	if !entry.IsPermanent && time.Now().After(entry.ExpiresAt) {
		delete(rl.blacklist, clientID)
		return false
	}
	
	return true
}

// checkGlobalRateLimit checks global rate limit
func (rl *RateLimiter) checkGlobalRateLimit() bool {
	rl.globalStats.mu.RLock()
	defer rl.globalStats.mu.RUnlock()
	
	now := time.Now()
	timeSinceReset := now.Sub(rl.globalStats.LastResetTime)
	
	if timeSinceReset >= rl.config.WindowSize {
		// Reset window
		rl.globalStats.CurrentRPS = 0
		rl.globalStats.LastResetTime = now
		return true
	}
	
	// Check current rate
	requestsInWindow := rl.globalStats.TotalRequests
	currentRate := float64(requestsInWindow) / timeSinceReset.Seconds()
	
	return currentRate <= float64(rl.config.GlobalRateLimit)
}

// getOrCreateLimiter gets or creates a client limiter
func (rl *RateLimiter) getOrCreateLimiter(clientID string, req *RateLimitRequest) *ClientLimiter {
	limiter, exists := rl.limiters[clientID]
	if !exists {
		limiter = &ClientLimiter{
			clientID:     clientID,
			rateLimit:    rl.config.DefaultRateLimit,
			burstLimit:   rl.config.DefaultBurstLimit,
			windowSize:   rl.config.WindowSize,
			tokens:       rl.config.DefaultBurstLimit,
			lastRefill:   time.Now(),
			requests:     make([]RequestInfo, 0),
			stats: &ClientStats{
				FirstSeen: req.Timestamp,
				LastSeen:  req.Timestamp,
			},
		}
		rl.limiters[clientID] = limiter
		
		rl.globalStats.mu.Lock()
		rl.globalStats.ActiveClients++
		rl.globalStats.mu.Unlock()
	}
	
	return limiter
}

// findMatchingRule finds the best matching rate limit rule
func (rl *RateLimiter) findMatchingRule(req *RateLimitRequest) *RateLimitRule {
	var bestRule *RateLimitRule
	highestPriority := int(^uint(0) >> 1) // Max int
	
	for _, rule := range rl.rules {
		if !rule.IsActive {
			continue
		}
		
		if rl.ruleMatches(rule, req) && rule.Priority < highestPriority {
			bestRule = rule
			highestPriority = rule.Priority
		}
	}
	
	return bestRule
}

// ruleMatches checks if a rule matches the request
func (rl *RateLimiter) ruleMatches(rule *RateLimitRule, req *RateLimitRequest) bool {
	// Check URL pattern
	if !rl.patternMatches(rule.Pattern, req.Path) {
		return false
	}
	
	// Check HTTP method
	if rule.Method != "*" {
		methods := strings.Split(rule.Method, ",")
		methodMatches := false
		for _, method := range methods {
			if strings.TrimSpace(method) == req.Method {
				methodMatches = true
				break
			}
		}
		if !methodMatches {
			return false
		}
	}
	
	// Check additional conditions
	for _, condition := range rule.Conditions {
		if !rl.conditionMatches(&condition, req) {
			return false
		}
	}
	
	return true
}

// patternMatches checks if a URL pattern matches the path
func (rl *RateLimiter) patternMatches(pattern, path string) bool {
	if pattern == "*" {
		return true
	}
	
	// Simple wildcard matching
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	
	return pattern == path
}

// conditionMatches checks if a condition matches the request
func (rl *RateLimiter) conditionMatches(condition *RuleCondition, req *RateLimitRequest) bool {
	var value interface{}
	
	switch condition.Type {
	case "header":
		value = req.Headers[condition.Field]
	case "query":
		// Would extract from query parameters
		value = ""
	case "size":
		value = req.Size
	default:
		return false
	}
	
	return rl.compareConditionValue(value, condition.Operator, condition.Value)
}

// compareConditionValue compares condition values
func (rl *RateLimiter) compareConditionValue(actual interface{}, operator string, expected interface{}) bool {
	switch operator {
	case "eq":
		return actual == expected
	case "ne":
		return actual != expected
	case "gt":
		if actualInt, ok := actual.(int64); ok {
			if expectedInt, ok := expected.(int64); ok {
				return actualInt > expectedInt
			}
		}
	case "lt":
		if actualInt, ok := actual.(int64); ok {
			if expectedInt, ok := expected.(int64); ok {
				return actualInt < expectedInt
			}
		}
	case "contains":
		if actualStr, ok := actual.(string); ok {
			if expectedStr, ok := expected.(string); ok {
				return strings.Contains(actualStr, expectedStr)
			}
		}
	}
	
	return false
}

// applyRuleLimit applies rate limiting based on a specific rule
func (rl *RateLimiter) applyRuleLimit(limiter *ClientLimiter, req *RateLimitRequest, rule *RateLimitRule) *RateLimitResult {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	
	// Update limiter configuration from rule
	limiter.rateLimit = rule.RateLimit
	limiter.burstLimit = rule.BurstLimit
	limiter.windowSize = rule.WindowSize
	
	return rl.processRateLimit(limiter, req, rule)
}

// applyDefaultLimit applies default rate limiting
func (rl *RateLimiter) applyDefaultLimit(limiter *ClientLimiter, req *RateLimitRequest) *RateLimitResult {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	
	return rl.processRateLimit(limiter, req, nil)
}

// processRateLimit processes the rate limiting logic
func (rl *RateLimiter) processRateLimit(limiter *ClientLimiter, req *RateLimitRequest, rule *RateLimitRule) *RateLimitResult {
	now := req.Timestamp
	
	// Check if client is currently blocked
	if limiter.blocked && now.Before(limiter.blockedUntil) {
		violation := &ViolationRecord{
			Timestamp: now,
			Severity:  "medium",
			Action:    "blocked_temporary",
		}
		
		rl.metrics.ViolationsByClient[limiter.clientID]++
		rl.metrics.BlocksByReason["temporary_block"]++
		
		return &RateLimitResult{
			Allowed:    false,
			RetryAfter: limiter.blockedUntil.Sub(now),
			Violation:  violation,
			Headers:    rl.generateErrorHeaders(),
			Metadata:   map[string]interface{}{"temporarily_blocked": true},
		}
	}
	
	// Unblock if block period has expired
	if limiter.blocked && now.After(limiter.blockedUntil) {
		limiter.blocked = false
		limiter.blockedUntil = time.Time{}
	}
	
	// Refill tokens using token bucket algorithm
	rl.refillTokens(limiter, now)
	
	// Update request tracking
	limiter.requests = append(limiter.requests, RequestInfo{
		Timestamp: now,
		Method:    req.Method,
		Path:      req.Path,
		UserAgent: req.UserAgent,
		Size:      req.Size,
		Metadata:  req.Metadata,
	})
	
	// Clean old requests (sliding window)
	if rl.config.SlidingWindow {
		rl.cleanOldRequests(limiter, now)
	}
	
	// Update stats
	limiter.stats.TotalRequests++
	limiter.stats.LastSeen = now
	rl.updateClientStats(limiter, now)
	
	// Check if request should be allowed
	if limiter.tokens > 0 {
		limiter.tokens--
		limiter.stats.AllowedRequests++
		
		return &RateLimitResult{
			Allowed:         true,
			RemainingTokens: limiter.tokens,
			ResetTime:       rl.calculateResetTime(limiter),
			RateLimit:       limiter.rateLimit,
			WindowSize:      limiter.windowSize,
			Headers:         rl.generateHeaders(limiter.rateLimit, limiter.tokens, rl.calculateResetTime(limiter)),
			Metadata:        map[string]interface{}{"client_id": limiter.clientID},
		}
	}
	
	// Rate limit exceeded
	limiter.stats.BlockedRequests++
	
	// Record violation
	violation := &ViolationRecord{
		Timestamp:   now,
		RequestRate: rl.calculateCurrentRate(limiter, now),
		Limit:       limiter.rateLimit,
		Severity:    "medium",
		Action:      "rate_limited",
	}
	limiter.violations = append(limiter.violations, *violation)
	
	// Apply rule actions if any
	if rule != nil && len(rule.Actions) > 0 {
		rl.applyRuleActions(limiter, rule.Actions, violation)
	}
	
	rl.metrics.ViolationsByClient[limiter.clientID]++
	rl.metrics.BlocksByReason["rate_limit_exceeded"]++
	
	return &RateLimitResult{
		Allowed:         false,
		RemainingTokens: 0,
		ResetTime:       rl.calculateResetTime(limiter),
		RetryAfter:      rl.calculateRetryAfter(limiter),
		RateLimit:       limiter.rateLimit,
		WindowSize:      limiter.windowSize,
		Violation:       violation,
		Headers:         rl.generateErrorHeaders(),
		Metadata:        map[string]interface{}{"client_id": limiter.clientID},
	}
}

// refillTokens refills tokens using token bucket algorithm
func (rl *RateLimiter) refillTokens(limiter *ClientLimiter, now time.Time) {
	elapsed := now.Sub(limiter.lastRefill)
	if elapsed <= 0 {
		return
	}
	
	tokensToAdd := int64(elapsed.Seconds() * float64(limiter.rateLimit))
	limiter.tokens += tokensToAdd
	
	// Cap at burst limit
	if limiter.tokens > limiter.burstLimit {
		limiter.tokens = limiter.burstLimit
	}
	
	limiter.lastRefill = now
}

// cleanOldRequests removes old requests for sliding window
func (rl *RateLimiter) cleanOldRequests(limiter *ClientLimiter, now time.Time) {
	cutoff := now.Add(-limiter.windowSize)
	validRequests := make([]RequestInfo, 0, len(limiter.requests))
	
	for _, req := range limiter.requests {
		if req.Timestamp.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	
	limiter.requests = validRequests
}

// updateClientStats updates client statistics
func (rl *RateLimiter) updateClientStats(limiter *ClientLimiter, now time.Time) {
	// Calculate current rate
	windowStart := now.Add(-limiter.windowSize)
	requestsInWindow := 0
	
	for _, req := range limiter.requests {
		if req.Timestamp.After(windowStart) {
			requestsInWindow++
		}
	}
	
	limiter.stats.CurrentRate = float64(requestsInWindow) / limiter.windowSize.Seconds()
	
	// Update peak rate
	if limiter.stats.CurrentRate > limiter.stats.PeakRate {
		limiter.stats.PeakRate = limiter.stats.CurrentRate
	}
	
	// Update average rate
	totalDuration := limiter.stats.LastSeen.Sub(limiter.stats.FirstSeen)
	if totalDuration > 0 {
		limiter.stats.AverageRate = float64(limiter.stats.TotalRequests) / totalDuration.Seconds()
	}
}

// calculateCurrentRate calculates current request rate
func (rl *RateLimiter) calculateCurrentRate(limiter *ClientLimiter, now time.Time) float64 {
	windowStart := now.Add(-limiter.windowSize)
	requestsInWindow := 0
	
	for _, req := range limiter.requests {
		if req.Timestamp.After(windowStart) {
			requestsInWindow++
		}
	}
	
	return float64(requestsInWindow) / limiter.windowSize.Seconds()
}

// applyRuleActions applies actions defined in rate limit rules
func (rl *RateLimiter) applyRuleActions(limiter *ClientLimiter, actions []RuleAction, violation *ViolationRecord) {
	for _, action := range actions {
		switch action.Type {
		case "block":
			limiter.blocked = true
			limiter.blockedUntil = time.Now().Add(action.Duration)
			violation.Action = "blocked_by_rule"
			violation.Severity = action.Severity
		case "throttle":
			// Reduce rate limit temporarily
			limiter.rateLimit = limiter.rateLimit / 2
			violation.Action = "throttled"
		case "alert":
			// Send alert (implementation would depend on alerting system)
			rl.logger.Printf("Rate limit alert for client %s: %s", limiter.clientID, violation.Action)
		}
	}
}

// calculateResetTime calculates when the rate limit will reset
func (rl *RateLimiter) calculateResetTime(limiter *ClientLimiter) time.Time {
	return limiter.lastRefill.Add(limiter.windowSize)
}

// calculateRetryAfter calculates when the client should retry
func (rl *RateLimiter) calculateRetryAfter(limiter *ClientLimiter) time.Duration {
	if limiter.blocked {
		return limiter.blockedUntil.Sub(time.Now())
	}
	
	// Calculate time needed to get at least one token
	secondsPerToken := 1.0 / float64(limiter.rateLimit)
	return time.Duration(secondsPerToken * float64(time.Second))
}

// generateHeaders generates rate limit response headers
func (rl *RateLimiter) generateHeaders(rateLimit, remaining int64, resetTime time.Time) map[string]string {
	if !rl.config.ReturnHeaders {
		return nil
	}
	
	headers := make(map[string]string)
	headers["X-RateLimit-Limit"] = fmt.Sprintf("%d", rateLimit)
	headers["X-RateLimit-Remaining"] = fmt.Sprintf("%d", remaining)
	headers["X-RateLimit-Reset"] = fmt.Sprintf("%d", resetTime.Unix())
	headers["X-RateLimit-Window"] = rl.config.WindowSize.String()
	
	return headers
}

// generateErrorHeaders generates headers for rate limit errors
func (rl *RateLimiter) generateErrorHeaders() map[string]string {
	if !rl.config.ReturnHeaders {
		return nil
	}
	
	headers := make(map[string]string)
	headers["X-RateLimit-Limit"] = "0"
	headers["X-RateLimit-Remaining"] = "0"
	headers["Retry-After"] = fmt.Sprintf("%d", int(rl.config.BlockDuration.Seconds()))
	
	return headers
}

// AddToBlacklist adds a client to the blacklist
func (rl *RateLimiter) AddToBlacklist(clientID, reason string, duration time.Duration, permanent bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	entry := &BlacklistEntry{
		ClientID:    clientID,
		Reason:      reason,
		BlockedAt:   time.Now(),
		IsPermanent: permanent,
		Metadata:    make(map[string]interface{}),
	}
	
	if !permanent {
		entry.ExpiresAt = time.Now().Add(duration)
	}
	
	rl.blacklist[clientID] = entry
	
	rl.globalStats.mu.Lock()
	rl.globalStats.BlacklistedClients++
	rl.globalStats.mu.Unlock()
	
	rl.logger.Printf("Added client %s to blacklist: %s", clientID, reason)
}

// AddToWhitelist adds a client to the whitelist
func (rl *RateLimiter) AddToWhitelist(clientID, reason string, duration *time.Duration, permanent bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	entry := &WhitelistEntry{
		ClientID:    clientID,
		Reason:      reason,
		AddedAt:     time.Now(),
		IsPermanent: permanent,
		Metadata:    make(map[string]interface{}),
	}
	
	if !permanent && duration != nil {
		expiresAt := time.Now().Add(*duration)
		entry.ExpiresAt = &expiresAt
	}
	
	rl.whitelist[clientID] = entry
	
	rl.globalStats.mu.Lock()
	rl.globalStats.WhitelistedClients++
	rl.globalStats.mu.Unlock()
	
	rl.logger.Printf("Added client %s to whitelist: %s", clientID, reason)
}

// cleanupRoutine performs periodic cleanup
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.cleanupStop:
			return
		}
	}
}

// cleanup removes idle limiters and expired entries
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	
	// Clean up idle client limiters
	for clientID, limiter := range rl.limiters {
		if now.Sub(limiter.stats.LastSeen) > rl.config.MaxIdleTime {
			delete(rl.limiters, clientID)
			
			rl.globalStats.mu.Lock()
			rl.globalStats.ActiveClients--
			rl.globalStats.mu.Unlock()
		}
	}
	
	// Clean up expired blacklist entries
	for clientID, entry := range rl.blacklist {
		if !entry.IsPermanent && now.After(entry.ExpiresAt) {
			delete(rl.blacklist, clientID)
			
			rl.globalStats.mu.Lock()
			rl.globalStats.BlacklistedClients--
			rl.globalStats.mu.Unlock()
		}
	}
	
	// Clean up expired whitelist entries
	for clientID, entry := range rl.whitelist {
		if !entry.IsPermanent && entry.ExpiresAt != nil && now.After(*entry.ExpiresAt) {
			delete(rl.whitelist, clientID)
			
			rl.globalStats.mu.Lock()
			rl.globalStats.WhitelistedClients--
			rl.globalStats.mu.Unlock()
		}
	}
}

// GetMetrics returns rate limiting metrics
func (rl *RateLimiter) GetMetrics() *RateLimitMetrics {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	metricsCopy := *rl.metrics
	return &metricsCopy
}

// GetGlobalStats returns global rate limiting statistics
func (rl *RateLimiter) GetGlobalStats() *GlobalRateStats {
	rl.globalStats.mu.RLock()
	defer rl.globalStats.mu.RUnlock()
	
	statsCopy := *rl.globalStats
	return &statsCopy
}

// GetClientStats returns statistics for a specific client
func (rl *RateLimiter) GetClientStats(clientID string) *ClientStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	if limiter, exists := rl.limiters[clientID]; exists {
		statsCopy := *limiter.stats
		return &statsCopy
	}
	
	return nil
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() error {
	close(rl.cleanupStop)
	rl.logger.Printf("Rate limiter stopped")
	return nil
}