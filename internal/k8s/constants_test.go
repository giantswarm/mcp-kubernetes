package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsClusterScoped verifies that IsClusterScoped correctly identifies
// cluster-scoped resources with various input formats.
func TestIsClusterScoped(t *testing.T) {
	tests := []struct {
		resourceType string
		isCluster    bool
		description  string
	}{
		// Core cluster-scoped resources
		{"nodes", true, "nodes is cluster-scoped"},
		{"node", true, "node (singular) is cluster-scoped"},
		{"Nodes", true, "Nodes (capitalized) is cluster-scoped"},
		{"NODE", true, "NODE (uppercase) is cluster-scoped"},
		{"persistentvolumes", true, "persistentvolumes is cluster-scoped"},
		{"persistentvolume", true, "persistentvolume (singular) is cluster-scoped"},
		{"pv", true, "pv (short) is cluster-scoped"},
		{"PV", true, "PV (uppercase short) is cluster-scoped"},
		{"namespaces", true, "namespaces is cluster-scoped"},
		{"namespace", true, "namespace (singular) is cluster-scoped"},
		{"ns", true, "ns (short) is cluster-scoped"},
		{"componentstatuses", true, "componentstatuses is cluster-scoped"},
		{"componentstatus", true, "componentstatus (singular) is cluster-scoped"},
		{"cs", true, "cs (short) is cluster-scoped"},

		// RBAC cluster-scoped resources
		{"clusterroles", true, "clusterroles is cluster-scoped"},
		{"clusterrole", true, "clusterrole (singular) is cluster-scoped"},
		{"clusterrolebindings", true, "clusterrolebindings is cluster-scoped"},
		{"clusterrolebinding", true, "clusterrolebinding (singular) is cluster-scoped"},

		// Storage cluster-scoped resources
		{"storageclasses", true, "storageclasses is cluster-scoped"},
		{"storageclass", true, "storageclass (singular) is cluster-scoped"},
		{"sc", true, "sc (short) is cluster-scoped"},
		{"volumeattachments", true, "volumeattachments is cluster-scoped"},
		{"volumeattachment", true, "volumeattachment (singular) is cluster-scoped"},
		{"csidrivers", true, "csidrivers is cluster-scoped"},
		{"csidriver", true, "csidriver (singular) is cluster-scoped"},
		{"csinodes", true, "csinodes is cluster-scoped"},
		{"csinode", true, "csinode (singular) is cluster-scoped"},
		{"csistoragecapacities", true, "csistoragecapacities is cluster-scoped"},
		{"csistoragecapacity", true, "csistoragecapacity (singular) is cluster-scoped"},

		// Networking cluster-scoped resources
		{"ingressclasses", true, "ingressclasses is cluster-scoped"},
		{"ingressclass", true, "ingressclass (singular) is cluster-scoped"},

		// Scheduling cluster-scoped resources
		{"priorityclasses", true, "priorityclasses is cluster-scoped"},
		{"priorityclass", true, "priorityclass (singular) is cluster-scoped"},
		{"pc", true, "pc (short) is cluster-scoped"},

		// Node resources
		{"runtimeclasses", true, "runtimeclasses is cluster-scoped"},
		{"runtimeclass", true, "runtimeclass (singular) is cluster-scoped"},

		// Policy resources (deprecated but still valid)
		{"podsecuritypolicies", true, "podsecuritypolicies is cluster-scoped"},
		{"podsecuritypolicy", true, "podsecuritypolicy (singular) is cluster-scoped"},
		{"psp", true, "psp (short) is cluster-scoped"},

		// Admission resources
		{"mutatingwebhookconfigurations", true, "mutatingwebhookconfigurations is cluster-scoped"},
		{"mutatingwebhookconfiguration", true, "mutatingwebhookconfiguration (singular) is cluster-scoped"},
		{"validatingwebhookconfigurations", true, "validatingwebhookconfigurations is cluster-scoped"},
		{"validatingwebhookconfiguration", true, "validatingwebhookconfiguration (singular) is cluster-scoped"},

		// API extension resources
		{"customresourcedefinitions", true, "customresourcedefinitions is cluster-scoped"},
		{"customresourcedefinition", true, "customresourcedefinition (singular) is cluster-scoped"},
		{"crd", true, "crd (short) is cluster-scoped"},
		{"crds", true, "crds (short plural) is cluster-scoped"},
		{"apiservices", true, "apiservices is cluster-scoped"},
		{"apiservice", true, "apiservice (singular) is cluster-scoped"},

		// Certificate resources
		{"certificatesigningrequests", true, "certificatesigningrequests is cluster-scoped"},
		{"certificatesigningrequest", true, "certificatesigningrequest (singular) is cluster-scoped"},
		{"csr", true, "csr (short) is cluster-scoped"},

		// Namespaced resources (should return false for IsClusterScoped)
		{"pods", false, "pods is not cluster-scoped"},
		{"services", false, "services is not cluster-scoped"},
		{"deployments", false, "deployments is not cluster-scoped"},

		// Unknown resources (CRDs) - should return false
		{"clusters.cluster.x-k8s.io", false, "CAPI Cluster CRD is unknown"},
		{"machines.cluster.x-k8s.io", false, "CAPI Machine CRD is unknown"},
		{"apps.argoproj.io", false, "ArgoCD Application CRD is unknown"},
		{"unknown", false, "unknown resource returns false"},
		{"", false, "empty string returns false"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := IsClusterScoped(tt.resourceType)
			assert.Equal(t, tt.isCluster, result, "IsClusterScoped(%q) = %v, want %v", tt.resourceType, result, tt.isCluster)
		})
	}
}

