package chaos

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	mathrand "math/rand"
)

// DiskCorruptor handles disk corruption scenarios
type DiskCorruptor struct {
	mu               sync.RWMutex
	config           ChaosConfig
	logger           *log.Logger
	corruptions      map[string]*DiskCorruption
	monitoredPaths   map[string]*MonitoredPath
}

// DiskCorruption represents a disk corruption instance
type DiskCorruption struct {
	ID              string              `json:"id"`
	Type            CorruptionType      `json:"type"`
	TargetPath      string              `json:"target_path"`
	StartTime       time.Time           `json:"start_time"`
	EndTime         *time.Time          `json:"end_time,omitempty"`
	Status          CorruptionStatus    `json:"status"`
	BytesCorrupted  int64               `json:"bytes_corrupted"`
	FilesAffected   []string            `json:"files_affected"`
	BackupPaths     map[string]string   `json:"backup_paths"` // original -> backup
	Parameters      map[string]interface{} `json:"parameters"`
	Metadata        map[string]string   `json:"metadata"`
}

// MonitoredPath represents a path being monitored for corruption
type MonitoredPath struct {
	Path        string            `json:"path"`
	Type        string            `json:"type"` // raft_log, storage, config
	Checksum    string            `json:"checksum"`
	LastCheck   time.Time         `json:"last_check"`
	FileCount   int               `json:"file_count"`
	TotalSize   int64             `json:"total_size"`
	Metadata    map[string]string `json:"metadata"`
}

// CorruptionType defines types of disk corruption
type CorruptionType string

const (
	CorruptionTypeRandom     CorruptionType = "random"      // Random byte corruption
	CorruptionTypeZeroes     CorruptionType = "zeroes"      // Fill with zeroes
	CorruptionTypePattern    CorruptionType = "pattern"     // Fill with specific pattern
	CorruptionTypeTruncate   CorruptionType = "truncate"    // Truncate files
	CorruptionTypeDelete     CorruptionType = "delete"      // Delete files
	CorruptionTypePermission CorruptionType = "permission"  // Change permissions
	CorruptionTypeSlow       CorruptionType = "slow_io"     // Slow I/O simulation
	CorruptionTypeFull       CorruptionType = "disk_full"   // Disk space exhaustion
)

// CorruptionStatus represents the status of a corruption
type CorruptionStatus string

const (
	CorruptionStatusActive   CorruptionStatus = "active"
	CorruptionStatusRestored CorruptionStatus = "restored"
	CorruptionStatusFailed   CorruptionStatus = "failed"
)

// NewDiskCorruptor creates a new disk corruptor
func NewDiskCorruptor(config ChaosConfig, logger *log.Logger) *DiskCorruptor {
	if logger == nil {
		logger = log.New(log.Writer(), "[DISK_CHAOS] ", log.LstdFlags)
	}
	
	return &DiskCorruptor{
		config:         config,
		logger:         logger,
		corruptions:    make(map[string]*DiskCorruption),
		monitoredPaths: make(map[string]*MonitoredPath),
	}
}

// AddMonitoredPath adds a path to be monitored for corruption
func (dc *DiskCorruptor) AddMonitoredPath(path, pathType string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path %s does not exist", path)
	}
	
	// Calculate initial checksum and metadata
	checksum, fileCount, totalSize, err := dc.calculatePathMetrics(path)
	if err != nil {
		return fmt.Errorf("failed to calculate path metrics: %w", err)
	}
	
	monitoredPath := &MonitoredPath{
		Path:      path,
		Type:      pathType,
		Checksum:  checksum,
		LastCheck: time.Now(),
		FileCount: fileCount,
		TotalSize: totalSize,
		Metadata:  make(map[string]string),
	}
	
	dc.monitoredPaths[path] = monitoredPath
	dc.logger.Printf("Added monitored path: %s (type: %s)", path, pathType)
	
	return nil
}

