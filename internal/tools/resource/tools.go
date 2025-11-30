package resource

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// GetResourceArgs defines the arguments for kubectl get operations
type GetResourceArgs struct {
	KubeContext  string `json:"kubeContext,omitempty"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resourceType"`
	Name         string `json:"name"`
}

// ListResourceArgs defines the arguments for kubectl list operations
type ListResourceArgs struct {
	KubeContext        string `json:"kubeContext,omitempty"`
	Namespace          string `json:"namespace"`
	ResourceType       string `json:"resourceType"`
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
	Name         string `json:"name"`
}

// PatchResourceArgs defines the arguments for kubectl patch operations
type PatchResourceArgs struct {
	KubeContext  string      `json:"kubeContext,omitempty"`
	Namespace    string      `json:"namespace"`
	ResourceType string      `json:"resourceType"`
	Name         string      `json:"name"`
	PatchType    string      `json:"patchType"`
	Patch        interface{} `json:"patch"`
}

// ScaleResourceArgs defines the arguments for kubectl scale operations
type ScaleResourceArgs struct {
	KubeContext  string `json:"kubeContext,omitempty"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resourceType"`
	Name         string `json:"name"`
	Replicas     int32  `json:"replicas"`
}

// RegisterResourceTools registers all resource management tools with the MCP server
func RegisterResourceTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// kubernetes_get tool
	getResourceTool := mcp.NewTool("kubernetes_get",
		mcp.WithDescription("Get a specific Kubernetes resource by name and namespace"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to get"),
		),
	)

	s.AddTool(getResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetResource(ctx, request, sc)
	})

	// kubernetes_list tool
	listResourceTool := mcp.NewTool("kubernetes_list",
		mcp.WithDescription("List Kubernetes resources with optional filtering. Supports both server-side selectors (labelSelector, fieldSelector) and client-side filtering for advanced scenarios like filtering nodes by taints, which native Kubernetes selectors don't support."),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace to list resources from"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource to list (e.g., pods, services, deployments, nodes)"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Server-side label selector for efficient filtering (e.g., 'app=nginx,env=prod'). Use this when possible for better performance."),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Server-side field selector (limited fields: metadata.name, metadata.namespace, spec.nodeName, status.phase). For fields not supported by Kubernetes, use 'filter' instead."),
		),
		mcp.WithObject("filter",
			mcp.Description("Client-side filter for advanced scenarios. USE CASES: 1) Find nodes with specific taints (incident investigation): {\"spec.taints[*].key\": \"karpenter.sh/unregistered\"}, 2) Filter by nested labels: {\"metadata.labels.app\": \"nginx\"}, 3) Multiple conditions (AND): {\"spec.taints[*].key\": \"node.kubernetes.io/unschedulable\", \"spec.taints[*].effect\": \"NoSchedule\"}. SYNTAX: Use dot notation for nested fields, [*] for array matching, exact values for comparison. Performance note: Prefer labelSelector/fieldSelector when available as they filter server-side."),
		),
		mcp.WithBoolean("allNamespaces",
			mcp.Description("List resources from all namespaces (default: false)"),
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
			mcp.Description("Maximum number of items to return per page (optional, default: 20, 0 = no limit)"),
		),
		mcp.WithString("continue",
			mcp.Description("Continue token from previous paginated request (optional)"),
		),
	)

	s.AddTool(listResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListResources(ctx, request, sc)
	})

	// kubernetes_describe tool
	describeResourceTool := mcp.NewTool("kubernetes_describe",
		mcp.WithDescription("Get detailed information about a Kubernetes resource including events"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to describe"),
		),
	)

	s.AddTool(describeResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleDescribeResource(ctx, request, sc)
	})

	// kubernetes_create tool
	createResourceTool := mcp.NewTool("kubernetes_create",
		mcp.WithDescription("Create a new Kubernetes resource from a manifest"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource should be created"),
		),
		mcp.WithObject("manifest",
			mcp.Required(),
			mcp.Description("Kubernetes manifest as JSON object"),
		),
	)

	s.AddTool(createResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleCreateResource(ctx, request, sc)
	})

	// kubernetes_apply tool
	applyResourceTool := mcp.NewTool("kubernetes_apply",
		mcp.WithDescription("Apply a Kubernetes manifest (create or update)"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource should be applied"),
		),
		mcp.WithObject("manifest",
			mcp.Required(),
			mcp.Description("Kubernetes manifest as JSON object"),
		),
	)

	s.AddTool(applyResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleApplyResource(ctx, request, sc)
	})

	// kubernetes_delete tool
	deleteResourceTool := mcp.NewTool("kubernetes_delete",
		mcp.WithDescription("Delete a Kubernetes resource"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to delete"),
		),
	)

	s.AddTool(deleteResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleDeleteResource(ctx, request, sc)
	})

	// kubernetes_patch tool
	patchResourceTool := mcp.NewTool("kubernetes_patch",
		mcp.WithDescription("Patch a Kubernetes resource with specific changes"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource (e.g., pod, service, deployment)"),
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

	s.AddTool(patchResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handlePatchResource(ctx, request, sc)
	})

	// kubernetes_scale tool
	scaleResourceTool := mcp.NewTool("kubernetes_scale",
		mcp.WithDescription("Scale a Kubernetes resource (deployment, replicaset, etc.)"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of scalable Kubernetes resource (deployment, replicaset, statefulset)"),
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

	s.AddTool(scaleResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleScaleResource(ctx, request, sc)
	})

	return nil
}
