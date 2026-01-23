package federation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// AuthMetricsRecorder defines the interface for recording authentication metrics.
// This allows decoupling from the concrete instrumentation implementation.
type AuthMetricsRecorder interface {
	// RecordWorkloadClusterAuth records a workload cluster authentication attempt.
	// authMode: "impersonation" or "sso-passthrough"
	// clusterName: Target cluster name
	// result: "success", "error", "token_missing"
	RecordWorkloadClusterAuth(ctx context.Context, authMode, clusterName, result string)
}

// noopAuthMetricsRecorder is a no-op implementation of AuthMetricsRecorder.
// Used as default to avoid nil checks throughout the codebase.
type noopAuthMetricsRecorder struct{}

func (n *noopAuthMetricsRecorder) RecordWorkloadClusterAuth(context.Context, string, string, string) {
}

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

	// GetRestConfig returns the REST configuration for the target cluster.
	// This is needed for operations that require direct REST access,
	// such as exec and port-forward which create SPDY connections.
	// If clusterName is empty, returns the local (Management Cluster) config.
	//
	// Like GetClient, the returned config is configured for user impersonation.
	GetRestConfig(ctx context.Context, clusterName string, user *UserInfo) (*rest.Config, error)

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

	// CheckAccess verifies if the user can perform the specified action on a cluster.
	// This performs a SelfSubjectAccessReview to check permissions without actually
	// attempting the operation.
	//
	// Pre-flight checks improve user experience by failing fast with clear error
	// messages and reduce noise in Kubernetes audit logs from failed requests.
	//
	// Parameters:
	//   - ctx: Context for the request
	//   - clusterName: Target cluster (empty for local/management cluster)
	//   - user: Authenticated user info for impersonation
	//   - check: Describes the action to check (verb, resource, namespace, etc.)
	//
	// Returns:
	//   - *AccessCheckResult: Contains Allowed/Denied status and reason
	//   - error: Non-nil if the check itself failed (not the same as denied)
	//
	// Example:
	//
	//	result, err := manager.CheckAccess(ctx, "prod-cluster", user, &AccessCheck{
	//		Verb:      "delete",
	//		Resource:  "pods",
	//		Namespace: "production",
	//	})
	//	if err != nil {
	//		return err // Check failed
	//	}
	//	if !result.Allowed {
	//		return fmt.Errorf("permission denied: %s", result.Reason)
	//	}
	CheckAccess(ctx context.Context, clusterName string, user *UserInfo, check *AccessCheck) (*AccessCheckResult, error)

	// Close releases all cached clients and resources.
	// After Close is called, all other methods will return ErrManagerClosed.
	Close() error

	// Stats returns current cache and manager statistics for monitoring.
	// This is useful for health endpoints and operational dashboards.
	Stats() ManagerStats
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

	// Auth metrics recorder for tracking authentication mode usage
	authMetrics AuthMetricsRecorder

	// Connection validation timeout for workload cluster health checks
	connectionValidationTimeout time.Duration

	// Connectivity configuration for network topology handling
	connectivityConfig *ConnectivityConfig

	// validateConnectivity enables connectivity validation before caching clients.
	// When enabled, CheckConnectivityWithRetry is called before caching a new client.
	validateConnectivity bool

	// workloadClusterAuthMode determines how to authenticate to workload clusters.
	// Default is "impersonation" (existing behavior).
	// "sso-passthrough" forwards user's SSO token directly to WC API servers.
	workloadClusterAuthMode WorkloadClusterAuthMode

	// ssoPassthroughConfig holds configuration for SSO passthrough mode.
	// Only used when workloadClusterAuthMode is WorkloadClusterAuthModeSSOPassthrough.
	ssoPassthroughConfig *SSOPassthroughConfig

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

