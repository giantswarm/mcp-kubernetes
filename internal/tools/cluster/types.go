package cluster

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
)

// Default and maximum values for the kubernetes_cluster_health nodesLimit param.
const (
	// DefaultNodesLimit is the default cap on the number of nodes returned.
	DefaultNodesLimit = 20

	// MaxNodesLimit is the absolute maximum allowed for nodesLimit.
	MaxNodesLimit = 1000
)

// ClusterHealthOutput is the response shape for kubernetes_cluster_health.
// It wraps k8s.ClusterHealth so the tool layer can apply truncation and
// expose summary counters without changing the k8s.ClusterManager interface.
type ClusterHealthOutput struct {
	// Status is the overall cluster health status.
	Status string `json:"status"`

	// Components contains the per-component health entries.
	Components []k8s.ComponentHealth `json:"components"`

	// Nodes contains node health entries, possibly truncated to nodesLimit.
	Nodes []NodeHealthOutput `json:"nodes"`

	// TotalNodes is the total node count before truncation.
	TotalNodes int `json:"totalNodes"`

	// ReturnedNodes is the number of nodes returned in this response.
	ReturnedNodes int `json:"returnedNodes"`

	// ReadyNodes is the number of nodes with Ready=true across the full
	// node list (computed before truncation), so callers can detect
	// readiness issues even when results are truncated.
	ReadyNodes int `json:"readyNodes"`

	// NodesTruncated is true when the node list was clipped by nodesLimit.
	// When true, increase nodesLimit to see more.
	NodesTruncated bool `json:"nodesTruncated,omitempty"`
}

// NodeHealthOutput is the per-node health entry in the response.
// Conditions are omitted by default; set includeNodeConditions=true to
// include them.
type NodeHealthOutput struct {
	Name       string                 `json:"name"`
	Ready      bool                   `json:"ready"`
	Conditions []corev1.NodeCondition `json:"conditions,omitempty"`
}
