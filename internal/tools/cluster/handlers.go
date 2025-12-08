package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// handleGetAPIResources handles kubectl api-resources operations
func handleGetAPIResources(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)

	// Extract filter parameters
	apiGroup, _ := args["apiGroup"].(string)
	namespacedOnly, _ := args["namespaced"].(bool)
	verbsStr, _ := args["verbs"].(string)

	// Parse verbs
	var verbs []string
	if verbsStr != "" {
		for _, verb := range strings.Split(verbsStr, ",") {
			verbs = append(verbs, strings.TrimSpace(verb))
		}
	}

	// Extract pagination parameters with sensible defaults
	var limit, offset int = 20, 0 // Default page size for API resources
	if limitVal, ok := args["limit"]; ok {
		if limitFloat, ok := limitVal.(float64); ok {
			limit = int(limitFloat)
		}
	}
	if offsetVal, ok := args["offset"]; ok {
		if offsetFloat, ok := offsetVal.(float64); ok {
			offset = int(offsetFloat)
		}
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()
	paginatedResponse, err := k8sClient.GetAPIResources(ctx, kubeContext, limit, offset, apiGroup, namespacedOnly, verbs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get API resources: %v", err)), nil
	}

	// Convert paginated response to JSON
	jsonData, err := json.MarshalIndent(paginatedResponse, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal API resources: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleGetClusterHealth handles kubectl cluster health operations
func handleGetClusterHealth(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()
	health, err := k8sClient.GetClusterHealth(ctx, kubeContext)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get cluster health: %v", err)), nil
	}

	// Convert health to JSON for output
	jsonData, err := json.MarshalIndent(health, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal cluster health: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}
