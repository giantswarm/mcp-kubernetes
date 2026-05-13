package pod

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
	portForwardTools = []string{
		"port_forward",
		"list_port_forward_sessions",
		"stop_port_forward_session",
		"stop_all_port_forward_sessions",
	}
	// readOnlyPodTools are pod tools that are always registered regardless of
	// non-destructive mode.
	readOnlyPodTools = []string{
		"logs",
	}
	// mutatingPodTools are pod tools gated by IsMutatingOperationAllowed —
	// registered only when non-destructive mode is off, dry-run is on, or the
	// operation is whitelisted.
	mutatingPodTools = []string{
		"exec",
	}
)

// registerPodToolsWith builds a fresh ServerContext with the given options and
// returns the tools registered by RegisterPodTools, keyed by tool name.
func registerPodToolsWith(t *testing.T, opts ...server.Option) map[string]*mcpserver.ServerTool {
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
	require.NoError(t, RegisterPodTools(mcpSrv, sc))

	return mcpSrv.ListTools()
}

func TestRegisterPodTools_LocalMode(t *testing.T) {
	// Disable non-destructive mode so this test focuses on in-cluster gating only.
	tools := registerPodToolsWith(t, server.WithNonDestructiveMode(false))

	for _, name := range append(readOnlyPodTools, mutatingPodTools...) {
		assert.Contains(t, tools, name, "tool %s should be registered in local mode", name)
	}

	for _, name := range portForwardTools {
		assert.Contains(t, tools, name, "tool %s should be registered in local mode", name)
	}
}

func TestRegisterPodTools_InClusterMode(t *testing.T) {
	// Disable non-destructive mode so this test focuses on in-cluster gating only.
	tools := registerPodToolsWith(t,
		server.WithInCluster(true),
		server.WithNonDestructiveMode(false),
	)

	for _, name := range append(readOnlyPodTools, mutatingPodTools...) {
		assert.Contains(t, tools, name, "tool %s should still be registered in in-cluster mode", name)
	}

	for _, name := range portForwardTools {
		assert.NotContains(t, tools, name, "tool %s should NOT be registered in in-cluster mode", name)
	}
}

func TestRegisterPodTools_NonDestructiveMode_HidesMutating(t *testing.T) {
	// Default config has NonDestructiveMode=true.
	tools := registerPodToolsWith(t)

	for _, name := range readOnlyPodTools {
		assert.Contains(t, tools, name, "read-only tool %q should be registered", name)
	}
	for _, name := range mutatingPodTools {
		assert.NotContains(t, tools, name, "mutating tool %q should be hidden in non-destructive mode", name)
	}
	// Port-forward family is hidden together when port_forward itself isn't permitted.
	for _, name := range portForwardTools {
		assert.NotContains(t, tools, name, "port-forward tool %q should be hidden in non-destructive mode", name)
	}
}

func TestRegisterPodTools_DryRun_RegistersAll(t *testing.T) {
	tools := registerPodToolsWith(t,
		server.WithNonDestructiveMode(true),
		server.WithDryRun(true),
	)

	for _, name := range append(readOnlyPodTools, mutatingPodTools...) {
		assert.Contains(t, tools, name, "tool %s should be registered with dry-run", name)
	}
	for _, name := range portForwardTools {
		assert.Contains(t, tools, name, "tool %s should be registered with dry-run", name)
	}
}

func TestRegisterPodTools_Whitelist_RegistersWhitelisted(t *testing.T) {
	customConfig := server.NewDefaultConfig()
	customConfig.NonDestructiveMode = true
	customConfig.DryRun = false
	customConfig.AllowedOperations = []string{"get", "list", "describe", "exec"}

	tools := registerPodToolsWith(t, server.WithConfig(customConfig))

	for _, name := range readOnlyPodTools {
		assert.Contains(t, tools, name)
	}
	assert.Contains(t, tools, "exec", "exec should be registered when whitelisted")

	// port-forward is not whitelisted, so the whole port-forward family stays hidden.
	for _, name := range portForwardTools {
		assert.NotContains(t, tools, name, "port-forward tool %q should be hidden when not whitelisted", name)
	}
}
