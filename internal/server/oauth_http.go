package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	oauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/providers/dex"
	"github.com/giantswarm/mcp-oauth/providers/google"
	"github.com/giantswarm/mcp-oauth/security"
	oauthserver "github.com/giantswarm/mcp-oauth/server"
	"github.com/giantswarm/mcp-oauth/storage"
	"github.com/giantswarm/mcp-oauth/storage/memory"
	"github.com/giantswarm/mcp-oauth/storage/valkey"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/logging"
	mcpoauth "github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server/middleware"
)

const (
	// OAuth provider constants
	OAuthProviderDex    = "dex"
	OAuthProviderGoogle = "google"

	// DefaultOAuthScopes are the default Google OAuth scopes for Kubernetes management
	DefaultOAuthScopes = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"

	// DefaultRefreshTokenTTL is the default TTL for refresh tokens (90 days)
	DefaultRefreshTokenTTL = 90 * 24 * time.Hour

	// DefaultIPRateLimit is the default rate limit for requests per IP (requests/second)
	DefaultIPRateLimit = 10

	// DefaultIPBurst is the default burst size for IP rate limiting
	DefaultIPBurst = 20

	// DefaultUserRateLimit is the default rate limit for authenticated users (requests/second)
	DefaultUserRateLimit = 100

	// DefaultUserBurst is the default burst size for authenticated user rate limiting
	DefaultUserBurst = 200

	// DefaultMaxClientsPerIP is the default maximum number of clients per IP address
	DefaultMaxClientsPerIP = 10

	// DefaultReadHeaderTimeout is the default timeout for reading request headers
	DefaultReadHeaderTimeout = 10 * time.Second

	// DefaultWriteTimeout is the default timeout for writing responses (increased for long-running MCP operations)
	DefaultWriteTimeout = 120 * time.Second

	// DefaultIdleTimeout is the default idle timeout for keepalive connections
	DefaultIdleTimeout = 120 * time.Second

	// DefaultShutdownTimeout is the default timeout for graceful server shutdown
	DefaultShutdownTimeout = 30 * time.Second
)

var (
	// dexOAuthScopes are the OAuth scopes requested when using Dex OIDC provider
	dexOAuthScopes = []string{"openid", "profile", "email", "groups", "offline_access"}

	// googleOAuthScopes are the OAuth scopes requested when using Google OAuth provider
	googleOAuthScopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
	}
)

// createHTTPClientWithCA creates an HTTP client that trusts certificates signed by
// the CA in the specified file. The CA file should contain PEM-encoded certificate(s).
// This is used for Dex deployments with private/internal CAs.
func createHTTPClientWithCA(caFile string) (*http.Client, error) {
	// #nosec G304 -- caFile is a configuration value from operator, not user input
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file %s: %w", caFile, err)
	}

	// Create a certificate pool with system CAs and add the custom CA
	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		// If we can't load system certs, start with an empty pool
		caCertPool = x509.NewCertPool()
	}

	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", caFile)
	}

	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// OAuthStorageType represents the type of token storage backend.
type OAuthStorageType string

const (
	// OAuthStorageTypeMemory uses in-memory storage (default, not recommended for production)
	OAuthStorageTypeMemory OAuthStorageType = "memory"
	// OAuthStorageTypeValkey uses Valkey (Redis-compatible) for persistent storage
	OAuthStorageTypeValkey OAuthStorageType = "valkey"
)

// OAuthStorageConfig holds configuration for OAuth token storage backend.
type OAuthStorageConfig struct {
	// Type is the storage backend type: "memory" or "valkey" (default: "memory")
	Type OAuthStorageType

	// Valkey configuration (used when Type is "valkey")
	Valkey ValkeyStorageConfig
}

// ValkeyStorageConfig holds configuration for Valkey storage backend.
type ValkeyStorageConfig struct {
	// URL is the Valkey server address (e.g., "valkey.namespace.svc:6379")
	URL string

	// Password is the optional password for Valkey authentication
	Password string

	// TLSEnabled enables TLS for Valkey connections
	TLSEnabled bool

	// KeyPrefix is the prefix for all Valkey keys (default: "mcp:")
	KeyPrefix string

	// DB is the Valkey database number (default: 0)
	DB int
}

