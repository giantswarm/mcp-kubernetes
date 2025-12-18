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

// IsClusterScoped checks if the given resource type is cluster-scoped (not namespaced).
// The check is case-insensitive and supports canonical names, singular forms, and aliases.
func IsClusterScoped(resourceType string) bool {
	return clusterScopedResources[strings.ToLower(resourceType)]
}
