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
	require.False(t, authorizationServer.Config.AllowPrivateIPClientMetadata)
	require.False(t, authorizationServer.Config.AllowPrivateIPRedirectURIs)
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
	require.False(t, authorizationServer.Config.AllowPrivateIPClientMetadata)
	require.False(t, authorizationServer.Config.AllowPrivateIPRedirectURIs)
}
