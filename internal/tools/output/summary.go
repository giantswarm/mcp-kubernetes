package output

import (
	"sort"
	"strings"
)

// ResourceSummary provides aggregated counts for large query results.
// This dramatically reduces response size while maintaining usefulness.
type ResourceSummary struct {
	// Total is the total number of resources
	Total int `json:"total"`

	// ByStatus shows counts grouped by status (e.g., Running, Pending, Failed)
	ByStatus map[string]int `json:"byStatus,omitempty"`

	// ByCluster shows counts grouped by cluster name
	ByCluster map[string]int `json:"byCluster,omitempty"`

	// ByNamespace shows counts grouped by namespace
	ByNamespace map[string]int `json:"byNamespace,omitempty"`

	// ByKind shows counts grouped by resource kind
	ByKind map[string]int `json:"byKind,omitempty"`

	// Sample contains the first few resource names for context
	Sample []string `json:"sample,omitempty"`

	// HasMore indicates if there are more items not shown in sample
	HasMore bool `json:"hasMore,omitempty"`
}

// SummaryOptions configures summary generation.
type SummaryOptions struct {
	// MaxSampleSize limits the number of sample names included
	MaxSampleSize int

	// IncludeByStatus groups by status field
	IncludeByStatus bool

	// IncludeByCluster groups by cluster (for multi-cluster results)
	IncludeByCluster bool

	// IncludeByNamespace groups by namespace
	IncludeByNamespace bool

	// IncludeByKind groups by resource kind
	IncludeByKind bool

	// ClusterField is the field path for cluster name (default: metadata.annotations.cluster)
	ClusterField string
}

// DefaultSummaryOptions returns sensible defaults for summary generation.
func DefaultSummaryOptions() *SummaryOptions {
	return &SummaryOptions{
		MaxSampleSize:      10,
		IncludeByStatus:    true,
		IncludeByCluster:   false, // Only enable for multi-cluster queries
		IncludeByNamespace: true,
		IncludeByKind:      false, // Usually querying single kind
		ClusterField:       "metadata.labels.cluster",
	}
}

// GenerateSummary creates a summary from a list of resources.
func GenerateSummary(objects []map[string]interface{}, opts *SummaryOptions) *ResourceSummary {
	if opts == nil {
		opts = DefaultSummaryOptions()
	}

	summary := &ResourceSummary{
		Total:       len(objects),
		ByStatus:    make(map[string]int),
		ByCluster:   make(map[string]int),
		ByNamespace: make(map[string]int),
		ByKind:      make(map[string]int),
		Sample:      make([]string, 0, opts.MaxSampleSize),
	}

	for i, obj := range objects {
		// Collect sample names
		if i < opts.MaxSampleSize {
			if name := getResourceName(obj); name != "" {
				summary.Sample = append(summary.Sample, name)
			}
		}

		// Group by status
		if opts.IncludeByStatus {
			if status := extractStatus(obj); status != "" {
				summary.ByStatus[status]++
			}
		}

		// Group by cluster
		if opts.IncludeByCluster {
			if cluster := extractFieldValue(obj, opts.ClusterField); cluster != "" {
				summary.ByCluster[cluster]++
			}
		}

		// Group by namespace
		if opts.IncludeByNamespace {
			if ns := extractNamespace(obj); ns != "" {
				summary.ByNamespace[ns]++
			}
		}

		// Group by kind
		if opts.IncludeByKind {
			if kind := extractKind(obj); kind != "" {
				summary.ByKind[kind]++
			}
		}
	}

	// Set HasMore if there are more items than sample
	summary.HasMore = len(objects) > opts.MaxSampleSize

	// Clean up empty maps
	if len(summary.ByStatus) == 0 {
		summary.ByStatus = nil
	}
	if len(summary.ByCluster) == 0 {
		summary.ByCluster = nil
	}
	if len(summary.ByNamespace) == 0 {
		summary.ByNamespace = nil
	}
	if len(summary.ByKind) == 0 {
		summary.ByKind = nil
	}

	return summary
}

// GenerateFleetSummary creates a summary optimized for fleet-wide queries.
func GenerateFleetSummary(objects []map[string]interface{}, clusterField string) *ResourceSummary {
	opts := DefaultSummaryOptions()
	opts.IncludeByCluster = true
	opts.IncludeByNamespace = false // Less relevant in fleet context
	if clusterField != "" {
		opts.ClusterField = clusterField
	}
	return GenerateSummary(objects, opts)
}

