package federation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// TestConfigWithImpersonation_AgentHeader verifies that the agent identifier
// is always added to impersonation requests for audit trail purposes.
func TestConfigWithImpersonation_AgentHeader(t *testing.T) {
	t.Run("adds agent header when user has no extra headers", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)
		require.NotNil(t, result.Impersonate.Extra)
		assert.Contains(t, result.Impersonate.Extra, ImpersonationAgentExtraKey)
		assert.Equal(t, []string{ImpersonationAgentName}, result.Impersonate.Extra[ImpersonationAgentExtraKey])
	})

	t.Run("adds agent header when user has existing extra headers", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
			Extra: map[string][]string{
				"department": {"engineering"},
				"team":       {"platform"},
			},
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)
		require.NotNil(t, result.Impersonate.Extra)

		// Agent header should be present
		assert.Contains(t, result.Impersonate.Extra, ImpersonationAgentExtraKey)
		assert.Equal(t, []string{ImpersonationAgentName}, result.Impersonate.Extra[ImpersonationAgentExtraKey])

		// User's extra headers should also be preserved
		assert.Contains(t, result.Impersonate.Extra, "department")
		assert.Equal(t, []string{"engineering"}, result.Impersonate.Extra["department"])
		assert.Contains(t, result.Impersonate.Extra, "team")
		assert.Equal(t, []string{"platform"}, result.Impersonate.Extra["team"])
	})

	t.Run("user extra headers take precedence over default agent", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
			Extra: map[string][]string{
				ImpersonationAgentExtraKey: {"custom-agent"},
			},
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)
		// User's custom agent should take precedence
		assert.Equal(t, []string{"custom-agent"}, result.Impersonate.Extra[ImpersonationAgentExtraKey])
	})

	t.Run("preserves all user extra headers including sub claim", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
			Extra: map[string][]string{
				"sub":        {"user-id-12345"},
				"org":        {"giantswarm"},
				"department": {"engineering", "security"},
			},
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)
		require.NotNil(t, result.Impersonate.Extra)

		// All user extras preserved
		assert.Equal(t, []string{"user-id-12345"}, result.Impersonate.Extra["sub"])
		assert.Equal(t, []string{"giantswarm"}, result.Impersonate.Extra["org"])
		assert.Equal(t, []string{"engineering", "security"}, result.Impersonate.Extra["department"])

		// Agent also present
		assert.Contains(t, result.Impersonate.Extra, ImpersonationAgentExtraKey)
	})
}

// TestConfigWithImpersonation_FullConfig verifies the complete impersonation
// configuration matches what Kubernetes expects.
func TestConfigWithImpersonation_FullConfig(t *testing.T) {
	t.Run("sets all impersonation fields correctly", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://api.cluster.example.com:6443",
			TLSClientConfig: rest.TLSClientConfig{
				CAData:   []byte("ca-data"),
				CertData: []byte("cert-data"),
				KeyData:  []byte("key-data"),
			},
		}
		user := &UserInfo{
			Email:  "jane@giantswarm.io",
			Groups: []string{"github:org:giantswarm", "platform-team", "system:authenticated"},
			Extra: map[string][]string{
				"sub": {"jane-user-id"},
			},
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)

		// User email becomes Impersonate-User header
		assert.Equal(t, "jane@giantswarm.io", result.Impersonate.UserName)

		// Groups become Impersonate-Group headers
		assert.Equal(t, []string{"github:org:giantswarm", "platform-team", "system:authenticated"}, result.Impersonate.Groups)

		// Extra headers include agent and user's extras
		require.NotNil(t, result.Impersonate.Extra)
		assert.Equal(t, []string{ImpersonationAgentName}, result.Impersonate.Extra[ImpersonationAgentExtraKey])
		assert.Equal(t, []string{"jane-user-id"}, result.Impersonate.Extra["sub"])

		// Original config values are preserved
		assert.Equal(t, "https://api.cluster.example.com:6443", result.Host)
		assert.Equal(t, []byte("ca-data"), result.CAData)
	})

	t.Run("handles empty groups correctly", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{}, // Empty groups
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)
		assert.Equal(t, "user@example.com", result.Impersonate.UserName)
		assert.Empty(t, result.Impersonate.Groups)
		// Agent header should still be present
		assert.Contains(t, result.Impersonate.Extra, ImpersonationAgentExtraKey)
	})

	t.Run("handles nil groups correctly", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: nil, // Nil groups
		}

		result := ConfigWithImpersonation(config, user)

		require.NotNil(t, result)
		assert.Equal(t, "user@example.com", result.Impersonate.UserName)
		assert.Nil(t, result.Impersonate.Groups)
		// Agent header should still be present
		assert.Contains(t, result.Impersonate.Extra, ImpersonationAgentExtraKey)
	})
}

