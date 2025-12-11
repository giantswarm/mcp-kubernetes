package k8s

import (
	"testing"
	"time"
)

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
	// Use very short TTL for testing
	cache := &clientCache{
		entries:     make(map[string]*clientCacheEntry),
		ttl:         50 * time.Millisecond,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}
	// Don't start the cleanup goroutine - we'll call cleanup manually
	go func() {
		<-cache.stopCleanup
		close(cache.cleanupDone)
	}()
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
