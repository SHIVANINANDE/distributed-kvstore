package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// AuthManager manages JWT-based authentication and authorization
type AuthManager struct {
	mu             sync.RWMutex
	logger         *log.Logger
	config         AuthConfig
	privateKey     *rsa.PrivateKey
	publicKey      *rsa.PublicKey
	tokenStore     TokenStore
	refreshTokens  map[string]*RefreshToken
	blacklistedTokens map[string]time.Time
	userStore      UserStore
	sessionStore   SessionStore
	metrics        *AuthMetrics
}

// AuthConfig contains configuration for authentication
type AuthConfig struct {
	// JWT configuration
	Issuer           string        `json:"issuer"`             // JWT issuer
	TokenTTL         time.Duration `json:"token_ttl"`          // Access token TTL
	RefreshTokenTTL  time.Duration `json:"refresh_token_ttl"`  // Refresh token TTL
	SigningAlgorithm string        `json:"signing_algorithm"`  // Signing algorithm (RS256, HS256)
	
	// Key configuration
	PrivateKeyPath   string        `json:"private_key_path"`   // Path to private key
	PublicKeyPath    string        `json:"public_key_path"`    // Path to public key
	KeySize          int           `json:"key_size"`           // RSA key size
	
	// Security settings
	RequireHTTPS     bool          `json:"require_https"`      // Require HTTPS for tokens
	AllowRefresh     bool          `json:"allow_refresh"`      // Allow token refresh
	MaxLoginAttempts int           `json:"max_login_attempts"` // Max failed login attempts
	LockoutDuration  time.Duration `json:"lockout_duration"`   // Account lockout duration
	
	// Session configuration
	SessionTimeout   time.Duration `json:"session_timeout"`    // Session timeout
	MaxSessions      int           `json:"max_sessions"`       // Max concurrent sessions per user
	EnableMFA        bool          `json:"enable_mfa"`         // Enable multi-factor authentication
	
	// Password policy
	PasswordPolicy   *PasswordPolicy `json:"password_policy"`  // Password requirements
	
	// Token blacklist cleanup
	CleanupInterval  time.Duration `json:"cleanup_interval"`   // Blacklist cleanup interval
}

// PasswordPolicy defines password requirements
type PasswordPolicy struct {
	MinLength        int  `json:"min_length"`        // Minimum password length
	RequireUppercase bool `json:"require_uppercase"` // Require uppercase letters
	RequireLowercase bool `json:"require_lowercase"` // Require lowercase letters
	RequireNumbers   bool `json:"require_numbers"`   // Require numbers
	RequireSymbols   bool `json:"require_symbols"`   // Require symbols
	MaxAge           time.Duration `json:"max_age"`   // Maximum password age
	HistorySize      int  `json:"history_size"`      // Password history size
}

// JWTClaims represents JWT claims
type JWTClaims struct {
	UserID       string            `json:"sub"`        // Subject (user ID)
	Username     string            `json:"username"`   // Username
	Email        string            `json:"email"`      // Email address
	Roles        []string          `json:"roles"`      // User roles
	Permissions  []string          `json:"permissions"` // User permissions
	SessionID    string            `json:"session_id"` // Session ID
	Issuer       string            `json:"iss"`        // Issuer
	Audience     string            `json:"aud"`        // Audience
	ExpiresAt    int64             `json:"exp"`        // Expiration time
	IssuedAt     int64             `json:"iat"`        // Issued at
	NotBefore    int64             `json:"nbf"`        // Not before
	TokenType    string            `json:"token_type"` // Token type (access/refresh)
	Metadata     map[string]interface{} `json:"metadata"` // Additional metadata
}

