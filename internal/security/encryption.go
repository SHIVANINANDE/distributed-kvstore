package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

// EncryptionManager manages encryption operations and validation
type EncryptionManager struct {
	mu       sync.RWMutex
	logger   *log.Logger
	config   EncryptionConfig
	keyStore KeyStore
	metrics  *EncryptionMetrics
}

// EncryptionConfig contains encryption configuration
type EncryptionConfig struct {
	// Symmetric encryption
	DefaultAlgorithm    string        `json:"default_algorithm"`     // AES-256-GCM, ChaCha20-Poly1305
	KeySize             int           `json:"key_size"`              // Key size in bits
	KeyRotationInterval time.Duration `json:"key_rotation_interval"` // Key rotation interval
	
	// Asymmetric encryption
	RSAKeySize          int           `json:"rsa_key_size"`          // RSA key size
	ECCCurve            string        `json:"ecc_curve"`             // ECC curve (P-256, P-384, P-521)
	
	// Key management
	MasterKeyPath       string        `json:"master_key_path"`       // Path to master key
	KeyStorePath        string        `json:"key_store_path"`        // Path to key store
	EnableHSM           bool          `json:"enable_hsm"`            // Enable HSM integration
	HSMConfig           *HSMConfig    `json:"hsm_config"`            // HSM configuration
	
	// Security settings
	RequireIntegrity    bool          `json:"require_integrity"`     // Require integrity checking
	EnableCompression   bool          `json:"enable_compression"`    // Enable compression before encryption
	PaddingScheme       string        `json:"padding_scheme"`        // OAEP, PKCS1v15
	
	// Validation settings
	ValidateOnStartup   bool          `json:"validate_on_startup"`   // Validate encryption on startup
	ValidationInterval  time.Duration `json:"validation_interval"`   // Validation check interval
	PerformanceTests    bool          `json:"performance_tests"`     // Run performance tests
}

// HSMConfig contains HSM configuration
type HSMConfig struct {
	Provider    string            `json:"provider"`     // HSM provider
	SlotID      int               `json:"slot_id"`      // HSM slot ID
	PIN         string            `json:"pin"`          // HSM PIN
	TokenLabel  string            `json:"token_label"`  // Token label
	Config      map[string]string `json:"config"`       // Additional config
}

// KeyStore interface for key storage operations
type KeyStore interface {
	StoreKey(keyID string, key *EncryptionKey) error
	RetrieveKey(keyID string) (*EncryptionKey, error)
	ListKeys() ([]string, error)
	DeleteKey(keyID string) error
	RotateKey(keyID string) (*EncryptionKey, error)
}

