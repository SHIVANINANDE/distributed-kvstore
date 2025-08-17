package performance

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// MemoryOptimizer implements advanced memory allocation optimizations
type MemoryOptimizer struct {
	mu              sync.RWMutex
	logger          *log.Logger
	config          MemoryConfig
	allocators      map[string]*PoolAllocator
	arenas          map[string]*MemoryArena
	metrics         *MemoryMetrics
	gcOptimizer     *GCOptimizer
	isRunning       bool
	monitoringStop  chan struct{}
}

// MemoryConfig contains configuration for memory optimizations
type MemoryConfig struct {
	EnablePooling       bool          `json:"enable_pooling"`        // Enable object pooling
	EnableArenas        bool          `json:"enable_arenas"`         // Enable memory arenas
	EnableGCTuning      bool          `json:"enable_gc_tuning"`      // Enable GC optimizations
	PoolSizes          map[string]int `json:"pool_sizes"`           // Pool sizes by type
	ArenaSize          int64         `json:"arena_size"`            // Arena size in bytes
	MaxArenas          int           `json:"max_arenas"`            // Maximum number of arenas
	GCTargetPercent    int           `json:"gc_target_percent"`     // GC target percentage
	MonitoringInterval time.Duration `json:"monitoring_interval"`   // Memory monitoring interval
	PreallocateSize    int64         `json:"preallocate_size"`      // Preallocate memory size
}

// PoolAllocator implements object pooling for specific types
type PoolAllocator struct {
	mu         sync.Mutex
	name       string
	objectSize int
	pool       []unsafe.Pointer
	maxSize    int
	metrics    *PoolMetrics
	factory    func() unsafe.Pointer
	reset      func(unsafe.Pointer)
}

// MemoryArena implements arena-based memory allocation
type MemoryArena struct {
	mu        sync.Mutex
	id        string
	data      []byte
	size      int64
	offset    int64
	allocated int64
	chunks    []*ArenaChunk
	metrics   *ArenaMetrics
}

// ArenaChunk represents a chunk of memory within an arena
type ArenaChunk struct {
	offset int64
	size   int64
	inUse  bool
	owner  string
}

// GCOptimizer optimizes garbage collection behavior
type GCOptimizer struct {
	mu                sync.Mutex
	logger            *log.Logger
	config            MemoryConfig
	stats             runtime.MemStats
	lastGC            time.Time
	gcFrequency       time.Duration
	targetHeapSize    uint64
	metrics           *GCMetrics
	optimizationLevel int
}

// MemoryMetrics tracks memory optimization performance
type MemoryMetrics struct {
	TotalAllocations   uint64                    `json:"total_allocations"`
	TotalDeallocations uint64                    `json:"total_deallocations"`
	PoolHits           uint64                    `json:"pool_hits"`
	PoolMisses         uint64                    `json:"pool_misses"`
	ArenaAllocations   uint64                    `json:"arena_allocations"`
	ArenaUtilization   float64                   `json:"arena_utilization"`
	MemoryUsage        uint64                    `json:"memory_usage"`
	GCPauses           []time.Duration           `json:"gc_pauses"`
	PoolMetrics        map[string]*PoolMetrics   `json:"pool_metrics"`
	ArenaMetrics       map[string]*ArenaMetrics  `json:"arena_metrics"`
	GCMetrics          *GCMetrics                `json:"gc_metrics"`
}

// PoolMetrics tracks metrics for object pools
type PoolMetrics struct {
	Allocations   uint64  `json:"allocations"`
	Deallocations uint64  `json:"deallocations"`
	Hits          uint64  `json:"hits"`
	Misses        uint64  `json:"misses"`
	HitRate       float64 `json:"hit_rate"`
	CurrentSize   int     `json:"current_size"`
	MaxSize       int     `json:"max_size"`
	MemoryUsage   uint64  `json:"memory_usage"`
}

// ArenaMetrics tracks metrics for memory arenas
type ArenaMetrics struct {
	TotalSize     int64   `json:"total_size"`
	AllocatedSize int64   `json:"allocated_size"`
	Utilization   float64 `json:"utilization"`
	Fragmentations int    `json:"fragmentations"`
	Allocations   uint64  `json:"allocations"`
	Deallocations uint64  `json:"deallocations"`
}

