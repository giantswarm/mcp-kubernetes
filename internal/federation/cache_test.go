package federation

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// mockMetricsRecorder tracks cache metrics for testing.
type mockMetricsRecorder struct {
	mu          sync.Mutex
	hits        int
	misses      int
	evictions   map[string]int
	sizeUpdates []int
}

func newMockMetricsRecorder() *mockMetricsRecorder {
	return &mockMetricsRecorder{
		evictions: make(map[string]int),
	}
}

func (m *mockMetricsRecorder) RecordCacheHit(_ context.Context, _ string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hits++
}

func (m *mockMetricsRecorder) RecordCacheMiss(_ context.Context, _ string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.misses++
}

func (m *mockMetricsRecorder) RecordCacheEviction(_ context.Context, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evictions[reason]++
}

func (m *mockMetricsRecorder) SetCacheSize(_ context.Context, size int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sizeUpdates = append(m.sizeUpdates, size)
}

func (m *mockMetricsRecorder) getHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits
}

func (m *mockMetricsRecorder) getMisses() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.misses
}

func (m *mockMetricsRecorder) getEvictions(reason string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.evictions[reason]
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		clusterName string
		userEmail   string
		expected    string
	}{
		{
			clusterName: "prod-cluster",
			userEmail:   "admin@example.com",
			expected:    "prod-cluster|admin@example.com",
		},
		{
			clusterName: "",
			userEmail:   "user@example.com",
			expected:    "|user@example.com",
		},
		{
			clusterName: "staging",
			userEmail:   "",
			expected:    "staging|",
		},
		{
			clusterName: "",
			userEmail:   "",
			expected:    "|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := cacheKey(tt.clusterName, tt.userEmail)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNewClientCache(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("default configuration", func(t *testing.T) {
		cache := NewClientCache()
		defer cache.Close()

		assert.Equal(t, 0, cache.Size())
		assert.False(t, cache.closed)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := CacheConfig{
			TTL:             5 * time.Minute,
			MaxEntries:      500,
			CleanupInterval: 30 * time.Second,
		}

		cache := NewClientCache(
			WithCacheConfig(config),
			WithCacheLogger(logger),
		)
		defer cache.Close()

		assert.Equal(t, config.TTL, cache.config.TTL)
		assert.Equal(t, config.MaxEntries, cache.config.MaxEntries)
		assert.Equal(t, config.CleanupInterval, cache.config.CleanupInterval)
	})

	t.Run("invalid config values use defaults", func(t *testing.T) {
		config := CacheConfig{
			TTL:             0,
			MaxEntries:      -1,
			CleanupInterval: 0,
		}

		cache := NewClientCache(WithCacheConfig(config))
		defer cache.Close()

		defaults := DefaultCacheConfig()
		assert.Equal(t, defaults.TTL, cache.config.TTL)
		assert.Equal(t, defaults.MaxEntries, cache.config.MaxEntries)
		assert.Equal(t, defaults.CleanupInterval, cache.config.CleanupInterval)
	})
}

func TestClientCache_SetAndGet(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := newMockMetricsRecorder()

	cache := NewClientCache(
		WithCacheLogger(logger),
		WithCacheMetrics(metrics),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	t.Run("set and get client", func(t *testing.T) {
		clusterName := "test-cluster"
		userEmail := "user@example.com"

		// Initially should be a miss
		got := cache.Get(ctx, clusterName, userEmail)
		assert.Nil(t, got)
		assert.Equal(t, 1, metrics.getMisses())

		// Set the client
		cache.Set(ctx, clusterName, userEmail, fakeClient, fakeDynamic, nil)
		assert.Equal(t, 1, cache.Size())

		// Now should be a hit
		got = cache.Get(ctx, clusterName, userEmail)
		assert.NotNil(t, got)
		assert.Equal(t, 1, metrics.getHits())
		assert.Equal(t, fakeClient, got.clientset)
		assert.Equal(t, fakeDynamic, got.dynamicClient)
	})

	t.Run("different users have different cache entries", func(t *testing.T) {
		clusterName := "shared-cluster"
		user1 := "alice@example.com"
		user2 := "bob@example.com"

		cache.Set(ctx, clusterName, user1, fakeClient, fakeDynamic, nil)
		cache.Set(ctx, clusterName, user2, fakeClient, fakeDynamic, nil)

		// Both should be retrievable
		got1 := cache.Get(ctx, clusterName, user1)
		got2 := cache.Get(ctx, clusterName, user2)

		assert.NotNil(t, got1)
		assert.NotNil(t, got2)
		assert.Equal(t, user1, got1.userEmail)
		assert.Equal(t, user2, got2.userEmail)
	})
}

func TestClientCache_TTLExpiration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := newMockMetricsRecorder()

	// Use a mock clock for deterministic testing
	currentTime := time.Now()
	mockClock := func() time.Time {
		return currentTime
	}

	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             5 * time.Minute,
			MaxEntries:      100,
			CleanupInterval: 1 * time.Hour, // Disable automatic cleanup
		}),
		WithCacheLogger(logger),
		WithCacheMetrics(metrics),
		withCacheClock(mockClock),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	clusterName := "ttl-test"
	userEmail := "user@example.com"

	// Set a client
	cache.Set(ctx, clusterName, userEmail, fakeClient, fakeDynamic, nil)

	// Should be retrievable immediately
	got := cache.Get(ctx, clusterName, userEmail)
	assert.NotNil(t, got)

	// Advance time past TTL
	currentTime = currentTime.Add(6 * time.Minute)

	// Should now be expired (miss)
	got = cache.Get(ctx, clusterName, userEmail)
	assert.Nil(t, got)
}

