package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/giantswarm/mcp-oauth/handler"
	oauthserver "github.com/giantswarm/mcp-oauth/server"
	"github.com/giantswarm/mcp-oauth/storage"
	"github.com/stretchr/testify/require"
)

func TestCIMDPreRegisteredAuthorization(t *testing.T) {
	const clientID = "https://client.invalid/metadata.json"
	authorizationServer, _, err := createOAuthServer(OAuthConfig{
		BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true,
		CIMDRestrictClientMetadata: true,
		CIMDPreRegisteredClientMetadata: []oauthserver.ClientMetadata{{
			ClientID: clientID, ClientName: "Test client",
			RedirectURIs: []string{"http://127.0.0.1:3000/callback"},
		}},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authorizationServer.Shutdown(context.WithoutCancel(t.Context()))) })

	for _, testCase := range []struct {
		name        string
		clientID    string
		redirectURI string
		statusCode  int
		body        string
	}{
		{"pre-registered client with another loopback port", clientID, "http://127.0.0.1:54321/callback", http.StatusOK, "Test client"},
		{"unregistered redirect URI", clientID, "https://attacker.invalid/callback", http.StatusBadRequest, "invalid_request"},
		{"unlisted CIMD client", "https://unlisted.invalid/metadata.json", "http://127.0.0.1:54321/callback", http.StatusBadRequest, "invalid_client"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			parameters := url.Values{
				"response_type":         {"code"},
				"client_id":             {testCase.clientID},
				"redirect_uri":          {testCase.redirectURI},
				"state":                 {"0123456789abcdef0123456789abcdef"},
				"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
				"code_challenge_method": {"S256"},
			}
			request := httptest.NewRequest(http.MethodGet, "https://mcp.example.com/oauth/authorize?"+parameters.Encode(), nil)
			response := httptest.NewRecorder()
			handler.New(authorizationServer, nil).ServeAuthorization(response, request)
			require.Equal(t, testCase.statusCode, response.Code)
			require.Empty(t, response.Header().Get("Location"))
			require.Contains(t, response.Body.String(), testCase.body)
		})
	}
}

func TestCIMDRestrictionPreservesRegisteredClients(t *testing.T) {
	authorizationServer, _, err := createOAuthServer(OAuthConfig{
		BaseURL: "https://mcp.example.com", Provider: OAuthProviderGoogle,
		GoogleClientID: "client", GoogleClientSecret: "fixture", EnableCIMD: true,
		CIMDRestrictClientMetadata: true,
		CIMDPreRegisteredClientMetadata: []oauthserver.ClientMetadata{{
			ClientID: "https://client.invalid/metadata.json", ClientName: "Test client",
			RedirectURIs: []string{"http://127.0.0.1:3000/callback"},
		}},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authorizationServer.Shutdown(context.WithoutCancel(t.Context()))) })

	registeredClient := &storage.Client{
		ClientID: "registered-client", ClientType: storage.ClientTypePublic,
		TokenEndpointAuthMethod: "none",
		RedirectURIs:            []string{"http://127.0.0.1:3000/callback"},
	}
	require.NoError(t, authorizationServer.SaveClient(t.Context(), registeredClient))
	client, err := authorizationServer.GetClient(t.Context(), registeredClient.ClientID)
	require.NoError(t, err)
	require.Equal(t, registeredClient.ClientID, client.ClientID)
}
