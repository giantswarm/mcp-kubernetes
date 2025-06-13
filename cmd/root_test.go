package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCmdProperties(t *testing.T) {
	assert.Equal(t, "mcp-kubernetes", rootCmd.Use)
	assert.Equal(t, "MCP server for Kubernetes operations", rootCmd.Short)
	assert.True(t, strings.Contains(rootCmd.Long, "Model Context Protocol"))
	assert.True(t, strings.Contains(rootCmd.Long, "Kubernetes"))
	assert.True(t, rootCmd.SilenceUsage)
}

func TestSetVersion(t *testing.T) {
	originalVersion := rootCmd.Version
	defer func() {
		rootCmd.Version = originalVersion
	}()

	testVersion := "v1.2.3-test"
	SetVersion(testVersion)

	assert.Equal(t, testVersion, rootCmd.Version)
}

func TestRootCommandHasSubcommands(t *testing.T) {
	subcommands := rootCmd.Commands()

	// Check that expected subcommands exist
	var foundCommands []string
	for _, cmd := range subcommands {
		foundCommands = append(foundCommands, cmd.Use)
	}

	assert.Contains(t, foundCommands, "version")
	assert.Contains(t, foundCommands, "self-update")
	assert.Contains(t, foundCommands, "serve")

	// Ensure we have at least the minimum expected commands
	assert.GreaterOrEqual(t, len(foundCommands), 3)
}
