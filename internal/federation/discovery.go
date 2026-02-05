package federation

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// Giant Swarm specific label keys for CAPI clusters.
const (
	// LabelClusterName is the standard CAPI cluster name label.
	LabelClusterName = "cluster.x-k8s.io/cluster-name"

	// LabelGiantSwarmCluster is Giant Swarm's cluster label.
	LabelGiantSwarmCluster = "giantswarm.io/cluster"

	// LabelGiantSwarmOrganization is the organization/tenant label.
	LabelGiantSwarmOrganization = "giantswarm.io/organization"

	// LabelGiantSwarmRelease is the Giant Swarm release version label.
	LabelGiantSwarmRelease = "release.giantswarm.io/version"

	// AnnotationClusterDescription is the cluster description annotation.
	AnnotationClusterDescription = "cluster.giantswarm.io/description"
)

// Common infrastructure provider references.
const (
	// ProviderAWS indicates an AWS CAPI cluster.
	ProviderAWS = "aws"

	// ProviderAzure indicates an Azure CAPI cluster.
	ProviderAzure = "azure"

	// ProviderVSphere indicates a vSphere CAPI cluster.
	ProviderVSphere = "vsphere"

	// ProviderGCP indicates a GCP CAPI cluster.
	ProviderGCP = "gcp"

	// ProviderUnknown is used when the provider cannot be determined.
	ProviderUnknown = "unknown"
)

// ErrCAPICRDNotInstalled indicates that CAPI CRDs are not installed on the cluster.
var ErrCAPICRDNotInstalled = fmt.Errorf("CAPI CRDs not installed")

// ClusterDiscoveryError provides context about cluster discovery failures.
type ClusterDiscoveryError struct {
	Reason string
	Err    error
}

