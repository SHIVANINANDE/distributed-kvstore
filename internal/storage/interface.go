package storage

import "time"

type StorageEngine interface {
	Put(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Delete(key []byte) error
	Exists(key []byte) (bool, error)
	List(prefix []byte) (map[string][]byte, error)
	Close() error
	Backup(path string) error
	Restore(path string) error
	Stats() map[string]interface{}
	
	// Batch operations for better concurrency
	BatchPut(items []KeyValue) error
	BatchGet(keys [][]byte) ([]KeyValue, error)
	BatchDelete(keys [][]byte) error
}

// KeyValue represents a key-value pair
type KeyValue struct {
	Key   []byte
	Value []byte
	Found bool
}

type StorageConfig interface {
	GetDataPath() string
	IsInMemory() bool
	IsSyncWrites() bool
	IsValueLogGC() bool
	GetGCInterval() time.Duration
}