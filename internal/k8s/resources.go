package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// ResourceSummary represents minimal information about a Kubernetes resource
type ResourceSummary struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Kind              string            `json:"kind"`
	APIVersion        string            `json:"apiVersion"`
	Status            string            `json:"status,omitempty"`
	Age               string            `json:"age"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels,omitempty"`
	Ready             string            `json:"ready,omitempty"`
	AdditionalInfo    map[string]string `json:"additionalInfo,omitempty"`
}

// ResourceManager implementation

// Get retrieves a specific resource by name and namespace.
func (c *kubernetesClient) Get(ctx context.Context, kubeContext, namespace, resourceType, name string) (runtime.Object, error) {
	// Validate operation
	if err := c.isOperationAllowed("get"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	c.logOperation("get", kubeContext, namespace, resourceType, name)

	// Get dynamic client for the context
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Resolve resource type to GVR
	gvr, namespaced, err := c.resolveResourceType(resourceType, kubeContext)
	if err != nil {
		return nil, err
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// Get the resource
	obj, err := resourceInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get %s %q: %w", resourceType, name, err)
	}

	return obj, nil
}

// List retrieves all resources of a specific type in a namespace.
func (c *kubernetesClient) List(ctx context.Context, kubeContext, namespace, resourceType string, opts ListOptions) ([]runtime.Object, error) {
	// Validate operation
	if err := c.isOperationAllowed("list"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if !opts.AllNamespaces && namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	c.logOperation("list", kubeContext, namespace, resourceType, "")

	// Get dynamic client for the context
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Resolve resource type to GVR
	gvr, namespaced, err := c.resolveResourceType(resourceType, kubeContext)
	if err != nil {
		return nil, err
	}

	// Prepare list options
	listOpts := metav1.ListOptions{
		LabelSelector: opts.LabelSelector,
		FieldSelector: opts.FieldSelector,
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && !opts.AllNamespaces && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// List the resources
	list, err := resourceInterface.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", resourceType, err)
	}

	// Convert to []runtime.Object
	var objects []runtime.Object
	for _, item := range list.Items {
		objects = append(objects, &item)
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("listed resources", "resourceType", resourceType, "namespace", namespace, "count", len(objects))
	}

	return objects, nil
}

// ListSummary retrieves minimal summary information about resources of a specific type in a namespace.
func (c *kubernetesClient) ListSummary(ctx context.Context, kubeContext, namespace, resourceType string, opts ListOptions) ([]ResourceSummary, error) {
	// Validate operation
	if err := c.isOperationAllowed("list"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if !opts.AllNamespaces && namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	c.logOperation("list-summary", kubeContext, namespace, resourceType, "")

	// Get dynamic client for the context
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Resolve resource type to GVR
	gvr, namespaced, err := c.resolveResourceType(resourceType, kubeContext)
	if err != nil {
		return nil, err
	}

	// Prepare list options
	listOpts := metav1.ListOptions{
		LabelSelector: opts.LabelSelector,
		FieldSelector: opts.FieldSelector,
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && !opts.AllNamespaces && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// List the resources
	list, err := resourceInterface.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", resourceType, err)
	}

	// Convert to ResourceSummary
	var summaries []ResourceSummary
	for _, item := range list.Items {
		summary := c.createResourceSummary(&item)
		summaries = append(summaries, summary)
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("listed resource summaries", "resourceType", resourceType, "namespace", namespace, "count", len(summaries))
	}

	return summaries, nil
}

// createResourceSummary creates a minimal summary from a full Kubernetes resource
func (c *kubernetesClient) createResourceSummary(obj *unstructured.Unstructured) ResourceSummary {
	summary := ResourceSummary{
		Name:              obj.GetName(),
		Namespace:         obj.GetNamespace(),
		Kind:              obj.GetKind(),
		APIVersion:        obj.GetAPIVersion(),
		Labels:            obj.GetLabels(),
		CreationTimestamp: obj.GetCreationTimestamp().Time,
		Age:               formatAge(obj.GetCreationTimestamp().Time),
		AdditionalInfo:    make(map[string]string),
	}

	// Extract status and additional info based on resource type
	switch strings.ToLower(obj.GetKind()) {
	case "pod":
		c.extractPodSummary(obj, &summary)
	case "deployment":
		c.extractDeploymentSummary(obj, &summary)
	case "service":
		c.extractServiceSummary(obj, &summary)
	case "node":
		c.extractNodeSummary(obj, &summary)
	case "namespace":
		c.extractNamespaceSummary(obj, &summary)
	case "replicaset":
		c.extractReplicaSetSummary(obj, &summary)
	case "statefulset":
		c.extractStatefulSetSummary(obj, &summary)
	case "daemonset":
		c.extractDaemonSetSummary(obj, &summary)
	case "configmap", "secret":
		c.extractGenericSummary(obj, &summary)
	default:
		c.extractGenericSummary(obj, &summary)
	}

	return summary
}

// extractPodSummary extracts pod-specific summary information
func (c *kubernetesClient) extractPodSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	// Pod status
	if status, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
		summary.Status = status
	}

	// Pod ready status
	if conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); found {
		readyCount := 0
		totalCount := 0
		for _, condition := range conditions {
			if condMap, ok := condition.(map[string]interface{}); ok {
				if condType, found := condMap["type"].(string); found && condType == "Ready" {
					if status, found := condMap["status"].(string); found && status == "True" {
						readyCount = 1
					}
					totalCount = 1
					break
				}
			}
		}
		summary.Ready = fmt.Sprintf("%d/%d", readyCount, totalCount)
	}

	// Node information
	if nodeName, found, _ := unstructured.NestedString(obj.Object, "spec", "nodeName"); found {
		summary.AdditionalInfo["node"] = nodeName
	}

	// Restart count
	if containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses"); found {
		totalRestarts := int64(0)
		for _, status := range containerStatuses {
			if statusMap, ok := status.(map[string]interface{}); ok {
				if restartCount, found, _ := unstructured.NestedInt64(statusMap, "restartCount"); found {
					totalRestarts += restartCount
				}
			}
		}
		summary.AdditionalInfo["restarts"] = fmt.Sprintf("%d", totalRestarts)
	}
}

