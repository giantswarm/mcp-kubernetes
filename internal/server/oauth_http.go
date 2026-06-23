package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	oauth "github.com/giantswarm/mcp-oauth"
	"github.com/giantswarm/mcp-oauth/handler"
	oauthinstrumentation "github.com/giantswarm/mcp-oauth/instrumentation"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/providers/dex"
	"github.com/giantswarm/mcp-oauth/providers/google"
	"github.com/giantswarm/mcp-oauth/providers/oidc"
	"github.com/giantswarm/mcp-oauth/security"
	oauthserver "github.com/giantswarm/mcp-oauth/server"
	"github.com/giantswarm/mcp-oauth/storage"
	"github.com/giantswarm/mcp-oauth/storage/memory"
	"github.com/giantswarm/mcp-oauth/storage/valkey"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
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
	// ServiceVersion is the version of mcp-kubernetes for instrumentation
	ServiceVersion string

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

	// DexKubernetesAuthenticatorClientID is the client ID of the Kubernetes authenticator
	// in Dex (typically "dex-k8s-authenticator"). When set, requests tokens with this
	// audience via Dex cross-client authentication, enabling the ID token to be used
	// for Kubernetes API authentication.
	DexKubernetesAuthenticatorClientID string

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

	// TLSCertFile is the path to the TLS certificate file (PEM format)
	// If both TLSCertFile and TLSKeyFile are provided, the server will use HTTPS
	TLSCertFile string

	// TLSKeyFile is the path to the TLS private key file (PEM format)
	// If both TLSCertFile and TLSKeyFile are provided, the server will use HTTPS
	TLSKeyFile string

	// InstrumentationProvider is the OpenTelemetry instrumentation provider for metrics/tracing
	InstrumentationProvider *instrumentation.Provider

	// Storage configures the token storage backend
	// Defaults to in-memory storage if not specified
	Storage OAuthStorageConfig

	// RedirectURISecurity configures security validation for redirect URIs
	// All options default to secure values in mcp-oauth
	RedirectURISecurity RedirectURISecurityConfig

	// TrustedPublicRegistrationSchemes lists URI schemes allowed for unauthenticated
	// client registration. Enables Cursor/VSCode without registration tokens.
	// Best suited for internal/development deployments due to platform-specific
	// limitations in custom URI scheme security. Schemes must conform to RFC 3986.
	TrustedPublicRegistrationSchemes []string

	// DisableStrictSchemeMatching allows mixed scheme clients to register without token
	DisableStrictSchemeMatching bool

	// EnableCIMD enables Client ID Metadata Documents per MCP 2025-11-25.
	// When enabled, clients can use HTTPS URLs as client identifiers.
	// Default: true (enabled for MCP 2025-11-25 compliance)
	EnableCIMD bool

	// CIMDAllowPrivateIPs allows CIMD metadata URLs to resolve to private/internal IPs.
	// See cmd.OAuthServeConfig.CIMDAllowPrivateIPs for detailed documentation.
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
	// resolve to private IP addresses (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16).
	// This is required when your IdP (like Dex) runs on a private network.
	// Maps to AllowPrivateIPJWKS in mcp-oauth v0.2.40+.
	// WARNING: Reduces SSRF protection. Only enable for internal deployments.
	// Default: false (blocked for security)
	SSOAllowPrivateIPs bool

	// TrustedIssuers lists external JWT issuers whose tokens are accepted at /mcp.
	// Each entry's JWKS is used to verify the Bearer JWT's signature when its `iss`
	// matches. AllowedAudiences restricts the accepted `aud` values; when empty,
	// any audience is accepted.
	TrustedIssuers []TrustedIssuerConfig
}

// intersectGroups returns the members of tokenGroups present in allowList,
// preserving allowList order and dropping duplicates. It is the allow-list gate
// for M2M group impersonation: only configured groups carried by the token are
// honored.
func intersectGroups(allowList, tokenGroups []string) []string {
	if len(allowList) == 0 || len(tokenGroups) == 0 {
		return nil
	}
	present := make(map[string]struct{}, len(tokenGroups))
	for _, g := range tokenGroups {
		present[g] = struct{}{}
	}
	var matched []string
	seen := make(map[string]struct{}, len(allowList))
	for _, g := range allowList {
		if _, ok := present[g]; !ok {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		matched = append(matched, g)
	}
	return matched
}

// matchesSubGlob reports whether s matches a sub-claim pattern.
// matchesSubGlob matches s against pattern using a single leading or trailing
// unionStrings returns a slice containing all elements from a and b with
// duplicates removed. Order is a first, then elements from b not already in a.
func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a))
	result := make([]string, 0, len(a)+len(b))
	for _, v := range a {
		seen[v] = struct{}{}
		result = append(result, v)
	}
	for _, v := range b {
		if _, ok := seen[v]; !ok {
			result = append(result, v)
		}
	}
	return result
}

func appendIfMissing(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// wildcard (*). A leading * anchors to a suffix ("*@example.com" matches any
// string ending in "@example.com"). A trailing * anchors to a prefix
// ("system:serviceaccount:kagent:*" matches any SA in that namespace).
// An exact pattern (no *) requires an exact string equality.
func matchesSubGlob(pattern, s string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return prefix != "" && strings.HasPrefix(s, prefix)
	}
	if strings.HasPrefix(pattern, "*") {
		suffix := pattern[1:]
		return suffix != "" && strings.HasSuffix(s, suffix)
	}
	return pattern == s
}

