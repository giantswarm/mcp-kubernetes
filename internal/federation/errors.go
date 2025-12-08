package federation

import (
	"errors"
	"fmt"
	"strings"
	"time"
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

	// ErrAccessDenied indicates that the user does not have permission to
	// perform the requested operation. This is returned after a SubjectAccessReview
	// determines the user lacks the required RBAC permissions.
	ErrAccessDenied = errors.New("access denied")

	// ErrAccessCheckFailed indicates that the access check itself failed
	// (e.g., due to API server errors), not that access was denied.
	ErrAccessCheckFailed = errors.New("access check failed")

	// ErrInvalidAccessCheck indicates that the AccessCheck parameters are invalid.
	ErrInvalidAccessCheck = errors.New("invalid access check parameters")

	// ErrClusterUnreachable indicates that the cluster API server is not reachable.
	// This typically occurs due to network issues such as:
	//   - VPC peering not configured
	//   - Security group rules blocking access
	//   - DNS resolution failures
	ErrClusterUnreachable = errors.New("cluster unreachable")

	// ErrTLSHandshakeFailed indicates that the TLS handshake with the cluster failed.
	// Common causes include:
	//   - Certificate signed by unknown authority
	//   - Expired certificate
	//   - Certificate hostname mismatch
	ErrTLSHandshakeFailed = errors.New("TLS handshake failed")

	// ErrConnectionTimeout indicates that the connection to the cluster timed out.
	// This can happen when:
	//   - The cluster is behind a firewall
	//   - Network latency is too high
	//   - The cluster is not running
	ErrConnectionTimeout = errors.New("connection timeout")
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

// ImpersonationError provides detailed context about impersonation failures.
// This error is returned when the MCP server cannot impersonate a user on a
// target cluster, typically due to RBAC configuration issues.
//
// # Common Causes
//
// 1. Missing impersonation RBAC permissions on the workload cluster:
//
//	The admin credentials used by the MCP server need permission to
//	impersonate users and groups on the target cluster.
//
// 2. Invalid user identity data:
//
//	The OAuth-derived user info contains data that cannot be used
//	for impersonation (e.g., malformed email, invalid group names).
//
// 3. Cluster API server rejecting impersonation:
//
//	The workload cluster's API server may have policies that prevent
//	impersonation of certain users or groups.
type ImpersonationError struct {
	// ClusterName is the target cluster where impersonation failed.
	ClusterName string

	// UserEmail is the email of the user being impersonated (for logging only).
	UserEmail string

	// GroupCount is the number of groups in the impersonation request.
	GroupCount int

	// Reason describes what went wrong.
	Reason string

	// Err is the underlying error that caused the failure.
	Err error
}

// Error implements the error interface.
func (e *ImpersonationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("impersonation failed for cluster %q (user %s, %d groups): %s: %v",
			e.ClusterName, AnonymizeEmail(e.UserEmail), e.GroupCount, e.Reason, e.Err)
	}
	return fmt.Sprintf("impersonation failed for cluster %q (user %s, %d groups): %s",
		e.ClusterName, AnonymizeEmail(e.UserEmail), e.GroupCount, e.Reason)
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *ImpersonationError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
// This allows ImpersonationError to match against ErrImpersonationFailed.
func (e *ImpersonationError) Is(target error) bool {
	return target == ErrImpersonationFailed
}

// UserFacingError returns a sanitized error message safe for end users.
// This provides actionable guidance without exposing internal details.
//
// Unlike cluster-related errors that use a generic message to prevent
// enumeration, impersonation errors indicate a configuration issue that
// the user's administrator needs to address.
func (e *ImpersonationError) UserFacingError() string {
	return "insufficient permissions to access this cluster - please contact your administrator to verify your RBAC configuration"
}

// AccessDeniedError provides detailed context about a permission denial.
// This error is returned when a SubjectAccessReview determines the user
// lacks permission to perform an operation.
//
// # Usage
//
// AccessDeniedError provides actionable information about what permission is missing:
//
//	if errors.Is(err, federation.ErrAccessDenied) {
//		var accessErr *federation.AccessDeniedError
//		if errors.As(err, &accessErr) {
//			fmt.Printf("You need %s permission on %s/%s in namespace %s\n",
//				accessErr.Verb, accessErr.APIGroup, accessErr.Resource, accessErr.Namespace)
//		}
//	}
type AccessDeniedError struct {
	// ClusterName is the cluster where the permission check was performed.
	ClusterName string

	// UserEmail is the email of the user (for logging only, anonymized in Error()).
	UserEmail string

	// Verb is the action that was denied (e.g., "delete", "create").
	Verb string

	// Resource is the resource type for which access was denied.
	Resource string

	// APIGroup is the API group of the resource.
	APIGroup string

	// Namespace is the namespace where access was denied (empty for cluster-scoped).
	Namespace string

	// Name is the specific resource name if checked (empty for type-level checks).
	Name string

	// Reason provides details about why access was denied (from Kubernetes).
	Reason string
}

// Error implements the error interface.
func (e *AccessDeniedError) Error() string {
	var resource string
	if e.APIGroup != "" {
		resource = e.APIGroup + "/" + e.Resource
	} else {
		resource = e.Resource
	}

	location := "cluster-wide"
	if e.Namespace != "" {
		location = "namespace " + e.Namespace
	}

	target := resource
	if e.Name != "" {
		target = resource + "/" + e.Name
	}

	return fmt.Sprintf("access denied: user %s cannot %s %s in %s on cluster %q: %s",
		AnonymizeEmail(e.UserEmail), e.Verb, target, location, e.ClusterName, e.Reason)
}

// Unwrap returns nil as there is no underlying error.
func (e *AccessDeniedError) Unwrap() error {
	return nil
}

// Is implements custom error matching for errors.Is().
// This allows AccessDeniedError to match against ErrAccessDenied.
func (e *AccessDeniedError) Is(target error) bool {
	return target == ErrAccessDenied
}

// UserFacingError returns a message suitable for displaying to end users.
// This provides enough context for the user to understand what permission
// they need without exposing internal system details.
func (e *AccessDeniedError) UserFacingError() string {
	var resource string
	if e.APIGroup != "" {
		resource = e.APIGroup + "/" + e.Resource
	} else {
		resource = e.Resource
	}

	target := resource
	if e.Name != "" {
		target = resource + "/" + e.Name
	}

	location := ""
	if e.Namespace != "" {
		location = fmt.Sprintf(" in namespace %q", e.Namespace)
	}

	return fmt.Sprintf("permission denied: you cannot %s %s%s - please contact your administrator to request access",
		e.Verb, target, location)
}

// AccessCheckError provides context when the access check itself fails.
// This is different from AccessDeniedError: it means we couldn't determine
// whether access is allowed, not that access is denied.
type AccessCheckError struct {
	// ClusterName is the cluster where the check was attempted.
	ClusterName string

	// Check contains the access check parameters.
	Check *AccessCheck

	// Reason describes what went wrong during the check.
	Reason string

	// Err is the underlying error.
	Err error
}

// Error implements the error interface.
func (e *AccessCheckError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("access check failed for cluster %q (%s %s): %s: %v",
			e.ClusterName, e.Check.Verb, e.Check.Resource, e.Reason, e.Err)
	}
	return fmt.Sprintf("access check failed for cluster %q (%s %s): %s",
		e.ClusterName, e.Check.Verb, e.Check.Resource, e.Reason)
}

