package access

import (
	"context"
	"encoding/json"
	"testing"

	mcpoauth "github.com/giantswarm/mcp-oauth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/access/testdata"
)

func TestCanIResponse_JSONFormat(t *testing.T) {
	response := &CanIResponse{
		Allowed: true,
		Denied:  false,
		Reason:  "RBAC: allowed by ClusterRoleBinding",
		User:    "test@example.com",
		Cluster: "local",
		Check: &AccessCheckInfo{
			Verb:        "get",
			Resource:    "pods",
			APIGroup:    "",
			Namespace:   "default",
			Name:        "",
			Subresource: "",
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify required fields are present
	assert.Contains(t, string(data), `"allowed":true`)
	assert.Contains(t, string(data), `"user":"test@example.com"`)
	assert.Contains(t, string(data), `"cluster":"local"`)
	assert.Contains(t, string(data), `"verb":"get"`)
	assert.Contains(t, string(data), `"resource":"pods"`)

	// Unmarshal and verify round-trip
	var parsed CanIResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, response.Allowed, parsed.Allowed)
	assert.Equal(t, response.User, parsed.User)
	assert.Equal(t, response.Cluster, parsed.Cluster)
	assert.Equal(t, response.Check.Verb, parsed.Check.Verb)
	assert.Equal(t, response.Check.Resource, parsed.Check.Resource)
}

func TestCanIResponse_DeniedFormat(t *testing.T) {
	response := &CanIResponse{
		Allowed: false,
		Denied:  true,
		Reason:  "RBAC: delete denied in namespace production",
		User:    "dev@example.com",
		Cluster: "prod-cluster",
		Check: &AccessCheckInfo{
			Verb:      "delete",
			Resource:  "pods",
			Namespace: "production",
		},
	}

	data, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	t.Logf("Denied response JSON:\n%s", string(data))

	// Verify denial fields
	assert.Contains(t, string(data), `"allowed": false`)
	assert.Contains(t, string(data), `"denied": true`)
	assert.Contains(t, string(data), `"reason"`)
}

func TestAccessCheckInfo_AllFields(t *testing.T) {
	info := &AccessCheckInfo{
		Verb:        "create",
		Resource:    "pods",
		APIGroup:    "core",
		Namespace:   "default",
		Name:        "my-pod",
		Subresource: "exec",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var parsed AccessCheckInfo
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, info.Verb, parsed.Verb)
	assert.Equal(t, info.Resource, parsed.Resource)
	assert.Equal(t, info.APIGroup, parsed.APIGroup)
	assert.Equal(t, info.Namespace, parsed.Namespace)
	assert.Equal(t, info.Name, parsed.Name)
	assert.Equal(t, info.Subresource, parsed.Subresource)
}

func TestClusterDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		want        string
	}{
		{
			name:        "empty cluster name returns local",
			clusterName: "",
			want:        "local",
		},
		{
			name:        "non-empty cluster name returns name",
			clusterName: "prod-cluster",
			want:        "prod-cluster",
		},
		{
			name:        "management cluster name",
			clusterName: "management-cluster",
			want:        "management-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterDisplayName(tt.clusterName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrInvalidAccessCheck",
			err:      federation.ErrInvalidAccessCheck,
			expected: true,
		},
		{
			name:     "ErrUserInfoRequired",
			err:      federation.ErrUserInfoRequired,
			expected: true,
		},
		{
			name:     "ErrInvalidClusterName",
			err:      federation.ErrInvalidClusterName,
			expected: true,
		},
		{
			name:     "ErrAccessDenied is not a validation error",
			err:      federation.ErrAccessDenied,
			expected: false,
		},
		{
			name:     "ErrAccessCheckFailed is not a validation error",
			err:      federation.ErrAccessCheckFailed,
			expected: false,
		},
		{
			name:     "generic error is not a validation error",
			err:      federation.ErrClusterNotFound,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidationError(tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSanitizeEvaluationError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unable to find resource",
			input:    "unable to find resource definition for custom.io/v1/myresources",
			expected: "resource type not recognized",
		},
		{
			name:     "not found error",
			input:    "resource mygroup.io/v1beta1 not found in discovery",
			expected: "resource type not recognized",
		},
		{
			name:     "no matches for kind",
			input:    "no matches for kind \"CustomThing\" in version \"v1\"",
			expected: "resource type not recognized",
		},
		{
			name:     "webhook failure",
			input:    "webhook \"validating.policy.io\" denied request: internal error",
			expected: "policy evaluation failed",
		},
		{
			name:     "admission controller",
			input:    "admission controller denied: policy xyz-internal-123 rejected",
			expected: "policy evaluation failed",
		},
		{
			name:     "timeout error",
			input:    "timeout waiting for policy engine response",
			expected: "permission check timed out",
		},
		{
			name:     "deadline exceeded",
			input:    "context deadline exceeded during evaluation",
			expected: "permission check timed out",
		},
		{
			name:     "internal server error",
			input:    "internal error: rbac cache failed to sync",
			expected: "internal evaluation error",
		},
		{
			name:     "server error",
			input:    "server error during policy evaluation",
			expected: "internal evaluation error",
		},
		{
			name:     "unknown error - should not leak details",
			input:    "failed to evaluate RBAC for user admin@internal.corp on namespace secret-project-xyz",
			expected: "unable to evaluate permissions",
		},
		{
			name:     "case insensitive - UNABLE TO FIND",
			input:    "UNABLE TO FIND resource",
			expected: "resource type not recognized",
		},
		{
			name:     "case insensitive - Webhook",
			input:    "Webhook validation failed",
			expected: "policy evaluation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeEvaluationError(tt.input)
			assert.Equal(t, tt.expected, result)

			// Security verification: ensure the original error is not in the output
			if tt.input != "" && tt.expected != "" {
				// The sanitized output should not contain any part of the original
				// that could leak internal details (except common words)
				assert.NotContains(t, result, "internal.corp")
				assert.NotContains(t, result, "secret-project")
				assert.NotContains(t, result, "xyz-internal")
				assert.NotContains(t, result, "custom.io")
				assert.NotContains(t, result, "mygroup.io")
			}
		})
	}
}

