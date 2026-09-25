package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireBearerToken rejects every request whose Authorization header does not
// carry the static bearer token. It protects the MCP endpoints of the HTTP
// transports when OAuth is not enabled; the comparison is constant-time.
func RequireBearerToken(token string) func(http.Handler) http.Handler {
	expected := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || subtle.ConstantTimeCompare([]byte(presented), expected) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="mcp-kubernetes"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
