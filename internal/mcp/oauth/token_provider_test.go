// Package oauth provides tests for OAuth token provider and context handling.
// These tests verify the correct storage and retrieval of OAuth access tokens
// in request contexts for downstream Kubernetes API authentication.
package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// TestContextWithAccessToken tests storing and retrieving access tokens from context.
func TestContextWithAccessToken(t *testing.T) {
	t.Run("stores and retrieves access token", func(t *testing.T) {
		ctx := context.Background()
		token := "test-access-token-12345"

		// Store token in context
		ctxWithToken := ContextWithAccessToken(ctx, token)

		// Retrieve token from context
		retrievedToken, ok := GetAccessTokenFromContext(ctxWithToken)
		assert.True(t, ok)
		assert.Equal(t, token, retrievedToken)
	})

	t.Run("returns false for missing token", func(t *testing.T) {
		ctx := context.Background()

		token, ok := GetAccessTokenFromContext(ctx)
		assert.False(t, ok)
		assert.Equal(t, "", token)
	})

	t.Run("returns false for empty token", func(t *testing.T) {
		ctx := context.Background()
		ctxWithToken := ContextWithAccessToken(ctx, "")

		token, ok := GetAccessTokenFromContext(ctxWithToken)
		assert.False(t, ok)
		assert.Equal(t, "", token)
	})

	t.Run("preserves other context values", func(t *testing.T) {
		type testKey string
		ctx := context.WithValue(context.Background(), testKey("other-key"), "other-value")

		ctxWithToken := ContextWithAccessToken(ctx, "my-token")

		// Original value should still be accessible
		assert.Equal(t, "other-value", ctxWithToken.Value(testKey("other-key")))

		// Token should also be accessible
		token, ok := GetAccessTokenFromContext(ctxWithToken)
		assert.True(t, ok)
		assert.Equal(t, "my-token", token)
	})

	t.Run("overwrites existing token", func(t *testing.T) {
		ctx := context.Background()
		ctx = ContextWithAccessToken(ctx, "first-token")
		ctx = ContextWithAccessToken(ctx, "second-token")

		token, ok := GetAccessTokenFromContext(ctx)
		assert.True(t, ok)
		assert.Equal(t, "second-token", token)
	})
}

// TestGetIDToken tests the extraction of ID token from OAuth2 token.
func TestGetIDToken(t *testing.T) {
	t.Run("returns empty string for nil token", func(t *testing.T) {
		result := GetIDToken(nil)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty string when id_token not present", func(t *testing.T) {
		token := &oauth2.Token{
			AccessToken: "access-token",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Hour),
		}
		result := GetIDToken(token)
		assert.Equal(t, "", result)
	})

	t.Run("returns id_token when present as string", func(t *testing.T) {
		expectedIDToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test"
		token := &oauth2.Token{
			AccessToken: "access-token",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Hour),
		}
		// Use WithExtra to set the id_token field
		token = token.WithExtra(map[string]interface{}{
			"id_token": expectedIDToken,
		})

		result := GetIDToken(token)
		assert.Equal(t, expectedIDToken, result)
	})

	t.Run("returns empty string when id_token is not a string", func(t *testing.T) {
		token := &oauth2.Token{
			AccessToken: "access-token",
			TokenType:   "Bearer",
		}
		// Set id_token as a non-string type
		token = token.WithExtra(map[string]interface{}{
			"id_token": 12345, // not a string
		})

		result := GetIDToken(token)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty string when id_token is nil in extra", func(t *testing.T) {
		token := &oauth2.Token{
			AccessToken: "access-token",
			TokenType:   "Bearer",
		}
		token = token.WithExtra(map[string]interface{}{
			"id_token": nil,
		})

		result := GetIDToken(token)
		assert.Equal(t, "", result)
	})
}

// TestHasUserInfo tests the HasUserInfo convenience function.
func TestHasUserInfo(t *testing.T) {
	t.Run("returns false for context without user info", func(t *testing.T) {
		ctx := context.Background()
		assert.False(t, HasUserInfo(ctx))
	})

	t.Run("returns false for context with access token only", func(t *testing.T) {
		ctx := context.Background()
		ctx = ContextWithAccessToken(ctx, "some-token")
		// Access token is different from user info - they use different context keys
		assert.False(t, HasUserInfo(ctx))
	})
}

// TestGetUserEmailFromContext tests the GetUserEmailFromContext convenience function.
func TestGetUserEmailFromContext(t *testing.T) {
	t.Run("returns empty string for context without user info", func(t *testing.T) {
		ctx := context.Background()
		assert.Equal(t, "", GetUserEmailFromContext(ctx))
	})
}

// TestGetUserGroupsFromContext tests the GetUserGroupsFromContext convenience function.
func TestGetUserGroupsFromContext(t *testing.T) {
	t.Run("returns nil for context without user info", func(t *testing.T) {
		ctx := context.Background()
		assert.Nil(t, GetUserGroupsFromContext(ctx))
	})
}

// TestUserInfoFromContext tests the UserInfoFromContext wrapper.
func TestUserInfoFromContext(t *testing.T) {
	t.Run("returns false for context without user info", func(t *testing.T) {
		ctx := context.Background()
		user, ok := UserInfoFromContext(ctx)
		assert.False(t, ok)
		assert.Nil(t, user)
	})

	t.Run("is consistent with HasUserInfo", func(t *testing.T) {
		ctx := context.Background()
		_, ok := UserInfoFromContext(ctx)
		hasInfo := HasUserInfo(ctx)
		assert.Equal(t, ok, hasInfo)
	})
}

// TestContextKeyUniqueness ensures our context keys don't collide with other packages.
func TestContextKeyUniqueness(t *testing.T) {
	// Create a context with our key
	ctx := context.Background()
	ctx = ContextWithAccessToken(ctx, "our-token")

	// Try to retrieve using a plain string key (should fail)
	// This tests that contextKey type prevents collisions
	val := ctx.Value("oauth_access_token")
	assert.Nil(t, val, "plain string key should not retrieve our token")

	// Our typed key should still work
	token, ok := GetAccessTokenFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "our-token", token)
}
