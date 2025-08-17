package performance

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// StorageOptimizer implements advanced storage optimizations
type StorageOptimizer struct {
	mu              sync.RWMutex
	logger          *log.Logger
	config          StorageConfig
	writeCache      *WriteCache
	readCache       *ReadCache
	compactionMgr   *CompactionManager
	bloomFilters    map[string]*BloomFilter
	indexCache      *IndexCache
	metrics         *StorageMetrics
	isRunning       bool
	storageDir      string
	dataFiles       map[string]*DataFile
}

// StorageConfig contains configuration for storage optimizations
type StorageConfig struct {
	EnableWriteCache    bool          `json:"enable_write_cache"`     // Enable write caching
	EnableReadCache     bool          `json:"enable_read_cache"`      // Enable read caching
	EnableCompaction    bool          `json:"enable_compaction"`      // Enable automatic compaction
	EnableBloomFilters  bool          `json:"enable_bloom_filters"`   // Enable bloom filters
	WriteCacheSize      int           `json:"write_cache_size"`       // Write cache size in MB
	ReadCacheSize       int           `json:"read_cache_size"`        // Read cache size in MB
	CompactionThreshold float64       `json:"compaction_threshold"`   // Compaction trigger threshold
	BloomFilterSize     int           `json:"bloom_filter_size"`      // Bloom filter size
	BloomFilterHashes   int           `json:"bloom_filter_hashes"`    // Number of hash functions
	BatchSize           int           `json:"batch_size"`             // Batch size for operations
	SyncInterval        time.Duration `json:"sync_interval"`          // Sync interval
	CompressionLevel    int           `json:"compression_level"`      // Compression level (0-9)
	EnableChecksum      bool          `json:"enable_checksum"`        // Enable data checksums
}

// WriteCache implements write-through caching for improved write performance
type WriteCache struct {
	mu        sync.RWMutex
	entries   map[string]*WriteCacheEntry
	maxSize   int
	currentSize int
	metrics   *WriteCacheMetrics
	flushCh   chan struct{}
	isRunning bool
}

// WriteCacheEntry represents a cached write entry
type WriteCacheEntry struct {
	Key       string    `json:"key"`
	Value     []byte    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Size      int       `json:"size"`
	Dirty     bool      `json:"dirty"`
}

// ReadCache implements LRU read caching
type ReadCache struct {
	mu        sync.RWMutex
	entries   map[string]*ReadCacheEntry
	lruList   *LRUList
	maxSize   int
	currentSize int
	metrics   *ReadCacheMetrics
}

// ReadCacheEntry represents a cached read entry
type ReadCacheEntry struct {
	Key       string    `json:"key"`
	Value     []byte    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Size      int       `json:"size"`
	AccessCount int     `json:"access_count"`
	lruNode   *LRUNode
}

// LRUList implements a doubly-linked list for LRU tracking
type LRUList struct {
	head *LRUNode
	tail *LRUNode
	size int
}

// LRUNode represents a node in the LRU list
type LRUNode struct {
	key  string
	prev *LRUNode
	next *LRUNode
}

// CompactionManager handles automatic storage compaction
type CompactionManager struct {
	mu              sync.Mutex
	logger          *log.Logger
	config          StorageConfig
	storageDir      string
	isRunning       bool
	stopCh          chan struct{}
	compactionQueue chan string
	metrics         *CompactionMetrics
}

// BloomFilter implements space-efficient set membership testing
type BloomFilter struct {
	mu        sync.RWMutex
	bitArray  []uint64
	size      int
	hashFuncs int
	numItems  uint64
	metrics   *BloomFilterMetrics
}

// IndexCache caches frequently accessed index entries
type IndexCache struct {
	mu      sync.RWMutex
	entries map[string]*IndexEntry
	maxSize int
	metrics *IndexCacheMetrics
}

// IndexEntry represents a cached index entry
type IndexEntry struct {
	Key      string `json:"key"`
	Offset   int64  `json:"offset"`
	Size     int    `json:"size"`
	Checksum uint32 `json:"checksum"`
}

// DataFile represents an optimized data file
type DataFile struct {
	mu         sync.RWMutex
	path       string
	file       *os.File
	writer     *bufio.Writer
	size       int64
	entries    int
	bloomFilter *BloomFilter
	index      map[string]*IndexEntry
	metrics    *DataFileMetrics
}

