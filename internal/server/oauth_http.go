package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
)

const (
	// DefaultIPRateLimit is the default rate limit for requests per IP (requests/second)
	DefaultIPRateLimit = 10

	// DefaultIPBurst is the default burst size for IP rate limiting
	DefaultIPBurst = 20

	// DefaultUserRateLimit is the default rate limit for authenticated users (requests/second)
	DefaultUserRateLimit = 100

	// DefaultUserBurst is the default burst size for authenticated user rate limiting
	DefaultUserBurst = 200

	// DefaultReadHeaderTimeout is the default timeout for reading request headers
	DefaultReadHeaderTimeout = 10 * time.Second

	// DefaultWriteTimeout is the default timeout for writing responses (increased for long-running MCP operations)
	DefaultWriteTimeout = 120 * time.Second

	// DefaultIdleTimeout is the default idle timeout for keepalive connections
	DefaultIdleTimeout = 120 * time.Second

	// DefaultShutdownTimeout is the default timeout for graceful server shutdown
	DefaultShutdownTimeout = 30 * time.Second
)

// OAuthConfig holds configuration for OAuth server creation
type OAuthConfig struct {
	BaseURL            string
	GoogleClientID     string
	GoogleClientSecret string
	DisableStreaming   bool
	DebugMode          bool // Enable debug logging

	// Security Settings (secure by default)
	// See oauth.Config for detailed documentation
	AllowPublicClientRegistration bool   // Default: false (requires registration token)
	RegistrationAccessToken       string // Required if AllowPublicClientRegistration=false
	AllowInsecureAuthWithoutState bool   // Default: false (state parameter required)
	MaxClientsPerIP               int    // Default: 10 (prevents DoS)
	EncryptionKey                 []byte // AES-256 key for token encryption (32 bytes)

	// HTTP Security Settings
	EnableHSTS     bool   // Enable HSTS header (for reverse proxy scenarios)
	AllowedOrigins string // Comma-separated list of allowed CORS origins

	// Interstitial page branding configuration
	// If nil, uses the default mcp-oauth interstitial page
	Interstitial *oauth.InterstitialConfig
}

// OAuthHTTPServer wraps an MCP server with OAuth 2.1 authentication
type OAuthHTTPServer struct {
	mcpServer        *mcpserver.MCPServer
	oauthHandler     *oauth.Handler
	tokenProvider    *oauth.TokenProvider
	httpServer       *http.Server
	serverType       string // "streamable-http"
	disableStreaming bool
}

// buildOAuthConfig converts OAuthConfig to oauth.Config
// This eliminates code duplication between NewOAuthHTTPServer and CreateOAuthHandler
func buildOAuthConfig(config OAuthConfig) *oauth.Config {
	// Create logger with appropriate level
	var logger *slog.Logger
	if config.DebugMode {
		// Debug level logging
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		logger.Debug("Debug logging enabled for OAuth handler")
	} else {
		// Info level logging (default)
		logger = slog.Default()
	}

	oauthConfig := &oauth.Config{
		BaseURL:            config.BaseURL,
		GoogleClientID:     config.GoogleClientID,
		GoogleClientSecret: config.GoogleClientSecret,
		Logger:             logger,
		Security: oauth.SecurityConfig{
			AllowPublicClientRegistration: config.AllowPublicClientRegistration,
			RegistrationAccessToken:       config.RegistrationAccessToken,
			AllowInsecureAuthWithoutState: config.AllowInsecureAuthWithoutState,
			MaxClientsPerIP:               config.MaxClientsPerIP,
			EncryptionKey:                 config.EncryptionKey,
			EnableAuditLogging:            true, // Always enable audit logging
		},
		RateLimit: oauth.RateLimitConfig{
			Rate:      DefaultIPRateLimit,
			Burst:     DefaultIPBurst,
			UserRate:  DefaultUserRateLimit,
			UserBurst: DefaultUserBurst,
		},
	}

	// Pass through interstitial config if provided
	if config.Interstitial != nil {
		oauthConfig.Interstitial = config.Interstitial
	}

	return oauthConfig
}

