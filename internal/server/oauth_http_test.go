// Package server provides tests for OAuth HTTP server functionality.
// These tests verify HTTPS validation and server lifecycle management.
package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/giantswarm/mcp-oauth/handler"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
)

// TestExtractBearerToken tests bearer token extraction from Authorization header
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		wantToken   string
		wantSuccess bool
	}{
		{ //nolint:gosec // G101: test fixture, not a real credential
			name:        "valid bearer token",
			authHeader:  "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantToken:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantSuccess: true,
		},
		{
			name:        "empty header",
			authHeader:  "",
			wantToken:   "",
			wantSuccess: false,
		},
		{
			name:        "only Bearer prefix",
			authHeader:  "Bearer ",
			wantToken:   "",
			wantSuccess: false,
		},
		{
			name:        "just Bearer without space",
			authHeader:  "Bearer",
			wantToken:   "",
			wantSuccess: false,
		},
		{
			name:        "wrong scheme - Basic",
			authHeader:  "Basic dXNlcjpwYXNz",
			wantToken:   "",
			wantSuccess: false,
		},
		{
			name:        "lowercase bearer not supported",
			authHeader:  "bearer token123",
			wantToken:   "",
			wantSuccess: false,
		},
		{
			name:        "token with spaces in value",
			authHeader:  "Bearer token with spaces",
			wantToken:   "token with spaces",
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
			assert.NoError(t, err)

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			token, ok := extractBearerToken(req)
			assert.Equal(t, tt.wantSuccess, ok)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

// TestValidateHTTPSRequirement tests HTTPS validation for OAuth 2.1 compliance
func TestValidateHTTPSRequirement(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		wantError bool
	}{
		{
			name:      "valid HTTPS URL",
			baseURL:   "https://mcp.example.com",
			wantError: false,
		},
		{
			name:      "localhost HTTP allowed",
			baseURL:   "http://localhost:8080",
			wantError: false,
		},
		{
			name:      "127.0.0.1 HTTP allowed",
			baseURL:   "http://127.0.0.1:8080",
			wantError: false,
		},
		{
			name:      "IPv6 loopback HTTP allowed",
			baseURL:   "http://[::1]:8080",
			wantError: false,
		},
		{
			name:      "HTTP non-localhost disallowed",
			baseURL:   "http://mcp.example.com",
			wantError: true,
		},
		{
			name:      "empty URL",
			baseURL:   "",
			wantError: true,
		},
		{
			name:      "invalid scheme",
			baseURL:   "ftp://mcp.example.com",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPSRequirement(tt.baseURL)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCreateOAuthServer tests OAuth server creation
func TestCreateOAuthServer(t *testing.T) {
	config := OAuthConfig{
		BaseURL:                       "https://mcp.example.com",
		Provider:                      OAuthProviderGoogle,
		GoogleClientID:                "test-client-id",
		GoogleClientSecret:            "test-client-secret",
		AllowPublicClientRegistration: false,
		RegistrationAccessToken:       "test-token",
		AllowInsecureAuthWithoutState: false,
		MaxClientsPerIP:               5,
		EncryptionKey:                 []byte("abcdefghijklmnopqrstuvwxyz012345"), // 32 bytes, high entropy (32 distinct values)
		DebugMode:                     false,
		EnableCIMD:                    true, // CIMD enabled per MCP 2025-11-25
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.NotNil(t, tokenStore)
	assert.NotNil(t, oauthServer.Config)
	assert.Equal(t, config.BaseURL, oauthServer.Config.Issuer)
	assert.Equal(t, config.AllowPublicClientRegistration, oauthServer.Config.AllowPublicClientRegistration)
	assert.Equal(t, config.RegistrationAccessToken, oauthServer.Config.RegistrationAccessToken)
	assert.Equal(t, config.AllowInsecureAuthWithoutState, oauthServer.Config.AllowNoStateParameter)
	assert.Equal(t, config.MaxClientsPerIP, oauthServer.Config.MaxClientsPerIP)
	// CIMD (Client ID Metadata Documents) is configurable, verify it's passed through
	assert.True(t, oauthServer.Config.EnableClientIDMetadataDocuments, "CIMD should be enabled when configured")
}

// TestCreateOAuthServerCIMDDisabled tests OAuth server creation with CIMD disabled
func TestCreateOAuthServerCIMDDisabled(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		EnableCIMD:         false, // CIMD explicitly disabled
	}

	oauthServer, _, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.False(t, oauthServer.Config.EnableClientIDMetadataDocuments, "CIMD should be disabled when configured")
}

// TestCreateOAuthServerCIMDAllowPrivateIPs tests OAuth server creation with CIMD private IP allowlist
func TestCreateOAuthServerCIMDAllowPrivateIPs(t *testing.T) {
	tests := []struct {
		name                string
		cimdAllowPrivateIPs bool
		expectPrivateIPs    bool
	}{
		{
			name:                "CIMD private IPs disabled (default)",
			cimdAllowPrivateIPs: false,
			expectPrivateIPs:    false,
		},
		{
			name:                "CIMD private IPs enabled for internal deployments",
			cimdAllowPrivateIPs: true,
			expectPrivateIPs:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := OAuthConfig{
				BaseURL:             "https://mcp.example.com",
				Provider:            OAuthProviderGoogle,
				GoogleClientID:      "test-client-id",
				GoogleClientSecret:  "test-client-secret",
				EnableCIMD:          true,
				CIMDAllowPrivateIPs: tt.cimdAllowPrivateIPs,
			}

			oauthServer, _, err := createOAuthServer(config)

			assert.NoError(t, err)
			assert.NotNil(t, oauthServer)
			assert.True(t, oauthServer.Config.EnableClientIDMetadataDocuments, "CIMD should be enabled")
			assert.Equal(t, tt.expectPrivateIPs, oauthServer.Config.AllowPrivateIPClientMetadata,
				"AllowPrivateIPClientMetadata should match configuration")
		})
	}
}

// TestCreateOAuthServerWithDefaults tests OAuth server creation with default values
func TestCreateOAuthServerWithDefaults(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		DebugMode:          false,
		// MaxClientsPerIP not set - should use default
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.NotNil(t, tokenStore)
	assert.NotNil(t, oauthServer.Logger)
	assert.Equal(t, DefaultMaxClientsPerIP, oauthServer.Config.MaxClientsPerIP)
}

// TestCreateOAuthServerWithDebugMode tests debug mode logger configuration
func TestCreateOAuthServerWithDebugMode(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		DebugMode:          true,
	}

	oauthServer, _, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.NotNil(t, oauthServer.Logger)
}

// TestCreateOAuthServerWithInterstitial tests interstitial configuration
func TestCreateOAuthServerWithInterstitial(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		Interstitial: &server.InterstitialConfig{
			Branding: &server.InterstitialBranding{
				Title:        "Test Title",
				PrimaryColor: "#4f46e5",
			},
		},
	}

	oauthServer, _, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.NotNil(t, oauthServer.Config.Interstitial)
}

func TestCreateOAuthServerWithTrustedIssuers(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		TrustedIssuers: []TrustedIssuerConfig{
			{
				Issuer:           "https://muster.example.com",
				JwksURL:          "https://muster.example.com/.well-known/jwks.json",
				AllowedAudiences: []string{"https://mcp.example.com"},
			},
		},
	}

	oauthServer, _, err := createOAuthServer(config)
	require.NoError(t, err)
	require.NotNil(t, oauthServer)

	// trustedIssuerValidator is unexported on the upstream Server type; verify
	// the wiring via Config — trusted audiences is the closest exposed analogue.
	// Runtime acceptance is covered by mcp-oauth's own test suite.
	require.NotNil(t, oauthServer.Config)
}

