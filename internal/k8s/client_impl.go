package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

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

	// Safety settings
	NonDestructiveMode   bool
	DryRun               bool
	AllowedOperations    []string
	RestrictedNamespaces []string

	// Performance settings
	QPSLimit   float32
	BurstLimit int
	Timeout    time.Duration

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
	}

	// Load kubeconfig
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
	if _, exists := client.kubeconfigData.Contexts[client.currentContext]; !exists {
		return nil, fmt.Errorf("context %q does not exist in kubeconfig", client.currentContext)
	}

	return client, nil
}

// loadKubeconfig loads the kubeconfig from the specified path or default locations.
func (c *kubernetesClient) loadKubeconfig() error {
	var err error

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
	c.mu.RLock()
	if restConfig, exists := c.restConfigs[contextName]; exists {
		c.mu.RUnlock()
		return restConfig, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if restConfig, exists := c.restConfigs[contextName]; exists {
		return restConfig, nil
	}

	// Create rest config for the specified context
	contextConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{},
		&clientcmd.ConfigOverrides{
			CurrentContext: contextName,
			Context:        clientcmdapi.Context{},
		},
	)

	restConfig, err := contextConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create rest config for context %q: %w", contextName, err)
	}

	// Apply performance settings
	restConfig.QPS = c.qpsLimit
	restConfig.Burst = c.burstLimit
	restConfig.Timeout = c.timeout

	// Cache the config
	c.restConfigs[contextName] = restConfig

	return restConfig, nil
}

// getClientset returns a typed clientset for the specified context.
func (c *kubernetesClient) getClientset(contextName string) (kubernetes.Interface, error) {
	c.mu.RLock()
	if clientset, exists := c.clientsets[contextName]; exists {
		c.mu.RUnlock()
		return clientset, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if clientset, exists := c.clientsets[contextName]; exists {
		return clientset, nil
	}

	restConfig, err := c.getRestConfig(contextName)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset for context %q: %w", contextName, err)
	}

	// Cache the clientset
	c.clientsets[contextName] = clientset

	return clientset, nil
}

// getDynamicClient returns a dynamic client for the specified context.
func (c *kubernetesClient) getDynamicClient(contextName string) (dynamic.Interface, error) {
	c.mu.RLock()
	if dynamicClient, exists := c.dynamicClients[contextName]; exists {
		c.mu.RUnlock()
		return dynamicClient, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if dynamicClient, exists := c.dynamicClients[contextName]; exists {
		return dynamicClient, nil
	}

	restConfig, err := c.getRestConfig(contextName)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client for context %q: %w", contextName, err)
	}

	// Cache the client
	c.dynamicClients[contextName] = dynamicClient

	return dynamicClient, nil
}

// getDiscoveryClient returns a discovery client for the specified context.
func (c *kubernetesClient) getDiscoveryClient(contextName string) (discovery.DiscoveryInterface, error) {
	c.mu.RLock()
	if discoveryClient, exists := c.discoveryClients[contextName]; exists {
		c.mu.RUnlock()
		return discoveryClient, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if discoveryClient, exists := c.discoveryClients[contextName]; exists {
		return discoveryClient, nil
	}

	restConfig, err := c.getRestConfig(contextName)
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client for context %q: %w", contextName, err)
	}

	// Cache the client
	c.discoveryClients[contextName] = discoveryClient

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

	// Validate context exists
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
