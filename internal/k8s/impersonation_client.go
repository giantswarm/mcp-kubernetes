package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// InClusterImpersonationFactory creates clients that authenticate as the
// server's in-cluster ServiceAccount and add Impersonate-* headers. The
// kube-apiserver enforces RBAC against the impersonated subject and records
// it in the audit log.
type InClusterImpersonationFactory struct {
	clusterHost          string
	namespace            string
	qpsLimit             float32
	burstLimit           int
	timeout              time.Duration
	nonDestructiveMode   bool
	dryRun               bool
	allowedOperations    []string
	restrictedNamespaces []string
	logger               Logger
}

// NewInClusterImpersonationFactory builds the factory from in-cluster config.
// Fails if not running inside a Kubernetes cluster.
func NewInClusterImpersonationFactory(config *ClientConfig) (*InClusterImpersonationFactory, error) {
	if config == nil {
		return nil, fmt.Errorf("client configuration is required")
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	qpsLimit := config.QPSLimit
	if qpsLimit == 0 {
		qpsLimit = DefaultQPSLimit
	}
	burstLimit := config.BurstLimit
	if burstLimit == 0 {
		burstLimit = DefaultBurstLimit
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout * time.Second
	}

	namespace := DefaultNamespace
	if data, err := os.ReadFile(DefaultNamespacePath); err == nil {
		namespace = string(data)
	}

	return &InClusterImpersonationFactory{
		clusterHost:          inClusterConfig.Host,
		namespace:            namespace,
		qpsLimit:             qpsLimit,
		burstLimit:           burstLimit,
		timeout:              timeout,
		nonDestructiveMode:   config.NonDestructiveMode,
		dryRun:               config.DryRun,
		allowedOperations:    config.AllowedOperations,
		restrictedNamespaces: config.RestrictedNamespaces,
		logger:               config.Logger,
	}, nil
}

// CreateImpersonationClient returns a Client authenticated as the server's
// in-cluster SA with Impersonate-* headers applied for the given identity.
func (f *InClusterImpersonationFactory) CreateImpersonationClient(identity ImpersonationIdentity) (Client, error) {
	if identity.UserName == "" {
		return nil, fmt.Errorf("impersonation identity requires a non-empty UserName")
	}

	cfg := &rest.Config{
		Host: f.clusterHost,
		TLSClientConfig: rest.TLSClientConfig{
			CAFile: DefaultCACertPath,
		},
		// BearerTokenFile causes the transport to read and refresh the SA token
		// from disk on each request — no manual rotation required.
		BearerTokenFile: DefaultTokenPath,
		QPS:             f.qpsLimit,
		Burst:           f.burstLimit,
		Timeout:         f.timeout,
		Impersonate: rest.ImpersonationConfig{
			UserName: identity.UserName,
			Groups:   identity.Groups,
			Extra:    identity.Extra,
		},
	}

	return &impersonationClient{
		restConfig:           cfg,
		namespace:            f.namespace,
		nonDestructiveMode:   f.nonDestructiveMode,
		dryRun:               f.dryRun,
		allowedOperations:    f.allowedOperations,
		restrictedNamespaces: f.restrictedNamespaces,
		logger:               f.logger,
	}, nil
}

// impersonationClient implements Client using the server's in-cluster SA with
// Impersonate-* headers derived from ImpersonationIdentity.
type impersonationClient struct {
	restConfig *rest.Config
	namespace  string

	clientsetLazy       lazyValue[kubernetes.Interface]
	dynamicClientLazy   lazyValue[dynamic.Interface]
	discoveryClientLazy lazyValue[discovery.DiscoveryInterface]

	nonDestructiveMode   bool
	dryRun               bool
	allowedOperations    []string
	restrictedNamespaces []string
	logger               Logger
}

func (c *impersonationClient) getClientset() (kubernetes.Interface, error) {
	return c.clientsetLazy.Get(func() (kubernetes.Interface, error) {
		cs, err := kubernetes.NewForConfig(c.restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientset: %w", err)
		}
		return cs, nil
	})
}

func (c *impersonationClient) getDynamicClient() (dynamic.Interface, error) {
	return c.dynamicClientLazy.Get(func() (dynamic.Interface, error) {
		dc, err := dynamic.NewForConfig(c.restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create dynamic client: %w", err)
		}
		return dc, nil
	})
}

func (c *impersonationClient) getDiscoveryClient() (discovery.DiscoveryInterface, error) {
	return c.discoveryClientLazy.Get(func() (discovery.DiscoveryInterface, error) {
		dc, err := discovery.NewDiscoveryClientForConfig(c.restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create discovery client: %w", err)
		}
		return dc, nil
	})
}

func (c *impersonationClient) isOperationAllowed(operation string) error {
	if len(c.allowedOperations) > 0 {
		if !slices.Contains(c.allowedOperations, operation) {
			return fmt.Errorf("operation %q is not allowed", operation)
		}
		return nil
	}
	if c.nonDestructiveMode {
		if slices.Contains([]string{"delete", "patch", "scale", "create", "apply"}, operation) && !c.dryRun {
			return fmt.Errorf("destructive operation %q is not allowed in non-destructive mode", operation)
		}
	}
	return nil
}

func (c *impersonationClient) isNamespaceRestricted(namespace string) error {
	if slices.Contains(c.restrictedNamespaces, namespace) {
		return fmt.Errorf("access to namespace %q is restricted", namespace)
	}
	return nil
}

func (c *impersonationClient) getInClusterNamespace() string {
	return c.namespace
}

// ========== ContextManager ==========

func (c *impersonationClient) ListContexts(_ context.Context) ([]ContextInfo, error) {
	return []ContextInfo{{
		Name:      InClusterContext,
		Cluster:   InClusterContext,
		User:      c.restConfig.Impersonate.UserName,
		Namespace: c.getInClusterNamespace(),
		Current:   true,
	}}, nil
}

func (c *impersonationClient) GetCurrentContext(_ context.Context) (*ContextInfo, error) {
	return &ContextInfo{
		Name:      InClusterContext,
		Cluster:   InClusterContext,
		User:      c.restConfig.Impersonate.UserName,
		Namespace: c.getInClusterNamespace(),
		Current:   true,
	}, nil
}

func (c *impersonationClient) SwitchContext(_ context.Context, contextName string) error {
	if contextName != InClusterContext {
		return fmt.Errorf("impersonation client only supports in-cluster context")
	}
	return nil
}

// ========== ResourceManager ==========

func (c *impersonationClient) Get(ctx context.Context, _, namespace, resourceType, apiGroup, name string) (*GetResponse, error) {
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return getResource(ctx, dynamicClient, discoveryClient, namespace, resourceType, apiGroup, name)
}

func (c *impersonationClient) List(ctx context.Context, _, namespace, resourceType, apiGroup string, opts ListOptions) (*PaginatedListResponse, error) {
	if namespace != "" && !opts.AllNamespaces {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return listResources(ctx, dynamicClient, discoveryClient, namespace, resourceType, apiGroup, opts)
}

func (c *impersonationClient) Describe(ctx context.Context, _, namespace, resourceType, apiGroup, name string) (*ResourceDescription, error) {
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return describeResource(ctx, dynamicClient, discoveryClient, clientset, namespace, resourceType, apiGroup, name)
}

func (c *impersonationClient) Create(ctx context.Context, _, namespace string, obj runtime.Object) (runtime.Object, error) {
	if err := c.isOperationAllowed("create"); err != nil {
		return nil, err
	}
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return createResource(ctx, dynamicClient, discoveryClient, namespace, obj, c.dryRun)
}

func (c *impersonationClient) Apply(ctx context.Context, _, namespace string, obj runtime.Object) (runtime.Object, error) {
	if err := c.isOperationAllowed("apply"); err != nil {
		return nil, err
	}
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return applyResource(ctx, dynamicClient, discoveryClient, namespace, obj, c.dryRun)
}

func (c *impersonationClient) Delete(ctx context.Context, _, namespace, resourceType, apiGroup, name string) (*DeleteResponse, error) {
	if err := c.isOperationAllowed("delete"); err != nil {
		return nil, err
	}
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return deleteResource(ctx, dynamicClient, discoveryClient, namespace, resourceType, apiGroup, name, c.dryRun)
}

func (c *impersonationClient) Patch(ctx context.Context, _, namespace, resourceType, apiGroup, name string, patchType types.PatchType, data []byte) (*PatchResponse, error) {
	if err := c.isOperationAllowed("patch"); err != nil {
		return nil, err
	}
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return patchResource(ctx, dynamicClient, discoveryClient, namespace, resourceType, apiGroup, name, patchType, data, c.dryRun)
}

func (c *impersonationClient) Scale(ctx context.Context, _, namespace, resourceType, apiGroup, name string, replicas int32) (*ScaleResponse, error) {
	if err := c.isOperationAllowed("scale"); err != nil {
		return nil, err
	}
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return scaleResource(ctx, dynamicClient, discoveryClient, namespace, resourceType, apiGroup, name, replicas, c.dryRun)
}

// ========== PodManager ==========

func (c *impersonationClient) GetLogs(ctx context.Context, _, namespace, podName, containerName string, opts LogOptions) (io.ReadCloser, error) {
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return getLogs(ctx, clientset, namespace, podName, containerName, opts)
}

func (c *impersonationClient) Exec(ctx context.Context, _, namespace, podName, containerName string, command []string, opts ExecOptions) (*ExecResult, error) {
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return execInPod(ctx, clientset, c.restConfig, namespace, podName, containerName, command, opts)
}

func (c *impersonationClient) PortForward(ctx context.Context, _, namespace, podName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return portForwardToPod(ctx, clientset, c.restConfig, namespace, podName, ports, opts)
}

func (c *impersonationClient) PortForwardToService(ctx context.Context, _, namespace, serviceName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}
	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return portForwardToService(ctx, clientset, c.restConfig, namespace, serviceName, ports, opts)
}

// ========== ClusterManager ==========

func (c *impersonationClient) GetAPIResources(ctx context.Context, _ string, limit, offset int, apiGroup string, namespacedOnly bool, verbs []string) (*PaginatedAPIResourceResponse, error) {
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return getAPIResources(ctx, discoveryClient, limit, offset, apiGroup, namespacedOnly, verbs)
}

func (c *impersonationClient) GetClusterHealth(ctx context.Context, _ string) (*ClusterHealth, error) {
	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return getClusterHealth(ctx, clientset, discoveryClient)
}
