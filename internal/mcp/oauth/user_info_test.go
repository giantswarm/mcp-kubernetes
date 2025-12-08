// Package oauth provides tests for user info conversion and validation.
// These tests verify the correct conversion between OAuth UserInfo and
// federation UserInfo types for Kubernetes impersonation.
package oauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
)

// TestToFederationUserInfo tests conversion from OAuth UserInfo to federation UserInfo.
func TestToFederationUserInfo(t *testing.T) {
	t.Run("returns nil for nil input", func(t *testing.T) {
		result := ToFederationUserInfo(nil)
		assert.Nil(t, result)
	})

	t.Run("converts basic user info", func(t *testing.T) {
		user := &UserInfo{
			ID:    "user-123",
			Email: "user@example.com",
		}

		result := ToFederationUserInfo(user)

		require.NotNil(t, result)
		assert.Equal(t, "user@example.com", result.Email)
		assert.Nil(t, result.Groups)
		// Subject should be added to extra
		require.NotNil(t, result.Extra)
		assert.Equal(t, []string{"user-123"}, result.Extra["sub"])
	})

	t.Run("converts user with groups", func(t *testing.T) {
		user := &UserInfo{
			ID:     "user-456",
			Email:  "admin@example.com",
			Groups: []string{"platform-admins", "developers"},
		}

		result := ToFederationUserInfo(user)

		require.NotNil(t, result)
		assert.Equal(t, "admin@example.com", result.Email)
		assert.Equal(t, []string{"platform-admins", "developers"}, result.Groups)
		assert.Equal(t, []string{"user-456"}, result.Extra["sub"])
	})

	t.Run("handles empty ID gracefully", func(t *testing.T) {
		user := &UserInfo{
			Email: "user@example.com",
		}

		result := ToFederationUserInfo(user)

		require.NotNil(t, result)
		assert.Equal(t, "user@example.com", result.Email)
		// Extra should be nil when ID is empty
		assert.Nil(t, result.Extra)
	})

	t.Run("handles empty groups", func(t *testing.T) {
		user := &UserInfo{
			ID:     "user-789",
			Email:  "user@example.com",
			Groups: []string{}, // explicitly empty
		}

		result := ToFederationUserInfo(user)

		require.NotNil(t, result)
		assert.Empty(t, result.Groups)
	})

	t.Run("creates defensive copy of groups slice", func(t *testing.T) {
		originalGroups := []string{"group1", "group2"}
		user := &UserInfo{
			ID:     "user-123",
			Email:  "user@example.com",
			Groups: originalGroups,
		}

		result := ToFederationUserInfo(user)

		require.NotNil(t, result)
		assert.Equal(t, []string{"group1", "group2"}, result.Groups)

		// Modify original slice - should NOT affect the result
		originalGroups[0] = "modified-group"

		// Result should still have original values
		assert.Equal(t, []string{"group1", "group2"}, result.Groups)
		// Original user's groups should be modified
		assert.Equal(t, []string{"modified-group", "group2"}, user.Groups)
	})
}

// TestToFederationUserInfoWithExtra tests conversion with additional extra claims.
func TestToFederationUserInfoWithExtra(t *testing.T) {
	t.Run("returns nil for nil input", func(t *testing.T) {
		extra := map[string][]string{"custom": {"value"}}
		result := ToFederationUserInfoWithExtra(nil, extra)
		assert.Nil(t, result)
	})

	t.Run("merges extra claims", func(t *testing.T) {
		user := &UserInfo{
			ID:    "user-123",
			Email: "user@example.com",
		}
		extra := map[string][]string{
			"tenant":   {"giantswarm"},
			"role":     {"admin"},
			"audience": {"api", "web"},
		}

		result := ToFederationUserInfoWithExtra(user, extra)

		require.NotNil(t, result)
		assert.Equal(t, "user@example.com", result.Email)

		// Original sub claim should be present
		assert.Equal(t, []string{"user-123"}, result.Extra["sub"])

		// Additional claims should be merged
		assert.Equal(t, []string{"giantswarm"}, result.Extra["tenant"])
		assert.Equal(t, []string{"admin"}, result.Extra["role"])
		assert.Equal(t, []string{"api", "web"}, result.Extra["audience"])
	})

	t.Run("caller extra overrides auto-extracted claims", func(t *testing.T) {
		user := &UserInfo{
			ID:    "original-id",
			Email: "user@example.com",
		}
		extra := map[string][]string{
			"sub": {"overridden-id"}, // explicitly override the subject
		}

		result := ToFederationUserInfoWithExtra(user, extra)

		require.NotNil(t, result)
		// Caller-provided value should take precedence
		assert.Equal(t, []string{"overridden-id"}, result.Extra["sub"])
	})

	t.Run("handles nil extra map", func(t *testing.T) {
		user := &UserInfo{
			ID:    "user-123",
			Email: "user@example.com",
		}

		result := ToFederationUserInfoWithExtra(user, nil)

		require.NotNil(t, result)
		// Should behave like ToFederationUserInfo
		assert.Equal(t, []string{"user-123"}, result.Extra["sub"])
	})

	t.Run("handles empty extra map", func(t *testing.T) {
		user := &UserInfo{
			ID:    "user-123",
			Email: "user@example.com",
		}

		result := ToFederationUserInfoWithExtra(user, map[string][]string{})

		require.NotNil(t, result)
		// Should behave like ToFederationUserInfo
		assert.Equal(t, []string{"user-123"}, result.Extra["sub"])
	})

	t.Run("adds extra to user without ID", func(t *testing.T) {
		user := &UserInfo{
			Email: "user@example.com",
		}
		extra := map[string][]string{
			"custom": {"value"},
		}

		result := ToFederationUserInfoWithExtra(user, extra)

		require.NotNil(t, result)
		assert.NotNil(t, result.Extra)
		assert.Equal(t, []string{"value"}, result.Extra["custom"])
		// No sub claim since ID was empty
		_, hasSub := result.Extra["sub"]
		assert.False(t, hasSub)
	})
}

