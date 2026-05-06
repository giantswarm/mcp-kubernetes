package resource

import (
	"context"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

var (
	readOnlyResourceTools = []string{
		"kubernetes_get",
		"kubernetes_list",
		"kubernetes_describe",
	}
	mutatingResourceTools = []string{
		"kubernetes_create",
		"kubernetes_apply",
		"kubernetes_delete",
		"kubernetes_patch",
		"kubernetes_scale",
	}
)

// registerResourceToolsWith builds a fresh ServerContext with the given options
// and returns the tools registered by RegisterResourceTools, keyed by tool name.
func registerResourceToolsWith(t *testing.T, opts ...server.Option) map[string]*mcpserver.ServerTool {
	t.Helper()

	baseOpts := []server.Option{
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	}
	sc, err := server.NewServerContext(context.Background(), append(baseOpts, opts...)...)
	require.NoError(t, err)

	mcpSrv := mcpserver.NewMCPServer("test", "0.0.1",
		mcpserver.WithToolCapabilities(true),
	)
	require.NoError(t, RegisterResourceTools(mcpSrv, sc))

	return mcpSrv.ListTools()
}

func TestRegisterResourceTools_NonDestructiveMode_HidesMutating(t *testing.T) {
	tools := registerResourceToolsWith(t,
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)

	for _, name := range readOnlyResourceTools {
		assert.Contains(t, tools, name, "read-only tool %q should be registered", name)
	}

	for _, name := range mutatingResourceTools {
		assert.NotContains(t, tools, name, "mutating tool %q should be hidden in non-destructive mode", name)
	}
}

func TestRegisterResourceTools_DestructiveMode_RegistersAll(t *testing.T) {
	tools := registerResourceToolsWith(t,
		server.WithNonDestructiveMode(false),
	)

	for _, name := range append(readOnlyResourceTools, mutatingResourceTools...) {
		assert.Contains(t, tools, name, "tool %q should be registered when non-destructive is off", name)
	}
}

func TestRegisterResourceTools_DryRun_RegistersAll(t *testing.T) {
	tools := registerResourceToolsWith(t,
		server.WithNonDestructiveMode(true),
		server.WithDryRun(true),
	)

	for _, name := range append(readOnlyResourceTools, mutatingResourceTools...) {
		assert.Contains(t, tools, name, "tool %q should be registered with dry-run", name)
	}
}

func TestRegisterResourceTools_Whitelist_RegistersWhitelisted(t *testing.T) {
	customConfig := server.NewDefaultConfig()
	customConfig.NonDestructiveMode = true
	customConfig.DryRun = false
	customConfig.AllowedOperations = []string{"get", "list", "describe", "create"}

	tools := registerResourceToolsWith(t,
		server.WithConfig(customConfig),
	)

	for _, name := range readOnlyResourceTools {
		assert.Contains(t, tools, name, "read-only tool %q should be registered", name)
	}
	assert.Contains(t, tools, "kubernetes_create", "kubernetes_create should be registered when 'create' is whitelisted")

	for _, name := range []string{"kubernetes_apply", "kubernetes_delete", "kubernetes_patch", "kubernetes_scale"} {
		assert.NotContains(t, tools, name, "tool %q should be hidden when not whitelisted", name)
	}
}
