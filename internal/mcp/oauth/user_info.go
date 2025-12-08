package oauth

import (
	"github.com/giantswarm/mcp-kubernetes/internal/federation"
)

// ToFederationUserInfo converts a UserInfo (from OAuth provider) to a federation.UserInfo.
// This enables using OAuth-authenticated user info for Kubernetes impersonation
// in multi-cluster operations.
//
// The conversion maps:
//   - UserInfo.Email -> federation.UserInfo.Email (used as Impersonate-User)
//   - UserInfo.Groups -> federation.UserInfo.Groups (used as Impersonate-Group)
//   - UserInfo.ID -> federation.UserInfo.Extra["sub"] (subject claim)
//
// Returns nil if the input user info is nil.
//
// Note: This function creates a defensive copy of the Groups slice to prevent
// unintended modifications from affecting the original UserInfo.
func ToFederationUserInfo(user *UserInfo) *federation.UserInfo {
	if user == nil {
		return nil
	}

	// Create defensive copy of groups to prevent shared slice mutations
	var groupsCopy []string
	if len(user.Groups) > 0 {
		groupsCopy = make([]string, len(user.Groups))
		copy(groupsCopy, user.Groups)
	}

	fedUser := &federation.UserInfo{
		Email:  user.Email,
		Groups: groupsCopy,
	}

	// Add the subject (ID) to extra claims if present
	if user.ID != "" {
		fedUser.Extra = map[string][]string{
			"sub": {user.ID},
		}
	}

	return fedUser
}

// ToFederationUserInfoWithExtra converts a UserInfo to federation.UserInfo with additional
// extra claims. This is useful when you need to pass additional context beyond the
// standard OAuth claims.
//
// The extra map is merged with the automatically extracted claims (like "sub").
// Caller-provided values take precedence over auto-extracted values.
//
// # Security Warning
//
// The extra parameter is merged directly into the federation.UserInfo.Extra map,
// which is used for Kubernetes Impersonate-Extra headers. Callers MUST ensure that:
//   - The extra map contains only trusted, validated data
//   - Values do not originate from untrusted user input without validation
//   - Keys and values comply with Kubernetes impersonation header requirements
//
// Failure to validate the extra parameter could lead to impersonation of
// unintended identities or injection of malicious header values.
func ToFederationUserInfoWithExtra(user *UserInfo, extra map[string][]string) *federation.UserInfo {
	fedUser := ToFederationUserInfo(user)
	if fedUser == nil {
		return nil
	}

	// Merge extra claims
	if len(extra) > 0 {
		if fedUser.Extra == nil {
			fedUser.Extra = make(map[string][]string)
		}
		for k, v := range extra {
			fedUser.Extra[k] = v
		}
	}

	return fedUser
}

// ValidateUserInfoForImpersonation performs comprehensive validation of UserInfo
// for Kubernetes impersonation use cases.
//
// This function validates:
//   - User is not nil (ErrUserInfoRequired)
//   - Email is not empty (ErrUserEmailRequired) - used as Impersonate-User header
//   - All fields pass federation.ValidateUserInfo security checks including:
//   - Email format and length validation
//   - Group name validation (length, control characters)
//   - Extra header key/value validation
//
// This provides defense-in-depth by ensuring that user info is both present
// and safe for use in HTTP headers before impersonation occurs.
func ValidateUserInfoForImpersonation(user *UserInfo) error {
	if user == nil {
		return federation.ErrUserInfoRequired
	}
	if user.Email == "" {
		return federation.ErrUserEmailRequired
	}

	// Perform full validation using federation layer validation
	// This checks email format, group names, and extra header safety
	fedUser := ToFederationUserInfo(user)
	if err := federation.ValidateUserInfo(fedUser); err != nil {
		return err
	}

	return nil
}
