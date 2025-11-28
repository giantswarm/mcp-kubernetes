package oauth

import (
	"context"

	oauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/storage"
	"golang.org/x/oauth2"
)

// TokenProvider implements a token provider interface using the mcp-oauth library's storage.
// It bridges the mcp-oauth storage with code that needs token access.
type TokenProvider struct {
	store storage.TokenStore
}

// NewTokenProvider creates a new token provider from an mcp-oauth TokenStore.
func NewTokenProvider(store storage.TokenStore) *TokenProvider {
	return &TokenProvider{
		store: store,
	}
}

// GetToken retrieves a Google OAuth token for the given user ID.
func (p *TokenProvider) GetToken(ctx context.Context, userID string) (*oauth2.Token, error) {
	return p.store.GetToken(ctx, userID)
}

// SaveToken saves a Google OAuth token for the given user ID.
// This is used when tokens are refreshed or initially acquired.
func (p *TokenProvider) SaveToken(ctx context.Context, userID string, token *oauth2.Token) error {
	return p.store.SaveToken(ctx, userID, token)
}

// UserInfo represents Google user information.
// This is a convenience wrapper around the library's providers.UserInfo type.
type UserInfo = providers.UserInfo

// GetUserFromContext retrieves the authenticated user info from the request context.
// This is set by the OAuth middleware after validating the Bearer token.
// Returns the user info and true if present, or nil and false if not authenticated.
func GetUserFromContext(ctx context.Context) (*UserInfo, bool) {
	return oauth.UserInfoFromContext(ctx)
}

// ContextWithUserInfo creates a context with the given user info.
// This is useful for testing code that depends on authenticated user context.
func ContextWithUserInfo(ctx context.Context, userInfo *UserInfo) context.Context {
	return oauth.ContextWithUserInfo(ctx, userInfo)
}
