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
			Rate:      10,  // 10 req/sec per IP
			Burst:     20,  // Allow burst of 20
			UserRate:  100, // 100 req/sec per authenticated user
			UserBurst: 200, // Allow burst of 200
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
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Force HTTPS (only if using HTTPS)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Prevent XSS
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Restrict referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for OAuth endpoints
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origins from environment (comma-separated)
		allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
		var allowedOrigins []string
		if allowedOriginsEnv != "" {
			allowedOrigins = strings.Split(allowedOriginsEnv, ",")
		}

		origin := r.Header.Get("Origin")

		// If allowed origins is configured, check if origin is allowed
		if len(allowedOrigins) > 0 && origin != "" {
			for _, allowed := range allowedOrigins {
				if origin == strings.TrimSpace(allowed) {
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

// Start starts the OAuth-enabled HTTP server
func (s *OAuthHTTPServer) Start(addr string) error {
	// Validate HTTPS requirement for OAuth 2.1
	baseURL := s.oauthHandler.GetServer().Config.Issuer
	if err := validateHTTPSRequirement(baseURL); err != nil {
		return err
	}

	mux := http.NewServeMux()

	// Get the OAuth HTTP handler
	libHandler := s.oauthHandler.GetHandler()

	// ========== OAuth 2.1 Endpoints ==========

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

	// ========== MCP Endpoints ==========

	// Register MCP endpoints based on server type
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

	default:
		return fmt.Errorf("unsupported server type: %s", s.serverType)
	}

	// Create HTTP server with security and CORS middleware
	handler := securityHeadersMiddleware(corsMiddleware(mux))

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      120 * time.Second, // Increased for long-running MCP operations
		IdleTimeout:       120 * time.Second,
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
