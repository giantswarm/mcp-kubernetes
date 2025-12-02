package tools

import (
	"context"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// EmptyRequest represents a request with no parameters.
// Used by tools that don't require any input arguments.
type EmptyRequest struct{}

// GetK8sClient returns the appropriate Kubernetes client for the given context.
// If downstream OAuth is enabled and an access token is present in the context,
// it returns a per-user client. Otherwise, it returns the shared service account client.
//
// Tool handlers should use this function instead of directly calling sc.K8sClient()
// to ensure proper OAuth passthrough when enabled.
func GetK8sClient(ctx context.Context, sc *server.ServerContext) k8s.Client {
	return sc.K8sClientForContext(ctx)
}