// TestMergeExtraWithAgent tests the helper function directly.
func TestMergeExtraWithAgent(t *testing.T) {
	t.Run("creates extra with agent when user extra is nil", func(t *testing.T) {
		result := mergeExtraWithAgent(nil)

		require.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, []string{ImpersonationAgentName}, result[ImpersonationAgentExtraKey])
	})

	t.Run("creates extra with agent when user extra is empty", func(t *testing.T) {
		result := mergeExtraWithAgent(map[string][]string{})

		require.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, []string{ImpersonationAgentName}, result[ImpersonationAgentExtraKey])
	})

	t.Run("merges user extra with agent", func(t *testing.T) {
		userExtra := map[string][]string{
			"department": {"engineering"},
			"sub":        {"user-123"},
		}

		result := mergeExtraWithAgent(userExtra)

		require.NotNil(t, result)
		assert.Len(t, result, 3)
		assert.Equal(t, []string{ImpersonationAgentName}, result[ImpersonationAgentExtraKey])
		assert.Equal(t, []string{"engineering"}, result["department"])
		assert.Equal(t, []string{"user-123"}, result["sub"])
	})

	t.Run("user agent override takes precedence", func(t *testing.T) {
		userExtra := map[string][]string{
			ImpersonationAgentExtraKey: {"custom-mcp-client"},
		}

		result := mergeExtraWithAgent(userExtra)

		require.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, []string{"custom-mcp-client"}, result[ImpersonationAgentExtraKey])
	})

	t.Run("does not modify original user extra map", func(t *testing.T) {
		userExtra := map[string][]string{
			"department": {"engineering"},
		}

		result := mergeExtraWithAgent(userExtra)

		// Result should have agent
		assert.Contains(t, result, ImpersonationAgentExtraKey)

		// Original should NOT have agent
		assert.NotContains(t, userExtra, ImpersonationAgentExtraKey)
		assert.Len(t, userExtra, 1)
	})
}

