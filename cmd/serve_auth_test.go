package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateHTTPAuth(t *testing.T) {
	token := strings.Repeat("a", minAuthTokenLength)
	oauth := OAuthServeConfig{Enabled: true}

	tests := []struct {
		name    string
		config  ServeConfig
		wantErr string
	}{
		{name: "stdio needs no authentication", config: ServeConfig{Transport: transportStdio}},
		{name: "streamable-http with OAuth", config: ServeConfig{Transport: transportStreamableHTTP, OAuth: oauth}},
		{name: "streamable-http with bearer token", config: ServeConfig{Transport: transportStreamableHTTP, AuthToken: token}},
		{name: "sse with bearer token", config: ServeConfig{Transport: transportSSE, AuthToken: token}},
		{
			name:    "streamable-http without authentication is refused",
			config:  ServeConfig{Transport: transportStreamableHTTP},
			wantErr: "the streamable-http transport requires authentication",
		},
		{
			name:    "sse without authentication is refused",
			config:  ServeConfig{Transport: transportSSE},
			wantErr: "the sse transport requires authentication",
		},
		{
			name:    "sse with OAuth is refused",
			config:  ServeConfig{Transport: transportSSE, OAuth: oauth},
			wantErr: "supported on the streamable-http transport only",
		},
		{
			name:    "OAuth and bearer token together are refused",
			config:  ServeConfig{Transport: transportStreamableHTTP, OAuth: oauth, AuthToken: token},
			wantErr: "mutually exclusive",
		},
		{
			name:    "short bearer token is refused",
			config:  ServeConfig{Transport: transportStreamableHTTP, AuthToken: token[1:]},
			wantErr: "at least 32 characters",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPAuth(tt.config)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMemoryStorageWarning(t *testing.T) {
	assert.Contains(t, memoryStorageWarning(OAuthStorageTypeMemory), "in-memory token storage")
	assert.Contains(t, memoryStorageWarning(""), "in-memory token storage")
	assert.Empty(t, memoryStorageWarning(OAuthStorageTypeValkey))
}

func TestValidateOAuthEncryption(t *testing.T) {
	const key = "c2VjcmV0LWtleS10aGF0LWlzLTMyLWJ5dGVzLWxvbmch"
	tests := []struct {
		name    string
		config  OAuthServeConfig
		wantErr bool
	}{
		{name: "valkey with a key", config: OAuthServeConfig{Storage: OAuthStorageConfig{Type: OAuthStorageTypeValkey}, EncryptionKey: key}},
		{name: "valkey without a key is refused", config: OAuthServeConfig{Storage: OAuthStorageConfig{Type: OAuthStorageTypeValkey}}, wantErr: true},
		{name: "memory without a key", config: OAuthServeConfig{Storage: OAuthStorageConfig{Type: OAuthStorageTypeMemory}}},
		{name: "default storage without a key", config: OAuthServeConfig{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthEncryption(tt.config)
			if tt.wantErr {
				assert.ErrorContains(t, err, "OAUTH_ENCRYPTION_KEY (--oauth-encryption-key) is required with valkey storage")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateMaxRequestSize(t *testing.T) {
	tests := []struct {
		name    string
		config  ServeConfig
		wantErr bool
	}{
		{name: "positive limit", config: ServeConfig{Transport: transportStreamableHTTP, MaxRequestSize: 1}},
		{name: "stdio ignores the limit", config: ServeConfig{Transport: transportStdio}},
		{name: "zero is refused", config: ServeConfig{Transport: transportStreamableHTTP}, wantErr: true},
		{name: "negative is refused", config: ServeConfig{Transport: transportSSE, MaxRequestSize: -1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMaxRequestSize(tt.config)
			if tt.wantErr {
				assert.ErrorContains(t, err, "--max-request-size must be a positive number")
				return
			}
			assert.NoError(t, err)
		})
	}
}
