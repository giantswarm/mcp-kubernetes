package k8s

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"

	"github.com/giantswarm/mcp-kubernetes/internal/logging"
)

// Shared resource operation functions that can be used by both
// kubernetesClient and bearerTokenClient implementations.

// getResource retrieves a specific resource by name and namespace.
func getResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	builtinResources map[string]schema.GroupVersionResource, namespace, resourceType, apiGroup, name string) (runtime.Object, error) {

	gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, builtinResources, discoveryClient)
	if err != nil {
		return nil, err
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	obj, err := resourceInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get %s %q: %w", resourceType, name, err)
	}

	return obj, nil
}

// listResources retrieves resources with pagination support.
func listResources(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	builtinResources map[string]schema.GroupVersionResource, namespace, resourceType, apiGroup string, opts ListOptions) (*PaginatedListResponse, error) {

	listStart := time.Now()

	gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, builtinResources, discoveryClient)
	if err != nil {
		slog.Debug("resource type resolution failed",
			slog.String("resourceType", resourceType),
			slog.String("apiGroup", apiGroup),
			slog.Duration("elapsed", time.Since(listStart)),
			logging.SanitizedErr(err))
		return nil, err
	}
	slog.Debug("resolved resource type",
		slog.String("gvr", gvr.String()),
		slog.Bool("namespaced", namespaced),
		slog.Duration("elapsed", time.Since(listStart)))

	listOpts := metav1.ListOptions{
		LabelSelector: opts.LabelSelector,
		FieldSelector: opts.FieldSelector,
	}

	if opts.Limit > 0 {
		listOpts.Limit = opts.Limit
	}
	if opts.Continue != "" {
		listOpts.Continue = opts.Continue
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && !opts.AllNamespaces && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	list, err := resourceInterface.List(ctx, listOpts)
	if err != nil {
		slog.Debug("K8s API list failed",
			slog.String("resourceType", resourceType),
			slog.Bool("allNamespaces", opts.AllNamespaces),
			slog.Duration("elapsed", time.Since(listStart)),
			logging.SanitizedErr(err))
		return nil, fmt.Errorf("failed to list %s: %w", resourceType, err)
	}
	slog.Debug("K8s API list completed",
		slog.Int("items", len(list.Items)),
		slog.Duration("elapsed", time.Since(listStart)))

	var objects []runtime.Object
	for i := range list.Items {
		objects = append(objects, &list.Items[i])
	}

	response := &PaginatedListResponse{
		Items:           objects,
		Continue:        list.GetContinue(),
		ResourceVersion: list.GetResourceVersion(),
		TotalItems:      len(objects),
	}

	if list.GetContinue() != "" {
		remaining := int64(-1)
		response.RemainingItems = &remaining
	}

	return response, nil
}

