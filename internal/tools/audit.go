// Package tools provides shared utilities and types for MCP tool implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// ToolHandler is the signature for MCP tool handler functions that take ServerContext.
type ToolHandler func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error)

// allowedArgsCache memoizes the set of declared parameter names per tool so we
// only parse RawInputSchema once.
var allowedArgsCache sync.Map // map[string]map[string]struct{}, keyed by tool name

// WrapWithAuditLogging wraps a tool handler with audit logging and unknown-arg
// rejection.
//
// Audit-logging behavior captures:
//   - Tool invocation timing
//   - User identity from OAuth context (if available)
//   - Cluster and resource information from request arguments
//   - Success/error status from the handler result
//   - OpenTelemetry trace context for correlation
//
// Unknown-arg rejection: if the caller passes any argument key that is not
// declared in the tool's input schema, the wrapper returns a tool-result error
// listing the unknown args and the valid ones. This prevents silent footguns
// where, for example, an LLM passes `cursor` to a tool that expects `nextCursor`
// and the wrong-named arg is silently dropped.
//
// If no instrumentation provider is available, audit logging is skipped, but
// arg validation still runs.
func WrapWithAuditLogging(
	tool mcp.Tool,
	handler ToolHandler,
	sc *server.ServerContext,
) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := tool.Name
	allowed := allowedArgsForTool(tool)

	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		// Validate unknown args before any other work so callers get a fast,
		// visible failure instead of silent argument drop.
		if errMsg := validateKnownArgs(args, allowed); errMsg != "" {
			return mcp.NewToolResultError(errMsg), nil
		}

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

// allowedArgsForTool extracts the set of declared input parameter names from a
// tool's schema. Falls back gracefully if the schema cannot be introspected
// (returns nil, which disables validation for that tool).
func allowedArgsForTool(tool mcp.Tool) map[string]struct{} {
	if cached, ok := allowedArgsCache.Load(tool.Name); ok {
		return cached.(map[string]struct{})
	}

	allowed := extractAllowedArgs(tool)
	allowedArgsCache.Store(tool.Name, allowed)
	return allowed
}

func extractAllowedArgs(tool mcp.Tool) map[string]struct{} {
	// Prefer the structured schema when present.
	if len(tool.InputSchema.Properties) > 0 {
		out := make(map[string]struct{}, len(tool.InputSchema.Properties))
		for k := range tool.InputSchema.Properties {
			out[k] = struct{}{}
		}
		return out
	}

	// Fall back to RawInputSchema (used by mcp.WithInputSchema[T]()).
	if len(tool.RawInputSchema) > 0 {
		var parsed struct {
			Properties           map[string]any `json:"properties"`
			AdditionalProperties any            `json:"additionalProperties"`
		}
		if err := json.Unmarshal(tool.RawInputSchema, &parsed); err == nil {
			// If the schema explicitly allows additional properties, skip
			// validation by returning nil.
			if b, ok := parsed.AdditionalProperties.(bool); ok && b {
				return nil
			}
			out := make(map[string]struct{}, len(parsed.Properties))
			for k := range parsed.Properties {
				out[k] = struct{}{}
			}
			return out
		}
	}

	// Schema unavailable or unparseable; disable validation.
	return nil
}

// validateKnownArgs returns a non-empty error message if args contains any key
// not declared in the allowed set. Returns "" when the args are valid or when
// allowed is nil (validation disabled).
func validateKnownArgs(args map[string]interface{}, allowed map[string]struct{}) string {
	if allowed == nil {
		return ""
	}
	var unknown []string
	for k := range args {
		if _, ok := allowed[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return ""
	}
	sort.Strings(unknown)

	valid := make([]string, 0, len(allowed))
	for k := range allowed {
		valid = append(valid, k)
	}
	sort.Strings(valid)

	return fmt.Sprintf(
		"unknown argument(s): %s. Valid arguments are: %s",
		strings.Join(unknown, ", "),
		strings.Join(valid, ", "),
	)
}
