package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// TLSManager manages TLS configuration testing and validation
type TLSManager struct {
	mu       sync.RWMutex
	logger   *log.Logger
	config   TLSConfig
	metrics  *TLSMetrics
}

// TLSConfig contains TLS configuration settings
type TLSConfig struct {
	// Certificate configuration
	CertFile         string        `json:"cert_file"`          // Path to certificate file
	KeyFile          string        `json:"key_file"`           // Path to private key file
	CAFile           string        `json:"ca_file"`            // Path to CA certificate file
	
	// TLS settings
	MinVersion       uint16        `json:"min_version"`        // Minimum TLS version
	MaxVersion       uint16        `json:"max_version"`        // Maximum TLS version
	CipherSuites     []uint16      `json:"cipher_suites"`      // Allowed cipher suites
	CurvePreferences []tls.CurveID `json:"curve_preferences"`  // Preferred elliptic curves
	
	// Security settings
	RequireClientCert bool          `json:"require_client_cert"` // Require client certificates
	VerifyClientCert  bool          `json:"verify_client_cert"`  // Verify client certificates
	InsecureSkipVerify bool         `json:"insecure_skip_verify"` // Skip certificate verification (dev only)
	
	// OCSP and CRL
	EnableOCSP       bool          `json:"enable_ocsp"`         // Enable OCSP stapling
	CRLFile          string        `json:"crl_file"`            // Certificate revocation list
	
	// Session management
	SessionTicketKey []byte        `json:"session_ticket_key"`  // Session ticket key
	SessionTimeout   time.Duration `json:"session_timeout"`     // Session timeout
	
	// Testing configuration
	TestHosts        []string      `json:"test_hosts"`          // Hosts to test TLS configuration
	TestTimeout      time.Duration `json:"test_timeout"`        // Test timeout duration
	TestInterval     time.Duration `json:"test_interval"`       // Test interval for monitoring
}

// TLSTestResult represents the result of a TLS test
type TLSTestResult struct {
	Host             string                 `json:"host"`
	Port             string                 `json:"port"`
	Success          bool                   `json:"success"`
	Error            string                 `json:"error,omitempty"`
	TLSVersion       string                 `json:"tls_version"`
	CipherSuite      string                 `json:"cipher_suite"`
	Certificate      *CertificateInfo       `json:"certificate"`
	SecurityScore    int                    `json:"security_score"`
	Recommendations  []string               `json:"recommendations"`
	TestDetails      *TLSTestDetails        `json:"test_details"`
	TestedAt         time.Time              `json:"tested_at"`
}

// CertificateInfo contains certificate information
type CertificateInfo struct {
	Subject          string            `json:"subject"`
	Issuer           string            `json:"issuer"`
	SerialNumber     string            `json:"serial_number"`
	NotBefore        time.Time         `json:"not_before"`
	NotAfter         time.Time         `json:"not_after"`
	DNSNames         []string          `json:"dns_names"`
	IPAddresses      []string          `json:"ip_addresses"`
	IsCA             bool              `json:"is_ca"`
	KeyUsage         []string          `json:"key_usage"`
	ExtKeyUsage      []string          `json:"ext_key_usage"`
	SignatureAlg     string            `json:"signature_algorithm"`
	PublicKeyAlg     string            `json:"public_key_algorithm"`
	KeySize          int               `json:"key_size"`
	IsExpired        bool              `json:"is_expired"`
	ExpiresInDays    int               `json:"expires_in_days"`
	IsWildcard       bool              `json:"is_wildcard"`
	IsSelfSigned     bool              `json:"is_self_signed"`
}

