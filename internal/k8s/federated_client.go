// Package k8s provides Kubernetes client implementations.
package k8s

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// FederatedClient implements the Client interface for federated multi-cluster operations.
// It wraps kubernetes.Interface and dynamic.Interface from the federation manager
// to provide a consistent k8s.Client interface for remote workload clusters.
//
// Unlike kubernetesClient, FederatedClient operates on a single target cluster
// and does not support context switching (all kubeContext parameters are ignored).
//
// # Thread Safety
//
// FederatedClient is safe for concurrent use. All underlying clients are
// thread-safe, and the struct contains no mutable state.
//
// # Security
//
// The underlying clients are pre-configured with user impersonation headers,
// ensuring all operations are performed under the authenticated user's identity.
type FederatedClient struct {
	// clusterName is the name of the target cluster
	clusterName string

	// clientset is the Kubernetes clientset for the target cluster
	clientset kubernetes.Interface

	// dynamicClient is the dynamic client for the target cluster
	dynamicClient dynamic.Interface

	// restConfig is the REST configuration for the target cluster
	restConfig *rest.Config

	// discoveryClient is derived from clientset for resource type resolution
	discoveryClient discovery.DiscoveryInterface
}

// FederatedClientConfig holds the configuration for creating a FederatedClient.
type FederatedClientConfig struct {
	// ClusterName is the name of the target cluster (for logging/debugging)
	ClusterName string

	// Clientset is the Kubernetes clientset from the federation manager
	Clientset kubernetes.Interface

	// DynamicClient is the dynamic client from the federation manager
	DynamicClient dynamic.Interface

	// RestConfig is the REST configuration from the federation manager
	RestConfig *rest.Config
}

// NewFederatedClient creates a new FederatedClient from federation manager clients.
// All required fields must be provided or an error is returned.
func NewFederatedClient(config *FederatedClientConfig) (*FederatedClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.Clientset == nil {
		return nil, fmt.Errorf("clientset is required")
	}
	if config.DynamicClient == nil {
		return nil, fmt.Errorf("dynamic client is required")
	}
	if config.RestConfig == nil {
		return nil, fmt.Errorf("rest config is required")
	}

	return &FederatedClient{
		clusterName:     config.ClusterName,
		clientset:       config.Clientset,
		dynamicClient:   config.DynamicClient,
		restConfig:      config.RestConfig,
		discoveryClient: config.Clientset.Discovery(),
	}, nil
}

// ClusterName returns the name of the target cluster.
func (c *FederatedClient) ClusterName() string {
	return c.clusterName
}

// ContextManager implementation - context operations are not applicable for federated clients

// ListContexts returns a single context representing this federated cluster.
// Context switching is not supported for federated clients.
func (c *FederatedClient) ListContexts(_ context.Context) ([]ContextInfo, error) {
	// For federated clients, we return a single context representing the target cluster
	return []ContextInfo{
		{
			Name:    c.clusterName,
			Cluster: c.clusterName,
			Current: true,
		},
	}, nil
}

// GetCurrentContext returns the current context (always the target cluster).
func (c *FederatedClient) GetCurrentContext(_ context.Context) (*ContextInfo, error) {
	return &ContextInfo{
		Name:    c.clusterName,
		Cluster: c.clusterName,
		Current: true,
	}, nil
}

// SwitchContext is not supported for federated clients.
// The client is bound to a single target cluster.
func (c *FederatedClient) SwitchContext(_ context.Context, _ string) error {
	return fmt.Errorf("context switching is not supported for federated clients - client is bound to cluster %q", c.clusterName)
}

// ResourceManager implementation

// Get retrieves a specific resource by name and namespace.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Get(ctx context.Context, _, namespace, resourceType, apiGroup, name string) (*GetResponse, error) {
	c.logOperation("get", namespace, resourceType, name)
	return getResource(ctx, c.dynamicClient, c.discoveryClient, namespace, resourceType, apiGroup, name)
}

// List retrieves resources with pagination support.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) List(ctx context.Context, _, namespace, resourceType, apiGroup string, opts ListOptions) (*PaginatedListResponse, error) {
	c.logOperation("list", namespace, resourceType, "")
	return listResources(ctx, c.dynamicClient, c.discoveryClient, namespace, resourceType, apiGroup, opts)
}