// actorChainSubjects returns the set of subjects in the token's RFC 8693 §4.4
// delegation chain (act, act.act, ...). The token is already validated by the
// upstream ValidateToken middleware, so the act claim is read from the
// authentic payload without re-verifying the signature. Returns nil when the
// token carries no act claim or cannot be parsed; callers must still seed the
// set with the validated leaf actor (userInfo.ActorSubject).
func actorChainSubjects(token string) map[string]struct{} {
	rawClaims, err := oidc.ParseUnverifiedClaims(token)
	if err != nil {
		return nil
	}
	actRaw, ok := rawClaims["act"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(actRaw)
	if err != nil {
		return nil
	}
	var actor oidc.ActorClaim
	if err := json.Unmarshal(encoded, &actor); err != nil {
		return nil
	}
	subjects := make(map[string]struct{})
	for a := &actor; a != nil; a = a.Act {
		if a.Subject != "" {
			subjects[a.Subject] = struct{}{}
		}
	}
	return subjects
}

// ActorConfig defines an OBO actor (RFC 8693 act.sub) and the human subjects it
// is permitted to act on behalf of.
type ActorConfig struct {
	// Sub is the actor's JWT subject (act.sub claim), typically a K8s SA sub:
	// "system:serviceaccount:<ns>:<name>".
	Sub string `json:"sub"`
	// AllowedSubjects is the set of human subject values (userInfo.ID) that this
	// actor may impersonate. Trailing-star glob patterns are supported
	// (e.g. "*@example.com"). Empty means any subject is allowed, still bounded
	// by the issuer's effective-subject-claim pattern and the RBAC outer grant.
	AllowedSubjects []string `json:"allowedSubjects,omitempty"`
}

// TrustedIssuerConfig holds the configuration for a single trusted external JWT issuer.
type TrustedIssuerConfig struct {
	// Issuer is the expected `iss` claim value in the JWT.
	Issuer string `json:"issuer"`
	// JwksURL is the JWKS endpoint used to verify signatures.
	JwksURL string `json:"jwksURL"`
	// Alias is a stable short identifier used by the chart to key per-issuer
	// Namespaces and impersonation ClusterRoles. Optional; must be a valid
	// DNS-1123 label if set and unique across entries.
	Alias string `json:"alias,omitempty"`
	// ImpersonateUser, when set, enforces that the token's effective subject
	// (userInfo.ID, after any SubjectClaim remap) equals this value verbatim.
	// The chart scopes the mcp-kubernetes ServiceAccount's impersonate-users
	// RBAC resourceName to exactly this value. Empty means no subject restriction.
	ImpersonateUser string `json:"impersonateUser,omitempty"`
	// ImpersonateGroups, when set, is the allow-list of groups this issuer's
	// tokens may project as Impersonate-Group. The intersection of this list and
	// the token's groups claim becomes the set of impersonated groups; a token
	// carrying none of these groups is rejected. Empty means the token's groups
	// are used directly without restriction.
	ImpersonateGroups []string `json:"impersonateGroups,omitempty"`
	// AllowedAudiences restricts accepted `aud` values. Empty means any audience.
	AllowedAudiences []string `json:"allowedAudiences,omitempty"`
	// AllowedTargetClusters limits which management/workload cluster names this
	// issuer's tokens may be impersonated onto. Empty means any cluster.
	AllowedTargetClusters []string `json:"allowedTargetClusters,omitempty"`
	// AllowedClaims constrains accepted tokens to those whose JWT claims match
	// all entries exactly (e.g. {"sub": "system:serviceaccount:kagent:*"}).
	// Wildcard suffix matching is supported for the "sub" claim only.
	// When SubjectClaim is set, the entry under that key (not "sub") gates the
	// impersonated subject; mcp-oauth evaluates AllowedClaims against the raw
	// token before the subject is remapped, so the opaque sub cannot carry the
	// remapped pattern.
	AllowedClaims map[string]string `json:"allowedClaims,omitempty"`
	// SubjectClaim names the verified claim whose value becomes the impersonated
	// subject, replacing the standard sub claim. Set to "email" on the muster-obo
	// issuer so muster's opaque sub is remapped to the human email. mcp-oauth
	// fails closed if the claim is absent or not a non-empty string.
	SubjectClaim string `json:"subjectClaim,omitempty"`
	// AcceptedTypHeaders overrides the default RFC 9068 typ=at+jwt check.
	// Set to ["", "JWT"] to accept Kubernetes ServiceAccount tokens.
	AcceptedTypHeaders []string `json:"acceptedTypHeaders,omitempty"`
	// AllowPrivateIPJWKS permits JWKS endpoints on private/loopback addresses.
	// Prefer AllowPrivateIPJWKSHosts for a narrower escape hatch.
	AllowPrivateIPJWKS bool `json:"allowPrivateIPJWKS,omitempty"`
	// AllowPrivateIPJWKSHosts lists the specific hostnames whose JWKS URL is
	// permitted to resolve to a private IP. All other hosts retain SSRF
	// protection. Use instead of AllowPrivateIPJWKS when the endpoint is a
	// known in-cluster service (e.g. muster.agentic-platform.svc.cluster.local).
	AllowPrivateIPJWKSHosts []string `json:"allowPrivateIPJWKSHosts,omitempty"`
	// AllowedActors, when set, restricts which OBO actors (act.sub) are permitted
	// for this issuer and, per actor, which human subjects they may impersonate.
	// Empty means any actor is accepted (the issuer's JWKS is the trust boundary).
	AllowedActors []ActorConfig `json:"allowedActors,omitempty"`
}

// EffectiveSubjectKey returns the AllowedClaims key that gates the impersonated
// subject: SubjectClaim when set, otherwise the standard "sub". The validated
// subject (userInfo.ID) is matched against AllowedClaims[EffectiveSubjectKey()].
func (c TrustedIssuerConfig) EffectiveSubjectKey() string {
	if c.SubjectClaim != "" {
		return c.SubjectClaim
	}
	return "sub"
}

// RedirectURISecurityConfig holds configuration for redirect URI security validation.
// All options default to secure values in mcp-oauth. Use Disable* flags to opt-out.
type RedirectURISecurityConfig struct {
	// DisableProductionMode disables strict HTTPS/private IP enforcement
	DisableProductionMode bool

	// AllowLocalhostRedirectURIs allows http://localhost for native apps (RFC 8252)
	AllowLocalhostRedirectURIs bool

	// AllowPrivateIPRedirectURIs allows private IP addresses in redirect URIs
	AllowPrivateIPRedirectURIs bool

	// AllowLinkLocalRedirectURIs allows link-local addresses (169.254.x.x)
	AllowLinkLocalRedirectURIs bool

	// DisableDNSValidation disables hostname resolution checks
	DisableDNSValidation bool

	// DisableDNSValidationStrict disables fail-closed DNS validation
	DisableDNSValidationStrict bool

	// DisableAuthorizationTimeValidation disables redirect URI checks at auth time
	DisableAuthorizationTimeValidation bool
}

// OAuthHTTPServer wraps an MCP server with OAuth 2.1 authentication
type OAuthHTTPServer struct {
	mcpServer               *mcpserver.MCPServer
	oauthServer             *oauth.Server
	oauthHandler            *handler.Handler
	tokenStore              storage.TokenStore
	httpServer              *http.Server
	serverType              string // "streamable-http"
	disableStreaming        bool
	instrumentationProvider *instrumentation.Provider
	healthChecker           *HealthChecker
	// trustedIssuersByIssuer maps issuer URL to the configured entries for that
	// issuer. Multiple entries per URL are supported (e.g. M2M + OBO from the
	// same STS); the matching entry is selected by subject pattern at request time.
	trustedIssuersByIssuer map[string][]TrustedIssuerConfig
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
		// Build scopes list, adding cross-client audience if configured
		scopes := make([]string, len(dexOAuthScopes))
		copy(scopes, dexOAuthScopes)
		if config.DexKubernetesAuthenticatorClientID != "" {
			// Request cross-client audience for Kubernetes API authentication
			// See: https://dexidp.io/docs/custom-scopes-claims-clients/#cross-client-trust-and-authorized-party
			audienceScope := "audience:server:client_id:" + config.DexKubernetesAuthenticatorClientID
			scopes = append(scopes, audienceScope)
			logger.Info("Requesting cross-client audience for Kubernetes API authentication",
				"kubernetesAuthenticatorClientID", config.DexKubernetesAuthenticatorClientID)
		}

		dexConfig := &dex.Config{
			IssuerURL:    config.DexIssuerURL,
			ClientID:     config.DexClientID,
			ClientSecret: config.DexClientSecret,
			RedirectURL:  redirectURL,
			Scopes:       scopes,
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

		var valkeyOpts []valkey.Option
		if len(config.EncryptionKey) > 0 {
			encryptor, err := security.NewEncryptor(config.EncryptionKey)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create encryptor for Valkey storage: %w", err)
			}
			valkeyOpts = append(valkeyOpts, valkey.WithEncryptor(encryptor))
		}

		valkeyStore, err := valkey.New(valkeyConfig, valkeyOpts...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Valkey storage: %w", err)
		}
		if len(config.EncryptionKey) > 0 {
			logger.Info("Token encryption at rest enabled for Valkey storage (AES-256-GCM)")
		}

		// Valkey store implements all required interfaces
		tokenStore = valkeyStore
		clientStore = valkeyStore
		flowStore = valkeyStore
		logger.Info("Using Valkey storage backend", "address", config.Storage.Valkey.URL, "tls", config.Storage.Valkey.TLSEnabled)

	case OAuthStorageTypeMemory, "":
		var memOpts []memory.Option
		if len(config.EncryptionKey) > 0 {
			encryptor, err := security.NewEncryptor(config.EncryptionKey)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create encryptor: %w", err)
			}
			memOpts = append(memOpts, memory.WithEncryptor(encryptor))
		}
		memStore := memory.New(memOpts...)
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

		// Enable Client ID Metadata Documents (CIMD) per MCP 2025-11-25
		// Allows clients to use HTTPS URLs as client identifiers
		// The authorization server fetches client metadata from that URL
		// Configurable via config.EnableCIMD (defaults to true for MCP compliance)
		EnableClientIDMetadataDocuments: config.EnableCIMD,

		// Allow CIMD metadata URLs that resolve to private IP addresses
		// Required for internal deployments where MCP servers communicate over private networks
		AllowPrivateIPClientMetadata: config.CIMDAllowPrivateIPs,

		// Trusted scheme registration for Cursor/VSCode compatibility
		// Allows unauthenticated registration for clients using these schemes only
		TrustedPublicRegistrationSchemes: config.TrustedPublicRegistrationSchemes,
		DisableStrictSchemeMatching:      config.DisableStrictSchemeMatching,

		// Redirect URI Security Configuration
		// mcp-oauth defaults to secure values; we pass explicit disable flags
		DisableProductionMode:              config.RedirectURISecurity.DisableProductionMode,
		AllowLocalhostRedirectURIs:         config.RedirectURISecurity.AllowLocalhostRedirectURIs,
		AllowPrivateIPRedirectURIs:         config.RedirectURISecurity.AllowPrivateIPRedirectURIs,
		AllowLinkLocalRedirectURIs:         config.RedirectURISecurity.AllowLinkLocalRedirectURIs,
		DisableDNSValidation:               config.RedirectURISecurity.DisableDNSValidation,
		DisableDNSValidationStrict:         config.RedirectURISecurity.DisableDNSValidationStrict,
		DisableAuthorizationTimeValidation: config.RedirectURISecurity.DisableAuthorizationTimeValidation,

		// AllowPrivateIPJWKS for JWKS fetching from internal IdPs (mcp-oauth v0.2.40+)
		// Enables SSO token forwarding when IdP (e.g., Dex) is on a private network
		AllowPrivateIPJWKS: config.SSOAllowPrivateIPs,
	}

	// Debug logging for registration token configuration
	if config.DebugMode {
		if config.RegistrationAccessToken != "" {
			tokenPrefix := config.RegistrationAccessToken
			if len(tokenPrefix) > 8 {
				tokenPrefix = tokenPrefix[:8] + "..."
			}
			logger.Info("Registration access token configured", "token_length", len(config.RegistrationAccessToken), "token_prefix", tokenPrefix)
		} else {
			logger.Warn("Registration access token is empty - public registration must be enabled")
		}
		logger.Info("Client registration configuration", "allow_public", config.AllowPublicClientRegistration, "has_token", config.RegistrationAccessToken != "")
	}

	// Configure interstitial page branding if provided
	if config.Interstitial != nil {
		serverConfig.Interstitial = config.Interstitial
	}

	var opts []oauth.ServerOption

	// Instrumentation for mcp-oauth internal metrics (CIMD, rate limiting, etc.)
	// Uses Prometheus exporter to expose metrics alongside mcp-kubernetes metrics.
	inst, err := oauthinstrumentation.New(oauthinstrumentation.Config{
		Enabled:         true,
		ServiceName:     "mcp-kubernetes",
		ServiceVersion:  config.ServiceVersion,
		MetricsExporter: "prometheus",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create instrumentation: %w", err)
	}
	opts = append(opts, oauth.WithInstrumentation(inst))

	// Set up audit logging (always enabled for security)
	auditor := security.NewAuditor(logger, true)
	opts = append(opts, oauth.WithAuditor(auditor))
	logger.Info("Security audit logging enabled")

	// Set up IP-based rate limiting
	ipRateLimiter := security.NewRateLimiter(DefaultIPRateLimit, DefaultIPBurst, logger)
	opts = append(opts, oauth.WithRateLimiter(ipRateLimiter))
	logger.Info("IP-based rate limiting enabled", "rate", DefaultIPRateLimit, "burst", DefaultIPBurst)

	// Set up user-based rate limiting
	userRateLimiter := security.NewRateLimiter(DefaultUserRateLimit, DefaultUserBurst, logger)
	opts = append(opts, oauth.WithUserRateLimiter(userRateLimiter))
	logger.Info("User-based rate limiting enabled", "rate", DefaultUserRateLimit, "burst", DefaultUserBurst)

	// Set up client registration rate limiting with configured maxClientsPerIP
	// This aligns the rate limiter with the server's MaxClientsPerIP configuration
	clientRegRL := security.NewClientRegistrationRateLimiterWithConfig(
		maxClientsPerIP,
		security.DefaultRegistrationWindow,
		security.DefaultMaxRegistrationEntries,
		logger,
	)
	opts = append(opts, oauth.WithClientRegistrationRateLimiter(clientRegRL))
	logger.Info("Client registration rate limiting enabled",
		"maxClientsPerIP", maxClientsPerIP,
		"window", security.DefaultRegistrationWindow)

	if len(config.TrustedAudiences) > 0 {
		opts = append(opts, oauthserver.WithTrustedAudiences(config.TrustedAudiences))
	}

	if len(config.TrustedIssuers) > 0 {
		// mcp-oauth's OIDCValidator is keyed map[issuerURL]TrustedIssuer; duplicate
		// issuer URLs overwrite each other. When a single issuer hosts multiple trust
		// domains (e.g. M2M SA sub + OBO email sub), deduplicate into one entry per
		// issuer with unioned audiences. AllowedClaims is omitted for multi-entry
		// issuers because per-entry subject matching runs in AccessTokenInjector.
		type mergedIssuer struct {
			oauthserver.TrustedIssuer
			entryCount int
		}
		merged := make(map[string]*mergedIssuer, len(config.TrustedIssuers))
		var issuerOrder []string
		for _, ti := range config.TrustedIssuers {
			if e, ok := merged[ti.Issuer]; !ok {
				merged[ti.Issuer] = &mergedIssuer{
					TrustedIssuer: oauthserver.TrustedIssuer{
						Issuer:                  ti.Issuer,
						JwksURL:                 ti.JwksURL,
						AllowedAudiences:        ti.AllowedAudiences,
						AllowedClaims:           ti.AllowedClaims,
						SubjectClaim:            ti.SubjectClaim,
						AcceptedTypHeaders:      ti.AcceptedTypHeaders,
						AllowPrivateIPJWKS:      ti.AllowPrivateIPJWKS,
						AllowPrivateIPJWKSHosts: ti.AllowPrivateIPJWKSHosts,
					},
					entryCount: 1,
				}
				issuerOrder = append(issuerOrder, ti.Issuer)
			} else {
				e.entryCount++
				e.AllowedAudiences = unionStrings(e.AllowedAudiences, ti.AllowedAudiences)
				if ti.AllowPrivateIPJWKS {
					e.AllowPrivateIPJWKS = true
				}
				e.AllowPrivateIPJWKSHosts = unionStrings(e.AllowPrivateIPJWKSHosts, ti.AllowPrivateIPJWKSHosts)
			}
		}
		issuers := make([]oauthserver.TrustedIssuer, 0, len(merged))
		for _, iss := range issuerOrder {
			e := merged[iss]
			if e.entryCount > 1 {
				// Per-entry allowedClaims enforcement is in AccessTokenInjector; drop
				// it here so a later entry's pattern cannot reject earlier entries' tokens.
				e.AllowedClaims = nil
				e.SubjectClaim = ""
			}
			issuers = append(issuers, e.TrustedIssuer)
		}
		opts = append(opts, oauthserver.WithTrustedIssuers(issuers))
	}

	// Create OAuth server
	server, err := oauth.NewServer(
		provider,
		tokenStore,  // TokenStore
		clientStore, // ClientStore
		flowStore,   // FlowStore
		serverConfig,
		logger,
		opts...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OAuth server: %w", err)
	}

	return server, tokenStore, nil
}