// WithAuthMetrics sets the metrics recorder for authentication events.
// This enables tracking of workload cluster authentication by mode
// (impersonation vs sso-passthrough).
func WithAuthMetrics(metrics AuthMetricsRecorder) ManagerOption {
	return func(m *Manager) {
		m.authMetrics = metrics
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

// WithConnectivityConfig sets the connectivity configuration for the Manager.
// This controls how the manager establishes and validates connections to
// workload clusters, including timeouts and retry behavior.
//
// # Network Topology Considerations
//
// Configure this based on your network topology:
//   - For VPC-peered clusters: use DefaultConnectivityConfig()
//   - For cross-region or konnectivity: use HighLatencyConnectivityConfig()
//
// Example:
//
//	manager, err := federation.NewManager(provider,
//	    federation.WithConnectivityConfig(federation.HighLatencyConnectivityConfig()),
//	)
func WithConnectivityConfig(config ConnectivityConfig) ManagerOption {
	return func(m *Manager) {
		m.connectivityConfig = &config
	}
}

// WithConnectivityValidation enables connectivity validation before caching clients.
// When enabled, the manager will verify that a workload cluster is reachable before
// caching the client. This catches network issues early but adds latency to the
// first request for each cluster.
//
// # Trade-offs
//
//   - Enabled: Catches network issues early, better error messages, slight latency increase
//   - Disabled: Faster first request, but network errors surface during actual operations
//
// Default: false (disabled)
func WithConnectivityValidation(enabled bool) ManagerOption {
	return func(m *Manager) {
		m.validateConnectivity = enabled
	}
}

// WithWorkloadClusterAuthMode sets the authentication mode for workload clusters.
//
// # Supported Modes
//
//   - WorkloadClusterAuthModeImpersonation (default): Uses admin credentials from
//     kubeconfig secrets with user impersonation headers. This is the existing behavior.
//
//   - WorkloadClusterAuthModeSSOPassthrough: Forwards the user's SSO/OAuth ID token
//     directly to workload cluster API servers. This eliminates the need for admin
//     credentials and requires WC API servers to be configured with OIDC authentication.
//
// # Security Model Comparison
//
// Impersonation mode:
//   - ServiceAccount reads kubeconfig secrets (contains admin credentials)
//   - All WC API requests use admin creds + Impersonate-User/Group headers
//   - Higher privilege requirements for ServiceAccount
//
// SSO Passthrough mode:
//   - Only CA certificate needed (no admin credentials)
//   - User's ID token forwarded as Bearer token
//   - WC API server validates token via OIDC
//   - Lower privilege requirements, better audit trail
func WithWorkloadClusterAuthMode(mode WorkloadClusterAuthMode) ManagerOption {
	return func(m *Manager) {
		m.workloadClusterAuthMode = mode
	}
}

// WithSSOPassthroughConfig sets the configuration for SSO passthrough mode.
// This must be set when using WorkloadClusterAuthModeSSOPassthrough.
//
// The config includes:
//   - CAConfigMapSuffix: suffix for CA ConfigMaps (default: "-ca-public")
//   - TokenExtractor: function to extract SSO token from context
//
// Example:
//
//	manager, err := federation.NewManager(provider,
//	    federation.WithWorkloadClusterAuthMode(federation.WorkloadClusterAuthModeSSOPassthrough),
//	    federation.WithSSOPassthroughConfig(&federation.SSOPassthroughConfig{
//	        CAConfigMapSuffix: "-ca-public",
//	        TokenExtractor: oauth.GetAccessTokenFromContext,
//	    }),
//	)
func WithSSOPassthroughConfig(config *SSOPassthroughConfig) ManagerOption {
	return func(m *Manager) {
		m.ssoPassthroughConfig = config
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
		authMetrics:                 &noopAuthMetricsRecorder{},
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

// GetRestConfig returns the REST configuration for the target cluster.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
// Returns ErrInvalidClusterName if the cluster name fails validation.
func (m *Manager) GetRestConfig(ctx context.Context, clusterName string, user *UserInfo) (*rest.Config, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	// Validate user info (required to prevent privilege escalation)
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// If no cluster specified, return local rest config with impersonation
	if clusterName == "" {
		return m.getLocalRestConfigWithImpersonation(ctx, user)
	}

	// Validate cluster name
	if err := ValidateClusterName(clusterName); err != nil {
		return nil, err
	}

	// Get remote cluster rest config with impersonation
	return m.getRemoteRestConfigWithImpersonation(ctx, clusterName, user)
}

// ListClusters returns all available workload clusters.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
//
// The results are filtered based on the user's RBAC permissions - only clusters
// in namespaces the user can access will be returned.
//
// This method queries CAPI Cluster resources (cluster.x-k8s.io/v1beta2) on the
// Management Cluster and extracts metadata including:
//   - Provider (AWS, Azure, vSphere, etc.)
//   - Giant Swarm release version
//   - Kubernetes version
//   - Cluster status and readiness
//
// Returns ClusterDiscoveryError if CAPI CRDs are not installed.
func (m *Manager) ListClusters(ctx context.Context, user *UserInfo) ([]ClusterSummary, error) {
	return m.listClustersWithOptions(ctx, user, nil)
}

// GetClusterSummary returns information about a specific cluster.
// Returns ErrUserInfoRequired if user is nil (to prevent privilege escalation).
// Returns ErrInvalidClusterName if the cluster name fails validation.
// Returns ErrClusterNotFound if the cluster doesn't exist or the user
// doesn't have permission to access it.
//
// The method queries CAPI Cluster resources using a field selector for efficiency,
// and returns detailed metadata including provider, release, Kubernetes version,
// and status information.
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

	// Get user's dynamic client for Management Cluster
	dynamicClient, err := m.GetDynamicClient(ctx, "", user)
	if err != nil {
		return nil, fmt.Errorf("failed to get dynamic client: %w", err)
	}

	// Query cluster by name using field selector for efficiency
	summary, err := m.getClusterByName(ctx, dynamicClient, clusterName, user)
	if err != nil {
		return nil, err
	}

	if summary == nil {
		m.logger.Debug("Cluster not found",
			"cluster", clusterName,
			UserHashAttr(user.Email))
		return nil, &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      "no CAPI Cluster resource found with this name",
		}
	}

	m.logger.Debug("Found cluster",
		"cluster", clusterName,
		"namespace", summary.Namespace,
		UserHashAttr(user.Email))
	return summary, nil
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

// getLocalRestConfigWithImpersonation returns the local rest config for the user.
// With OAuth downstream, the ClientProvider returns a config authenticated as the user.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getLocalRestConfigWithImpersonation(ctx context.Context, user *UserInfo) (*rest.Config, error) {
	// Use empty string for local cluster
	const localClusterName = ""

	// Try to get from cache or create new
	_, _, restConfig, err := m.cache.GetOrCreateFull(ctx, localClusterName, user.Email, func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return m.clientProvider.GetClientsForUser(ctx, user)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get local rest config for user: %w", err)
	}

	return restConfig, nil
}

// getRemoteRestConfigWithImpersonation returns a rest config for a remote workload cluster.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getRemoteRestConfigWithImpersonation(ctx context.Context, clusterName string, user *UserInfo) (*rest.Config, error) {
	// Get config from cache or create new
	_, _, restConfig, err := m.cache.GetOrCreateFull(ctx, clusterName, user.Email, func(ctx context.Context) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
		return m.createRemoteClusterClient(ctx, clusterName, user)
	})
	if err != nil {
		return nil, err
	}

	return restConfig, nil
}

