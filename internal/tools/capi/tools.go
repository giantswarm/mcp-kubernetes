package capi

import (
	"github.com/giantswarm/mcp-toolkit/tooldesc"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// RegisterCAPITools registers all CAPI discovery tools with the MCP server.
// These tools are only registered when federation mode is enabled.
//
// Tools registered:
//   - capi_list_clusters: List all workload clusters with optional filtering
//   - capi_get_cluster: Get detailed information about a specific cluster
//   - capi_resolve_cluster: Resolve a partial cluster name to its full identifier
//   - capi_cluster_health: Check the health status of a cluster
func RegisterCAPITools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// Only register CAPI tools when federation is enabled
	if !sc.FederationEnabled() {
		return nil
	}

	// capi_list_clusters tool
	listClustersTool := mcp.NewTool("capi_list_clusters",
		tooldesc.Describe("List the workload clusters managed by CAPI that you can access: name, organization, provider, release, status, age.", "Results are limited by default; narrow with the filters or raise limit."),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithString("organization",
			mcp.Description("Filter by organization namespace (e.g., 'org-acme')"),
		),
		mcp.WithString("provider",
			mcp.Description("Filter by infrastructure provider (aws, azure, gcp, vsphere)"),
		),
		mcp.WithString("status",
			mcp.Description("Filter by cluster status (Provisioned, Provisioning, Deleting, Failed)"),
		),
		mcp.WithBoolean("readyOnly",
			mcp.Description("Only show clusters that are fully ready (default: false)"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Filter clusters by Kubernetes label selector (e.g., 'environment=production')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of clusters to return (default: 100, max: 500). Use filters to narrow results if truncated."),
		),
	)

	s.AddTool(listClustersTool, tools.WrapWithAuditLogging("capi_list_clusters", handleListClusters, sc))

	// capi_get_cluster tool
	getClusterTool := mcp.NewTool("capi_get_cluster",
		tooldesc.Describe("Get detailed information about a specific CAPI cluster including metadata, status, labels, and annotations.", ""),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the cluster to get details for"),
		),
	)

	s.AddTool(getClusterTool, tools.WrapWithAuditLogging("capi_get_cluster", handleGetCluster, sc))

	// capi_resolve_cluster tool
	resolveClusterTool := mcp.NewTool("capi_resolve_cluster",
		tooldesc.Describe("Resolve a partial cluster name pattern to its full identifier. Useful when you only know part of a cluster name.", ""),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Partial cluster name or pattern to search for (e.g., 'prod' to find 'prod-wc-01')"),
		),
	)

	s.AddTool(resolveClusterTool, tools.WrapWithAuditLogging("capi_resolve_cluster", handleResolveCluster, sc))

	// capi_cluster_health tool
	clusterHealthTool := mcp.NewTool("capi_cluster_health",
		tooldesc.Describe("Check the health status of a CAPI cluster. Returns overall health, component status, and individual health checks.", ""),
		tools.ReadOnly(),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the cluster to check health for"),
		),
	)

	s.AddTool(clusterHealthTool, tools.WrapWithAuditLogging("capi_cluster_health", handleClusterHealth, sc))

	return nil
}
