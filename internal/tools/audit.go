// Package tools provides shared utilities and types for MCP tool implementations.
package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// ToolHandler is the signature for MCP tool handler functions that take ServerContext.
type ToolHandler func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error)

// WrapWithAuditLogging wraps a tool handler with audit logging.
// This function creates a wrapper that automatically captures:
//   - Tool invocation timing
//   - User identity from OAuth context (if available)
//   - Cluster and resource information from request arguments
//   - Success/error status from the handler result
//   - OpenTelemetry trace context for correlation
//
// The wrapper logs tool invocations using the AuditLogger from the instrumentation provider.
// If no instrumentation provider is available, the handler is called without audit logging.
func WrapWithAuditLogging(
	toolName string,
	handler ToolHandler,
	sc *server.ServerContext,
) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the instrumentation provider
		provider := sc.InstrumentationProvider()
		if provider == nil || provider.AuditLogger() == nil {
			// No audit logging available, just call the handler
			return handler(ctx, request, sc)
		}

		auditLogger := provider.AuditLogger()

		// Create tool invocation with span context
		invocation := instrumentation.NewToolInvocation(toolName).
			WithSpanContext(ctx)

		// Extract user info from OAuth context
		if user, ok := oauth.UserInfoFromContext(ctx); ok && user != nil {
			invocation.WithUser(user.Email, user.Groups)
		}

		// Extract cluster and resource info from request arguments
		args := request.GetArguments()
		extractAuditInfoFromArgs(invocation, args)

		// Execute the actual handler
		result, err := handler(ctx, request, sc)

		// Determine success/error status
		if err != nil {
			invocation.CompleteWithError(err)
		} else if result != nil && result.IsError {
			// MCP tool errors are returned in the result, not as Go errors
			invocation.Complete(false, nil)
			// Try to extract error message from result content
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(mcp.TextContent); ok {
					invocation.Error = textContent.Text
				}
			}
		} else {
			invocation.CompleteSuccess()
		}

		// Log the tool invocation (metrics-safe, uses cardinality-controlled values)
		auditLogger.LogToolInvocation(invocation)

		return result, err
	}
}

// extractAuditInfoFromArgs extracts cluster, namespace, and resource information
// from tool request arguments for audit logging.
func extractAuditInfoFromArgs(invocation *instrumentation.ToolInvocation, args map[string]interface{}) {
	// Extract cluster name (for federation/multi-cluster operations)
	if cluster, ok := args["cluster"].(string); ok && cluster != "" {
		invocation.WithCluster(cluster)
	} else if kubeContext, ok := args["kubeContext"].(string); ok && kubeContext != "" {
		// Fall back to kubeContext as cluster identifier
		invocation.WithCluster(kubeContext)
	}

	// Extract resource information
	namespace, _ := args["namespace"].(string)
	resourceType, _ := args["resourceType"].(string)
	resourceName := extractResourceName(args)

	if namespace != "" || resourceType != "" || resourceName != "" {
		invocation.WithResource(namespace, resourceType, resourceName)
	}
}

// extractResourceName extracts the resource name from various argument patterns.
// Different tools use different parameter names for the resource name.
func extractResourceName(args map[string]interface{}) string {
	// Try common parameter names for resource name
	nameKeys := []string{"name", "podName", "resourceName", "pattern", "sessionID"}
	for _, key := range nameKeys {
		if name, ok := args[key].(string); ok && name != "" {
			return name
		}
	}
	return ""
}