// createRemoteClusterClient creates Kubernetes clients for a remote workload cluster.
// This is called by the cache on cache miss.
//
// # Security Model
//
// The authentication method depends on the configured workloadClusterAuthMode:
//
// Impersonation mode (default):
//  1. Retrieves the kubeconfig secret using user's MC RBAC permissions
//  2. Optionally validates connectivity to the cluster
//  3. Configures impersonation headers for WC operations
//  4. Creates clientset and dynamic client with the impersonated config
//
// SSO Passthrough mode:
//  1. Retrieves only the CA certificate from a CA-only secret
//  2. Extracts the cluster API endpoint from the CAPI Cluster resource
//  3. Forwards user's SSO token directly to WC API server
//  4. No impersonation - user's own RBAC applies via OIDC auth
//
// # Network Topology
//
// When connectivity validation is enabled (WithConnectivityValidation(true)),
// this method verifies the cluster is reachable before caching the client.
// This is useful for detecting network issues early, especially in complex
// topologies with VPC peering, Transit Gateway, or konnectivity.
func (m *Manager) createRemoteClusterClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	m.logger.Debug("Creating remote cluster client",
		"cluster", clusterName,
		"auth_mode", string(m.workloadClusterAuthMode),
		UserHashAttr(user.Email),
		"group_count", len(user.Groups))

	// Use SSO passthrough if configured
	if m.workloadClusterAuthMode == WorkloadClusterAuthModeSSOPassthrough {
		return m.createSSOPassthroughClient(ctx, clusterName, user)
	}

	// Default: impersonation mode
	return m.createImpersonationClient(ctx, clusterName, user)
}