// TLSTestDetails contains detailed test information
type TLSTestDetails struct {
	SupportedVersions    []string          `json:"supported_versions"`
	SupportedCiphers     []string          `json:"supported_ciphers"`
	SupportedCurves      []string          `json:"supported_curves"`
	SupportsForwardSecrecy bool            `json:"supports_forward_secrecy"`
	SupportsALPN         bool              `json:"supports_alpn"`
	SupportsSNI          bool              `json:"supports_sni"`
	SupportsOCSP         bool              `json:"supports_ocsp"`
	SupportsSCT          bool              `json:"supports_sct"`
	Vulnerabilities      []string          `json:"vulnerabilities"`
	ComplianceCheck      *ComplianceResult `json:"compliance_check"`
	PerformanceMetrics   *TLSPerformance   `json:"performance_metrics"`
}

// ComplianceResult represents compliance check results
type ComplianceResult struct {
	Standard         string            `json:"standard"`
	Compliant        bool              `json:"compliant"`
	Score            int               `json:"score"`
	MaxScore         int               `json:"max_score"`
	Violations       []string          `json:"violations"`
	Recommendations  []string          `json:"recommendations"`
	Details          map[string]bool   `json:"details"`
}

// TLSPerformance contains TLS performance metrics
type TLSPerformance struct {
	HandshakeTime    time.Duration     `json:"handshake_time"`
	ConnectTime      time.Duration     `json:"connect_time"`
	TotalTime        time.Duration     `json:"total_time"`
	BytesTransferred int64             `json:"bytes_transferred"`
	Throughput       float64           `json:"throughput_mbps"`
}

// TLSMetrics tracks TLS testing metrics
type TLSMetrics struct {
	TotalTests       int64             `json:"total_tests"`
	SuccessfulTests  int64             `json:"successful_tests"`
	FailedTests      int64             `json:"failed_tests"`
	AverageScore     float64           `json:"average_score"`
	TestsByHost      map[string]int64  `json:"tests_by_host"`
	ErrorTypes       map[string]int64  `json:"error_types"`
	LastTestTime     time.Time         `json:"last_test_time"`
}

// TLSTestReport represents a comprehensive TLS test report
type TLSTestReport struct {
	ReportID         string            `json:"report_id"`
	GeneratedAt      time.Time         `json:"generated_at"`
	TestDuration     time.Duration     `json:"test_duration"`
	Summary          *TLSTestSummary   `json:"summary"`
	Results          []*TLSTestResult  `json:"results"`
	OverallScore     int               `json:"overall_score"`
	Recommendations  []string          `json:"recommendations"`
	ComplianceStatus map[string]bool   `json:"compliance_status"`
	Metrics          *TLSMetrics       `json:"metrics"`
}

// TLSTestSummary provides test summary information
type TLSTestSummary struct {
	TotalHosts       int               `json:"total_hosts"`
	SuccessfulTests  int               `json:"successful_tests"`
	FailedTests      int               `json:"failed_tests"`
	AverageScore     float64           `json:"average_score"`
	HighRiskHosts    []string          `json:"high_risk_hosts"`
	ExpiringCerts    []string          `json:"expiring_certs"`
	CriticalIssues   []string          `json:"critical_issues"`
}

// NewTLSManager creates a new TLS configuration manager
func NewTLSManager(config TLSConfig, logger *log.Logger) (*TLSManager, error) {
	if logger == nil {
		logger = log.New(log.Writer(), "[TLS] ", log.LstdFlags)
	}
	
	// Set default values
	if config.MinVersion == 0 {
		config.MinVersion = tls.VersionTLS12
	}
	if config.MaxVersion == 0 {
		config.MaxVersion = tls.VersionTLS13
	}
	if config.TestTimeout == 0 {
		config.TestTimeout = 30 * time.Second
	}
	if config.TestInterval == 0 {
		config.TestInterval = 24 * time.Hour
	}
	if config.SessionTimeout == 0 {
		config.SessionTimeout = 24 * time.Hour
	}
	
	// Set secure cipher suites by default
	if len(config.CipherSuites) == 0 {
		config.CipherSuites = getSecureCipherSuites()
	}
	
	// Set secure curve preferences by default
	if len(config.CurvePreferences) == 0 {
		config.CurvePreferences = getSecureCurves()
	}
	
	tm := &TLSManager{
		logger: logger,
		config: config,
		metrics: &TLSMetrics{
			TestsByHost: make(map[string]int64),
			ErrorTypes:  make(map[string]int64),
		},
	}
	
	logger.Printf("TLS manager initialized with min version %d, max version %d", config.MinVersion, config.MaxVersion)
	return tm, nil
}