// extractDeploymentSummary extracts deployment-specific summary information
func (c *kubernetesClient) extractDeploymentSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	// Replicas information
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		summary.AdditionalInfo["replicas"] = fmt.Sprintf("%d", replicas)
	}

	readyReplicas, readyFound, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	availableReplicas, availableFound, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas")

	if readyFound && availableFound {
		summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, availableReplicas)
		if readyReplicas == availableReplicas {
			summary.Status = "Ready"
		} else {
			summary.Status = "Updating"
		}
	}

	// Deployment strategy
	if strategy, found, _ := unstructured.NestedString(obj.Object, "spec", "strategy", "type"); found {
		summary.AdditionalInfo["strategy"] = strategy
	}
}

// extractServiceSummary extracts service-specific summary information
func (c *kubernetesClient) extractServiceSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	// Service type
	if serviceType, found, _ := unstructured.NestedString(obj.Object, "spec", "type"); found {
		summary.Status = serviceType
	}

	// Cluster IP
	if clusterIP, found, _ := unstructured.NestedString(obj.Object, "spec", "clusterIP"); found {
		summary.AdditionalInfo["clusterIP"] = clusterIP
	}

	// External IP for LoadBalancer services
	if externalIPs, found, _ := unstructured.NestedSlice(obj.Object, "status", "loadBalancer", "ingress"); found && len(externalIPs) > 0 {
		if ingress, ok := externalIPs[0].(map[string]interface{}); ok {
			if ip, found := ingress["ip"].(string); found {
				summary.AdditionalInfo["externalIP"] = ip
			}
		}
	}

	// Ports
	if ports, found, _ := unstructured.NestedSlice(obj.Object, "spec", "ports"); found {
		var portStrings []string
		for _, port := range ports {
			if portMap, ok := port.(map[string]interface{}); ok {
				if portNum, found := portMap["port"].(int64); found {
					portStr := fmt.Sprintf("%d", portNum)
					if protocol, found := portMap["protocol"].(string); found && protocol != "TCP" {
						portStr += "/" + protocol
					}
					portStrings = append(portStrings, portStr)
				}
			}
		}
		if len(portStrings) > 0 {
			summary.AdditionalInfo["ports"] = strings.Join(portStrings, ",")
		}
	}
}