func TestSanitizeEvaluationError_SecurityProperties(t *testing.T) {
	// These tests verify that sensitive information is never leaked
	sensitiveInputs := []string{
		"unable to find resource definition for acme-corp.internal.io/v1/secretresources",
		"webhook \"validating.acme-corp.io\" denied request: user admin@acme-corp.com not authorized",
		"admission controller denied: policy prod-secrets-policy rejected request from 192.168.1.100",
		"internal error: failed to connect to authz service at https://authz.internal:8443",
		"server error: LDAP lookup failed for user cn=admin,ou=users,dc=acme,dc=corp",
	}

	for _, input := range sensitiveInputs {
		t.Run("no leak: "+input[:30]+"...", func(t *testing.T) {
			result := sanitizeEvaluationError(input)

			// Verify none of the potentially sensitive patterns appear in output
			sensitivePatterns := []string{
				"acme-corp",
				"internal.io",
				"admin@",
				"192.168",
				"authz.internal",
				"8443",
				"LDAP",
				"cn=admin",
				"dc=acme",
			}

			for _, pattern := range sensitivePatterns {
				assert.NotContains(t, result, pattern,
					"sanitized output should not contain sensitive pattern: %s", pattern)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		substrings []string
		expected   bool
	}{
		{
			name:       "contains first substring",
			s:          "hello world",
			substrings: []string{"hello", "foo"},
			expected:   true,
		},
		{
			name:       "contains second substring",
			s:          "hello world",
			substrings: []string{"foo", "world"},
			expected:   true,
		},
		{
			name:       "contains none",
			s:          "hello world",
			substrings: []string{"foo", "bar"},
			expected:   false,
		},
		{
			name:       "case insensitive match",
			s:          "HELLO WORLD",
			substrings: []string{"hello"},
			expected:   true,
		},
		{
			name:       "empty string",
			s:          "",
			substrings: []string{"hello"},
			expected:   false,
		},
		{
			name:       "empty substrings",
			s:          "hello",
			substrings: []string{},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.s, tt.substrings...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanIResponse_WithEvaluationError(t *testing.T) {
	// Test that evaluation errors are properly included in the response
	// Note: The actual reason shown here would be sanitized in the handler,
	// this test just verifies JSON serialization of the response structure
	response := &CanIResponse{
		Allowed: false,
		Denied:  false,
		Reason:  "evaluation issue: resource type not recognized",
		User:    "test@example.com",
		Cluster: "local",
		Check: &AccessCheckInfo{
			Verb:     "get",
			Resource: "customresources",
			APIGroup: "custom.io",
		},
	}

	data, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	t.Logf("Evaluation error response JSON:\n%s", string(data))

	// Verify sanitized error format is used
	assert.Contains(t, string(data), "evaluation issue")
	assert.Contains(t, string(data), "resource type not recognized")
	// The internal details (custom.io/v1) should NOT be in the reason
	assert.NotContains(t, string(data), "custom.io/v1")
}

func TestCanIResponse_ClusterScoped(t *testing.T) {
	// Test cluster-scoped resource check (no namespace)
	response := &CanIResponse{
		Allowed: true,
		User:    "admin@example.com",
		Cluster: "local",
		Check: &AccessCheckInfo{
			Verb:     "create",
			Resource: "namespaces",
			// No namespace for cluster-scoped resource
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed CanIResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed.Allowed)
	assert.Equal(t, "namespaces", parsed.Check.Resource)
	assert.Empty(t, parsed.Check.Namespace)
}

func TestCanIResponse_WithSubresource(t *testing.T) {
	// Test check for a subresource (e.g., pods/exec)
	response := &CanIResponse{
		Allowed: false,
		Denied:  true,
		Reason:  "pods/exec requires additional permissions",
		User:    "dev@example.com",
		Cluster: "dev-cluster",
		Check: &AccessCheckInfo{
			Verb:        "create",
			Resource:    "pods",
			Namespace:   "default",
			Subresource: "exec",
		},
	}

	data, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	t.Logf("Subresource check response JSON:\n%s", string(data))

	assert.Contains(t, string(data), `"subresource": "exec"`)
}

func TestCanIResponse_SpecificResourceName(t *testing.T) {
	// Test check for a specific resource by name
	response := &CanIResponse{
		Allowed: true,
		User:    "ops@example.com",
		Cluster: "prod-cluster",
		Check: &AccessCheckInfo{
			Verb:      "delete",
			Resource:  "pods",
			Namespace: "production",
			Name:      "my-important-pod",
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed CanIResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "my-important-pod", parsed.Check.Name)
}

func TestCanIResponse_OmitEmpty(t *testing.T) {
	// Test that omitempty works correctly
	response := &CanIResponse{
		Allowed: true,
		User:    "test@example.com",
		Check: &AccessCheckInfo{
			Verb:     "get",
			Resource: "pods",
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	// These should be omitted due to omitempty
	assert.NotContains(t, string(data), `"denied"`)
	assert.NotContains(t, string(data), `"reason"`)
	assert.NotContains(t, string(data), `"apiGroup"`)
	assert.NotContains(t, string(data), `"namespace"`)
	assert.NotContains(t, string(data), `"name"`)
	assert.NotContains(t, string(data), `"subresource"`)

	// But cluster should show "local" by default from the handler (test the raw struct here)
	// In handler, empty cluster is converted to "local"
}

func TestHandleCanI_MissingVerb(t *testing.T) {
	ctx := context.Background()

	// Create server context with mock federation manager
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(&testdata.MockFederationManager{}),
	)
	require.NoError(t, err)

	// Create request without verb
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"resource": "pods",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "verb is required")
}

func TestHandleCanI_MissingResource(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(&testdata.MockFederationManager{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb": "get",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "resource is required")
}

func TestHandleCanI_NoFederationManager(t *testing.T) {
	ctx := context.Background()

	// Create server context WITHOUT federation manager
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":     "get",
		"resource": "pods",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "federation mode")
}

func TestHandleCanI_NoUserInfo(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(&testdata.MockFederationManager{}),
	)
	require.NoError(t, err)

	// Context without user info
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":     "get",
		"resource": "pods",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "authentication required")
}

func TestHandleCanI_Allowed(t *testing.T) {
	ctx := context.Background()

	// Add user info to context using mcp-oauth library function
	userInfo := &oauth.UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessResult: &federation.AccessCheckResult{
			Allowed: true,
			Reason:  "RBAC: allowed by ClusterRoleBinding",
		},
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":      "get",
		"resource":  "pods",
		"namespace": "default",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Parse the response JSON
	var response CanIResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.True(t, response.Allowed)
	assert.False(t, response.Denied)
	assert.Equal(t, "test@example.com", response.User)
	assert.Equal(t, "local", response.Cluster)
	assert.Equal(t, "get", response.Check.Verb)
	assert.Equal(t, "pods", response.Check.Resource)
	assert.Equal(t, "default", response.Check.Namespace)
}

func TestHandleCanI_Denied(t *testing.T) {
	ctx := context.Background()

	userInfo := &oauth.UserInfo{
		Email:  "dev@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessResult: &federation.AccessCheckResult{
			Allowed: false,
			Denied:  true,
			Reason:  "RBAC: delete denied",
		},
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":      "delete",
		"resource":  "pods",
		"namespace": "production",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response CanIResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.False(t, response.Allowed)
	assert.True(t, response.Denied)
	assert.Equal(t, "RBAC: delete denied", response.Reason)
}

func TestHandleCanI_WithCluster(t *testing.T) {
	ctx := context.Background()

	userInfo := &oauth.UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessResult: &federation.AccessCheckResult{
			Allowed: true,
		},
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":     "get",
		"resource": "pods",
		"cluster":  "prod-cluster",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response CanIResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.Equal(t, "prod-cluster", response.Cluster)
}

func TestHandleCanI_WithEvaluationError(t *testing.T) {
	ctx := context.Background()

	userInfo := &oauth.UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessResult: &federation.AccessCheckResult{
			Allowed:         false,
			EvaluationError: "unable to find resource definition for internal.acme-corp.io/v1",
		},
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":     "get",
		"resource": "customresources",
		"apiGroup": "custom.io",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response CanIResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify sanitized output is used
	assert.Contains(t, response.Reason, "evaluation issue")
	assert.Contains(t, response.Reason, "resource type not recognized")
	// Verify internal details are NOT leaked
	assert.NotContains(t, response.Reason, "acme-corp")
	assert.NotContains(t, response.Reason, "internal.acme-corp.io")
}

func TestHandleCanI_ValidationError(t *testing.T) {
	ctx := context.Background()

	userInfo := &oauth.UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessErr: federation.ErrInvalidAccessCheck,
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":     "get",
		"resource": "pods",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "invalid request")
}

func TestHandleCanI_NonValidationError(t *testing.T) {
	ctx := context.Background()

	userInfo := &oauth.UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessErr: federation.ErrAccessCheckFailed,
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":     "get",
		"resource": "pods",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	// Non-validation errors should show a generic message
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "failed to check permissions")
}

func TestHandleCanI_AllOptionalParams(t *testing.T) {
	ctx := context.Background()

	userInfo := &oauth.UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}
	ctx = mcpoauth.ContextWithUserInfo(ctx, userInfo)

	mockManager := &testdata.MockFederationManager{
		CheckAccessResult: &federation.AccessCheckResult{
			Allowed: true,
		},
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"verb":        "create",
		"resource":    "pods",
		"apiGroup":    "",
		"namespace":   "default",
		"name":        "my-pod",
		"subresource": "exec",
		"cluster":     "prod-cluster",
	}

	result, err := HandleCanI(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response CanIResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.Equal(t, "create", response.Check.Verb)
	assert.Equal(t, "pods", response.Check.Resource)
	assert.Equal(t, "default", response.Check.Namespace)
	assert.Equal(t, "my-pod", response.Check.Name)
	assert.Equal(t, "exec", response.Check.Subresource)
	assert.Equal(t, "prod-cluster", response.Cluster)
}
