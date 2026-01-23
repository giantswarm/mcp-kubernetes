package federation

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

// Test constants for use across federation package tests.
const (
	// testToken is a mock OAuth/SSO token for testing purposes.
	testToken = "test-token"

	// testValidKubeconfig is a minimal valid kubeconfig for testing.
	// Uses insecure-skip-tls-verify to avoid certificate validation issues in tests.
	testValidKubeconfig = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster.example.com:6443
    insecure-skip-tls-verify: true
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
    token: test-token-for-testing
`

	// testInvalidKubeconfig is an invalid kubeconfig for testing error handling.
	testInvalidKubeconfig = `
not-valid-yaml: [[[
`
)

// newTestLogger creates a logger for tests that only outputs errors.
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// createTestFakeDynamicClient creates a fake dynamic client with CAPI cluster GVR registered.
// This is required because the dynamic fake client needs explicit registration of list kinds.
func createTestFakeDynamicClient(scheme *runtime.Scheme, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	gvrToListKind := map[schema.GroupVersionResource]string{
		CAPIClusterGVR: "ClusterList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)
}

// createTestCAPICluster creates an unstructured CAPI Cluster resource for testing.
func createTestCAPICluster(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"paused": false,
			},
			"status": map[string]interface{}{
				"phase": "Provisioned",
			},
		},
	}
}

// createTestKubeconfigSecret creates a Secret with kubeconfig data for testing.
func createTestKubeconfigSecret(clusterName, namespace, key, kubeconfigData string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + CAPISecretSuffix,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			key: []byte(kubeconfigData),
		},
	}
}

// setupTestManager creates a Manager with fake clients for testing.
// It accepts optional CAPI cluster resources and kubeconfig secrets.
// The manager is automatically closed when the test completes via t.Cleanup(),
// so callers don't need to manually defer Close().
func setupTestManager(t *testing.T, clusters []*unstructured.Unstructured, secrets []*corev1.Secret) *Manager {
	t.Helper()

	logger := newTestLogger()

	// Create fake Kubernetes client with secrets
	fakeClient := fake.NewClientset()
	for _, secret := range secrets {
		_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	// Create fake dynamic client with custom list kinds
	scheme := runtime.NewScheme()

	// Convert clusters to runtime.Objects for the fake client
	var objects []runtime.Object
	for _, cluster := range clusters {
		objects = append(objects, cluster)
	}

	fakeDynamic := createTestFakeDynamicClient(scheme, objects...)

	// Use StaticClientProvider for testing - all users get the same clients
	clientProvider := &StaticClientProvider{
		Clientset:     fakeClient,
		DynamicClient: fakeDynamic,
		RestConfig:    nil,
	}

	manager, err := NewManager(clientProvider, WithManagerLogger(logger))
	require.NoError(t, err)

	// Automatically cleanup when test completes (even on failure)
	t.Cleanup(func() {
		_ = manager.Close()
	})

	return manager
}

// testUser creates a test UserInfo for testing.
func testUser() *UserInfo {
	return &UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
}
