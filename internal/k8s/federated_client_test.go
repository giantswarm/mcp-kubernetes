package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewFederatedClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *FederatedClientConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config should fail",
			config:  nil,
			wantErr: true,
			errMsg:  "config is required",
		},
		{
			name: "nil clientset should fail",
			config: &FederatedClientConfig{
				ClusterName:   "test-cluster",
				Clientset:     nil,
				DynamicClient: &fakedynamic.FakeDynamicClient{},
				RestConfig:    &rest.Config{Host: "https://test:6443"},
			},
			wantErr: true,
			errMsg:  "clientset is required",
		},
		{
			name: "nil dynamic client should fail",
			config: &FederatedClientConfig{
				ClusterName:   "test-cluster",
				Clientset:     fake.NewSimpleClientset(),
				DynamicClient: nil,
				RestConfig:    &rest.Config{Host: "https://test:6443"},
			},
			wantErr: true,
			errMsg:  "dynamic client is required",
		},
		{
			name: "nil rest config should fail",
			config: &FederatedClientConfig{
				ClusterName:   "test-cluster",
				Clientset:     fake.NewSimpleClientset(),
				DynamicClient: &fakedynamic.FakeDynamicClient{},
				RestConfig:    nil,
			},
			wantErr: true,
			errMsg:  "rest config is required",
		},
		{
			name: "valid config should succeed",
			config: &FederatedClientConfig{
				ClusterName:   "test-cluster",
				Clientset:     fake.NewSimpleClientset(),
				DynamicClient: &fakedynamic.FakeDynamicClient{},
				RestConfig:    &rest.Config{Host: "https://test:6443"},
			},
			wantErr: false,
		},
		{
			name: "empty cluster name should succeed",
			config: &FederatedClientConfig{
				ClusterName:   "",
				Clientset:     fake.NewSimpleClientset(),
				DynamicClient: &fakedynamic.FakeDynamicClient{},
				RestConfig:    &rest.Config{Host: "https://test:6443"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewFederatedClient(tt.config)
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

func TestFederatedClient_ClusterName(t *testing.T) {
	client := createTestFederatedClient(t, "my-test-cluster")
	assert.Equal(t, "my-test-cluster", client.ClusterName())
}

func TestFederatedClient_ListContexts(t *testing.T) {
	client := createTestFederatedClient(t, "federated-cluster")
	ctx := context.Background()

	contexts, err := client.ListContexts(ctx)
	require.NoError(t, err)
	require.Len(t, contexts, 1)

	assert.Equal(t, "federated-cluster", contexts[0].Name)
	assert.Equal(t, "federated-cluster", contexts[0].Cluster)
	assert.True(t, contexts[0].Current)
}

func TestFederatedClient_GetCurrentContext(t *testing.T) {
	client := createTestFederatedClient(t, "current-cluster")
	ctx := context.Background()

	currentCtx, err := client.GetCurrentContext(ctx)
	require.NoError(t, err)
	require.NotNil(t, currentCtx)

	assert.Equal(t, "current-cluster", currentCtx.Name)
	assert.Equal(t, "current-cluster", currentCtx.Cluster)
	assert.True(t, currentCtx.Current)
}

func TestFederatedClient_SwitchContext(t *testing.T) {
	client := createTestFederatedClient(t, "bound-cluster")
	ctx := context.Background()

	t.Run("switching context should fail", func(t *testing.T) {
		err := client.SwitchContext(ctx, "other-cluster")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context switching is not supported")
		assert.Contains(t, err.Error(), "bound-cluster")
	})

	t.Run("switching to same cluster should also fail", func(t *testing.T) {
		// Even switching to the same cluster fails because context switching
		// is fundamentally not supported for federated clients
		err := client.SwitchContext(ctx, "bound-cluster")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context switching is not supported")
	})
}

func TestFederatedClient_ImplementsClientInterface(t *testing.T) {
	// This test verifies at runtime that FederatedClient implements Client
	client := createTestFederatedClient(t, "test-cluster")

	// The compile-time check is already in federated_client.go, but we
	// also verify it works at runtime
	var _ Client = client
}

func TestFederatedClient_ContextParameterIgnored(t *testing.T) {
	// This test documents that kubeContext parameters are ignored
	// by FederatedClient methods (they operate on a single cluster)
	client := createTestFederatedClient(t, "target-cluster")
	ctx := context.Background()

	// GetAPIResources ignores the kubeContext parameter
	// The fake client returns empty results, which is fine for this test
	result, err := client.GetAPIResources(ctx, "ignored-context", 10, 0, "", false, nil)
	// The important thing is that the method can be called with any context parameter
	// and it doesn't affect which cluster is targeted
	require.NoError(t, err)
	assert.NotNil(t, result)

	// GetClusterHealth also ignores kubeContext
	health, err := client.GetClusterHealth(ctx, "another-ignored-context")
	require.NoError(t, err)
	assert.NotNil(t, health)
}

func TestFederatedClient_DiscoveryClientDerived(t *testing.T) {
	client := createTestFederatedClient(t, "test-cluster")

	// Verify that discovery client is derived from clientset
	assert.NotNil(t, client.discoveryClient)
}

func TestFederatedClientConfig_Fields(t *testing.T) {
	// Test that config fields are properly assigned
	clientset := fake.NewSimpleClientset()
	dynamicClient := &fakedynamic.FakeDynamicClient{}
	restConfig := &rest.Config{Host: "https://api.cluster.example.com:6443"}

	config := &FederatedClientConfig{
		ClusterName:   "config-test-cluster",
		Clientset:     clientset,
		DynamicClient: dynamicClient,
		RestConfig:    restConfig,
	}

	client, err := NewFederatedClient(config)
	require.NoError(t, err)

	assert.Equal(t, "config-test-cluster", client.clusterName)
	assert.Equal(t, clientset, client.clientset)
	assert.Equal(t, dynamicClient, client.dynamicClient)
	assert.Equal(t, restConfig, client.restConfig)
}

// createTestFederatedClient creates a FederatedClient for testing
func createTestFederatedClient(t *testing.T, clusterName string) *FederatedClient {
	t.Helper()

	config := &FederatedClientConfig{
		ClusterName:   clusterName,
		Clientset:     fake.NewSimpleClientset(),
		DynamicClient: &fakedynamic.FakeDynamicClient{},
		RestConfig:    &rest.Config{Host: "https://test-api:6443"},
	}

	client, err := NewFederatedClient(config)
	require.NoError(t, err)
	return client
}

// Verify compile-time interface compliance
var (
	_ kubernetes.Interface = (*fake.Clientset)(nil)
	_ dynamic.Interface    = (*fakedynamic.FakeDynamicClient)(nil)
)