// GCMetrics tracks garbage collection metrics
type GCMetrics struct {
	NumGC        uint32        `json:"num_gc"`
	PauseTotal   time.Duration `json:"pause_total"`
	PauseAvg     time.Duration `json:"pause_avg"`
	HeapSize     uint64        `json:"heap_size"`
	HeapInUse    uint64        `json:"heap_in_use"`
	GCFrequency  time.Duration `json:"gc_frequency"`
	LastGC       time.Time     `json:"last_gc"`
}

// NewMemoryOptimizer creates a new memory optimizer
func NewMemoryOptimizer(config MemoryConfig, logger *log.Logger) *MemoryOptimizer {
	if logger == nil {
		logger = log.New(log.Writer(), "[MEMORY] ", log.LstdFlags)
	}
	
	// Set default values
	if config.ArenaSize == 0 {
		config.ArenaSize = 64 * 1024 * 1024 // 64MB
	}
	if config.MaxArenas == 0 {
		config.MaxArenas = 16
	}
	if config.GCTargetPercent == 0 {
		config.GCTargetPercent = 100
	}
	if config.MonitoringInterval == 0 {
		config.MonitoringInterval = 5 * time.Second
	}
	if config.PoolSizes == nil {
		config.PoolSizes = map[string]int{
			"small":  1000,  // Small objects (< 1KB)
			"medium": 500,   // Medium objects (1KB - 64KB)  
			"large":  100,   // Large objects (> 64KB)
		}
	}
	
	mo := &MemoryOptimizer{
		logger:         logger,
		config:         config,
		allocators:     make(map[string]*PoolAllocator),
		arenas:         make(map[string]*MemoryArena),
		monitoringStop: make(chan struct{}),
		metrics: &MemoryMetrics{
			PoolMetrics:  make(map[string]*PoolMetrics),
			ArenaMetrics: make(map[string]*ArenaMetrics),
			GCMetrics:    &GCMetrics{},
		},
	}
	
	if config.EnableGCTuning {
		mo.gcOptimizer = NewGCOptimizer(config, logger)
		mo.metrics.GCMetrics = mo.gcOptimizer.metrics
	}
	
	return mo
}

// Start initializes the memory optimization system
func (mo *MemoryOptimizer) Start(ctx context.Context) error {
	mo.mu.Lock()
	defer mo.mu.Unlock()
	
	if mo.isRunning {
		return fmt.Errorf("memory optimizer already running")
	}
	
	mo.logger.Printf("Starting memory optimizer with config: pooling=%t, arenas=%t, gc_tuning=%t", 
		mo.config.EnablePooling, mo.config.EnableArenas, mo.config.EnableGCTuning)
	
	// Initialize allocators
	if mo.config.EnablePooling {
		mo.initializeAllocators()
	}
	
	// Initialize arenas
	if mo.config.EnableArenas {
		mo.initializeArenas()
	}
	
	// Start GC optimizer
	if mo.gcOptimizer != nil {
		if err := mo.gcOptimizer.Start(ctx); err != nil {
			return fmt.Errorf("failed to start GC optimizer: %w", err)
		}
	}
	
	// Preallocate memory if configured
	if mo.config.PreallocateSize > 0 {
		mo.preallocateMemory()
	}
	
	mo.isRunning = true
	
	// Start monitoring
	go mo.monitoringRoutine(ctx)
	
	return nil
}

// initializeAllocators creates object pool allocators
func (mo *MemoryOptimizer) initializeAllocators() {
	// Small object allocator (up to 1KB)
	mo.allocators["small"] = NewPoolAllocator("small", 1024, mo.config.PoolSizes["small"],
		func() unsafe.Pointer {
			return unsafe.Pointer(&[1024]byte{})
		},
		func(ptr unsafe.Pointer) {
			// Reset small object
			obj := (*[1024]byte)(ptr)
			for i := range obj {
				obj[i] = 0
			}
		})
	
	// Medium object allocator (up to 64KB)
	mo.allocators["medium"] = NewPoolAllocator("medium", 64*1024, mo.config.PoolSizes["medium"],
		func() unsafe.Pointer {
			return unsafe.Pointer(&[65536]byte{})
		},
		func(ptr unsafe.Pointer) {
			// Reset medium object
			obj := (*[65536]byte)(ptr)
			for i := range obj {
				obj[i] = 0
			}
		})
	
	// Large object allocator (up to 1MB)
	mo.allocators["large"] = NewPoolAllocator("large", 1024*1024, mo.config.PoolSizes["large"],
		func() unsafe.Pointer {
			return unsafe.Pointer(&[1048576]byte{})
		},
		func(ptr unsafe.Pointer) {
			// Reset large object
			obj := (*[1048576]byte)(ptr)
			for i := range obj {
				obj[i] = 0
			}
		})
	
	// Update metrics
	for name, allocator := range mo.allocators {
		mo.metrics.PoolMetrics[name] = allocator.metrics
	}
	
	mo.logger.Printf("Initialized %d object pool allocators", len(mo.allocators))
}