// OAuthConfig holds MCP-specific OAuth configuration
// Uses the mcp-oauth library's types directly to avoid duplication
type OAuthConfig struct {
	// BaseURL is the MCP server base URL (e.g., https://mcp.example.com)
	BaseURL string

	// Provider specifies the OAuth provider: "dex" or "google"
	Provider string

	// GoogleClientID is the Google OAuth Client ID
	GoogleClientID string

	// GoogleClientSecret is the Google OAuth Client Secret
	GoogleClientSecret string

	// DexIssuerURL is the Dex OIDC issuer URL
	DexIssuerURL string

	// DexClientID is the Dex OAuth Client ID
	DexClientID string

	// DexClientSecret is the Dex OAuth Client Secret
	DexClientSecret string

	// DexConnectorID is the optional Dex connector ID to bypass connector selection
	DexConnectorID string

	// DexCAFile is the path to a CA certificate file for Dex TLS verification
	// Use this when Dex uses a private/internal CA
	DexCAFile string

	// DisableStreaming disables streaming for streamable-http transport
	DisableStreaming bool

	// DebugMode enables debug logging
	DebugMode bool

	// EncryptionKey is the AES-256 key for encrypting tokens at rest (32 bytes)
	// If empty, tokens are stored unencrypted in memory
	EncryptionKey []byte

	// RegistrationAccessToken is the token required for client registration
	// Required if AllowPublicClientRegistration is false
	RegistrationAccessToken string

	// AllowPublicClientRegistration allows unauthenticated dynamic client registration
	// WARNING: This can lead to DoS attacks. Default: false
	AllowPublicClientRegistration bool

	// AllowInsecureAuthWithoutState allows authorization requests without state parameter
	// WARNING: Disabling this weakens CSRF protection. Default: false
	AllowInsecureAuthWithoutState bool

	// MaxClientsPerIP limits the number of clients that can be registered per IP
	MaxClientsPerIP int

	// EnableHSTS enables HSTS header (for reverse proxy scenarios)
	EnableHSTS bool

	// AllowedOrigins is a comma-separated list of allowed CORS origins
	AllowedOrigins string

	// Interstitial configures the OAuth success page for custom URL schemes
	// If nil, uses the default mcp-oauth interstitial page
	Interstitial *oauthserver.InterstitialConfig

	// InstrumentationProvider is the OpenTelemetry instrumentation provider for metrics/tracing
	InstrumentationProvider *instrumentation.Provider

	// Storage configures the token storage backend
	// Defaults to in-memory storage if not specified
	Storage OAuthStorageConfig
}

// OAuthHTTPServer wraps an MCP server with OAuth 2.1 authentication
type OAuthHTTPServer struct {
	mcpServer               *mcpserver.MCPServer
	oauthServer             *oauth.Server
	oauthHandler            *oauth.Handler
	tokenStore              storage.TokenStore
	httpServer              *http.Server
	serverType              string // "streamable-http"
	disableStreaming        bool
	instrumentationProvider *instrumentation.Provider
	healthChecker           *HealthChecker
}

