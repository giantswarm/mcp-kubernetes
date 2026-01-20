package cmd

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// OAuth provider constants - use server package constants for consistency
const (
	OAuthProviderDex    = server.OAuthProviderDex
	OAuthProviderGoogle = server.OAuthProviderGoogle
)

// URI scheme constants for URL validation
const (
	schemeHTTPS = "https"
	schemeHTTP  = "http"
)

// Request size limiting constants
const (
	// DefaultMaxRequestSize is the default maximum request body size in bytes (5MB).
	// This provides protection against denial-of-service attacks via oversized requests.
	// The minimum recommended size (1MB) is defined in internal/server/middleware.MinRecommendedRequestSize.
	DefaultMaxRequestSize int64 = 5 * 1024 * 1024
)

// ServeConfig holds all configuration for the serve command.
type ServeConfig struct {
	// Transport settings
	Transport string
	HTTPAddr  string

	// Endpoint paths
	SSEEndpoint     string
	MessageEndpoint string
	HTTPEndpoint    string

	// Kubernetes client settings
	NonDestructiveMode bool
	DryRun             bool
	QPSLimit           float32
	BurstLimit         int
	DebugMode          bool
	InCluster          bool

	// OAuth configuration
	OAuth           OAuthServeConfig
	DownstreamOAuth bool

	// CAPI Mode configuration (multi-cluster federation)
	CAPIMode CAPIModeConfig

	// Metrics server configuration
	Metrics MetricsServeConfig

	// MaxRequestSize is the maximum allowed request body size in bytes.
	// Requests exceeding this limit will receive a 413 Request Entity Too Large response.
	// Default: 5MB (5242880 bytes)
	MaxRequestSize int64
}

// MetricsServeConfig holds configuration for the metrics server.
type MetricsServeConfig struct {
	// Enabled determines whether to start the metrics server (default: true)
	Enabled bool

	// Addr is the address for the metrics server (e.g., ":9090")
	Addr string
}

// CAPIModeConfig holds CAPI federation mode configuration.
type CAPIModeConfig struct {
	// Enabled enables CAPI federation mode for multi-cluster operations
	Enabled bool

	// Cache configuration
	CacheTTL             string
	CacheMaxEntries      int
	CacheCleanupInterval string

	// OAuthTokenLifetime is the expected lifetime of OAuth tokens from your provider.
	// If CacheTTL exceeds this value, a warning is logged. This helps prevent
	// authentication failures from using cached clients with expired tokens.
	// Defaults to 1 hour if not specified.
	OAuthTokenLifetime string

	// Connectivity configuration
	ConnectivityTimeout        string
	ConnectivityRetryAttempts  int
	ConnectivityRetryBackoff   string
	ConnectivityRequestTimeout string
	ConnectivityQPS            float32
	ConnectivityBurst          int
}

