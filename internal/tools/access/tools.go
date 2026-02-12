package access

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	mcpserver "github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// CanITool allows agents to check if the authenticated user has permission
// to perform a specific action on a Kubernetes resource.
//
// This tool uses SelfSubjectAccessReview to verify permissions before attempting
// operations, providing better error messages and reducing audit log noise.
var CanITool = mcp.NewTool("can_i",
	mcp.WithDescription("Check if you have permission to perform an action on a Kubernetes resource. "+
		"Use this before attempting operations to get clear feedback about permissions."),
	mcp.WithString("verb",
		mcp.Required(),
		mcp.Description("The action to check (get, list, watch, create, update, patch, delete)"),
	),
	mcp.WithString("resource",
		mcp.Required(),
		mcp.Description("The resource type to check (pods, deployments, secrets, etc.)"),
	),
	mcp.WithString("apiGroup",
		mcp.Description("API group for the resource (empty for core resources, 'apps' for deployments, etc.)"),
	),
	mcp.WithString("namespace",
		mcp.Description("Namespace to check permissions in (empty for cluster-scoped resources)"),
	),
	mcp.WithString("name",
		mcp.Description("Specific resource name to check (optional, for fine-grained checks)"),
	),
	mcp.WithString("subresource",
		mcp.Description("Subresource to check (e.g., 'logs', 'exec', 'portforward' for pods)"),
	),
	mcp.WithString("cluster",
		mcp.Description("Target cluster name (empty for local/management cluster)"),
	),
)

// RegisterTools registers the access tools with the MCP server.
func RegisterTools(mcpServer *server.MCPServer, sc *mcpserver.ServerContext) {
	mcpServer.AddTool(CanITool, tools.WrapWithAuditLogging("can_i", HandleCanI, sc))
}
