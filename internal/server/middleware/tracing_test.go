package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
)

// recordSpans installs an in-memory tracer provider and the W3C propagator
// as the globals the instrumentation provider installs, restoring the
// previous ones after the test.
func recordSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prevProvider, prevPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})
	return exporter
}

func postMCP(t *testing.T, url, sessionID, traceparent, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/mcp", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set(mcpserver.HeaderKeySessionID, sessionID)
	}
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp
}

func TestToolCallIsTracedAndJoinsTheCallersTrace(t *testing.T) {
	exporter := recordSpans(t)

	mcpSrv := mcpserver.NewMCPServer("mcp-kubernetes", "test", instrumentation.MCPServerOptions()...)
	mcpSrv.AddTool(mcp.NewTool("echo"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpserver.NewStreamableHTTPServer(mcpSrv, mcpserver.WithEndpointPath("/mcp")))
	for _, path := range []string{"/healthz", "/readyz"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}
	ts := httptest.NewServer(Tracing(mux))
	defer ts.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(ts.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}
	require.Empty(t, exporter.GetSpans(), "the probes are not traced")

	init := postMCP(t, ts.URL, "", "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	sessionID := init.Header.Get(mcpserver.HeaderKeySessionID)
	exporter.Reset()

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	postMCP(t, ts.URL, sessionID, "00-"+traceID+"-00f067aa0ba902b7-01",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{}}}`)

	byName := map[string]sdktrace.ReadOnlySpan{}
	for _, s := range exporter.GetSpans().Snapshots() {
		require.Equal(t, traceID, s.SpanContext().TraceID().String(), "span %s joins the caller's trace", s.Name())
		byName[s.Name()] = s
	}
	require.Contains(t, byName, "POST /mcp")
	require.Contains(t, byName, "mcp.tools/call")
	require.Contains(t, byName, "tool.echo")

	httpSpan := byName["POST /mcp"]
	require.Equal(t, trace.SpanKindServer, httpSpan.SpanKind())
	require.Equal(t, "00f067aa0ba902b7", httpSpan.Parent().SpanID().String())
	call := byName["mcp.tools/call"]
	require.Equal(t, trace.SpanKindServer, call.SpanKind())
	require.Equal(t, httpSpan.SpanContext().SpanID(), call.Parent().SpanID())
	attrs := map[string]string{}
	for _, kv := range call.Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	require.Equal(t, "echo", attrs[instrumentation.AttrGenAIToolName])
	require.Equal(t, "echo", attrs["mcp.tool.name"])
	require.Equal(t, "tools/call", attrs["mcp.method"])
	require.Equal(t, call.SpanContext().SpanID(), byName["tool.echo"].Parent().SpanID())
}
