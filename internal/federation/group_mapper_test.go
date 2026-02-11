package federation

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestNewGroupMapper(t *testing.T) {
	logger := slog.Default()

	t.Run("returns nil for empty mappings", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{}, logger)
		require.NoError(t, err)
		assert.Nil(t, mapper)
	})

	t.Run("returns nil for nil mappings", func(t *testing.T) {
		mapper, err := NewGroupMapper(nil, logger)
		require.NoError(t, err)
		assert.Nil(t, mapper)
	})

	t.Run("creates mapper with valid mappings", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "abc123-def456",
			"customer:GroupB": "xyz789-012345",
		}, logger)
		require.NoError(t, err)
		require.NotNil(t, mapper)
		assert.Equal(t, 2, mapper.MappingCount())
	})

	t.Run("rejects empty source group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"":               "target",
			"customer:Group": "other-target",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source group must not be empty")
	})

	t.Run("rejects whitespace-only source group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"   ": "target",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source group must not be empty")
	})

	t.Run("rejects empty target group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:Group": "",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target group for source")
	})

	t.Run("rejects control characters in source", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"group\x00name": "target",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control characters")
	})

	t.Run("rejects control characters in target", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"source": "target\nnewline",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control characters")
	})

	t.Run("rejects duplicate target groups", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"source-a": "same-target",
			"source-b": "same-target",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate target group")
	})

	t.Run("rejects system:masters as target group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "system:masters",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.Contains(t, err.Error(), "system:masters")
		assert.Contains(t, err.Error(), "privilege escalation")
	})

	t.Run("rejects system:masters even with other valid mappings", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "valid-target",
			"customer:GroupB": "system:masters",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "system:masters")
	})

	t.Run("rejects system:nodes as target group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "system:nodes",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.Contains(t, err.Error(), "system:nodes")
	})

	t.Run("rejects system:kube-controller-manager as target group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "system:kube-controller-manager",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.Contains(t, err.Error(), "system:kube-controller-manager")
	})

	t.Run("rejects system:kube-scheduler as target group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "system:kube-scheduler",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.Contains(t, err.Error(), "system:kube-scheduler")
	})

	t.Run("rejects system:kube-proxy as target group", func(t *testing.T) {
		_, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "system:kube-proxy",
		}, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.Contains(t, err.Error(), "system:kube-proxy")
	})

	t.Run("rejects too many mappings", func(t *testing.T) {
		oversized := make(map[string]string, MaxMappingCount+1)
		for i := 0; i <= MaxMappingCount; i++ {
			oversized[fmt.Sprintf("source-%d", i)] = fmt.Sprintf("target-%d", i)
		}
		_, err := NewGroupMapper(oversized, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many group mappings")
	})

	t.Run("accepts exactly MaxMappingCount mappings", func(t *testing.T) {
		exact := make(map[string]string, MaxMappingCount)
		for i := 0; i < MaxMappingCount; i++ {
			exact[fmt.Sprintf("source-%d", i)] = fmt.Sprintf("target-%d", i)
		}
		mapper, err := NewGroupMapper(exact, logger)
		require.NoError(t, err)
		require.NotNil(t, mapper)
		assert.Equal(t, MaxMappingCount, mapper.MappingCount())
	})

	t.Run("allows non-dangerous system: target groups with warning", func(t *testing.T) {
		// system:authenticated and other non-denied system groups should be allowed
		// (they produce a warning log but are not rejected)
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "system:authenticated",
		}, logger)
		require.NoError(t, err)
		require.NotNil(t, mapper)
	})

	t.Run("uses default logger when nil", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"source": "target",
		}, nil)
		require.NoError(t, err)
		require.NotNil(t, mapper)
	})

	t.Run("defensively copies mappings before validation", func(t *testing.T) {
		original := map[string]string{
			"source": "target",
		}
		mapper, err := NewGroupMapper(original, logger)
		require.NoError(t, err)
		require.NotNil(t, mapper)

		// Modify original - should not affect mapper
		original["new-source"] = "new-target"
		assert.Equal(t, 1, mapper.MappingCount(), "mapper should not be affected by external mutation")
	})
}

