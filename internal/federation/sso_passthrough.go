// Package federation provides SSO token passthrough for workload cluster authentication.
package federation

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// SSOPassthroughConfig holds configuration for SSO token passthrough mode.
type SSOPassthroughConfig struct {
	// CAConfigMapSuffix is the suffix for CA ConfigMaps (default: "-ca-public").
	// The full ConfigMap name is: ${CLUSTER_NAME}${CAConfigMapSuffix}
	// These ConfigMaps should be created by an operator that extracts the CA certificate
	// from the CAPI-generated secret.
	CAConfigMapSuffix string

	// TokenExtractor extracts the user's SSO token from context.
	// This is typically oauth.GetAccessTokenFromContext.
	TokenExtractor TokenExtractor
}

// DefaultSSOPassthroughConfig returns the default configuration for SSO passthrough.
func DefaultSSOPassthroughConfig() *SSOPassthroughConfig {
	return &SSOPassthroughConfig{
		CAConfigMapSuffix: DefaultCAConfigMapSuffix,
	}
}

// GetCAForCluster retrieves the CA certificate for a workload cluster from a CA ConfigMap.
// This is used in SSO passthrough mode where we don't need the full kubeconfig with admin credentials.
//
// # Security Model
//
// This method only retrieves the cluster's CA certificate, not any credentials.
// The CA certificate is public information needed for TLS verification.
// Since CA certificates are public keys, they are stored in ConfigMaps rather than Secrets.
//
// # ConfigMap Convention
//
// The CA ConfigMap is expected to be named: ${CLUSTER_NAME}${caConfigMapSuffix}
// The CA certificate is stored in the key defined by CAConfigMapKey ("ca.crt").
//
// These ConfigMaps should be created by an operator that extracts the CA certificate
// (tls.crt) from the CAPI-generated ${CLUSTER_NAME}-ca secret.
func (m *Manager) GetCAForCluster(ctx context.Context, clusterName string, user *UserInfo) ([]byte, string, error) {
	// Validate inputs
	if err := ValidateUserInfo(user); err != nil {
		return nil, "", err
	}
	if err := ValidateClusterName(clusterName); err != nil {
		return nil, "", err
	}

	// Fail fast if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, "", &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      "context cancelled or expired",
		}
	}

	// Get user-scoped clients for MC operations (cluster discovery and ConfigMap access)
	clientset, dynamicClient, _, err := m.clientProvider.GetClientsForUser(ctx, user)
	if err != nil {
		m.logger.Debug("Failed to get user clients for CA retrieval",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, "", &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      "failed to create client for user",
		}
	}

	// Find the cluster to determine its namespace and endpoint
	clusterInfo, err := m.findClusterInfo(ctx, clusterName, dynamicClient, user)
	if err != nil {
		return nil, "", err
	}

	// Get cluster endpoint from the CAPI Cluster resource
	endpoint, err := m.getClusterEndpoint(ctx, clusterName, dynamicClient)
	if err != nil {
		return nil, "", err
	}

	// Retrieve CA certificate from the CA ConfigMap
	// Note: ConfigMaps are used instead of Secrets because the CA certificate is public information
	caData, err := m.getCAFromConfigMap(ctx, clusterInfo, clientset, user)
	if err != nil {
		return nil, "", err
	}

	return caData, endpoint, nil
}

// getClusterEndpoint extracts the API server endpoint from a CAPI Cluster resource.
func (m *Manager) getClusterEndpoint(ctx context.Context, clusterName string, dynamicClient dynamic.Interface) (string, error) {
	// List all CAPI Cluster resources to find the one we need
	list, err := dynamicClient.Resource(CAPIClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", &ClusterNotFoundError{
			ClusterName: clusterName,
			Reason:      fmt.Sprintf("failed to query CAPI clusters: %v", err),
		}
	}

	for _, cluster := range list.Items {
		if cluster.GetName() == clusterName {
			// Try to get endpoint from spec.controlPlaneEndpoint
			spec, found, err := unstructuredNestedMap(cluster.Object, "spec")
			if err != nil || !found {
				continue
			}

			cpEndpoint, found, err := unstructuredNestedMap(spec, "controlPlaneEndpoint")
			if err != nil || !found {
				continue
			}

			host, _, _ := unstructuredNestedString(cpEndpoint, "host")
			port, _, _ := unstructuredNestedInt64(cpEndpoint, "port")

			if host != "" {
				if port > 0 {
					return fmt.Sprintf("https://%s:%d", host, port), nil
				}
				return fmt.Sprintf("https://%s:6443", host), nil
			}
		}
	}

	return "", &ClusterNotFoundError{
		ClusterName: clusterName,
		Reason:      "could not determine cluster API endpoint from CAPI Cluster resource",
	}
}

// unstructuredNestedMap is a helper for extracting nested maps from unstructured objects.
func unstructuredNestedMap(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool, error) {
	current := obj
	for _, field := range fields {
		val, ok := current[field]
		if !ok {
			return nil, false, nil
		}
		nested, ok := val.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("field %s is not a map", field)
		}
		current = nested
	}
	return current, true, nil
}

// unstructuredNestedString is a helper for extracting nested strings from unstructured objects.
func unstructuredNestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	current := obj
	for i, field := range fields {
		val, ok := current[field]
		if !ok {
			return "", false, nil
		}
		if i == len(fields)-1 {
			str, ok := val.(string)
			return str, ok, nil
		}
		nested, ok := val.(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("field %s is not a map", field)
		}
		current = nested
	}
	return "", false, nil
}

