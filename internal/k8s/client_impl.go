package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// kubernetesClient implements the Client interface using client-go.
type kubernetesClient struct {
	// Configuration
	config *ClientConfig

	// Client cache for multi-cluster support
	mu               sync.RWMutex
	clientsets       map[string]kubernetes.Interface         // Context name -> clientset
	dynamicClients   map[string]dynamic.Interface            // Context name -> dynamic client
	discoveryClients map[string]discovery.DiscoveryInterface // Context name -> discovery client
	restConfigs      map[string]*rest.Config                 // Context name -> rest config

	// Kubeconfig management
	kubeconfigData *clientcmdapi.Config
	currentContext string

	// Resource type mappings
	builtinResources map[string]schema.GroupVersionResource

	// Safety and performance settings
	nonDestructiveMode   bool
	dryRun               bool
	allowedOperations    []string
	restrictedNamespaces []string

	// Performance settings
	qpsLimit   float32
	burstLimit int
	timeout    time.Duration
}

// ClientConfig holds configuration for the Kubernetes client.
type ClientConfig struct {
	// Kubeconfig settings
	KubeconfigPath string
	Context        string

	// Authentication mode
	InCluster bool // Use in-cluster service account authentication instead of kubeconfig

	// Safety settings
	NonDestructiveMode   bool
	DryRun               bool
	AllowedOperations    []string
	RestrictedNamespaces []string

	// Performance settings
	QPSLimit   float32
	BurstLimit int
	Timeout    time.Duration

	// Debug settings
	DebugMode bool

	// Logging
	Logger Logger
}

// Logger interface for client logging (simple version for now).
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// NewClient creates a new Kubernetes client with the given configuration.
func NewClient(config *ClientConfig) (*kubernetesClient, error) {
	if config == nil {
		return nil, fmt.Errorf("client configuration is required")
	}

	// Set defaults
	if config.QPSLimit == 0 {
		config.QPSLimit = 20.0
	}
	if config.BurstLimit == 0 {
		config.BurstLimit = 30
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	client := &kubernetesClient{
		config:               config,
		clientsets:           make(map[string]kubernetes.Interface),
		dynamicClients:       make(map[string]dynamic.Interface),
		discoveryClients:     make(map[string]discovery.DiscoveryInterface),
		restConfigs:          make(map[string]*rest.Config),
		nonDestructiveMode:   config.NonDestructiveMode,
		dryRun:               config.DryRun,
		allowedOperations:    config.AllowedOperations,
		restrictedNamespaces: config.RestrictedNamespaces,
		qpsLimit:             config.QPSLimit,
		burstLimit:           config.BurstLimit,
		timeout:              config.Timeout,
		builtinResources:     make(map[string]schema.GroupVersionResource),
	}

	// Initialize builtin resources
	client.builtinResources = map[string]schema.GroupVersionResource{
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

	// Handle authentication mode
	if config.InCluster {
		// In-cluster mode: use service account authentication
		client.currentContext = "in-cluster"

		// Validate in-cluster environment
		if err := client.validateInClusterEnvironment(); err != nil {
			return nil, fmt.Errorf("in-cluster authentication not available: %w", err)
		}

		if config.Logger != nil {
			config.Logger.Info("Using in-cluster authentication")
		}
	} else {
		// Kubeconfig mode: load kubeconfig
		if err := client.loadKubeconfig(); err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}

		// Set current context
		if config.Context != "" {
			client.currentContext = config.Context
		} else {
			client.currentContext = client.kubeconfigData.CurrentContext
		}

		// Validate current context exists
		if _, exists := client.kubeconfigData.Contexts[client.currentContext]; !exists && client.currentContext != "" {
			return nil, fmt.Errorf("context %q does not exist in kubeconfig", client.currentContext)
		}

		if config.Logger != nil {
			config.Logger.Info("Using kubeconfig authentication", "context", client.currentContext)
		}
	}

	return client, nil
}

// validateInClusterEnvironment checks if the required in-cluster authentication files are present.
func (c *kubernetesClient) validateInClusterEnvironment() error {
	// Check if service account token file exists
	tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		return fmt.Errorf("service account token not found at %s", tokenPath)
	}

	// Check if CA certificate file exists
	caPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	if _, err := os.Stat(caPath); os.IsNotExist(err) {
		return fmt.Errorf("service account CA certificate not found at %s", caPath)
	}

	// Check if namespace file exists
	namespacePath := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	if _, err := os.Stat(namespacePath); os.IsNotExist(err) {
		return fmt.Errorf("service account namespace not found at %s", namespacePath)
	}

	return nil
}

