package logging

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnonymizeEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		wantLen  int
		wantSame bool // true if same input should produce same output
	}{
		{
			name:    "empty email",
			email:   "",
			wantLen: 0,
		},
		{
			name:    "valid email",
			email:   "test@example.com",
			wantLen: 21, // "user:" (5) + 16 hex chars (8 bytes * 2)
		},
		{
			name:    "different email produces different hash",
			email:   "other@example.com",
			wantLen: 21,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnonymizeEmail(tt.email)

			if tt.email == "" {
				assert.Empty(t, result)
				return
			}

			assert.Len(t, result, tt.wantLen)
			assert.Contains(t, result, "user:")

			// Same input should produce same output
			result2 := AnonymizeEmail(tt.email)
			assert.Equal(t, result, result2)
		})
	}

	// Different emails produce different hashes
	hash1 := AnonymizeEmail("test@example.com")
	hash2 := AnonymizeEmail("other@example.com")
	assert.NotEqual(t, hash1, hash2)
}

func TestSanitizeHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "empty host",
			host:     "",
			expected: "<empty>",
		},
		{
			name:     "hostname without IP",
			host:     "https://api.cluster.example.com:6443",
			expected: "https://api.cluster.example.com:6443",
		},
		{
			name:     "IP address URL",
			host:     "https://192.168.1.100:6443",
			expected: "https://<redacted-ip>:6443",
		},
		{
			name:     "bare IP address",
			host:     "192.168.1.100",
			expected: "<redacted-ip>",
		},
		{
			name:     "IP with port no scheme",
			host:     "10.0.0.1:6443",
			expected: "<redacted-ip>:6443",
		},
		// IPv6 tests
		{
			name:     "IPv6 address URL with brackets",
			host:     "https://[2001:db8::1]:6443",
			expected: "https://<redacted-ip>:6443",
		},
		{
			name:     "bare IPv6 address",
			host:     "2001:db8::1",
			expected: "<redacted-ip>",
		},
		{
			name:     "IPv6 with brackets no scheme",
			host:     "[2001:db8:85a3::8a2e:370:7334]:6443",
			expected: "<redacted-ip>:6443",
		},
		{
			name:     "full IPv6 address",
			host:     "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expected: "<redacted-ip>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHost(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "empty token",
			token:    "",
			expected: "<empty>",
		},
		{
			name:     "short token",
			token:    "abc",
			expected: "[token:3 chars]",
		},
		{
			name:     "exactly 4 chars",
			token:    "abcd",
			expected: "[token:4 chars]",
		},
		{
			name:     "normal token",
			token:    "eyJhbGciOiJSUzI1NiIsImtpZCI6...",
			expected: "[token:31 chars]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeToken(tt.token)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Verify no token content is leaked
	t.Run("no token prefix leaked", func(t *testing.T) {
		token := "eyJhbGciOiJSUzI1NiIsImtpZCI6..." //nolint:gosec // Test token, not a real credential
		result := SanitizeToken(token)
		assert.NotContains(t, result, "eyJ", "token prefix should not be leaked")
		assert.NotContains(t, result, token[:4], "any token content should not be leaked")
	})
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "empty email",
			email:    "",
			expected: "",
		},
		{
			name:     "valid email",
			email:    "user@example.com",
			expected: "example.com",
		},
		{
			name:     "email with subdomain",
			email:    "user@mail.example.org",
			expected: "mail.example.org",
		},
		{
			name:     "invalid email no @",
			email:    "invalid",
			expected: "",
		},
		{
			name:     "email with multiple @",
			email:    "user@domain@example.com",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractDomain(tt.email)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSlogAttributes(t *testing.T) {
	// Test that all attribute functions return correct types and keys
	t.Run("Operation", func(t *testing.T) {
		attr := Operation("list")
		assert.Equal(t, KeyOperation, attr.Key)
		assert.Equal(t, "list", attr.Value.String())
	})

	t.Run("Namespace", func(t *testing.T) {
		attr := Namespace("default")
		assert.Equal(t, KeyNamespace, attr.Key)
		assert.Equal(t, "default", attr.Value.String())
	})

	t.Run("ResourceType", func(t *testing.T) {
		attr := ResourceType("pods")
		assert.Equal(t, KeyResourceType, attr.Key)
		assert.Equal(t, "pods", attr.Value.String())
	})

	t.Run("ResourceName", func(t *testing.T) {
		attr := ResourceName("my-pod")
		assert.Equal(t, KeyResourceName, attr.Key)
		assert.Equal(t, "my-pod", attr.Value.String())
	})

	t.Run("Cluster", func(t *testing.T) {
		attr := Cluster("prod-cluster")
		assert.Equal(t, KeyCluster, attr.Key)
		assert.Equal(t, "prod-cluster", attr.Value.String())
	})

	t.Run("Status", func(t *testing.T) {
		attr := Status(StatusSuccess)
		assert.Equal(t, KeyStatus, attr.Key)
		assert.Equal(t, StatusSuccess, attr.Value.String())
	})

	t.Run("Err with nil", func(t *testing.T) {
		attr := Err(nil)
		assert.Equal(t, KeyError, attr.Key)
		assert.Equal(t, "", attr.Value.String())
	})

	t.Run("Err with error", func(t *testing.T) {
		testErr := fmt.Errorf("test error message")
		attr := Err(testErr)
		assert.Equal(t, KeyError, attr.Key)
		assert.Equal(t, "test error message", attr.Value.String())
	})

	t.Run("SanitizedErr with nil", func(t *testing.T) {
		attr := SanitizedErr(nil)
		assert.Equal(t, KeyError, attr.Key)
		assert.Equal(t, "", attr.Value.String())
	})

	t.Run("SanitizedErr with IP in error message", func(t *testing.T) {
		testErr := fmt.Errorf("failed to connect to https://192.168.1.100:6443: connection refused")
		attr := SanitizedErr(testErr)
		assert.Equal(t, KeyError, attr.Key)
		assert.NotContains(t, attr.Value.String(), "192.168.1.100", "IP address should be sanitized")
		assert.Contains(t, attr.Value.String(), "<redacted-ip>", "IP should be replaced with redacted marker")
		assert.Contains(t, attr.Value.String(), "connection refused", "rest of error should be preserved")
	})

	t.Run("SanitizedErr with hostname only", func(t *testing.T) {
		testErr := fmt.Errorf("failed to connect to https://api.cluster.example.com:6443")
		attr := SanitizedErr(testErr)
		assert.Equal(t, KeyError, attr.Key)
		assert.Contains(t, attr.Value.String(), "api.cluster.example.com", "hostname should be preserved")
	})

	t.Run("UserHash", func(t *testing.T) {
		attr := UserHash("user@example.com")
		assert.Equal(t, KeyUserHash, attr.Key)
		assert.Contains(t, attr.Value.String(), "user:")
	})

	t.Run("Host", func(t *testing.T) {
		attr := Host("https://192.168.1.1:6443")
		assert.Equal(t, KeyHost, attr.Key)
		assert.NotContains(t, attr.Value.String(), "192.168")
	})

	t.Run("Domain", func(t *testing.T) {
		attr := Domain("user@example.com")
		assert.Equal(t, "user_domain", attr.Key)
		assert.Equal(t, "example.com", attr.Value.String())
	})
}

func TestWithOperationLogger(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	opLogger := WithOperation(logger, "test.operation")
	opLogger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "operation")
	assert.Contains(t, output, "test.operation")
}

func TestWithToolLogger(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	toolLogger := WithTool(logger, "kubernetes_list")
	toolLogger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "tool")
	assert.Contains(t, output, "kubernetes_list")
}

func TestWithClusterLogger(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	clusterLogger := WithCluster(logger, "prod-cluster")
	clusterLogger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "cluster")
	assert.Contains(t, output, "prod-cluster")
}
