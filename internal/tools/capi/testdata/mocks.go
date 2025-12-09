// Package testdata provides mock implementations for testing the capi package.
package testdata

import (
	"context"
	"io"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// MockFederationManager implements federation.ClusterClientManager for testing.
type MockFederationManager struct {
	Clusters        []federation.ClusterSummary
	ClusterDetails  map[string]*federation.ClusterSummary
	ListClustersErr error
	GetClusterErr   error
	CheckAccessErr  error
}

// Ensure MockFederationManager implements ClusterClientManager
var _ federation.ClusterClientManager = (*MockFederationManager)(nil)

// GetClient implements federation.ClusterClientManager.
func (m *MockFederationManager) GetClient(_ context.Context, _ string, _ *federation.UserInfo) (kubernetes.Interface, error) {
	return nil, nil
}

// GetDynamicClient implements federation.ClusterClientManager.
func (m *MockFederationManager) GetDynamicClient(_ context.Context, _ string, _ *federation.UserInfo) (dynamic.Interface, error) {
	return nil, nil
}

// ListClusters implements federation.ClusterClientManager.
func (m *MockFederationManager) ListClusters(_ context.Context, _ *federation.UserInfo) ([]federation.ClusterSummary, error) {
	if m.ListClustersErr != nil {
		return nil, m.ListClustersErr
	}
	return m.Clusters, nil
}

// GetClusterSummary implements federation.ClusterClientManager.
func (m *MockFederationManager) GetClusterSummary(_ context.Context, clusterName string, _ *federation.UserInfo) (*federation.ClusterSummary, error) {
	if m.GetClusterErr != nil {
		return nil, m.GetClusterErr
	}
	if m.ClusterDetails != nil {
		if cluster, ok := m.ClusterDetails[clusterName]; ok {
			return cluster, nil
		}
	}
	return nil, &federation.ClusterNotFoundError{ClusterName: clusterName, Reason: "not found"}
}

// CheckAccess implements federation.ClusterClientManager.
func (m *MockFederationManager) CheckAccess(_ context.Context, _ string, _ *federation.UserInfo, _ *federation.AccessCheck) (*federation.AccessCheckResult, error) {
	return nil, m.CheckAccessErr
}

// Close implements federation.ClusterClientManager.
func (m *MockFederationManager) Close() error {
	return nil
}

// Stats implements federation.ClusterClientManager.
func (m *MockFederationManager) Stats() federation.ManagerStats {
	return federation.ManagerStats{}
}

// MockK8sClient implements k8s.Client interface for testing.
type MockK8sClient struct{}

// Ensure MockK8sClient implements k8s.Client
var _ k8s.Client = (*MockK8sClient)(nil)

// ListContexts implements k8s.ContextManager.
func (m *MockK8sClient) ListContexts(_ context.Context) ([]k8s.ContextInfo, error) {
	return nil, nil
}

// GetCurrentContext implements k8s.ContextManager.
func (m *MockK8sClient) GetCurrentContext(_ context.Context) (*k8s.ContextInfo, error) {
	return &k8s.ContextInfo{Name: "test"}, nil
}

// SwitchContext implements k8s.ContextManager.
func (m *MockK8sClient) SwitchContext(_ context.Context, _ string) error {
	return nil
}

// Get implements k8s.ResourceManager.
func (m *MockK8sClient) Get(_ context.Context, _, _, _, _, _ string) (runtime.Object, error) {
	return nil, nil
}

// List implements k8s.ResourceManager.
func (m *MockK8sClient) List(_ context.Context, _, _, _, _ string, _ k8s.ListOptions) (*k8s.PaginatedListResponse, error) {
	return nil, nil
}

// Describe implements k8s.ResourceManager.
func (m *MockK8sClient) Describe(_ context.Context, _, _, _, _, _ string) (*k8s.ResourceDescription, error) {
	return nil, nil
}

// Create implements k8s.ResourceManager.
func (m *MockK8sClient) Create(_ context.Context, _, _ string, _ runtime.Object) (runtime.Object, error) {
	return nil, nil
}

// Apply implements k8s.ResourceManager.
func (m *MockK8sClient) Apply(_ context.Context, _, _ string, _ runtime.Object) (runtime.Object, error) {
	return nil, nil
}

// Delete implements k8s.ResourceManager.
func (m *MockK8sClient) Delete(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

// Patch implements k8s.ResourceManager.
func (m *MockK8sClient) Patch(_ context.Context, _, _, _, _, _ string, _ types.PatchType, _ []byte) (runtime.Object, error) {
	return nil, nil
}

// Scale implements k8s.ResourceManager.
func (m *MockK8sClient) Scale(_ context.Context, _, _, _, _, _ string, _ int32) error {
	return nil
}

// GetLogs implements k8s.PodManager.
func (m *MockK8sClient) GetLogs(_ context.Context, _, _, _, _ string, _ k8s.LogOptions) (io.ReadCloser, error) {
	return nil, nil
}

// Exec implements k8s.PodManager.
func (m *MockK8sClient) Exec(_ context.Context, _, _, _, _ string, _ []string, _ k8s.ExecOptions) (*k8s.ExecResult, error) {
	return nil, nil
}

// PortForward implements k8s.PodManager.
func (m *MockK8sClient) PortForward(_ context.Context, _, _, _ string, _ []string, _ k8s.PortForwardOptions) (*k8s.PortForwardSession, error) {
	return nil, nil
}

// PortForwardToService implements k8s.PodManager.
func (m *MockK8sClient) PortForwardToService(_ context.Context, _, _, _ string, _ []string, _ k8s.PortForwardOptions) (*k8s.PortForwardSession, error) {
	return nil, nil
}

// GetAPIResources implements k8s.ClusterManager.
func (m *MockK8sClient) GetAPIResources(_ context.Context, _ string, _, _ int, _ string, _ bool, _ []string) (*k8s.PaginatedAPIResourceResponse, error) {
	return nil, nil
}

// GetClusterHealth implements k8s.ClusterManager.
func (m *MockK8sClient) GetClusterHealth(_ context.Context, _ string) (*k8s.ClusterHealth, error) {
	return nil, nil
}

// MockLogger implements server.Logger for testing.
type MockLogger struct{}

// Ensure MockLogger implements server.Logger
var _ server.Logger = (*MockLogger)(nil)

// Info implements server.Logger.
func (m *MockLogger) Info(_ string, _ ...interface{}) {}

// Debug implements server.Logger.
func (m *MockLogger) Debug(_ string, _ ...interface{}) {}

// Warn implements server.Logger.
func (m *MockLogger) Warn(_ string, _ ...interface{}) {}

// Error implements server.Logger.
func (m *MockLogger) Error(_ string, _ ...interface{}) {}

// With implements server.Logger.
func (m *MockLogger) With(_ ...interface{}) server.Logger {
	return m
}

// CreateTestClusters creates a set of test clusters for testing.
func CreateTestClusters() []federation.ClusterSummary {
	now := time.Now()
	return []federation.ClusterSummary{
		{
			Name:                "prod-wc-01",
			Namespace:           "org-acme",
			Provider:            "aws",
			Release:             "20.1.0",
			KubernetesVersion:   "1.28.5",
			Status:              "Provisioned",
			Ready:               true,
			ControlPlaneReady:   true,
			InfrastructureReady: true,
			NodeCount:           15,
			CreatedAt:           now.Add(-45 * 24 * time.Hour),
			Labels: map[string]string{
				"giantswarm.io/organization": "acme",
				"environment":                "production",
			},
		},
		{
			Name:                "staging-wc",
			Namespace:           "org-acme",
			Provider:            "aws",
			Release:             "20.2.0",
			KubernetesVersion:   "1.29.0",
			Status:              "Provisioned",
			Ready:               true,
			ControlPlaneReady:   true,
			InfrastructureReady: true,
			NodeCount:           5,
			CreatedAt:           now.Add(-12 * 24 * time.Hour),
			Labels: map[string]string{
				"giantswarm.io/organization": "acme",
				"environment":                "staging",
			},
		},
		{
			Name:                "dev-cluster",
			Namespace:           "org-beta",
			Provider:            "azure",
			Release:             "19.3.0",
			KubernetesVersion:   "1.27.5",
			Status:              "Provisioning",
			Ready:               false,
			ControlPlaneReady:   true,
			InfrastructureReady: false,
			NodeCount:           0,
			CreatedAt:           now.Add(-1 * time.Hour),
			Labels: map[string]string{
				"giantswarm.io/organization": "beta",
				"environment":                "development",
			},
		},
	}
}

// CreateTestClusterDetailsMap creates a map of test cluster details.
func CreateTestClusterDetailsMap() map[string]*federation.ClusterSummary {
	clusters := CreateTestClusters()
	result := make(map[string]*federation.ClusterSummary)
	for i := range clusters {
		result[clusters[i].Name] = &clusters[i]
	}
	return result
}
