// Package resource provides tests for resource handler functionality.
package resource

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// getErrorText safely extracts error text from an MCP result.
// Returns empty string if result is nil, has no content, or content is not TextContent.
func getErrorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent in result, got %T", result.Content[0])
	return textContent.Text
}

// TestNonDestructiveModeBlocksMutatingOperations verifies that non-destructive mode
// blocks all mutating operations (create, apply, delete, patch, scale) when dry-run is disabled.
func TestNonDestructiveModeBlocksMutatingOperations(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode enabled and dry-run disabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	tests := []struct {
		name      string
		operation string
		handler   func(context.Context, mcp.CallToolRequest, *server.ServerContext) (*mcp.CallToolResult, error)
		args      map[string]interface{}
		wantError string
	}{
		{
			name:      "create is blocked in non-destructive mode",
			operation: "create",
			handler:   handleCreateResource,
			args: map[string]interface{}{
				"namespace": "default",
				"manifest": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "test"},
				},
			},
			wantError: "Create operations are not allowed in non-destructive mode",
		},
		{
			name:      "apply is blocked in non-destructive mode",
			operation: "apply",
			handler:   handleApplyResource,
			args: map[string]interface{}{
				"namespace": "default",
				"manifest": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "test"},
				},
			},
			wantError: "Apply operations are not allowed in non-destructive mode",
		},
		{
			name:      "delete is blocked in non-destructive mode",
			operation: "delete",
			handler:   handleDeleteResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "configmap",
				"name":         "test",
			},
			wantError: "Delete operations are not allowed in non-destructive mode",
		},
		{
			name:      "patch is blocked in non-destructive mode",
			operation: "patch",
			handler:   handlePatchResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "configmap",
				"name":         "test",
				"patchType":    "merge",
				"patch":        map[string]interface{}{"data": map[string]interface{}{"key": "value"}},
			},
			wantError: "Patch operations are not allowed in non-destructive mode",
		},
		{
			name:      "scale is blocked in non-destructive mode",
			operation: "scale",
			handler:   handleScaleResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "deployment",
				"name":         "test",
				// JSON numbers unmarshal to float64, so we use float64 here to match
				"replicas": float64(3),
			},
			wantError: "Scale operations are not allowed in non-destructive mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = tt.args

			result, err := tt.handler(ctx, request, sc)
			require.NoError(t, err)
			assert.True(t, result.IsError, "expected error result")
			assert.Contains(t, getErrorText(t, result), tt.wantError)
		})
	}
}

// TestDryRunModeAllowsMutatingOperationsWithValidation verifies that dry-run mode
// allows mutating operations to proceed (for API validation) even when non-destructive mode is enabled.
func TestDryRunModeAllowsMutatingOperationsWithValidation(t *testing.T) {
	ctx := context.Background()

	// Create server context with both non-destructive mode AND dry-run enabled
	// This should allow operations to proceed (they'll be validated but not applied)
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(true),
	)
	require.NoError(t, err)

	tests := []struct {
		name      string
		operation string
		handler   func(context.Context, mcp.CallToolRequest, *server.ServerContext) (*mcp.CallToolResult, error)
		args      map[string]interface{}
	}{
		{
			name:      "create is allowed with dry-run",
			operation: "create",
			handler:   handleCreateResource,
			args: map[string]interface{}{
				"namespace": "default",
				"manifest": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "test"},
				},
			},
		},
		{
			name:      "apply is allowed with dry-run",
			operation: "apply",
			handler:   handleApplyResource,
			args: map[string]interface{}{
				"namespace": "default",
				"manifest": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "test"},
				},
			},
		},
		{
			name:      "delete is allowed with dry-run",
			operation: "delete",
			handler:   handleDeleteResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "configmap",
				"name":         "test",
			},
		},
		{
			name:      "patch is allowed with dry-run",
			operation: "patch",
			handler:   handlePatchResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "configmap",
				"name":         "test",
				"patchType":    "merge",
				"patch":        map[string]interface{}{"data": map[string]interface{}{"key": "value"}},
			},
		},
		{
			name:      "scale is allowed with dry-run",
			operation: "scale",
			handler:   handleScaleResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "deployment",
				"name":         "test",
				// JSON numbers unmarshal to float64, so we use float64 here to match
				"replicas": float64(3),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = tt.args

			result, err := tt.handler(ctx, request, sc)
			require.NoError(t, err)
			// With dry-run enabled, the request should pass the non-destructive check
			// The actual k8s operation may fail (because our mock returns nil),
			// but the important thing is that we didn't get blocked by non-destructive mode
			if result.IsError {
				// Verify the error is NOT about non-destructive mode
				errorText := getErrorText(t, result)
				assert.NotContains(t, errorText, "not allowed in non-destructive mode",
					"dry-run mode should allow operation to proceed past non-destructive check")
			}
		})
	}
}

