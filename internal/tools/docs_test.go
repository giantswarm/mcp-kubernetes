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
	"exec and port forwarding only where mutations are allowed, the CAPI tools only with federation."

// allTools registers every tool group as cmd/serve.go does, in the mode that
// registers the most (outside the cluster, mutations allowed, federation),
// and lists them as clients see them: without the deprecated aliases.
func allTools(t *testing.T) []mcp.Tool {
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
	return tools.HideDeprecatedAliasesFilter(context.Background(), all)
}

// TestToolsDocIsCurrent keeps docs/tools.md — every tool's description and
// input schema as a model reads them — equal to the registered tools.
// TOOLS_DOC_UPDATE=1 rewrites the file (`make tools-doc`).
func TestToolsDocIsCurrent(t *testing.T) {
	tooldesctest.AssertDoc(t, allTools(t), docsHeader, "../../docs/tools.md")
}

// TestToolDescriptionBudgets holds the registered descriptions to the
// budget of giantswarm/giantswarm#38592.
func TestToolDescriptionBudgets(t *testing.T) {
	require.NoError(t, tooldesc.Check(allTools(t), tooldesc.DefaultBudget))
}
