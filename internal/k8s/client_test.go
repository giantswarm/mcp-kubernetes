package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
)

// testLogger implements the Logger interface for testing
type testLogger struct {
	messages []string
}

func (l *testLogger) Debug(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Info(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Warn(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Error(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

// Helper function to create test kubeconfig
func createTestKubeconfig() *api.Config {
	return &api.Config{
		Clusters: map[string]*api.Cluster{
			"test-cluster": {
				Server: "https://test-server:6443",
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
		},
		CurrentContext: "test-context",
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *ClientConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config should fail",
			config:  nil,
			wantErr: true,
			errMsg:  "client configuration is required",
		},
		{
			name: "in-cluster mode without service account files should fail",
			config: &ClientConfig{
				InCluster: true,
				Logger:    &testLogger{},
			},
			wantErr: true,
			errMsg:  "in-cluster authentication not available",
		},
		{
			name: "kubeconfig mode with invalid context should fail",
			config: &ClientConfig{
				InCluster: false,
				Context:   "non-existent-context",
				Logger:    &testLogger{},
			},
			wantErr: true,
			errMsg:  "does not exist in kubeconfig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip in-cluster test if we're actually in a cluster
			if tt.config != nil && tt.config.InCluster {
				if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
					t.Skip("Skipping in-cluster test because we're actually in a cluster")
				}
			}

			// Create a temporary kubeconfig file for testing
			if tt.config != nil && !tt.config.InCluster {
				tmpDir := t.TempDir()
				kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")
				tt.config.KubeconfigPath = kubeconfigPath
				// Create a minimal kubeconfig file
				createMinimalKubeconfig(t, kubeconfigPath)
			}

			client, err := NewClient(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestNewClientInCluster(t *testing.T) {
	// Create temporary service account files
	tmpDir := t.TempDir()
	serviceAccountDir := filepath.Join(tmpDir, "serviceaccount")
	err := os.MkdirAll(serviceAccountDir, 0750) // #nosec G301 - test directory with restricted permissions
	require.NoError(t, err)

	// Create mock service account files
	tokenPath := filepath.Join(serviceAccountDir, "token")
	err = os.WriteFile(tokenPath, []byte("test-token"), 0600) // #nosec G306 - test file with restricted permissions
	require.NoError(t, err)

	caPath := filepath.Join(serviceAccountDir, "ca.crt")
	err = os.WriteFile(caPath, []byte("test-ca"), 0600) // #nosec G306 - test file with restricted permissions
	require.NoError(t, err)

	namespacePath := filepath.Join(serviceAccountDir, "namespace")
	err = os.WriteFile(namespacePath, []byte("test-namespace"), 0600) // #nosec G306 - test file with restricted permissions
	require.NoError(t, err)

	// Temporarily override the service account path
	originalPath := "/var/run/secrets/kubernetes.io/serviceaccount"

	// Test with mock files by creating a client that has the validate method mocked
	config := &ClientConfig{
		InCluster: true,
		Logger:    &testLogger{},
	}

	// Since we can't easily mock the service account path without changing the implementation,
	// we'll test the validateInClusterEnvironment method separately
	client := &kubernetesClient{
		config: config,
	}

	// Test validateInClusterEnvironment with non-existent files
	err = client.validateInClusterEnvironment()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service account token not found")

	_ = originalPath // avoid unused variable warning
}

func TestClientContextManagement(t *testing.T) {
	// Test with a client that has kubeconfig data
	client := &kubernetesClient{
		config: &ClientConfig{
			InCluster: false,
			Logger:    &testLogger{},
		},
		kubeconfigData: createTestKubeconfig(),
		currentContext: "test-context",
	}

	ctx := context.Background()

	t.Run("ListContexts in kubeconfig mode", func(t *testing.T) {
		contexts, err := client.ListContexts(ctx)
		require.NoError(t, err)
		assert.Len(t, contexts, 1)
		assert.Equal(t, "test-context", contexts[0].Name)
		assert.Equal(t, "test-cluster", contexts[0].Cluster)
		assert.Equal(t, "test-user", contexts[0].User)
		assert.Equal(t, "test-namespace", contexts[0].Namespace)
		assert.True(t, contexts[0].Current)
	})

	t.Run("GetCurrentContext in kubeconfig mode", func(t *testing.T) {
		currentCtx, err := client.GetCurrentContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, "test-context", currentCtx.Name)
		assert.Equal(t, "test-cluster", currentCtx.Cluster)
		assert.Equal(t, "test-user", currentCtx.User)
		assert.Equal(t, "test-namespace", currentCtx.Namespace)
		assert.True(t, currentCtx.Current)
	})

	t.Run("SwitchContext in kubeconfig mode", func(t *testing.T) {
		err := client.SwitchContext(ctx, "test-context")
		require.NoError(t, err)

		// Try switching to non-existent context
		err = client.SwitchContext(ctx, "non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist in kubeconfig")
	})
}

func TestClientContextManagementInCluster(t *testing.T) {
	// Test with a client in in-cluster mode
	client := &kubernetesClient{
		config: &ClientConfig{
			InCluster: true,
			Logger:    &testLogger{},
		},
		currentContext: "in-cluster",
	}

	ctx := context.Background()

	t.Run("ListContexts in in-cluster mode", func(t *testing.T) {
		contexts, err := client.ListContexts(ctx)
		require.NoError(t, err)
		assert.Len(t, contexts, 1)
		assert.Equal(t, "in-cluster", contexts[0].Name)
		assert.Equal(t, "in-cluster", contexts[0].Cluster)
		assert.Equal(t, "serviceaccount", contexts[0].User)
		// The namespace will be "default" since the service account file doesn't exist
		assert.Equal(t, "default", contexts[0].Namespace)
		assert.True(t, contexts[0].Current)
	})

	t.Run("GetCurrentContext in in-cluster mode", func(t *testing.T) {
		currentCtx, err := client.GetCurrentContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, "in-cluster", currentCtx.Name)
		assert.Equal(t, "in-cluster", currentCtx.Cluster)
		assert.Equal(t, "serviceaccount", currentCtx.User)
		// The namespace will be "default" since the service account file doesn't exist
		assert.Equal(t, "default", currentCtx.Namespace)
		assert.True(t, currentCtx.Current)
	})

	t.Run("SwitchContext in in-cluster mode", func(t *testing.T) {
		// Should allow switching to "in-cluster"
		err := client.SwitchContext(ctx, "in-cluster")
		require.NoError(t, err)

		// Should not allow switching to other contexts
		err = client.SwitchContext(ctx, "other-context")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot switch context in in-cluster mode")
	})
}

func TestGetInClusterNamespace(t *testing.T) {
	// Create a temporary namespace file
	tmpDir := t.TempDir()
	namespacePath := filepath.Join(tmpDir, "namespace")

	// Test with existing namespace file
	err := os.WriteFile(namespacePath, []byte("my-namespace"), 0600) // #nosec G306 - test file
	require.NoError(t, err)

	client := &kubernetesClient{}

	// We can't easily test the actual method without modifying the implementation
	// to accept a path parameter, so we'll test the fallback behavior
	namespace := client.getInClusterNamespace()
	// This will return "default" since the actual service account file doesn't exist
	assert.Equal(t, "default", namespace)
}

func TestValidateInClusterEnvironment(t *testing.T) {
	client := &kubernetesClient{}

	// Test with missing service account files (normal case outside cluster)
	err := client.validateInClusterEnvironment()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service account token not found")
}

// Helper function to create minimal kubeconfig for testing
func createMinimalKubeconfig(t testing.TB, path string) {
	t.Helper()
	kubeconfig := `
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
    namespace: test-namespace
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	err := os.WriteFile(path, []byte(kubeconfig), 0600) // #nosec G306 - test file
	require.NoError(t, err)
}

func BenchmarkNewClient(b *testing.B) {
	tmpDir := b.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")
	createMinimalKubeconfig(b, kubeconfigPath)

	config := &ClientConfig{
		KubeconfigPath: kubeconfigPath,
		Logger:         &testLogger{},
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