// TestValidateUserInfoForImpersonation tests validation of user info for Kubernetes impersonation.
func TestValidateUserInfoForImpersonation(t *testing.T) {
	t.Run("returns error for nil user", func(t *testing.T) {
		err := ValidateUserInfoForImpersonation(nil)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrUserInfoRequired)
	})

	t.Run("returns error for empty email", func(t *testing.T) {
		user := &UserInfo{
			ID: "user-123",
			// Email is empty
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrUserEmailRequired)
	})

	t.Run("succeeds with valid email", func(t *testing.T) {
		user := &UserInfo{
			Email: "user@example.com",
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.NoError(t, err)
	})

	t.Run("succeeds with email and groups", func(t *testing.T) {
		user := &UserInfo{
			ID:     "user-123",
			Email:  "admin@example.com",
			Groups: []string{"admins", "developers"},
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.NoError(t, err)
	})

	t.Run("succeeds with minimal valid user", func(t *testing.T) {
		user := &UserInfo{
			Email: "minimal@example.com",
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.NoError(t, err)
	})

	// Tests for enhanced validation (federation.ValidateUserInfo integration)
	t.Run("returns error for invalid email format", func(t *testing.T) {
		user := &UserInfo{
			Email: "not-a-valid-email",
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrInvalidEmail)
	})

	t.Run("returns error for email with control characters", func(t *testing.T) {
		user := &UserInfo{
			Email: "user\x00@example.com", // null byte
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrInvalidEmail)
	})

	t.Run("returns error for group with control characters", func(t *testing.T) {
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"valid-group", "invalid\ngroup"}, // newline in group name
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrInvalidGroupName)
	})

	t.Run("returns error for empty group name", func(t *testing.T) {
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"valid-group", ""}, // empty group name
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrInvalidGroupName)
	})

	t.Run("returns error for too many groups", func(t *testing.T) {
		// Create more groups than allowed (max is 100)
		groups := make([]string, 101)
		for i := range groups {
			groups[i] = "group"
		}

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: groups,
		}

		err := ValidateUserInfoForImpersonation(user)

		assert.Error(t, err)
		assert.ErrorIs(t, err, federation.ErrInvalidGroupName)
	})
}

// TestUserInfoTypeAlias verifies that UserInfo is correctly aliased from providers.UserInfo.
func TestUserInfoTypeAlias(t *testing.T) {
	t.Run("UserInfo has expected fields from providers.UserInfo", func(t *testing.T) {
		user := &UserInfo{
			ID:            "test-id",
			Email:         "test@example.com",
			EmailVerified: true,
			Name:          "Test User",
			GivenName:     "Test",
			FamilyName:    "User",
			Picture:       "https://example.com/photo.jpg",
			Locale:        "en-US",
			Groups:        []string{"group1", "group2"},
		}

		// Verify all fields are accessible
		assert.Equal(t, "test-id", user.ID)
		assert.Equal(t, "test@example.com", user.Email)
		assert.True(t, user.EmailVerified)
		assert.Equal(t, "Test User", user.Name)
		assert.Equal(t, "Test", user.GivenName)
		assert.Equal(t, "User", user.FamilyName)
		assert.Equal(t, "https://example.com/photo.jpg", user.Picture)
		assert.Equal(t, "en-US", user.Locale)
		assert.Equal(t, []string{"group1", "group2"}, user.Groups)
	})
}
