package contexttools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleListContexts handles kubectl context list operations
func handleListContexts(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	contexts, err := sc.K8sClient().ListContexts(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list contexts: %v", err)), nil
	}

	// Convert contexts to JSON for output
	jsonData, err := json.MarshalIndent(contexts, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal contexts: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleGetCurrentContext handles kubectl context get-current operations
func handleGetCurrentContext(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	currentContext, err := sc.K8sClient().GetCurrentContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get current context: %v", err)), nil
	}

	// Convert current context to JSON for output
	jsonData, err := json.MarshalIndent(currentContext, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal current context: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleUseContext handles kubectl context use operations
func handleUseContext(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	
	contextName, ok := args["contextName"].(string)
	if !ok || contextName == "" {
		return mcp.NewToolResultError("contextName is required"), nil
	}

	err := sc.K8sClient().SwitchContext(ctx, contextName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to switch context: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully switched to context: %s", contextName)), nil
} 