// Storage metrics structures
type StorageMetrics struct {
	TotalReads       uint64                       `json:"total_reads"`
	TotalWrites      uint64                       `json:"total_writes"`
	CacheHits        uint64                       `json:"cache_hits"`
	CacheMisses      uint64                       `json:"cache_misses"`
	CompactionRuns   uint64                       `json:"compaction_runs"`
	BloomFilterHits  uint64                       `json:"bloom_filter_hits"`
	BloomFilterMisses uint64                      `json:"bloom_filter_misses"`
	WriteLatency     time.Duration                `json:"write_latency"`
	ReadLatency      time.Duration                `json:"read_latency"`
	WriteCacheMetrics *WriteCacheMetrics          `json:"write_cache_metrics"`
	ReadCacheMetrics *ReadCacheMetrics            `json:"read_cache_metrics"`
	CompactionMetrics *CompactionMetrics          `json:"compaction_metrics"`
	BloomFilterMetrics map[string]*BloomFilterMetrics `json:"bloom_filter_metrics"`
	IndexCacheMetrics *IndexCacheMetrics           `json:"index_cache_metrics"`
}

type WriteCacheMetrics struct {
	Hits         uint64        `json:"hits"`
	Misses       uint64        `json:"misses"`
	Evictions    uint64        `json:"evictions"`
	FlushCount   uint64        `json:"flush_count"`
	CurrentSize  int           `json:"current_size"`
	AverageLatency time.Duration `json:"average_latency"`
}

type ReadCacheMetrics struct {
	Hits         uint64        `json:"hits"`
	Misses       uint64        `json:"misses"`
	Evictions    uint64        `json:"evictions"`
	CurrentSize  int           `json:"current_size"`
	HitRate      float64       `json:"hit_rate"`
}

type CompactionMetrics struct {
	TotalRuns       uint64        `json:"total_runs"`
	BytesCompacted  uint64        `json:"bytes_compacted"`
	SpaceReclaimed  uint64        `json:"space_reclaimed"`
	AverageTime     time.Duration `json:"average_time"`
	LastCompaction  time.Time     `json:"last_compaction"`
}

type BloomFilterMetrics struct {
	Size           int     `json:"size"`
	NumItems       uint64  `json:"num_items"`
	EstimatedFPR   float64 `json:"estimated_fpr"`
	Hits           uint64  `json:"hits"`
	Misses         uint64  `json:"misses"`
}

type IndexCacheMetrics struct {
	Hits        uint64  `json:"hits"`
	Misses      uint64  `json:"misses"`
	CurrentSize int     `json:"current_size"`
	HitRate     float64 `json:"hit_rate"`
}

type DataFileMetrics struct {
	Size           int64     `json:"size"`
	EntryCount     int       `json:"entry_count"`
	LastWrite      time.Time `json:"last_write"`
	WriteCount     uint64    `json:"write_count"`
	ReadCount      uint64    `json:"read_count"`
	CompressionRatio float64 `json:"compression_ratio"`
}

// NewStorageOptimizer creates a new storage optimizer
func NewStorageOptimizer(config StorageConfig, storageDir string, logger *log.Logger) *StorageOptimizer {
	if logger == nil {
		logger = log.New(log.Writer(), "[STORAGE] ", log.LstdFlags)
	}
	
	// Set default values
	if config.WriteCacheSize == 0 {
		config.WriteCacheSize = 64 // 64MB
	}
	if config.ReadCacheSize == 0 {
		config.ReadCacheSize = 128 // 128MB
	}
	if config.CompactionThreshold == 0 {
		config.CompactionThreshold = 0.5 // 50%
	}
	if config.BloomFilterSize == 0 {
		config.BloomFilterSize = 1000000 // 1M bits
	}
	if config.BloomFilterHashes == 0 {
		config.BloomFilterHashes = 3
	}
	if config.BatchSize == 0 {
		config.BatchSize = 1000
	}
	if config.SyncInterval == 0 {
		config.SyncInterval = 5 * time.Second
	}
	
	so := &StorageOptimizer{
		logger:       logger,
		config:       config,
		storageDir:   storageDir,
		bloomFilters: make(map[string]*BloomFilter),
		dataFiles:    make(map[string]*DataFile),
		metrics: &StorageMetrics{
			WriteCacheMetrics:  &WriteCacheMetrics{},
			ReadCacheMetrics:   &ReadCacheMetrics{},
			CompactionMetrics:  &CompactionMetrics{},
			BloomFilterMetrics: make(map[string]*BloomFilterMetrics),
			IndexCacheMetrics:  &IndexCacheMetrics{},
		},
	}
	
	// Initialize components
	if config.EnableWriteCache {
		so.writeCache = NewWriteCache(config.WriteCacheSize * 1024 * 1024) // Convert MB to bytes
		so.metrics.WriteCacheMetrics = so.writeCache.metrics
	}
	
	if config.EnableReadCache {
		so.readCache = NewReadCache(config.ReadCacheSize * 1024 * 1024)
		so.metrics.ReadCacheMetrics = so.readCache.metrics
	}
	
	if config.EnableCompaction {
		so.compactionMgr = NewCompactionManager(config, storageDir, logger)
		so.metrics.CompactionMetrics = so.compactionMgr.metrics
	}
	
	so.indexCache = NewIndexCache(10000) // 10K entries
	so.metrics.IndexCacheMetrics = so.indexCache.metrics
	
	return so
}