func TestGroupMapper_MapGroups(t *testing.T) {
	logger := slog.Default()

	t.Run("nil mapper passes groups through", func(t *testing.T) {
		var mapper *GroupMapper
		groups := []string{"group-a", "group-b"}
		result := mapper.MapGroups(groups, "user@example.com")
		assert.Equal(t, groups, result)
	})

	t.Run("nil groups returns nil", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{"source": "target"}, logger)
		require.NoError(t, err)

		result := mapper.MapGroups(nil, "user@example.com")
		assert.Nil(t, result)
	})

	t.Run("empty groups returns empty", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{"source": "target"}, logger)
		require.NoError(t, err)

		result := mapper.MapGroups([]string{}, "user@example.com")
		assert.Empty(t, result)
	})

	t.Run("maps matching groups", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "abc123-def456",
			"customer:GroupB": "xyz789-012345",
		}, logger)
		require.NoError(t, err)

		groups := []string{"customer:GroupA", "customer:GroupB"}
		result := mapper.MapGroups(groups, "user@example.com")

		assert.Equal(t, []string{"abc123-def456", "xyz789-012345"}, result)
	})

	t.Run("passes through unmapped groups", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "abc123-def456",
		}, logger)
		require.NoError(t, err)

		groups := []string{"system:authenticated", "customer:GroupA", "other-group"}
		result := mapper.MapGroups(groups, "user@example.com")

		assert.Equal(t, []string{"system:authenticated", "abc123-def456", "other-group"}, result)
	})

	t.Run("no mapping needed returns original slice", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "abc123-def456",
		}, logger)
		require.NoError(t, err)

		groups := []string{"system:authenticated", "other-group"}
		result := mapper.MapGroups(groups, "user@example.com")

		// Should return the exact same slice (no allocation)
		assert.Equal(t, groups, result)
	})

	t.Run("does not modify original slice", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "abc123-def456",
		}, logger)
		require.NoError(t, err)

		original := []string{"customer:GroupA", "other-group"}
		originalCopy := make([]string, len(original))
		copy(originalCopy, original)

		mapper.MapGroups(original, "user@example.com")

		assert.Equal(t, originalCopy, original, "original slice must not be modified")
	})

	t.Run("handles Azure AD group GUID mapping", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:Platform Engineers": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"customer:Developers":         "b2c3d4e5-f6a7-8901-bcde-f12345678901",
		}, logger)
		require.NoError(t, err)

		groups := []string{
			"system:authenticated",
			"customer:Platform Engineers",
			"customer:Developers",
		}
		result := mapper.MapGroups(groups, "user@company.com")

		assert.Equal(t, []string{
			"system:authenticated",
			"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"b2c3d4e5-f6a7-8901-bcde-f12345678901",
		}, result)
	})

	t.Run("handles LDAP DN to short name mapping", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"ldap:group:cn=admins,dc=example,dc=com": "admins",
			"ldap:group:cn=devops,dc=example,dc=com": "devops",
		}, logger)
		require.NoError(t, err)

		groups := []string{
			"ldap:group:cn=admins,dc=example,dc=com",
			"ldap:group:cn=devops,dc=example,dc=com",
		}
		result := mapper.MapGroups(groups, "user@example.com")

		assert.Equal(t, []string{"admins", "devops"}, result)
	})
}

func TestGroupMapper_MappingCount(t *testing.T) {
	t.Run("nil mapper returns 0", func(t *testing.T) {
		var mapper *GroupMapper
		assert.Equal(t, 0, mapper.MappingCount())
	})

	t.Run("returns correct count", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"a": "1",
			"b": "2",
			"c": "3",
		}, slog.Default())
		require.NoError(t, err)
		assert.Equal(t, 3, mapper.MappingCount())
	})
}

func TestGroupMapper_String(t *testing.T) {
	t.Run("nil mapper", func(t *testing.T) {
		var mapper *GroupMapper
		assert.Equal(t, "GroupMapper{disabled}", mapper.String())
	})

	t.Run("mapper with mappings", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"a": "1",
			"b": "2",
		}, slog.Default())
		require.NoError(t, err)
		assert.Equal(t, "GroupMapper{mappings=2}", mapper.String())
	})
}

