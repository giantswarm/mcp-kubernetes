package server

import (
	"context"
	"testing"

	oauthserver "github.com/giantswarm/mcp-oauth/server"
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
	require.Equal(t, []string{"client.internal"}, authorizationServer.Config.AllowPrivateIPClientMetadataHosts) //nolint:staticcheck // SA1019: the deprecated host list keeps working until it is removed.
	require.Equal(t, []string{"callback.internal"}, authorizationServer.Config.AllowPrivateIPRedirectURIHosts)
	require.False(t, authorizationServer.Config.AllowPrivateIPClientMetadata) //nolint:staticcheck // SA1019: verifies that host lists do not enable the blanket compatibility fields.
	require.False(t, authorizationServer.Config.AllowPrivateIPRedirectURIs)   //nolint:staticcheck // SA1019: verifies that host lists do not enable the blanket compatibility fields.
}

func TestPinnedCIMDClientWithRestrictClientMetadata(t *testing.T) {
	const pinned = "https://muster.example.com/.well-known/oauth-client.json"
	configuration := OAuthConfig{BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true,
		CIMDPreRegisteredClientMetadata: []oauthserver.ClientMetadata{{
			ClientID:     pinned,
			ClientName:   "muster",
			RedirectURIs: []string{"https://muster.example.com/oauth/callback"},
		}},
		CIMDRestrictClientMetadata: true}
	authorizationServer, _, err := createOAuthServer(configuration)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authorizationServer.Shutdown(context.WithoutCancel(t.Context()))) })

	client, err := authorizationServer.GetClient(t.Context(), pinned)
	require.NoError(t, err)
	require.Equal(t, []string{"https://muster.example.com/oauth/callback"}, client.RedirectURIs)

	_, err = authorizationServer.GetClient(t.Context(), "https://unpinned.invalid/client.json")
	require.ErrorIs(t, err, oauthserver.ErrClientMetadataNotAllowed)
}

func TestRestrictClientMetadataWithoutPinsIsRefused(t *testing.T) {
	configuration := OAuthConfig{BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true,
		CIMDRestrictClientMetadata: true}
	_, _, err := createOAuthServer(configuration)
	require.Error(t, err)
}

func TestDeprecatedPrivateIPSettingsArePassedThrough(t *testing.T) {
	configuration := OAuthConfig{BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true,
		CIMDAllowPrivateIPs: true,
		RedirectURISecurity: RedirectURISecurityConfig{AllowPrivateIPRedirectURIs: true}}
	authorizationServer, _, err := createOAuthServer(configuration)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authorizationServer.Shutdown(context.WithoutCancel(t.Context()))) })
	require.True(t, authorizationServer.Config.AllowPrivateIPClientMetadata) //nolint:staticcheck // SA1019: the deprecated settings keep working until they are removed.
	require.True(t, authorizationServer.Config.AllowPrivateIPRedirectURIs)   //nolint:staticcheck // SA1019: the deprecated settings keep working until they are removed.
}
