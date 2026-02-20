package cluster

import (
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// RegisterClusterTools registers all cluster management tools with the MCP server
func RegisterClusterTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// Get cluster/context parameters based on server mode
	clusterContextParams := tools.AddClusterContextParams(sc)

	// kubernetes_api_resources tool
	apiResourcesOpts := []mcp.ToolOption{
		mcp.WithDescription("List available API resources in the cluster"),
	}
	apiResourcesOpts = append(apiResourcesOpts, clusterContextParams...)
	apiResourcesOpts = append(apiResourcesOpts,
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
	apiResourcesTool := mcp.NewTool("kubernetes_api_resources", apiResourcesOpts...)

	s.AddTool(apiResourcesTool, tools.WrapWithAuditLogging("kubernetes_api_resources", handleGetAPIResources, sc))

	// kubernetes_cluster_health tool
	clusterHealthOpts := []mcp.ToolOption{
		mcp.WithDescription("Check the health status of cluster components"),
	}
	clusterHealthOpts = append(clusterHealthOpts, clusterContextParams...)
	clusterHealthTool := mcp.NewTool("kubernetes_cluster_health", clusterHealthOpts...)

	s.AddTool(clusterHealthTool, tools.WrapWithAuditLogging("kubernetes_cluster_health", handleGetClusterHealth, sc))

	return nil
}
