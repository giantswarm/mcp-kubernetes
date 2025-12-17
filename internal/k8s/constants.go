package k8s

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