// TestNonDestructiveModeDisabledAllowsAllOperations verifies that when non-destructive
// mode is disabled, all operations are allowed regardless of dry-run setting.
func TestNonDestructiveModeDisabledAllowsAllOperations(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode disabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(false),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	tests := []struct {
		name      string
		operation string
		handler   func(context.Context, mcp.CallToolRequest, *server.ServerContext) (*mcp.CallToolResult, error)
		args      map[string]interface{}
	}{
		{
			name:      "create is allowed when non-destructive mode is disabled",
			operation: "create",
			handler:   handleCreateResource,
			args: map[string]interface{}{
				"namespace": "default",
				"manifest": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "test"},
				},
			},
		},
		{
			name:      "delete is allowed when non-destructive mode is disabled",
			operation: "delete",
			handler:   handleDeleteResource,
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "configmap",
				"name":         "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = tt.args

			result, err := tt.handler(ctx, request, sc)
			require.NoError(t, err)
			// The request should NOT be blocked by non-destructive mode
			if result.IsError {
				errorText := getErrorText(t, result)
				assert.NotContains(t, errorText, "not allowed in non-destructive mode",
					"non-destructive mode is disabled, should not block operation")
			}
		})
	}
}

// TestAllowedOperationsExplicitlyAllowsOperations verifies that operations can be
// explicitly allowed via AllowedOperations even in non-destructive mode.
func TestAllowedOperationsExplicitlyAllowsOperations(t *testing.T) {
	ctx := context.Background()

	// Create a custom config that allows create operations
	customConfig := server.NewDefaultConfig()
	customConfig.NonDestructiveMode = true
	customConfig.DryRun = false
	customConfig.AllowedOperations = []string{"get", "list", "describe", "create"} // Explicitly allow create

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithConfig(customConfig),
	)
	require.NoError(t, err)

	t.Run("create is allowed when explicitly in AllowedOperations", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace": "default",
			"manifest": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "test"},
			},
		}

		result, err := handleCreateResource(ctx, request, sc)
		require.NoError(t, err)
		// Should NOT be blocked by non-destructive mode because create is in AllowedOperations
		if result.IsError {
			errorText := getErrorText(t, result)
			assert.NotContains(t, errorText, "Create operations are not allowed in non-destructive mode",
				"create should be allowed when explicitly in AllowedOperations")
		}
	})

	t.Run("delete is still blocked when not in AllowedOperations", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace":    "default",
			"resourceType": "configmap",
			"name":         "test",
		}

		result, err := handleDeleteResource(ctx, request, sc)
		require.NoError(t, err)
		assert.True(t, result.IsError, "delete should be blocked")
		assert.Contains(t, getErrorText(t, result), "Delete operations are not allowed in non-destructive mode")
	})
}

// TestReadOperationsAlwaysAllowed verifies that read operations (get, list, describe)
// are always allowed regardless of mode settings.
func TestReadOperationsAlwaysAllowed(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode enabled and dry-run disabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	t.Run("get is always allowed", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace":    "default",
			"resourceType": "configmap",
			"name":         "test",
		}

		result, err := handleGetResource(ctx, request, sc)
		require.NoError(t, err)
		// Get should not be blocked by non-destructive mode
		if result.IsError {
			errorText := getErrorText(t, result)
			assert.NotContains(t, errorText, "non-destructive mode",
				"get should always be allowed")
		}
	})

	t.Run("list is always allowed", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace":    "default",
			"resourceType": "configmap",
		}

		result, err := handleListResources(ctx, request, sc)
		require.NoError(t, err)
		// List should not be blocked by non-destructive mode
		if result.IsError {
			errorText := getErrorText(t, result)
			assert.NotContains(t, errorText, "non-destructive mode",
				"list should always be allowed")
		}
	})

	t.Run("describe is always allowed", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace":    "default",
			"resourceType": "configmap",
			"name":         "test",
		}

		result, err := handleDescribeResource(ctx, request, sc)
		require.NoError(t, err)
		// Describe should not be blocked by non-destructive mode
		if result.IsError {
			errorText := getErrorText(t, result)
			assert.NotContains(t, errorText, "non-destructive mode",
				"describe should always be allowed")
		}
	})
}