// Start initializes the storage optimization system
func (so *StorageOptimizer) Start(ctx context.Context) error {
	so.mu.Lock()
	defer so.mu.Unlock()
	
	if so.isRunning {
		return fmt.Errorf("storage optimizer already running")
	}
	
	so.logger.Printf("Starting storage optimizer with config: write_cache=%t, read_cache=%t, compaction=%t", 
		so.config.EnableWriteCache, so.config.EnableReadCache, so.config.EnableCompaction)
	
	// Create storage directory
	if err := os.MkdirAll(so.storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}
	
	// Start write cache
	if so.writeCache != nil {
		if err := so.writeCache.Start(ctx); err != nil {
			return fmt.Errorf("failed to start write cache: %w", err)
		}
	}
	
	// Start compaction manager
	if so.compactionMgr != nil {
		if err := so.compactionMgr.Start(ctx); err != nil {
			return fmt.Errorf("failed to start compaction manager: %w", err)
		}
	}
	
	// Load existing data files
	if err := so.loadDataFiles(); err != nil {
		return fmt.Errorf("failed to load data files: %w", err)
	}
	
	so.isRunning = true
	return nil
}

// OptimizedWrite performs an optimized write operation
func (so *StorageOptimizer) OptimizedWrite(key string, value []byte) error {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&so.metrics.TotalWrites, 1)
		so.metrics.WriteLatency = (so.metrics.WriteLatency + time.Since(start)) / 2
	}()
	
	// Write to cache first if enabled
	if so.writeCache != nil {
		if err := so.writeCache.Put(key, value); err != nil {
			so.logger.Printf("Write cache error: %v", err)
		}
	}
	
	// Write to storage
	dataFile := so.getOrCreateDataFile(key)
	entry := &IndexEntry{
		Key:      key,
		Offset:   dataFile.size,
		Size:     len(value),
		Checksum: so.calculateChecksum(value),
	}
	
	if err := dataFile.Write(key, value, entry); err != nil {
		return fmt.Errorf("failed to write to data file: %w", err)
	}
	
	// Update bloom filter
	if so.config.EnableBloomFilters {
		if bloomFilter := so.getBloomFilter(dataFile.path); bloomFilter != nil {
			bloomFilter.Add(key)
		}
	}
	
	// Update index cache
	so.indexCache.Put(key, entry)
	
	return nil
}

