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

		// Namespaced resources (should return false)
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
		{"replicasets", false, "replicasets is namespaced"},
		{"endpoints", false, "endpoints is namespaced"},
		{"serviceaccounts", false, "serviceaccounts is namespaced"},

		// Edge cases
		{"", false, "empty string is not cluster-scoped"},
		{"unknown", false, "unknown resource is not cluster-scoped"},
		{"PODS", false, "PODS (uppercase namespaced) is namespaced"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := IsClusterScoped(tt.resourceType)
			assert.Equal(t, tt.isCluster, result, "IsClusterScoped(%q) = %v, want %v", tt.resourceType, result, tt.isCluster)
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
