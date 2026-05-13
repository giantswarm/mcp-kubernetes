package resource

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// GetResourceArgs defines the arguments for kubectl get operations
type GetResourceArgs struct {
	KubeContext  string `json:"kubeContext,omitempty"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resourceType"`
	APIGroup     string `json:"apiGroup,omitempty"`
	Name         string `json:"name"`
}

// ListResourceArgs defines the arguments for kubectl list operations
type ListResourceArgs struct {
	KubeContext        string `json:"kubeContext,omitempty"`
	Namespace          string `json:"namespace"`
	ResourceType       string `json:"resourceType"`
	APIGroup           string `json:"apiGroup,omitempty"`
	LabelSelector      string `json:"labelSelector,omitempty"`
	FieldSelector      string `json:"fieldSelector,omitempty"`
	AllNamespaces      bool   `json:"allNamespaces,omitempty"`
	FullOutput         bool   `json:"fullOutput,omitempty"`
	IncludeLabels      bool   `json:"includeLabels,omitempty"`
	IncludeAnnotations bool   `json:"includeAnnotations,omitempty"`
	Limit              int64  `json:"limit,omitempty"`
	Continue           string `json:"continue,omitempty"`
}

// DescribeResourceArgs defines the arguments for kubectl describe operations
type DescribeResourceArgs struct {
	KubeContext  string `json:"kubeContext,omitempty"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resourceType"`
	APIGroup     string `json:"apiGroup,omitempty"`
	Name         string `json:"name"`
}

// CreateResourceArgs defines the arguments for kubectl create operations
type CreateResourceArgs struct {
	KubeContext string      `json:"kubeContext,omitempty"`
	Namespace   string      `json:"namespace"`
	Manifest    interface{} `json:"manifest"`
}

// ApplyResourceArgs defines the arguments for kubectl apply operations
type ApplyResourceArgs struct {
	KubeContext string      `json:"kubeContext,omitempty"`
	Namespace   string      `json:"namespace"`
	Manifest    interface{} `json:"manifest"`
}

// DeleteResourceArgs defines the arguments for kubectl delete operations
type DeleteResourceArgs struct {
	KubeContext  string `json:"kubeContext,omitempty"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resourceType"`
	APIGroup     string `json:"apiGroup,omitempty"`
	Name         string `json:"name"`
}

// PatchResourceArgs defines the arguments for kubectl patch operations
type PatchResourceArgs struct {
	KubeContext  string      `json:"kubeContext,omitempty"`
	Namespace    string      `json:"namespace"`
	ResourceType string      `json:"resourceType"`
	APIGroup     string      `json:"apiGroup,omitempty"`
	Name         string      `json:"name"`
	PatchType    string      `json:"patchType"`
	Patch        interface{} `json:"patch"`
}

// ScaleResourceArgs defines the arguments for kubectl scale operations
type ScaleResourceArgs struct {
	KubeContext  string `json:"kubeContext,omitempty"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resourceType"`
	APIGroup     string `json:"apiGroup,omitempty"`
	Name         string `json:"name"`
	Replicas     int32  `json:"replicas"`
}