// TestTLSConfiguration performs comprehensive TLS configuration testing
func (tm *TLSManager) TestTLSConfiguration() (*TLSTestReport, error) {
	startTime := time.Now()
	
	tm.logger.Printf("Starting comprehensive TLS configuration test")
	
	var results []*TLSTestResult
	var totalScore int
	
	// Test each configured host
	for _, host := range tm.config.TestHosts {
		result := tm.testSingleHost(host)
		results = append(results, result)
		
		tm.mu.Lock()
		tm.metrics.TotalTests++
		tm.metrics.TestsByHost[host]++
		if result.Success {
			tm.metrics.SuccessfulTests++
			totalScore += result.SecurityScore
		} else {
			tm.metrics.FailedTests++
			tm.metrics.ErrorTypes[result.Error]++
		}
		tm.metrics.LastTestTime = time.Now()
		tm.mu.Unlock()
	}
	
	// Calculate average score
	averageScore := 0.0
	successfulTests := 0
	for _, result := range results {
		if result.Success {
			averageScore += float64(result.SecurityScore)
			successfulTests++
		}
	}
	if successfulTests > 0 {
		averageScore /= float64(successfulTests)
	}
	
	tm.mu.Lock()
	tm.metrics.AverageScore = averageScore
	tm.mu.Unlock()
	
	// Generate summary
	summary := tm.generateSummary(results)
	
	// Generate overall recommendations
	recommendations := tm.generateRecommendations(results)
	
	// Check compliance status
	complianceStatus := tm.checkComplianceStatus(results)
	
	report := &TLSTestReport{
		ReportID:         fmt.Sprintf("tls-test-%d", time.Now().Unix()),
		GeneratedAt:      time.Now(),
		TestDuration:     time.Since(startTime),
		Summary:          summary,
		Results:          results,
		OverallScore:     int(averageScore),
		Recommendations:  recommendations,
		ComplianceStatus: complianceStatus,
		Metrics:          tm.GetMetrics(),
	}
	
	tm.logger.Printf("TLS configuration test completed. Overall score: %d/100", report.OverallScore)
	
	return report, nil
}

// testSingleHost tests TLS configuration for a single host
func (tm *TLSManager) testSingleHost(hostPort string) *TLSTestResult {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		// Assume default HTTPS port if not specified
		host = hostPort
		port = "443"
		hostPort = net.JoinHostPort(host, port)
	}
	
	result := &TLSTestResult{
		Host:     host,
		Port:     port,
		TestedAt: time.Now(),
	}
	
	// Create TLS configuration for testing
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: tm.config.InsecureSkipVerify,
		MinVersion:         tm.config.MinVersion,
		MaxVersion:         tm.config.MaxVersion,
	}
	
	// Establish connection with timeout
	dialer := &net.Dialer{
		Timeout: tm.config.TestTimeout,
	}
	
	startTime := time.Now()
	conn, err := tls.DialWithDialer(dialer, "tcp", hostPort, tlsConfig)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to connect: %v", err)
		return result
	}
	defer conn.Close()
	
	// Get connection state
	state := conn.ConnectionState()
	result.Success = true
	result.TLSVersion = tm.getTLSVersionString(state.Version)
	result.CipherSuite = tm.getCipherSuiteString(state.CipherSuite)
	
	// Analyze certificate
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		result.Certificate = tm.analyzeCertificate(cert, host)
	}
	
	// Perform detailed testing
	result.TestDetails = tm.performDetailedTests(host, port)
	
	// Calculate security score
	result.SecurityScore = tm.calculateSecurityScore(result)
	
	// Generate recommendations
	result.Recommendations = tm.generateHostRecommendations(result)
	
	// Record performance metrics
	if result.TestDetails != nil {
		result.TestDetails.PerformanceMetrics = &TLSPerformance{
			ConnectTime: time.Since(startTime),
			TotalTime:   time.Since(startTime),
		}
	}
	
	return result
}

