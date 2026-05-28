package middleware

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giantswarm/mcp-oauth/handler"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireIdentity(t *testing.T) {
	const issuer = "https://muster.glean.azuretest.gigantic.io"

	tests := []struct {
		name           string
		userInfo       *providers.UserInfo
		wantStatus     int
		wantPassedThru bool
		wantWWWAuth    bool
		wantAudit      bool
	}{
		{
			name:           "no user info on context passes through",
			userInfo:       nil,
			wantStatus:     http.StatusOK,
			wantPassedThru: true,
		},
		{
			name: "user info with email passes through",
			userInfo: &providers.UserInfo{
				ID:          "user-123",
				Email:       "alice@example.com",
				TokenSource: providers.TokenSourceJWT,
			},
			wantStatus:     http.StatusOK,
			wantPassedThru: true,
		},
		{
			name: "empty email is rejected with 401",
			userInfo: &providers.UserInfo{
				ID:          "system:serviceaccount:ns:sa",
				Email:       "",
				TokenSource: providers.TokenSourceJWT,
			},
			wantStatus:  http.StatusUnauthorized,
			wantWWWAuth: true,
			wantAudit:   true,
		},
		{
			name: "empty email with groups is still rejected (impersonation needs Impersonate-User)",
			userInfo: &providers.UserInfo{
				ID:          "system:serviceaccount:ns:sa",
				Email:       "",
				Groups:      []string{"system:masters"},
				TokenSource: providers.TokenSourceJWT,
			},
			wantStatus:  http.StatusUnauthorized,
			wantWWWAuth: true,
			wantAudit:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var auditBuf bytes.Buffer
			auditor := security.NewAuditor(
				slog.New(slog.NewJSONHandler(&auditBuf, nil)),
				true,
			)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			var nextCalled bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			mw := RequireIdentity(auditor, logger)(next)

			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
			if tt.userInfo != nil || tt.name == "no user info on context passes through" {
				ctx := req.Context()
				if tt.userInfo != nil {
					ctx = handler.ContextWithUserInfo(ctx, tt.userInfo)
				}
				req = req.WithContext(ctx)
			}
			req.Header.Set("Authorization", "Bearer "+fakeJWT(t, issuer))

			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantPassedThru, nextCalled)

			if tt.wantWWWAuth {
				wa := rec.Header().Get("WWW-Authenticate")
				assert.Contains(t, wa, `error="invalid_token"`)
				assert.Contains(t, wa, "no 'email' claim")
			} else {
				assert.Empty(t, rec.Header().Get("WWW-Authenticate"))
			}

			if tt.wantAudit {
				var auditRecord map[string]any
				dec := json.NewDecoder(&auditBuf)
				require.NoError(t, dec.Decode(&auditRecord))
				audit, ok := auditRecord["audit"].(map[string]any)
				require.True(t, ok, "audit group missing in log record")
				assert.Equal(t, EventTokenMissingIdentity, audit["event_type"])
				details, ok := audit["details"].(map[string]any)
				require.True(t, ok, "audit details missing")
				assert.Equal(t, string(tt.userInfo.TokenSource), details["validation_method"])
				assert.Equal(t, issuer, details["jwt_issuer"])
			}
		})
	}
}

func TestRequireIdentity_NilAuditorDoesNotPanic(t *testing.T) {
	mw := RequireIdentity(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(handler.ContextWithUserInfo(req.Context(), &providers.UserInfo{
		TokenSource: providers.TokenSourceJWT,
	}))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTIssuer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid jwt with iss",
			input: fakeJWT(t, "https://issuer.example"),
			want:  "https://issuer.example",
		},
		{
			name:  "not a jwt",
			input: "opaque-string",
			want:  "",
		},
		{
			name:  "malformed payload segment",
			input: "header.not-base64!.sig",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jwtIssuer(tt.input))
		})
	}
}

// fakeJWT returns a structurally valid JWT (header.payload.sig) with the given
// issuer. The signature is not real; the middleware does not verify it.
func fakeJWT(t *testing.T, issuer string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]string{"iss": issuer, "sub": "test"})
	require.NoError(t, err)
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}