// createImpersonationClient creates a client using admin credentials with impersonation headers.
// This is the original/default behavior.
func (m *Manager) createImpersonationClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	// Retrieve the kubeconfig for the cluster using user's credentials
	// This enforces MC RBAC - user must have permission to read the secret
	baseConfig, err := m.GetKubeconfigForCluster(ctx, clusterName, user)
	if err != nil {
		m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeImpersonation), clusterName, "error")
		return nil, nil, nil, err
	}

	// Apply connectivity configuration if specified
	if m.connectivityConfig != nil {
		ApplyConnectivityConfig(baseConfig, *m.connectivityConfig)
	}

	// Optionally validate connectivity before caching
	if m.validateConnectivity {
		if err := m.checkClusterConnectivity(ctx, clusterName, baseConfig); err != nil {
			m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeImpersonation), clusterName, "error")
			return nil, nil, nil, err
		}
	}

	// Configure impersonation for WC operations
	// The kubeconfig contains admin credentials; we impersonate the user
	impersonatedConfig := ConfigWithImpersonation(baseConfig, user)

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(impersonatedConfig)
	if err != nil {
		m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeImpersonation), clusterName, "error")
		return nil, nil, nil, fmt.Errorf("failed to create clientset for cluster %s: %w", clusterName, err)
	}

	// Create the dynamic client
	dynClient, err := dynamic.NewForConfig(impersonatedConfig)
	if err != nil {
		m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeImpersonation), clusterName, "error")
		return nil, nil, nil, fmt.Errorf("failed to create dynamic client for cluster %s: %w", clusterName, err)
	}

	m.logger.Debug("Successfully created impersonation client",
		"cluster", clusterName,
		"endpoint_type", GetEndpointType(baseConfig.Host),
		UserHashAttr(user.Email))

	// Record successful auth metric
	m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeImpersonation), clusterName, "success")

	return clientset, dynClient, impersonatedConfig, nil
}

// createSSOPassthroughClient creates a client using the user's SSO token directly.
// This forwards the user's ID token to the workload cluster API server.
func (m *Manager) createSSOPassthroughClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	// Validate SSO passthrough is properly configured
	if m.ssoPassthroughConfig == nil {
		m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeSSOPassthrough), clusterName, "error")
		return nil, nil, nil, fmt.Errorf("SSO passthrough mode enabled but no configuration provided")
	}
	if m.ssoPassthroughConfig.TokenExtractor == nil {
		m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeSSOPassthrough), clusterName, "error")
		return nil, nil, nil, fmt.Errorf("SSO passthrough mode enabled but no token extractor configured")
	}

	// Create client using SSO passthrough
	clientset, dynClient, restConfig, err := m.CreateSSOPassthroughClient(ctx, clusterName, user)
	if err != nil {
		// Determine error type for metrics using sentinel error
		result := "error"
		if errors.Is(err, ErrSSOTokenMissing) {
			result = "token_missing"
		}
		m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeSSOPassthrough), clusterName, result)
		return nil, nil, nil, err
	}

	// Optionally validate connectivity before caching
	if m.validateConnectivity {
		if err := m.checkClusterConnectivity(ctx, clusterName, restConfig); err != nil {
			m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeSSOPassthrough), clusterName, "error")
			return nil, nil, nil, err
		}
	}

	m.logger.Debug("Successfully created SSO passthrough client",
		"cluster", clusterName,
		"endpoint_type", GetEndpointType(restConfig.Host),
		UserHashAttr(user.Email))

	// Record successful auth metric
	m.authMetrics.RecordWorkloadClusterAuth(ctx, string(WorkloadClusterAuthModeSSOPassthrough), clusterName, "success")

	return clientset, dynClient, restConfig, nil
}

