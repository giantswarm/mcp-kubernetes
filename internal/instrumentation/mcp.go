package instrumentation

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpotel "github.com/mark3labs/mcp-go/otel"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// AttrGenAIToolName is the OpenTelemetry GenAI key naming the tool a
// tools/call runs, the key muster puts on its side of the same call.
const AttrGenAIToolName = "gen_ai.tool.name"

// MCPServerOptions emit an mcp.<method> server span for every JSON-RPC
// request and a tool.<name> span around each tool handler. The tools/call
// span also carries gen_ai.tool.name. The propagator extracts nothing: the
// HTTP server span in the request context already joined the caller's
// traceparent, and the MCP span nests under it. Register them before the
// other tool middlewares so the tool span covers them.
func MCPServerOptions() []mcpserver.ServerOption {
	return []mcpserver.ServerOption{
		mcpserver.WithToolHandlerMiddleware(toolNameAttribute),
		mcpotel.WithServerTracingPropagator(otel.Tracer(TracerName), propagation.NewCompositeTextMapPropagator()),
	}
}

// toolNameAttribute runs outside mcp-go's tool span, so the span in ctx is
// the tools/call server span.
func toolNameAttribute(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String(AttrGenAIToolName, req.Params.Name))
		return next(ctx, req)
	}
}
