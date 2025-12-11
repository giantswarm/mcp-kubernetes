package k8s

import (
	"container/list"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockMetrics implements CacheMetricsCallback for testing.
type mockMetrics struct {
	mu        sync.Mutex
	hits      int
	misses    int
	evictions map[string]int
	sizeLog   []int
}

func newMockMetrics() *mockMetrics {
	return &mockMetrics{
		evictions: make(map[string]int),
	}
}

func (m *mockMetrics) OnCacheHit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hits++
}

func (m *mockMetrics) OnCacheMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.misses++
}

func (m *mockMetrics) OnCacheEviction(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evictions[reason]++
}

func (m *mockMetrics) OnCacheSizeChange(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sizeLog = append(m.sizeLog, size)
}

func (m *mockMetrics) getHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits
}

func (m *mockMetrics) getMisses() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.misses
}

func (m *mockMetrics) getEvictions(reason string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.evictions[reason]
}

// newTestCache creates a cache for testing with configurable cleanup behavior.
// If autoCleanup is true, the cleanup goroutine runs normally.
// If false, cleanup must be triggered manually via cache.cleanup().
func newTestCache(ttl time.Duration, autoCleanup bool) *clientCache {
	if autoCleanup {
		return newClientCache(ttl)
	}

	// Create cache without automatic cleanup goroutine
	return newTestCacheWithConfig(ClientCacheConfig{
		TTL:        ttl,
		MaxEntries: DefaultClientCacheMaxEntries,
	}, false)
}

// newTestCacheWithConfig creates a cache for testing with full config control.
func newTestCacheWithConfig(config ClientCacheConfig, autoCleanup bool) *clientCache {
	if config.TTL <= 0 {
		config.TTL = DefaultClientCacheTTL
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = DefaultClientCacheMaxEntries
	}

	if autoCleanup {
		return newClientCacheWithConfig(config)
	}

	// Create cache without automatic cleanup goroutine
	c := &clientCache{
		entries:     make(map[string]*list.Element),
		lruList:     list.New(),
		ttl:         config.TTL,
		maxSize:     config.MaxEntries,
		metrics:     config.Metrics,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start a dummy goroutine that just waits for close signal
	go func() {
		<-c.stopCleanup
		close(c.cleanupDone)
	}()

	return c
}

func TestHashToken(t *testing.T) {
	tests := []struct {
		name   string
		token1 string
		token2 string
		same   bool
	}{
		{
			name:   "same tokens produce same hash",
			token1: "test-token-123",
			token2: "test-token-123",
			same:   true,
		},
		{
			name:   "different tokens produce different hash",
			token1: "test-token-123",
			token2: "test-token-456",
			same:   false,
		},
		{
			name:   "empty tokens produce same hash",
			token1: "",
			token2: "",
			same:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashToken(tt.token1)
			hash2 := hashToken(tt.token2)

			if tt.same && hash1 != hash2 {
				t.Errorf("expected same hash, got different: %s != %s", hash1, hash2)
			}
			if !tt.same && hash1 == hash2 {
				t.Errorf("expected different hash, got same: %s == %s", hash1, hash2)
			}

			// Verify hash is hex encoded (64 chars for SHA-256)
			if len(hash1) != 64 {
				t.Errorf("expected hash length 64, got %d", len(hash1))
			}
		})
	}
}

func TestClientCacheGetSet(t *testing.T) {
	cache := newClientCache(1 * time.Hour)
	defer cache.Close()

	token := "test-token"
	client := &bearerTokenClient{bearerToken: token}

	// Initially should return nil
	if got := cache.Get(token); got != nil {
		t.Errorf("expected nil for missing token, got %v", got)
	}

	// Set and retrieve
	cache.Set(token, client)
	got := cache.Get(token)
	if got == nil {
		t.Fatal("expected cached client, got nil")
	}

	if got.(*bearerTokenClient).bearerToken != token {
		t.Errorf("expected token %s, got %s", token, got.(*bearerTokenClient).bearerToken)
	}
}

