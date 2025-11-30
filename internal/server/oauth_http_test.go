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
