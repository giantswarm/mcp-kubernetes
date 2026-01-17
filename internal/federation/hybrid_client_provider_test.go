package federation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// Test constants to reduce string repetition and satisfy goconst
const testToken = "test-token"

// mockInClusterConfig returns a mock in-cluster config provider for testing.
func mockInClusterConfig(config *rest.Config, err error) InClusterConfigProvider {
	return func() (*rest.Config, error) {
		return config, err
	}
}

func TestNewHybridOAuthClientProvider(t *testing.T) {
	t.Run("creates provider with valid config", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       newTestLogger(),
		}

		provider, err := NewHybridOAuthClientProvider(config)

		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Same(t, userProvider, provider.userProvider)
	})

	t.Run("fails with nil config", func(t *testing.T) {
		provider, err := NewHybridOAuthClientProvider(nil)

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "config is required")
	})

	t.Run("fails with nil user provider", func(t *testing.T) {
		config := &HybridOAuthClientProviderConfig{
			UserProvider: nil,
			Logger:       newTestLogger(),
		}

		provider, err := NewHybridOAuthClientProvider(config)

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "user provider is required")
	})

	t.Run("uses default logger when not provided", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       nil, // Should use default
		}

		provider, err := NewHybridOAuthClientProvider(config)

		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.NotNil(t, provider.logger)
	})
}

func TestHybridOAuthClientProvider_GetClientsForUser(t *testing.T) {
	t.Run("delegates to user provider", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		// Set up a token extractor so we can test the flow
		userProvider.SetTokenExtractor(func(ctx context.Context) (string, bool) {
			return testToken, true
		})

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       newTestLogger(),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		// This will fail to create actual clients but we can verify delegation
		clientset, dynamicClient, restConfig, err := provider.GetClientsForUser(context.Background(), user)

		// Token was found so error should not be about missing token
		if err != nil {
			assert.NotContains(t, err.Error(), "OAuth token not found")
		} else {
			assert.NotNil(t, clientset)
			assert.NotNil(t, dynamicClient)
			assert.NotNil(t, restConfig)
		}
	})
}

func TestHybridOAuthClientProvider_GetPrivilegedClientForSecrets(t *testing.T) {
	t.Run("returns ServiceAccount client when config is available", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		// Create a mock in-cluster config
		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(mockConfig, nil),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		client, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)

		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("returns error when in-cluster config fails", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		configErr := errors.New("not running in cluster")

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(nil, configErr),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		user := &UserInfo{
			Email: "user@example.com",
		}

		client, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)

		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to initialize ServiceAccount client")
	})

	t.Run("initialization is lazy and cached", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		callCount := 0
		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       newTestLogger(),
			ConfigProvider: func() (*rest.Config, error) {
				callCount++
				return mockConfig, nil
			},
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		user := &UserInfo{Email: "user@example.com"}

		// First call
		client1, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)
		require.NoError(t, err)
		assert.NotNil(t, client1)
		assert.Equal(t, 1, callCount, "config should be created on first call")

		// Second call
		client2, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)
		require.NoError(t, err)
		assert.NotNil(t, client2)
		assert.Equal(t, 1, callCount, "config should be cached, not created again")

		// Should return the same client
		assert.Same(t, client1, client2)
	})
}

func TestHybridOAuthClientProvider_HasPrivilegedAccess(t *testing.T) {
	t.Run("returns true when ServiceAccount client is available", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(mockConfig, nil),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		hasAccess := provider.HasPrivilegedAccess()

		assert.True(t, hasAccess)
	})

	t.Run("returns false when in-cluster config fails", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		configErr := errors.New("not running in cluster")

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(nil, configErr),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		hasAccess := provider.HasPrivilegedAccess()

		assert.False(t, hasAccess)
	})
}

