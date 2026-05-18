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
		"get",
		"list",
		"describe",
	}
	mutatingResourceTools = []string{
		"create",
		"apply",
		"delete",
		"patch",
		"scale",
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

func TestRegisterResourceTools_RegistersDeprecatedAliases(t *testing.T) {
	tools := registerResourceToolsWith(t,
		server.WithNonDestructiveMode(false),
	)

	for _, primary := range append(readOnlyResourceTools, mutatingResourceTools...) {
		alias := "kubernetes_" + primary
		entry, ok := tools[alias]
		require.True(t, ok, "deprecated alias %q should be registered alongside %q", alias, primary)
		assert.Contains(t, entry.Tool.Description, "[DEPRECATED]",
			"alias %q description should advertise its deprecation", alias)
	}
}

func TestRegisterResourceTools_AliasesGatedWithPrimaries(t *testing.T) {
	tools := registerResourceToolsWith(t,
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)

	for _, primary := range mutatingResourceTools {
		alias := "kubernetes_" + primary
		assert.NotContains(t, tools, alias,
			"alias %q should be hidden in non-destructive mode along with its primary", alias)
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
	assert.Contains(t, tools, "create", "create should be registered when 'create' is whitelisted")

	for _, name := range []string{"apply", "delete", "patch", "scale"} {
		assert.NotContains(t, tools, name, "tool %q should be hidden when not whitelisted", name)
	}
}
