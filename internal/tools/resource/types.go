package resource

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceSummary represents a compact view of a Kubernetes resource
type ResourceSummary struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Kind              string            `json:"kind"`
	APIVersion        string            `json:"apiVersion"`
	CreationTimestamp string            `json:"creationTimestamp"`
	Age               string            `json:"age"`
	Status            string            `json:"status,omitempty"`
	Ready             string            `json:"ready,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	// Additional fields specific to resource types
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// ListSummaryResponse contains the summarized list of resources
type ListSummaryResponse struct {
	Kind     string            `json:"kind"`
	Items    []ResourceSummary `json:"items"`
	Total    int               `json:"total"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PaginatedSummaryResponse contains a paginated list of summarized resources with metadata
type PaginatedSummaryResponse struct {
	Kind            string                 `json:"kind"`
	Items           []ResourceSummary      `json:"items"`
	Continue        string                 `json:"continue,omitempty"`        // Token for next page
	RemainingItems  *int64                 `json:"remainingItems,omitempty"`  // Estimated remaining items (if available)
	ResourceVersion string                 `json:"resourceVersion,omitempty"` // Resource version for consistency
	TotalItems      int                    `json:"totalItems"`                // Number of items in this response
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// SummarizeResources converts a list of runtime.Objects to compact ResourceSummary objects
func SummarizeResources(objects []runtime.Object, includeLabels, includeAnnotations bool) *ListSummaryResponse {
	if len(objects) == 0 {
		return &ListSummaryResponse{
			Kind:  "List",
			Items: []ResourceSummary{},
			Total: 0,
		}
	}

	summaries := make([]ResourceSummary, 0, len(objects))
	var resourceKind string

	for _, obj := range objects {
		if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
			summary := summarizeResource(unstructuredObj, includeLabels, includeAnnotations)
			summaries = append(summaries, summary)

			// Set resource kind from first object
			if resourceKind == "" {
				resourceKind = unstructuredObj.GetKind()
			}
		}
	}

	return &ListSummaryResponse{
		Kind:  fmt.Sprintf("%sList", resourceKind),
		Items: summaries,
		Total: len(summaries),
	}
}

// SummarizePaginatedResources converts a paginated list of runtime.Objects to compact ResourceSummary objects
func SummarizePaginatedResources(objects []runtime.Object, includeLabels, includeAnnotations bool, continue_ string, resourceVersion string, remainingItems *int64) *PaginatedSummaryResponse {
	if len(objects) == 0 {
		return &PaginatedSummaryResponse{
			Kind:            "List",
			Items:           []ResourceSummary{},
			TotalItems:      0,
			Continue:        continue_,
			ResourceVersion: resourceVersion,
			RemainingItems:  remainingItems,
		}
	}

	summaries := make([]ResourceSummary, 0, len(objects))
	var resourceKind string

	for _, obj := range objects {
		if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
			summary := summarizeResource(unstructuredObj, includeLabels, includeAnnotations)
			summaries = append(summaries, summary)

			// Set resource kind from first object
			if resourceKind == "" {
				resourceKind = unstructuredObj.GetKind()
			}
		}
	}

	return &PaginatedSummaryResponse{
		Kind:            fmt.Sprintf("%sList", resourceKind),
		Items:           summaries,
		TotalItems:      len(summaries),
		Continue:        continue_,
		ResourceVersion: resourceVersion,
		RemainingItems:  remainingItems,
	}
}