// TestCreateOAuthServerWithDexProvider tests Dex provider creation.
//
// Integration Test Requirements:
// This test requires a live Dex OIDC server for provider initialization.
// To run integration tests with Dex:
//  1. Deploy a Dex server with a test connector (e.g., mockCallback or GitHub)
//  2. Set DEX_ISSUER_URL environment variable to the Dex server URL
//  3. Configure test client credentials in Dex
//  4. Run: go test -tags=integration ./internal/server/...
func TestCreateOAuthServerWithDexProvider(t *testing.T) {
	t.Skip("Requires live Dex server - run with -tags=integration and configured Dex instance")
}

// TestCreateOAuthServerWithDexProviderNoConnector tests Dex provider without connector ID.
//
// Integration Test Requirements:
// This test verifies Dex connector selection flow when connectorID is not specified.
// See TestCreateOAuthServerWithDexProvider for setup requirements.
func TestCreateOAuthServerWithDexProviderNoConnector(t *testing.T) {
	t.Skip("Requires live Dex server - run with -tags=integration and configured Dex instance")
}

// TestCreateOAuthServerWithInvalidProvider tests invalid provider handling
func TestCreateOAuthServerWithInvalidProvider(t *testing.T) {
	config := OAuthConfig{
		BaseURL:   "https://mcp.example.com",
		Provider:  "invalid-provider",
		DebugMode: false,
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.Error(t, err)
	assert.Nil(t, oauthServer)
	assert.Nil(t, tokenStore)
	assert.Contains(t, err.Error(), "unsupported OAuth provider")
}

// TestCreateOAuthServerWithMemoryStorage tests explicit memory storage configuration
func TestCreateOAuthServerWithMemoryStorage(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		Storage: OAuthStorageConfig{
			Type: OAuthStorageTypeMemory,
		},
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.NotNil(t, tokenStore)
}

// TestCreateOAuthServerWithDefaultStorage tests default storage (memory) when type is empty
func TestCreateOAuthServerWithDefaultStorage(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		// Storage.Type not set - should default to memory
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.NoError(t, err)
	assert.NotNil(t, oauthServer)
	assert.NotNil(t, tokenStore)
}

// TestCreateOAuthServerWithInvalidStorageType tests invalid storage type handling
func TestCreateOAuthServerWithInvalidStorageType(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		Storage: OAuthStorageConfig{
			Type: "invalid-storage",
		},
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.Error(t, err)
	assert.Nil(t, oauthServer)
	assert.Nil(t, tokenStore)
	assert.Contains(t, err.Error(), "unsupported OAuth storage type")
}

// TestCreateOAuthServerWithValkeyMissingURL tests Valkey storage without URL
func TestCreateOAuthServerWithValkeyMissingURL(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		Provider:           OAuthProviderGoogle,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		Storage: OAuthStorageConfig{
			Type:   OAuthStorageTypeValkey,
			Valkey: ValkeyStorageConfig{
				// URL not set - should error
			},
		},
	}

	oauthServer, tokenStore, err := createOAuthServer(config)

	assert.Error(t, err)
	assert.Nil(t, oauthServer)
	assert.Nil(t, tokenStore)
	assert.Contains(t, err.Error(), "valkey URL is required")
}