// TestDefaultConfigNonDestructiveModeEnabled verifies that the default configuration
// has non-destructive mode enabled (security by default).
func TestDefaultConfigNonDestructiveModeEnabled(t *testing.T) {
	config := server.NewDefaultConfig()
	assert.True(t, config.NonDestructiveMode, "non-destructive mode should be enabled by default")
	assert.False(t, config.DryRun, "dry-run should be disabled by default")
	assert.Contains(t, config.AllowedOperations, "get", "get should be in default allowed operations")
	assert.Contains(t, config.AllowedOperations, "list", "list should be in default allowed operations")
	assert.Contains(t, config.AllowedOperations, "describe", "describe should be in default allowed operations")
	assert.NotContains(t, config.AllowedOperations, "create", "create should NOT be in default allowed operations")
	assert.NotContains(t, config.AllowedOperations, "delete", "delete should NOT be in default allowed operations")
}

// TestErrorMessagesIncludeDryRunHint verifies that error messages for blocked operations
// include a hint about using dry-run mode.
func TestErrorMessagesIncludeDryRunHint(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace": "default",
		"manifest": map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	}

	result, err := handleCreateResource(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)

	errorText := getErrorText(t, result)
	assert.Contains(t, errorText, "--dry-run",
		"error message should include hint about dry-run option")
}

// TestDefaultNamespaceUsedWhenNotProvided verifies that when no namespace is provided,
// the handler defaults to "default" namespace following kubectl behavior.
// This works for all resources - the K8s API handles cluster-scoped resources correctly.
func TestDefaultNamespaceUsedWhenNotProvided(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	// All these resources should work without explicit namespace
	// - For cluster-scoped resources, the K8s API ignores the namespace
	// - For namespaced resources, it uses "default" namespace
	tests := []struct {
		resourceType string
		description  string
	}{
		// Cluster-scoped resources
		{"nodes", "nodes works without namespace"},
		{"namespaces", "namespaces works without namespace"},
		{"persistentvolumes", "persistentvolumes works without namespace"},
		{"clusterroles", "clusterroles works without namespace"},
		// Namespaced resources (will use "default" namespace)
		{"pods", "pods uses default namespace"},
		{"deployments", "deployments uses default namespace"},
		{"services", "services uses default namespace"},
		// CRDs/unknown resources
		{"clusters", "CRDs work without namespace"},
		{"helmreleases", "CRDs work without namespace"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"resourceType": tt.resourceType,
				// No namespace provided - should default to "default"
			}

			result, err := handleListResources(ctx, request, sc)
			require.NoError(t, err)
			// Should NOT get error about namespace being required
			// Any errors should be from K8s API (resource not found, etc.), not validation
			if result.IsError {
				errorText := getErrorText(t, result)
				assert.NotContains(t, errorText, "namespace is required",
					"resource %q should not require explicit namespace", tt.resourceType)
			}
		})
	}
}

// TestAllNamespacesOverridesDefault verifies that allNamespaces=true
// works correctly and doesn't use the default namespace.
func TestAllNamespacesOverridesDefault(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resourceType":  "pods",
		"allNamespaces": true,
		// No namespace provided
	}

	result, err := handleListResources(ctx, request, sc)
	require.NoError(t, err)
	// Should NOT get error about namespace being required
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "namespace is required",
			"allNamespaces=true should work without explicit namespace")
	}
}

// TestExplicitNamespaceUsed verifies that when a namespace is explicitly provided,
// it is used instead of the default.
func TestExplicitNamespaceUsed(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resourceType": "pods",
		"namespace":    "kube-system",
	}

	result, err := handleListResources(ctx, request, sc)
	require.NoError(t, err)
	// The mock client will return results - we're just verifying no validation error
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "namespace is required",
			"explicit namespace should be accepted")
	}
}

// TestGetResourceDefaultNamespace verifies that the get tool uses default namespace
// when no namespace is provided.
func TestGetResourceDefaultNamespace(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resourceType": "pods",
		"name":         "my-pod",
		// No namespace provided - should default to "default"
	}

	result, err := handleGetResource(ctx, request, sc)
	require.NoError(t, err)
	// Should NOT get error about namespace being required
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "namespace is required",
			"get should not require explicit namespace")
	}
}