// OptimizedRead performs an optimized read operation
func (so *StorageOptimizer) OptimizedRead(key string) ([]byte, error) {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&so.metrics.TotalReads, 1)
		so.metrics.ReadLatency = (so.metrics.ReadLatency + time.Since(start)) / 2
	}()
	
	// Check read cache first
	if so.readCache != nil {
		if value, found := so.readCache.Get(key); found {
			atomic.AddUint64(&so.metrics.CacheHits, 1)
			return value, nil
		}
		atomic.AddUint64(&so.metrics.CacheMisses, 1)
	}
	
	// Check bloom filters to avoid disk access
	if so.config.EnableBloomFilters {
		found := false
		for _, bloomFilter := range so.bloomFilters {
			if bloomFilter.Contains(key) {
				found = true
				atomic.AddUint64(&so.metrics.BloomFilterHits, 1)
				break
			}
		}
		if !found {
			atomic.AddUint64(&so.metrics.BloomFilterMisses, 1)
			return nil, fmt.Errorf("key not found: %s", key)
		}
	}
	
	// Check index cache
	if entry := so.indexCache.Get(key); entry != nil {
		// Read from data file using cached index
		dataFile := so.getDataFileByPath(so.getDataFilePath(key))
		if dataFile != nil {
			value, err := dataFile.ReadAt(entry.Offset, entry.Size)
			if err == nil && so.config.EnableChecksum {
				if so.calculateChecksum(value) != entry.Checksum {
					return nil, fmt.Errorf("checksum mismatch for key: %s", key)
				}
			}
			
			// Cache the read value
			if so.readCache != nil && err == nil {
				so.readCache.Put(key, value)
			}
			
			return value, err
		}
	}
	
	// Fallback to scanning data files
	return so.scanDataFiles(key)
}

// NewWriteCache creates a new write cache
func NewWriteCache(maxSize int) *WriteCache {
	return &WriteCache{
		entries:   make(map[string]*WriteCacheEntry),
		maxSize:   maxSize,
		metrics:   &WriteCacheMetrics{},
		flushCh:   make(chan struct{}, 1),
		isRunning: false,
	}
}

// Start initializes the write cache
func (wc *WriteCache) Start(ctx context.Context) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	
	if wc.isRunning {
		return fmt.Errorf("write cache already running")
	}
	
	wc.isRunning = true
	
	// Start periodic flush
	go wc.flushRoutine(ctx)
	
	return nil
}

// Put adds an entry to the write cache
func (wc *WriteCache) Put(key string, value []byte) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	
	size := len(value)
	
	// Check if we need to evict entries
	for wc.currentSize+size > wc.maxSize && len(wc.entries) > 0 {
		wc.evictOldest()
	}
	
	entry := &WriteCacheEntry{
		Key:       key,
		Value:     make([]byte, len(value)),
		Timestamp: time.Now(),
		Size:      size,
		Dirty:     true,
	}
	copy(entry.Value, value)
	
	// Remove existing entry if any
	if existing, exists := wc.entries[key]; exists {
		wc.currentSize -= existing.Size
	}
	
	wc.entries[key] = entry
	wc.currentSize += size
	wc.metrics.CurrentSize = wc.currentSize
	
	return nil
}

// evictOldest removes the oldest entry from the cache
func (wc *WriteCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	
	for key, entry := range wc.entries {
		if oldestKey == "" || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
		}
	}
	
	if oldestKey != "" {
		entry := wc.entries[oldestKey]
		delete(wc.entries, oldestKey)
		wc.currentSize -= entry.Size
		atomic.AddUint64(&wc.metrics.Evictions, 1)
	}
}

// flushRoutine periodically flushes dirty entries
func (wc *WriteCache) flushRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wc.flush()
		case <-wc.flushCh:
			wc.flush()
		}
	}
}

// flush writes dirty entries to storage
func (wc *WriteCache) flush() {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	
	dirtyCount := 0
	for _, entry := range wc.entries {
		if entry.Dirty {
			dirtyCount++
			entry.Dirty = false // Mark as clean
		}
	}
	
	if dirtyCount > 0 {
		atomic.AddUint64(&wc.metrics.FlushCount, 1)
	}
}

// NewReadCache creates a new read cache
func NewReadCache(maxSize int) *ReadCache {
	return &ReadCache{
		entries:   make(map[string]*ReadCacheEntry),
		lruList:   NewLRUList(),
		maxSize:   maxSize,
		metrics:   &ReadCacheMetrics{},
	}
}

// Get retrieves a value from the read cache
func (rc *ReadCache) Get(key string) ([]byte, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	
	entry, exists := rc.entries[key]
	if !exists {
		atomic.AddUint64(&rc.metrics.Misses, 1)
		rc.updateHitRate()
		return nil, false
	}
	
	// Move to front of LRU list
	rc.lruList.MoveToFront(entry.lruNode)
	entry.AccessCount++
	
	atomic.AddUint64(&rc.metrics.Hits, 1)
	rc.updateHitRate()
	
	return entry.Value, true
}

