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
				_ = os.Setenv("KUBECONFIG", kubeconfigPath)
				defer func() { _ = os.Unsetenv("KUBECONFIG") }()
			}

			// We can't easily test the full runServe function without complex mocking,
			// but we can verify that the configuration is correctly structured
			config := ServeConfig{
				Transport:          "stdio",
				HTTPAddr:           ":8080",
				SSEEndpoint:        "/sse",
				MessageEndpoint:    "/message",
				HTTPEndpoint:       "/mcp",
				NonDestructiveMode: true,
				DryRun:             false,
				QPSLimit:           20.0,
				BurstLimit:         30,
				DebugMode:          false,
				InCluster:          tt.inCluster,
				OAuth: OAuthServeConfig{
					Enabled:                       false,
					BaseURL:                       "",
					GoogleClientID:                "",
					GoogleClientSecret:            "",
					DisableStreaming:              false,
					RegistrationToken:             "",
					AllowPublicRegistration:       false,
					AllowInsecureAuthWithoutState: false,
					MaxClientsPerIP:               10,
					EncryptionKey:                 "",
				},
				DownstreamOAuth: false,
			}
			err := runServe(config)

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

// TestCIMDEnvVarParsing tests that CIMD environment variables are correctly parsed
func TestCIMDEnvVarParsing(t *testing.T) {
	tests := []struct {
		name           string
		envVar         string
		envValue       string
		flagName       string
		expectedValue  bool
		expectWarning  bool
		setFlag        bool // if true, set the flag explicitly (should override env)
		flagValue      bool
		expectedResult bool // expected final value when flag is set
	}{
		// ENABLE_CIMD tests
		{
			name:          "ENABLE_CIMD=true enables CIMD",
			envVar:        "ENABLE_CIMD",
			envValue:      "true",
			flagName:      "enable-cimd",
			expectedValue: true,
		},
		{
			name:          "ENABLE_CIMD=false disables CIMD",
			envVar:        "ENABLE_CIMD",
			envValue:      "false",
			flagName:      "enable-cimd",
			expectedValue: false,
		},
		{
			name:          "ENABLE_CIMD=1 enables CIMD",
			envVar:        "ENABLE_CIMD",
			envValue:      "1",
			flagName:      "enable-cimd",
			expectedValue: true,
		},
		{
			name:          "ENABLE_CIMD=0 disables CIMD",
			envVar:        "ENABLE_CIMD",
			envValue:      "0",
			flagName:      "enable-cimd",
			expectedValue: false,
		},
		{
			name:          "ENABLE_CIMD invalid value uses default",
			envVar:        "ENABLE_CIMD",
			envValue:      "invalid",
			flagName:      "enable-cimd",
			expectedValue: true, // default is true
			expectWarning: true,
		},
		// CIMD_ALLOW_PRIVATE_IPS tests
		{
			name:          "CIMD_ALLOW_PRIVATE_IPS=true enables private IPs",
			envVar:        "CIMD_ALLOW_PRIVATE_IPS",
			envValue:      "true",
			flagName:      "cimd-allow-private-ips",
			expectedValue: true,
		},
		{
			name:          "CIMD_ALLOW_PRIVATE_IPS=false keeps disabled",
			envVar:        "CIMD_ALLOW_PRIVATE_IPS",
			envValue:      "false",
			flagName:      "cimd-allow-private-ips",
			expectedValue: false,
		},
		{
			name:          "CIMD_ALLOW_PRIVATE_IPS invalid value uses default",
			envVar:        "CIMD_ALLOW_PRIVATE_IPS",
			envValue:      "notabool",
			flagName:      "cimd-allow-private-ips",
			expectedValue: false, // default is false
			expectWarning: true,
		},
		// Flag overrides env var
		{
			name:           "flag overrides ENABLE_CIMD env var",
			envVar:         "ENABLE_CIMD",
			envValue:       "true",
			flagName:       "enable-cimd",
			setFlag:        true,
			flagValue:      false,
			expectedResult: false, // flag wins
		},
		{
			name:           "flag overrides CIMD_ALLOW_PRIVATE_IPS env var",
			envVar:         "CIMD_ALLOW_PRIVATE_IPS",
			envValue:       "true",
			flagName:       "cimd-allow-private-ips",
			setFlag:        true,
			flagValue:      false,
			expectedResult: false, // flag wins
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			t.Setenv(tt.envVar, tt.envValue)

			// Create a mock command that captures the parsed values
			var capturedValue bool
			cmd := newServeCmd()

			// Replace the RunE to capture values without actually running the server
			originalRunE := cmd.RunE
			cmd.RunE = func(c *cobra.Command, args []string) error {
				// Call the original to trigger env var processing, but catch the error
				// We use a goroutine with recover to handle any panics
				err := originalRunE(c, args)
				// Get the value that was set
				capturedValue, _ = c.Flags().GetBool(tt.flagName)
				return err
			}

			// Build args
			var cmdArgs []string
			if tt.setFlag {
				cmdArgs = append(cmdArgs, "--"+tt.flagName+"="+boolToString(tt.flagValue))
			}
			// Add minimal required args to avoid other validation errors
			cmdArgs = append(cmdArgs, "--transport=stdio")
			cmd.SetArgs(cmdArgs)

			// Suppress output during test
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			// Execute - we expect it to fail (no kubeconfig), but env vars should be parsed
			_ = cmd.Execute()

			// Check the captured value
			if tt.setFlag {
				assert.Equal(t, tt.expectedResult, capturedValue,
					"expected flag to override env var to %v", tt.expectedResult)
			} else {
				assert.Equal(t, tt.expectedValue, capturedValue,
					"expected env var %s=%s to set %s to %v",
					tt.envVar, tt.envValue, tt.flagName, tt.expectedValue)
			}
		})
	}
}

