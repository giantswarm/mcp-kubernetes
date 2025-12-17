package pod

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

func TestPortForwardResponseStructure(t *testing.T) {
	// Test that the PortForwardResponse struct can be properly marshaled to JSON
	response := PortForwardResponse{
		Success:      true,
		Message:      "Port forwarding session established to service mimir-query-frontend",
		SessionID:    "mimir/service/mimir-query-frontend:8888:8080",
		ResourceType: "service",
		ResourceName: "mimir-query-frontend",
		Namespace:    "mimir",
		PortMappings: []PortMapping{
			{
				LocalPort:  8888,
				RemotePort: 8080,
			},
		},
		Instructions: "This is a long-running session. Use 'list_port_forward_sessions' to view active sessions and 'stop_port_forward_session' to stop this session.",
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal response to JSON: %v", err)
	}

	// Verify the JSON structure
	var parsedResponse PortForwardResponse
	err = json.Unmarshal(jsonData, &parsedResponse)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify key fields
	if parsedResponse.Success != true {
		t.Errorf("Expected Success to be true, got %v", parsedResponse.Success)
	}

	if parsedResponse.ResourceType != "service" {
		t.Errorf("Expected ResourceType to be 'service', got %s", parsedResponse.ResourceType)
	}

	if parsedResponse.ResourceName != "mimir-query-frontend" {
		t.Errorf("Expected ResourceName to be 'mimir-query-frontend', got %s", parsedResponse.ResourceName)
	}

	if parsedResponse.Namespace != "mimir" {
		t.Errorf("Expected Namespace to be 'mimir', got %s", parsedResponse.Namespace)
	}

	if len(parsedResponse.PortMappings) != 1 {
		t.Errorf("Expected 1 port mapping, got %d", len(parsedResponse.PortMappings))
	} else {
		if parsedResponse.PortMappings[0].LocalPort != 8888 {
			t.Errorf("Expected LocalPort to be 8888, got %d", parsedResponse.PortMappings[0].LocalPort)
		}
		if parsedResponse.PortMappings[0].RemotePort != 8080 {
			t.Errorf("Expected RemotePort to be 8080, got %d", parsedResponse.PortMappings[0].RemotePort)
		}
	}

	// Print the JSON for verification
	t.Logf("Generated JSON response:\n%s", string(jsonData))
}

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

// TestNonDestructiveModeBlocksExec verifies that exec operations are blocked
// in non-destructive mode when dry-run is disabled.
func TestNonDestructiveModeBlocksExec(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode enabled and dry-run disabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace":     "default",
		"podName":       "test-pod",
		"containerName": "main",
		"command":       []interface{}{"ls", "-la"},
	}

	result, err := handleExec(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError, "expected error result")
	assert.Contains(t, getErrorText(t, result), "Exec operations are not allowed in non-destructive mode")
}

// TestNonDestructiveModeBlocksPortForward verifies that port-forward operations are blocked
// in non-destructive mode when dry-run is disabled.
func TestNonDestructiveModeBlocksPortForward(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode enabled and dry-run disabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace":    "default",
		"resourceName": "test-pod",
		"ports":        []interface{}{"8080:80"},
	}

	result, err := handlePortForward(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError, "expected error result")
	assert.Contains(t, getErrorText(t, result), "Port-Forward operations are not allowed in non-destructive mode")
}

// TestDryRunModeAllowsExec verifies that exec operations are allowed when dry-run mode
// is enabled, even with non-destructive mode enabled.
func TestDryRunModeAllowsExec(t *testing.T) {
	ctx := context.Background()

	// Create server context with both non-destructive mode AND dry-run enabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(true),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace":     "default",
		"podName":       "test-pod",
		"containerName": "main",
		"command":       []interface{}{"ls", "-la"},
	}

	result, err := handleExec(ctx, request, sc)
	require.NoError(t, err)
	// With dry-run enabled, the request should pass the non-destructive check
	// The actual k8s operation may fail (because our mock returns nil),
	// but the important thing is that we didn't get blocked by non-destructive mode
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "not allowed in non-destructive mode",
			"dry-run mode should allow exec to proceed past non-destructive check")
	}
}

// TestDryRunModeAllowsPortForward verifies that port-forward operations are allowed when
// dry-run mode is enabled, even with non-destructive mode enabled.
func TestDryRunModeAllowsPortForward(t *testing.T) {
	ctx := context.Background()

	// Create server context with both non-destructive mode AND dry-run enabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(true),
		server.WithDryRun(true),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace":    "default",
		"resourceName": "test-pod",
		"ports":        []interface{}{"8080:80"},
	}

	result, err := handlePortForward(ctx, request, sc)
	require.NoError(t, err)
	// With dry-run enabled, the request should pass the non-destructive check
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "not allowed in non-destructive mode",
			"dry-run mode should allow port-forward to proceed past non-destructive check")
	}
}

// TestNonDestructiveModeDisabledAllowsExec verifies that when non-destructive mode
// is disabled, exec operations are allowed.
func TestNonDestructiveModeDisabledAllowsExec(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode disabled
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithNonDestructiveMode(false),
		server.WithDryRun(false),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace":     "default",
		"podName":       "test-pod",
		"containerName": "main",
		"command":       []interface{}{"ls", "-la"},
	}

	result, err := handleExec(ctx, request, sc)
	require.NoError(t, err)
	// The request should NOT be blocked by non-destructive mode
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "not allowed in non-destructive mode",
			"non-destructive mode is disabled, should not block operation")
	}
}

// TestAllowedOperationsExplicitlyAllowsExec verifies that exec can be explicitly allowed
// via AllowedOperations even in non-destructive mode.
func TestAllowedOperationsExplicitlyAllowsExec(t *testing.T) {
	ctx := context.Background()

	// Create a custom config that allows exec operations
	customConfig := server.NewDefaultConfig()
	customConfig.NonDestructiveMode = true
	customConfig.DryRun = false
	customConfig.AllowedOperations = []string{"get", "list", "describe", "exec"} // Explicitly allow exec

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithConfig(customConfig),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace":     "default",
		"podName":       "test-pod",
		"containerName": "main",
		"command":       []interface{}{"ls", "-la"},
	}

	result, err := handleExec(ctx, request, sc)
	require.NoError(t, err)
	// Should NOT be blocked by non-destructive mode because exec is in AllowedOperations
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "Exec operations are not allowed in non-destructive mode",
			"exec should be allowed when explicitly in AllowedOperations")
	}
}

// TestExecErrorMessageIncludesDryRunHint verifies that the error message for blocked
// exec operations includes a hint about using dry-run mode.
func TestExecErrorMessageIncludesDryRunHint(t *testing.T) {
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
		"namespace":     "default",
		"podName":       "test-pod",
		"containerName": "main",
		"command":       []interface{}{"ls", "-la"},
	}

	result, err := handleExec(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)

	errorText := getErrorText(t, result)
	assert.Contains(t, errorText, "--dry-run",
		"error message should include hint about dry-run option")
}

// TestLogsAlwaysAllowed verifies that logs operations are always allowed
// regardless of non-destructive mode settings (logs is read-only).
func TestLogsAlwaysAllowed(t *testing.T) {
	ctx := context.Background()

	// Create server context with non-destructive mode enabled and dry-run disabled
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
		"podName":   "test-pod",
	}

	result, err := handleGetLogs(ctx, request, sc)
	require.NoError(t, err)
	// Logs should not be blocked by non-destructive mode
	if result.IsError {
		errorText := getErrorText(t, result)
		assert.NotContains(t, errorText, "non-destructive mode",
			"logs should always be allowed as it is read-only")
	}
}
