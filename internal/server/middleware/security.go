package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SecurityHeadersConfig holds configuration for security headers middleware
type SecurityHeadersConfig struct {
	// EnableHSTS enables HSTS header (for reverse proxy scenarios)
	EnableHSTS bool

	// EnableCrossOriginIsolation enables strict COOP/COEP headers
	// When true: COOP=same-origin, COEP=require-corp, CORP=same-origin
	// When false (default): no cross-origin headers set for OAuth popup compatibility
	EnableCrossOriginIsolation bool
}

// SecurityHeaders adds comprehensive security headers to all HTTP responses
func SecurityHeaders(config SecurityHeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent MIME sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// Force HTTPS (configurable for reverse proxy scenarios)
			if r.TLS != nil || config.EnableHSTS {
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

			// Cross-Origin policies
			// Only set COOP/COEP/CORP when explicitly enabled for features like
			// SharedArrayBuffer. When disabled (default), no cross-origin headers
			// are set to ensure maximum compatibility with OAuth popup flows.
			if config.EnableCrossOriginIsolation {
				w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
				w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
				w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			}
			// When disabled: no COOP/COEP headers - browser defaults apply

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
