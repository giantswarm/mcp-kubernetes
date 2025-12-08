package capi

import (
	"time"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
)

// ClusterListOutput represents the output for the capi_list_clusters tool.
type ClusterListOutput struct {
	// Clusters contains the list of cluster summaries.
	Clusters []ClusterListItem `json:"clusters"`

	// TotalCount is the total number of clusters returned.
	TotalCount int `json:"totalCount"`

	// FilterApplied indicates whether any filtering was applied.
	FilterApplied bool `json:"filterApplied,omitempty"`
}

// ClusterListItem represents a single cluster in the list output.
// This is a simplified view suitable for tabular display.
type ClusterListItem struct {
	// Name is the cluster name.
	Name string `json:"name"`

	// Namespace is the organization namespace.
	Namespace string `json:"namespace"`

	// Organization is the Giant Swarm organization (extracted from labels).
	Organization string `json:"organization,omitempty"`

	// Provider is the infrastructure provider (aws, azure, vsphere, etc.).
	Provider string `json:"provider,omitempty"`

	// Release is the Giant Swarm release version.
	Release string `json:"release,omitempty"`

	// Status is the cluster lifecycle phase.
	Status string `json:"status"`

	// Ready indicates if the cluster is fully operational.
	Ready bool `json:"ready"`

	// Age is the human-readable age of the cluster.
	Age string `json:"age"`

	// NodeCount is the number of worker nodes.
	NodeCount int `json:"nodeCount,omitempty"`
}

// ClusterDetailOutput represents the output for the capi_get_cluster tool.
// This provides comprehensive information about a single cluster.
type ClusterDetailOutput struct {
	// Name is the cluster name.
	Name string `json:"name"`

	// Namespace is the organization namespace.
	Namespace string `json:"namespace"`

	// Metadata contains cluster metadata.
	Metadata ClusterMetadata `json:"metadata"`

	// Status contains cluster status information.
	Status ClusterStatus `json:"status"`

	// Labels contains all Kubernetes labels.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations contains all Kubernetes annotations.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ClusterMetadata contains cluster metadata information.
type ClusterMetadata struct {
	// Organization is the Giant Swarm organization.
	Organization string `json:"organization,omitempty"`

	// Provider is the infrastructure provider.
	Provider string `json:"provider,omitempty"`

	// Release is the Giant Swarm release version.
	Release string `json:"release,omitempty"`

	// KubernetesVersion is the Kubernetes version.
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// CreatedAt is the cluster creation timestamp.
	CreatedAt time.Time `json:"createdAt"`

	// Age is the human-readable age.
	Age string `json:"age"`

	// Description is the cluster description from annotations.
	Description string `json:"description,omitempty"`
}

// ClusterStatus contains cluster status information.
type ClusterStatus struct {
	// Phase is the lifecycle phase (Provisioned, Provisioning, Deleting, etc.).
	Phase string `json:"phase"`

	// Ready indicates if the cluster is fully operational.
	Ready bool `json:"ready"`

	// ControlPlaneReady indicates if the control plane is ready.
	ControlPlaneReady bool `json:"controlPlaneReady"`

	// InfrastructureReady indicates if the infrastructure is ready.
	InfrastructureReady bool `json:"infrastructureReady"`

	// NodeCount is the number of worker nodes.
	NodeCount int `json:"nodeCount,omitempty"`
}

// ClusterResolveOutput represents the output for the capi_resolve_cluster tool.
type ClusterResolveOutput struct {
	// Resolved indicates if the pattern resolved to exactly one cluster.
	Resolved bool `json:"resolved"`

	// Cluster is the resolved cluster (present when Resolved is true).
	Cluster *ClusterListItem `json:"cluster,omitempty"`

	// Matches contains matching clusters (present when multiple matches exist).
	Matches []ClusterListItem `json:"matches,omitempty"`

	// Message provides human-readable context about the result.
	Message string `json:"message"`
}

// ClusterHealthOutput represents the output for the capi_cluster_health tool.
type ClusterHealthOutput struct {
	// Name is the cluster name.
	Name string `json:"name"`

	// Status is the overall health status (HEALTHY, UNHEALTHY, DEGRADED, UNKNOWN).
	Status string `json:"status"`

	// Message provides a human-readable health summary.
	Message string `json:"message"`

	// Components contains individual component health status.
	Components ClusterHealthComponents `json:"components"`

	// Checks contains individual health check results.
	Checks []HealthCheck `json:"checks,omitempty"`
}

// ClusterHealthComponents contains health status of cluster components.
type ClusterHealthComponents struct {
	// ControlPlane indicates control plane health.
	ControlPlane ComponentHealth `json:"controlPlane"`

	// Infrastructure indicates infrastructure health.
	Infrastructure ComponentHealth `json:"infrastructure"`

	// Nodes indicates worker node health.
	Nodes ComponentHealth `json:"nodes"`
}

// ComponentHealth represents the health of a cluster component.
type ComponentHealth struct {
	// Status is the component status (healthy, unhealthy, unknown).
	Status string `json:"status"`

	// Ready indicates the number of ready instances.
	Ready int `json:"ready,omitempty"`

	// Total indicates the total number of instances.
	Total int `json:"total,omitempty"`

	// Message provides additional context.
	Message string `json:"message,omitempty"`
}

// HealthCheck represents a single health check result.
type HealthCheck struct {
	// Name is the health check name.
	Name string `json:"name"`

	// Status is the check status (pass, fail, warn).
	Status string `json:"status"`

	// Message provides details about the check result.
	Message string `json:"message,omitempty"`
}

// Health status constants.
const (
	// HealthStatusHealthy indicates the cluster is healthy.
	HealthStatusHealthy = "HEALTHY"

	// HealthStatusUnhealthy indicates the cluster is unhealthy.
	HealthStatusUnhealthy = "UNHEALTHY"

	// HealthStatusDegraded indicates the cluster is degraded but functional.
	HealthStatusDegraded = "DEGRADED"

	// HealthStatusUnknown indicates the health status cannot be determined.
	HealthStatusUnknown = "UNKNOWN"
)

// Component health status constants.
const (
	// ComponentStatusHealthy indicates the component is healthy.
	ComponentStatusHealthy = "healthy"

	// ComponentStatusUnhealthy indicates the component is unhealthy.
	ComponentStatusUnhealthy = "unhealthy"

	// ComponentStatusUnknown indicates the component status is unknown.
	ComponentStatusUnknown = "unknown"
)

// Health check status constants.
const (
	// CheckStatusPass indicates the check passed.
	CheckStatusPass = "pass"

	// CheckStatusFail indicates the check failed.
	CheckStatusFail = "fail"

	// CheckStatusWarn indicates the check has a warning.
	CheckStatusWarn = "warn"
)

// formatAge converts a duration to a human-readable age string.
// Examples: "5d", "2h", "30m", "10s"
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return formatDuration(d, time.Minute, "m")
	}
	if d < 24*time.Hour {
		return formatDuration(d, time.Hour, "h")
	}
	return formatDuration(d, 24*time.Hour, "d")
}

