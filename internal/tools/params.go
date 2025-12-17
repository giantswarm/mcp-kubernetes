// Package tools provides shared utilities and types for MCP tool implementations.
package tools

import (
	mcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// AddClusterContextParams returns tool options for cluster and kubeContext parameters
// based on the server's operating mode. This ensures backwards compatibility:
//   - cluster parameter is only added when federation is enabled
//   - kubeContext parameter is only added when NOT in in-cluster mode
//
// Usage in tool registration:
//
//	opts := []mcp.ToolOption{
//	    mcp.WithDescription("..."),
//	}
//	opts = append(opts, tools.AddClusterContextParams(sc)...)
//	opts = append(opts, /* tool-specific params */...)
//	tool := mcp.NewTool("tool_name", opts...)
func AddClusterContextParams(sc *server.ServerContext) []mcp.ToolOption {
	var opts []mcp.ToolOption

	// Add cluster parameter only when federation (CAPI mode) is enabled
	if sc.FederationEnabled() {
		opts = append(opts, mcp.WithString("cluster",
			mcp.Description("Target cluster name for multi-cluster operations (optional, empty for local/default cluster)"),
		))
	}

	// Add kubeContext parameter only when NOT in in-cluster mode
	if !sc.InClusterMode() {
		opts = append(opts, mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		))
	}

	return opts
}
