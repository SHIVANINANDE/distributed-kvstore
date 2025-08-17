package security

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// RBACManager manages role-based access control
type RBACManager struct {
	mu          sync.RWMutex
	logger      *log.Logger
	config      RBACConfig
	roles       map[string]*Role
	permissions map[string]*Permission
	policies    map[string]*Policy
	resources   map[string]*Resource
	authManager *AuthManager
	auditLogger AuditLogger
	metrics     *RBACMetrics
}

// RBACConfig contains configuration for RBAC
type RBACConfig struct {
	// Policy configuration
	DefaultDenyAll     bool          `json:"default_deny_all"`     // Default to deny all access
	EnableInheritance  bool          `json:"enable_inheritance"`   // Enable role inheritance
	EnableDynamicRoles bool          `json:"enable_dynamic_roles"` // Enable dynamic role assignment
	
	// Cache configuration
	EnableCache        bool          `json:"enable_cache"`         // Enable permission caching
	CacheTTL           time.Duration `json:"cache_ttl"`            // Cache TTL
	CacheSize          int           `json:"cache_size"`           // Cache size limit
	
	// Validation configuration
	EnableValidation   bool          `json:"enable_validation"`    // Enable policy validation
	StrictMode         bool          `json:"strict_mode"`          // Strict validation mode
	
	// Temporal access
	EnableTemporal     bool          `json:"enable_temporal"`      // Enable temporal access control
	MaxGrantDuration   time.Duration `json:"max_grant_duration"`   // Maximum grant duration
	
	// Audit configuration
	EnableAudit        bool          `json:"enable_audit"`         // Enable access audit logging
	AuditAllAccess     bool          `json:"audit_all_access"`     // Audit all access attempts
	AuditDeniedOnly    bool          `json:"audit_denied_only"`    // Audit only denied access
}

// Role represents a security role
type Role struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Permissions   []string          `json:"permissions"`    // Permission IDs
	ParentRoles   []string          `json:"parent_roles"`   // Inherited roles
	IsSystem      bool              `json:"is_system"`      // System-defined role
	IsActive      bool              `json:"is_active"`      // Role is active
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	CreatedBy     string            `json:"created_by"`
	Metadata      map[string]interface{} `json:"metadata"`
	Constraints   *RoleConstraints  `json:"constraints"`    // Role constraints
}

// Permission represents a specific permission
type Permission struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Resource    string            `json:"resource"`       // Resource type
	Action      string            `json:"action"`         // Action type
	Effect      PermissionEffect  `json:"effect"`         // Allow or Deny
	Conditions  []Condition       `json:"conditions"`     // Access conditions
	IsSystem    bool              `json:"is_system"`      // System permission
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Policy represents an access control policy
type Policy struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        PolicyType        `json:"type"`           // RBAC, ABAC, etc.
	Rules       []PolicyRule      `json:"rules"`          // Policy rules
	Priority    int               `json:"priority"`       // Policy priority
	IsActive    bool              `json:"is_active"`      // Policy is active
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Resource represents a protected resource
type Resource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`           // Resource type
	Path        string            `json:"path"`           // Resource path
	Parent      string            `json:"parent"`         // Parent resource
	Children    []string          `json:"children"`       // Child resources
	Attributes  map[string]interface{} `json:"attributes"` // Resource attributes
	Metadata    map[string]interface{} `json:"metadata"`
}

// RoleConstraints defines constraints for role assignment
type RoleConstraints struct {
	TimeWindows     []TimeWindow      `json:"time_windows"`     // Valid time windows
	IPRestrictions  []string          `json:"ip_restrictions"`  // IP address restrictions
	LocationLimits  []string          `json:"location_limits"`  // Geographic restrictions
	MaxSessions     int               `json:"max_sessions"`     // Maximum concurrent sessions
	RequiresMFA     bool              `json:"requires_mfa"`     // Requires MFA
	ExpiresAt       *time.Time        `json:"expires_at"`       // Role expiration
	Metadata        map[string]interface{} `json:"metadata"`
}

// TimeWindow represents a valid time window for access
type TimeWindow struct {
	StartTime string `json:"start_time"` // Format: "15:04"
	EndTime   string `json:"end_time"`   // Format: "15:04"
	Days      []int  `json:"days"`       // Day of week (0=Sunday)
	TimeZone  string `json:"timezone"`   // Timezone
}

// Condition represents an access condition
type Condition struct {
	Type      ConditionType     `json:"type"`       // Condition type
	Attribute string            `json:"attribute"`  // Attribute to check
	Operator  ConditionOperator `json:"operator"`   // Comparison operator
	Value     interface{}       `json:"value"`      // Expected value
	Values    []interface{}     `json:"values"`     // Multiple values (for IN operator)
}

// PolicyRule represents a rule within a policy
type PolicyRule struct {
	ID          string            `json:"id"`
	Effect      PermissionEffect  `json:"effect"`     // Allow or Deny
	Subject     string            `json:"subject"`    // User, role, or group
	Resource    string            `json:"resource"`   // Resource pattern
	Action      string            `json:"action"`     // Action pattern
	Conditions  []Condition       `json:"conditions"` // Rule conditions
	Priority    int               `json:"priority"`   // Rule priority
	Metadata    map[string]interface{} `json:"metadata"`
}

// AccessRequest represents an access request
type AccessRequest struct {
	UserID      string            `json:"user_id"`
	Username    string            `json:"username"`
	Roles       []string          `json:"roles"`
	Resource    string            `json:"resource"`
	Action      string            `json:"action"`
	Context     *AccessContext    `json:"context"`
	RequestedAt time.Time         `json:"requested_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AccessContext provides context for access decisions