// OAuthServeConfig holds OAuth-specific configuration.
type OAuthServeConfig struct {
	Enabled                            bool
	BaseURL                            string
	Provider                           string // "dex" or "google"
	GoogleClientID                     string
	GoogleClientSecret                 string
	DexIssuerURL                       string
	DexClientID                        string
	DexClientSecret                    string
	DexConnectorID                     string // optional: bypasses connector selection screen
	DexCAFile                          string // optional: CA certificate file for Dex TLS verification
	DexKubernetesAuthenticatorClientID string // optional: enables cross-client audience for K8s API auth
	DisableStreaming                   bool
	RegistrationToken                  string
	AllowPublicRegistration            bool
	AllowInsecureAuthWithoutState      bool
	AllowPrivateURLs                   bool // skip private IP validation for internal deployments
	MaxClientsPerIP                    int
	EncryptionKey                      string
	TLSCertFile                        string
	TLSKeyFile                         string

	// Redirect URI Security Configuration
	// These settings control security validation of redirect URIs during client registration.
	// All settings default to secure values - operators must explicitly opt-out.
	RedirectURISecurity RedirectURISecurityConfig

	// TrustedPublicRegistrationSchemes lists URI schemes that are allowed for
	// unauthenticated client registration. Clients registering with redirect URIs
	// using ONLY these schemes do NOT need a RegistrationAccessToken.
	// This enables Cursor and other MCP clients that don't support registration tokens.
	//
	// Security: Custom URI schemes can only be intercepted by the app that registered
	// the scheme with the OS. However, this has platform-specific limitations and is
	// most appropriate for internal/development deployments. For high-security production
	// deployments, use registration tokens instead.
	//
	// Schemes must conform to RFC 3986 syntax. Dangerous schemes are blocked.
	// Example: ["cursor", "vscode", "vscode-insiders", "windsurf"]
	TrustedPublicRegistrationSchemes []string

	// DisableStrictSchemeMatching allows clients with mixed redirect URI schemes
	// (e.g., cursor:// AND https://) to register without a token if ANY URI uses
	// a trusted scheme. Default: false (all URIs must use trusted schemes).
	// WARNING: Reduces security - only enable if you have specific requirements.
	DisableStrictSchemeMatching bool

	// EnableCIMD enables Client ID Metadata Documents (CIMD) per MCP 2025-11-25.
	// When enabled, clients can use HTTPS URLs as client identifiers, and the
	// authorization server will fetch client metadata from that URL.
	// This enables decentralized client registration where clients host their
	// own metadata documents.
	// Default: true (enabled per MCP 2025-11-25 specification)
	EnableCIMD bool

	// CIMDAllowPrivateIPs allows CIMD metadata URLs that resolve to private IP addresses
	// (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 per RFC 1918). This also allows loopback
	// addresses (127.0.0.0/8, ::1) and link-local addresses.
	// WARNING: Reduces SSRF protection. Only enable for internal/VPN deployments where
	// MCP servers legitimately communicate over private networks.
	//
	// Use cases:
	//   - Home lab deployments
	//   - Air-gapped environments
	//   - Internal enterprise networks
	//   - Any deployment where MCP servers communicate over private networks
	//
	// Default: false (blocked for security)
	CIMDAllowPrivateIPs bool

	// TrustedAudiences lists client IDs whose tokens are accepted for SSO.
	// When upstream aggregators (like muster) forward a user's ID token,
	// mcp-kubernetes will accept it if the token's audience matches any
	// entry in this list. This enables Single Sign-On across the MCP ecosystem.
	//
	// Security:
	//   - Only explicitly listed client IDs are trusted
	//   - Tokens must still be from the configured issuer (Dex/Google)
	//   - The IdP's cryptographic signature proves token authenticity
	//
	// Example: ["muster-client", "another-aggregator"]
	TrustedAudiences []string

	// SSOAllowPrivateIPs allows JWKS endpoints (used for SSO token validation) that
	// resolve to private IP addresses (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 per RFC 1918).
	// This also allows loopback addresses (127.0.0.0/8, ::1) and link-local addresses.
	//
	// When TrustedAudiences is configured, mcp-kubernetes validates forwarded ID tokens
	// by fetching the IdP's JWKS (JSON Web Key Set) to verify signatures. If your IdP
	// (like Dex) is deployed on a private network, JWKS fetching will fail without this option.
	//
	// Use cases:
	//   - Home lab deployments where Dex runs on an internal network
	//   - Air-gapped environments
	//   - Internal enterprise networks
	//   - Any deployment where the IdP is only accessible over private networks
	//
	// WARNING: Reduces SSRF protection. Only enable for internal deployments
	// that are not exposed to the public internet.
	//
	// Default: false (blocked for security)
	SSOAllowPrivateIPs bool

	// Storage configuration
	Storage server.OAuthStorageConfig
}

// RedirectURISecurityConfig is an alias to server.RedirectURISecurityConfig.
// See server.RedirectURISecurityConfig for field documentation.
type RedirectURISecurityConfig = server.RedirectURISecurityConfig

// Type aliases for OAuth storage configuration - use server package types directly
// to avoid duplication and ensure consistency across the codebase.
type (
	// OAuthStorageType represents the type of token storage backend.
	OAuthStorageType = server.OAuthStorageType
	// OAuthStorageConfig holds configuration for OAuth token storage backend.
	OAuthStorageConfig = server.OAuthStorageConfig
	// ValkeyStorageConfig holds configuration for Valkey storage backend.
	ValkeyStorageConfig = server.ValkeyStorageConfig
)

// Storage type constants - re-exported from server package for convenience.
const (
	OAuthStorageTypeMemory = server.OAuthStorageTypeMemory
	OAuthStorageTypeValkey = server.OAuthStorageTypeValkey
)

// loadEnvIfEmpty loads an environment variable into a string pointer if it's empty.
func loadEnvIfEmpty(target *string, envKey string) {
	if *target == "" {
		*target = os.Getenv(envKey)
	}
}

// validateSecureURL validates that a URL uses HTTPS and is not vulnerable to SSRF attacks.
// It checks for:
// - Valid URL format
// - HTTPS scheme (HTTP not allowed)
// - No private/local IP addresses (unless allowPrivate is true)
// - No localhost references
func validateSecureURL(urlStr string, fieldName string, allowPrivate bool) error {
	// Check for empty URL
	if urlStr == "" {
		return fmt.Errorf("%s must be a valid URL: empty URL provided", fieldName)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", fieldName, err)
	}

	// Require HTTPS
	if parsedURL.Scheme != schemeHTTPS {
		if parsedURL.Scheme == "" {
			return fmt.Errorf("%s must be a valid URL with HTTPS scheme", fieldName)
		}
		return fmt.Errorf("%s must use HTTPS (got: %s)", fieldName, parsedURL.Scheme)
	}

	// Extract hostname for validation
	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("%s must have a valid hostname", fieldName)
	}

	// Check for localhost references
	if strings.ToLower(hostname) == "localhost" {
		return fmt.Errorf("%s cannot use localhost", fieldName)
	}

	// Resolve hostname to IP addresses to check for private IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// DNS lookup failure - this could be transient or the domain doesn't exist yet
		// For development/testing purposes, we'll allow this but log a warning
		slog.Warn("could not resolve hostname to validate IP address",
			"field", fieldName,
			"hostname", hostname,
			"error", err)
		return nil
	}

	// Check if any resolved IP is private or loopback (unless allowPrivate is true)
	if !allowPrivate {
		for _, ip := range ips {
			if isPrivateOrLoopbackIP(ip) {
				return fmt.Errorf("%s resolves to a private or loopback IP address (%s), which could be a security risk", fieldName, ip.String())
			}
		}
	}

	return nil
}