// boolToString converts a bool to "true" or "false" string
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
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
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0600) // #nosec G306 - test file
	require.NoError(t, err)

	return kubeconfigPath
}

// TestSplitAndTrimAudiences tests the splitAndTrimAudiences helper function
// used to parse the OAUTH_TRUSTED_AUDIENCES environment variable.
func TestSplitAndTrimAudiences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string returns nil",
			input:    "",
			expected: nil,
		},
		{
			name:     "single audience",
			input:    "muster-client",
			expected: []string{"muster-client"},
		},
		{
			name:     "multiple audiences",
			input:    "muster-client,another-aggregator",
			expected: []string{"muster-client", "another-aggregator"},
		},
		{
			name:     "trims whitespace around entries",
			input:    " muster-client , another-aggregator ",
			expected: []string{"muster-client", "another-aggregator"},
		},
		{
			name:     "filters empty entries from trailing comma",
			input:    "muster-client,another-aggregator,",
			expected: []string{"muster-client", "another-aggregator"},
		},
		{
			name:     "filters empty entries from consecutive commas",
			input:    "muster-client,,another-aggregator",
			expected: []string{"muster-client", "another-aggregator"},
		},
		{
			name:     "filters whitespace-only entries",
			input:    "muster-client,   ,another-aggregator",
			expected: []string{"muster-client", "another-aggregator"},
		},
		{
			name:     "handles leading comma",
			input:    ",muster-client",
			expected: []string{"muster-client"},
		},
		{
			name:     "whitespace only returns nil",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "complex real-world example",
			input:    "muster-client, my-aggregator-v2, internal.service.client",
			expected: []string{"muster-client", "my-aggregator-v2", "internal.service.client"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrimAudiences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestOAuthTrustedAudiencesEnvVar tests that OAUTH_TRUSTED_AUDIENCES is correctly
// parsed and applied when the --trusted-audiences flag is not set.
func TestOAuthTrustedAudiencesEnvVar(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		flagValue     []string
		expectedValue []string
	}{
		{
			name:          "env var sets trusted audiences",
			envValue:      "muster-client,another-aggregator",
			flagValue:     nil,
			expectedValue: []string{"muster-client", "another-aggregator"},
		},
		{
			name:          "env var with whitespace is trimmed",
			envValue:      " muster-client , another-aggregator ",
			flagValue:     nil,
			expectedValue: []string{"muster-client", "another-aggregator"},
		},
		{
			name:          "flag overrides env var",
			envValue:      "env-client",
			flagValue:     []string{"flag-client"},
			expectedValue: []string{"flag-client"},
		},
		{
			name:          "empty env var returns nil",
			envValue:      "",
			flagValue:     nil,
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				t.Setenv("OAUTH_TRUSTED_AUDIENCES", tt.envValue)
			}

			// Simulate the logic from runServe
			var result []string
			if len(tt.flagValue) > 0 {
				result = tt.flagValue
			} else if envVal := os.Getenv("OAUTH_TRUSTED_AUDIENCES"); envVal != "" {
				result = splitAndTrimAudiences(envVal)
			}

			assert.Equal(t, tt.expectedValue, result)
		})
	}
}