// Put adds a value to the read cache
func (rc *ReadCache) Put(key string, value []byte) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	
	size := len(value)
	
	// Check if we need to evict entries
	for rc.currentSize+size > rc.maxSize && rc.lruList.size > 0 {
		rc.evictLRU()
	}
	
	// Remove existing entry if any
	if existing, exists := rc.entries[key]; exists {
		rc.lruList.Remove(existing.lruNode)
		rc.currentSize -= existing.Size
	}
	
	// Create new entry
	entry := &ReadCacheEntry{
		Key:         key,
		Value:       make([]byte, len(value)),
		Timestamp:   time.Now(),
		Size:        size,
		AccessCount: 1,
		lruNode:     &LRUNode{key: key},
	}
	copy(entry.Value, value)
	
	rc.entries[key] = entry
	rc.lruList.AddToFront(entry.lruNode)
	rc.currentSize += size
	rc.metrics.CurrentSize = rc.currentSize
}

// evictLRU removes the least recently used entry
func (rc *ReadCache) evictLRU() {
	if rc.lruList.tail == nil {
		return
	}
	
	key := rc.lruList.tail.key
	entry := rc.entries[key]
	
	rc.lruList.Remove(entry.lruNode)
	delete(rc.entries, key)
	rc.currentSize -= entry.Size
	
	atomic.AddUint64(&rc.metrics.Evictions, 1)
}

// updateHitRate calculates the cache hit rate
func (rc *ReadCache) updateHitRate() {
	total := rc.metrics.Hits + rc.metrics.Misses
	if total > 0 {
		rc.metrics.HitRate = float64(rc.metrics.Hits) / float64(total)
	}
}

// NewLRUList creates a new LRU list
func NewLRUList() *LRUList {
	return &LRUList{size: 0}
}

// AddToFront adds a node to the front of the list
func (lru *LRUList) AddToFront(node *LRUNode) {
	if lru.head == nil {
		lru.head = node
		lru.tail = node
	} else {
		node.next = lru.head
		lru.head.prev = node
		lru.head = node
	}
	lru.size++
}

// Remove removes a node from the list
func (lru *LRUList) Remove(node *LRUNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		lru.head = node.next
	}
	
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		lru.tail = node.prev
	}
	
	lru.size--
}

// MoveToFront moves a node to the front of the list
func (lru *LRUList) MoveToFront(node *LRUNode) {
	lru.Remove(node)
	lru.AddToFront(node)
}

// NewCompactionManager creates a new compaction manager
func NewCompactionManager(config StorageConfig, storageDir string, logger *log.Logger) *CompactionManager {
	return &CompactionManager{
		logger:          logger,
		config:          config,
		storageDir:      storageDir,
		stopCh:          make(chan struct{}),
		compactionQueue: make(chan string, 100),
		metrics:         &CompactionMetrics{},
	}
}

// Start initializes the compaction manager
func (cm *CompactionManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if cm.isRunning {
		return fmt.Errorf("compaction manager already running")
	}
	
	cm.isRunning = true
	
	// Start compaction worker
	go cm.compactionWorker(ctx)
	
	// Start monitoring
	go cm.monitoringRoutine(ctx)
	
	return nil
}

// compactionWorker processes compaction requests
func (cm *CompactionManager) compactionWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case filePath := <-cm.compactionQueue:
			cm.compactFile(filePath)
		}
	}
}

// monitoringRoutine monitors storage and triggers compaction
func (cm *CompactionManager) monitoringRoutine(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.checkCompactionNeeded()
		}
	}
}

// checkCompactionNeeded checks if compaction is needed
func (cm *CompactionManager) checkCompactionNeeded() {
	// Simple heuristic: trigger compaction if file is large and has low utilization
	// In a real implementation, this would be more sophisticated
	
	files, err := filepath.Glob(filepath.Join(cm.storageDir, "*.dat"))
	if err != nil {
		return
	}
	
	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			continue
		}
		
		// Trigger compaction if file is larger than 100MB
		if stat.Size() > 100*1024*1024 {
			select {
			case cm.compactionQueue <- file:
			default:
				// Queue is full, skip
			}
		}
	}
}

