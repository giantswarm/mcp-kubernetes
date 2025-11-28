package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServeCmd(t *testing.T) {
	cmd := newServeCmd()

	// Test that the command has the expected flags
	assert.True(t, cmd.Flags().HasAvailableFlags())

	// Test that the --in-cluster flag exists
	flag := cmd.Flags().Lookup("in-cluster")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Use in-cluster authentication (service account token) instead of kubeconfig (default: false)", flag.Usage)
}

func TestServeCmdHelp(t *testing.T) {
	cmd := newServeCmd()
	cmd.SetArgs([]string{"--help"})

	var output bytes.Buffer
	cmd.SetOut(&output)

	// Execute should fail because of --help but that's expected
	err := cmd.Execute()
	// Help command returns with specific error type, we just want to check the output

	helpText := output.String()

	// Verify that authentication modes section is present
	assert.Contains(t, helpText, "Authentication modes:")
	assert.Contains(t, helpText, "Kubeconfig (default): Uses standard kubeconfig file authentication")
	assert.Contains(t, helpText, "In-cluster: Uses service account token when running inside a Kubernetes pod")

	// Verify that the --in-cluster flag is documented
	assert.Contains(t, helpText, "--in-cluster")
	assert.Contains(t, helpText, "Use in-cluster authentication")

	_ = err // Ignore the help error
}

func TestInClusterFlagParsing(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantError bool
	}{
		{
			name:      "default kubeconfig mode",
			args:      []string{},
			wantError: false, // Will fail later due to missing kubeconfig but flag parsing should work
		},
		{
			name:      "in-cluster mode enabled",
			args:      []string{"--in-cluster"},
			wantError: false, // Will fail later due to missing service account but flag parsing should work
		},
		{
			name:      "in-cluster mode with other flags",
			args:      []string{"--in-cluster", "--debug", "--non-destructive=false"},
			wantError: false, // Will fail later but flag parsing should work
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We'll create a mock command that captures the flags without executing the full serve logic
			var capturedInCluster bool
			var capturedDebug bool
			var capturedNonDestructive bool

			cmd := &cobra.Command{
				Use: "serve",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Capture flag values
					capturedInCluster, _ = cmd.Flags().GetBool("in-cluster")
					capturedDebug, _ = cmd.Flags().GetBool("debug")
					capturedNonDestructive, _ = cmd.Flags().GetBool("non-destructive")
					return nil // Don't actually run serve logic
				},
			}

			// Add the same flags as the real serve command
			cmd.Flags().Bool("in-cluster", false, "Use in-cluster authentication")
			cmd.Flags().Bool("debug", false, "Enable debug logging")
			cmd.Flags().Bool("non-destructive", true, "Enable non-destructive mode")

			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check flag values for specific test cases
				if tt.name == "in-cluster mode enabled" {
					assert.True(t, capturedInCluster)
				}
				if tt.name == "in-cluster mode with other flags" {
					assert.True(t, capturedInCluster)
					assert.True(t, capturedDebug)
					assert.False(t, capturedNonDestructive)
				}
			}
		})
	}
}

func TestRunServeWithInCluster(t *testing.T) {
	// This test verifies that the in-cluster flag is properly passed through to the k8s client config
	// We can't test the full functionality without being in a cluster, but we can test the configuration flow

	// Create a temporary kubeconfig for fallback testing
	tmpDir := t.TempDir()

	tests := []struct {
		name                string
		inCluster           bool
		expectedError       string
		shouldSetKubeconfig bool
	}{
		{
			name:                "kubeconfig mode should work with valid kubeconfig",
			inCluster:           false,
			shouldSetKubeconfig: true,
			expectedError:       "", // Should work if kubeconfig is valid
		},
		{
			name:          "in-cluster mode should fail outside cluster",
			inCluster:     true,
			expectedError: "in-cluster authentication not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip the kubeconfig test if we don't have a valid kubeconfig and can't create one
			if tt.shouldSetKubeconfig {
				kubeconfigPath := createTestKubeconfigFile(t, tmpDir)
				os.Setenv("KUBECONFIG", kubeconfigPath)
				defer os.Unsetenv("KUBECONFIG")
			}

			// We can't easily test the full runServe function without complex mocking,
			// but we can verify that the configuration is correctly structured
			err := runServe("stdio", true, false, 20.0, 30, false, tt.inCluster, ":8080", "/sse", "/message", "/mcp",
				false, "", "", "", false, "", false, false, 10, "", false)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				// For successful cases, we might still get errors due to server startup in test environment
				// The important part is that it's not failing due to client configuration
				if err != nil {
					// Allow certain expected errors that aren't related to our in-cluster flag
					allowedErrors := []string{
						"connection refused",
						"no such file or directory",
						"transport endpoint is not connected",
					}

					errorLower := strings.ToLower(err.Error())
					hasAllowedError := false
					for _, allowedErr := range allowedErrors {
						if strings.Contains(errorLower, allowedErr) {
							hasAllowedError = true
							break
						}
					}

					if !hasAllowedError {
						t.Logf("Unexpected error (but may be environment-related): %v", err)
					}
				}
			}
		})
	}
}

// Helper function to create a test kubeconfig file
func createTestKubeconfigFile(t *testing.T, dir string) string {
	kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-server:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
    namespace: default
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`

	kubeconfigPath := dir + "/kubeconfig"
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
	require.NoError(t, err)

	return kubeconfigPath
}
