package federation

import (
	"context"
	"fmt"
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
// This method uses the user's credentials for ALL Management Cluster operations:
//  1. Finds the Cluster resource using user's dynamic client (RBAC enforced)
//  2. Fetches the kubeconfig secret using user's client (RBAC enforced)
//  3. Parses the kubeconfig into a rest.Config
//
// The user must have RBAC permission to:
//   - List/Get Cluster resources (cluster.x-k8s.io/v1beta1)
//   - Get Secrets in the cluster's namespace
//
// This provides defense in depth: users can only access kubeconfig secrets
// they have permission to read on the Management Cluster.
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

	// Get user-scoped clients for MC operations
	clientset, dynamicClient, _, err := m.clientProvider.GetClientsForUser(ctx, user)
	if err != nil {
		m.logger.Debug("Failed to get user clients for kubeconfig retrieval",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      "failed to create client for user",
		}
	}

	// Find the cluster to determine its namespace (using user's RBAC)
	clusterInfo, err := m.findClusterInfo(ctx, clusterName, dynamicClient, user)
	if err != nil {
		return nil, err
	}

	// Retrieve and parse the kubeconfig secret (using user's RBAC)
	return m.getKubeconfigFromSecret(ctx, clusterInfo, clientset, user)
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
// This method uses the user's dynamic client, ensuring:
//   - User must have RBAC permission to list Cluster resources
//   - Only clusters the user has access to will be found
//   - Defense in depth: MC RBAC is enforced before any WC access
//
// The cluster name must be validated using ValidateClusterName() before calling
// this method to prevent path traversal or injection attacks.
//
// All user-facing errors are sanitized via UserFacingError() to prevent
// information leakage through error response differentiation.
func (m *Manager) findClusterInfo(ctx context.Context, clusterName string, dynamicClient dynamic.Interface, user *UserInfo) (*ClusterInfo, error) {
	// List all CAPI Cluster resources across all namespaces (using user's RBAC)
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
			ClusterName: info.Name,
			SecretName:  secretName,
			Namespace:   info.Namespace,
			Reason:      "failed to fetch secret",
			Err:         err,
			NotFound:    isNotFoundError(err),
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
			ClusterName: info.Name,
			SecretName:  secretName,
			Namespace:   info.Namespace,
			Reason:      "invalid kubeconfig data",
			Err:         err,
			NotFound:    false,
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
		ClusterName: info.Name,
		SecretName:  secretName,
		Namespace:   info.Namespace,
		Reason:      fmt.Sprintf("secret missing '%s' or '%s' key", CAPISecretKey, CAPISecretKeyAlternate),
		NotFound:    false,
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

// sanitizeHost returns the host for logging purposes.
// Returns "<empty>" for empty hosts to make logs more readable.
func sanitizeHost(host string) string {
	if host == "" {
		return "<empty>"
	}
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
// were performed via the MCP server, providing a clear audit trail. The agent header
// is merged with any existing extra headers from the UserInfo.
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
// The user's extra values take precedence if they specify a custom agent value.
func mergeExtraWithAgent(userExtra map[string][]string) map[string][]string {
	// Start with the agent identifier
	extra := map[string][]string{
		ImpersonationAgentExtraKey: {ImpersonationAgentName},
	}

	// Merge user's extra headers (user values take precedence)
	for k, v := range userExtra {
		extra[k] = v
	}

	return extra
}