// CorruptPath corrupts a specific path
func (dc *DiskCorruptor) CorruptPath(path string, corruptionType CorruptionType, params map[string]interface{}) (string, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	
	if !dc.config.DiskEnabled {
		return "", fmt.Errorf("disk chaos is disabled")
	}
	
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("path %s does not exist", path)
	}
	
	// Generate corruption ID
	corruptionID := fmt.Sprintf("corruption_%s_%d", corruptionType, time.Now().UnixNano())
	
	dc.logger.Printf("Starting corruption %s on path %s", corruptionID, path)
	
	// Create corruption record
	corruption := &DiskCorruption{
		ID:         corruptionID,
		Type:       corruptionType,
		TargetPath: path,
		StartTime:  time.Now(),
		Status:     CorruptionStatusActive,
		Parameters: params,
		Metadata:   make(map[string]string),
		BackupPaths: make(map[string]string),
		FilesAffected: make([]string, 0),
	}
	
	// Create backups before corruption
	if err := dc.createBackups(corruption); err != nil {
		return "", fmt.Errorf("failed to create backups: %w", err)
	}
	
	// Execute corruption
	if err := dc.executeCorruption(corruption); err != nil {
		corruption.Status = CorruptionStatusFailed
		return "", fmt.Errorf("failed to execute corruption: %w", err)
	}
	
	dc.corruptions[corruptionID] = corruption
	
	dc.logger.Printf("Corruption %s executed successfully", corruptionID)
	return corruptionID, nil
}

// executeCorruption executes the actual corruption based on type
func (dc *DiskCorruptor) executeCorruption(corruption *DiskCorruption) error {
	switch corruption.Type {
	case CorruptionTypeRandom:
		return dc.corruptRandomBytes(corruption)
	case CorruptionTypeZeroes:
		return dc.corruptWithZeroes(corruption)
	case CorruptionTypePattern:
		return dc.corruptWithPattern(corruption)
	case CorruptionTypeTruncate:
		return dc.truncateFiles(corruption)
	case CorruptionTypeDelete:
		return dc.deleteFiles(corruption)
	case CorruptionTypePermission:
		return dc.corruptPermissions(corruption)
	case CorruptionTypeSlow:
		return dc.simulateSlowIO(corruption)
	case CorruptionTypeFull:
		return dc.simulateDiskFull(corruption)
	default:
		return fmt.Errorf("unknown corruption type: %s", corruption.Type)
	}
}

// corruptRandomBytes randomly corrupts bytes in files
func (dc *DiskCorruptor) corruptRandomBytes(corruption *DiskCorruption) error {
	dc.logger.Printf("Applying random byte corruption to %s", corruption.TargetPath)
	
	// Get corruption size from parameters
	corruptionSize := dc.config.CorruptionSize
	if size, ok := corruption.Parameters["size"].(int64); ok {
		corruptionSize = size
	}
	
	// Get files to corrupt
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	if len(files) == 0 {
		return fmt.Errorf("no files found to corrupt")
	}
	
	// Randomly select files to corrupt
	selectedFiles := dc.selectRandomFiles(files, 3) // Corrupt up to 3 files
	
	for _, filePath := range selectedFiles {
		if err := dc.corruptRandomBytesInFile(filePath, corruptionSize); err != nil {
			dc.logger.Printf("Failed to corrupt file %s: %v", filePath, err)
		} else {
			corruption.FilesAffected = append(corruption.FilesAffected, filePath)
		}
	}
	
	corruption.BytesCorrupted = corruptionSize * int64(len(corruption.FilesAffected))
	return nil
}

// corruptRandomBytesInFile corrupts random bytes in a specific file
func (dc *DiskCorruptor) corruptRandomBytesInFile(filePath string, corruptionSize int64) error {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()
	
	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	
	fileSize := stat.Size()
	if fileSize == 0 {
		return nil // Skip empty files
	}
	
	// Limit corruption size to file size
	if corruptionSize > fileSize {
		corruptionSize = fileSize
	}
	
	// Generate random positions and corrupt data
	for i := int64(0); i < corruptionSize; i++ {
		position := mathrand.Int63n(fileSize)
		
		// Seek to position
		if _, err := file.Seek(position, 0); err != nil {
			continue
		}
		
		// Write random byte
		randomByte := make([]byte, 1)
		rand.Read(randomByte)
		file.Write(randomByte)
	}
	
	// Sync to ensure corruption is written
	file.Sync()
	
	dc.logger.Printf("Corrupted %d bytes in file %s", corruptionSize, filePath)
	return nil
}

