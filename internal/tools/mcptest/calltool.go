// Package mcptest provides shared helpers for tests that exercise MCP tool
// handlers via the in-process JSON-RPC entry point on
// github.com/mark3labs/mcp-go/server.MCPServer.
package mcptest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

// CallTool builds a minimal JSON-RPC tools/call envelope, hands it to the
// in-process MCP server, and returns the raw response. Tests typically type
// the result as mcp.JSONRPCResponse and inspect its CallToolResult.
//
// Using the JSON-RPC entry point (rather than calling handlers directly)
// keeps schema validation, error mapping, and the additionalProperties
// rejection path on the hot path of the test, which is exactly the surface
// area the input-validation tests are meant to pin.
func CallTool(t *testing.T, srv *mcpserver.MCPServer, toolName string, args map[string]any) mcp.JSONRPCMessage {
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