// extractNodeSummary extracts node-specific summary information
func (c *kubernetesClient) extractNodeSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	// Node ready status
	if conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); found {
		for _, condition := range conditions {
			if condMap, ok := condition.(map[string]interface{}); ok {
				if condType, found := condMap["type"].(string); found && condType == "Ready" {
					if status, found := condMap["status"].(string); found {
						if status == "True" {
							summary.Status = "Ready"
						} else {
							summary.Status = "NotReady"
						}
					}
					break
				}
			}
		}
	}

	// Node role
	if labels, found := summary.Labels, summary.Labels != nil; found {
		for key := range labels {
			if strings.HasPrefix(key, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(key, "node-role.kubernetes.io/")
				if role == "" {
					role = "worker"
				}
				summary.AdditionalInfo["role"] = role
				break
			}
		}
	}

	// Kubernetes version
	if version, found, _ := unstructured.NestedString(obj.Object, "status", "nodeInfo", "kubeletVersion"); found {
		summary.AdditionalInfo["version"] = version
	}
}

// extractNamespaceSummary extracts namespace-specific summary information
func (c *kubernetesClient) extractNamespaceSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if phase, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
		summary.Status = phase
	}
}

// extractReplicaSetSummary extracts replicaset-specific summary information
func (c *kubernetesClient) extractReplicaSetSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		summary.AdditionalInfo["replicas"] = fmt.Sprintf("%d", replicas)
	}

	readyReplicas, readyFound, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	if readyFound {
		summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, readyReplicas)
		if readyReplicas > 0 {
			summary.Status = "Ready"
		} else {
			summary.Status = "NotReady"
		}
	}
}

// extractStatefulSetSummary extracts statefulset-specific summary information
func (c *kubernetesClient) extractStatefulSetSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		summary.AdditionalInfo["replicas"] = fmt.Sprintf("%d", replicas)
	}

	readyReplicas, readyFound, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	if readyFound {
		summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, readyReplicas)
		if readyReplicas > 0 {
			summary.Status = "Ready"
		} else {
			summary.Status = "NotReady"
		}
	}
}

// extractDaemonSetSummary extracts daemonset-specific summary information
func (c *kubernetesClient) extractDaemonSetSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	desiredReplicas, desiredFound, _ := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	readyReplicas, readyFound, _ := unstructured.NestedInt64(obj.Object, "status", "numberReady")

	if desiredFound && readyFound {
		summary.Ready = fmt.Sprintf("%d/%d", readyReplicas, desiredReplicas)
		if readyReplicas == desiredReplicas {
			summary.Status = "Ready"
		} else {
			summary.Status = "Updating"
		}
	}
}

// extractGenericSummary extracts generic summary information for unknown resource types
func (c *kubernetesClient) extractGenericSummary(obj *unstructured.Unstructured, summary *ResourceSummary) {
	// Try to extract common status fields
	if status, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
		summary.Status = status
	} else if conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); found {
		// Look for a Ready condition
		for _, condition := range conditions {
			if condMap, ok := condition.(map[string]interface{}); ok {
				if condType, found := condMap["type"].(string); found && condType == "Ready" {
					if status, found := condMap["status"].(string); found {
						summary.Status = status
					}
					break
				}
			}
		}
	}
}

// formatAge formats a time duration as a human-readable age string
func formatAge(creationTime time.Time) string {
	duration := time.Since(creationTime)

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}

