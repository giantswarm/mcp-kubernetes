package federation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// DefaultConnectionValidationTimeout is the default timeout for validating cluster connectivity.
// This can be overridden using WithManagerConnectionValidationTimeout.
const DefaultConnectionValidationTimeout = 10 * time.Second

// ClusterInfo contains information about a CAPI cluster needed for kubeconfig retrieval.
type ClusterInfo struct {
	// Name is the cluster name.
	Name string

	// Namespace is the namespace where the cluster resource and its kubeconfig secret reside.
	Namespace string
}

// GetKubeconfigForCluster retrieves the kubeconfig secret for a CAPI cluster
// and returns a rest.Config suitable for creating clients.
//
// # Security Model
//
// This method implements a split-credential model for enhanced security:
//
// When PrivilegedSecretAccessProvider is available:
//  1. Finds the Cluster resource using SERVICEACCOUNT credentials (privileged CAPI discovery)
//  2. Fetches the kubeconfig secret using SERVICEACCOUNT credentials (privileged)
//  3. Parses the kubeconfig into a rest.Config
//
// This prevents users from bypassing impersonation:
//   - Users can discover clusters without needing cluster-scoped CAPI permissions
//   - But they cannot extract kubeconfig secrets via kubectl
//   - mcp-kubernetes reads secrets using ServiceAccount credentials
//   - All workload cluster operations enforce impersonation
//
// When only basic ClientProvider is available (fallback mode):
//  1. Finds the Cluster resource using user's dynamic client (RBAC enforced)
//  2. Fetches the kubeconfig secret using user's client (RBAC enforced)
//  3. User must have RBAC permission to read secrets and list CAPI clusters
//
// # Audit Trail
//
// All privileged access is logged with the user identity for accountability.
//
// Security notes:
//   - Never logs kubeconfig contents (sensitive credential data)
//   - All user-facing errors are sanitized to prevent information leakage
func (m *Manager) GetKubeconfigForCluster(ctx context.Context, clusterName string, user *UserInfo) (*rest.Config, error) {
	// Validate inputs at API boundary (defense in depth)
	// This ensures validation even if called directly, not just via GetClient/GetDynamicClient
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}
	if err := ValidateClusterName(clusterName); err != nil {
		return nil, err
	}

	// Fail fast if context is already cancelled or expired
	if err := ctx.Err(); err != nil {
		return nil, &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      "context cancelled or expired",
		}
	}

	// Get a dynamic client for CAPI cluster discovery.
	// This uses the same split-credential strategy as the CAPI tools:
	// 1. Try ServiceAccount credentials (privileged) - no cluster-scoped RBAC needed for user
	// 2. Fall back to user credentials if privileged access is unavailable
	dynamicClient, err := m.getDynamicClientForCAPIDiscovery(ctx, user)
	if err != nil {
		m.logger.Debug("Failed to get dynamic client for CAPI discovery in kubeconfig retrieval",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)

		// Preserve strict-mode sentinel so callers can distinguish policy
		// rejections from transient failures.
		if errors.Is(err, ErrStrictPrivilegedAccessRequired) {
			return nil, err
		}

		return nil, &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      "failed to create client for CAPI discovery",
		}
	}

	// Find the cluster to determine its namespace
	clusterInfo, err := m.findClusterInfo(ctx, clusterName, dynamicClient, user)
	if err != nil {
		return nil, err
	}

	// Determine which client to use for secret retrieval
	secretClient, err := m.getSecretAccessClient(ctx, clusterName, user)
	if err != nil {
		return nil, err
	}

	// Retrieve and parse the kubeconfig secret
	return m.getKubeconfigFromSecret(ctx, clusterInfo, secretClient, user)
}

// ErrStrictPrivilegedAccessRequired is returned when strict mode is enabled and
// privileged access fails.
var ErrStrictPrivilegedAccessRequired = fmt.Errorf("privileged secret access required but unavailable (strict mode enabled)")