// formatDuration formats a duration with the given unit.
func formatDuration(d time.Duration, unit time.Duration, suffix string) string {
	count := int(d / unit)
	return formatInt(count) + suffix
}

// formatInt converts an int to a string without importing strconv.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + formatInt(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// clusterSummaryToListItem converts a federation.ClusterSummary to a ClusterListItem.
func clusterSummaryToListItem(c federation.ClusterSummary) ClusterListItem {
	return ClusterListItem{
		Name:         c.Name,
		Namespace:    c.Namespace,
		Organization: c.Organization(),
		Provider:     c.Provider,
		Release:      c.Release,
		Status:       c.Status,
		Ready:        c.Ready,
		Age:          formatAge(c.ClusterAge()),
		NodeCount:    c.NodeCount,
	}
}

// clusterSummaryToDetail converts a federation.ClusterSummary to a ClusterDetailOutput.
func clusterSummaryToDetail(c *federation.ClusterSummary) ClusterDetailOutput {
	return ClusterDetailOutput{
		Name:      c.Name,
		Namespace: c.Namespace,
		Metadata: ClusterMetadata{
			Organization:      c.Organization(),
			Provider:          c.Provider,
			Release:           c.Release,
			KubernetesVersion: c.KubernetesVersion,
			CreatedAt:         c.CreatedAt,
			Age:               formatAge(c.ClusterAge()),
			Description:       c.Description(),
		},
		Status: ClusterStatus{
			Phase:               c.Status,
			Ready:               c.Ready,
			ControlPlaneReady:   c.ControlPlaneReady,
			InfrastructureReady: c.InfrastructureReady,
			NodeCount:           c.NodeCount,
		},
		Labels:      c.Labels,
		Annotations: c.Annotations,
	}
}

// buildHealthOutput builds a ClusterHealthOutput from a ClusterSummary.
func buildHealthOutput(c *federation.ClusterSummary) ClusterHealthOutput {
	output := ClusterHealthOutput{
		Name: c.Name,
		Components: ClusterHealthComponents{
			ControlPlane:   buildControlPlaneHealth(c),
			Infrastructure: buildInfrastructureHealth(c),
			Nodes:          buildNodesHealth(c),
		},
	}

	// Determine overall health status
	output.Status, output.Message = determineOverallHealth(c, output.Components)

	// Add health checks
	output.Checks = buildHealthChecks(c)

	return output
}

