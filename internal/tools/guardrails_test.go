package tools_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The facts below fix agent mistakes seen in practice; each message says
// which. Reword a description freely, but keep the fact, or the mistake
// comes back. Before cutting any other sentence, find its pull request with
// git log -S'<sentence>' and read why it is there.

func toolByName(t *testing.T, all []mcp.Tool, name string) mcp.Tool {
	t.Helper()
	for _, tool := range all {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool %s", name)
	return mcp.Tool{}
}

func paramDescription(t *testing.T, tool mcp.Tool, param string) string {
	t.Helper()
	prop, ok := tool.InputSchema.Properties[param].(map[string]any)
	require.True(t, ok, "%s has no parameter %s", tool.Name, param)
	d, _ := prop["description"].(string)
	return d
}

// TestListKeepsItsOrderingFacts: agents reported healthy clusters as broken
// because a limited event list handed them stale events (#559).
func TestListKeepsItsOrderingFacts(t *testing.T) {
	d := toolByName(t, allTools(t), "list").Description
	for fact, why := range map[string]string{
		"newest first":        "limit on events must read as the most recent activity",
		"alphabetical prefix": "a limited list of any other type is not the newest or the worst",
		"unsorted API order":  "a continue token on events silently drops newest-first",
		"exact values":        "nothing selects by age: filter is equality-only, so agents must not try it",
	} {
		assert.Contains(t, d, fact, "list's description lost a fact from #559: %s", why)
	}
}

// TestNamespaceSaysClusterScopedIgnoreIt: agents spent 10+ calls trying
// namespaces for cluster-scoped resources such as nodes (#191).
func TestNamespaceSaysClusterScopedIgnoreIt(t *testing.T) {
	all := allTools(t)
	for _, name := range []string{"get", "describe", "delete", "patch", "list"} {
		d := paramDescription(t, toolByName(t, all, name), "namespace")
		assert.Contains(t, d, "default 'default'", "%s.namespace lost a fact from #191: the default namespace", name)
		assert.Contains(t, d, "Ignored for cluster-scoped resources", "%s.namespace lost a fact from #191: cluster-scoped resources take no namespace", name)
	}
}