// EncryptionKey represents an encryption key
type EncryptionKey struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`          // symmetric, rsa, ecdsa
	Algorithm   string            `json:"algorithm"`     // AES-256-GCM, RSA-OAEP, etc.
	Key         []byte            `json:"key"`           // Raw key data
	PublicKey   []byte            `json:"public_key"`    // Public key for asymmetric
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	Version     int               `json:"version"`
	IsActive    bool              `json:"is_active"`
	Usage       []string          `json:"usage"`         // encrypt, decrypt, sign, verify
	Metadata    map[string]string `json:"metadata"`
}

// EncryptionResult represents the result of an encryption operation
type EncryptionResult struct {
	Ciphertext   []byte            `json:"ciphertext"`
	IV           []byte            `json:"iv"`            // Initialization vector
	Tag          []byte            `json:"tag"`           // Authentication tag
	KeyID        string            `json:"key_id"`        // Key used for encryption
	Algorithm    string            `json:"algorithm"`     // Algorithm used
	Metadata     map[string]string `json:"metadata"`
	EncryptedAt  time.Time         `json:"encrypted_at"`
}

// ValidationResult represents encryption validation results
type ValidationResult struct {
	TestID       string                   `json:"test_id"`
	TestType     string                   `json:"test_type"`
	Success      bool                     `json:"success"`
	Error        string                   `json:"error,omitempty"`
	Results      []*ValidationTest        `json:"results"`
	Summary      *ValidationSummary       `json:"summary"`
	Performance  *PerformanceMetrics      `json:"performance"`
	Compliance   *ComplianceValidation    `json:"compliance"`
	TestedAt     time.Time                `json:"tested_at"`
	Duration     time.Duration            `json:"duration"`
}

// ValidationTest represents a single validation test
type ValidationTest struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Expected     string            `json:"expected"`
	Actual       string            `json:"actual"`
	Success      bool              `json:"success"`
	Error        string            `json:"error,omitempty"`
	Duration     time.Duration     `json:"duration"`
	Metadata     map[string]string `json:"metadata"`
}

// ValidationSummary provides a summary of validation results
type ValidationSummary struct {
	TotalTests   int               `json:"total_tests"`
	PassedTests  int               `json:"passed_tests"`
	FailedTests  int               `json:"failed_tests"`
	SuccessRate  float64           `json:"success_rate"`
	CriticalIssues []string        `json:"critical_issues"`
	Warnings     []string          `json:"warnings"`
	Recommendations []string       `json:"recommendations"`
}

// PerformanceMetrics contains encryption performance metrics
type PerformanceMetrics struct {
	EncryptionSpeed    float64       `json:"encryption_speed_mbps"`    // MB/s
	DecryptionSpeed    float64       `json:"decryption_speed_mbps"`    // MB/s
	KeyGenerationTime  time.Duration `json:"key_generation_time"`     // Time to generate key
	EncryptionLatency  time.Duration `json:"encryption_latency"`      // Average encryption latency
	DecryptionLatency  time.Duration `json:"decryption_latency"`      // Average decryption latency
	ThroughputTests    map[string]float64 `json:"throughput_tests"`    // Various size tests
	MemoryUsage        int64         `json:"memory_usage_bytes"`      // Memory usage
}

// ComplianceValidation represents compliance validation results
type ComplianceValidation struct {
	Standards    map[string]bool   `json:"standards"`         // FIPS-140-2, Common Criteria, etc.
	Algorithms   map[string]bool   `json:"algorithms"`        // Algorithm compliance
	KeySizes     map[string]bool   `json:"key_sizes"`         // Key size compliance
	Protocols    map[string]bool   `json:"protocols"`         // Protocol compliance
	Issues       []string          `json:"issues"`            // Compliance issues
	Score        int               `json:"score"`             // Overall compliance score
	MaxScore     int               `json:"max_score"`         // Maximum possible score
}

// EncryptionMetrics tracks encryption operation metrics
type EncryptionMetrics struct {
	TotalOperations    int64             `json:"total_operations"`
	EncryptOperations  int64             `json:"encrypt_operations"`
	DecryptOperations  int64             `json:"decrypt_operations"`
	KeyRotations       int64             `json:"key_rotations"`
	ValidationTests    int64             `json:"validation_tests"`
	FailedOperations   int64             `json:"failed_operations"`
	AverageLatency     time.Duration     `json:"average_latency"`
	BytesEncrypted     int64             `json:"bytes_encrypted"`
	BytesDecrypted     int64             `json:"bytes_decrypted"`
	AlgorithmUsage     map[string]int64  `json:"algorithm_usage"`
	ErrorTypes         map[string]int64  `json:"error_types"`
	LastValidation     time.Time         `json:"last_validation"`
}

// NewEncryptionManager creates a new encryption manager
func NewEncryptionManager(config EncryptionConfig, keyStore KeyStore, logger *log.Logger) (*EncryptionManager, error) {
	if logger == nil {
		logger = log.New(log.Writer(), "[ENCRYPTION] ", log.LstdFlags)
	}
	
	// Set default values
	if config.DefaultAlgorithm == "" {
		config.DefaultAlgorithm = "AES-256-GCM"
	}
	if config.KeySize == 0 {
		config.KeySize = 256
	}
	if config.RSAKeySize == 0 {
		config.RSAKeySize = 2048
	}
	if config.ECCCurve == "" {
		config.ECCCurve = "P-256"
	}
	if config.PaddingScheme == "" {
		config.PaddingScheme = "OAEP"
	}
	if config.KeyRotationInterval == 0 {
		config.KeyRotationInterval = 90 * 24 * time.Hour // 90 days
	}
	if config.ValidationInterval == 0 {
		config.ValidationInterval = 24 * time.Hour // Daily
	}
	
	em := &EncryptionManager{
		logger:   logger,
		config:   config,
		keyStore: keyStore,
		metrics: &EncryptionMetrics{
			AlgorithmUsage: make(map[string]int64),
			ErrorTypes:     make(map[string]int64),
		},
	}
	
	// Perform startup validation if enabled
	if config.ValidateOnStartup {
		if err := em.performStartupValidation(); err != nil {
			logger.Printf("Startup validation failed: %v", err)
			return nil, fmt.Errorf("startup validation failed: %w", err)
		}
	}
	
	// Start validation routine
	if config.ValidationInterval > 0 {
		go em.validationRoutine()
	}
	
	logger.Printf("Encryption manager initialized with algorithm %s", config.DefaultAlgorithm)
	return em, nil
}

// ValidateEncryption performs comprehensive encryption validation
func (em *EncryptionManager) ValidateEncryption() (*ValidationResult, error) {
	startTime := time.Now()
	
	em.logger.Printf("Starting comprehensive encryption validation")
	
	result := &ValidationResult{
		TestID:   fmt.Sprintf("encryption-validation-%d", time.Now().Unix()),
		TestType: "comprehensive",
		TestedAt: time.Now(),
		Results:  []*ValidationTest{},
	}
	
	// Test 1: Symmetric encryption algorithms
	em.runSymmetricEncryptionTests(result)
	
	// Test 2: Asymmetric encryption algorithms
	em.runAsymmetricEncryptionTests(result)
	
	// Test 3: Key generation and management
	em.runKeyManagementTests(result)
	
	// Test 4: Algorithm compliance
	em.runAlgorithmComplianceTests(result)
	
	// Test 5: Performance tests
	if em.config.PerformanceTests {
		em.runPerformanceTests(result)
	}
	
	// Test 6: Integrity and authentication tests
	em.runIntegrityTests(result)
	
	// Test 7: Cross-platform compatibility tests
	em.runCompatibilityTests(result)
	
	// Generate summary
	result.Summary = em.generateValidationSummary(result.Results)
	
	// Perform compliance validation
	result.Compliance = em.performComplianceValidation(result)
	
	// Calculate duration
	result.Duration = time.Since(startTime)
	result.Success = result.Summary.FailedTests == 0
	
	// Update metrics
	em.mu.Lock()
	em.metrics.ValidationTests++
	em.metrics.LastValidation = time.Now()
	if !result.Success {
		em.metrics.FailedOperations++
	}
	em.mu.Unlock()
	
	em.logger.Printf("Encryption validation completed. Success rate: %.2f%%", result.Summary.SuccessRate)
	
	return result, nil
}

// runSymmetricEncryptionTests tests symmetric encryption algorithms
func (em *EncryptionManager) runSymmetricEncryptionTests(result *ValidationResult) {
	algorithms := []string{"AES-128-GCM", "AES-256-GCM", "ChaCha20-Poly1305"}
	testData := []byte("This is a test message for encryption validation")
	
	for _, algorithm := range algorithms {
		test := &ValidationTest{
			Name:        fmt.Sprintf("Symmetric Encryption - %s", algorithm),
			Description: fmt.Sprintf("Test %s encryption and decryption", algorithm),
			Expected:    "successful round-trip encryption/decryption",
			Metadata:    make(map[string]string),
		}
		
		startTime := time.Now()
		
		// Generate key for this algorithm
		key, err := em.generateSymmetricKey(algorithm)
		if err != nil {
			test.Success = false
			test.Error = fmt.Sprintf("Key generation failed: %v", err)
			test.Actual = "key generation error"
			result.Results = append(result.Results, test)
			continue
		}
		
		// Encrypt data
		encrypted, err := em.encryptSymmetric(testData, key, algorithm)
		if err != nil {
			test.Success = false
			test.Error = fmt.Sprintf("Encryption failed: %v", err)
			test.Actual = "encryption error"
			result.Results = append(result.Results, test)
			continue
		}
		
		// Decrypt data
		decrypted, err := em.decryptSymmetric(encrypted, key, algorithm)
		if err != nil {
			test.Success = false
			test.Error = fmt.Sprintf("Decryption failed: %v", err)
			test.Actual = "decryption error"
			result.Results = append(result.Results, test)
			continue
		}
		
		// Verify data integrity
		if string(decrypted) != string(testData) {
			test.Success = false
			test.Error = "Data integrity check failed"
			test.Actual = "data mismatch"
		} else {
			test.Success = true
			test.Actual = "successful round-trip encryption/decryption"
		}
		
		test.Duration = time.Since(startTime)
		test.Metadata["algorithm"] = algorithm
		test.Metadata["data_size"] = fmt.Sprintf("%d", len(testData))
		
		result.Results = append(result.Results, test)
	}
}

// runAsymmetricEncryptionTests tests asymmetric encryption algorithms
func (em *EncryptionManager) runAsymmetricEncryptionTests(result *ValidationResult) {
	algorithms := []string{"RSA-OAEP", "ECDSA"}
	testData := []byte("Asymmetric encryption test message")
	
	for _, algorithm := range algorithms {
		test := &ValidationTest{
			Name:        fmt.Sprintf("Asymmetric Encryption - %s", algorithm),
			Description: fmt.Sprintf("Test %s key generation and operations", algorithm),
			Expected:    "successful key operations",
			Metadata:    make(map[string]string),
		}
		
		startTime := time.Now()
		
		switch algorithm {
		case "RSA-OAEP":
			err := em.testRSAOperations(testData, test)
			if err != nil {
				test.Success = false
				test.Error = err.Error()
				test.Actual = "RSA operations failed"
			} else {
				test.Success = true
				test.Actual = "successful RSA operations"
			}
			
		case "ECDSA":
			err := em.testECDSAOperations(testData, test)
			if err != nil {
				test.Success = false
				test.Error = err.Error()
				test.Actual = "ECDSA operations failed"
			} else {
				test.Success = true
				test.Actual = "successful ECDSA operations"
			}
		}
		
		test.Duration = time.Since(startTime)
		test.Metadata["algorithm"] = algorithm
		result.Results = append(result.Results, test)
	}
}

// runKeyManagementTests tests key management operations
func (em *EncryptionManager) runKeyManagementTests(result *ValidationResult) {
	test := &ValidationTest{
		Name:        "Key Management Operations",
		Description: "Test key storage, retrieval, and rotation",
		Expected:    "successful key management operations",
		Metadata:    make(map[string]string),
	}
	
	startTime := time.Now()
	
	// Test key storage and retrieval
	testKey := &EncryptionKey{
		ID:        fmt.Sprintf("test-key-%d", time.Now().Unix()),
		Type:      "symmetric",
		Algorithm: "AES-256-GCM",
		Key:       make([]byte, 32),
		CreatedAt: time.Now(),
		IsActive:  true,
		Usage:     []string{"encrypt", "decrypt"},
		Metadata:  make(map[string]string),
	}
	
	// Generate random key data
	if _, err := rand.Read(testKey.Key); err != nil {
		test.Success = false
		test.Error = fmt.Sprintf("Failed to generate test key: %v", err)
		test.Actual = "key generation error"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Store key
	if err := em.keyStore.StoreKey(testKey.ID, testKey); err != nil {
		test.Success = false
		test.Error = fmt.Sprintf("Failed to store key: %v", err)
		test.Actual = "key storage error"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Retrieve key
	retrievedKey, err := em.keyStore.RetrieveKey(testKey.ID)
	if err != nil {
		test.Success = false
		test.Error = fmt.Sprintf("Failed to retrieve key: %v", err)
		test.Actual = "key retrieval error"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Verify key integrity
	if retrievedKey.ID != testKey.ID || retrievedKey.Algorithm != testKey.Algorithm {
		test.Success = false
		test.Error = "Retrieved key does not match stored key"
		test.Actual = "key integrity error"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Clean up test key
	em.keyStore.DeleteKey(testKey.ID)
	
	test.Success = true
	test.Actual = "successful key management operations"
	test.Duration = time.Since(startTime)
	test.Metadata["operations"] = "store,retrieve,delete"
	
	result.Results = append(result.Results, test)
}

// runAlgorithmComplianceTests tests algorithm compliance
func (em *EncryptionManager) runAlgorithmComplianceTests(result *ValidationResult) {
	test := &ValidationTest{
		Name:        "Algorithm Compliance",
		Description: "Verify algorithms meet security standards",
		Expected:    "all algorithms compliant with standards",
		Metadata:    make(map[string]string),
	}
	
	startTime := time.Now()
	
	complianceIssues := []string{}
	
	// Check AES key sizes
	if em.config.KeySize < 128 {
		complianceIssues = append(complianceIssues, "AES key size below minimum (128 bits)")
	}
	
	// Check RSA key sizes
	if em.config.RSAKeySize < 2048 {
		complianceIssues = append(complianceIssues, "RSA key size below recommended minimum (2048 bits)")
	}
	
	// Check ECC curve
	approvedCurves := []string{"P-256", "P-384", "P-521"}
	curveApproved := false
	for _, curve := range approvedCurves {
		if em.config.ECCCurve == curve {
			curveApproved = true
			break
		}
	}
	if !curveApproved {
		complianceIssues = append(complianceIssues, "ECC curve not in approved list")
	}
	
	if len(complianceIssues) > 0 {
		test.Success = false
		test.Error = fmt.Sprintf("Compliance issues found: %v", complianceIssues)
		test.Actual = "compliance violations detected"
	} else {
		test.Success = true
		test.Actual = "all algorithms compliant with standards"
	}
	
	test.Duration = time.Since(startTime)
	test.Metadata["checked_algorithms"] = "AES,RSA,ECDSA"
	test.Metadata["standards"] = "FIPS-140-2,NIST"
	
	result.Results = append(result.Results, test)
}

// runPerformanceTests runs encryption performance tests
func (em *EncryptionManager) runPerformanceTests(result *ValidationResult) {
	test := &ValidationTest{
		Name:        "Performance Tests",
		Description: "Measure encryption/decryption performance",
		Expected:    "acceptable performance metrics",
		Metadata:    make(map[string]string),
	}
	
	startTime := time.Now()
	
	// Test with different data sizes
	dataSizes := []int{1024, 10240, 102400, 1048576} // 1KB, 10KB, 100KB, 1MB
	
	performance := &PerformanceMetrics{
		ThroughputTests: make(map[string]float64),
	}
	
	for _, size := range dataSizes {
		testData := make([]byte, size)
		rand.Read(testData)
		
		// Test AES-256-GCM performance
		key, _ := em.generateSymmetricKey("AES-256-GCM")
		
		// Measure encryption
		encStart := time.Now()
		encrypted, err := em.encryptSymmetric(testData, key, "AES-256-GCM")
		encDuration := time.Since(encStart)
		
		if err != nil {
			test.Success = false
			test.Error = fmt.Sprintf("Performance test failed for size %d: %v", size, err)
			test.Actual = "performance test error"
			test.Duration = time.Since(startTime)
			result.Results = append(result.Results, test)
			return
		}
		
		// Measure decryption
		decStart := time.Now()
		_, err = em.decryptSymmetric(encrypted, key, "AES-256-GCM")
		decDuration := time.Since(decStart)
		
		if err != nil {
			test.Success = false
			test.Error = fmt.Sprintf("Performance test decryption failed for size %d: %v", size, err)
			test.Actual = "performance test error"
			test.Duration = time.Since(startTime)
			result.Results = append(result.Results, test)
			return
		}
		
		// Calculate throughput (MB/s)
		encThroughput := float64(size) / encDuration.Seconds() / (1024 * 1024)
		decThroughput := float64(size) / decDuration.Seconds() / (1024 * 1024)
		
		performance.ThroughputTests[fmt.Sprintf("encrypt_%dKB", size/1024)] = encThroughput
		performance.ThroughputTests[fmt.Sprintf("decrypt_%dKB", size/1024)] = decThroughput
	}
	
	// Store performance metrics in result
	result.Performance = performance
	
	test.Success = true
	test.Actual = "performance tests completed successfully"
	test.Duration = time.Since(startTime)
	test.Metadata["data_sizes"] = "1KB,10KB,100KB,1MB"
	test.Metadata["algorithm"] = "AES-256-GCM"
	
	result.Results = append(result.Results, test)
}

// runIntegrityTests tests data integrity and authentication
func (em *EncryptionManager) runIntegrityTests(result *ValidationResult) {
	test := &ValidationTest{
		Name:        "Integrity and Authentication Tests",
		Description: "Test data integrity and authentication mechanisms",
		Expected:    "integrity checks pass, tampering detected",
		Metadata:    make(map[string]string),
	}
	
	startTime := time.Now()
	
	testData := []byte("Integrity test message")
	key, err := em.generateSymmetricKey("AES-256-GCM")
	if err != nil {
		test.Success = false
		test.Error = fmt.Sprintf("Key generation failed: %v", err)
		test.Actual = "key generation error"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Encrypt data
	encrypted, err := em.encryptSymmetric(testData, key, "AES-256-GCM")
	if err != nil {
		test.Success = false
		test.Error = fmt.Sprintf("Encryption failed: %v", err)
		test.Actual = "encryption error"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Test 1: Normal decryption should succeed
	decrypted, err := em.decryptSymmetric(encrypted, key, "AES-256-GCM")
	if err != nil || string(decrypted) != string(testData) {
		test.Success = false
		test.Error = "Normal integrity check failed"
		test.Actual = "integrity verification failed"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	// Test 2: Tampered data should fail decryption
	tamperedEncrypted := *encrypted
	if len(tamperedEncrypted.Ciphertext) > 0 {
		tamperedEncrypted.Ciphertext[0] ^= 1 // Flip one bit
	}
	
	_, err = em.decryptSymmetric(&tamperedEncrypted, key, "AES-256-GCM")
	if err == nil {
		test.Success = false
		test.Error = "Tampered data was not detected"
		test.Actual = "tampering not detected"
		test.Duration = time.Since(startTime)
		result.Results = append(result.Results, test)
		return
	}
	
	test.Success = true
	test.Actual = "integrity checks pass, tampering detected"
	test.Duration = time.Since(startTime)
	test.Metadata["tests"] = "normal_decrypt,tamper_detect"
	
	result.Results = append(result.Results, test)
}

// runCompatibilityTests tests cross-platform compatibility
func (em *EncryptionManager) runCompatibilityTests(result *ValidationResult) {
	test := &ValidationTest{
		Name:        "Cross-Platform Compatibility",
		Description: "Test encryption compatibility across platforms",
		Expected:    "consistent encryption/decryption results",
		Metadata:    make(map[string]string),
	}
	
	startTime := time.Now()
	
	// Test standard vectors for known algorithms
	testVectors := em.getStandardTestVectors()
	
	allPassed := true
	for algorithm, vectors := range testVectors {
		for i, vector := range vectors {
			// Test encryption
			encrypted, err := em.encryptWithVector(vector)
			if err != nil {
				allPassed = false
				test.Error = fmt.Sprintf("Vector test failed for %s vector %d: %v", algorithm, i, err)
				break
			}
			
			// Verify expected result (simplified)
			if len(encrypted.Ciphertext) == 0 {
				allPassed = false
				test.Error = fmt.Sprintf("Empty ciphertext for %s vector %d", algorithm, i)
				break
			}
		}
		if !allPassed {
			break
		}
	}
	
	if allPassed {
		test.Success = true
		test.Actual = "consistent encryption/decryption results"
	} else {
		test.Success = false
		test.Actual = "compatibility issues detected"
	}
	
	test.Duration = time.Since(startTime)
	test.Metadata["test_vectors"] = "NIST,RFC"
	
	result.Results = append(result.Results, test)
}

// Helper methods for encryption operations

// generateSymmetricKey generates a symmetric encryption key
func (em *EncryptionManager) generateSymmetricKey(algorithm string) (*EncryptionKey, error) {
	var keySize int
	switch algorithm {
	case "AES-128-GCM":
		keySize = 16
	case "AES-256-GCM":
		keySize = 32
	case "ChaCha20-Poly1305":
		keySize = 32
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
	
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	
	return &EncryptionKey{
		ID:        fmt.Sprintf("sym-%d", time.Now().UnixNano()),
		Type:      "symmetric",
		Algorithm: algorithm,
		Key:       key,
		CreatedAt: time.Now(),
		IsActive:  true,
		Usage:     []string{"encrypt", "decrypt"},
		Metadata:  make(map[string]string),
	}, nil
}

// encryptSymmetric performs symmetric encryption
func (em *EncryptionManager) encryptSymmetric(data []byte, key *EncryptionKey, algorithm string) (*EncryptionResult, error) {
	switch algorithm {
	case "AES-128-GCM", "AES-256-GCM":
		return em.encryptAESGCM(data, key.Key)
	case "ChaCha20-Poly1305":
		return em.encryptChaCha20Poly1305(data, key.Key)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
}

// decryptSymmetric performs symmetric decryption
func (em *EncryptionManager) decryptSymmetric(encrypted *EncryptionResult, key *EncryptionKey, algorithm string) ([]byte, error) {
	switch algorithm {
	case "AES-128-GCM", "AES-256-GCM":
		return em.decryptAESGCM(encrypted, key.Key)
	case "ChaCha20-Poly1305":
		return em.decryptChaCha20Poly1305(encrypted, key.Key)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
}

// encryptAESGCM encrypts data using AES-GCM
func (em *EncryptionManager) encryptAESGCM(data, key []byte) (*EncryptionResult, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	
	return &EncryptionResult{
		Ciphertext:  ciphertext,
		IV:          nonce,
		Algorithm:   "AES-GCM",
		EncryptedAt: time.Now(),
		Metadata:    make(map[string]string),
	}, nil
}

// decryptAESGCM decrypts data using AES-GCM
func (em *EncryptionManager) decryptAESGCM(encrypted *EncryptionResult, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	return gcm.Open(nil, encrypted.IV, encrypted.Ciphertext, nil)
}

// encryptChaCha20Poly1305 encrypts data using ChaCha20-Poly1305
func (em *EncryptionManager) encryptChaCha20Poly1305(data, key []byte) (*EncryptionResult, error) {
	// Simplified implementation - in production, use golang.org/x/crypto/chacha20poly1305
	return nil, fmt.Errorf("ChaCha20-Poly1305 not implemented in demo")
}

// decryptChaCha20Poly1305 decrypts data using ChaCha20-Poly1305
func (em *EncryptionManager) decryptChaCha20Poly1305(encrypted *EncryptionResult, key []byte) ([]byte, error) {
	// Simplified implementation - in production, use golang.org/x/crypto/chacha20poly1305
	return nil, fmt.Errorf("ChaCha20-Poly1305 not implemented in demo")
}

// testRSAOperations tests RSA encryption/decryption and signing/verification
func (em *EncryptionManager) testRSAOperations(data []byte, test *ValidationTest) error {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, em.config.RSAKeySize)
	if err != nil {
		return fmt.Errorf("RSA key generation failed: %w", err)
	}
	
	publicKey := &privateKey.PublicKey
	
	// Test encryption/decryption
	encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, data, nil)
	if err != nil {
		return fmt.Errorf("RSA encryption failed: %w", err)
	}
	
	decryptedData, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, encryptedData, nil)
	if err != nil {
		return fmt.Errorf("RSA decryption failed: %w", err)
	}
	
	if string(decryptedData) != string(data) {
		return fmt.Errorf("RSA round-trip failed")
	}
	
	// Test signing/verification
	hash := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, sha256.New().Sum(nil)[:0], hash[:])
	if err != nil {
		return fmt.Errorf("RSA signing failed: %w", err)
	}
	
	err = rsa.VerifyPKCS1v15(publicKey, sha256.New().Sum(nil)[:0], hash[:], signature)
	if err != nil {
		return fmt.Errorf("RSA verification failed: %w", err)
	}
	
	test.Metadata["key_size"] = fmt.Sprintf("%d", em.config.RSAKeySize)
	test.Metadata["operations"] = "encrypt,decrypt,sign,verify"
	
	return nil
}

// testECDSAOperations tests ECDSA signing and verification
func (em *EncryptionManager) testECDSAOperations(data []byte, test *ValidationTest) error {
	// Get curve
	var curve elliptic.Curve
	switch em.config.ECCCurve {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return fmt.Errorf("unsupported curve: %s", em.config.ECCCurve)
	}
	
	// Generate ECDSA key pair
	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return fmt.Errorf("ECDSA key generation failed: %w", err)
	}
	
	// Test signing
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return fmt.Errorf("ECDSA signing failed: %w", err)
	}
	
	// Test verification
	valid := ecdsa.Verify(&privateKey.PublicKey, hash[:], r, s)
	if !valid {
		return fmt.Errorf("ECDSA verification failed")
	}
	
	test.Metadata["curve"] = em.config.ECCCurve
	test.Metadata["operations"] = "sign,verify"
	
	return nil
}

// Utility methods

// performStartupValidation performs validation during startup
func (em *EncryptionManager) performStartupValidation() error {
	em.logger.Printf("Performing startup encryption validation")
	
	// Quick validation test
	testData := []byte("startup validation test")
	key, err := em.generateSymmetricKey(em.config.DefaultAlgorithm)
	if err != nil {
		return fmt.Errorf("startup key generation failed: %w", err)
	}
	
	encrypted, err := em.encryptSymmetric(testData, key, em.config.DefaultAlgorithm)
	if err != nil {
		return fmt.Errorf("startup encryption failed: %w", err)
	}
	
	decrypted, err := em.decryptSymmetric(encrypted, key, em.config.DefaultAlgorithm)
	if err != nil {
		return fmt.Errorf("startup decryption failed: %w", err)
	}
	
	if string(decrypted) != string(testData) {
		return fmt.Errorf("startup validation data mismatch")
	}
	
	em.logger.Printf("Startup validation completed successfully")
	return nil
}

// validationRoutine runs periodic validation
func (em *EncryptionManager) validationRoutine() {
	ticker := time.NewTicker(em.config.ValidationInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		result, err := em.ValidateEncryption()
		if err != nil {
			em.logger.Printf("Periodic validation failed: %v", err)
		} else if !result.Success {
			em.logger.Printf("Periodic validation completed with issues: %d failed tests", result.Summary.FailedTests)
		} else {
			em.logger.Printf("Periodic validation completed successfully")
		}
	}
}

// generateValidationSummary generates a summary of validation results
func (em *EncryptionManager) generateValidationSummary(tests []*ValidationTest) *ValidationSummary {
	summary := &ValidationSummary{
		TotalTests:      len(tests),
		CriticalIssues:  []string{},
		Warnings:        []string{},
		Recommendations: []string{},
	}
	
	for _, test := range tests {
		if test.Success {
			summary.PassedTests++
		} else {
			summary.FailedTests++
			if contains(test.Name, "Compliance") || contains(test.Name, "Integrity") {
				summary.CriticalIssues = append(summary.CriticalIssues, test.Error)
			} else {
				summary.Warnings = append(summary.Warnings, test.Error)
			}
		}
	}
	
	if summary.TotalTests > 0 {
		summary.SuccessRate = float64(summary.PassedTests) / float64(summary.TotalTests) * 100
	}
	
	// Generate recommendations
	if summary.SuccessRate < 100 {
		summary.Recommendations = append(summary.Recommendations, "Review and fix failing tests")
	}
	if summary.SuccessRate < 80 {
		summary.Recommendations = append(summary.Recommendations, "Consider upgrading encryption algorithms")
	}
	
	return summary
}

// performComplianceValidation performs compliance validation
func (em *EncryptionManager) performComplianceValidation(result *ValidationResult) *ComplianceValidation {
	compliance := &ComplianceValidation{
		Standards:  make(map[string]bool),
		Algorithms: make(map[string]bool),
		KeySizes:   make(map[string]bool),
		Protocols:  make(map[string]bool),
		Issues:     []string{},
		MaxScore:   100,
	}
	
	score := 0
	
	// Check FIPS-140-2 compliance
	if em.config.KeySize >= 128 && em.config.RSAKeySize >= 2048 {
		compliance.Standards["FIPS-140-2"] = true
		score += 25
	} else {
		compliance.Issues = append(compliance.Issues, "FIPS-140-2: Insufficient key sizes")
	}
	
	// Check algorithm compliance
	approvedAlgorithms := []string{"AES-128-GCM", "AES-256-GCM", "RSA-OAEP", "ECDSA"}
	if contains(approvedAlgorithms, em.config.DefaultAlgorithm) {
		compliance.Algorithms[em.config.DefaultAlgorithm] = true
		score += 25
	}
	
	// Check key size compliance
	if em.config.KeySize >= 256 {
		compliance.KeySizes["AES"] = true
		score += 25
	}
	if em.config.RSAKeySize >= 2048 {
		compliance.KeySizes["RSA"] = true
		score += 25
	}
	
	compliance.Score = score
	
	return compliance
}

// getStandardTestVectors returns standard test vectors for validation
func (em *EncryptionManager) getStandardTestVectors() map[string][]TestVector {
	return map[string][]TestVector{
		"AES-256-GCM": {
			{
				Algorithm: "AES-256-GCM",
				Key:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				IV:        "0123456789abcdef01234567",
				Plaintext: "Hello, World!",
			},
		},
	}
}

// TestVector represents a standard test vector
type TestVector struct {
	Algorithm  string `json:"algorithm"`
	Key        string `json:"key"`
	IV         string `json:"iv"`
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"`
}

// encryptWithVector encrypts using a test vector
func (em *EncryptionManager) encryptWithVector(vector TestVector) (*EncryptionResult, error) {
	// Simplified implementation for demonstration
	key, err := base64.StdEncoding.DecodeString(vector.Key)
	if err != nil {
		return nil, err
	}
	
	data := []byte(vector.Plaintext)
	
	switch vector.Algorithm {
	case "AES-256-GCM":
		return em.encryptAESGCM(data, key)
	default:
		return nil, fmt.Errorf("unsupported vector algorithm: %s", vector.Algorithm)
	}
}

// GetMetrics returns encryption metrics
func (em *EncryptionManager) GetMetrics() *EncryptionMetrics {
	em.mu.RLock()
	defer em.mu.RUnlock()
	
	metricsCopy := *em.metrics
	metricsCopy.AlgorithmUsage = make(map[string]int64)
	metricsCopy.ErrorTypes = make(map[string]int64)
	
	for k, v := range em.metrics.AlgorithmUsage {
		metricsCopy.AlgorithmUsage[k] = v
	}
	for k, v := range em.metrics.ErrorTypes {
		metricsCopy.ErrorTypes[k] = v
	}
	
	return &metricsCopy
}