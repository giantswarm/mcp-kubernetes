package federation

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClientProvider creates Kubernetes clients scoped to a specific user's identity.
// This interface enables per-request client creation when OAuth downstream is enabled,
// ensuring that each user's RBAC permissions are enforced on the Management Cluster.
//
// # Security Model
//
// When OAuth downstream is enabled:
//   - Each request carries the user's OAuth access token
//   - GetClientsForUser creates clients authenticated as that user
//   - All Management Cluster operations (including kubeconfig secret retrieval)
//     are performed with the user's identity, enforcing their RBAC permissions
//
// This provides defense in depth: users must have RBAC permission to read
// kubeconfig secrets on the Management Cluster, AND their impersonated identity
// must have permissions on the Workload Cluster.
type ClientProvider interface {
	// GetClientsForUser returns Kubernetes clients authenticated as the specified user.
	// The returned clients use the user's OAuth token for authentication, ensuring
	// all operations are performed with the user's RBAC permissions.
	//
	// Parameters:
	//   - ctx: Context for the request (may contain OAuth token)
	//   - user: User identity information from OAuth claims
	//
	// Returns:
	//   - kubernetes.Interface: Clientset for typed API access
	//   - dynamic.Interface: Dynamic client for CRD access (e.g., CAPI resources)
	//   - *rest.Config: REST config for creating additional clients
	//   - error: Any error during client creation
	GetClientsForUser(ctx context.Context, user *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error)
}

// StaticClientProvider is a simple ClientProvider that returns pre-configured clients.
// This is useful for testing and for scenarios where per-user client creation is not needed
// (e.g., when using service account authentication without OAuth downstream).
//
// Note: When using StaticClientProvider, all users share the same client, so RBAC
// differentiation between users is not enforced at the client level. Use this only
// when appropriate for your security model.
type StaticClientProvider struct {
	Clientset     kubernetes.Interface
	DynamicClient dynamic.Interface
	RestConfig    *rest.Config
}

// GetClientsForUser returns the static clients regardless of user.
// This implementation ignores the user parameter - all users get the same clients.
func (p *StaticClientProvider) GetClientsForUser(_ context.Context, _ *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	return p.Clientset, p.DynamicClient, p.RestConfig, nil
}

// Ensure StaticClientProvider implements ClientProvider.
var _ ClientProvider = (*StaticClientProvider)(nil)

// UserInfo contains the authenticated user's identity information
// extracted from the OAuth token. This information is used to configure
// Kubernetes user impersonation headers.
type UserInfo struct {
	// Email is the user's email address from the OAuth token's email claim.
	// This is used as the Impersonate-User header value.
	Email string

	// Groups contains the user's group memberships from OAuth claims.
	// These are passed via Impersonate-Group headers for RBAC evaluation.
	Groups []string

	// Extra contains additional claims from the OAuth token that should be
	// propagated to the Kubernetes API server via Impersonate-Extra headers.
	// Common examples include organization IDs, tenant identifiers, or custom claims.
	Extra map[string][]string
}