// TestIsKnownNamespaced verifies that IsKnownNamespaced correctly identifies
// known namespaced resources.
func TestIsKnownNamespaced(t *testing.T) {
	tests := []struct {
		resourceType string
		isNamespaced bool
		description  string
	}{
		// Core namespaced resources
		{"pods", true, "pods is namespaced"},
		{"pod", true, "pod (singular) is namespaced"},
		{"po", true, "po (short) is namespaced"},
		{"Pods", true, "Pods (capitalized) is namespaced"},
		{"services", true, "services is namespaced"},
		{"service", true, "service (singular) is namespaced"},
		{"svc", true, "svc (short) is namespaced"},
		{"configmaps", true, "configmaps is namespaced"},
		{"configmap", true, "configmap (singular) is namespaced"},
		{"cm", true, "cm (short) is namespaced"},
		{"secrets", true, "secrets is namespaced"},
		{"secret", true, "secret (singular) is namespaced"},
		{"endpoints", true, "endpoints is namespaced"},
		{"ep", true, "ep (short) is namespaced"},
		{"persistentvolumeclaims", true, "pvc is namespaced"},
		{"pvc", true, "pvc (short) is namespaced"},
		{"serviceaccounts", true, "serviceaccounts is namespaced"},
		{"sa", true, "sa (short) is namespaced"},

		// Apps namespaced resources
		{"deployments", true, "deployments is namespaced"},
		{"deployment", true, "deployment (singular) is namespaced"},
		{"deploy", true, "deploy (short) is namespaced"},
		{"replicasets", true, "replicasets is namespaced"},
		{"rs", true, "rs (short) is namespaced"},
		{"statefulsets", true, "statefulsets is namespaced"},
		{"sts", true, "sts (short) is namespaced"},
		{"daemonsets", true, "daemonsets is namespaced"},
		{"ds", true, "ds (short) is namespaced"},

		// Batch namespaced resources
		{"jobs", true, "jobs is namespaced"},
		{"cronjobs", true, "cronjobs is namespaced"},
		{"cj", true, "cj (short) is namespaced"},

		// Networking namespaced resources
		{"ingresses", true, "ingresses is namespaced"},
		{"ing", true, "ing (short) is namespaced"},
		{"networkpolicies", true, "networkpolicies is namespaced"},
		{"netpol", true, "netpol (short) is namespaced"},

		// RBAC namespaced resources
		{"roles", true, "roles is namespaced"},
		{"rolebindings", true, "rolebindings is namespaced"},

		// Cluster-scoped resources (should return false)
		{"nodes", false, "nodes is not namespaced (cluster-scoped)"},
		{"namespaces", false, "namespaces is not namespaced (cluster-scoped)"},
		{"clusterroles", false, "clusterroles is not namespaced (cluster-scoped)"},

		// Unknown resources (CRDs) - should return false
		{"clusters.cluster.x-k8s.io", false, "CAPI Cluster CRD is unknown"},
		{"machines.cluster.x-k8s.io", false, "CAPI Machine CRD is unknown"},
		{"unknown", false, "unknown resource returns false"},
		{"", false, "empty string returns false"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := IsKnownNamespaced(tt.resourceType)
			assert.Equal(t, tt.isNamespaced, result, "IsKnownNamespaced(%q) = %v, want %v", tt.resourceType, result, tt.isNamespaced)
		})
	}
}

