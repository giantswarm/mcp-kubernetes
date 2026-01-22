// Package oauth provides adapters for integrating the github.com/giantswarm/mcp-oauth
// library with the mcp-kubernetes MCP server.
//
// This package bridges the mcp-oauth library with our existing server architecture,
// providing token provider integration and configuration mapping for Kubernetes
// contexts that may require OAuth authentication.
//
// # User Info Integration
//
// This package provides convenience functions for accessing authenticated user
// information from request contexts:
//
//   - [UserInfoFromContext]: Retrieves the full UserInfo from context
//   - [HasUserInfo]: Checks if user info is present in context
//   - [GetUserEmailFromContext]: Extracts just the user's email
//   - [GetUserGroupsFromContext]: Extracts the user's group memberships
//
// For Kubernetes impersonation, use the conversion functions:
//
//   - [ToFederationUserInfo]: Converts OAuth UserInfo to federation.UserInfo
//   - [ToFederationUserInfoWithExtra]: Converts with additional claims
//   - [ValidateUserInfoForImpersonation]: Validates minimum required fields
//
// # Silent Authentication
//
// This package provides types and functions for OIDC silent re-authentication,
// which enables seamless token refresh without user interaction when an IdP
// session already exists:
//
//   - [AuthorizationURLOptions]: OIDC parameters for authorization requests (prompt=none, login_hint, etc.)
//   - [SilentAuthError]: Error type for silent authentication failures
//   - [IsSilentAuthError]: Detects if an error indicates silent auth failed
//   - [ParseOAuthError]: Parses OAuth error responses into appropriate types
//   - [ParseCallbackQuery]: Convenience function for parsing OAuth callback parameters
//   - [CallbackResult]: Structured result type for OAuth callbacks
//
// Silent authentication workflow:
//
//  1. Client builds authorization URL with prompt=none
//  2. If IdP has active session, user gets new tokens without interaction
//  3. If no session, IdP returns login_required/consent_required error
//  4. Client detects via IsSilentAuthError() and falls back to interactive login
//
// Example:
//
//	// Build silent auth request
//	opts := &oauth.AuthorizationURLOptions{
//	    Prompt:    "none",
//	    LoginHint: "user@example.com",
//	}
//
//	// Handle callback
//	result := oauth.ParseCallbackQuery(code, state, errCode, errDesc, errURI)
//	if err := result.Err(); err != nil {
//	    if oauth.IsSilentAuthError(err) {
//	        // Fall back to interactive login
//	        return startInteractiveLogin(w, r)
//	    }
//	    return handleError(w, err)
//	}
//
// See: https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest
//
// # Dependency Security Note
//
// This package depends on github.com/giantswarm/mcp-oauth for OAuth 2.1 implementation.
// The library provides: PKCE enforcement, refresh token rotation, rate limiting, and audit logging.
// Security posture: Actively maintained, implements OAuth 2.1 specification.
package oauth
