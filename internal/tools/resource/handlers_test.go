// Package resource provides tests for resource handler functionality.
package resource

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestIsClusterScoped verifies that k8s.IsClusterScoped correctly identifies
// cluster-scoped resources.
func TestIsClusterScoped(t *testing.T) {
	tests := []struct {
		resourceType string
		isCluster    bool
		description  string
	}{
		// Cluster-scoped resources
		{"nodes", true, "nodes is cluster-scoped"},
		{"node", true, "node (singular) is cluster-scoped"},
		{"Nodes", true, "Nodes (capitalized) is cluster-scoped"},
		{"NODE", true, "NODE (uppercase) is cluster-scoped"},
		{"persistentvolumes", true, "persistentvolumes is cluster-scoped"},
		{"persistentvolume", true, "persistentvolume (singular) is cluster-scoped"},
		{"pv", true, "pv (short) is cluster-scoped"},
		{"namespaces", true, "namespaces is cluster-scoped"},
		{"namespace", true, "namespace (singular) is cluster-scoped"},
		{"ns", true, "ns (short) is cluster-scoped"},
		{"clusterroles", true, "clusterroles is cluster-scoped"},
		{"clusterrole", true, "clusterrole (singular) is cluster-scoped"},
		{"clusterrolebindings", true, "clusterrolebindings is cluster-scoped"},
		{"clusterrolebinding", true, "clusterrolebinding (singular) is cluster-scoped"},
		{"storageclasses", true, "storageclasses is cluster-scoped"},
		{"storageclass", true, "storageclass (singular) is cluster-scoped"},
		{"sc", true, "sc (short) is cluster-scoped"},
		{"ingressclasses", true, "ingressclasses is cluster-scoped"},
		{"priorityclasses", true, "priorityclasses is cluster-scoped"},
		{"pc", true, "pc (short) is cluster-scoped"},
		{"runtimeclasses", true, "runtimeclasses is cluster-scoped"},
		{"customresourcedefinitions", true, "customresourcedefinitions is cluster-scoped"},
		{"crd", true, "crd (short) is cluster-scoped"},
		{"crds", true, "crds (short plural) is cluster-scoped"},
		{"apiservices", true, "apiservices is cluster-scoped"},
		{"certificatesigningrequests", true, "certificatesigningrequests is cluster-scoped"},
		{"csr", true, "csr (short) is cluster-scoped"},
		{"mutatingwebhookconfigurations", true, "mutatingwebhookconfigurations is cluster-scoped"},
		{"validatingwebhookconfigurations", true, "validatingwebhookconfigurations is cluster-scoped"},
		{"csidrivers", true, "csidrivers is cluster-scoped"},
		{"csinodes", true, "csinodes is cluster-scoped"},
		{"volumeattachments", true, "volumeattachments is cluster-scoped"},
		{"csistoragecapacities", true, "csistoragecapacities is cluster-scoped"},

		// Namespaced resources
		{"pods", false, "pods is namespaced"},
		{"pod", false, "pod is namespaced"},
		{"services", false, "services is namespaced"},
		{"service", false, "service is namespaced"},
		{"svc", false, "svc is namespaced"},
		{"deployments", false, "deployments is namespaced"},
		{"deployment", false, "deployment is namespaced"},
		{"configmaps", false, "configmaps is namespaced"},
		{"configmap", false, "configmap is namespaced"},
		{"cm", false, "cm is namespaced"},
		{"secrets", false, "secrets is namespaced"},
		{"secret", false, "secret is namespaced"},
		{"roles", false, "roles is namespaced"},
		{"rolebindings", false, "rolebindings is namespaced"},
		{"ingresses", false, "ingresses is namespaced"},
		{"persistentvolumeclaims", false, "persistentvolumeclaims is namespaced"},
		{"pvc", false, "pvc is namespaced"},
		{"daemonsets", false, "daemonsets is namespaced"},
		{"statefulsets", false, "statefulsets is namespaced"},
		{"jobs", false, "jobs is namespaced"},
		{"cronjobs", false, "cronjobs is namespaced"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := k8s.IsClusterScoped(tt.resourceType)
			assert.Equal(t, tt.isCluster, result, "k8s.IsClusterScoped(%q) = %v, want %v", tt.resourceType, result, tt.isCluster)
		})
	}
}

// TestListClusterScopedResourcesWithoutNamespace verifies that cluster-scoped resources
// can be listed without providing a namespace.
func TestListClusterScopedResourcesWithoutNamespace(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	tests := []struct {
		resourceType string
		description  string
	}{
		{"nodes", "nodes can be listed without namespace"},
		{"node", "node (singular) can be listed without namespace"},
		{"persistentvolumes", "persistentvolumes can be listed without namespace"},
		{"pv", "pv (short) can be listed without namespace"},
		{"namespaces", "namespaces can be listed without namespace"},
		{"ns", "ns (short) can be listed without namespace"},
		{"clusterroles", "clusterroles can be listed without namespace"},
		{"storageclasses", "storageclasses can be listed without namespace"},
		{"sc", "sc (short for storageclass) can be listed without namespace"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"resourceType": tt.resourceType,
				// No namespace provided
			}

			result, err := handleListResources(ctx, request, sc)
			require.NoError(t, err)
			// Should NOT get error about namespace being required
			if result.IsError {
				errorText := getErrorText(t, result)
				assert.NotContains(t, errorText, "namespace is required",
					"cluster-scoped resource %q should not require namespace", tt.resourceType)
			}
		})
	}
}

// TestListNamespacedResourcesRequireNamespace verifies that namespaced resources
// require a namespace or allNamespaces=true.
func TestListNamespacedResourcesRequireNamespace(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	tests := []struct {
		resourceType string
		description  string
	}{
		{"pods", "pods require namespace"},
		{"services", "services require namespace"},
		{"deployments", "deployments require namespace"},
		{"configmaps", "configmaps require namespace"},
		{"secrets", "secrets require namespace"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"resourceType": tt.resourceType,
				// No namespace provided
			}

			result, err := handleListResources(ctx, request, sc)
			require.NoError(t, err)
			assert.True(t, result.IsError, "namespaced resource %q should require namespace", tt.resourceType)
			errorText := getErrorText(t, result)
			assert.Contains(t, errorText, "namespace is required for namespaced resources",
				"error message should indicate namespace is required for namespaced resources")
		})
	}
}

// TestListNamespacedResourcesWithAllNamespaces verifies that namespaced resources
// can be listed with allNamespaces=true without providing a specific namespace.
func TestListNamespacedResourcesWithAllNamespaces(t *testing.T) {
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
			"allNamespaces=true should bypass namespace requirement")
	}
}