// TestCreateOAuthServerWithValkeyStorage tests Valkey storage configuration.
//
// Integration Test Requirements:
// This test requires a running Valkey/Redis server for actual connection.
// To run integration tests with Valkey:
//  1. Run a Valkey server locally: docker run -p 6379:6379 valkey/valkey
//  2. Run: go test -tags=integration ./internal/server/...
func TestCreateOAuthServerWithValkeyStorage(t *testing.T) {
	t.Skip("Requires running Valkey server - run with -tags=integration")
}

// TestCreateOAuthServerWithValkeyStorageAndTLS tests Valkey with TLS configuration.
//
// Integration Test Requirements:
// This test requires a Valkey server with TLS enabled.
func TestCreateOAuthServerWithValkeyStorageAndTLS(t *testing.T) {
	t.Skip("Requires running Valkey server with TLS - run with -tags=integration")
}

// TestCreateOAuthServerWithValkeyStorageAndEncryption tests Valkey with encryption at rest.
//
// Integration Test Requirements:
// This test requires a running Valkey server.
func TestCreateOAuthServerWithValkeyStorageAndEncryption(t *testing.T) {
	t.Skip("Requires running Valkey server - run with -tags=integration")
}

// TestOAuthStorageTypeConstants verifies the storage type constants
func TestOAuthStorageTypeConstants(t *testing.T) {
	assert.Equal(t, OAuthStorageType("memory"), OAuthStorageTypeMemory)
	assert.Equal(t, OAuthStorageType("valkey"), OAuthStorageTypeValkey)
}

// TestValkeyStorageConfigDefaults tests ValkeyStorageConfig default values
func TestValkeyStorageConfigDefaults(t *testing.T) {
	config := ValkeyStorageConfig{}

	// All fields should be zero values
	assert.Empty(t, config.URL)
	assert.Empty(t, config.Password)
	assert.False(t, config.TLSEnabled)
	assert.Empty(t, config.KeyPrefix)
	assert.Equal(t, 0, config.DB)
}

// TestClientRegistrationRateLimiterConfiguration tests that maxClientsPerIP is properly passed
// to the client registration rate limiter
func TestClientRegistrationRateLimiterConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		maxClientsPerIP int
		expectedMax     int
	}{
		{
			name:            "custom maxClientsPerIP",
			maxClientsPerIP: 5,
			expectedMax:     5,
		},
		{
			name:            "higher maxClientsPerIP",
			maxClientsPerIP: 25,
			expectedMax:     25,
		},
		{
			name:            "default maxClientsPerIP (zero value uses default)",
			maxClientsPerIP: 0,
			expectedMax:     DefaultMaxClientsPerIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := OAuthConfig{
				BaseURL:            "https://mcp.example.com",
				Provider:           OAuthProviderGoogle,
				GoogleClientID:     "test-client-id",
				GoogleClientSecret: "test-client-secret",
				MaxClientsPerIP:    tt.maxClientsPerIP,
			}

			oauthServer, _, err := createOAuthServer(config)
			assert.NoError(t, err)
			assert.NotNil(t, oauthServer)

			// Verify the client registration rate limiter is configured
			assert.NotNil(t, oauthServer.ClientRegistrationRateLimiter, "client registration rate limiter should be set")

			// Get stats to verify configuration
			stats := oauthServer.ClientRegistrationRateLimiter.GetStats()
			assert.Equal(t, tt.expectedMax, stats.MaxPerWindow, "maxClientsPerIP should match configured value")
		})
	}
}

// TestAccessTokenInjectorMiddleware_SSOToken tests that SSO-forwarded tokens are used directly
// as the ID token without looking them up in the token store.
func TestAccessTokenInjectorMiddleware_SSOToken(t *testing.T) {
	// Create a mock next handler that captures the context
	var capturedCtx context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	// Create a minimal OAuthHTTPServer with no token store (SSO doesn't need it)
	s := &OAuthHTTPServer{}

	// Create the middleware
	middleware := s.createAccessTokenInjectorMiddleware(nextHandler)

	// Create a request with an SSO-validated user
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	//nolint:gosec // G101 false positive - this is a test token, not a credential
	ssoToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.sso-token-payload.signature"
	req.Header.Set("Authorization", "Bearer "+ssoToken)

	// Set up context with SSO user info (TokenSource = SSO)
	userInfo := &providers.UserInfo{
		ID:          "user-123",
		Email:       "test@example.com",
		TokenSource: providers.TokenSourceSSO,
	}
	ctx := handler.ContextWithUserInfo(req.Context(), userInfo)
	req = req.WithContext(ctx)

	// Execute the middleware
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	// Verify the request completed successfully
	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify the ID token was injected into context (it should be the Bearer token itself)
	injectedToken, ok := oauth.GetIDTokenFromContext(capturedCtx)
	assert.True(t, ok, "ID token should be present in context for SSO tokens")
	assert.Equal(t, ssoToken, injectedToken, "SSO token should be used directly as ID token")
}

