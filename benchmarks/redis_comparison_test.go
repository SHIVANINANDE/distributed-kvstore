package benchmarks

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"distributed-kvstore/internal/storage"
	"distributed-kvstore/internal/testutil"

	"github.com/go-redis/redis/v8"
)

// RedisComparison provides utilities for comparing our storage engine with Redis
type RedisComparison struct {
	ourEngine   storage.StorageEngine
	redisClient *redis.Client
}

func setupRedisComparison(b *testing.B) *RedisComparison {
	b.Helper()

	// Setup our storage engine
	config := storage.Config{
		InMemory:   true,
		SyncWrites: false,
		CacheEnabled: true,
		CacheSize:    10000,
		CacheTTL:     30 * time.Minute,
	}
	
	ourEngine, err := storage.NewStorageEngine(config)
	if err != nil {
		b.Fatalf("Failed to create our storage engine: %v", err)
	}

	// Setup Redis client (assumes Redis is running on localhost:6379)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	// Test Redis connection
	ctx := context.Background()
	_, err = redisClient.Ping(ctx).Result()
	if err != nil {
		b.Skipf("Redis not available for comparison: %v", err)
	}

	// Clear Redis database
	redisClient.FlushDB(ctx)

	return &RedisComparison{
		ourEngine:   ourEngine,
		redisClient: redisClient,
	}
}

func (rc *RedisComparison) Close() {
	if rc.ourEngine != nil {
		rc.ourEngine.Close()
	}
	if rc.redisClient != nil {
		rc.redisClient.Close()
	}
}

func BenchmarkVsRedis_SingleOperations(b *testing.B) {
	rc := setupRedisComparison(b)
	defer rc.Close()

	ctx := context.Background()
	
	// Generate test data
	keys := make([][]byte, 10000)
	values := make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		keys[i] = []byte(fmt.Sprintf("bench_key_%d", i))
		values[i] = []byte(testutil.GenerateRandomString(100))
	}

	b.Run("PUT_Operations", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % len(keys)
				err := rc.ourEngine.Put(keys[idx], values[idx])
				if err != nil {
					b.Fatalf("Put failed: %v", err)
				}
			}
		})

		b.Run("Redis", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % len(keys)
				err := rc.redisClient.Set(ctx, string(keys[idx]), string(values[idx]), 0).Err()
				if err != nil {
					b.Fatalf("Redis SET failed: %v", err)
				}
			}
		})
	})

	// Pre-populate for GET tests
	for i := 0; i < len(keys); i++ {
		rc.ourEngine.Put(keys[i], values[i])
		rc.redisClient.Set(ctx, string(keys[i]), string(values[i]), 0)
	}

	b.Run("GET_Operations", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % len(keys)
				_, err := rc.ourEngine.Get(keys[idx])
				if err != nil && err != storage.ErrKeyNotFound {
					b.Fatalf("Get failed: %v", err)
				}
			}
		})

		b.Run("Redis", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % len(keys)
				_, err := rc.redisClient.Get(ctx, string(keys[idx])).Result()
				if err != nil && err != redis.Nil {
					b.Fatalf("Redis GET failed: %v", err)
				}
			}
		})
	})

	b.Run("DELETE_Operations", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				idx := i % len(keys)
				// Re-add key for each iteration
				rc.ourEngine.Put(keys[idx], values[idx])
				b.StartTimer()

				err := rc.ourEngine.Delete(keys[idx])
				if err != nil {
					b.Fatalf("Delete failed: %v", err)
				}
			}
		})

		b.Run("Redis", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				idx := i % len(keys)
				// Re-add key for each iteration
				rc.redisClient.Set(ctx, string(keys[idx]), string(values[idx]), 0)
				b.StartTimer()

				err := rc.redisClient.Del(ctx, string(keys[idx])).Err()
				if err != nil {
					b.Fatalf("Redis DEL failed: %v", err)
				}
			}
		})
	})
}