func TestHybridOAuthClientProvider_SetTokenExtractor(t *testing.T) {
	t.Run("passes through to user provider", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       newTestLogger(),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		// Set token extractor via hybrid provider
		provider.SetTokenExtractor(func(ctx context.Context) (string, bool) {
			return "hybrid-token", true
		})

		// Verify it was set on the user provider
		token, ok := provider.userProvider.tokenExtractor(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "hybrid-token", token)
	})
}

func TestHybridOAuthClientProvider_SetMetrics(t *testing.T) {
	t.Run("passes through to user provider", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       newTestLogger(),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		metrics := &mockOAuthMetricsRecorder{}
		provider.SetMetrics(metrics)

		// Verify it was set on the user provider
		assert.Same(t, metrics, provider.userProvider.metrics)
	})
}

func TestHybridOAuthClientProvider_ImplementsInterfaces(t *testing.T) {
	t.Run("implements ClientProvider", func(t *testing.T) {
		var _ ClientProvider = (*HybridOAuthClientProvider)(nil)
	})

	t.Run("implements PrivilegedSecretAccessProvider", func(t *testing.T) {
		var _ PrivilegedSecretAccessProvider = (*HybridOAuthClientProvider)(nil)
	})
}

func TestGetSecretAccessClient_Integration(t *testing.T) {
	// These tests verify the Manager's getSecretAccessClient method works
	// correctly with both privileged and non-privileged providers.

	t.Run("uses privileged client when available", func(t *testing.T) {
		// Create a mock hybrid provider
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		hybridConfig := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(mockConfig, nil),
		}

		hybridProvider, err := NewHybridOAuthClientProvider(hybridConfig)
		require.NoError(t, err)

		// Create manager with hybrid provider
		manager, err := NewManager(hybridProvider, WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		defer func() { _ = manager.Close() }()

		user := &UserInfo{Email: "user@example.com"}

		// Get secret access client
		client, err := manager.getSecretAccessClient(context.Background(), "test-cluster", user)

		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("falls back to user client when privileged not available", func(t *testing.T) {
		// Create a regular static provider (not privileged)
		fakeClient := fake.NewClientset()
		testScheme := runtime.NewScheme()
		fakeDynamic := createTestFakeDynamicClient(testScheme)

		staticProvider := &StaticClientProvider{
			Clientset:     fakeClient,
			DynamicClient: fakeDynamic,
		}

		// Create manager with static provider
		manager, err := NewManager(staticProvider, WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		defer func() { _ = manager.Close() }()

		user := &UserInfo{Email: "user@example.com"}

		// Get secret access client - should use user client
		client, err := manager.getSecretAccessClient(context.Background(), "test-cluster", user)

		require.NoError(t, err)
		assert.NotNil(t, client)
		// Should be the same as the static client
		assert.Same(t, fakeClient, client)
	})

	t.Run("falls back gracefully when privileged access fails", func(t *testing.T) {
		// Create hybrid provider with failing config
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		// Set token so we can actually get user clients
		userProvider.SetTokenExtractor(func(ctx context.Context) (string, bool) {
			return testToken, true
		})

		configErr := errors.New("not running in cluster")

		hybridConfig := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(nil, configErr),
		}

		hybridProvider, err := NewHybridOAuthClientProvider(hybridConfig)
		require.NoError(t, err)

		// Create manager with hybrid provider
		manager, err := NewManager(hybridProvider, WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		defer func() { _ = manager.Close() }()

		user := &UserInfo{
			Email: "user@example.com",
			Extra: map[string][]string{
				UserExtraOAuthTokenKey: {"fallback-token"},
			},
		}

		// Get secret access client - should fall back to user client
		// This will fail because we're not in a real cluster, but it should
		// attempt to use user credentials, not return an error about privileged access
		client, err := manager.getSecretAccessClient(context.Background(), "test-cluster", user)

		// May fail due to not being in real cluster, but not due to privileged access
		if err != nil {
			assert.NotContains(t, err.Error(), "privileged")
		} else {
			assert.NotNil(t, client)
		}
	})
}

func TestDefaultInClusterConfigProvider(t *testing.T) {
	t.Run("returns a function", func(t *testing.T) {
		provider := DefaultInClusterConfigProvider()
		assert.NotNil(t, provider)

		// When not in cluster, this will return an error
		config, err := provider()
		// We expect an error when not running in-cluster
		assert.Error(t, err)
		assert.Nil(t, config)
	})
}

// mockPrivilegedAccessMetrics implements PrivilegedSecretAccessMetricsRecorder for testing
type mockPrivilegedAccessMetrics struct {
	recordings []struct {
		userDomain string
		result     string
	}
}

func (m *mockPrivilegedAccessMetrics) RecordPrivilegedSecretAccess(_ context.Context, userDomain, result string) {
	m.recordings = append(m.recordings, struct {
		userDomain string
		result     string
	}{userDomain, result})
}

func TestHybridOAuthClientProvider_RateLimiting(t *testing.T) {
	t.Run("allows requests within rate limit", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:       userProvider,
			Logger:             newTestLogger(),
			ConfigProvider:     mockInClusterConfig(mockConfig, nil),
			RateLimitPerSecond: 100.0, // High limit for testing
			RateLimitBurst:     100,
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		user := &UserInfo{Email: "user@example.com"}

		// Make several requests - all should succeed
		for i := 0; i < 10; i++ {
			client, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)
			require.NoError(t, err)
			assert.NotNil(t, client)
		}
	})

	t.Run("rate limits excessive requests", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:       userProvider,
			Logger:             newTestLogger(),
			ConfigProvider:     mockInClusterConfig(mockConfig, nil),
			RateLimitPerSecond: 1.0,
			RateLimitBurst:     2, // Only allow 2 requests
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		user := &UserInfo{Email: "user@example.com"}

		// First two requests should succeed (burst)
		for i := 0; i < 2; i++ {
			client, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)
			require.NoError(t, err)
			assert.NotNil(t, client)
		}

		// Third request should be rate limited
		client, err := provider.GetPrivilegedClientForSecrets(context.Background(), user)
		assert.ErrorIs(t, err, ErrRateLimited)
		assert.Nil(t, client)
	})

	t.Run("rate limits are per-user", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:       userProvider,
			Logger:             newTestLogger(),
			ConfigProvider:     mockInClusterConfig(mockConfig, nil),
			RateLimitPerSecond: 1.0,
			RateLimitBurst:     1, // Only allow 1 request per user
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		user1 := &UserInfo{Email: "user1@example.com"}
		user2 := &UserInfo{Email: "user2@example.com"}

		// First request for each user should succeed
		client1, err := provider.GetPrivilegedClientForSecrets(context.Background(), user1)
		require.NoError(t, err)
		assert.NotNil(t, client1)

		client2, err := provider.GetPrivilegedClientForSecrets(context.Background(), user2)
		require.NoError(t, err)
		assert.NotNil(t, client2)

		// Second request for user1 should be rate limited
		client, err := provider.GetPrivilegedClientForSecrets(context.Background(), user1)
		assert.ErrorIs(t, err, ErrRateLimited)
		assert.Nil(t, client)

		// User2 should also be rate limited for second request
		client, err = provider.GetPrivilegedClientForSecrets(context.Background(), user2)
		assert.ErrorIs(t, err, ErrRateLimited)
		assert.Nil(t, client)
	})
}

