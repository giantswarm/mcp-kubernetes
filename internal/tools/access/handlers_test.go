package access

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
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
			name:     "any other error",
			err:      federation.ErrAccessDenied,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidationError(tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCanIResponse_WithEvaluationError(t *testing.T) {
	// Test that evaluation errors are properly included in the response
	response := &CanIResponse{
		Allowed: false,
		Denied:  false,
		Reason:  "evaluation error: unable to find resource definition for custom.io/v1",
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

	assert.Contains(t, string(data), "evaluation error")
	assert.Contains(t, string(data), "custom.io")
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
