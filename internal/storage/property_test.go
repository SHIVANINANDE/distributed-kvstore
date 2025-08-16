package storage

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestStorageEngineProperties(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Property 1: PUT then GET should return the same value
	properties.Property("PUT then GET returns same value", prop.ForAll(
		func(key string, value string) bool {
			// Create test storage engine
			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT the value
			err = engine.Put([]byte(key), []byte(value))
			if err != nil {
				return false
			}

			// GET the value
			retrievedValue, err := engine.Get([]byte(key))
			if err != nil {
				return false
			}

			// Check if retrieved value matches original
			return string(retrievedValue) == value
		},
		gen.Identifier(),     // Generate valid identifiers for keys
		gen.AlphaString(),    // Generate alpha strings for values
	))

	// Property 2: DELETE after PUT should make key non-existent
	properties.Property("DELETE after PUT removes key", prop.ForAll(
		func(key string, value string) bool {
			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT the value
			err = engine.Put([]byte(key), []byte(value))
			if err != nil {
				return false
			}

			// DELETE the key
			err = engine.Delete([]byte(key))
			if err != nil {
				return false
			}

			// GET should return error (key not found)
			_, err = engine.Get([]byte(key))
			return err != nil // Should return error for non-existent key
		},
		gen.Identifier(),
		gen.AlphaString(),
	))

	// Property 3: EXISTS should return true after PUT and false after DELETE
	properties.Property("EXISTS reflects key presence correctly", prop.ForAll(
		func(key string, value string) bool {
			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// Initially key should not exist
			exists, err := engine.Exists([]byte(key))
			if err != nil || exists {
				return false
			}

			// PUT the value
			err = engine.Put([]byte(key), []byte(value))
			if err != nil {
				return false
			}

			// Now key should exist
			exists, err = engine.Exists([]byte(key))
			if err != nil || !exists {
				return false
			}

			// DELETE the key
			err = engine.Delete([]byte(key))
			if err != nil {
				return false
			}

			// Key should not exist anymore
			exists, err = engine.Exists([]byte(key))
			if err != nil || exists {
				return false
			}

			return true
		},
		gen.Identifier(),
		gen.AlphaString(),
	))

	// Property 4: Multiple PUT operations with same key should overwrite
	properties.Property("Multiple PUTs overwrite previous values", prop.ForAll(
		func(key string, value1 string, value2 string) bool {
			if value1 == value2 {
				return true // Skip when values are the same
			}

			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT first value
			err = engine.Put([]byte(key), []byte(value1))
			if err != nil {
				return false
			}

			// PUT second value (should overwrite)
			err = engine.Put([]byte(key), []byte(value2))
			if err != nil {
				return false
			}

			// GET should return the second value
			retrievedValue, err := engine.Get([]byte(key))
			if err != nil {
				return false
			}

			return string(retrievedValue) == value2
		},
		gen.Identifier(),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property 5: Key overwrite behavior is consistent
	properties.Property("Key overwrite behavior is consistent", prop.ForAll(
		func(key string, value1 string, value2 string, value3 string) bool {
			if value1 == value2 || value2 == value3 || value1 == value3 {
				return true // Skip when values are the same
			}

			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT first value
			err = engine.Put([]byte(key), []byte(value1))
			if err != nil {
				return false
			}

			// PUT second value (should overwrite)
			err = engine.Put([]byte(key), []byte(value2))
			if err != nil {
				return false
			}

			// PUT third value (should overwrite again)
			err = engine.Put([]byte(key), []byte(value3))
			if err != nil {
				return false
			}

			// GET should return the third value
			retrievedValue, err := engine.Get([]byte(key))
			if err != nil {
				return false
			}

			return string(retrievedValue) == value3
		},
		gen.Identifier(),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property 6: Multiple individual operations consistency
	properties.Property("Multiple individual PUT operations work correctly", prop.ForAll(
		func(keys []string, values []string) bool {
			if len(keys) == 0 || len(values) == 0 || len(keys) != len(values) {
				return true // Skip invalid inputs
			}

			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT all key-value pairs individually
			for i, key := range keys {
				err = engine.Put([]byte(key), []byte(values[i]))
				if err != nil {
					return false
				}
			}

			// Verify each key-value pair individually
			for i, key := range keys {
				retrievedValue, err := engine.Get([]byte(key))
				if err != nil {
					return false
				}
				if string(retrievedValue) != values[i] {
					return false
				}
			}

			return true
		},
		gen.SliceOfN(3, gen.Identifier()),
		gen.SliceOfN(3, gen.AlphaString()),
	))

	// Property 7: List operations return consistent results
	properties.Property("List operations return stored keys", prop.ForAll(
		func(prefix string, keys []string, values []string) bool {
			if len(keys) == 0 || len(values) == 0 || len(keys) != len(values) {
				return true // Skip invalid inputs
			}

			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT all key-value pairs with prefix
			expectedKeysWithPrefix := make(map[string]string)
			for i, key := range keys {
				fullKey := prefix + key
				err = engine.Put([]byte(fullKey), []byte(values[i]))
				if err != nil {
					return false
				}
				expectedKeysWithPrefix[fullKey] = values[i]
			}

			// List keys with prefix
			listedKeyMap, err := engine.List([]byte(prefix))
			if err != nil {
				return false
			}

			// Convert byte slices to strings for comparison
			listedKeys := make(map[string]string)
			for key, value := range listedKeyMap {
				listedKeys[key] = string(value)
			}

			// Check if all expected keys are present
			for expectedKey, expectedValue := range expectedKeysWithPrefix {
				if listedValue, found := listedKeys[expectedKey]; !found || listedValue != expectedValue {
					return false
				}
			}

			return true
		},
		gen.RegexMatch("^[a-z]{1,3}$"), // Short alpha prefix
		gen.SliceOfN(2, gen.Identifier()),
		gen.SliceOfN(2, gen.AlphaString()),
	))

	// Run all properties
	properties.TestingRun(t)
}

func TestStorageEnginePropertyEdgeCases(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Property: Empty values should be handled correctly
	properties.Property("Empty values can be stored and retrieved", prop.ForAll(
		func(key string) bool {
			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// PUT empty value
			err = engine.Put([]byte(key), []byte(""))
			if err != nil {
				return false
			}

			// GET should return empty value
			retrievedValue, err := engine.Get([]byte(key))
			if err != nil {
				return false
			}

			return len(retrievedValue) == 0
		},
		gen.Identifier(),
	))

	// Property: Large values can be stored and retrieved
	properties.Property("Large values can be stored and retrieved", prop.ForAll(
		func(key string, size uint) bool {
			if size > 1000 { // Limit size for test performance
				size = 1000
			}
			if size == 0 {
				size = 1
			}

			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// Create large value
			largeValue := make([]byte, size)
			for i := range largeValue {
				largeValue[i] = byte('A' + (i % 26))
			}

			// PUT large value
			err = engine.Put([]byte(key), largeValue)
			if err != nil {
				return false
			}

			// GET should return same large value
			retrievedValue, err := engine.Get([]byte(key))
			if err != nil {
				return false
			}

			if len(retrievedValue) != len(largeValue) {
				return false
			}

			for i := range largeValue {
				if retrievedValue[i] != largeValue[i] {
					return false
				}
			}

			return true
		},
		gen.Identifier(),
		gen.UInt(),
	))

	properties.TestingRun(t)
}

func TestStorageEngineConcurrencyProperties(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Property: Concurrent operations don't corrupt data
	properties.Property("Concurrent PUTs don't corrupt data", prop.ForAll(
		func(baseKey string, values []string) bool {
			if len(values) == 0 || len(values) > 10 {
				return true // Skip invalid inputs
			}

			config := Config{
				DataPath:   "",
				InMemory:   true,
				SyncWrites: false,
				ValueLogGC: false,
			}
			
			engine, err := NewEngine(config)
			if err != nil {
				return false
			}
			defer engine.Close()

			// Create unique keys for each value
			keys := make([]string, len(values))
			for i := range values {
				keys[i] = baseKey + "_" + string(rune('A'+i))
			}

			// Perform concurrent PUTs
			done := make(chan bool, len(values))
			for i, key := range keys {
				go func(k string, v string) {
					err := engine.Put([]byte(k), []byte(v))
					done <- err == nil
				}(key, values[i])
			}

			// Wait for all operations to complete
			for i := 0; i < len(values); i++ {
				if !<-done {
					return false
				}
			}

			// Verify all values are correct
			for i, key := range keys {
				retrievedValue, err := engine.Get([]byte(key))
				if err != nil {
					return false
				}
				if string(retrievedValue) != values[i] {
					return false
				}
			}

			return true
		},
		gen.Identifier(),
		gen.SliceOfN(2, gen.AlphaString()),
	))

	properties.TestingRun(t)
}