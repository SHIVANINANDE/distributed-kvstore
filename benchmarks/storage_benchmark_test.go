package benchmarks

import (
	"distributed-kvstore/internal/storage"
	"distributed-kvstore/internal/testutil"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Storage Engine Benchmarks

func BenchmarkStorageEngine_Put(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Generate test data
	keys := make([][]byte, b.N)
	values := make([][]byte, b.N)
	
	for i := 0; i < b.N; i++ {
		keys[i] = []byte(fmt.Sprintf("benchmark-key-%d", i))
		values[i] = []byte(fmt.Sprintf("benchmark-value-%d-%s", i, testutil.GenerateRandomString(100)))
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		err := engine.Put(keys[i], values[i])
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
	}
}

func BenchmarkStorageEngine_Get(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	numKeys := 10000
	keys := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("get-benchmark-key-%d", i))
		value := []byte(fmt.Sprintf("get-benchmark-value-%d", i))
		keys[i] = key
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := keys[i%numKeys]
		_, err := engine.Get(key)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

func BenchmarkStorageEngine_Delete(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	keys := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("delete-benchmark-key-%d", i))
		value := []byte(fmt.Sprintf("delete-benchmark-value-%d", i))
		keys[i] = key
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		err := engine.Delete(keys[i])
		if err != nil {
			b.Fatalf("Delete failed: %v", err)
		}
	}
}

func BenchmarkStorageEngine_Exists(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	numKeys := 10000
	keys := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("exists-benchmark-key-%d", i))
		value := []byte(fmt.Sprintf("exists-benchmark-value-%d", i))
		keys[i] = key
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := keys[i%numKeys]
		_, err := engine.Exists(key)
		if err != nil {
			b.Fatalf("Exists failed: %v", err)
		}
	}
}

func BenchmarkStorageEngine_List(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	prefix := "list-benchmark"
	numKeys := 1000
	
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("%s-key-%d", prefix, i))
		value := []byte(fmt.Sprintf("list-benchmark-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := engine.List([]byte(prefix))
		if err != nil {
			b.Fatalf("List failed: %v", err)
		}
	}
}

// Different value sizes
func BenchmarkStorageEngine_Put_SmallValues(b *testing.B) {
	benchmarkPutWithValueSize(b, 10)
}

func BenchmarkStorageEngine_Put_MediumValues(b *testing.B) {
	benchmarkPutWithValueSize(b, 1000)
}

func BenchmarkStorageEngine_Put_LargeValues(b *testing.B) {
	benchmarkPutWithValueSize(b, 10000)
}

func BenchmarkStorageEngine_Get_SmallValues(b *testing.B) {
	benchmarkGetWithValueSize(b, 10)
}

func BenchmarkStorageEngine_Get_MediumValues(b *testing.B) {
	benchmarkGetWithValueSize(b, 1000)
}

func BenchmarkStorageEngine_Get_LargeValues(b *testing.B) {
	benchmarkGetWithValueSize(b, 10000)
}

// Concurrent operations
func BenchmarkStorageEngine_Put_Concurrent(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("concurrent-put-key-%d", i))
			value := []byte(fmt.Sprintf("concurrent-put-value-%d", i))
			
			err := engine.Put(key, value)
			if err != nil {
				b.Fatalf("Concurrent Put failed: %v", err)
			}
			i++
		}
	})
}

func BenchmarkStorageEngine_Get_Concurrent(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("concurrent-get-key-%d", i))
		value := []byte(fmt.Sprintf("concurrent-get-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("concurrent-get-key-%d", i%numKeys))
			_, err := engine.Get(key)
			if err != nil {
				b.Fatalf("Concurrent Get failed: %v", err)
			}
			i++
		}
	})
}