func BenchmarkVsRedis_BatchOperations(b *testing.B) {
	rc := setupRedisComparison(b)
	defer rc.Close()

	ctx := context.Background()
	const batchSize = 100

	// Generate test data
	items := make([]storage.KeyValue, batchSize)
	keys := make([][]byte, batchSize)
	redisKeys := make([]string, batchSize)
	redisValues := make([]interface{}, batchSize*2) // Redis MSET expects key, value, key, value...

	for i := 0; i < batchSize; i++ {
		key := fmt.Sprintf("batch_key_%d", i)
		value := testutil.GenerateRandomString(100)
		
		items[i] = storage.KeyValue{Key: []byte(key), Value: []byte(value)}
		keys[i] = []byte(key)
		redisKeys[i] = key
		redisValues[i*2] = key
		redisValues[i*2+1] = value
	}

	b.Run("BATCH_PUT", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := rc.ourEngine.BatchPut(items)
				if err != nil {
					b.Fatalf("BatchPut failed: %v", err)
				}
			}
		})

		b.Run("Redis", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := rc.redisClient.MSet(ctx, redisValues...).Err()
				if err != nil {
					b.Fatalf("Redis MSET failed: %v", err)
				}
			}
		})
	})

	// Pre-populate for GET tests
	rc.ourEngine.BatchPut(items)
	rc.redisClient.MSet(ctx, redisValues...)

	b.Run("BATCH_GET", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := rc.ourEngine.BatchGet(keys)
				if err != nil {
					b.Fatalf("BatchGet failed: %v", err)
				}
			}
		})

		b.Run("Redis", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := rc.redisClient.MGet(ctx, redisKeys...).Result()
				if err != nil {
					b.Fatalf("Redis MGET failed: %v", err)
				}
			}
		})
	})

	b.Run("BATCH_DELETE", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Re-add data for each iteration
				rc.ourEngine.BatchPut(items)
				b.StartTimer()

				err := rc.ourEngine.BatchDelete(keys)
				if err != nil {
					b.Fatalf("BatchDelete failed: %v", err)
				}
			}
		})

		b.Run("Redis", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Re-add data for each iteration
				rc.redisClient.MSet(ctx, redisValues...)
				b.StartTimer()

				err := rc.redisClient.Del(ctx, redisKeys...).Err()
				if err != nil {
					b.Fatalf("Redis DEL failed: %v", err)
				}
			}
		})
	})
}

