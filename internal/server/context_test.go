// Package server provides tests for ServerContext functionality.
package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
)

// mockK8sClient is a minimal mock for testing.
type mockK8sClient struct {
	k8s.Client
}

// mockClientFactory is a mock client factory for testing.
type mockClientFactory struct {
	client    k8s.Client
	createErr error
}

func (f *mockClientFactory) CreateBearerTokenClient(token string) (k8s.Client, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.client, nil
}

func TestK8sClientForContext_DownstreamOAuthDisabled(t *testing.T) {
	// When downstream OAuth is disabled, should always return the shared client
	sharedClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:       sharedClient,
		downstreamOAuth: false,
		logger:          NewDefaultLogger(),
	}

	ctx := context.Background()
	client, err := sc.K8sClientForContext(ctx)

	assert.NoError(t, err)
	assert.Same(t, sharedClient, client)
}

func TestK8sClientForContext_NoClientFactory(t *testing.T) {
	// When client factory is nil, should return the shared client
	sharedClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:       sharedClient,
		downstreamOAuth: true,
		clientFactory:   nil, // No factory
		logger:          NewDefaultLogger(),
	}

	ctx := context.Background()
	client, err := sc.K8sClientForContext(ctx)

	assert.NoError(t, err)
	assert.Same(t, sharedClient, client)
}

func TestK8sClientForContext_StrictMode_NoToken_Denied(t *testing.T) {
	// When strict mode is enabled and no token is present, should return error
	sharedClient := &mockK8sClient{}
	perUserClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: true, // Strict mode enabled
		clientFactory: &mockClientFactory{
			client: perUserClient,
		},
		logger: NewDefaultLogger(),
	}

	ctx := context.Background() // No token in context
	client, err := sc.K8sClientForContext(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, ErrOAuthTokenMissing), "expected ErrOAuthTokenMissing, got %v", err)
}

func TestK8sClientForContext_StrictMode_EmptyToken_Denied(t *testing.T) {
	// When strict mode is enabled and token is empty, should return error
	sharedClient := &mockK8sClient{}
	perUserClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: true,
		clientFactory: &mockClientFactory{
			client: perUserClient,
		},
		logger: NewDefaultLogger(),
	}

	ctx := oauth.ContextWithAccessToken(context.Background(), "") // Empty token
	client, err := sc.K8sClientForContext(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, ErrOAuthTokenMissing))
}

func TestK8sClientForContext_StrictMode_ClientCreationFailed_Denied(t *testing.T) {
	// When strict mode is enabled and client creation fails, should return error
	sharedClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: true,
		clientFactory: &mockClientFactory{
			createErr: errors.New("token rejected by API server"),
		},
		logger: NewDefaultLogger(),
	}

	ctx := oauth.ContextWithAccessToken(context.Background(), "valid-token")
	client, err := sc.K8sClientForContext(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, ErrOAuthClientFailed))
}

func TestK8sClientForContext_StrictMode_Success(t *testing.T) {
	// When strict mode is enabled and token is valid, should return per-user client
	sharedClient := &mockK8sClient{}
	perUserClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: true,
		clientFactory: &mockClientFactory{
			client: perUserClient,
		},
		logger: NewDefaultLogger(),
	}

	ctx := oauth.ContextWithAccessToken(context.Background(), "valid-token")
	client, err := sc.K8sClientForContext(ctx)

	assert.NoError(t, err)
	assert.Same(t, perUserClient, client)
	assert.NotSame(t, sharedClient, client)
}

func TestK8sClientForContext_NonStrict_NoToken_Fallback(t *testing.T) {
	// When strict mode is disabled and no token is present, should fall back to shared client
	sharedClient := &mockK8sClient{}
	perUserClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: false, // Strict mode disabled
		clientFactory: &mockClientFactory{
			client: perUserClient,
		},
		logger: NewDefaultLogger(),
	}

	ctx := context.Background() // No token in context
	client, err := sc.K8sClientForContext(ctx)

	assert.NoError(t, err)
	assert.Same(t, sharedClient, client) // Falls back to shared client
}

func TestK8sClientForContext_NonStrict_ClientCreationFailed_Fallback(t *testing.T) {
	// When strict mode is disabled and client creation fails, should fall back
	sharedClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: false, // Strict mode disabled
		clientFactory: &mockClientFactory{
			createErr: errors.New("token rejected"),
		},
		logger: NewDefaultLogger(),
	}

	ctx := oauth.ContextWithAccessToken(context.Background(), "valid-token")
	client, err := sc.K8sClientForContext(ctx)

	assert.NoError(t, err)
	assert.Same(t, sharedClient, client) // Falls back to shared client
}

func TestK8sClientForContext_NonStrict_Success(t *testing.T) {
	// When strict mode is disabled and token is valid, should return per-user client
	sharedClient := &mockK8sClient{}
	perUserClient := &mockK8sClient{}

	sc := &ServerContext{
		k8sClient:             sharedClient,
		downstreamOAuth:       true,
		downstreamOAuthStrict: false,
		clientFactory: &mockClientFactory{
			client: perUserClient,
		},
		logger: NewDefaultLogger(),
	}

	ctx := oauth.ContextWithAccessToken(context.Background(), "valid-token")
	client, err := sc.K8sClientForContext(ctx)

	assert.NoError(t, err)
	assert.Same(t, perUserClient, client)
}

func TestDownstreamOAuthStrictEnabled(t *testing.T) {
	tests := []struct {
		name     string
		strict   bool
		expected bool
	}{
		{
			name:     "strict mode enabled",
			strict:   true,
			expected: true,
		},
		{
			name:     "strict mode disabled",
			strict:   false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &ServerContext{
				downstreamOAuthStrict: tt.strict,
			}
			assert.Equal(t, tt.expected, sc.DownstreamOAuthStrictEnabled())
		})
	}
}

func TestWithDownstreamOAuthStrict(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "enable strict mode",
			enabled: true,
		},
		{
			name:    "disable strict mode",
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &ServerContext{}
			opt := WithDownstreamOAuthStrict(tt.enabled)
			err := opt(sc)

			require.NoError(t, err)
			assert.Equal(t, tt.enabled, sc.downstreamOAuthStrict)
		})
	}
}

func TestOAuthErrors(t *testing.T) {
	// Test that error types are properly defined
	assert.NotNil(t, ErrOAuthTokenMissing)
	assert.NotNil(t, ErrOAuthClientFailed)

	// Test error messages are informative
	assert.Contains(t, ErrOAuthTokenMissing.Error(), "authentication")
	assert.Contains(t, ErrOAuthClientFailed.Error(), "authentication")
}