func TestClientCacheExpiration(t *testing.T) {
	// Use very short TTL for testing
	cache := newClientCache(50 * time.Millisecond)
	defer cache.Close()

	token := "test-token"
	client := &bearerTokenClient{bearerToken: token}

	cache.Set(token, client)

	// Should be available immediately
	if got := cache.Get(token); got == nil {
		t.Fatal("expected cached client, got nil")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	if got := cache.Get(token); got != nil {
		t.Errorf("expected nil for expired token, got %v", got)
	}
}

func TestClientCacheSize(t *testing.T) {
	cache := newClientCache(1 * time.Hour)
	defer cache.Close()

	if cache.Size() != 0 {
		t.Errorf("expected size 0, got %d", cache.Size())
	}

	cache.Set("token1", &bearerTokenClient{})
	cache.Set("token2", &bearerTokenClient{})

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	// Same token should not increase size
	cache.Set("token1", &bearerTokenClient{})
	if cache.Size() != 2 {
		t.Errorf("expected size 2 after re-setting same token, got %d", cache.Size())
	}
}

func TestClientCacheCleanup(t *testing.T) {
	// Use test helper with manual cleanup (autoCleanup=false)
	cache := newTestCache(50*time.Millisecond, false)
	defer cache.Close()

	cache.Set("token1", &bearerTokenClient{})
	cache.Set("token2", &bearerTokenClient{})

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	// Wait for entries to expire
	time.Sleep(100 * time.Millisecond)

	// Manual cleanup
	cache.cleanup()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after cleanup, got %d", cache.Size())
	}
}

func TestClientCacheClose(t *testing.T) {
	cache := newClientCache(1 * time.Hour)

	cache.Set("token", &bearerTokenClient{})
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	cache.Close()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after close, got %d", cache.Size())
	}
}

func TestClientCacheLRUEviction(t *testing.T) {
	// Create cache with max 3 entries
	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        1 * time.Hour,
		MaxEntries: 3,
	})
	defer cache.Close()

	// Add 3 entries
	cache.Set("token1", &bearerTokenClient{bearerToken: "1"})
	cache.Set("token2", &bearerTokenClient{bearerToken: "2"})
	cache.Set("token3", &bearerTokenClient{bearerToken: "3"})

	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}

	// Add 4th entry - should evict token1 (oldest)
	cache.Set("token4", &bearerTokenClient{bearerToken: "4"})

	if cache.Size() != 3 {
		t.Errorf("expected size 3 after LRU eviction, got %d", cache.Size())
	}

	// token1 should be evicted
	if cache.Get("token1") != nil {
		t.Error("expected token1 to be evicted")
	}

	// Others should still be present
	if cache.Get("token2") == nil {
		t.Error("expected token2 to be present")
	}
	if cache.Get("token3") == nil {
		t.Error("expected token3 to be present")
	}
	if cache.Get("token4") == nil {
		t.Error("expected token4 to be present")
	}
}

func TestClientCacheLRUOrderUpdate(t *testing.T) {
	// Create cache with max 3 entries
	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        1 * time.Hour,
		MaxEntries: 3,
	})
	defer cache.Close()

	// Add 3 entries
	cache.Set("token1", &bearerTokenClient{bearerToken: "1"})
	cache.Set("token2", &bearerTokenClient{bearerToken: "2"})
	cache.Set("token3", &bearerTokenClient{bearerToken: "3"})

	// Access token1 to make it recently used
	cache.Get("token1")

	// Add 4th entry - should evict token2 (now oldest)
	cache.Set("token4", &bearerTokenClient{bearerToken: "4"})

	// token2 should be evicted (was oldest after token1 was accessed)
	if cache.Get("token2") != nil {
		t.Error("expected token2 to be evicted")
	}

	// token1 should still be present (was accessed, moved to front)
	if cache.Get("token1") == nil {
		t.Error("expected token1 to be present after access")
	}
}

func TestClientCacheMetricsHitMiss(t *testing.T) {
	metrics := newMockMetrics()

	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        1 * time.Hour,
		MaxEntries: 10,
		Metrics:    metrics,
	})
	defer cache.Close()

	// Miss on empty cache
	cache.Get("token1")
	// Give goroutine time to record metric
	time.Sleep(10 * time.Millisecond)
	if metrics.getMisses() != 1 {
		t.Errorf("expected 1 miss, got %d", metrics.getMisses())
	}

	// Set and hit
	cache.Set("token1", &bearerTokenClient{})
	cache.Get("token1")
	time.Sleep(10 * time.Millisecond)
	if metrics.getHits() != 1 {
		t.Errorf("expected 1 hit, got %d", metrics.getHits())
	}

	// Another miss
	cache.Get("token2")
	time.Sleep(10 * time.Millisecond)
	if metrics.getMisses() != 2 {
		t.Errorf("expected 2 misses, got %d", metrics.getMisses())
	}
}