// initializeArenas creates memory arenas
func (mo *MemoryOptimizer) initializeArenas() {
	// Create initial arenas
	for i := 0; i < mo.config.MaxArenas/2; i++ {
		arenaID := fmt.Sprintf("arena_%d", i)
		arena := NewMemoryArena(arenaID, mo.config.ArenaSize)
		mo.arenas[arenaID] = arena
		mo.metrics.ArenaMetrics[arenaID] = arena.metrics
	}
	
	mo.logger.Printf("Initialized %d memory arenas", len(mo.arenas))
}

// preallocateMemory preallocates memory to reduce allocation overhead
func (mo *MemoryOptimizer) preallocateMemory() {
	// Allocate and immediately make it eligible for GC
	// This warms up the memory subsystem
	size := mo.config.PreallocateSize
	data := make([]byte, size)
	
	// Touch memory to ensure it's actually allocated
	for i := int64(0); i < size; i += 4096 {
		data[i] = byte(i % 256)
	}
	
	// Clear reference to allow GC
	data = nil
	runtime.GC()
	
	mo.logger.Printf("Preallocated %d bytes of memory", size)
}

// AllocateFromPool allocates an object from the appropriate pool
func (mo *MemoryOptimizer) AllocateFromPool(size int) (unsafe.Pointer, string) {
	var allocatorName string
	
	switch {
	case size <= 1024:
		allocatorName = "small"
	case size <= 64*1024:
		allocatorName = "medium"
	case size <= 1024*1024:
		allocatorName = "large"
	default:
		// Too large for pooling, use regular allocation
		return nil, ""
	}
	
	mo.mu.RLock()
	allocator, exists := mo.allocators[allocatorName]
	mo.mu.RUnlock()
	
	if !exists {
		return nil, ""
	}
	
	ptr := allocator.Allocate()
	atomic.AddUint64(&mo.metrics.TotalAllocations, 1)
	
	return ptr, allocatorName
}

// DeallocateToPool returns an object to the appropriate pool
func (mo *MemoryOptimizer) DeallocateToPool(ptr unsafe.Pointer, allocatorName string) {
	mo.mu.RLock()
	allocator, exists := mo.allocators[allocatorName]
	mo.mu.RUnlock()
	
	if !exists {
		return
	}
	
	allocator.Deallocate(ptr)
	atomic.AddUint64(&mo.metrics.TotalDeallocations, 1)
}

// AllocateFromArena allocates memory from an arena
func (mo *MemoryOptimizer) AllocateFromArena(size int64, owner string) ([]byte, string) {
	mo.mu.RLock()
	defer mo.mu.RUnlock()
	
	// Find an arena with enough space
	for arenaID, arena := range mo.arenas {
		if data := arena.Allocate(size, owner); data != nil {
			atomic.AddUint64(&mo.metrics.ArenaAllocations, 1)
			return data, arenaID
		}
	}
	
	// No arena has enough space, try to create a new one
	if len(mo.arenas) < mo.config.MaxArenas {
		arenaID := fmt.Sprintf("arena_%d", len(mo.arenas))
		arena := NewMemoryArena(arenaID, mo.config.ArenaSize)
		mo.arenas[arenaID] = arena
		mo.metrics.ArenaMetrics[arenaID] = arena.metrics
		
		if data := arena.Allocate(size, owner); data != nil {
			atomic.AddUint64(&mo.metrics.ArenaAllocations, 1)
			return data, arenaID
		}
	}
	
	return nil, ""
}

// DeallocateFromArena deallocates memory from an arena
func (mo *MemoryOptimizer) DeallocateFromArena(arenaID string, offset int64) {
	mo.mu.RLock()
	arena, exists := mo.arenas[arenaID]
	mo.mu.RUnlock()
	
	if !exists {
		return
	}
	
	arena.Deallocate(offset)
}

