package instrumentation

import (
	"context"
	"errors"
	"net"
	"syscall"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

// Error classes recorded as the error_class label of a failed operation.
//
// The set is fixed: the class is derived from the API status reason or the
// transport error, never from the error text, so the label's cardinality is
// independent of callers, namespaces, resources and clusters.
//
// The caller classes say the caller was refused or asked for something that
// cannot be served; with downstream OAuth every operation runs as the caller,
// so they are the caller's RBAC, input or timing, not the platform's health.
// The platform classes say the API server or the path to it failed.
const (
	// Caller classes.
	ErrorClassForbidden    = "forbidden"    // 403: RBAC denied the caller
	ErrorClassUnauthorized = "unauthorized" // 401: the caller's credentials were rejected
	ErrorClassNotFound     = "not_found"    // 404, 410, or a resource type the cluster does not serve
	ErrorClassConflict     = "conflict"     // 409: already exists or a stale resourceVersion
	ErrorClassInvalid      = "invalid"      // 400, 405, 406, 413, 415, 422: the request itself is wrong
	ErrorClassCanceled     = "canceled"     // the caller went away before the answer arrived

	// Platform classes.
	ErrorClassTimeout     = "timeout"      // 504, a server timeout, or a deadline exceeded on the way
	ErrorClassUnavailable = "unavailable"  // 503, 429, or the API server could not be reached
	ErrorClassServerError = "server_error" // any other 5xx
	ErrorClassOther       = "other"        // everything else
)

// ErrorClasses lists every value ErrorClass returns, caller classes first.
var ErrorClasses = []string{
	ErrorClassForbidden,
	ErrorClassUnauthorized,
	ErrorClassNotFound,
	ErrorClassConflict,
	ErrorClassInvalid,
	ErrorClassCanceled,
	ErrorClassTimeout,
	ErrorClassUnavailable,
	ErrorClassServerError,
	ErrorClassOther,
}

// ErrorClass maps an operation's error to one of the fixed ErrorClasses.
// It returns "" for a nil error.
func ErrorClass(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case apierrors.IsForbidden(err):
		return ErrorClassForbidden
	case apierrors.IsUnauthorized(err):
		return ErrorClassUnauthorized
	case apierrors.IsNotFound(err), apierrors.IsGone(err), apierrors.IsResourceExpired(err), meta.IsNoMatchError(err):
		return ErrorClassNotFound
	case apierrors.IsConflict(err), apierrors.IsAlreadyExists(err):
		return ErrorClassConflict
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err), apierrors.IsMethodNotSupported(err),
		apierrors.IsNotAcceptable(err), apierrors.IsRequestEntityTooLargeError(err),
		apierrors.IsUnsupportedMediaType(err):
		return ErrorClassInvalid
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return ErrorClassTimeout
	case apierrors.IsServiceUnavailable(err), apierrors.IsTooManyRequests(err):
		return ErrorClassUnavailable
	case apierrors.IsInternalError(err), apierrors.IsUnexpectedServerError(err), isServerErrorStatus(err):
		return ErrorClassServerError
	case errors.Is(err, context.Canceled):
		return ErrorClassCanceled
	case errors.Is(err, context.DeadlineExceeded), isNetTimeout(err):
		return ErrorClassTimeout
	case isUnreachable(err):
		return ErrorClassUnavailable
	default:
		return ErrorClassOther
	}
}

// isServerErrorStatus reports an API status error with a 5xx code the
// reasons above do not name.
func isServerErrorStatus(err error) bool {
	var status apierrors.APIStatus
	if !errors.As(err, &status) {
		return false
	}
	code := status.Status().Code
	return code >= 500 && code <= 599
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isUnreachable reports a transport failure on the way to the API server:
// a refused or reset connection, a failed dial or name lookup.
func isUnreachable(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