// TestDescribeResourceDefaultNamespace verifies that the describe tool uses default namespace
// when no namespace is provided.
func TestDescribeResourceDefaultNamespace(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resourceType": "pods",
		"name":         "my-pod",
		// No namespace provided - should default to "default"
	}

	result, err := handleDescribeResource(ctx, request, sc)
	require.NoError(t, err)
	// Should NOT get error about namespace being required
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "namespace is required",
			"describe should not require explicit namespace")
	}
}

// TestDeleteResourceDefaultNamespace verifies that the delete tool uses default namespace
// when no namespace is provided.
func TestDeleteResourceDefaultNamespace(t *testing.T) {
	ctx := context.Background()

	// Enable dry-run to allow delete operation
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithDryRun(true),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resourceType": "pods",
		"name":         "my-pod",
		// No namespace provided - should default to "default"
	}

	result, err := handleDeleteResource(ctx, request, sc)
	require.NoError(t, err)
	// Should NOT get error about namespace being required
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "namespace is required",
			"delete should not require explicit namespace")
	}
}

// TestPatchResourceDefaultNamespace verifies that the patch tool uses default namespace
// when no namespace is provided.
func TestPatchResourceDefaultNamespace(t *testing.T) {
	ctx := context.Background()

	// Enable dry-run to allow patch operation
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithDryRun(true),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resourceType": "pods",
		"name":         "my-pod",
		"patchType":    "merge",
		"patch":        map[string]interface{}{"metadata": map[string]interface{}{"labels": map[string]interface{}{"test": "value"}}},
		// No namespace provided - should default to "default"
	}

	result, err := handlePatchResource(ctx, request, sc)
	require.NoError(t, err)
	// Should NOT get error about namespace being required
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "namespace is required",
			"patch should not require explicit namespace")
	}
}

// TestGetOutputProcessorForFormat verifies the contract documented on
// getOutputProcessorForFormat:
//
//   - slim (and empty/default): SlimOutput=true + KindShaping=true. The LLM-
//     friendly default per #410.
//   - normal: SlimOutput=true, KindShaping=false. Blacklist-only behaviour
//     for callers that want managedFields stripped but every other field
//     (including HelmRelease spec.values) intact.
//   - wide / full: SlimOutput=false, KindShaping=false. Full manifest, only
//     secret masking applied.
//
// Secret masking must always be on regardless of format.
func TestGetOutputProcessorForFormat(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	tests := []struct {
		name            string
		outputFormat    string
		wantSlim        bool
		wantKindShaping bool
	}{
		{name: "empty defaults to slim + kind shaping", outputFormat: "", wantSlim: true, wantKindShaping: true},
		{name: "slim enables both slim and kind shaping", outputFormat: "slim", wantSlim: true, wantKindShaping: true},
		{name: "normal disables kind shaping but keeps slim", outputFormat: "normal", wantSlim: true, wantKindShaping: false},
		{name: "wide disables both slim and kind shaping", outputFormat: "wide", wantSlim: false, wantKindShaping: false},
		{name: "full is an alias for wide", outputFormat: "full", wantSlim: false, wantKindShaping: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := getOutputProcessorForFormat(sc, tt.outputFormat)
			require.NotNil(t, processor)
			cfg := processor.Config()
			assert.Equal(t, tt.wantSlim, cfg.SlimOutput,
				"SlimOutput for output=%q", tt.outputFormat)
			assert.Equal(t, tt.wantKindShaping, cfg.KindShaping,
				"KindShaping for output=%q", tt.outputFormat)
			assert.True(t, cfg.MaskSecrets,
				"MaskSecrets must stay enabled for output=%q", tt.outputFormat)
		})
	}
}

// eventObj builds a minimal event list item carrying exactly one of the three
// timestamp fields, mirroring how the apiserver populates them (core/v1
// events set lastTimestamp/firstTimestamp; events.k8s.io/v1 set eventTime).
func eventObj(name, timestampField, timestamp string) runtime.Object {
	obj := map[string]any{
		"kind":       "Event",
		"apiVersion": "v1",
		"metadata":   map[string]any{"name": name, "namespace": "kube-system"},
		"reason":     "BackOff",
	}
	if timestampField != "" {
		obj[timestampField] = timestamp
	}
	return &unstructured.Unstructured{Object: obj}
}