// createOAuthServer creates an OAuth server using mcp-oauth library directly
func createOAuthServer(config OAuthConfig) (*oauth.Server, storage.TokenStore, error) {
	// Create logger with appropriate level
	var logger *slog.Logger
	if config.DebugMode {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		logger.Debug("Debug logging enabled for OAuth handler")
	} else {
		logger = slog.Default()
	}

	redirectURL := config.BaseURL + "/oauth/callback"
	var provider providers.Provider
	var err error

	switch config.Provider {
	case OAuthProviderDex:
		dexConfig := &dex.Config{
			IssuerURL:    config.DexIssuerURL,
			ClientID:     config.DexClientID,
			ClientSecret: config.DexClientSecret,
			RedirectURL:  redirectURL,
			Scopes:       dexOAuthScopes,
		}
		// Add optional connector ID if provided (bypasses connector selection)
		if config.DexConnectorID != "" {
			dexConfig.ConnectorID = config.DexConnectorID
		}
		// Configure custom HTTP client with CA if provided
		if config.DexCAFile != "" {
			httpClient, err := createHTTPClientWithCA(config.DexCAFile)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create HTTP client with CA: %w", err)
			}
			dexConfig.HTTPClient = httpClient
			logger.Info("Using custom CA for Dex TLS verification", "caFile", config.DexCAFile)
		}
		provider, err = dex.NewProvider(dexConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Dex provider: %w", err)
		}
		logger.Info("Using Dex OIDC provider", "issuer", config.DexIssuerURL)

	case OAuthProviderGoogle:
		provider, err = google.NewProvider(&google.Config{
			ClientID:     config.GoogleClientID,
			ClientSecret: config.GoogleClientSecret,
			RedirectURL:  redirectURL,
			Scopes:       googleOAuthScopes,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Google provider: %w", err)
		}
		logger.Info("Using Google OAuth provider")

	default:
		return nil, nil, fmt.Errorf("unsupported OAuth provider: %s (supported: %s, %s)", config.Provider, OAuthProviderDex, OAuthProviderGoogle)
	}

	// Create storage backend based on configuration
	// Both memory.Store and valkey.Store implement TokenStore, ClientStore, and FlowStore
	var tokenStore storage.TokenStore
	var clientStore storage.ClientStore
	var flowStore storage.FlowStore

	switch config.Storage.Type {
	case OAuthStorageTypeValkey:
		if config.Storage.Valkey.URL == "" {
			return nil, nil, fmt.Errorf("valkey URL is required when using valkey storage (--valkey-url or VALKEY_URL)")
		}

		// Configure Valkey storage
		valkeyConfig := valkey.Config{
			Address:   config.Storage.Valkey.URL,
			Password:  config.Storage.Valkey.Password,
			DB:        config.Storage.Valkey.DB,
			KeyPrefix: config.Storage.Valkey.KeyPrefix,
			Logger:    logger,
		}

		// Configure TLS if enabled
		if config.Storage.Valkey.TLSEnabled {
			valkeyConfig.TLS = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}

		// Set default key prefix if not specified
		if valkeyConfig.KeyPrefix == "" {
			valkeyConfig.KeyPrefix = valkey.DefaultKeyPrefix
		}

		valkeyStore, err := valkey.New(valkeyConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Valkey storage: %w", err)
		}

		// Set up encryption for Valkey store if key is provided
		if len(config.EncryptionKey) > 0 {
			encryptor, err := security.NewEncryptor(config.EncryptionKey)
			if err != nil {
				// Close the Valkey store on error to release resources
				valkeyStore.Close()
				return nil, nil, fmt.Errorf("failed to create encryptor for Valkey storage: %w", err)
			}
			valkeyStore.SetEncryptor(encryptor)
			logger.Info("Token encryption at rest enabled for Valkey storage (AES-256-GCM)")
		}

		// Valkey store implements all required interfaces
		tokenStore = valkeyStore
		clientStore = valkeyStore
		flowStore = valkeyStore
		logger.Info("Using Valkey storage backend", "address", config.Storage.Valkey.URL, "tls", config.Storage.Valkey.TLSEnabled)

	case OAuthStorageTypeMemory, "":
		// Use memory storage (default)
		memStore := memory.New()
		tokenStore = memStore
		clientStore = memStore
		flowStore = memStore
		if config.Storage.Type == "" {
			logger.Info("Using in-memory storage backend (default)")
		} else {
			logger.Info("Using in-memory storage backend")
		}

	default:
		return nil, nil, fmt.Errorf("unsupported OAuth storage type: %s (supported: memory, valkey)", config.Storage.Type)
	}

	// Set defaults
	maxClientsPerIP := config.MaxClientsPerIP
	if maxClientsPerIP == 0 {
		maxClientsPerIP = DefaultMaxClientsPerIP
	}

	// Create server configuration using library types directly
	serverConfig := &oauthserver.Config{
		Issuer:                        config.BaseURL,
		RefreshTokenTTL:               int64(DefaultRefreshTokenTTL.Seconds()),
		AllowRefreshTokenRotation:     true,  // OAuth 2.1 best practice
		RequirePKCE:                   true,  // OAuth 2.1 requirement
		AllowPKCEPlain:                false, // Only S256, not plain
		AllowPublicClientRegistration: config.AllowPublicClientRegistration,
		RegistrationAccessToken:       config.RegistrationAccessToken,
		AllowNoStateParameter:         config.AllowInsecureAuthWithoutState,
		MaxClientsPerIP:               maxClientsPerIP,
	}

	// Configure interstitial page branding if provided
	if config.Interstitial != nil {
		serverConfig.Interstitial = config.Interstitial
	}

	// Create OAuth server
	server, err := oauth.NewServer(
		provider,
		tokenStore,  // TokenStore
		clientStore, // ClientStore
		flowStore,   // FlowStore
		serverConfig,
		logger,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OAuth server: %w", err)
	}

	// Set up encryption if key provided (only for memory storage; Valkey encryption is set above)
	if len(config.EncryptionKey) > 0 && config.Storage.Type != OAuthStorageTypeValkey {
		encryptor, err := security.NewEncryptor(config.EncryptionKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create encryptor: %w", err)
		}
		server.SetEncryptor(encryptor)
		logger.Info("Token encryption at rest enabled (AES-256-GCM)")
	}

	// Set up audit logging (always enabled for security)
	auditor := security.NewAuditor(logger, true)
	server.SetAuditor(auditor)
	logger.Info("Security audit logging enabled")

	// Set up IP-based rate limiting
	ipRateLimiter := security.NewRateLimiter(DefaultIPRateLimit, DefaultIPBurst, logger)
	server.SetRateLimiter(ipRateLimiter)
	logger.Info("IP-based rate limiting enabled", "rate", DefaultIPRateLimit, "burst", DefaultIPBurst)

	// Set up user-based rate limiting
	userRateLimiter := security.NewRateLimiter(DefaultUserRateLimit, DefaultUserBurst, logger)
	server.SetUserRateLimiter(userRateLimiter)
	logger.Info("User-based rate limiting enabled", "rate", DefaultUserRateLimit, "burst", DefaultUserBurst)

	// Set up client registration rate limiting
	clientRegRL := security.NewClientRegistrationRateLimiter(logger)
	server.SetClientRegistrationRateLimiter(clientRegRL)

	return server, tokenStore, nil
}

