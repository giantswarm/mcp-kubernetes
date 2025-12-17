package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
)

// Common log attribute keys for consistent naming across the codebase.
const (
	KeyOperation    = "operation"
	KeyNamespace    = "namespace"
	KeyResourceType = "resource_type"
	KeyResourceName = "resource_name"
	KeyCluster      = "cluster"
	KeyUserHash     = "user_hash"
	KeyDuration     = "duration"
	KeyStatus       = "status"
	KeyError        = "error"
	KeyHost         = "host"
	KeyTool         = "tool"
)

// Status values for consistent logging.
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// ipv4Regex matches IPv4 addresses for sanitization.
var ipv4Regex = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)

// ipv6Regex matches IPv6 addresses for sanitization.
// This regex matches common IPv6 formats including:
// - Full form: 2001:0db8:85a3:0000:0000:8a2e:0370:7334
// - Compressed form: 2001:db8:85a3::8a2e:370:7334
// - Bracketed form (used in URLs): [2001:db8::1]
var ipv6Regex = regexp.MustCompile(`\[?([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}\]?`)

// WithOperation returns a logger with the operation attribute set.
func WithOperation(logger *slog.Logger, operation string) *slog.Logger {
	return logger.With(slog.String(KeyOperation, operation))
}

// WithTool returns a logger with the tool attribute set.
func WithTool(logger *slog.Logger, tool string) *slog.Logger {
	return logger.With(slog.String(KeyTool, tool))
}

// WithCluster returns a logger with the cluster attribute set.
func WithCluster(logger *slog.Logger, cluster string) *slog.Logger {
	return logger.With(slog.String(KeyCluster, cluster))
}

// Operation returns a slog attribute for the operation name.
func Operation(op string) slog.Attr {
	return slog.String(KeyOperation, op)
}

// Namespace returns a slog attribute for the namespace.
func Namespace(ns string) slog.Attr {
	return slog.String(KeyNamespace, ns)
}

// ResourceType returns a slog attribute for the resource type.
func ResourceType(rt string) slog.Attr {
	return slog.String(KeyResourceType, rt)
}

// ResourceName returns a slog attribute for the resource name.
func ResourceName(name string) slog.Attr {
	return slog.String(KeyResourceName, name)
}

// Cluster returns a slog attribute for the cluster name.
func Cluster(name string) slog.Attr {
	return slog.String(KeyCluster, name)
}

// Status returns a slog attribute for the status.
func Status(status string) slog.Attr {
	return slog.String(KeyStatus, status)
}

// Err returns a slog attribute for an error.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.String(KeyError, "")
	}
	return slog.String(KeyError, err.Error())
}

// SanitizedErr returns a slog attribute for an error with IP addresses redacted.
// This should be used when logging errors that may contain hostnames or IP addresses
// from Kubernetes API server responses, which could leak network topology information.
func SanitizedErr(err error) slog.Attr {
	if err == nil {
		return slog.String(KeyError, "")
	}
	sanitized := SanitizeHost(err.Error())
	return slog.String(KeyError, sanitized)
}

// Host returns a slog attribute for a host with IP addresses sanitized.
func Host(host string) slog.Attr {
	return slog.String(KeyHost, SanitizeHost(host))
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

// UserHash returns a slog attribute with the anonymized user email.
// This is a convenience function to reduce repetition in logging calls and ensure
// consistent attribute naming across the codebase.
//
// Usage:
//
//	logger.Info("operation completed", logging.UserHash(user.Email))
func UserHash(email string) slog.Attr {
	return slog.String(KeyUserHash, AnonymizeEmail(email))
}

// SanitizeHost returns a sanitized version of the host for logging purposes.
// This function redacts IP addresses (both IPv4 and IPv6) to prevent sensitive
// network topology information from appearing in logs, while preserving enough
// context for debugging.
//
// Examples:
//   - "https://192.168.1.100:6443" -> "https://<redacted-ip>:6443"
//   - "https://api.cluster.example.com:6443" -> "https://api.cluster.example.com:6443"
//   - "192.168.1.100" -> "<redacted-ip>"
//   - "https://[2001:db8::1]:6443" -> "https://<redacted-ip>:6443"
//   - "2001:db8::1" -> "<redacted-ip>"
//   - "" -> "<empty>"
func SanitizeHost(host string) string {
	if host == "" {
		return "<empty>"
	}

	// Helper to redact both IPv4 and IPv6
	redactIPs := func(s string) string {
		result := ipv4Regex.ReplaceAllString(s, "<redacted-ip>")
		result = ipv6Regex.ReplaceAllString(result, "<redacted-ip>")
		return result
	}

	// Check if host has a scheme (is a URL) - if not, it's just a host/IP
	if !strings.Contains(host, "://") {
		// No scheme - just redact any IP addresses directly
		return redactIPs(host)
	}

	// Parse as URL to properly handle host extraction
	parsed, err := url.Parse(host)
	if err != nil {
		// If not a valid URL, just redact any IP addresses
		return redactIPs(host)
	}

	// For valid URLs, redact IP addresses in the host portion
	if ipv4Regex.MatchString(parsed.Host) || ipv6Regex.MatchString(parsed.Host) {
		// Replace IP portion, keeping the port if present
		sanitizedHost := redactIPs(parsed.Host)
		parsed.Host = sanitizedHost
		return parsed.String()
	}

	return host
}

// SanitizeToken returns a masked version of a token for logging.
// It returns a length indicator without exposing any token content,
// as even partial token prefixes (like JWT headers) can aid attacks.
func SanitizeToken(token string) string {
	if token == "" {
		return "<empty>"
	}
	return fmt.Sprintf("[token:%d chars]", len(token))
}

// ExtractDomain extracts the domain part from an email address.
// This is useful for lower-cardinality logging where the full email would
// create too many unique values.
func ExtractDomain(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

// Domain returns a slog attribute for the email domain (lower cardinality than full email).
func Domain(email string) slog.Attr {
	return slog.String("user_domain", ExtractDomain(email))
}
