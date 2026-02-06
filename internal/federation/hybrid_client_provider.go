// hybrid_client_provider.go provides the HybridOAuthClientProvider implementation
// for split-credential OAuth authentication in CAPI mode.
package federation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// PrivilegedAccessMetricsRecorder provides an interface for recording privileged access metrics.
// This enables monitoring of privileged access patterns for security observability.
// It covers both secret access and CAPI discovery metrics.
type PrivilegedAccessMetricsRecorder interface {
	// RecordPrivilegedAccess records a privileged access attempt.
	// Parameters:
	//   - ctx: Request context
	//   - userDomain: User's email domain (e.g., "giantswarm.io") for cardinality control
	//   - operation: The type of privileged operation ("secret_access" or "capi_discovery")
	//   - result: One of "success", "error", "rate_limited", "fallback"
	RecordPrivilegedAccess(ctx context.Context, userDomain, operation, result string)
}

// PrivilegedAccessProvider extends ClientProvider with the ability to access
// kubeconfig secrets and CAPI resources using ServiceAccount credentials instead of user credentials.
//
// # Security Model
//
// This interface enables a split-credential model for CAPI mode with OAuth downstream:
//   - ServiceAccount credentials are used for CAPI cluster discovery and kubeconfig secret access
//   - User OAuth tokens are used for operations on workload clusters (with impersonation)
//   - This prevents users from extracting admin kubeconfig credentials via kubectl
//
// The key security property is that users cannot bypass impersonation:
//   - Without this: Users with secret access could read kubeconfig directly and bypass impersonation
//   - With this: Only mcp-kubernetes reads kubeconfig secrets, impersonation is always enforced
//
// # CAPI Discovery
//
// CAPI cluster discovery uses ServiceAccount credentials because:
//   - Users need to discover clusters to use multi-cluster tools
//   - Granting every user cluster-scoped CAPI permissions is impractical
//   - The ServiceAccount can list all clusters, then filter by user's namespace permissions
//
// # Audit Trail
//
// All privileged access is logged with the user identity that triggered it,
// ensuring accountability and traceability in audit logs.
type PrivilegedAccessProvider interface {
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

	// GetPrivilegedDynamicClient returns a dynamic client using ServiceAccount credentials.
	// This client is used for CAPI cluster discovery operations.
	//
	// # Security
	//
	// - The returned client uses the pod's ServiceAccount credentials
	// - This enables CAPI cluster discovery without granting users cluster-scoped CAPI permissions
	// - The user parameter is used for audit logging only, not for authentication
	// - The returned client should ONLY be used for CAPI cluster listing/discovery
	//
	// Parameters:
	//   - ctx: Context for the request
	//   - user: User identity for audit logging (who triggered this access)
	//
	// Returns:
	//   - dynamic.Interface: Dynamic client for CAPI operations
	//   - error: Any error during client creation
	GetPrivilegedDynamicClient(ctx context.Context, user *UserInfo) (dynamic.Interface, error)

	// PrivilegedCAPIDiscovery returns whether CAPI cluster discovery uses
	// ServiceAccount credentials instead of user credentials.
	//
	// This setting acts as a cluster visibility switch:
	//   - true (default): All users can discover all CAPI clusters. Access
	//     control is enforced at the workload cluster level via impersonation
	//     or SSO passthrough. Maps to CredentialModeFullPrivileged.
	//   - false: Users can only discover clusters they have RBAC to list.
	//     This provides per-user cluster filtering at the MC level.
	//     Maps to CredentialModePrivilegedSecrets.
	PrivilegedCAPIDiscovery() bool

	// IsStrictMode returns true if strict privileged access mode is enabled.
	// When strict mode is enabled, the Manager will NOT fall back to user
	// credentials if ServiceAccount access fails at runtime. Instead, it
	// returns an error.
	IsStrictMode() bool
}

