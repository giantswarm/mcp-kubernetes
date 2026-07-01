package k8s

// ImpersonationIdentity is the resolved identity for an external-issuer (OBO)
// token. The access-token injector middleware populates it and stores it in the
// request context. K8sClientForContext reads it to build an impersonation client
// instead of a bearer-token passthrough client.
type ImpersonationIdentity struct {
	// UserName is the impersonated human subject for Impersonate-User.
	UserName string

	// Groups forwarded from the validated token's groups claim.
	Groups []string

	// AllowedTargetClusters from the matched TrustedIssuerConfig.
	// Empty means any cluster is permitted.
	AllowedTargetClusters []string
}

// ImpersonationClientFactory creates Kubernetes clients that authenticate as
// the server's in-cluster ServiceAccount and impersonate the provided identity.
type ImpersonationClientFactory interface {
	CreateImpersonationClient(identity ImpersonationIdentity) (Client, error)
}
