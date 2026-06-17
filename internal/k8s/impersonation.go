package k8s

// ImpersonationIdentity is the resolved identity for an external-issuer (M2M)
// token. The access-token injector middleware populates it and stores it in the
// request context. K8sClientForContext reads it to build an impersonation client
// instead of a bearer-token passthrough client.
type ImpersonationIdentity struct {
	// UserName is the Kubernetes subject for Impersonate-User.
	// Format: system:serviceaccount:kagent-<alias>:<saName>
	UserName string

	// Groups forwarded from the validated token's groups claim.
	Groups []string

	// Extra is sent verbatim as Impersonate-Extra-* headers.
	// Always contains "issuer" (raw issuer URL) and "agent" ("mcp-kubernetes").
	Extra map[string][]string

	// AllowedTargetClusters from the matched TrustedIssuerConfig.
	// Empty means any cluster is permitted.
	AllowedTargetClusters []string

	// Actor is reserved for the RFC 8693 OBO path (Phase 2).
	Actor string
}

// ImpersonationClientFactory creates Kubernetes clients that authenticate as
// the server's in-cluster ServiceAccount and impersonate the provided identity.
type ImpersonationClientFactory interface {
	CreateImpersonationClient(identity ImpersonationIdentity) (Client, error)
}
