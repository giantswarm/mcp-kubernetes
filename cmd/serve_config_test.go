package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIssuerURL(t *testing.T) {
	accepted := []string{
		"https://dex.example.com",
		"https://dex.example.com/path",
		"https://localhost:32000/dex",
		"https://LOCALHOST",
		"https://127.0.0.1:32000/dex",
		"https://10.0.0.1/dex",
		"https://253.1.2.3",
		"https://dex.does-not-resolve.invalid",
	}
	for _, issuer := range accepted {
		t.Run(issuer, func(t *testing.T) {
			require.NoError(t, validateIssuerURL(issuer, "Dex issuer URL"))
		})
	}

	refused := []struct {
		url    string
		errMsg string
	}{
		{"http://dex.example.com", "must use HTTPS"},
		{"not-a-url", "must use HTTPS"},
		{"dex.example.com", "must use HTTPS"},
		{"", "must use HTTPS"},
		{"https://", "must have a hostname"},
		{"https://dex.example.com/dex?tenant=one", "query component"},
		{"https://dex.example.com/dex#tenant", "fragment component"},
	}
	for _, tt := range refused {
		t.Run("refuses "+tt.url, func(t *testing.T) {
			err := validateIssuerURL(tt.url, "Dex issuer URL")
			require.ErrorContains(t, err, tt.errMsg)
		})
	}
}

// TestValidateOAuthClientID tests OAuth client ID validation
func TestValidateOAuthClientID(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
		wantErr  bool
		errMsg   string
	}{
		// Valid client IDs
		{
			name:     "empty client ID (optional)",
			clientID: "",
			wantErr:  false,
		},
		{
			name:     "simple alphanumeric",
			clientID: "dexk8sauthenticator",
			wantErr:  false,
		},
		{
			name:     "with hyphens",
			clientID: "dex-k8s-authenticator",
			wantErr:  false,
		},
		{
			name:     "with underscores",
			clientID: "dex_k8s_authenticator",
			wantErr:  false,
		},
		{
			name:     "with periods",
			clientID: "dex.k8s.authenticator",
			wantErr:  false,
		},
		{
			name:     "mixed valid characters",
			clientID: "my-app_v1.0",
			wantErr:  false,
		},
		{
			name:     "starts with number",
			clientID: "123-client",
			wantErr:  false,
		},
		{
			name:     "uppercase letters",
			clientID: "DEX-K8S-AUTHENTICATOR",
			wantErr:  false,
		},
		// Invalid client IDs
		{
			name:     "starts with hyphen",
			clientID: "-dex-k8s-authenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "starts with underscore",
			clientID: "_dex-k8s-authenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "starts with period",
			clientID: ".dex-k8s-authenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "contains space",
			clientID: "dex k8s authenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "contains special characters",
			clientID: "dex@k8s#authenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "contains newline (injection attempt)",
			clientID: "dex-k8s\nauthenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "contains colon (injection attempt)",
			clientID: "dex:k8s:authenticator",
			wantErr:  true,
			errMsg:   "contains invalid characters",
		},
		{
			name:     "too long (>256 chars)",
			clientID: string(make([]byte, 257)),
			wantErr:  true,
			errMsg:   "is too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthClientID(tt.clientID, "test client ID")
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateOAuthClientIDSecurityCases tests security-sensitive scenarios
func TestValidateOAuthClientIDSecurityCases(t *testing.T) {
	// These are injection attempts that should be rejected
	injectionAttempts := []string{
		"client openid",                   // Space injection
		"client\topenid",                  // Tab injection
		"client\nopenid",                  // Newline injection
		"client%20openid",                 // URL-encoded space (literal %)
		"client+openid",                   // Plus sign
		"audience:server:client_id:other", // Scope injection
		"client&other=scope",              // Parameter injection
		"client;other",                    // Semicolon injection
		"client|other",                    // Pipe injection
		"client`whoami`",                  // Command injection
		"client$(whoami)",                 // Command substitution
		"client${PATH}",                   // Variable expansion
		"<script>alert(1)</script>",       // XSS attempt
		"client' OR '1'='1",               // SQL injection pattern
	}

	for _, attempt := range injectionAttempts {
		t.Run(attempt, func(t *testing.T) {
			err := validateOAuthClientID(attempt, "test client ID")
			assert.Error(t, err, "injection attempt should be rejected: %s", attempt)
		})
	}
}

// TestValidateOAuthBaseURL tests OAuth base URL validation with localhost support for development
func TestValidateOAuthBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		// Valid URLs
		{
			name:    "HTTPS production URL",
			url:     "https://mcp.example.com",
			wantErr: false,
		},
		{
			name:    "HTTPS production URL with path",
			url:     "https://mcp.example.com/oauth",
			wantErr: false,
		},
		{
			name:    "HTTPS localhost (TLS development)",
			url:     "https://localhost:8080",
			wantErr: false,
		},
		{
			name:    "HTTP localhost (development)",
			url:     "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "HTTP 127.0.0.1 (development)",
			url:     "http://127.0.0.1:8080",
			wantErr: false,
		},
		{
			name:    "HTTP IPv6 loopback (development)",
			url:     "http://[::1]:8080",
			wantErr: false,
		},
		// Invalid URLs
		{
			name:    "HTTP non-localhost (production without HTTPS)",
			url:     "http://mcp.example.com",
			wantErr: true,
			errMsg:  "must use HTTPS for non-localhost",
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "invalid scheme",
			url:     "ftp://mcp.example.com",
			wantErr: true,
			errMsg:  "must use http or https scheme",
		},
		{
			name:    "no scheme",
			url:     "mcp.example.com",
			wantErr: true,
			errMsg:  "must use http or https scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthBaseURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
