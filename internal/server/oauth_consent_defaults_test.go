package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOAuthConsentIsEnabledWithoutOverrides(t *testing.T) {
	configuration := OAuthConfig{BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true}
	authorizationServer, _, err := createOAuthServer(configuration)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authorizationServer.Shutdown(context.WithoutCancel(t.Context()))) })
	require.True(t, authorizationServer.RequiresConsent("https://client.example/metadata.json", "https://client.example/callback"))
	require.Empty(t, authorizationServer.Config.TrustedClients)
	require.Len(t, authorizationServer.Config.ConsentCookieKey, 32)
	require.False(t, authorizationServer.Config.AllowPrivateIPClientMetadata) //nolint:staticcheck // SA1019: verifies the defaults of compatibility fields used by current deployments.
	require.False(t, authorizationServer.Config.AllowPrivateIPRedirectURIs)   //nolint:staticcheck // SA1019: verifies the defaults of compatibility fields used by current deployments.
}

func TestPrivateOAuthHostsRemainScoped(t *testing.T) {
	configuration := OAuthConfig{BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true,
		CIMDPrivateIPHosts:  []string{"client.internal"},
		RedirectURISecurity: RedirectURISecurityConfig{PrivateIPRedirectURIHosts: []string{"callback.internal"}}}
	authorizationServer, _, err := createOAuthServer(configuration)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authorizationServer.Shutdown(context.WithoutCancel(t.Context()))) })
	require.Equal(t, []string{"client.internal"}, authorizationServer.Config.AllowPrivateIPClientMetadataHosts)
	require.Equal(t, []string{"callback.internal"}, authorizationServer.Config.AllowPrivateIPRedirectURIHosts)
	require.False(t, authorizationServer.Config.AllowPrivateIPClientMetadata) //nolint:staticcheck // SA1019: verifies that host lists do not enable the blanket compatibility fields.
	require.False(t, authorizationServer.Config.AllowPrivateIPRedirectURIs)   //nolint:staticcheck // SA1019: verifies that host lists do not enable the blanket compatibility fields.
}
