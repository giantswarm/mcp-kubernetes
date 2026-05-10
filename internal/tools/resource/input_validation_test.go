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

// TestInputSchemaValidation_OutputArgAcceptedOnSiblingTools pins the fix for
// giantswarm/mcp-kubernetes#409: workflow authors mix `output: slim` into
// kubernetes_get and kubernetes_describe calls because kubernetes_list takes
// the same argument. Schema validation must accept those calls so the LLM
// agent doesn't see an opaque `&{[output]}` rejection that flips an entire
// workflow to isError=true.
func TestInputSchemaValidation_OutputArgAcceptedOnSiblingTools(t *testing.T) {
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

	tests := []struct {
		name     string
		toolName string
		args     map[string]any
	}{
		{
			name:     "kubernetes_get accepts output=slim",
			toolName: "kubernetes_get",
			args: map[string]any{
				"resourceType": "configmap",
				"name":         "test",
				"output":       "slim",
			},
		},
		{
			name:     "kubernetes_get accepts output=normal",
			toolName: "kubernetes_get",
			args: map[string]any{
				"resourceType": "configmap",
				"name":         "test",
				"output":       "normal",
			},
		},
		{
			name:     "kubernetes_get accepts output=wide",
			toolName: "kubernetes_get",
			args: map[string]any{
				"resourceType": "configmap",
				"name":         "test",
				"output":       "wide",
			},
		},
		{
			name:     "kubernetes_describe accepts output=slim",
			toolName: "kubernetes_describe",
			args: map[string]any{
				"resourceType": "configmap",
				"name":         "test",
				"output":       "slim",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := callTool(t, srv, tt.toolName, tt.args)
			jr, ok := resp.(mcp.JSONRPCResponse)
			require.True(t, ok, "expected JSON-RPC response, got %T", resp)
			result, ok := jr.Result.(*mcp.CallToolResult)
			require.True(t, ok, "expected *mcp.CallToolResult, got %T", jr.Result)
			// Validation must not reject the call. The handler may still produce
			// an error against the mock client (e.g. nil resource), but it must
			// not be an unknown-property rejection.
			if result.IsError {
				tc, ok := result.Content[0].(mcp.TextContent)
				require.True(t, ok, "expected TextContent, got %T", result.Content[0])
				require.NotContains(t, tc.Text, "additionalProperties",
					"output arg must not be rejected as an unknown property; got: %s", tc.Text)
				require.NotContains(t, tc.Text, "&{[output]}",
					"raw additionalProperties reject value must never reach the caller; got: %s", tc.Text)
			}
		})
	}
}

// TestInputSchemaValidation_OutputArgRejectsBogusValue keeps the enum
// constraint honest: an unknown enum value must still be rejected so callers
// get a clear, model-correctable error.
func TestInputSchemaValidation_OutputArgRejectsBogusValue(t *testing.T) {
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

	resp := callTool(t, srv, "kubernetes_get", map[string]any{
		"resourceType": "configmap",
		"name":         "test",
		"output":       "bogus",
	})

	jr, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok)
	result, ok := jr.Result.(*mcp.CallToolResult)
	require.True(t, ok)
	require.True(t, result.IsError, "bogus enum value should be rejected")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "output",
		"validation error should mention the offending property; got: %s", tc.Text)
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