// Describe provides detailed information about a resource.
func (c *kubernetesClient) Describe(ctx context.Context, kubeContext, namespace, resourceType, name string) (*ResourceDescription, error) {
	// Validate operation
	if err := c.isOperationAllowed("describe"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	c.logOperation("describe", kubeContext, namespace, resourceType, name)

	// Get the resource first
	resource, err := c.Get(ctx, kubeContext, namespace, resourceType, name)
	if err != nil {
		return nil, err
	}

	// Get related events
	events, err := c.getResourceEvents(ctx, kubeContext, namespace, name)
	if err != nil {
		if c.config.Logger != nil {
			c.config.Logger.Warn("failed to get events for resource", "error", err)
		}
		// Don't fail the operation if events can't be retrieved
	}

	// Create resource description
	description := &ResourceDescription{
		Resource: resource,
		Events:   events,
		Metadata: make(map[string]interface{}),
	}

	// Add additional metadata if available
	if unstructuredObj, ok := resource.(*unstructured.Unstructured); ok {
		description.Metadata["kind"] = unstructuredObj.GetKind()
		description.Metadata["apiVersion"] = unstructuredObj.GetAPIVersion()
		description.Metadata["resourceVersion"] = unstructuredObj.GetResourceVersion()
		description.Metadata["uid"] = string(unstructuredObj.GetUID())
		description.Metadata["creationTimestamp"] = unstructuredObj.GetCreationTimestamp()
		description.Metadata["labels"] = unstructuredObj.GetLabels()
		description.Metadata["annotations"] = unstructuredObj.GetAnnotations()
	}

	return description, nil
}

// Create creates a new resource from the provided object.
func (c *kubernetesClient) Create(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error) {
	// Validate operation
	if err := c.isOperationAllowed("create"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstruct := &unstructured.Unstructured{Object: unstructuredObj}

	c.logOperation("create", kubeContext, namespace, unstruct.GetKind(), unstruct.GetName())

	// Get dynamic client
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Resolve GVR from the object
	gvr, namespaced, err := c.resolveGVRFromObject(kubeContext, unstruct)
	if err != nil {
		return nil, err
	}

	// Set namespace if needed
	if namespaced && namespace != "" {
		unstruct.SetNamespace(namespace)
	}

	// Prepare create options
	createOpts := metav1.CreateOptions{}
	if c.dryRun {
		createOpts.DryRun = []string{metav1.DryRunAll}
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// Create the resource
	result, err := resourceInterface.Create(ctx, unstruct, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", unstruct.GetKind(), err)
	}

	return result, nil
}

// Apply applies a resource configuration (create or update).
func (c *kubernetesClient) Apply(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error) {
	// Validate operation
	if err := c.isOperationAllowed("apply"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstruct := &unstructured.Unstructured{Object: unstructuredObj}

	c.logOperation("apply", kubeContext, namespace, unstruct.GetKind(), unstruct.GetName())

	// Try to get existing resource first
	existingObj, err := c.Get(ctx, kubeContext, namespace, unstruct.GetKind(), unstruct.GetName())
	if err != nil {
		// Resource doesn't exist, create it
		return c.Create(ctx, kubeContext, namespace, obj)
	}

	// Resource exists, update it
	if existingUnstruct, ok := existingObj.(*unstructured.Unstructured); ok {
		// Preserve resource version for update
		unstruct.SetResourceVersion(existingUnstruct.GetResourceVersion())
	}

	// Get dynamic client
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Resolve GVR from the object
	gvr, namespaced, err := c.resolveGVRFromObject(kubeContext, unstruct)
	if err != nil {
		return nil, err
	}

	// Set namespace if needed
	if namespaced && namespace != "" {
		unstruct.SetNamespace(namespace)
	}

	// Prepare update options
	updateOpts := metav1.UpdateOptions{}
	if c.dryRun {
		updateOpts.DryRun = []string{metav1.DryRunAll}
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// Update the resource
	result, err := resourceInterface.Update(ctx, unstruct, updateOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to apply %s: %w", unstruct.GetKind(), err)
	}

	return result, nil
}

// Delete removes a resource by name and namespace.
func (c *kubernetesClient) Delete(ctx context.Context, kubeContext, namespace, resourceType, name string) error {
	// Validate operation
	if err := c.isOperationAllowed("delete"); err != nil {
		return err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return err
		}
	}

	c.logOperation("delete", kubeContext, namespace, resourceType, name)

	// Get dynamic client
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return err
	}

	// Resolve resource type to GVR
	gvr, namespaced, err := c.resolveResourceType(resourceType, kubeContext)
	if err != nil {
		return err
	}

	// Prepare delete options
	deleteOpts := metav1.DeleteOptions{}
	if c.dryRun {
		deleteOpts.DryRun = []string{metav1.DryRunAll}
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// Delete the resource
	err = resourceInterface.Delete(ctx, name, deleteOpts)
	if err != nil {
		return fmt.Errorf("failed to delete %s %q: %w", resourceType, name, err)
	}

	return nil
}

// Patch updates specific fields of a resource.
func (c *kubernetesClient) Patch(ctx context.Context, kubeContext, namespace, resourceType, name string, patchType types.PatchType, data []byte) (runtime.Object, error) {
	// Validate operation
	if err := c.isOperationAllowed("patch"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return nil, err
		}
	}

	c.logOperation("patch", kubeContext, namespace, resourceType, name)

	// Get dynamic client
	dynamicClient, err := c.getDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	// Resolve resource type to GVR
	gvr, namespaced, err := c.resolveResourceType(resourceType, kubeContext)
	if err != nil {
		return nil, err
	}

	// Prepare patch options
	patchOpts := metav1.PatchOptions{}
	if c.dryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	// Prepare resource interface
	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// Patch the resource
	result, err := resourceInterface.Patch(ctx, name, patchType, data, patchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to patch %s %q: %w", resourceType, name, err)
	}

	return result, nil
}

// Scale changes the number of replicas for scalable resources.
func (c *kubernetesClient) Scale(ctx context.Context, kubeContext, namespace, resourceType, name string, replicas int32) error {
	// Validate operation
	if err := c.isOperationAllowed("scale"); err != nil {
		return err
	}

	// Validate namespace access
	if namespace != "" {
		if err := c.isNamespaceRestricted(namespace); err != nil {
			return err
		}
	}

	c.logOperation("scale", kubeContext, namespace, resourceType, name)

	// Get clientset for scaling operations
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return err
	}

	// Prepare scale options
	scaleOpts := metav1.UpdateOptions{}
	if c.dryRun {
		scaleOpts.DryRun = []string{metav1.DryRunAll}
	}

	// Handle different scalable resource types
	switch strings.ToLower(resourceType) {
	case "deployment", "deployments":
		scale, err := clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment scale: %w", err)
		}
		scale.Spec.Replicas = replicas
		_, err = clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, scaleOpts)
		if err != nil {
			return fmt.Errorf("failed to scale deployment: %w", err)
		}

	case "replicaset", "replicasets":
		scale, err := clientset.AppsV1().ReplicaSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get replicaset scale: %w", err)
		}
		scale.Spec.Replicas = replicas
		_, err = clientset.AppsV1().ReplicaSets(namespace).UpdateScale(ctx, name, scale, scaleOpts)
		if err != nil {
			return fmt.Errorf("failed to scale replicaset: %w", err)
		}

	case "statefulset", "statefulsets":
		scale, err := clientset.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get statefulset scale: %w", err)
		}
		scale.Spec.Replicas = replicas
		_, err = clientset.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, scaleOpts)
		if err != nil {
			return fmt.Errorf("failed to scale statefulset: %w", err)
		}

	default:
		return fmt.Errorf("resource type %q is not scalable", resourceType)
	}

	return nil
}

