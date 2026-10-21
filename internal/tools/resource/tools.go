package resource

import (
	"fmt"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
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
		tooldesc.Describe("Get a specific Kubernetes resource by name.", "Secret data is always masked."),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	getResourceOpts = append(getResourceOpts, clusterContextParams...)
	getResourceOpts = append(getResourceOpts,
		tools.NamespaceParam(),
		tools.ResourceTypeParam(),
		tools.APIGroupParam(),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to get"),
		),
		mcp.WithString("output",
			mcp.Description("normal strips managedFields, ownerReferences, finalizers; slim (default) also HelmRelease values/history, long env lists; wide/full keep all."),
			mcp.Enum("slim", "normal", "wide", "full"),
		),
	)
	getResourceTool := mcp.NewTool("get", getResourceOpts...)

	s.AddTool(getResourceTool, tools.WrapWithAuditLogging("get", handleGetResource, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "get", handleGetResource, getResourceOpts...)

	// list tool
	listResourceOpts := []mcp.ToolOption{
		tooldesc.Describe("List Kubernetes resources of a type, narrowed by namespace, label or field selectors, or a client-side filter.", fmt.Sprintf("limit takes events newest first: an event query scans at most %d matches (the answer says when it hit that: narrow it) and returns no continue token; passing one gives unsorted API order. Every other type comes in API order (namespace/name), so limit takes an alphabetical prefix, not the newest or the worst. Nothing selects by age, filter included. Examples: {\"resourceType\": \"pods\", \"allNamespaces\": true}, {\"resourceType\": \"clusters\", \"apiGroup\": \"cluster.x-k8s.io\"}.", listSortScanLimit)),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	listResourceOpts = append(listResourceOpts, clusterContextParams...)
	listResourceOpts = append(listResourceOpts,
		mcp.WithString("namespace",
			mcp.Description("Namespace of a namespaced resource; default 'default', allNamespaces for every one. Ignored for cluster-scoped resources (nodes, PVs, …)."),
		),
		mcp.WithString("resourceType",
			mcp.Required(),
			mcp.Description("Resource type to list (pods, services, nodes, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io)."),
		),
		tools.APIGroupParam(),
		mcp.WithString("labelSelector",
			mcp.Description("Server-side label selector for efficient filtering (e.g., 'app=nginx,env=prod'). Use this when possible for better performance."),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Indexed fields only, per Kind: metadata.name, .namespace on all; e.g. pods spec.nodeName, status.phase; events reason, type, involvedObject.*."),
		),
		mcp.WithObject("filter",
			mcp.Description("Client-side, ANDed: dot path ([*] over arrays; \"images\" = every container image) to a value or {\"op\": \"prefix|suffix|contains|regex\", \"value\": \"…\"}."),
		),
		mcp.WithBoolean("allNamespaces",
			mcp.Description("List a namespaced resource across all namespaces; overrides namespace. Ignored for cluster-scoped resources."),
		),
		mcp.WithBoolean("fullOutput",
			mcp.Description("Return full manifests instead of the compact per-Kind summary with images and owner (default false); output applies only then."),
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
			mcp.Description("With fullOutput: normal strips managedFields, ownerReferences, finalizers; slim (default) also per-Kind fields; wide/full keep all. Secrets masked."),
			mcp.Enum("slim", "normal", "wide", "full"),
		),
	)
	listResourceTool := mcp.NewTool("list", listResourceOpts...)

	s.AddTool(listResourceTool, tools.WrapWithAuditLogging("list", handleListResources, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "list", handleListResources, listResourceOpts...)

	// describe tool
	describeResourceOpts := []mcp.ToolOption{
		tooldesc.Describe("Get detailed information about a Kubernetes resource, including its events.", "Secret data is always masked."),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	describeResourceOpts = append(describeResourceOpts, clusterContextParams...)
	describeResourceOpts = append(describeResourceOpts,
		tools.NamespaceParam(),
		tools.ResourceTypeParam(),
		tools.APIGroupParam(),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to describe"),
		),
		mcp.WithNumber("eventsLimit",
			mcp.Min(1),
			mcp.Max(MaxEventsLimit),
			mcp.Description(fmt.Sprintf("Most events to return, newest first (default %d, at most %d); totalEvents and eventsTruncated tell when clipped. output does not shape them.", DefaultEventsLimit, MaxEventsLimit)),
		),
		mcp.WithString("output",
			mcp.Description("normal strips managedFields, ownerReferences, finalizers; slim (default) also HelmRelease values/history, long env lists; wide/full keep all."),
			mcp.Enum("slim", "normal", "wide", "full"),
		),
	)
	describeResourceTool := mcp.NewTool("describe", describeResourceOpts...)

	s.AddTool(describeResourceTool, tools.WrapWithAuditLogging("describe", handleDescribeResource, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "describe", handleDescribeResource, describeResourceOpts...)

	// create tool
	createResourceOpts := []mcp.ToolOption{
		tooldesc.Describe("Create a new Kubernetes resource from a manifest", ""),
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
		tooldesc.Describe("Apply a Kubernetes manifest (create or update)", ""),
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
		tooldesc.Describe("Delete a Kubernetes resource.", ""),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	deleteResourceOpts = append(deleteResourceOpts, clusterContextParams...)
	deleteResourceOpts = append(deleteResourceOpts,
		tools.NamespaceParam(),
		tools.ResourceTypeParam(),
		tools.APIGroupParam(),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource to delete"),
		),
	)
	addMutatingTool(s, sc, "delete", "delete", handleDeleteResource, deleteResourceOpts...)

	// patch tool
	patchResourceOpts := []mcp.ToolOption{
		tooldesc.Describe("Patch a Kubernetes resource with specific changes.", ""),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	patchResourceOpts = append(patchResourceOpts, clusterContextParams...)
	patchResourceOpts = append(patchResourceOpts,
		tools.NamespaceParam(),
		tools.ResourceTypeParam(),
		tools.APIGroupParam(),
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
		tooldesc.Describe("Scale a Kubernetes resource (deployment, replicaset, etc.)", ""),
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
			mcp.Description("Scalable resource type (deployment, replicaset, statefulset), optionally with its API group as kubectl does (deployments.apps)."),
		),
		tools.APIGroupParam(),
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
	tools.MaybeAddDeprecatedAlias(s, sc, name, handler, opts...)
}
