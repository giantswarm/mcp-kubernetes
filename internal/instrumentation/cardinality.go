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

	// ClusterTypeCICD represents CI/CD clusters (e.g., cicdprod, cicddev).
	ClusterTypeCICD ClusterType = "cicd"

	// ClusterTypeOperations represents operations/infrastructure clusters.
	ClusterTypeOperations ClusterType = "operations"

	// ClusterTypeManagement represents management clusters (empty cluster name).
	ClusterTypeManagement ClusterType = "management"

	// ClusterTypeOther represents clusters that don't match any known pattern.
	ClusterTypeOther ClusterType = "other"
)

// ClassifyClusterName classifies a cluster name into a type for metrics.
// This prevents cardinality explosion by grouping clusters into categories
// instead of using the full cluster name.
//
// # Classification Rules
//
// The function uses case-insensitive pattern matching:
//
//	| Pattern                          | Classification |
//	|----------------------------------|----------------|
//	| Empty string                     | management     |
//	| Contains: cicd                   | cicd           |
//	| Contains: operations, ops        | operations     |
//	| Prefix: prod-, prod_             | production     |
//	| Contains: production, -prod-     | production     |
//	| Suffix: -prod                    | production     |
//	| Prefix: staging-, staging_, stg- | staging        |
//	| Contains: staging, -stg-         | staging        |
//	| Suffix: -stg                     | staging        |
//	| Prefix: dev-, dev_               | development    |
//	| Contains: development, -dev-     | development    |
//	| Suffix: -dev                     | development    |
//	| Prefix: demo (demo-, demotech)   | development    |
//	| Contains: -demo-                 | development    |
//	| Prefix: test-, test_             | development    |
//	| Contains: -test-                 | development    |
//	| Suffix: -test                    | development    |
//	| Everything else                  | other          |
//
// # Customization
//
// Organizations using different naming conventions (e.g., "live-", "prd-", "uat-")
// will see these clusters classified as "other". If you need custom classification,
// consider:
//   - Renaming clusters to follow the patterns above
//   - Implementing a custom classifier that wraps this function
//   - Contributing additional patterns via a pull request
//
// # Examples
//
//	ClassifyClusterName("")                   // "management"
//	ClassifyClusterName("prod-wc-01")         // "production"
//	ClassifyClusterName("my-production-env")  // "production"
//	ClassifyClusterName("staging-test")       // "staging"
//	ClassifyClusterName("stg-wc-01")          // "staging"
//	ClassifyClusterName("dev-cluster")        // "development"
//	ClassifyClusterName("cicdprod")           // "cicd"
//	ClassifyClusterName("cicddev")            // "cicd"
//	ClassifyClusterName("operations")         // "operations"
//	ClassifyClusterName("infra-ops")          // "operations"
//	ClassifyClusterName("demo-cluster")       // "development"
//	ClassifyClusterName("test-wc-01")         // "development"
//	ClassifyClusterName("my-cluster")         // "other"
//	ClassifyClusterName("us-east-1-cluster")  // "other"
func ClassifyClusterName(name string) string {
	if name == "" {
		return string(ClusterTypeManagement)
	}

	nameLower := strings.ToLower(name)

	// CI/CD patterns (check first as they often contain "prod" or "dev" in the name)
	if strings.Contains(nameLower, "cicd") {
		return string(ClusterTypeCICD)
	}

	// Operations patterns
	if strings.Contains(nameLower, "operations") ||
		strings.HasPrefix(nameLower, "ops-") ||
		strings.HasPrefix(nameLower, "ops_") ||
		strings.Contains(nameLower, "-ops-") ||
		strings.HasSuffix(nameLower, "-ops") {
		return string(ClusterTypeOperations)
	}

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

	// Development patterns (including demo and test clusters)
	if strings.HasPrefix(nameLower, "dev-") ||
		strings.HasPrefix(nameLower, "dev_") ||
		strings.Contains(nameLower, "development") ||
		strings.Contains(nameLower, "-dev-") ||
		strings.HasSuffix(nameLower, "-dev") ||
		strings.HasPrefix(nameLower, "demo") || // matches demo-, demo_, demotech, etc.
		strings.Contains(nameLower, "-demo-") ||
		strings.HasPrefix(nameLower, "test-") ||
		strings.HasPrefix(nameLower, "test_") ||
		strings.Contains(nameLower, "-test-") ||
		strings.HasSuffix(nameLower, "-test") {
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
