package instrumentation

import "strings"

// Cardinality management helpers for metrics.
// These functions reduce high-cardinality label values to prevent metrics explosion.
//
// # Warning
//
// High cardinality in metrics can cause:
// - Increased memory usage in Prometheus/metrics backends
// - Slower query performance
// - Higher storage costs
//
// Always use these helpers when recording metrics with user identifiers or cluster names.

// ClusterType represents a classification of cluster names for metrics.
type ClusterType string

// Cluster type classifications for metrics cardinality control.
const (
	// ClusterTypeProduction represents production clusters.
	ClusterTypeProduction ClusterType = "production"

	// ClusterTypeStaging represents staging/pre-production clusters.
	ClusterTypeStaging ClusterType = "staging"

	// ClusterTypeDevelopment represents development clusters.
	ClusterTypeDevelopment ClusterType = "development"

	// ClusterTypeManagement represents management clusters (empty cluster name).
	ClusterTypeManagement ClusterType = "management"

	// ClusterTypeOther represents clusters that don't match any known pattern.
	ClusterTypeOther ClusterType = "other"
)

// ClassifyClusterName classifies a cluster name into a type for metrics.
// This prevents cardinality explosion by grouping clusters into categories
// instead of using the full cluster name.
//
// Classification rules:
//   - Empty string -> "management" (local/management cluster)
//   - Prefix "prod-" or contains "production" -> "production"
//   - Prefix "staging-" or contains "staging" -> "staging"
//   - Prefix "dev-" or contains "development" -> "development"
//   - Everything else -> "other"
//
// Example:
//
//	ClassifyClusterName("")              // "management"
//	ClassifyClusterName("prod-wc-01")    // "production"
//	ClassifyClusterName("staging-test")  // "staging"
//	ClassifyClusterName("my-cluster")    // "other"
func ClassifyClusterName(name string) string {
	if name == "" {
		return string(ClusterTypeManagement)
	}

	nameLower := strings.ToLower(name)

	// Production patterns
	if strings.HasPrefix(nameLower, "prod-") ||
		strings.HasPrefix(nameLower, "prod_") ||
		strings.Contains(nameLower, "production") ||
		strings.Contains(nameLower, "-prod-") ||
		strings.HasSuffix(nameLower, "-prod") {
		return string(ClusterTypeProduction)
	}

	// Staging patterns
	if strings.HasPrefix(nameLower, "staging-") ||
		strings.HasPrefix(nameLower, "staging_") ||
		strings.HasPrefix(nameLower, "stg-") ||
		strings.Contains(nameLower, "staging") ||
		strings.Contains(nameLower, "-stg-") ||
		strings.HasSuffix(nameLower, "-stg") {
		return string(ClusterTypeStaging)
	}

	// Development patterns
	if strings.HasPrefix(nameLower, "dev-") ||
		strings.HasPrefix(nameLower, "dev_") ||
		strings.Contains(nameLower, "development") ||
		strings.Contains(nameLower, "-dev-") ||
		strings.HasSuffix(nameLower, "-dev") {
		return string(ClusterTypeDevelopment)
	}

	return string(ClusterTypeOther)
}

// ExtractUserDomain extracts the domain part from an email address.
// This reduces cardinality by using the domain instead of the full email.
//
// Example:
//
//	ExtractUserDomain("jane@giantswarm.io")  // "giantswarm.io"
//	ExtractUserDomain("user@example.com")   // "example.com"
//	ExtractUserDomain("invalid")            // "unknown"
//	ExtractUserDomain("")                   // "unknown"
func ExtractUserDomain(email string) string {
	if email == "" {
		return "unknown"
	}

	parts := strings.Split(email, "@")
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}

	return "unknown"
}

// ImpersonationResult constants for metrics.
const (
	// ImpersonationResultSuccess indicates successful impersonation.
	ImpersonationResultSuccess = "success"

	// ImpersonationResultError indicates an error during impersonation.
	ImpersonationResultError = "error"

	// ImpersonationResultDenied indicates impersonation was denied by RBAC.
	ImpersonationResultDenied = "denied"
)

// FederationClientResult constants for metrics.
const (
	// FederationClientResultSuccess indicates successful client creation.
	FederationClientResultSuccess = "success"

	// FederationClientResultError indicates an error during client creation.
	FederationClientResultError = "error"

	// FederationClientResultCached indicates the client was retrieved from cache.
	FederationClientResultCached = "cached"
)
