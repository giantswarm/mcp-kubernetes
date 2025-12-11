package k8s

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// DefaultClientCacheTTL is the default time-to-live for cached clients.
//
// This value is intentionally much shorter than typical OAuth token expiry
// (usually 1 hour for Google OAuth) to ensure:
//  1. We don't hold stale credentials longer than necessary
//  2. Memory usage stays bounded with many distinct users
//  3. Configuration changes (e.g., RBAC updates) take effect within a reasonable time
//
// The tradeoff is that users making requests more than 5 minutes apart will
// incur the cost of creating a new client, but this is acceptable given the
// security and resource benefits.
const DefaultClientCacheTTL = 5 * time.Minute

// DefaultClientCacheCleanupInterval is how often the cache cleanup runs.
// This is separate from TTL - entries are also removed on access if expired.
const DefaultClientCacheCleanupInterval = 1 * time.Minute

// DefaultClientCacheMaxEntries is the default maximum number of entries in the cache.
// This prevents unbounded memory growth in high-traffic multi-tenant scenarios.
// With a 5-minute TTL, this allows for ~100 concurrent users before LRU eviction kicks in.
const DefaultClientCacheMaxEntries = 100

// CacheMetricsCallback is an interface for recording cache metrics.
// This allows the cache to report metrics without depending on the instrumentation package.
type CacheMetricsCallback interface {
	// OnCacheHit is called when a cache hit occurs.
	OnCacheHit()
	// OnCacheMiss is called when a cache miss occurs.
	OnCacheMiss()
	// OnCacheEviction is called when an entry is evicted with the reason.
	// Reasons: "expired", "lru"
	OnCacheEviction(reason string)
	// OnCacheSizeChange is called when the cache size changes.
	OnCacheSizeChange(size int)
}

// clientCacheEntry represents a cached client with expiration time and LRU tracking.
type clientCacheEntry struct {
	key       string
	client    Client
	expiresAt time.Time
}

// clientCache provides thread-safe caching of Kubernetes clients by token hash.
// This avoids creating new clients for every request when using OAuth passthrough.
//
// The cache implements LRU (Least Recently Used) eviction when maxEntries is reached,
// ensuring bounded memory usage in high-traffic scenarios.
type clientCache struct {
	mu      sync.RWMutex
	entries map[string]*list.Element // key -> list element containing *clientCacheEntry
	lruList *list.List               // doubly-linked list for LRU ordering (front = most recent)
	ttl     time.Duration
	maxSize int

	// Metrics callback (optional)
	metrics CacheMetricsCallback

	// stopCleanup signals the cleanup goroutine to stop
	stopCleanup chan struct{}
	// cleanupDone signals that cleanup has finished
	cleanupDone chan struct{}
}

// ClientCacheConfig holds configuration options for the client cache.
type ClientCacheConfig struct {
	// TTL is the time-to-live for cached entries. Defaults to DefaultClientCacheTTL.
	TTL time.Duration
	// MaxEntries is the maximum number of entries before LRU eviction. Defaults to DefaultClientCacheMaxEntries.
	// Set to 0 for no limit (not recommended in production).
	MaxEntries int
	// Metrics is an optional callback for recording cache metrics.
	Metrics CacheMetricsCallback
}

// newClientCache creates a new client cache with the specified TTL.
// For more control, use newClientCacheWithConfig.
func newClientCache(ttl time.Duration) *clientCache {
	return newClientCacheWithConfig(ClientCacheConfig{
		TTL:        ttl,
		MaxEntries: DefaultClientCacheMaxEntries,
	})
}

// newClientCacheWithConfig creates a new client cache with the specified configuration.
func newClientCacheWithConfig(config ClientCacheConfig) *clientCache {
	if config.TTL <= 0 {
		config.TTL = DefaultClientCacheTTL
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = DefaultClientCacheMaxEntries
	}

	c := &clientCache{
		entries:     make(map[string]*list.Element),
		lruList:     list.New(),
		ttl:         config.TTL,
		maxSize:     config.MaxEntries,
		metrics:     config.Metrics,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go c.cleanupLoop()

	return c
}

// hashToken creates a SHA-256 hash of the token for use as a cache key.
// This avoids storing the raw token in memory as a map key.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a client from the cache if it exists and hasn't expired.
// Returns nil if no valid cached client exists.
//
// This method also updates the LRU order, moving the accessed entry to the front.
func (c *clientCache) Get(token string) Client {
	key := hashToken(token)

	c.mu.Lock()
	elem, exists := c.entries[key]
	if !exists {
		c.mu.Unlock()
		c.recordMiss()
		return nil
	}

	entry := elem.Value.(*clientCacheEntry)

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Expired - remove it
		c.removeElementLocked(elem, "expired")
		c.mu.Unlock()
		c.recordMiss()
		return nil
	}

	// Move to front of LRU list (most recently used)
	c.lruList.MoveToFront(elem)
	client := entry.client
	c.mu.Unlock()

	c.recordHit()
	return client
}