// Unwrap returns the underlying error.
func (e *AccessCheckError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
func (e *AccessCheckError) Is(target error) bool {
	return target == ErrAccessCheckFailed
}

// UserFacingError returns a message suitable for displaying to end users.
func (e *AccessCheckError) UserFacingError() string {
	return "unable to verify permissions - please try again or contact your administrator"
}

// ConnectivityTimeoutError provides detailed context about a connection timeout.
// This error indicates that the TCP connection or HTTP request timed out before
// completing. It's typically caused by network issues such as:
//   - Firewall rules blocking the connection
//   - No route to the target network
//   - High network latency
//   - Target cluster not running
//
// # Troubleshooting
//
// When encountering this error, verify:
//  1. VPC peering or Transit Gateway is properly configured
//  2. Security group rules allow traffic on port 6443
//  3. The target cluster is healthy and running
//  4. DNS resolution is working correctly
type ConnectivityTimeoutError struct {
	// ClusterName is the target cluster that timed out.
	ClusterName string

	// Host is the API server endpoint that couldn't be reached.
	Host string

	// Timeout is the duration waited before giving up (if known).
	Timeout time.Duration

	// Err is the underlying error that caused the timeout.
	Err error
}

// Error implements the error interface.
func (e *ConnectivityTimeoutError) Error() string {
	if e.Timeout > 0 {
		return fmt.Sprintf("connection to cluster %q (%s) timed out after %s",
			e.ClusterName, e.Host, e.Timeout)
	}
	if e.Err != nil {
		return fmt.Sprintf("connection to cluster %q (%s) timed out: %v",
			e.ClusterName, e.Host, e.Err)
	}
	return fmt.Sprintf("connection to cluster %q (%s) timed out", e.ClusterName, e.Host)
}

// Unwrap returns the underlying error.
func (e *ConnectivityTimeoutError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
func (e *ConnectivityTimeoutError) Is(target error) bool {
	switch target {
	case ErrConnectionTimeout:
		return true
	case ErrClusterUnreachable:
		return true
	case ErrConnectionFailed:
		return true
	}
	return false
}

// UserFacingError returns a message suitable for displaying to end users.
// This provides actionable guidance without exposing internal network details.
func (e *ConnectivityTimeoutError) UserFacingError() string {
	return "connection to cluster timed out - please verify the cluster is reachable from the management cluster"
}

// TLSError provides detailed context about a TLS/certificate failure.
// This error indicates that the TLS handshake failed, which can happen due to:
//   - Certificate signed by unknown authority
//   - Expired certificate
//   - Certificate hostname mismatch
//   - TLS protocol version mismatch
//
// # Security Note
//
// TLS errors should NOT be bypassed by disabling certificate verification.
// Instead, ensure the CA certificate is properly configured in the kubeconfig.
//
// # Troubleshooting
//
// When encountering this error:
//  1. Verify the kubeconfig contains the correct CA certificate
//  2. Check if the cluster certificate has expired
//  3. Ensure the certificate SANs include the endpoint hostname/IP
type TLSError struct {
	// ClusterName is the target cluster where TLS failed.
	ClusterName string

	// Host is the API server endpoint.
	Host string

	// Reason describes what went wrong in the TLS handshake.
	Reason string

	// Err is the underlying TLS error.
	Err error
}

// Error implements the error interface.
func (e *TLSError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("TLS handshake with cluster %q (%s) failed: %s: %v",
			e.ClusterName, e.Host, e.Reason, e.Err)
	}
	return fmt.Sprintf("TLS handshake with cluster %q (%s) failed: %s",
		e.ClusterName, e.Host, e.Reason)
}

// Unwrap returns the underlying error.
func (e *TLSError) Unwrap() error {
	return e.Err
}

// Is implements custom error matching for errors.Is().
func (e *TLSError) Is(target error) bool {
	switch target {
	case ErrTLSHandshakeFailed:
		return true
	case ErrConnectionFailed:
		return true
	}
	return false
}

// UserFacingError returns a message suitable for displaying to end users.
// This provides actionable guidance while maintaining security (not suggesting
// to bypass certificate verification).
func (e *TLSError) UserFacingError() string {
	switch {
	case strings.Contains(e.Reason, "expired"):
		return "cluster certificate has expired - please contact your administrator to renew the certificate"
	case strings.Contains(e.Reason, "unknown authority"):
		return "cluster certificate not trusted - please verify the kubeconfig contains the correct CA certificate"
	case strings.Contains(e.Reason, "mismatch") || strings.Contains(e.Reason, "doesn't match"):
		return "cluster certificate doesn't match the hostname - please contact your administrator"
	default:
		return "secure connection to cluster failed - please contact your administrator"
	}
}
