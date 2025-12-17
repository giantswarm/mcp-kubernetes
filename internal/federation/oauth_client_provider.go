// Package federation provides OAuth client provider implementation.
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

// UserExtraOAuthTokenKey is the key used in UserInfo.Extra to store OAuth tokens
// as a fallback mechanism for testing or alternative authentication flows.
// nolint:gosec // G101: This is a key name, not a credential
const UserExtraOAuthTokenKey = "oauth_token"

// OAuthClientProvider implements ClientProvider for OAuth downstream authentication.
// It creates per-user Kubernetes clients using the user's OAuth bearer token,
// ensuring all API operations are performed with the user's RBAC permissions.
//
// # Security Model
//
// When OAuth downstream is enabled, users authenticate to mcp-kubernetes via OAuth
// (e.g., through Dex or Google). Their OAuth token is then used directly for all
// Kubernetes API calls to both the Management Cluster and Workload Clusters.
//
// This means:
//   - The service account's RBAC permissions are NOT used for API operations
//   - Each user can only perform actions they are authorized for via their own RBAC
//   - Audit logs show the actual user identity, not the service account
//
// The service account is only used for:
//   - Pod lifecycle (mounting the projected token for potential fallback)
//   - Network connectivity to the API server
//
// # Usage
//
//	config := &OAuthClientProviderConfig{
//	    ClusterHost: "https://kubernetes.default.svc",
//	    CACertFile:  "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
//	    QPS:         50,
//	    Burst:       100,
//	    Timeout:     30 * time.Second,
//	}
//	provider, err := NewOAuthClientProvider(config)
//	if err != nil {
//	    return err
//	}
//	manager, err := NewManager(provider)
//	if err != nil {
//	    return err
//	}
//	defer manager.Close()
//
// OAuthAuthMetricsRecorder provides an interface for recording OAuth authentication metrics.
// This is used by OAuthClientProvider to track authentication success/failure rates.
type OAuthAuthMetricsRecorder interface {
	// RecordOAuthDownstreamAuth records an OAuth downstream authentication attempt.
	// result should be one of: "success", "fallback", "failure", "no_token"
	RecordOAuthDownstreamAuth(ctx context.Context, result string)
}

type OAuthClientProvider struct {
	// In-cluster configuration
	clusterHost string
	caCertFile  string

	// Client configuration
	qps     float32
	burst   int
	timeout time.Duration

	// Token extractor function for getting OAuth tokens from context
	tokenExtractor TokenExtractor

	// tokenExtractorOnce ensures SetTokenExtractor can only be called once.
	// This is a security measure to prevent runtime swapping of the extractor
	// which could lead to authentication bypass.
	tokenExtractorOnce sync.Once

	// metrics records OAuth authentication success/failure for monitoring
	metrics OAuthAuthMetricsRecorder
}

// OAuthClientProviderConfig contains configuration for creating an OAuthClientProvider.
type OAuthClientProviderConfig struct {
	// ClusterHost is the Kubernetes API server URL (e.g., "https://kubernetes.default.svc").
	ClusterHost string

	// CACertFile is the path to the CA certificate for TLS verification.
	CACertFile string

	// QPS is the queries per second rate limit for the Kubernetes client.
	QPS float32

	// Burst is the burst limit for the Kubernetes client.
	Burst int

	// Timeout is the request timeout for API calls.
	Timeout time.Duration
}

// DefaultOAuthClientProviderConfig returns a configuration with sensible defaults.
func DefaultOAuthClientProviderConfig() *OAuthClientProviderConfig {
	return &OAuthClientProviderConfig{
		ClusterHost: "https://kubernetes.default.svc",
		CACertFile:  "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		QPS:         50,
		Burst:       100,
		Timeout:     30 * time.Second,
	}
}

// NewOAuthClientProvider creates a new OAuthClientProvider from in-cluster configuration.
// It reads the cluster host and CA certificate from the standard in-cluster paths.
func NewOAuthClientProvider(config *OAuthClientProviderConfig) (*OAuthClientProvider, error) {
	if config == nil {
		config = DefaultOAuthClientProviderConfig()
	}

	// Validate required fields
	if config.ClusterHost == "" {
		return nil, fmt.Errorf("cluster host is required")
	}
	if config.CACertFile == "" {
		return nil, fmt.Errorf("CA cert file is required")
	}

	return &OAuthClientProvider{
		clusterHost: config.ClusterHost,
		caCertFile:  config.CACertFile,
		qps:         config.QPS,
		burst:       config.Burst,
		timeout:     config.Timeout,
	}, nil
}

