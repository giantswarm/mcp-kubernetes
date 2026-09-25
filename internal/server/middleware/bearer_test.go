package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireBearerToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	handler := RequireBearerToken(token)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "matching token", authorization: "Bearer " + token, wantStatus: http.StatusOK},
		{name: "no header", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", authorization: "Bearer " + token[:31] + "0", wantStatus: http.StatusUnauthorized},
		{name: "token prefix only", authorization: "Bearer " + token[:16], wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", authorization: "Basic " + token, wantStatus: http.StatusUnauthorized},
		{name: "empty bearer", authorization: "Bearer ", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusUnauthorized {
				assert.Equal(t, `Bearer realm="mcp-kubernetes"`, rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}
