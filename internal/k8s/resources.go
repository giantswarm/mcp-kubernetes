package k8s

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// ResourceManager implementation

// Get retrieves a specific resource by name and namespace.
func (c *kubernetesClient) Get(ctx context.Context, kubeContext, namespace, resourceType, apiGroup, name string) (runtime.Object, error) {
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
	gvr, namespaced, err := c.resolveResourceType(resourceType, apiGroup, kubeContext)
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

// List retrieves resources with pagination support.
func (c *kubernetesClient) List(ctx context.Context, kubeContext, namespace, resourceType, apiGroup string, opts ListOptions) (*PaginatedListResponse, error) {
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
	gvr, namespaced, err := c.resolveResourceType(resourceType, apiGroup, kubeContext)
	if err != nil {
		return nil, err
	}

	// Prepare list options with pagination
	listOpts := metav1.ListOptions{
		LabelSelector: opts.LabelSelector,
		FieldSelector: opts.FieldSelector,
	}

	// Add pagination parameters
	if opts.Limit > 0 {
		listOpts.Limit = opts.Limit
	}
	if opts.Continue != "" {
		listOpts.Continue = opts.Continue
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

	// Build paginated response
	response := &PaginatedListResponse{
		Items:           objects,
		Continue:        list.GetContinue(),
		ResourceVersion: list.GetResourceVersion(),
		TotalItems:      len(objects),
	}

	// Calculate remaining items if continue token is present
	if list.GetContinue() != "" {
		// Note: Kubernetes doesn't provide exact remaining count,
		// but we can indicate there are more items available
		remaining := int64(-1) // -1 indicates "more available but count unknown"
		response.RemainingItems = &remaining
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("listed resources",
			"resourceType", resourceType,
			"namespace", namespace,
			"count", len(objects),
			"continue", list.GetContinue(),
			"limit", opts.Limit)
	}

	return response, nil
}

// Describe provides detailed information about a resource.
func (c *kubernetesClient) Describe(ctx context.Context, kubeContext, namespace, resourceType, apiGroup, name string) (*ResourceDescription, error) {
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
	resource, err := c.Get(ctx, kubeContext, namespace, resourceType, apiGroup, name)
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
	existingObj, err := c.Get(ctx, kubeContext, namespace, unstruct.GetKind(), unstruct.GetAPIVersion(), unstruct.GetName())
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
func (c *kubernetesClient) Delete(ctx context.Context, kubeContext, namespace, resourceType, apiGroup, name string) error {
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
	gvr, namespaced, err := c.resolveResourceType(resourceType, apiGroup, kubeContext)
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
func (c *kubernetesClient) Patch(ctx context.Context, kubeContext, namespace, resourceType, apiGroup, name string, patchType types.PatchType, data []byte) (runtime.Object, error) {
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
	gvr, namespaced, err := c.resolveResourceType(resourceType, apiGroup, kubeContext)
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
func (c *kubernetesClient) Scale(ctx context.Context, kubeContext, namespace, resourceType, apiGroup, name string, replicas int32) error {
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

// resolveResourceType determines the GroupVersionResource for a given resource type.
// This method wraps resolveResourceTypeShared with debug logging support.
func (c *kubernetesClient) resolveResourceType(resourceType, apiGroup, contextName string) (schema.GroupVersionResource, bool, error) {
	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: starting", "resourceType", resourceType, "apiGroup", apiGroup, "contextName", contextName)
	}

	// Get discovery client for the context
	discoveryClient, err := c.getDiscoveryClient(contextName)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("resolveResourceType: failed to get discovery client", "error", err)
		}
		return schema.GroupVersionResource{}, false, fmt.Errorf("failed to get discovery client: %w", err)
	}

	// Use the shared implementation
	gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, c.builtinResources, discoveryClient)
	if err != nil {
		if c.config.DebugMode && c.config.Logger != nil {
			c.config.Logger.Error("resolveResourceType: resolution failed", "resourceType", resourceType, "error", err)
		}
		return gvr, namespaced, err
	}

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("resolveResourceType: resolved successfully",
			"resourceType", resourceType,
			"group", gvr.Group,
			"version", gvr.Version,
			"resource", gvr.Resource,
			"namespaced", namespaced)
	}

	return gvr, namespaced, nil
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
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("failed to parse API version: %w", err)
	}

	// For non-core groups, include version hint (e.g., \"apps/v1\") so resolution can prefer that version.
	// For core resources (empty group), we rely on the default core mappings.
	apiGroup := ""
	if gv.Group != "" {
		apiGroup = gv.String() // \"group/version\"
	}

	return c.resolveResourceType(obj.GetKind(), apiGroup, kubeContext)
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