func TestValidateGroupMappings(t *testing.T) {
	t.Run("empty mappings are valid", func(t *testing.T) {
		assert.NoError(t, validateGroupMappings(nil))
		assert.NoError(t, validateGroupMappings(map[string]string{}))
	})

	t.Run("valid mappings pass", func(t *testing.T) {
		assert.NoError(t, validateGroupMappings(map[string]string{
			"customer:GroupA":                        "abc123-def456",
			"ldap:group:cn=admins,dc=example,dc=com": "admins",
			"github:org:myorg":                       "org-myorg",
		}))
	})

	t.Run("rejects system:masters as target", func(t *testing.T) {
		err := validateGroupMappings(map[string]string{
			"any-group": "system:masters",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.Contains(t, err.Error(), "privilege escalation")
	})

	t.Run("rejects system:nodes as target", func(t *testing.T) {
		err := validateGroupMappings(map[string]string{
			"any-group": "system:nodes",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("rejects system:kube-controller-manager as target", func(t *testing.T) {
		err := validateGroupMappings(map[string]string{
			"any-group": "system:kube-controller-manager",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("rejects system:kube-scheduler as target", func(t *testing.T) {
		err := validateGroupMappings(map[string]string{
			"any-group": "system:kube-scheduler",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("rejects system:kube-proxy as target", func(t *testing.T) {
		err := validateGroupMappings(map[string]string{
			"any-group": "system:kube-proxy",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("rejects too many mappings", func(t *testing.T) {
		oversized := make(map[string]string, MaxMappingCount+1)
		for i := 0; i <= MaxMappingCount; i++ {
			oversized[fmt.Sprintf("source-%d", i)] = fmt.Sprintf("target-%d", i)
		}
		err := validateGroupMappings(oversized)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many group mappings")
	})

	t.Run("allows non-denied system: targets", func(t *testing.T) {
		// system:authenticated is not on the denylist
		assert.NoError(t, validateGroupMappings(map[string]string{
			"customer:GroupA": "system:authenticated",
		}))
	})
}

func TestDeniedTargetGroups(t *testing.T) {
	t.Run("system:masters is denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["system:masters"]
		assert.True(t, ok)
	})

	t.Run("system:nodes is denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["system:nodes"]
		assert.True(t, ok)
	})

	t.Run("system:kube-controller-manager is denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["system:kube-controller-manager"]
		assert.True(t, ok)
	})

	t.Run("system:kube-scheduler is denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["system:kube-scheduler"]
		assert.True(t, ok)
	})

	t.Run("system:kube-proxy is denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["system:kube-proxy"]
		assert.True(t, ok)
	})

	t.Run("denylist has expected size", func(t *testing.T) {
		// Guard against accidentally removing entries.
		// Update this count when adding new entries.
		assert.Equal(t, 5, len(deniedTargetGroups))
	})

	t.Run("system:authenticated is not denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["system:authenticated"]
		assert.False(t, ok)
	})

	t.Run("arbitrary groups are not denied", func(t *testing.T) {
		_, ok := deniedTargetGroups["my-custom-group"]
		assert.False(t, ok)
	})
}

func TestParseGroupMappingsJSON(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		result, err := ParseGroupMappingsJSON("")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("valid JSON object", func(t *testing.T) {
		result, err := ParseGroupMappingsJSON(`{"customer:GroupA": "abc123", "customer:GroupB": "xyz789"}`)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"customer:GroupA": "abc123",
			"customer:GroupB": "xyz789",
		}, result)
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := ParseGroupMappingsJSON(`{invalid}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse group mappings JSON")
	})

	t.Run("parses without semantic validation", func(t *testing.T) {
		// ParseGroupMappingsJSON only parses JSON; semantic validation
		// (like denylist checks) is deferred to NewGroupMapper.
		result, err := ParseGroupMappingsJSON(`{"customer:GroupA": "system:masters"}`)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"customer:GroupA": "system:masters",
		}, result)

		// But NewGroupMapper will reject it
		_, err = NewGroupMapper(result, slog.Default())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("handles special characters in group names", func(t *testing.T) {
		result, err := ParseGroupMappingsJSON(`{"ldap:group:cn=admins,dc=example,dc=com": "admins"}`)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"ldap:group:cn=admins,dc=example,dc=com": "admins",
		}, result)
	})

	t.Run("handles unicode in group names", func(t *testing.T) {
		result, err := ParseGroupMappingsJSON(`{"Développeurs": "developers"}`)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"Développeurs": "developers",
		}, result)
	})
}

func TestFormatGroupMappingsForLog(t *testing.T) {
	t.Run("empty mappings", func(t *testing.T) {
		assert.Equal(t, "none", FormatGroupMappingsForLog(nil))
		assert.Equal(t, "none", FormatGroupMappingsForLog(map[string]string{}))
	})

	t.Run("formats mappings with sorted sources", func(t *testing.T) {
		result := FormatGroupMappingsForLog(map[string]string{
			"customer:GroupB": "xyz",
			"customer:GroupA": "abc",
		})
		assert.Equal(t, "2 mappings (sources: customer:GroupA, customer:GroupB)", result)
	})
}

// TestGroupMapper_Integration tests the GroupMapper in an impersonation context
// to verify it works correctly with ConfigWithImpersonation.
func TestGroupMapper_Integration(t *testing.T) {
	t.Run("mapped groups are used in impersonation config", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:Platform Engineers": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		}, slog.Default())
		require.NoError(t, err)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"system:authenticated", "customer:Platform Engineers"},
		}

		// Apply mapping (as the Manager would do before ConfigWithImpersonation)
		mappedGroups := mapper.MapGroups(user.Groups, user.Email)

		// Create a user copy with mapped groups for impersonation
		mappedUser := &UserInfo{
			Email:  user.Email,
			Groups: mappedGroups,
			Extra:  user.Extra,
		}

		baseConfig := &rest.Config{Host: "https://test.example.com"}
		config := ConfigWithImpersonation(baseConfig, mappedUser)

		assert.Equal(t, "user@example.com", config.Impersonate.UserName)
		assert.Equal(t, []string{
			"system:authenticated",
			"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		}, config.Impersonate.Groups)
	})

	t.Run("original user groups are not modified", func(t *testing.T) {
		mapper, err := NewGroupMapper(map[string]string{
			"customer:GroupA": "guid-a",
		}, slog.Default())
		require.NoError(t, err)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"customer:GroupA", "unmapped-group"},
		}

		// Capture original groups
		originalGroups := make([]string, len(user.Groups))
		copy(originalGroups, user.Groups)

		mapper.MapGroups(user.Groups, user.Email)

		assert.Equal(t, originalGroups, user.Groups, "original user groups must not be modified")
	})
}