// analyzeCertificate analyzes a certificate and returns information
func (tm *TLSManager) analyzeCertificate(cert *x509.Certificate, hostname string) *CertificateInfo {
	now := time.Now()
	expiresInDays := int(cert.NotAfter.Sub(now).Hours() / 24)
	
	info := &CertificateInfo{
		Subject:       cert.Subject.String(),
		Issuer:        cert.Issuer.String(),
		SerialNumber:  cert.SerialNumber.String(),
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		DNSNames:      cert.DNSNames,
		IsCA:          cert.IsCA,
		SignatureAlg:  cert.SignatureAlgorithm.String(),
		PublicKeyAlg:  cert.PublicKeyAlgorithm.String(),
		IsExpired:     now.After(cert.NotAfter),
		ExpiresInDays: expiresInDays,
		IsSelfSigned:  cert.Subject.String() == cert.Issuer.String(),
	}
	
	// Convert IP addresses to strings
	for _, ip := range cert.IPAddresses {
		info.IPAddresses = append(info.IPAddresses, ip.String())
	}
	
	// Analyze key usage
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		info.KeyUsage = append(info.KeyUsage, "Digital Signature")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		info.KeyUsage = append(info.KeyUsage, "Key Encipherment")
	}
	if cert.KeyUsage&x509.KeyUsageDataEncipherment != 0 {
		info.KeyUsage = append(info.KeyUsage, "Data Encipherment")
	}
	
	// Analyze extended key usage
	for _, usage := range cert.ExtKeyUsage {
		switch usage {
		case x509.ExtKeyUsageServerAuth:
			info.ExtKeyUsage = append(info.ExtKeyUsage, "Server Authentication")
		case x509.ExtKeyUsageClientAuth:
			info.ExtKeyUsage = append(info.ExtKeyUsage, "Client Authentication")
		case x509.ExtKeyUsageTimeStamping:
			info.ExtKeyUsage = append(info.ExtKeyUsage, "Time Stamping")
		}
	}
	
	// Check for wildcard certificates
	for _, name := range cert.DNSNames {
		if len(name) > 0 && name[0] == '*' {
			info.IsWildcard = true
			break
		}
	}
	
	// Determine key size based on public key type
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		info.KeySize = pub.N.BitLen()
	case *ecdsa.PublicKey:
		info.KeySize = pub.Curve.Params().BitSize
	}
	
	return info
}

// performDetailedTests performs detailed TLS configuration tests
func (tm *TLSManager) performDetailedTests(host, port string) *TLSTestDetails {
	details := &TLSTestDetails{
		SupportedVersions: []string{},
		SupportedCiphers:  []string{},
		SupportedCurves:   []string{},
		Vulnerabilities:   []string{},
	}
	
	// Test supported TLS versions
	versions := []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13}
	for _, version := range versions {
		if tm.testTLSVersion(host, port, version) {
			details.SupportedVersions = append(details.SupportedVersions, tm.getTLSVersionString(version))
		}
	}
	
	// Test cipher suites (simplified for demonstration)
	testCiphers := []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	}
	
	for _, cipher := range testCiphers {
		if tm.testCipherSuite(host, port, cipher) {
			details.SupportedCiphers = append(details.SupportedCiphers, tm.getCipherSuiteString(cipher))
		}
	}
	
	// Check for forward secrecy support
	details.SupportsForwardSecrecy = tm.checkForwardSecrecy(details.SupportedCiphers)
	
	// Check vulnerabilities
	details.Vulnerabilities = tm.checkVulnerabilities(host, port, details)
	
	// Perform compliance check
	details.ComplianceCheck = tm.performComplianceCheck(details)
	
	return details
}