// NewPoolAllocator creates a new pool allocator
func NewPoolAllocator(name string, objectSize, maxSize int, factory func() unsafe.Pointer, reset func(unsafe.Pointer)) *PoolAllocator {
	return &PoolAllocator{
		name:       name,
		objectSize: objectSize,
		pool:       make([]unsafe.Pointer, 0, maxSize),
		maxSize:    maxSize,
		factory:    factory,
		reset:      reset,
		metrics: &PoolMetrics{
			MaxSize: maxSize,
		},
	}
}

// Allocate gets an object from the pool
func (pa *PoolAllocator) Allocate() unsafe.Pointer {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	
	atomic.AddUint64(&pa.metrics.Allocations, 1)
	
	if len(pa.pool) > 0 {
		// Get from pool
		ptr := pa.pool[len(pa.pool)-1]
		pa.pool = pa.pool[:len(pa.pool)-1]
		pa.metrics.CurrentSize = len(pa.pool)
		
		atomic.AddUint64(&pa.metrics.Hits, 1)
		pa.updateHitRate()
		
		return ptr
	}
	
	// Pool is empty, create new object
	atomic.AddUint64(&pa.metrics.Misses, 1)
	pa.updateHitRate()
	
	return pa.factory()
}

// Deallocate returns an object to the pool
func (pa *PoolAllocator) Deallocate(ptr unsafe.Pointer) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	
	atomic.AddUint64(&pa.metrics.Deallocations, 1)
	
	if len(pa.pool) < pa.maxSize {
		// Reset object before returning to pool
		if pa.reset != nil {
			pa.reset(ptr)
		}
		
		pa.pool = append(pa.pool, ptr)
		pa.metrics.CurrentSize = len(pa.pool)
		pa.metrics.MemoryUsage = uint64(len(pa.pool) * pa.objectSize)
	}
	// If pool is full, let object be garbage collected
}

// updateHitRate calculates the hit rate for the pool
func (pa *PoolAllocator) updateHitRate() {
	total := pa.metrics.Hits + pa.metrics.Misses
	if total > 0 {
		pa.metrics.HitRate = float64(pa.metrics.Hits) / float64(total)
	}
}

// NewMemoryArena creates a new memory arena
func NewMemoryArena(id string, size int64) *MemoryArena {
	return &MemoryArena{
		id:     id,
		data:   make([]byte, size),
		size:   size,
		chunks: make([]*ArenaChunk, 0),
		metrics: &ArenaMetrics{
			TotalSize: size,
		},
	}
}

// Allocate allocates memory from the arena
func (ma *MemoryArena) Allocate(size int64, owner string) []byte {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	
	// Align size to 8-byte boundary
	alignedSize := (size + 7) & ^int64(7)
	
	// Find a free chunk or allocate at the end
	offset := ma.findFreeSpace(alignedSize)
	if offset == -1 {
		return nil
	}
	
	// Create chunk
	chunk := &ArenaChunk{
		offset: offset,
		size:   alignedSize,
		inUse:  true,
		owner:  owner,
	}
	
	ma.chunks = append(ma.chunks, chunk)
	ma.allocated += alignedSize
	
	// Update metrics
	atomic.AddUint64(&ma.metrics.Allocations, 1)
	ma.metrics.AllocatedSize = ma.allocated
	ma.metrics.Utilization = float64(ma.allocated) / float64(ma.size)
	
	return ma.data[offset : offset+alignedSize]
}

// findFreeSpace finds free space in the arena
func (ma *MemoryArena) findFreeSpace(size int64) int64 {
	// Simple first-fit allocation
	if ma.offset+size <= ma.size {
		offset := ma.offset
		ma.offset += size
		return offset
	}
	
	// Try to find freed space
	for _, chunk := range ma.chunks {
		if !chunk.inUse && chunk.size >= size {
			chunk.inUse = true
			chunk.size = size // May create fragmentation
			return chunk.offset
		}
	}
	
	return -1
}

// Deallocate frees memory in the arena
func (ma *MemoryArena) Deallocate(offset int64) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	
	// Find and mark chunk as free
	for _, chunk := range ma.chunks {
		if chunk.offset == offset && chunk.inUse {
			chunk.inUse = false
			ma.allocated -= chunk.size
			
			// Update metrics
			atomic.AddUint64(&ma.metrics.Deallocations, 1)
			ma.metrics.AllocatedSize = ma.allocated
			ma.metrics.Utilization = float64(ma.allocated) / float64(ma.size)
			
			break
		}
	}
	
	// Coalesce free chunks to reduce fragmentation
	ma.coalesceChunks()
}

