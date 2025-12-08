package federation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClusterClientManager manages Kubernetes clients for multi-cluster operations.
// It retrieves clients for both the local Management Cluster and remote Workload
// Clusters, with support for user impersonation.
//
// All methods are thread-safe and can be called concurrently from multiple
// tool handlers.
type ClusterClientManager interface {
	// GetClient returns a Kubernetes client for the target cluster,
	// configured to impersonate the provided user.
	// If clusterName is empty, returns the local (Management Cluster) client.
	//
	// The returned client has Impersonate-User and Impersonate-Group headers
	// configured based on the UserInfo, ensuring all operations are executed
	// under the authenticated user's identity.
	GetClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, error)

	// GetDynamicClient returns a dynamic client for the target cluster,
	// useful for working with CRDs like CAPI resources.
	// If clusterName is empty, returns the local (Management Cluster) dynamic client.
	//
	// Like GetClient, the returned client is configured for user impersonation.
	GetDynamicClient(ctx context.Context, clusterName string, user *UserInfo) (dynamic.Interface, error)

	// ListClusters returns a list of available workload clusters.
	// The list is filtered based on the user's RBAC permissions - only clusters
	// the user has access to view will be returned.
	//
	// This method queries CAPI Cluster resources on the Management Cluster.
	ListClusters(ctx context.Context, user *UserInfo) ([]ClusterSummary, error)

	// GetClusterSummary returns detailed information about a specific cluster.
	// Returns ErrClusterNotFound if the cluster doesn't exist or the user
	// doesn't have permission to access it.
	GetClusterSummary(ctx context.Context, clusterName string, user *UserInfo) (*ClusterSummary, error)

	// Close releases all cached clients and resources.
	// After Close is called, all other methods will return ErrManagerClosed.
	Close() error
}

// Manager implements ClusterClientManager for CAPI-based multi-cluster federation.
type Manager struct {
	// ClientProvider creates per-user clients for Management Cluster access.
	// This ensures all operations (including kubeconfig secret retrieval) are
	// performed with the user's RBAC permissions, not elevated admin privileges.
	clientProvider ClientProvider

	// Client cache for remote workload cluster clients (per user)
	cache *ClientCache

	// Cache configuration (set via options, applied during NewManager)
	cacheConfig  *CacheConfig
	cacheMetrics CacheMetricsRecorder

	// Connection validation timeout for workload cluster health checks
	connectionValidationTimeout time.Duration

	// Logger for operational messages
	logger *slog.Logger

	// Lifecycle management
	mu     sync.RWMutex
	closed bool
}

// Ensure Manager implements ClusterClientManager.
var _ ClusterClientManager = (*Manager)(nil)

// ManagerOption is a functional option for configuring Manager.
type ManagerOption func(*Manager)

// WithManagerLogger sets the logger for the Manager.
func WithManagerLogger(logger *slog.Logger) ManagerOption {
	return func(m *Manager) {
		m.logger = logger
	}
}

// WithManagerCacheConfig sets the cache configuration for the Manager.
// This option can be combined with WithManagerCacheMetrics.
func WithManagerCacheConfig(config CacheConfig) ManagerOption {
	return func(m *Manager) {
		m.cacheConfig = &config
	}
}

// WithManagerCacheMetrics sets the metrics recorder for the cache.
// This option can be combined with WithManagerCacheConfig.
func WithManagerCacheMetrics(metrics CacheMetricsRecorder) ManagerOption {
	return func(m *Manager) {
		m.cacheMetrics = metrics
	}
}

// WithManagerConnectionValidationTimeout sets the timeout for validating
// connections to workload clusters. This is useful for high-latency environments
// where the default timeout (10s) may be insufficient.
//
// The timeout applies to health checks performed when using
// GetKubeconfigForClusterValidated.
func WithManagerConnectionValidationTimeout(timeout time.Duration) ManagerOption {
	return func(m *Manager) {
		m.connectionValidationTimeout = timeout
	}
}