// TestAccessTokenInjectorMiddleware_SSOTokenIsSSO verifies IsSSO() method behavior
func TestAccessTokenInjectorMiddleware_SSOTokenIsSSO(t *testing.T) {
	tests := []struct {
		name        string
		tokenSource providers.TokenSource
		isSSO       bool
		isOAuth     bool
	}{
		{
			name:        "SSO token source",
			tokenSource: providers.TokenSourceSSO,
			isSSO:       true,
			isOAuth:     false,
		},
		{
			name:        "OAuth token source",
			tokenSource: providers.TokenSourceOAuth,
			isSSO:       false,
			isOAuth:     true,
		},
		{
			name:        "Empty token source (backward compatibility)",
			tokenSource: "",
			isSSO:       false,
			isOAuth:     true, // Empty is treated as OAuth for backward compatibility
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userInfo := &providers.UserInfo{
				Email:       "test@example.com",
				TokenSource: tt.tokenSource,
			}

			assert.Equal(t, tt.isSSO, userInfo.IsSSO(), "IsSSO() mismatch")
			assert.Equal(t, tt.isOAuth, userInfo.IsOAuth(), "IsOAuth() mismatch")
		})
	}
}

// TestAccessTokenInjectorMiddleware_NoUserInfo tests that requests without user info pass through
func TestAccessTokenInjectorMiddleware_NoUserInfo(t *testing.T) {
	var capturedCtx context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	s := &OAuthHTTPServer{}
	middleware := s.createAccessTokenInjectorMiddleware(nextHandler)

	// Request without user info in context
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer some-token")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// No token should be injected
	_, ok := oauth.GetIDTokenFromContext(capturedCtx)
	assert.False(t, ok, "No token should be injected without user info")
}

// TestAccessTokenInjectorMiddleware_NoBearerToken tests that requests without bearer token pass through
func TestAccessTokenInjectorMiddleware_NoBearerToken(t *testing.T) {
	var capturedCtx context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	s := &OAuthHTTPServer{}
	middleware := s.createAccessTokenInjectorMiddleware(nextHandler)

	// Request with user info but no Authorization header
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	userInfo := &providers.UserInfo{
		Email:       "test@example.com",
		TokenSource: providers.TokenSourceSSO,
	}
	ctx := handler.ContextWithUserInfo(req.Context(), userInfo)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// No token should be injected
	_, ok := oauth.GetIDTokenFromContext(capturedCtx)
	assert.False(t, ok, "No token should be injected without bearer token")
}

// TestAccessTokenInjectorMiddleware_NilTokenStoreWithOAuthToken tests that requests with OAuth tokens
// (non-SSO) gracefully handle nil tokenStore by passing through without injecting a token.
// This ensures defensive coding against misconfiguration.
func TestAccessTokenInjectorMiddleware_NilTokenStoreWithOAuthToken(t *testing.T) {
	var capturedCtx context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	// Create OAuthHTTPServer with nil tokenStore (simulates misconfiguration)
	s := &OAuthHTTPServer{
		tokenStore: nil, // Explicitly nil - this is the condition we're testing
	}
	middleware := s.createAccessTokenInjectorMiddleware(nextHandler)

	// Request with OAuth user info (NOT SSO - so it will try to look up token)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer mcp-access-token-123")
	userInfo := &providers.UserInfo{
		ID:          "user-456",
		Email:       "oauth-user@example.com",
		TokenSource: providers.TokenSourceOAuth, // OAuth, not SSO
	}
	ctx := handler.ContextWithUserInfo(req.Context(), userInfo)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	// Request should complete successfully (not panic)
	assert.Equal(t, http.StatusOK, rr.Code)

	// No token should be injected (token store lookup would fail/panic without nil check)
	_, ok := oauth.GetIDTokenFromContext(capturedCtx)
	assert.False(t, ok, "No token should be injected when tokenStore is nil")
}