// TestIsKnownResource verifies that IsKnownResource correctly identifies
// whether a resource is in our known list (either cluster-scoped or namespaced).
func TestIsKnownResource(t *testing.T) {
	tests := []struct {
		resourceType string
		isKnown      bool
		description  string
	}{
		// Known cluster-scoped resources
		{"nodes", true, "nodes is known"},
		{"namespaces", true, "namespaces is known"},
		{"clusterroles", true, "clusterroles is known"},
		{"persistentvolumes", true, "persistentvolumes is known"},
		{"pv", true, "pv (short) is known"},
		{"crd", true, "crd is known"},

		// Known namespaced resources
		{"pods", true, "pods is known"},
		{"deployments", true, "deployments is known"},
		{"services", true, "services is known"},
		{"configmaps", true, "configmaps is known"},
		{"secrets", true, "secrets is known"},
		{"ingresses", true, "ingresses is known"},

		// Unknown resources (CRDs) - should return false
		{"clusters.cluster.x-k8s.io", false, "CAPI Cluster CRD is unknown"},
		{"machines.cluster.x-k8s.io", false, "CAPI Machine CRD is unknown"},
		{"apps.argoproj.io", false, "ArgoCD Application CRD is unknown"},
		{"helmreleases.helm.toolkit.fluxcd.io", false, "Flux HelmRelease CRD is unknown"},
		{"kustomizations.kustomize.toolkit.fluxcd.io", false, "Flux Kustomization CRD is unknown"},
		{"clusters", false, "clusters (without API group) is unknown"},
		{"awsclusters", false, "CAPA AWSCluster is unknown"},
		{"azureclusters", false, "CAPZ AzureCluster is unknown"},
		{"unknown", false, "unknown resource returns false"},
		{"", false, "empty string returns false"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := IsKnownResource(tt.resourceType)
			assert.Equal(t, tt.isKnown, result, "IsKnownResource(%q) = %v, want %v", tt.resourceType, result, tt.isKnown)
		})
	}
}

// TestClusterScopedResourcesCompleteness verifies that both singular and plural
// forms are present for each cluster-scoped resource type.
func TestClusterScopedResourcesCompleteness(t *testing.T) {
	// Each entry is [plural, singular, ...aliases]
	resourceForms := [][]string{
		{"nodes", "node"},
		{"persistentvolumes", "persistentvolume", "pv"},
		{"namespaces", "namespace", "ns"},
		{"componentstatuses", "componentstatus", "cs"},
		{"clusterroles", "clusterrole"},
		{"clusterrolebindings", "clusterrolebinding"},
		{"storageclasses", "storageclass", "sc"},
		{"volumeattachments", "volumeattachment"},
		{"csidrivers", "csidriver"},
		{"csinodes", "csinode"},
		{"csistoragecapacities", "csistoragecapacity"},
		{"ingressclasses", "ingressclass"}, // no standard short name
		{"priorityclasses", "priorityclass", "pc"},
		{"runtimeclasses", "runtimeclass"},
		{"podsecuritypolicies", "podsecuritypolicy", "psp"},
		{"mutatingwebhookconfigurations", "mutatingwebhookconfiguration"},
		{"validatingwebhookconfigurations", "validatingwebhookconfiguration"},
		{"customresourcedefinitions", "customresourcedefinition", "crd", "crds"},
		{"apiservices", "apiservice"},
		{"certificatesigningrequests", "certificatesigningrequest", "csr"},
	}

	for _, forms := range resourceForms {
		for _, form := range forms {
			t.Run(form, func(t *testing.T) {
				assert.True(t, IsClusterScoped(form),
					"Expected %q to be recognized as cluster-scoped", form)
			})
		}
	}
}

// TestNamespacedResourcesCompleteness verifies that both singular and plural
// forms are present for each known namespaced resource type.
func TestNamespacedResourcesCompleteness(t *testing.T) {
	// Each entry is [plural, singular, ...aliases]
	resourceForms := [][]string{
		{"pods", "pod", "po"},
		{"services", "service", "svc"},
		{"configmaps", "configmap", "cm"},
		{"secrets", "secret"},
		{"endpoints", "endpoint", "ep"},
		{"persistentvolumeclaims", "persistentvolumeclaim", "pvc"},
		{"serviceaccounts", "serviceaccount", "sa"},
		{"deployments", "deployment", "deploy"},
		{"replicasets", "replicaset", "rs"},
		{"statefulsets", "statefulset", "sts"},
		{"daemonsets", "daemonset", "ds"},
		{"jobs", "job"},
		{"cronjobs", "cronjob", "cj"},
		{"ingresses", "ingress", "ing"},
		{"roles", "role"},
		{"rolebindings", "rolebinding"},
	}

	for _, forms := range resourceForms {
		for _, form := range forms {
			t.Run(form, func(t *testing.T) {
				assert.True(t, IsKnownNamespaced(form),
					"Expected %q to be recognized as namespaced", form)
			})
		}
	}
}
