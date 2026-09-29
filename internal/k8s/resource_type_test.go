package k8s

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

// preferredResourcesDiscovery serves a fixed resource list from
// ServerPreferredResources, which the client-go fake leaves empty. Like a real
// server, it lists each resource at its preferred version only; a group
// version that is not preferred is served by ServerResourcesForGroupVersion.
type preferredResourcesDiscovery struct {
	*fakediscovery.FakeDiscovery
	lists        []*metav1.APIResourceList
	notPreferred []*metav1.APIResourceList
	// groupVersionErr, when set, is what ServerResourcesForGroupVersion fails with.
	groupVersionErr error
	// preferredCalls counts the ServerPreferredResources calls: each one is a
	// download of the server's whole discovery data.
	preferredCalls int
}

func (d *preferredResourcesDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	d.preferredCalls++
	return d.lists, nil
}

func (d *preferredResourcesDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	if d.groupVersionErr != nil {
		return nil, d.groupVersionErr
	}
	for _, list := range append(slices.Clone(d.lists), d.notPreferred...) {
		if list.GroupVersion == groupVersion {
			return list, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, groupVersion)
}

func testDiscovery() *preferredResourcesDiscovery {
	return &preferredResourcesDiscovery{
		FakeDiscovery: &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{}},
		notPreferred: []*metav1.APIResourceList{
			{GroupVersion: "cluster.x-k8s.io/v1beta1", APIResources: []metav1.APIResource{
				{Name: "clusters", SingularName: "cluster", Kind: "Cluster", Namespaced: true, ShortNames: []string{"cl"}},
			}},
		},
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
			// A group whose first label looks like an API version.
			{GroupVersion: "v1.example.com/v1alpha1", APIResources: []metav1.APIResource{
				{Name: "widgets", SingularName: "widget", Kind: "Widget", Namespaced: true},
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
		{"core group, qualified", "pods.v1.", "", schema.GroupVersionResource{Version: "v1", Resource: "pods"}},
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

// A version named in the qualified form is required, as kubectl requires it:
// it neither falls back to another version nor gives way to apiGroup's.
func TestResolveResourceType_QualifiedVersionIsRequired(t *testing.T) {
	_, _, err := resolveResourceTypeShared("deployments.v9.apps", "", testDiscovery())
	require.Error(t, err)
	assert.Equal(t, "unknown resource type: deployments.v9.apps", err.Error())

	_, _, err = resolveResourceTypeShared("deployments.v1.apps", "apps/v1beta1", testDiscovery())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `names API version "v1", but apiGroup names "v1beta1"`)

	gvr, _, err := resolveResourceTypeShared("deployments.v1.apps", "apps/v1", testDiscovery())
	require.NoError(t, err, "the same version in both places is no conflict")
	assert.Equal(t, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, gvr)

	_, _, err = resolveResourceTypeShared("clusters.v1alpha9.cluster.x-k8s.io", "", testDiscovery())
	require.Error(t, err, "a version the group does not serve at all")
	assert.Equal(t, "unknown resource type: clusters.v1alpha9.cluster.x-k8s.io", err.Error())
}

// A version the server serves but does not prefer resolves, as in kubectl:
// the preferred lists hold each resource at its preferred version only.
func TestResolveResourceType_QualifiedVersionNotPreferred(t *testing.T) {
	gvr, namespaced, err := resolveResourceTypeShared("clusters.v1beta1.cluster.x-k8s.io", "", testDiscovery())
	require.NoError(t, err)
	assert.Equal(t, schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "clusters"}, gvr)
	assert.True(t, namespaced)

	gvr, _, err = resolveResourceTypeShared("clusters.v1beta2.cluster.x-k8s.io", "", testDiscovery())
	require.NoError(t, err)
	assert.Equal(t, "v1beta2", gvr.Version, "the preferred version still resolves from the preferred lists")
}

// Which reading of <a>.<b>.<rest> applies — <b> as the version of group <rest>,
// or <b>.<rest> as the group — is what the server serves that decides, as in
// kubectl, not how <b> looks.
func TestResolveResourceType_DiscoveryPicksTheReading(t *testing.T) {
	gvr, _, err := resolveResourceTypeShared("widgets.v1.example.com", "", testDiscovery())
	require.NoError(t, err, "no group example.com is served, so v1.example.com is the group")
	assert.Equal(t, schema.GroupVersionResource{Group: "v1.example.com", Version: "v1alpha1", Resource: "widgets"}, gvr)

	d := testDiscovery()
	d.groupVersionErr = fmt.Errorf("connection refused")
	_, _, err = resolveResourceTypeShared("deployments.v1.apps", "", d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discovery of apps/v1 failed: connection refused")

	_, _, err = resolveResourceTypeShared("deployments.apps", "", d)
	require.NoError(t, err, "a type with one dot has a single reading and needs no version lookup")
}

// The version reading is checked against its one group version, and a type it
// applies to resolves from that small list: no download of the whole
// discovery data. The group reading still searches the preferred lists once.
func TestResolveResourceType_VersionReadingNeedsNoFullDiscovery(t *testing.T) {
	d := testDiscovery()
	gvr, namespaced, err := resolveResourceTypeShared("deployments.v1.apps", "", d)
	require.NoError(t, err)
	assert.Equal(t, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, gvr)
	assert.True(t, namespaced)
	assert.Zero(t, d.preferredCalls)

	d = testDiscovery()
	_, _, err = resolveResourceTypeShared("clusters.cluster.x-k8s.io", "", d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.preferredCalls, "x-k8s.io/cluster is not served, so the group reading searches once")
}

// A lookup of the version that is refused or fails is reported as such, not
// as an unknown type, so the caller learns what broke.
func TestResolveResourceType_QualifiedVersionLookupFails(t *testing.T) {
	d := testDiscovery()
	d.groupVersionErr = apierrors.NewForbidden(schema.GroupResource{Group: "cluster.x-k8s.io"}, "", fmt.Errorf("no access"))

	_, _, err := resolveResourceTypeShared("clusters.v1beta1.cluster.x-k8s.io", "", d)
	require.Error(t, err)
	assert.True(t, apierrors.IsForbidden(err), "the discovery error is wrapped: %v", err)
	assert.Contains(t, err.Error(), "discovery of cluster.x-k8s.io/v1beta1 failed")
	assert.NotContains(t, err.Error(), "unknown resource type")
}

func TestScalableAppsKind(t *testing.T) {
	for resourceType, want := range map[string]string{
		"deployment": "deployment", "deployments": "deployment", "Deployments.apps": "deployment",
		"deployments.v1.apps": "deployment", "replicasets.apps": "replicaset", "statefulsets.v1.apps": "statefulset",
	} {
		kind, ok := scalableAppsKind(resourceType)
		assert.True(t, ok, resourceType)
		assert.Equal(t, want, kind, resourceType)
	}
	for _, resourceType := range []string{"pods", "deployments.example.com", "clusters.cluster.x-k8s.io", "deployments.v9.apps"} {
		_, ok := scalableAppsKind(resourceType)
		assert.False(t, ok, resourceType)
	}
}

// Scaling through a resolved GVR accepts every spelling the resolver does:
// "deployments.apps" used to resolve and then fail as "not scalable".
func TestScaleResourceWithGVR_QualifiedType(t *testing.T) {
	deployment := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "web", "namespace": "default"},
		"spec":     map[string]any{"replicas": int64(1)},
	}}
	for _, resourceType := range []string{"deployments.apps", "deployments.v1.apps", "deploy"} {
		t.Run(resourceType, func(t *testing.T) {
			gvr, namespaced, err := resolveResourceTypeShared(resourceType, "", testDiscovery())
			require.NoError(t, err)
			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), deployment.DeepCopy())

			err = scaleResourceWithGVR(t.Context(), client, gvr, namespaced, "default", resourceType, "web", 3, false)
			require.NoError(t, err)

			got, err := client.Resource(gvr).Namespace("default").Get(t.Context(), "web", metav1.GetOptions{})
			require.NoError(t, err)
			replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
			assert.Equal(t, int64(3), replicas)
		})
	}

	capi := schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta2", Resource: "clusters"}
	err := scaleResourceWithGVR(t.Context(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), capi, true,
		"default", "clusters.cluster.x-k8s.io", "c1", 3, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not scalable")
}
