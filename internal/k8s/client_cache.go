package k8s

import (
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
//
// Note: There is a benign race condition between checking expiration and
// returning the client. Between releasing RLock and checking expiresAt,
// another goroutine could delete the entry. This is safe because:
//   - Worst case for expired check: we try to delete an already-deleted key (no-op)
//   - Worst case for return: we return a client that was just expired by another
//     goroutine, which will be replaced on the next request anyway
//
// This tradeoff is intentional to avoid holding locks during the time check.
func (c *clientCache) Get(token string) Client {
	key := hashToken(token)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil
	}

	// Check if expired (see note above about benign race condition)
	if time.Now().After(entry.expiresAt) {
		// Expired - remove it (safe even if already removed by another goroutine)
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