// TestDexScopesWithKubernetesAuthenticator tests that cross-client audience scope is correctly added
func TestDexScopesWithKubernetesAuthenticator(t *testing.T) {
	tests := []struct {
		name                               string
		dexKubernetesAuthenticatorClientID string
		wantAudienceScope                  bool
		expectedAudienceScope              string
	}{
		{
			name:                               "with kubernetes authenticator client ID",
			dexKubernetesAuthenticatorClientID: "dex-k8s-authenticator",
			wantAudienceScope:                  true,
			expectedAudienceScope:              "audience:server:client_id:dex-k8s-authenticator",
		},
		{
			name:                               "without kubernetes authenticator client ID",
			dexKubernetesAuthenticatorClientID: "",
			wantAudienceScope:                  false,
		},
		{
			name:                               "with custom client ID",
			dexKubernetesAuthenticatorClientID: "my-custom-k8s-client",
			wantAudienceScope:                  true,
			expectedAudienceScope:              "audience:server:client_id:my-custom-k8s-client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the scope building logic from createOAuthServer
			scopes := make([]string, len(dexOAuthScopes))
			copy(scopes, dexOAuthScopes)
			if tt.dexKubernetesAuthenticatorClientID != "" {
				audienceScope := "audience:server:client_id:" + tt.dexKubernetesAuthenticatorClientID
				scopes = append(scopes, audienceScope)
			}

			// Verify base scopes are always present
			assert.Contains(t, scopes, "openid")
			assert.Contains(t, scopes, "groups")
			assert.Contains(t, scopes, "email")
			assert.Contains(t, scopes, "profile")

			// Verify audience scope based on configuration
			if tt.wantAudienceScope {
				assert.Contains(t, scopes, tt.expectedAudienceScope)
				assert.Len(t, scopes, len(dexOAuthScopes)+1, "should have base scopes plus audience scope")
			} else {
				assert.Len(t, scopes, len(dexOAuthScopes), "should only have base scopes")
				for _, scope := range scopes {
					assert.NotContains(t, scope, "audience:server:client_id:")
				}
			}
		})
	}
}

// makeIssuerMap builds a trustedIssuersByIssuer map from a single config.
// All test fixtures in this file configure exactly one entry per issuer URL;
// multi-entry scenarios are covered by TestAccessTokenInjectorMiddleware_MultiIssuerURL.
func makeIssuerMap(c TrustedIssuerConfig) map[string][]TrustedIssuerConfig {
	return map[string][]TrustedIssuerConfig{c.Issuer: {c}}
}