// NewOAuthClientProviderFromInCluster creates an OAuthClientProvider using
// the in-cluster configuration. This is the typical way to create the provider
// when running inside a Kubernetes pod.
func NewOAuthClientProviderFromInCluster() (*OAuthClientProvider, error) {
	// Get in-cluster config to extract host and CA cert path
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	config := &OAuthClientProviderConfig{
		ClusterHost: inClusterConfig.Host,
		CACertFile:  "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		QPS:         50,
		Burst:       100,
		Timeout:     30 * time.Second,
	}

	return NewOAuthClientProvider(config)
}

// TokenExtractor is a function type for extracting OAuth tokens from context.
// This allows for dependency injection of token extraction logic.
type TokenExtractor func(ctx context.Context) (string, bool)

// SetTokenExtractor sets the token extractor function for the provider.
// This should be called after creating the provider to configure how tokens
// are extracted from context.
//
// # Security: Immutable After First Set
//
// This method can only be called once per provider instance. Subsequent calls
// will be ignored and a warning will be logged. This prevents runtime swapping
// of the token extractor which could lead to authentication bypass or confusion
// about which tokens are being used.
//
// If you need to change the extractor, create a new OAuthClientProvider instance.
func (p *OAuthClientProvider) SetTokenExtractor(extractor TokenExtractor) {
	p.tokenExtractorOnce.Do(func() {
		p.tokenExtractor = extractor
	})
	// Check if the extractor was actually set (Do only runs once)
	if p.tokenExtractor != nil && fmt.Sprintf("%p", p.tokenExtractor) != fmt.Sprintf("%p", extractor) {
		slog.Warn("SetTokenExtractor called multiple times; subsequent calls are ignored for security")
	}
}

// SetMetrics sets the metrics recorder for tracking authentication success/failure.
// This should be called during initialization to enable metrics collection.
func (p *OAuthClientProvider) SetMetrics(metrics OAuthAuthMetricsRecorder) {
	p.metrics = metrics
}

// GetClientsForUser returns Kubernetes clients authenticated with the user's OAuth token.
// The user's OAuth token is extracted from context using the configured TokenExtractor
// and used as the bearer token for all API requests.
//
// This method creates fresh clients for each call. The federation Manager handles
// caching of these clients per (cluster, user) pair.
//
// # Metrics
//
// If metrics are configured via SetMetrics, this method records authentication outcomes:
//   - "success": Token extracted from context successfully
//   - "fallback": Token obtained from user.Extra (testing/alternative flows)
//   - "no_token": No token available in context or user.Extra
//   - "failure": Client creation failed after token extraction
func (p *OAuthClientProvider) GetClientsForUser(ctx context.Context, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	if user == nil {
		p.recordMetric(ctx, "failure")
		return nil, nil, nil, fmt.Errorf("user info is required")
	}

	// Extract OAuth token from context using the configured extractor
	var bearerToken string
	var tokenSource string
	if p.tokenExtractor != nil {
		token, ok := p.tokenExtractor(ctx)
		if ok && token != "" {
			bearerToken = token
			tokenSource = "context"
		}
	}

	// Fallback: check if token is in user.Extra (for testing or alternative flows)
	if bearerToken == "" && user.Extra != nil {
		if tokens, ok := user.Extra[UserExtraOAuthTokenKey]; ok && len(tokens) > 0 {
			bearerToken = tokens[0]
			tokenSource = "fallback"
		}
	}

	if bearerToken == "" {
		p.recordMetric(ctx, "no_token")
		return nil, nil, nil, fmt.Errorf("OAuth token not found in context or user info")
	}

	// Record how the token was obtained
	if tokenSource == "fallback" {
		p.recordMetric(ctx, "fallback")
	}

	// Create REST config with user's bearer token
	restConfig := &rest.Config{
		Host:        p.clusterHost,
		BearerToken: bearerToken,
		TLSClientConfig: rest.TLSClientConfig{
			CAFile: p.caCertFile,
		},
		QPS:     p.qps,
		Burst:   p.burst,
		Timeout: p.timeout,
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		p.recordMetric(ctx, "failure")
		return nil, nil, nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		p.recordMetric(ctx, "failure")
		return nil, nil, nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Record success metric (only if token came from context, not fallback)
	if tokenSource == "context" {
		p.recordMetric(ctx, "success")
	}

	return clientset, dynamicClient, restConfig, nil
}

// recordMetric safely records an OAuth authentication metric if metrics are configured.
func (p *OAuthClientProvider) recordMetric(ctx context.Context, result string) {
	if p.metrics != nil {
		p.metrics.RecordOAuthDownstreamAuth(ctx, result)
	}
}

// Ensure OAuthClientProvider implements ClientProvider.
var _ ClientProvider = (*OAuthClientProvider)(nil)
