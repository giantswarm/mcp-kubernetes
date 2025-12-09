package instrumentation

import "testing"

func TestClassifyClusterName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Management cluster (empty)
		{
			name:     "empty string returns management",
			input:    "",
			expected: "management",
		},
		// Production patterns
		{
			name:     "prod- prefix",
			input:    "prod-wc-01",
			expected: "production",
		},
		{
			name:     "prod_ prefix",
			input:    "prod_cluster",
			expected: "production",
		},
		{
			name:     "contains production",
			input:    "my-production-cluster",
			expected: "production",
		},
		{
			name:     "contains -prod-",
			input:    "us-east-prod-01",
			expected: "production",
		},
		{
			name:     "ends with -prod",
			input:    "cluster-prod",
			expected: "production",
		},
		{
			name:     "uppercase PROD prefix",
			input:    "PROD-CLUSTER",
			expected: "production",
		},
		// Staging patterns
		{
			name:     "staging- prefix",
			input:    "staging-cluster",
			expected: "staging",
		},
		{
			name:     "staging_ prefix",
			input:    "staging_01",
			expected: "staging",
		},
		{
			name:     "stg- prefix",
			input:    "stg-wc-01",
			expected: "staging",
		},
		{
			name:     "contains staging",
			input:    "my-staging-env",
			expected: "staging",
		},
		{
			name:     "ends with -stg",
			input:    "cluster-stg",
			expected: "staging",
		},
		// Development patterns
		{
			name:     "dev- prefix",
			input:    "dev-cluster",
			expected: "development",
		},
		{
			name:     "dev_ prefix",
			input:    "dev_test",
			expected: "development",
		},
		{
			name:     "contains development",
			input:    "development-env",
			expected: "development",
		},
		{
			name:     "contains -dev-",
			input:    "us-west-dev-01",
			expected: "development",
		},
		{
			name:     "ends with -dev",
			input:    "cluster-dev",
			expected: "development",
		},
		// Other (no pattern match)
		{
			name:     "random cluster name",
			input:    "my-cluster",
			expected: "other",
		},
		{
			name:     "numeric cluster name",
			input:    "cluster-123",
			expected: "other",
		},
		{
			name:     "region-based name",
			input:    "us-east-1-cluster",
			expected: "other",
		},
		{
			name:     "team-based name",
			input:    "team-alpha-cluster",
			expected: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyClusterName(tt.input)
			if result != tt.expected {
				t.Errorf("ClassifyClusterName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractUserDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid email",
			input:    "jane@giantswarm.io",
			expected: "giantswarm.io",
		},
		{
			name:     "valid email with subdomain",
			input:    "user@mail.example.com",
			expected: "mail.example.com",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "no @ symbol",
			input:    "invalid",
			expected: "unknown",
		},
		{
			name:     "@ at start",
			input:    "@domain.com",
			expected: "domain.com",
		},
		{
			name:     "@ at end",
			input:    "user@",
			expected: "unknown",
		},
		{
			name:     "multiple @ symbols",
			input:    "user@domain@example.com",
			expected: "unknown",
		},
		{
			name:     "simple username",
			input:    "admin",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractUserDomain(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractUserDomain(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestClusterTypeConstants(t *testing.T) {
	// Verify constants are defined correctly
	if ClusterTypeProduction != "production" {
		t.Errorf("ClusterTypeProduction = %q, want %q", ClusterTypeProduction, "production")
	}
	if ClusterTypeStaging != "staging" {
		t.Errorf("ClusterTypeStaging = %q, want %q", ClusterTypeStaging, "staging")
	}
	if ClusterTypeDevelopment != "development" {
		t.Errorf("ClusterTypeDevelopment = %q, want %q", ClusterTypeDevelopment, "development")
	}
	if ClusterTypeManagement != "management" {
		t.Errorf("ClusterTypeManagement = %q, want %q", ClusterTypeManagement, "management")
	}
	if ClusterTypeOther != "other" {
		t.Errorf("ClusterTypeOther = %q, want %q", ClusterTypeOther, "other")
	}
}

func TestImpersonationResultConstants(t *testing.T) {
	// Verify constants are defined correctly
	if ImpersonationResultSuccess != "success" {
		t.Errorf("ImpersonationResultSuccess = %q, want %q", ImpersonationResultSuccess, "success")
	}
	if ImpersonationResultError != "error" {
		t.Errorf("ImpersonationResultError = %q, want %q", ImpersonationResultError, "error")
	}
	if ImpersonationResultDenied != "denied" {
		t.Errorf("ImpersonationResultDenied = %q, want %q", ImpersonationResultDenied, "denied")
	}
}

func TestFederationClientResultConstants(t *testing.T) {
	// Verify constants are defined correctly
	if FederationClientResultSuccess != "success" {
		t.Errorf("FederationClientResultSuccess = %q, want %q", FederationClientResultSuccess, "success")
	}
	if FederationClientResultError != "error" {
		t.Errorf("FederationClientResultError = %q, want %q", FederationClientResultError, "error")
	}
	if FederationClientResultCached != "cached" {
		t.Errorf("FederationClientResultCached = %q, want %q", FederationClientResultCached, "cached")
	}
}
