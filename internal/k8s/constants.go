package k8s

import "strings"

const (
	// Service account paths - default Kubernetes in-cluster locations
	DefaultServiceAccountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
	DefaultTokenPath          = DefaultServiceAccountPath + "/token"
	DefaultCACertPath         = DefaultServiceAccountPath + "/ca.crt"
	DefaultNamespacePath      = DefaultServiceAccountPath + "/namespace"

	// Default performance settings
	DefaultQPSLimit   = 20.0
	DefaultBurstLimit = 30
	DefaultTimeout    = 30 // seconds

	// Discovery timeout
	DiscoveryTimeoutSeconds = 30

	// In-cluster context name
	InClusterContext = "in-cluster"
)

// clusterScopedResources is the authoritative list of cluster-scoped resource types.
// This map includes canonical plural names, singular forms, and short aliases.
// It is the single source of truth for determining if a resource is cluster-scoped.
var clusterScopedResources = map[string]bool{
	// Core resources
	"nodes":             true,
	"node":              true,
	"persistentvolumes": true,
	"persistentvolume":  true,
	"pv":                true,
	"namespaces":        true,
	"namespace":         true,
	"ns":                true,
	"componentstatuses": true,
	"componentstatus":   true,
	"cs":                true,

	// RBAC resources
	"clusterroles":        true,
	"clusterrole":         true,
	"clusterrolebindings": true,
	"clusterrolebinding":  true,

	// Storage resources
	"storageclasses":       true,
	"storageclass":         true,
	"sc":                   true,
	"volumeattachments":    true,
	"volumeattachment":     true,
	"csidrivers":           true,
	"csidriver":            true,
	"csinodes":             true,
	"csinode":              true,
	"csistoragecapacities": true,
	"csistoragecapacity":   true,

	// Networking resources
	"ingressclasses": true,
	"ingressclass":   true,

	// Scheduling resources
	"priorityclasses": true,
	"priorityclass":   true,
	"pc":              true,

	// Node resources
	"runtimeclasses": true,
	"runtimeclass":   true,

	// Policy resources (deprecated but still valid)
	"podsecuritypolicies": true,
	"podsecuritypolicy":   true,
	"psp":                 true,

	// Admission resources
	"mutatingwebhookconfigurations":   true,
	"mutatingwebhookconfiguration":    true,
	"validatingwebhookconfigurations": true,
	"validatingwebhookconfiguration":  true,

	// API extension resources
	"customresourcedefinitions": true,
	"customresourcedefinition":  true,
	"crd":                       true,
	"crds":                      true,
	"apiservices":               true,
	"apiservice":                true,

	// Certificate resources
	"certificatesigningrequests": true,
	"certificatesigningrequest":  true,
	"csr":                        true,
}

// knownNamespacedResources is a list of common namespaced resource types.
// This is used for early validation to provide helpful error messages.
// Resources not in either list are considered "unknown" and validation is deferred to the API.
var knownNamespacedResources = map[string]bool{
	// Core resources
	"pods": true, "pod": true, "po": true,
	"services": true, "service": true, "svc": true,
	"configmaps": true, "configmap": true, "cm": true,
	"secrets": true, "secret": true,
	"endpoints": true, "endpoint": true, "ep": true,
	"events": true, "event": true, "ev": true,
	"persistentvolumeclaims": true, "persistentvolumeclaim": true, "pvc": true,
	"serviceaccounts": true, "serviceaccount": true, "sa": true,
	"resourcequotas": true, "resourcequota": true, "quota": true,
	"limitranges": true, "limitrange": true, "limits": true,
	"replicationcontrollers": true, "replicationcontroller": true, "rc": true,
	"podtemplates": true, "podtemplate": true,

	// Apps resources
	"deployments": true, "deployment": true, "deploy": true,
	"replicasets": true, "replicaset": true, "rs": true,
	"statefulsets": true, "statefulset": true, "sts": true,
	"daemonsets": true, "daemonset": true, "ds": true,
	"controllerrevisions": true, "controllerrevision": true,

	// Batch resources
	"jobs": true, "job": true,
	"cronjobs": true, "cronjob": true, "cj": true,

	// Networking resources
	"ingresses": true, "ingress": true, "ing": true,
	"networkpolicies": true, "networkpolicy": true, "netpol": true,

	// RBAC resources (namespaced)
	"roles": true, "role": true,
	"rolebindings": true, "rolebinding": true,

	// Policy resources
	"poddisruptionbudgets": true, "poddisruptionbudget": true, "pdb": true,

	// Autoscaling resources
	"horizontalpodautoscalers": true, "horizontalpodautoscaler": true, "hpa": true,
}

// IsClusterScoped checks if the given resource type is a known cluster-scoped (not namespaced) resource.
// The check is case-insensitive and supports canonical names, singular forms, and aliases.
// Returns true only for known built-in cluster-scoped resources.
// For unknown resources (e.g., CRDs), this returns false - use IsKnownResource to check if
// the resource is in the known list at all.
func IsClusterScoped(resourceType string) bool {
	return clusterScopedResources[strings.ToLower(resourceType)]
}

// IsKnownNamespaced checks if the given resource type is a known namespaced resource.
// Returns true for common built-in namespaced resources like pods, deployments, services, etc.
func IsKnownNamespaced(resourceType string) bool {
	return knownNamespacedResources[strings.ToLower(resourceType)]
}

// IsKnownResource checks if the given resource type is in our known resource list
// (either cluster-scoped or namespaced). Returns false for unknown resources like CRDs.
func IsKnownResource(resourceType string) bool {
	lower := strings.ToLower(resourceType)
	return clusterScopedResources[lower] || knownNamespacedResources[lower]
}