// RegisterResourceTools registers all resource management tools with the MCP server
func RegisterResourceTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// Get cluster/context parameters based on server mode
	clusterContextParams := tools.AddClusterContextParams(sc)

	// get tool
	getResourceOpts := []mcp.ToolOption{
		mcp.WithDescription(`Get a specific Kubernetes resource by name.

Namespace Handling:
- For namespaced resources (pods, services, deployments): Uses 'default' namespace if not specified
- For cluster-scoped resources (nodes, namespaces, PVs, clusterroles): Namespace is automatically ignored
- The tool automatically determines resource scope via Kubernetes API discovery`),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	getResourceOpts = append(getResourceOpts, clusterContextParams...)
	getResourceOpts = append(getResourceOpts,
		mcp.WithString("namespace",
			mcp.Description(`Namespace for namespaced resources. Uses 'default' if not specified.
For cluster-scoped resources (nodes, namespaces, PVs, clusterroles), this is ignored.`),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to get"),
		),
		mcp.WithString("output",
			mcp.Description("Output format: 'slim' (default; blacklist exclusion + per-Kind shaping — HelmRelease drops spec.values / status.history, Deployment / StatefulSet / DaemonSet collapse long container env lists), 'normal' (blacklist exclusion only — managedFields, last-applied-configuration, transition timestamps), 'wide' / 'full' (no field stripping, full manifest). Secret data is always masked regardless of output. See docs/slim-output-tuning.md."),
			mcp.Enum("slim", "normal", "wide", "full"),
		),
	)
	getResourceTool := mcp.NewTool("get", getResourceOpts...)

	s.AddTool(getResourceTool, tools.WrapWithAuditLogging("get", handleGetResource, sc))

	// list tool
	listResourceOpts := []mcp.ToolOption{
		mcp.WithDescription(`List Kubernetes resources with optional filtering.

Namespace Handling:
- Namespaced resources (pods, services, deployments): Uses 'default' namespace if not specified
- Cluster-scoped resources (nodes, namespaces, persistentvolumes, clusterroles): Namespace is automatically ignored
- Use 'allNamespaces=true' to list namespaced resources across all namespaces
- The tool automatically determines whether a resource is namespaced or cluster-scoped via Kubernetes API discovery

Examples:
- List nodes: {"resourceType": "nodes"}
- List pods in default namespace: {"resourceType": "pods"}
- List pods in kube-system: {"resourceType": "pods", "namespace": "kube-system"}
- List all pods: {"resourceType": "pods", "allNamespaces": true}
- List CAPI clusters: {"resourceType": "clusters", "apiGroup": "cluster.x-k8s.io"}

Supports both server-side selectors (labelSelector, fieldSelector) and client-side filtering for advanced scenarios.`),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	listResourceOpts = append(listResourceOpts, clusterContextParams...)
	listResourceOpts = append(listResourceOpts,
		mcp.WithString("namespace",
			mcp.Description(`Namespace for namespaced resources (pods, services, deployments, etc.).
- For namespaced resources: Uses 'default' if not specified.
- For cluster-scoped resources (nodes, namespaces, PVs, clusterroles): This parameter is ignored.
- The tool automatically determines resource scope via Kubernetes API discovery.`),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource to list (e.g., pods, services, deployments, nodes)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Server-side label selector for efficient filtering (e.g., 'app=nginx,env=prod'). Use this when possible for better performance."),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Server-side field selector (limited fields: metadata.name, metadata.namespace, spec.nodeName, status.phase). For fields not supported by Kubernetes, use 'filter' instead."),
		),
		mcp.WithObject("filter",
			mcp.Description("Client-side filter for advanced scenarios not supported by fieldSelector (e.g., filtering nodes by taints). Supports dot notation for nested fields and [*] for array matching. Examples: {\"spec.taints[*].key\": \"karpenter.sh/unregistered\"} or {\"metadata.labels.app\": \"nginx\"}. See docs/client-side-filtering.md for full syntax and use cases. Performance note: Prefer labelSelector/fieldSelector when available as they filter server-side."),
		),
		mcp.WithBoolean("allNamespaces",
			mcp.Description(`List namespaced resources across ALL namespaces.
- Applies only to namespaced resources (pods, services, etc.)
- Ignored for cluster-scoped resources (nodes, PVs, etc.)
- When true, overrides the 'namespace' parameter.`),
		),
		mcp.WithBoolean("fullOutput",
			mcp.Description("Return full resource manifests instead of summary (default: false, returns compact summary)"),
		),
		mcp.WithBoolean("includeLabels",
			mcp.Description("Include resource labels in summary output (default: false)"),
		),
		mcp.WithBoolean("includeAnnotations",
			mcp.Description("Include resource annotations in summary output (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return per page (optional, default: 20, max: 1000)"),
		),
		mcp.WithString("continue",
			mcp.Description("Continue token from previous paginated request (optional)"),
		),
		mcp.WithBoolean("summary",
			mcp.Description("Return aggregated counts (by status, namespace) instead of full objects. Useful for fleet-scale operations with many results. Default: false"),
		),
		mcp.WithString("output",
			mcp.Description("Output format: 'slim' (default; blacklist exclusion + per-Kind shaping), 'normal' (blacklist exclusion only), 'wide' / 'full' (no field stripping). Secret data is always masked regardless of output. See docs/slim-output-tuning.md for the full per-Kind shape table."),
			mcp.Enum("slim", "normal", "wide", "full"),
		),
	)
	listResourceTool := mcp.NewTool("list", listResourceOpts...)

	s.AddTool(listResourceTool, tools.WrapWithAuditLogging("list", handleListResources, sc))

	// describe tool
	describeResourceOpts := []mcp.ToolOption{
		mcp.WithDescription(`Get detailed information about a Kubernetes resource including events.

Namespace Handling:
- For namespaced resources (pods, services, deployments): Uses 'default' namespace if not specified
- For cluster-scoped resources (nodes, namespaces, PVs, clusterroles): Namespace is automatically ignored
- The tool automatically determines resource scope via Kubernetes API discovery`),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	describeResourceOpts = append(describeResourceOpts, clusterContextParams...)
	describeResourceOpts = append(describeResourceOpts,
		mcp.WithString("namespace",
			mcp.Description(`Namespace for namespaced resources. Uses 'default' if not specified.
For cluster-scoped resources (nodes, namespaces, PVs, clusterroles), this is ignored.`),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to describe"),
		),
		mcp.WithNumber("eventsLimit",
			mcp.Min(1),
			mcp.Max(MaxEventsLimit),
			mcp.Description(fmt.Sprintf("Maximum number of events to return, sorted newest-first by lastTimestamp. Default: %d. Range: [1, %d]. Use totalEvents/eventsTruncated in the response to detect clipping.", DefaultEventsLimit, MaxEventsLimit)),
		),
		mcp.WithString("output",
			mcp.Description("Output format: 'slim' (default; blacklist exclusion + per-Kind shaping for the resource — HelmRelease drops spec.values / status.history, workload templates collapse long env lists), 'normal' (blacklist exclusion only), 'wide' / 'full' (no field stripping). Secret data is always masked regardless of output. Event-list shaping is controlled by eventsLimit, not by this parameter."),
			mcp.Enum("slim", "normal", "wide", "full"),
		),
	)
	describeResourceTool := mcp.NewTool("describe", describeResourceOpts...)

	s.AddTool(describeResourceTool, tools.WrapWithAuditLogging("describe", handleDescribeResource, sc))

	// create tool
	createResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Create a new Kubernetes resource from a manifest"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	createResourceOpts = append(createResourceOpts, clusterContextParams...)
	createResourceOpts = append(createResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource should be created"),
		),
		mcp.WithObject("manifest",
			mcp.Required(),
			mcp.Description("Kubernetes manifest as JSON object"),
		),
	)
	addMutatingTool(s, sc, "create", "create", handleCreateResource, createResourceOpts...)

	// apply tool
	applyResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Apply a Kubernetes manifest (create or update)"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	applyResourceOpts = append(applyResourceOpts, clusterContextParams...)
	applyResourceOpts = append(applyResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource should be applied"),
		),
		mcp.WithObject("manifest",
			mcp.Required(),
			mcp.Description("Kubernetes manifest as JSON object"),
		),
	)
	addMutatingTool(s, sc, "apply", "apply", handleApplyResource, applyResourceOpts...)

	// delete tool
	deleteResourceOpts := []mcp.ToolOption{
		mcp.WithDescription(`Delete a Kubernetes resource.

Namespace Handling:
- For namespaced resources (pods, services, deployments): Uses 'default' namespace if not specified
- For cluster-scoped resources (nodes, namespaces, PVs, clusterroles): Namespace is automatically ignored
- The tool automatically determines resource scope via Kubernetes API discovery`),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	deleteResourceOpts = append(deleteResourceOpts, clusterContextParams...)
	deleteResourceOpts = append(deleteResourceOpts,
		mcp.WithString("namespace",
			mcp.Description(`Namespace for namespaced resources. Uses 'default' if not specified.
For cluster-scoped resources (nodes, namespaces, PVs, clusterroles), this is ignored.`),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to delete"),
		),
	)
	addMutatingTool(s, sc, "delete", "delete", handleDeleteResource, deleteResourceOpts...)

	// patch tool
	patchResourceOpts := []mcp.ToolOption{
		mcp.WithDescription(`Patch a Kubernetes resource with specific changes.

Namespace Handling:
- For namespaced resources (pods, services, deployments): Uses 'default' namespace if not specified
- For cluster-scoped resources (nodes, namespaces, PVs, clusterroles): Namespace is automatically ignored
- The tool automatically determines resource scope via Kubernetes API discovery`),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	patchResourceOpts = append(patchResourceOpts, clusterContextParams...)
	patchResourceOpts = append(patchResourceOpts,
		mcp.WithString("namespace",
			mcp.Description(`Namespace for namespaced resources. Uses 'default' if not specified.
For cluster-scoped resources (nodes, namespaces, PVs, clusterroles), this is ignored.`),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to patch"),
		),
		mcp.WithString("patchType",
			mcp.Required(),
			mcp.Description("Type of patch (strategic, merge, json)"),
			mcp.Enum("strategic", "merge", "json"),
		),
		mcp.WithObject("patch",
			mcp.Required(),
			mcp.Description("Patch data as JSON object"),
		),
	)
	addMutatingTool(s, sc, "patch", "patch", handlePatchResource, patchResourceOpts...)

	// scale tool
	scaleResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Scale a Kubernetes resource (deployment, replicaset, etc.)"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	scaleResourceOpts = append(scaleResourceOpts, clusterContextParams...)
	scaleResourceOpts = append(scaleResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of scalable Kubernetes resource (deployment, replicaset, statefulset)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to scale"),
		),
		mcp.WithNumber("replicas",
			mcp.Required(),
			mcp.Description("Number of replicas to scale to"),
		),
	)
	addMutatingTool(s, sc, "scale", "scale", handleScaleResource, scaleResourceOpts...)

	return nil
}

// addMutatingTool registers a mutating tool only if the operation verb is permitted
// by the current safety policy (tools.IsMutatingOperationAllowed). Disallowed tools
// are not exposed at all, so MCP clients never see them in the tool list.
//
// op is the verb checked against AllowedOperations (e.g., "create", "delete").
// name is the public MCP tool name (e.g., "create").
func addMutatingTool(
	s *mcpserver.MCPServer,
	sc *server.ServerContext,
	op, name string,
	handler tools.ToolHandler,
	opts ...mcp.ToolOption,
) {
	if !tools.IsMutatingOperationAllowed(sc, op) {
		return
	}
	s.AddTool(mcp.NewTool(name, opts...), tools.WrapWithAuditLogging(name, handler, sc))
}
