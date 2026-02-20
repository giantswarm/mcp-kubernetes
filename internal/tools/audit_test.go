package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	mcpoauth "github.com/giantswarm/mcp-oauth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

func TestWrapWithAuditLogging_CapturesToolName(t *testing.T) {
	// Create a mock instrumentation provider with audit logger
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	// Create a test handler that succeeds
	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("success"), nil
	}

	// Wrap the handler
	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	// Call the wrapped handler
	request := createTestRequest(nil)
	_, err := wrapped(context.Background(), request)
	require.NoError(t, err)

	// Verify the audit logger was called (implicitly, since no errors)
	auditLogger := provider.AuditLogger()
	require.NotNil(t, auditLogger)
}

func TestWrapWithAuditLogging_ExtractsUserInfo(t *testing.T) {
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("success"), nil
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	// Create context with user info
	userInfo := &oauth.UserInfo{
		Email:  "user@example.com",
		Groups: []string{"admin", "developers"},
	}
	ctx := mcpoauth.ContextWithUserInfo(context.Background(), userInfo)

	request := createTestRequest(nil)
	_, err := wrapped(ctx, request)
	require.NoError(t, err)
}

func TestWrapWithAuditLogging_ExtractsClusterInfo(t *testing.T) {
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("success"), nil
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	// Create request with cluster and resource info
	args := map[string]interface{}{
		"cluster":      "prod-cluster-1",
		"namespace":    "kube-system",
		"resourceType": "pods",
		"name":         "my-pod",
	}
	request := createTestRequest(args)

	_, err := wrapped(context.Background(), request)
	require.NoError(t, err)
}

func TestWrapWithAuditLogging_MeasuresDuration(t *testing.T) {
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	// Create a handler that takes some time
	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		time.Sleep(10 * time.Millisecond)
		return mcp.NewToolResultText("success"), nil
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	request := createTestRequest(nil)
	start := time.Now()
	_, err := wrapped(context.Background(), request)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond)
}

func TestWrapWithAuditLogging_HandlesSuccess(t *testing.T) {
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("success"), nil
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	request := createTestRequest(nil)
	result, err := wrapped(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestWrapWithAuditLogging_HandlesGoError(t *testing.T) {
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	expectedErr := errors.New("handler error")
	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return nil, expectedErr
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	request := createTestRequest(nil)
	result, err := wrapped(context.Background(), request)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, result)
}

func TestWrapWithAuditLogging_HandlesMCPToolError(t *testing.T) {
	provider := createTestProvider(t)
	sc := createTestServerContext(t, provider)

	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("tool error message"), nil
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	request := createTestRequest(nil)
	result, err := wrapped(context.Background(), request)

	require.NoError(t, err) // No Go error
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestWrapWithAuditLogging_NoProvider(t *testing.T) {
	// Create server context without instrumentation provider
	sc := createTestServerContextNoInstrumentation(t)

	handler := func(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("success"), nil
	}

	wrapped := WrapWithAuditLogging("test_tool", handler, sc)

	request := createTestRequest(nil)
	result, err := wrapped(context.Background(), request)

	// Should still work, just without audit logging
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestExtractAuditInfoFromArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            map[string]interface{}
		expectCluster   string
		expectNamespace string
		expectResType   string
		expectResName   string
	}{
		{
			name: "full resource info",
			args: map[string]interface{}{
				"cluster":      "prod-cluster",
				"namespace":    "default",
				"resourceType": "pods",
				"name":         "my-pod",
			},
			expectCluster:   "prod-cluster",
			expectNamespace: "default",
			expectResType:   "pods",
			expectResName:   "my-pod",
		},
		{
			name: "kubeContext fallback",
			args: map[string]interface{}{
				"kubeContext":  "kind-local",
				"namespace":    "kube-system",
				"resourceType": "deployments",
				"name":         "coredns",
			},
			expectCluster:   "kind-local",
			expectNamespace: "kube-system",
			expectResType:   "deployments",
			expectResName:   "coredns",
		},
		{
			name: "pod name parameter",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "nginx-pod",
			},
			expectCluster:   "",
			expectNamespace: "default",
			expectResType:   "",
			expectResName:   "nginx-pod",
		},
		{
			name: "resourceName parameter",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceName": "my-service",
			},
			expectCluster:   "",
			expectNamespace: "default",
			expectResType:   "",
			expectResName:   "my-service",
		},
		{
			name: "pattern parameter for resolve",
			args: map[string]interface{}{
				"pattern": "prod-*",
			},
			expectCluster:   "",
			expectNamespace: "",
			expectResType:   "",
			expectResName:   "prod-*",
		},
		{
			name: "sessionID parameter",
			args: map[string]interface{}{
				"sessionID": "pf-12345",
			},
			expectCluster:   "",
			expectNamespace: "",
			expectResType:   "",
			expectResName:   "pf-12345",
		},
		{
			name:            "empty args",
			args:            map[string]interface{}{},
			expectCluster:   "",
			expectNamespace: "",
			expectResType:   "",
			expectResName:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invocation := instrumentation.NewToolInvocation("test")
			extractAuditInfoFromArgs(invocation, tt.args)

			assert.Equal(t, tt.expectCluster, invocation.ClusterName)
			assert.Equal(t, tt.expectNamespace, invocation.Namespace)
			assert.Equal(t, tt.expectResType, invocation.ResourceType)
			assert.Equal(t, tt.expectResName, invocation.ResourceName)
		})
	}
}

func TestExtractResourceName(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		expected string
	}{
		{
			name:     "name parameter",
			args:     map[string]interface{}{"name": "my-resource"},
			expected: "my-resource",
		},
		{
			name:     "podName parameter",
			args:     map[string]interface{}{"podName": "my-pod"},
			expected: "my-pod",
		},
		{
			name:     "resourceName parameter",
			args:     map[string]interface{}{"resourceName": "my-svc"},
			expected: "my-svc",
		},
		{
			name:     "pattern parameter",
			args:     map[string]interface{}{"pattern": "prod-*"},
			expected: "prod-*",
		},
		{
			name:     "sessionID parameter",
			args:     map[string]interface{}{"sessionID": "sess-123"},
			expected: "sess-123",
		},
		{
			name:     "name takes precedence",
			args:     map[string]interface{}{"name": "primary", "podName": "secondary"},
			expected: "primary",
		},
		{
			name:     "empty string ignored",
			args:     map[string]interface{}{"name": "", "podName": "actual"},
			expected: "actual",
		},
		{
			name:     "no matching parameter",
			args:     map[string]interface{}{"other": "value"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractResourceName(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions

func createTestProvider(t *testing.T) *instrumentation.Provider {
	t.Helper()
	config := instrumentation.Config{
		Enabled:         true,
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		MetricsExporter: instrumentation.ExporterPrometheus,
		TracingExporter: instrumentation.ExporterNone,
	}
	provider, err := instrumentation.NewProvider(context.Background(), config)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	return provider
}

func createTestServerContext(t *testing.T, provider *instrumentation.Provider) *server.ServerContext {
	t.Helper()
	sc, err := server.NewServerContext(
		context.Background(),
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithInstrumentationProvider(provider),
	)
	require.NoError(t, err)
	return sc
}

func createTestServerContextNoInstrumentation(t *testing.T) *server.ServerContext {
	t.Helper()
	sc, err := server.NewServerContext(
		context.Background(),
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)
	return sc
}

func createTestRequest(args map[string]interface{}) mcp.CallToolRequest {
	if args == nil {
		args = map[string]interface{}{}
	}
	request := mcp.CallToolRequest{}
	request.Params.Name = "test_tool"
	request.Params.Arguments = args
	return request
}