func TestClientCacheMetricsEviction(t *testing.T) {
	metrics := newMockMetrics()

	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        50 * time.Millisecond,
		MaxEntries: 2,
		Metrics:    metrics,
	})
	defer cache.Close()

	// Add 2 entries
	cache.Set("token1", &bearerTokenClient{})
	cache.Set("token2", &bearerTokenClient{})

	// Add 3rd - triggers LRU eviction
	cache.Set("token3", &bearerTokenClient{})
	time.Sleep(20 * time.Millisecond)

	if metrics.getEvictions("lru") != 1 {
		t.Errorf("expected 1 LRU eviction, got %d", metrics.getEvictions("lru"))
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Access expired entry - triggers expired eviction
	cache.Get("token2")
	time.Sleep(20 * time.Millisecond)

	if metrics.getEvictions("expired") < 1 {
		t.Errorf("expected at least 1 expired eviction, got %d", metrics.getEvictions("expired"))
	}
}

func TestClientCacheMaxSize(t *testing.T) {
	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        1 * time.Hour,
		MaxEntries: 5,
	})
	defer cache.Close()

	if cache.MaxSize() != 5 {
		t.Errorf("expected max size 5, got %d", cache.MaxSize())
	}
}

func TestClientCacheStats(t *testing.T) {
	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        1 * time.Hour,
		MaxEntries: 10,
	})
	defer cache.Close()

	cache.Set("token1", &bearerTokenClient{})
	cache.Set("token2", &bearerTokenClient{})

	stats := cache.Stats()
	if stats.Size != 2 {
		t.Errorf("expected stats.Size 2, got %d", stats.Size)
	}
	if stats.MaxSize != 10 {
		t.Errorf("expected stats.MaxSize 10, got %d", stats.MaxSize)
	}
}

func TestClientCacheConcurrentAccess(t *testing.T) {
	cache := newClientCacheWithConfig(ClientCacheConfig{
		TTL:        1 * time.Hour,
		MaxEntries: 50,
	})
	defer cache.Close()

	const goroutines = 100
	const operations = 100

	var wg sync.WaitGroup
	var hits atomic.Int64
	var misses atomic.Int64

	// Concurrent writers and readers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				token := string(rune('A' + (id+j)%26)) // Rotate through 26 tokens
				if j%2 == 0 {
					cache.Set(token, &bearerTokenClient{bearerToken: token})
				} else {
					if cache.Get(token) != nil {
						hits.Add(1)
					} else {
						misses.Add(1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is in consistent state
	size := cache.Size()
	if size < 0 || size > 50 {
		t.Errorf("cache size out of bounds: %d", size)
	}

	t.Logf("Concurrent test: size=%d, hits=%d, misses=%d", size, hits.Load(), misses.Load())
}

func TestTryResolveBuiltinResource(t *testing.T) {
	builtinResources := initBuiltinResources()

	tests := []struct {
		name         string
		resourceType string
		apiGroup     string
		expectFound  bool
		expectGroup  string
	}{
		{
			name:         "pods are builtin",
			resourceType: "pods",
			apiGroup:     "",
			expectFound:  true,
			expectGroup:  "",
		},
		{
			name:         "deployments are builtin",
			resourceType: "deployments",
			apiGroup:     "",
			expectFound:  true,
			expectGroup:  "apps",
		},
		{
			name:         "services are builtin",
			resourceType: "services",
			apiGroup:     "",
			expectFound:  true,
			expectGroup:  "",
		},
		{
			name:         "custom resources are not builtin",
			resourceType: "mycustomresources",
			apiGroup:     "",
			expectFound:  false,
		},
		{
			name:         "pods with correct api group",
			resourceType: "pods",
			apiGroup:     "core",
			expectFound:  true,
			expectGroup:  "",
		},
		{
			name:         "deployments with correct api group",
			resourceType: "deployments",
			apiGroup:     "apps",
			expectFound:  true,
			expectGroup:  "apps",
		},
		{
			name:         "deployments with wrong api group",
			resourceType: "deployments",
			apiGroup:     "extensions",
			expectFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvr, _, found := tryResolveBuiltinResource(tt.resourceType, tt.apiGroup, builtinResources)

			if found != tt.expectFound {
				t.Errorf("expected found=%v, got %v", tt.expectFound, found)
			}

			if found && gvr.Group != tt.expectGroup {
				t.Errorf("expected group %s, got %s", tt.expectGroup, gvr.Group)
			}
		})
	}
}
