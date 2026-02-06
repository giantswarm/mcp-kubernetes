// credential_mode.go defines the credential modes that determine how the Manager
// resolves clients for CAPI cluster discovery and kubeconfig secret access.
package federation

import "fmt"

// CredentialMode describes how the Manager authenticates for CAPI discovery
// and kubeconfig secret retrieval.
//
// The mode is resolved once at Manager construction from the ClientProvider
// configuration and remains constant for the Manager's lifetime.
//
// # Three Credential Modes
//
// The Manager supports three credential configurations:
//
//	┌─────────────────────────────────┬───────────────────┬───────────────────┐
//	│ Mode                            │ CAPI Discovery    │ Secret Access     │
//	├─────────────────────────────────┼───────────────────┼───────────────────┤
//	│ CredentialModeUser              │ User RBAC         │ User RBAC         │
//	│ CredentialModeFullPrivileged    │ ServiceAccount    │ ServiceAccount    │
//	│ CredentialModePrivilegedSecrets │ User RBAC         │ ServiceAccount    │
//	└─────────────────────────────────┴───────────────────┴───────────────────┘
//
// # Runtime Fallback
//
// When the mode is CredentialModeFullPrivileged or CredentialModePrivilegedSecrets,
// the privileged (ServiceAccount) client may fail to initialize at runtime (e.g.,
// the pod is not running in a cluster). In that case, the Manager falls back to
// user credentials unless strict mode is enabled, in which case it returns an error.
//
// # Configuration
//
// The mode is determined by the ClientProvider passed to NewManager:
//
//   - StaticClientProvider (or any basic ClientProvider): CredentialModeUser
//   - PrivilegedSecretAccessProvider with PrivilegedCAPIDiscovery() == true:
//     CredentialModeFullPrivileged
//   - PrivilegedSecretAccessProvider with PrivilegedCAPIDiscovery() == false:
//     CredentialModePrivilegedSecrets
type CredentialMode int

const (
	// CredentialModeUser uses user RBAC for both CAPI discovery and secret access.
	// This is the default when the ClientProvider does not implement
	// PrivilegedSecretAccessProvider.
	//
	// Requirements: Users must have RBAC to list CAPI clusters and read kubeconfig secrets.
	CredentialModeUser CredentialMode = iota

	// CredentialModeFullPrivileged uses ServiceAccount credentials for both
	// CAPI discovery and secret access.
	//
	// This is the recommended production configuration. Users do not need any
	// cluster-scoped CAPI permissions or secret read permissions. All access
	// to workload clusters is enforced via impersonation.
	CredentialModeFullPrivileged

	// CredentialModePrivilegedSecrets uses ServiceAccount credentials for secret
	// access but user RBAC for CAPI discovery.
	//
	// This is a hybrid mode: users must have RBAC to list CAPI clusters, but they
	// do not need secret read permissions. Use this when you want users to only see
	// clusters they have explicit RBAC to list, while still preventing direct
	// kubeconfig secret access.
	CredentialModePrivilegedSecrets
)

// String returns a human-readable name for the credential mode.
func (m CredentialMode) String() string {
	switch m {
	case CredentialModeUser:
		return "user"
	case CredentialModeFullPrivileged:
		return "full-privileged"
	case CredentialModePrivilegedSecrets:
		return "privileged-secrets-only"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// resolveCredentialMode determines the credential mode from the ClientProvider
// configuration. This is called once during Manager construction.
//
// The resolution logic:
//  1. If the provider does not implement PrivilegedSecretAccessProvider → CredentialModeUser
//  2. If it does and PrivilegedCAPIDiscovery() is true → CredentialModeFullPrivileged
//  3. If it does and PrivilegedCAPIDiscovery() is false → CredentialModePrivilegedSecrets
func resolveCredentialMode(provider ClientProvider) (CredentialMode, PrivilegedSecretAccessProvider) {
	privProvider, ok := provider.(PrivilegedSecretAccessProvider)
	if !ok {
		return CredentialModeUser, nil
	}

	if privProvider.PrivilegedCAPIDiscovery() {
		return CredentialModeFullPrivileged, privProvider
	}

	return CredentialModePrivilegedSecrets, privProvider
}