// testTLSVersion tests if a specific TLS version is supported
func (tm *TLSManager) testTLSVersion(host, port string, version uint16) bool {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: tm.config.InsecureSkipVerify,
		MinVersion:         version,
		MaxVersion:         version,
	}
	
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), tlsConfig)
	if err != nil {
		return false
	}
	defer conn.Close()
	
	return conn.ConnectionState().Version == version
}

// testCipherSuite tests if a specific cipher suite is supported
func (tm *TLSManager) testCipherSuite(host, port string, cipher uint16) bool {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: tm.config.InsecureSkipVerify,
		CipherSuites:       []uint16{cipher},
		MinVersion:         tls.VersionTLS12,
	}
	
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), tlsConfig)
	if err != nil {
		return false
	}
	defer conn.Close()
	
	return conn.ConnectionState().CipherSuite == cipher
}

// checkForwardSecrecy checks if the connection supports forward secrecy
func (tm *TLSManager) checkForwardSecrecy(ciphers []string) bool {
	forwardSecrecyCiphers := []string{
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
		"TLS_AES_128_GCM_SHA256",
		"TLS_AES_256_GCM_SHA384",
		"TLS_CHACHA20_POLY1305_SHA256",
	}
	
	for _, cipher := range ciphers {
		for _, fsCipher := range forwardSecrecyCiphers {
			if cipher == fsCipher {
				return true
			}
		}
	}
	return false
}

// checkVulnerabilities checks for known TLS vulnerabilities
func (tm *TLSManager) checkVulnerabilities(host, port string, details *TLSTestDetails) []string {
	var vulnerabilities []string
	
	// Check for weak TLS versions
	for _, version := range details.SupportedVersions {
		if version == "TLS 1.0" || version == "TLS 1.1" {
			vulnerabilities = append(vulnerabilities, fmt.Sprintf("Weak TLS version supported: %s", version))
		}
	}
	
	// Check for weak cipher suites
	weakCiphers := []string{"RC4", "DES", "3DES", "MD5"}
	for _, cipher := range details.SupportedCiphers {
		for _, weak := range weakCiphers {
			if contains(cipher, weak) {
				vulnerabilities = append(vulnerabilities, fmt.Sprintf("Weak cipher suite: %s", cipher))
			}
		}
	}
	
	// Check for BEAST vulnerability (TLS 1.0 with CBC ciphers)
	if contains(details.SupportedVersions, "TLS 1.0") {
		for _, cipher := range details.SupportedCiphers {
			if contains(cipher, "CBC") {
				vulnerabilities = append(vulnerabilities, "BEAST vulnerability: TLS 1.0 with CBC cipher")
				break
			}
		}
	}
	
	return vulnerabilities
}

// performComplianceCheck performs compliance checks against standards
func (tm *TLSManager) performComplianceCheck(details *TLSTestDetails) *ComplianceResult {
	result := &ComplianceResult{
		Standard: "NIST TLS Guidelines",
		Details:  make(map[string]bool),
	}
	
	score := 0
	maxScore := 5
	
	// Check minimum TLS version
	if !contains(details.SupportedVersions, "TLS 1.0") && !contains(details.SupportedVersions, "TLS 1.1") {
		score++
		result.Details["secure_tls_versions"] = true
	} else {
		result.Violations = append(result.Violations, "Weak TLS versions (1.0/1.1) supported")
	}
	
	// Check forward secrecy
	if details.SupportsForwardSecrecy {
		score++
		result.Details["forward_secrecy"] = true
	} else {
		result.Violations = append(result.Violations, "Forward secrecy not supported")
	}
	
	// Check for strong cipher suites
	hasStrongCipher := false
	for _, cipher := range details.SupportedCiphers {
		if contains(cipher, "AES_256_GCM") || contains(cipher, "CHACHA20_POLY1305") {
			hasStrongCipher = true
			break
		}
	}
	if hasStrongCipher {
		score++
		result.Details["strong_ciphers"] = true
	} else {
		result.Violations = append(result.Violations, "No strong cipher suites found")
	}
	
	// Check for TLS 1.3 support
	if contains(details.SupportedVersions, "TLS 1.3") {
		score++
		result.Details["tls_1_3_support"] = true
	} else {
		result.Recommendations = append(result.Recommendations, "Enable TLS 1.3 support")
	}
	
	// Check for vulnerabilities
	if len(details.Vulnerabilities) == 0 {
		score++
		result.Details["no_known_vulnerabilities"] = true
	} else {
		for _, vuln := range details.Vulnerabilities {
			result.Violations = append(result.Violations, vuln)
		}
	}
	
	result.Score = score
	result.MaxScore = maxScore
	result.Compliant = score >= maxScore-1 // Allow one minor violation
	
	return result
}

