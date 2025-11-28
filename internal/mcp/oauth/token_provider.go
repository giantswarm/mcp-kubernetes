package oauth

import (
	"context"

	oauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/storage"
	"golang.org/x/oauth2"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// accessTokenKey is the context key for storing the user's OAuth access token.
	// This token can be used for downstream Kubernetes API authentication.
	accessTokenKey contextKey = "oauth_access_token"
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

// ContextWithAccessToken creates a context with the given OAuth access token.
// This is used to pass the user's Google OAuth access token for downstream
// Kubernetes API authentication.
func ContextWithAccessToken(ctx context.Context, accessToken string) context.Context {
	return context.WithValue(ctx, accessTokenKey, accessToken)
}

// GetAccessTokenFromContext retrieves the OAuth access token from the context.
// This returns the user's Google OAuth access token that can be used for
// downstream Kubernetes API authentication.
// Returns the access token and true if present, or empty string and false if not available.
func GetAccessTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(accessTokenKey).(string)
	return token, ok && token != ""
}