// User represents a user in the system
type User struct {
	ID              string            `json:"id"`
	Username        string            `json:"username"`
	Email           string            `json:"email"`
	PasswordHash    string            `json:"password_hash"`
	Salt            string            `json:"salt"`
	Roles           []string          `json:"roles"`
	IsActive        bool              `json:"is_active"`
	IsLocked        bool              `json:"is_locked"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	LastLogin       time.Time         `json:"last_login"`
	FailedAttempts  int               `json:"failed_attempts"`
	LockedUntil     time.Time         `json:"locked_until"`
	PasswordChanged time.Time         `json:"password_changed"`
	MFAEnabled      bool              `json:"mfa_enabled"`
	MFASecret       string            `json:"mfa_secret"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// Session represents a user session
type Session struct {
	ID          string            `json:"id"`
	UserID      string            `json:"user_id"`
	TokenID     string            `json:"token_id"`
	IPAddress   string            `json:"ip_address"`
	UserAgent   string            `json:"user_agent"`
	CreatedAt   time.Time         `json:"created_at"`
	LastAccess  time.Time         `json:"last_access"`
	ExpiresAt   time.Time         `json:"expires_at"`
	IsActive    bool              `json:"is_active"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// RefreshToken represents a refresh token
type RefreshToken struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IssuedAt  time.Time `json:"issued_at"`
	Used      bool      `json:"used"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	MFACode    string `json:"mfa_code,omitempty"`
	RememberMe bool   `json:"remember_me"`
	IPAddress  string `json:"ip_address"`
	UserAgent  string `json:"user_agent"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	User         *UserInfo `json:"user"`
	SessionID    string    `json:"session_id"`
}

// UserInfo represents safe user information for responses
type UserInfo struct {
	ID        string            `json:"id"`
	Username  string            `json:"username"`
	Email     string            `json:"email"`
	Roles     []string          `json:"roles"`
	LastLogin time.Time         `json:"last_login"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TokenStore interface for token storage
type TokenStore interface {
	Store(tokenID string, token *JWTClaims, ttl time.Duration) error
	Retrieve(tokenID string) (*JWTClaims, error)
	Delete(tokenID string) error
	BlacklistToken(tokenID string, expiry time.Time) error
	IsBlacklisted(tokenID string) bool
	Cleanup() error
}

// UserStore interface for user storage
type UserStore interface {
	GetUser(username string) (*User, error)
	GetUserByID(userID string) (*User, error)
	CreateUser(user *User) error
	UpdateUser(user *User) error
	DeleteUser(userID string) error
	UpdateLastLogin(userID string, loginTime time.Time) error
	IncrementFailedAttempts(userID string) error
	ResetFailedAttempts(userID string) error
	LockUser(userID string, until time.Time) error
	UnlockUser(userID string) error
}

// SessionStore interface for session storage
type SessionStore interface {
	CreateSession(session *Session) error
	GetSession(sessionID string) (*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(sessionID string) error
	GetUserSessions(userID string) ([]*Session, error)
	CleanupExpiredSessions() error
}

// AuthMetrics tracks authentication metrics
type AuthMetrics struct {
	TotalLogins         int64             `json:"total_logins"`
	SuccessfulLogins    int64             `json:"successful_logins"`
	FailedLogins        int64             `json:"failed_logins"`
	TokensIssued        int64             `json:"tokens_issued"`
	TokensRefreshed     int64             `json:"tokens_refreshed"`
	TokensBlacklisted   int64             `json:"tokens_blacklisted"`
	ActiveSessions      int64             `json:"active_sessions"`
	LockedAccounts      int64             `json:"locked_accounts"`
	MFAVerifications    int64             `json:"mfa_verifications"`
	LoginsByHour        map[string]int64  `json:"logins_by_hour"`
	FailureReasons      map[string]int64  `json:"failure_reasons"`
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config AuthConfig, tokenStore TokenStore, userStore UserStore, sessionStore SessionStore, logger *log.Logger) (*AuthManager, error) {
	if logger == nil {
		logger = log.New(log.Writer(), "[AUTH] ", log.LstdFlags)
	}
	
	// Set default values
	if config.TokenTTL == 0 {
		config.TokenTTL = 15 * time.Minute
	}
	if config.RefreshTokenTTL == 0 {
		config.RefreshTokenTTL = 7 * 24 * time.Hour // 7 days
	}
	if config.SigningAlgorithm == "" {
		config.SigningAlgorithm = "RS256"
	}
	if config.KeySize == 0 {
		config.KeySize = 2048
	}
	if config.MaxLoginAttempts == 0 {
		config.MaxLoginAttempts = 5
	}
	if config.LockoutDuration == 0 {
		config.LockoutDuration = 30 * time.Minute
	}
	if config.SessionTimeout == 0 {
		config.SessionTimeout = 24 * time.Hour
	}
	if config.MaxSessions == 0 {
		config.MaxSessions = 5
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = time.Hour
	}
	
	// Set default password policy
	if config.PasswordPolicy == nil {
		config.PasswordPolicy = &PasswordPolicy{
			MinLength:        8,
			RequireUppercase: true,
			RequireLowercase: true,
			RequireNumbers:   true,
			RequireSymbols:   true,
			MaxAge:           90 * 24 * time.Hour, // 90 days
			HistorySize:      5,
		}
	}
	
	am := &AuthManager{
		logger:            logger,
		config:            config,
		tokenStore:        tokenStore,
		refreshTokens:     make(map[string]*RefreshToken),
		blacklistedTokens: make(map[string]time.Time),
		userStore:         userStore,
		sessionStore:      sessionStore,
		metrics: &AuthMetrics{
			LoginsByHour:   make(map[string]int64),
			FailureReasons: make(map[string]int64),
		},
	}
	
	// Load or generate RSA keys
	if err := am.loadOrGenerateKeys(); err != nil {
		return nil, fmt.Errorf("failed to load or generate keys: %w", err)
	}
	
	// Start cleanup routine
	go am.cleanupRoutine()
	
	logger.Printf("Authentication manager initialized with %s algorithm", config.SigningAlgorithm)
	return am, nil
}

// loadOrGenerateKeys loads existing keys or generates new ones
func (am *AuthManager) loadOrGenerateKeys() error {
	// Try to load existing keys
	if am.config.PrivateKeyPath != "" && am.config.PublicKeyPath != "" {
		if err := am.loadKeys(); err == nil {
			return nil
		}
		am.logger.Printf("Failed to load existing keys, generating new ones")
	}
	
	// Generate new keys
	return am.generateKeys()
}

// loadKeys loads RSA keys from files
func (am *AuthManager) loadKeys() error {
	// Load private key
	privateKeyData, err := readKeyFile(am.config.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	
	privateKeyBlock, _ := pem.Decode(privateKeyData)
	if privateKeyBlock == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}
	
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	
	// Load public key
	publicKeyData, err := readKeyFile(am.config.PublicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}
	
	publicKeyBlock, _ := pem.Decode(publicKeyData)
	if publicKeyBlock == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}
	
	publicKeyInterface, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}
	
	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}
	
	am.privateKey = privateKey
	am.publicKey = publicKey
	
	return nil
}