// corruptWithZeroes fills files with zero bytes
func (dc *DiskCorruptor) corruptWithZeroes(corruption *DiskCorruption) error {
	dc.logger.Printf("Applying zero-byte corruption to %s", corruption.TargetPath)
	
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	selectedFiles := dc.selectRandomFiles(files, 2)
	
	for _, filePath := range selectedFiles {
		if err := dc.fillFileWithZeroes(filePath); err != nil {
			dc.logger.Printf("Failed to zero file %s: %v", filePath, err)
		} else {
			corruption.FilesAffected = append(corruption.FilesAffected, filePath)
		}
	}
	
	return nil
}

// fillFileWithZeroes fills a file with zero bytes
func (dc *DiskCorruptor) fillFileWithZeroes(filePath string) error {
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Write zeroes
	zeroes := make([]byte, stat.Size())
	_, err = file.Write(zeroes)
	file.Sync()
	
	dc.logger.Printf("Filled file %s with zeroes (%d bytes)", filePath, stat.Size())
	return err
}

// corruptWithPattern fills files with a specific pattern
func (dc *DiskCorruptor) corruptWithPattern(corruption *DiskCorruption) error {
	pattern, ok := corruption.Parameters["pattern"].(string)
	if !ok {
		pattern = "DEADBEEF" // Default pattern
	}
	
	dc.logger.Printf("Applying pattern corruption (%s) to %s", pattern, corruption.TargetPath)
	
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	selectedFiles := dc.selectRandomFiles(files, 2)
	
	for _, filePath := range selectedFiles {
		if err := dc.fillFileWithPattern(filePath, pattern); err != nil {
			dc.logger.Printf("Failed to fill file %s with pattern: %v", filePath, err)
		} else {
			corruption.FilesAffected = append(corruption.FilesAffected, filePath)
		}
	}
	
	return nil
}

// fillFileWithPattern fills a file with a repeating pattern
func (dc *DiskCorruptor) fillFileWithPattern(filePath, pattern string) error {
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Repeat pattern to fill file
	patternBytes := []byte(pattern)
	remaining := stat.Size()
	
	for remaining > 0 {
		writeSize := int64(len(patternBytes))
		if writeSize > remaining {
			writeSize = remaining
		}
		
		_, err := file.Write(patternBytes[:writeSize])
		if err != nil {
			return err
		}
		
		remaining -= writeSize
	}
	
	file.Sync()
	dc.logger.Printf("Filled file %s with pattern %s", filePath, pattern)
	return nil
}

// truncateFiles truncates files to a smaller size
func (dc *DiskCorruptor) truncateFiles(corruption *DiskCorruption) error {
	dc.logger.Printf("Applying truncation corruption to %s", corruption.TargetPath)
	
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	selectedFiles := dc.selectRandomFiles(files, 3)
	
	for _, filePath := range selectedFiles {
		if err := dc.truncateFile(filePath); err != nil {
			dc.logger.Printf("Failed to truncate file %s: %v", filePath, err)
		} else {
			corruption.FilesAffected = append(corruption.FilesAffected, filePath)
		}
	}
	
	return nil
}

// truncateFile truncates a file to half its size
func (dc *DiskCorruptor) truncateFile(filePath string) error {
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	
	newSize := stat.Size() / 2
	if newSize < 1 {
		newSize = 1
	}
	
	err = os.Truncate(filePath, newSize)
	if err != nil {
		return err
	}
	
	dc.logger.Printf("Truncated file %s from %d to %d bytes", filePath, stat.Size(), newSize)
	return nil
}

// deleteFiles deletes random files
func (dc *DiskCorruptor) deleteFiles(corruption *DiskCorruption) error {
	dc.logger.Printf("Applying file deletion corruption to %s", corruption.TargetPath)
	
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	// Limit to deleting at most 2 files for safety
	selectedFiles := dc.selectRandomFiles(files, 2)
	
	for _, filePath := range selectedFiles {
		if err := os.Remove(filePath); err != nil {
			dc.logger.Printf("Failed to delete file %s: %v", filePath, err)
		} else {
			corruption.FilesAffected = append(corruption.FilesAffected, filePath)
			dc.logger.Printf("Deleted file %s", filePath)
		}
	}
	
	return nil
}

// corruptPermissions corrupts file permissions
func (dc *DiskCorruptor) corruptPermissions(corruption *DiskCorruption) error {
	dc.logger.Printf("Applying permission corruption to %s", corruption.TargetPath)
	
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	selectedFiles := dc.selectRandomFiles(files, 3)
	
	for _, filePath := range selectedFiles {
		// Change permissions to make file inaccessible
		if err := os.Chmod(filePath, 0000); err != nil {
			dc.logger.Printf("Failed to corrupt permissions for %s: %v", filePath, err)
		} else {
			corruption.FilesAffected = append(corruption.FilesAffected, filePath)
			dc.logger.Printf("Corrupted permissions for file %s", filePath)
		}
	}
	
	return nil
}

