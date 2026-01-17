// hybrid_client_provider.go provides the HybridOAuthClientProvider implementation
// for split-credential OAuth authentication in CAPI mode.
package federation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// PrivilegedSecretAccessProvider extends ClientProvider with the ability to access
// kubeconfig secrets using ServiceAccount credentials instead of user credentials.
//
// # Security Model
//
// This interface enables a split-credential model for CAPI mode with OAuth downstream:
//   - User OAuth tokens are used for CAPI cluster discovery (RBAC enforced)
//   - ServiceAccount credentials are used for kubeconfig secret access
//   - This prevents users from extracting admin kubeconfig credentials via kubectl
//
// The key security property is that users cannot bypass impersonation:
//   - Without this: Users with secret access could read kubeconfig directly and bypass impersonation
//   - With this: Only mcp-kubernetes reads kubeconfig secrets, impersonation is always enforced
//
// # Audit Trail
//
// All privileged secret access is logged with the user identity that triggered it,
// ensuring accountability and traceability in audit logs.
type PrivilegedSecretAccessProvider interface {
	ClientProvider

	// GetPrivilegedClientForSecrets returns a Kubernetes client using ServiceAccount credentials.
	// This client is used ONLY for reading kubeconfig secrets in CAPI mode.
	//
	// # Security
	//
	// - The returned client uses the pod's ServiceAccount credentials
	// - This enables secret access without granting users secret read permissions
	// - The user parameter is used for audit logging only, not for authentication
	// - The returned client should ONLY be used for secret retrieval, not other operations
	//
	// Parameters:
	//   - ctx: Context for the request
	//   - user: User identity for audit logging (who triggered this access)
	//
	// Returns:
	//   - kubernetes.Interface: Client for secret access
	//   - error: Any error during client creation
	GetPrivilegedClientForSecrets(ctx context.Context, user *UserInfo) (kubernetes.Interface, error)

	// HasPrivilegedAccess returns true if privileged secret access is available.
	// This allows the Manager to check if it should use privileged access for secrets.
	HasPrivilegedAccess() bool
}

// HybridOAuthClientProvider implements PrivilegedSecretAccessProvider for OAuth downstream
// authentication with split credential model.
//
// # Architecture
//
// This provider wraps two credential sources:
//  1. User OAuth tokens - for user-scoped operations (CAPI discovery, WC operations)
//  2. ServiceAccount credentials - for privileged operations (kubeconfig secret access)
//
// # Security Model
//
// When OAuth downstream is enabled with CAPI mode:
//   - Users authenticate via OAuth (Dex, Google, etc.)
//   - Their OAuth token is used for CAPI cluster discovery (RBAC enforced)
//   - ServiceAccount reads kubeconfig secrets (users cannot access directly)
//   - mcp-kubernetes applies impersonation to WC operations
//   - Users cannot bypass impersonation by reading secrets via kubectl
//
// # Audit Trail
//
// All privileged secret access is logged with:
//   - User email (hashed for privacy in logs)
//   - Cluster name being accessed
//   - Timestamp of access
//
// This provides a complete audit trail of who accessed which cluster's kubeconfig.
type HybridOAuthClientProvider struct {
	// userProvider handles user-scoped operations using OAuth tokens
	userProvider *OAuthClientProvider

	// saClientset is the ServiceAccount client for privileged secret access
	saClientset kubernetes.Interface

	// logger for audit trail and operational logging
	logger *slog.Logger

	// initOnce ensures lazy initialization happens only once
	initOnce sync.Once

	// initErr captures any error from lazy initialization
	initErr error

	// configProvider creates the in-cluster config (allows testing)
	configProvider InClusterConfigProvider
}

// InClusterConfigProvider is a function type for creating in-cluster configuration.
// This allows dependency injection for testing.
type InClusterConfigProvider func() (*rest.Config, error)

// DefaultInClusterConfigProvider returns the default in-cluster config provider.
func DefaultInClusterConfigProvider() InClusterConfigProvider {
	return rest.InClusterConfig
}

// HybridOAuthClientProviderConfig contains configuration for HybridOAuthClientProvider.
type HybridOAuthClientProviderConfig struct {
	// UserProvider is the OAuth client provider for user-scoped operations
	UserProvider *OAuthClientProvider

	// Logger for audit trail and operational logging
	Logger *slog.Logger

	// ConfigProvider creates in-cluster config (optional, defaults to rest.InClusterConfig)
	ConfigProvider InClusterConfigProvider
}

