package federation

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
// The method:
//  1. Finds the Cluster resource to determine the namespace
//  2. Constructs the secret name using CAPI convention (${CLUSTER_NAME}-kubeconfig)
//  3. Fetches the secret from the Management Cluster
//  4. Extracts kubeconfig data (supports both 'value' and 'kubeconfig' keys)
//  5. Parses the kubeconfig into a rest.Config
//  6. Optionally validates the connection
//
// Security notes:
//   - Requires 'get' permission on Secrets in the cluster's namespace
//   - Never logs kubeconfig contents (sensitive credential data)
//   - Uses the admin localClient for secret retrieval (not impersonated)
func (m *Manager) GetKubeconfigForCluster(ctx context.Context, clusterName string) (*rest.Config, error) {
	// Find the cluster to determine its namespace
	clusterInfo, err := m.findClusterInfo(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	// Retrieve and parse the kubeconfig secret
	return m.getKubeconfigFromSecret(ctx, clusterInfo)
}

// GetKubeconfigForClusterValidated retrieves the kubeconfig and validates
// that the resulting config can establish a connection to the cluster.
//
// This is useful when you want to ensure the credentials are valid before
// caching or using them for operations.
func (m *Manager) GetKubeconfigForClusterValidated(ctx context.Context, clusterName string) (*rest.Config, error) {
	config, err := m.GetKubeconfigForCluster(ctx, clusterName)
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
// # Security Considerations
//
// This method uses the admin localDynamic client (not impersonated) because:
//
//  1. CAPI Cluster resources may be in namespaces the user doesn't have direct
//     access to on the Management Cluster.
//
//  2. The actual authorization is deferred to the workload cluster via
//     impersonation headers. When the user attempts operations on the workload
//     cluster, the Kubernetes RBAC on that cluster will enforce permissions.
//
//  3. Knowing a cluster name exists is not considered a security risk in most
//     deployment models. However, if cluster enumeration is a concern:
//     - Consider implementing namespace-scoped RBAC for Cluster resources
//     - Use ListClusters() with user impersonation for user-facing listings
//     - This internal method should only be called after input validation
//
// The cluster name must be validated using ValidateClusterName() before calling
// this method to prevent path traversal or injection attacks.
//
// All user-facing errors are sanitized via UserFacingError() to prevent
// cluster existence leakage through error response differentiation.
func (m *Manager) findClusterInfo(ctx context.Context, clusterName string) (*ClusterInfo, error) {
	// List all CAPI Cluster resources across all namespaces
	// Note: We don't use FieldSelector because the fake dynamic client doesn't support it well
	list, err := m.localDynamic.Resource(CAPIClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Debug("Failed to list CAPI Cluster resources",
			"cluster", clusterName,
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
				"namespace", namespace)
			return &ClusterInfo{
				Name:      clusterName,
				Namespace: namespace,
			}, nil
		}
	}

	m.logger.Debug("CAPI Cluster not found",
		"cluster", clusterName)
	return nil, &ClusterNotFoundError{
		ClusterName: clusterName,
		Reason:      "no CAPI Cluster resource found with this name",
	}
}

// getKubeconfigFromSecret retrieves the kubeconfig data from the cluster's secret.
//
// By CAPI convention, the secret is named ${CLUSTER_NAME}-kubeconfig and contains
// the kubeconfig data in either the 'value' or 'kubeconfig' key.
func (m *Manager) getKubeconfigFromSecret(ctx context.Context, info *ClusterInfo) (*rest.Config, error) {
	secretName := info.Name + CAPISecretSuffix

	// Fetch the secret using the admin client (not impersonated)
	secret, err := m.localClient.CoreV1().Secrets(info.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		m.logger.Debug("Failed to fetch kubeconfig secret",
			"cluster", info.Name,
			"namespace", info.Namespace,
			"secret", secretName,
			"error", err)

		// Determine if this is a "not found" error
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
func ConfigWithImpersonation(config *rest.Config, user *UserInfo) *rest.Config {
	if config == nil || user == nil {
		return config
	}

	impersonatedConfig := rest.CopyConfig(config)
	impersonatedConfig.Impersonate = rest.ImpersonationConfig{
		UserName: user.Email,
		Groups:   user.Groups,
		Extra:    user.Extra,
	}

	return impersonatedConfig
}