// Default rate limiting values for privileged secret access
const (
	// DefaultPrivilegedAccessRateLimit is the default rate limit per user (requests per second)
	DefaultPrivilegedAccessRateLimit = 10.0

	// DefaultPrivilegedAccessBurst is the default burst size per user
	DefaultPrivilegedAccessBurst = 20

	// DefaultRateLimiterCleanupInterval is how often to clean up expired rate limiters
	DefaultRateLimiterCleanupInterval = 5 * time.Minute

	// DefaultRateLimiterExpiry is how long to keep a user's rate limiter after last use
	DefaultRateLimiterExpiry = 10 * time.Minute
)

// userRateLimiter tracks rate limiting state for a single user
type userRateLimiter struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// HybridOAuthClientProvider implements PrivilegedAccessProvider for OAuth downstream
// authentication with split credential model.
//
// This provider wraps two credential sources: user OAuth tokens for user-scoped operations
// (workload cluster operations with impersonation) and ServiceAccount credentials for privileged
// operations (kubeconfig secret access and CAPI cluster discovery). This split-credential model
// prevents users from extracting admin kubeconfig credentials via kubectl and bypassing impersonation.
//
// See docs/rbac-security.md for the complete security model, rate limiting configuration,
// and audit trail details.
type HybridOAuthClientProvider struct {
	// userProvider handles user-scoped operations using OAuth tokens
	userProvider *OAuthClientProvider

	// saClientset is the ServiceAccount client for privileged secret access
	saClientset kubernetes.Interface

	// saDynamicClient is the ServiceAccount dynamic client for CAPI discovery
	saDynamicClient dynamic.Interface

	// logger for audit trail and operational logging
	logger *slog.Logger

	// initOnce ensures lazy initialization happens only once
	initOnce sync.Once

	// initErr captures any error from lazy initialization
	initErr error

	// configProvider creates the in-cluster config (allows testing)
	configProvider InClusterConfigProvider

	// strictPrivilegedAccess when true, fails instead of falling back to user credentials
	strictPrivilegedAccess bool

	// privilegedCAPIDiscovery when true, uses ServiceAccount for CAPI discovery
	privilegedCAPIDiscovery bool

	// metrics for recording privileged access events
	metrics PrivilegedAccessMetricsRecorder

	// Rate limiting for privileged access
	rateLimitPerSecond float64
	rateLimitBurst     int
	rateLimiters       map[string]*userRateLimiter
	rateLimitersMu     sync.RWMutex
	rateLimiterExpiry  time.Duration

	// Background cleanup for rate limiters
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
	cleanupOnce     sync.Once
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

	// StrictPrivilegedAccess when true, causes GetPrivilegedClientForSecrets to fail
	// instead of falling back to user credentials when ServiceAccount access is unavailable.
	//
	// # Security Consideration
	//
	// Enable this in production environments where the split-credential model is required.
	// When disabled (default), the system falls back to user credentials, which may require
	// users to have secret read permissions (weaker security model).
	//
	// Default: false (fallback enabled for backward compatibility)
	StrictPrivilegedAccess bool

	// PrivilegedCAPIDiscovery controls whether CAPI cluster discovery uses
	// ServiceAccount credentials instead of user credentials. This acts as
	// the cluster visibility switch:
	//
	//   - true (default): All users can discover all CAPI clusters.
	//     Access control is enforced at the workload cluster level.
	//   - false: Users can only discover clusters they have explicit RBAC
	//     to list, providing per-user cluster visibility filtering.
	//
	// Default: true
	PrivilegedCAPIDiscovery *bool

	// Metrics recorder for privileged access events (optional)
	// When configured, records success/failure/rate_limited/fallback events
	Metrics PrivilegedAccessMetricsRecorder

	// RateLimitPerSecond is the rate limit for privileged access per user (requests/second)
	// Default: 10.0
	RateLimitPerSecond float64

	// RateLimitBurst is the burst size for privileged access per user
	// Default: 20
	RateLimitBurst int

	// RateLimiterCleanupInterval is how often to clean up expired rate limiters
	// Default: 5 minutes
	RateLimiterCleanupInterval time.Duration

	// RateLimiterExpiry is how long to keep a user's rate limiter after last use
	// Default: 10 minutes
	RateLimiterExpiry time.Duration
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

	// Apply rate limiting defaults
	rateLimitPerSecond := config.RateLimitPerSecond
	if rateLimitPerSecond <= 0 {
		rateLimitPerSecond = DefaultPrivilegedAccessRateLimit
	}

	rateLimitBurst := config.RateLimitBurst
	if rateLimitBurst <= 0 {
		rateLimitBurst = DefaultPrivilegedAccessBurst
	}

	cleanupInterval := config.RateLimiterCleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = DefaultRateLimiterCleanupInterval
	}

	rateLimiterExpiry := config.RateLimiterExpiry
	if rateLimiterExpiry <= 0 {
		rateLimiterExpiry = DefaultRateLimiterExpiry
	}

	// Default privileged CAPI discovery to true
	privilegedCAPIDiscovery := true
	if config.PrivilegedCAPIDiscovery != nil {
		privilegedCAPIDiscovery = *config.PrivilegedCAPIDiscovery
	}

	p := &HybridOAuthClientProvider{
		userProvider:            config.UserProvider,
		logger:                  logger,
		configProvider:          configProvider,
		strictPrivilegedAccess:  config.StrictPrivilegedAccess,
		privilegedCAPIDiscovery: privilegedCAPIDiscovery,
		metrics:                 config.Metrics,
		rateLimitPerSecond:      rateLimitPerSecond,
		rateLimitBurst:          rateLimitBurst,
		rateLimiters:            make(map[string]*userRateLimiter),
		cleanupInterval:         cleanupInterval,
		rateLimiterExpiry:       rateLimiterExpiry,
		stopCleanup:             make(chan struct{}),
	}

	// Start background cleanup goroutine
	go p.rateLimiterCleanupLoop()

	return p, nil
}