// NewOAuthHTTPServer creates a new OAuth-enabled HTTP server
func NewOAuthHTTPServer(mcpServer *mcpserver.MCPServer, serverType string, config OAuthConfig) (*OAuthHTTPServer, error) {
	oauthServer, tokenStore, err := createOAuthServer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth server: %w", err)
	}

	// Create HTTP handler
	oauthHandler := oauth.NewHandler(oauthServer, oauthServer.Logger)

	return &OAuthHTTPServer{
		mcpServer:               mcpServer,
		oauthServer:             oauthServer,
		oauthHandler:            oauthHandler,
		tokenStore:              tokenStore,
		serverType:              serverType,
		disableStreaming:        config.DisableStreaming,
		instrumentationProvider: config.InstrumentationProvider,
	}, nil
}

// CreateOAuthServer creates an OAuth server for use with HTTP transport
// This allows creating the server before the HTTP server to inject the token store
func CreateOAuthServer(config OAuthConfig) (*oauth.Server, storage.TokenStore, error) {
	return createOAuthServer(config)
}

// NewOAuthHTTPServerWithServer creates a new OAuth-enabled HTTP server with an existing OAuth server
func NewOAuthHTTPServerWithServer(mcpServer *mcpserver.MCPServer, serverType string, oauthServer *oauth.Server, tokenStore storage.TokenStore, disableStreaming bool) (*OAuthHTTPServer, error) {
	// Create HTTP handler
	oauthHandler := oauth.NewHandler(oauthServer, oauthServer.Logger)

	return &OAuthHTTPServer{
		mcpServer:        mcpServer,
		oauthServer:      oauthServer,
		oauthHandler:     oauthHandler,
		tokenStore:       tokenStore,
		serverType:       serverType,
		disableStreaming: disableStreaming,
	}, nil
}

// setupOAuthRoutes registers OAuth 2.1 endpoints on the mux
func (s *OAuthHTTPServer) setupOAuthRoutes(mux *http.ServeMux) {
	// Protected Resource Metadata endpoint (RFC 9728)
	mux.HandleFunc("/.well-known/oauth-protected-resource", s.oauthHandler.ServeProtectedResourceMetadata)

	// Authorization Server Metadata endpoint (RFC 8414)
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.oauthHandler.ServeAuthorizationServerMetadata)

	// Dynamic Client Registration endpoint (RFC 7591)
	mux.HandleFunc("/oauth/register", s.oauthHandler.ServeClientRegistration)

	// OAuth Authorization endpoint
	mux.HandleFunc("/oauth/authorize", s.oauthHandler.ServeAuthorization)

	// OAuth Token endpoint
	mux.HandleFunc("/oauth/token", s.oauthHandler.ServeToken)

	// OAuth Callback endpoint (from provider)
	mux.HandleFunc("/oauth/callback", s.oauthHandler.ServeCallback)

	// Token Revocation endpoint (RFC 7009)
	mux.HandleFunc("/oauth/revoke", s.oauthHandler.ServeTokenRevocation)

	// Token Introspection endpoint (RFC 7662)
	mux.HandleFunc("/oauth/introspect", s.oauthHandler.ServeTokenIntrospection)
}