// validOAuthClientIDPattern defines allowed characters for OAuth client IDs.
// Client IDs should only contain alphanumeric characters, hyphens, underscores, and periods.
// This is a defensive validation to prevent injection attacks via malformed client IDs.
var validOAuthClientIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// validURISchemePattern defines valid URI scheme syntax per RFC 3986 Section 3.1.
// Scheme must start with a letter and be followed by any combination of letters,
// digits, plus (+), hyphen (-), or period (.).
// Examples: http, https, cursor, vscode, vscode-insiders, my.app
var validURISchemePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*$`)

// validateOAuthClientID validates that an OAuth client ID contains only safe characters.
// This is a defense-in-depth measure - the OAuth provider will also validate client IDs,
// but we validate early to provide clear error messages and prevent any injection attempts.
func validateOAuthClientID(clientID, fieldName string) error {
	if clientID == "" {
		return nil // Empty is valid (optional field)
	}

	// Check length bounds (reasonable limits for client IDs)
	if len(clientID) > 256 {
		return fmt.Errorf("%s is too long (max 256 characters)", fieldName)
	}

	// Validate character set
	if !validOAuthClientIDPattern.MatchString(clientID) {
		return fmt.Errorf("%s contains invalid characters (allowed: alphanumeric, hyphens, underscores, periods; must start with alphanumeric)", fieldName)
	}

	return nil
}

// validateTrustedSchemes validates that all trusted URI schemes conform to RFC 3986 syntax.
// This is a defense-in-depth measure to ensure only valid scheme names are accepted.
// Per RFC 3986 Section 3.1, schemes must:
// - Start with a letter (a-z, A-Z)
// - Followed by any combination of letters, digits, plus (+), hyphen (-), or period (.)
// Schemes are normalized to lowercase for comparison.
func validateTrustedSchemes(schemes []string) error {
	if len(schemes) == 0 {
		return nil // Empty is valid (feature disabled)
	}

	for _, scheme := range schemes {
		// Check for empty scheme
		if scheme == "" {
			return fmt.Errorf("trusted scheme cannot be empty")
		}

		// Check length bounds (reasonable limit for scheme names)
		if len(scheme) > 64 {
			return fmt.Errorf("trusted scheme %q is too long (max 64 characters)", scheme)
		}

		// Validate against RFC 3986 scheme syntax
		if !validURISchemePattern.MatchString(scheme) {
			return fmt.Errorf("trusted scheme %q is invalid per RFC 3986 (must start with letter, followed by letters, digits, +, -, or .)", scheme)
		}

		// Warn about potentially dangerous schemes (but still allow them - operator's choice)
		lowerScheme := strings.ToLower(scheme)
		switch lowerScheme {
		case schemeHTTP, schemeHTTPS:
			slog.Warn("trustedPublicRegistrationSchemes includes a web scheme - this allows unauthenticated registration for web clients which may be a security risk",
				"scheme", scheme,
				"recommendation", "Consider using a registration token for web clients instead")
		case "javascript", "data", "file", "ftp":
			return fmt.Errorf("trusted scheme %q is not allowed - these schemes pose security risks (XSS, local file access)", scheme)
		}
	}

	return nil
}

// isPrivateOrLoopbackIP checks if an IP address is private, loopback, or link-local.
func isPrivateOrLoopbackIP(ip net.IP) bool {
	// Check for loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	// 10.0.0.0/8
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges (fc00::/7 - Unique Local Addresses)
	if len(ip) == net.IPv6len && ip[0] == 0xfc || ip[0] == 0xfd {
		return true
	}

	return false
}

// validateOAuthBaseURL validates the OAuth base URL, allowing localhost for development
// This is less strict than validateSecureURL because OAuth base URL can be localhost
// for local development, but Dex issuer URL cannot (SSRF protection)
func validateOAuthBaseURL(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("OAuth base URL cannot be empty")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("OAuth base URL must be a valid URL: %w", err)
	}

	// Require HTTPS for non-localhost addresses
	if parsedURL.Scheme != schemeHTTPS {
		host := parsedURL.Hostname()
		// Allow HTTP only for loopback addresses (localhost, 127.0.0.1, ::1)
		if parsedURL.Scheme == schemeHTTP {
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				// HTTP localhost is allowed for development
				return nil
			}
			return fmt.Errorf("OAuth base URL must use HTTPS for non-localhost addresses (got: %s). Use HTTPS or localhost for development", baseURL)
		}
		return fmt.Errorf("OAuth base URL must use http or https scheme (got: %s)", parsedURL.Scheme)
	}

	// HTTPS is always allowed (including localhost with HTTPS)
	return nil
}