// calculateSecurityScore calculates a security score for the TLS configuration
func (tm *TLSManager) calculateSecurityScore(result *TLSTestResult) int {
	score := 0
	
	// TLS version score (30 points)
	switch result.TLSVersion {
	case "TLS 1.3":
		score += 30
	case "TLS 1.2":
		score += 25
	case "TLS 1.1":
		score += 10
	case "TLS 1.0":
		score += 5
	}
	
	// Cipher suite score (25 points)
	if contains(result.CipherSuite, "AES_256_GCM") || contains(result.CipherSuite, "CHACHA20_POLY1305") {
		score += 25
	} else if contains(result.CipherSuite, "AES_128_GCM") {
		score += 20
	} else if contains(result.CipherSuite, "AES") {
		score += 15
	}
	
	// Certificate score (25 points)
	if result.Certificate != nil {
		cert := result.Certificate
		if !cert.IsExpired {
			score += 10
		}
		if cert.ExpiresInDays > 30 {
			score += 5
		}
		if cert.KeySize >= 2048 {
			score += 5
		}
		if !cert.IsSelfSigned {
			score += 5
		}
	}
	
	// Security features score (20 points)
	if result.TestDetails != nil {
		if result.TestDetails.SupportsForwardSecrecy {
			score += 10
		}
		if len(result.TestDetails.Vulnerabilities) == 0 {
			score += 10
		}
	}
	
	// Cap at 100
	if score > 100 {
		score = 100
	}
	
	return score
}

// generateHostRecommendations generates recommendations for a specific host
func (tm *TLSManager) generateHostRecommendations(result *TLSTestResult) []string {
	var recommendations []string
	
	// TLS version recommendations
	if result.TLSVersion != "TLS 1.3" {
		recommendations = append(recommendations, "Upgrade to TLS 1.3 for improved security")
	}
	
	// Certificate recommendations
	if result.Certificate != nil {
		cert := result.Certificate
		if cert.ExpiresInDays < 30 {
			recommendations = append(recommendations, fmt.Sprintf("Certificate expires in %d days - renew soon", cert.ExpiresInDays))
		}
		if cert.KeySize < 2048 {
			recommendations = append(recommendations, "Use at least 2048-bit RSA or 256-bit ECDSA keys")
		}
		if cert.IsSelfSigned {
			recommendations = append(recommendations, "Use certificates from a trusted CA instead of self-signed")
		}
	}
	
	// Security feature recommendations
	if result.TestDetails != nil {
		if !result.TestDetails.SupportsForwardSecrecy {
			recommendations = append(recommendations, "Enable forward secrecy with ECDHE cipher suites")
		}
		if len(result.TestDetails.Vulnerabilities) > 0 {
			recommendations = append(recommendations, "Address identified vulnerabilities")
		}
	}
	
	return recommendations
}