func TestClientCache_Delete(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := newMockMetricsRecorder()

	cache := NewClientCache(
		WithCacheLogger(logger),
		WithCacheMetrics(metrics),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	clusterName := "delete-test"
	userEmail := "user@example.com"

	// Set a client
	cache.Set(ctx, clusterName, userEmail, fakeClient, fakeDynamic, nil)
	assert.Equal(t, 1, cache.Size())

	// Delete it
	cache.Delete(ctx, clusterName, userEmail)
	assert.Equal(t, 0, cache.Size())
	assert.Equal(t, 1, metrics.getEvictions("manual"))

	// Should no longer be retrievable
	got := cache.Get(ctx, clusterName, userEmail)
	assert.Nil(t, got)
}

func TestClientCache_DeleteByCluster(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewClientCache(
		WithCacheLogger(logger),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Add multiple users for the same cluster
	cache.Set(ctx, "cluster-a", "user1@example.com", fakeClient, fakeDynamic, nil)
	cache.Set(ctx, "cluster-a", "user2@example.com", fakeClient, fakeDynamic, nil)
	cache.Set(ctx, "cluster-b", "user1@example.com", fakeClient, fakeDynamic, nil)

	assert.Equal(t, 3, cache.Size())

	// Delete all entries for cluster-a
	cache.DeleteByCluster(ctx, "cluster-a")
	assert.Equal(t, 1, cache.Size())

	// cluster-a entries should be gone
	assert.Nil(t, cache.Get(ctx, "cluster-a", "user1@example.com"))
	assert.Nil(t, cache.Get(ctx, "cluster-a", "user2@example.com"))

	// cluster-b entry should still exist
	assert.NotNil(t, cache.Get(ctx, "cluster-b", "user1@example.com"))
}

func TestClientCache_LRUEviction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := newMockMetricsRecorder()

	currentTime := time.Now()
	mockClock := func() time.Time {
		return currentTime
	}

	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             10 * time.Minute,
			MaxEntries:      3, // Small cache for testing
			CleanupInterval: 1 * time.Hour,
		}),
		WithCacheLogger(logger),
		WithCacheMetrics(metrics),
		withCacheClock(mockClock),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Add 3 entries (at capacity)
	cache.Set(ctx, "cluster-1", "user@example.com", fakeClient, fakeDynamic, nil)
	currentTime = currentTime.Add(1 * time.Second)
	cache.Set(ctx, "cluster-2", "user@example.com", fakeClient, fakeDynamic, nil)
	currentTime = currentTime.Add(1 * time.Second)
	cache.Set(ctx, "cluster-3", "user@example.com", fakeClient, fakeDynamic, nil)

	assert.Equal(t, 3, cache.Size())

	// Access cluster-1 to update its lastAccessed time
	currentTime = currentTime.Add(1 * time.Second)
	cache.Get(ctx, "cluster-1", "user@example.com")

	// Add a 4th entry - should evict cluster-2 (least recently accessed)
	currentTime = currentTime.Add(1 * time.Second)
	cache.Set(ctx, "cluster-4", "user@example.com", fakeClient, fakeDynamic, nil)

	assert.Equal(t, 3, cache.Size())
	assert.Equal(t, 1, metrics.getEvictions("lru"))

	// cluster-2 should be evicted (it was accessed before cluster-1 was touched)
	assert.Nil(t, cache.Get(ctx, "cluster-2", "user@example.com"))
	assert.NotNil(t, cache.Get(ctx, "cluster-1", "user@example.com"))
	assert.NotNil(t, cache.Get(ctx, "cluster-3", "user@example.com"))
	assert.NotNil(t, cache.Get(ctx, "cluster-4", "user@example.com"))
}

