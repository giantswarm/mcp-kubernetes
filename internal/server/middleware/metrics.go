package middleware

import (
	"net/http"
	"regexp"
	"time"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// newResponseWriter creates a new responseWriter wrapper.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status code
	}
}

// WriteHeader captures the status code before writing the header.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures that a response was written.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter to support http.Flusher etc.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Flush implements http.Flusher for streaming responses.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// HTTPMetrics creates middleware that records HTTP request metrics.
// It records the total number of requests and request duration for each
// method/path/status combination.
//
// The middleware normalizes paths to prevent high cardinality:
// - /mcp/{session_id} -> /mcp/:id (for session-based endpoints)
// - UUID patterns are replaced with :id
//
// The provider parameter can be nil, in which case the middleware is a no-op
// that just passes through to the next handler.
func HTTPMetrics(provider *instrumentation.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip metrics recording if provider is nil or disabled
			if provider == nil || !provider.Enabled() {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Wrap the response writer to capture the status code
			wrapped := newResponseWriter(w)

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Record the metrics
			duration := time.Since(start)
			path := normalizePath(r.URL.Path)

			provider.Metrics().RecordHTTPRequest(
				r.Context(),
				r.Method,
				path,
				wrapped.statusCode,
				duration,
			)
		})
	}
}

// Regex patterns for path normalization to control metric cardinality
var (
	// UUID pattern (e.g., 550e8400-e29b-41d4-a716-446655440000)
	uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

	// Session ID pattern for MCP streamable HTTP (alphanumeric, typically 8-32 chars)
	sessionIDPattern = regexp.MustCompile(`^/mcp/[a-zA-Z0-9_-]{8,64}$`)

	// Generic numeric ID pattern in paths
	numericIDPattern = regexp.MustCompile(`/\d+(/|$)`)
)

// normalizePath normalizes URL paths to prevent high cardinality in metrics.
// This replaces dynamic path segments (UUIDs, session IDs, numeric IDs) with
// placeholder values to ensure bounded metric cardinality.
func normalizePath(path string) string {
	// Handle MCP session endpoints (e.g., /mcp/abc123xyz)
	if sessionIDPattern.MatchString(path) {
		return "/mcp/:session"
	}

	// Replace UUIDs with :uuid
	path = uuidPattern.ReplaceAllString(path, ":uuid")

	// Replace numeric IDs in paths with :id
	path = numericIDPattern.ReplaceAllString(path, "/:id$1")

	return path
}