// generateKeys generates new RSA key pair
func (am *AuthManager) generateKeys() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, am.config.KeySize)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}
	
	am.privateKey = privateKey
	am.publicKey = &privateKey.PublicKey
	
	// Save keys if paths are configured
	if am.config.PrivateKeyPath != "" {
		if err := am.savePrivateKey(); err != nil {
			am.logger.Printf("Failed to save private key: %v", err)
		}
	}
	
	if am.config.PublicKeyPath != "" {
		if err := am.savePublicKey(); err != nil {
			am.logger.Printf("Failed to save public key: %v", err)
		}
	}
	
	return nil
}

// savePrivateKey saves the private key to file
func (am *AuthManager) savePrivateKey() error {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(am.privateKey)
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	
	return writeKeyFile(am.config.PrivateKeyPath, pem.EncodeToMemory(privateKeyPEM))
}

// savePublicKey saves the public key to file
func (am *AuthManager) savePublicKey() error {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(am.publicKey)
	if err != nil {
		return err
	}
	
	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	
	return writeKeyFile(am.config.PublicKeyPath, pem.EncodeToMemory(publicKeyPEM))
}

// Login authenticates a user and returns tokens
func (am *AuthManager) Login(req *LoginRequest) (*LoginResponse, error) {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	am.metrics.TotalLogins++
	
	// Get user
	user, err := am.userStore.GetUser(req.Username)
	if err != nil {
		am.metrics.FailedLogins++
		am.metrics.FailureReasons["user_not_found"]++
		return nil, fmt.Errorf("invalid credentials")
	}
	
	// Check if user is active
	if !user.IsActive {
		am.metrics.FailedLogins++
		am.metrics.FailureReasons["user_inactive"]++
		return nil, fmt.Errorf("account is inactive")
	}
	
	// Check if user is locked
	if user.IsLocked && time.Now().Before(user.LockedUntil) {
		am.metrics.FailedLogins++
		am.metrics.FailureReasons["user_locked"]++
		return nil, fmt.Errorf("account is locked until %v", user.LockedUntil)
	}
	
	// Verify password
	if !am.verifyPassword(req.Password, user.PasswordHash, user.Salt) {
		// Increment failed attempts
		am.userStore.IncrementFailedAttempts(user.ID)
		
		// Lock user if too many failed attempts
		if user.FailedAttempts+1 >= am.config.MaxLoginAttempts {
			lockUntil := time.Now().Add(am.config.LockoutDuration)
			am.userStore.LockUser(user.ID, lockUntil)
			am.metrics.LockedAccounts++
		}
		
		am.metrics.FailedLogins++
		am.metrics.FailureReasons["invalid_password"]++
		return nil, fmt.Errorf("invalid credentials")
	}
	
	// Verify MFA if enabled
	if user.MFAEnabled && req.MFACode != "" {
		if !am.verifyMFA(user.MFASecret, req.MFACode) {
			am.metrics.FailedLogins++
			am.metrics.FailureReasons["invalid_mfa"]++
			return nil, fmt.Errorf("invalid MFA code")
		}
		am.metrics.MFAVerifications++
	} else if user.MFAEnabled {
		am.metrics.FailedLogins++
		am.metrics.FailureReasons["mfa_required"]++
		return nil, fmt.Errorf("MFA code required")
	}
	
	// Reset failed attempts and unlock user
	am.userStore.ResetFailedAttempts(user.ID)
	if user.IsLocked {
		am.userStore.UnlockUser(user.ID)
	}
	
	// Create session
	session := &Session{
		ID:         generateSessionID(),
		UserID:     user.ID,
		IPAddress:  req.IPAddress,
		UserAgent:  req.UserAgent,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		ExpiresAt:  time.Now().Add(am.config.SessionTimeout),
		IsActive:   true,
		Metadata:   make(map[string]interface{}),
	}
	
	// Check max sessions per user
	if err := am.enforceMaxSessions(user.ID); err != nil {
		am.logger.Printf("Failed to enforce max sessions: %v", err)
	}
	
	if err := am.sessionStore.CreateSession(session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	
	// Generate access token
	accessToken, err := am.generateToken(user, session.ID, "access")
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	
	// Generate refresh token if enabled
	var refreshToken string
	if am.config.AllowRefresh {
		refreshToken, err = am.generateRefreshToken(user.ID, session.ID)
		if err != nil {
			am.logger.Printf("Failed to generate refresh token: %v", err)
		}
	}
	
	// Update last login
	am.userStore.UpdateLastLogin(user.ID, time.Now())
	
	am.metrics.SuccessfulLogins++
	am.metrics.TokensIssued++
	am.metrics.ActiveSessions++
	
	// Track logins by hour
	hourKey := time.Now().Format("2006-01-02-15")
	am.metrics.LoginsByHour[hourKey]++
	
	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(am.config.TokenTTL.Seconds()),
		User: &UserInfo{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Roles:     user.Roles,
			LastLogin: user.LastLogin,
			Metadata:  user.Metadata,
		},
		SessionID: session.ID,
	}, nil
}