// getSecretAccessClient returns the appropriate client for kubeconfig secret access.
//
// The behavior depends on the Manager's credentialMode (resolved at construction):
//
//   - CredentialModeFullPrivileged, CredentialModePrivilegedSecrets: Uses ServiceAccount
//     credentials via GetPrivilegedClientForSecrets. If the ServiceAccount client fails
//     at runtime, falls back to user credentials unless strict mode is enabled.
//
//   - CredentialModeUser: Uses user credentials (user must have RBAC to read secrets).
func (m *Manager) getSecretAccessClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, error) {
	switch m.credentialMode {
	case CredentialModeFullPrivileged, CredentialModePrivilegedSecrets:
		client, err := m.privilegedProvider.GetPrivilegedClientForSecrets(ctx, user)
		if err == nil {
			m.logger.Debug("Using ServiceAccount for privileged secret access",
				"credential_mode", m.credentialMode.String(),
				"cluster", clusterName,
				UserHashAttr(user.Email))
			return client, nil
		}

		// Runtime failure: ServiceAccount client couldn't be created.
		// Fall back to user credentials unless strict mode is enabled.
		if m.privilegedProvider.IsStrictMode() {
			m.logger.Error("Privileged secret access failed in strict mode",
				"credential_mode", m.credentialMode.String(),
				"cluster", clusterName,
				UserHashAttr(user.Email),
				"error", err)
			return nil, ErrStrictPrivilegedAccessRequired
		}

		m.logger.Warn("Privileged secret access failed at runtime, falling back to user credentials",
			"credential_mode", m.credentialMode.String(),
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		m.privilegedProvider.RecordMetric(ctx, user.Email, PrivilegedOperationSecretAccess, "fallback")

		return m.getUserSecretClient(ctx, clusterName, user)

	case CredentialModeUser:
		return m.getUserSecretClient(ctx, clusterName, user)

	default:
		return nil, fmt.Errorf("unknown credential mode: %s", m.credentialMode)
	}
}

// getUserSecretClient returns the user-scoped clientset for secret access.
func (m *Manager) getUserSecretClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, error) {
	m.logger.Debug("Using user credentials for secret access",
		"credential_mode", m.credentialMode.String(),
		"cluster", clusterName,
		UserHashAttr(user.Email))

	clientset, _, _, err := m.clientProvider.GetClientsForUser(ctx, user)
	if err != nil {
		m.logger.Debug("Failed to get user client for secret access",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, fmt.Errorf("failed to create user client for secret access (cluster %s): %w", clusterName, err)
	}

	return clientset, nil
}

// GetKubeconfigForClusterValidated retrieves the kubeconfig and validates
// that the resulting config can establish a connection to the cluster.
//
// This is useful when you want to ensure the credentials are valid before
// caching or using them for operations.
func (m *Manager) GetKubeconfigForClusterValidated(ctx context.Context, clusterName string, user *UserInfo) (*rest.Config, error) {
	config, err := m.GetKubeconfigForCluster(ctx, clusterName, user)
	if err != nil {
		return nil, err
	}

	// Validate the connection
	if err := m.validateClusterConnection(ctx, clusterName, config); err != nil {
		return nil, err
	}

	return config, nil
}

// findClusterInfo locates a CAPI Cluster resource by name and returns its namespace.
// This performs a cluster-wide search since we don't know the namespace upfront.
//
// # Security Model
//
// The caller is responsible for providing the appropriate dynamic client:
//   - When called with a privileged client (from getDynamicClientForCAPIDiscovery),
//     the ServiceAccount credentials are used for discovery, so users don't need
//     cluster-scoped CAPI permissions.
//   - When called with a user-scoped client, the user must have RBAC permission
//     to list Cluster resources.
//
// The cluster name must be validated using ValidateClusterName() before calling
// this method to prevent path traversal or injection attacks.
//
// All user-facing errors are sanitized via UserFacingError() to prevent
// information leakage through error response differentiation.
func (m *Manager) findClusterInfo(ctx context.Context, clusterName string, dynamicClient dynamic.Interface, user *UserInfo) (*ClusterInfo, error) {
	// List all CAPI Cluster resources across all namespaces
	//
	// Note: We don't use FieldSelector because the fake dynamic client doesn't support it well.
	// TODO(performance): In production with many clusters, consider using a FieldSelector
	// (metadata.name=clusterName) or LabelSelector to reduce API server load. This would
	// require updating the test infrastructure to support field selectors in the fake client,
	// or using integration tests with a real API server.
	list, err := dynamicClient.Resource(CAPIClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Debug("Failed to list CAPI Cluster resources",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      fmt.Sprintf("failed to query CAPI clusters: %v", err),
		}
	}

	// Find the cluster by name in the list
	for _, cluster := range list.Items {
		if cluster.GetName() == clusterName {
			namespace := cluster.GetNamespace()
			m.logger.Debug("Found CAPI Cluster",
				"cluster", clusterName,
				"namespace", namespace,
				UserHashAttr(user.Email))
			return &ClusterInfo{
				Name:      clusterName,
				Namespace: namespace,
			}, nil
		}
	}

	m.logger.Debug("CAPI Cluster not found",
		"cluster", clusterName,
		UserHashAttr(user.Email))
	return nil, &ClusterNotFoundError{
		ClusterName: clusterName,
		Reason:      "no CAPI Cluster resource found with this name",
	}
}

// getKubeconfigFromSecret retrieves the kubeconfig data from the cluster's secret.
//
// # Security Model
//
// This method uses the user's client, ensuring:
//   - User must have RBAC permission to get Secrets in the cluster's namespace
//   - Only secrets the user has access to can be retrieved
//   - Defense in depth: MC RBAC is enforced for secret access
//
// By CAPI convention, the secret is named ${CLUSTER_NAME}-kubeconfig and contains
// the kubeconfig data in either the 'value' or 'kubeconfig' key.
func (m *Manager) getKubeconfigFromSecret(ctx context.Context, info *ClusterInfo, clientset kubernetes.Interface, user *UserInfo) (*rest.Config, error) {
	secretName := info.Name + CAPISecretSuffix

	// Fetch the secret using the user's client (RBAC enforced)
	secret, err := clientset.CoreV1().Secrets(info.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		m.logger.Debug("Failed to fetch kubeconfig secret",
			"cluster", info.Name,
			"namespace", info.Namespace,
			"secret", secretName,
			UserHashAttr(user.Email),
			"error", err)

		// Determine if this is a "not found" or "forbidden" error
		return nil, &KubeconfigError{
			ClusterName:  info.Name,
			ResourceName: secretName,
			Namespace:    info.Namespace,
			Reason:       "failed to fetch secret",
			Err:          err,
			NotFound:     isNotFoundError(err),
		}
	}

	// Extract kubeconfig data from the secret
	kubeconfigData, err := m.extractKubeconfigData(secret.Data, info, secretName)
	if err != nil {
		return nil, err
	}

	// Parse the kubeconfig YAML into a rest.Config
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		m.logger.Debug("Failed to parse kubeconfig data",
			"cluster", info.Name,
			"namespace", info.Namespace,
			UserHashAttr(user.Email),
			"error", err)
		return nil, &KubeconfigError{
			ClusterName:  info.Name,
			ResourceName: secretName,
			Namespace:    info.Namespace,
			Reason:       "invalid kubeconfig data",
			Err:          err,
			NotFound:     false,
		}
	}

	// Log success without exposing sensitive data
	m.logger.Debug("Successfully parsed kubeconfig",
		"cluster", info.Name,
		"namespace", info.Namespace,
		UserHashAttr(user.Email),
		"host", sanitizeHost(config.Host))

	return config, nil
}

