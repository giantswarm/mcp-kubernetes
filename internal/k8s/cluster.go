package k8s

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Health status constants for cluster health reporting.
const (
	clusterHealthHealthy   = "Healthy"
	clusterHealthUnhealthy = "Unhealthy"
	clusterHealthDegraded  = "Degraded"
	clusterHealthUnknown   = "Unknown"
)

// ClusterManager implementation

// getAllAPIResources returns all available API resources in the cluster (helper method).
func (c *kubernetesClient) getAllAPIResources(ctx context.Context, kubeContext string) ([]APIResourceInfo, error) {
	// Validate operation
	if err := c.isOperationAllowed("api-resources"); err != nil {
		return nil, err
	}

	c.logOperation("get-api-resources", kubeContext, "", "", "")

	// Get discovery client for the context
	discoveryClient, err := c.getDiscoveryClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Get server preferred resources
	apiResourceLists, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get API resources: %w", err)
	}

	var apiResources []APIResourceInfo

	// Process each API resource list
	for _, apiResourceList := range apiResourceLists {
		// Parse group version
		gv, err := parseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			if c.config.Logger != nil {
				c.config.Logger.Warn("failed to parse group version",
					"groupVersion", apiResourceList.GroupVersion, "error", err)
			}
			continue
		}

		// Process each API resource in the list
		for _, apiResource := range apiResourceList.APIResources {
			// Skip sub-resources (they contain '/')
			if strings.Contains(apiResource.Name, "/") {
				continue
			}

			resourceInfo := APIResourceInfo{
				Name:         apiResource.Name,
				SingularName: apiResource.SingularName,
				Namespaced:   apiResource.Namespaced,
				Kind:         apiResource.Kind,
				Verbs:        apiResource.Verbs,
				Group:        gv.Group,
				Version:      gv.Version,
			}

			apiResources = append(apiResources, resourceInfo)
		}
	}

	return apiResources, nil
}

// GetAPIResources returns available API resources with pagination support.
func (c *kubernetesClient) GetAPIResources(ctx context.Context, kubeContext string, limit, offset int, apiGroup string, namespacedOnly bool, verbs []string) (*PaginatedAPIResourceResponse, error) {
	// First get all API resources using the helper method
	allResources, err := c.getAllAPIResources(ctx, kubeContext)
	if err != nil {
		return nil, err
	}

	// Apply filters
	var filteredResources []APIResourceInfo
	for _, resource := range allResources {
		// Filter by API group if specified
		if apiGroup != "" && resource.Group != apiGroup {
			continue
		}

		// Filter by namespaced if specified
		if namespacedOnly && !resource.Namespaced {
			continue
		}

		// Filter by verbs if specified
		if len(verbs) > 0 {
			hasAllVerbs := true
			for _, verb := range verbs {
				found := false
				for _, resourceVerb := range resource.Verbs {
					if resourceVerb == verb {
						found = true
						break
					}
				}
				if !found {
					hasAllVerbs = false
					break
				}
			}
			if !hasAllVerbs {
				continue
			}
		}

		filteredResources = append(filteredResources, resource)
	}

	totalCount := len(filteredResources)

	// Apply pagination
	if offset < 0 {
		offset = 0
	}

	var paginatedItems []APIResourceInfo
	hasMore := false
	nextOffset := 0

	if offset < totalCount {
		end := totalCount
		if limit > 0 && offset+limit < totalCount {
			end = offset + limit
			hasMore = true
			nextOffset = end
		}
		paginatedItems = filteredResources[offset:end]
	}

	return &PaginatedAPIResourceResponse{
		Items:      paginatedItems,
		TotalItems: len(paginatedItems),
		TotalCount: totalCount,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	}, nil
}

