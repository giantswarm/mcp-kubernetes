package middleware

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecurityHeaders tests that security headers are properly set
func TestSecurityHeaders(t *testing.T) {
	tests := []struct {
		name        string
		hstsEnabled bool
		hasTLS      bool
		wantHSTS    bool
	}{
		{
			name:        "HSTS enabled with TLS",
			hstsEnabled: true,
			hasTLS:      true,
			wantHSTS:    true,
		},
		{
			name:        "HSTS enabled without TLS",
			hstsEnabled: true,
			hasTLS:      false,
			wantHSTS:    true,
		},
		{
			name:        "HSTS disabled with TLS",
			hstsEnabled: false,
			hasTLS:      true,
			wantHSTS:    true,
		},
		{
			name:        "HSTS disabled without TLS",
			hstsEnabled: false,
			hasTLS:      false,
			wantHSTS:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := SecurityHeaders(tt.hstsEnabled)(handler)

			req := httptest.NewRequest("GET", "/", nil)
			if tt.hasTLS {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			// Check common security headers
			assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
			assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
			assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "default-src 'self'")
			assert.Contains(t, rec.Header().Get("Permissions-Policy"), "geolocation=()")
			assert.Equal(t, "same-origin", rec.Header().Get("Cross-Origin-Opener-Policy"))
			assert.Equal(t, "require-corp", rec.Header().Get("Cross-Origin-Embedder-Policy"))
			assert.Equal(t, "same-origin", rec.Header().Get("Cross-Origin-Resource-Policy"))

			// Check HSTS header
			if tt.wantHSTS {
				assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=31536000")
			} else {
				assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
			}
		})
	}
}

// TestCORS tests CORS header handling
func TestCORS(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		method         string
		wantCORS       bool
		wantStatus     int
	}{
		{
			name:           "allowed origin",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://example.com",
			method:         "GET",
			wantCORS:       true,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "disallowed origin",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://evil.com",
			method:         "GET",
			wantCORS:       false,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "no origin header",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "",
			method:         "GET",
			wantCORS:       false,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "empty allowed origins list",
			allowedOrigins: []string{},
			requestOrigin:  "https://example.com",
			method:         "GET",
			wantCORS:       false,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "OPTIONS preflight request",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://example.com",
			method:         "OPTIONS",
			wantCORS:       true,
			wantStatus:     http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := CORS(tt.allowedOrigins)(handler)

			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			// Check CORS headers
			assert.Equal(t, "GET, POST, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
			assert.Equal(t, "Authorization, Content-Type", rec.Header().Get("Access-Control-Allow-Headers"))
			assert.Equal(t, "3600", rec.Header().Get("Access-Control-Max-Age"))

			if tt.wantCORS {
				assert.Equal(t, tt.requestOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "Origin", rec.Header().Get("Vary"))
			} else {
				assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestValidateAllowedOrigins tests CORS origin validation
func TestValidateAllowedOrigins(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      []string
		wantError bool
	}{
		{
			name:      "empty string",
			input:     "",
			want:      nil,
			wantError: false,
		},
		{
			name:      "single valid origin",
			input:     "https://example.com",
			want:      []string{"https://example.com"},
			wantError: false,
		},
		{
			name:      "multiple valid origins",
			input:     "https://example.com,https://another.com",
			want:      []string{"https://example.com", "https://another.com"},
			wantError: false,
		},
		{
			name:      "origins with trailing slash normalized",
			input:     "https://example.com/",
			want:      []string{"https://example.com"},
			wantError: false,
		},
		{
			name:      "whitespace trimmed",
			input:     " https://example.com , https://another.com ",
			want:      []string{"https://example.com", "https://another.com"},
			wantError: false,
		},
		{
			name:      "invalid URL format",
			input:     "not-a-url",
			want:      nil,
			wantError: true,
		},
		{
			name:      "missing scheme",
			input:     "example.com",
			want:      nil,
			wantError: true,
		},
		{
			name:      "invalid scheme",
			input:     "ftp://example.com",
			want:      nil,
			wantError: true,
		},
		{
			name:      "URL with path not allowed",
			input:     "https://example.com/path",
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAllowedOrigins(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestMaxRequestSize tests request body size limiting middleware
func TestMaxRequestSize(t *testing.T) {
	tests := []struct {
		name           string
		maxBytes       int64
		bodySize       int
		wantBodyRead   bool
		wantReadError  bool
		wantBytesRead  int // Number of bytes expected to be read (limited by maxBytes)
	}{
		{
			name:          "request within limit",
			maxBytes:      1024,
			bodySize:      100,
			wantBodyRead:  true,
			wantReadError: false,
			wantBytesRead: 100,
		},
		{
			name:          "request exactly at limit",
			maxBytes:      1024,
			bodySize:      1024,
			wantBodyRead:  true,
			wantReadError: false,
			wantBytesRead: 1024,
		},
		{
			name:          "request exceeds limit",
			maxBytes:      1024,
			bodySize:      2048,
			wantBodyRead:  false,
			wantReadError: true,
			wantBytesRead: 0, // ReadAll returns error, partial read not guaranteed
		},
		{
			name:          "limit disabled with zero",
			maxBytes:      0,
			bodySize:      10000,
			wantBodyRead:  true,
			wantReadError: false,
			wantBytesRead: 10000,
		},
		{
			name:          "limit disabled with negative",
			maxBytes:      -1,
			bodySize:      10000,
			wantBodyRead:  true,
			wantReadError: false,
			wantBytesRead: 10000,
		},
		{
			name:          "empty body within limit",
			maxBytes:      1024,
			bodySize:      0,
			wantBodyRead:  true,
			wantReadError: false,
			wantBytesRead: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyWasRead bool
			var readError error
			var bytesRead int
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Try to read the body to trigger the limit check
				body, err := io.ReadAll(r.Body)
				readError = err
				if err == nil {
					bodyWasRead = true
					bytesRead = len(body)
					w.WriteHeader(http.StatusOK)
				} else {
					// Body read failed - MaxBytesReader returns an error
					// In production, the ResponseWriter may set 413 automatically
					// For testing with httptest.ResponseRecorder, we manually set it
					w.WriteHeader(http.StatusRequestEntityTooLarge)
				}
			})

			middleware := MaxRequestSize(tt.maxBytes)(handler)

			// Create a body of the specified size
			body := strings.Repeat("a", tt.bodySize)
			req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
			req.ContentLength = int64(tt.bodySize)
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			if tt.wantBodyRead {
				assert.True(t, bodyWasRead, "expected body to be read successfully")
				assert.NoError(t, readError)
				assert.Equal(t, tt.wantBytesRead, bytesRead)
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				// When body exceeds limit, ReadAll returns an error
				assert.Error(t, readError, "expected read error for body exceeding limit")
				assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
			}
		})
	}
}

// TestMaxRequestSizePassthrough tests that middleware passes through when disabled
func TestMaxRequestSizePassthrough(t *testing.T) {
	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test with zero maxBytes (disabled)
	middleware := MaxRequestSize(0)(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "expected handler to be called")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestMaxRequestSizeChunkedTransfer tests handling of chunked transfer encoding
func TestMaxRequestSizeChunkedTransfer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := MaxRequestSize(100)(handler)

	// Create a chunked body that exceeds the limit
	body := strings.Repeat("a", 200)
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	// Don't set ContentLength to simulate chunked transfer
	req.ContentLength = -1
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	// The middleware should return 413 when the body exceeds the limit
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
