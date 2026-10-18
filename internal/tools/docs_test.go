package tools_test

import (
	"context"
	"testing"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
	"github.com/giantswarm/mcp-toolkit/tooldesc/tooldesctest"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/capi"
	capitestdata "github.com/giantswarm/mcp-kubernetes/internal/tools/capi/testdata"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/cluster"
	contexttools "github.com/giantswarm/mcp-kubernetes/internal/tools/context"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/pod"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource"
	resourcetestdata "github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

const docsHeader = "# The tools\n\nGenerated from the registered tools (`make tools-doc`); through muster every tool is `x_<server>_<name>`. " +
	"This page lists every tool; a deployment serves a subset by its mode: the context tools and port forwarding only outside the cluster, " +
	"create, apply, delete, patch, scale, exec and port forwarding only where mutations are allowed, the CAPI tools only with federation. " +
	"Parameters vary the same way: `cluster` only with federation, `kubeContext` only outside the cluster."

// registeredTools registers every tool group as cmd/serve.go does, in the
// mode that registers the most (outside the cluster, mutations allowed,
// federation), deprecated aliases included.
func registeredTools(t *testing.T) []mcp.Tool {
	t.Helper()
	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(&resourcetestdata.MockK8sClient{}),
		server.WithLogger(&resourcetestdata.MockLogger{}),
		server.WithFederationManager(&capitestdata.MockFederationManager{}),
		server.WithNonDestructiveMode(false),
	)
	require.NoError(t, err)
	s := mcpserver.NewMCPServer("mcp-kubernetes", "test")
	require.NoError(t, resource.RegisterResourceTools(s, sc))
	require.NoError(t, pod.RegisterPodTools(s, sc))
	require.NoError(t, contexttools.RegisterContextTools(s, sc))
	require.NoError(t, cluster.RegisterClusterTools(s, sc))
	require.NoError(t, capi.RegisterCAPITools(s, sc))
	var all []mcp.Tool
	for _, st := range s.ListTools() {
		all = append(all, st.Tool)
	}
	return all
}

// allTools are the registered tools as clients list them: without the
// deprecated aliases.
func allTools(t *testing.T) []mcp.Tool {
	return tools.HideDeprecatedAliasesFilter(context.Background(), registeredTools(t))
}

// TestToolsDocIsCurrent keeps docs/tools.md — every tool's description and
// input schema as a model reads them — equal to the registered tools.
// TOOLS_DOC_UPDATE=1 rewrites the file (`make tools-doc`).
func TestToolsDocIsCurrent(t *testing.T) {
	tooldesctest.AssertDoc(t, allTools(t), docsHeader, "../../docs/tools.md")
}

// TestToolDescriptionBudgets holds the registered descriptions, the hidden
// aliases' included, to the budget of giantswarm/giantswarm#38592.
func TestToolDescriptionBudgets(t *testing.T) {
	require.NoError(t, tooldesc.Check(registeredTools(t), tooldesc.DefaultBudget))
}

// TestReadOnlyToolsAreNotDestructive: a read-only tool says so in full, or
// mcp-go's defaults report it destructive and not idempotent.
func TestReadOnlyToolsAreNotDestructive(t *testing.T) {
	for _, tool := range registeredTools(t) {
		a := tool.Annotations
		if a.ReadOnlyHint == nil || !*a.ReadOnlyHint {
			continue
		}
		require.Equal(t, mcp.ToBoolPtr(false), a.DestructiveHint, "%s is read-only but reported destructive", tool.Name)
		require.Equal(t, mcp.ToBoolPtr(true), a.IdempotentHint, "%s is read-only but not reported idempotent", tool.Name)
	}
}