// generateToken generates a JWT token
func (am *AuthManager) generateToken(user *User, sessionID, tokenType string) (string, error) {
	now := time.Now()
	claims := &JWTClaims{
		UserID:      user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Roles:       user.Roles,
		Permissions: am.getUserPermissions(user),
		SessionID:   sessionID,
		Issuer:      am.config.Issuer,
		Audience:    "kvstore-api",
		ExpiresAt:   now.Add(am.config.TokenTTL).Unix(),
		IssuedAt:    now.Unix(),
		NotBefore:   now.Unix(),
		TokenType:   tokenType,
		Metadata:    make(map[string]interface{}),
	}
	
	// Create JWT header
	header := map[string]interface{}{
		"alg": am.config.SigningAlgorithm,
		"typ": "JWT",
	}
	
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	
	// Base64 encode header and claims
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	
	// Create signing input
	signingInput := headerB64 + "." + claimsB64
	
	// Sign the token
	signature, err := am.signToken(signingInput)
	if err != nil {
		return "", err
	}
	
	// Create final token
	token := signingInput + "." + signature
	
	// Store token in token store
	tokenID := am.generateTokenID(claims)
	if err := am.tokenStore.Store(tokenID, claims, am.config.TokenTTL); err != nil {
		am.logger.Printf("Failed to store token: %v", err)
	}
	
	return token, nil
}

