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

	// context_list tool
	listContextsOpts := []mcp.ToolOption{
		mcp.WithDescription("List all available Kubernetes contexts"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	s.AddTool(mcp.NewTool("context_list", listContextsOpts...), tools.WrapWithAuditLogging("context_list", handleListContexts, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "context_list", handleListContexts, listContextsOpts...)

	// context_get_current tool
	getCurrentContextOpts := []mcp.ToolOption{
		mcp.WithDescription("Get the current Kubernetes context"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
	}
	s.AddTool(mcp.NewTool("context_get_current", getCurrentContextOpts...), tools.WrapWithAuditLogging("context_get_current", handleGetCurrentContext, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "context_get_current", handleGetCurrentContext, getCurrentContextOpts...)

	// context_use tool
	useContextOpts := []mcp.ToolOption{
		mcp.WithDescription("Switch to a different Kubernetes context"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithString("contextName",
			mcp.Required(),
			mcp.Description("Name of the Kubernetes context to switch to"),
		),
	}
	s.AddTool(mcp.NewTool("context_use", useContextOpts...), tools.WrapWithAuditLogging("context_use", handleUseContext, sc))
	tools.MaybeAddDeprecatedAlias(s, sc, "context_use", handleUseContext, useContextOpts...)

	return nil
}
