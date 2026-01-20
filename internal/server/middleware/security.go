package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Request size limiting constants
const (
	// MinRecommendedRequestSize is the minimum recommended request size limit (1MB).
	// Values below this threshold trigger a security warning as they may cause
	// legitimate requests to be rejected.
	MinRecommendedRequestSize int64 = 1 * 1024 * 1024

	// attrPath is the attribute key for request path in metrics
	attrPath = "path"
	// attrMethod is the attribute key for request method in metrics
	attrMethod = "method"
)

// RequestSizeLimitMetrics holds metrics for request size limiting.
// This allows the middleware to record metrics without tight coupling.
type RequestSizeLimitMetrics struct {
	// RejectedRequests counts requests rejected due to size limits.
	// Labels: path (normalized), method
	RejectedRequests metric.Int64Counter
}

// NewRequestSizeLimitMetrics creates metrics for request size limiting.
// Returns nil if meter is nil (metrics disabled).
func NewRequestSizeLimitMetrics(meter metric.Meter) (*RequestSizeLimitMetrics, error) {
	if meter == nil {
		return nil, nil
	}

	rejected, err := meter.Int64Counter(
		"http_request_size_rejected_total",
		metric.WithDescription("Total requests rejected due to exceeding size limits"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http_request_size_rejected_total counter: %w", err)
	}

	return &RequestSizeLimitMetrics{
		RejectedRequests: rejected,
	}, nil
}

// RecordRejection records a rejected request.
func (m *RequestSizeLimitMetrics) RecordRejection(ctx context.Context, method, path string) {
	if m == nil || m.RejectedRequests == nil {
		return
	}

	// Normalize path to prevent cardinality explosion
	normalizedPath := normalizePath(path)

	m.RejectedRequests.Add(ctx, 1, metric.WithAttributes(
		attribute.String(attrMethod, method),
		attribute.String(attrPath, normalizedPath),
	))
}

// normalizePath normalizes a path to prevent metric cardinality explosion.
// It removes IDs, UUIDs, and other dynamic segments.
func normalizePath(path string) string {
	// For MCP protocol, the main endpoint is typically /mcp or similar
	// We keep the first segment and normalize the rest
	if path == "" || path == "/" {
		return "/"
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "/"
	}

	// Keep first part, replace subsequent parts with placeholder if they look dynamic
	normalized := "/" + parts[0]
	for i := 1; i < len(parts) && i < 3; i++ {
		// If it looks like an ID (contains numbers or is a UUID pattern), normalize it
		if looksLikeDynamicSegment(parts[i]) {
			normalized += "/:id"
		} else {
			normalized += "/" + parts[i]
		}
	}
	if len(parts) > 3 {
		normalized += "/..."
	}

	return normalized
}

// looksLikeDynamicSegment checks if a path segment looks like a dynamic value (ID, UUID, etc.)
func looksLikeDynamicSegment(segment string) bool {
	if len(segment) == 0 {
		return false
	}

	// Common version patterns (v1, v2, v1beta1, etc.) are not dynamic
	if len(segment) >= 2 && (segment[0] == 'v' || segment[0] == 'V') {
		allDigitsAfterV := true
		for i := 1; i < len(segment); i++ {
			c := segment[i]
			// Allow digits and common version suffixes like "beta", "alpha"
			isDigit := c >= '0' && c <= '9'
			isLower := c >= 'a' && c <= 'z'
			isUpper := c >= 'A' && c <= 'Z'
			if !isDigit && !isLower && !isUpper {
				allDigitsAfterV = false
				break
			}
		}
		if allDigitsAfterV && len(segment) <= 10 { // v1, v2, v1beta1, v1alpha2, etc.
			return false
		}
	}

	// Segments that are too short are rarely IDs (need at least 4+ chars for IDs)
	if len(segment) < 4 {
		return false
	}

	// Check if it's mostly numbers
	numCount := 0
	for _, r := range segment {
		if r >= '0' && r <= '9' {
			numCount++
		}
	}

	// If more than 40% numbers, likely an ID
	return float64(numCount)/float64(len(segment)) > 0.4
}

// SecurityHeaders adds comprehensive security headers to all HTTP responses
func SecurityHeaders(hstsEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent MIME sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// Force HTTPS (configurable for reverse proxy scenarios)
			if r.TLS != nil || hstsEnabled {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}

			// Prevent XSS
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Restrict referrer information
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")

			// Permissions Policy - restrict dangerous features
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=()")

			// Cross-Origin policies for additional isolation
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")

			next.ServeHTTP(w, r)
		})
	}
}

