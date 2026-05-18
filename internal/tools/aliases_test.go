package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

func TestMaybeAddDeprecatedAlias_RegistersMappedAlias(t *testing.T) {
	sc := createTestServerContextNoInstrumentation(t)
	srv := mcpserver.NewMCPServer("test", "0.0.1", mcpserver.WithToolCapabilities(true))

	handler := func(ctx context.Context, req mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	}
	opts := []mcp.ToolOption{
		mcp.WithDescription("Original description"),
		mcp.WithString("name", mcp.Required(), mcp.Description("resource name")),
	}

	MaybeAddDeprecatedAlias(srv, sc, "get", handler, opts...)

	tools := srv.ListTools()
	alias, ok := tools["kubernetes_get"]
	require.True(t, ok, "alias kubernetes_get should be registered")

	assert.Contains(t, alias.Tool.Description, "[DEPRECATED]", "alias description should carry deprecation marker")
	assert.Contains(t, alias.Tool.Description, "`get`", "alias description should name its replacement")
	assert.NotContains(t, alias.Tool.Description, "Original description", "alias description should override the primary's, not append to it")

	// Schema (excluding description) carries through.
	props := alias.Tool.InputSchema.Properties
	require.NotNil(t, props, "input schema properties should be set on the alias")
	_, hasName := props["name"]
	assert.True(t, hasName, "alias should inherit the `name` parameter from the primary's opts")
}

func TestMaybeAddDeprecatedAlias_NoOpForUnmappedName(t *testing.T) {
	sc := createTestServerContextNoInstrumentation(t)
	srv := mcpserver.NewMCPServer("test", "0.0.1", mcpserver.WithToolCapabilities(true))

	handler := func(ctx context.Context, req mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	}

	MaybeAddDeprecatedAlias(srv, sc, "port_forward", handler, mcp.WithDescription("x"))

	tools := srv.ListTools()
	assert.Empty(t, tools, "no alias should be registered for an unmapped primary name")
}

func TestMaybeAddDeprecatedAlias_HandlerCalledViaAlias(t *testing.T) {
	sc := createTestServerContextNoInstrumentation(t)
	srv := mcpserver.NewMCPServer("test", "0.0.1", mcpserver.WithToolCapabilities(true))

	called := false
	handler := func(ctx context.Context, req mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("ok"), nil
	}

	MaybeAddDeprecatedAlias(srv, sc, "get", handler, mcp.WithDescription("desc"))

	tools := srv.ListTools()
	alias := tools["kubernetes_get"]
	require.NotNil(t, alias)

	_, err := alias.Handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	assert.True(t, called, "calling the alias should invoke the primary's handler")
}

func TestDeprecatedAliasMap_AllPrefixedConsistently(t *testing.T) {
	for primary, alias := range deprecatedAliasFor {
		assert.True(t, strings.HasPrefix(alias, "kubernetes_"),
			"alias %q for primary %q should start with kubernetes_", alias, primary)
		assert.Equal(t, "kubernetes_"+primary, alias,
			"alias %q should be exactly kubernetes_<primary>", alias)
	}
}