// NewOAuthHTTPServer creates a new OAuth-enabled HTTP server
func NewOAuthHTTPServer(mcpServer *mcpserver.MCPServer, serverType string, config OAuthConfig) (*OAuthHTTPServer, error) {
	oauthServer, tokenStore, err := createOAuthServer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth server: %w", err)
	}

	// Build issuer→config lookup map for the access-token injector middleware.
	// Multiple entries per issuer URL are collected into a slice; the correct
	// entry is selected at request time by matching the token subject pattern.
	issuerMap := make(map[string][]TrustedIssuerConfig, len(config.TrustedIssuers))
	for _, ti := range config.TrustedIssuers {
		issuerMap[ti.Issuer] = append(issuerMap[ti.Issuer], ti)
	}

	// Create HTTP handler
	oauthHandler := handler.New(oauthServer, oauthServer.Logger)

	return &OAuthHTTPServer{
		mcpServer:               mcpServer,
		oauthServer:             oauthServer,
		oauthHandler:            oauthHandler,
		tokenStore:              tokenStore,
		serverType:              serverType,
		disableStreaming:        config.DisableStreaming,
		instrumentationProvider: config.InstrumentationProvider,
		trustedIssuersByIssuer:  issuerMap,
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
	oauthHandler := handler.New(oauthServer, oauthServer.Logger)

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
		// Create Streamable HTTP server with HTTPContextFunc to propagate access token
		// to mcp-go's tool execution context. The token is set in r.Context() by our
		// middleware chain, and we copy it to mcp-go's internal context here.
		httpContextFunc := s.createHTTPContextFunc()

		var httpServer http.Handler
		if s.disableStreaming {
			httpServer = mcpserver.NewStreamableHTTPServer(s.mcpServer,
				mcpserver.WithEndpointPath("/mcp"),
				mcpserver.WithDisableStreaming(true),
				mcpserver.WithHTTPContextFunc(httpContextFunc),
			)
		} else {
			httpServer = mcpserver.NewStreamableHTTPServer(s.mcpServer,
				mcpserver.WithEndpointPath("/mcp"),
				mcpserver.WithHTTPContextFunc(httpContextFunc),
			)
		}

		// Create middleware to inject access token into request context for downstream K8s auth
		accessTokenInjector := s.createAccessTokenInjectorMiddleware(httpServer)

		// Fail fast when the validated UserInfo carries no email claim; without
		// an email there is no Impersonate-User to send to the Kubernetes API.
		requireIdentity := middleware.RequireIdentity(s.oauthServer.Auditor, s.oauthServer.Logger)

		// Wrap MCP endpoint with OAuth middleware (ValidateToken validates and adds user info)
		// Then enforce that UserInfo has an email before the injector / tool dispatch.
		mux.Handle("/mcp", s.oauthHandler.ValidateToken(requireIdentity(accessTokenInjector)))

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

	// Note: Prometheus metrics are now served on a separate metrics server
	// for security. The /metrics endpoint should NOT be exposed on the main
	// HTTP server to prevent unauthorized access to operational information.
	// See MetricsServer for the dedicated metrics endpoint.

	// Setup health check endpoints
	if s.healthChecker != nil {
		s.healthChecker.RegisterHealthEndpoints(mux)
	}

	// Create HTTP server with security, CORS, and metrics middleware
	// Order: Metrics (outermost) -> Security Headers -> CORS -> Handler
	// Metrics middleware wraps everything to capture all request metrics
	handler := middleware.HTTPMetrics(s.instrumentationProvider)(
		middleware.SecurityHeaders(config.EnableHSTS)(
			middleware.CORS(allowedOrigins)(mux),
		),
	)

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
	}

	// Start server with TLS if certificates are provided
	if config.TLSCertFile != "" && config.TLSKeyFile != "" {
		return s.httpServer.ListenAndServeTLS(config.TLSCertFile, config.TLSKeyFile)
	}

	// Start server without TLS
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
func (s *OAuthHTTPServer) GetOAuthHandler() *handler.Handler {
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

// extractBearerToken extracts the bearer token from the Authorization header.
// Returns the token string and true if found, or empty string and false if not.
func extractBearerToken(r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", false
	}

	// Bearer token format: "Bearer <token>"
	const bearerPrefix = "Bearer "
	if len(authHeader) <= len(bearerPrefix) {
		return "", false
	}
	if authHeader[:len(bearerPrefix)] != bearerPrefix {
		return "", false
	}

	return authHeader[len(bearerPrefix):], true
}