// extractKubeconfigData extracts the kubeconfig YAML from secret data.
// It tries the primary key ('value') first, then falls back to the alternate key ('kubeconfig').
func (m *Manager) extractKubeconfigData(data map[string][]byte, info *ClusterInfo, secretName string) ([]byte, error) {
	// Try the primary key first (CAPI standard)
	if kubeconfigData, ok := data[CAPISecretKey]; ok && len(kubeconfigData) > 0 {
		m.logger.Debug("Found kubeconfig data using primary key",
			"cluster", info.Name,
			"key", CAPISecretKey)
		return kubeconfigData, nil
	}

	// Try the alternate key (used by some providers)
	if kubeconfigData, ok := data[CAPISecretKeyAlternate]; ok && len(kubeconfigData) > 0 {
		m.logger.Debug("Found kubeconfig data using alternate key",
			"cluster", info.Name,
			"key", CAPISecretKeyAlternate)
		return kubeconfigData, nil
	}

	// Neither key found
	m.logger.Debug("Kubeconfig secret missing expected keys",
		"cluster", info.Name,
		"namespace", info.Namespace,
		"available_keys", getSecretKeys(data))
	return nil, &KubeconfigError{
		ClusterName:  info.Name,
		ResourceName: secretName,
		Namespace:    info.Namespace,
		Reason:       fmt.Sprintf("secret missing '%s' or '%s' key", CAPISecretKey, CAPISecretKeyAlternate),
		NotFound:     false,
	}
}

