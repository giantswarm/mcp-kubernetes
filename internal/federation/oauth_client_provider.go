// Package federation provides OAuth client provider implementation.
package federation

import (
	"context"
	"fmt"
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
func (p *OAuthClientProvider) SetTokenExtractor(extractor TokenExtractor) {
	p.tokenExtractor = extractor
}

// GetClientsForUser returns Kubernetes clients authenticated with the user's OAuth token.
// The user's OAuth token is extracted from context using the configured TokenExtractor
// and used as the bearer token for all API requests.
//
// This method creates fresh clients for each call. The federation Manager handles
// caching of these clients per (cluster, user) pair.
func (p *OAuthClientProvider) GetClientsForUser(ctx context.Context, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	if user == nil {
		return nil, nil, nil, fmt.Errorf("user info is required")
	}

	// Extract OAuth token from context using the configured extractor
	var bearerToken string
	if p.tokenExtractor != nil {
		token, ok := p.tokenExtractor(ctx)
		if ok && token != "" {
			bearerToken = token
		}
	}

	// Fallback: check if token is in user.Extra (for testing or alternative flows)
	if bearerToken == "" && user.Extra != nil {
		if tokens, ok := user.Extra[UserExtraOAuthTokenKey]; ok && len(tokens) > 0 {
			bearerToken = tokens[0]
		}
	}

	if bearerToken == "" {
		return nil, nil, nil, fmt.Errorf("OAuth token not found in context or user info")
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
		return nil, nil, nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return clientset, dynamicClient, restConfig, nil
}

// Ensure OAuthClientProvider implements ClientProvider.
var _ ClientProvider = (*OAuthClientProvider)(nil)
