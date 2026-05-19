package pod

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/mcptest"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// TestInputSchemaValidation_LogsAcceptsOutputArg pins the fix for
// giantswarm/mcp-kubernetes#409 on the pod side: the logs tool must accept
// the `output` argument for argument-shape symmetry with list / get /
// describe, even though it is currently a no-op for log content.
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

	resp := mcptest.CallTool(t, srv, "logs", map[string]any{
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
