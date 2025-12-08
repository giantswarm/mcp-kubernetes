package federation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// CacheConfig holds configuration options for the ClientCache.
//
// # Security Considerations
//
// The TTL setting has security implications: cached clients may persist after a
// user's OAuth token is invalidated or revoked. To mitigate this:
//   - Set TTL to be less than or equal to your OAuth token lifetime
//   - Use DeleteByCluster() when cluster credentials are rotated
//   - Use Delete() when a user's access should be immediately revoked
//
// # Capacity Planning
//
// Cache entries are keyed by (clusterName, userEmail) pairs. With the default
// MaxEntries of 1000, this could represent:
//   - 1000 users accessing 1 cluster each, or
//   - 100 users accessing 10 clusters each, or
//   - 10 users accessing 100 clusters each
//
// Monitor the mcp_client_cache_entries metric and adjust MaxEntries based on
// your actual usage patterns. LRU eviction ensures the most active users/clusters
// are retained when capacity is exceeded.
type CacheConfig struct {
	// TTL is the time-to-live for cached clients. After this duration,
	// entries are eligible for eviction.
	//
	// Security note: Set this to be less than or equal to your OAuth token
	// lifetime to ensure cached clients don't outlive user authorization.
	//
	// Default: 10 minutes.
	TTL time.Duration

	// MaxEntries is the maximum number of entries the cache can hold.
	// When exceeded, least recently accessed entries are evicted.
	//
	// Each unique (clusterName, userEmail) pair creates one cache entry.
	// Monitor the mcp_client_cache_entries metric to tune this value.
	//
	// Default: 1000.
	MaxEntries int

	// CleanupInterval is how often the background cleanup runs to remove
	// expired entries.
	//
	// Default: 1 minute.
	CleanupInterval time.Duration
}

// DefaultCacheConfig returns a CacheConfig with sensible defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:             10 * time.Minute,
		MaxEntries:      1000,
		CleanupInterval: 1 * time.Minute,
	}
}

// cachedClient holds a cached Kubernetes client along with metadata.
type cachedClient struct {
	// Kubernetes clients
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	restConfig    *rest.Config

	// Cache metadata
	createdAt time.Time
	expiry    time.Time

	// lastAccessedNanos stores the last accessed time as Unix nanoseconds.
	// Using atomic for lock-free reads during concurrent access.
	lastAccessedNanos atomic.Int64

	// Identity info for this cached client
	clusterName string
	userEmail   string
}

// isExpired returns true if the cached client has passed its TTL.
func (c *cachedClient) isExpired(now time.Time) bool {
	return now.After(c.expiry)
}

// touch updates the last accessed time atomically.
func (c *cachedClient) touch(now time.Time) {
	c.lastAccessedNanos.Store(now.UnixNano())
}

// getLastAccessed returns the last accessed time.
func (c *cachedClient) getLastAccessed() time.Time {
	return time.Unix(0, c.lastAccessedNanos.Load())
}

// CacheMetricsRecorder defines the interface for recording cache metrics.
// This allows decoupling from the concrete instrumentation implementation.
type CacheMetricsRecorder interface {
	// RecordCacheHit records a cache hit event.
	RecordCacheHit(ctx context.Context, clusterName string)

	// RecordCacheMiss records a cache miss event.
	RecordCacheMiss(ctx context.Context, clusterName string)

	// RecordCacheEviction records a cache eviction event.
	RecordCacheEviction(ctx context.Context, reason string)

	// SetCacheSize sets the current cache size gauge.
	SetCacheSize(ctx context.Context, size int)
}

// noopMetricsRecorder is a no-op implementation of CacheMetricsRecorder.
type noopMetricsRecorder struct{}

func (n *noopMetricsRecorder) RecordCacheHit(context.Context, string)      {}
func (n *noopMetricsRecorder) RecordCacheMiss(context.Context, string)     {}
func (n *noopMetricsRecorder) RecordCacheEviction(context.Context, string) {}
func (n *noopMetricsRecorder) SetCacheSize(context.Context, int)           {}

// ClientCache provides thread-safe caching of Kubernetes clients with
// TTL-based eviction and memory management.
//
// The cache is keyed by a composite of cluster name and user email to ensure
// that clients configured for different users are never shared.
type ClientCache struct {
	mu      sync.RWMutex
	clients map[string]*cachedClient

	// Configuration
	config CacheConfig
	logger *slog.Logger

	// Singleflight to prevent thundering herd when creating clients
	createGroup singleflight.Group

	// Metrics recorder
	metrics CacheMetricsRecorder

	// Lifecycle
	stopCh chan struct{}
	wg     sync.WaitGroup
	closed bool

	// Clock abstraction for testing
	now func() time.Time
}

// ClientCacheOption is a functional option for configuring ClientCache.
type ClientCacheOption func(*ClientCache)

// WithCacheConfig sets the cache configuration.
func WithCacheConfig(config CacheConfig) ClientCacheOption {
	return func(c *ClientCache) {
		c.config = config
	}
}

