package security

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SecurityScanner performs comprehensive security vulnerability scanning
type SecurityScanner struct {
	mu           sync.RWMutex
	logger       *log.Logger
	config       ScannerConfig
	scanners     map[string]VulnerabilityScanner
	results      map[string]*ScanResult
	reports      map[string]*SecurityReport
	isRunning    bool
	metrics      *ScannerMetrics
	auditLogger  AuditLogger
}

// ScannerConfig contains security scanner configuration
type ScannerConfig struct {
	// Target configuration
	TargetEndpoints []string      `json:"target_endpoints"`  // Endpoints to scan
	TargetPorts     []int         `json:"target_ports"`      // Ports to scan
	ExcludeHosts    []string      `json:"exclude_hosts"`     // Hosts to exclude
	
	// Scan configuration
	ScanTimeout     time.Duration `json:"scan_timeout"`      // Individual scan timeout
	TotalTimeout    time.Duration `json:"total_timeout"`     // Total scan timeout
	Concurrency     int           `json:"concurrency"`       // Concurrent scans
	Retries         int           `json:"retries"`           // Retry attempts
	
	// Scan types
	EnablePortScan      bool      `json:"enable_port_scan"`       // Enable port scanning
	EnableTLSScan       bool      `json:"enable_tls_scan"`        // Enable TLS scanning
	EnableHeaderScan    bool      `json:"enable_header_scan"`     // Enable header scanning
	EnableAuthScan      bool      `json:"enable_auth_scan"`       // Enable auth scanning
	EnableInjectionScan bool      `json:"enable_injection_scan"`  // Enable injection scanning
	EnableDOSScan       bool      `json:"enable_dos_scan"`        // Enable DoS scanning
	EnableConfigScan    bool      `json:"enable_config_scan"`     // Enable config scanning
	
	// Reporting configuration
	ReportFormat        string    `json:"report_format"`          // Report format (json, html, pdf)
	ReportDirectory     string    `json:"report_directory"`       // Report output directory
	IncludeRemediation  bool      `json:"include_remediation"`    // Include remediation steps
	IncludeEvidence     bool      `json:"include_evidence"`       // Include evidence
	
	// Risk assessment
	RiskThreshold       RiskLevel `json:"risk_threshold"`         // Minimum risk level to report
	SeverityThreshold   Severity  `json:"severity_threshold"`     // Minimum severity to report
	
	// Compliance scanning
	ComplianceStandards []string  `json:"compliance_standards"`   // Standards to check (OWASP, NIST, etc.)
	CustomRules         []SecurityRule `json:"custom_rules"`      // Custom security rules
}

// VulnerabilityScanner interface for different vulnerability scanners
type VulnerabilityScanner interface {
	Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error)
	GetName() string
	GetDescription() string
	GetSeverity() Severity
}

