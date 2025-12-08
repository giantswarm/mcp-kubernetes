package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode"
)

// Validation constants for security limits.
const (
	// MaxEmailLength is the maximum allowed length for an email address.
	MaxEmailLength = 254

	// MaxGroupNameLength is the maximum allowed length for a group name.
	MaxGroupNameLength = 256

	// MaxGroupCount is the maximum number of groups allowed per user.
	MaxGroupCount = 100

	// MaxExtraKeyLength is the maximum allowed length for an extra header key.
	MaxExtraKeyLength = 256

	// MaxExtraValueLength is the maximum allowed length for an extra header value.
	MaxExtraValueLength = 1024

	// MaxExtraCount is the maximum number of extra headers allowed.
	MaxExtraCount = 50

	// MaxClusterNameLength is the maximum allowed length for a cluster name.
	// Kubernetes names are limited to 253 characters.
	MaxClusterNameLength = 253
)

// Validation errors.
var (
	// ErrUserInfoRequired indicates that user information is required but was not provided.
	ErrUserInfoRequired = fmt.Errorf("user information is required for cluster operations")

	// ErrInvalidEmail indicates that the email address format is invalid.
	ErrInvalidEmail = fmt.Errorf("invalid email address format")

	// ErrInvalidGroupName indicates that a group name is invalid.
	ErrInvalidGroupName = fmt.Errorf("invalid group name")

	// ErrInvalidExtraHeader indicates that an extra header key or value is invalid.
	ErrInvalidExtraHeader = fmt.Errorf("invalid extra header")

	// ErrInvalidClusterName indicates that a cluster name is invalid.
	ErrInvalidClusterName = fmt.Errorf("invalid cluster name")
)

// ValidationError provides detailed context about a validation failure.
type ValidationError struct {
	Field  string
	Value  string // Sanitized value (may be truncated or anonymized)
	Reason string
	Err    error
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("validation failed for %s %q: %s", e.Field, e.Value, e.Reason)
	}
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Reason)
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *ValidationError) Unwrap() error {
	return e.Err
}

// UserFacingError returns a sanitized error message safe for end users.
func (e *ValidationError) UserFacingError() string {
	return fmt.Sprintf("invalid %s provided", e.Field)
}

// validClusterNameRegex matches valid Kubernetes resource names.
// Must start with lowercase alphanumeric, contain only lowercase alphanumeric or hyphens,
// and end with lowercase alphanumeric.
var validClusterNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validEmailRegex is a simplified email validation pattern.
// It's intentionally permissive to avoid false negatives while catching obvious issues.
var validEmailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// validHeaderKeyRegex matches valid HTTP header key characters.
// Header keys should only contain alphanumeric characters, hyphens, and underscores.
var validHeaderKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateUserInfo validates the UserInfo struct for security.
// Returns ErrUserInfoRequired if user is nil.
// Returns a ValidationError if any field fails validation.
func ValidateUserInfo(user *UserInfo) error {
	if user == nil {
		return ErrUserInfoRequired
	}

	// Validate email if provided
	if user.Email != "" {
		if err := validateEmail(user.Email); err != nil {
			return err
		}
	}

	// Validate groups
	if len(user.Groups) > MaxGroupCount {
		return &ValidationError{
			Field:  "groups",
			Reason: fmt.Sprintf("too many groups (max %d)", MaxGroupCount),
			Err:    ErrInvalidGroupName,
		}
	}

	for i, group := range user.Groups {
		if err := validateGroupName(group, i); err != nil {
			return err
		}
	}

	// Validate extra headers
	if len(user.Extra) > MaxExtraCount {
		return &ValidationError{
			Field:  "extra",
			Reason: fmt.Sprintf("too many extra headers (max %d)", MaxExtraCount),
			Err:    ErrInvalidExtraHeader,
		}
	}

	for key, values := range user.Extra {
		if err := validateExtraHeader(key, values); err != nil {
			return err
		}
	}

	return nil
}

// validateEmail validates an email address format.
func validateEmail(email string) error {
	if len(email) > MaxEmailLength {
		return &ValidationError{
			Field:  "email",
			Value:  truncateForError(email, 20),
			Reason: fmt.Sprintf("email too long (max %d characters)", MaxEmailLength),
			Err:    ErrInvalidEmail,
		}
	}

	if containsControlCharacters(email) {
		return &ValidationError{
			Field:  "email",
			Value:  truncateForError(email, 20),
			Reason: "email contains invalid control characters",
			Err:    ErrInvalidEmail,
		}
	}

	if !validEmailRegex.MatchString(email) {
		return &ValidationError{
			Field:  "email",
			Value:  truncateForError(email, 20),
			Reason: "email format is invalid",
			Err:    ErrInvalidEmail,
		}
	}

	return nil
}

