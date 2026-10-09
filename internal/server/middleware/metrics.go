package middleware

import (
	"net/http"
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
// The path label is the route pattern the http.ServeMux inside the handler
// chain matched (http.Request.Pattern), never the raw request path: the label
// set stays bounded by the registered routes, and a path that is not valid
// UTF-8, which the Prometheus registry refuses at scrape time and which would
// fail every later scrape, never reaches a label. A request no route matched
// is labelled unmatchedRoute.
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

			// Call the next handler; the ServeMux it reaches sets r.Pattern
			next.ServeHTTP(wrapped, r)

			provider.Metrics().RecordHTTPRequest(
				r.Context(),
				r.Method,
				routeLabel(r),
				wrapped.statusCode,
				time.Since(start),
			)
		})
	}
}

// unmatchedRoute is the path label of a request no registered route matched.
const unmatchedRoute = "unmatched"

// routeLabel returns the bounded path label of a served request: the pattern
// the ServeMux matched, or unmatchedRoute when none did (a 404, a redirect to
// the clean path, or a handler chain without a ServeMux).
func routeLabel(r *http.Request) string {
	if r.Pattern == "" {
		return unmatchedRoute
	}
	return r.Pattern
}
