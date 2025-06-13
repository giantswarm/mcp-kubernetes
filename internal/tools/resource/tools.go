package resource

import (
	"context"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
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
	KubeContext   string `json:"kubeContext,omitempty"`
	Namespace     string `json:"namespace"`
	ResourceType  string `json:"resourceType"`
	LabelSelector string `json:"labelSelector,omitempty"`
	FieldSelector string `json:"fieldSelector,omitempty"`
	AllNamespaces bool   `json:"allNamespaces,omitempty"`
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
	// kubectl_get tool
	getResourceTool := mcp.NewTool("kubectl_get",
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

	// kubectl_list tool
	listResourceTool := mcp.NewTool("kubectl_list",
		mcp.WithDescription("List Kubernetes resources of a specific type"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace to list resources from"),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Type of Kubernetes resource to list (e.g., pods, services, deployments)"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Label selector to filter resources (optional)"),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Field selector to filter resources (optional)"),
		),
		mcp.WithBoolean("allNamespaces",
			mcp.Description("List resources from all namespaces (default: false)"),
		),
	)

	s.AddTool(listResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListResources(ctx, request, sc)
	})

	// kubectl_describe tool
	describeResourceTool := mcp.NewTool("kubectl_describe",
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

	// kubectl_create tool
	createResourceTool := mcp.NewTool("kubectl_create",
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

	// kubectl_apply tool
	applyResourceTool := mcp.NewTool("kubectl_apply",
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

	// kubectl_delete tool
	deleteResourceTool := mcp.NewTool("kubectl_delete",
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

	// kubectl_patch tool
	patchResourceTool := mcp.NewTool("kubectl_patch",
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

	// kubectl_scale tool
	scaleResourceTool := mcp.NewTool("kubectl_scale",
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