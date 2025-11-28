package k8s

import (
	"context"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/portforward"
)

// Client defines the interface for Kubernetes operations.
// It supports multi-cluster operations by accepting kubecontext parameters
// and provides comprehensive functionality for all MCP tools.
type Client interface {
	// Context Management Operations
	ContextManager

	// Resource Management Operations
	ResourceManager

	// Pod Operations
	PodManager

	// Cluster Operations
	ClusterManager
}

// ClientFactory creates Kubernetes clients with custom authentication.
// This is used for creating per-user clients when OAuth passthrough is enabled.
type ClientFactory interface {
	// CreateBearerTokenClient creates a new Kubernetes client that uses the provided
	// bearer token for authentication. This is used for OAuth passthrough where
	// the user's Google OAuth access token is used to authenticate with Kubernetes.
	// The baseClient provides the cluster connection details (host, CA cert).
	CreateBearerTokenClient(bearerToken string) (Client, error)
}

// ContextManager handles Kubernetes context operations.
type ContextManager interface {
	// ListContexts returns all available Kubernetes contexts.
	ListContexts(ctx context.Context) ([]ContextInfo, error)

	// GetCurrentContext returns the currently active context.
	GetCurrentContext(ctx context.Context) (*ContextInfo, error)

	// SwitchContext changes the active Kubernetes context.
	SwitchContext(ctx context.Context, contextName string) error
}

// ResourceManager handles Kubernetes resource operations.
type ResourceManager interface {
	// Get retrieves a specific resource by name and namespace.
	Get(ctx context.Context, kubeContext, namespace, resourceType, name string) (runtime.Object, error)

	// List retrieves resources with pagination support.
	List(ctx context.Context, kubeContext, namespace, resourceType string, opts ListOptions) (*PaginatedListResponse, error)

	// Describe provides detailed information about a resource.
	Describe(ctx context.Context, kubeContext, namespace, resourceType, name string) (*ResourceDescription, error)

	// Create creates a new resource from the provided object.
	Create(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error)

	// Apply applies a resource configuration (create or update).
	Apply(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error)

	// Delete removes a resource by name and namespace.
	Delete(ctx context.Context, kubeContext, namespace, resourceType, name string) error

	// Patch updates specific fields of a resource.
	Patch(ctx context.Context, kubeContext, namespace, resourceType, name string, patchType types.PatchType, data []byte) (runtime.Object, error)

	// Scale changes the number of replicas for scalable resources.
	Scale(ctx context.Context, kubeContext, namespace, resourceType, name string, replicas int32) error
}

// PodManager handles pod-specific operations.
type PodManager interface {
	// GetLogs retrieves logs from a pod container.
	GetLogs(ctx context.Context, kubeContext, namespace, podName, containerName string, opts LogOptions) (io.ReadCloser, error)

	// Exec executes a command inside a pod container.
	Exec(ctx context.Context, kubeContext, namespace, podName, containerName string, command []string, opts ExecOptions) (*ExecResult, error)

	// PortForward sets up port forwarding to a pod.
	PortForward(ctx context.Context, kubeContext, namespace, podName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error)

	// PortForwardToService sets up port forwarding to the first available pod behind a service.
	PortForwardToService(ctx context.Context, kubeContext, namespace, serviceName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error)
}

// ClusterManager handles cluster-level operations.
type ClusterManager interface {
	// GetAPIResources returns available API resources with pagination support.
	GetAPIResources(ctx context.Context, kubeContext string, limit, offset int, apiGroup string, namespacedOnly bool, verbs []string) (*PaginatedAPIResourceResponse, error)

	// GetClusterHealth returns the health status of the cluster.
	GetClusterHealth(ctx context.Context, kubeContext string) (*ClusterHealth, error)
}

// ContextInfo represents information about a Kubernetes context.
type ContextInfo struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace"`
	Current   bool   `json:"current"`
}

// ListOptions provides configuration for list operations.
type ListOptions struct {
	LabelSelector string `json:"labelSelector,omitempty"`
	FieldSelector string `json:"fieldSelector,omitempty"`
	AllNamespaces bool   `json:"allNamespaces,omitempty"`

	// Pagination options
	Limit    int64  `json:"limit,omitempty"`    // Maximum number of items to return (0 = no limit)
	Continue string `json:"continue,omitempty"` // Continue token from previous request
}

// PaginatedListResponse contains a paginated list of resources with metadata
type PaginatedListResponse struct {
	Items           []runtime.Object `json:"items"`
	Continue        string           `json:"continue,omitempty"`        // Token for next page
	RemainingItems  *int64           `json:"remainingItems,omitempty"`  // Estimated remaining items (if available)
	ResourceVersion string           `json:"resourceVersion,omitempty"` // Resource version for consistency
	TotalItems      int              `json:"totalItems"`                // Number of items in this response
}

// ResourceDescription contains detailed information about a resource.
type ResourceDescription struct {
	Resource runtime.Object         `json:"resource"`
	Events   []corev1.Event         `json:"events,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// LogOptions configures log retrieval.
type LogOptions struct {
	Follow     bool       `json:"follow,omitempty"`
	Previous   bool       `json:"previous,omitempty"`
	Timestamps bool       `json:"timestamps,omitempty"`
	SinceTime  *time.Time `json:"sinceTime,omitempty"`
	TailLines  *int64     `json:"tailLines,omitempty"`

	// Pagination options for log output
	SinceLines *int64 `json:"sinceLines,omitempty"` // Skip this many lines from the beginning
	MaxLines   *int64 `json:"maxLines,omitempty"`   // Maximum number of lines to return
}

// ExecOptions configures command execution in pods.
type ExecOptions struct {
	Stdin  io.Reader `json:"-"`
	Stdout io.Writer `json:"-"`
	Stderr io.Writer `json:"-"`
	TTY    bool      `json:"tty,omitempty"`
}

// ExecResult contains the result of command execution.
type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// PortForwardOptions configures port forwarding.
type PortForwardOptions struct {
	Stdout io.Writer `json:"-"`
	Stderr io.Writer `json:"-"`
}

// PortForwardSession represents an active port forwarding session.
type PortForwardSession struct {
	LocalPorts  []int                      `json:"localPorts"`
	RemotePorts []int                      `json:"remotePorts"`
	StopChan    chan struct{}              `json:"-"`
	ReadyChan   chan struct{}              `json:"-"`
	Forwarder   *portforward.PortForwarder `json:"-"`
}

// APIResourceInfo represents information about an API resource.
type APIResourceInfo struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Verbs        []string `json:"verbs"`
	Group        string   `json:"group"`
	Version      string   `json:"version"`
}

// PaginatedAPIResourceResponse contains a paginated list of API resources
type PaginatedAPIResourceResponse struct {
	Items      []APIResourceInfo `json:"items"`
	TotalItems int               `json:"totalItems"` // Number of items in this response
	TotalCount int               `json:"totalCount"` // Total number of items available
	HasMore    bool              `json:"hasMore"`    // Whether there are more items available
	NextOffset int               `json:"nextOffset"` // Offset for next page (if hasMore is true)
}

// ClusterHealth represents the health status of a Kubernetes cluster.
type ClusterHealth struct {
	Status     string            `json:"status"`
	Components []ComponentHealth `json:"components"`
	Nodes      []NodeHealth      `json:"nodes"`
}

// ComponentHealth represents the health of a cluster component.
type ComponentHealth struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// NodeHealth represents the health of a cluster node.
type NodeHealth struct {
	Name       string                 `json:"name"`
	Ready      bool                   `json:"ready"`
	Conditions []corev1.NodeCondition `json:"conditions"`
}
