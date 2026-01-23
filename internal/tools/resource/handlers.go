package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/logging"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/output"
)

// recordK8sOperation records metrics for a Kubernetes operation.
// Delegates to ServerContext which handles nil checks internally.
func recordK8sOperation(ctx context.Context, sc *server.ServerContext, clusterName, operation, resourceType, namespace, status string, duration time.Duration) {
	sc.RecordK8sOperation(ctx, clusterName, operation, resourceType, namespace, status, duration)
}

// checkMutatingOperation is a convenience wrapper around tools.CheckMutatingOperation.
// It verifies if a mutating operation is allowed given the current server configuration.
func checkMutatingOperation(sc *server.ServerContext, operation string) *mcp.CallToolResult {
	return tools.CheckMutatingOperation(sc, operation)
}

// handleGetResource handles kubectl get operations
func handleGetResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	namespace, _ := args["namespace"].(string)
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	getResponse, err := k8sClient.Get(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Apply output processing (slim output, secret masking)
	processor := getOutputProcessor(sc)
	processedObj, err := output.ProcessSingleRuntimeObject(processor, getResponse.Resource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resource: %v", err)), nil
	}

	// Build response with metadata
	response := map[string]interface{}{
		"resource": processedObj,
		"_meta":    getResponse.Meta,
	}

	// Convert the response to JSON for output
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// getOutputProcessor creates an output processor from server context configuration.
func getOutputProcessor(sc *server.ServerContext) *output.Processor {
	outputCfg := sc.OutputConfig()
	cfg := &output.Config{
		MaxItems:         outputCfg.MaxItems,
		MaxClusters:      outputCfg.MaxClusters,
		MaxResponseBytes: outputCfg.MaxResponseBytes,
		SlimOutput:       outputCfg.SlimOutput,
		MaskSecrets:      outputCfg.MaskSecrets,
		SummaryThreshold: outputCfg.SummaryThreshold,
	}
	return output.NewProcessor(cfg)
}

// handleListResources handles kubectl list operations
func handleListResources(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	handlerStart := time.Now()
	slog.Debug("list resources handler started", slog.String("method", request.Method))
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	namespace, _ := args["namespace"].(string)
	allNamespaces, _ := args["allNamespaces"].(bool)

	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API simply ignores the namespace.
	// This approach requires no static resource lists and works with any CRD.
	if !allNamespaces && namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	labelSelector, _ := args["labelSelector"].(string)
	fieldSelector, _ := args["fieldSelector"].(string)

	// Client-side filtering parameter
	var filterCriteria FilterCriteria
	if filterArg, ok := args["filter"]; ok {
		filterMap, ok := filterArg.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("filter parameter must be an object/map"), nil
		}
		filterCriteria = filterMap
	}

	// New parameters for controlling output format
	fullOutput, _ := args["fullOutput"].(bool)
	includeLabels, _ := args["includeLabels"].(bool)
	includeAnnotations, _ := args["includeAnnotations"].(bool)

	// Summary mode parameter for fleet-scale operations
	summaryMode, _ := args["summary"].(bool)

	// Output format parameter (normal, wide, slim)
	outputFormat, _ := args["output"].(string)
	if outputFormat == "" {
		outputFormat = "slim" // Default to slim output for reduced context usage
	}

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

	// Track namespace for metrics (use "all" for cluster-wide operations)
	metricsNamespace := namespace
	if allNamespaces {
		namespace = ""
		metricsNamespace = "all"
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()
	slog.Debug("acquired cluster client", slog.Duration("elapsed", time.Since(handlerStart)))

	k8sStart := time.Now()
	paginatedResponse, err := k8sClient.List(ctx, kubeContext, namespace, resourceType, apiGroup, opts)
	k8sDuration := time.Since(k8sStart)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationList, resourceType, metricsNamespace, instrumentation.StatusError, k8sDuration)
		slog.Debug("K8s list failed",
			slog.String("resourceType", resourceType),
			slog.Duration("duration", k8sDuration),
			logging.SanitizedErr(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list resources: %v", err)), nil
	}
	slog.Debug("K8s list completed",
		slog.String("resourceType", resourceType),
		slog.Int("items", paginatedResponse.TotalItems),
		slog.Duration("duration", k8sDuration))

	// Apply client-side filtering if criteria provided
	if len(filterCriteria) > 0 {
		filteredItems, err := ApplyClientSideFilter(paginatedResponse.Items, filterCriteria)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid filter criteria: %v", err)), nil
		}
		paginatedResponse.Items = filteredItems
		paginatedResponse.TotalItems = len(paginatedResponse.Items)

		// Warn on large result sets after filtering (potential performance issue)
		if paginatedResponse.TotalItems > 1000 {
			slog.Warn("large result set after client-side filtering",
				logging.ResourceType(resourceType),
				slog.Int("item_count", paginatedResponse.TotalItems),
				slog.Int("filter_count", len(filterCriteria)))
		}
	}
	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationList, resourceType, metricsNamespace, instrumentation.StatusSuccess, k8sDuration)

	// Get output processor for slim output and secret masking
	processor := getOutputProcessor(sc)

	// Handle summary mode - return aggregated counts instead of full items
	if summaryMode {
		return handleSummaryResponse(paginatedResponse.Items, processor, resourceType)
	}

	// Apply output processing (slim output, secret masking) based on output format
	if outputFormat == "slim" || outputFormat == "normal" {
		processedItems, result, err := output.ProcessRuntimeObjects(processor, paginatedResponse.Items)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to process resources: %v", err)), nil
		}
		paginatedResponse.Items = processedItems

		// Log truncation warnings
		if result.Metadata.Truncated {
			slog.Info("response truncated",
				logging.ResourceType(resourceType),
				slog.Int("final_count", result.Metadata.FinalCount),
				slog.Int("original_count", result.Metadata.OriginalCount))
		}
	}

	if fullOutput {
		// Return full paginated output with any processing warnings
		jsonData, err := json.MarshalIndent(paginatedResponse, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal paginated resources: %v", err)), nil
		}
		slog.Debug("list resources handler completed",
			slog.Int("bytes", len(jsonData)),
			slog.Duration("elapsed", time.Since(handlerStart)))
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

	slog.Debug("list resources handler completed",
		slog.Int("bytes", len(jsonData)),
		slog.Duration("elapsed", time.Since(handlerStart)))
	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleDescribeResource handles kubectl describe operations
func handleDescribeResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	namespace, _ := args["namespace"].(string)
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	description, err := k8sClient.Describe(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to describe resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Apply output processing (slim output, secret masking)
	processor := getOutputProcessor(sc)
	processedResource, err := output.ProcessSingleRuntimeObject(processor, description.Resource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resource: %v", err)), nil
	}

	// Build response with processed resource
	response := map[string]interface{}{
		"resource": processedResource,
		"metadata": description.Metadata,
		"_meta":    description.Meta,
	}

	// Include events if present
	if len(description.Events) > 0 {
		response["events"] = description.Events
	}

	// Convert the response to JSON for output
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal description: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleCreateResource handles kubectl create operations
func handleCreateResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "create"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

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

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	createdObj, err := k8sClient.Create(ctx, kubeContext, namespace, obj)
	duration := time.Since(start)

	// Extract resource type from manifest for metrics (use "unknown" if not available)
	resourceType := "unknown"
	if m, ok := manifestData.(map[string]interface{}); ok {
		if kind, ok := m["kind"].(string); ok {
			resourceType = kind
		}
	}

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationCreate, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationCreate, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the created resource to JSON for output
	jsonData, err := json.MarshalIndent(createdObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal created resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleApplyResource handles kubectl apply operations
func handleApplyResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "apply"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

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

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	appliedObj, err := k8sClient.Apply(ctx, kubeContext, namespace, obj)
	duration := time.Since(start)

	// Extract resource type from manifest for metrics (use "unknown" if not available)
	resourceType := "unknown"
	if m, ok := manifestData.(map[string]interface{}); ok {
		if kind, ok := m["kind"].(string); ok {
			resourceType = kind
		}
	}

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationApply, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to apply resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationApply, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the applied resource to JSON for output
	jsonData, err := json.MarshalIndent(appliedObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal applied resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleDeleteResource handles kubectl delete operations
func handleDeleteResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "delete"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace := request.GetString("namespace", "")
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	deleteResponse, err := k8sClient.Delete(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationDelete, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationDelete, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the response to JSON for output (includes _meta)
	jsonData, err := json.MarshalIndent(deleteResponse, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handlePatchResource handles kubectl patch operations
func handlePatchResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "patch"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace := request.GetString("namespace", "")
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
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

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	patchResponse, err := k8sClient.Patch(ctx, kubeContext, namespace, resourceType, apiGroup, name, patchType, patchBytes)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationPatch, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to patch resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationPatch, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Apply output processing (slim output, secret masking)
	processor := getOutputProcessor(sc)
	processedObj, err := output.ProcessSingleRuntimeObject(processor, patchResponse.Resource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resource: %v", err)), nil
	}

	// Build response with metadata
	response := map[string]interface{}{
		"resource": processedObj,
		"_meta":    patchResponse.Meta,
	}

	// Convert the response to JSON for output
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal patched resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleScaleResource handles kubectl scale operations
func handleScaleResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "scale"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

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

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	scaleResponse, err := k8sClient.Scale(ctx, kubeContext, namespace, resourceType, apiGroup, name, int32(replicas))
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationScale, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to scale resource: %v", err)), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationScale, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the scale response to JSON for output (includes _meta)
	jsonData, err := json.MarshalIndent(scaleResponse, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleSummaryResponse generates a summary response for large result sets.
// This provides aggregated counts by status, namespace, etc. instead of full items.
func handleSummaryResponse(items []runtime.Object, processor *output.Processor, resourceType string) (*mcp.CallToolResult, error) {
	// Convert to maps for summary generation
	maps, err := output.FromRuntimeObjects(items)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resources for summary: %v", err)), nil
	}

	// Generate summary
	opts := output.DefaultSummaryOptions()
	opts.IncludeByNamespace = true
	opts.IncludeByStatus = true
	opts.MaxSampleSize = 10

	summary := processor.GenerateSummary(maps, opts)

	// Build response with summary
	response := map[string]interface{}{
		"kind":           resourceType + "Summary",
		"total":          summary.Total,
		"sample":         summary.Sample,
		"hasMore":        summary.HasMore,
		"_isSummaryMode": true,
		"_hint":          "Use summary=false or add filters to see full resource details",
	}

	if len(summary.ByStatus) > 0 {
		response["byStatus"] = summary.ByStatus
	}

	if len(summary.ByNamespace) > 0 {
		// Limit namespace count to top 10 for readability
		if len(summary.ByNamespace) > 10 {
			topNamespaces := output.TopCounts(summary.ByNamespace, 10)
			nsMap := make(map[string]int)
			for _, entry := range topNamespaces {
				nsMap[entry.Key] = entry.Count
			}
			response["byNamespace"] = nsMap
			response["_namespacesTruncated"] = true
		} else {
			response["byNamespace"] = summary.ByNamespace
		}
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}