// NewOAuthHTTPServer creates a new OAuth-enabled HTTP server
func NewOAuthHTTPServer(mcpServer *mcpserver.MCPServer, serverType string, config OAuthConfig) (*OAuthHTTPServer, error) {
	oauthHandler, err := oauth.NewHandler(buildOAuthConfig(config))
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth handler: %w", err)
	}

	// Create token provider for downstream OAuth passthrough
	tokenProvider := oauth.NewTokenProvider(oauthHandler.GetStore())

	return &OAuthHTTPServer{
		mcpServer:        mcpServer,
		oauthHandler:     oauthHandler,
		tokenProvider:    tokenProvider,
		serverType:       serverType,
		disableStreaming: config.DisableStreaming,
	}, nil
}

// CreateOAuthHandler creates an OAuth handler for use with HTTP transport
// This allows creating the handler before the server to inject the token provider
func CreateOAuthHandler(config OAuthConfig) (*oauth.Handler, error) {
	return oauth.NewHandler(buildOAuthConfig(config))
}

// NewOAuthHTTPServerWithHandler creates a new OAuth-enabled HTTP server with an existing handler
func NewOAuthHTTPServerWithHandler(mcpServer *mcpserver.MCPServer, serverType string, oauthHandler *oauth.Handler, disableStreaming bool) (*OAuthHTTPServer, error) {
	// Create token provider for downstream OAuth passthrough
	tokenProvider := oauth.NewTokenProvider(oauthHandler.GetStore())

	return &OAuthHTTPServer{
		mcpServer:        mcpServer,
		oauthHandler:     oauthHandler,
		tokenProvider:    tokenProvider,
		serverType:       serverType,
		disableStreaming: disableStreaming,
	}, nil
}