// WithCacheLogger sets the logger for the cache.
func WithCacheLogger(logger *slog.Logger) ClientCacheOption {
	return func(c *ClientCache) {
		c.logger = logger
	}
}

// WithCacheMetrics sets the metrics recorder for the cache.
func WithCacheMetrics(metrics CacheMetricsRecorder) ClientCacheOption {
	return func(c *ClientCache) {
		c.metrics = metrics
	}
}

// withCacheClock sets the clock function for testing.
func withCacheClock(now func() time.Time) ClientCacheOption {
	return func(c *ClientCache) {
		c.now = now
	}
}

// NewClientCache creates a new ClientCache with the provided options.
// The cache automatically starts a background goroutine for cleanup.
func NewClientCache(opts ...ClientCacheOption) *ClientCache {
	c := &ClientCache{
		clients: make(map[string]*cachedClient),
		config:  DefaultCacheConfig(),
		logger:  slog.Default(),
		metrics: &noopMetricsRecorder{},
		stopCh:  make(chan struct{}),
		now:     time.Now,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Validate configuration
	if c.config.TTL <= 0 {
		c.config.TTL = DefaultCacheConfig().TTL
	}
	if c.config.MaxEntries <= 0 {
		c.config.MaxEntries = DefaultCacheConfig().MaxEntries
	}
	if c.config.CleanupInterval <= 0 {
		c.config.CleanupInterval = DefaultCacheConfig().CleanupInterval
	}

	// Start background cleanup
	c.wg.Add(1)
	go c.cleanupLoop()

	c.logger.Info("Client cache initialized",
		"ttl", c.config.TTL,
		"max_entries", c.config.MaxEntries,
		"cleanup_interval", c.config.CleanupInterval)

	return c
}

// cacheKey generates a composite cache key from cluster name and user email.
// Format: "${clusterName}|${userEmail}"
func cacheKey(clusterName, userEmail string) string {
	return fmt.Sprintf("%s|%s", clusterName, userEmail)
}

// Get retrieves a cached client for the given cluster and user.
// Returns nil if no valid cached client exists.
// This method is thread-safe and records cache hit/miss metrics.
func (c *ClientCache) Get(ctx context.Context, clusterName, userEmail string) *cachedClient {
	key := cacheKey(clusterName, userEmail)
	now := c.now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil
	}

	client, ok := c.clients[key]
	if !ok {
		c.metrics.RecordCacheMiss(ctx, clusterName)
		return nil
	}

	if client.isExpired(now) {
		c.metrics.RecordCacheMiss(ctx, clusterName)
		return nil
	}

	// Touch to update LRU ordering. This is safe under RLock because
	// lastAccessedNanos uses atomic operations for lock-free updates.
	client.touch(now)
	c.metrics.RecordCacheHit(ctx, clusterName)

	return client
}

// Set stores a client in the cache for the given cluster and user.
// This method is thread-safe.
func (c *ClientCache) Set(ctx context.Context, clusterName, userEmail string, clientset kubernetes.Interface, dynamicClient dynamic.Interface, restConfig *rest.Config) {
	c.setAndReturn(ctx, clusterName, userEmail, clientset, dynamicClient, restConfig)
}

// setAndReturn stores a client in the cache and returns the cached entry.
// This is used internally by GetOrCreate to avoid a redundant Get after Set.
func (c *ClientCache) setAndReturn(ctx context.Context, clusterName, userEmail string, clientset kubernetes.Interface, dynamicClient dynamic.Interface, restConfig *rest.Config) *cachedClient {
	key := cacheKey(clusterName, userEmail)
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	// Evict LRU entries if at capacity
	c.evictIfNeededLocked(ctx)

	client := &cachedClient{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		restConfig:    restConfig,
		createdAt:     now,
		expiry:        now.Add(c.config.TTL),
		clusterName:   clusterName,
		userEmail:     userEmail,
	}
	client.lastAccessedNanos.Store(now.UnixNano())
	c.clients[key] = client

	c.metrics.SetCacheSize(ctx, len(c.clients))

	c.logger.Debug("Cached client",
		"cluster", clusterName,
		UserHashAttr(userEmail),
		"expiry", c.config.TTL)

	return client
}

// GetOrCreate retrieves a cached client or creates a new one using the provided factory.
// This method uses singleflight to prevent thundering herd when multiple goroutines
// request the same client simultaneously.
//
// The factory function is called only on cache miss and is guaranteed to be called
// at most once per unique key, even under high concurrency.
func (c *ClientCache) GetOrCreate(
	ctx context.Context,
	clusterName, userEmail string,
	factory func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error),
) (kubernetes.Interface, dynamic.Interface, error) {
	// Check cache first (fast path)
	if cached := c.Get(ctx, clusterName, userEmail); cached != nil {
		return cached.clientset, cached.dynamicClient, nil
	}

	// Use singleflight to prevent duplicate creation
	key := cacheKey(clusterName, userEmail)

	result, err, _ := c.createGroup.Do(key, func() (interface{}, error) {
		// Double-check cache inside singleflight
		if cached := c.Get(ctx, clusterName, userEmail); cached != nil {
			return cached, nil
		}

		// Create new client
		clientset, dynamicClient, restConfig, err := factory(ctx)
		if err != nil {
			return nil, err
		}

		// Store in cache and return the entry directly (avoiding redundant Get)
		return c.setAndReturn(ctx, clusterName, userEmail, clientset, dynamicClient, restConfig), nil
	})

	if err != nil {
		return nil, nil, err
	}

	cached := result.(*cachedClient)
	return cached.clientset, cached.dynamicClient, nil
}