// createAccessTokenInjectorMiddleware creates middleware that injects the user's
// OAuth provider token (specifically the ID token) into the request context.
// This token can then be used for downstream Kubernetes API authentication.
//
// Token Injection Strategy:
//
//  1. SSO Token Forwarding (IsSSO() == true):
//     When the token was validated via TrustedAudiences/JWKS (SSO token forwarding),
//     the Bearer token IS the ID token forwarded from a trusted upstream service
//     (e.g., muster). We use it directly without token store lookup.
//
//  2. Normal OAuth Flow (IsSSO() == false):
//     When the token was issued by this server's OAuth flow, we look up the provider's
//     ID token from the token store using the MCP access token as the key.
//     This ensures we get the most up-to-date provider token, including any tokens
//     that have been proactively refreshed by mcp-oauth's token validation.
//
// Background: mcp-oauth stores provider tokens keyed by the MCP access token during
// token exchange and proactive refresh. Looking up by access token (instead of email)
// ensures we always get the current provider token, even after automatic refreshes.
func (s *OAuthHTTPServer) createAccessTokenInjectorMiddleware(next http.Handler) http.Handler {
	// Helper to record SSO token injection metrics
	recordMetric := func(ctx context.Context, result string) {
		if s.instrumentationProvider != nil && s.instrumentationProvider.Metrics() != nil {
			s.instrumentationProvider.Metrics().RecordSSOTokenInjection(ctx, result)
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log entry to confirm middleware is being called
		slog.Debug("AccessTokenInjector: middleware entry", //nolint:gosec // G706: values emitted via structured slog handler which escapes control chars
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("content_type", r.Header.Get("Content-Type")))

		ctx := r.Context()

		// Get user info from context (set by ValidateToken middleware)
		// This confirms the token has been validated
		userInfo, ok := handler.UserInfoFromContext(ctx)
		if !ok || userInfo == nil {
			slog.Debug("AccessTokenInjector: no user info in context")
			recordMetric(ctx, "no_user")
			next.ServeHTTP(w, r)
			return
		}

		// Extract the Bearer token from the Authorization header
		bearerToken, ok := extractBearerToken(r)
		if !ok || bearerToken == "" {
			slog.Debug("AccessTokenInjector: no bearer token in Authorization header",
				"user_id", userInfo.ID)
			recordMetric(ctx, "no_token")
			next.ServeHTTP(w, r)
			return
		}

		// External-issuer path: token was validated against a TrustedIssuer entry.
		// The Bearer's aud is the muster STS, not kube-apiserver — passthrough
		// would be rejected. Derive an ImpersonationIdentity instead; the inner
		// branch selects OBO (act claim present) or M2M (no act claim).
		if userInfo.IsExternalIssuer() {
			candidates := s.trustedIssuersByIssuer[userInfo.Issuer]
			if len(candidates) == 0 {
				slog.Warn("AccessTokenInjector: external-issuer token with unknown issuer, rejecting",
					"issuer", userInfo.Issuer)
				http.Error(w, "forbidden: unknown trusted issuer", http.StatusForbidden)
				return
			}
			// Select the best matching entry for this issuer+subject combination.
			// Entries with an allowedClaims subject pattern are tried first; entries
			// without one are eligible as a fallback for any subject (the JWKS URL
			// is the trust boundary for passthrough entries).
			var tiConfig *TrustedIssuerConfig
			var fallback *TrustedIssuerConfig
			for i := range candidates {
				subjectKey := candidates[i].EffectiveSubjectKey()
				subPattern, hasSubPattern := candidates[i].AllowedClaims[subjectKey]
				if !hasSubPattern || subPattern == "" {
					if fallback == nil {
						fallback = &candidates[i]
					}
					continue
				}
				if matchesSubGlob(subPattern, userInfo.ID) {
					tiConfig = &candidates[i]
					break
				}
			}
			if tiConfig == nil {
				tiConfig = fallback
			}
			if tiConfig == nil {
				slog.Warn("AccessTokenInjector: subject does not match any configured entry, rejecting",
					"issuer", userInfo.Issuer, "subject", userInfo.ID)
				http.Error(w, "forbidden: subject does not match allowed pattern", http.StatusForbidden)
				return
			}

			// OBO path: sub=human, act.sub=agent SA.
			if userInfo.IsOBO() {
				actorSub := userInfo.ActorSubject

				// When allowedActors is configured, enforce the actor allow-list and
				// per-actor subject scoping. K8s RBAC cannot couple impersonate-users
				// to impersonate-userextras/actor (independent verbs) so both checks
				// must live here. Walk the full RFC 8693 act chain: a configured actor
				// is authorized when its sub appears anywhere in the chain, so multi-hop
				// A2A (human → agentA → agentB → MCP) is honored.
				// Empty allowedActors means any actor is accepted.
				if len(tiConfig.AllowedActors) > 0 {
					chainSubjects := actorChainSubjects(bearerToken)
					if chainSubjects == nil {
						chainSubjects = make(map[string]struct{}, 1)
					}
					if actorSub != "" {
						chainSubjects[actorSub] = struct{}{}
					}
					var matchedActor *ActorConfig
					for i := range tiConfig.AllowedActors {
						if _, ok := chainSubjects[tiConfig.AllowedActors[i].Sub]; ok {
							matchedActor = &tiConfig.AllowedActors[i]
							break
						}
					}
					if matchedActor == nil {
						slog.Warn("AccessTokenInjector: no actor in delegation chain matches allowedActors, rejecting",
							"issuer", userInfo.Issuer, "leafActor", actorSub, "chainLen", len(chainSubjects))
						http.Error(w, "forbidden: actor not permitted for this issuer", http.StatusForbidden)
						return
					}
					if len(matchedActor.AllowedSubjects) > 0 {
						humanSub := userInfo.ID
						subAllowed := false
						for _, pattern := range matchedActor.AllowedSubjects {
							if matchesSubGlob(pattern, humanSub) {
								subAllowed = true
								break
							}
						}
						if !subAllowed {
							slog.Warn("AccessTokenInjector: OBO human subject not in actor's allowedSubjects, rejecting",
								"issuer", userInfo.Issuer, "actor", actorSub, "subject", humanSub)
							http.Error(w, "forbidden: subject not permitted for this actor", http.StatusForbidden)
							return
						}
					}
				}

				identity := k8s.ImpersonationIdentity{
					UserName: userInfo.ID,
					// system:authenticated must be explicit; impersonation does not
					// inherit the real-auth group set.
					Groups: []string{"system:authenticated"},
					Extra: map[string][]string{
						"issuer": {userInfo.Issuer},
						"agent":  {"mcp-kubernetes"},
					},
					Actor:                 actorSub,
					AllowedTargetClusters: tiConfig.AllowedTargetClusters,
				}
				ctx = ContextWithImpersonationIdentity(ctx, identity)
				r = r.WithContext(ctx)
				recordMetric(ctx, "obo_success")
				next.ServeHTTP(w, r)
				return
			}

			// M2M path.
			// If impersonateUser is set, enforce exact subject match.
			if tiConfig.ImpersonateUser != "" && tiConfig.ImpersonateUser != userInfo.ID {
				slog.Warn("AccessTokenInjector: M2M subject does not match impersonateUser, rejecting",
					"issuer", userInfo.Issuer, "subject", userInfo.ID)
				http.Error(w, "forbidden: subject not permitted for impersonation", http.StatusForbidden)
				return
			}

			var impersonateGroups []string
			if len(tiConfig.ImpersonateGroups) > 0 {
				// Intersect configured group allow-list with token groups.
				impersonateGroups = intersectGroups(tiConfig.ImpersonateGroups, userInfo.Groups)
				if len(impersonateGroups) == 0 {
					slog.Warn("AccessTokenInjector: M2M token carries no allow-listed group, rejecting",
						"issuer", userInfo.Issuer, "subject", userInfo.ID)
					http.Error(w, "forbidden: no permitted group in token", http.StatusForbidden)
					return
				}
			} else {
				impersonateGroups = userInfo.Groups
			}
			// system:authenticated is not added automatically for impersonation (unlike
			// real auth); without it the impersonated identity lacks system:discovery
			// access and API resource enumeration silently returns empty results.
			impersonateGroups = appendIfMissing(impersonateGroups, "system:authenticated")

			identity := k8s.ImpersonationIdentity{
				UserName: userInfo.ID,
				Groups:   impersonateGroups,
				Extra: map[string][]string{
					"issuer": {userInfo.Issuer},
					"agent":  {"mcp-kubernetes"},
				},
				AllowedTargetClusters: tiConfig.AllowedTargetClusters,
			}
			slog.Debug("AccessTokenInjector: M2M impersonation",
				"issuer", userInfo.Issuer, "subject", userInfo.ID, "groups", impersonateGroups)

			ctx = ContextWithImpersonationIdentity(ctx, identity)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
			return
		}

		// SSO Token Forwarding: If the token was validated via TrustedAudiences/JWKS,
		// the Bearer token IS the ID token - use it directly instead of looking it up.
		// This happens when an upstream aggregator (e.g., muster) forwards a user's ID token.
		if userInfo.IsSSO() {
			slog.Debug("AccessTokenInjector: using SSO-forwarded token directly as ID token",
				logging.UserHash(userInfo.Email))
			ctx = mcpoauth.ContextWithIDToken(ctx, bearerToken)
			r = r.WithContext(ctx)
			recordMetric(ctx, "sso_success")
			next.ServeHTTP(w, r)
			return
		}

		// Normal OAuth flow: Look up the provider's ID token from the token store
		// Safety check: tokenStore should always be set in production, but handle nil defensively
		if s.tokenStore == nil {
			slog.Debug("AccessTokenInjector: tokenStore is nil, cannot look up provider token",
				logging.UserHash(userInfo.Email))
			recordMetric(ctx, "no_store")
			next.ServeHTTP(w, r)
			return
		}

		slog.Debug("AccessTokenInjector: looking up provider token by MCP access token",
			logging.UserHash(userInfo.Email))

		// Retrieve the provider token using the MCP access token (Bearer token) as the key
		// This is the same key used by mcp-oauth during token exchange and proactive refresh
		token, err := s.tokenStore.GetToken(ctx, bearerToken)
		if err != nil {
			slog.Debug("AccessTokenInjector: failed to get token from store by access token", //nolint:gosec // G706: email is hashed, err is internal
				logging.UserHash(userInfo.Email), logging.Err(err))
			// Fallback to email-based lookup for backwards compatibility
			token, err = s.tokenStore.GetToken(ctx, userInfo.Email)
			if err != nil {
				slog.Debug("AccessTokenInjector: fallback email lookup also failed",
					logging.UserHash(userInfo.Email), logging.Err(err))
				recordMetric(ctx, "lookup_failed")
				next.ServeHTTP(w, r)
				return
			}
			slog.Debug("AccessTokenInjector: using fallback email-based token lookup",
				logging.UserHash(userInfo.Email))
		}
		if token == nil {
			slog.Debug("AccessTokenInjector: token is nil for user",
				logging.UserHash(userInfo.Email))
			recordMetric(ctx, "lookup_failed")
			next.ServeHTTP(w, r)
			return
		}

		// Extract the ID token for Kubernetes OIDC authentication
		// Kubernetes OIDC validates the ID token, not the access token
		idToken := mcpoauth.GetIDToken(token)
		if idToken == "" {
			slog.Debug("AccessTokenInjector: no ID token in stored token", //nolint:gosec // G706: email is hashed, other values are bools
				logging.UserHash(userInfo.Email),
				slog.Bool("has_access_token", token.AccessToken != ""),
				slog.Bool("has_refresh_token", token.RefreshToken != ""))
			slog.Debug("AccessTokenInjector: calling next handler without ID token")
			recordMetric(ctx, "no_id_token")
			next.ServeHTTP(w, r)
			slog.Debug("AccessTokenInjector: next handler returned (no ID token path)")
			return
		}

		slog.Debug("AccessTokenInjector: successfully injected ID token",
			logging.UserHash(userInfo.Email))
		ctx = mcpoauth.ContextWithIDToken(ctx, idToken)
		r = r.WithContext(ctx)
		recordMetric(ctx, "oauth_success")

		slog.Debug("AccessTokenInjector: calling next handler (mcp-go)")
		next.ServeHTTP(w, r)
		slog.Debug("AccessTokenInjector: next handler returned")
	})
}

// createHTTPContextFunc creates an HTTPContextFunc that copies the ID token
// from the HTTP request context (set by our middleware) to mcp-go's internal context.
// This is necessary because mcp-go creates its own context for tool execution and
// doesn't automatically inherit values from the HTTP request context.
func (s *OAuthHTTPServer) createHTTPContextFunc() mcpserver.HTTPContextFunc {
	return func(ctx context.Context, r *http.Request) context.Context {
		// The ID token was set in r.Context() by createAccessTokenInjectorMiddleware
		// We need to copy it to mcp-go's context for tool handlers to access it
		idToken, ok := mcpoauth.GetIDTokenFromContext(r.Context())
		if ok && idToken != "" {
			slog.Debug("HTTPContextFunc: propagating ID token to mcp-go context")
			return mcpoauth.ContextWithIDToken(ctx, idToken)
		}
		slog.Debug("HTTPContextFunc: no ID token in request context to propagate")
		return ctx
	}
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