// ManagerStats provides statistics about the manager for monitoring and health checks.
type ManagerStats struct {
	// CacheSize is the current number of cached client entries.
	CacheSize int

	// CacheMaxEntries is the maximum cache capacity.
	CacheMaxEntries int

	// CacheTTL is the configured time-to-live for cache entries.
	CacheTTL time.Duration

	// Closed indicates whether the manager has been closed.
	Closed bool
}

// Stats returns current cache and manager statistics for monitoring.
// This is useful for health endpoints and operational dashboards.
func (m *Manager) Stats() ManagerStats {
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()

	stats := ManagerStats{
		Closed: closed,
	}

	if m.cache != nil {
		cacheStats := m.cache.Stats()
		stats.CacheSize = cacheStats.Size
		stats.CacheMaxEntries = cacheStats.MaxEntries
		stats.CacheTTL = cacheStats.TTL
	}

	return stats
}

// checkClusterConnectivity validates connectivity to a workload cluster.
// This is a helper method that uses the configured ConnectivityConfig.
func (m *Manager) checkClusterConnectivity(ctx context.Context, clusterName string, config *rest.Config) error {
	cc := DefaultConnectivityConfig()
	if m.connectivityConfig != nil {
		cc = *m.connectivityConfig
	}

	m.logger.Debug("Validating cluster connectivity",
		"cluster", clusterName,
		"host", sanitizeHost(config.Host),
		"endpoint_type", GetEndpointType(config.Host),
		"timeout", cc.ConnectionTimeout,
		"retry_attempts", cc.RetryAttempts)

	err := CheckConnectivityWithRetry(ctx, clusterName, config, cc)
	if err != nil {
		m.logger.Debug("Cluster connectivity check failed",
			"cluster", clusterName,
			"host", sanitizeHost(config.Host),
			"error", err)
		return err
	}

	m.logger.Debug("Cluster connectivity validated",
		"cluster", clusterName,
		"host", sanitizeHost(config.Host))
	return nil
}

// CheckClusterConnectivity validates connectivity to a workload cluster.
// This is a public method that allows callers to explicitly check connectivity
// without caching the client.
//
// # Use Cases
//
// This method is useful for:
//   - Debugging network issues between MC and WC
//   - Implementing health checks for cluster lists
//   - Pre-validating clusters before batch operations
//
// Example:
//
//	err := manager.CheckClusterConnectivity(ctx, "prod-cluster", user)
//	if err != nil {
//	    log.Printf("Cluster unreachable: %v", err)
//	}
func (m *Manager) CheckClusterConnectivity(ctx context.Context, clusterName string, user *UserInfo) error {
	if err := m.checkClosed(); err != nil {
		return err
	}

	// Validate user info
	if err := ValidateUserInfo(user); err != nil {
		return err
	}

	// Validate cluster name
	if err := ValidateClusterName(clusterName); err != nil {
		return err
	}

	// Get the kubeconfig for the cluster
	config, err := m.GetKubeconfigForCluster(ctx, clusterName, user)
	if err != nil {
		return err
	}

	// Check connectivity
	return m.checkClusterConnectivity(ctx, clusterName, config)
}
