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
}

type StorageConfig interface {
	GetDataPath() string
	IsInMemory() bool
	IsSyncWrites() bool
	IsValueLogGC() bool
	GetGCInterval() time.Duration
}