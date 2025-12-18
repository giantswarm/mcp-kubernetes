package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultNamespace verifies the default namespace constant.
func TestDefaultNamespace(t *testing.T) {
	assert.Equal(t, "default", DefaultNamespace,
		"DefaultNamespace should be 'default' following kubectl behavior")
}

// TestBuiltinClusterScopedResources verifies that the builtin cluster-scoped
// resources map correctly identifies cluster-scoped resources.
func TestBuiltinClusterScopedResources(t *testing.T) {
	// These resources should be cluster-scoped
	clusterScoped := []string{
		"nodes",
		"namespaces",
		"persistentvolumes",
		"clusterroles",
		"clusterrolebindings",
	}

	for _, resource := range clusterScoped {
		t.Run(resource+"_is_cluster_scoped", func(t *testing.T) {
			assert.True(t, builtinClusterScopedResources[resource],
				"Expected %q to be cluster-scoped", resource)
		})
	}

	// These resources should NOT be in the cluster-scoped map
	namespaced := []string{
		"pods",
		"services",
		"deployments",
		"configmaps",
		"secrets",
		"roles",
		"rolebindings",
	}

	for _, resource := range namespaced {
		t.Run(resource+"_is_namespaced", func(t *testing.T) {
			assert.False(t, builtinClusterScopedResources[resource],
				"Expected %q to NOT be in cluster-scoped map", resource)
		})
	}
}
