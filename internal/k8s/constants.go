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

	// DefaultNamespace is used when no namespace is specified, following kubectl behavior.
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	DefaultNamespace = "default"

	// Operation names used by access control and listing helpers.
	OperationApply      = "apply"
	OperationCreate     = "create"
	OperationDelete     = "delete"
	OperationPatch      = "patch"
	OperationScale      = "scale"
	OperationListAll    = "Listing across all namespaces"
	ListScopeNamespaced = "namespaced"
	ListScopeCluster    = "cluster"

	// resourceDeployments matches the k8s "deployments" resource kind. Defined
	// here to avoid duplicate string literals in non-test code.
	resourceDeployments = "deployments"

	// Cluster health component names.
	componentAPIServer             = "API Server"
	componentEtcd                  = "etcd"
	componentKubeAPIServer         = "kube-apiserver"
	componentKubeControllerManager = "kube-controller-manager"
	componentKubeScheduler         = "kube-scheduler"
)
