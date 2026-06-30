package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/giantswarm/mcp-oauth/handler"
	"github.com/giantswarm/mcp-oauth/security"
)

// EventTokenMissingIdentity is the audit event type emitted when a structurally
// valid bearer token reaches /mcp without an email claim, so Kubernetes
// impersonation cannot proceed.
const EventTokenMissingIdentity = "token_missing_identity"

// RequireIdentity rejects requests whose validated UserInfo carries no email
// claim. Without an email there is no Impersonate-User to send to the
// Kubernetes API; the request would otherwise pass through and fail later
// with a misleading "unknown resource type" error from the discovery client.
//
// Must be wired after the OAuth ValidateToken middleware so UserInfo is on
// the context. Requests without UserInfo are passed through unchanged
// (ValidateToken already handled those).
func RequireIdentity(auditor *security.Auditor, logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userInfo, ok := handler.UserInfoFromContext(r.Context())
			if !ok || userInfo == nil {
				next.ServeHTTP(w, r)
				return
			}

			if userInfo.Email != "" {
				next.ServeHTTP(w, r)
				return
			}
			// External-issuer (OBO) tokens carry no email in UserInfo;
			// AccessTokenInjector handles their identity enforcement.
			if userInfo.IsExternalIssuer() {
				next.ServeHTTP(w, r)
				return
			}

			bearer := extractBearerToken(r)
			issuer := jwtIssuer(bearer)

			if auditor != nil {
				auditor.LogEvent(r.Context(), security.Event{
					Type:      EventTokenMissingIdentity,
					UserID:    userInfo.ID,
					IPAddress: security.GetClientIP(r, false, 0),
					UserAgent: r.UserAgent(),
					Details: map[string]any{
						"validation_method": string(userInfo.TokenSource),
						"jwt_issuer":        issuer,
						"group_count":       len(userInfo.Groups),
					},
				})
			}

			logger.WarnContext(r.Context(),
				"rejecting request: bearer token has no email claim",
				slog.String("validation_method", string(userInfo.TokenSource)),
				slog.String("jwt_issuer", issuer),
				slog.Int("group_count", len(userInfo.Groups)),
			)

			const desc = "bearer token has no 'email' claim; impersonation requires an email"
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer error="invalid_token", error_description=%q`, desc))
			http.Error(w, desc, http.StatusUnauthorized)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return auth[len(prefix):]
}

// jwtIssuer extracts the iss claim from a JWT without verifying the signature.
// The token was already validated upstream; this read is for audit logging
// only. Returns an empty string when the input is not a JWT.
func jwtIssuer(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Iss
}