func TestClientCache_GetOrCreate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := newMockMetricsRecorder()

	cache := NewClientCache(
		WithCacheLogger(logger),
		WithCacheMetrics(metrics),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	var factoryCalls int32

	factory := func(_ context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		atomic.AddInt32(&factoryCalls, 1)
		return fakeClient, fakeDynamic, nil, nil
	}

	clusterName := "getorcreate-test"
	userEmail := "user@example.com"

	// First call should invoke factory
	clientset, dynamicClient, err := cache.GetOrCreate(ctx, clusterName, userEmail, factory)
	require.NoError(t, err)
	assert.NotNil(t, clientset)
	assert.NotNil(t, dynamicClient)
	assert.Equal(t, int32(1), atomic.LoadInt32(&factoryCalls))

	// Second call should hit cache, not invoke factory
	clientset2, dynamicClient2, err := cache.GetOrCreate(ctx, clusterName, userEmail, factory)
	require.NoError(t, err)
	assert.NotNil(t, clientset2)
	assert.NotNil(t, dynamicClient2)
	assert.Equal(t, int32(1), atomic.LoadInt32(&factoryCalls)) // Still 1
}

func TestClientCache_GetOrCreate_ConcurrentSingleflight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewClientCache(
		WithCacheLogger(logger),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	var factoryCalls int32

	// Slow factory to simulate expensive client creation
	factory := func(_ context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt32(&factoryCalls, 1)
		return fakeClient, fakeDynamic, nil, nil
	}

	clusterName := "singleflight-test"
	userEmail := "user@example.com"

	// Launch multiple goroutines simultaneously
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := cache.GetOrCreate(ctx, clusterName, userEmail, factory)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// Singleflight should ensure factory is only called once
	assert.Equal(t, int32(1), atomic.LoadInt32(&factoryCalls))
	assert.Equal(t, 1, cache.Size())
}

func TestClientCache_Close(t *testing.T) {
	cache := NewClientCache()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	cache.Set(ctx, "test", "user@example.com", fakeClient, fakeDynamic, nil)
	assert.Equal(t, 1, cache.Size())

	// Close should succeed
	err := cache.Close()
	assert.NoError(t, err)

	// Close should be idempotent
	err = cache.Close()
	assert.NoError(t, err)

	// Operations after close should be no-ops
	cache.Set(ctx, "test2", "user@example.com", fakeClient, fakeDynamic, nil)
	assert.Nil(t, cache.Get(ctx, "test", "user@example.com"))
}

func TestClientCache_Stats(t *testing.T) {
	currentTime := time.Now()
	mockClock := func() time.Time {
		return currentTime
	}

	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             10 * time.Minute,
			MaxEntries:      100,
			CleanupInterval: 1 * time.Hour,
		}),
		withCacheClock(mockClock),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Empty cache stats
	stats := cache.Stats()
	assert.Equal(t, 0, stats.Size)
	assert.Equal(t, 100, stats.MaxEntries)
	assert.Equal(t, 10*time.Minute, stats.TTL)

	// Add some entries
	cache.Set(ctx, "cluster-1", "user@example.com", fakeClient, fakeDynamic, nil)
	currentTime = currentTime.Add(2 * time.Minute)
	cache.Set(ctx, "cluster-2", "user@example.com", fakeClient, fakeDynamic, nil)

	stats = cache.Stats()
	assert.Equal(t, 2, stats.Size)
	assert.Equal(t, 2*time.Minute, stats.OldestEntry)
	assert.Equal(t, time.Duration(0), stats.NewestEntry)
}