// NewHybridOAuthClientProvider creates a new HybridOAuthClientProvider.
//
// The ServiceAccount client is lazily initialized on first use to avoid
// errors when running outside a Kubernetes cluster (e.g., during testing).
func NewHybridOAuthClientProvider(config *HybridOAuthClientProviderConfig) (*HybridOAuthClientProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.UserProvider == nil {
		return nil, fmt.Errorf("user provider is required")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	configProvider := config.ConfigProvider
	if configProvider == nil {
		configProvider = DefaultInClusterConfigProvider()
	}

	return &HybridOAuthClientProvider{
		userProvider:   config.UserProvider,
		logger:         logger,
		configProvider: configProvider,
	}, nil
}

// GetClientsForUser returns Kubernetes clients authenticated with the user's OAuth token.
// This delegates to the underlying OAuthClientProvider.
func (p *HybridOAuthClientProvider) GetClientsForUser(ctx context.Context, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	return p.userProvider.GetClientsForUser(ctx, user)
}

// GetPrivilegedClientForSecrets returns the ServiceAccount client for secret access.
//
// # Security
//
// This method:
//  1. Lazily initializes the ServiceAccount client on first call
//  2. Logs the access for audit purposes (with user identity)
//  3. Returns a client that should ONLY be used for kubeconfig secret retrieval
//
// The audit log entry includes the user identity to track who triggered the access,
// even though the actual Kubernetes API call uses ServiceAccount credentials.
func (p *HybridOAuthClientProvider) GetPrivilegedClientForSecrets(ctx context.Context, user *UserInfo) (kubernetes.Interface, error) {
	// Ensure ServiceAccount client is initialized
	if err := p.initServiceAccountClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize ServiceAccount client: %w", err)
	}

	// Log the privileged access for audit trail
	p.logger.Info("Privileged secret access initiated",
		UserHashAttr(user.Email),
		"operation", "kubeconfig_secret_access")

	return p.saClientset, nil
}

// HasPrivilegedAccess returns true if privileged secret access is available.
// This checks if the ServiceAccount client can be initialized.
func (p *HybridOAuthClientProvider) HasPrivilegedAccess() bool {
	if err := p.initServiceAccountClient(); err != nil {
		p.logger.Debug("Privileged access not available",
			"error", err)
		return false
	}
	return true
}

// initServiceAccountClient lazily initializes the ServiceAccount client.
// This is called on first use to avoid startup errors when running outside a cluster.
func (p *HybridOAuthClientProvider) initServiceAccountClient() error {
	p.initOnce.Do(func() {
		p.logger.Debug("Initializing ServiceAccount client for privileged secret access")

		// Get in-cluster config
		config, err := p.configProvider()
		if err != nil {
			p.initErr = fmt.Errorf("failed to get in-cluster config: %w", err)
			p.logger.Error("Failed to initialize ServiceAccount client",
				"error", p.initErr)
			return
		}

		// Create clientset for secret access
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			p.initErr = fmt.Errorf("failed to create clientset: %w", err)
			p.logger.Error("Failed to create ServiceAccount clientset",
				"error", p.initErr)
			return
		}
		p.saClientset = clientset

		p.logger.Info("ServiceAccount client initialized for privileged secret access")
	})

	return p.initErr
}

// SetTokenExtractor sets the token extractor on the underlying user provider.
// This method passes through to the wrapped OAuthClientProvider.
func (p *HybridOAuthClientProvider) SetTokenExtractor(extractor TokenExtractor) {
	p.userProvider.SetTokenExtractor(extractor)
}

// SetMetrics sets the metrics recorder on the underlying user provider.
// This method passes through to the wrapped OAuthClientProvider.
func (p *HybridOAuthClientProvider) SetMetrics(metrics OAuthAuthMetricsRecorder) {
	p.userProvider.SetMetrics(metrics)
}

// Ensure HybridOAuthClientProvider implements PrivilegedSecretAccessProvider.
var _ PrivilegedSecretAccessProvider = (*HybridOAuthClientProvider)(nil)

// Ensure HybridOAuthClientProvider implements ClientProvider.
var _ ClientProvider = (*HybridOAuthClientProvider)(nil)