// Close stops the background cleanup goroutine and releases resources.
func (p *HybridOAuthClientProvider) Close() {
	p.cleanupOnce.Do(func() {
		close(p.stopCleanup)
	})
}

// RateLimitPerSecond returns the configured rate limit per second (after applying defaults).
func (p *HybridOAuthClientProvider) RateLimitPerSecond() float64 {
	return p.rateLimitPerSecond
}

// RateLimitBurst returns the configured burst size (after applying defaults).
func (p *HybridOAuthClientProvider) RateLimitBurst() int {
	return p.rateLimitBurst
}

// PrivilegedCAPIDiscovery returns whether privileged CAPI discovery is enabled.
func (p *HybridOAuthClientProvider) PrivilegedCAPIDiscovery() bool {
	return p.privilegedCAPIDiscovery
}

// rateLimiterCleanupLoop periodically removes expired rate limiters.
func (p *HybridOAuthClientProvider) rateLimiterCleanupLoop() {
	ticker := time.NewTicker(p.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCleanup:
			return
		case <-ticker.C:
			p.cleanupExpiredRateLimiters()
		}
	}
}

// cleanupExpiredRateLimiters removes rate limiters that haven't been used recently.
func (p *HybridOAuthClientProvider) cleanupExpiredRateLimiters() {
	p.rateLimitersMu.Lock()
	defer p.rateLimitersMu.Unlock()

	now := time.Now()
	expiredCount := 0
	for userHash, rl := range p.rateLimiters {
		if now.Sub(rl.lastAccess) > p.rateLimiterExpiry {
			delete(p.rateLimiters, userHash)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		p.logger.Debug("Cleaned up expired rate limiters",
			"expired_count", expiredCount,
			"remaining", len(p.rateLimiters))
	}
}

// getRateLimiter returns or creates a rate limiter for the given user.
func (p *HybridOAuthClientProvider) getRateLimiter(userEmail string) *rate.Limiter {
	userHash := AnonymizeEmail(userEmail)

	p.rateLimitersMu.Lock()
	defer p.rateLimitersMu.Unlock()

	rl, exists := p.rateLimiters[userHash]
	if !exists {
		rl = &userRateLimiter{
			limiter:    rate.NewLimiter(rate.Limit(p.rateLimitPerSecond), p.rateLimitBurst),
			lastAccess: time.Now(),
		}
		p.rateLimiters[userHash] = rl
	} else {
		rl.lastAccess = time.Now()
	}

	return rl.limiter
}

// IsStrictMode returns true if strict privileged access mode is enabled.
func (p *HybridOAuthClientProvider) IsStrictMode() bool {
	return p.strictPrivilegedAccess
}

// Privileged access operation constants for metrics.
const (
	// PrivilegedOperationSecretAccess is the operation label for kubeconfig secret access.
	PrivilegedOperationSecretAccess = "secret_access"

	// PrivilegedOperationCAPIDiscovery is the operation label for CAPI cluster discovery.
	PrivilegedOperationCAPIDiscovery = "capi_discovery"
)

// recordMetric safely records a privileged access metric if metrics are configured.
// This is used internally for success/error/rate_limited events within the provider.
// Fallback metrics are recorded separately by the Manager via recordPrivilegedMetric.
func (p *HybridOAuthClientProvider) recordMetric(ctx context.Context, userEmail, operation, result string) {
	if p.metrics != nil {
		userDomain := extractUserDomain(userEmail)
		p.metrics.RecordPrivilegedAccess(ctx, userDomain, operation, result)
	}
}

// unknownDomain is returned when the user's email domain cannot be determined.
const unknownDomain = "unknown_domain"

// extractUserDomain extracts the domain from an email address.
func extractUserDomain(email string) string {
	if email == "" {
		return unknownDomain
	}
	parts := splitEmail(email)
	if len(parts) != 2 {
		return unknownDomain
	}
	return parts[1]
}

// splitEmail splits an email into local and domain parts.
func splitEmail(email string) []string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return []string{email[:i], email[i+1:]}
		}
	}
	return []string{email}
}