// loadKubeconfig loads the kubeconfig from the specified path or default locations.
func (c *kubernetesClient) loadKubeconfig() error {
	var err error

	{
		kconf := os.Getenv("KUBECONFIG")
		if strings.HasPrefix(kconf, "~/") {
			uhd, _ := os.UserHomeDir()
			kconf = filepath.Join(uhd, kconf[2:])
		}

		if kconf != "" && c.config.KubeconfigPath == "" {
			c.config.KubeconfigPath = kconf
		}
	}

	// Load kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.config.KubeconfigPath != "" {
		loadingRules.ExplicitPath = c.config.KubeconfigPath
	}

	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)

	rawConfig, err := config.RawConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	c.kubeconfigData = &rawConfig

	return nil
}

// getRestConfig returns a rest.Config for the specified context.
func (c *kubernetesClient) getRestConfig(contextName string) (*rest.Config, error) {
	// Use current context if none specified
	if contextName == "" {
		contextName = c.currentContext
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getRestConfig: starting", "contextName", contextName)
	}

	c.mu.RLock()
	if restConfig, exists := c.restConfigs[contextName]; exists {
		c.mu.RUnlock()
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: found cached config", "contextName", contextName)
		}
		return restConfig, nil
	}
	c.mu.RUnlock()

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getRestConfig: acquiring write lock")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if restConfig, exists := c.restConfigs[contextName]; exists {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: found cached config after write lock", "contextName", contextName)
		}
		return restConfig, nil
	}

	var restConfig *rest.Config
	var err error

	if c.config.InCluster {
		// In-cluster mode: use service account authentication
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: creating in-cluster config")
		}

		restConfig, err = rest.InClusterConfig()
		if err != nil {
			if c.config.DebugMode && c.config.Logger != nil {
				c.config.Logger.Error("getRestConfig: InClusterConfig() failed", "error", err)
			}
			return nil, fmt.Errorf("failed to create in-cluster rest config: %w", err)
		}

		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: got in-cluster REST config", "host", restConfig.Host)
		}
	} else {
		// Kubeconfig mode: use clientcmd
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: creating loading rules")
		}

		// Create rest config for the specified context
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		if c.config.KubeconfigPath != "" {
			loadingRules.ExplicitPath = c.config.KubeconfigPath
		}

		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: creating context config", "kubeconfigPath", c.config.KubeconfigPath)
		}

		contextConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{
				CurrentContext: contextName,
			},
		)

		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: calling ClientConfig()")
		}

		restConfig, err = contextConfig.ClientConfig()
		if err != nil {
			if c.config.DebugMode && c.config.Logger != nil {
				c.config.Logger.Error("getRestConfig: ClientConfig() failed", "error", err)
			}
			return nil, fmt.Errorf("failed to create rest config for context %q: %w", contextName, err)
		}

		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getRestConfig: got REST config", "host", restConfig.Host, "serverName", restConfig.ServerName)
		}
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getRestConfig: applying performance settings", "qps", c.qpsLimit, "burst", c.burstLimit, "timeout", c.timeout)
	}

	// Apply performance settings
	restConfig.QPS = c.qpsLimit
	restConfig.Burst = c.burstLimit
	restConfig.Timeout = c.timeout

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getRestConfig: caching config", "contextName", contextName)
	}

	// Cache the config
	c.restConfigs[contextName] = restConfig

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getRestConfig: completed successfully", "contextName", contextName)
	}

	return restConfig, nil
}

// getRestConfigUnsafe returns a rest.Config for the specified context without using locks.
func (c *kubernetesClient) getRestConfigUnsafe(contextName string) (*rest.Config, error) {
	// Use current context if none specified
	if contextName == "" {
		contextName = c.currentContext
	}

	// Check cache first (caller must hold write lock)
	if restConfig, exists := c.restConfigs[contextName]; exists {
		return restConfig, nil
	}

	var restConfig *rest.Config
	var err error

	if c.config.InCluster {
		// In-cluster mode: use service account authentication
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster rest config: %w", err)
		}
	} else {
		// Kubeconfig mode: use clientcmd
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		if c.config.KubeconfigPath != "" {
			loadingRules.ExplicitPath = c.config.KubeconfigPath
		}

		contextConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{
				CurrentContext: contextName,
			},
		)

		restConfig, err = contextConfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create rest config for context %q: %w", contextName, err)
		}
	}

	// Apply performance settings
	restConfig.QPS = c.qpsLimit
	restConfig.Burst = c.burstLimit
	restConfig.Timeout = c.timeout

	// Cache the config (caller must hold write lock)
	c.restConfigs[contextName] = restConfig

	return restConfig, nil
}

