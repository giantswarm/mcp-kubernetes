package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestListResponseMetaFields verifies that ListResponseMeta struct has the correct JSON tags.
func TestListResponseMetaFields(t *testing.T) {
	meta := &ListResponseMeta{
		ResourceScope:      "cluster",
		RequestedNamespace: "kube-system",
		EffectiveNamespace: "",
		Hint:               "nodes is cluster-scoped; namespace parameter was ignored",
	}

	// Verify struct fields are accessible
	assert.Equal(t, "cluster", meta.ResourceScope)
	assert.Equal(t, "kube-system", meta.RequestedNamespace)
	assert.Equal(t, "", meta.EffectiveNamespace)
	assert.Contains(t, meta.Hint, "cluster-scoped")
}

// TestListResponseMetaClusterScoped verifies metadata for cluster-scoped resources.
func TestListResponseMetaClusterScoped(t *testing.T) {
	meta := &ListResponseMeta{
		ResourceScope:      "cluster",
		RequestedNamespace: "some-namespace",
		EffectiveNamespace: "",
		Hint:               "nodes is cluster-scoped; namespace parameter was ignored",
	}

	assert.Equal(t, "cluster", meta.ResourceScope)
	assert.Empty(t, meta.EffectiveNamespace, "cluster-scoped resources should have empty effective namespace")
	assert.NotEmpty(t, meta.Hint, "should have hint about ignored namespace")
}

// TestListResponseMetaNamespaced verifies metadata for namespaced resources.
func TestListResponseMetaNamespaced(t *testing.T) {
	meta := &ListResponseMeta{
		ResourceScope:      "namespaced",
		RequestedNamespace: "production",
		EffectiveNamespace: "production",
		Hint:               "",
	}

	assert.Equal(t, "namespaced", meta.ResourceScope)
	assert.Equal(t, "production", meta.EffectiveNamespace)
	assert.Empty(t, meta.Hint, "namespaced resource with matching namespace should have no hint")
}

// TestListResponseMetaAllNamespaces verifies metadata for all-namespaces query.
func TestListResponseMetaAllNamespaces(t *testing.T) {
	meta := &ListResponseMeta{
		ResourceScope:      "namespaced",
		RequestedNamespace: "",
		EffectiveNamespace: "",
		Hint:               "Listing across all namespaces",
	}

	assert.Equal(t, "namespaced", meta.ResourceScope)
	assert.Empty(t, meta.EffectiveNamespace)
	assert.Contains(t, meta.Hint, "all namespaces")
}

// TestPaginatedListResponseWithMeta verifies that PaginatedListResponse includes Meta field.
func TestPaginatedListResponseWithMeta(t *testing.T) {
	response := &PaginatedListResponse{
		Items:      nil,
		TotalItems: 0,
		Meta: &ListResponseMeta{
			ResourceScope:      "cluster",
			RequestedNamespace: "default",
			EffectiveNamespace: "",
			Hint:               "nodes is cluster-scoped",
		},
	}

	assert.NotNil(t, response.Meta)
	assert.Equal(t, "cluster", response.Meta.ResourceScope)
}

// TestBuildListResponseMeta verifies the BuildListResponseMeta helper function.
func TestBuildListResponseMeta(t *testing.T) {
	tests := []struct {
		name           string
		namespaced     bool
		requestedNS    string
		effectiveNS    string
		resourceType   string
		allNamespaces  bool
		expectScope    string
		expectHint     string
		expectHintNone bool
	}{
		{
			name:           "cluster-scoped resource with namespace provided",
			namespaced:     false,
			requestedNS:    "kube-system",
			effectiveNS:    "",
			resourceType:   "nodes",
			allNamespaces:  false,
			expectScope:    "cluster",
			expectHint:     "nodes is cluster-scoped; namespace parameter was ignored",
			expectHintNone: false,
		},
		{
			name:           "cluster-scoped resource without namespace",
			namespaced:     false,
			requestedNS:    "",
			effectiveNS:    "",
			resourceType:   "nodes",
			allNamespaces:  false,
			expectScope:    "cluster",
			expectHint:     "",
			expectHintNone: true,
		},
		{
			name:           "namespaced resource with namespace",
			namespaced:     true,
			requestedNS:    "production",
			effectiveNS:    "production",
			resourceType:   "pods",
			allNamespaces:  false,
			expectScope:    "namespaced",
			expectHint:     "",
			expectHintNone: true,
		},
		{
			name:           "namespaced resource with allNamespaces",
			namespaced:     true,
			requestedNS:    "",
			effectiveNS:    "",
			resourceType:   "pods",
			allNamespaces:  true,
			expectScope:    "namespaced",
			expectHint:     "Listing across all namespaces",
			expectHintNone: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := BuildListResponseMeta(tt.namespaced, tt.requestedNS, tt.effectiveNS, tt.resourceType, tt.allNamespaces)

			assert.Equal(t, tt.expectScope, meta.ResourceScope)
			assert.Equal(t, tt.requestedNS, meta.RequestedNamespace)
			assert.Equal(t, tt.effectiveNS, meta.EffectiveNamespace)

			if tt.expectHintNone {
				assert.Empty(t, meta.Hint)
			} else {
				assert.Contains(t, meta.Hint, tt.expectHint)
			}
		})
	}
}

// TestBuildScopeCacheKey verifies the cache key building function.
func TestBuildScopeCacheKey(t *testing.T) {
	tests := []struct {
		name         string
		contextName  string
		resourceType string
		apiGroup     string
		expected     string
	}{
		{
			name:         "core resource without api group",
			contextName:  "my-context",
			resourceType: "pods",
			apiGroup:     "",
			expected:     "my-context:pods",
		},
		{
			name:         "resource with api group",
			contextName:  "prod-cluster",
			resourceType: "deployments",
			apiGroup:     "apps",
			expected:     "prod-cluster:apps/deployments",
		},
		{
			name:         "CRD with full api group",
			contextName:  "dev",
			resourceType: "clusters",
			apiGroup:     "cluster.x-k8s.io",
			expected:     "dev:cluster.x-k8s.io/clusters",
		},
		{
			name:         "uppercase resource type normalized",
			contextName:  "test",
			resourceType: "PODS",
			apiGroup:     "",
			expected:     "test:pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildScopeCacheKey(tt.contextName, tt.resourceType, tt.apiGroup)
			assert.Equal(t, tt.expected, result)
		})
	}
}