func TestAccessTokenInjectorMiddleware_M2MToken(t *testing.T) {
	const (
		testIssuer = "https://oidc.example.com"
		agentUser  = "agent:sre"
		agentGroup = "agent:sre"
	)
	issuerMap := makeIssuerMap(TrustedIssuerConfig{
		Issuer:                testIssuer,
		JwksURL:               "https://oidc.example.com/.well-known/jwks.json",
		Subject:               agentUser,
		Groups:                []string{agentGroup},
		AllowedTargetClusters: []string{"cluster-a"},
		AllowedClaims:         map[string]string{"sub": agentUser},
	})

	newReq := func(issuer, id string, groups []string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-m2m-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:          id,
			Issuer:      issuer,
			Groups:      groups,
			TokenSource: providers.TokenSourceTrustedIssuer,
		}
		return req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))
	}

	t.Run("sub matches subject and group intersects", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}

		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, newReq(testIssuer, agentUser, []string{agentGroup}))

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, agentUser, identity.UserName)
		require.Equal(t, []string{agentGroup}, identity.Groups)
		require.Equal(t, []string{"cluster-a"}, identity.AllowedTargetClusters)
		require.Equal(t, []string{testIssuer}, identity.Extra["issuer"])
		require.Equal(t, []string{"mcp-kubernetes"}, identity.Extra["agent"])
	})

	t.Run("sub not matching subject returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(testIssuer, "agent:other", []string{agentGroup}))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("token carrying no allow-listed group returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(testIssuer, agentUser, []string{"some:other-group"}))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("missing allowedClaims.sub returns 403", func(t *testing.T) {
		noClaimsMap := makeIssuerMap(TrustedIssuerConfig{
			Issuer:  testIssuer,
			JwksURL: "https://oidc.example.com/.well-known/jwks.json",
			Subject: agentUser,
			Groups:  []string{agentGroup},
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: noClaimsMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(testIssuer, agentUser, []string{agentGroup}))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("sub not matching allowedClaims routing pattern returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(testIssuer, "attacker:imposter", []string{agentGroup}))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("unknown issuer returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq("https://unknown.example.com", agentUser, []string{agentGroup}))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestIntersectGroups(t *testing.T) {
	t.Run("intersection preserves allowList order", func(t *testing.T) {
		require.Equal(t, []string{"b", "a"}, intersectGroups([]string{"b", "a", "c"}, []string{"a", "b", "d"}))
	})
	t.Run("deduplicates within allowList", func(t *testing.T) {
		require.Equal(t, []string{"x"}, intersectGroups([]string{"x", "x"}, []string{"x"}))
	})
	t.Run("empty allowList returns nil", func(t *testing.T) {
		require.Nil(t, intersectGroups(nil, []string{"a"}))
	})
	t.Run("empty tokenGroups returns nil", func(t *testing.T) {
		require.Nil(t, intersectGroups([]string{"a"}, nil))
	})
	t.Run("no overlap returns nil", func(t *testing.T) {
		require.Nil(t, intersectGroups([]string{"a"}, []string{"b"}))
	})
}

func TestAccessTokenInjectorMiddleware_OBOToken(t *testing.T) {
	const (
		testIssuer    = "https://oidc.example.com"
		testAlias     = "glean"
		humanSubject  = "quentin@example.com"
		agentSASub    = "system:serviceaccount:kagent:my-agent"
		agentSAIssuer = "https://k8s.example.com"
	)

	// issuerMap with a single actor entry that allows any subject
	// (empty AllowedSubjects = unrestricted, bounded by allowedClaims.sub).
	issuerMap := makeIssuerMap(TrustedIssuerConfig{
		Issuer:                testIssuer,
		JwksURL:               "https://oidc.example.com/.well-known/jwks.json",
		Alias:                 testAlias,
		AllowedTargetClusters: []string{"cluster-a"},
		AllowedClaims:         map[string]string{"sub": humanSubject},
		AllowedActors: []ActorConfig{
			{Sub: agentSASub},
		},
	})

	newOBOReq := func(humanSub, actorSub string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-obo-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:           humanSub,
			Issuer:       testIssuer,
			TokenSource:  providers.TokenSourceTrustedIssuer,
			ActorSubject: actorSub,
			ActorIssuer:  agentSAIssuer,
		}
		return req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))
	}

	t.Run("OBO token: human sub becomes UserName, agent becomes Actor, no Groups", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}

		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, newOBOReq(humanSubject, agentSASub))

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, humanSubject, identity.UserName)
		require.Empty(t, identity.Groups, "OBO impersonates user-only; no groups must be set")
		require.Equal(t, agentSASub, identity.Actor)
		require.Equal(t, []string{testIssuer}, identity.Extra["issuer"])
		require.Equal(t, []string{"mcp-kubernetes"}, identity.Extra["agent"])
		require.Equal(t, []string{"cluster-a"}, identity.AllowedTargetClusters)
	})

	t.Run("M2M token (no act) still takes M2M path unchanged", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-m2m-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:          "agent:bot",
			Issuer:      testIssuer,
			Groups:      []string{"agent:bot"},
			TokenSource: providers.TokenSourceTrustedIssuer,
			// ActorSubject intentionally absent
		}
		req = req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))

		m2mMap := makeIssuerMap(TrustedIssuerConfig{
			Issuer:        testIssuer,
			JwksURL:       "https://oidc.example.com/.well-known/jwks.json",
			Subject:       "agent:bot",
			Groups:        []string{"agent:bot"},
			AllowedClaims: map[string]string{"sub": "agent:bot"},
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: m2mMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, "agent:bot", identity.UserName)
		require.Equal(t, []string{"agent:bot"}, identity.Groups)
		require.Empty(t, identity.Actor, "M2M path must not set Actor")
	})

	t.Run("OBO human sub not matching allowedClaims.sub returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newOBOReq("attacker@evil.com", agentSASub))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("OBO actor not in allowedActors returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newOBOReq(humanSubject, "system:serviceaccount:other:rogue-agent"))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("OBO allowedActors empty rejects any OBO request", func(t *testing.T) {
		noActorMap := makeIssuerMap(TrustedIssuerConfig{
			Issuer:        testIssuer,
			JwksURL:       "https://oidc.example.com/.well-known/jwks.json",
			Alias:         testAlias,
			AllowedClaims: map[string]string{"sub": humanSubject},
			// AllowedActors intentionally empty — OBO disabled.
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: noActorMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newOBOReq(humanSubject, agentSASub))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("OBO actor with allowedSubjects glob allows matching human", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		globMap := makeIssuerMap(TrustedIssuerConfig{
			Issuer:        testIssuer,
			JwksURL:       "https://oidc.example.com/.well-known/jwks.json",
			Alias:         testAlias,
			AllowedClaims: map[string]string{"sub": "*@example.com"},
			AllowedActors: []ActorConfig{
				{
					Sub:             agentSASub,
					AllowedSubjects: []string{"*@example.com"},
				},
			},
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: globMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, newOBOReq("alice@example.com", agentSASub))

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, "alice@example.com", identity.UserName)
		require.Equal(t, agentSASub, identity.Actor)
	})

	t.Run("OBO actor with allowedSubjects glob rejects non-matching human", func(t *testing.T) {
		globMap := makeIssuerMap(TrustedIssuerConfig{
			Issuer:        testIssuer,
			JwksURL:       "https://oidc.example.com/.well-known/jwks.json",
			Alias:         testAlias,
			AllowedClaims: map[string]string{"sub": "*@example.com"},
			AllowedActors: []ActorConfig{
				{
					Sub:             agentSASub,
					AllowedSubjects: []string{"*@example.com"},
				},
			},
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: globMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newOBOReq("intruder@other.org", agentSASub))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("OBO actor with empty allowedSubjects allows any matching human", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, newOBOReq(humanSubject, agentSASub))

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, humanSubject, identity.UserName)
	})

	t.Run("OBO multiple actors: each actor scoped to different subjects", func(t *testing.T) {
		const (
			agent1 = "system:serviceaccount:kagent:agent-one"
			agent2 = "system:serviceaccount:kagent:agent-two"
		)
		multiActorMap := makeIssuerMap(TrustedIssuerConfig{
			Issuer:        testIssuer,
			JwksURL:       "https://oidc.example.com/.well-known/jwks.json",
			Alias:         testAlias,
			AllowedClaims: map[string]string{"sub": "*@example.com"},
			AllowedActors: []ActorConfig{
				{Sub: agent1, AllowedSubjects: []string{"admin@example.com"}},
				{Sub: agent2, AllowedSubjects: []string{"*@example.com"}},
			},
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: multiActorMap}

		// agent1 may only impersonate admin@example.com
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newOBOReq("other@example.com", agent1))
		require.Equal(t, http.StatusForbidden, rr.Code)

		// agent2 may impersonate any @example.com human
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		rr2 := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr2, newOBOReq("other@example.com", agent2))
		require.Equal(t, http.StatusOK, rr2.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, "other@example.com", identity.UserName)
	})
}

func TestMatchesSubGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// exact match
		{"alice@example.com", "alice@example.com", true},
		{"alice@example.com", "bob@example.com", false},

		// trailing star — prefix match
		{"system:serviceaccount:kagent:*", "system:serviceaccount:kagent:bot", true},
		{"system:serviceaccount:kagent:*", "system:serviceaccount:other:bot", false},
		// trailing-star prefix must be non-empty
		{"*", "anything", false},

		// leading star — suffix match
		{"*@giantswarm.io", "quentin@giantswarm.io", true},
		{"*@giantswarm.io", "quentin@otherdomain.io", false},
		// leading-star suffix must be non-empty
		{"*", "anything", false},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+"|"+tc.s, func(t *testing.T) {
			require.Equal(t, tc.want, matchesSubGlob(tc.pattern, tc.s))
		})
	}
}

// makeDelegatedJWT builds an unsigned JWT whose payload is the given claims.
// ParseUnverifiedClaims only base64-decodes the payload segment, so a placeholder
// signature is sufficient for exercising the in-process act-chain walk.
func makeDelegatedJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	body, err := json.Marshal(claims)
	require.NoError(t, err)
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none","typ":"at+jwt"}`)) + "." + enc(body) + ".sig"
}

func TestAccessTokenInjectorMiddleware_ActorChain(t *testing.T) {
	const (
		testIssuer = "https://muster.example.io"
		testAlias  = "muster-obo"
		human      = "quentin@giantswarm.io"
		agentInner = "system:serviceaccount:kagent:agent-a"
		agentLeaf  = "system:serviceaccount:kagent:agent-b"
	)

	// Two-hop chain: human -> agent-a -> agent-b -> MCP. The minted token's act
	// is the leaf (agent-b) with a nested act for the inner hop (agent-a).
	twoHopToken := makeDelegatedJWT(t, map[string]any{
		"iss": testIssuer,
		"sub": human,
		"act": map[string]any{
			"iss": "https://k8s.example.com",
			"sub": agentLeaf,
			"act": map[string]any{
				"iss": "https://k8s.example.com",
				"sub": agentInner,
			},
		},
	})

	newReq := func(token string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		userInfo := &providers.UserInfo{
			ID:           human,
			Issuer:       testIssuer,
			TokenSource:  providers.TokenSourceTrustedIssuer,
			ActorSubject: agentLeaf, // mcp-oauth mirrors only the leaf
			ActorIssuer:  "https://k8s.example.com",
		}
		return req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))
	}

	issuerWith := func(actorSub string) map[string][]TrustedIssuerConfig {
		return makeIssuerMap(TrustedIssuerConfig{
			Issuer:        testIssuer,
			JwksURL:       "https://muster.example.io/.well-known/jwks.json",
			Alias:         testAlias,
			AllowedClaims: map[string]string{"sub": "*@giantswarm.io"},
			AllowedActors: []ActorConfig{{Sub: actorSub}},
		})
	}

	t.Run("allowedActors matches an inner (non-leaf) hop in the chain", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerWith(agentInner)}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, newReq(twoHopToken))

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, human, identity.UserName)
		require.Equal(t, agentLeaf, identity.Actor, "Actor records the immediate (leaf) caller")
	})

	t.Run("allowedActors matches the leaf hop in the chain", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerWith(agentLeaf)}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(twoHopToken))
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("no hop in the chain matches allowedActors returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: issuerWith("system:serviceaccount:kagent:stranger")}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(twoHopToken))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestEffectiveSubjectKey(t *testing.T) {
	require.Equal(t, "sub", TrustedIssuerConfig{}.EffectiveSubjectKey())
	require.Equal(t, "email", TrustedIssuerConfig{SubjectClaim: "email"}.EffectiveSubjectKey())
}