func TestClientCache_Cleanup(t *testing.T) {
	metrics := newMockMetricsRecorder()

	// Thread-safe mock clock
	var currentTimeNanos atomic.Int64
	currentTimeNanos.Store(time.Now().UnixNano())
	mockClock := func() time.Time {
		return time.Unix(0, currentTimeNanos.Load())
	}

	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             1 * time.Minute,
			MaxEntries:      100,
			CleanupInterval: 100 * time.Millisecond, // Fast cleanup for testing
		}),
		WithCacheMetrics(metrics),
		withCacheClock(mockClock),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Add some entries
	cache.Set(ctx, "cluster-1", "user@example.com", fakeClient, fakeDynamic, nil)
	cache.Set(ctx, "cluster-2", "user@example.com", fakeClient, fakeDynamic, nil)

	assert.Equal(t, 2, cache.Size())

	// Advance time past TTL (thread-safe)
	currentTimeNanos.Store(time.Now().Add(2 * time.Minute).UnixNano())

	// Wait for cleanup to run
	time.Sleep(300 * time.Millisecond)

	// Entries should be cleaned up
	assert.Equal(t, 0, cache.Size())
	assert.Equal(t, 2, metrics.getEvictions("expired"))
}

func TestClientCache_ConcurrentAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             10 * time.Minute,
			MaxEntries:      1000,
			CleanupInterval: 1 * time.Hour,
		}),
		WithCacheLogger(logger),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	var wg sync.WaitGroup
	numGoroutines := 100
	iterations := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			clusterName := "concurrent-cluster"
			userEmail := "user@example.com"

			for j := 0; j < iterations; j++ {
				// Mix of operations
				switch j % 4 {
				case 0:
					cache.Set(ctx, clusterName, userEmail, fakeClient, fakeDynamic, nil)
				case 1:
					cache.Get(ctx, clusterName, userEmail)
				case 2:
					cache.Size()
				case 3:
					cache.Stats()
				}
			}
		}(i)
	}

	wg.Wait()

	// Cache should still be operational
	assert.True(t, cache.Size() >= 0)
}

func TestClientCache_RaceCondition(t *testing.T) {
	// This test is designed to be run with -race flag
	// go test -race ./internal/federation/...

	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             1 * time.Minute,
			MaxEntries:      10,
			CleanupInterval: 50 * time.Millisecond,
		}),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	var wg sync.WaitGroup

	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Set(ctx, "cluster", "user@example.com", fakeClient, fakeDynamic, nil)
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Get(ctx, "cluster", "user@example.com")
			}
		}(i)
	}

	// Delete goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cache.Delete(ctx, "cluster", "user@example.com")
			}
		}(i)
	}

	// Stats goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cache.Stats()
				cache.Size()
			}
		}(i)
	}

	wg.Wait()
}

func TestCachedClient_IsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		expiry   time.Time
		checkAt  time.Time
		expected bool
	}{
		{
			name:     "not expired",
			expiry:   now.Add(5 * time.Minute),
			checkAt:  now,
			expected: false,
		},
		{
			name:     "expired",
			expiry:   now.Add(-1 * time.Minute),
			checkAt:  now,
			expected: true,
		},
		{
			name:     "exactly at expiry",
			expiry:   now,
			checkAt:  now,
			expected: false,
		},
		{
			name:     "just past expiry",
			expiry:   now,
			checkAt:  now.Add(1 * time.Nanosecond),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &cachedClient{
				expiry: tt.expiry,
			}
			assert.Equal(t, tt.expected, client.isExpired(tt.checkAt))
		})
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	assert.Equal(t, 10*time.Minute, config.TTL)
	assert.Equal(t, 1000, config.MaxEntries)
	assert.Equal(t, 1*time.Minute, config.CleanupInterval)
}

func BenchmarkClientCache_Get(b *testing.B) {
	cache := NewClientCache()
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		cache.Set(ctx, "cluster", "user@example.com", fakeClient, fakeDynamic, nil)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get(ctx, "cluster", "user@example.com")
		}
	})
}

func BenchmarkClientCache_Set(b *testing.B) {
	cache := NewClientCache(
		WithCacheConfig(CacheConfig{
			TTL:             10 * time.Minute,
			MaxEntries:      100000,
			CleanupInterval: 1 * time.Hour,
		}),
	)
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(ctx, "cluster", "user@example.com", fakeClient, fakeDynamic, nil)
	}
}

func BenchmarkClientCache_GetOrCreate(b *testing.B) {
	cache := NewClientCache()
	defer cache.Close()

	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	factory := func(_ context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return fakeClient, fakeDynamic, nil, nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = cache.GetOrCreate(ctx, "cluster", "user@example.com", factory)
		}
	})
}
