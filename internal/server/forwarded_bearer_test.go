package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-oauth/handler"
	"github.com/giantswarm/mcp-oauth/security"
	"github.com/giantswarm/mcp-oauth/storage"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"golang.org/x/oauth2"

	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
)

// The fixture serves discovery and JWKS over a private CA and signs RS256
// tokens with a kid and no typ, as Dex does.
type forwardedDex struct {
	issuer        string
	caFile        string
	key           *rsa.PrivateKey
	keysAvailable atomic.Bool
}

func newForwardedDex(t *testing.T) *forwardedDex {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	d := &forwardedDex{key: key}
	d.keysAvailable.Store(true)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                d.issuer,
				"authorization_endpoint":                d.issuer + "/auth",
				"token_endpoint":                        d.issuer + "/token",
				"userinfo_endpoint":                     d.issuer + "/userinfo",
				"jwks_uri":                              d.issuer + "/keys",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			}))
		case "/keys":
			if !d.keysAvailable.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key: &key.PublicKey, KeyID: "dex-key", Algorithm: string(jose.RS256), Use: "sig",
			}}}))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer provider-access-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"sub": "fixture-user", "email": "dev@lab.local", "email_verified": true, "groups": []string{"developers"},
			}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"localhost"}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &key.PublicKey, key)
	require.NoError(t, err)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	d.issuer = strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	d.caFile = filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(d.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))
	return d
}

func (d *forwardedDex) token(t *testing.T, extra map[string]any, key *rsa.PrivateKey) string {
	t.Helper()
	if key == nil {
		key = d.key
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "dex-key"}}, nil)
	require.NoError(t, err)
	claims := map[string]any{
		"iss": d.issuer, "sub": "fixture-user", "aud": []string{"kubernetes", "agent-platform"},
		"exp": time.Now().Add(10 * time.Minute).Unix(), "iat": time.Now().Add(-time.Minute).Unix(),
		"email": "dev@lab.local", "email_verified": true, "groups": []string{"developers"},
	}
	maps.Copy(claims, extra)
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(t, err)
	return token
}

func (d *forwardedDex) server(t *testing.T, trusted []string) *OAuthHTTPServer {
	t.Helper()
	srv, err := NewOAuthHTTPServer(mcpserver.NewMCPServer("fixture", "1"), "streamable-http", OAuthConfig{
		BaseURL: "https://mcp.example.com", Provider: OAuthProviderDex,
		DexIssuerURL: d.issuer, DexClientID: "mcp-kubernetes", DexClientSecret: "test-secret",
		DexCAFile: d.caFile, TrustedAudiences: trusted, SSOAllowPrivateIPs: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.oauthServer.Shutdown(context.WithoutCancel(t.Context()))) })
	return srv
}

func TestMCPRefusesForwardedIDTokens(t *testing.T) {
	issuer := newForwardedDex(t)
	server := issuer.server(t, []string{"agent-platform", "kubernetes"})
	require.True(t, server.oauthServer.Config.RequireResourceAccessTokens)
	require.False(t, server.oauthServer.Config.AllowPrivateIPJWKS)
	mux := http.NewServeMux()
	require.NoError(t, server.setupMCPRoutes(mux))
	endpoint := httptest.NewServer(mux)
	t.Cleanup(endpoint.Close)
	wireRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint.URL+"/mcp", nil)
	require.NoError(t, err)
	wireRequest.Header.Set("Authorization", "Bearer "+issuer.token(t, nil, nil))
	wireResponse, err := endpoint.Client().Do(wireRequest)
	require.NoError(t, err)
	require.NoError(t, wireResponse.Body.Close())
	require.Equal(t, http.StatusUnauthorized, wireResponse.StatusCode)
	var auditLog bytes.Buffer
	server.oauthServer.Auditor = security.NewAuditor(slog.New(slog.NewJSONHandler(&auditLog, nil)), true)
	var refusal string
	for _, testCase := range []struct {
		name   string
		claims map[string]any
	}{
		{name: "trusted client audience"},
		{name: "MCP audience on an ID token", claims: map[string]any{"aud": "https://mcp.example.com"}},
		{name: "untrusted audience", claims: map[string]any{"aud": "other"}},
		{name: "expired ID token", claims: map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}},
		{name: "delegated identity", claims: map[string]any{"act": map[string]any{"sub": "agent"}}},
		{name: "sender constrained identity", claims: map[string]any{"cnf": map[string]any{"jkt": "fixture"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			auditLog.Reset()
			bearer := issuer.token(t, testCase.claims, nil)
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			request.Header.Set("Authorization", "Bearer "+bearer)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Contains(t, response.Header().Get("WWW-Authenticate"), "invalid_token")
			body := response.Body.String()
			if refusal == "" {
				refusal = body
			}
			require.Equal(t, refusal, body)
			require.NotContains(t, body, bearer)
			var event struct {
				Audit struct {
					Type    string         `json:"event_type"`
					Details map[string]any `json:"details"`
				} `json:"audit"`
			}
			require.NoError(t, json.NewDecoder(&auditLog).Decode(&event))
			require.Equal(t, security.EventAuthFailure, event.Audit.Type)
			require.NotEmpty(t, event.Audit.Details["reason"])
		})
	}
}