// GetClusterHealth returns the health status of the cluster.
func (c *kubernetesClient) GetClusterHealth(ctx context.Context, kubeContext string) (*ClusterHealth, error) {
	// Validate operation
	if err := c.isOperationAllowed("cluster-health"); err != nil {
		return nil, err
	}

	c.logOperation("get-cluster-health", kubeContext, "", "", "")

	// Get clientset for the context
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return nil, err
	}

	health := &ClusterHealth{
		Status:     clusterHealthUnknown,
		Components: []ComponentHealth{},
		Nodes:      []NodeHealth{},
	}

	// Check cluster version/connectivity
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		health.Status = clusterHealthUnhealthy
		health.Components = append(health.Components, ComponentHealth{
			Name:    "API Server",
			Status:  clusterHealthUnhealthy,
			Message: fmt.Sprintf("Failed to get server version: %v", err),
		})
		return health, nil
	}

	// API Server is healthy if we can get version
	health.Components = append(health.Components, ComponentHealth{
		Name:    "API Server",
		Status:  clusterHealthHealthy,
		Message: fmt.Sprintf("Version: %s", version.String()),
	})

	// Check component statuses (if available)
	componentStatuses, err := clientset.CoreV1().ComponentStatuses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if c.config.Logger != nil {
			c.config.Logger.Warn("failed to get component statuses", "error", err)
		}
	} else {
		for _, component := range componentStatuses.Items {
			componentHealth := ComponentHealth{
				Name:   component.Name,
				Status: clusterHealthUnknown,
			}

			// Check if component is healthy
			for _, condition := range component.Conditions {
				if condition.Type == corev1.ComponentHealthy {
					if condition.Status == corev1.ConditionTrue {
						componentHealth.Status = clusterHealthHealthy
					} else {
						componentHealth.Status = clusterHealthUnhealthy
						componentHealth.Message = condition.Message
					}
					break
				}
			}

			health.Components = append(health.Components, componentHealth)
		}
	}

	// Check node health
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if c.config.Logger != nil {
			c.config.Logger.Warn("failed to get nodes", "error", err)
		}
	} else {
		for _, node := range nodes.Items {
			nodeHealth := NodeHealth{
				Name:       node.Name,
				Ready:      false,
				Conditions: node.Status.Conditions,
			}

			// Check if node is ready
			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					nodeHealth.Ready = condition.Status == corev1.ConditionTrue
					break
				}
			}

			health.Nodes = append(health.Nodes, nodeHealth)
		}
	}

	// Determine overall cluster health
	health.Status = c.calculateOverallHealth(health.Components, health.Nodes)

	return health, nil
}

// Helper methods for cluster operations

// parseGroupVersion parses a group/version string.
func parseGroupVersion(groupVersion string) (GroupVersion, error) {
	if groupVersion == "" {
		return GroupVersion{}, fmt.Errorf("empty group version")
	}

	// Handle core API group (no group prefix)
	if !strings.Contains(groupVersion, "/") {
		return GroupVersion{
			Group:   "",
			Version: groupVersion,
		}, nil
	}

	// Split group/version
	parts := strings.SplitN(groupVersion, "/", 2)
	if len(parts) != 2 {
		return GroupVersion{}, fmt.Errorf("invalid group version format: %s", groupVersion)
	}

	return GroupVersion{
		Group:   parts[0],
		Version: parts[1],
	}, nil
}

// GroupVersion represents a Kubernetes API group and version.
type GroupVersion struct {
	Group   string
	Version string
}

// calculateOverallHealth determines the overall cluster health based on components and nodes.
func (c *kubernetesClient) calculateOverallHealth(components []ComponentHealth, nodes []NodeHealth) string {
	// Check if any critical components are unhealthy
	criticalComponents := map[string]bool{
		"etcd":                    true,
		"kube-apiserver":          true,
		"kube-controller-manager": true,
		"kube-scheduler":          true,
	}

	for _, component := range components {
		if criticalComponents[component.Name] && component.Status == clusterHealthUnhealthy {
			return clusterHealthUnhealthy
		}
	}

	// Check if majority of nodes are ready
	if len(nodes) > 0 {
		readyNodes := 0
		for _, node := range nodes {
			if node.Ready {
				readyNodes++
			}
		}

		// If less than half the nodes are ready, cluster is degraded
		if readyNodes < len(nodes)/2 {
			return clusterHealthDegraded
		}
	}

	// Check if any components are unhealthy
	for _, component := range components {
		if component.Status == clusterHealthUnhealthy {
			return clusterHealthDegraded
		}
	}

	return clusterHealthHealthy
}