// GetClientsForUser returns Kubernetes clients authenticated with the user's OAuth token.
// This delegates to the underlying OAuthClientProvider.
func (p *HybridOAuthClientProvider) GetClientsForUser(ctx context.Context, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	return p.userProvider.GetClientsForUser(ctx, user)
}

// ErrRateLimited is returned when a user exceeds the rate limit for privileged access.
var ErrRateLimited = fmt.Errorf("rate limit exceeded for privileged access")

// GetPrivilegedClientForSecrets returns the ServiceAccount client for secret access.
//
// # Security
//
// This method:
//  1. Checks rate limiting per user (prevents abuse)
//  2. Lazily initializes the ServiceAccount client on first call
//  3. Logs the access for audit purposes (with user identity)
//  4. Records metrics for monitoring
//  5. Returns a client that should ONLY be used for kubeconfig secret retrieval
//
// The audit log entry includes the user identity to track who triggered the access,
// even though the actual Kubernetes API call uses ServiceAccount credentials.
func (p *HybridOAuthClientProvider) GetPrivilegedClientForSecrets(ctx context.Context, user *UserInfo) (kubernetes.Interface, error) {
	// Check rate limit for this user
	limiter := p.getRateLimiter(user.Email)
	if !limiter.Allow() {
		p.logger.Warn("Privileged secret access rate limited",
			UserHashAttr(user.Email),
			"operation", PrivilegedOperationSecretAccess)
		p.recordMetric(ctx, user.Email, PrivilegedOperationSecretAccess, "rate_limited")
		return nil, ErrRateLimited
	}

	// Ensure ServiceAccount client is initialized
	if err := p.initServiceAccountClient(); err != nil {
		p.recordMetric(ctx, user.Email, PrivilegedOperationSecretAccess, "error")
		return nil, fmt.Errorf("failed to initialize ServiceAccount client: %w", err)
	}

	// Log the privileged access for audit trail.
	// Use Debug level because secret access is high-frequency in CAPI mode
	// (triggered on every workload cluster operation that needs a kubeconfig).
	p.logger.Debug("Privileged secret access initiated",
		UserHashAttr(user.Email),
		"operation", PrivilegedOperationSecretAccess)

	p.recordMetric(ctx, user.Email, PrivilegedOperationSecretAccess, "success")
	return p.saClientset, nil
}