// signToken signs the token using the private key
func (am *AuthManager) signToken(signingInput string) (string, error) {
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, am.privateKey, sha256.New().Sum(nil)[:0], hash[:])
	if err != nil {
		return "", err
	}
	
	return base64.RawURLEncoding.EncodeToString(signature), nil
}

// VerifyToken verifies and parses a JWT token
func (am *AuthManager) VerifyToken(tokenString string) (*JWTClaims, error) {
	// Split token into parts
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}
	
	headerB64, claimsB64, signatureB64 := parts[0], parts[1], parts[2]
	
	// Verify signature
	signingInput := headerB64 + "." + claimsB64
	if err := am.verifySignature(signingInput, signatureB64); err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}
	
	// Decode claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode claims: %w", err)
	}
	
	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal claims: %w", err)
	}
	
	// Verify token expiry
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}
	
	// Verify not before
	if time.Now().Unix() < claims.NotBefore {
		return nil, fmt.Errorf("token not yet valid")
	}
	
	// Check if token is blacklisted
	tokenID := am.generateTokenID(&claims)
	if am.tokenStore.IsBlacklisted(tokenID) {
		return nil, fmt.Errorf("token is blacklisted")
	}
	
	return &claims, nil
}

// verifySignature verifies the token signature
func (am *AuthManager) verifySignature(signingInput, signatureB64 string) error {
	signature, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return err
	}
	
	hash := sha256.Sum256([]byte(signingInput))
	return rsa.VerifyPKCS1v15(am.publicKey, sha256.New().Sum(nil)[:0], hash[:], signature)
}

// RefreshToken refreshes an access token using a refresh token
func (am *AuthManager) RefreshToken(refreshTokenString string) (*LoginResponse, error) {
	if !am.config.AllowRefresh {
		return nil, fmt.Errorf("token refresh not allowed")
	}
	
	am.mu.Lock()
	defer am.mu.Unlock()
	
	// Verify refresh token
	refreshToken, exists := am.refreshTokens[refreshTokenString]
	if !exists {
		return nil, fmt.Errorf("invalid refresh token")
	}
	
	// Check if refresh token is expired
	if time.Now().After(refreshToken.ExpiresAt) {
		delete(am.refreshTokens, refreshTokenString)
		return nil, fmt.Errorf("refresh token expired")
	}
	
	// Check if refresh token is already used
	if refreshToken.Used {
		delete(am.refreshTokens, refreshTokenString)
		return nil, fmt.Errorf("refresh token already used")
	}
	
	// Get user
	user, err := am.userStore.GetUserByID(refreshToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	
	// Mark refresh token as used
	refreshToken.Used = true
	
	// Generate new access token
	accessToken, err := am.generateToken(user, refreshToken.SessionID, "access")
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	
	// Generate new refresh token
	newRefreshToken, err := am.generateRefreshToken(user.ID, refreshToken.SessionID)
	if err != nil {
		am.logger.Printf("Failed to generate new refresh token: %v", err)
	}
	
	am.metrics.TokensRefreshed++
	
	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(am.config.TokenTTL.Seconds()),
		User: &UserInfo{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Roles:     user.Roles,
			LastLogin: user.LastLogin,
			Metadata:  user.Metadata,
		},
		SessionID: refreshToken.SessionID,
	}, nil
}

// Logout logs out a user and invalidates tokens
func (am *AuthManager) Logout(tokenString, sessionID string) error {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	// Parse token to get claims
	claims, err := am.VerifyToken(tokenString)
	if err != nil {
		// Even if token is invalid, try to logout session
		am.logger.Printf("Invalid token during logout: %v", err)
	}
	
	// Blacklist the token
	if claims != nil {
		tokenID := am.generateTokenID(claims)
		expiry := time.Unix(claims.ExpiresAt, 0)
		am.tokenStore.BlacklistToken(tokenID, expiry)
		am.metrics.TokensBlacklisted++
	}
	
	// Invalidate session
	if sessionID != "" {
		session, err := am.sessionStore.GetSession(sessionID)
		if err == nil {
			session.IsActive = false
			am.sessionStore.UpdateSession(session)
			am.metrics.ActiveSessions--
		}
	}
	
	// Remove any refresh tokens for this session
	for token, refreshToken := range am.refreshTokens {
		if refreshToken.SessionID == sessionID {
			delete(am.refreshTokens, token)
		}
	}
	
	return nil
}