// buildControlPlaneHealth builds control plane health from cluster status.
func buildControlPlaneHealth(c *federation.ClusterSummary) ComponentHealth {
	if c.ControlPlaneReady {
		return ComponentHealth{
			Status:  ComponentStatusHealthy,
			Message: "Control plane is ready",
		}
	}
	return ComponentHealth{
		Status:  ComponentStatusUnhealthy,
		Message: "Control plane is not ready",
	}
}

// buildInfrastructureHealth builds infrastructure health from cluster status.
func buildInfrastructureHealth(c *federation.ClusterSummary) ComponentHealth {
	if c.InfrastructureReady {
		return ComponentHealth{
			Status:  ComponentStatusHealthy,
			Message: "Infrastructure is ready",
		}
	}
	return ComponentHealth{
		Status:  ComponentStatusUnhealthy,
		Message: "Infrastructure is not ready",
	}
}

// buildNodesHealth builds nodes health from cluster status.
func buildNodesHealth(c *federation.ClusterSummary) ComponentHealth {
	if c.NodeCount > 0 {
		return ComponentHealth{
			Status:  ComponentStatusHealthy,
			Ready:   c.NodeCount,
			Total:   c.NodeCount,
			Message: formatInt(c.NodeCount) + " node(s) ready",
		}
	}
	return ComponentHealth{
		Status:  ComponentStatusUnknown,
		Message: "Node count unavailable",
	}
}

// determineOverallHealth determines the overall health status based on components.
func determineOverallHealth(c *federation.ClusterSummary, comp ClusterHealthComponents) (string, string) {
	// If the cluster is deleting, it's neither healthy nor unhealthy
	if c.Status == "Deleting" {
		return HealthStatusUnknown, "Cluster is being deleted"
	}

	// If provisioning, report as degraded
	if c.Status == "Provisioning" {
		return HealthStatusDegraded, "Cluster is still provisioning"
	}

	// If fully ready, it's healthy
	if c.Ready {
		return HealthStatusHealthy, "Cluster is healthy and ready"
	}

	// Check individual components
	unhealthyCount := 0
	if comp.ControlPlane.Status == ComponentStatusUnhealthy {
		unhealthyCount++
	}
	if comp.Infrastructure.Status == ComponentStatusUnhealthy {
		unhealthyCount++
	}

	if unhealthyCount >= 2 {
		return HealthStatusUnhealthy, "Multiple components are unhealthy"
	}
	if unhealthyCount == 1 {
		return HealthStatusDegraded, "One or more components are not ready"
	}

	return HealthStatusUnknown, "Unable to determine cluster health"
}

// buildHealthChecks builds a list of health checks from cluster status.
func buildHealthChecks(c *federation.ClusterSummary) []HealthCheck {
	checks := make([]HealthCheck, 0, 4)

	// Control plane ready check
	cpCheck := HealthCheck{Name: "control-plane-ready"}
	if c.ControlPlaneReady {
		cpCheck.Status = CheckStatusPass
		cpCheck.Message = "Control plane is ready"
	} else {
		cpCheck.Status = CheckStatusFail
		cpCheck.Message = "Control plane is not ready"
	}
	checks = append(checks, cpCheck)

	// Infrastructure ready check
	infraCheck := HealthCheck{Name: "infrastructure-ready"}
	if c.InfrastructureReady {
		infraCheck.Status = CheckStatusPass
		infraCheck.Message = "Infrastructure is ready"
	} else {
		infraCheck.Status = CheckStatusFail
		infraCheck.Message = "Infrastructure is not ready"
	}
	checks = append(checks, infraCheck)

	// Cluster phase check
	phaseCheck := HealthCheck{Name: "cluster-phase"}
	switch c.Status {
	case "Provisioned":
		phaseCheck.Status = CheckStatusPass
		phaseCheck.Message = "Cluster is provisioned"
	case "Provisioning":
		phaseCheck.Status = CheckStatusWarn
		phaseCheck.Message = "Cluster is still provisioning"
	case "Deleting":
		phaseCheck.Status = CheckStatusWarn
		phaseCheck.Message = "Cluster is being deleted"
	case "Failed":
		phaseCheck.Status = CheckStatusFail
		phaseCheck.Message = "Cluster is in failed state"
	default:
		phaseCheck.Status = CheckStatusWarn
		phaseCheck.Message = "Cluster phase: " + c.Status
	}
	checks = append(checks, phaseCheck)

	// Node health check
	nodeCheck := HealthCheck{Name: "nodes"}
	if c.NodeCount > 0 {
		nodeCheck.Status = CheckStatusPass
		nodeCheck.Message = formatInt(c.NodeCount) + " worker node(s) detected"
	} else {
		nodeCheck.Status = CheckStatusWarn
		nodeCheck.Message = "No worker node information available"
	}
	checks = append(checks, nodeCheck)

	return checks
}