// ErrPrivilegedCAPIDiscoveryDisabled is returned when privileged CAPI discovery is disabled.
var ErrPrivilegedCAPIDiscoveryDisabled = fmt.Errorf("privileged CAPI discovery is disabled")

// GetPrivilegedDynamicClient returns the ServiceAccount dynamic client for CAPI discovery.
//
// # Security
//
// This method:
//  1. Checks if privileged CAPI discovery is enabled
//  2. Checks rate limiting per user (prevents abuse)
//  3. Lazily initializes the ServiceAccount dynamic client on first call
//  4. Logs the access for audit purposes (with user identity)
//  5. Records metrics for monitoring
//  6. Returns a client that should ONLY be used for CAPI cluster discovery
//
// The audit log entry includes the user identity to track who triggered the access,
// even though the actual Kubernetes API call uses ServiceAccount credentials.
func (p *HybridOAuthClientProvider) GetPrivilegedDynamicClient(ctx context.Context, user *UserInfo) (dynamic.Interface, error) {
	// Check if privileged CAPI discovery is enabled
	if !p.privilegedCAPIDiscovery {
		return nil, ErrPrivilegedCAPIDiscoveryDisabled
	}

	// Check rate limit for this user
	limiter := p.getRateLimiter(user.Email)
	if !limiter.Allow() {
		p.logger.Warn("Privileged CAPI access rate limited",
			UserHashAttr(user.Email),
			"operation", PrivilegedOperationCAPIDiscovery)
		p.recordMetric(ctx, user.Email, PrivilegedOperationCAPIDiscovery, "rate_limited")
		return nil, ErrRateLimited
	}

	// Ensure ServiceAccount clients are initialized
	if err := p.initServiceAccountClient(); err != nil {
		p.recordMetric(ctx, user.Email, PrivilegedOperationCAPIDiscovery, "error")
		return nil, fmt.Errorf("failed to initialize ServiceAccount client: %w", err)
	}

	// Log the privileged access for audit trail.
	// Use Debug level because CAPI discovery is high-frequency (every ListClusters/ResolveCluster call).
	p.logger.Debug("Privileged CAPI access initiated",
		UserHashAttr(user.Email),
		"operation", PrivilegedOperationCAPIDiscovery)

	p.recordMetric(ctx, user.Email, PrivilegedOperationCAPIDiscovery, "success")
	return p.saDynamicClient, nil
}

// initServiceAccountClient lazily initializes the ServiceAccount clients.
// This is called on first use to avoid startup errors when running outside a cluster.
func (p *HybridOAuthClientProvider) initServiceAccountClient() error {
	p.initOnce.Do(func() {
		p.logger.Debug("Initializing ServiceAccount clients for privileged access")

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

		// Create dynamic client for CAPI discovery
		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			// Clear clientset to avoid partial initialization state.
			// sync.Once ensures this function only runs once, so a failure here
			// is permanent -- subsequent calls return initErr without retrying.
			p.saClientset = nil
			p.initErr = fmt.Errorf("failed to create dynamic client: %w", err)
			p.logger.Error("Failed to create ServiceAccount dynamic client",
				"error", p.initErr)
			return
		}
		p.saDynamicClient = dynamicClient

		p.logger.Info("ServiceAccount clients initialized for privileged access")
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

// SetPrivilegedAccessMetrics sets the metrics recorder for privileged access events.
func (p *HybridOAuthClientProvider) SetPrivilegedAccessMetrics(metrics PrivilegedAccessMetricsRecorder) {
	p.metrics = metrics
}

// Ensure HybridOAuthClientProvider implements PrivilegedAccessProvider.
var _ PrivilegedAccessProvider = (*HybridOAuthClientProvider)(nil)

// Ensure HybridOAuthClientProvider implements ClientProvider.
var _ ClientProvider = (*HybridOAuthClientProvider)(nil)