// setupMCPRoutes registers MCP endpoints on the mux
func (s *OAuthHTTPServer) setupMCPRoutes(mux *http.ServeMux) error {

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
		mux.Handle("/mcp", s.oauthHandler.ValidateToken(accessTokenInjector))

		return nil
	default:
		return fmt.Errorf("unsupported server type: %s", s.serverType)
	}
}

// validateStartConfig validates the configuration before starting the server
func (s *OAuthHTTPServer) validateStartConfig(config OAuthConfig) ([]string, error) {
	// Validate HTTPS requirement for OAuth 2.1
	baseURL := s.oauthServer.Config.Issuer
	if err := validateHTTPSRequirement(baseURL); err != nil {
		return nil, err
	}

	// Validate and parse allowed CORS origins
	allowedOrigins, err := middleware.ValidateAllowedOrigins(config.AllowedOrigins)
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

	// Setup Prometheus metrics endpoint if instrumentation is enabled
	if s.instrumentationProvider != nil && s.instrumentationProvider.Enabled() {
		if prometheusHandler := s.instrumentationProvider.PrometheusHandler(); prometheusHandler != nil {
			// The prometheus exporter is registered with the global prometheus registry
			// Use promhttp.Handler() to expose metrics
			mux.Handle("/metrics", promhttp.Handler())
		}
	}

	// Setup health check endpoints
	if s.healthChecker != nil {
		s.healthChecker.RegisterHealthEndpoints(mux)
	}

	// Create HTTP server with security and CORS middleware
	handler := middleware.SecurityHeaders(config.EnableHSTS)(middleware.CORS(allowedOrigins)(mux))

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
	// Shutdown OAuth server (handles rate limiters, storage cleanup, etc.)
	if s.oauthServer != nil {
		if err := s.oauthServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown OAuth server: %w", err)
		}
	}

	// Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// GetOAuthServer returns the OAuth server for testing or direct access
func (s *OAuthHTTPServer) GetOAuthServer() *oauth.Server {
	return s.oauthServer
}

// GetOAuthHandler returns the OAuth handler for testing or direct access
func (s *OAuthHTTPServer) GetOAuthHandler() *oauth.Handler {
	return s.oauthHandler
}

// GetTokenStore returns the token store for downstream OAuth passthrough
func (s *OAuthHTTPServer) GetTokenStore() storage.TokenStore {
	return s.tokenStore
}

// SetHealthChecker sets the health checker for health check endpoints.
func (s *OAuthHTTPServer) SetHealthChecker(hc *HealthChecker) {
	s.healthChecker = hc
}

// createAccessTokenInjectorMiddleware creates middleware that injects the user's
// Google OAuth access token into the request context. This token can then be used
// for downstream Kubernetes API authentication.
func (s *OAuthHTTPServer) createAccessTokenInjectorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get user info from context (set by ValidateToken middleware)
		userInfo, ok := oauth.UserInfoFromContext(ctx)
		if !ok || userInfo == nil {
			slog.Debug("AccessTokenInjector: no user info in context")
			next.ServeHTTP(w, r)
			return
		}
		if userInfo.Email == "" {
			slog.Debug("AccessTokenInjector: user info has no email", "user_id", userInfo.ID)
			next.ServeHTTP(w, r)
			return
		}

		slog.Debug("AccessTokenInjector: looking up token for user", logging.UserHash(userInfo.Email))

		// Retrieve the user's stored OAuth token
		token, err := s.tokenStore.GetToken(ctx, userInfo.Email)
		if err != nil {
			slog.Debug("AccessTokenInjector: failed to get token from store", logging.UserHash(userInfo.Email), logging.Err(err))
			next.ServeHTTP(w, r)
			return
		}
		if token == nil {
			slog.Debug("AccessTokenInjector: token is nil for user", logging.UserHash(userInfo.Email))
			next.ServeHTTP(w, r)
			return
		}

		// Extract the ID token for Kubernetes OIDC authentication
		// Kubernetes OIDC validates the ID token, not the access token
		idToken := mcpoauth.GetIDToken(token)
		if idToken == "" {
			slog.Debug("AccessTokenInjector: no ID token in stored token", logging.UserHash(userInfo.Email), slog.Bool("has_access_token", token.AccessToken != ""), slog.Bool("has_refresh_token", token.RefreshToken != ""))
			next.ServeHTTP(w, r)
			return
		}

		slog.Debug("AccessTokenInjector: successfully injected ID token", logging.UserHash(userInfo.Email))
		ctx = mcpoauth.ContextWithAccessToken(ctx, idToken)
		r = r.WithContext(ctx)

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