// Describe provides detailed information about a resource.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Describe(ctx context.Context, _, namespace, resourceType, apiGroup, name string) (*ResourceDescription, error) {
	c.logOperation("describe", namespace, resourceType, name)
	return describeResource(ctx, c.dynamicClient, c.discoveryClient, c.clientset, namespace, resourceType, apiGroup, name)
}

// Create creates a new resource from the provided object.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Create(ctx context.Context, _, namespace string, obj runtime.Object) (runtime.Object, error) {
	c.logOperation("create", namespace, "resource", "")
	return createResource(ctx, c.dynamicClient, c.discoveryClient, namespace, obj, false)
}

// Apply applies a resource configuration (create or update).
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Apply(ctx context.Context, _, namespace string, obj runtime.Object) (runtime.Object, error) {
	c.logOperation("apply", namespace, "resource", "")
	return applyResource(ctx, c.dynamicClient, c.discoveryClient, namespace, obj, false)
}

// Delete removes a resource by name and namespace.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Delete(ctx context.Context, _, namespace, resourceType, apiGroup, name string) (*DeleteResponse, error) {
	c.logOperation("delete", namespace, resourceType, name)
	return deleteResource(ctx, c.dynamicClient, c.discoveryClient, namespace, resourceType, apiGroup, name, false)
}

// Patch updates specific fields of a resource.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Patch(ctx context.Context, _, namespace, resourceType, apiGroup, name string, patchType types.PatchType, data []byte) (*PatchResponse, error) {
	c.logOperation("patch", namespace, resourceType, name)
	return patchResource(ctx, c.dynamicClient, c.discoveryClient, namespace, resourceType, apiGroup, name, patchType, data, false)
}

// Scale changes the number of replicas for scalable resources.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Scale(ctx context.Context, _, namespace, resourceType, apiGroup, name string, replicas int32) (*ScaleResponse, error) {
	c.logOperation("scale", namespace, resourceType, name)
	return scaleResource(ctx, c.dynamicClient, c.discoveryClient, namespace, resourceType, apiGroup, name, replicas, false)
}

// PodManager implementation

// GetLogs retrieves logs from a pod container.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) GetLogs(ctx context.Context, _, namespace, podName, containerName string, opts LogOptions) (io.ReadCloser, error) {
	c.logOperation("logs", namespace, "pod", podName)
	return getLogs(ctx, c.clientset, namespace, podName, containerName, opts)
}

// Exec executes a command inside a pod container.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) Exec(ctx context.Context, _, namespace, podName, containerName string, command []string, opts ExecOptions) (*ExecResult, error) {
	c.logOperation("exec", namespace, "pod", podName)
	return execInPod(ctx, c.clientset, c.restConfig, namespace, podName, containerName, command, opts)
}

// PortForward sets up port forwarding to a pod.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) PortForward(ctx context.Context, _, namespace, podName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	c.logOperation("port-forward", namespace, "pod", podName)
	return portForwardToPod(ctx, c.clientset, c.restConfig, namespace, podName, ports, opts)
}

// PortForwardToService sets up port forwarding to the first available pod behind a service.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) PortForwardToService(ctx context.Context, _, namespace, serviceName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	c.logOperation("port-forward", namespace, "service", serviceName)
	return portForwardToService(ctx, c.clientset, c.restConfig, namespace, serviceName, ports, opts)
}

// ClusterManager implementation

// GetAPIResources returns available API resources with pagination support.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) GetAPIResources(ctx context.Context, _ string, limit, offset int, apiGroup string, namespacedOnly bool, verbs []string) (*PaginatedAPIResourceResponse, error) {
	c.logOperation("api-resources", "", "", "")
	return getAPIResources(ctx, c.discoveryClient, limit, offset, apiGroup, namespacedOnly, verbs)
}

// GetClusterHealth returns the health status of the cluster.
// The kubeContext parameter is ignored (federated clients operate on a single cluster).
func (c *FederatedClient) GetClusterHealth(ctx context.Context, _ string) (*ClusterHealth, error) {
	c.logOperation("cluster-health", "", "", "")
	return getClusterHealth(ctx, c.clientset, c.discoveryClient)
}

// logOperation logs a kubernetes operation for debugging.
func (c *FederatedClient) logOperation(operation, namespace, resource, name string) {
	slog.Debug("kubernetes operation (federated)",
		slog.String("operation", operation),
		slog.String("cluster", c.clusterName),
		slog.String("namespace", namespace),
		slog.String("resource", resource),
		slog.String("name", name))
}

// Ensure FederatedClient implements Client interface at compile time.
var _ Client = (*FederatedClient)(nil)
