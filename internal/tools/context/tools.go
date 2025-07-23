package contexttools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// RegisterContextTools registers all context management tools with the MCP server
func RegisterContextTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// kubernetes_context_list tool
	listContextsTool := mcp.NewTool("kubernetes_context_list",
		mcp.WithDescription("List all available Kubernetes contexts"),
	)

	s.AddTool(listContextsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListContexts(ctx, request, sc)
	})

	// kubernetes_context_get_current tool
	getCurrentContextTool := mcp.NewTool("kubernetes_context_get_current",
		mcp.WithDescription("Get the current Kubernetes context"),
	)

	s.AddTool(getCurrentContextTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetCurrentContext(ctx, request, sc)
	})

	// kubernetes_context_use tool
	useContextTool := mcp.NewTool("kubernetes_context_use",
		mcp.WithDescription("Switch to a different Kubernetes context"),
		mcp.WithString("contextName",
			mcp.Required(),
			mcp.Description("Name of the Kubernetes context to switch to"),
		),
	)

	s.AddTool(useContextTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleUseContext(ctx, request, sc)
	})

	return nil
}