// Set adds a client to the cache with the configured TTL.
// If the cache is at capacity, the least recently used entry is evicted.
func (c *clientCache) Set(token string, client Client) {
	key := hashToken(token)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if elem, exists := c.entries[key]; exists {
		// Update existing entry and move to front
		entry := elem.Value.(*clientCacheEntry)
		entry.client = client
		entry.expiresAt = now.Add(c.ttl)
		c.lruList.MoveToFront(elem)
		return
	}

	// Evict LRU entries if at capacity
	for c.maxSize > 0 && c.lruList.Len() >= c.maxSize {
		c.evictOldestLocked()
	}

	// Add new entry at front of LRU list
	entry := &clientCacheEntry{
		key:       key,
		client:    client,
		expiresAt: now.Add(c.ttl),
	}
	elem := c.lruList.PushFront(entry)
	c.entries[key] = elem

	c.reportSizeChangeLocked()
}

// evictOldestLocked removes the least recently used entry.
// Must be called with mu held.
func (c *clientCache) evictOldestLocked() {
	oldest := c.lruList.Back()
	if oldest != nil {
		c.removeElementLocked(oldest, "lru")
	}
}

// removeElementLocked removes an element from the cache.
// Must be called with mu held.
func (c *clientCache) removeElementLocked(elem *list.Element, reason string) {
	entry := elem.Value.(*clientCacheEntry)
	delete(c.entries, entry.key)
	c.lruList.Remove(elem)

	// Record eviction metric (do this before reporting size change)
	if c.metrics != nil {
		// Use goroutine to avoid holding lock during callback
		go c.metrics.OnCacheEviction(reason)
	}

	c.reportSizeChangeLocked()
}

// reportSizeChangeLocked reports the current cache size to metrics.
// Must be called with mu held.
func (c *clientCache) reportSizeChangeLocked() {
	if c.metrics != nil {
		size := len(c.entries)
		go c.metrics.OnCacheSizeChange(size)
	}
}

// recordHit records a cache hit metric.
func (c *clientCache) recordHit() {
	if c.metrics != nil {
		go c.metrics.OnCacheHit()
	}
}

// recordMiss records a cache miss metric.
func (c *clientCache) recordMiss() {
	if c.metrics != nil {
		go c.metrics.OnCacheMiss()
	}
}

// cleanupLoop periodically removes expired entries from the cache.
func (c *clientCache) cleanupLoop() {
	ticker := time.NewTicker(DefaultClientCacheCleanupInterval)
	defer ticker.Stop()
	defer close(c.cleanupDone)

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes all expired entries from the cache.
func (c *clientCache) cleanup() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Iterate through list and remove expired entries
	var next *list.Element
	for elem := c.lruList.Front(); elem != nil; elem = next {
		next = elem.Next()
		entry := elem.Value.(*clientCacheEntry)
		if now.After(entry.expiresAt) {
			c.removeElementLocked(elem, "expired")
		}
	}
}

// Close stops the cleanup goroutine and clears the cache.
func (c *clientCache) Close() {
	close(c.stopCleanup)
	<-c.cleanupDone

	c.mu.Lock()
	c.entries = make(map[string]*list.Element)
	c.lruList.Init()
	c.mu.Unlock()
}

// Size returns the current number of entries in the cache.
func (c *clientCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// MaxSize returns the maximum number of entries allowed in the cache.
func (c *clientCache) MaxSize() int {
	return c.maxSize
}

// Stats returns cache statistics for monitoring.
type CacheStats struct {
	Size    int
	MaxSize int
}

// Stats returns current cache statistics.
func (c *clientCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Size:    len(c.entries),
		MaxSize: c.maxSize,
	}
}