// describeResource provides detailed information about a resource.
func describeResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	clientset kubernetes.Interface, builtinResources map[string]schema.GroupVersionResource, namespace, resourceType, apiGroup, name string) (*ResourceDescription, error) {

	resource, err := getResource(ctx, dynamicClient, discoveryClient, builtinResources, namespace, resourceType, apiGroup, name)
	if err != nil {
		return nil, err
	}

	events, _ := getResourceEventsShared(ctx, clientset, namespace, name)

	description := &ResourceDescription{
		Resource: resource,
		Events:   events,
		Metadata: make(map[string]interface{}),
	}

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

// createResource creates a new resource from the provided object.
func createResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	namespace string, obj runtime.Object, dryRun bool) (runtime.Object, error) {

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstruct := &unstructured.Unstructured{Object: unstructuredObj}

	gvr, namespaced, err := resolveGVRFromObjectShared(unstruct, discoveryClient)
	if err != nil {
		return nil, err
	}

	if namespaced && namespace != "" {
		unstruct.SetNamespace(namespace)
	}

	createOpts := metav1.CreateOptions{}
	if dryRun {
		createOpts.DryRun = []string{metav1.DryRunAll}
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	result, err := resourceInterface.Create(ctx, unstruct, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", unstruct.GetKind(), err)
	}

	return result, nil
}

// applyResource applies a resource configuration (create or update).
func applyResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	namespace string, obj runtime.Object, dryRun bool) (runtime.Object, error) {

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstruct := &unstructured.Unstructured{Object: unstructuredObj}

	gvr, namespaced, err := resolveGVRFromObjectShared(unstruct, discoveryClient)
	if err != nil {
		return nil, err
	}

	if namespaced && namespace != "" {
		unstruct.SetNamespace(namespace)
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	// Try to get existing resource
	existing, err := resourceInterface.Get(ctx, unstruct.GetName(), metav1.GetOptions{})
	if err != nil {
		// Resource doesn't exist, create it
		return createResource(ctx, dynamicClient, discoveryClient, namespace, obj, dryRun)
	}

	// Resource exists, update it
	unstruct.SetResourceVersion(existing.GetResourceVersion())

	updateOpts := metav1.UpdateOptions{}
	if dryRun {
		updateOpts.DryRun = []string{metav1.DryRunAll}
	}

	result, err := resourceInterface.Update(ctx, unstruct, updateOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to apply %s: %w", unstruct.GetKind(), err)
	}

	return result, nil
}

// deleteResource removes a resource by name and namespace.
func deleteResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	builtinResources map[string]schema.GroupVersionResource, namespace, resourceType, apiGroup, name string, dryRun bool) error {

	gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, builtinResources, discoveryClient)
	if err != nil {
		return err
	}

	deleteOpts := metav1.DeleteOptions{}
	if dryRun {
		deleteOpts.DryRun = []string{metav1.DryRunAll}
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	err = resourceInterface.Delete(ctx, name, deleteOpts)
	if err != nil {
		return fmt.Errorf("failed to delete %s %q: %w", resourceType, name, err)
	}

	return nil
}

// patchResource updates specific fields of a resource.
func patchResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	builtinResources map[string]schema.GroupVersionResource, namespace, resourceType, apiGroup, name string,
	patchType types.PatchType, data []byte, dryRun bool) (runtime.Object, error) {

	gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, builtinResources, discoveryClient)
	if err != nil {
		return nil, err
	}

	patchOpts := metav1.PatchOptions{}
	if dryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	result, err := resourceInterface.Patch(ctx, name, patchType, data, patchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to patch %s %q: %w", resourceType, name, err)
	}

	return result, nil
}

// scaleResource changes the number of replicas for scalable resources.
func scaleResource(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface,
	builtinResources map[string]schema.GroupVersionResource, namespace, resourceType, apiGroup, name string,
	replicas int32, dryRun bool) error {

	// For scale operations, we need to use a different approach with the dynamic client
	// We'll use JSON patch to update the replicas field

	patchData := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	patchOpts := metav1.PatchOptions{}
	if dryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, builtinResources, discoveryClient)
	if err != nil {
		return err
	}

	// Validate this is a scalable resource
	switch strings.ToLower(resourceType) {
	case "deployment", "deployments", "deploy",
		"replicaset", "replicasets", "rs",
		"statefulset", "statefulsets", "sts":
		// OK, these are scalable
	default:
		return fmt.Errorf("resource type %q is not scalable", resourceType)
	}

	var resourceInterface dynamic.ResourceInterface
	if namespaced && namespace != "" {
		resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = dynamicClient.Resource(gvr)
	}

	_, err = resourceInterface.Patch(ctx, name, types.MergePatchType, []byte(patchData), patchOpts)
	if err != nil {
		return fmt.Errorf("failed to scale %s %q: %w", resourceType, name, err)
	}

	return nil
}

// getLogs retrieves logs from a pod container.
func getLogs(ctx context.Context, clientset kubernetes.Interface, namespace, podName, containerName string, opts LogOptions) (io.ReadCloser, error) {
	logOpts := &corev1.PodLogOptions{
		Container:  containerName,
		Follow:     opts.Follow,
		Previous:   opts.Previous,
		Timestamps: opts.Timestamps,
	}

	if opts.SinceTime != nil {
		logOpts.SinceTime = &metav1.Time{Time: *opts.SinceTime}
	}

	if opts.TailLines != nil {
		logOpts.TailLines = opts.TailLines
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, logOpts)
	logs, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for pod %s/%s: %w", namespace, podName, err)
	}

	return logs, nil
}