func TestHybridOAuthClientProvider_Metrics(t *testing.T) {
	t.Run("records success metric", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		metrics := &mockPrivilegedAccessMetrics{}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(mockConfig, nil),
			Metrics:        metrics,
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		user := &UserInfo{Email: "user@example.com"}

		_, err = provider.GetPrivilegedClientForSecrets(context.Background(), user)
		require.NoError(t, err)

		require.Len(t, metrics.recordings, 1)
		assert.Equal(t, "example.com", metrics.recordings[0].userDomain)
		assert.Equal(t, "success", metrics.recordings[0].result)
	})

	t.Run("records rate_limited metric", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		metrics := &mockPrivilegedAccessMetrics{}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:       userProvider,
			Logger:             newTestLogger(),
			ConfigProvider:     mockInClusterConfig(mockConfig, nil),
			Metrics:            metrics,
			RateLimitPerSecond: 1.0,
			RateLimitBurst:     1,
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		user := &UserInfo{Email: "user@example.com"}

		// First request succeeds
		_, err = provider.GetPrivilegedClientForSecrets(context.Background(), user)
		require.NoError(t, err)

		// Second request is rate limited
		_, err = provider.GetPrivilegedClientForSecrets(context.Background(), user)
		assert.ErrorIs(t, err, ErrRateLimited)

		require.Len(t, metrics.recordings, 2)
		assert.Equal(t, "success", metrics.recordings[0].result)
		assert.Equal(t, "rate_limited", metrics.recordings[1].result)
	})

	t.Run("records error metric on init failure", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		configErr := errors.New("not running in cluster")
		metrics := &mockPrivilegedAccessMetrics{}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(nil, configErr),
			Metrics:        metrics,
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		user := &UserInfo{Email: "user@example.com"}

		_, err = provider.GetPrivilegedClientForSecrets(context.Background(), user)
		assert.Error(t, err)

		require.Len(t, metrics.recordings, 1)
		assert.Equal(t, "error", metrics.recordings[0].result)
	})
}

