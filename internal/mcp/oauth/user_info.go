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
func ToFederationUserInfo(user *UserInfo) *federation.UserInfo {
	if user == nil {
		return nil
	}

	fedUser := &federation.UserInfo{
		Email:  user.Email,
		Groups: user.Groups,
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

// ValidateUserInfoForImpersonation checks if the UserInfo contains the minimum
// required fields for Kubernetes impersonation.
//
// At minimum, the Email field must be non-empty since it's used as the
// Impersonate-User header value. Groups are optional but recommended for
// proper RBAC evaluation.
func ValidateUserInfoForImpersonation(user *UserInfo) error {
	if user == nil {
		return federation.ErrUserInfoRequired
	}
	if user.Email == "" {
		return federation.ErrUserEmailRequired
	}
	return nil
}
