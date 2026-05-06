package resource

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

// TestInputSchemaValidation_RejectsUnknownProperty pins the literal motivating
// scenario from giantswarm/giantswarm#36458: a caller that sends `cursor`
// instead of `continue` to kubernetes_list must receive a tool execution error
// referencing the unknown property, so the model can self-correct.
//
// This is an end-to-end check that:
//  1. WithInputSchemaValidation is wired into the server, and
//  2. WithSchemaAdditionalProperties(false) is set on the kubernetes_list
//     schema so unknown properties are rejected.
func TestInputSchemaValidation_RejectsUnknownProperty(t *testing.T) {
	srv := mcpserver.NewMCPServer("test", "0.0.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithInputSchemaValidation(),
	)

	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	require.NoError(t, RegisterResourceTools(srv, sc))

	resp := callTool(t, srv, "kubernetes_list", map[string]any{
		"resourceType": "pods",
		"cursor":       "some-token",
	})

	jr, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok, "expected JSON-RPC response, got %T", resp)
	result, ok := jr.Result.(*mcp.CallToolResult)
	require.True(t, ok, "expected *mcp.CallToolResult, got %T", jr.Result)
	require.True(t, result.IsError, "expected validation to mark result as error")
	require.NotEmpty(t, result.Content)

	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	require.Contains(t, tc.Text, "cursor",
		"validation error should mention the offending property; got: %s", tc.Text)
}

// TestInputSchemaValidation_AcceptsKnownProperty makes sure a well-formed call
// passes validation and reaches the handler — so the test above is meaningful
// (i.e. it isn't a side effect of an unrelated server-level rejection).
func TestInputSchemaValidation_AcceptsKnownProperty(t *testing.T) {
	srv := mcpserver.NewMCPServer("test", "0.0.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithInputSchemaValidation(),
	)

	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	require.NoError(t, RegisterResourceTools(srv, sc))

	resp := callTool(t, srv, "kubernetes_list", map[string]any{
		"resourceType": "pods",
		"continue":     "some-token",
	})

	jr, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok)
	result, ok := jr.Result.(*mcp.CallToolResult)
	require.True(t, ok)
	require.False(t, result.IsError, "well-formed call should not be rejected")
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
