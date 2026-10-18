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
			mcp.Description("Target cluster; empty for the local one."),
		))
	}

	// Add kubeContext parameter only when NOT in in-cluster mode
	if !sc.InClusterMode() {
		opts = append(opts, mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context; default: the current one."),
		))
	}

	return opts
}

// NamespaceParam is the namespace argument of the tools that address one
// resource by name.
func NamespaceParam() mcp.ToolOption {
	return mcp.WithString("namespace", mcp.Description("Namespace of a namespaced resource; default 'default'. Ignored for cluster-scoped resources (nodes, namespaces, PVs, clusterroles)."))
}

// ResourceTypeParam is the required resourceType argument of the tools that
// address one resource by name.
func ResourceTypeParam() mcp.ToolOption {
	return mcp.WithString("resourceType", mcp.Required(), mcp.Description("Resource type (pod, service, deployment, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io)."))
}

// APIGroupParam is the optional apiGroup argument of the resource tools.
func APIGroupParam() mcp.ToolOption {
	return mcp.WithString("apiGroup", mcp.Description("Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')"))
}