func TestHybridOAuthClientProvider_StrictMode(t *testing.T) {
	t.Run("IsStrictMode returns correct value", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		// Non-strict mode
		config := &HybridOAuthClientProviderConfig{
			UserProvider:           userProvider,
			Logger:                 newTestLogger(),
			StrictPrivilegedAccess: false,
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()
		assert.False(t, provider.IsStrictMode())

		// Strict mode
		config2 := &HybridOAuthClientProviderConfig{
			UserProvider:           userProvider,
			Logger:                 newTestLogger(),
			StrictPrivilegedAccess: true,
		}

		provider2, err := NewHybridOAuthClientProvider(config2)
		require.NoError(t, err)
		defer provider2.Close()
		assert.True(t, provider2.IsStrictMode())
	})
}

func TestHybridOAuthClientProvider_SetPrivilegedAccessMetrics(t *testing.T) {
	t.Run("sets metrics correctly", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		mockConfig := &rest.Config{
			Host: "https://kubernetes.default.svc",
		}

		config := &HybridOAuthClientProviderConfig{
			UserProvider:   userProvider,
			Logger:         newTestLogger(),
			ConfigProvider: mockInClusterConfig(mockConfig, nil),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)
		defer provider.Close()

		metrics := &mockPrivilegedAccessMetrics{}
		provider.SetPrivilegedAccessMetrics(metrics)

		user := &UserInfo{Email: "user@example.com"}
		_, err = provider.GetPrivilegedClientForSecrets(context.Background(), user)
		require.NoError(t, err)

		require.Len(t, metrics.recordings, 1)
		assert.Equal(t, "success", metrics.recordings[0].result)
	})
}

func TestExtractUserDomain(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"user@example.com", "example.com"},
		{"admin@giantswarm.io", "giantswarm.io"},
		{"", unknownDomain},
		{"noemail", unknownDomain},
		{"@nodomain", "nodomain"},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			result := extractUserDomain(tt.email)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHybridOAuthClientProvider_Close(t *testing.T) {
	t.Run("can be called multiple times safely", func(t *testing.T) {
		userProvider, err := NewOAuthClientProvider(DefaultOAuthClientProviderConfig())
		require.NoError(t, err)

		config := &HybridOAuthClientProviderConfig{
			UserProvider: userProvider,
			Logger:       newTestLogger(),
		}

		provider, err := NewHybridOAuthClientProvider(config)
		require.NoError(t, err)

		// Close multiple times - should not panic
		provider.Close()
		provider.Close()
		provider.Close()
	})
}

// Note: mockMetricsRecorder and newMockMetricsRecorder are defined in cache_test.go
// and shared across all test files in the federation package.
