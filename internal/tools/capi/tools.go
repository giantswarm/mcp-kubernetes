package capi

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
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
		mcp.WithDescription("List all Workload Clusters managed by CAPI that you have access to. Returns cluster name, organization, provider, release version, status, and age. Results are limited by default; use filters or increase limit to see more."),
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

	s.AddTool(listClustersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListClusters(ctx, request, sc)
	})

	// capi_get_cluster tool
	getClusterTool := mcp.NewTool("capi_get_cluster",
		mcp.WithDescription("Get detailed information about a specific CAPI cluster including metadata, status, labels, and annotations."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the cluster to get details for"),
		),
	)

	s.AddTool(getClusterTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetCluster(ctx, request, sc)
	})

	// capi_resolve_cluster tool
	resolveClusterTool := mcp.NewTool("capi_resolve_cluster",
		mcp.WithDescription("Resolve a partial cluster name pattern to its full identifier. Useful when you only know part of a cluster name."),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Partial cluster name or pattern to search for (e.g., 'prod' to find 'prod-wc-01')"),
		),
	)

	s.AddTool(resolveClusterTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleResolveCluster(ctx, request, sc)
	})

	// capi_cluster_health tool
	clusterHealthTool := mcp.NewTool("capi_cluster_health",
		mcp.WithDescription("Check the health status of a CAPI cluster. Returns overall health, component status, and individual health checks."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the cluster to check health for"),
		),
	)

	s.AddTool(clusterHealthTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleClusterHealth(ctx, request, sc)
	})

	return nil
}
