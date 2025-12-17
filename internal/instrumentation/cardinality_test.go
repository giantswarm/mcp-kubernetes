package instrumentation

import "testing"

func TestClassifyClusterName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ClusterType
	}{
		// Management cluster (empty)
		{
			name:     "empty string returns management",
			input:    "",
			expected: ClusterTypeManagement,
		},
		// Production patterns
		{
			name:     "prod- prefix",
			input:    "prod-wc-01",
			expected: ClusterTypeProduction,
		},
		{
			name:     "prod_ prefix",
			input:    "prod_cluster",
			expected: ClusterTypeProduction,
		},
		{
			name:     "contains production",
			input:    "my-production-cluster",
			expected: ClusterTypeProduction,
		},
		{
			name:     "contains -prod-",
			input:    "us-east-prod-01",
			expected: ClusterTypeProduction,
		},
		{
			name:     "ends with -prod",
			input:    "cluster-prod",
			expected: ClusterTypeProduction,
		},
		{
			name:     "uppercase PROD prefix",
			input:    "PROD-CLUSTER",
			expected: ClusterTypeProduction,
		},
		// Staging patterns
		{
			name:     "staging- prefix",
			input:    "staging-cluster",
			expected: ClusterTypeStaging,
		},
		{
			name:     "staging_ prefix",
			input:    "staging_01",
			expected: ClusterTypeStaging,
		},
		{
			name:     "stg- prefix",
			input:    "stg-wc-01",
			expected: ClusterTypeStaging,
		},
		{
			name:     "contains staging",
			input:    "my-staging-env",
			expected: ClusterTypeStaging,
		},
		{
			name:     "ends with -stg",
			input:    "cluster-stg",
			expected: ClusterTypeStaging,
		},
		// Development patterns
		{
			name:     "dev- prefix",
			input:    "dev-cluster",
			expected: ClusterTypeDevelopment,
		},
		{
			name:     "dev_ prefix",
			input:    "dev_test",
			expected: ClusterTypeDevelopment,
		},
		{
			name:     "contains development",
			input:    "development-env",
			expected: ClusterTypeDevelopment,
		},
		{
			name:     "contains -dev-",
			input:    "us-west-dev-01",
			expected: ClusterTypeDevelopment,
		},
		{
			name:     "ends with -dev",
			input:    "cluster-dev",
			expected: ClusterTypeDevelopment,
		},
		// Other (no pattern match)
		{
			name:     "random cluster name",
			input:    "my-cluster",
			expected: ClusterTypeOther,
		},
		{
			name:     "numeric cluster name",
			input:    "cluster-123",
			expected: ClusterTypeOther,
		},
		{
			name:     "region-based name",
			input:    "us-east-1-cluster",
			expected: ClusterTypeOther,
		},
		{
			name:     "team-based name",
			input:    "team-alpha-cluster",
			expected: ClusterTypeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyClusterName(tt.input)
			if result != string(tt.expected) {
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
	// Verify constants are defined correctly using the typed constants
	// We test that constants are not empty and have the expected type
	constants := []ClusterType{
		ClusterTypeProduction,
		ClusterTypeStaging,
		ClusterTypeDevelopment,
		ClusterTypeManagement,
		ClusterTypeOther,
	}

	for _, c := range constants {
		if c == "" {
			t.Error("ClusterType constant should not be empty")
		}
	}

	// Verify we have 5 distinct constant values
	seen := make(map[ClusterType]bool)
	for _, c := range constants {
		if seen[c] {
			t.Errorf("Duplicate ClusterType constant: %q", c)
		}
		seen[c] = true
	}
	if len(seen) != 5 {
		t.Errorf("Expected 5 unique ClusterType constants, got %d", len(seen))
	}
}

func TestImpersonationResultConstants(t *testing.T) {
	// Verify constants are defined correctly
	if ImpersonationResultSuccess != StatusSuccess {
		t.Errorf("ImpersonationResultSuccess = %q, want %q", ImpersonationResultSuccess, StatusSuccess)
	}
	if ImpersonationResultError != StatusError {
		t.Errorf("ImpersonationResultError = %q, want %q", ImpersonationResultError, StatusError)
	}
	if ImpersonationResultDenied != "denied" {
		t.Errorf("ImpersonationResultDenied = %q, want %q", ImpersonationResultDenied, "denied")
	}
}

func TestFederationClientResultConstants(t *testing.T) {
	// Verify constants are defined correctly
	if FederationClientResultSuccess != StatusSuccess {
		t.Errorf("FederationClientResultSuccess = %q, want %q", FederationClientResultSuccess, StatusSuccess)
	}
	if FederationClientResultError != StatusError {
		t.Errorf("FederationClientResultError = %q, want %q", FederationClientResultError, StatusError)
	}
	if FederationClientResultCached != "cached" {
		t.Errorf("FederationClientResultCached = %q, want %q", FederationClientResultCached, "cached")
	}
}