// coalesceChunks merges adjacent free chunks
func (ma *MemoryArena) coalesceChunks() {
	// Simple coalescing algorithm
	for i := 0; i < len(ma.chunks)-1; i++ {
		current := ma.chunks[i]
		next := ma.chunks[i+1]
		
		if !current.inUse && !next.inUse && current.offset+current.size == next.offset {
			// Merge chunks
			current.size += next.size
			ma.chunks = append(ma.chunks[:i+1], ma.chunks[i+2:]...)
			ma.metrics.Fragmentations--
			i-- // Recheck current position
		}
	}
}

// NewGCOptimizer creates a new GC optimizer
func NewGCOptimizer(config MemoryConfig, logger *log.Logger) *GCOptimizer {
	return &GCOptimizer{
		logger: logger,
		config: config,
		metrics: &GCMetrics{},
	}
}

// Start initializes the GC optimizer
func (gc *GCOptimizer) Start(ctx context.Context) error {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	
	// Set GC target percentage
	runtime.GOMAXPROCS(runtime.NumCPU())
	runtime.SetGCPercent(gc.config.GCTargetPercent)
	
	gc.logger.Printf("Started GC optimizer with target percent: %d", gc.config.GCTargetPercent)
	
	// Start monitoring
	go gc.monitorGC(ctx)
	
	return nil
}

// monitorGC monitors garbage collection performance
func (gc *GCOptimizer) monitorGC(ctx context.Context) {
	ticker := time.NewTicker(gc.config.MonitoringInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gc.updateGCMetrics()
		}
	}
}

// updateGCMetrics updates GC performance metrics
func (gc *GCOptimizer) updateGCMetrics() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	
	runtime.ReadMemStats(&gc.stats)
	
	gc.metrics.NumGC = gc.stats.NumGC
	gc.metrics.PauseTotal = time.Duration(gc.stats.PauseTotalNs)
	gc.metrics.HeapSize = gc.stats.HeapSys
	gc.metrics.HeapInUse = gc.stats.HeapInuse
	
	if gc.stats.NumGC > 0 {
		gc.metrics.PauseAvg = time.Duration(gc.stats.PauseTotalNs / uint64(gc.stats.NumGC))
	}
	
	// Calculate GC frequency
	now := time.Now()
	if !gc.lastGC.IsZero() {
		gc.metrics.GCFrequency = now.Sub(gc.lastGC)
	}
	gc.lastGC = now
	gc.metrics.LastGC = now
	
	// Adaptive GC tuning
	gc.adaptiveGCTuning()
}

// adaptiveGCTuning adjusts GC parameters based on performance
func (gc *GCOptimizer) adaptiveGCTuning() {
	avgPause := gc.metrics.PauseAvg
	heapGrowth := float64(gc.stats.HeapInuse) / float64(gc.stats.HeapSys)
	
	// Adjust GC target based on pause times and heap growth
	currentTarget := runtime.GOMAXPROCS(-1)
	newTarget := currentTarget
	
	if avgPause > 10*time.Millisecond {
		// Pause times too high, be more aggressive
		newTarget = int(float64(currentTarget) * 0.9)
		if newTarget < 50 {
			newTarget = 50
		}
	} else if avgPause < 1*time.Millisecond && heapGrowth < 0.7 {
		// Pause times low and heap not growing much, be less aggressive
		newTarget = int(float64(currentTarget) * 1.1)
		if newTarget > 200 {
			newTarget = 200
		}
	}
	
	if newTarget != currentTarget {
		runtime.SetGCPercent(newTarget)
		gc.logger.Printf("Adjusted GC target percent from %d to %d (pause: %v, heap: %.2f)", 
			currentTarget, newTarget, avgPause, heapGrowth)
	}
}

// monitoringRoutine performs periodic memory monitoring
func (mo *MemoryOptimizer) monitoringRoutine(ctx context.Context) {
	ticker := time.NewTicker(mo.config.MonitoringInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mo.monitoringStop:
			return
		case <-ticker.C:
			mo.updateMetrics()
		}
	}
}