// unstructuredNestedInt64 is a helper for extracting nested int64 from unstructured objects.
func unstructuredNestedInt64(obj map[string]interface{}, fields ...string) (int64, bool, error) {
	current := obj
	for i, field := range fields {
		val, ok := current[field]
		if !ok {
			return 0, false, nil
		}
		if i == len(fields)-1 {
			// Handle both int64 and float64 (JSON numbers are parsed as float64)
			switch v := val.(type) {
			case int64:
				return v, true, nil
			case float64:
				return int64(v), true, nil
			case int:
				return int64(v), true, nil
			default:
				return 0, false, nil
			}
		}
		nested, ok := val.(map[string]interface{})
		if !ok {
			return 0, false, fmt.Errorf("field %s is not a map", field)
		}
		current = nested
	}
	return 0, false, nil
}

// getCAFromConfigMap retrieves the CA certificate from a CA ConfigMap.
// ConfigMaps are used instead of Secrets because CA certificates are public information.
func (m *Manager) getCAFromConfigMap(ctx context.Context, info *ClusterInfo, clientset kubernetes.Interface, user *UserInfo) ([]byte, error) {
	// Get CA ConfigMap suffix from manager config (or use default)
	caConfigMapSuffix := DefaultCAConfigMapSuffix
	if m.ssoPassthroughConfig != nil && m.ssoPassthroughConfig.CAConfigMapSuffix != "" {
		caConfigMapSuffix = m.ssoPassthroughConfig.CAConfigMapSuffix
	}

	configMapName := info.Name + caConfigMapSuffix

	// Fetch the CA ConfigMap
	configMap, err := clientset.CoreV1().ConfigMaps(info.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		m.logger.Debug("Failed to fetch CA ConfigMap",
			"cluster", info.Name,
			"namespace", info.Namespace,
			"configmap", configMapName,
			UserHashAttr(user.Email),
			"error", err)

		return nil, &KubeconfigError{
			ClusterName: info.Name,
			SecretName:  configMapName, // Reusing SecretName field for ConfigMap name
			Namespace:   info.Namespace,
			Reason:      "failed to fetch CA ConfigMap",
			Err:         err,
			NotFound:    isNotFoundError(err),
		}
	}

	// Extract CA certificate data
	caData, ok := configMap.Data[CAConfigMapKey]
	if !ok || len(caData) == 0 {
		m.logger.Debug("CA ConfigMap missing expected key",
			"cluster", info.Name,
			"namespace", info.Namespace,
			"configmap", configMapName,
			"expected_key", CAConfigMapKey,
			"available_keys", getConfigMapKeys(configMap.Data))

		return nil, &KubeconfigError{
			ClusterName: info.Name,
			SecretName:  configMapName,
			Namespace:   info.Namespace,
			Reason:      fmt.Sprintf("ConfigMap missing '%s' key", CAConfigMapKey),
			NotFound:    false,
		}
	}

	m.logger.Debug("Successfully retrieved CA certificate from ConfigMap",
		"cluster", info.Name,
		"namespace", info.Namespace,
		UserHashAttr(user.Email))

	return []byte(caData), nil
}

// getConfigMapKeys returns the keys present in a ConfigMap's data.
func getConfigMapKeys(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}

// CreateSSOPassthroughClient creates a Kubernetes client for a workload cluster
// using SSO token passthrough instead of impersonation.
//
// # Security Model
//
// This method uses the user's SSO/OAuth ID token directly for authentication
// to the workload cluster API server, rather than using admin credentials
// with impersonation headers.
//
// Requirements:
//   - The workload cluster API server must be configured with OIDC authentication
//   - The API server must trust the same Identity Provider
//   - The API server must accept tokens with the upstream aggregator's audience
//
// Benefits:
//   - No admin credentials needed (only CA certificate)
//   - User identity verified directly by WC API server
//   - Better audit trail (shows direct OIDC auth, not impersonation)
//   - Reduced privilege requirements for ServiceAccount
func (m *Manager) CreateSSOPassthroughClient(ctx context.Context, clusterName string, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	// Validate SSO passthrough is configured
	if m.ssoPassthroughConfig == nil || m.ssoPassthroughConfig.TokenExtractor == nil {
		return nil, nil, nil, fmt.Errorf("SSO passthrough mode is not configured")
	}

	// Extract SSO token from context
	ssoToken, ok := m.ssoPassthroughConfig.TokenExtractor(ctx)
	if !ok || ssoToken == "" {
		m.logger.Debug("No SSO token available for passthrough",
			"cluster", clusterName,
			UserHashAttr(user.Email))
		return nil, nil, nil, ErrSSOTokenMissing
	}

	// Get CA certificate and endpoint for the cluster
	caData, endpoint, err := m.GetCAForCluster(ctx, clusterName, user)
	if err != nil {
		return nil, nil, nil, err
	}

	// Create REST config with SSO token (no impersonation)
	restConfig := &rest.Config{
		Host:        endpoint,
		BearerToken: ssoToken,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
		// Apply connectivity settings if configured
		QPS:   50,
		Burst: 100,
	}

	// Apply connectivity configuration if available
	if m.connectivityConfig != nil {
		ApplyConnectivityConfig(restConfig, *m.connectivityConfig)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		m.logger.Debug("Failed to create SSO passthrough clientset",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, nil, nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		m.logger.Debug("Failed to create SSO passthrough dynamic client",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, nil, nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	m.logger.Debug("Successfully created SSO passthrough client",
		"cluster", clusterName,
		"endpoint", sanitizeHost(endpoint),
		UserHashAttr(user.Email))

	return clientset, dynClient, restConfig, nil
}
