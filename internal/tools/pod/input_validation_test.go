package pod

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// TestInputSchemaValidation_LogsAcceptsOutputArg pins the fix for
// giantswarm/mcp-kubernetes#409 on the pod side: kubernetes_logs must accept
// the `output` argument for argument-shape symmetry with kubernetes_list /
// _get / _describe, even though it is currently a no-op for log content.
func TestInputSchemaValidation_LogsAcceptsOutputArg(t *testing.T) {
	srv := mcpserver.NewMCPServer("test", "0.0.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithInputSchemaValidation(),
	)

	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	require.NoError(t, RegisterPodTools(srv, sc))

	resp := callTool(t, srv, "kubernetes_logs", map[string]any{
		"namespace": "default",
		"podName":   "test",
		"output":    "slim",
	})

	jr, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok, "expected JSON-RPC response, got %T", resp)
	result, ok := jr.Result.(*mcp.CallToolResult)
	require.True(t, ok, "expected *mcp.CallToolResult, got %T", jr.Result)
	if result.IsError {
		tc, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		require.NotContains(t, tc.Text, "additionalProperties",
			"output arg must not be rejected as an unknown property; got: %s", tc.Text)
		require.NotContains(t, tc.Text, "&{[output]}",
			"raw additionalProperties reject value must never reach the caller; got: %s", tc.Text)
	}
}

func callTool(t *testing.T, srv *mcpserver.MCPServer, toolName string, args map[string]any) mcp.JSONRPCMessage {
	t.Helper()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return srv.HandleMessage(context.Background(), raw)
}
