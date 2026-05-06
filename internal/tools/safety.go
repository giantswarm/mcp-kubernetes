// Package tools provides shared utilities for MCP tool handlers.
package tools

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// CheckMutatingOperation verifies if a mutating operation is allowed given the current
// server configuration. Returns an error result if blocked, nil if allowed.
//
// This centralizes the non-destructive mode check to avoid code duplication across
// all tool handlers that perform mutating operations.
//
// Operations are allowed if:
//   - NonDestructiveMode is disabled, OR
//   - DryRun mode is enabled (operations will be validated but not applied), OR
//   - The operation is explicitly listed in AllowedOperations
//
// Protected operations include: create, apply, delete, patch, scale, exec, port-forward
func CheckMutatingOperation(sc *server.ServerContext, operation string) *mcp.CallToolResult {
	if IsMutatingOperationAllowed(sc, operation) {
		return nil
	}

	return mcp.NewToolResultError(fmt.Sprintf(
		"%s operations are not allowed in non-destructive mode (use --dry-run to validate without applying)",
		cases.Title(language.English).String(operation),
	))
}

// IsMutatingOperationAllowed reports whether the given operation verb would be
// permitted under the current server configuration. The rules are:
//   - NonDestructiveMode is disabled, OR
//   - DryRun mode is enabled, OR
//   - The operation is listed in AllowedOperations.
//
// In particular, AllowedOperations is honored even when NonDestructiveMode=true
// and DryRun=false: the whitelist is the explicit escape hatch for selectively
// enabling specific verbs without flipping the global mode.
//
// This predicate is used at two sites and a change to either rule affects both:
//   - At tool-registration time, to skip exposing destructive tools that
//     cannot be invoked under the current configuration.
//   - At handler invocation time, via CheckMutatingOperation, to enforce the
//     same rule at the call site as a defence-in-depth check.
func IsMutatingOperationAllowed(sc *server.ServerContext, operation string) bool {
	config := sc.Config()
	if !config.NonDestructiveMode || config.DryRun {
		return true
	}

	for _, op := range config.AllowedOperations {
		if op == operation {
			return true
		}
	}

	return false
}
