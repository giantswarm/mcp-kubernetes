package cluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleGetAPIResources handles kubectl api-resources operations
func handleGetAPIResources(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)

	resources, err := sc.K8sClient().GetAPIResources(ctx, kubeContext)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get API resources: %v", err)), nil
	}

	// Convert resources to JSON for output
	jsonData, err := json.MarshalIndent(resources, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal API resources: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleGetClusterHealth handles kubectl cluster health operations
func handleGetClusterHealth(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)

	health, err := sc.K8sClient().GetClusterHealth(ctx, kubeContext)
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