// getClientset returns a Kubernetes clientset for the specified context.
func (c *kubernetesClient) getClientset(contextName string) (kubernetes.Interface, error) {
	// Use current context if none specified
	if contextName == "" {
		contextName = c.currentContext
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getClientset: starting", "contextName", contextName)
	}

	c.mu.RLock()
	if clientset, exists := c.clientsets[contextName]; exists {
		c.mu.RUnlock()
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getClientset: found cached clientset", "contextName", contextName)
		}
		return clientset, nil
	}
	c.mu.RUnlock()

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getClientset: acquiring write lock")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if clientset, exists := c.clientsets[contextName]; exists {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getClientset: found cached clientset after write lock", "contextName", contextName)
		}
		return clientset, nil
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getClientset: getting REST config", "contextName", contextName)
	}

	// Call unsafe version since we already hold the write lock
	restConfig, err := c.getRestConfigUnsafe(contextName)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("getClientset: failed to get REST config", "error", err)
		}
		return nil, err
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getClientset: creating clientset from REST config")
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("getClientset: failed to create clientset", "error", err)
		}
		return nil, fmt.Errorf("failed to create clientset for context %q: %w", contextName, err)
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getClientset: caching clientset", "contextName", contextName)
	}

	// Cache the clientset
	c.clientsets[contextName] = clientset

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getClientset: completed successfully", "contextName", contextName)
	}

	return clientset, nil
}

// getDynamicClient returns a dynamic client for the specified context.
func (c *kubernetesClient) getDynamicClient(contextName string) (dynamic.Interface, error) {
	// Use current context if none specified
	if contextName == "" {
		contextName = c.currentContext
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDynamicClient: starting", "contextName", contextName, "currentContext", c.currentContext)
	}

	c.mu.RLock()
	if dynamicClient, exists := c.dynamicClients[contextName]; exists {
		c.mu.RUnlock()
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getDynamicClient: found cached client", "contextName", contextName)
		}
		return dynamicClient, nil
	}
	c.mu.RUnlock()

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDynamicClient: acquiring write lock")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if dynamicClient, exists := c.dynamicClients[contextName]; exists {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getDynamicClient: found cached client after write lock", "contextName", contextName)
		}
		return dynamicClient, nil
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDynamicClient: getting REST config", "contextName", contextName)
	}

	// Call unsafe version since we already hold the write lock
	restConfig, err := c.getRestConfigUnsafe(contextName)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("getDynamicClient: failed to get REST config", "error", err)
		}
		return nil, err
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDynamicClient: creating dynamic client from REST config")
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("getDynamicClient: failed to create dynamic client", "error", err)
		}
		return nil, fmt.Errorf("failed to create dynamic client for context %q: %w", contextName, err)
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDynamicClient: caching dynamic client", "contextName", contextName)
	}

	// Cache the client
	c.dynamicClients[contextName] = dynamicClient

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDynamicClient: completed successfully", "contextName", contextName)
	}

	return dynamicClient, nil
}

// getDiscoveryClient returns a discovery client for the specified context.
func (c *kubernetesClient) getDiscoveryClient(contextName string) (discovery.DiscoveryInterface, error) {
	// Use current context if none specified
	if contextName == "" {
		contextName = c.currentContext
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDiscoveryClient: starting", "contextName", contextName)
	}

	c.mu.RLock()
	if discoveryClient, exists := c.discoveryClients[contextName]; exists {
		c.mu.RUnlock()
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getDiscoveryClient: found cached discovery client", "contextName", contextName)
		}
		return discoveryClient, nil
	}
	c.mu.RUnlock()

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDiscoveryClient: acquiring write lock")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if discoveryClient, exists := c.discoveryClients[contextName]; exists {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("getDiscoveryClient: found cached discovery client after write lock", "contextName", contextName)
		}
		return discoveryClient, nil
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDiscoveryClient: getting REST config", "contextName", contextName)
	}

	// Call unsafe version since we already hold the write lock
	restConfig, err := c.getRestConfigUnsafe(contextName)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("getDiscoveryClient: failed to get REST config", "error", err)
		}
		return nil, err
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDiscoveryClient: creating discovery client from REST config")
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("getDiscoveryClient: failed to create discovery client", "error", err)
		}
		return nil, fmt.Errorf("failed to create discovery client for context %q: %w", contextName, err)
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDiscoveryClient: caching discovery client", "contextName", contextName)
	}

	// Cache the client
	c.discoveryClients[contextName] = discoveryClient

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("getDiscoveryClient: completed successfully", "contextName", contextName)
	}

	return discoveryClient, nil
}

