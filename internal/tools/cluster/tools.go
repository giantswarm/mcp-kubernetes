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
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
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
		mcp.WithDescription("Check the health status of cluster components. Returns overall status, component health, and a node list (capped by nodesLimit). Per-node conditions are omitted by default; set includeNodeConditions=true to include them."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	}
	clusterHealthOpts = append(clusterHealthOpts, clusterContextParams...)
	clusterHealthOpts = append(clusterHealthOpts,
		mcp.WithNumber("nodesLimit",
			mcp.Min(1),
			mcp.Max(MaxNodesLimit),
			mcp.Description("Maximum number of nodes to include in the response. Default: 20. Maximum: 1000. Use readyNodes/totalNodes to detect readiness issues even when truncated."),
		),
		mcp.WithBoolean("includeNodeConditions",
			mcp.Description("Include the full per-node conditions array in the response (default: false). The Ready field on each node already conveys overall readiness."),
		),
	)
	clusterHealthTool := mcp.NewTool("kubernetes_cluster_health", clusterHealthOpts...)

	s.AddTool(clusterHealthTool, tools.WrapWithAuditLogging("kubernetes_cluster_health", handleGetClusterHealth, sc))

	return nil
}