// Helper methods

// resolveResourceType determines the GroupVersionResource for a given resource type
func (c *kubernetesClient) resolveResourceType(resourceType, contextName string) (schema.GroupVersionResource, bool, error) {
	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: starting", "resourceType", resourceType, "contextName", contextName)
	}

	// Normalize to lower case for comparison
	resourceType = strings.ToLower(resourceType)

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: normalized resource", "normalizedType", resourceType)
	}

	// Check built-in resources first
	if gvr, exists := c.builtinResources[resourceType]; exists {
		namespaced := c.isResourceNamespaced(gvr)
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("resolveResourceType: found in builtin resources",
				"resourceType", resourceType,
				"group", gvr.Group,
				"version", gvr.Version,
				"resource", gvr.Resource,
				"namespaced", namespaced)
		}
		return gvr, namespaced, nil
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: not found in builtin, discovering from API")
	}

	// Get discovery client for the context
	discoveryClient, err := c.getDiscoveryClient(contextName)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("resolveResourceType: failed to get discovery client", "error", err)
		}
		return schema.GroupVersionResource{}, false, fmt.Errorf("failed to get discovery client: %w", err)
	}

	// Set up timeout for discovery API call to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: calling ServerPreferredResources with timeout")
	}

	type discoveryResult struct {
		resourceLists []*metav1.APIResourceList
		err           error
	}

	// Use goroutine with channel to implement timeout
	resultChan := make(chan discoveryResult, 1)
	go func() {
		resourceLists, err := discoveryClient.ServerPreferredResources()
		resultChan <- discoveryResult{resourceLists: resourceLists, err: err}
	}()

	var resourceLists []*metav1.APIResourceList
	select {
	case result := <-resultChan:
		resourceLists = result.resourceLists
		err = result.err
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("resolveResourceType: ServerPreferredResources completed", "listsCount", len(resourceLists), "error", err)
		}
	case <-ctx.Done():
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("resolveResourceType: ServerPreferredResources timed out")
		}
		return schema.GroupVersionResource{}, false, fmt.Errorf("API discovery timed out after 30 seconds")
	}

	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Warn("resolveResourceType: ServerPreferredResources returned error, continuing with partial results", "error", err)
		}
		// Continue with partial results - this is common and expected
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: searching through API resources", "targetResource", resourceType)
	}

	// Search through all API resources
	for _, resourceList := range resourceLists {
		if resourceList == nil {
			continue
		}

		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			if c.config.DebugMode && c.config.Logger != nil {
				c.config.Logger.Warn("resolveResourceType: failed to parse group version",
					"groupVersion", resourceList.GroupVersion, "error", err)
			}
			continue
		}

		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Debug("resolveResourceType: checking resource list",
				"groupVersion", resourceList.GroupVersion,
				"resourceCount", len(resourceList.APIResources))
		}

		for _, resource := range resourceList.APIResources {
			// Check if this resource matches what we're looking for
			matches := []string{
				strings.ToLower(resource.Name),         // e.g., "pods"
				strings.ToLower(resource.Kind),         // e.g., "Pod"
				strings.ToLower(resource.SingularName), // e.g., "pod"
			}

			// Also check short names
			for _, shortName := range resource.ShortNames {
				matches = append(matches, strings.ToLower(shortName))
			}

			if c.config.DebugMode && c.config.Logger != nil {
				c.config.Logger.Debug("resolveResourceType: checking resource",
					"resourceName", resource.Name,
					"kind", resource.Kind,
					"singular", resource.SingularName,
					"shortNames", resource.ShortNames,
					"matches", matches)
			}

			for _, match := range matches {
				if match == resourceType {
					gvr := gv.WithResource(resource.Name)
					if c.config.DebugMode && c.config.Logger != nil {
						c.config.Logger.Debug("resolveResourceType: found match!",
							"resourceType", resourceType,
							"matched", match,
							"group", gvr.Group,
							"version", gvr.Version,
							"resource", gvr.Resource,
							"namespaced", resource.Namespaced)
					}
					return gvr, resource.Namespaced, nil
				}
			}
		}
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Error("resolveResourceType: no matching resource found", "resourceType", resourceType)
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf("unknown resource type: %s", resourceType)
}