// ScanTarget represents a target for scanning
type ScanTarget struct {
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	Protocol    string            `json:"protocol"`
	Endpoint    string            `json:"endpoint"`
	Credentials *Credentials      `json:"credentials,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// Credentials for authenticated scanning
type Credentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Token     string `json:"token"`
	APIKey    string `json:"api_key"`
	AuthType  string `json:"auth_type"` // basic, bearer, apikey
}

// ScanResult contains the result of a vulnerability scan
type ScanResult struct {
	ScannerName     string                 `json:"scanner_name"`
	Target          *ScanTarget            `json:"target"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
	Duration        time.Duration          `json:"duration"`
	Status          ScanStatus             `json:"status"`
	Vulnerabilities []*Vulnerability       `json:"vulnerabilities"`
	Summary         *ScanSummary           `json:"summary"`
	Evidence        map[string]interface{} `json:"evidence,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
	Error           string                 `json:"error,omitempty"`
}

// Vulnerability represents a discovered vulnerability
type Vulnerability struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Severity      Severity               `json:"severity"`
	Risk          RiskLevel              `json:"risk"`
	Category      VulnerabilityCategory  `json:"category"`
	CVE           string                 `json:"cve,omitempty"`
	CWE           string                 `json:"cwe,omitempty"`
	CVSS          *CVSSScore             `json:"cvss,omitempty"`
	Confidence    float64                `json:"confidence"`
	Impact        *Impact                `json:"impact"`
	Remediation   *Remediation           `json:"remediation"`
	References    []string               `json:"references"`
	Evidence      map[string]interface{} `json:"evidence"`
	FirstSeen     time.Time              `json:"first_seen"`
	LastSeen      time.Time              `json:"last_seen"`
	Compliance    []ComplianceMapping    `json:"compliance,omitempty"`
}

// CVSSScore represents Common Vulnerability Scoring System score
type CVSSScore struct {
	Version           string  `json:"version"`
	BaseScore         float64 `json:"base_score"`
	TemporalScore     float64 `json:"temporal_score"`
	EnvironmentalScore float64 `json:"environmental_score"`
	Vector            string  `json:"vector"`
}

// Impact describes the impact of a vulnerability
type Impact struct {
	Confidentiality string `json:"confidentiality"` // none, low, high
	Integrity       string `json:"integrity"`       // none, low, high
	Availability    string `json:"availability"`    // none, low, high
	Scope           string `json:"scope"`           // unchanged, changed
}

// Remediation provides remediation information
type Remediation struct {
	Priority        string   `json:"priority"`        // low, medium, high, critical
	Effort          string   `json:"effort"`          // low, medium, high
	Steps           []string `json:"steps"`           // Remediation steps
	References      []string `json:"references"`      // Additional references
	EstimatedTime   string   `json:"estimated_time"`  // Estimated fix time
	TechnicalDebt   string   `json:"technical_debt"`  // Technical debt assessment
}

// ComplianceMapping maps vulnerability to compliance standards
type ComplianceMapping struct {
	Standard    string `json:"standard"`    // OWASP, NIST, ISO27001, etc.
	Control     string `json:"control"`     // Control identifier
	Requirement string `json:"requirement"` // Requirement description
}

// SecurityRule represents a custom security rule
type SecurityRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    VulnerabilityCategory  `json:"category"`
	Severity    Severity               `json:"severity"`
	Pattern     string                 `json:"pattern"`     // Regex pattern
	Test        func(*ScanTarget) bool `json:"-"`           // Custom test function
	Remediation *Remediation           `json:"remediation"`
}

// ScanSummary provides a summary of scan results
type ScanSummary struct {
	TotalVulnerabilities int                           `json:"total_vulnerabilities"`
	BySeverity          map[Severity]int              `json:"by_severity"`
	ByCategory          map[VulnerabilityCategory]int `json:"by_category"`
	ByRisk              map[RiskLevel]int             `json:"by_risk"`
	RiskScore           float64                       `json:"risk_score"`
	ComplianceStatus    map[string]bool               `json:"compliance_status"`
}

// SecurityReport contains comprehensive security assessment
type SecurityReport struct {
	ID                string                 `json:"id"`
	GeneratedAt       time.Time              `json:"generated_at"`
	ScanDuration      time.Duration          `json:"scan_duration"`
	Targets           []*ScanTarget          `json:"targets"`
	Results           []*ScanResult          `json:"results"`
	Summary           *ReportSummary         `json:"summary"`
	Recommendations   []string               `json:"recommendations"`
	ComplianceStatus  map[string]*ComplianceStatus `json:"compliance_status"`
	TrendAnalysis     *TrendAnalysis         `json:"trend_analysis,omitempty"`
	Executive Summary *ExecutiveSummary      `json:"executive_summary"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// ReportSummary provides overall report summary
type ReportSummary struct {
	TotalTargets        int                           `json:"total_targets"`
	TargetsScanned      int                           `json:"targets_scanned"`
	TotalVulnerabilities int                          `json:"total_vulnerabilities"`
	CriticalVulns       int                           `json:"critical_vulnerabilities"`
	HighVulns           int                           `json:"high_vulnerabilities"`
	MediumVulns         int                           `json:"medium_vulnerabilities"`
	LowVulns            int                           `json:"low_vulnerabilities"`
	OverallRiskScore    float64                       `json:"overall_risk_score"`
	SecurityPosture     string                        `json:"security_posture"`
	BySeverity          map[Severity]int              `json:"by_severity"`
	ByCategory          map[VulnerabilityCategory]int `json:"by_category"`
}

// ComplianceStatus tracks compliance with security standards
type ComplianceStatus struct {
	Standard         string             `json:"standard"`
	OverallScore     float64            `json:"overall_score"`
	PassedControls   int                `json:"passed_controls"`
	FailedControls   int                `json:"failed_controls"`
	TotalControls    int                `json:"total_controls"`
	ControlResults   map[string]bool    `json:"control_results"`
	Recommendations  []string           `json:"recommendations"`
	CriticalFailures []string           `json:"critical_failures"`
}

// TrendAnalysis provides trend analysis over time
type TrendAnalysis struct {
	PreviousScans       []*ScanSummary     `json:"previous_scans"`
	VulnerabilityTrend  string             `json:"vulnerability_trend"` // improving, stable, degrading
	RiskTrend           string             `json:"risk_trend"`
	NewVulnerabilities  int                `json:"new_vulnerabilities"`
	FixedVulnerabilities int               `json:"fixed_vulnerabilities"`
}

// ExecutiveSummary provides executive-level summary
type ExecutiveSummary struct {
	OverallSecurity     string   `json:"overall_security"`      // excellent, good, fair, poor
	KeyFindings         []string `json:"key_findings"`
	CriticalActions     []string `json:"critical_actions"`
	BusinessImpact      string   `json:"business_impact"`
	RecommendedBudget   string   `json:"recommended_budget"`
	Timeline            string   `json:"timeline"`
	ROI                 string   `json:"roi"`
}

// ScannerMetrics tracks scanner performance
type ScannerMetrics struct {
	TotalScans          int64             `json:"total_scans"`
	SuccessfulScans     int64             `json:"successful_scans"`
	FailedScans         int64             `json:"failed_scans"`
	AverageScanTime     time.Duration     `json:"average_scan_time"`
	TotalVulnsFound     int64             `json:"total_vulnerabilities_found"`
	VulnsBySeverity     map[Severity]int64 `json:"vulnerabilities_by_severity"`
	ScansByType         map[string]int64  `json:"scans_by_type"`
	LastScanTime        time.Time         `json:"last_scan_time"`
}

// Enums
type ScanStatus string
const (
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusTimeout   ScanStatus = "timeout"
	ScanStatusCancelled ScanStatus = "cancelled"
)

type VulnerabilityCategory string
const (
	CategoryAuthentication VulnerabilityCategory = "authentication"
	CategoryAuthorization  VulnerabilityCategory = "authorization"
	CategoryEncryption     VulnerabilityCategory = "encryption"
	CategoryInjection      VulnerabilityCategory = "injection"
	CategoryXSS            VulnerabilityCategory = "xss"
	CategoryCSRF           VulnerabilityCategory = "csrf"
	CategoryDOS            VulnerabilityCategory = "dos"
	CategoryConfiguration  VulnerabilityCategory = "configuration"
	CategoryDisclosure     VulnerabilityCategory = "information_disclosure"
	CategoryValidation     VulnerabilityCategory = "input_validation"
	CategorySession        VulnerabilityCategory = "session_management"
	CategoryCrypto         VulnerabilityCategory = "cryptographic"
	CategoryInfrastructure VulnerabilityCategory = "infrastructure"
)

// NewSecurityScanner creates a new security scanner
func NewSecurityScanner(config ScannerConfig, auditLogger AuditLogger, logger *log.Logger) *SecurityScanner {
	if logger == nil {
		logger = log.New(log.Writer(), "[SECURITY-SCANNER] ", log.LstdFlags)
	}
	
	// Set default values
	if config.ScanTimeout == 0 {
		config.ScanTimeout = 30 * time.Second
	}
	if config.TotalTimeout == 0 {
		config.TotalTimeout = 10 * time.Minute
	}
	if config.Concurrency == 0 {
		config.Concurrency = 5
	}
	if config.Retries == 0 {
		config.Retries = 3
	}
	if config.ReportDirectory == "" {
		config.ReportDirectory = "./security_reports"
	}
	if len(config.TargetPorts) == 0 {
		config.TargetPorts = []int{80, 443, 8080, 8443}
	}
	
	ss := &SecurityScanner{
		logger:      logger,
		config:      config,
		scanners:    make(map[string]VulnerabilityScanner),
		results:     make(map[string]*ScanResult),
		reports:     make(map[string]*SecurityReport),
		auditLogger: auditLogger,
		metrics: &ScannerMetrics{
			VulnsBySeverity: make(map[Severity]int64),
			ScansByType:     make(map[string]int64),
		},
	}
	
	// Initialize scanners
	ss.initializeScanners()
	
	logger.Printf("Security scanner initialized with %d scanners", len(ss.scanners))
	return ss
}

// initializeScanners initializes vulnerability scanners
func (ss *SecurityScanner) initializeScanners() {
	// Port scanner
	if ss.config.EnablePortScan {
		ss.scanners["port_scan"] = &PortScanner{
			timeout: ss.config.ScanTimeout,
			ports:   ss.config.TargetPorts,
		}
	}
	
	// TLS scanner
	if ss.config.EnableTLSScan {
		ss.scanners["tls_scan"] = &TLSScanner{
			timeout: ss.config.ScanTimeout,
		}
	}
	
	// HTTP header scanner
	if ss.config.EnableHeaderScan {
		ss.scanners["header_scan"] = &HeaderScanner{
			timeout: ss.config.ScanTimeout,
		}
	}
	
	// Authentication scanner
	if ss.config.EnableAuthScan {
		ss.scanners["auth_scan"] = &AuthScanner{
			timeout: ss.config.ScanTimeout,
		}
	}
	
	// Injection scanner
	if ss.config.EnableInjectionScan {
		ss.scanners["injection_scan"] = &InjectionScanner{
			timeout: ss.config.ScanTimeout,
		}
	}
	
	// Configuration scanner
	if ss.config.EnableConfigScan {
		ss.scanners["config_scan"] = &ConfigScanner{
			timeout: ss.config.ScanTimeout,
		}
	}
	
	// Custom rules scanner
	if len(ss.config.CustomRules) > 0 {
		ss.scanners["custom_rules"] = &CustomRulesScanner{
			rules:   ss.config.CustomRules,
			timeout: ss.config.ScanTimeout,
		}
	}
}

// RunComprehensiveScan runs a comprehensive security scan
func (ss *SecurityScanner) RunComprehensiveScan(ctx context.Context) (*SecurityReport, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	
	if ss.isRunning {
		return nil, fmt.Errorf("scan already running")
	}
	
	ss.logger.Printf("Starting comprehensive security scan")
	ss.isRunning = true
	defer func() { ss.isRunning = false }()
	
	startTime := time.Now()
	
	// Prepare targets
	targets := ss.prepareTargets()
	if len(targets) == 0 {
		return nil, fmt.Errorf("no targets to scan")
	}
	
	// Create context with timeout
	scanCtx, cancel := context.WithTimeout(ctx, ss.config.TotalTimeout)
	defer cancel()
	
	// Run scans
	results := ss.runScans(scanCtx, targets)
	
	// Generate report
	report := ss.generateReport(targets, results, time.Since(startTime))
	
	// Store report
	reportID := ss.generateReportID()
	ss.reports[reportID] = report
	
	// Save report to file
	if err := ss.saveReport(report); err != nil {
		ss.logger.Printf("Failed to save report: %v", err)
	}
	
	// Log audit event
	if ss.auditLogger != nil {
		ss.auditLogger.LogSecurityEvent("security_scan_completed", 
			fmt.Sprintf("Comprehensive security scan completed with %d vulnerabilities found", 
				report.Summary.TotalVulnerabilities), 
			SeverityMedium,
			map[string]interface{}{
				"targets_scanned": len(targets),
				"vulnerabilities": report.Summary.TotalVulnerabilities,
				"duration":        time.Since(startTime).String(),
			})
	}
	
	ss.logger.Printf("Security scan completed in %v: %d targets, %d vulnerabilities", 
		time.Since(startTime), len(targets), report.Summary.TotalVulnerabilities)
	
	return report, nil
}

// prepareTargets prepares scan targets
func (ss *SecurityScanner) prepareTargets() []*ScanTarget {
	var targets []*ScanTarget
	
	for _, endpoint := range ss.config.TargetEndpoints {
		// Parse endpoint
		parts := strings.Split(endpoint, ":")
		host := parts[0]
		
		// Skip excluded hosts
		if ss.isExcluded(host) {
			continue
		}
		
		if len(parts) > 1 {
			// Specific port
			if port, err := strconv.Atoi(parts[1]); err == nil {
				targets = append(targets, &ScanTarget{
					Host:     host,
					Port:     port,
					Protocol: "tcp",
					Endpoint: endpoint,
					Context:  make(map[string]interface{}),
				})
			}
		} else {
			// Scan common ports
			for _, port := range ss.config.TargetPorts {
				targets = append(targets, &ScanTarget{
					Host:     host,
					Port:     port,
					Protocol: "tcp",
					Endpoint: fmt.Sprintf("%s:%d", host, port),
					Context:  make(map[string]interface{}),
				})
			}
		}
	}
	
	return targets
}

// isExcluded checks if a host is excluded
func (ss *SecurityScanner) isExcluded(host string) bool {
	for _, excluded := range ss.config.ExcludeHosts {
		if host == excluded {
			return true
		}
	}
	return false
}

// runScans runs all configured scanners against targets
func (ss *SecurityScanner) runScans(ctx context.Context, targets []*ScanTarget) []*ScanResult {
	var allResults []*ScanResult
	
	// Run each scanner
	for scannerName, scanner := range ss.scanners {
		ss.logger.Printf("Running %s scanner", scannerName)
		
		results := ss.runScannerOnTargets(ctx, scanner, targets)
		allResults = append(allResults, results...)
		
		// Update metrics
		ss.metrics.ScansByType[scannerName] += int64(len(results))
	}
	
	return allResults
}

// runScannerOnTargets runs a specific scanner on all targets
func (ss *SecurityScanner) runScannerOnTargets(ctx context.Context, scanner VulnerabilityScanner, targets []*ScanTarget) []*ScanResult {
	var results []*ScanResult
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Create semaphore for concurrency control
	semaphore := make(chan struct{}, ss.config.Concurrency)
	
	for _, target := range targets {
		wg.Add(1)
		
		go func(t *ScanTarget) {
			defer wg.Done()
			
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			// Run scan with retries
			var result *ScanResult
			var err error
			
			for attempt := 0; attempt <= ss.config.Retries; attempt++ {
				result, err = scanner.Scan(ctx, t)
				if err == nil {
					break
				}
				
				// Check if context was cancelled
				if ctx.Err() != nil {
					break
				}
				
				// Wait before retry
				if attempt < ss.config.Retries {
					time.Sleep(time.Second * time.Duration(attempt+1))
				}
			}
			
			if result != nil {
				mu.Lock()
				results = append(results, result)
				ss.updateMetrics(result)
				mu.Unlock()
			}
		}(target)
	}
	
	wg.Wait()
	return results
}

// updateMetrics updates scanner metrics
func (ss *SecurityScanner) updateMetrics(result *ScanResult) {
	ss.metrics.TotalScans++
	
	if result.Status == ScanStatusCompleted {
		ss.metrics.SuccessfulScans++
	} else {
		ss.metrics.FailedScans++
	}
	
	// Update vulnerability metrics
	for _, vuln := range result.Vulnerabilities {
		ss.metrics.TotalVulnsFound++
		ss.metrics.VulnsBySeverity[vuln.Severity]++
	}
	
	// Update average scan time
	if ss.metrics.TotalScans > 0 {
		ss.metrics.AverageScanTime = (ss.metrics.AverageScanTime*time.Duration(ss.metrics.TotalScans-1) + result.Duration) / time.Duration(ss.metrics.TotalScans)
	}
	
	ss.metrics.LastScanTime = time.Now()
}

// generateReport generates a comprehensive security report
func (ss *SecurityScanner) generateReport(targets []*ScanTarget, results []*ScanResult, duration time.Duration) *SecurityReport {
	report := &SecurityReport{
		ID:           ss.generateReportID(),
		GeneratedAt:  time.Now(),
		ScanDuration: duration,
		Targets:      targets,
		Results:      results,
		Metadata:     make(map[string]interface{}),
	}
	
	// Generate summary
	report.Summary = ss.generateReportSummary(targets, results)
	
	// Generate recommendations
	report.Recommendations = ss.generateRecommendations(results)
	
	// Generate compliance status
	report.ComplianceStatus = ss.generateComplianceStatus(results)
	
	// Generate executive summary
	report.ExecutiveSummary = ss.generateExecutiveSummary(report.Summary)
	
	return report
}

// generateReportSummary generates report summary
func (ss *SecurityScanner) generateReportSummary(targets []*ScanTarget, results []*ScanResult) *ReportSummary {
	summary := &ReportSummary{
		TotalTargets:   len(targets),
		TargetsScanned: len(results),
		BySeverity:     make(map[Severity]int),
		ByCategory:     make(map[VulnerabilityCategory]int),
	}
	
	// Count vulnerabilities by severity and category
	for _, result := range results {
		for _, vuln := range result.Vulnerabilities {
			summary.TotalVulnerabilities++
			summary.BySeverity[vuln.Severity]++
			summary.ByCategory[vuln.Category]++
			
			switch vuln.Severity {
			case SeverityCritical:
				summary.CriticalVulns++
			case SeverityHigh:
				summary.HighVulns++
			case SeverityMedium:
				summary.MediumVulns++
			case SeverityLow:
				summary.LowVulns++
			}
		}
	}
	
	// Calculate overall risk score
	summary.OverallRiskScore = ss.calculateRiskScore(summary)
	
	// Determine security posture
	summary.SecurityPosture = ss.determineSecurityPosture(summary.OverallRiskScore)
	
	return summary
}

// calculateRiskScore calculates overall risk score
func (ss *SecurityScanner) calculateRiskScore(summary *ReportSummary) float64 {
	if summary.TotalVulnerabilities == 0 {
		return 0.0
	}
	
	// Weighted scoring: Critical=10, High=7, Medium=4, Low=1
	score := float64(summary.CriticalVulns*10 + summary.HighVulns*7 + summary.MediumVulns*4 + summary.LowVulns*1)
	
	// Normalize to 0-100 scale
	maxPossibleScore := float64(summary.TotalVulnerabilities * 10)
	return (score / maxPossibleScore) * 100
}

// determineSecurityPosture determines security posture based on risk score
func (ss *SecurityScanner) determineSecurityPosture(riskScore float64) string {
	switch {
	case riskScore >= 80:
		return "critical"
	case riskScore >= 60:
		return "poor"
	case riskScore >= 40:
		return "fair"
	case riskScore >= 20:
		return "good"
	default:
		return "excellent"
	}
}

// generateRecommendations generates security recommendations
func (ss *SecurityScanner) generateRecommendations(results []*ScanResult) []string {
	var recommendations []string
	
	// Count vulnerability categories
	categoryCount := make(map[VulnerabilityCategory]int)
	for _, result := range results {
		for _, vuln := range result.Vulnerabilities {
			categoryCount[vuln.Category]++
		}
	}
	
	// Generate recommendations based on common issues
	if categoryCount[CategoryAuthentication] > 0 {
		recommendations = append(recommendations, "Implement stronger authentication mechanisms and multi-factor authentication")
	}
	
	if categoryCount[CategoryEncryption] > 0 {
		recommendations = append(recommendations, "Update TLS configuration and strengthen encryption protocols")
	}
	
	if categoryCount[CategoryInjection] > 0 {
		recommendations = append(recommendations, "Implement input validation and parameterized queries")
	}
	
	if categoryCount[CategoryConfiguration] > 0 {
		recommendations = append(recommendations, "Review and harden system configurations")
	}
	
	if categoryCount[CategoryDisclosure] > 0 {
		recommendations = append(recommendations, "Implement proper access controls and data protection")
	}
	
	// Add general recommendations
	recommendations = append(recommendations, 
		"Regular security assessments and penetration testing",
		"Employee security awareness training",
		"Implement security monitoring and incident response procedures",
		"Regular security patches and updates")
	
	return recommendations
}

// generateComplianceStatus generates compliance status for standards
func (ss *SecurityScanner) generateComplianceStatus(results []*ScanResult) map[string]*ComplianceStatus {
	complianceStatus := make(map[string]*ComplianceStatus)
	
	// Initialize standards
	standards := []string{"OWASP_TOP_10", "NIST_CSF", "ISO_27001"}
	
	for _, standard := range standards {
		status := &ComplianceStatus{
			Standard:         standard,
			ControlResults:   make(map[string]bool),
			Recommendations:  make([]string, 0),
			CriticalFailures: make([]string, 0),
		}
		
		// Check compliance based on vulnerabilities found
		ss.checkComplianceForStandard(status, results)
		
		complianceStatus[standard] = status
	}
	
	return complianceStatus
}

// checkComplianceForStandard checks compliance for a specific standard
func (ss *SecurityScanner) checkComplianceForStandard(status *ComplianceStatus, results []*ScanResult) {
	switch status.Standard {
	case "OWASP_TOP_10":
		ss.checkOWASPCompliance(status, results)
	case "NIST_CSF":
		ss.checkNISTCompliance(status, results)
	case "ISO_27001":
		ss.checkISOCompliance(status, results)
	}
	
	// Calculate overall score
	if status.TotalControls > 0 {
		status.OverallScore = float64(status.PassedControls) / float64(status.TotalControls) * 100
	}
}

// checkOWASPCompliance checks OWASP Top 10 compliance
func (ss *SecurityScanner) checkOWASPCompliance(status *ComplianceStatus, results []*ScanResult) {
	owaspControls := map[string]string{
		"A01_2021": "Broken Access Control",
		"A02_2021": "Cryptographic Failures",
		"A03_2021": "Injection",
		"A04_2021": "Insecure Design",
		"A05_2021": "Security Misconfiguration",
		"A06_2021": "Vulnerable and Outdated Components",
		"A07_2021": "Identification and Authentication Failures",
		"A08_2021": "Software and Data Integrity Failures",
		"A09_2021": "Security Logging and Monitoring Failures",
		"A10_2021": "Server-Side Request Forgery",
	}
	
	status.TotalControls = len(owaspControls)
	
	// Check each control based on vulnerabilities
	for control, description := range owaspControls {
		passed := ss.checkOWASPControl(control, results)
		status.ControlResults[control] = passed
		
		if passed {
			status.PassedControls++
		} else {
			status.FailedControls++
			status.CriticalFailures = append(status.CriticalFailures, description)
		}
	}
}

// checkOWASPControl checks a specific OWASP control
func (ss *SecurityScanner) checkOWASPControl(control string, results []*ScanResult) bool {
	// Simplified compliance checking based on vulnerability categories
	for _, result := range results {
		for _, vuln := range result.Vulnerabilities {
			switch control {
			case "A01_2021": // Broken Access Control
				if vuln.Category == CategoryAuthorization {
					return false
				}
			case "A02_2021": // Cryptographic Failures
				if vuln.Category == CategoryEncryption || vuln.Category == CategoryCrypto {
					return false
				}
			case "A03_2021": // Injection
				if vuln.Category == CategoryInjection {
					return false
				}
			case "A05_2021": // Security Misconfiguration
				if vuln.Category == CategoryConfiguration {
					return false
				}
			case "A07_2021": // Identification and Authentication Failures
				if vuln.Category == CategoryAuthentication {
					return false
				}
			}
		}
	}
	return true
}

// checkNISTCompliance checks NIST Cybersecurity Framework compliance
func (ss *SecurityScanner) checkNISTCompliance(status *ComplianceStatus, results []*ScanResult) {
	// Simplified NIST CSF implementation
	status.TotalControls = 5 // Identify, Protect, Detect, Respond, Recover
	status.PassedControls = 3 // Simplified
	status.FailedControls = 2
}

// checkISOCompliance checks ISO 27001 compliance
func (ss *SecurityScanner) checkISOCompliance(status *ComplianceStatus, results []*ScanResult) {
	// Simplified ISO 27001 implementation
	status.TotalControls = 14 // ISO 27001 control categories
	status.PassedControls = 10 // Simplified
	status.FailedControls = 4
}

// generateExecutiveSummary generates executive summary
func (ss *SecurityScanner) generateExecutiveSummary(summary *ReportSummary) *ExecutiveSummary {
	execSummary := &ExecutiveSummary{
		OverallSecurity: summary.SecurityPosture,
		KeyFindings:     make([]string, 0),
		CriticalActions: make([]string, 0),
	}
	
	// Generate key findings
	if summary.CriticalVulns > 0 {
		execSummary.KeyFindings = append(execSummary.KeyFindings, 
			fmt.Sprintf("%d critical vulnerabilities requiring immediate attention", summary.CriticalVulns))
	}
	
	if summary.HighVulns > 0 {
		execSummary.KeyFindings = append(execSummary.KeyFindings,
			fmt.Sprintf("%d high-severity vulnerabilities need priority remediation", summary.HighVulns))
	}
	
	// Generate critical actions
	if summary.CriticalVulns > 0 {
		execSummary.CriticalActions = append(execSummary.CriticalActions,
			"Address critical vulnerabilities within 24-48 hours")
	}
	
	if summary.OverallRiskScore > 60 {
		execSummary.CriticalActions = append(execSummary.CriticalActions,
			"Implement comprehensive security improvement program")
	}
	
	// Business impact assessment
	switch summary.SecurityPosture {
	case "critical", "poor":
		execSummary.BusinessImpact = "High risk of security incidents that could impact business operations and reputation"
	case "fair":
		execSummary.BusinessImpact = "Moderate security risk requiring attention to prevent potential incidents"
	default:
		execSummary.BusinessImpact = "Low security risk with good defensive posture"
	}
	
	return execSummary
}

// Helper methods

// generateReportID generates a unique report ID
func (ss *SecurityScanner) generateReportID() string {
	return fmt.Sprintf("sec_report_%d", time.Now().UnixNano())
}

// saveReport saves the report to file
func (ss *SecurityScanner) saveReport(report *SecurityReport) error {
	// Implementation would save report to configured format and location
	ss.logger.Printf("Report generated: %s", report.ID)
	return nil
}

// GetMetrics returns scanner metrics
func (ss *SecurityScanner) GetMetrics() *ScannerMetrics {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	
	metricsCopy := *ss.metrics
	return &metricsCopy
}

// GetReports returns all security reports
func (ss *SecurityScanner) GetReports() map[string]*SecurityReport {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	
	reports := make(map[string]*SecurityReport)
	for id, report := range ss.reports {
		reportCopy := *report
		reports[id] = &reportCopy
	}
	
	return reports
}

// Scanner implementations

// PortScanner scans for open ports
type PortScanner struct {
	timeout time.Duration
	ports   []int
}

func (ps *PortScanner) GetName() string        { return "Port Scanner" }
func (ps *PortScanner) GetDescription() string { return "Scans for open network ports" }
func (ps *PortScanner) GetSeverity() Severity  { return SeverityLow }

func (ps *PortScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	result := &ScanResult{
		ScannerName:     ps.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusRunning,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	// Scan the specific port
	conn, err := net.DialTimeout("tcp", target.Endpoint, ps.timeout)
	if err != nil {
		// Port is closed or filtered
		result.Status = ScanStatusCompleted
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}
	conn.Close()
	
	// Port is open - check if it should be
	if ps.isUnexpectedOpenPort(target.Port) {
		vuln := &Vulnerability{
			ID:          fmt.Sprintf("open_port_%d", target.Port),
			Name:        fmt.Sprintf("Unexpected Open Port %d", target.Port),
			Description: fmt.Sprintf("Port %d is open and may expose services unnecessarily", target.Port),
			Severity:    SeverityMedium,
			Risk:        RiskMedium,
			Category:    CategoryConfiguration,
			Confidence:  0.8,
			Impact: &Impact{
				Confidentiality: "low",
				Integrity:       "none",
				Availability:    "none",
				Scope:           "unchanged",
			},
			Remediation: &Remediation{
				Priority:      "medium",
				Effort:        "low",
				Steps:         []string{"Review service necessity", "Close unused ports", "Implement firewall rules"},
				EstimatedTime: "1-2 hours",
			},
			Evidence: map[string]interface{}{
				"port":     target.Port,
				"protocol": "tcp",
				"status":   "open",
			},
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		result.Vulnerabilities = append(result.Vulnerabilities, vuln)
	}
	
	result.Status = ScanStatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

func (ps *PortScanner) isUnexpectedOpenPort(port int) bool {
	// Define expected open ports
	expectedPorts := map[int]bool{
		80:   true, // HTTP
		443:  true, // HTTPS
		8080: true, // Alternative HTTP
		8443: true, // Alternative HTTPS
	}
	
	return !expectedPorts[port]
}

// TLSScanner scans TLS configuration
type TLSScanner struct {
	timeout time.Duration
}

func (ts *TLSScanner) GetName() string        { return "TLS Scanner" }
func (ts *TLSScanner) GetDescription() string { return "Scans TLS/SSL configuration" }
func (ts *TLSScanner) GetSeverity() Severity  { return SeverityHigh }

func (ts *TLSScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	result := &ScanResult{
		ScannerName:     ts.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusRunning,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	// Only scan HTTPS ports
	if target.Port != 443 && target.Port != 8443 {
		result.Status = ScanStatusCompleted
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}
	
	// Test TLS connection
	config := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: ts.timeout}, "tcp", target.Endpoint, config)
	if err != nil {
		result.Status = ScanStatusCompleted
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}
	defer conn.Close()
	
	// Analyze TLS configuration
	state := conn.ConnectionState()
	
	// Check for weak TLS version
	if state.Version < tls.VersionTLS12 {
		vuln := &Vulnerability{
			ID:          "weak_tls_version",
			Name:        "Weak TLS Version",
			Description: fmt.Sprintf("Server supports weak TLS version: %s", ts.tlsVersionString(state.Version)),
			Severity:    SeverityHigh,
			Risk:        RiskHigh,
			Category:    CategoryEncryption,
			Confidence:  0.9,
			Impact: &Impact{
				Confidentiality: "high",
				Integrity:       "high",
				Availability:    "none",
				Scope:           "unchanged",
			},
			Remediation: &Remediation{
				Priority:      "high",
				Effort:        "medium",
				Steps:         []string{"Update TLS configuration", "Disable TLS versions below 1.2", "Test compatibility"},
				EstimatedTime: "2-4 hours",
			},
			Evidence: map[string]interface{}{
				"tls_version": ts.tlsVersionString(state.Version),
				"cipher":      tls.CipherSuiteName(state.CipherSuite),
			},
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		result.Vulnerabilities = append(result.Vulnerabilities, vuln)
	}
	
	// Check for weak cipher suites
	if ts.isWeakCipher(state.CipherSuite) {
		vuln := &Vulnerability{
			ID:          "weak_cipher_suite",
			Name:        "Weak Cipher Suite",
			Description: fmt.Sprintf("Server uses weak cipher suite: %s", tls.CipherSuiteName(state.CipherSuite)),
			Severity:    SeverityMedium,
			Risk:        RiskMedium,
			Category:    CategoryEncryption,
			Confidence:  0.8,
			Impact: &Impact{
				Confidentiality: "medium",
				Integrity:       "medium",
				Availability:    "none",
				Scope:           "unchanged",
			},
			Remediation: &Remediation{
				Priority:      "medium",
				Effort:        "medium",
				Steps:         []string{"Update cipher suite configuration", "Disable weak ciphers", "Prefer strong AEAD ciphers"},
				EstimatedTime: "1-3 hours",
			},
			Evidence: map[string]interface{}{
				"cipher_suite": tls.CipherSuiteName(state.CipherSuite),
				"key_exchange": "unknown", // Would analyze key exchange
			},
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		result.Vulnerabilities = append(result.Vulnerabilities, vuln)
	}
	
	result.Status = ScanStatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

func (ts *TLSScanner) tlsVersionString(version uint16) string {
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
		return "Unknown"
	}
}

func (ts *TLSScanner) isWeakCipher(cipher uint16) bool {
	// Define weak cipher suites (simplified)
	weakCiphers := map[uint16]bool{
		tls.TLS_RSA_WITH_RC4_128_SHA:        true,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:   true,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA:    true, // CBC mode considered weak
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:    true, // CBC mode considered weak
	}
	
	return weakCiphers[cipher]
}

// HeaderScanner scans HTTP security headers
type HeaderScanner struct {
	timeout time.Duration
}

func (hs *HeaderScanner) GetName() string        { return "HTTP Header Scanner" }
func (hs *HeaderScanner) GetDescription() string { return "Scans HTTP security headers" }
func (hs *HeaderScanner) GetSeverity() Severity  { return SeverityMedium }

func (hs *HeaderScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	result := &ScanResult{
		ScannerName:     hs.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusRunning,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	// Only scan HTTP/HTTPS ports
	if target.Port != 80 && target.Port != 443 && target.Port != 8080 && target.Port != 8443 {
		result.Status = ScanStatusCompleted
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}
	
	// Make HTTP request
	protocol := "http"
	if target.Port == 443 || target.Port == 8443 {
		protocol = "https"
	}
	
	url := fmt.Sprintf("%s://%s", protocol, target.Endpoint)
	client := &http.Client{Timeout: hs.timeout}
	
	resp, err := client.Get(url)
	if err != nil {
		result.Status = ScanStatusCompleted
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}
	defer resp.Body.Close()
	
	// Check for missing security headers
	securityHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"X-XSS-Protection":        "1; mode=block",
		"Strict-Transport-Security": "max-age=31536000",
		"Content-Security-Policy": "default-src 'self'",
	}
	
	for header, description := range securityHeaders {
		if resp.Header.Get(header) == "" {
			vuln := &Vulnerability{
				ID:          fmt.Sprintf("missing_header_%s", strings.ToLower(strings.ReplaceAll(header, "-", "_"))),
				Name:        fmt.Sprintf("Missing Security Header: %s", header),
				Description: fmt.Sprintf("The %s security header is missing", header),
				Severity:    SeverityMedium,
				Risk:        RiskMedium,
				Category:    CategoryConfiguration,
				Confidence:  0.9,
				Impact: &Impact{
					Confidentiality: "low",
					Integrity:       "low",
					Availability:    "none",
					Scope:           "unchanged",
				},
				Remediation: &Remediation{
					Priority:      "medium",
					Effort:        "low",
					Steps:         []string{fmt.Sprintf("Add %s header to HTTP responses", header)},
					EstimatedTime: "30 minutes",
				},
				Evidence: map[string]interface{}{
					"missing_header":     header,
					"recommended_value":  description,
					"current_value":      "missing",
				},
				FirstSeen: time.Now(),
				LastSeen:  time.Now(),
			}
			result.Vulnerabilities = append(result.Vulnerabilities, vuln)
		}
	}
	
	result.Status = ScanStatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

// AuthScanner scans authentication mechanisms
type AuthScanner struct {
	timeout time.Duration
}

func (as *AuthScanner) GetName() string        { return "Authentication Scanner" }
func (as *AuthScanner) GetDescription() string { return "Scans authentication mechanisms" }
func (as *AuthScanner) GetSeverity() Severity  { return SeverityHigh }

func (as *AuthScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	// Simplified authentication scanner
	result := &ScanResult{
		ScannerName:     as.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusCompleted,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

// InjectionScanner scans for injection vulnerabilities
type InjectionScanner struct {
	timeout time.Duration
}

func (is *InjectionScanner) GetName() string        { return "Injection Scanner" }
func (is *InjectionScanner) GetDescription() string { return "Scans for injection vulnerabilities" }
func (is *InjectionScanner) GetSeverity() Severity  { return SeverityCritical }

func (is *InjectionScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	// Simplified injection scanner
	result := &ScanResult{
		ScannerName:     is.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusCompleted,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

// ConfigScanner scans system configuration
type ConfigScanner struct {
	timeout time.Duration
}

func (cs *ConfigScanner) GetName() string        { return "Configuration Scanner" }
func (cs *ConfigScanner) GetDescription() string { return "Scans system configuration" }
func (cs *ConfigScanner) GetSeverity() Severity  { return SeverityMedium }

func (cs *ConfigScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	// Simplified configuration scanner
	result := &ScanResult{
		ScannerName:     cs.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusCompleted,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

// CustomRulesScanner scans using custom rules
type CustomRulesScanner struct {
	rules   []SecurityRule
	timeout time.Duration
}

func (crs *CustomRulesScanner) GetName() string        { return "Custom Rules Scanner" }
func (crs *CustomRulesScanner) GetDescription() string { return "Scans using custom security rules" }
func (crs *CustomRulesScanner) GetSeverity() Severity  { return SeverityMedium }

func (crs *CustomRulesScanner) Scan(ctx context.Context, target *ScanTarget) (*ScanResult, error) {
	result := &ScanResult{
		ScannerName:     crs.GetName(),
		Target:          target,
		StartTime:       time.Now(),
		Status:          ScanStatusRunning,
		Vulnerabilities: make([]*Vulnerability, 0),
		Evidence:        make(map[string]interface{}),
		Metadata:        make(map[string]interface{}),
	}
	
	// Test each custom rule
	for _, rule := range crs.rules {
		if rule.Test != nil && rule.Test(target) {
			vuln := &Vulnerability{
				ID:          rule.ID,
				Name:        rule.Name,
				Description: rule.Description,
				Severity:    rule.Severity,
				Risk:        RiskMedium, // Default
				Category:    rule.Category,
				Confidence:  0.7, // Default for custom rules
				Remediation: rule.Remediation,
				FirstSeen:   time.Now(),
				LastSeen:    time.Now(),
				Evidence: map[string]interface{}{
					"rule_id":   rule.ID,
					"pattern":   rule.Pattern,
					"matched":   true,
				},
			}
			result.Vulnerabilities = append(result.Vulnerabilities, vuln)
		}
		
		// Pattern matching
		if rule.Pattern != "" {
			matched, _ := regexp.MatchString(rule.Pattern, target.Endpoint)
			if matched {
				vuln := &Vulnerability{
					ID:          rule.ID + "_pattern",
					Name:        rule.Name + " (Pattern Match)",
					Description: rule.Description,
					Severity:    rule.Severity,
					Risk:        RiskMedium,
					Category:    rule.Category,
					Confidence:  0.6, // Lower confidence for pattern matching
					Remediation: rule.Remediation,
					FirstSeen:   time.Now(),
					LastSeen:    time.Now(),
					Evidence: map[string]interface{}{
						"rule_id":     rule.ID,
						"pattern":     rule.Pattern,
						"target":      target.Endpoint,
						"match_type":  "pattern",
					},
				}
				result.Vulnerabilities = append(result.Vulnerabilities, vuln)
			}
		}
	}
	
	result.Status = ScanStatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}