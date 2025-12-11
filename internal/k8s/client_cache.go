package k8s

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// DefaultClientCacheTTL is the default time-to-live for cached clients.
// This should be shorter than typical OAuth token expiry to ensure
// we don't use stale credentials.
const DefaultClientCacheTTL = 5 * time.Minute

// DefaultClientCacheCleanupInterval is how often the cache cleanup runs.
const DefaultClientCacheCleanupInterval = 1 * time.Minute

// clientCacheEntry represents a cached client with expiration time.
type clientCacheEntry struct {
	client    Client
	expiresAt time.Time
}

// clientCache provides thread-safe caching of Kubernetes clients by token hash.
// This avoids creating new clients for every request when using OAuth passthrough.
type clientCache struct {
	mu      sync.RWMutex
	entries map[string]*clientCacheEntry
	ttl     time.Duration

	// stopCleanup signals the cleanup goroutine to stop
	stopCleanup chan struct{}
	// cleanupDone signals that cleanup has finished
	cleanupDone chan struct{}
}

// newClientCache creates a new client cache with the specified TTL.
func newClientCache(ttl time.Duration) *clientCache {
	if ttl <= 0 {
		ttl = DefaultClientCacheTTL
	}

	c := &clientCache{
		entries:     make(map[string]*clientCacheEntry),
		ttl:         ttl,
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
func (c *clientCache) Get(token string) Client {
	key := hashToken(token)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Expired - remove it
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil
	}

	return entry.client
}

// Set adds a client to the cache with the configured TTL.
func (c *clientCache) Set(token string, client Client) {
	key := hashToken(token)

	c.mu.Lock()
	c.entries[key] = &clientCacheEntry{
		client:    client,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
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

	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// Close stops the cleanup goroutine and clears the cache.
func (c *clientCache) Close() {
	close(c.stopCleanup)
	<-c.cleanupDone

	c.mu.Lock()
	c.entries = make(map[string]*clientCacheEntry)
	c.mu.Unlock()
}

// Size returns the current number of entries in the cache.
func (c *clientCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