// NewManager creates a new ClusterClientManager with the provided ClientProvider.
//
// # Security Model
//
// The ClientProvider is responsible for creating per-user Kubernetes clients.
// This ensures that ALL Management Cluster operations (including kubeconfig
// secret retrieval) are performed with the user's RBAC permissions.
//
// When OAuth downstream is enabled:
//   - Each user's OAuth token is used to authenticate with the Management Cluster
//   - Users can only access kubeconfig secrets they have RBAC permission to read
//   - This provides defense in depth: MC RBAC + WC RBAC both enforced
//
// Parameters:
//   - clientProvider: Creates per-user clients for Management Cluster access
//   - opts: Functional options for configuration
//
// Example with OAuth downstream:
//
//	provider := &OAuthClientProvider{factory: bearerTokenFactory}
//	manager, err := federation.NewManager(provider,
//	    federation.WithManagerLogger(logger),
//	)
func NewManager(clientProvider ClientProvider, opts ...ManagerOption) (*Manager, error) {
	if clientProvider == nil {
		return nil, fmt.Errorf("client provider is required")
	}

	m := &Manager{
		clientProvider:              clientProvider,
		connectionValidationTimeout: DefaultConnectionValidationTimeout,
		logger:                      slog.Default(),
	}

	// Apply options
	for _, opt := range opts {
		opt(m)
	}

	// Build cache options from configuration set via Manager options
	cacheOpts := []ClientCacheOption{WithCacheLogger(m.logger)}
	if m.cacheConfig != nil {
		cacheOpts = append(cacheOpts, WithCacheConfig(*m.cacheConfig))
	}
	if m.cacheMetrics != nil {
		cacheOpts = append(cacheOpts, WithCacheMetrics(m.cacheMetrics))
	}
	m.cache = NewClientCache(cacheOpts...)

	m.logger.Info("Federation manager initialized",
		"cache_enabled", m.cache != nil)

	return m, nil
}

// checkClosed returns ErrManagerClosed if the manager has been closed.
// This is a helper to avoid repeating the closed-check pattern in every method.
func (m *Manager) checkClosed() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrManagerClosed
	}
	return nil
}

// GetClient returns a Kubernetes client for the target cluster.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
// Returns ErrInvalidClusterName if the cluster name fails validation.
func (m *Manager) GetClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	// Validate user info (required to prevent privilege escalation)
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// If no cluster specified, return local client with impersonation
	if clusterName == "" {
		return m.getLocalClientWithImpersonation(ctx, user)
	}

	// Validate cluster name
	if err := ValidateClusterName(clusterName); err != nil {
		return nil, err
	}

	// Get remote cluster client with impersonation
	return m.getRemoteClientWithImpersonation(ctx, clusterName, user)
}

// GetDynamicClient returns a dynamic client for the target cluster.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
// Returns ErrInvalidClusterName if the cluster name fails validation.
func (m *Manager) GetDynamicClient(ctx context.Context, clusterName string, user *UserInfo) (dynamic.Interface, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	// Validate user info (required to prevent privilege escalation)
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// If no cluster specified, return local dynamic client with impersonation
	if clusterName == "" {
		return m.getLocalDynamicWithImpersonation(ctx, user)
	}

	// Validate cluster name
	if err := ValidateClusterName(clusterName); err != nil {
		return nil, err
	}

	// Get remote cluster dynamic client with impersonation
	return m.getRemoteDynamicWithImpersonation(ctx, clusterName, user)
}

// ListClusters returns all available workload clusters.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
func (m *Manager) ListClusters(ctx context.Context, user *UserInfo) ([]ClusterSummary, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	// Validate user info (required to prevent privilege escalation)
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// This will be implemented in a separate issue (#111 - CAPI Cluster Discovery)
	// For now, return an empty list
	m.logger.Debug("ListClusters called - CAPI discovery not yet implemented",
		"user_hash", AnonymizeEmail(user.Email))
	return []ClusterSummary{}, nil
}

// GetClusterSummary returns information about a specific cluster.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
// Returns ErrInvalidClusterName if the cluster name fails validation.
func (m *Manager) GetClusterSummary(ctx context.Context, clusterName string, user *UserInfo) (*ClusterSummary, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	// Validate user info (required to prevent privilege escalation)
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// Validate cluster name
	if err := ValidateClusterName(clusterName); err != nil {
		return nil, err
	}

	// This will be implemented in a separate issue (#111 - CAPI Cluster Discovery)
	// For now, return not found
	m.logger.Debug("GetClusterSummary called - CAPI discovery not yet implemented",
		"cluster", clusterName,
		"user_hash", AnonymizeEmail(user.Email))
	return nil, &ClusterNotFoundError{
		ClusterName: clusterName,
		Reason:      "CAPI cluster discovery not yet implemented",
	}
}