// Error implements the error interface.
func (e *ClusterDiscoveryError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("cluster discovery failed: %s: %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("cluster discovery failed: %s", e.Reason)
}

// Unwrap returns the underlying error.
func (e *ClusterDiscoveryError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
func (e *ClusterDiscoveryError) Is(target error) bool {
	return target == ErrCAPICRDNotInstalled && strings.Contains(e.Reason, "CRD not installed")
}

// UserFacingError returns a sanitized error message safe for end users.
func (e *ClusterDiscoveryError) UserFacingError() string {
	if strings.Contains(e.Reason, "CRD not installed") {
		return "this management cluster does not have CAPI installed"
	}
	return "unable to discover clusters - please try again or contact your administrator"
}

// AmbiguousClusterError is returned when a cluster name pattern matches multiple clusters.
type AmbiguousClusterError struct {
	Pattern string
	// Matches contains the clusters that matched the pattern, used to provide
	// helpful feedback to users about which clusters they might have meant.
	Matches []ClusterSummary
}

// Error implements the error interface.
func (e *AmbiguousClusterError) Error() string {
	names := make([]string, len(e.Matches))
	for i, m := range e.Matches {
		names[i] = fmt.Sprintf("%s/%s", m.Namespace, m.Name)
	}
	return fmt.Sprintf("ambiguous cluster name: %d clusters match pattern %q: %s",
		len(e.Matches), e.Pattern, strings.Join(names, ", "))
}

// UserFacingError returns a user-friendly error message.
func (e *AmbiguousClusterError) UserFacingError() string {
	names := make([]string, len(e.Matches))
	for i, m := range e.Matches {
		names[i] = m.Name
	}
	return fmt.Sprintf("multiple clusters match '%s': %s - please use a more specific name",
		e.Pattern, strings.Join(names, ", "))
}

// wrapCAPIListError wraps errors from CAPI cluster list operations into ClusterDiscoveryError.
// It detects CRD-not-installed errors using Kubernetes API error types for robust detection.
func wrapCAPIListError(err error) error {
	if err == nil {
		return nil
	}

	// Use Kubernetes API error detection for robust CRD-not-installed detection
	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		return &ClusterDiscoveryError{
			Reason: "CAPI Cluster CRD not installed on this cluster",
			Err:    err,
		}
	}

	return &ClusterDiscoveryError{
		Reason: "failed to list CAPI clusters",
		Err:    err,
	}
}

// listCAPIClusterResources queries all CAPI Cluster resources using the provided dynamic client.
// The results are filtered by the user's RBAC permissions (enforced by the client).
//
// Returns ErrCAPICRDNotInstalled if the CAPI Cluster CRD is not present on the cluster.
func listCAPIClusterResources(ctx context.Context, dynamicClient dynamic.Interface) (*unstructured.UnstructuredList, error) {
	list, err := dynamicClient.Resource(CAPIClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, wrapCAPIListError(err)
	}
	return list, nil
}

// clusterSummaryFromUnstructured extracts ClusterSummary data from an unstructured CAPI Cluster resource.
func clusterSummaryFromUnstructured(cluster *unstructured.Unstructured) ClusterSummary {
	summary := ClusterSummary{
		Name:        cluster.GetName(),
		Namespace:   cluster.GetNamespace(),
		Labels:      cluster.GetLabels(),
		Annotations: cluster.GetAnnotations(),
		CreatedAt:   cluster.GetCreationTimestamp().Time,
	}

	// Extract provider from infrastructure reference
	summary.Provider = extractProvider(cluster)

	// Extract Giant Swarm release version from labels
	if labels := cluster.GetLabels(); labels != nil {
		if release, ok := labels[LabelGiantSwarmRelease]; ok {
			summary.Release = release
		}
	}

	// Extract Kubernetes version from spec.topology.version or status.version
	summary.KubernetesVersion = extractKubernetesVersion(cluster)

	// Extract status information
	summary.Status, summary.Ready, summary.ControlPlaneReady, summary.InfrastructureReady = extractClusterStatus(cluster)

	// Extract node count from status
	summary.NodeCount = extractNodeCount(cluster)

	return summary
}

// extractProvider determines the infrastructure provider from the cluster's infrastructure reference.
// CAPI clusters have an infrastructureRef field pointing to the provider-specific resource.
func extractProvider(cluster *unstructured.Unstructured) string {
	// Check spec.infrastructureRef.kind
	spec, found, err := unstructured.NestedMap(cluster.Object, "spec")
	if err != nil || !found {
		return ProviderUnknown
	}

	infraRef, found, err := unstructured.NestedMap(spec, "infrastructureRef")
	if err != nil || !found {
		return ProviderUnknown
	}

	// Get the kind from the infrastructure reference
	kind, found, err := unstructured.NestedString(infraRef, "kind")
	if err != nil || !found {
		return ProviderUnknown
	}

	// Map the kind to a provider name
	return mapInfraKindToProvider(kind)
}

// mapInfraKindToProvider maps CAPI infrastructure kinds to provider names.
func mapInfraKindToProvider(kind string) string {
	kindLower := strings.ToLower(kind)

	switch {
	case strings.Contains(kindLower, "aws"):
		return ProviderAWS
	case strings.Contains(kindLower, "azure"):
		return ProviderAzure
	case strings.Contains(kindLower, "vsphere"):
		return ProviderVSphere
	case strings.Contains(kindLower, "gcp"), strings.Contains(kindLower, "google"):
		return ProviderGCP
	default:
		// Return the kind itself if we can't map it (e.g., "DockerCluster" -> "docker")
		return strings.TrimSuffix(kindLower, "cluster")
	}
}

// extractKubernetesVersion extracts the Kubernetes version from the cluster resource.
// It checks multiple locations where the version might be specified.
func extractKubernetesVersion(cluster *unstructured.Unstructured) string {
	// Try spec.topology.version first (ClusterClass topology)
	version, found, err := unstructured.NestedString(cluster.Object, "spec", "topology", "version")
	if err == nil && found && version != "" {
		return version
	}

	// Try status.version
	version, found, err = unstructured.NestedString(cluster.Object, "status", "version")
	if err == nil && found && version != "" {
		return version
	}

	// Try spec.controlPlaneRef.version (alternative location)
	version, found, err = unstructured.NestedString(cluster.Object, "spec", "controlPlaneRef", "version")
	if err == nil && found && version != "" {
		return version
	}

	return ""
}

// extractClusterStatus extracts the cluster phase and ready conditions.
// Returns phase, ready, controlPlaneReady, infrastructureReady.
func extractClusterStatus(cluster *unstructured.Unstructured) (phase string, ready, controlPlaneReady, infrastructureReady bool) {
	// Extract phase from status.phase
	phaseStr, found, err := unstructured.NestedString(cluster.Object, "status", "phase")
	if err != nil || !found {
		phase = string(ClusterPhaseUnknown)
	} else {
		phase = phaseStr
	}

	// Extract controlPlaneReady from status.controlPlaneReady
	cpReady, found, err := unstructured.NestedBool(cluster.Object, "status", "controlPlaneReady")
	if err == nil && found {
		controlPlaneReady = cpReady
	}

	// Extract infrastructureReady from status.infrastructureReady
	infraReady, found, err := unstructured.NestedBool(cluster.Object, "status", "infrastructureReady")
	if err == nil && found {
		infrastructureReady = infraReady
	}

	// Cluster is considered ready when both control plane and infrastructure are ready
	// and the phase is "Provisioned"
	ready = controlPlaneReady && infrastructureReady && ClusterPhase(phase) == ClusterPhaseProvisioned

	return phase, ready, controlPlaneReady, infrastructureReady
}

// extractNodeCount extracts the worker node count from the cluster status.
func extractNodeCount(cluster *unstructured.Unstructured) int {
	// Try status.workerNodes (if available)
	count, found, err := unstructured.NestedInt64(cluster.Object, "status", "workerNodes")
	if err == nil && found {
		return int(count)
	}

	// Try status.readyReplicas as fallback
	count, found, err = unstructured.NestedInt64(cluster.Object, "status", "readyReplicas")
	if err == nil && found {
		return int(count)
	}

	return 0
}

// discoverClusters discovers all CAPI clusters accessible to the user.
// Results are returned as ClusterSummary structs with extracted metadata.
func (m *Manager) discoverClusters(ctx context.Context, dynamicClient dynamic.Interface, user *UserInfo) ([]ClusterSummary, error) {
	list, err := listCAPIClusterResources(ctx, dynamicClient)
	if err != nil {
		m.logger.Debug("Failed to list CAPI clusters for discovery",
			UserHashAttr(user.Email),
			"error", err)
		return nil, err
	}

	// Convert unstructured items to ClusterSummary
	clusters := make([]ClusterSummary, 0, len(list.Items))
	for _, item := range list.Items {
		summary := clusterSummaryFromUnstructured(&item)
		clusters = append(clusters, summary)
	}

	m.logger.Debug("Discovered CAPI clusters",
		UserHashAttr(user.Email),
		"count", len(clusters))

	return clusters, nil
}

// getClusterByName retrieves a specific CAPI cluster by name using a field selector.
// This is more efficient than discoverClusters when looking for a specific cluster,
// as it filters on the server side rather than loading all clusters.
//
// Note: Client-side filtering is also performed as a defensive measure since some
// backends (including test fakes) may not support field selectors.
//
// Returns nil, nil if the cluster is not found.
func (m *Manager) getClusterByName(ctx context.Context, dynamicClient dynamic.Interface, clusterName string, user *UserInfo) (*ClusterSummary, error) {
	// Use field selector to query by name directly on the server
	// This provides server-side filtering when supported by the backend
	listOpts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", clusterName),
	}

	list, err := dynamicClient.Resource(CAPIClusterGVR).List(ctx, listOpts)
	if err != nil {
		m.logger.Debug("Failed to get CAPI cluster by name",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, wrapCAPIListError(err)
	}

	// Client-side filtering as defensive measure (some backends don't support field selectors)
	for i := range list.Items {
		if list.Items[i].GetName() == clusterName {
			summary := clusterSummaryFromUnstructured(&list.Items[i])
			return &summary, nil
		}
	}

	return nil, nil
}

// findClusterByName searches for a cluster with an exact name match.
// Returns nil if not found.
func findClusterByName(clusters []ClusterSummary, name string) *ClusterSummary {
	for i := range clusters {
		if clusters[i].Name == name {
			return &clusters[i]
		}
	}
	return nil
}

// findClustersByPattern searches for clusters matching a name pattern.
// The pattern can match the beginning of the name, end of the name, or be contained within it.
func findClustersByPattern(clusters []ClusterSummary, pattern string) []ClusterSummary {
	var matches []ClusterSummary
	patternLower := strings.ToLower(pattern)

	for _, cluster := range clusters {
		nameLower := strings.ToLower(cluster.Name)
		if strings.Contains(nameLower, patternLower) {
			matches = append(matches, cluster)
		}
	}

	return matches
}

// ResolveCluster finds a cluster by name pattern, handling ambiguity.
// If the pattern matches exactly one cluster, returns its details.
// If the pattern matches multiple clusters, returns an AmbiguousClusterError.
// If no clusters match, returns ErrClusterNotFound.
func (m *Manager) ResolveCluster(ctx context.Context, namePattern string, user *UserInfo) (*ClusterSummary, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// Empty pattern is invalid
	if namePattern == "" {
		return nil, &ValidationError{
			Field:  "cluster name pattern",
			Reason: "pattern cannot be empty",
			Err:    ErrInvalidClusterName,
		}
	}

	// Get dynamic client for CAPI discovery (privileged or user credentials)
	dynamicClient, err := m.getDynamicClientForCAPIDiscovery(ctx, user)
	if err != nil {
		return nil, err
	}

	// Discover all clusters
	clusters, err := m.discoverClusters(ctx, dynamicClient, user)
	if err != nil {
		return nil, err
	}

	// Try exact match first
	if exactMatch := findClusterByName(clusters, namePattern); exactMatch != nil {
		m.logger.Debug("Resolved cluster by exact match",
			"cluster", exactMatch.Name,
			"namespace", exactMatch.Namespace,
			UserHashAttr(user.Email))
		return exactMatch, nil
	}

	// Try pattern match
	matches := findClustersByPattern(clusters, namePattern)

	switch len(matches) {
	case 0:
		m.logger.Debug("No clusters match pattern",
			"pattern", namePattern,
			UserHashAttr(user.Email))
		return nil, &ClusterNotFoundError{
			ClusterName: namePattern,
			Reason:      "no cluster matches the provided name pattern",
		}
	case 1:
		m.logger.Debug("Resolved cluster by pattern match",
			"pattern", namePattern,
			"cluster", matches[0].Name,
			"namespace", matches[0].Namespace,
			UserHashAttr(user.Email))
		return &matches[0], nil
	default:
		m.logger.Debug("Ambiguous cluster pattern",
			"pattern", namePattern,
			"match_count", len(matches),
			UserHashAttr(user.Email))
		return nil, &AmbiguousClusterError{
			Pattern: namePattern,
			Matches: matches,
		}
	}
}

// ClusterListOptions provides options for filtering cluster listings.
type ClusterListOptions struct {
	// Namespace filters clusters to a specific namespace (organization).
	// If empty, all namespaces are searched.
	Namespace string

	// LabelSelector filters clusters by label selector expression.
	// Uses standard Kubernetes label selector syntax.
	LabelSelector string

	// Provider filters clusters by infrastructure provider.
	Provider string

	// Status filters clusters by phase.
	Status ClusterPhase

	// ReadyOnly filters to only include ready clusters.
	ReadyOnly bool
}

// listClustersWithOptions lists clusters with optional filtering.
//
// # Split-Credential Model
//
// When the ClientProvider implements PrivilegedSecretAccessProvider and has privileged
// access available, this method uses ServiceAccount credentials for CAPI cluster discovery.
// This is necessary because:
//   - Users need to discover clusters to use multi-cluster tools
//   - Granting every user cluster-scoped CAPI permissions is impractical
//   - The ServiceAccount has the mcp-kubernetes-capi ClusterRole for CAPI access
//
// When privileged access is not available, it falls back to user credentials.
func (m *Manager) listClustersWithOptions(ctx context.Context, user *UserInfo, opts *ClusterListOptions) ([]ClusterSummary, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	// Get dynamic client for CAPI discovery (privileged or user credentials)
	dynamicClient, err := m.getDynamicClientForCAPIDiscovery(ctx, user)
	if err != nil {
		return nil, err
	}

	// Build list options
	listOpts := metav1.ListOptions{}
	if opts != nil && opts.LabelSelector != "" {
		listOpts.LabelSelector = opts.LabelSelector
	}

	// Query clusters (namespace scoped if specified)
	var list *unstructured.UnstructuredList
	if opts != nil && opts.Namespace != "" {
		list, err = dynamicClient.Resource(CAPIClusterGVR).Namespace(opts.Namespace).List(ctx, listOpts)
	} else {
		list, err = dynamicClient.Resource(CAPIClusterGVR).List(ctx, listOpts)
	}

	if err != nil {
		return nil, wrapCAPIListError(err)
	}

	// Convert and filter results
	clusters := make([]ClusterSummary, 0, len(list.Items))
	for _, item := range list.Items {
		summary := clusterSummaryFromUnstructured(&item)

		// Apply client-side filters
		if opts != nil {
			// Filter by provider
			if opts.Provider != "" && summary.Provider != opts.Provider {
				continue
			}

			// Filter by status
			if opts.Status != "" && summary.Status != string(opts.Status) {
				continue
			}

			// Filter to ready clusters only
			if opts.ReadyOnly && !summary.Ready {
				continue
			}
		}

		clusters = append(clusters, summary)
	}

	m.logger.Debug("Listed CAPI clusters",
		UserHashAttr(user.Email),
		"total", len(list.Items),
		"filtered", len(clusters))

	return clusters, nil
}

// getDynamicClientForCAPIDiscovery returns a dynamic client suitable for CAPI cluster discovery.
//
// It implements the split-credential fallback strategy:
//  1. Try ServiceAccount credentials via PrivilegedSecretAccessProvider (preferred)
//  2. Fall back to user credentials if privileged access is unavailable
//
// This ensures CAPI discovery works in all deployment scenarios while preferring
// the more secure split-credential model when available.
func (m *Manager) getDynamicClientForCAPIDiscovery(ctx context.Context, user *UserInfo) (dynamic.Interface, error) {
	dynamicClient, err := m.getPrivilegedDynamicClientForCAPI(ctx, user)
	if err != nil {
		m.logger.Debug("Privileged CAPI access not available, using user credentials",
			UserHashAttr(user.Email),
			"error", err)

		// Fall back to user credentials
		dynamicClient, err = m.GetDynamicClient(ctx, "", user)
		if err != nil {
			return nil, fmt.Errorf("failed to get dynamic client: %w", err)
		}
	}
	return dynamicClient, nil
}

// getPrivilegedDynamicClientForCAPI returns a privileged dynamic client for CAPI discovery.
// Returns an error if privileged access is not available.
func (m *Manager) getPrivilegedDynamicClientForCAPI(ctx context.Context, user *UserInfo) (dynamic.Interface, error) {
	// Check if the client provider supports privileged access
	privilegedProvider, ok := m.clientProvider.(PrivilegedSecretAccessProvider)
	if !ok {
		return nil, fmt.Errorf("client provider does not support privileged access")
	}

	if !privilegedProvider.HasPrivilegedAccess() {
		return nil, fmt.Errorf("privileged access not available")
	}

	return privilegedProvider.GetPrivilegedDynamicClient(ctx, user)
}

// ClusterAge returns the age of a cluster as a duration.
func (cs *ClusterSummary) ClusterAge() time.Duration {
	return time.Since(cs.CreatedAt)
}

// IsGiantSwarmCluster returns true if this cluster has Giant Swarm labels.
func (cs *ClusterSummary) IsGiantSwarmCluster() bool {
	if cs.Labels == nil {
		return false
	}
	_, hasGSLabel := cs.Labels[LabelGiantSwarmCluster]
	_, hasOrgLabel := cs.Labels[LabelGiantSwarmOrganization]
	return hasGSLabel || hasOrgLabel
}

// Organization returns the Giant Swarm organization for this cluster.
func (cs *ClusterSummary) Organization() string {
	if cs.Labels == nil {
		return ""
	}
	return cs.Labels[LabelGiantSwarmOrganization]
}

// Description returns the cluster description from annotations.
func (cs *ClusterSummary) Description() string {
	if cs.Annotations == nil {
		return ""
	}
	return cs.Annotations[AnnotationClusterDescription]
}