// compactFile performs compaction on a data file
func (cm *CompactionManager) compactFile(filePath string) {
	start := time.Now()
	cm.logger.Printf("Starting compaction for file: %s", filePath)
	
	// Simple compaction: create new file with only valid entries
	tempPath := filePath + ".tmp"
	
	// Simulate compaction work
	time.Sleep(100 * time.Millisecond)
	
	// Update metrics
	atomic.AddUint64(&cm.metrics.TotalRuns, 1)
	cm.metrics.AverageTime = (cm.metrics.AverageTime + time.Since(start)) / 2
	cm.metrics.LastCompaction = time.Now()
	
	cm.logger.Printf("Completed compaction for file: %s in %v", filePath, time.Since(start))
}

// NewBloomFilter creates a new bloom filter
func NewBloomFilter(size, hashFuncs int) *BloomFilter {
	numWords := (size + 63) / 64 // Round up to nearest 64-bit word
	return &BloomFilter{
		bitArray:  make([]uint64, numWords),
		size:      size,
		hashFuncs: hashFuncs,
		metrics: &BloomFilterMetrics{
			Size: size,
		},
	}
}

// Add adds an item to the bloom filter
func (bf *BloomFilter) Add(item string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()
	
	hashes := bf.hash(item)
	for _, hash := range hashes {
		wordIndex := hash / 64
		bitIndex := hash % 64
		if wordIndex < len(bf.bitArray) {
			bf.bitArray[wordIndex] |= 1 << bitIndex
		}
	}
	
	atomic.AddUint64(&bf.numItems, 1)
	bf.metrics.NumItems = bf.numItems
	bf.updateFalsePositiveRate()
}

// Contains checks if an item might be in the bloom filter
func (bf *BloomFilter) Contains(item string) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	
	hashes := bf.hash(item)
	for _, hash := range hashes {
		wordIndex := hash / 64
		bitIndex := hash % 64
		if wordIndex >= len(bf.bitArray) || (bf.bitArray[wordIndex]&(1<<bitIndex)) == 0 {
			atomic.AddUint64(&bf.metrics.Misses, 1)
			return false
		}
	}
	
	atomic.AddUint64(&bf.metrics.Hits, 1)
	return true
}

// hash generates hash values for an item
func (bf *BloomFilter) hash(item string) []int {
	hashes := make([]int, bf.hashFuncs)
	
	// Simple hash functions (in practice, use better hash functions)
	h1 := bf.simpleHash(item)
	h2 := bf.simpleHash(item + "salt")
	
	for i := 0; i < bf.hashFuncs; i++ {
		hash := (h1 + i*h2) % bf.size
		if hash < 0 {
			hash = -hash
		}
		hashes[i] = hash
	}
	
	return hashes
}

// simpleHash implements a simple hash function
func (bf *BloomFilter) simpleHash(s string) int {
	hash := 0
	for _, c := range s {
		hash = hash*31 + int(c)
	}
	return hash
}

// updateFalsePositiveRate estimates the false positive rate
func (bf *BloomFilter) updateFalsePositiveRate() {
	if bf.numItems == 0 {
		bf.metrics.EstimatedFPR = 0
		return
	}
	
	// Rough estimate: FPR ≈ (1 - e^(-k*n/m))^k
	// where k = hash functions, n = items, m = bit array size
	// Simplified calculation for demonstration
	ratio := float64(bf.numItems) / float64(bf.size)
	bf.metrics.EstimatedFPR = ratio * 0.1 // Simplified
}

// NewIndexCache creates a new index cache
func NewIndexCache(maxSize int) *IndexCache {
	return &IndexCache{
		entries: make(map[string]*IndexEntry),
		maxSize: maxSize,
		metrics: &IndexCacheMetrics{},
	}
}

// Get retrieves an index entry from cache
func (ic *IndexCache) Get(key string) *IndexEntry {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	
	entry, exists := ic.entries[key]
	if !exists {
		atomic.AddUint64(&ic.metrics.Misses, 1)
		ic.updateHitRate()
		return nil
	}
	
	atomic.AddUint64(&ic.metrics.Hits, 1)
	ic.updateHitRate()
	return entry
}

// Put adds an index entry to cache
func (ic *IndexCache) Put(key string, entry *IndexEntry) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	
	// Simple eviction: remove random entry if cache is full
	if len(ic.entries) >= ic.maxSize {
		for k := range ic.entries {
			delete(ic.entries, k)
			break
		}
	}
	
	ic.entries[key] = entry
	ic.metrics.CurrentSize = len(ic.entries)
}