// validateClusterConnection attempts to connect to the cluster API server
// to verify the kubeconfig is valid and the cluster is reachable.
func (m *Manager) validateClusterConnection(ctx context.Context, clusterName string, config *rest.Config) error {
	// Create a validation context with timeout (configurable via WithManagerConnectionValidationTimeout)
	validationCtx, cancel := context.WithTimeout(ctx, m.connectionValidationTimeout)
	defer cancel()

	// Set up the config for REST client creation
	// We need to set GroupVersion and NegotiatedSerializer for the REST client
	configCopy := rest.CopyConfig(config)
	configCopy.APIPath = "/api"
	configCopy.GroupVersion = &schema.GroupVersion{Version: "v1"}
	configCopy.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	// Create a minimal REST client for validation
	restClient, err := rest.RESTClientFor(configCopy)
	if err != nil {
		m.logger.Debug("Failed to create REST client for validation",
			"cluster", clusterName,
			"error", err)
		return &ConnectionError{
			ClusterName: clusterName,
			Host:        sanitizeHost(config.Host),
			Reason:      "failed to create REST client",
			Err:         err,
		}
	}

	// Perform a simple health check (GET /healthz)
	result := restClient.Get().AbsPath("/healthz").Do(validationCtx)
	if err := result.Error(); err != nil {
		m.logger.Debug("Cluster health check failed",
			"cluster", clusterName,
			"host", sanitizeHost(config.Host),
			"error", err)
		return &ConnectionError{
			ClusterName: clusterName,
			Host:        sanitizeHost(config.Host),
			Reason:      "health check failed",
			Err:         err,
		}
	}

	m.logger.Debug("Cluster connection validated",
		"cluster", clusterName,
		"host", sanitizeHost(config.Host))

	return nil
}

// isNotFoundError checks if the error is a Kubernetes "not found" error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Use the standard Kubernetes API machinery to check for not found errors
	return apierrors.IsNotFound(err)
}

