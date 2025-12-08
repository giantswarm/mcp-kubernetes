package federation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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
	// Local Management Cluster clients
	localClient  kubernetes.Interface
	localDynamic dynamic.Interface

	// Logger for operational messages
	logger *slog.Logger

	// Lifecycle management
	mu     sync.RWMutex
	closed bool
}

// Ensure Manager implements ClusterClientManager.
var _ ClusterClientManager = (*Manager)(nil)

// NewManager creates a new ClusterClientManager with the provided local clients.
//
// Parameters:
//   - localClient: Kubernetes clientset for the Management Cluster
//   - localDynamic: Dynamic client for the Management Cluster (for CAPI CRDs)
//   - logger: Structured logger for operational messages (can be nil)
//
// The local clients should be configured with admin credentials for the
// Management Cluster. These credentials are only used to:
//   - Read CAPI Cluster resources for discovery
//   - Read kubeconfig Secrets for Workload Cluster access
//   - Establish TLS connections to Workload Clusters
//
// All actual operations are executed under user impersonation.
func NewManager(localClient kubernetes.Interface, localDynamic dynamic.Interface, logger *slog.Logger) (*Manager, error) {
	if localClient == nil {
		return nil, fmt.Errorf("local client is required")
	}
	if localDynamic == nil {
		return nil, fmt.Errorf("local dynamic client is required")
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		localClient:  localClient,
		localDynamic: localDynamic,
		logger:       logger,
	}, nil
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

	// Future: clean up cached clients and connections
	// This will be implemented in issue #106 (Client Caching)

	return nil
}

// getLocalClientWithImpersonation returns the local client configured for user impersonation.
// The ctx parameter is reserved for future use when impersonation is implemented.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getLocalClientWithImpersonation(_ context.Context, user *UserInfo) (kubernetes.Interface, error) {
	// Impersonation will be implemented in issue #109
	// For now, return the local client directly
	m.logger.Debug("getLocalClientWithImpersonation - impersonation not yet implemented",
		"user_hash", AnonymizeEmail(user.Email),
		"group_count", len(user.Groups))
	return m.localClient, nil
}

// getLocalDynamicWithImpersonation returns the local dynamic client configured for user impersonation.
// The ctx parameter is reserved for future use when impersonation is implemented.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getLocalDynamicWithImpersonation(_ context.Context, user *UserInfo) (dynamic.Interface, error) {
	// Impersonation will be implemented in issue #109
	// For now, return the local dynamic client directly
	m.logger.Debug("getLocalDynamicWithImpersonation - impersonation not yet implemented",
		"user_hash", AnonymizeEmail(user.Email),
		"group_count", len(user.Groups))
	return m.localDynamic, nil
}

// getRemoteClientWithImpersonation returns a client for a remote workload cluster.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getRemoteClientWithImpersonation(_ context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, error) {
	// This will be implemented in issues:
	// - #106 (Client Caching)
	// - #107 (Kubeconfig Secret Retrieval)
	// - #109 (User Impersonation)
	m.logger.Debug("getRemoteClientWithImpersonation - not yet implemented",
		"cluster", clusterName,
		"user_hash", AnonymizeEmail(user.Email),
		"group_count", len(user.Groups))
	return nil, &ClusterNotFoundError{
		ClusterName: clusterName,
		Reason:      "remote cluster client retrieval not yet implemented",
	}
}

// getRemoteDynamicWithImpersonation returns a dynamic client for a remote workload cluster.
// Note: user is guaranteed to be non-nil and validated by the public API methods.
func (m *Manager) getRemoteDynamicWithImpersonation(_ context.Context, clusterName string, user *UserInfo) (dynamic.Interface, error) {
	// This will be implemented in issues:
	// - #106 (Client Caching)
	// - #107 (Kubeconfig Secret Retrieval)
	// - #109 (User Impersonation)
	m.logger.Debug("getRemoteDynamicWithImpersonation - not yet implemented",
		"cluster", clusterName,
		"user_hash", AnonymizeEmail(user.Email),
		"group_count", len(user.Groups))
	return nil, &ClusterNotFoundError{
		ClusterName: clusterName,
		Reason:      "remote cluster dynamic client retrieval not yet implemented",
	}
}