func TestMCPResourceAccessToken(t *testing.T) {
	issuer := newForwardedDex(t)
	server := issuer.server(t, []string{"agent-platform"})
	metadataStore, ok := server.tokenStore.(storage.TokenMetadataStore)
	require.True(t, ok)
	providerStore, ok := server.tokenStore.(storage.UserProviderTokenStore)
	require.True(t, ok)
	identityToken := issuer.token(t, nil, nil)
	providerToken := (&oauth2.Token{AccessToken: "provider-access-token", Expiry: time.Now().Add(time.Hour)}).
		WithExtra(map[string]any{"id_token": identityToken})
	for _, testCase := range []struct {
		name     string
		audience string
		status   int
	}{
		{name: "own resource", audience: server.oauthServer.Config.GetResourceIdentifier(), status: http.StatusOK},
		{name: "other resource", audience: "https://other.example.com", status: http.StatusUnauthorized},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			accessToken := "resource-access-" + strings.ReplaceAll(testCase.name, " ", "-")
			require.NoError(t, providerStore.SaveUserProviderToken(t.Context(), "fixture-user", providerToken))
			require.NoError(t, providerStore.SaveProviderTokenRef(t.Context(), accessToken, "fixture-user", time.Now().Add(time.Hour)))
			require.NoError(t, metadataStore.SaveTokenMetadata(t.Context(), accessToken, storage.TokenMetadata{
				UserID: "fixture-user", ClientID: "client", TokenType: "access", Audience: testCase.audience,
				ExpiresAt: time.Now().Add(time.Hour),
			}))
			mux := http.NewServeMux()
			require.NoError(t, server.setupMCPRoutes(mux))
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`))
			request.Header.Set("Authorization", "Bearer "+accessToken)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			require.Equal(t, testCase.status, response.Code, response.Body.String())
			if testCase.status != http.StatusOK {
				return
			}
			require.Contains(t, response.Body.String(), "serverInfo")
			validated := server.oauthHandler.ValidateToken(server.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				identity, found := handler.UserInfoFromContext(request.Context())
				require.True(t, found)
				require.False(t, identity.IsSSO())
				injected, found := oauth.GetIDTokenFromContext(request.Context())
				require.True(t, found)
				require.Equal(t, identityToken, injected)
				require.NotEqual(t, accessToken, injected)
				writer.WriteHeader(http.StatusNoContent)
			})))
			response = httptest.NewRecorder()
			validated.ServeHTTP(response, request)
			require.Equal(t, http.StatusNoContent, response.Code)
		})
	}
}

func TestInvalidBearerUsesSharedRateLimiter(t *testing.T) {
	issuer := newForwardedDex(t)
	server := issuer.server(t, []string{"agent-platform"})
	server.oauthServer.RateLimiter.Stop()
	server.oauthServer.RateLimiter = security.NewRateLimiter(0, 1, server.oauthServer.Logger)
	mux := http.NewServeMux()
	require.NoError(t, server.setupMCPRoutes(mux))
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
		request.Header.Set("Authorization", "Bearer "+issuer.token(t, nil, nil))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, status, response.Code, response.Body.String())
	}
}