// generateSummary generates a test summary
func (tm *TLSManager) generateSummary(results []*TLSTestResult) *TLSTestSummary {
	summary := &TLSTestSummary{
		TotalHosts:    len(results),
		HighRiskHosts: []string{},
		ExpiringCerts: []string{},
		CriticalIssues: []string{},
	}
	
	var totalScore float64
	for _, result := range results {
		if result.Success {
			summary.SuccessfulTests++
			totalScore += float64(result.SecurityScore)
			
			// Check for high-risk hosts
			if result.SecurityScore < 50 {
				summary.HighRiskHosts = append(summary.HighRiskHosts, result.Host)
			}
			
			// Check for expiring certificates
			if result.Certificate != nil && result.Certificate.ExpiresInDays < 30 {
				summary.ExpiringCerts = append(summary.ExpiringCerts, result.Host)
			}
			
			// Check for critical issues
			if result.TestDetails != nil && len(result.TestDetails.Vulnerabilities) > 0 {
				summary.CriticalIssues = append(summary.CriticalIssues, fmt.Sprintf("%s: %s", result.Host, result.TestDetails.Vulnerabilities[0]))
			}
		} else {
			summary.FailedTests++
			summary.CriticalIssues = append(summary.CriticalIssues, fmt.Sprintf("%s: %s", result.Host, result.Error))
		}
	}
	
	if summary.SuccessfulTests > 0 {
		summary.AverageScore = totalScore / float64(summary.SuccessfulTests)
	}
	
	return summary
}

// generateRecommendations generates overall recommendations
func (tm *TLSManager) generateRecommendations(results []*TLSTestResult) []string {
	recommendations := make(map[string]int)
	
	for _, result := range results {
		for _, rec := range result.Recommendations {
			recommendations[rec]++
		}
	}
	
	var sorted []string
	for rec := range recommendations {
		sorted = append(sorted, rec)
	}
	
	return sorted
}

// checkComplianceStatus checks compliance status for various standards
func (tm *TLSManager) checkComplianceStatus(results []*TLSTestResult) map[string]bool {
	compliance := make(map[string]bool)
	
	// Check overall compliance
	allCompliant := true
	for _, result := range results {
		if !result.Success || result.SecurityScore < 70 {
			allCompliant = false
			break
		}
	}
	
	compliance["NIST_TLS_Guidelines"] = allCompliant
	compliance["PCI_DSS"] = allCompliant
	compliance["SOC2"] = allCompliant
	
	return compliance
}

// GetMetrics returns TLS testing metrics
func (tm *TLSManager) GetMetrics() *TLSMetrics {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	metricsCopy := *tm.metrics
	metricsCopy.TestsByHost = make(map[string]int64)
	metricsCopy.ErrorTypes = make(map[string]int64)
	
	for k, v := range tm.metrics.TestsByHost {
		metricsCopy.TestsByHost[k] = v
	}
	for k, v := range tm.metrics.ErrorTypes {
		metricsCopy.ErrorTypes[k] = v
	}
	
	return &metricsCopy
}

// Helper functions

// getSecureCipherSuites returns a list of secure cipher suites
func getSecureCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	}
}

// getSecureCurves returns a list of secure elliptic curves
func getSecureCurves() []tls.CurveID {
	return []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
		tls.CurveP384,
		tls.CurveP521,
	}
}

// getTLSVersionString converts TLS version to string
func (tm *TLSManager) getTLSVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (%d)", version)
	}
}

// getCipherSuiteString converts cipher suite to string
func (tm *TLSManager) getCipherSuiteString(cipher uint16) string {
	switch cipher {
	case tls.TLS_AES_128_GCM_SHA256:
		return "TLS_AES_128_GCM_SHA256"
	case tls.TLS_AES_256_GCM_SHA384:
		return "TLS_AES_256_GCM_SHA384"
	case tls.TLS_CHACHA20_POLY1305_SHA256:
		return "TLS_CHACHA20_POLY1305_SHA256"
	case tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:
		return "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	case tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:
		return "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	default:
		return fmt.Sprintf("Unknown (%d)", cipher)
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && 
			(s[:len(substr)] == substr || 
			 s[len(s)-len(substr):] == substr || 
			 containsSubstring(s, substr))))
}

// containsSubstring checks if string contains substring anywhere
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}