// sanitizeHost returns a sanitized version of the host for logging purposes.
// This function redacts IP addresses to prevent sensitive network topology
// information from appearing in logs, while preserving enough context for debugging.
//
// # Security Considerations
//
// IP addresses can reveal internal network topology, VPC CIDR ranges, and
// infrastructure details. This function replaces IP addresses with a redacted
// placeholder while preserving:
//   - The URL scheme (https://)
//   - The port number (useful for debugging connectivity issues)
//   - Hostnames (generally safe and needed for debugging)
//
// Examples:
//   - "https://10.0.1.50:6443" -> "https://[redacted-ip]:6443"
//   - "https://api.cluster.example.com:6443" -> "https://api.cluster.example.com:6443"
//   - "" -> "<empty>"
func sanitizeHost(host string) string {
	if host == "" {
		return "<empty>"
	}

	// Extract scheme if present
	scheme := ""
	hostPart := host
	if strings.HasPrefix(host, "https://") {
		scheme = "https://"
		hostPart = strings.TrimPrefix(host, "https://")
	} else if strings.HasPrefix(host, "http://") {
		scheme = "http://"
		hostPart = strings.TrimPrefix(host, "http://")
	}

	// Split host and port using net.SplitHostPort for proper IPv4/IPv6 handling
	hostOnly, port, err := net.SplitHostPort(hostPart)
	if err != nil {
		// No port in the host, try to parse the whole thing as IP
		hostOnly = hostPart
		port = ""
	}

	// Check if the host is an IP address
	ip := net.ParseIP(hostOnly)
	if ip != nil {
		// Redact the IP address
		redacted := "[redacted-ip]"
		if port != "" {
			return scheme + redacted + ":" + port
		}
		return scheme + redacted
	}

	// Not an IP address, return the original (hostnames are generally safe)
	return host
}

// getSecretKeys returns a slice of key names from secret data for debugging.
// This is safe to log as it only returns key names, not values.
func getSecretKeys(data map[string][]byte) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}

// ConfigWithImpersonation returns a copy of the config with impersonation configured.
// This is used to create per-user clients from the base kubeconfig credentials.
//
// # Audit Trail
//
// This function automatically adds the "agent: mcp-kubernetes" extra header to all
// impersonated requests. This allows Kubernetes audit logs to identify that operations
// were performed via the MCP server, providing a clear audit trail.
//
// Security: The agent header is immutable and added AFTER user extras. Any attempt
// by a user to override it via OAuth extra claims will be ignored. This ensures
// audit trail integrity even if other user claims are manipulated.
//
// The resulting HTTP headers will include:
//
//	Impersonate-User: <user.Email>
//	Impersonate-Group: <user.Groups[0]>
//	Impersonate-Group: <user.Groups[1]>
//	...
//	Impersonate-Extra-agent: mcp-kubernetes
//	Impersonate-Extra-<key>: <value>  (for each entry in user.Extra)
//
// # Security
//
// This function panics if config is non-nil but user is nil. This is a deliberate
// security measure: silently returning a non-impersonated config when impersonation
// was expected could lead to privilege escalation. The panic indicates a programming
// error that must be fixed.
//
// Nil handling:
//   - If config is nil, returns nil (nothing to configure)
//   - If user is nil with non-nil config, panics (programming error - use ValidateUserInfo first)
func ConfigWithImpersonation(config *rest.Config, user *UserInfo) *rest.Config {
	if config == nil {
		return nil
	}
	if user == nil {
		// Security: Do not silently skip impersonation. This would return a config
		// with elevated (admin) privileges instead of user-scoped access.
		panic("federation: ConfigWithImpersonation called with nil user - this is a programming error; validate user with ValidateUserInfo before calling")
	}

	impersonatedConfig := rest.CopyConfig(config)

	// Build extra headers, merging user's extra with the agent identifier
	extra := mergeExtraWithAgent(user.Extra)

	impersonatedConfig.Impersonate = rest.ImpersonationConfig{
		UserName: user.Email,
		Groups:   user.Groups,
		Extra:    extra,
	}

	return impersonatedConfig
}