func eventNames(items []runtime.Object) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		u, ok := it.(*unstructured.Unstructured)
		if !ok {
			names = append(names, "<not-unstructured>")
			continue
		}
		names = append(names, u.GetName())
	}
	return names
}

// TestSortItemsForResourceType_Events pins the newest-first ordering of event
// list results. The apiserver returns events in etcd key order (namespace/name),
// so without this sort a `limit` yields an arbitrary alphabetical slice: stale
// events crowd out current ones and a caller asking for "recent events" is
// silently handed old ones. Ordering must also agree with the `lastSeen` value
// summarizeResource reports, hence the shared timestamp precedence.
func TestSortItemsForResourceType_Events(t *testing.T) {
	t.Run("newest first regardless of input order", func(t *testing.T) {
		items := []runtime.Object{
			eventObj("old", "lastTimestamp", "2026-07-29T10:00:00Z"),
			eventObj("newest", "lastTimestamp", "2026-07-29T12:00:00Z"),
			eventObj("middle", "lastTimestamp", "2026-07-29T11:00:00Z"),
		}

		sortItemsForResourceType(items, "events")

		assert.Equal(t, []string{"newest", "middle", "old"}, eventNames(items))
	})

	t.Run("all accepted resourceType spellings sort", func(t *testing.T) {
		for _, resourceType := range []string{"events", "event", "ev", "Events", "EVENT"} {
			items := []runtime.Object{
				eventObj("old", "lastTimestamp", "2026-07-29T10:00:00Z"),
				eventObj("new", "lastTimestamp", "2026-07-29T12:00:00Z"),
			}
			sortItemsForResourceType(items, resourceType)
			assert.Equal(t, []string{"new", "old"}, eventNames(items),
				"resourceType %q must sort", resourceType)
		}
	})

	t.Run("each timestamp field is honoured", func(t *testing.T) {
		items := []runtime.Object{
			eventObj("first-ts", "firstTimestamp", "2026-07-29T10:00:00Z"),
			eventObj("event-ts", "eventTime", "2026-07-29T12:00:00.123456Z"),
			eventObj("last-ts", "lastTimestamp", "2026-07-29T11:00:00Z"),
		}

		sortItemsForResourceType(items, "events")

		assert.Equal(t, []string{"event-ts", "last-ts", "first-ts"}, eventNames(items))
	})

	t.Run("lastTimestamp wins over the other fields", func(t *testing.T) {
		// An event carrying every field must be ordered by lastTimestamp,
		// which is the one summarizeResource reports as lastSeen.
		multi := &unstructured.Unstructured{Object: map[string]any{
			"kind":           "Event",
			"metadata":       map[string]any{"name": "multi"},
			"firstTimestamp": "2026-07-29T09:00:00Z",
			"eventTime":      "2026-07-29T09:30:00Z",
			"lastTimestamp":  "2026-07-29T13:00:00Z",
		}}
		items := []runtime.Object{
			eventObj("noon", "lastTimestamp", "2026-07-29T12:00:00Z"),
			multi,
		}

		sortItemsForResourceType(items, "events")

		assert.Equal(t, []string{"multi", "noon"}, eventNames(items))
	})

	t.Run("timestampless and unparseable events sort last", func(t *testing.T) {
		items := []runtime.Object{
			eventObj("no-timestamp", "", ""),
			eventObj("garbage", "lastTimestamp", "not-a-timestamp"),
			eventObj("dated", "lastTimestamp", "2026-07-29T10:00:00Z"),
		}

		sortItemsForResourceType(items, "events")

		// The dated event must come first; the two without usable recency
		// keep their relative input order (the sort is stable).
		assert.Equal(t, []string{"dated", "no-timestamp", "garbage"}, eventNames(items))
	})

	t.Run("non-event resourceTypes keep API order", func(t *testing.T) {
		items := []runtime.Object{
			eventObj("b-pod", "lastTimestamp", "2026-07-29T10:00:00Z"),
			eventObj("a-pod", "lastTimestamp", "2026-07-29T12:00:00Z"),
		}

		sortItemsForResourceType(items, "pods")

		assert.Equal(t, []string{"b-pod", "a-pod"}, eventNames(items))
	})

	t.Run("empty and single-item slices are safe", func(t *testing.T) {
		assert.NotPanics(t, func() {
			sortItemsForResourceType(nil, "events")
			sortItemsForResourceType([]runtime.Object{}, "events")
			sortItemsForResourceType([]runtime.Object{
				eventObj("only", "lastTimestamp", "2026-07-29T10:00:00Z"),
			}, "events")
		})
	})

	t.Run("a non-unstructured item leaves the slice untouched", func(t *testing.T) {
		// Rather than reorder a partially-comparable slice, bail out: a
		// half-sorted result would be worse than the API's own order.
		items := []runtime.Object{
			eventObj("old", "lastTimestamp", "2026-07-29T10:00:00Z"),
			&corev1.Event{},
			eventObj("new", "lastTimestamp", "2026-07-29T12:00:00Z"),
		}

		sortItemsForResourceType(items, "events")

		assert.Equal(t, []string{"old", "<not-unstructured>", "new"}, eventNames(items))
	})
}