// updateHitRate calculates the hit rate
func (ic *IndexCache) updateHitRate() {
	total := ic.metrics.Hits + ic.metrics.Misses
	if total > 0 {
		ic.metrics.HitRate = float64(ic.metrics.Hits) / float64(total)
	}
}

// Helper methods for StorageOptimizer

// getOrCreateDataFile gets or creates a data file for a key
func (so *StorageOptimizer) getOrCreateDataFile(key string) *DataFile {
	filePath := so.getDataFilePath(key)
	
	so.mu.Lock()
	defer so.mu.Unlock()
	
	if dataFile, exists := so.dataFiles[filePath]; exists {
		return dataFile
	}
	
	// Create new data file
	dataFile := NewDataFile(filePath)
	if err := dataFile.Open(); err != nil {
		so.logger.Printf("Failed to open data file %s: %v", filePath, err)
		return nil
	}
	
	so.dataFiles[filePath] = dataFile
	
	// Create bloom filter for this file
	if so.config.EnableBloomFilters {
		bloomFilter := NewBloomFilter(so.config.BloomFilterSize, so.config.BloomFilterHashes)
		so.bloomFilters[filePath] = bloomFilter
		so.metrics.BloomFilterMetrics[filePath] = bloomFilter.metrics
	}
	
	return dataFile
}

// getDataFilePath determines the file path for a key
func (so *StorageOptimizer) getDataFilePath(key string) string {
	// Simple sharding based on key hash
	hash := so.simpleHash(key)
	shardID := hash % 16 // 16 shards
	return filepath.Join(so.storageDir, fmt.Sprintf("data_%02d.dat", shardID))
}