// execInPod executes a command inside a pod container.
func execInPod(ctx context.Context, clientset kubernetes.Interface, restConfig *rest.Config,
	namespace, podName, containerName string, command []string, opts ExecOptions) (*ExecResult, error) {

	execReq := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, http.MethodPost, execReq.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	streamOpts := remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Tty:    opts.TTY,
	}

	err = exec.StreamWithContext(ctx, streamOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to execute command in pod %s/%s: %w", namespace, podName, err)
	}

	return &ExecResult{ExitCode: 0}, nil
}

// portForwardToPod sets up port forwarding to a pod.
func portForwardToPod(ctx context.Context, clientset kubernetes.Interface, restConfig *rest.Config,
	namespace, podName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {

	// Validate pod is running
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("pod %s/%s not found: %w", namespace, podName, err)
	}
	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("pod %s/%s is not running", namespace, podName)
	}

	// Parse ports
	localPorts := make([]int, len(ports))
	remotePorts := make([]int, len(ports))

	for i, port := range ports {
		if strings.Contains(port, ":") {
			parts := strings.Split(port, ":")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port format: %s", port)
			}

			localPort, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid local port: %s", parts[0])
			}

			remotePort, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid remote port: %s", parts[1])
			}

			localPorts[i] = localPort
			remotePorts[i] = remotePort
		} else {
			portNum, err := strconv.Atoi(port)
			if err != nil {
				return nil, fmt.Errorf("invalid port: %s", port)
			}

			localPorts[i] = portNum
			remotePorts[i] = portNum
		}
	}

	portForwardReq := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("portforward")

	roundTripper, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, portForwardReq.URL())

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)

	stdout := opts.Stdout
	stderr := opts.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			errChan <- err
		}
	}()

	select {
	case <-readyChan:
		// Ready
	case err := <-errChan:
		close(stopChan)
		return nil, fmt.Errorf("port forwarding failed: %w", err)
	case <-ctx.Done():
		close(stopChan)
		return nil, fmt.Errorf("port forwarding cancelled: %w", ctx.Err())
	}

	return &PortForwardSession{
		LocalPorts:  localPorts,
		RemotePorts: remotePorts,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		Forwarder:   forwarder,
	}, nil
}

// portForwardToService sets up port forwarding to a service.
func portForwardToService(ctx context.Context, clientset kubernetes.Interface, restConfig *rest.Config,
	namespace, serviceName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {

	// Get the service
	service, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("service %s/%s not found: %w", namespace, serviceName, err)
	}

	if len(service.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s/%s has no selector", namespace, serviceName)
	}

	labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: service.Spec.Selector,
	})

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for service %s/%s: %w", namespace, serviceName, err)
	}

	var targetPod string
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			targetPod = pod.Name
			break
		}
	}

	if targetPod == "" {
		return nil, fmt.Errorf("no running pods found for service %s/%s", namespace, serviceName)
	}

	return portForwardToPod(ctx, clientset, restConfig, namespace, targetPod, ports, opts)
}