// mergeExtraWithAgent creates a new extra map that includes the agent identifier.
// The agent identifier is always added to provide an audit trail in Kubernetes logs.
// If the user already has extra headers, they are preserved and the agent is added.
//
// # Security: Immutable Audit Trail
//
// The agent identifier is added AFTER user extras, making it immutable.
// This ensures the audit trail cannot be tampered with, even if a user
// attempts to override it via OAuth extra claims. Any user-specified
// "agent" value will be overwritten with the canonical "mcp-kubernetes" value.
func mergeExtraWithAgent(userExtra map[string][]string) map[string][]string {
	// Pre-allocate map with expected capacity (user extras + agent)
	extra := make(map[string][]string, len(userExtra)+1)

	// Copy user's extra headers first
	for k, v := range userExtra {
		extra[k] = v
	}

	// Add agent identifier LAST to ensure it cannot be overridden (immutable audit trail)
	extra[ImpersonationAgentExtraKey] = []string{ImpersonationAgentName}

	return extra
}

// ImpersonationTraceIDKey is the key used for trace ID in impersonation extra headers.
// This appears as "Impersonate-Extra-trace-id: <trace_id>" in HTTP requests.
const ImpersonationTraceIDKey = "trace-id"

// ConfigWithImpersonationAndTraceID returns a copy of the config with impersonation
// configured, including trace ID for distributed tracing correlation.
//
// This function extends ConfigWithImpersonation by adding the trace ID to the
// impersonation extra headers. This allows Kubernetes audit logs to be correlated
// with OpenTelemetry traces, bridging the "audit gap" when the MCP server acts as a proxy.
//
// # Audit Trail Enhancement
//
// The resulting HTTP headers will include:
//
//	Impersonate-User: <user.Email>
//	Impersonate-Group: <user.Groups[...]>
//	Impersonate-Extra-agent: mcp-kubernetes
//	Impersonate-Extra-trace-id: <traceID>
//	Impersonate-Extra-<key>: <value>  (for each entry in user.Extra)
//
// Kubernetes Audit Log on WC will show:
//
//	{
//	  "user": {
//	    "username": "jane@giantswarm.io",
//	    "extra": {
//	      "agent": ["mcp-kubernetes"],
//	      "trace-id": ["abc123def456"]
//	    }
//	  }
//	}
//
// # Security
//
// Both agent and trace-id are added AFTER user extras to ensure they cannot be
// overridden by manipulated OAuth claims.
//
// # Parameters
//
//   - config: The base REST config (typically from cluster kubeconfig)
//   - user: User identity information for impersonation
//   - traceID: OpenTelemetry trace ID (empty string if tracing is disabled)
func ConfigWithImpersonationAndTraceID(config *rest.Config, user *UserInfo, traceID string) *rest.Config {
	if config == nil {
		return nil
	}
	if user == nil {
		// Security: Do not silently skip impersonation.
		panic("federation: ConfigWithImpersonationAndTraceID called with nil user - this is a programming error; validate user with ValidateUserInfo before calling")
	}

	impersonatedConfig := rest.CopyConfig(config)

	// Build extra headers, merging user's extra with agent and trace ID
	extra := mergeExtraWithAgentAndTraceID(user.Extra, traceID)

	impersonatedConfig.Impersonate = rest.ImpersonationConfig{
		UserName: user.Email,
		Groups:   user.Groups,
		Extra:    extra,
	}

	return impersonatedConfig
}

// mergeExtraWithAgentAndTraceID creates a new extra map that includes
// the agent identifier and optionally the trace ID.
//
// # Security: Immutable Audit Trail
//
// Both agent and trace-id are added AFTER user extras, making them immutable.
// This ensures the audit trail cannot be tampered with.
func mergeExtraWithAgentAndTraceID(userExtra map[string][]string, traceID string) map[string][]string {
	// Pre-allocate map with expected capacity
	capacity := len(userExtra) + 2 // +1 for agent, +1 for trace-id
	extra := make(map[string][]string, capacity)

	// Copy user's extra headers first
	for k, v := range userExtra {
		extra[k] = v
	}

	// Add agent identifier (immutable)
	extra[ImpersonationAgentExtraKey] = []string{ImpersonationAgentName}

	// Add trace ID if provided (immutable)
	if traceID != "" {
		extra[ImpersonationTraceIDKey] = []string{traceID}
	}

	return extra
}
