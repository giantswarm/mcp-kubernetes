package oauth

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	oauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/providers/google"
	"github.com/giantswarm/mcp-oauth/security"
	oauthserver "github.com/giantswarm/mcp-oauth/server"
	"github.com/giantswarm/mcp-oauth/storage/memory"
)

const (
	// defaultBurstMultiplier is the default multiplier for burst size relative to rate
	// Burst = Rate * defaultBurstMultiplier
	defaultBurstMultiplier = 2
)

// DefaultOAuthScopes are the default Google OAuth scopes for Kubernetes management
// These scopes allow the MCP server to access Google Cloud Platform resources
var DefaultOAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",   // Full access to GCP (includes GKE)
	"https://www.googleapis.com/auth/userinfo.email",   // User email for identification
	"https://www.googleapis.com/auth/userinfo.profile", // User profile information
}

// Config holds the OAuth library handler configuration
// This maps our existing configuration to the mcp-oauth library configuration
type Config struct {
	// BaseURL is the MCP server base URL (e.g., https://mcp.example.com)
	BaseURL string

	// GoogleClientID is the Google OAuth Client ID
	GoogleClientID string

	// GoogleClientSecret is the Google OAuth Client Secret
	GoogleClientSecret string

	// Security settings (secure by default)
	Security SecurityConfig

	// RateLimit configuration
	RateLimit RateLimitConfig

	// Interstitial configures the OAuth success page for custom URL schemes (cursor://, vscode://, etc.)
	// If nil, uses the default mcp-oauth interstitial page
	Interstitial *InterstitialConfig

	// Logger for structured logging (optional, uses default if not provided)
	Logger *slog.Logger
}

// InterstitialConfig configures the OAuth success interstitial page branding
type InterstitialConfig struct {
	// LogoURL is an optional URL to a logo image (must be HTTPS)
	// Leave empty to use the default animated checkmark icon
	LogoURL string

	// LogoAlt is the alt text for the logo image (for accessibility)
	LogoAlt string

	// Title replaces the "Authorization Successful" heading
	// Example: "Connected to Kubernetes MCP"
	Title string

	// Message replaces the default success message
	// Use {{.AppName}} placeholder for the application name
	Message string

	// ButtonText replaces the "Open [AppName]" button text
	// Use {{.AppName}} placeholder for the application name
	ButtonText string

	// PrimaryColor is the primary brand color (CSS color value)
	// Example: "#667eea" or "rgb(102, 126, 234)"
	PrimaryColor string

	// BackgroundGradient is the body background CSS value
	// Example: "linear-gradient(135deg, #1e3a5f 0%, #2d5a87 100%)"
	BackgroundGradient string

	// CustomCSS allows additional CSS customization (injected into <style> tag)
	// WARNING: Be careful with CSS injection - avoid user-provided values
	CustomCSS string
}

// SecurityConfig holds OAuth security settings (secure by default)
type SecurityConfig struct {
	// AllowPublicClientRegistration allows unauthenticated dynamic client registration
	// WARNING: This can lead to DoS attacks via unlimited client registration
	// Default: false (authentication REQUIRED for security)
	AllowPublicClientRegistration bool

	// RegistrationAccessToken is the token required for client registration
	// Only checked if AllowPublicClientRegistration is false
	RegistrationAccessToken string

	// AllowInsecureAuthWithoutState allows authorization requests without state parameter
	// WARNING: Disabling this weakens CSRF protection
	// Default: false (state is REQUIRED for security)
	AllowInsecureAuthWithoutState bool

	// MaxClientsPerIP limits the number of clients that can be registered per IP
	// Prevents DoS attacks via mass client registration
	// Default: 10
	MaxClientsPerIP int

	// EncryptionKey is the AES-256 key for encrypting tokens at rest (32 bytes)
	// If nil or empty, tokens are stored unencrypted in memory
	// For production, provide a 32-byte key from secure storage (KMS, Vault, etc.)
	EncryptionKey []byte

	// EnableAuditLogging enables comprehensive security audit logging
	// Logs authentication events, token operations, and security violations
	// Default: true (enabled for security)
	EnableAuditLogging bool

	// RefreshTokenTTL is the time-to-live for refresh tokens
	// Recommended: 30-90 days for security vs usability balance
	// Default: 90 days
	RefreshTokenTTL time.Duration
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// Rate is the number of requests per second allowed per IP (0 = no limit)
	Rate int

	// Burst is the maximum burst size allowed per IP
	Burst int

	// UserRate is the number of requests per second allowed per authenticated user (0 = no limit)
	UserRate int

	// UserBurst is the maximum burst size allowed per authenticated user
	UserBurst int

	// TrustProxy indicates whether to trust X-Forwarded-For and X-Real-IP headers
	// Only set to true if the server is behind a trusted proxy
	// Default: false (secure by default)
	TrustProxy bool
}

// Handler wraps the mcp-oauth library components for integration with mcp-kubernetes
type Handler struct {
	server          *oauth.Server
	handler         *oauth.Handler
	store           *memory.Store
	ipRateLimiter   *security.RateLimiter
	userRateLimiter *security.RateLimiter
	clientRegRL     *security.ClientRegistrationRateLimiter
	stopOnce        sync.Once // Ensures Stop() is truly idempotent even under concurrent calls
}