// ClusterSummary provides basic information about a workload cluster.
// This is returned by ListClusters and contains metadata useful for
// cluster selection and display purposes.
type ClusterSummary struct {
	// Name is the unique identifier of the cluster within its namespace.
	// This corresponds to the Cluster API Cluster resource name.
	Name string `json:"name"`

	// Namespace is the organization namespace on the Management Cluster
	// where the CAPI Cluster resource is located.
	Namespace string `json:"namespace"`

	// Provider indicates the infrastructure provider (e.g., "aws", "azure", "vsphere").
	// This is extracted from the CAPI infrastructure reference.
	Provider string `json:"provider,omitempty"`

	// Release is the Giant Swarm release version running on the cluster.
	// Format follows semver, e.g., "19.3.0".
	Release string `json:"release,omitempty"`

	// KubernetesVersion is the Kubernetes version running on the cluster.
	// Format follows semver, e.g., "1.28.5".
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// Status indicates the current lifecycle phase of the cluster.
	// Common values: "Provisioned", "Provisioning", "Deleting", "Failed".
	Status string `json:"status"`

	// Ready indicates whether the cluster is fully operational and
	// ready to accept workloads.
	Ready bool `json:"ready"`

	// ControlPlaneReady indicates whether the control plane components
	// are healthy and operational.
	ControlPlaneReady bool `json:"controlPlaneReady"`

	// InfrastructureReady indicates whether the underlying infrastructure
	// (VMs, networks, etc.) is provisioned and healthy.
	InfrastructureReady bool `json:"infrastructureReady"`

	// NodeCount is the current number of worker nodes in the cluster.
	// This may differ from the desired count during scaling operations.
	NodeCount int `json:"nodeCount,omitempty"`

	// CreatedAt is the timestamp when the cluster was initially created.
	CreatedAt time.Time `json:"createdAt"`

	// Labels contains the Kubernetes labels applied to the Cluster resource.
	// These often include organization, team, or environment tags.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations contains the Kubernetes annotations on the Cluster resource.
	// May include operational metadata or external references.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ClusterPhase represents the lifecycle phase of a CAPI cluster.
type ClusterPhase string

// Standard CAPI cluster phases.
const (
	// ClusterPhasePending indicates the cluster is awaiting provisioning.
	ClusterPhasePending ClusterPhase = "Pending"

	// ClusterPhaseProvisioning indicates the cluster is being created.
	ClusterPhaseProvisioning ClusterPhase = "Provisioning"

	// ClusterPhaseProvisioned indicates the cluster is fully operational.
	ClusterPhaseProvisioned ClusterPhase = "Provisioned"

	// ClusterPhaseDeleting indicates the cluster is being deleted.
	ClusterPhaseDeleting ClusterPhase = "Deleting"

	// ClusterPhaseFailed indicates the cluster encountered a fatal error.
	ClusterPhaseFailed ClusterPhase = "Failed"

	// ClusterPhaseUnknown indicates the cluster phase cannot be determined.
	ClusterPhaseUnknown ClusterPhase = "Unknown"
)

// CAPI resource identifiers and conventions for cluster lookup.
var (
	// CAPIClusterGVR is the GroupVersionResource for CAPI Cluster objects.
	CAPIClusterGVR = schema.GroupVersionResource{
		Group:    "cluster.x-k8s.io",
		Version:  "v1beta2",
		Resource: "clusters",
	}
)

// CAPISecretSuffix is the suffix used by CAPI for kubeconfig secrets.
// The full secret name is: ${CLUSTER_NAME}-kubeconfig
// nolint:gosec // G101: This is not a hardcoded credential, it's a suffix for secret naming convention
const CAPISecretSuffix = "-kubeconfig"

// DefaultCAConfigMapSuffix is the default suffix for CA ConfigMaps used in SSO passthrough mode.
// The full ConfigMap name is: ${CLUSTER_NAME}-ca-public
// This ConfigMap contains only the cluster's CA certificate (public key), not any credentials.
// An operator should create these ConfigMaps by extracting tls.crt from the CAPI-generated
// ${CLUSTER_NAME}-ca secret (which also contains the private key).
const DefaultCAConfigMapSuffix = "-ca-public"

// CAConfigMapKey is the key within the CA ConfigMap that contains the CA certificate data.
const CAConfigMapKey = "ca.crt"

// Default client configuration for SSO passthrough mode.
// These values are used when no connectivity config is provided.
const (
	// DefaultSSOPassthroughQPS is the default QPS limit for SSO passthrough clients.
	DefaultSSOPassthroughQPS float32 = 50
	// DefaultSSOPassthroughBurst is the default burst limit for SSO passthrough clients.
	DefaultSSOPassthroughBurst int = 100
)

// WorkloadClusterAuthMode defines how mcp-kubernetes authenticates to workload clusters.
type WorkloadClusterAuthMode string

const (
	// WorkloadClusterAuthModeImpersonation uses admin credentials from kubeconfig secrets
	// with user impersonation headers. This is the default and existing behavior.
	// Security model:
	//   - ServiceAccount reads kubeconfig secret (contains admin credentials)
	//   - All WC API requests use admin credentials + Impersonate-User/Group headers
	//   - WC RBAC enforced via impersonation
	WorkloadClusterAuthModeImpersonation WorkloadClusterAuthMode = "impersonation"

	// WorkloadClusterAuthModeSSOPassthrough forwards the user's SSO/OAuth ID token
	// directly to the workload cluster API server. This eliminates the need for
	// admin credentials in kubeconfig secrets.
	// Security model:
	//   - Only CA certificate is needed from the cluster (no admin credentials)
	//   - User's ID token is forwarded as Bearer token to WC API server
	//   - WC API server validates token via its OIDC configuration
	//   - User's own RBAC permissions apply (no impersonation)
	// Requirements:
	//   - WC API servers must be configured with OIDC authentication
	//   - Same Identity Provider must be trusted by all clusters
	//   - API servers must accept tokens with the upstream aggregator's audience
	WorkloadClusterAuthModeSSOPassthrough WorkloadClusterAuthMode = "sso-passthrough"
)

// CAPISecretKey is the key within the kubeconfig secret that contains
// the actual kubeconfig YAML data (standard CAPI convention).
const CAPISecretKey = "value"

// CAPISecretKeyAlternate is an alternate key used by some CAPI providers
// for storing kubeconfig data in secrets.
const CAPISecretKeyAlternate = "kubeconfig"

// ImpersonationHeaders contains the header names used for Kubernetes
// user impersonation.
const (
	// ImpersonateUserHeader is the header name for the impersonated user.
	ImpersonateUserHeader = "Impersonate-User"

	// ImpersonateGroupHeader is the header name for impersonated groups.
	ImpersonateGroupHeader = "Impersonate-Group"

	// ImpersonateExtraHeaderPrefix is the prefix for extra impersonation headers.
	ImpersonateExtraHeaderPrefix = "Impersonate-Extra-"
)

// Impersonation agent identification and audit trail headers.
const (
	// ImpersonationAgentName is the identifier used in Impersonate-Extra-agent headers.
	// This allows audit logs to identify that operations were performed via mcp-kubernetes.
	ImpersonationAgentName = "mcp-kubernetes"

	// ImpersonationAgentExtraKey is the key used for the agent identifier in extra headers.
	// This appears as "Impersonate-Extra-agent: mcp-kubernetes" in HTTP requests.
	ImpersonationAgentExtraKey = "agent"

	// OriginalGroupsExtraKey is the impersonation extra header key used to record the
	// original (pre-mapping) OIDC groups when group mapping is active. This ensures
	// the Kubernetes audit log on the workload cluster contains both the mapped groups
	// (in Impersonate-Group headers) and the original groups (in this extra header),
	// providing a complete audit trail in a single log source.
	//
	// The key uses a domain-prefixed format to avoid collisions with other extra headers.
	// It appears as "Impersonate-Extra-Mcp.giantswarm.io%2Foriginal-Groups: <group>"
	// in HTTP requests and as "mcp.giantswarm.io/original-groups" in the K8s audit log
	// user.extra field.
	OriginalGroupsExtraKey = "mcp.giantswarm.io/original-groups"
)

// AccessCheck describes a permission check to perform against a Kubernetes cluster.
// This is used with SubjectAccessReview to verify if the authenticated user can
// perform a specific action before attempting the operation.
//
// # Usage
//
// AccessCheck is typically used for pre-flight checks before destructive operations:
//
//	check := &AccessCheck{
//		Verb:      "delete",
//		Resource:  "pods",
//		APIGroup:  "", // core API group
//		Namespace: "production",
//	}
//	result, err := manager.CheckAccess(ctx, "my-cluster", user, check)
//	if err != nil {
//		return err
//	}
//	if !result.Allowed {
//		return fmt.Errorf("permission denied: %s", result.Reason)
//	}
type AccessCheck struct {
	// Verb is the Kubernetes API verb to check (e.g., "get", "list", "create", "delete", "patch", "watch").
	// This is required.
	Verb string

	// Resource is the Kubernetes resource type to check (e.g., "pods", "deployments", "secrets").
	// This is required.
	Resource string

	// APIGroup is the API group for the resource (e.g., "", "apps", "batch").
	// Use "" for core API resources like pods and services.
	APIGroup string

	// Namespace is the namespace to check permissions in.
	// Leave empty for cluster-scoped resources or cluster-wide checks.
	Namespace string

	// Name is the specific resource name to check.
	// Leave empty to check permissions for all resources of the type.
	Name string

	// Subresource is the subresource to check (e.g., "logs", "exec", "portforward" for pods).
	// Leave empty for the main resource.
	Subresource string
}

// AccessCheckResult contains the result of a SubjectAccessReview check.
type AccessCheckResult struct {
	// Allowed indicates whether the requested action is permitted.
	Allowed bool

	// Denied indicates whether the requested action was explicitly denied.
	// This is different from !Allowed - a request can be neither allowed nor denied
	// (e.g., when no policy matches).
	Denied bool

	// Reason provides a human-readable explanation of the decision.
	// This may include information about which RBAC rule matched or why access was denied.
	Reason string

	// EvaluationError contains any error that occurred during policy evaluation.
	// A non-empty EvaluationError typically means the result is inconclusive.
	EvaluationError string
}
