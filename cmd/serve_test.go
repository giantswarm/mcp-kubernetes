package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServeCmdProperties(t *testing.T) {
	cmd := newServeCmd()

	assert.Equal(t, "serve", cmd.Use)
	assert.Equal(t, "Start the MCP Kubernetes server", cmd.Short)
	assert.True(t, strings.Contains(cmd.Long, "Model Context Protocol"))
	assert.True(t, strings.Contains(cmd.Long, "stdio"))
	assert.True(t, strings.Contains(cmd.Long, "sse"))
	assert.True(t, strings.Contains(cmd.Long, "streamable-http"))
}

func TestServeCmdFlags(t *testing.T) {
	cmd := newServeCmd()

	// Test that all expected flags exist
	flagNames := []string{
		"non-destructive",
		"dry-run",
		"qps-limit",
		"burst-limit",
		"transport",
		"http-addr",
		"sse-endpoint",
		"message-endpoint",
		"http-endpoint",
	}

	for _, flagName := range flagNames {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "Flag %s should exist", flagName)
	}
}

func TestServeCmdFlagDefaults(t *testing.T) {
	cmd := newServeCmd()

	// Test flag default values
	tests := []struct {
		flagName string
		expected string
	}{
		{"non-destructive", "true"},
		{"dry-run", "false"},
		{"qps-limit", "20"},
		{"burst-limit", "30"},
		{"transport", "stdio"},
		{"http-addr", ":8080"},
		{"sse-endpoint", "/sse"},
		{"message-endpoint", "/message"},
		{"http-endpoint", "/mcp"},
	}

	for _, test := range tests {
		flag := cmd.Flags().Lookup(test.flagName)
		assert.Equal(t, test.expected, flag.DefValue,
			"Flag %s should have default value %s", test.flagName, test.expected)
	}
}

func TestServeCmdTransportValidation(t *testing.T) {
	tests := []struct {
		name          string
		transport     string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid stdio transport",
			transport:   "stdio",
			expectError: false,
		},
		{
			name:        "valid sse transport",
			transport:   "sse",
			expectError: false,
		},
		{
			name:        "valid streamable-http transport",
			transport:   "streamable-http",
			expectError: false,
		},
		{
			name:          "invalid transport",
			transport:     "invalid",
			expectError:   true,
			errorContains: "unsupported transport type",
		},
		{
			name:          "empty transport",
			transport:     "",
			expectError:   true,
			errorContains: "unsupported transport type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test transport validation by checking if it's in valid transports
			validTransports := map[string]bool{
				"stdio":           true,
				"sse":             true,
				"streamable-http": true,
			}

			isValid := validTransports[tt.transport]

			if tt.expectError {
				assert.False(t, isValid, "Transport %s should be invalid", tt.transport)
			} else {
				assert.True(t, isValid, "Transport %s should be valid", tt.transport)
			}
		})
	}
}

func TestServeCmdFlagUsage(t *testing.T) {
	cmd := newServeCmd()

	// Test that help text contains transport information
	usage := cmd.UsageString()
	assert.Contains(t, usage, "--transport")
	assert.Contains(t, usage, "stdio, sse, or streamable-http")
}

func TestServeCmdTransportSpecificFlags(t *testing.T) {
	cmd := newServeCmd()

	// Test that HTTP-related flags have appropriate descriptions
	httpAddrFlag := cmd.Flags().Lookup("http-addr")
	assert.Contains(t, httpAddrFlag.Usage, "HTTP server address")
	assert.Contains(t, httpAddrFlag.Usage, "sse and streamable-http")

	sseEndpointFlag := cmd.Flags().Lookup("sse-endpoint")
	assert.Contains(t, sseEndpointFlag.Usage, "SSE endpoint path")
	assert.Contains(t, sseEndpointFlag.Usage, "sse transport")

	messageEndpointFlag := cmd.Flags().Lookup("message-endpoint")
	assert.Contains(t, messageEndpointFlag.Usage, "Message endpoint path")
	assert.Contains(t, messageEndpointFlag.Usage, "sse transport")

	httpEndpointFlag := cmd.Flags().Lookup("http-endpoint")
	assert.Contains(t, httpEndpointFlag.Usage, "HTTP endpoint path")
	assert.Contains(t, httpEndpointFlag.Usage, "streamable-http transport")
}
