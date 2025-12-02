package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// handleGetResource handles kubectl get operations
func handleGetResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Use the appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	obj, err := k8sClient.Get(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get resource: %v", err)), nil
	}

	// Convert the resource to JSON for output
	jsonData, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleListResources handles kubectl list operations
func handleListResources(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	namespace, _ := args["namespace"].(string)
	allNamespaces, _ := args["allNamespaces"].(bool)

	// Namespace is not required when listing namespaces or all resources across namespaces
	if resourceType != "namespace" && !allNamespaces && namespace == "" {
		return mcp.NewToolResultError("namespace is required unless listing namespaces or using --all-namespaces"), nil
	}

	labelSelector, _ := args["labelSelector"].(string)
	fieldSelector, _ := args["fieldSelector"].(string)

	// New parameters for controlling output format
	fullOutput, _ := args["fullOutput"].(bool)
	includeLabels, _ := args["includeLabels"].(bool)
	includeAnnotations, _ := args["includeAnnotations"].(bool)

	// Pagination parameters with sensible defaults
	var limit int64 = 20 // Default page size
	if limitVal, ok := args["limit"]; ok {
		if limitFloat, ok := limitVal.(float64); ok {
			limit = int64(limitFloat)
		}
	}
	continueToken, _ := args["continue"].(string)

	opts := k8s.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
		AllNamespaces: allNamespaces,
		Limit:         limit,
		Continue:      continueToken,
	}

	if allNamespaces || resourceType == "namespace" {
		namespace = ""
	}
	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	paginatedResponse, err := k8sClient.List(ctx, kubeContext, namespace, resourceType, apiGroup, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list resources: %v", err)), nil
	}

	if fullOutput {
		// Return full paginated output
		jsonData, err := json.MarshalIndent(paginatedResponse, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal paginated resources: %v", err)), nil
		}
		return mcp.NewToolResultText(string(jsonData)), nil
	}

	// Return summarized paginated output
	summary := SummarizePaginatedResources(
		paginatedResponse.Items,
		includeLabels,
		includeAnnotations,
		paginatedResponse.Continue,
		paginatedResponse.ResourceVersion,
		paginatedResponse.RemainingItems,
	)
	jsonData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal paginated resource summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleDescribeResource handles kubectl describe operations
func handleDescribeResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	description, err := k8sClient.Describe(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to describe resource: %v", err)), nil
	}

	// Convert the description to JSON for output
	jsonData, err := json.MarshalIndent(description, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal description: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleCreateResource handles kubectl create operations
func handleCreateResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if create operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "create" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Create operations are not allowed in non-destructive mode"), nil
		}
	}

	kubeContext := request.GetString("kubeContext", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	manifestData, ok := request.GetArguments()["manifest"]
	if !ok || manifestData == nil {
		return mcp.NewToolResultError("manifest is required"), nil
	}

	// Convert the manifest to a runtime.Object
	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal manifest: %v", err)), nil
	}

	var obj runtime.Object
	if err := json.Unmarshal(manifestJSON, &obj); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse manifest: %v", err)), nil
	}

	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	createdObj, err := k8sClient.Create(ctx, kubeContext, namespace, obj)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create resource: %v", err)), nil
	}

	// Convert the created resource to JSON for output
	jsonData, err := json.MarshalIndent(createdObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal created resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleApplyResource handles kubectl apply operations
func handleApplyResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if apply operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "apply" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Apply operations are not allowed in non-destructive mode"), nil
		}
	}

	kubeContext := request.GetString("kubeContext", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	manifestData, ok := request.GetArguments()["manifest"]
	if !ok || manifestData == nil {
		return mcp.NewToolResultError("manifest is required"), nil
	}

	// Convert the manifest to a runtime.Object
	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal manifest: %v", err)), nil
	}

	var obj runtime.Object
	if err := json.Unmarshal(manifestJSON, &obj); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse manifest: %v", err)), nil
	}

	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	appliedObj, err := k8sClient.Apply(ctx, kubeContext, namespace, obj)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to apply resource: %v", err)), nil
	}

	// Convert the applied resource to JSON for output
	jsonData, err := json.MarshalIndent(appliedObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal applied resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleDeleteResource handles kubectl delete operations
func handleDeleteResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if delete operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "delete" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Delete operations are not allowed in non-destructive mode"), nil
		}
	}

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	err = k8sClient.Delete(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete resource: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Resource %s/%s deleted successfully", resourceType, name)), nil
}

// handlePatchResource handles kubectl patch operations
func handlePatchResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if patch operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "patch" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Patch operations are not allowed in non-destructive mode"), nil
		}
	}

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	patchTypeStr, err := request.RequireString("patchType")
	if err != nil {
		return mcp.NewToolResultError("patchType is required"), nil
	}

	patchData, ok := request.GetArguments()["patch"]
	if !ok || patchData == nil {
		return mcp.NewToolResultError("patch is required"), nil
	}

	// Convert patch type string to types.PatchType
	var patchType types.PatchType
	switch patchTypeStr {
	case "strategic":
		patchType = types.StrategicMergePatchType
	case "merge":
		patchType = types.MergePatchType
	case "json":
		patchType = types.JSONPatchType
	default:
		return mcp.NewToolResultError("Invalid patch type. Must be one of: strategic, merge, json"), nil
	}

	// Convert patch data to JSON bytes
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal patch data: %v", err)), nil
	}

	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	patchedObj, err := k8sClient.Patch(ctx, kubeContext, namespace, resourceType, apiGroup, name, patchType, patchBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to patch resource: %v", err)), nil
	}

	// Convert the patched resource to JSON for output
	jsonData, err := json.MarshalIndent(patchedObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal patched resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleScaleResource handles kubectl scale operations
func handleScaleResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if scale operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "scale" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Scale operations are not allowed in non-destructive mode"), nil
		}
	}

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	replicas, err := request.RequireFloat("replicas")
	if err != nil {
		return mcp.NewToolResultError("replicas is required"), nil
	}

	// Use appropriate k8s client (per-user if OAuth downstream enabled)
	k8sClient := tools.GetK8sClient(ctx, sc)
	err = k8sClient.Scale(ctx, kubeContext, namespace, resourceType, apiGroup, name, int32(replicas))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to scale resource: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Resource %s/%s scaled to %d replicas successfully", resourceType, name, int32(replicas))), nil
}
