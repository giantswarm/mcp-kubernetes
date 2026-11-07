package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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
	mcpserver "github.com/mark3labs/mcp-go/server"

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
	t.Cleanup(func() { require.NoError(t, srv.oauthServer.Shutdown(context.Background())) })
	return srv
}

func TestMCPForwardedDexToken(t *testing.T) {
	d := newForwardedDex(t)
	srv := d.server(t, []string{"agent-platform"})
	mux := http.NewServeMux()
	require.NoError(t, srv.setupMCPRoutes(mux))
	badKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		extra  map[string]any
		key    *rsa.PrivateKey
		raw    string
		status int
	}{
		{name: "trusted cross-client audiences", status: http.StatusOK},
		{name: "untrusted audience", extra: map[string]any{"aud": "other"}, status: http.StatusUnauthorized},
		{name: "untrusted issuer", extra: map[string]any{"iss": "https://untrusted.example.com"}, status: http.StatusUnauthorized},
		{name: "expired", extra: map[string]any{"exp": time.Now().Add(-10 * time.Minute).Unix()}, status: http.StatusUnauthorized},
		{name: "bad signature", key: badKey, status: http.StatusUnauthorized},
		{name: "delegated", extra: map[string]any{"act": map[string]any{"sub": "agent"}}, status: http.StatusUnauthorized},
		{name: "sender constrained without proof", extra: map[string]any{"cnf": map[string]any{"jkt": "fixture-thumbprint"}}, status: http.StatusUnauthorized},
		{name: "missing email", extra: map[string]any{"email": ""}, status: http.StatusUnauthorized},
		{name: "unknown opaque", raw: "unknown-opaque-token", status: http.StatusUnauthorized},
		{name: "malformed", raw: "not.a.jwt", status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bearer := tc.raw
			if bearer == "" {
				bearer = d.token(t, tc.extra, tc.key)
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`))
			req.Header.Set("Authorization", "Bearer "+bearer)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status == http.StatusOK {
				require.Contains(t, response.Body.String(), "serverInfo")
			}
		})
	}
}

func TestForwardedDexIdentityAndLimits(t *testing.T) {
	d := newForwardedDex(t)
	bearer := d.token(t, nil, nil)
	srv := d.server(t, []string{"agent-platform"})
	calls := 0
	next := srv.authenticateForwardedBearer(srv.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		info, ok := handler.UserInfoFromContext(r.Context())
		require.True(t, ok)
		require.True(t, info.IsSSO())
		session, ok := handler.SessionIDFromContext(r.Context())
		require.True(t, ok)
		require.Equal(t, srv.oauthServer.SessionIDForBearer(bearer), session)
		require.Equal(t, "dev@lab.local", info.Email)
		require.Equal(t, []string{"developers"}, info.Groups)
		injected, ok := oauth.GetIDTokenFromContext(r.Context())
		require.True(t, ok)
		require.Equal(t, bearer, injected)
		w.WriteHeader(http.StatusOK)
	})))
	call := func() *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
		request.Header.Set("Authorization", "Bearer "+bearer)
		response := httptest.NewRecorder()
		next.ServeHTTP(response, request)
		return response
	}
	require.Equal(t, http.StatusOK, call().Code)
	srv.oauthServer.UserRateLimiter.Stop()
	srv.oauthServer.UserRateLimiter = security.NewRateLimiter(0, 1, srv.oauthServer.Logger)
	require.Equal(t, http.StatusOK, call().Code)
	require.Equal(t, http.StatusTooManyRequests, call().Code)
	srv.oauthServer.UserRateLimiter.Stop()
	srv.oauthServer.UserRateLimiter = nil
	srv.oauthServer.RateLimiter.Stop()
	srv.oauthServer.RateLimiter = security.NewRateLimiter(0, 1, srv.oauthServer.Logger)
	require.Equal(t, http.StatusOK, call().Code)
	require.Equal(t, http.StatusTooManyRequests, call().Code)
	require.Equal(t, 3, calls)
}

func TestForwardedDexTrustAndAvailability(t *testing.T) {
	for _, tc := range []struct {
		name          string
		trusted       []string
		keysAvailable bool
		want          int
	}{
		{name: "forwarding disabled", keysAvailable: true, want: http.StatusUnauthorized},
		{name: "issuer keys unavailable", trusted: []string{"agent-platform"}, want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newForwardedDex(t)
			d.keysAvailable.Store(tc.keysAvailable)
			srv := d.server(t, tc.trusted)
			next := srv.authenticateForwardedBearer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("rejected token reached the handler")
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			req.Header.Set("Authorization", "Bearer "+d.token(t, nil, nil))
			response := httptest.NewRecorder()
			next.ServeHTTP(response, req)
			require.Equal(t, tc.want, response.Code, response.Body.String())
			if tc.want == http.StatusServiceUnavailable {
				require.Equal(t, "1", response.Header().Get("Retry-After"))
			}
		})
	}
}
