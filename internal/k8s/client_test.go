package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
)

// MockLogger for testing
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Debug(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Info(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Warn(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Error(msg string, args ...interface{}) {
	m.Called(msg, args)
}

// Helper function to create test kubeconfig
func createTestKubeconfig() *api.Config {
	return &api.Config{
		Clusters: map[string]*api.Cluster{
			"test-cluster": {
				Server: "https://test.example.com",
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			"test-user": {
				Token: "test-token",
			},
		},
		Contexts: map[string]*api.Context{
			"test-context": {
				Cluster:   "test-cluster",
				AuthInfo:  "test-user",
				Namespace: "test-namespace",
			},
			"another-context": {
				Cluster:   "test-cluster",
				AuthInfo:  "test-user",
				Namespace: "another-namespace",
			},
		},
		CurrentContext: "test-context",
	}
}

// Test NewClient function with simplified approach
func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		config      *ClientConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "client configuration is required",
		},
		{
			name: "valid config with defaults",
			config: &ClientConfig{
				NonDestructiveMode: true,
			},
			expectError: false,
		},
		{
			name: "valid config with custom values",
			config: &ClientConfig{
				QPSLimit:             50.0,
				BurstLimit:           100,
				Timeout:              60 * time.Second,
				NonDestructiveMode:   false,
				DryRun:               true,
				AllowedOperations:    []string{"get", "list"},
				RestrictedNamespaces: []string{"kube-system"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary kubeconfig file for testing
			tmpDir := t.TempDir()
			kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")

			if tt.config != nil {
				tt.config.KubeconfigPath = kubeconfigPath
				// Create a minimal kubeconfig file
				createMinimalKubeconfig(t, kubeconfigPath)
			}

			client, err := NewClient(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)

				// Verify defaults are set correctly
				if tt.config.QPSLimit == 0 {
					assert.Equal(t, float32(20.0), client.qpsLimit)
				} else {
					assert.Equal(t, tt.config.QPSLimit, client.qpsLimit)
				}

				if tt.config.BurstLimit == 0 {
					assert.Equal(t, 30, client.burstLimit)
				} else {
					assert.Equal(t, tt.config.BurstLimit, client.burstLimit)
				}

				if tt.config.Timeout == 0 {
					assert.Equal(t, 30*time.Second, client.timeout)
				} else {
					assert.Equal(t, tt.config.Timeout, client.timeout)
				}
			}
		})
	}
}

func TestKubernetesClient_BasicContextOperations(t *testing.T) {
	client := &kubernetesClient{
		config:         &ClientConfig{},
		kubeconfigData: createTestKubeconfig(),
		currentContext: "test-context",
	}

	ctx := context.Background()

	t.Run("ListContexts", func(t *testing.T) {
		contexts, err := client.ListContexts(ctx)
		require.NoError(t, err)
		require.Len(t, contexts, 2)

		// Verify contexts
		contextNames := make(map[string]bool)
		for _, context := range contexts {
			contextNames[context.Name] = context.Current
		}

		assert.True(t, contextNames["test-context"])
		assert.False(t, contextNames["another-context"])
	})

	t.Run("GetCurrentContext", func(t *testing.T) {
		currentContext, err := client.GetCurrentContext(ctx)
		require.NoError(t, err)

		assert.Equal(t, "test-context", currentContext.Name)
		assert.Equal(t, "test-cluster", currentContext.Cluster)
		assert.Equal(t, "test-user", currentContext.User)
		assert.Equal(t, "test-namespace", currentContext.Namespace)
		assert.True(t, currentContext.Current)
	})

	t.Run("SwitchContext", func(t *testing.T) {
		err := client.SwitchContext(ctx, "another-context")
		require.NoError(t, err)
		assert.Equal(t, "another-context", client.currentContext)

		// Test switching to non-existent context
		err = client.SwitchContext(ctx, "non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist in kubeconfig")
	})
}

func TestKubernetesClient_SecurityValidation(t *testing.T) {
	tests := []struct {
		name                 string
		nonDestructiveMode   bool
		dryRun               bool
		allowedOperations    []string
		restrictedNamespaces []string
		operation            string
		namespace            string
		expectError          bool
		errorContains        string
	}{
		{
			name:              "allowed operation",
			allowedOperations: []string{"get", "list"},
			operation:         "get",
			expectError:       false,
		},
		{
			name:              "disallowed operation",
			allowedOperations: []string{"get", "list"},
			operation:         "delete",
			expectError:       true,
			errorContains:     "is not allowed",
		},
		{
			name:               "destructive operation in non-destructive mode without dry-run",
			nonDestructiveMode: true,
			dryRun:             false,
			operation:          "delete",
			expectError:        true,
			errorContains:      "destructive operation",
		},
		{
			name:               "destructive operation in non-destructive mode with dry-run",
			nonDestructiveMode: true,
			dryRun:             true,
			operation:          "delete",
			expectError:        false,
		},
		{
			name:                 "restricted namespace",
			restrictedNamespaces: []string{"kube-system"},
			namespace:            "kube-system",
			expectError:          true,
			errorContains:        "is restricted",
		},
		{
			name:                 "allowed namespace",
			restrictedNamespaces: []string{"kube-system"},
			namespace:            "default",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &kubernetesClient{
				nonDestructiveMode:   tt.nonDestructiveMode,
				dryRun:               tt.dryRun,
				allowedOperations:    tt.allowedOperations,
				restrictedNamespaces: tt.restrictedNamespaces,
			}

			// Test operation check
			if tt.operation != "" {
				err := client.isOperationAllowed(tt.operation)
				if tt.expectError && tt.errorContains != "is restricted" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.errorContains)
				} else if !tt.expectError {
					assert.NoError(t, err)
				}
			}

			// Test namespace check
			if tt.namespace != "" {
				err := client.isNamespaceRestricted(tt.namespace)
				if tt.expectError && tt.errorContains == "is restricted" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.errorContains)
				} else if tt.namespace != "" {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestKubernetesClient_LogOperation(t *testing.T) {
	mockLogger := &MockLogger{}
	client := &kubernetesClient{
		config: &ClientConfig{
			Logger: mockLogger,
		},
	}

	// Expect debug log call
	mockLogger.On("Debug", "kubernetes operation", mock.AnythingOfType("[]interface {}")).Return()

	client.logOperation("get", "test-context", "default", "pods", "test-pod")

	mockLogger.AssertExpectations(t)
}

// Helper function to create minimal kubeconfig for testing
func createMinimalKubeconfig(t testing.TB, path string) {
	t.Helper()
	kubeconfig := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test.example.com
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	err := os.WriteFile(path, []byte(kubeconfig), 0644)
	require.NoError(t, err)
}

// Benchmark tests
func BenchmarkNewClient(b *testing.B) {
	tmpDir := b.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")
	createMinimalKubeconfig(b, kubeconfigPath)

	config := &ClientConfig{
		KubeconfigPath: kubeconfigPath,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client, err := NewClient(config)
		if err != nil {
			b.Fatal(err)
		}
		_ = client
	}
}
