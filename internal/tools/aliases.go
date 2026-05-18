package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// deprecatedAliasNames is the inverted view of deprecatedAliasFor: a set of
// every alias name. Built once at init() so IsDeprecatedAlias is a cheap map
// lookup.
var deprecatedAliasNames = func() map[string]struct{} {
	out := make(map[string]struct{}, len(deprecatedAliasFor))
	for _, alias := range deprecatedAliasFor {
		out[alias] = struct{}{}
	}
	return out
}()

// IsDeprecatedAlias reports whether name is a deprecated backward-compat alias
// registered by MaybeAddDeprecatedAlias.
func IsDeprecatedAlias(name string) bool {
	_, ok := deprecatedAliasNames[name]
	return ok
}

// HideDeprecatedAliasesFilter is a mcpserver.ToolFilterFunc that strips
// deprecated aliases out of the tools/list response. The aliases remain
// fully invokable via tools/call (the mcp-go filter chain only runs on list),
// so existing clients calling kubernetes_<name> keep working, but new
// clients can't discover the deprecated names. Pair this with
// MaybeAddDeprecatedAlias on the registration side.
func HideDeprecatedAliasesFilter(_ context.Context, tools []mcp.Tool) []mcp.Tool {
	out := tools[:0]
	for _, t := range tools {
		if IsDeprecatedAlias(t.Name) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// deprecatedAliasFor maps a current (bare) tool name to the deprecated
// `kubernetes_`-prefixed name kept as a backward-compatibility alias.
// Entries here are registered alongside their primaries by
// MaybeAddDeprecatedAlias. Remove an entry to retire its alias.
var deprecatedAliasFor = map[string]string{
	"get":                 "kubernetes_get",
	"list":                "kubernetes_list",
	"describe":            "kubernetes_describe",
	"create":              "kubernetes_create",
	"apply":               "kubernetes_apply",
	"delete":              "kubernetes_delete",
	"patch":               "kubernetes_patch",
	"scale":               "kubernetes_scale",
	"logs":                "kubernetes_logs",
	"exec":                "kubernetes_exec",
	"api_resources":       "kubernetes_api_resources",
	"cluster_health":      "kubernetes_cluster_health",
	"context_list":        "kubernetes_context_list",
	"context_get_current": "kubernetes_context_get_current",
	"context_use":         "kubernetes_context_use",
}

// MaybeAddDeprecatedAlias registers a deprecated alias for primaryName if one
// is mapped in deprecatedAliasFor. The alias shares the same input schema and
// handler; only the name and description differ. Its description is replaced
// with a [DEPRECATED] banner pointing to primaryName. Audit logs record the
// alias name actually invoked, so operators can track residual usage.
func MaybeAddDeprecatedAlias(
	s *mcpserver.MCPServer,
	sc *server.ServerContext,
	primaryName string,
	handler ToolHandler,
	opts ...mcp.ToolOption,
) {
	aliasName, ok := deprecatedAliasFor[primaryName]
	if !ok {
		return
	}
	aliasOpts := append([]mcp.ToolOption(nil), opts...)
	aliasOpts = append(aliasOpts, mcp.WithDescription(
		fmt.Sprintf("[DEPRECATED] Use `%s` instead. Backward-compat alias for the previous `%s` name; will be removed in a future release.", primaryName, aliasName),
	))
	s.AddTool(mcp.NewTool(aliasName, aliasOpts...), WrapWithAuditLogging(aliasName, handler, sc))
}
