package security

import (
	"errors"
	"fmt"
)

// Common security errors
var (
	// ErrInvalidRequest indicates the operation request is invalid or malformed
	ErrInvalidRequest = errors.New("invalid operation request")

	// ErrMissingOperation indicates no operation type was specified
	ErrMissingOperation = errors.New("operation type is required")

	// ErrUnauthorized indicates the operation is not authorized
	ErrUnauthorized = errors.New("operation not authorized")

	// ErrForbidden indicates access to the resource is forbidden
	ErrForbidden = errors.New("access forbidden")

	// ErrInvalidCredentials indicates authentication credentials are invalid
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrCredentialsMissing indicates required credentials are missing
	ErrCredentialsMissing = errors.New("credentials required but not provided")
)

// SecurityError represents a security-related error with additional context
type SecurityError struct {
	Type     string                 `json:"type"`
	Resource string                 `json:"resource,omitempty"`
	Value    string                 `json:"value,omitempty"`
	Reason   string                 `json:"reason"`
	Details  map[string]interface{} `json:"details,omitempty"`
	Err      error                  `json:"-"`
}

// Error implements the error interface
func (e *SecurityError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("security error: %s %s '%s': %s (%v)", e.Type, e.Resource, e.Value, e.Reason, e.Err)
	}
	return fmt.Sprintf("security error: %s %s '%s': %s", e.Type, e.Resource, e.Value, e.Reason)
}

// Unwrap implements the unwrapping interface for error chains
func (e *SecurityError) Unwrap() error {
	return e.Err
}

// NewForbiddenError creates a new forbidden access error
func NewForbiddenError(resource, value, reason string) *SecurityError {
	return &SecurityError{
		Type:     "forbidden",
		Resource: resource,
		Value:    value,
		Reason:   reason,
		Err:      ErrForbidden,
	}
}

// NewUnauthorizedError creates a new unauthorized operation error
func NewUnauthorizedError(resource, value, reason string) *SecurityError {
	return &SecurityError{
		Type:     "unauthorized",
		Resource: resource,
		Value:    value,
		Reason:   reason,
		Err:      ErrUnauthorized,
	}
}

// NewCredentialsError creates a new credentials-related error
func NewCredentialsError(reason string, err error) *SecurityError {
	return &SecurityError{
		Type:   "credentials",
		Reason: reason,
		Err:    err,
	}
}

// NewValidationError creates a new validation error
func NewValidationError(resource, value, reason string, err error) *SecurityError {
	return &SecurityError{
		Type:     "validation",
		Resource: resource,
		Value:    value,
		Reason:   reason,
		Err:      err,
	}
}

// IsForbiddenError checks if an error is a forbidden access error
func IsForbiddenError(err error) bool {
	var secErr *SecurityError
	return errors.As(err, &secErr) && secErr.Type == "forbidden"
}

// IsUnauthorizedError checks if an error is an unauthorized operation error
func IsUnauthorizedError(err error) bool {
	var secErr *SecurityError
	return errors.As(err, &secErr) && secErr.Type == "unauthorized"
}

// IsCredentialsError checks if an error is a credentials-related error
func IsCredentialsError(err error) bool {
	var secErr *SecurityError
	return errors.As(err, &secErr) && secErr.Type == "credentials"
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
	var secErr *SecurityError
	return errors.As(err, &secErr) && secErr.Type == "validation"
}

// SanitizeError removes sensitive information from error messages
func SanitizeError(err error) error {
	if err == nil {
		return nil
	}

	var secErr *SecurityError
	if errors.As(err, &secErr) {
		// Create a sanitized copy without sensitive details
		sanitized := &SecurityError{
			Type:     secErr.Type,
			Resource: secErr.Resource,
			Reason:   secErr.Reason,
		}

		// Don't include the original value or detailed error information
		// that might contain sensitive data
		if secErr.Type == "credentials" {
			sanitized.Reason = "authentication failed"
		}

		return sanitized
	}

	// For non-security errors, return a generic error message
	return errors.New("operation failed")
}