// summarizeResource extracts key information from an unstructured Kubernetes resource
func summarizeResource(obj *unstructured.Unstructured, includeLabels, includeAnnotations bool) ResourceSummary {
	summary := ResourceSummary{
		Name:              obj.GetName(),
		Namespace:         obj.GetNamespace(),
		Kind:              obj.GetKind(),
		APIVersion:        obj.GetAPIVersion(),
		CreationTimestamp: obj.GetCreationTimestamp().Format(time.RFC3339),
		Age:               formatAge(obj.GetCreationTimestamp().Time),
		Extra:             make(map[string]interface{}),
	}

	// Include labels if requested and not empty
	if includeLabels {
		labels := obj.GetLabels()
		if len(labels) > 0 {
			summary.Labels = labels
		}
	}

	// Include annotations if requested and not empty
	if includeAnnotations {
		annotations := obj.GetAnnotations()
		if len(annotations) > 0 {
			// Filter out system annotations to reduce noise
			filteredAnnotations := make(map[string]string)
			for k, v := range annotations {
				if !isSystemAnnotation(k) {
					filteredAnnotations[k] = v
				}
			}
			if len(filteredAnnotations) > 0 {
				summary.Annotations = filteredAnnotations
			}
		}
	}

	// Extract resource-specific information
	extractResourceSpecificInfo(obj, &summary)

	return summary
}

// extractResourceSpecificInfo adds resource-type-specific fields to the summary
func extractResourceSpecificInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	kind := strings.ToLower(obj.GetKind())

	switch kind {
	case "pod":
		extractPodInfo(obj, summary)
	case "deployment":
		extractDeploymentInfo(obj, summary)
	case "service":
		extractServiceInfo(obj, summary)
	case "replicaset":
		extractReplicaSetInfo(obj, summary)
	case "statefulset":
		extractStatefulSetInfo(obj, summary)
	case "daemonset":
		extractDaemonSetInfo(obj, summary)
	case "configmap":
		extractConfigMapInfo(obj, summary)
	case "secret":
		extractSecretInfo(obj, summary)
	case "persistentvolumeclaim", "pvc":
		extractPVCInfo(obj, summary)
	case "ingress":
		extractIngressInfo(obj, summary)
	case "job":
		extractJobInfo(obj, summary)
	case "cronjob":
		extractCronJobInfo(obj, summary)
	case "node":
		extractNodeInfo(obj, summary)
	}
}

func extractPodInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if status, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
		summary.Status = status
	}

	// Calculate ready containers
	var readyContainers, totalContainers int
	if containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses"); found {
		totalContainers = len(containerStatuses)
		for _, cs := range containerStatuses {
			if csMap, ok := cs.(map[string]interface{}); ok {
				if ready, found, _ := unstructured.NestedBool(csMap, "ready"); found && ready {
					readyContainers++
				}
			}
		}
		summary.Ready = fmt.Sprintf("%d/%d", readyContainers, totalContainers)
	}

	// Add container count
	if containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "containers"); found {
		summary.Extra["containers"] = len(containers)
	}

	// Add node name
	if nodeName, found, _ := unstructured.NestedString(obj.Object, "spec", "nodeName"); found {
		summary.Extra["node"] = nodeName
	}

	// Add restart count
	if containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses"); found {
		var totalRestarts int64
		for _, cs := range containerStatuses {
			if csMap, ok := cs.(map[string]interface{}); ok {
				if restarts, found, _ := unstructured.NestedInt64(csMap, "restartCount"); found {
					totalRestarts += restarts
				}
			}
		}
		summary.Extra["restarts"] = totalRestarts
	}
}

func extractDeploymentInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		summary.Extra["replicas"] = replicas
	}

	if readyReplicas, found, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas"); found {
		summary.Extra["readyReplicas"] = readyReplicas
	}

	if availableReplicas, found, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas"); found {
		summary.Extra["availableReplicas"] = availableReplicas
	}

	// Set ready status
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, replicas)
}

func extractServiceInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if serviceType, found, _ := unstructured.NestedString(obj.Object, "spec", "type"); found {
		summary.Extra["type"] = serviceType
	}

	if clusterIP, found, _ := unstructured.NestedString(obj.Object, "spec", "clusterIP"); found {
		summary.Extra["clusterIP"] = clusterIP
	}

	// Add ports
	if ports, found, _ := unstructured.NestedSlice(obj.Object, "spec", "ports"); found {
		portStrings := make([]string, 0, len(ports))
		for _, port := range ports {
			if portMap, ok := port.(map[string]interface{}); ok {
				portNum, _, _ := unstructured.NestedInt64(portMap, "port")
				protocol, _, _ := unstructured.NestedString(portMap, "protocol")
				if protocol == "" {
					protocol = "TCP"
				}
				portStrings = append(portStrings, fmt.Sprintf("%d/%s", portNum, protocol))
			}
		}
		summary.Extra["ports"] = strings.Join(portStrings, ",")
	}
}

func extractReplicaSetInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, replicas)
	summary.Extra["replicas"] = replicas
}

func extractStatefulSetInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, replicas)
	summary.Extra["replicas"] = replicas
}

func extractDaemonSetInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	desired, _, _ := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "numberReady")
	summary.Ready = fmt.Sprintf("%d/%d", ready, desired)
}

func extractConfigMapInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if data, found, _ := unstructured.NestedMap(obj.Object, "data"); found {
		summary.Extra["keys"] = len(data)
	}
}

func extractSecretInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if data, found, _ := unstructured.NestedMap(obj.Object, "data"); found {
		summary.Extra["keys"] = len(data)
	}
	if secretType, found, _ := unstructured.NestedString(obj.Object, "type"); found {
		summary.Extra["type"] = secretType
	}
}

func extractPVCInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if phase, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
		summary.Status = phase
	}
	if capacity, found, _ := unstructured.NestedMap(obj.Object, "status", "capacity"); found {
		if storage, ok := capacity["storage"]; ok {
			summary.Extra["capacity"] = storage
		}
	}
}

func extractIngressInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if rules, found, _ := unstructured.NestedSlice(obj.Object, "spec", "rules"); found {
		hosts := make([]string, 0, len(rules))
		for _, rule := range rules {
			if ruleMap, ok := rule.(map[string]interface{}); ok {
				if host, found, _ := unstructured.NestedString(ruleMap, "host"); found && host != "" {
					hosts = append(hosts, host)
				}
			}
		}
		if len(hosts) > 0 {
			summary.Extra["hosts"] = strings.Join(hosts, ",")
		}
	}
}

func extractJobInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	succeeded, _, _ := unstructured.NestedInt64(obj.Object, "status", "succeeded")
	failed, _, _ := unstructured.NestedInt64(obj.Object, "status", "failed")

	if succeeded > 0 {
		summary.Status = "Succeeded"
	} else if failed > 0 {
		summary.Status = "Failed"
	} else {
		summary.Status = "Running"
	}

	summary.Extra["succeeded"] = succeeded
	summary.Extra["failed"] = failed
}

func extractCronJobInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if schedule, found, _ := unstructured.NestedString(obj.Object, "spec", "schedule"); found {
		summary.Extra["schedule"] = schedule
	}
	if lastSchedule, found, _ := unstructured.NestedString(obj.Object, "status", "lastScheduleTime"); found {
		summary.Extra["lastSchedule"] = lastSchedule
	}
}

func extractNodeInfo(obj *unstructured.Unstructured, summary *ResourceSummary) {
	// Check node conditions for ready status
	if conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); found {
		for _, condition := range conditions {
			if condMap, ok := condition.(map[string]interface{}); ok {
				if condType, found, _ := unstructured.NestedString(condMap, "type"); found && condType == "Ready" {
					if status, found, _ := unstructured.NestedString(condMap, "status"); found {
						summary.Status = status
					}
				}
			}
		}
	}
}

// formatAge formats a time duration as a human-readable age string
func formatAge(t time.Time) string {
	duration := time.Since(t)

	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", int(duration.Seconds()))
}

// isSystemAnnotation checks if an annotation is a system annotation that should be filtered out
func isSystemAnnotation(key string) bool {
	systemPrefixes := []string{
		"kubectl.kubernetes.io/",
		"deployment.kubernetes.io/",
		"control-plane.alpha.kubernetes.io/",
		"node.alpha.kubernetes.io/",
		"volume.beta.kubernetes.io/",
		"pv.kubernetes.io/",
	}

	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
