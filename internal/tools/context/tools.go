package contexttools

import (
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// RegisterContextTools registers all context management tools with the MCP server.
// These tools are only registered when NOT running in in-cluster mode, as
// kubeconfig context switching is not applicable when using service account authentication.
func RegisterContextTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// Skip registering context tools when running in-cluster mode
	// as context switching is not applicable with service account authentication
	if sc.InClusterMode() {
		return nil
	}

	// kubernetes_context_list tool
	listContextsTool := mcp.NewTool("kubernetes_context_list",
		mcp.WithDescription("List all available Kubernetes contexts"),
		mcp.WithInputSchema[tools.EmptyRequest](),
	)

	s.AddTool(listContextsTool, tools.WrapWithAuditLogging("kubernetes_context_list", handleListContexts, sc))

	// kubernetes_context_get_current tool
	getCurrentContextTool := mcp.NewTool("kubernetes_context_get_current",
		mcp.WithDescription("Get the current Kubernetes context"),
		mcp.WithInputSchema[tools.EmptyRequest](),
	)

	s.AddTool(getCurrentContextTool, tools.WrapWithAuditLogging("kubernetes_context_get_current", handleGetCurrentContext, sc))

	// kubernetes_context_use tool
	useContextTool := mcp.NewTool("kubernetes_context_use",
		mcp.WithDescription("Switch to a different Kubernetes context"),
		mcp.WithString("contextName",
			mcp.Required(),
			mcp.Description("Name of the Kubernetes context to switch to"),
		),
	)

	s.AddTool(useContextTool, tools.WrapWithAuditLogging("kubernetes_context_use", handleUseContext, sc))

	return nil
}