// Close releases all cached clients and resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.logger.Info("Closing federation manager")
	m.closed = true

	// Close the client cache
	if m.cache != nil {
		if err := m.cache.Close(); err != nil {
			m.logger.Error("Error closing client cache", "error", err)
			return fmt.Errorf("failed to close client cache: %w", err)
		}
	}

	return nil
}

// getLocalClientWithImpersonation returns the local client for the user.
// With OAuth downstream, the ClientProvider returns a client authenticated as the user.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getLocalClientWithImpersonation(ctx context.Context, user *UserInfo) (kubernetes.Interface, error) {
	// Use empty string for local cluster
	const localClusterName = ""

	// Try to get from cache or create new
	clientset, _, err := m.cache.GetOrCreate(ctx, localClusterName, user.Email, func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return m.clientProvider.GetClientsForUser(ctx, user)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get local client for user: %w", err)
	}

	return clientset, nil
}

// getLocalDynamicWithImpersonation returns the local dynamic client for the user.
// With OAuth downstream, the ClientProvider returns a client authenticated as the user.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getLocalDynamicWithImpersonation(ctx context.Context, user *UserInfo) (dynamic.Interface, error) {
	// Use empty string for local cluster
	const localClusterName = ""

	// Try to get from cache or create new
	_, dynamicClient, err := m.cache.GetOrCreate(ctx, localClusterName, user.Email, func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return m.clientProvider.GetClientsForUser(ctx, user)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get local dynamic client for user: %w", err)
	}

	return dynamicClient, nil
}

// getRemoteClientWithImpersonation returns a client for a remote workload cluster.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getRemoteClientWithImpersonation(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, error) {
	// Get client from cache or create new
	clientset, _, err := m.cache.GetOrCreate(ctx, clusterName, user.Email, func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return m.createRemoteClusterClient(ctx, clusterName, user)
	})
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

// getRemoteDynamicWithImpersonation returns a dynamic client for a remote workload cluster.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getRemoteDynamicWithImpersonation(ctx context.Context, clusterName string, user *UserInfo) (dynamic.Interface, error) {
	// Get client from cache or create new
	_, dynamicClient, err := m.cache.GetOrCreate(ctx, clusterName, user.Email, func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return m.createRemoteClusterClient(ctx, clusterName, user)
	})
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}

// createRemoteClusterClient creates Kubernetes clients for a remote workload cluster.
// This is called by the cache on cache miss.
//
// # Security Model
//
// The method uses the user's credentials for ALL operations:
//  1. Retrieves the kubeconfig secret using user's MC RBAC permissions
//  2. Configures impersonation headers for WC operations
//  3. Creates clientset and dynamic client with the impersonated config
//
// This ensures defense in depth: the user must have permission to read the
// kubeconfig secret on the MC, AND their impersonated identity must have
// permissions on the WC.
func (m *Manager) createRemoteClusterClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	m.logger.Debug("Creating remote cluster client",
		"cluster", clusterName,
		"user_hash", AnonymizeEmail(user.Email),
		"group_count", len(user.Groups))

	// Retrieve the kubeconfig for the cluster using user's credentials
	// This enforces MC RBAC - user must have permission to read the secret
	baseConfig, err := m.GetKubeconfigForCluster(ctx, clusterName, user)
	if err != nil {
		return nil, nil, nil, err
	}

	// Configure impersonation for WC operations
	// The kubeconfig contains admin credentials; we impersonate the user
	impersonatedConfig := ConfigWithImpersonation(baseConfig, user)

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(impersonatedConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create clientset for cluster %s: %w", clusterName, err)
	}

	// Create the dynamic client
	dynClient, err := dynamic.NewForConfig(impersonatedConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create dynamic client for cluster %s: %w", clusterName, err)
	}

	m.logger.Debug("Successfully created remote cluster client",
		"cluster", clusterName,
		"user_hash", AnonymizeEmail(user.Email))

	return clientset, dynClient, impersonatedConfig, nil
}
