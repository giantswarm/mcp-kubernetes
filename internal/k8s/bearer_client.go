package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// bearerTokenClient implements the Client interface using bearer token authentication.
// This is used for OAuth passthrough where the user's Google OAuth access token
// is used to authenticate with Kubernetes instead of a service account token.
type bearerTokenClient struct {
	// Bearer token for authentication
	bearerToken string

	// In-cluster configuration (host, CA cert)
	clusterHost string
	caCertFile  string

	// Client cache (lazily initialized)
	mu              sync.RWMutex
	clientset       kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	restConfig      *rest.Config

	// Resource type mappings (shared with base client)
	builtinResources map[string]schema.GroupVersionResource

	// Safety and performance settings (inherited from factory)
	nonDestructiveMode   bool
	dryRun               bool
	allowedOperations    []string
	restrictedNamespaces []string
	qpsLimit             float32
	burstLimit           int
	timeout              time.Duration

	// Debug settings
	debugMode bool
	logger    Logger
}

// BearerTokenClientFactory creates bearer token clients from in-cluster configuration.
// It implements the ClientFactory interface.
type BearerTokenClientFactory struct {
	// In-cluster configuration
	clusterHost string
	caCertFile  string

	// Safety and performance settings to pass to created clients
	nonDestructiveMode   bool
	dryRun               bool
	allowedOperations    []string
	restrictedNamespaces []string
	qpsLimit             float32
	burstLimit           int
	timeout              time.Duration
	debugMode            bool
	logger               Logger

	// Shared builtin resources mapping
	builtinResources map[string]schema.GroupVersionResource
}

