package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		wantErr bool
	}{
		{
			name: "in-memory engine",
			config: Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			},
			wantErr: false,
		},
		{
			name: "file-based engine",
			config: Config{
				DataPath:   filepath.Join(os.TempDir(), "test-badger"),
				InMemory:   false,
				SyncWrites: true,
				ValueLogGC: true,
				GCInterval: 1 * time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewEngine(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEngine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if engine != nil {
				defer engine.Close()
				if !tt.config.InMemory {
					defer os.RemoveAll(tt.config.DataPath)
				}
			}
		})
	}
}

func TestEngine_Put_Get(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"simple key-value", "key1", "value1"},
		{"empty value", "key2", ""},
		{"unicode key-value", "키", "값"},
		{"special chars", "key!@#$%", "value!@#$%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.Put([]byte(tt.key), []byte(tt.value))
			if err != nil {
				t.Errorf("Put() error = %v", err)
				return
			}

			got, err := engine.Get([]byte(tt.key))
			if err != nil {
				t.Errorf("Get() error = %v", err)
				return
			}

			if string(got) != tt.value {
				t.Errorf("Get() = %v, want %v", string(got), tt.value)
			}
		})
	}
}

func TestEngine_Get_NotFound(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	_, err := engine.Get([]byte("nonexistent"))
	if err != ErrKeyNotFound {
		t.Errorf("Get() error = %v, want %v", err, ErrKeyNotFound)
	}
}

func TestEngine_Delete(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	key := []byte("delete-test")
	value := []byte("delete-value")

	err := engine.Put(key, value)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	err = engine.Delete(key)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = engine.Get(key)
	if err != ErrKeyNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, ErrKeyNotFound)
	}
}

func TestEngine_Exists(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	key := []byte("exists-test")
	value := []byte("exists-value")

	exists, err := engine.Exists(key)
	if err != nil {
		t.Errorf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true for nonexistent key")
	}

	err = engine.Put(key, value)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	exists, err = engine.Exists(key)
	if err != nil {
		t.Errorf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false for existing key")
	}
}

func TestEngine_List(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	testData := map[string]string{
		"prefix:key1": "value1",
		"prefix:key2": "value2",
		"prefix:key3": "value3",
		"other:key1":  "other1",
	}

	for key, value := range testData {
		err := engine.Put([]byte(key), []byte(value))
		if err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}

	result, err := engine.List([]byte("prefix:"))
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("List() returned %d items, want 3", len(result))
	}

	for key, expectedValue := range testData {
		if key[:7] == "prefix:" {
			if gotValue, exists := result[key]; !exists {
				t.Errorf("List() missing key %s", key)
			} else if string(gotValue) != expectedValue {
				t.Errorf("List() key %s = %s, want %s", key, string(gotValue), expectedValue)
			}
		}
	}
}

func TestEngine_Stats(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	err := engine.Put([]byte("stats-test"), []byte("stats-value"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	stats := engine.Stats()
	
	expectedFields := []string{"lsm_structure", "tables", "lsm_size", "vlog_size", "total_size"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("Stats() missing field %s", field)
		}
	}
}

func TestConfig_Interface(t *testing.T) {
	config := Config{
		DataPath:   "/test/path",
		InMemory:   true,
		SyncWrites: true,
		ValueLogGC: true,
		GCInterval: 5 * time.Minute,
	}

	if config.GetDataPath() != "/test/path" {
		t.Errorf("GetDataPath() = %s, want /test/path", config.GetDataPath())
	}

	if !config.IsInMemory() {
		t.Error("IsInMemory() = false, want true")
	}

	if !config.IsSyncWrites() {
		t.Error("IsSyncWrites() = false, want true")
	}

	if !config.IsValueLogGC() {
		t.Error("IsValueLogGC() = false, want true")
	}

	if config.GetGCInterval() != 5*time.Minute {
		t.Errorf("GetGCInterval() = %v, want 5m", config.GetGCInterval())
	}
}

func TestEngine_Update(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	key := []byte("update-test")
	originalValue := []byte("original")
	updatedValue := []byte("updated")

	err := engine.Put(key, originalValue)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	err = engine.Put(key, updatedValue)
	if err != nil {
		t.Errorf("Put() update error = %v", err)
	}

	got, err := engine.Get(key)
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}

	if string(got) != string(updatedValue) {
		t.Errorf("Get() after update = %s, want %s", string(got), string(updatedValue))
	}
}

func setupTestEngine(t *testing.T) (*Engine, func()) {
	config := Config{
		DataPath:   "",
		InMemory:   true,
		SyncWrites: false,
		ValueLogGC: false,
	}

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("Failed to create test engine: %v", err)
	}

	return engine, func() {
		engine.Close()
	}
}