// Delete removes a cached client for the given cluster and user.
// This is useful for invalidating cache entries when credentials change.
func (c *ClientCache) Delete(ctx context.Context, clusterName, userEmail string) {
	key := cacheKey(clusterName, userEmail)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if _, ok := c.clients[key]; ok {
		delete(c.clients, key)
		c.metrics.RecordCacheEviction(ctx, "manual")
		c.metrics.SetCacheSize(ctx, len(c.clients))

		c.logger.Debug("Deleted cached client",
			"cluster", clusterName,
			UserHashAttr(userEmail))
	}
}

// DeleteByCluster removes all cached clients for the given cluster.
// This is useful when cluster credentials are rotated.
func (c *ClientCache) DeleteByCluster(ctx context.Context, clusterName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	deleted := 0
	for key, client := range c.clients {
		if client.clusterName == clusterName {
			delete(c.clients, key)
			deleted++
		}
	}

	if deleted > 0 {
		c.metrics.SetCacheSize(ctx, len(c.clients))
		c.logger.Debug("Deleted cached clients for cluster",
			"cluster", clusterName,
			"count", deleted)
	}
}

// Size returns the current number of entries in the cache.
func (c *ClientCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.clients)
}

// Close stops the background cleanup goroutine and clears the cache.
// After Close is called, all cache operations become no-ops.
func (c *ClientCache) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Signal cleanup goroutine to stop
	close(c.stopCh)

	// Wait for cleanup goroutine to finish
	c.wg.Wait()

	// Clear all entries
	c.mu.Lock()
	c.clients = make(map[string]*cachedClient)
	c.mu.Unlock()

	c.logger.Info("Client cache closed")
	return nil
}

// cleanupLoop periodically removes expired entries from the cache.
func (c *ClientCache) cleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes all expired entries from the cache.
func (c *ClientCache) cleanup() {
	now := c.now()
	ctx := context.Background()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	expiredCount := 0
	for key, client := range c.clients {
		if client.isExpired(now) {
			delete(c.clients, key)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		c.metrics.SetCacheSize(ctx, len(c.clients))
		for i := 0; i < expiredCount; i++ {
			c.metrics.RecordCacheEviction(ctx, "expired")
		}
		c.logger.Debug("Cleaned up expired cache entries",
			"expired_count", expiredCount,
			"remaining", len(c.clients))
	}
}

// evictIfNeededLocked evicts LRU entries if the cache is at capacity.
// Must be called with c.mu held.
func (c *ClientCache) evictIfNeededLocked(ctx context.Context) {
	if len(c.clients) < c.config.MaxEntries {
		return
	}

	// Find the least recently accessed entry
	var oldestKey string
	var oldestTime time.Time

	for key, client := range c.clients {
		lastAccessed := client.getLastAccessed()
		if oldestKey == "" || lastAccessed.Before(oldestTime) {
			oldestKey = key
			oldestTime = lastAccessed
		}
	}

	if oldestKey != "" {
		delete(c.clients, oldestKey)
		c.metrics.RecordCacheEviction(ctx, "lru")
		c.logger.Debug("Evicted LRU cache entry",
			"key", oldestKey,
			"last_accessed", oldestTime)
	}
}

// Stats returns current cache statistics.
type CacheStats struct {
	// Size is the current number of entries in the cache.
	Size int

	// MaxEntries is the maximum capacity.
	MaxEntries int

	// TTL is the configured time-to-live.
	TTL time.Duration

	// OldestEntry is the age of the oldest entry (if any).
	OldestEntry time.Duration

	// NewestEntry is the age of the newest entry (if any).
	NewestEntry time.Duration
}

// Stats returns current cache statistics for monitoring.
func (c *ClientCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Size:       len(c.clients),
		MaxEntries: c.config.MaxEntries,
		TTL:        c.config.TTL,
	}

	if len(c.clients) == 0 {
		return stats
	}

	now := c.now()
	var oldest, newest time.Time

	for _, client := range c.clients {
		if oldest.IsZero() || client.createdAt.Before(oldest) {
			oldest = client.createdAt
		}
		if newest.IsZero() || client.createdAt.After(newest) {
			newest = client.createdAt
		}
	}

	if !oldest.IsZero() {
		stats.OldestEntry = now.Sub(oldest)
	}
	if !newest.IsZero() {
		stats.NewestEntry = now.Sub(newest)
	}

	return stats
}
