// Package testdata provides mock implementations for testing the resource package.
package testdata

import (
	"context"
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// Compile-time interface compliance checks.
// These ensure the mocks always satisfy the interfaces they're meant to implement.
var (
	_ k8s.Client    = (*MockK8sClient)(nil)
	_ server.Logger = (*MockLogger)(nil)
)

// MockK8sClient implements k8s.Client interface for testing.
// It returns nil/empty values for all operations, allowing tests to verify
// that handler-level checks work correctly.
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
func (m *MockK8sClient) Get(_ context.Context, _, _, _, _, _ string) (runtime.Object, error) {
	return nil, nil
}

// List implements k8s.ResourceManager.
func (m *MockK8sClient) List(_ context.Context, _, _, _, _ string, _ k8s.ListOptions) (*k8s.PaginatedListResponse, error) {
	return &k8s.PaginatedListResponse{
		Items:      []runtime.Object{},
		TotalItems: 0,
	}, nil
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
