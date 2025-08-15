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
}

var _ StorageEngine = (*Engine)(nil)

type Config struct {
	DataPath    string
	InMemory    bool
	SyncWrites  bool
	ValueLogGC  bool
	GCInterval  time.Duration
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
	
	opts = opts.WithSyncWrites(config.SyncWrites)
	opts = opts.WithLogger(nil) // Disable badger's default logger
	
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger database: %w", err)
	}

	engine := &Engine{db: db}
	
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