package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelfUpdateCmd(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		expectError   bool
		errorContains string
	}{
		{
			name:          "self-update with dev version should fail",
			version:       "dev",
			expectError:   true,
			errorContains: "cannot self-update a development version",
		},
		{
			name:          "self-update with empty version should fail",
			version:       "",
			expectError:   true,
			errorContains: "cannot self-update a development version",
		},
		// Note: Testing actual updates would require network access and real releases
		// which is not suitable for unit tests
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the root command version
			originalVersion := rootCmd.Version
			defer func() {
				rootCmd.Version = originalVersion
			}()
			rootCmd.Version = tt.version

			// Create the self-update command
			cmd := newSelfUpdateCmd()

			// Execute the command
			err := cmd.Execute()

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSelfUpdateCmdProperties(t *testing.T) {
	cmd := newSelfUpdateCmd()

	assert.Equal(t, "self-update", cmd.Use)
	assert.Equal(t, "Update mcp-kubernetes to the latest version", cmd.Short)
	assert.True(t, strings.Contains(cmd.Long, "mcp-kubernetes"))
	assert.True(t, strings.Contains(cmd.Long, "GitHub"))
}

func TestGithubRepoSlug(t *testing.T) {
	// Ensure the GitHub repository slug is correctly set
	assert.Equal(t, "giantswarm/mcp-kubernetes", githubRepoSlug)
}