// getAPIResources returns available API resources with pagination support.
func getAPIResources(ctx context.Context, discoveryClient discovery.DiscoveryInterface,
	limit, offset int, apiGroup string, namespacedOnly bool, verbs []string) (*PaginatedAPIResourceResponse, error) {

	apiResourceLists, err := discoveryClient.ServerPreferredResources()
	// ServerPreferredResources may return partial results with an error
	// (e.g., when some API groups are unavailable). We continue with
	// whatever results we got, as partial data is better than none.
	_ = err // Intentionally ignore error - continue with partial results

	var apiResources []APIResourceInfo

	for _, apiResourceList := range apiResourceLists {
		if apiResourceList == nil {
			continue
		}

		gv, err := parseGroupVersionShared(apiResourceList.GroupVersion)
		if err != nil {
			continue
		}

		for _, apiResource := range apiResourceList.APIResources {
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

	// Apply filters
	var filteredResources []APIResourceInfo
	for _, resource := range apiResources {
		if apiGroup != "" && resource.Group != apiGroup {
			continue
		}

		if namespacedOnly && !resource.Namespaced {
			continue
		}

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

// getClusterHealth returns the health status of the cluster.
func getClusterHealth(ctx context.Context, clientset kubernetes.Interface, discoveryClient discovery.DiscoveryInterface) (*ClusterHealth, error) {
	health := &ClusterHealth{
		Status:     clusterHealthUnknown,
		Components: []ComponentHealth{},
		Nodes:      []NodeHealth{},
	}

	version, err := discoveryClient.ServerVersion()
	if err != nil {
		health.Status = clusterHealthUnhealthy
		health.Components = append(health.Components, ComponentHealth{
			Name:    "API Server",
			Status:  clusterHealthUnhealthy,
			Message: fmt.Sprintf("Failed to get server version: %s", err.Error()),
		})
		return health, nil
	}

	health.Components = append(health.Components, ComponentHealth{
		Name:    "API Server",
		Status:  clusterHealthHealthy,
		Message: fmt.Sprintf("Version: %s", version.String()),
	})

	componentStatuses, err := clientset.CoreV1().ComponentStatuses().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, component := range componentStatuses.Items {
			componentHealth := ComponentHealth{
				Name:   component.Name,
				Status: clusterHealthUnknown,
			}

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

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			nodeHealth := NodeHealth{
				Name:       node.Name,
				Ready:      false,
				Conditions: node.Status.Conditions,
			}

			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					nodeHealth.Ready = condition.Status == corev1.ConditionTrue
					break
				}
			}

			health.Nodes = append(health.Nodes, nodeHealth)
		}
	}

	health.Status = calculateOverallHealthShared(health.Components, health.Nodes)

	return health, nil
}

// Shared helper functions

// groupsMatch determines if a requested API group matches an actual group value.
// It treats "core" and "" (empty string) as equivalent for core resources.
func groupsMatch(requested, actual string) bool {
	if requested == actual {
		return true
	}

	// Treat "core" and empty string as equivalent for the core API group.
	if (requested == "core" && actual == "") || (requested == "" && actual == "core") {
		return true
	}

	return false
}

// parseAPIGroup parses an apiGroup string into group and optional preferred version.
// Supports formats: "group" or "group/version" (e.g., "apps" or "apps/v1").
func parseAPIGroup(apiGroup string) (group, preferredVersion string) {
	apiGroup = strings.ToLower(apiGroup)
	if apiGroup == "" {
		return "", ""
	}

	parts := strings.SplitN(apiGroup, "/", 2)
	group = parts[0]
	if len(parts) == 2 {
		preferredVersion = parts[1]
	}
	return group, preferredVersion
}

// resolveResourceTypeShared determines the GroupVersionResource for a given resource type.
// It supports optional apiGroup hints in the form "group" or "group/version", similar to resolveResourceType.
func resolveResourceTypeShared(resourceType, apiGroup string, builtinResources map[string]schema.GroupVersionResource,
	discoveryClient discovery.DiscoveryInterface) (schema.GroupVersionResource, bool, error) {

	resourceType = strings.ToLower(resourceType)
	requestedGroup, preferredVersion := parseAPIGroup(apiGroup)

	// Check built-in resources first
	if builtinResources != nil {
		if gvr, exists := builtinResources[resourceType]; exists {
			if requestedGroup == "" || groupsMatch(requestedGroup, gvr.Group) {
				namespaced := isResourceNamespacedShared(gvr)
				return gvr, namespaced, nil
			}
		}
	}

	// Set up timeout for discovery API call
	ctx, cancel := context.WithTimeout(context.Background(), DiscoveryTimeoutSeconds*time.Second)
	defer cancel()

	type discoveryResult struct {
		resourceLists []*metav1.APIResourceList
		err           error
	}

	resultChan := make(chan discoveryResult, 1)
	go func() {
		resourceLists, err := discoveryClient.ServerPreferredResources()
		resultChan <- discoveryResult{resourceLists: resourceLists, err: err}
	}()

	var resourceLists []*metav1.APIResourceList
	select {
	case result := <-resultChan:
		resourceLists = result.resourceLists
		// Continue with partial results even on error
	case <-ctx.Done():
		return schema.GroupVersionResource{}, false, fmt.Errorf("API discovery timed out after 30 seconds")
	}

	// Helper to search API resources with optional group/version preference
	searchResources := func(preferVersion string) (schema.GroupVersionResource, bool, bool) {
		for _, resourceList := range resourceLists {
			if resourceList == nil {
				continue
			}

			gv, err := parseGroupVersionShared(resourceList.GroupVersion)
			if err != nil {
				continue
			}

			// Filter by requested group if provided
			if requestedGroup != "" && !groupsMatch(requestedGroup, gv.Group) {
				continue
			}

			// Optionally filter by preferred version
			if preferVersion != "" && gv.Version != preferVersion {
				continue
			}

			for _, resource := range resourceList.APIResources {
				matches := []string{
					strings.ToLower(resource.Name),
					strings.ToLower(resource.Kind),
					strings.ToLower(resource.SingularName),
				}

				for _, shortName := range resource.ShortNames {
					matches = append(matches, strings.ToLower(shortName))
				}

				for _, match := range matches {
					if match == resourceType {
						gvr := schema.GroupVersionResource{
							Group:    gv.Group,
							Version:  gv.Version,
							Resource: resource.Name,
						}
						return gvr, resource.Namespaced, true
					}
				}
			}
		}

		return schema.GroupVersionResource{}, false, false
	}

	// First, if a preferred version was specified (via apiGroup like "apps/v1"), search with that preference
	if preferredVersion != "" {
		if gvr, namespaced, found := searchResources(preferredVersion); found {
			return gvr, namespaced, nil
		}
	}

	// Fallback: search without version preference (still honoring requested group if provided)
	if gvr, namespaced, found := searchResources(""); found {
		return gvr, namespaced, nil
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf("unknown resource type: %s", resourceType)
}

// isResourceNamespacedShared determines if a resource is namespaced.
func isResourceNamespacedShared(gvr schema.GroupVersionResource) bool {
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

// resolveGVRFromObjectShared resolves GroupVersionResource from an unstructured object.
func resolveGVRFromObjectShared(obj *unstructured.Unstructured, discoveryClient discovery.DiscoveryInterface) (schema.GroupVersionResource, bool, error) {
	_, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("failed to parse API version: %w", err)
	}

	return resolveResourceTypeShared(obj.GetKind(), "", nil, discoveryClient)
}

// getResourceEventsShared retrieves events related to a specific resource.
func getResourceEventsShared(ctx context.Context, clientset kubernetes.Interface, namespace, name string) ([]corev1.Event, error) {
	eventList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", name),
	})
	if err != nil {
		return nil, err
	}

	return eventList.Items, nil
}

// parseGroupVersionShared parses a group/version string.
func parseGroupVersionShared(groupVersion string) (GroupVersion, error) {
	if groupVersion == "" {
		return GroupVersion{}, fmt.Errorf("empty group version")
	}

	if !strings.Contains(groupVersion, "/") {
		return GroupVersion{
			Group:   "",
			Version: groupVersion,
		}, nil
	}

	parts := strings.SplitN(groupVersion, "/", 2)
	if len(parts) != 2 {
		return GroupVersion{}, fmt.Errorf("invalid group version format: %s", groupVersion)
	}

	return GroupVersion{
		Group:   parts[0],
		Version: parts[1],
	}, nil
}

// calculateOverallHealthShared determines the overall cluster health.
func calculateOverallHealthShared(components []ComponentHealth, nodes []NodeHealth) string {
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

	if len(nodes) > 0 {
		readyNodes := 0
		for _, node := range nodes {
			if node.Ready {
				readyNodes++
			}
		}

		if readyNodes < len(nodes)/2 {
			return clusterHealthDegraded
		}
	}

	for _, component := range components {
		if component.Status == clusterHealthUnhealthy {
			return clusterHealthDegraded
		}
	}

	return clusterHealthHealthy
}
