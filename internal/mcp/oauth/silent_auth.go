package oauth

import (
	mcpoauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/providers"
)

// AuthorizationURLOptions contains optional OIDC parameters for the authorization request.
// These parameters enable advanced authentication flows like silent re-authentication
// and user hints per OpenID Connect Core 1.0 Section 3.1.2.1.
//
// This is a type alias for the mcp-oauth library's providers.AuthorizationURLOptions type.
//
// Key parameters for silent authentication:
//   - Prompt: Set to "none" for silent authentication (no UI displayed)
//   - LoginHint: Pre-fill the user's email for faster re-authentication
//   - IDTokenHint: Pass a previously issued ID token as a session hint
//
// Example for silent re-authentication:
//
//	opts := &oauth.AuthorizationURLOptions{
//	    Prompt:    "none",
//	    LoginHint: "user@example.com",
//	}
//
// See: https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest
type AuthorizationURLOptions = providers.AuthorizationURLOptions

// SilentAuthError represents an error from a silent authentication attempt.
// These errors indicate the IdP requires user interaction and the client
// should fall back to interactive login.
//
// This is a type alias for the mcp-oauth library's SilentAuthError type.
//
// Silent authentication fails when:
//   - No active session at the IdP (login_required)
//   - User hasn't granted required scopes (consent_required)
//   - IdP needs user interaction for other reasons (interaction_required)
//   - Multiple accounts and none selected (account_selection_required)
//
// See: https://openid.net/specs/openid-connect-core-1_0.html#AuthError
type SilentAuthError = mcpoauth.SilentAuthError

// CallbackResult represents the result of an OAuth authorization callback.
// It parses and holds the query parameters from the OAuth redirect.
//
// This is a type alias for the mcp-oauth library's CallbackResult type.
//
// The callback may contain either:
//   - Success: Code and State parameters
//   - Error: Error, ErrorDescription, and optionally ErrorURI parameters
//
// Use Err() to get a typed error for error responses, including SilentAuthError
// for silent authentication failures.
type CallbackResult = mcpoauth.CallbackResult

// ErrSilentAuthFailed is a sentinel error for when silent authentication is not possible.
// This occurs when the IdP requires user interaction (login or consent) but the
// authorization request used prompt=none for silent authentication.
//
// Use IsSilentAuthError to check if an error indicates silent auth failure.
var ErrSilentAuthFailed = mcpoauth.ErrSilentAuthFailed

// Silent authentication error codes (OIDC Core Section 3.1.2.6).
// These indicate the IdP requires user interaction and silent auth failed.
const (
	// ErrorCodeLoginRequired indicates the user is not logged in at the IdP.
	// The client should redirect to interactive login.
	ErrorCodeLoginRequired = mcpoauth.ErrorCodeLoginRequired

	// ErrorCodeConsentRequired indicates the user hasn't consented to the requested scopes.
	// The client should redirect to interactive login with consent.
	ErrorCodeConsentRequired = mcpoauth.ErrorCodeConsentRequired

	// ErrorCodeInteractionRequired indicates the IdP requires user interaction
	// for reasons not covered by login_required or consent_required.
	ErrorCodeInteractionRequired = mcpoauth.ErrorCodeInteractionRequired

	// ErrorCodeAccountSelectionRequired indicates multiple accounts are available
	// and the user must select one.
	ErrorCodeAccountSelectionRequired = mcpoauth.ErrorCodeAccountSelectionRequired
)

// IsSilentAuthError returns true if the error indicates silent authentication failed
// and interactive login is required. This checks for:
//   - *SilentAuthError type (including wrapped errors)
//   - Error strings containing known silent auth error codes
//
// Use this function to detect when to fall back from silent to interactive login.
//
// Example usage:
//
//	result := handleCallback(r)
//	if err := result.Err(); err != nil {
//	    if oauth.IsSilentAuthError(err) {
//	        // Fall back to interactive login
//	        return startInteractiveLogin(w, r)
//	    }
//	    // Handle other errors
//	    return handleError(w, err)
//	}
func IsSilentAuthError(err error) bool {
	return mcpoauth.IsSilentAuthError(err)
}

// ParseOAuthError parses an OAuth error response and returns the appropriate error type.
// For silent auth failure codes (login_required, consent_required, interaction_required,
// account_selection_required), returns a *SilentAuthError.
// For other errors, returns a generic error with the code and description.
// Returns nil if errorCode is empty.
//
// Example usage:
//
//	err := oauth.ParseOAuthError(r.URL.Query().Get("error"), r.URL.Query().Get("error_description"))
//	if err != nil {
//	    if oauth.IsSilentAuthError(err) {
//	        // Handle silent auth failure
//	    }
//	}
func ParseOAuthError(errorCode, errorDescription string) error {
	return mcpoauth.ParseOAuthError(errorCode, errorDescription)
}

// ParseCallbackQuery creates a CallbackResult from URL query parameters.
// This is a convenience function for parsing OAuth callback query strings.
//
// Parameters:
//   - code: The authorization code (from "code" query param)
//   - state: The state parameter (from "state" query param)
//   - errorCode: The error code (from "error" query param)
//   - errorDescription: The error description (from "error_description" query param)
//   - errorURI: The error URI (from "error_uri" query param)
//
// Example usage:
//
//	q := r.URL.Query()
//	result := oauth.ParseCallbackQuery(
//	    q.Get("code"),
//	    q.Get("state"),
//	    q.Get("error"),
//	    q.Get("error_description"),
//	    q.Get("error_uri"),
//	)
//	if err := result.Err(); err != nil {
//	    if oauth.IsSilentAuthError(err) {
//	        // Silent auth failed, fall back to interactive login
//	        return startInteractiveLogin(w, r)
//	    }
//	    return handleError(w, err)
//	}
//	// Process result.Code
func ParseCallbackQuery(code, state, errorCode, errorDescription, errorURI string) *CallbackResult {
	return mcpoauth.ParseCallbackQuery(code, state, errorCode, errorDescription, errorURI)
}