// simulateSlowIO simulates slow I/O operations
func (dc *DiskCorruptor) simulateSlowIO(corruption *DiskCorruption) error {
	dc.logger.Printf("Simulating slow I/O for %s", corruption.TargetPath)
	
	// In a real implementation, this would:
	// - Use cgroups to limit I/O bandwidth
	// - Add artificial delays to I/O operations
	// - Use tools like fio to create I/O pressure
	
	corruption.Metadata["slow_io"] = "active"
	corruption.Metadata["simulation"] = "true"
	
	return nil
}

// simulateDiskFull simulates disk space exhaustion
func (dc *DiskCorruptor) simulateDiskFull(corruption *DiskCorruption) error {
	dc.logger.Printf("Simulating disk full for %s", corruption.TargetPath)
	
	// Create a large dummy file to consume disk space
	dummyFile := filepath.Join(corruption.TargetPath, "chaos_dummy_file")
	
	// Calculate available space (simplified)
	targetSize := int64(1024 * 1024 * 100) // 100MB dummy file
	if size, ok := corruption.Parameters["size"].(int64); ok {
		targetSize = size
	}
	
	file, err := os.Create(dummyFile)
	if err != nil {
		return fmt.Errorf("failed to create dummy file: %w", err)
	}
	defer file.Close()
	
	// Write random data to consume space
	buffer := make([]byte, 1024*1024) // 1MB buffer
	rand.Read(buffer)
	
	written := int64(0)
	for written < targetSize {
		writeSize := int64(len(buffer))
		if written+writeSize > targetSize {
			writeSize = targetSize - written
		}
		
		n, err := file.Write(buffer[:writeSize])
		if err != nil {
			break
		}
		
		written += int64(n)
	}
	
	file.Sync()
	
	corruption.FilesAffected = append(corruption.FilesAffected, dummyFile)
	corruption.BytesCorrupted = written
	corruption.Metadata["dummy_file"] = dummyFile
	
	dc.logger.Printf("Created dummy file %s with %d bytes", dummyFile, written)
	return nil
}

// RestoreCorruption restores a corruption by reverting changes
func (dc *DiskCorruptor) RestoreCorruption(corruptionID string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	
	corruption, exists := dc.corruptions[corruptionID]
	if !exists {
		return fmt.Errorf("corruption %s not found", corruptionID)
	}
	
	if corruption.Status != CorruptionStatusActive {
		return fmt.Errorf("corruption %s is not active", corruptionID)
	}
	
	dc.logger.Printf("Restoring corruption %s", corruptionID)
	
	// Restore from backups
	if err := dc.restoreFromBackups(corruption); err != nil {
		return fmt.Errorf("failed to restore from backups: %w", err)
	}
	
	// Clean up dummy files
	if dummyFile, ok := corruption.Metadata["dummy_file"]; ok {
		if err := os.Remove(dummyFile); err != nil {
			dc.logger.Printf("Failed to remove dummy file %s: %v", dummyFile, err)
		}
	}
	
	// Update status
	now := time.Now()
	corruption.Status = CorruptionStatusRestored
	corruption.EndTime = &now
	
	dc.logger.Printf("Corruption %s restored successfully", corruptionID)
	return nil
}

