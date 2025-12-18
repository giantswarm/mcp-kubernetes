package resource

import (
	"context"
	"log/slog"

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

	// kubernetes_get tool
	getResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Get a specific Kubernetes resource by name and namespace"),
	}
	getResourceOpts = append(getResourceOpts, clusterContextParams...)
	getResourceOpts = append(getResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
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
	)
	getResourceTool := mcp.NewTool("kubernetes_get", getResourceOpts...)

	s.AddTool(getResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetResource(ctx, request, sc)
	})

	// kubernetes_list tool
	listResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("List Kubernetes resources with optional filtering. Supports both server-side selectors (labelSelector, fieldSelector) and client-side filtering for advanced scenarios like filtering nodes by taints, which native Kubernetes selectors don't support."),
	}
	listResourceOpts = append(listResourceOpts, clusterContextParams...)
	listResourceOpts = append(listResourceOpts,
		mcp.WithString("namespace",
			mcp.Description("Namespace to list resources from. Required for namespaced resources, omit for cluster-scoped resources (nodes, persistentvolumes, namespaces, clusterroles, etc.)"),
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
			mcp.Description("List namespaced resources from all namespaces (default: false). Not needed for cluster-scoped resources."),
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
			mcp.Description("Output format: 'slim' (default, removes verbose fields), 'normal' (standard output), 'wide' (includes all fields)"),
			mcp.Enum("slim", "normal", "wide"),
		),
	)
	listResourceTool := mcp.NewTool("kubernetes_list", listResourceOpts...)

	s.AddTool(listResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		slog.Debug("kubernetes_list tool invoked", slog.String("tool", "kubernetes_list"))
		return handleListResources(ctx, request, sc)
	})

	// kubernetes_describe tool
	describeResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Get detailed information about a Kubernetes resource including events"),
	}
	describeResourceOpts = append(describeResourceOpts, clusterContextParams...)
	describeResourceOpts = append(describeResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
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
	)
	describeResourceTool := mcp.NewTool("kubernetes_describe", describeResourceOpts...)

	s.AddTool(describeResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleDescribeResource(ctx, request, sc)
	})

	// kubernetes_create tool
	createResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Create a new Kubernetes resource from a manifest"),
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
	createResourceTool := mcp.NewTool("kubernetes_create", createResourceOpts...)

	s.AddTool(createResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleCreateResource(ctx, request, sc)
	})

	// kubernetes_apply tool
	applyResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Apply a Kubernetes manifest (create or update)"),
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
	applyResourceTool := mcp.NewTool("kubernetes_apply", applyResourceOpts...)

	s.AddTool(applyResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleApplyResource(ctx, request, sc)
	})

	// kubernetes_delete tool
	deleteResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Delete a Kubernetes resource"),
	}
	deleteResourceOpts = append(deleteResourceOpts, clusterContextParams...)
	deleteResourceOpts = append(deleteResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
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
	deleteResourceTool := mcp.NewTool("kubernetes_delete", deleteResourceOpts...)

	s.AddTool(deleteResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleDeleteResource(ctx, request, sc)
	})

	// kubernetes_patch tool
	patchResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Patch a Kubernetes resource with specific changes"),
	}
	patchResourceOpts = append(patchResourceOpts, clusterContextParams...)
	patchResourceOpts = append(patchResourceOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
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
	patchResourceTool := mcp.NewTool("kubernetes_patch", patchResourceOpts...)

	s.AddTool(patchResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handlePatchResource(ctx, request, sc)
	})

	// kubernetes_scale tool
	scaleResourceOpts := []mcp.ToolOption{
		mcp.WithDescription("Scale a Kubernetes resource (deployment, replicaset, etc.)"),
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
	scaleResourceTool := mcp.NewTool("kubernetes_scale", scaleResourceOpts...)

	s.AddTool(scaleResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleScaleResource(ctx, request, sc)
	})

	return nil
}
