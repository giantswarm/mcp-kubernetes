// Package server provides tests for OAuth HTTP server functionality.
// These tests verify HTTPS validation, CORS origin parsing, security headers,
// and server lifecycle management.
package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestValidateAllowedOrigins tests CORS origin validation
func TestValidateAllowedOrigins(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      []string
		wantError bool
	}{
		{
			name:      "empty string",
			input:     "",
			want:      nil,
			wantError: false,
		},
		{
			name:      "single valid origin",
			input:     "https://example.com",
			want:      []string{"https://example.com"},
			wantError: false,
		},
		{
			name:      "multiple valid origins",
			input:     "https://example.com,https://another.com",
			want:      []string{"https://example.com", "https://another.com"},
			wantError: false,
		},
		{
			name:      "origins with trailing slash normalized",
			input:     "https://example.com/",
			want:      []string{"https://example.com"},
			wantError: false,
		},
		{
			name:      "whitespace trimmed",
			input:     " https://example.com , https://another.com ",
			want:      []string{"https://example.com", "https://another.com"},
			wantError: false,
		},
		{
			name:      "invalid URL format",
			input:     "not-a-url",
			want:      nil,
			wantError: true,
		},
		{
			name:      "missing scheme",
			input:     "example.com",
			want:      nil,
			wantError: true,
		},
		{
			name:      "invalid scheme",
			input:     "ftp://example.com",
			want:      nil,
			wantError: true,
		},
		{
			name:      "URL with path not allowed",
			input:     "https://example.com/path",
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAllowedOrigins(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestSecurityHeadersMiddleware tests that security headers are properly set
func TestSecurityHeadersMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		hstsEnabled bool
		hasTLS      bool
		wantHSTS    bool
	}{
		{
			name:        "HSTS enabled with TLS",
			hstsEnabled: true,
			hasTLS:      true,
			wantHSTS:    true,
		},
		{
			name:        "HSTS enabled without TLS",
			hstsEnabled: true,
			hasTLS:      false,
			wantHSTS:    true,
		},
		{
			name:        "HSTS disabled with TLS",
			hstsEnabled: false,
			hasTLS:      true,
			wantHSTS:    true,
		},
		{
			name:        "HSTS disabled without TLS",
			hstsEnabled: false,
			hasTLS:      false,
			wantHSTS:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := securityHeadersMiddleware(tt.hstsEnabled)(handler)

			req := httptest.NewRequest("GET", "/", nil)
			if tt.hasTLS {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			// Check common security headers
			assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
			assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
			assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "default-src 'self'")
			assert.Contains(t, rec.Header().Get("Permissions-Policy"), "geolocation=()")
			assert.Equal(t, "same-origin", rec.Header().Get("Cross-Origin-Opener-Policy"))
			assert.Equal(t, "require-corp", rec.Header().Get("Cross-Origin-Embedder-Policy"))
			assert.Equal(t, "same-origin", rec.Header().Get("Cross-Origin-Resource-Policy"))

			// Check HSTS header
			if tt.wantHSTS {
				assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=31536000")
			} else {
				assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
			}
		})
	}
}

// TestCORSMiddleware tests CORS header handling
func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		method         string
		wantCORS       bool
		wantStatus     int
	}{
		{
			name:           "allowed origin",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://example.com",
			method:         "GET",
			wantCORS:       true,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "disallowed origin",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://evil.com",
			method:         "GET",
			wantCORS:       false,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "no origin header",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "",
			method:         "GET",
			wantCORS:       false,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "empty allowed origins list",
			allowedOrigins: []string{},
			requestOrigin:  "https://example.com",
			method:         "GET",
			wantCORS:       false,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "OPTIONS preflight request",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://example.com",
			method:         "OPTIONS",
			wantCORS:       true,
			wantStatus:     http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := corsMiddleware(tt.allowedOrigins)(handler)

			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			// Check CORS headers
			assert.Equal(t, "GET, POST, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
			assert.Equal(t, "Authorization, Content-Type", rec.Header().Get("Access-Control-Allow-Headers"))
			assert.Equal(t, "3600", rec.Header().Get("Access-Control-Max-Age"))

			if tt.wantCORS {
				assert.Equal(t, tt.requestOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "Origin", rec.Header().Get("Vary"))
			} else {
				assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestBuildOAuthConfig tests OAuth configuration building
func TestBuildOAuthConfig(t *testing.T) {
	config := OAuthConfig{
		BaseURL:                       "https://mcp.example.com",
		GoogleClientID:                "test-client-id",
		GoogleClientSecret:            "test-client-secret",
		AllowPublicClientRegistration: false,
		RegistrationAccessToken:       "test-token",
		AllowInsecureAuthWithoutState: false,
		MaxClientsPerIP:               5,
		EncryptionKey:                 []byte("test-encryption-key-32-bytes!"),
		DebugMode:                     false,
	}

	oauthConfig := buildOAuthConfig(config)

	assert.NotNil(t, oauthConfig)
	assert.Equal(t, config.BaseURL, oauthConfig.BaseURL)
	assert.Equal(t, config.GoogleClientID, oauthConfig.GoogleClientID)
	assert.Equal(t, config.GoogleClientSecret, oauthConfig.GoogleClientSecret)
	assert.Equal(t, config.AllowPublicClientRegistration, oauthConfig.Security.AllowPublicClientRegistration)
	assert.Equal(t, config.RegistrationAccessToken, oauthConfig.Security.RegistrationAccessToken)
	assert.Equal(t, config.AllowInsecureAuthWithoutState, oauthConfig.Security.AllowInsecureAuthWithoutState)
	assert.Equal(t, config.MaxClientsPerIP, oauthConfig.Security.MaxClientsPerIP)
	assert.Equal(t, config.EncryptionKey, oauthConfig.Security.EncryptionKey)
	assert.True(t, oauthConfig.Security.EnableAuditLogging)
	assert.Equal(t, DefaultIPRateLimit, oauthConfig.RateLimit.Rate)
	assert.Equal(t, DefaultIPBurst, oauthConfig.RateLimit.Burst)
	assert.Equal(t, DefaultUserRateLimit, oauthConfig.RateLimit.UserRate)
	assert.Equal(t, DefaultUserBurst, oauthConfig.RateLimit.UserBurst)
}

// TestBuildOAuthConfigWithDebugMode tests debug mode logger configuration
func TestBuildOAuthConfigWithDebugMode(t *testing.T) {
	config := OAuthConfig{
		BaseURL:            "https://mcp.example.com",
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		DebugMode:          true,
	}

	oauthConfig := buildOAuthConfig(config)

	assert.NotNil(t, oauthConfig)
	assert.NotNil(t, oauthConfig.Logger)
}