// isResourceNamespaced determines if a resource is namespaced based on its GroupVersionResource
func (c *kubernetesClient) isResourceNamespaced(gvr schema.GroupVersionResource) bool {
	// Cluster-scoped resources
	clusterScopedResources := map[string]bool{
		"nodes":                           true,
		"persistentvolumes":               true,
		"clusterroles":                    true,
		"clusterrolebindings":             true,
		"namespaces":                      true,
		"storageclasses":                  true,
		"ingressclasses":                  true,
		"priorityclasses":                 true,
		"runtimeclasses":                  true,
		"podsecuritypolicies":             true,
		"volumeattachments":               true,
		"csidrivers":                      true,
		"csinodes":                        true,
		"csistoragecapacities":            true,
		"mutatingwebhookconfigurations":   true,
		"validatingwebhookconfigurations": true,
		"customresourcedefinitions":       true,
		"apiservices":                     true,
	}

	return !clusterScopedResources[gvr.Resource]
}

// resolveGVRFromObject resolves GroupVersionResource from an unstructured object.
func (c *kubernetesClient) resolveGVRFromObject(kubeContext string, obj *unstructured.Unstructured) (schema.GroupVersionResource, bool, error) {
	_, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("failed to parse API version: %w", err)
	}

	return c.resolveResourceType(obj.GetKind(), kubeContext)
}

// getResourceEvents retrieves events related to a specific resource.
func (c *kubernetesClient) getResourceEvents(ctx context.Context, kubeContext, namespace, name string) ([]corev1.Event, error) {
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return nil, err
	}

	// List events with field selector for the specific resource
	eventList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", name),
	})
	if err != nil {
		return nil, err
	}

	return eventList.Items, nil
}
