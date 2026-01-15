// Package server provides tests for OAuth HTTP server functionality.
// These tests verify HTTPS validation and server lifecycle management.
package server

import (
	"testing"

	"github.com/giantswarm/mcp-oauth/server"
	"github.com/stretchr/testify/assert"
)

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
		EncryptionKey:                 []byte("12345678901234567890123456789012"), // Exactly 32 bytes
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
