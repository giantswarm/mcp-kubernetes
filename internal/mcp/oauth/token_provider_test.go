package oauth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
}

