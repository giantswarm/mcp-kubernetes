package server

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-oauth/providers/oidc"
)

func TestOAuthServerConfiguredIssuer(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		trustCA  bool
		mismatch bool
		error    string
	}{
		{name: "loopback IP with private CA", trustCA: true},
		{name: "untrusted CA", error: "certificate signed by unknown authority"},
		{name: "mismatched issuer", trustCA: true, mismatch: true, error: "does not match"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var requests atomic.Int32
			providerServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				require.Equal(t, "/dex/.well-known/openid-configuration", request.URL.Path)
				issuer := "https://" + request.Host + "/dex"
				if testCase.mismatch {
					issuer += "/other"
				}
				writer.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(writer).Encode(oidc.DiscoveryDocument{
					Issuer: issuer, AuthorizationEndpoint: "https://" + request.Host + "/auth",
					TokenEndpoint: "https://" + request.Host + "/token", JWKSUri: "https://" + request.Host + "/keys",
				}))
			}))
			t.Cleanup(providerServer.Close)
			configuration := OAuthConfig{
				BaseURL: "https://mcp.example.com", Provider: OAuthProviderDex,
				DexIssuerURL: providerServer.URL + "/dex", DexClientID: "mcp-kubernetes", DexClientSecret: "fixture",
			}
			if testCase.trustCA {
				configuration.DexCAFile = filepath.Join(t.TempDir(), "issuer-ca.pem")
				certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: providerServer.Certificate().Raw})
				require.NoError(t, os.WriteFile(configuration.DexCAFile, certificate, 0o600))
			}
			oauthServer, _, err := createOAuthServer(configuration)
			if testCase.error != "" {
				require.ErrorContains(t, err, testCase.error)
				return
			}
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, oauthServer.Shutdown(context.WithoutCancel(t.Context()))) })
			require.EqualValues(t, 1, requests.Load())
		})
	}
}
