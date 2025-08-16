package storage

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
)

type Engine struct {
	db *badger.DB
	// Performance optimization pools
	readTxnPool   chan *badger.Txn
	writeTxnPool  chan *badger.Txn
	maxPoolSize   int
}

var _ StorageEngine = (*Engine)(nil)

type Config struct {
	DataPath    string
	InMemory    bool
	SyncWrites  bool
	ValueLogGC  bool
	GCInterval  time.Duration
	// WAL settings
	WALEnabled      bool
	WALThreshold    int64  // WAL file size threshold for rotation
	MaxWALFiles     int    // Maximum number of WAL files to keep
	FSyncThreshold  int    // Number of writes before fsync
	// Cache settings
	CacheEnabled    bool
	CacheSize       int
	CacheTTL        time.Duration
	CacheCleanupInterval time.Duration
}

var _ StorageConfig = (*Config)(nil)

func (c *Config) GetDataPath() string {
	return c.DataPath
}

func (c *Config) IsInMemory() bool {
	return c.InMemory
}

func (c *Config) IsSyncWrites() bool {
	return c.SyncWrites
}

func (c *Config) IsValueLogGC() bool {
	return c.ValueLogGC
}

func (c *Config) GetGCInterval() time.Duration {
	return c.GCInterval
}

func NewEngine(config Config) (*Engine, error) {
	opts := badger.DefaultOptions(config.DataPath)
	
	if config.InMemory {
		opts = opts.WithInMemory(true)
	}
	
	// WAL and durability settings
	opts = opts.WithSyncWrites(config.SyncWrites)
	if config.WALEnabled {
		// Enable value log (WAL) with custom thresholds
		opts = opts.WithValueThreshold(1) // Store all values in value log (WAL)
		if config.WALThreshold > 0 {
			opts = opts.WithValueLogFileSize(config.WALThreshold)
		}
		if config.MaxWALFiles > 0 {
			opts = opts.WithValueLogMaxEntries(uint32(config.MaxWALFiles * 1000))
		}
		if config.FSyncThreshold > 0 {
			// Use periodic syncing for better performance with manual sync control
			opts = opts.WithSyncWrites(false)
			// Note: Manual fsync will be handled in batch operations
		}
	}
	
	opts = opts.WithLogger(nil) // Disable badger's default logger
	
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger database: %w", err)
	}

	engine := &Engine{
		db:          db,
		maxPoolSize: 50, // Configurable pool size for concurrent operations
	}
	
	// Initialize transaction pools for better concurrency
	engine.readTxnPool = make(chan *badger.Txn, engine.maxPoolSize)
	engine.writeTxnPool = make(chan *badger.Txn, engine.maxPoolSize)
	
	if config.ValueLogGC && !config.InMemory {
		go engine.runGC(config.GCInterval)
	}
	
	return engine, nil
}

func (e *Engine) Put(key, value []byte) error {
	return e.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

func (e *Engine) Get(key []byte) ([]byte, error) {
	var value []byte
	err := e.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		
		value, err = item.ValueCopy(nil)
		return err
	})
	
	if err == badger.ErrKeyNotFound {
		return nil, ErrKeyNotFound
	}
	
	return value, err
}

func (e *Engine) Delete(key []byte) error {
	return e.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (e *Engine) Exists(key []byte) (bool, error) {
	err := e.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})
	
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	
	return err == nil, err
}

func (e *Engine) List(prefix []byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	
	err := e.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			
			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			
			result[string(key)] = value
		}
		
		return nil
	})
	
	return result, err
}

func (e *Engine) Close() error {
	return e.db.Close()
}

func (e *Engine) Backup(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()
	
	_, err = e.db.Backup(file, 0)
	return err
}

func (e *Engine) Restore(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()
	
	return e.db.Load(file, 256)
}

func (e *Engine) Stats() map[string]interface{} {
	lsm := e.db.LevelsToString()
	tables := e.db.Tables()
	lsmSize, vlogSize := e.db.Size()
	
	return map[string]interface{}{
		"lsm_structure": lsm,
		"tables":        tables,
		"lsm_size":      lsmSize,
		"vlog_size":     vlogSize,
		"total_size":    lsmSize + vlogSize,
	}
}

// BatchPut performs multiple put operations in a single transaction
func (e *Engine) BatchPut(items []KeyValue) error {
	return e.db.Update(func(txn *badger.Txn) error {
		for _, item := range items {
			if err := txn.Set(item.Key, item.Value); err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchGet retrieves multiple values efficiently
func (e *Engine) BatchGet(keys [][]byte) ([]KeyValue, error) {
	results := make([]KeyValue, len(keys))
	
	err := e.db.View(func(txn *badger.Txn) error {
		for i, key := range keys {
			item, err := txn.Get(key)
			if err == badger.ErrKeyNotFound {
				results[i] = KeyValue{Key: key, Value: nil, Found: false}
				continue
			}
			if err != nil {
				return err
			}
			
			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			
			results[i] = KeyValue{Key: key, Value: value, Found: true}
		}
		return nil
	})
	
	return results, err
}

// BatchDelete performs multiple delete operations in a single transaction
func (e *Engine) BatchDelete(keys [][]byte) error {
	return e.db.Update(func(txn *badger.Txn) error {
		for _, key := range keys {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

func (e *Engine) runGC(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for range ticker.C {
		again := true
		for again {
			err := e.db.RunValueLogGC(0.7)
			again = err == nil
		}
		
		log.Printf("BadgerDB garbage collection completed")
	}
}

var ErrKeyNotFound = fmt.Errorf("key not found")