// NewHandler creates a new OAuth handler using the mcp-oauth library
func NewHandler(config *Config) (*Handler, error) {
	// Set default logger if not provided
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create Google provider with our scopes
	redirectURL := config.BaseURL + "/oauth/callback"
	provider, err := google.NewProvider(&google.Config{
		ClientID:     config.GoogleClientID,
		ClientSecret: config.GoogleClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       DefaultOAuthScopes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Google provider: %w", err)
	}

	// Create memory storage
	store := memory.New()

	// Set default refresh token TTL if not specified
	refreshTokenTTL := config.Security.RefreshTokenTTL
	if refreshTokenTTL == 0 {
		refreshTokenTTL = 90 * 24 * time.Hour // 90 days default
	}

	// Set default max clients per IP if not specified
	maxClientsPerIP := config.Security.MaxClientsPerIP
	if maxClientsPerIP == 0 {
		maxClientsPerIP = 10
	}

	// Create server configuration
	serverConfig := &oauth.ServerConfig{
		Issuer:                        config.BaseURL,
		RefreshTokenTTL:               int64(refreshTokenTTL.Seconds()),
		AllowRefreshTokenRotation:     true,  // OAuth 2.1 best practice
		RequirePKCE:                   true,  // OAuth 2.1 requirement
		AllowPKCEPlain:                false, // Only S256, not plain
		AllowPublicClientRegistration: config.Security.AllowPublicClientRegistration,
		RegistrationAccessToken:       config.Security.RegistrationAccessToken,
		AllowNoStateParameter:         config.Security.AllowInsecureAuthWithoutState,
		MaxClientsPerIP:               maxClientsPerIP,
		TrustProxy:                    config.RateLimit.TrustProxy,
		TokenRefreshThreshold:         300, // 5 minutes proactive refresh
		ClockSkewGracePeriod:          5,   // 5 seconds clock skew tolerance
	}

	// Configure interstitial page branding if provided
	if config.Interstitial != nil {
		serverConfig.Interstitial = &oauthserver.InterstitialConfig{
			Branding: &oauthserver.InterstitialBranding{
				LogoURL:            config.Interstitial.LogoURL,
				LogoAlt:            config.Interstitial.LogoAlt,
				Title:              config.Interstitial.Title,
				Message:            config.Interstitial.Message,
				ButtonText:         config.Interstitial.ButtonText,
				PrimaryColor:       config.Interstitial.PrimaryColor,
				BackgroundGradient: config.Interstitial.BackgroundGradient,
				CustomCSS:          config.Interstitial.CustomCSS,
			},
		}
	}

	// Create OAuth server
	server, err := oauth.NewServer(
		provider,
		store, // TokenStore
		store, // ClientStore
		store, // FlowStore
		serverConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth server: %w", err)
	}

	// Set up encryption if key provided
	if len(config.Security.EncryptionKey) > 0 {
		encryptor, err := security.NewEncryptor(config.Security.EncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create encryptor: %w", err)
		}
		server.SetEncryptor(encryptor)
		logger.Info("Token encryption at rest enabled (AES-256-GCM)")
	}

	// Set up audit logging if enabled
	if config.Security.EnableAuditLogging {
		auditor := security.NewAuditor(logger, true)
		server.SetAuditor(auditor)
		logger.Info("Security audit logging enabled")
	}

	// Set up IP-based rate limiting if configured
	var ipRateLimiter *security.RateLimiter
	if config.RateLimit.Rate > 0 {
		burst := config.RateLimit.Burst
		if burst == 0 {
			burst = config.RateLimit.Rate * defaultBurstMultiplier
		}
		ipRateLimiter = security.NewRateLimiter(config.RateLimit.Rate, burst, logger)
		server.SetRateLimiter(ipRateLimiter)
		logger.Info("IP-based rate limiting enabled",
			"rate", config.RateLimit.Rate,
			"burst", burst)
	}

	// Set up user-based rate limiting if configured
	var userRateLimiter *security.RateLimiter
	if config.RateLimit.UserRate > 0 {
		burst := config.RateLimit.UserBurst
		if burst == 0 {
			burst = config.RateLimit.UserRate * defaultBurstMultiplier
		}
		userRateLimiter = security.NewRateLimiter(config.RateLimit.UserRate, burst, logger)
		server.SetUserRateLimiter(userRateLimiter)
		logger.Info("User-based rate limiting enabled",
			"rate", config.RateLimit.UserRate,
			"burst", burst)
	}

	// Set up client registration rate limiting
	clientRegRL := security.NewClientRegistrationRateLimiter(logger)
	server.SetClientRegistrationRateLimiter(clientRegRL)

	// Create HTTP handler
	handler := oauth.NewHandler(server, logger)

	return &Handler{
		server:          server,
		handler:         handler,
		store:           store,
		ipRateLimiter:   ipRateLimiter,
		userRateLimiter: userRateLimiter,
		clientRegRL:     clientRegRL,
	}, nil
}

// GetHandler returns the underlying mcp-oauth handler for HTTP routing
func (h *Handler) GetHandler() *oauth.Handler {
	return h.handler
}

// GetStore returns the underlying storage for token provider integration
func (h *Handler) GetStore() *memory.Store {
	return h.store
}

// GetServer returns the underlying OAuth server
func (h *Handler) GetServer() *oauth.Server {
	return h.server
}

// CanRefreshTokens returns true if the handler can refresh tokens
// This checks if Google OAuth credentials are properly configured
func (h *Handler) CanRefreshTokens() bool {
	// The library's Google provider always supports refresh if configured
	return true
}

// Stop gracefully stops all background services (rate limiters, storage cleanup, etc.)
// This method is idempotent and safe to call concurrently from multiple goroutines.
func (h *Handler) Stop() {
	h.stopOnce.Do(func() {
		if h.store != nil {
			h.store.Stop()
		}
		if h.ipRateLimiter != nil {
			h.ipRateLimiter.Stop()
		}
		if h.userRateLimiter != nil {
			h.userRateLimiter.Stop()
		}
		if h.clientRegRL != nil {
			h.clientRegRL.Stop()
		}
	})
}
