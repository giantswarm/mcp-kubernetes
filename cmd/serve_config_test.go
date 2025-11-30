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
			err := validateSecureURL(tt.url, tt.fieldName)
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