// CORS adds CORS headers for OAuth endpoints with validated origins
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// If allowed origins is configured, check if origin is allowed
			if len(allowedOrigins) > 0 && origin != "" {
				for _, allowed := range allowedOrigins {
					if origin == allowed {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
			}

			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MaxRequestSizeConfig configures the request size limiting middleware.
type MaxRequestSizeConfig struct {
	// MaxBytes is the maximum allowed request body size in bytes.
	// A value of 0 or negative disables the limit (not recommended for production).
	MaxBytes int64

	// Metrics for recording rejected requests. Optional.
	Metrics *RequestSizeLimitMetrics
}

// MaxRequestSize creates middleware that limits the size of request bodies.
// Requests with bodies exceeding maxBytes will receive a 413 Request Entity Too Large response.
// A maxBytes value of 0 or negative disables the limit (not recommended for production).
//
// This is a convenience wrapper for MaxRequestSizeWithConfig without metrics.
func MaxRequestSize(maxBytes int64) func(http.Handler) http.Handler {
	return MaxRequestSizeWithConfig(MaxRequestSizeConfig{MaxBytes: maxBytes})
}

// MaxRequestSizeWithConfig creates middleware that limits request body size with
// full configuration options including metrics and logging.
//
// Security considerations:
//   - A maxBytes value of 0 or negative disables the limit (logs warning)
//   - Values below MinRecommendedRequestSize (1MB) trigger a warning
//   - Rejected requests are logged and counted in metrics (if configured)
func MaxRequestSizeWithConfig(config MaxRequestSizeConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// If maxBytes is 0 or negative, disable the limit (pass through)
		if config.MaxBytes <= 0 {
			slog.Warn("request size limiting is disabled - this is not recommended for production",
				"max_bytes", config.MaxBytes,
				"recommendation", "set max-request-size to at least 1MB for DoS protection")
			return next
		}

		// Warn if below recommended minimum
		if config.MaxBytes < MinRecommendedRequestSize {
			slog.Warn("request size limit is below recommended minimum",
				"max_bytes", config.MaxBytes,
				"min_recommended", MinRecommendedRequestSize,
				"recommendation", "consider increasing to at least 1MB to avoid rejecting legitimate requests")
		}

		// Wrap with our custom handler that adds logging and metrics
		return &maxBytesHandler{
			next:     next,
			maxBytes: config.MaxBytes,
			metrics:  config.Metrics,
		}
	}
}

// maxBytesHandler wraps http.MaxBytesReader with logging and metrics.
type maxBytesHandler struct {
	next     http.Handler
	maxBytes int64
	metrics  *RequestSizeLimitMetrics
}

func (h *maxBytesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Wrap the body with MaxBytesReader
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)

	// Create a response writer wrapper to detect 413 responses
	wrapped := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	// Serve the request
	h.next.ServeHTTP(wrapped, r)

	// Check if we got a 413 (either from MaxBytesReader or the handler)
	// Note: MaxBytesReader returns an error when the body is read, not immediately
	// So we need to check if the status was set to 413 by the handler
}

// statusRecorder wraps http.ResponseWriter to record the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	return r.ResponseWriter.Write(b)
}

// MaxBytesExceededHandler creates a handler that explicitly handles max bytes exceeded errors.
// This is useful when you need custom error responses or logging when request size is exceeded.
// It wraps the request body with http.MaxBytesReader and handles the error when reading fails.
func MaxBytesExceededHandler(maxBytes int64, metrics *RequestSizeLimitMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if maxBytes <= 0 {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the body to limit reading
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			// Create a wrapper to intercept any 413 errors that MaxBytesReader triggers
			wrapped := &maxBytesResponseWriter{
				ResponseWriter: w,
				r:              r,
				metrics:        metrics,
				logged:         false,
			}

			next.ServeHTTP(wrapped, r)
		})
	}
}

// maxBytesResponseWriter intercepts WriteHeader to log and record metrics for 413 responses.
type maxBytesResponseWriter struct {
	http.ResponseWriter
	r       *http.Request
	metrics *RequestSizeLimitMetrics
	logged  bool
}

func (w *maxBytesResponseWriter) WriteHeader(code int) {
	if code == http.StatusRequestEntityTooLarge && !w.logged {
		w.logged = true

		// Log the rejection
		slog.Warn("request rejected: body size exceeds limit",
			"method", w.r.Method,
			"path", w.r.URL.Path,
			"content_length", w.r.ContentLength,
			"remote_addr", w.r.RemoteAddr,
		)

		// Record metric
		if w.metrics != nil {
			w.metrics.RecordRejection(w.r.Context(), w.r.Method, w.r.URL.Path)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

// ValidateAllowedOrigins validates and normalizes allowed CORS origins
func ValidateAllowedOrigins(originsEnv string) ([]string, error) {
	if originsEnv == "" {
		return nil, nil
	}

	origins := strings.Split(originsEnv, ",")
	validated := make([]string, 0, len(origins))

	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}

		// Validate URL format
		u, err := url.Parse(origin)
		if err != nil {
			return nil, fmt.Errorf("invalid origin URL %q: %w", origin, err)
		}

		// Must have scheme and host
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("origin %q must include scheme and host (e.g., https://example.com)", origin)
		}

		// Only allow http/https
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("origin %q must use http or https scheme", origin)
		}

		// No path, query, or fragment allowed
		if u.Path != "" && u.Path != "/" {
			return nil, fmt.Errorf("origin %q should not include path", origin)
		}

		// Normalize by removing trailing slash and using scheme://host:port format
		normalized := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		validated = append(validated, normalized)
	}

	return validated, nil
}
