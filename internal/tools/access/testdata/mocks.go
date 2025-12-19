// Package testdata provides mock implementations for testing the access package.
package testdata

import (
	"context"
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// MockFederationManager implements federation.ClusterClientManager for testing.
type MockFederationManager struct {
	CheckAccessResult *federation.AccessCheckResult
	CheckAccessErr    error
}

// GetClient implements federation.ClusterClientManager.
func (m *MockFederationManager) GetClient(_ context.Context, _ string, _ *federation.UserInfo) (kubernetes.Interface, error) {
	return nil, nil
}

// GetDynamicClient implements federation.ClusterClientManager.
func (m *MockFederationManager) GetDynamicClient(_ context.Context, _ string, _ *federation.UserInfo) (dynamic.Interface, error) {
	return nil, nil
}

// GetRestConfig implements federation.ClusterClientManager.
func (m *MockFederationManager) GetRestConfig(_ context.Context, _ string, _ *federation.UserInfo) (*rest.Config, error) {
	return &rest.Config{Host: "https://mock-cluster:6443"}, nil
}

// ListClusters implements federation.ClusterClientManager.
func (m *MockFederationManager) ListClusters(_ context.Context, _ *federation.UserInfo) ([]federation.ClusterSummary, error) {
	return nil, nil
}

// GetClusterSummary implements federation.ClusterClientManager.
func (m *MockFederationManager) GetClusterSummary(_ context.Context, _ string, _ *federation.UserInfo) (*federation.ClusterSummary, error) {
	return nil, nil
}

// CheckAccess implements federation.ClusterClientManager.
func (m *MockFederationManager) CheckAccess(_ context.Context, _ string, _ *federation.UserInfo, _ *federation.AccessCheck) (*federation.AccessCheckResult, error) {
	return m.CheckAccessResult, m.CheckAccessErr
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
func (m *MockK8sClient) Get(_ context.Context, _, _, _, _, _ string) (*k8s.GetResponse, error) {
	return &k8s.GetResponse{
		Resource: nil,
		Meta:     nil,
	}, nil
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
func (m *MockK8sClient) Delete(_ context.Context, _, _, _, _, _ string) (*k8s.DeleteResponse, error) {
	return &k8s.DeleteResponse{
		Message: "deleted",
		Meta:    nil,
	}, nil
}

// Patch implements k8s.ResourceManager.
func (m *MockK8sClient) Patch(_ context.Context, _, _, _, _, _ string, _ types.PatchType, _ []byte) (*k8s.PatchResponse, error) {
	return &k8s.PatchResponse{
		Resource: nil,
		Meta:     nil,
	}, nil
}

// Scale implements k8s.ResourceManager.
func (m *MockK8sClient) Scale(_ context.Context, _, _, _, _, _ string, _ int32) (*k8s.ScaleResponse, error) {
	return &k8s.ScaleResponse{
		Message:  "scaled",
		Replicas: 0,
		Meta:     nil,
	}, nil
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