// validateGroupName validates a single group name.
func validateGroupName(group string, index int) error {
	if group == "" {
		return &ValidationError{
			Field:  fmt.Sprintf("groups[%d]", index),
			Reason: "group name cannot be empty",
			Err:    ErrInvalidGroupName,
		}
	}

	if len(group) > MaxGroupNameLength {
		return &ValidationError{
			Field:  fmt.Sprintf("groups[%d]", index),
			Value:  truncateForError(group, 20),
			Reason: fmt.Sprintf("group name too long (max %d characters)", MaxGroupNameLength),
			Err:    ErrInvalidGroupName,
		}
	}

	if containsControlCharacters(group) {
		return &ValidationError{
			Field:  fmt.Sprintf("groups[%d]", index),
			Value:  truncateForError(group, 20),
			Reason: "group name contains invalid control characters",
			Err:    ErrInvalidGroupName,
		}
	}

	return nil
}

// validateExtraHeader validates an extra impersonation header key and values.
func validateExtraHeader(key string, values []string) error {
	if key == "" {
		return &ValidationError{
			Field:  "extra header key",
			Reason: "header key cannot be empty",
			Err:    ErrInvalidExtraHeader,
		}
	}

	if len(key) > MaxExtraKeyLength {
		return &ValidationError{
			Field:  "extra header key",
			Value:  truncateForError(key, 20),
			Reason: fmt.Sprintf("header key too long (max %d characters)", MaxExtraKeyLength),
			Err:    ErrInvalidExtraHeader,
		}
	}

	if !validHeaderKeyRegex.MatchString(key) {
		return &ValidationError{
			Field:  "extra header key",
			Value:  truncateForError(key, 20),
			Reason: "header key contains invalid characters (only alphanumeric, hyphen, underscore allowed)",
			Err:    ErrInvalidExtraHeader,
		}
	}

	for i, value := range values {
		if len(value) > MaxExtraValueLength {
			return &ValidationError{
				Field:  fmt.Sprintf("extra[%s][%d]", key, i),
				Value:  truncateForError(value, 20),
				Reason: fmt.Sprintf("header value too long (max %d characters)", MaxExtraValueLength),
				Err:    ErrInvalidExtraHeader,
			}
		}

		if containsControlCharacters(value) {
			return &ValidationError{
				Field:  fmt.Sprintf("extra[%s][%d]", key, i),
				Value:  truncateForError(value, 20),
				Reason: "header value contains invalid control characters",
				Err:    ErrInvalidExtraHeader,
			}
		}
	}

	return nil
}

// ValidateClusterName validates a cluster name against Kubernetes naming conventions.
func ValidateClusterName(name string) error {
	if name == "" {
		return &ValidationError{
			Field:  "cluster name",
			Reason: "cluster name cannot be empty",
			Err:    ErrInvalidClusterName,
		}
	}

	if len(name) > MaxClusterNameLength {
		return &ValidationError{
			Field:  "cluster name",
			Value:  truncateForError(name, 20),
			Reason: fmt.Sprintf("cluster name too long (max %d characters)", MaxClusterNameLength),
			Err:    ErrInvalidClusterName,
		}
	}

	// Check for path traversal attempts
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return &ValidationError{
			Field:  "cluster name",
			Value:  truncateForError(name, 20),
			Reason: "cluster name contains invalid path characters",
			Err:    ErrInvalidClusterName,
		}
	}

	if !validClusterNameRegex.MatchString(name) {
		return &ValidationError{
			Field:  "cluster name",
			Value:  truncateForError(name, 20),
			Reason: "cluster name must consist of lowercase alphanumeric characters or hyphens, start with alphanumeric, and end with alphanumeric",
			Err:    ErrInvalidClusterName,
		}
	}

	return nil
}

// containsControlCharacters checks if a string contains control characters.
func containsControlCharacters(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// truncateForError truncates a string for safe inclusion in error messages.
func truncateForError(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AnonymizeEmail returns a hashed representation of an email for logging purposes.
// This allows correlation of log entries without exposing PII.
func AnonymizeEmail(email string) string {
	if email == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(email))
	return "user:" + hex.EncodeToString(hash[:8])
}

// UserHashAttr returns a slog attribute with the anonymized user email.
// This is a convenience function to reduce repetition in logging calls and ensure
// consistent attribute naming across the codebase.
//
// Usage:
//
//	m.logger.Debug("Operation completed", UserHashAttr(user.Email))
func UserHashAttr(email string) slog.Attr {
	return slog.String("user_hash", AnonymizeEmail(email))
}

// AnonymizeUserInfo returns anonymized user identifiers for logging.
// Returns a map with "user_hash" and "group_count" for safe logging.
func AnonymizeUserInfo(user *UserInfo) map[string]interface{} {
	if user == nil {
		return map[string]interface{}{
			"user_hash":   "",
			"group_count": 0,
		}
	}
	return map[string]interface{}{
		"user_hash":   AnonymizeEmail(user.Email),
		"group_count": len(user.Groups),
	}
}