// ShouldUseSummary determines if summary mode should be suggested.
// Returns true if the result set is large enough to benefit from summarization.
func ShouldUseSummary(itemCount, threshold int) bool {
	if threshold <= 0 {
		threshold = 500 // Default threshold
	}
	return itemCount > threshold
}

// getResourceName extracts the resource name from metadata.
func getResourceName(obj map[string]interface{}) string {
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return ""
	}

	name, _ := metadata["name"].(string)
	namespace, _ := metadata["namespace"].(string)

	if namespace != "" {
		return namespace + "/" + name
	}
	return name
}

// extractStatus extracts status from various resource types.
func extractStatus(obj map[string]interface{}) string {
	kind := strings.ToLower(extractKind(obj))

	// Handle different resource types
	switch kind {
	case "pod":
		return extractPodStatus(obj)
	case "deployment", "replicaset", "statefulset", "daemonset":
		return extractWorkloadStatus(obj)
	case "node":
		return extractNodeStatus(obj)
	case "persistentvolumeclaim", "pvc":
		return extractPhaseStatus(obj)
	case "job":
		return extractJobStatus(obj)
	default:
		// Try generic status.phase
		return extractPhaseStatus(obj)
	}
}

// extractPodStatus extracts status from a Pod resource.
func extractPodStatus(obj map[string]interface{}) string {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return "Unknown"
	}

	phase, _ := status["phase"].(string)
	if phase != "" {
		return phase
	}

	return "Unknown"
}

// extractWorkloadStatus extracts status from workload resources.
func extractWorkloadStatus(obj map[string]interface{}) string {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return "Unknown"
	}

	// Check if fully available
	replicas, _ := getNestedInt(obj, "spec", "replicas")
	availableReplicas, _ := getNestedInt(status, "availableReplicas")
	readyReplicas, _ := getNestedInt(status, "readyReplicas")

	if replicas == 0 {
		return "Scaled to Zero"
	}
	if readyReplicas >= replicas && availableReplicas >= replicas {
		return "Ready"
	}
	if readyReplicas > 0 {
		return "Partially Ready"
	}
	return "Not Ready"
}

// extractNodeStatus extracts status from a Node resource.
func extractNodeStatus(obj map[string]interface{}) string {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return "Unknown"
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return "Unknown"
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		condStatus, _ := condMap["status"].(string)

		if condType == "Ready" {
			if condStatus == "True" {
				return "Ready"
			}
			return "NotReady"
		}
	}

	return "Unknown"
}

// extractJobStatus extracts status from a Job resource.
func extractJobStatus(obj map[string]interface{}) string {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return "Unknown"
	}

	// Check conditions
	conditions, ok := status["conditions"].([]interface{})
	if ok {
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)

			if condStatus == "True" {
				switch condType {
				case "Complete":
					return "Succeeded"
				case "Failed":
					return "Failed"
				}
			}
		}
	}

	// Check succeeded/failed counts
	succeeded, _ := getNestedInt(status, "succeeded")
	failed, _ := getNestedInt(status, "failed")

	if succeeded > 0 {
		return "Succeeded"
	}
	if failed > 0 {
		return "Failed"
	}

	return "Running"
}

// extractPhaseStatus extracts status.phase from a resource.
func extractPhaseStatus(obj map[string]interface{}) string {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return ""
	}

	phase, _ := status["phase"].(string)
	return phase
}

// extractNamespace extracts the namespace from metadata.
func extractNamespace(obj map[string]interface{}) string {
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return ""
	}

	namespace, _ := metadata["namespace"].(string)
	return namespace
}

// extractKind extracts the kind from a resource.
func extractKind(obj map[string]interface{}) string {
	kind, _ := obj["kind"].(string)
	return kind
}

// extractFieldValue extracts a value at a dot-separated path.
func extractFieldValue(obj map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	current := interface{}(obj)

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = m[part]
	}

	str, _ := current.(string)
	return str
}

// getNestedInt extracts an integer from a nested path.
func getNestedInt(obj interface{}, keys ...string) (int, bool) {
	current := obj
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return 0, false
		}
		current = m[key]
	}

	// Handle different numeric types
	switch v := current.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// TopCounts returns the top N entries from a count map.
func TopCounts(counts map[string]int, n int) []CountEntry {
	if len(counts) == 0 {
		return nil
	}

	entries := make([]CountEntry, 0, len(counts))
	for k, v := range counts {
		entries = append(entries, CountEntry{Key: k, Count: v})
	}

	// Sort by count descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})

	if n > 0 && len(entries) > n {
		return entries[:n]
	}
	return entries
}

// CountEntry represents a key-count pair.
type CountEntry struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}
