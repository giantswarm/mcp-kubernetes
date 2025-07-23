package cluster

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// RegisterClusterTools registers all cluster management tools with the MCP server
func RegisterClusterTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// kubernetes_api_resources tool
	apiResourcesTool := mcp.NewTool("kubernetes_api_resources",
		mcp.WithDescription("List available API resources in the cluster"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("apiGroup",
			mcp.Description("Filter by API group (optional)"),
		),
		mcp.WithBoolean("namespaced",
			mcp.Description("Filter by namespaced resources only (optional)"),
		),
		mcp.WithString("verbs",
			mcp.Description("Filter by supported verbs (e.g., 'get,list,create') (optional)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return per page (optional, default: 20, 0 = no limit)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Number of items to skip (optional, for simple offset-based pagination)"),
		),
	)

	s.AddTool(apiResourcesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetAPIResources(ctx, request, sc)
	})

	// kubernetes_cluster_health tool
	clusterHealthTool := mcp.NewTool("kubernetes_cluster_health",
		mcp.WithDescription("Check the health status of cluster components"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
	)

	s.AddTool(clusterHealthTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetClusterHealth(ctx, request, sc)
	})

	return nil
}
