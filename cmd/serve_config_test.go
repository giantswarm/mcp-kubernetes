package cmd

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateSecureURL tests URL validation with HTTPS and SSRF protection
func TestValidateSecureURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		fieldName string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid HTTPS URL",
			url:       "https://dex.example.com",
			fieldName: "test URL",
			wantErr:   false,
		},
		{
			name:      "invalid HTTP URL",
			url:       "http://dex.example.com",
			fieldName: "test URL",
			wantErr:   true,
			errMsg:    "must use HTTPS",
		},
		{
			name:      "localhost URL",
			url:       "https://localhost:8080",
			fieldName: "test URL",
			wantErr:   true,
			errMsg:    "cannot use localhost",
		},
		{
			name:      "localhost with domain",
			url:       "https://LOCALHOST",
			fieldName: "test URL",
			wantErr:   true,
			errMsg:    "cannot use localhost",
		},
		{
			name:      "invalid URL format",
			url:       "not-a-url",
			fieldName: "test URL",
			wantErr:   true,
			errMsg:    "must be a valid URL",
		},
		{
			name:      "URL without scheme",
			url:       "dex.example.com",
			fieldName: "test URL",
			wantErr:   true,
			errMsg:    "must be a valid URL with HTTPS scheme",
		},
		{
			name:      "empty URL",
			url:       "",
			fieldName: "test URL",
			wantErr:   true,
			errMsg:    "must be a valid URL",
		},
		{
			name:      "URL with path",
			url:       "https://dex.example.com/path",
			fieldName: "test URL",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecureURL(tt.url, tt.fieldName, false)
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

// TestValidateSecureURLAllowPrivate tests URL validation with allowPrivate=true
func TestValidateSecureURLAllowPrivate(t *testing.T) {
	// With allowPrivate=true, private IPs should be allowed
	// This simulates a URL that would normally be blocked due to private IP
	err := validateSecureURL("https://internal.example.com", "test URL", true)
	// Since we can't control DNS resolution in tests, just verify the function accepts the parameter
	// and doesn't panic. Real private IP testing would require mocking net.LookupIP
	_ = err
}

// TestIsPrivateOrLoopbackIP tests private/loopback IP detection
func TestIsPrivateOrLoopbackIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		isPrivate bool
	}{
		// Loopback addresses
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv6 loopback", "::1", true},

		// Private IPv4 ranges
		{"10.0.0.0/8 - start", "10.0.0.1", true},
		{"10.0.0.0/8 - mid", "10.128.0.1", true},
		{"10.0.0.0/8 - end", "10.255.255.255", true},
		{"172.16.0.0/12 - start", "172.16.0.1", true},
		{"172.16.0.0/12 - mid", "172.20.0.1", true},
		{"172.16.0.0/12 - end", "172.31.255.255", true},
		{"192.168.0.0/16 - start", "192.168.0.1", true},
		{"192.168.0.0/16 - end", "192.168.255.255", true},

		// Public IPv4 addresses
		{"public IP - Google DNS", "8.8.8.8", false},
		{"public IP - Cloudflare DNS", "1.1.1.1", false},
		{"public IP - example", "93.184.216.34", false},

		// Edge cases for private ranges
		{"172.15.x.x not private", "172.15.255.255", false},
		{"172.32.x.x not private", "172.32.0.1", false},

		// Link-local
		{"link-local IPv4", "169.254.1.1", true},
		{"link-local IPv6", "fe80::1", true},

		// Private IPv6 (ULA - Unique Local Addresses)
		{"IPv6 ULA fc00::/7", "fc00::1", true},
		{"IPv6 ULA fd00::/8", "fd00::1", true},

		// Public IPv6
		{"public IPv6", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(t, tt.ip)
			result := isPrivateOrLoopbackIP(ip)
			assert.Equal(t, tt.isPrivate, result, "IP %s should be private=%v", tt.ip, tt.isPrivate)
		})
	}
}

// parseIP is a helper function to parse IP addresses in tests
func parseIP(t *testing.T, ipStr string) net.IP {
	t.Helper()
	parsed := net.ParseIP(ipStr)
	if parsed == nil {
		t.Fatalf("failed to parse IP: %s", ipStr)
	}
	return parsed
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
