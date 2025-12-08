package federation

import (
	"time"
)

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

// CAPISecretSuffix is the suffix used by CAPI for kubeconfig secrets.
// The full secret name is: ${CLUSTER_NAME}-kubeconfig
const CAPISecretSuffix = "-kubeconfig"

// CAPISecretKey is the key within the kubeconfig secret that contains
// the actual kubeconfig YAML data.
const CAPISecretKey = "value"

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