// TestUnstructuredEventTime_MatchesEffectiveEventTime pins the two event-time
// helpers to the same field precedence. summarizeResource reports `lastSeen`
// from the unstructured path while the describe handler sorts via the typed
// path; if they disagreed, list output would be ordered by one timestamp and
// labelled with another.
func TestUnstructuredEventTime_MatchesEffectiveEventTime(t *testing.T) {
	lastTS := metav1.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	firstTS := metav1.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	eventTS := metav1.NewMicroTime(time.Date(2026, 7, 29, 11, 0, 0, 0, time.UTC))

	tests := []struct {
		name  string
		typed corev1.Event
		obj   map[string]any
	}{
		{
			name:  "lastTimestamp preferred",
			typed: corev1.Event{LastTimestamp: lastTS, FirstTimestamp: firstTS, EventTime: eventTS},
			obj: map[string]any{
				"lastTimestamp":  lastTS.UTC().Format(time.RFC3339),
				"firstTimestamp": firstTS.UTC().Format(time.RFC3339),
				"eventTime":      eventTS.UTC().Format(time.RFC3339Nano),
			},
		},
		{
			name:  "eventTime when lastTimestamp absent",
			typed: corev1.Event{FirstTimestamp: firstTS, EventTime: eventTS},
			obj: map[string]any{
				"firstTimestamp": firstTS.UTC().Format(time.RFC3339),
				"eventTime":      eventTS.UTC().Format(time.RFC3339Nano),
			},
		},
		{
			name:  "firstTimestamp as last resort",
			typed: corev1.Event{FirstTimestamp: firstTS},
			obj:   map[string]any{"firstTimestamp": firstTS.UTC().Format(time.RFC3339)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unstructuredEventTime(&unstructured.Unstructured{Object: tt.obj})
			assert.True(t, got.Equal(effectiveEventTime(tt.typed)),
				"unstructured %v must match typed %v", got, effectiveEventTime(tt.typed))
		})
	}

	t.Run("no timestamp yields the zero time", func(t *testing.T) {
		got := unstructuredEventTime(&unstructured.Unstructured{Object: map[string]any{}})
		assert.True(t, got.IsZero())
	})
}

// TestLocallyOrderedResourceType pins which Kinds get the widened scan. The
// whole point of the wider fetch is that a newest-first sort is meaningless
// if the API server already truncated the result to the caller's page in its
// own key order, so this decision has to be right for events and inert for
// everything else.
func TestLocallyOrderedResourceType(t *testing.T) {
	t.Run("events are ordered locally", func(t *testing.T) {
		for _, resourceType := range []string{"events", "event", "ev", "Events", "EVENT"} {
			assert.True(t, locallyOrderedResourceType(resourceType, ""),
				"resourceType %q must use the widened scan", resourceType)
		}
	})

	t.Run("other Kinds keep server-side paging", func(t *testing.T) {
		for _, resourceType := range []string{"pods", "deployments", "nodes", "clusters"} {
			assert.False(t, locallyOrderedResourceType(resourceType, ""),
				"resourceType %q must not widen its scan", resourceType)
		}
	})

	t.Run("an explicit continue token opts out", func(t *testing.T) {
		// A continue token is a position in the API server's key-order
		// pagination. Re-ordering locally would make the caller's next page
		// meaningless, so honour the server's paging instead.
		assert.False(t, locallyOrderedResourceType("events", "eyJ2IjoibWV0YS5rOHMuaW8vdjEi"),
			"a caller-supplied continue token must disable the widened scan")
	})
}