func BenchmarkVsRedis_ConcurrentLoad(b *testing.B) {
	rc := setupRedisComparison(b)
	defer rc.Close()

	ctx := context.Background()
	const numKeys = 1000

	// Generate test data
	keys := make([][]byte, numKeys)
	values := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = []byte(fmt.Sprintf("concurrent_key_%d", i))
		values[i] = []byte(testutil.GenerateRandomString(100))
	}

	// Pre-populate both stores
	for i := 0; i < numKeys; i++ {
		rc.ourEngine.Put(keys[i], values[i])
		rc.redisClient.Set(ctx, string(keys[i]), string(values[i]), 0)
	}

	b.Run("ConcurrentReads", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := rand.Intn(numKeys)
					_, err := rc.ourEngine.Get(keys[idx])
					if err != nil && err != storage.ErrKeyNotFound {
						b.Fatalf("Get failed: %v", err)
					}
				}
			})
		})

		b.Run("Redis", func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := rand.Intn(numKeys)
					_, err := rc.redisClient.Get(ctx, string(keys[idx])).Result()
					if err != nil && err != redis.Nil {
						b.Fatalf("Redis GET failed: %v", err)
					}
				}
			})
		})
	})

	b.Run("ConcurrentWrites", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := rand.Intn(numKeys)
					err := rc.ourEngine.Put(keys[idx], values[idx])
					if err != nil {
						b.Fatalf("Put failed: %v", err)
					}
				}
			})
		})

		b.Run("Redis", func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := rand.Intn(numKeys)
					err := rc.redisClient.Set(ctx, string(keys[idx]), string(values[idx]), 0).Err()
					if err != nil {
						b.Fatalf("Redis SET failed: %v", err)
					}
				}
			})
		})
	})

	b.Run("MixedReadWrite", func(b *testing.B) {
		b.Run("OurEngine", func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := rand.Intn(numKeys)
					
					// 70% reads, 30% writes
					if rand.Float32() < 0.7 {
						_, err := rc.ourEngine.Get(keys[idx])
						if err != nil && err != storage.ErrKeyNotFound {
							b.Fatalf("Get failed: %v", err)
						}
					} else {
						err := rc.ourEngine.Put(keys[idx], values[idx])
						if err != nil {
							b.Fatalf("Put failed: %v", err)
						}
					}
				}
			})
		})

		b.Run("Redis", func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := rand.Intn(numKeys)
					
					// 70% reads, 30% writes
					if rand.Float32() < 0.7 {
						_, err := rc.redisClient.Get(ctx, string(keys[idx])).Result()
						if err != nil && err != redis.Nil {
							b.Fatalf("Redis GET failed: %v", err)
						}
					} else {
						err := rc.redisClient.Set(ctx, string(keys[idx]), string(values[idx]), 0).Err()
						if err != nil {
							b.Fatalf("Redis SET failed: %v", err)
						}
					}
				}
			})
		})
	})
}

// Performance comparison test that provides summary results
func TestPerformanceSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance summary in short mode")
	}

	rc := setupRedisComparison(&testing.B{})
	defer rc.Close()

	ctx := context.Background()
	const numOps = 10000

	// Generate test data
	keys := make([][]byte, numOps)
	values := make([][]byte, numOps)
	for i := 0; i < numOps; i++ {
		keys[i] = []byte(fmt.Sprintf("perf_key_%d", i))
		values[i] = []byte(testutil.GenerateRandomString(100))
	}

	t.Log("=== Performance Comparison Summary ===")

	// Test PUT operations
	start := time.Now()
	for i := 0; i < numOps; i++ {
		rc.ourEngine.Put(keys[i], values[i])
	}
	ourPutTime := time.Since(start)

	start = time.Now()
	for i := 0; i < numOps; i++ {
		rc.redisClient.Set(ctx, string(keys[i]), string(values[i]), 0)
	}
	redisPutTime := time.Since(start)

	putRatio := float64(redisPutTime) / float64(ourPutTime)
	t.Logf("PUT Operations (%d ops):", numOps)
	t.Logf("  Our Engine: %v (%.2f ops/sec)", ourPutTime, float64(numOps)/ourPutTime.Seconds())
	t.Logf("  Redis:      %v (%.2f ops/sec)", redisPutTime, float64(numOps)/redisPutTime.Seconds())
	t.Logf("  Ratio:      %.2fx (our engine vs Redis)", putRatio)
	
	// Test GET operations
	start = time.Now()
	for i := 0; i < numOps; i++ {
		rc.ourEngine.Get(keys[i])
	}
	ourGetTime := time.Since(start)

	start = time.Now()
	for i := 0; i < numOps; i++ {
		rc.redisClient.Get(ctx, string(keys[i]))
	}
	redisGetTime := time.Since(start)

	getRatio := float64(redisGetTime) / float64(ourGetTime)
	t.Logf("GET Operations (%d ops):", numOps)
	t.Logf("  Our Engine: %v (%.2f ops/sec)", ourGetTime, float64(numOps)/ourGetTime.Seconds())
	t.Logf("  Redis:      %v (%.2f ops/sec)", redisGetTime, float64(numOps)/redisGetTime.Seconds())
	t.Logf("  Ratio:      %.2fx (our engine vs Redis)", getRatio)

	t.Log("=== End Performance Summary ===")
}