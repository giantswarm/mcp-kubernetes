package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
)

// preferredResourcesDiscovery serves a fixed resource list from
// ServerPreferredResources, which the client-go fake leaves empty.
type preferredResourcesDiscovery struct {
	*fakediscovery.FakeDiscovery
	lists []*metav1.APIResourceList
}

func (d *preferredResourcesDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return d.lists, nil
}

func testDiscovery() *preferredResourcesDiscovery {
	return &preferredResourcesDiscovery{
		FakeDiscovery: &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{}},
		lists: []*metav1.APIResourceList{
			{GroupVersion: "v1", APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Kind: "Pod", Namespaced: true, ShortNames: []string{"po"}},
			}},
			{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{
				{Name: "deployments", SingularName: "deployment", Kind: "Deployment", Namespaced: true, ShortNames: []string{"deploy"}},
			}},
			{GroupVersion: "cluster.x-k8s.io/v1beta2", APIResources: []metav1.APIResource{
				{Name: "clusters", SingularName: "cluster", Kind: "Cluster", Namespaced: true, ShortNames: []string{"cl"}},
			}},
			{GroupVersion: "infrastructure.cluster.x-k8s.io/v1beta2", APIResources: []metav1.APIResource{
				{Name: "awsclusters", SingularName: "awscluster", Kind: "AWSCluster", Namespaced: true},
			}},
		},
	}
}

// A resource type may name its API group the way kubectl accepts it,
// <resource>.<group> or <resource>.<version>.<group>: agents write
// "clusters.cluster.x-k8s.io" and used to get "unknown resource type".
func TestResolveResourceType_GroupQualified(t *testing.T) {
	capiClusters := schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta2", Resource: "clusters"}
	tests := []struct {
		name, resourceType, apiGroup string
		want                         schema.GroupVersionResource
	}{
		{"resource.group", "clusters.cluster.x-k8s.io", "", capiClusters},
		{"kind.group", "Cluster.cluster.x-k8s.io", "", capiClusters},
		{"case-insensitive", "Clusters.Cluster.X-K8s.IO", "", capiClusters},
		{"group matching apiGroup", "clusters.cluster.x-k8s.io", "cluster.x-k8s.io", capiClusters},
		{"nested group", "awsclusters.infrastructure.cluster.x-k8s.io", "",
			schema.GroupVersionResource{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta2", Resource: "awsclusters"}},
		{"resource.version.group", "deployments.v1.apps", "",
			schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
		{"plain name and apiGroup, unchanged", "clusters", "cluster.x-k8s.io", capiClusters},
		{"plain name, unchanged", "pods", "", schema.GroupVersionResource{Version: "v1", Resource: "pods"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvr, namespaced, err := resolveResourceTypeShared(tt.resourceType, tt.apiGroup, testDiscovery())
			require.NoError(t, err)
			assert.Equal(t, tt.want, gvr)
			assert.True(t, namespaced)
		})
	}
}

func TestResolveResourceType_GroupQualifiedErrors(t *testing.T) {
	_, _, err := resolveResourceTypeShared("clusters.cluster.x-k8s.io", "apps", testDiscovery())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `names API group "cluster.x-k8s.io", but apiGroup is "apps"`)

	_, _, err = resolveResourceTypeShared("clusters.unknown.example.com", "", testDiscovery())
	require.Error(t, err)
	assert.Equal(t, "unknown resource type: clusters.unknown.example.com", err.Error(),
		"the error names the type as it was asked for")
}
