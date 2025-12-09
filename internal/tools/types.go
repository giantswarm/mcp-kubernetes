package tools

import (
	"context"
	"errors"

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
// When downstream OAuth strict mode is enabled and authentication fails,
// returns (nil, error) with an authentication error.
//
// Tool handlers should use this function instead of directly calling sc.K8sClient()
// to ensure proper OAuth passthrough when enabled.
//
// # Error Handling
//
// Returns (nil, error) when downstream OAuth strict mode is enabled and:
//   - No OAuth token is present in the context (server.ErrOAuthTokenMissing)
//   - The OAuth token cannot be used to create a client (server.ErrOAuthClientFailed)
//
// Returns (client, nil) on success.
func GetK8sClient(ctx context.Context, sc *server.ServerContext) (k8s.Client, error) {
	return sc.K8sClientForContext(ctx)
}

// MustGetK8sClient returns the appropriate Kubernetes client for the given context.
// Unlike GetK8sClient, this function panics if authentication fails in strict mode.
//
// Use this function only in contexts where authentication failures are unexpected
// and indicate a programming error (e.g., internal operations that don't require
// user authentication).
//
// Deprecated: Prefer GetK8sClient for proper error handling.
func MustGetK8sClient(ctx context.Context, sc *server.ServerContext) k8s.Client {
	client, err := sc.K8sClientForContext(ctx)
	if err != nil {
		panic("failed to get k8s client: " + err.Error())
	}
	return client
}

// IsAuthenticationError returns true if the error is an OAuth authentication error.
// This can be used to distinguish between authentication failures and other errors.
func IsAuthenticationError(err error) bool {
	return errors.Is(err, server.ErrOAuthTokenMissing) || errors.Is(err, server.ErrOAuthClientFailed)
}

// FormatAuthenticationError returns a user-friendly error message for authentication errors.
// If the error is not an authentication error, returns a generic message.
func FormatAuthenticationError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, server.ErrOAuthTokenMissing) {
		return "authentication required: please log in to access this resource"
	}
	if errors.Is(err, server.ErrOAuthClientFailed) {
		return "authentication failed: your session may have expired, please log in again"
	}
	return "authentication error: unable to verify your credentials"
}