// securityHeadersMiddleware adds security headers to all HTTP responses
func securityHeadersMiddleware(hstsEnabled bool) func(http.Handler) http.Handler {
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

// corsMiddleware adds CORS headers for OAuth endpoints with validated origins
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
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

// validateAllowedOrigins validates and normalizes allowed CORS origins
func validateAllowedOrigins(originsEnv string) ([]string, error) {
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

// setupOAuthRoutes registers OAuth 2.1 endpoints on the mux
func (s *OAuthHTTPServer) setupOAuthRoutes(mux *http.ServeMux) {
	libHandler := s.oauthHandler.GetHandler()

	// Protected Resource Metadata endpoint (RFC 9728)
	mux.HandleFunc("/.well-known/oauth-protected-resource", libHandler.ServeProtectedResourceMetadata)

	// Authorization Server Metadata endpoint (RFC 8414)
	mux.HandleFunc("/.well-known/oauth-authorization-server", libHandler.ServeAuthorizationServerMetadata)

	// Dynamic Client Registration endpoint (RFC 7591)
	mux.HandleFunc("/oauth/register", libHandler.ServeClientRegistration)

	// OAuth Authorization endpoint
	mux.HandleFunc("/oauth/authorize", libHandler.ServeAuthorization)

	// OAuth Token endpoint
	mux.HandleFunc("/oauth/token", libHandler.ServeToken)

	// OAuth Callback endpoint (from provider)
	mux.HandleFunc("/oauth/callback", libHandler.ServeCallback)

	// Token Revocation endpoint (RFC 7009)
	mux.HandleFunc("/oauth/revoke", libHandler.ServeTokenRevocation)

	// Token Introspection endpoint (RFC 7662)
	mux.HandleFunc("/oauth/introspect", libHandler.ServeTokenIntrospection)
}

// setupMCPRoutes registers MCP endpoints on the mux
func (s *OAuthHTTPServer) setupMCPRoutes(mux *http.ServeMux) error {
	libHandler := s.oauthHandler.GetHandler()

	switch s.serverType {
	case "streamable-http":
		// Create Streamable HTTP server
		var httpServer http.Handler
		if s.disableStreaming {
			httpServer = mcpserver.NewStreamableHTTPServer(s.mcpServer,
				mcpserver.WithEndpointPath("/mcp"),
				mcpserver.WithDisableStreaming(true),
			)
		} else {
			httpServer = mcpserver.NewStreamableHTTPServer(s.mcpServer,
				mcpserver.WithEndpointPath("/mcp"),
			)
		}

		// Create middleware to inject access token into context for downstream K8s auth
		accessTokenInjector := s.createAccessTokenInjectorMiddleware(httpServer)

		// Wrap MCP endpoint with OAuth middleware (ValidateToken validates and adds user info)
		// Then our injector adds the access token for downstream use
		mux.Handle("/mcp", libHandler.ValidateToken(accessTokenInjector))

		return nil
	default:
		return fmt.Errorf("unsupported server type: %s", s.serverType)
	}
}

// validateStartConfig validates the configuration before starting the server
func (s *OAuthHTTPServer) validateStartConfig(config OAuthConfig) ([]string, error) {
	// Validate HTTPS requirement for OAuth 2.1
	baseURL := s.oauthHandler.GetServer().Config.Issuer
	if err := validateHTTPSRequirement(baseURL); err != nil {
		return nil, err
	}

	// Validate and parse allowed CORS origins
	allowedOrigins, err := validateAllowedOrigins(config.AllowedOrigins)
	if err != nil {
		return nil, fmt.Errorf("invalid ALLOWED_ORIGINS: %w", err)
	}

	return allowedOrigins, nil
}

// Start starts the OAuth-enabled HTTP server
func (s *OAuthHTTPServer) Start(addr string, config OAuthConfig) error {
	// Validate configuration
	allowedOrigins, err := s.validateStartConfig(config)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()

	// Setup OAuth 2.1 endpoints
	s.setupOAuthRoutes(mux)

	// Setup MCP endpoints
	if err := s.setupMCPRoutes(mux); err != nil {
		return err
	}

	// Create HTTP server with security and CORS middleware
	handler := securityHeadersMiddleware(config.EnableHSTS)(corsMiddleware(allowedOrigins)(mux))

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
	}

	// Start server
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *OAuthHTTPServer) Shutdown(ctx context.Context) error {
	// Stop the OAuth handler's background services
	if s.oauthHandler != nil {
		s.oauthHandler.Stop()
	}

	// Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// GetOAuthHandler returns the OAuth handler for testing or direct access
func (s *OAuthHTTPServer) GetOAuthHandler() *oauth.Handler {
	return s.oauthHandler
}

// GetTokenProvider returns the token provider for downstream OAuth passthrough
func (s *OAuthHTTPServer) GetTokenProvider() *oauth.TokenProvider {
	return s.tokenProvider
}

// createAccessTokenInjectorMiddleware creates middleware that injects the user's
// Google OAuth access token into the request context. This token can then be used
// for downstream Kubernetes API authentication.
func (s *OAuthHTTPServer) createAccessTokenInjectorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get user info from context (set by ValidateToken middleware)
		userInfo, ok := oauth.GetUserFromContext(ctx)
		if ok && userInfo != nil && userInfo.Email != "" {
			// Retrieve the user's stored Google OAuth token
			token, err := s.tokenProvider.GetToken(ctx, userInfo.Email)
			if err == nil && token != nil {
				// Extract the ID token for Kubernetes OIDC authentication
				// Kubernetes OIDC validates the ID token, not the access token
				idToken := oauth.GetIDToken(token)
				if idToken != "" {
					ctx = oauth.ContextWithAccessToken(ctx, idToken)
					r = r.WithContext(ctx)
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// validateHTTPSRequirement ensures OAuth 2.1 HTTPS compliance
// Allows HTTP only for loopback addresses (localhost, 127.0.0.1, ::1)
func validateHTTPSRequirement(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("base URL cannot be empty")
	}

	// Parse URL to properly validate scheme and host
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}

	// Allow HTTP only for loopback addresses
	if u.Scheme == "http" {
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return fmt.Errorf("OAuth 2.1 requires HTTPS for production (got: %s). Use HTTPS or localhost for development", baseURL)
		}
	} else if u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s. Must be http (localhost only) or https", u.Scheme)
	}

	return nil
}
