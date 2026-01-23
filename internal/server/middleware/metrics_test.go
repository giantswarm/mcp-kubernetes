package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponseWriter_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		expectedCode int
	}{
		{
			name:         "captures 200 OK",
			statusCode:   http.StatusOK,
			expectedCode: http.StatusOK,
		},
		{
			name:         "captures 404 Not Found",
			statusCode:   http.StatusNotFound,
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "captures 500 Internal Server Error",
			statusCode:   http.StatusInternalServerError,
			expectedCode: http.StatusInternalServerError,
		},
		{
			name:         "captures 201 Created",
			statusCode:   http.StatusCreated,
			expectedCode: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			rw := newResponseWriter(recorder)

			rw.WriteHeader(tt.statusCode)

			assert.Equal(t, tt.expectedCode, rw.statusCode)
			assert.True(t, rw.written)
		})
	}
}

func TestResponseWriter_DefaultsTo200(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	// Write response body without explicitly setting status
	_, err := rw.Write([]byte("hello"))
	assert.NoError(t, err)

	// Default status should be 200 OK
	assert.Equal(t, http.StatusOK, rw.statusCode)
	assert.True(t, rw.written)
}

func TestResponseWriter_OnlyFirstWriteHeaderCounts(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	rw.WriteHeader(http.StatusAccepted)
	rw.WriteHeader(http.StatusBadRequest) // This should be ignored

	assert.Equal(t, http.StatusAccepted, rw.statusCode)
}

func TestResponseWriter_Flush(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	// Should not panic even if underlying doesn't support Flush
	rw.Flush()
}

func TestResponseWriter_Unwrap(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	assert.Equal(t, recorder, rw.Unwrap())
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple path unchanged",
			input:    "/mcp",
			expected: "/mcp",
		},
		{
			name:     "health endpoint unchanged",
			input:    "/healthz",
			expected: "/healthz",
		},
		{
			name:     "readiness endpoint unchanged",
			input:    "/readyz",
			expected: "/readyz",
		},
		{
			name:     "metrics endpoint unchanged",
			input:    "/metrics",
			expected: "/metrics",
		},
		{
			name:     "oauth authorize unchanged",
			input:    "/oauth/authorize",
			expected: "/oauth/authorize",
		},
		{
			name:     "oauth token unchanged",
			input:    "/oauth/token",
			expected: "/oauth/token",
		},
		{
			name:     "MCP session ID normalized",
			input:    "/mcp/abc123xyz890def456",
			expected: "/mcp/:session",
		},
		{
			name:     "MCP session ID with dashes normalized",
			input:    "/mcp/session-id-12345",
			expected: "/mcp/:session",
		},
		{
			name:     "MCP session ID with underscores normalized",
			input:    "/mcp/session_id_12345",
			expected: "/mcp/:session",
		},
		{
			name:     "UUID normalized",
			input:    "/api/resources/550e8400-e29b-41d4-a716-446655440000",
			expected: "/api/resources/:uuid",
		},
		{
			name:     "multiple UUIDs normalized",
			input:    "/api/550e8400-e29b-41d4-a716-446655440000/sub/660e8400-e29b-41d4-a716-446655440001",
			expected: "/api/:uuid/sub/:uuid",
		},
		{
			name:     "numeric ID normalized",
			input:    "/api/items/12345",
			expected: "/api/items/:id",
		},
		{
			name:     "numeric ID in middle of path",
			input:    "/api/items/12345/details",
			expected: "/api/items/:id/details",
		},
		{
			name:     "well-known path unchanged",
			input:    "/.well-known/oauth-authorization-server",
			expected: "/.well-known/oauth-authorization-server",
		},
		{
			name:     "SSE endpoint unchanged",
			input:    "/sse",
			expected: "/sse",
		},
		{
			name:     "message endpoint unchanged",
			input:    "/message",
			expected: "/message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPMetrics_NilProvider(t *testing.T) {
	// When provider is nil, the middleware should just pass through
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
}

func TestHTTPMetrics_MiddlewareChaining(t *testing.T) {
	// Test that the middleware properly chains to the next handler
	callOrder := []string{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callOrder = append(callOrder, "handler")
		w.WriteHeader(http.StatusCreated)
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("POST", "/api/resources", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, callOrder, "handler")
}

func TestHTTPMetrics_PreservesResponseBody(t *testing.T) {
	expectedBody := `{"status":"ok","data":{"id":123}}`

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, expectedBody, rec.Body.String())
}

func TestHTTPMetrics_CapturesErrorStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
