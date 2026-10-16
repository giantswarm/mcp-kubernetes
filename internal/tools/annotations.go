package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// ReadOnly annotates a tool that changes nothing: read-only, idempotent and
// not destructive, which mcp-go's default would report it as.
func ReadOnly() mcp.ToolOption {
	return func(t *mcp.Tool) {
		mcp.WithReadOnlyHintAnnotation(true)(t)
		mcp.WithDestructiveHintAnnotation(false)(t)
		mcp.WithIdempotentHintAnnotation(true)(t)
	}
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