// createBackups creates backups of files before corruption
func (dc *DiskCorruptor) createBackups(corruption *DiskCorruption) error {
	files, err := dc.getFilesToCorrupt(corruption.TargetPath)
	if err != nil {
		return err
	}
	
	backupDir := filepath.Join(os.TempDir(), "chaos_backups", corruption.ID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	
	for _, filePath := range files {
		backupPath := filepath.Join(backupDir, filepath.Base(filePath))
		if err := dc.copyFile(filePath, backupPath); err != nil {
			dc.logger.Printf("Failed to backup file %s: %v", filePath, err)
		} else {
			corruption.BackupPaths[filePath] = backupPath
		}
	}
	
	return nil
}

// restoreFromBackups restores files from backups
func (dc *DiskCorruptor) restoreFromBackups(corruption *DiskCorruption) error {
	for originalPath, backupPath := range corruption.BackupPaths {
		if err := dc.copyFile(backupPath, originalPath); err != nil {
			dc.logger.Printf("Failed to restore file %s: %v", originalPath, err)
		} else {
			dc.logger.Printf("Restored file %s from backup", originalPath)
		}
	}
	
	// Clean up backup directory
	if len(corruption.BackupPaths) > 0 {
		// Get any backup path to determine backup directory
		for _, backupPath := range corruption.BackupPaths {
			backupDir := filepath.Dir(backupPath)
			os.RemoveAll(backupDir)
			break
		}
	}
	
	return nil
}

// Helper functions

// getFilesToCorrupt gets a list of files that can be corrupted
func (dc *DiskCorruptor) getFilesToCorrupt(path string) ([]string, error) {
	var files []string
	
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		
		if !info.IsDir() && info.Size() > 0 {
			files = append(files, filePath)
		}
		
		return nil
	})
	
	return files, err
}

// selectRandomFiles selects a random subset of files
func (dc *DiskCorruptor) selectRandomFiles(files []string, maxCount int) []string {
	if len(files) == 0 {
		return files
	}
	
	if maxCount >= len(files) {
		return files
	}
	
	// Shuffle and select first maxCount files
	shuffled := make([]string, len(files))
	copy(shuffled, files)
	
	mathrand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	
	return shuffled[:maxCount]
}

// copyFile copies a file from source to destination
func (dc *DiskCorruptor) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}
	
	return destFile.Sync()
}

// calculatePathMetrics calculates checksum and metrics for a path
func (dc *DiskCorruptor) calculatePathMetrics(path string) (string, int, int64, error) {
	fileCount := 0
	totalSize := int64(0)
	
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		
		return nil
	})
	
	// Simple checksum based on file count and total size
	checksum := fmt.Sprintf("%d_%d_%d", fileCount, totalSize, time.Now().Unix())
	
	return checksum, fileCount, totalSize, err
}

// GetCorruption returns information about a specific corruption
func (dc *DiskCorruptor) GetCorruption(corruptionID string) (*DiskCorruption, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	
	corruption, exists := dc.corruptions[corruptionID]
	if !exists {
		return nil, fmt.Errorf("corruption %s not found", corruptionID)
	}
	
	// Return a copy
	corruptionCopy := *corruption
	return &corruptionCopy, nil
}

// GetActiveCorruptions returns all active corruptions
func (dc *DiskCorruptor) GetActiveCorruptions() map[string]*DiskCorruption {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	
	result := make(map[string]*DiskCorruption)
	for corruptionID, corruption := range dc.corruptions {
		if corruption.Status == CorruptionStatusActive {
			corruptionCopy := *corruption
			result[corruptionID] = &corruptionCopy
		}
	}
	
	return result
}

// GetDiskStats returns disk corruption statistics
func (dc *DiskCorruptor) GetDiskStats() DiskCorruptorStats {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	
	stats := DiskCorruptorStats{
		TotalCorruptions:    len(dc.corruptions),
		ActiveCorruptions:   0,
		MonitoredPaths:      len(dc.monitoredPaths),
		CorruptionsByType:   make(map[CorruptionType]int),
		CorruptionsByStatus: make(map[CorruptionStatus]int),
	}
	
	for _, corruption := range dc.corruptions {
		stats.CorruptionsByType[corruption.Type]++
		stats.CorruptionsByStatus[corruption.Status]++
		
		if corruption.Status == CorruptionStatusActive {
			stats.ActiveCorruptions++
		}
		
		stats.TotalBytesCorrupted += corruption.BytesCorrupted
		stats.TotalFilesAffected += len(corruption.FilesAffected)
	}
	
	return stats
}

// DiskCorruptorStats contains disk corruption statistics
type DiskCorruptorStats struct {
	TotalCorruptions     int                             `json:"total_corruptions"`
	ActiveCorruptions    int                             `json:"active_corruptions"`
	MonitoredPaths       int                             `json:"monitored_paths"`
	TotalBytesCorrupted  int64                           `json:"total_bytes_corrupted"`
	TotalFilesAffected   int                             `json:"total_files_affected"`
	CorruptionsByType    map[CorruptionType]int          `json:"corruptions_by_type"`
	CorruptionsByStatus  map[CorruptionStatus]int        `json:"corruptions_by_status"`
}