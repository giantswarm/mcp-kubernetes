package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionCmd(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		expectedOutput string
	}{
		{
			name:           "version command with dev version",
			version:        "dev",
			expectedOutput: "mcp-kubernetes version dev\n",
		},
		{
			name:           "version command with semantic version",
			version:        "v1.2.3",
			expectedOutput: "mcp-kubernetes version v1.2.3\n",
		},
		{
			name:           "version command with empty version",
			version:        "",
			expectedOutput: "mcp-kubernetes version \n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the root command version
			originalVersion := rootCmd.Version
			defer func() {
				rootCmd.Version = originalVersion
			}()
			rootCmd.Version = tt.version

			// Create the version command
			cmd := newVersionCmd()

			// Capture output
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			// Execute the command
			err := cmd.Execute()

			// Assertions
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, buf.String())
		})
	}
}

func TestVersionCmdProperties(t *testing.T) {
	cmd := newVersionCmd()

	assert.Equal(t, "version", cmd.Use)
	assert.Equal(t, "Print the version number of mcp-kubernetes", cmd.Short)
	assert.True(t, strings.Contains(cmd.Long, "mcp-kubernetes"))
}
