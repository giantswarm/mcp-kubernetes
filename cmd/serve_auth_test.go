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