func BenchmarkStorageEngine_Mixed_Operations(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with some data
	numInitialKeys := 5000
	for i := 0; i < numInitialKeys; i++ {
		key := []byte(fmt.Sprintf("mixed-initial-key-%d", i))
		value := []byte(fmt.Sprintf("mixed-initial-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		operation := i % 4
		keyNum := i % numInitialKeys
		
		switch operation {
		case 0: // Put
			key := []byte(fmt.Sprintf("mixed-put-key-%d", i))
			value := []byte(fmt.Sprintf("mixed-put-value-%d", i))
			err := engine.Put(key, value)
			if err != nil {
				b.Fatalf("Mixed Put failed: %v", err)
			}
			
		case 1: // Get
			key := []byte(fmt.Sprintf("mixed-initial-key-%d", keyNum))
			_, err := engine.Get(key)
			if err != nil {
				b.Fatalf("Mixed Get failed: %v", err)
			}
			
		case 2: // Exists
			key := []byte(fmt.Sprintf("mixed-initial-key-%d", keyNum))
			_, err := engine.Exists(key)
			if err != nil {
				b.Fatalf("Mixed Exists failed: %v", err)
			}
			
		case 3: // Delete (but put it back for next iteration)
			key := []byte(fmt.Sprintf("mixed-initial-key-%d", keyNum))
			value := []byte(fmt.Sprintf("mixed-initial-value-%d", keyNum))
			err := engine.Delete(key)
			if err != nil {
				b.Fatalf("Mixed Delete failed: %v", err)
			}
			// Put it back
			err = engine.Put(key, value)
			if err != nil {
				b.Fatalf("Mixed Put-back failed: %v", err)
			}
		}
	}
}

// Sequential vs Random access patterns
func BenchmarkStorageEngine_Get_Sequential(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("sequential-key-%06d", i))
		value := []byte(fmt.Sprintf("sequential-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("sequential-key-%06d", i%numKeys))
		_, err := engine.Get(key)
		if err != nil {
			b.Fatalf("Sequential Get failed: %v", err)
		}
	}
}

func BenchmarkStorageEngine_Get_Random(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("random-key-%06d", i))
		value := []byte(fmt.Sprintf("random-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	// Pre-generate random indices
	rand.Seed(42) // Fixed seed for reproducible benchmarks
	randomIndices := make([]int, b.N)
	for i := 0; i < b.N; i++ {
		randomIndices[i] = rand.Intn(numKeys)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("random-key-%06d", randomIndices[i]))
		_, err := engine.Get(key)
		if err != nil {
			b.Fatalf("Random Get failed: %v", err)
		}
	}
}

// Helper functions

func setupBenchmarkStorage(b *testing.B) *storage.Engine {
	b.Helper()
	
	config := storage.Config{
		DataPath:   "",
		InMemory:   true,
		SyncWrites: false,
		ValueLogGC: false,
	}
	
	engine, err := storage.NewEngine(config)
	if err != nil {
		b.Fatalf("Failed to create benchmark storage engine: %v", err)
	}
	
	b.Cleanup(func() {
		engine.Close()
	})
	
	return engine
}

func benchmarkPutWithValueSize(b *testing.B, valueSize int) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-generate test data
	keys := make([][]byte, b.N)
	values := make([][]byte, b.N)
	
	for i := 0; i < b.N; i++ {
		keys[i] = []byte(fmt.Sprintf("size-put-key-%d", i))
		values[i] = []byte(testutil.GenerateRandomString(valueSize))
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		err := engine.Put(keys[i], values[i])
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
	}
}

func benchmarkGetWithValueSize(b *testing.B, valueSize int) {
	engine := setupBenchmarkStorage(b)
	
	// Pre-populate with test data
	numKeys := 10000
	keys := make([][]byte, numKeys)
	
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("size-get-key-%d", i))
		value := []byte(testutil.GenerateRandomString(valueSize))
		keys[i] = key
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Setup Put failed: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := keys[i%numKeys]
		_, err := engine.Get(key)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

// Memory and performance measurement benchmarks

func BenchmarkStorageEngine_Memory_Usage(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("memory-key-%d", i))
		value := []byte(fmt.Sprintf("memory-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
	}
}

func BenchmarkStorageEngine_Throughput(b *testing.B) {
	engine := setupBenchmarkStorage(b)
	
	start := time.Now()
	
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("throughput-key-%d", i))
		value := []byte(fmt.Sprintf("throughput-value-%d", i))
		
		err := engine.Put(key, value)
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
	}
	
	duration := time.Since(start)
	ops := float64(b.N) / duration.Seconds()
	
	b.ReportMetric(ops, "ops/sec")
}