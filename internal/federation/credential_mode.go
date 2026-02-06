// credential_mode.go defines the credential modes that determine how the Manager
// resolves clients for CAPI cluster discovery and kubeconfig secret access.
package federation

import "fmt"

// CredentialMode describes how the Manager authenticates for Management Cluster
// operations: CAPI cluster discovery and kubeconfig secret/ConfigMap retrieval.
//
// The mode is resolved once at Manager construction from the ClientProvider
// configuration and remains constant for the Manager's lifetime.
//
// # Two Orthogonal Axes
//
// The Manager's authentication model has two independent dimensions:
//
//  1. CredentialMode (this type): Controls how the Manager authenticates
//     to the Management Cluster for CAPI discovery and kubeconfig/CA access.
//     This determines WHICH credentials are used to read MC resources.
//
//  2. WorkloadClusterAuthMode: Controls how the Manager authenticates
//     to Workload Cluster API servers after discovery.
//     This determines HOW users access WC resources.
//
// These axes are orthogonal -- any CredentialMode can be combined with any
// WorkloadClusterAuthMode:
//
//	┌─────────────────────────────────┬────────────────────────────┬────────────────────────────┐
//	│                                 │ WC Auth: Impersonation     │ WC Auth: SSO Passthrough   │
//	├─────────────────────────────────┼────────────────────────────┼────────────────────────────┤
//	│ CredentialModeUser              │ User RBAC → admin creds    │ User RBAC → SSO token      │
//	│                                 │ + impersonation headers    │ forwarded to WC             │
//	├─────────────────────────────────┼────────────────────────────┼────────────────────────────┤
//	│ CredentialModeFullPrivileged    │ SA creds → admin creds     │ SA creds → SSO token       │
//	│                                 │ + impersonation headers    │ forwarded to WC             │
//	├─────────────────────────────────┼────────────────────────────┼────────────────────────────┤
//	│ CredentialModePrivilegedSecrets │ User RBAC disc → SA creds  │ User RBAC disc → SSO token │
//	│                                 │ + impersonation headers    │ forwarded to WC             │
//	└─────────────────────────────────┴────────────────────────────┴────────────────────────────┘
//
// Key differences between the two WC auth modes after MC discovery:
//   - Impersonation: reads kubeconfig Secrets (admin credentials), creates WC
//     clients with admin creds + Impersonate-User/Group headers
//   - SSO Passthrough: reads CA ConfigMaps (public CA cert + endpoint), creates
//     WC clients with the user's SSO token as Bearer token
//
// # Three Credential Modes
//
// The Manager supports three credential configurations for MC access:
//
//	┌─────────────────────────────────┬───────────────────┬───────────────────┐
//	│ Mode                            │ CAPI Discovery    │ Secret Access     │
//	├─────────────────────────────────┼───────────────────┼───────────────────┤
//	│ CredentialModeUser              │ User RBAC         │ User RBAC         │
//	│ CredentialModeFullPrivileged    │ ServiceAccount    │ ServiceAccount    │
//	│ CredentialModePrivilegedSecrets │ User RBAC         │ ServiceAccount    │
//	└─────────────────────────────────┴───────────────────┴───────────────────┘
//
// Note: In SSO passthrough mode, "Secret Access" column applies to ConfigMap
// access for CA certificates. Since CA certs are public information, SSO
// passthrough always uses user credentials for this step regardless of mode.
// The CredentialMode only affects CAPI discovery in the SSO passthrough path.
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
// The mode is determined by the WithPrivilegedAccess option passed to NewManager:
//
//   - No WithPrivilegedAccess option: CredentialModeUser
//   - WithPrivilegedAccess where PrivilegedCAPIDiscovery() == true:
//     CredentialModeFullPrivileged
//   - WithPrivilegedAccess where PrivilegedCAPIDiscovery() == false:
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

// resolveCredentialMode determines the credential mode from the explicitly
// configured PrivilegedSecretAccessProvider. This is called once during
// Manager construction after options have been applied.
//
// The resolution logic:
//  1. If provider is nil (no WithPrivilegedAccess option) → CredentialModeUser
//  2. If provider.PrivilegedCAPIDiscovery() is true → CredentialModeFullPrivileged
//  3. If provider.PrivilegedCAPIDiscovery() is false → CredentialModePrivilegedSecrets
func resolveCredentialMode(provider PrivilegedSecretAccessProvider) CredentialMode {
	if provider == nil {
		return CredentialModeUser
	}

	if provider.PrivilegedCAPIDiscovery() {
		return CredentialModeFullPrivileged
	}

	return CredentialModePrivilegedSecrets
}