type AccessContext struct {
	IPAddress    string            `json:"ip_address"`
	UserAgent    string            `json:"user_agent"`
	SessionID    string            `json:"session_id"`
	RequestID    string            `json:"request_id"`
	Timestamp    time.Time         `json:"timestamp"`
	Location     *Location         `json:"location"`
	Device       *DeviceInfo       `json:"device"`
	Environment  string            `json:"environment"`  // dev, staging, prod
	Attributes   map[string]interface{} `json:"attributes"`
}

// Location represents geographical location
type Location struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// DeviceInfo represents device information
type DeviceInfo struct {
	Type         string `json:"type"`          // mobile, desktop, tablet
	OS           string `json:"os"`            // Operating system
	Browser      string `json:"browser"`       // Browser type
	Fingerprint  string `json:"fingerprint"`   // Device fingerprint
	IsTrusted    bool   `json:"is_trusted"`    // Is trusted device
}

// AccessResult represents the result of an access check
type AccessResult struct {
	Granted       bool              `json:"granted"`
	Effect        PermissionEffect  `json:"effect"`
	Reason        string            `json:"reason"`
	MatchedRules  []string          `json:"matched_rules"`
	FailedRules   []string          `json:"failed_rules"`
	Permissions   []string          `json:"permissions"`
	Context       *AccessContext    `json:"context"`
	ProcessedAt   time.Time         `json:"processed_at"`
	TTL           time.Duration     `json:"ttl"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// RBACMetrics tracks RBAC metrics
type RBACMetrics struct {
	TotalAccessChecks   int64             `json:"total_access_checks"`
	GrantedAccess       int64             `json:"granted_access"`
	DeniedAccess        int64             `json:"denied_access"`
	CacheHits           int64             `json:"cache_hits"`
	CacheMisses         int64             `json:"cache_misses"`
	PolicyEvaluations   int64             `json:"policy_evaluations"`
	RoleAssignments     int64             `json:"role_assignments"`
	PermissionGrants    int64             `json:"permission_grants"`
	AccessByResource    map[string]int64  `json:"access_by_resource"`
	AccessByAction      map[string]int64  `json:"access_by_action"`
	AccessByRole        map[string]int64  `json:"access_by_role"`
	DenialReasons       map[string]int64  `json:"denial_reasons"`
}

// Enums
type PermissionEffect string
const (
	EffectAllow PermissionEffect = "Allow"
	EffectDeny  PermissionEffect = "Deny"
)

type PolicyType string
const (
	PolicyTypeRBAC PolicyType = "RBAC"
	PolicyTypeABAC PolicyType = "ABAC"
	PolicyTypeACL  PolicyType = "ACL"
)

type ConditionType string
const (
	ConditionTime      ConditionType = "time"
	ConditionIP        ConditionType = "ip"
	ConditionLocation  ConditionType = "location"
	ConditionAttribute ConditionType = "attribute"
	ConditionResource  ConditionType = "resource"
)

type ConditionOperator string
const (
	OperatorEquals    ConditionOperator = "eq"
	OperatorNotEquals ConditionOperator = "ne"
	OperatorIn        ConditionOperator = "in"
	OperatorNotIn     ConditionOperator = "nin"
	OperatorGreater   ConditionOperator = "gt"
	OperatorLess      ConditionOperator = "lt"
	OperatorContains  ConditionOperator = "contains"
	OperatorMatches   ConditionOperator = "matches"
)

// AuditLogger interface for audit logging
type AuditLogger interface {
	LogAccess(request *AccessRequest, result *AccessResult) error
	LogRoleChange(userID, role, action string, metadata map[string]interface{}) error
	LogPermissionChange(permissionID, action string, metadata map[string]interface{}) error
	LogPolicyChange(policyID, action string, metadata map[string]interface{}) error
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager(config RBACConfig, authManager *AuthManager, auditLogger AuditLogger, logger *log.Logger) *RBACManager {
	if logger == nil {
		logger = log.New(log.Writer(), "[RBAC] ", log.LstdFlags)
	}
	
	// Set default values
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.CacheSize == 0 {
		config.CacheSize = 10000
	}
	if config.MaxGrantDuration == 0 {
		config.MaxGrantDuration = 24 * time.Hour
	}
	
	rbac := &RBACManager{
		logger:      logger,
		config:      config,
		roles:       make(map[string]*Role),
		permissions: make(map[string]*Permission),
		policies:    make(map[string]*Policy),
		resources:   make(map[string]*Resource),
		authManager: authManager,
		auditLogger: auditLogger,
		metrics: &RBACMetrics{
			AccessByResource: make(map[string]int64),
			AccessByAction:   make(map[string]int64),
			AccessByRole:     make(map[string]int64),
			DenialReasons:    make(map[string]int64),
		},
	}
	
	// Initialize default roles and permissions
	rbac.initializeDefaults()
	
	logger.Printf("RBAC manager initialized with %d roles, %d permissions", 
		len(rbac.roles), len(rbac.permissions))
	
	return rbac
}

// initializeDefaults creates default roles and permissions
func (rbac *RBACManager) initializeDefaults() {
	// Default permissions
	rbac.createDefaultPermissions()
	
	// Default roles
	rbac.createDefaultRoles()
	
	// Default policies
	rbac.createDefaultPolicies()
	
	// Default resources
	rbac.createDefaultResources()
}

// createDefaultPermissions creates system default permissions
func (rbac *RBACManager) createDefaultPermissions() {
	permissions := []*Permission{
		{
			ID:          "perm_read_all",
			Name:        "Read All",
			Description: "Read access to all resources",
			Resource:    "*",
			Action:      "read",
			Effect:      EffectAllow,
			IsSystem:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "perm_write_all",
			Name:        "Write All",
			Description: "Write access to all resources",
			Resource:    "*",
			Action:      "write",
			Effect:      EffectAllow,
			IsSystem:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "perm_delete_all",
			Name:        "Delete All",
			Description: "Delete access to all resources",
			Resource:    "*",
			Action:      "delete",
			Effect:      EffectAllow,
			IsSystem:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "perm_manage_users",
			Name:        "Manage Users",
			Description: "Manage user accounts",
			Resource:    "users",
			Action:      "*",
			Effect:      EffectAllow,
			IsSystem:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "perm_manage_system",
			Name:        "Manage System",
			Description: "System administration",
			Resource:    "system",
			Action:      "*",
			Effect:      EffectAllow,
			IsSystem:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "perm_read_own",
			Name:        "Read Own",
			Description: "Read own resources",
			Resource:    "user_data",
			Action:      "read",
			Effect:      EffectAllow,
			IsSystem:    true,
			CreatedAt:   time.Now(),
			Conditions: []Condition{
				{
					Type:      ConditionAttribute,
					Attribute: "owner",
					Operator:  OperatorEquals,
					Value:     "${user.id}",
				},
			},
		},
	}
	
	for _, perm := range permissions {
		rbac.permissions[perm.ID] = perm
	}
}

// createDefaultRoles creates system default roles
func (rbac *RBACManager) createDefaultRoles() {
	roles := []*Role{
		{
			ID:          "role_super_admin",
			Name:        "Super Administrator",
			Description: "Full system access",
			Permissions: []string{"perm_read_all", "perm_write_all", "perm_delete_all", "perm_manage_users", "perm_manage_system"},
			IsSystem:    true,
			IsActive:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "role_admin",
			Name:        "Administrator",
			Description: "Administrative access",
			Permissions: []string{"perm_read_all", "perm_write_all", "perm_manage_users"},
			IsSystem:    true,
			IsActive:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "role_user",
			Name:        "User",
			Description: "Standard user access",
			Permissions: []string{"perm_read_own"},
			IsSystem:    true,
			IsActive:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "role_reader",
			Name:        "Reader",
			Description: "Read-only access",
			Permissions: []string{"perm_read_own"},
			IsSystem:    true,
			IsActive:    true,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "role_guest",
			Name:        "Guest",
			Description: "Limited guest access",
			Permissions: []string{},
			IsSystem:    true,
			IsActive:    true,
			CreatedAt:   time.Now(),
		},
	}
	
	for _, role := range roles {
		rbac.roles[role.ID] = role
	}
}

// createDefaultPolicies creates default access policies
func (rbac *RBACManager) createDefaultPolicies() {
	policies := []*Policy{
		{
			ID:          "policy_default_deny",
			Name:        "Default Deny",
			Description: "Default deny all access",
			Type:        PolicyTypeRBAC,
			Priority:    1000,
			IsActive:    rbac.config.DefaultDenyAll,
			CreatedAt:   time.Now(),
			Rules: []PolicyRule{
				{
					ID:       "rule_deny_all",
					Effect:   EffectDeny,
					Subject:  "*",
					Resource: "*",
					Action:   "*",
					Priority: 1000,
				},
			},
		},
		{
			ID:          "policy_admin_access",
			Name:        "Administrator Access",
			Description: "Full access for administrators",
			Type:        PolicyTypeRBAC,
			Priority:    100,
			IsActive:    true,
			CreatedAt:   time.Now(),
			Rules: []PolicyRule{
				{
					ID:       "rule_admin_all",
					Effect:   EffectAllow,
					Subject:  "role:role_admin",
					Resource: "*",
					Action:   "*",
					Priority: 100,
				},
			},
		},
		{
			ID:          "policy_user_access",
			Name:        "User Access",
			Description: "Standard user access policy",
			Type:        PolicyTypeRBAC,
			Priority:    500,
			IsActive:    true,
			CreatedAt:   time.Now(),
			Rules: []PolicyRule{
				{
					ID:       "rule_user_read",
					Effect:   EffectAllow,
					Subject:  "role:role_user",
					Resource: "data/*",
					Action:   "read",
					Priority: 500,
					Conditions: []Condition{
						{
							Type:      ConditionAttribute,
							Attribute: "owner",
							Operator:  OperatorEquals,
							Value:     "${user.id}",
						},
					},
				},
			},
		},
	}
	
	for _, policy := range policies {
		rbac.policies[policy.ID] = policy
	}
}

// createDefaultResources creates default resource definitions
func (rbac *RBACManager) createDefaultResources() {
	resources := []*Resource{
		{
			ID:   "res_data",
			Name: "Data",
			Type: "kvstore_data",
			Path: "/data",
			Attributes: map[string]interface{}{
				"type": "data",
			},
		},
		{
			ID:   "res_users",
			Name: "Users",
			Type: "user_management",
			Path: "/users",
			Attributes: map[string]interface{}{
				"type": "users",
			},
		},
		{
			ID:   "res_system",
			Name: "System",
			Type: "system_admin",
			Path: "/system",
			Attributes: map[string]interface{}{
				"type": "system",
			},
		},
	}
	
	for _, resource := range resources {
		rbac.resources[resource.ID] = resource
	}
}

// CheckAccess checks if a user has access to perform an action on a resource
func (rbac *RBACManager) CheckAccess(ctx context.Context, request *AccessRequest) (*AccessResult, error) {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()
	
	rbac.metrics.TotalAccessChecks++
	
	// Set request timestamp
	if request.RequestedAt.IsZero() {
		request.RequestedAt = time.Now()
	}
	
	// Initialize result
	result := &AccessResult{
		Granted:     false,
		Effect:      EffectDeny,
		ProcessedAt: time.Now(),
		Context:     request.Context,
		Metadata:    make(map[string]interface{}),
	}
	
	// Track metrics
	rbac.metrics.AccessByResource[request.Resource]++
	rbac.metrics.AccessByAction[request.Action]++
	for _, role := range request.Roles {
		rbac.metrics.AccessByRole[role]++
	}
	
	// Check cache first if enabled
	if rbac.config.EnableCache {
		if cachedResult := rbac.getCachedResult(request); cachedResult != nil {
			rbac.metrics.CacheHits++
			return cachedResult, nil
		}
		rbac.metrics.CacheMisses++
	}
	
	// Evaluate policies
	granted, reason, matchedRules, failedRules := rbac.evaluatePolicies(request)
	
	result.Granted = granted
	result.Reason = reason
	result.MatchedRules = matchedRules
	result.FailedRules = failedRules
	
	if granted {
		result.Effect = EffectAllow
		result.Permissions = rbac.getUserPermissions(request.Roles)
		rbac.metrics.GrantedAccess++
	} else {
		result.Effect = EffectDeny
		rbac.metrics.DeniedAccess++
		rbac.metrics.DenialReasons[reason]++
	}
	
	// Cache result if enabled
	if rbac.config.EnableCache {
		rbac.cacheResult(request, result)
	}
	
	// Audit log if enabled
	if rbac.config.EnableAudit {
		if rbac.config.AuditAllAccess || (!granted && rbac.config.AuditDeniedOnly) || (!rbac.config.AuditDeniedOnly && granted) {
			if rbac.auditLogger != nil {
				rbac.auditLogger.LogAccess(request, result)
			}
		}
	}
	
	return result, nil
}

// evaluatePolicies evaluates all policies for an access request
func (rbac *RBACManager) evaluatePolicies(request *AccessRequest) (bool, string, []string, []string) {
	var matchedRules []string
	var failedRules []string
	
	// Get applicable policies sorted by priority
	policies := rbac.getApplicablePolicies(request)
	
	// Evaluate each policy
	for _, policy := range policies {
		rbac.metrics.PolicyEvaluations++
		
		granted, reason, matched, failed := rbac.evaluatePolicy(policy, request)
		matchedRules = append(matchedRules, matched...)
		failedRules = append(failedRules, failed...)
		
		// For explicit allow/deny, return immediately
		if granted {
			return true, "Access granted by policy: " + policy.Name, matchedRules, failedRules
		} else if len(matched) > 0 {
			// Explicit deny
			return false, "Access denied by policy: " + policy.Name + " - " + reason, matchedRules, failedRules
		}
	}
	
	// Default behavior
	if rbac.config.DefaultDenyAll {
		return false, "Default deny - no matching allow rules", matchedRules, failedRules
	}
	
	return true, "Default allow", matchedRules, failedRules
}

// evaluatePolicy evaluates a single policy
func (rbac *RBACManager) evaluatePolicy(policy *Policy, request *AccessRequest) (bool, string, []string, []string) {
	if !policy.IsActive {
		return false, "Policy inactive", nil, nil
	}
	
	var matchedRules []string
	var failedRules []string
	
	// Evaluate each rule in the policy
	for _, rule := range policy.Rules {
		if rbac.ruleMatches(&rule, request) {
			matchedRules = append(matchedRules, rule.ID)
			
			// Check rule conditions
			if rbac.evaluateConditions(rule.Conditions, request) {
				if rule.Effect == EffectAllow {
					return true, "Rule granted access", matchedRules, failedRules
				} else {
					return false, "Rule denied access", matchedRules, failedRules
				}
			} else {
				failedRules = append(failedRules, rule.ID)
			}
		}
	}
	
	return false, "No matching rules", matchedRules, failedRules
}

// ruleMatches checks if a rule matches the request
func (rbac *RBACManager) ruleMatches(rule *PolicyRule, request *AccessRequest) bool {
	// Check subject (user or role)
	if !rbac.subjectMatches(rule.Subject, request) {
		return false
	}
	
	// Check resource
	if !rbac.resourceMatches(rule.Resource, request.Resource) {
		return false
	}
	
	// Check action
	if !rbac.actionMatches(rule.Action, request.Action) {
		return false
	}
	
	return true
}

// subjectMatches checks if the subject matches the request
func (rbac *RBACManager) subjectMatches(subject string, request *AccessRequest) bool {
	if subject == "*" {
		return true
	}
	
	// Check role-based subjects
	if strings.HasPrefix(subject, "role:") {
		roleID := strings.TrimPrefix(subject, "role:")
		for _, userRole := range request.Roles {
			if userRole == roleID {
				return true
			}
			// Check role inheritance
			if rbac.config.EnableInheritance && rbac.hasInheritedRole(userRole, roleID) {
				return true
			}
		}
		return false
	}
	
	// Check user-based subjects
	if strings.HasPrefix(subject, "user:") {
		userID := strings.TrimPrefix(subject, "user:")
		return userID == request.UserID
	}
	
	// Direct match
	return subject == request.Username || subject == request.UserID
}

// resourceMatches checks if the resource pattern matches
func (rbac *RBACManager) resourceMatches(pattern, resource string) bool {
	if pattern == "*" {
		return true
	}
	
	// Simple wildcard matching
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(resource, prefix)
	}
	
	return pattern == resource
}

// actionMatches checks if the action pattern matches
func (rbac *RBACManager) actionMatches(pattern, action string) bool {
	if pattern == "*" {
		return true
	}
	
	return pattern == action
}

// evaluateConditions evaluates rule conditions
func (rbac *RBACManager) evaluateConditions(conditions []Condition, request *AccessRequest) bool {
	for _, condition := range conditions {
		if !rbac.evaluateCondition(&condition, request) {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition
func (rbac *RBACManager) evaluateCondition(condition *Condition, request *AccessRequest) bool {
	switch condition.Type {
	case ConditionTime:
		return rbac.evaluateTimeCondition(condition, request)
	case ConditionIP:
		return rbac.evaluateIPCondition(condition, request)
	case ConditionLocation:
		return rbac.evaluateLocationCondition(condition, request)
	case ConditionAttribute:
		return rbac.evaluateAttributeCondition(condition, request)
	case ConditionResource:
		return rbac.evaluateResourceCondition(condition, request)
	default:
		return false
	}
}

// evaluateTimeCondition evaluates time-based conditions
func (rbac *RBACManager) evaluateTimeCondition(condition *Condition, request *AccessRequest) bool {
	now := request.RequestedAt
	if now.IsZero() {
		now = time.Now()
	}
	
	// Simple time range check
	switch condition.Operator {
	case OperatorGreater:
		if timeValue, ok := condition.Value.(string); ok {
			if condTime, err := time.Parse("15:04", timeValue); err == nil {
				currentTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)
				condTimeToday := time.Date(0, 1, 1, condTime.Hour(), condTime.Minute(), 0, 0, time.UTC)
				return currentTime.After(condTimeToday)
			}
		}
	case OperatorLess:
		if timeValue, ok := condition.Value.(string); ok {
			if condTime, err := time.Parse("15:04", timeValue); err == nil {
				currentTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)
				condTimeToday := time.Date(0, 1, 1, condTime.Hour(), condTime.Minute(), 0, 0, time.UTC)
				return currentTime.Before(condTimeToday)
			}
		}
	}
	
	return false
}

// evaluateIPCondition evaluates IP-based conditions
func (rbac *RBACManager) evaluateIPCondition(condition *Condition, request *AccessRequest) bool {
	if request.Context == nil {
		return false
	}
	
	userIP := request.Context.IPAddress
	
	switch condition.Operator {
	case OperatorEquals:
		return userIP == condition.Value.(string)
	case OperatorIn:
		for _, ip := range condition.Values {
			if userIP == ip.(string) {
				return true
			}
		}
		return false
	case OperatorContains:
		return strings.Contains(userIP, condition.Value.(string))
	}
	
	return false
}

// evaluateLocationCondition evaluates location-based conditions
func (rbac *RBACManager) evaluateLocationCondition(condition *Condition, request *AccessRequest) bool {
	if request.Context == nil || request.Context.Location == nil {
		return false
	}
	
	location := request.Context.Location
	
	switch condition.Attribute {
	case "country":
		return rbac.compareValue(location.Country, condition.Operator, condition.Value, condition.Values)
	case "region":
		return rbac.compareValue(location.Region, condition.Operator, condition.Value, condition.Values)
	case "city":
		return rbac.compareValue(location.City, condition.Operator, condition.Value, condition.Values)
	}
	
	return false
}

// evaluateAttributeCondition evaluates attribute-based conditions
func (rbac *RBACManager) evaluateAttributeCondition(condition *Condition, request *AccessRequest) bool {
	// Get attribute value
	var attributeValue interface{}
	
	// Check context attributes
	if request.Context != nil && request.Context.Attributes != nil {
		if val, exists := request.Context.Attributes[condition.Attribute]; exists {
			attributeValue = val
		}
	}
	
	// Check request metadata
	if attributeValue == nil && request.Metadata != nil {
		if val, exists := request.Metadata[condition.Attribute]; exists {
			attributeValue = val
		}
	}
	
	// Handle variable substitution
	expectedValue := condition.Value
	if strVal, ok := expectedValue.(string); ok {
		if strVal == "${user.id}" {
			expectedValue = request.UserID
		} else if strVal == "${user.username}" {
			expectedValue = request.Username
		}
	}
	
	return rbac.compareValue(attributeValue, condition.Operator, expectedValue, condition.Values)
}

// evaluateResourceCondition evaluates resource-based conditions
func (rbac *RBACManager) evaluateResourceCondition(condition *Condition, request *AccessRequest) bool {
	resource := rbac.resources[request.Resource]
	if resource == nil {
		return false
	}
	
	// Check resource attributes
	if resource.Attributes != nil {
		if val, exists := resource.Attributes[condition.Attribute]; exists {
			return rbac.compareValue(val, condition.Operator, condition.Value, condition.Values)
		}
	}
	
	return false
}

// compareValue compares values based on operator
func (rbac *RBACManager) compareValue(actual interface{}, operator ConditionOperator, expected interface{}, expectedList []interface{}) bool {
	switch operator {
	case OperatorEquals:
		return actual == expected
	case OperatorNotEquals:
		return actual != expected
	case OperatorIn:
		for _, val := range expectedList {
			if actual == val {
				return true
			}
		}
		return false
	case OperatorNotIn:
		for _, val := range expectedList {
			if actual == val {
				return false
			}
		}
		return true
	case OperatorContains:
		if actualStr, ok := actual.(string); ok {
			if expectedStr, ok := expected.(string); ok {
				return strings.Contains(actualStr, expectedStr)
			}
		}
		return false
	case OperatorMatches:
		// Simple pattern matching - in production, use regex
		if actualStr, ok := actual.(string); ok {
			if expectedStr, ok := expected.(string); ok {
				return actualStr == expectedStr
			}
		}
		return false
	}
	
	return false
}

// getApplicablePolicies returns policies applicable to the request
func (rbac *RBACManager) getApplicablePolicies(request *AccessRequest) []*Policy {
	var policies []*Policy
	
	for _, policy := range rbac.policies {
		if policy.IsActive {
			policies = append(policies, policy)
		}
	}
	
	// Sort by priority (lower number = higher priority)
	for i := 0; i < len(policies)-1; i++ {
		for j := i + 1; j < len(policies); j++ {
			if policies[i].Priority > policies[j].Priority {
				policies[i], policies[j] = policies[j], policies[i]
			}
		}
	}
	
	return policies
}

// getUserPermissions gets permissions for user roles
func (rbac *RBACManager) getUserPermissions(userRoles []string) []string {
	var permissions []string
	seen := make(map[string]bool)
	
	for _, roleID := range userRoles {
		if role, exists := rbac.roles[roleID]; exists && role.IsActive {
			for _, permID := range role.Permissions {
				if !seen[permID] {
					permissions = append(permissions, permID)
					seen[permID] = true
				}
			}
			
			// Check inherited roles
			if rbac.config.EnableInheritance {
				for _, parentRoleID := range role.ParentRoles {
					parentPerms := rbac.getUserPermissions([]string{parentRoleID})
					for _, perm := range parentPerms {
						if !seen[perm] {
							permissions = append(permissions, perm)
							seen[perm] = true
						}
					}
				}
			}
		}
	}
	
	return permissions
}

// hasInheritedRole checks if a role inherits from another role
func (rbac *RBACManager) hasInheritedRole(userRole, targetRole string) bool {
	if role, exists := rbac.roles[userRole]; exists {
		for _, parentRole := range role.ParentRoles {
			if parentRole == targetRole {
				return true
			}
			// Recursive check
			if rbac.hasInheritedRole(parentRole, targetRole) {
				return true
			}
		}
	}
	return false
}

// Role Management Methods

// CreateRole creates a new role
func (rbac *RBACManager) CreateRole(role *Role) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()
	
	if role.ID == "" {
		return fmt.Errorf("role ID is required")
	}
	
	if _, exists := rbac.roles[role.ID]; exists {
		return fmt.Errorf("role already exists")
	}
	
	// Validate permissions
	for _, permID := range role.Permissions {
		if _, exists := rbac.permissions[permID]; !exists {
			return fmt.Errorf("permission not found: %s", permID)
		}
	}
	
	// Set timestamps
	now := time.Now()
	role.CreatedAt = now
	role.UpdatedAt = now
	
	rbac.roles[role.ID] = role
	rbac.metrics.RoleAssignments++
	
	// Audit log
	if rbac.auditLogger != nil {
		rbac.auditLogger.LogRoleChange("", role.ID, "create", map[string]interface{}{
			"role_name": role.Name,
			"permissions": role.Permissions,
		})
	}
	
	rbac.logger.Printf("Created role: %s", role.Name)
	return nil
}

// AssignRole assigns a role to a user
func (rbac *RBACManager) AssignRole(userID, roleID string) error {
	// This would typically update the user's roles in the user store
	// For now, we'll just log the assignment
	
	if rbac.auditLogger != nil {
		rbac.auditLogger.LogRoleChange(userID, roleID, "assign", map[string]interface{}{
			"user_id": userID,
			"role_id": roleID,
		})
	}
	
	rbac.logger.Printf("Assigned role %s to user %s", roleID, userID)
	return nil
}

// CreatePermission creates a new permission
func (rbac *RBACManager) CreatePermission(permission *Permission) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()
	
	if permission.ID == "" {
		return fmt.Errorf("permission ID is required")
	}
	
	if _, exists := rbac.permissions[permission.ID]; exists {
		return fmt.Errorf("permission already exists")
	}
	
	// Set timestamps
	now := time.Now()
	permission.CreatedAt = now
	permission.UpdatedAt = now
	
	rbac.permissions[permission.ID] = permission
	rbac.metrics.PermissionGrants++
	
	// Audit log
	if rbac.auditLogger != nil {
		rbac.auditLogger.LogPermissionChange(permission.ID, "create", map[string]interface{}{
			"permission_name": permission.Name,
			"resource": permission.Resource,
			"action": permission.Action,
		})
	}
	
	rbac.logger.Printf("Created permission: %s", permission.Name)
	return nil
}

// Cache methods (simplified implementations)

// getCachedResult retrieves a cached access result
func (rbac *RBACManager) getCachedResult(request *AccessRequest) *AccessResult {
	// Simplified cache implementation
	// In production, use proper cache with TTL
	return nil
}

// cacheResult caches an access result
func (rbac *RBACManager) cacheResult(request *AccessRequest, result *AccessResult) {
	// Simplified cache implementation
	// In production, implement proper caching with TTL
}

// GetMetrics returns RBAC metrics
func (rbac *RBACManager) GetMetrics() *RBACMetrics {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()
	
	metricsCopy := *rbac.metrics
	return &metricsCopy
}

// GetRoles returns all roles
func (rbac *RBACManager) GetRoles() map[string]*Role {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()
	
	roles := make(map[string]*Role)
	for id, role := range rbac.roles {
		roleCopy := *role
		roles[id] = &roleCopy
	}
	
	return roles
}

// GetPermissions returns all permissions
func (rbac *RBACManager) GetPermissions() map[string]*Permission {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()
	
	permissions := make(map[string]*Permission)
	for id, perm := range rbac.permissions {
		permCopy := *perm
		permissions[id] = &permCopy
	}
	
	return permissions
}