// TestTruncateToCallerLimit pins the second half of the widened-scan path.
// The server is asked for listSortScanLimit items so the sort has something
// meaningful to order; the caller's much smaller page size must then be
// applied here, and the continue token dropped because it cannot express
// "the next N by recency" once the items have been reordered.
func TestTruncateToCallerLimit(t *testing.T) {
	events := func(n int) []runtime.Object {
		items := make([]runtime.Object, 0, n)
		for i := 0; i < n; i++ {
			items = append(items, eventObj(fmt.Sprintf("e%03d", i), "lastTimestamp", "2026-07-29T10:00:00Z"))
		}
		return items
	}

	t.Run("keeps the newest limit items", func(t *testing.T) {
		resp := &k8s.PaginatedListResponse{Items: events(200), TotalItems: 200}

		scanned, capped := truncateToCallerLimit(resp, 15)

		assert.Equal(t, 200, scanned, "the full scan must be reported")
		assert.False(t, capped, "200 items is well under the ceiling")
		assert.Len(t, resp.Items, 15)
		assert.Equal(t, 15, resp.TotalItems)
		// The slice is a prefix of the sorted input, so item 0 stays first.
		first, ok := resp.Items[0].(*unstructured.Unstructured)
		require.True(t, ok)
		assert.Equal(t, "e000", first.GetName())
	})

	t.Run("a scan smaller than the limit is untouched", func(t *testing.T) {
		resp := &k8s.PaginatedListResponse{Items: events(3), TotalItems: 3}

		scanned, capped := truncateToCallerLimit(resp, 15)

		assert.Equal(t, 3, scanned)
		assert.False(t, capped)
		assert.Len(t, resp.Items, 3)
		assert.Equal(t, 3, resp.TotalItems)
	})

	t.Run("continue token and remainingItems are cleared", func(t *testing.T) {
		remaining := int64(4000)
		resp := &k8s.PaginatedListResponse{
			Items:          events(50),
			TotalItems:     50,
			Continue:       "server-key-order-token",
			RemainingItems: &remaining,
		}

		_, capped := truncateToCallerLimit(resp, 15)

		assert.Empty(t, resp.Continue, "a key-order token is meaningless after reordering")
		assert.Nil(t, resp.RemainingItems)
		assert.True(t, capped, "a leftover server token means the scan was incomplete")
	})

	t.Run("hitting the ceiling is reported as capped", func(t *testing.T) {
		resp := &k8s.PaginatedListResponse{
			Items:      events(int(listSortScanLimit)),
			TotalItems: int(listSortScanLimit),
		}

		scanned, capped := truncateToCallerLimit(resp, 15)

		assert.Equal(t, int(listSortScanLimit), scanned)
		assert.True(t, capped, "a scan that reached the ceiling may have missed older items")
	})

	t.Run("limit zero keeps the whole scan", func(t *testing.T) {
		resp := &k8s.PaginatedListResponse{Items: events(40), TotalItems: 40}

		truncateToCallerLimit(resp, 0)

		assert.Len(t, resp.Items, 40)
	})
}

// TestListScanNotice pins the transparency string. An agent must be able to
// tell "the newest 15 of everything there is" from "the newest 15 of the
// first N we were willing to read" — otherwise a capped scan reads as a
// complete answer, which is the same class of silent-truncation bug the
// newest-first ordering was added to fix.
func TestListScanNotice(t *testing.T) {
	t.Run("complete scan says so", func(t *testing.T) {
		notice := listScanNotice("events", 312, false)
		assert.Contains(t, notice, "newest-first")
		assert.Contains(t, notice, "all 312")
		assert.NotContains(t, notice, "ceiling")
	})

	t.Run("capped scan warns and suggests narrowing", func(t *testing.T) {
		notice := listScanNotice("events", int(listSortScanLimit), true)
		assert.Contains(t, notice, "ceiling")
		assert.Contains(t, notice, "fieldSelector")
	})
}

func TestAppendHint(t *testing.T) {
	assert.Equal(t, "note", appendHint("", "note"))
	assert.Equal(t, "existing", appendHint("existing", ""))
	assert.Equal(t, "existing; note", appendHint("existing", "note"))
	assert.Equal(t, "", appendHint("", ""))
}
