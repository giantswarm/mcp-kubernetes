package cluster

import (
	"github.com/giantswarm/mcp-toolkit/tooldesc"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// RegisterClusterTools registers all cluster management tools with the MCP server
func RegisterClusterTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// Get cluster/context parameters based on server mode
	clusterContextParams := tools.AddClusterContextParams(sc)

	// api_resources tool
	apiResourcesOpts := []mcp.ToolOption{
		tooldesc.Describe("List available API resources in the cluster", ""),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
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
	apiResourcesTool := mcp.NewTool("api_resources", apiResourcesOpts...)

	s.AddTool(apiResourcesTool, tools.WrapWithAuditLogging("api_resources", handleGetAPIResources, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "api_resources", handleGetAPIResources, apiResourcesOpts...)

	// cluster_health tool
	clusterHealthOpts := []mcp.ToolOption{
		tooldesc.Describe("Check the health of the cluster's components and nodes: overall status, component health and a node list.", "The node list is capped by nodesLimit; per-node conditions only with includeNodeConditions: true."),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	clusterHealthOpts = append(clusterHealthOpts, clusterContextParams...)
	clusterHealthOpts = append(clusterHealthOpts,
		mcp.WithNumber("nodesLimit",
			mcp.Min(1),
			mcp.Max(MaxNodesLimit),
			mcp.Description("Most nodes to include (default 20, at most 1000); readyNodes and totalNodes count them all."),
		),
		mcp.WithBoolean("includeNodeConditions",
			mcp.Description("Include the full per-node conditions array in the response (default: false). The Ready field on each node already conveys overall readiness."),
		),
	)
	clusterHealthTool := mcp.NewTool("cluster_health", clusterHealthOpts...)

	s.AddTool(clusterHealthTool, tools.WrapWithAuditLogging("cluster_health", handleGetClusterHealth, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "cluster_health", handleGetClusterHealth, clusterHealthOpts...)

	return nil
}