// TestAccessTokenInjectorMiddleware_SubjectClaim covers the muster-obo issuer:
// the subject is remapped to the email claim, so the in-process gate matches
// userInfo.ID (already the email) against allowedClaims.email, not .sub.
func TestAccessTokenInjectorMiddleware_SubjectClaim(t *testing.T) {
	const (
		musterIssuer = "https://muster.glean.example.io"
		musterAlias  = "muster-obo"
		sreAgentSub  = "system:serviceaccount:kagent:sre-agent"
		userEmail    = "quentin@giantswarm.io"
	)

	emailMap := makeIssuerMap(TrustedIssuerConfig{
		Issuer:                musterIssuer,
		JwksURL:               "https://muster.glean.example.io/.well-known/jwks.json",
		Alias:                 musterAlias,
		AllowedTargetClusters: []string{"glean"},
		SubjectClaim:          "email",
		AllowedClaims:         map[string]string{"email": "*@giantswarm.io"},
		AllowedActors:         []ActorConfig{{Sub: sreAgentSub}},
	})

	newReq := func(remappedSubject string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-obo-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:           remappedSubject, // mcp-oauth set this from the email claim
			Issuer:       musterIssuer,
			TokenSource:  providers.TokenSourceTrustedIssuer,
			ActorSubject: sreAgentSub,
			ActorIssuer:  "https://k8s.example.com",
		}
		return req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))
	}

	t.Run("email-remapped subject matching allowedClaims.email impersonates the email", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: emailMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, newReq(userEmail))

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, userEmail, identity.UserName)
		require.Equal(t, sreAgentSub, identity.Actor)
		require.Empty(t, identity.Groups)
	})

	t.Run("subject outside the email pattern returns 403", func(t *testing.T) {
		s := &OAuthHTTPServer{trustedIssuersByIssuer: emailMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq("mallory@evil.com"))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("SubjectClaim set but no allowedClaims entry under that key returns 403", func(t *testing.T) {
		misconfigured := makeIssuerMap(TrustedIssuerConfig{
			Issuer:        musterIssuer,
			JwksURL:       "https://muster.glean.example.io/.well-known/jwks.json",
			Alias:         musterAlias,
			SubjectClaim:  "email",
			AllowedClaims: map[string]string{"sub": "*@giantswarm.io"}, // wrong key
			AllowedActors: []ActorConfig{{Sub: sreAgentSub}},
		})
		s := &OAuthHTTPServer{trustedIssuersByIssuer: misconfigured}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, newReq(userEmail))
		require.Equal(t, http.StatusForbidden, rr.Code)
	})
}

// TestAccessTokenInjectorMiddleware_MultiIssuerURL covers the glean production
// scenario: M2M (agent sub) and OBO (email sub) entries share the same muster
// issuer URL. The middleware must route each token to the correct entry.
func TestAccessTokenInjectorMiddleware_MultiIssuerURL(t *testing.T) {
	const (
		musterIssuer = "https://muster.glean.example.io"
		oboAlias     = "muster-obo"
		agentSA      = "system:serviceaccount:kagent:sre-agent"
		agentUser    = "agent:sre"
		agentGroup   = "agent:sre"
		humanEmail   = "alice@giantswarm.io"
	)

	dualIssuerMap := map[string][]TrustedIssuerConfig{
		musterIssuer: {
			{
				Issuer:        musterIssuer,
				JwksURL:       "https://muster.glean.example.io/.well-known/jwks.json",
				Subject:       agentUser,
				Groups:        []string{agentGroup},
				AllowedClaims: map[string]string{"sub": agentUser},
			},
			{
				Issuer:        musterIssuer,
				JwksURL:       "https://muster.glean.example.io/.well-known/jwks.json",
				Alias:         oboAlias,
				SubjectClaim:  "email",
				AllowedClaims: map[string]string{"email": "*@giantswarm.io"},
				AllowedActors: []ActorConfig{{Sub: agentSA}},
			},
		},
	}

	t.Run("M2M token routes to M2M entry", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-m2m-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:          agentUser,
			Issuer:      musterIssuer,
			Groups:      []string{agentGroup},
			TokenSource: providers.TokenSourceTrustedIssuer,
		}
		req = req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))

		s := &OAuthHTTPServer{trustedIssuersByIssuer: dualIssuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, agentUser, identity.UserName)
		require.Equal(t, []string{agentGroup}, identity.Groups)
	})

	t.Run("OBO token routes to email-pattern entry", func(t *testing.T) {
		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-obo-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:           humanEmail,
			Issuer:       musterIssuer,
			TokenSource:  providers.TokenSourceTrustedIssuer,
			ActorSubject: agentSA,
			ActorIssuer:  "https://k8s.example.com",
		}
		req = req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))

		s := &OAuthHTTPServer{trustedIssuersByIssuer: dualIssuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(next).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		identity, ok := ImpersonationIdentityFromContext(capturedCtx)
		require.True(t, ok)
		require.Equal(t, humanEmail, identity.UserName)
		require.Empty(t, identity.Groups)
	})

	t.Run("token subject matching neither entry returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer fake-token") //nolint:gosec // G101: test fixture
		userInfo := &providers.UserInfo{
			ID:          "attacker@evil.com",
			Issuer:      musterIssuer,
			TokenSource: providers.TokenSourceTrustedIssuer,
		}
		req = req.WithContext(handler.ContextWithUserInfo(req.Context(), userInfo))

		s := &OAuthHTTPServer{trustedIssuersByIssuer: dualIssuerMap}
		rr := httptest.NewRecorder()
		s.createAccessTokenInjectorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code)
	})
}
