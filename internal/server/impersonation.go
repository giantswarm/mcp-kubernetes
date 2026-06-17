package server

import (
	"context"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
)

type impersonationContextKey struct{}

// ContextWithImpersonationIdentity stores an ImpersonationIdentity in ctx.
func ContextWithImpersonationIdentity(ctx context.Context, identity k8s.ImpersonationIdentity) context.Context {
	return context.WithValue(ctx, impersonationContextKey{}, identity)
}

// ImpersonationIdentityFromContext retrieves the ImpersonationIdentity stored
// by ContextWithImpersonationIdentity. Returns the zero value and false if absent.
func ImpersonationIdentityFromContext(ctx context.Context) (k8s.ImpersonationIdentity, bool) {
	v, ok := ctx.Value(impersonationContextKey{}).(k8s.ImpersonationIdentity)
	return v, ok
}