// NewBearerTokenClientFactory creates a new factory for bearer token clients.
// It reads the in-cluster configuration to get the cluster host and CA cert.
func NewBearerTokenClientFactory(config *ClientConfig) (*BearerTokenClientFactory, error) {
	if config == nil {
		return nil, fmt.Errorf("client configuration is required")
	}

	// Get in-cluster configuration for host and CA cert
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	// Set defaults
	qpsLimit := config.QPSLimit
	if qpsLimit == 0 {
		qpsLimit = 20.0
	}
	burstLimit := config.BurstLimit
	if burstLimit == 0 {
		burstLimit = 30
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Initialize builtin resources mapping
	builtinResources := initBuiltinResources()

	return &BearerTokenClientFactory{
		clusterHost:          inClusterConfig.Host,
		caCertFile:           "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		nonDestructiveMode:   config.NonDestructiveMode,
		dryRun:               config.DryRun,
		allowedOperations:    config.AllowedOperations,
		restrictedNamespaces: config.RestrictedNamespaces,
		qpsLimit:             qpsLimit,
		burstLimit:           burstLimit,
		timeout:              timeout,
		debugMode:            config.DebugMode,
		logger:               config.Logger,
		builtinResources:     builtinResources,
	}, nil
}

// CreateBearerTokenClient creates a new Kubernetes client that uses the provided
// bearer token for authentication.
func (f *BearerTokenClientFactory) CreateBearerTokenClient(bearerToken string) (Client, error) {
	if bearerToken == "" {
		return nil, fmt.Errorf("bearer token is required")
	}

	return &bearerTokenClient{
		bearerToken:          bearerToken,
		clusterHost:          f.clusterHost,
		caCertFile:           f.caCertFile,
		builtinResources:     f.builtinResources,
		nonDestructiveMode:   f.nonDestructiveMode,
		dryRun:               f.dryRun,
		allowedOperations:    f.allowedOperations,
		restrictedNamespaces: f.restrictedNamespaces,
		qpsLimit:             f.qpsLimit,
		burstLimit:           f.burstLimit,
		timeout:              f.timeout,
		debugMode:            f.debugMode,
		logger:               f.logger,
	}, nil
}

// initBuiltinResources initializes the builtin resources mapping.
// This is shared between the main client and bearer token clients.
func initBuiltinResources() map[string]schema.GroupVersionResource {
	return map[string]schema.GroupVersionResource{
		// Core/v1 resources
		"pods":                   {Group: "", Version: "v1", Resource: "pods"},
		"pod":                    {Group: "", Version: "v1", Resource: "pods"},
		"services":               {Group: "", Version: "v1", Resource: "services"},
		"service":                {Group: "", Version: "v1", Resource: "services"},
		"svc":                    {Group: "", Version: "v1", Resource: "services"},
		"nodes":                  {Group: "", Version: "v1", Resource: "nodes"},
		"node":                   {Group: "", Version: "v1", Resource: "nodes"},
		"namespaces":             {Group: "", Version: "v1", Resource: "namespaces"},
		"namespace":              {Group: "", Version: "v1", Resource: "namespaces"},
		"ns":                     {Group: "", Version: "v1", Resource: "namespaces"},
		"configmaps":             {Group: "", Version: "v1", Resource: "configmaps"},
		"configmap":              {Group: "", Version: "v1", Resource: "configmaps"},
		"cm":                     {Group: "", Version: "v1", Resource: "configmaps"},
		"secrets":                {Group: "", Version: "v1", Resource: "secrets"},
		"secret":                 {Group: "", Version: "v1", Resource: "secrets"},
		"persistentvolumes":      {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"persistentvolume":       {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"pv":                     {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"persistentvolumeclaims": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"persistentvolumeclaim":  {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"pvc":                    {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},

		// Apps/v1 resources
		"deployments":  {Group: "apps", Version: "v1", Resource: "deployments"},
		"deployment":   {Group: "apps", Version: "v1", Resource: "deployments"},
		"deploy":       {Group: "apps", Version: "v1", Resource: "deployments"},
		"replicasets":  {Group: "apps", Version: "v1", Resource: "replicasets"},
		"replicaset":   {Group: "apps", Version: "v1", Resource: "replicasets"},
		"rs":           {Group: "apps", Version: "v1", Resource: "replicasets"},
		"daemonsets":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"daemonset":    {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"ds":           {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"statefulsets": {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"statefulset":  {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"sts":          {Group: "apps", Version: "v1", Resource: "statefulsets"},

		// Batch resources
		"jobs":     {Group: "batch", Version: "v1", Resource: "jobs"},
		"job":      {Group: "batch", Version: "v1", Resource: "jobs"},
		"cronjobs": {Group: "batch", Version: "v1", Resource: "cronjobs"},
		"cronjob":  {Group: "batch", Version: "v1", Resource: "cronjobs"},
		"cj":       {Group: "batch", Version: "v1", Resource: "cronjobs"},

		// Networking resources
		"ingresses": {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		"ingress":   {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		"ing":       {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},

		// RBAC resources
		"roles":               {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		"role":                {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		"rolebindings":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		"rolebinding":         {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		"clusterroles":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		"clusterrole":         {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		"clusterrolebindings": {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
		"clusterrolebinding":  {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
		"serviceaccounts":     {Group: "", Version: "v1", Resource: "serviceaccounts"},
		"serviceaccount":      {Group: "", Version: "v1", Resource: "serviceaccounts"},
		"sa":                  {Group: "", Version: "v1", Resource: "serviceaccounts"},
	}
}

// getRestConfig returns the REST config with bearer token authentication.
func (c *bearerTokenClient) getRestConfig() (*rest.Config, error) {
	c.mu.RLock()
	if c.restConfig != nil {
		config := c.restConfig
		c.mu.RUnlock()
		return config, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.restConfig != nil {
		return c.restConfig, nil
	}

	// Create REST config with bearer token
	config := &rest.Config{
		Host:        c.clusterHost,
		BearerToken: c.bearerToken,
		TLSClientConfig: rest.TLSClientConfig{
			CAFile: c.caCertFile,
		},
		QPS:     c.qpsLimit,
		Burst:   c.burstLimit,
		Timeout: c.timeout,
	}

	c.restConfig = config
	return config, nil
}

// getClientset returns the Kubernetes clientset.
func (c *bearerTokenClient) getClientset() (kubernetes.Interface, error) {
	c.mu.RLock()
	if c.clientset != nil {
		cs := c.clientset
		c.mu.RUnlock()
		return cs, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.clientset != nil {
		return c.clientset, nil
	}

	config, err := c.getRestConfigUnsafe()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	c.clientset = clientset
	return clientset, nil
}

// getRestConfigUnsafe returns the REST config without locking.
// Caller must hold the write lock.
func (c *bearerTokenClient) getRestConfigUnsafe() (*rest.Config, error) {
	if c.restConfig != nil {
		return c.restConfig, nil
	}

	config := &rest.Config{
		Host:        c.clusterHost,
		BearerToken: c.bearerToken,
		TLSClientConfig: rest.TLSClientConfig{
			CAFile: c.caCertFile,
		},
		QPS:     c.qpsLimit,
		Burst:   c.burstLimit,
		Timeout: c.timeout,
	}

	c.restConfig = config
	return config, nil
}

// getDynamicClient returns the dynamic client.
func (c *bearerTokenClient) getDynamicClient() (dynamic.Interface, error) {
	c.mu.RLock()
	if c.dynamicClient != nil {
		dc := c.dynamicClient
		c.mu.RUnlock()
		return dc, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dynamicClient != nil {
		return c.dynamicClient, nil
	}

	config, err := c.getRestConfigUnsafe()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	c.dynamicClient = dynamicClient
	return dynamicClient, nil
}

// getDiscoveryClient returns the discovery client.
func (c *bearerTokenClient) getDiscoveryClient() (discovery.DiscoveryInterface, error) {
	c.mu.RLock()
	if c.discoveryClient != nil {
		dc := c.discoveryClient
		c.mu.RUnlock()
		return dc, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.discoveryClient != nil {
		return c.discoveryClient, nil
	}

	config, err := c.getRestConfigUnsafe()
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	c.discoveryClient = discoveryClient
	return discoveryClient, nil
}

// isOperationAllowed checks if an operation is allowed based on configuration.
func (c *bearerTokenClient) isOperationAllowed(operation string) error {
	if len(c.allowedOperations) > 0 {
		allowed := false
		for _, allowedOp := range c.allowedOperations {
			if allowedOp == operation {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("operation %q is not allowed", operation)
		}
	}

	if c.nonDestructiveMode {
		destructiveOps := []string{"delete", "patch", "scale", "create", "apply"}
		for _, destructiveOp := range destructiveOps {
			if destructiveOp == operation {
				if !c.dryRun {
					return fmt.Errorf("destructive operation %q is not allowed in non-destructive mode", operation)
				}
				break
			}
		}
	}

	return nil
}

// isNamespaceRestricted checks if a namespace is restricted.
func (c *bearerTokenClient) isNamespaceRestricted(namespace string) error {
	for _, restrictedNs := range c.restrictedNamespaces {
		if restrictedNs == namespace {
			return fmt.Errorf("access to namespace %q is restricted", namespace)
		}
	}
	return nil
}

// logOperation logs an operation for debugging.
func (c *bearerTokenClient) logOperation(operation, context, namespace, resource, name string) {
	if c.logger != nil {
		c.logger.Debug("kubernetes operation (bearer token)",
			"operation", operation,
			"context", context,
			"namespace", namespace,
			"resource", resource,
			"name", name,
		)
	}
}

// getInClusterNamespace reads the namespace from the service account namespace file.
func (c *bearerTokenClient) getInClusterNamespace() string {
	namespacePath := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	data, err := os.ReadFile(namespacePath)
	if err != nil {
		return "default"
	}
	return string(data)
}

// ========== ContextManager Implementation ==========

// ListContexts returns available contexts (only in-cluster for bearer token client).
func (c *bearerTokenClient) ListContexts(ctx context.Context) ([]ContextInfo, error) {
	c.logOperation("list-contexts", "", "", "", "")
	return []ContextInfo{
		{
			Name:      "in-cluster",
			Cluster:   "in-cluster",
			User:      "oauth-user",
			Namespace: c.getInClusterNamespace(),
			Current:   true,
		},
	}, nil
}

// GetCurrentContext returns the current context.
func (c *bearerTokenClient) GetCurrentContext(ctx context.Context) (*ContextInfo, error) {
	c.logOperation("get-current-context", "in-cluster", "", "", "")
	return &ContextInfo{
		Name:      "in-cluster",
		Cluster:   "in-cluster",
		User:      "oauth-user",
		Namespace: c.getInClusterNamespace(),
		Current:   true,
	}, nil
}

// SwitchContext is not supported for bearer token clients.
func (c *bearerTokenClient) SwitchContext(ctx context.Context, contextName string) error {
	c.logOperation("switch-context", contextName, "", "", "")
	if contextName != "in-cluster" {
		return fmt.Errorf("cannot switch context: bearer token client only supports in-cluster context")
	}
	return nil
}

// ========== ResourceManager Implementation ==========
// These methods delegate to the existing implementation in resources.go
// by using the internal clients created with bearer token authentication.

// Get retrieves a specific resource by name and namespace.
func (c *bearerTokenClient) Get(ctx context.Context, kubeContext, namespace, resourceType, name string) (runtime.Object, error) {
	c.logOperation("get", kubeContext, namespace, resourceType, name)

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

	// Use the shared resource operations from resources.go
	return getResource(ctx, dynamicClient, discoveryClient, c.builtinResources, namespace, resourceType, name)
}

// List retrieves resources with pagination support.
func (c *bearerTokenClient) List(ctx context.Context, kubeContext, namespace, resourceType string, opts ListOptions) (*PaginatedListResponse, error) {
	c.logOperation("list", kubeContext, namespace, resourceType, "")

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

	return listResources(ctx, dynamicClient, discoveryClient, c.builtinResources, namespace, resourceType, opts)
}

// Describe provides detailed information about a resource.
func (c *bearerTokenClient) Describe(ctx context.Context, kubeContext, namespace, resourceType, name string) (*ResourceDescription, error) {
	c.logOperation("describe", kubeContext, namespace, resourceType, name)

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

	return describeResource(ctx, dynamicClient, discoveryClient, clientset, c.builtinResources, namespace, resourceType, name)
}

// Create creates a new resource.
func (c *bearerTokenClient) Create(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error) {
	c.logOperation("create", kubeContext, namespace, "", "")

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

// Apply applies a resource configuration.
func (c *bearerTokenClient) Apply(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error) {
	c.logOperation("apply", kubeContext, namespace, "", "")

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

// Delete removes a resource.
func (c *bearerTokenClient) Delete(ctx context.Context, kubeContext, namespace, resourceType, name string) error {
	c.logOperation("delete", kubeContext, namespace, resourceType, name)

	if err := c.isOperationAllowed("delete"); err != nil {
		return err
	}

	if err := c.isNamespaceRestricted(namespace); err != nil {
		return err
	}

	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return err
	}

	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return err
	}

	return deleteResource(ctx, dynamicClient, discoveryClient, c.builtinResources, namespace, resourceType, name, c.dryRun)
}

// Patch updates specific fields of a resource.
func (c *bearerTokenClient) Patch(ctx context.Context, kubeContext, namespace, resourceType, name string, patchType types.PatchType, data []byte) (runtime.Object, error) {
	c.logOperation("patch", kubeContext, namespace, resourceType, name)

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

	return patchResource(ctx, dynamicClient, discoveryClient, c.builtinResources, namespace, resourceType, name, patchType, data, c.dryRun)
}

// Scale changes the number of replicas.
func (c *bearerTokenClient) Scale(ctx context.Context, kubeContext, namespace, resourceType, name string, replicas int32) error {
	c.logOperation("scale", kubeContext, namespace, resourceType, name)

	if err := c.isOperationAllowed("scale"); err != nil {
		return err
	}

	if err := c.isNamespaceRestricted(namespace); err != nil {
		return err
	}

	dynamicClient, err := c.getDynamicClient()
	if err != nil {
		return err
	}

	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return err
	}

	return scaleResource(ctx, dynamicClient, discoveryClient, c.builtinResources, namespace, resourceType, name, replicas, c.dryRun)
}

// ========== PodManager Implementation ==========

// GetLogs retrieves logs from a pod container.
func (c *bearerTokenClient) GetLogs(ctx context.Context, kubeContext, namespace, podName, containerName string, opts LogOptions) (io.ReadCloser, error) {
	c.logOperation("logs", kubeContext, namespace, "pod", podName)

	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	return getLogs(ctx, clientset, namespace, podName, containerName, opts)
}

// Exec executes a command inside a pod container.
func (c *bearerTokenClient) Exec(ctx context.Context, kubeContext, namespace, podName, containerName string, command []string, opts ExecOptions) (*ExecResult, error) {
	c.logOperation("exec", kubeContext, namespace, "pod", podName)

	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	config, err := c.getRestConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	return execInPod(ctx, clientset, config, namespace, podName, containerName, command, opts)
}

// PortForward sets up port forwarding to a pod.
func (c *bearerTokenClient) PortForward(ctx context.Context, kubeContext, namespace, podName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	c.logOperation("port-forward", kubeContext, namespace, "pod", podName)

	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	config, err := c.getRestConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	return portForwardToPod(ctx, clientset, config, namespace, podName, ports, opts)
}

// PortForwardToService sets up port forwarding to a service.
func (c *bearerTokenClient) PortForwardToService(ctx context.Context, kubeContext, namespace, serviceName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	c.logOperation("port-forward-service", kubeContext, namespace, "service", serviceName)

	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	config, err := c.getRestConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	return portForwardToService(ctx, clientset, config, namespace, serviceName, ports, opts)
}

// ========== ClusterManager Implementation ==========

// GetAPIResources returns available API resources.
func (c *bearerTokenClient) GetAPIResources(ctx context.Context, kubeContext string, limit, offset int, apiGroup string, namespacedOnly bool, verbs []string) (*PaginatedAPIResourceResponse, error) {
	c.logOperation("api-resources", kubeContext, "", "", "")

	discoveryClient, err := c.getDiscoveryClient()
	if err != nil {
		return nil, err
	}

	return getAPIResources(ctx, discoveryClient, limit, offset, apiGroup, namespacedOnly, verbs)
}

// GetClusterHealth returns the health status of the cluster.
func (c *bearerTokenClient) GetClusterHealth(ctx context.Context, kubeContext string) (*ClusterHealth, error) {
	c.logOperation("cluster-health", kubeContext, "", "", "")

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
