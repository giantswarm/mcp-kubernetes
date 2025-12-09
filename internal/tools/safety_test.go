// Package tools provides tests for shared tool utilities.
package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// TestCheckMutatingOperation_BlockedInNonDestructiveMode verifies that mutating
// operations are blocked when non-destructive mode is enabled and dry-run is disabled.
func TestCheckMutatingOperation_BlockedInNonDestructiveMode(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	// Test various mutating operations
	operations := []string{"create", "apply", "delete", "patch", "scale", "exec", "port-forward"}
	for _, op := range operations {
		t.Run(op+" is blocked", func(t *testing.T) {
			result := CheckMutatingOperation(sc, op)
			assert.NotNil(t, result, "%s should be blocked in non-destructive mode", op)
			assert.True(t, result.IsError)
		})
	}
}

// TestCheckMutatingOperation_AllowedWithDryRun verifies that mutating operations
// are allowed when dry-run mode is enabled.
func TestCheckMutatingOperation_AllowedWithDryRun(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(true),
	)
	require.NoError(t, err)

	// Test various mutating operations
	operations := []string{"create", "apply", "delete", "patch", "scale", "exec", "port-forward"}
	for _, op := range operations {
		t.Run(op+" is allowed with dry-run", func(t *testing.T) {
			result := CheckMutatingOperation(sc, op)
			assert.Nil(t, result, "%s should be allowed when dry-run is enabled", op)
		})
	}
}

// TestCheckMutatingOperation_AllowedWhenNonDestructiveDisabled verifies that
// operations are allowed when non-destructive mode is disabled.
func TestCheckMutatingOperation_AllowedWhenNonDestructiveDisabled(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(false),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	// Test various mutating operations
	operations := []string{"create", "apply", "delete", "patch", "scale", "exec", "port-forward"}
	for _, op := range operations {
		t.Run(op+" is allowed when non-destructive disabled", func(t *testing.T) {
			result := CheckMutatingOperation(sc, op)
			assert.Nil(t, result, "%s should be allowed when non-destructive mode is disabled", op)
		})
	}
}

// TestCheckMutatingOperation_AllowedOperationsWhitelist verifies that operations
// in the AllowedOperations list are permitted even in non-destructive mode.
func TestCheckMutatingOperation_AllowedOperationsWhitelist(t *testing.T) {
	ctx := context.Background()

	customConfig := server.NewDefaultConfig()
	customConfig.NonDestructiveMode = true
	customConfig.DryRun = false
	customConfig.AllowedOperations = []string{"get", "list", "describe", "create", "exec"}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithConfig(customConfig),
	)
	require.NoError(t, err)

	// Allowed operations should pass
	t.Run("create is allowed when in whitelist", func(t *testing.T) {
		result := CheckMutatingOperation(sc, "create")
		assert.Nil(t, result)
	})

	t.Run("exec is allowed when in whitelist", func(t *testing.T) {
		result := CheckMutatingOperation(sc, "exec")
		assert.Nil(t, result)
	})

	// Non-whitelisted operations should be blocked
	t.Run("delete is blocked when not in whitelist", func(t *testing.T) {
		result := CheckMutatingOperation(sc, "delete")
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("port-forward is blocked when not in whitelist", func(t *testing.T) {
		result := CheckMutatingOperation(sc, "port-forward")
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

// TestCheckMutatingOperation_ReadOperationsAlwaysAllowed verifies that read
// operations in the default AllowedOperations are always allowed.
func TestCheckMutatingOperation_ReadOperationsAlwaysAllowed(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	// Default allowed operations should pass
	readOps := []string{"get", "list", "describe"}
	for _, op := range readOps {
		t.Run(op+" is allowed by default", func(t *testing.T) {
			result := CheckMutatingOperation(sc, op)
			assert.Nil(t, result, "%s should be allowed by default", op)
		})
	}
}

// TestCheckMutatingOperation_ErrorMessageFormat verifies the error message format.
func TestCheckMutatingOperation_ErrorMessageFormat(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	result := CheckMutatingOperation(sc, "delete")
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)

	textContent, ok := result.Content[0].(interface{ Text() string })
	if ok {
		text := textContent.Text()
		assert.Contains(t, text, "Delete")
		assert.Contains(t, text, "non-destructive mode")
		assert.Contains(t, text, "--dry-run")
	}
}