// generateRefreshToken generates a refresh token
func (am *AuthManager) generateRefreshToken(userID, sessionID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	
	refreshToken := &RefreshToken{
		Token:     token,
		UserID:    userID,
		SessionID: sessionID,
		ExpiresAt: time.Now().Add(am.config.RefreshTokenTTL),
		IssuedAt:  time.Now(),
		Used:      false,
	}
	
	am.refreshTokens[token] = refreshToken
	
	return token, nil
}

// generateTokenID generates a unique token ID for storage
func (am *AuthManager) generateTokenID(claims *JWTClaims) string {
	data := fmt.Sprintf("%s:%s:%d", claims.UserID, claims.SessionID, claims.IssuedAt)
	hash := sha256.Sum256([]byte(data))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// verifyPassword verifies a password against a hash
func (am *AuthManager) verifyPassword(password, hash, salt string) bool {
	// Simplified password verification - in production, use bcrypt or similar
	inputHash := sha256.Sum256([]byte(password + salt))
	expectedHash := base64.StdEncoding.EncodeToString(inputHash[:])
	return expectedHash == hash
}

// verifyMFA verifies a multi-factor authentication code
func (am *AuthManager) verifyMFA(secret, code string) bool {
	// Simplified MFA verification - in production, use TOTP library
	// This is just a placeholder implementation
	return code == "123456" // Always accept this code for demo
}

// getUserPermissions gets permissions for a user based on roles
func (am *AuthManager) getUserPermissions(user *User) []string {
	var permissions []string
	
	for _, role := range user.Roles {
		switch role {
		case "admin":
			permissions = append(permissions, "read", "write", "delete", "manage_users", "manage_system")
		case "user":
			permissions = append(permissions, "read", "write")
		case "reader":
			permissions = append(permissions, "read")
		}
	}
	
	return permissions
}

// enforceMaxSessions enforces maximum sessions per user
func (am *AuthManager) enforceMaxSessions(userID string) error {
	sessions, err := am.sessionStore.GetUserSessions(userID)
	if err != nil {
		return err
	}
	
	// Count active sessions
	activeSessions := 0
	for _, session := range sessions {
		if session.IsActive && time.Now().Before(session.ExpiresAt) {
			activeSessions++
		}
	}
	
	// Remove oldest sessions if over limit
	if activeSessions >= am.config.MaxSessions {
		// Sort sessions by creation time
		for i := 0; i < len(sessions)-am.config.MaxSessions+1; i++ {
			if sessions[i].IsActive {
				sessions[i].IsActive = false
				am.sessionStore.UpdateSession(sessions[i])
				am.metrics.ActiveSessions--
			}
		}
	}
	
	return nil
}

// cleanupRoutine performs periodic cleanup of expired tokens and sessions
func (am *AuthManager) cleanupRoutine() {
	ticker := time.NewTicker(am.config.CleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		am.cleanup()
	}
}

// cleanup removes expired tokens and sessions
func (am *AuthManager) cleanup() {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	now := time.Now()
	
	// Clean up expired refresh tokens
	for token, refreshToken := range am.refreshTokens {
		if now.After(refreshToken.ExpiresAt) || refreshToken.Used {
			delete(am.refreshTokens, token)
		}
	}
	
	// Clean up blacklisted tokens
	for tokenID, expiry := range am.blacklistedTokens {
		if now.After(expiry) {
			delete(am.blacklistedTokens, tokenID)
		}
	}
	
	// Clean up token store
	am.tokenStore.Cleanup()
	
	// Clean up expired sessions
	am.sessionStore.CleanupExpiredSessions()
}

// GetMetrics returns authentication metrics
func (am *AuthManager) GetMetrics() *AuthMetrics {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	metricsCopy := *am.metrics
	return &metricsCopy
}

// Helper functions

// readKeyFile reads a key file from disk
func readKeyFile(path string) ([]byte, error) {
	// Simplified file reading - in production, handle file permissions properly
	return []byte{}, fmt.Errorf("file reading not implemented in demo")
}

// writeKeyFile writes a key file to disk
func writeKeyFile(path string, data []byte) error {
	// Simplified file writing - in production, handle file permissions properly
	return fmt.Errorf("file writing not implemented in demo")
}