// TestImpersonationError tests the ImpersonationError type.
func TestImpersonationError(t *testing.T) {
	t.Run("error message includes all details", func(t *testing.T) {
		err := &ImpersonationError{
			ClusterName: "prod-cluster",
			UserEmail:   "jane@example.com",
			GroupCount:  3,
			Reason:      "RBAC denied",
			Err:         errors.New("forbidden"),
		}

		errMsg := err.Error()

		assert.Contains(t, errMsg, "prod-cluster")
		assert.Contains(t, errMsg, "user:") // Anonymized email
		assert.Contains(t, errMsg, "3 groups")
		assert.Contains(t, errMsg, "RBAC denied")
		assert.Contains(t, errMsg, "forbidden")
		// Should NOT contain the actual email
		assert.NotContains(t, errMsg, "jane@example.com")
	})

	t.Run("error message without underlying error", func(t *testing.T) {
		err := &ImpersonationError{
			ClusterName: "prod-cluster",
			UserEmail:   "jane@example.com",
			GroupCount:  2,
			Reason:      "invalid user identity",
		}

		errMsg := err.Error()

		assert.Contains(t, errMsg, "prod-cluster")
		assert.Contains(t, errMsg, "invalid user identity")
		assert.NotContains(t, errMsg, "nil")
	})

	t.Run("Is matches ErrImpersonationFailed", func(t *testing.T) {
		err := &ImpersonationError{
			ClusterName: "test-cluster",
			Reason:      "test",
		}

		assert.True(t, errors.Is(err, ErrImpersonationFailed))
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlying := errors.New("original error")
		err := &ImpersonationError{
			ClusterName: "test-cluster",
			Reason:      "test",
			Err:         underlying,
		}

		assert.True(t, errors.Is(err, underlying))
	})

	t.Run("UserFacingError returns safe message", func(t *testing.T) {
		err := &ImpersonationError{
			ClusterName: "sensitive-prod-cluster",
			UserEmail:   "admin@company.com",
			GroupCount:  5,
			Reason:      "internal RBAC details",
		}

		userMsg := err.UserFacingError()

		// Should not contain sensitive info
		assert.NotContains(t, userMsg, "sensitive-prod-cluster")
		assert.NotContains(t, userMsg, "admin@company.com")
		assert.NotContains(t, userMsg, "internal RBAC details")

		// Should contain helpful guidance
		assert.Contains(t, userMsg, "permissions")
		assert.Contains(t, userMsg, "administrator")
	})
}

// TestImpersonationConstants tests that impersonation constants are correctly defined.
func TestImpersonationConstants(t *testing.T) {
	assert.Equal(t, "mcp-kubernetes", ImpersonationAgentName)
	assert.Equal(t, "agent", ImpersonationAgentExtraKey)
	assert.Equal(t, "Impersonate-User", ImpersonateUserHeader)
	assert.Equal(t, "Impersonate-Group", ImpersonateGroupHeader)
	assert.Equal(t, "Impersonate-Extra-", ImpersonateExtraHeaderPrefix)
}

// TestGroupMappingBehavior documents and tests the group mapping behavior
// for different OAuth providers.
func TestGroupMappingBehavior(t *testing.T) {
	// This test documents how different OAuth provider groups are passed through
	// to Kubernetes without transformation. This is important because it ensures
	// consistency with direct kubectl access.

	testCases := []struct {
		name           string
		groups         []string
		expectedGroups []string
		description    string
	}{
		{
			name:           "GitHub groups",
			groups:         []string{"github:org:giantswarm", "github:team:platform"},
			expectedGroups: []string{"github:org:giantswarm", "github:team:platform"},
			description:    "GitHub OAuth groups are passed as-is",
		},
		{
			name:           "Azure AD groups",
			groups:         []string{"azure:group:abc123-def456", "azure:group:xyz789"},
			expectedGroups: []string{"azure:group:abc123-def456", "azure:group:xyz789"},
			description:    "Azure AD groups are passed as-is",
		},
		{
			name:           "LDAP groups",
			groups:         []string{"ldap:group:cn=admins,dc=example,dc=com"},
			expectedGroups: []string{"ldap:group:cn=admins,dc=example,dc=com"},
			description:    "LDAP DN-style groups are passed as-is",
		},
		{
			name:           "Mixed groups from SSO",
			groups:         []string{"system:authenticated", "github:org:myorg", "team:sre"},
			expectedGroups: []string{"system:authenticated", "github:org:myorg", "team:sre"},
			description:    "Mixed group formats are all preserved",
		},
		{
			name:           "Kubernetes system groups",
			groups:         []string{"system:authenticated", "system:masters"},
			expectedGroups: []string{"system:authenticated", "system:masters"},
			description:    "Kubernetes system groups are preserved",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &rest.Config{
				Host: "https://test.example.com",
			}
			user := &UserInfo{
				Email:  "user@example.com",
				Groups: tc.groups,
			}

			result := ConfigWithImpersonation(config, user)

			require.NotNil(t, result)
			assert.Equal(t, tc.expectedGroups, result.Impersonate.Groups,
				"Groups should pass through unchanged: %s", tc.description)
		})
	}
}
