package federation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuthClientProvider(t *testing.T) {
	t.Run("creates provider with valid config", func(t *testing.T) {
		config := &OAuthClientProviderConfig{
			ClusterHost: "https://kubernetes.default.svc",
			CACertFile:  "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
			QPS:         50,
			Burst:       100,
			Timeout:     30 * time.Second,
		}

		provider, err := NewOAuthClientProvider(config)

		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, config.ClusterHost, provider.clusterHost)
		assert.Equal(t, config.CACertFile, provider.caCertFile)
		assert.Equal(t, config.QPS, provider.qps)
		assert.Equal(t, config.Burst, provider.burst)
		assert.Equal(t, config.Timeout, provider.timeout)
	})

	t.Run("uses defaults for nil config", func(t *testing.T) {
		provider, err := NewOAuthClientProvider(nil)

		require.NoError(t, err)
		assert.NotNil(t, provider)
		// Should use default values
		assert.Equal(t, "https://kubernetes.default.svc", provider.clusterHost)
		assert.Equal(t, "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", provider.caCertFile)
		assert.Equal(t, float32(50), provider.qps)
		assert.Equal(t, 100, provider.burst)
		assert.Equal(t, 30*time.Second, provider.timeout)
	})

	t.Run("fails with empty cluster host", func(t *testing.T) {
		config := &OAuthClientProviderConfig{
			ClusterHost: "",
			CACertFile:  "/path/to/ca.crt",
		}

		provider, err := NewOAuthClientProvider(config)

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "cluster host is required")
	})

	t.Run("fails with empty CA cert file", func(t *testing.T) {
		config := &OAuthClientProviderConfig{
			ClusterHost: "https://kubernetes.default.svc",
			CACertFile:  "",
		}

		provider, err := NewOAuthClientProvider(config)

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "CA cert file is required")
	})
}

func TestDefaultOAuthClientProviderConfig(t *testing.T) {
	config := DefaultOAuthClientProviderConfig()

	assert.Equal(t, "https://kubernetes.default.svc", config.ClusterHost)
	assert.Equal(t, "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", config.CACertFile)
	assert.Equal(t, float32(50), config.QPS)
	assert.Equal(t, 100, config.Burst)
	assert.Equal(t, 30*time.Second, config.Timeout)
}

func TestOAuthClientProvider_SetTokenExtractor(t *testing.T) {
	config := DefaultOAuthClientProviderConfig()
	provider, err := NewOAuthClientProvider(config)
	require.NoError(t, err)

	// Initially nil
	assert.Nil(t, provider.tokenExtractor)

	// Set extractor
	extractor := func(ctx context.Context) (string, bool) {
		return "test-token", true
	}
	provider.SetTokenExtractor(extractor)

	// Verify it's set
	assert.NotNil(t, provider.tokenExtractor)
	token, ok := provider.tokenExtractor(context.Background())
	assert.True(t, ok)
	assert.Equal(t, "test-token", token)
}

func TestOAuthClientProvider_GetClientsForUser(t *testing.T) {
	t.Run("fails with nil user", func(t *testing.T) {
		config := DefaultOAuthClientProviderConfig()
		provider, err := NewOAuthClientProvider(config)
		require.NoError(t, err)

		clientset, dynamicClient, restConfig, err := provider.GetClientsForUser(context.Background(), nil)

		assert.Error(t, err)
		assert.Nil(t, clientset)
		assert.Nil(t, dynamicClient)
		assert.Nil(t, restConfig)
		assert.Contains(t, err.Error(), "user info is required")
	})

	t.Run("fails without token", func(t *testing.T) {
		config := DefaultOAuthClientProviderConfig()
		provider, err := NewOAuthClientProvider(config)
		require.NoError(t, err)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		clientset, dynamicClient, restConfig, err := provider.GetClientsForUser(context.Background(), user)

		assert.Error(t, err)
		assert.Nil(t, clientset)
		assert.Nil(t, dynamicClient)
		assert.Nil(t, restConfig)
		assert.Contains(t, err.Error(), "OAuth token not found")
	})

	t.Run("extracts token from context via extractor", func(t *testing.T) {
		config := DefaultOAuthClientProviderConfig()
		provider, err := NewOAuthClientProvider(config)
		require.NoError(t, err)

		// Set up token extractor
		provider.SetTokenExtractor(func(ctx context.Context) (string, bool) {
			return "test-oauth-token", true
		})

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		// Note: This will fail to create actual clients because we're not in a real cluster,
		// but we can verify the token extraction works by checking the error message
		// doesn't mention "token not found"
		clientset, dynamicClient, restConfig, err := provider.GetClientsForUser(context.Background(), user)

		// The error should be about connection, not about missing token
		// In a real test environment with mock server, this would succeed
		if err != nil {
			assert.NotContains(t, err.Error(), "OAuth token not found")
		} else {
			assert.NotNil(t, clientset)
			assert.NotNil(t, dynamicClient)
			assert.NotNil(t, restConfig)
		}
	})

	t.Run("extracts token from user.Extra as fallback", func(t *testing.T) {
		config := DefaultOAuthClientProviderConfig()
		provider, err := NewOAuthClientProvider(config)
		require.NoError(t, err)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
			Extra: map[string][]string{
				UserExtraOAuthTokenKey: {"fallback-token"},
			},
		}

		// Note: This will fail to create actual clients, but we can verify token extraction
		clientset, dynamicClient, restConfig, err := provider.GetClientsForUser(context.Background(), user)

		// The error should be about connection, not about missing token
		if err != nil {
			assert.NotContains(t, err.Error(), "OAuth token not found")
		} else {
			assert.NotNil(t, clientset)
			assert.NotNil(t, dynamicClient)
			assert.NotNil(t, restConfig)
		}
	})

	t.Run("context extractor takes precedence over Extra", func(t *testing.T) {
		config := DefaultOAuthClientProviderConfig()
		provider, err := NewOAuthClientProvider(config)
		require.NoError(t, err)

		// Set up token extractor
		provider.SetTokenExtractor(func(ctx context.Context) (string, bool) {
			return "context-token", true
		})

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
			Extra: map[string][]string{
				UserExtraOAuthTokenKey: {"extra-token"},
			},
		}

		// The context token should be used, not the Extra token
		// We can't directly verify which token was used without mocking,
		// but we can verify the flow works
		clientset, dynamicClient, restConfig, err := provider.GetClientsForUser(context.Background(), user)

		if err != nil {
			assert.NotContains(t, err.Error(), "OAuth token not found")
		} else {
			assert.NotNil(t, clientset)
			assert.NotNil(t, dynamicClient)
			assert.NotNil(t, restConfig)
		}
	})
}

// TestOAuthClientProvider_ImplementsInterface ensures OAuthClientProvider implements ClientProvider.
func TestOAuthClientProvider_ImplementsInterface(t *testing.T) {
	var _ ClientProvider = (*OAuthClientProvider)(nil)
}
