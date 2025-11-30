package oauth

import (
	"context"

	oauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/storage"
	"golang.org/x/oauth2"
)

// contextKey is a custom type for context keys to avoid collisions.
// Using a custom type instead of a plain string prevents key collisions
// with other packages that might use the same string key in the context.
// This is a Go best practice recommended in the context package documentation:
// https://pkg.go.dev/context#WithValue
type contextKey string

const (
	// accessTokenKey is the context key for storing the user's OAuth access token.
	// This token can be used for downstream Kubernetes API authentication.
	// The custom contextKey type ensures this key cannot collide with string keys
	// from other packages.
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

// ContextWithAccessToken creates a context with the given OAuth ID token.
// This is used to pass the user's Google OAuth ID token for downstream
// Kubernetes OIDC authentication.
// Note: Kubernetes OIDC requires the ID token, not the access token.
func ContextWithAccessToken(ctx context.Context, idToken string) context.Context {
	return context.WithValue(ctx, accessTokenKey, idToken)
}

// GetAccessTokenFromContext retrieves the OAuth ID token from the context.
// This returns the user's Google OAuth ID token that can be used for
// downstream Kubernetes OIDC authentication.
// Returns the ID token and true if present, or empty string and false if not available.
func GetAccessTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(accessTokenKey).(string)
	return token, ok && token != ""
}

// GetIDToken extracts the ID token from an OAuth2 token.
// Google OAuth responses include an id_token in the Extra data.
// Kubernetes OIDC authentication requires the ID token, not the access token.
func GetIDToken(token *oauth2.Token) string {
	if token == nil {
		return ""
	}
	idToken := token.Extra("id_token")
	if idToken == nil {
		return ""
	}
	if s, ok := idToken.(string); ok {
		return s
	}
	return ""
}
