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
)

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
func (e *ClusterNotFoundError) UserFacingError() string {
	return "cluster not found or access denied"
}

// KubeconfigError provides detailed context about kubeconfig retrieval failures.
type KubeconfigError struct {
	ClusterName string
	SecretName  string
	Namespace   string
	Reason      string
	Err         error
	// NotFound indicates the kubeconfig secret was not found (vs other errors like invalid data).
	// When true, Unwrap() returns ErrKubeconfigSecretNotFound instead of ErrKubeconfigInvalid.
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
// This allows KubeconfigError to match against both the underlying error
// and our sentinel errors (ErrKubeconfigSecretNotFound, ErrKubeconfigInvalid).
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
func (e *KubeconfigError) UserFacingError() string {
	if e.NotFound {
		return "cluster credentials not available"
	}
	return "cluster configuration error"
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
func (e *ConnectionError) UserFacingError() string {
	return "unable to connect to cluster"
}