// updateMetrics updates memory optimization metrics
func (mo *MemoryOptimizer) updateMetrics() {
	mo.mu.RLock()
	defer mo.mu.RUnlock()
	
	// Update pool metrics
	totalHits := uint64(0)
	totalMisses := uint64(0)
	
	for _, allocator := range mo.allocators {
		totalHits += allocator.metrics.Hits
		totalMisses += allocator.metrics.Misses
	}
	
	mo.metrics.PoolHits = totalHits
	mo.metrics.PoolMisses = totalMisses
	
	// Update arena utilization
	totalUtilization := 0.0
	arenaCount := 0
	
	for _, arena := range mo.arenas {
		totalUtilization += arena.metrics.Utilization
		arenaCount++
	}
	
	if arenaCount > 0 {
		mo.metrics.ArenaUtilization = totalUtilization / float64(arenaCount)
	}
	
	// Update memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	mo.metrics.MemoryUsage = memStats.Alloc
}

// GetMetrics returns current memory optimization metrics
func (mo *MemoryOptimizer) GetMetrics() *MemoryMetrics {
	mo.mu.RLock()
	defer mo.mu.RUnlock()
	
	// Return a copy of metrics
	metricsCopy := *mo.metrics
	return &metricsCopy
}

// ForceGC forces a garbage collection cycle
func (mo *MemoryOptimizer) ForceGC() {
	if mo.gcOptimizer != nil {
		mo.logger.Printf("Forcing garbage collection")
		runtime.GC()
		
		// Update metrics after forced GC
		mo.gcOptimizer.updateGCMetrics()
	}
}

// CompactArenas compacts memory arenas to reduce fragmentation
func (mo *MemoryOptimizer) CompactArenas() {
	mo.mu.Lock()
	defer mo.mu.Unlock()
	
	mo.logger.Printf("Compacting memory arenas")
	
	for arenaID, arena := range mo.arenas {
		arena.mu.Lock()
		
		// Simple compaction: recreate arena if fragmentation is high
		if len(arena.chunks) > 100 { // Arbitrary threshold
			newArena := NewMemoryArena(arenaID, arena.size)
			
			// Copy active chunks to new arena
			for _, chunk := range arena.chunks {
				if chunk.inUse {
					newData := newArena.Allocate(chunk.size, chunk.owner)
					if newData != nil {
						copy(newData, arena.data[chunk.offset:chunk.offset+chunk.size])
					}
				}
			}
			
			mo.arenas[arenaID] = newArena
			mo.metrics.ArenaMetrics[arenaID] = newArena.metrics
		}
		
		arena.mu.Unlock()
	}
}

// GetMemoryStats returns detailed memory statistics
func (mo *MemoryOptimizer) GetMemoryStats() *MemoryStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	stats := &MemoryStats{
		Alloc:        memStats.Alloc,
		TotalAlloc:   memStats.TotalAlloc,
		Sys:          memStats.Sys,
		NumGC:        memStats.NumGC,
		PauseTotalNs: memStats.PauseTotalNs,
		HeapAlloc:    memStats.HeapAlloc,
		HeapSys:      memStats.HeapSys,
		HeapInuse:    memStats.HeapInuse,
		StackInuse:   memStats.StackInuse,
		StackSys:     memStats.StackSys,
	}
	
	return stats
}

// MemoryStats contains detailed memory statistics
type MemoryStats struct {
	Alloc        uint64 `json:"alloc"`
	TotalAlloc   uint64 `json:"total_alloc"`
	Sys          uint64 `json:"sys"`
	NumGC        uint32 `json:"num_gc"`
	PauseTotalNs uint64 `json:"pause_total_ns"`
	HeapAlloc    uint64 `json:"heap_alloc"`
	HeapSys      uint64 `json:"heap_sys"`
	HeapInuse    uint64 `json:"heap_inuse"`
	StackInuse   uint64 `json:"stack_inuse"`
	StackSys     uint64 `json:"stack_sys"`
}

// Stop gracefully stops the memory optimizer
func (mo *MemoryOptimizer) Stop() error {
	mo.mu.Lock()
	defer mo.mu.Unlock()
	
	if !mo.isRunning {
		return nil
	}
	
	mo.logger.Printf("Stopping memory optimizer")
	
	// Stop monitoring
	close(mo.monitoringStop)
	
	// Stop GC optimizer
	if mo.gcOptimizer != nil {
		// GC optimizer doesn't need explicit stopping
	}
	
	mo.isRunning = false
	return nil
}