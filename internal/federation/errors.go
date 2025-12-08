package federation

import (
	"errors"
	"fmt"
)

// Sentinel errors for common federation failure scenarios.
// These errors can be checked using errors.Is() for programmatic error handling.
var (
	// ErrClusterNotFound indicates that the requested cluster does not exist
	// or the user does not have permission to access it.
	ErrClusterNotFound = errors.New("cluster not found")

	// ErrKubeconfigSecretNotFound indicates that the CAPI kubeconfig secret
	// for the cluster is missing from the Management Cluster.
	ErrKubeconfigSecretNotFound = errors.New("kubeconfig secret not found")

	// ErrKubeconfigInvalid indicates that the kubeconfig secret contains
	// malformed or unparseable kubeconfig data.
	ErrKubeconfigInvalid = errors.New("kubeconfig data is invalid")

	// ErrConnectionFailed indicates a network or TLS error when attempting
	// to connect to the target cluster.
	ErrConnectionFailed = errors.New("failed to connect to cluster")

	// ErrImpersonationFailed indicates that the user impersonation could not
	// be configured on the cluster client.
	ErrImpersonationFailed = errors.New("failed to configure user impersonation")

	// ErrManagerClosed indicates that the ClusterClientManager has been closed
	// and can no longer be used.
	ErrManagerClosed = errors.New("federation manager is closed")

	// ErrUserEmailRequired indicates that the user's email is required but not present.
	// The email is used as the Impersonate-User header value for Kubernetes RBAC.
	ErrUserEmailRequired = errors.New("user email is required for impersonation")
)

// userFacingClusterError is the standardized message returned to users for all
// cluster-related errors. Using a single message prevents error response
// differentiation attacks that could leak cluster existence information.
const userFacingClusterError = "cluster access denied or unavailable"

// ClusterNotFoundError provides detailed context about a cluster lookup failure.
type ClusterNotFoundError struct {
	ClusterName string
	Namespace   string
	Reason      string
}

// Error implements the error interface.
func (e *ClusterNotFoundError) Error() string {
	if e.Namespace != "" {
		return fmt.Sprintf("cluster %q not found in namespace %q: %s", e.ClusterName, e.Namespace, e.Reason)
	}
	return fmt.Sprintf("cluster %q not found: %s", e.ClusterName, e.Reason)
}

// Unwrap returns the underlying sentinel error for use with errors.Is().
func (e *ClusterNotFoundError) Unwrap() error {
	return ErrClusterNotFound
}

// UserFacingError returns a sanitized error message safe for end users.
// This prevents leaking internal cluster names and namespace structure.
//
// Security: Returns a generic message that doesn't reveal whether the cluster
// exists, preventing cluster enumeration attacks.
func (e *ClusterNotFoundError) UserFacingError() string {
	return userFacingClusterError
}

// KubeconfigError provides detailed context about kubeconfig retrieval failures.
//
// # Error Matching Semantics
//
// This error type implements both Is() and Unwrap() with distinct behaviors:
//
//   - Is() matches against sentinel errors (ErrKubeconfigSecretNotFound, ErrKubeconfigInvalid)
//     based on the NotFound field. This allows callers to use errors.Is() to distinguish
//     between "secret not found" and "secret found but invalid" scenarios.
//
//   - Unwrap() returns the underlying cause (Err field), allowing errors.Is() to also match
//     against the root cause (e.g., a Kubernetes API error).
//
// Example usage:
//
//	if errors.Is(err, federation.ErrKubeconfigSecretNotFound) {
//	    // Handle missing secret
//	} else if errors.Is(err, federation.ErrKubeconfigInvalid) {
//	    // Handle malformed kubeconfig
//	}
type KubeconfigError struct {
	ClusterName string
	SecretName  string
	Namespace   string
	Reason      string
	Err         error
	// NotFound indicates the kubeconfig secret was not found (vs other errors like invalid data).
	// When true, Is() matches ErrKubeconfigSecretNotFound; otherwise it matches ErrKubeconfigInvalid.
	NotFound bool
}

// Error implements the error interface.
func (e *KubeconfigError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("kubeconfig error for cluster %q (secret %s/%s): %s: %v",
			e.ClusterName, e.Namespace, e.SecretName, e.Reason, e.Err)
	}
	return fmt.Sprintf("kubeconfig error for cluster %q (secret %s/%s): %s",
		e.ClusterName, e.Namespace, e.SecretName, e.Reason)
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *KubeconfigError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
// This allows KubeconfigError to match against our sentinel errors:
//   - ErrKubeconfigSecretNotFound: matches when NotFound is true
//   - ErrKubeconfigInvalid: matches when NotFound is false (i.e., the secret
//     exists but contains invalid data, missing keys, or unparseable content)
//
// Note: The underlying error (Err field) is matched via Unwrap(), not Is().
func (e *KubeconfigError) Is(target error) bool {
	switch target {
	case ErrKubeconfigSecretNotFound:
		return e.NotFound
	case ErrKubeconfigInvalid:
		return !e.NotFound
	}
	return false
}

// UserFacingError returns a sanitized error message safe for end users.
// This prevents leaking internal secret names and namespace structure.
//
// Security: Returns a generic message regardless of whether the secret was
// not found vs. invalid data. This prevents attackers from determining
// cluster existence based on error response differentiation.
func (e *KubeconfigError) UserFacingError() string {
	// Always return the same message to prevent cluster existence leakage
	return userFacingClusterError
}

// ConnectionError provides detailed context about cluster connection failures.
type ConnectionError struct {
	ClusterName string
	Host        string
	Reason      string
	Err         error
}

// Error implements the error interface.
func (e *ConnectionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("connection to cluster %q (%s) failed: %s: %v",
			e.ClusterName, e.Host, e.Reason, e.Err)
	}
	return fmt.Sprintf("connection to cluster %q (%s) failed: %s",
		e.ClusterName, e.Host, e.Reason)
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
// This allows ConnectionError to match against ErrConnectionFailed.
func (e *ConnectionError) Is(target error) bool {
	return target == ErrConnectionFailed
}

// UserFacingError returns a sanitized error message safe for end users.
// This prevents leaking internal host URLs and network topology.
//
// Security: Returns a generic message consistent with other cluster errors
// to prevent error response differentiation attacks.
func (e *ConnectionError) UserFacingError() string {
	return userFacingClusterError
}