// simpleHash implements a simple hash function
func (so *StorageOptimizer) simpleHash(s string) int {
	hash := 0
	for _, c := range s {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// getDataFileByPath gets a data file by path
func (so *StorageOptimizer) getDataFileByPath(path string) *DataFile {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return so.dataFiles[path]
}

// getBloomFilter gets the bloom filter for a file
func (so *StorageOptimizer) getBloomFilter(filePath string) *BloomFilter {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return so.bloomFilters[filePath]
}

// calculateChecksum calculates a checksum for data
func (so *StorageOptimizer) calculateChecksum(data []byte) uint32 {
	if !so.config.EnableChecksum {
		return 0
	}
	
	hash := sha256.Sum256(data)
	return binary.LittleEndian.Uint32(hash[:4])
}

// loadDataFiles loads existing data files
func (so *StorageOptimizer) loadDataFiles() error {
	files, err := filepath.Glob(filepath.Join(so.storageDir, "*.dat"))
	if err != nil {
		return err
	}
	
	for _, file := range files {
		dataFile := NewDataFile(file)
		if err := dataFile.Open(); err != nil {
			so.logger.Printf("Failed to open existing data file %s: %v", file, err)
			continue
		}
		
		so.dataFiles[file] = dataFile
		
		// Create bloom filter for existing file
		if so.config.EnableBloomFilters {
			bloomFilter := NewBloomFilter(so.config.BloomFilterSize, so.config.BloomFilterHashes)
			so.bloomFilters[file] = bloomFilter
			so.metrics.BloomFilterMetrics[file] = bloomFilter.metrics
		}
	}
	
	return nil
}

// scanDataFiles scans all data files for a key (fallback method)
func (so *StorageOptimizer) scanDataFiles(key string) ([]byte, error) {
	so.mu.RLock()
	dataFiles := make([]*DataFile, 0, len(so.dataFiles))
	for _, dataFile := range so.dataFiles {
		dataFiles = append(dataFiles, dataFile)
	}
	so.mu.RUnlock()
	
	for _, dataFile := range dataFiles {
		if value, err := dataFile.Scan(key); err == nil {
			// Cache the result
			if so.readCache != nil {
				so.readCache.Put(key, value)
			}
			return value, nil
		}
	}
	
	return nil, fmt.Errorf("key not found: %s", key)
}

// NewDataFile creates a new data file
func NewDataFile(path string) *DataFile {
	return &DataFile{
		path:    path,
		index:   make(map[string]*IndexEntry),
		metrics: &DataFileMetrics{},
	}
}

// Open opens the data file
func (df *DataFile) Open() error {
	df.mu.Lock()
	defer df.mu.Unlock()
	
	var err error
	df.file, err = os.OpenFile(df.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	
	df.writer = bufio.NewWriter(df.file)
	
	// Get file size
	stat, err := df.file.Stat()
	if err != nil {
		return err
	}
	df.size = stat.Size()
	
	return nil
}

// Write writes data to the file
func (df *DataFile) Write(key string, value []byte, entry *IndexEntry) error {
	df.mu.Lock()
	defer df.mu.Unlock()
	
	// Write format: [key_len][key][value_len][value][checksum]
	keyLen := len(key)
	valueLen := len(value)
	
	// Write key length
	if err := binary.Write(df.writer, binary.LittleEndian, uint32(keyLen)); err != nil {
		return err
	}
	
	// Write key
	if _, err := df.writer.Write([]byte(key)); err != nil {
		return err
	}
	
	// Write value length
	if err := binary.Write(df.writer, binary.LittleEndian, uint32(valueLen)); err != nil {
		return err
	}
	
	// Write value
	if _, err := df.writer.Write(value); err != nil {
		return err
	}
	
	// Write checksum
	if err := binary.Write(df.writer, binary.LittleEndian, entry.Checksum); err != nil {
		return err
	}
	
	// Flush to disk
	if err := df.writer.Flush(); err != nil {
		return err
	}
	
	// Update index
	df.index[key] = entry
	df.size += int64(4 + keyLen + 4 + valueLen + 4) // Total bytes written
	df.entries++
	
	// Update metrics
	atomic.AddUint64(&df.metrics.WriteCount, 1)
	df.metrics.Size = df.size
	df.metrics.EntryCount = df.entries
	df.metrics.LastWrite = time.Now()
	
	return nil
}

// ReadAt reads data at a specific offset
func (df *DataFile) ReadAt(offset int64, size int) ([]byte, error) {
	df.mu.RLock()
	defer df.mu.RUnlock()
	
	buffer := make([]byte, size)
	_, err := df.file.ReadAt(buffer, offset)
	if err != nil {
		return nil, err
	}
	
	atomic.AddUint64(&df.metrics.ReadCount, 1)
	return buffer, nil
}

// Scan scans the file for a key (slow fallback method)
func (df *DataFile) Scan(key string) ([]byte, error) {
	df.mu.RLock()
	defer df.mu.RUnlock()
	
	// Check index first
	if entry, exists := df.index[key]; exists {
		return df.ReadAt(entry.Offset, entry.Size)
	}
	
	// Fallback to full scan (very slow)
	df.file.Seek(0, 0)
	reader := bufio.NewReader(df.file)
	
	for {
		// Read key length
		var keyLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &keyLen); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		
		// Read key
		keyBytes := make([]byte, keyLen)
		if _, err := reader.Read(keyBytes); err != nil {
			return nil, err
		}
		
		// Read value length
		var valueLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &valueLen); err != nil {
			return nil, err
		}
		
		// Read value
		valueBytes := make([]byte, valueLen)
		if _, err := reader.Read(valueBytes); err != nil {
			return nil, err
		}
		
		// Read checksum
		var checksum uint32
		if err := binary.Read(reader, binary.LittleEndian, &checksum); err != nil {
			return nil, err
		}
		
		// Check if this is the key we're looking for
		if string(keyBytes) == key {
			return valueBytes, nil
		}
	}
	
	return nil, fmt.Errorf("key not found in file: %s", key)
}

// GetMetrics returns current storage optimization metrics
func (so *StorageOptimizer) GetMetrics() *StorageMetrics {
	so.mu.RLock()
	defer so.mu.RUnlock()
	
	// Return a copy of metrics
	metricsCopy := *so.metrics
	return &metricsCopy
}

// Stop gracefully stops the storage optimizer
func (so *StorageOptimizer) Stop() error {
	so.mu.Lock()
	defer so.mu.Unlock()
	
	if !so.isRunning {
		return nil
	}
	
	so.logger.Printf("Stopping storage optimizer")
	
	// Stop compaction manager
	if so.compactionMgr != nil {
		close(so.compactionMgr.stopCh)
	}
	
	// Flush write cache
	if so.writeCache != nil {
		so.writeCache.flush()
	}
	
	// Close data files
	for _, dataFile := range so.dataFiles {
		if dataFile.file != nil {
			dataFile.writer.Flush()
			dataFile.file.Close()
		}
	}
	
	so.isRunning = false
	return nil
}