// isOperationAllowed checks if an operation is allowed based on configuration.
func (c *kubernetesClient) isOperationAllowed(operation string) error {
	// Check if operation is in allowed list (if specified)
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

	// Check if operation is destructive and non-destructive mode is enabled
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
func (c *kubernetesClient) isNamespaceRestricted(namespace string) error {
	for _, restrictedNs := range c.restrictedNamespaces {
		if restrictedNs == namespace {
			return fmt.Errorf("access to namespace %q is restricted", namespace)
		}
	}
	return nil
}

// logOperation logs an operation for debugging and audit purposes.
func (c *kubernetesClient) logOperation(operation, context, namespace, resource, name string) {
	if c.config.Logger != nil {
		c.config.Logger.Debug("kubernetes operation",
			"operation", operation,
			"context", context,
			"namespace", namespace,
			"resource", resource,
			"name", name,
		)
	}
}

// ContextManager implementation

// ListContexts returns all available Kubernetes contexts.
func (c *kubernetesClient) ListContexts(ctx context.Context) ([]ContextInfo, error) {
	c.logOperation("list-contexts", "", "", "", "")

	if c.config.InCluster {
		// In-cluster mode: return single simulated context
		return []ContextInfo{
			{
				Name:      "in-cluster",
				Cluster:   "in-cluster",
				User:      "serviceaccount",
				Namespace: c.getInClusterNamespace(),
				Current:   true,
			},
		}, nil
	}

	// Kubeconfig mode: return contexts from kubeconfig
	var contexts []ContextInfo

	for contextName, contextInfo := range c.kubeconfigData.Contexts {
		contexts = append(contexts, ContextInfo{
			Name:      contextName,
			Cluster:   contextInfo.Cluster,
			User:      contextInfo.AuthInfo,
			Namespace: contextInfo.Namespace,
			Current:   contextName == c.currentContext,
		})
	}

	return contexts, nil
}

// GetCurrentContext returns the currently active context.
func (c *kubernetesClient) GetCurrentContext(ctx context.Context) (*ContextInfo, error) {
	c.logOperation("get-current-context", c.currentContext, "", "", "")

	if c.config.InCluster {
		// In-cluster mode: return simulated context
		return &ContextInfo{
			Name:      "in-cluster",
			Cluster:   "in-cluster",
			User:      "serviceaccount",
			Namespace: c.getInClusterNamespace(),
			Current:   true,
		}, nil
	}

	// Kubeconfig mode: return context from kubeconfig
	contextInfo, exists := c.kubeconfigData.Contexts[c.currentContext]
	if !exists {
		return nil, fmt.Errorf("current context %q does not exist", c.currentContext)
	}

	return &ContextInfo{
		Name:      c.currentContext,
		Cluster:   contextInfo.Cluster,
		User:      contextInfo.AuthInfo,
		Namespace: contextInfo.Namespace,
		Current:   true,
	}, nil
}

// SwitchContext changes the active Kubernetes context.
func (c *kubernetesClient) SwitchContext(ctx context.Context, contextName string) error {
	c.logOperation("switch-context", contextName, "", "", "")

	if c.config.InCluster {
		// In-cluster mode: only allow switching to "in-cluster" context
		if contextName != "in-cluster" {
			return fmt.Errorf("cannot switch context in in-cluster mode: only 'in-cluster' context is available")
		}
		// Context is already "in-cluster", no change needed
		return nil
	}

	// Kubeconfig mode: validate context exists and switch
	if _, exists := c.kubeconfigData.Contexts[contextName]; !exists {
		return fmt.Errorf("context %q does not exist in kubeconfig", contextName)
	}

	// Update current context
	c.mu.Lock()
	c.currentContext = contextName
	c.mu.Unlock()

	if c.config.Logger != nil {
		c.config.Logger.Info("switched kubernetes context", "context", contextName)
	}

	return nil
}

// getInClusterNamespace reads the namespace from the service account namespace file.
func (c *kubernetesClient) getInClusterNamespace() string {
	namespacePath := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	data, err := os.ReadFile(namespacePath)
	if err != nil {
		// Fallback to default namespace if we can't read the file
		return "default"
	}
	return string(data)
}
