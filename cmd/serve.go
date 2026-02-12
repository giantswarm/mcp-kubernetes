package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/logging"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/capi"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/cluster"
	contexttools "github.com/giantswarm/mcp-kubernetes/internal/tools/context"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/pod"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource"
)

// Transport type constants for the MCP server.
const (
	transportStdio          = "stdio"
	transportSSE            = "sse"
	transportStreamableHTTP = "streamable-http"
)

// envValueTrue is the string value used to enable boolean environment variables.
const envValueTrue = "true"

// defaultOAuthTokenLifetime is a reasonable default assumption for OAuth token lifetime.
// Most OIDC providers issue tokens with 1-hour lifetime. This is used to warn operators
// if their cache TTL exceeds this value, which could lead to using expired tokens.
const defaultOAuthTokenLifetime = 1 * time.Hour

// parseDurationEnv parses a duration from an environment variable value.
// Returns the parsed duration and true if successful, or zero and false if parsing fails.
// Logs a warning if the value is present but invalid.
func parseDurationEnv(value, envName string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		slog.Warn("invalid duration for environment variable",
			"env", envName,
			"value", value,
			"error", err)
		return 0, false
	}
	return d, true
}

// parseIntEnv parses an integer from an environment variable value.
// Returns the parsed int and true if successful, or zero and false if parsing fails.
// Logs a warning if the value is present but invalid.
func parseIntEnv(value, envName string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		slog.Warn("invalid integer for environment variable",
			"env", envName,
			"value", value,
			"error", err)
		return 0, false
	}
	return n, true
}

// parseFloat64Env parses a float64 from an environment variable value.
// Returns the parsed float and true if successful, or zero and false if parsing fails.
// Logs a warning if the value is present but invalid.
func parseFloat64Env(value, envName string) (float64, bool) {
	if value == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		slog.Warn("invalid float for environment variable",
			"env", envName,
			"value", value,
			"error", err)
		return 0, false
	}
	return f, true
}

// parseFloat32Env parses a float32 from an environment variable value.
// Delegates to parseFloat64Env and casts the result.
func parseFloat32Env(value, envName string) (float32, bool) {
	f, ok := parseFloat64Env(value, envName)
	return float32(f), ok
}

// splitAndTrimAudiences splits a comma-separated string into a slice of trimmed audiences.
// Empty entries are filtered out. Returns nil if the result is empty.
// This is used to parse OAUTH_TRUSTED_AUDIENCES env var.
func splitAndTrimAudiences(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// newServeCmd creates the Cobra command for starting the MCP server.
func newServeCmd() *cobra.Command {
	var (
		nonDestructiveMode bool
		dryRun             bool
		qpsLimit           float32
		burstLimit         int
		debugMode          bool
		inCluster          bool

		// Transport options
		transport       string
		httpAddr        string
		sseEndpoint     string
		messageEndpoint string
		httpEndpoint    string

		// Metrics server options
		metricsEnabled bool
		metricsAddr    string

		// OAuth options
		enableOAuth                        bool
		oauthBaseURL                       string
		oauthProvider                      string
		googleClientID                     string
		googleClientSecret                 string
		dexIssuerURL                       string
		dexClientID                        string
		dexClientSecret                    string
		dexConnectorID                     string
		dexCAFile                          string
		dexKubernetesAuthenticatorClientID string
		disableStreaming                   bool
		registrationToken                  string
		allowPublicRegistration            bool
		allowInsecureAuthWithoutState      bool
		allowPrivateOAuthURLs              bool
		maxClientsPerIP                    int
		oauthEncryptionKey                 string
		downstreamOAuth                    bool
		tlsCertFile                        string
		tlsKeyFile                         string

		// OAuth storage options
		oauthStorageType string
		valkeyURL        string
		valkeyPassword   string
		valkeyTLS        bool
		valkeyKeyPrefix  string
		valkeyDB         int

		// Redirect URI security options (all default to secure values)
		disableProductionMode              bool
		allowLocalhostRedirectURIs         bool
		allowPrivateIPRedirectURIs         bool
		allowLinkLocalRedirectURIs         bool
		disableDNSValidation               bool
		disableDNSValidationStrict         bool
		disableAuthorizationTimeValidation bool

		// Trusted scheme registration for Cursor/VSCode
		trustedPublicRegistrationSchemes []string
		disableStrictSchemeMatching      bool

		// CIMD (Client ID Metadata Documents) - MCP 2025-11-25
		enableCIMD          bool
		cimdAllowPrivateIPs bool

		// Trusted audiences for SSO token forwarding
		trustedAudiences   []string
		ssoAllowPrivateIPs bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP Kubernetes server",
		Long: `Start the MCP Kubernetes server to provide tools for interacting
with Kubernetes clusters via the Model Context Protocol.

Supports multiple transport types:
  - stdio: Standard input/output (default)
  - sse: Server-Sent Events over HTTP
  - streamable-http: Streamable HTTP transport

Authentication modes:
  - Kubeconfig (default): Uses standard kubeconfig file authentication
  - In-cluster: Uses service account token when running inside a Kubernetes pod
  - OAuth (optional): Enable OAuth 2.1 authentication for HTTP transports

Downstream OAuth (--downstream-oauth):
  When enabled with --enable-oauth and --in-cluster, the server will use each user's
  OAuth access token to authenticate with the Kubernetes API instead of the service
  account token. This ensures users only have their configured RBAC permissions.
  Requires the Kubernetes cluster to be configured for OIDC authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load TLS paths from environment if not provided via flags
			loadEnvIfEmpty(&tlsCertFile, "TLS_CERT_FILE")
			loadEnvIfEmpty(&tlsKeyFile, "TLS_KEY_FILE")

			// Build OAuth storage config from flags
			storageConfig := server.OAuthStorageConfig{
				Type: server.OAuthStorageType(oauthStorageType),
				Valkey: server.ValkeyStorageConfig{
					URL:        valkeyURL,
					Password:   valkeyPassword,
					TLSEnabled: valkeyTLS,
					KeyPrefix:  valkeyKeyPrefix,
					DB:         valkeyDB,
				},
			}
			// Load env vars only for flags not explicitly set by user
			loadOAuthStorageEnvVars(cmd, &storageConfig)

			// CIMD env var - only apply if flag was not explicitly set
			if !cmd.Flags().Changed("enable-cimd") {
				if envVal := os.Getenv("ENABLE_CIMD"); envVal != "" {
					if parsed, err := strconv.ParseBool(envVal); err == nil {
						enableCIMD = parsed
					} else {
						slog.Warn("invalid ENABLE_CIMD value, using default",
							"value", envVal,
							"default", enableCIMD,
							"error", err)
					}
				}
			}

			// CIMD allow private IPs env var - only apply if flag was not explicitly set
			if !cmd.Flags().Changed("cimd-allow-private-ips") {
				if envVal := os.Getenv("CIMD_ALLOW_PRIVATE_IPS"); envVal != "" {
					if parsed, err := strconv.ParseBool(envVal); err == nil {
						cimdAllowPrivateIPs = parsed
					} else {
						slog.Warn("invalid CIMD_ALLOW_PRIVATE_IPS value, using default",
							"value", envVal,
							"default", cimdAllowPrivateIPs,
							"error", err)
					}
				}
			}

			// SSO allow private IPs env var - only apply if flag was not explicitly set
			if !cmd.Flags().Changed("sso-allow-private-ips") {
				if envVal := os.Getenv("SSO_ALLOW_PRIVATE_IPS"); envVal != "" {
					if parsed, err := strconv.ParseBool(envVal); err == nil {
						ssoAllowPrivateIPs = parsed
					} else {
						slog.Warn("invalid SSO_ALLOW_PRIVATE_IPS value, using default",
							"value", envVal,
							"default", ssoAllowPrivateIPs,
							"error", err)
					}
				}
			}

			// Security warning: CLI password flags may be visible in process listings
			if cmd.Flags().Changed("valkey-password") {
				slog.Warn("valkey password provided via CLI flag",
					"warning", "password may be visible in process listings (ps aux)",
					"recommendation", "use the VALKEY_PASSWORD environment variable instead")
			}

			config := ServeConfig{
				Transport:          transport,
				HTTPAddr:           httpAddr,
				SSEEndpoint:        sseEndpoint,
				MessageEndpoint:    messageEndpoint,
				HTTPEndpoint:       httpEndpoint,
				NonDestructiveMode: nonDestructiveMode,
				DryRun:             dryRun,
				QPSLimit:           qpsLimit,
				BurstLimit:         burstLimit,
				DebugMode:          debugMode,
				InCluster:          inCluster,
				OAuth: OAuthServeConfig{
					Enabled:                            enableOAuth,
					BaseURL:                            oauthBaseURL,
					Provider:                           oauthProvider,
					GoogleClientID:                     googleClientID,
					GoogleClientSecret:                 googleClientSecret,
					DexIssuerURL:                       dexIssuerURL,
					DexClientID:                        dexClientID,
					DexClientSecret:                    dexClientSecret,
					DexConnectorID:                     dexConnectorID,
					DexCAFile:                          dexCAFile,
					DexKubernetesAuthenticatorClientID: dexKubernetesAuthenticatorClientID,
					DisableStreaming:                   disableStreaming,
					RegistrationToken:                  registrationToken,
					AllowPublicRegistration:            allowPublicRegistration,
					AllowInsecureAuthWithoutState:      allowInsecureAuthWithoutState,
					AllowPrivateURLs:                   allowPrivateOAuthURLs,
					MaxClientsPerIP:                    maxClientsPerIP,
					EncryptionKey:                      oauthEncryptionKey,
					TLSCertFile:                        tlsCertFile,
					TLSKeyFile:                         tlsKeyFile,
					Storage:                            storageConfig,
					RedirectURISecurity: RedirectURISecurityConfig{
						DisableProductionMode:              disableProductionMode,
						AllowLocalhostRedirectURIs:         allowLocalhostRedirectURIs,
						AllowPrivateIPRedirectURIs:         allowPrivateIPRedirectURIs,
						AllowLinkLocalRedirectURIs:         allowLinkLocalRedirectURIs,
						DisableDNSValidation:               disableDNSValidation,
						DisableDNSValidationStrict:         disableDNSValidationStrict,
						DisableAuthorizationTimeValidation: disableAuthorizationTimeValidation,
					},
					TrustedPublicRegistrationSchemes: trustedPublicRegistrationSchemes,
					DisableStrictSchemeMatching:      disableStrictSchemeMatching,
					EnableCIMD:                       enableCIMD,
					CIMDAllowPrivateIPs:              cimdAllowPrivateIPs,
					TrustedAudiences:                 trustedAudiences,
					SSOAllowPrivateIPs:               ssoAllowPrivateIPs,
				},
				DownstreamOAuth: downstreamOAuth,
				Metrics: MetricsServeConfig{
					Enabled: metricsEnabled,
					Addr:    metricsAddr,
				},
			}
			return runServe(config)
		},
	}

	// Add flags for configuring the server
	cmd.Flags().BoolVar(&nonDestructiveMode, "non-destructive", true, "Enable non-destructive mode (default: true)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Enable dry run mode (default: false)")
	cmd.Flags().Float32Var(&qpsLimit, "qps-limit", 20.0, "QPS limit for Kubernetes API calls (default: 20.0)")
	cmd.Flags().IntVar(&burstLimit, "burst-limit", 30, "Burst limit for Kubernetes API calls (default: 30)")
	cmd.Flags().BoolVar(&debugMode, "debug", false, "Enable debug logging (default: false)")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "Use in-cluster authentication (service account token) instead of kubeconfig (default: false)")

	// Transport flags
	cmd.Flags().StringVar(&transport, "transport", transportStdio, "Transport type: stdio, sse, or streamable-http")
	cmd.Flags().StringVar(&httpAddr, "http-addr", ":8080", "HTTP server address (for sse and streamable-http transports)")
	cmd.Flags().StringVar(&sseEndpoint, "sse-endpoint", "/sse", "SSE endpoint path (for sse transport)")
	cmd.Flags().StringVar(&messageEndpoint, "message-endpoint", "/message", "Message endpoint path (for sse transport)")
	cmd.Flags().StringVar(&httpEndpoint, "http-endpoint", "/mcp", "HTTP endpoint path (for streamable-http transport)")

	// Metrics server flags
	cmd.Flags().BoolVar(&metricsEnabled, "metrics-enabled", true, "Enable dedicated metrics server (default: true)")
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":9090", "Metrics server address (default: :9090)")

	// OAuth flags
	cmd.Flags().BoolVar(&enableOAuth, "enable-oauth", false, "Enable OAuth 2.1 authentication (for HTTP transports)")
	cmd.Flags().StringVar(&oauthBaseURL, "oauth-base-url", "", "OAuth base URL (e.g., https://mcp.example.com)")
	cmd.Flags().StringVar(&oauthProvider, "oauth-provider", OAuthProviderDex, fmt.Sprintf("OAuth provider: %s or %s (default: %s)", OAuthProviderDex, OAuthProviderGoogle, OAuthProviderDex))
	cmd.Flags().StringVar(&googleClientID, "google-client-id", "", "Google OAuth Client ID (can also be set via GOOGLE_CLIENT_ID env var)")
	cmd.Flags().StringVar(&googleClientSecret, "google-client-secret", "", "Google OAuth Client Secret (can also be set via GOOGLE_CLIENT_SECRET env var)")
	cmd.Flags().StringVar(&dexIssuerURL, "dex-issuer-url", "", "Dex OIDC issuer URL (can also be set via DEX_ISSUER_URL env var)")
	cmd.Flags().StringVar(&dexClientID, "dex-client-id", "", "Dex OAuth Client ID (can also be set via DEX_CLIENT_ID env var)")
	cmd.Flags().StringVar(&dexClientSecret, "dex-client-secret", "", "Dex OAuth Client Secret (can also be set via DEX_CLIENT_SECRET env var)")
	cmd.Flags().StringVar(&dexConnectorID, "dex-connector-id", "", "Dex connector ID to bypass connector selection (optional, can also be set via DEX_CONNECTOR_ID env var)")
	cmd.Flags().StringVar(&dexCAFile, "dex-ca-file", "", "Path to CA certificate file for Dex TLS verification (optional, can also be set via DEX_CA_FILE env var)")
	cmd.Flags().StringVar(&dexKubernetesAuthenticatorClientID, "dex-k8s-authenticator-client-id", "", "Dex client ID for Kubernetes API authentication (enables cross-client audience, can also be set via DEX_K8S_AUTHENTICATOR_CLIENT_ID env var)")
	cmd.Flags().BoolVar(&disableStreaming, "disable-streaming", false, "Disable streaming for streamable-http transport")
	cmd.Flags().StringVar(&registrationToken, "registration-token", "", "OAuth client registration access token (required if public registration is disabled)")
	cmd.Flags().BoolVar(&allowPublicRegistration, "allow-public-registration", false, "Allow unauthenticated OAuth client registration (NOT RECOMMENDED for production)")
	cmd.Flags().BoolVar(&allowInsecureAuthWithoutState, "allow-insecure-auth-without-state", false, "Allow authorization requests without state parameter (for older MCP client compatibility)")
	cmd.Flags().BoolVar(&allowPrivateOAuthURLs, "allow-private-oauth-urls", false, "Allow OAuth URLs that resolve to private/internal IP addresses (for internal deployments)")
	cmd.Flags().IntVar(&maxClientsPerIP, "max-clients-per-ip", 10, "Maximum number of OAuth clients that can be registered per IP address")
	cmd.Flags().StringVar(&oauthEncryptionKey, "oauth-encryption-key", "", "AES-256 encryption key for token encryption (32 bytes, can also be set via OAUTH_ENCRYPTION_KEY env var)")
	cmd.Flags().BoolVar(&downstreamOAuth, "downstream-oauth", false, "Use OAuth access tokens for downstream Kubernetes API authentication (requires --enable-oauth and --in-cluster)")

	// TLS flags for HTTPS support
	cmd.Flags().StringVar(&tlsCertFile, "tls-cert-file", "", "Path to TLS certificate file (PEM format). If provided with --tls-key-file, enables HTTPS")
	cmd.Flags().StringVar(&tlsKeyFile, "tls-key-file", "", "Path to TLS private key file (PEM format). If provided with --tls-cert-file, enables HTTPS")

	// OAuth storage flags
	cmd.Flags().StringVar(&oauthStorageType, "oauth-storage-type", "memory", "OAuth token storage type: memory or valkey (can also be set via OAUTH_STORAGE_TYPE env var)")
	cmd.Flags().StringVar(&valkeyURL, "valkey-url", "", "Valkey server address (e.g., valkey.namespace.svc:6379, can also be set via VALKEY_URL env var)")
	cmd.Flags().StringVar(&valkeyPassword, "valkey-password", "", "Valkey authentication password (can also be set via VALKEY_PASSWORD env var)")
	cmd.Flags().BoolVar(&valkeyTLS, "valkey-tls", false, "Enable TLS for Valkey connections (can also be set via VALKEY_TLS_ENABLED env var)")
	cmd.Flags().StringVar(&valkeyKeyPrefix, "valkey-key-prefix", "mcp:", "Prefix for all Valkey keys (can also be set via VALKEY_KEY_PREFIX env var)")
	cmd.Flags().IntVar(&valkeyDB, "valkey-db", 0, "Valkey database number (can also be set via VALKEY_DB env var)")

	// Redirect URI security flags (all default to secure values)
	// Use --disable-* flags to explicitly opt-out of security features
	cmd.Flags().BoolVar(&disableProductionMode, "disable-production-mode", false, "Disable production mode security (allows HTTP, private IPs in redirect URIs). WARNING: Significantly weakens security")
	cmd.Flags().BoolVar(&allowLocalhostRedirectURIs, "allow-localhost-redirect-uris", false, "Allow http://localhost redirect URIs for native apps (RFC 8252)")
	cmd.Flags().BoolVar(&allowPrivateIPRedirectURIs, "allow-private-ip-redirect-uris", false, "Allow private IP addresses (10.x, 172.16.x, 192.168.x) in redirect URIs. WARNING: SSRF risk")
	cmd.Flags().BoolVar(&allowLinkLocalRedirectURIs, "allow-link-local-redirect-uris", false, "Allow link-local addresses (169.254.x.x) in redirect URIs. WARNING: Cloud metadata SSRF risk")
	cmd.Flags().BoolVar(&disableDNSValidation, "disable-dns-validation", false, "Disable DNS validation of redirect URI hostnames. WARNING: Allows DNS rebinding attacks")
	cmd.Flags().BoolVar(&disableDNSValidationStrict, "disable-dns-validation-strict", false, "Disable fail-closed DNS validation (allow registration on DNS failures). WARNING: Validation bypass risk")
	cmd.Flags().BoolVar(&disableAuthorizationTimeValidation, "disable-authorization-time-validation", false, "Disable redirect URI validation at authorization time. WARNING: Allows TOCTOU attacks")

	// Trusted scheme registration for Cursor/VSCode compatibility
	cmd.Flags().StringSliceVar(&trustedPublicRegistrationSchemes, "trusted-public-registration-schemes", nil, "URI schemes allowed for unauthenticated client registration (e.g., cursor,vscode). Best for internal/dev deployments. Must conform to RFC 3986 scheme syntax")
	cmd.Flags().BoolVar(&disableStrictSchemeMatching, "disable-strict-scheme-matching", false, "Allow mixed redirect URI schemes with trusted scheme registration. WARNING: Reduces security")

	// CIMD (Client ID Metadata Documents) - MCP 2025-11-25
	cmd.Flags().BoolVar(&enableCIMD, "enable-cimd", true, "Enable Client ID Metadata Documents (CIMD) per MCP 2025-11-25. Allows clients to use HTTPS URLs as client identifiers (can also be set via ENABLE_CIMD env var)")
	cmd.Flags().BoolVar(&cimdAllowPrivateIPs, "cimd-allow-private-ips", false, "Allow CIMD metadata URLs to resolve to private IPs (SSRF risk; internal deployments only)")

	// Trusted audiences for SSO token forwarding from aggregators
	cmd.Flags().StringSliceVar(&trustedAudiences, "oauth-trusted-audiences", nil, "Client IDs whose tokens are accepted for SSO (e.g., muster-client). Enables token forwarding from trusted aggregators (can also be set via OAUTH_TRUSTED_AUDIENCES env var as comma-separated list)")
	cmd.Flags().BoolVar(&ssoAllowPrivateIPs, "sso-allow-private-ips", false, "Allow JWKS endpoints (for SSO token validation) to resolve to private IPs. Required when your IdP (e.g., Dex) runs on an internal network. (SSRF risk; internal deployments only, can also be set via SSO_ALLOW_PRIVATE_IPS env var)")

	return cmd
}

// validateEncryptionKey validates an AES-256 encryption key for security weaknesses
func validateEncryptionKey(key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 bytes, got %d bytes", len(key))
	}

	// Check for all-zero key
	allZero := true
	for _, b := range key {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return fmt.Errorf("encryption key is all zeros - use a cryptographically secure random key (openssl rand -base64 32)")
	}

	// Check for repeated patterns (simple entropy check)
	// Count unique bytes - a good key should have high entropy
	uniqueBytes := make(map[byte]bool)
	for _, b := range key {
		uniqueBytes[b] = true
	}
	if len(uniqueBytes) < 16 {
		return fmt.Errorf("encryption key appears to have low entropy (only %d unique bytes) - use a cryptographically secure random key (openssl rand -base64 32)", len(uniqueBytes))
	}

	return nil
}

// runServe contains the main server logic with support for multiple transports
func runServe(config ServeConfig) error {
	// Configure default slog logger level based on debug mode
	// This ensures all slog.Debug() calls throughout the codebase respect the --debug flag
	if config.DebugMode {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	// Create Kubernetes client configuration with structured logging
	var k8sLogger = logging.NewSlogAdapter(slog.Default())

	k8sConfig := &k8s.ClientConfig{
		NonDestructiveMode: config.NonDestructiveMode,
		DryRun:             config.DryRun,
		QPSLimit:           config.QPSLimit,
		BurstLimit:         config.BurstLimit,
		Timeout:            30 * time.Second,
		DebugMode:          config.DebugMode,
		InCluster:          config.InCluster,
		Logger:             k8sLogger,
	}

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient(k8sConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Validate downstream OAuth configuration
	if config.DownstreamOAuth {
		if !config.OAuth.Enabled {
			return fmt.Errorf("--downstream-oauth requires --enable-oauth to be set")
		}
		if !config.InCluster {
			return fmt.Errorf("--downstream-oauth requires --in-cluster mode (must be running inside a Kubernetes cluster)")
		}
	}

	// Setup graceful shutdown - listen for both SIGINT and SIGTERM
	shutdownCtx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize OpenTelemetry instrumentation provider
	instrumentationConfig := instrumentation.DefaultConfig()
	instrumentationConfig.ServiceVersion = rootCmd.Version
	instrumentationProvider, err := instrumentation.NewProvider(shutdownCtx, instrumentationConfig)
	if err != nil {
		return fmt.Errorf("failed to create instrumentation provider: %w", err)
	}
	defer func() {
		if shutdownErr := instrumentationProvider.Shutdown(context.Background()); shutdownErr != nil {
			if config.Transport != transportStdio {
				slog.Error("error during instrumentation shutdown", "error", shutdownErr)
			}
		}
	}()

	if instrumentationProvider.Enabled() {
		slog.Info("opentelemetry instrumentation enabled",
			"metrics_exporter", instrumentationConfig.MetricsExporter,
			"tracing_exporter", instrumentationConfig.TracingExporter)
	}

	// Create server context with kubernetes client and shutdown context
	var serverContextOptions []server.Option
	serverContextOptions = append(serverContextOptions, server.WithK8sClient(k8sClient))
	serverContextOptions = append(serverContextOptions, server.WithInstrumentationProvider(instrumentationProvider))

	// Set in-cluster mode flag
	if config.InCluster {
		serverContextOptions = append(serverContextOptions, server.WithInCluster(true))
	}

	// Create client factory for downstream OAuth if enabled
	if config.DownstreamOAuth {
		clientFactory, err := k8s.NewBearerTokenClientFactory(k8sConfig)
		if err != nil {
			return fmt.Errorf("failed to create bearer token client factory: %w", err)
		}
		serverContextOptions = append(serverContextOptions, server.WithClientFactory(clientFactory))
		serverContextOptions = append(serverContextOptions, server.WithDownstreamOAuth(true))

		// Strict mode is always enabled - fail closed for security
		// This ensures requests without valid OAuth tokens fail with authentication errors
		// rather than silently falling back to service account (which could be a security risk)
		serverContextOptions = append(serverContextOptions, server.WithDownstreamOAuthStrict(true))

		slog.Info("downstream OAuth enabled: requests without valid tokens will fail",
			"strict_mode", true)
	}

	// Load CAPI mode configuration from environment variables
	loadCAPIModeConfig(&config.CAPIMode)

	// Create federation manager if CAPI mode is enabled
	var fedManager federation.ClusterClientManager
	var hybridProvider *federation.HybridOAuthClientProvider // Track for cleanup
	if config.CAPIMode.Enabled {
		// CAPI mode requires downstream OAuth and in-cluster mode
		if !config.DownstreamOAuth {
			return fmt.Errorf("CAPI mode requires downstream OAuth to be enabled (--downstream-oauth)")
		}
		if !config.InCluster {
			return fmt.Errorf("CAPI mode requires in-cluster mode (--in-cluster)")
		}

		// Create OAuth client provider
		oauthProvider, err := federation.NewOAuthClientProviderFromInCluster()
		if err != nil {
			return fmt.Errorf("failed to create OAuth client provider: %w", err)
		}

		// Set the token extractor to use the OAuth token from context
		oauthProvider.SetTokenExtractor(oauth.GetAccessTokenFromContext)

		// Set metrics recorder if instrumentation is enabled
		if instrumentationProvider.Enabled() {
			oauthProvider.SetMetrics(instrumentationProvider.Metrics())
		}

		// Create HybridOAuthClientProvider for split-credential model
		// This wraps the OAuth provider with ServiceAccount-based secret access
		hybridConfig := &federation.HybridOAuthClientProviderConfig{
			UserProvider:           oauthProvider,
			StrictPrivilegedAccess: config.CAPIMode.PrivilegedSecretAccess.Strict,
			RateLimitPerSecond:     config.CAPIMode.PrivilegedSecretAccess.RateLimitPerSecond,
			RateLimitBurst:         config.CAPIMode.PrivilegedSecretAccess.RateLimitBurst,
		}

		hybridProvider, err = federation.NewHybridOAuthClientProvider(hybridConfig)
		if err != nil {
			return fmt.Errorf("failed to create hybrid OAuth client provider: %w", err)
		}

		// Set privileged access metrics if instrumentation is enabled
		if instrumentationProvider.Enabled() {
			hybridProvider.SetPrivilegedAccessMetrics(instrumentationProvider.Metrics())
		}

		// Log the privileged access configuration (using provider's actual values after defaults applied)
		slog.Info("privileged secret access enabled (split-credential model)",
			"strict_mode", hybridProvider.StrictPrivilegedAccess(),
			"rate_limit_per_second", hybridProvider.RateLimitPerSecond(),
			"rate_limit_burst", hybridProvider.RateLimitBurst())

		// Build federation manager options
		var managerOpts []federation.ManagerOption

		// Configure workload cluster authentication mode
		wcAuthMode := config.CAPIMode.WorkloadClusterAuth.Mode
		if wcAuthMode == "" {
			wcAuthMode = string(federation.WorkloadClusterAuthModeImpersonation) // default
		}

		switch wcAuthMode {
		case string(federation.WorkloadClusterAuthModeImpersonation):
			managerOpts = append(managerOpts, federation.WithWorkloadClusterAuthMode(federation.WorkloadClusterAuthModeImpersonation))
			slog.Info("Workload cluster auth mode: impersonation",
				"description", "using admin credentials with user impersonation headers")

		case string(federation.WorkloadClusterAuthModeSSOPassthrough):
			// SSO passthrough requires OAuth downstream to be enabled
			// because the TokenExtractor needs the user's OAuth token from context
			if !config.DownstreamOAuth {
				return fmt.Errorf("SSO passthrough mode (WC_AUTH_MODE=sso-passthrough) requires downstream OAuth to be enabled (--downstream-oauth). " +
					"The SSO token must be available in the request context for forwarding to workload clusters")
			}
			managerOpts = append(managerOpts, federation.WithWorkloadClusterAuthMode(federation.WorkloadClusterAuthModeSSOPassthrough))

			// Configure SSO passthrough
			ssoConfig := federation.DefaultSSOPassthroughConfig()
			ssoConfig.TokenExtractor = oauth.GetAccessTokenFromContext

			// Use custom CA ConfigMap suffix if configured
			if config.CAPIMode.WorkloadClusterAuth.CAConfigMapSuffix != "" {
				ssoConfig.CAConfigMapSuffix = config.CAPIMode.WorkloadClusterAuth.CAConfigMapSuffix
			}

			managerOpts = append(managerOpts, federation.WithSSOPassthroughConfig(ssoConfig))

			// Log configuration including security-relevant options
			slog.Info("Workload cluster auth mode: sso-passthrough",
				"description", "forwarding user SSO token directly to WC API servers",
				"ca_configmap_suffix", ssoConfig.CAConfigMapSuffix,
				"disable_caching", config.CAPIMode.WorkloadClusterAuth.DisableCaching)

			if config.CAPIMode.WorkloadClusterAuth.DisableCaching {
				slog.Info("SSO passthrough caching disabled",
					"description", "each request creates a fresh client with current SSO token",
					"security_benefit", "tokens never cached, immediate revocation effect")
			}

		default:
			return fmt.Errorf("invalid workload cluster auth mode: %s (supported: impersonation, sso-passthrough)", wcAuthMode)
		}

		// Configure cache (skip if SSO passthrough with caching disabled)
		ssoPassthroughNoCaching := wcAuthMode == string(federation.WorkloadClusterAuthModeSSOPassthrough) &&
			config.CAPIMode.WorkloadClusterAuth.DisableCaching

		if ssoPassthroughNoCaching {
			// Security: In SSO passthrough mode with caching disabled,
			// each request creates a fresh client with the current SSO token.
			// This ensures token revocation takes effect immediately.
			slog.Debug("Client caching disabled for SSO passthrough mode")
		} else if config.CAPIMode.CacheTTL != "" {
			ttl, err := time.ParseDuration(config.CAPIMode.CacheTTL)
			if err != nil {
				return fmt.Errorf("invalid cache TTL: %w", err)
			}

			// Determine OAuth token lifetime for validation
			// Use configured value if available, otherwise use default
			tokenLifetime := defaultOAuthTokenLifetime
			if config.CAPIMode.OAuthTokenLifetime != "" {
				if parsed, err := time.ParseDuration(config.CAPIMode.OAuthTokenLifetime); err == nil {
					tokenLifetime = parsed
				} else {
					slog.Warn("invalid OAUTH_TOKEN_LIFETIME, using default",
						"value", config.CAPIMode.OAuthTokenLifetime,
						"default", defaultOAuthTokenLifetime,
						"error", err)
				}
			}

			// Security warning: Cache TTL exceeding OAuth token lifetime
			// could lead to using expired tokens for cached clients.
			if ttl > tokenLifetime {
				// Provide more specific guidance for SSO passthrough mode
				if wcAuthMode == string(federation.WorkloadClusterAuthModeSSOPassthrough) {
					slog.Warn("SECURITY: cache TTL exceeds OAuth token lifetime in SSO passthrough mode",
						"cache_ttl", ttl,
						"token_lifetime", tokenLifetime,
						"risk", "expired tokens may be used for cached clients until cache eviction",
						"recommendation", "set CLIENT_CACHE_TTL <= token_lifetime, or enable WC_DISABLE_CACHING=true for high-security deployments")
				} else {
					slog.Warn("cache TTL exceeds OAuth token lifetime",
						"cache_ttl", ttl,
						"token_lifetime", tokenLifetime,
						"recommendation", "set CLIENT_CACHE_TTL <= token_lifetime or configure longer token lifetimes in your OAuth provider")
				}
			}

			cacheConfig := federation.CacheConfig{
				TTL:        ttl,
				MaxEntries: config.CAPIMode.CacheMaxEntries,
			}
			if config.CAPIMode.CacheCleanupInterval != "" {
				interval, err := time.ParseDuration(config.CAPIMode.CacheCleanupInterval)
				if err != nil {
					return fmt.Errorf("invalid cache cleanup interval: %w", err)
				}
				cacheConfig.CleanupInterval = interval
			}
			managerOpts = append(managerOpts, federation.WithManagerCacheConfig(cacheConfig))
		}

		// Configure connectivity
		if config.CAPIMode.ConnectivityTimeout != "" {
			connectivityConfig := federation.DefaultConnectivityConfig()
			if timeout, ok := parseDurationEnv(config.CAPIMode.ConnectivityTimeout, "CONNECTIVITY_TIMEOUT"); ok {
				connectivityConfig.ConnectionTimeout = timeout
			}
			if config.CAPIMode.ConnectivityRetryAttempts > 0 {
				connectivityConfig.RetryAttempts = config.CAPIMode.ConnectivityRetryAttempts
			}
			if backoff, ok := parseDurationEnv(config.CAPIMode.ConnectivityRetryBackoff, "CONNECTIVITY_RETRY_BACKOFF"); ok {
				connectivityConfig.RetryBackoff = backoff
			}
			if reqTimeout, ok := parseDurationEnv(config.CAPIMode.ConnectivityRequestTimeout, "CONNECTIVITY_REQUEST_TIMEOUT"); ok {
				connectivityConfig.RequestTimeout = reqTimeout
			}
			if config.CAPIMode.ConnectivityQPS > 0 {
				connectivityConfig.QPS = config.CAPIMode.ConnectivityQPS
			}
			if config.CAPIMode.ConnectivityBurst > 0 {
				connectivityConfig.Burst = config.CAPIMode.ConnectivityBurst
			}
			managerOpts = append(managerOpts, federation.WithConnectivityConfig(connectivityConfig))
		}

		// Add instrumentation metrics if enabled
		if instrumentationProvider.Enabled() {
			managerOpts = append(managerOpts, federation.WithManagerCacheMetrics(instrumentationProvider.Metrics()))
			managerOpts = append(managerOpts, federation.WithAuthMetrics(instrumentationProvider.Metrics()))
		}

		// Create federation manager with the hybrid provider for split-credential model
		fedManager, err = federation.NewManager(hybridProvider, managerOpts...)
		if err != nil {
			return fmt.Errorf("failed to create federation manager: %w", err)
		}

		serverContextOptions = append(serverContextOptions, server.WithFederationManager(fedManager))

		slog.Info("CAPI federation mode enabled: multi-cluster operations available")
	}

	serverContext, err := server.NewServerContext(shutdownCtx, serverContextOptions...)
	if err != nil {
		return fmt.Errorf("failed to create server context: %w", err)
	}
	defer func() {
		if err := serverContext.Shutdown(); err != nil {
			// Only log shutdown errors for non-stdio transports to avoid output interference
			if config.Transport != transportStdio {
				slog.Error("error during server context shutdown", "error", err)
			}
		}
		// Close federation manager if it was created
		if fedManager != nil {
			if err := fedManager.Close(); err != nil {
				if config.Transport != transportStdio {
					slog.Error("error during federation manager shutdown", "error", err)
				}
			}
		}
		// Close hybrid provider (stops background goroutine)
		if hybridProvider != nil {
			hybridProvider.Close()
		}
	}()

	// Create MCP server
	hooks := &mcpserver.Hooks{}
	hooks.AddAfterInitialize(func(ctx context.Context, id any, msg *mcp.InitializeRequest, result *mcp.InitializeResult) {
		slog.Info("session initialized",
			"client_name", msg.Params.ClientInfo.Name,
			"client_version", msg.Params.ClientInfo.Version,
			"protocol_version", msg.Params.ProtocolVersion,
		)
	})

	mcpSrv := mcpserver.NewMCPServer("mcp-kubernetes", rootCmd.Version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithHooks(hooks),
	)

	// Register all tool categories
	if err := resource.RegisterResourceTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register resource tools: %w", err)
	}

	if err := pod.RegisterPodTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register pod tools: %w", err)
	}

	if err := contexttools.RegisterContextTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register context tools: %w", err)
	}

	if err := cluster.RegisterClusterTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register cluster tools: %w", err)
	}

	// Register CAPI discovery tools (only registers when federation is enabled)
	if err := capi.RegisterCAPITools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register CAPI tools: %w", err)
	}

	// Start the appropriate server based on transport type
	switch config.Transport {
	case transportStdio:
		// Don't print startup message for stdio mode as it interferes with MCP communication
		return runStdioServer(mcpSrv)
	case transportSSE:
		slog.Info("starting MCP Kubernetes server", "transport", config.Transport)
		return runSSEServer(mcpSrv, config.HTTPAddr, config.SSEEndpoint, config.MessageEndpoint, shutdownCtx, config.DebugMode, instrumentationProvider, config.Metrics)
	case transportStreamableHTTP:
		slog.Info("starting MCP Kubernetes server", "transport", config.Transport)
		if config.OAuth.Enabled {
			// Get OAuth credentials from env vars if not provided via flags
			loadEnvIfEmpty(&config.OAuth.GoogleClientID, "GOOGLE_CLIENT_ID")
			loadEnvIfEmpty(&config.OAuth.GoogleClientSecret, "GOOGLE_CLIENT_SECRET")
			loadEnvIfEmpty(&config.OAuth.DexIssuerURL, "DEX_ISSUER_URL")
			loadEnvIfEmpty(&config.OAuth.DexClientID, "DEX_CLIENT_ID")
			loadEnvIfEmpty(&config.OAuth.DexClientSecret, "DEX_CLIENT_SECRET")
			loadEnvIfEmpty(&config.OAuth.DexConnectorID, "DEX_CONNECTOR_ID")
			loadEnvIfEmpty(&config.OAuth.DexCAFile, "DEX_CA_FILE")
			loadEnvIfEmpty(&config.OAuth.DexKubernetesAuthenticatorClientID, "DEX_K8S_AUTHENTICATOR_CLIENT_ID")
			loadEnvIfEmpty(&config.OAuth.EncryptionKey, "OAUTH_ENCRYPTION_KEY")
			// Note: Valkey storage and CIMD env vars are loaded in RunE closure where cmd is available

			// Load trusted audiences from environment variable if not set via flag
			if len(config.OAuth.TrustedAudiences) == 0 {
				if envVal := os.Getenv("OAUTH_TRUSTED_AUDIENCES"); envVal != "" {
					config.OAuth.TrustedAudiences = splitAndTrimAudiences(envVal)
				}
			}

			// Validate OAuth configuration
			if config.OAuth.BaseURL == "" {
				return fmt.Errorf("--oauth-base-url is required when --enable-oauth is set")
			}
			// Validate OAuth base URL (allows localhost for development, but requires HTTPS for production)
			if err := validateOAuthBaseURL(config.OAuth.BaseURL); err != nil {
				return err
			}

			// Validate TLS configuration - both cert and key must be provided together
			if (config.OAuth.TLSCertFile != "" && config.OAuth.TLSKeyFile == "") ||
				(config.OAuth.TLSCertFile == "" && config.OAuth.TLSKeyFile != "") {
				return fmt.Errorf("both --tls-cert-file and --tls-key-file must be provided together for HTTPS")
			}

			// Provider-specific validation
			switch config.OAuth.Provider {
			case OAuthProviderDex:
				if config.OAuth.DexIssuerURL == "" {
					return fmt.Errorf("dex issuer URL is required when using Dex provider (--dex-issuer-url or DEX_ISSUER_URL)")
				}
				// Validate Dex issuer URL is HTTPS and not vulnerable to SSRF
				if err := validateSecureURL(config.OAuth.DexIssuerURL, "Dex issuer URL", config.OAuth.AllowPrivateURLs); err != nil {
					return err
				}
				if config.OAuth.DexClientID == "" {
					return fmt.Errorf("dex client ID is required when using Dex provider (--dex-client-id or DEX_CLIENT_ID)")
				}
				if config.OAuth.DexClientSecret == "" {
					return fmt.Errorf("dex client secret is required when using Dex provider (--dex-client-secret or DEX_CLIENT_SECRET)")
				}
				// Validate Kubernetes authenticator client ID format (if provided)
				if err := validateOAuthClientID(config.OAuth.DexKubernetesAuthenticatorClientID, "Dex Kubernetes authenticator client ID"); err != nil {
					return err
				}
				// Warn if Kubernetes authenticator client ID is set but downstream OAuth is not enabled
				if config.OAuth.DexKubernetesAuthenticatorClientID != "" && !config.DownstreamOAuth {
					slog.Warn("Dex Kubernetes authenticator client ID is configured but downstream OAuth is disabled; cross-client audience tokens will be requested but not used for Kubernetes API authentication",
						"hint", "enable --downstream-oauth to use OAuth tokens for Kubernetes API authentication")
				}
			case OAuthProviderGoogle:
				if config.OAuth.GoogleClientID == "" {
					return fmt.Errorf("google client ID is required when using Google provider (--google-client-id or GOOGLE_CLIENT_ID)")
				}
				if config.OAuth.GoogleClientSecret == "" {
					return fmt.Errorf("google client secret is required when using Google provider (--google-client-secret or GOOGLE_CLIENT_SECRET)")
				}
			default:
				return fmt.Errorf("unsupported OAuth provider: %s (supported: %s, %s)", config.OAuth.Provider, OAuthProviderDex, OAuthProviderGoogle)
			}

			// Validate trusted schemes if configured (RFC 3986 compliance)
			if err := validateTrustedSchemes(config.OAuth.TrustedPublicRegistrationSchemes); err != nil {
				return fmt.Errorf("invalid trusted public registration scheme: %w", err)
			}

			// Log security warning when SSO token forwarding is enabled
			// This is a security-sensitive configuration that operators should be aware of
			if len(config.OAuth.TrustedAudiences) > 0 {
				slog.Warn("SSO token forwarding enabled: tokens from trusted upstream clients will be accepted",
					"trusted_audiences", config.OAuth.TrustedAudiences,
					"security_note", "ensure these client IDs are from services you control and trust")

				// Additional info log when SSO private IPs are configured (mcp-oauth v0.2.40+)
				if config.OAuth.SSOAllowPrivateIPs {
					slog.Info("SSO private IP allowance configured",
						"sso_allow_private_ips", true,
						"note", "JWKS endpoints on private networks will be allowed for SSO token validation")
				}
			}

			// Registration token is required unless:
			// 1. Public registration is enabled (anyone can register), OR
			// 2. Trusted schemes are configured (Cursor/VSCode can register without token), OR
			// 3. CIMD is enabled (clients use HTTPS URLs as client IDs)
			hasTrustedSchemes := len(config.OAuth.TrustedPublicRegistrationSchemes) > 0
			cimdEnabled := config.OAuth.EnableCIMD
			if !config.OAuth.AllowPublicRegistration && config.OAuth.RegistrationToken == "" && !hasTrustedSchemes && !cimdEnabled {
				return fmt.Errorf("--registration-token is required when public registration is disabled, " +
					"no trusted schemes are configured, and CIMD is disabled. " +
					"Either set --registration-token, enable --allow-public-registration, " +
					"configure --trusted-public-registration-schemes, or enable --enable-cimd")
			}

			// Prepare encryption key if provided (must be base64 encoded)
			var encryptionKey []byte
			if config.OAuth.EncryptionKey != "" {
				// Decode from base64
				decoded, err := base64.StdEncoding.DecodeString(config.OAuth.EncryptionKey)
				if err != nil {
					return fmt.Errorf("OAuth encryption key must be base64 encoded (use: openssl rand -base64 32): %w", err)
				}

				// Validate key for security weaknesses
				if err := validateEncryptionKey(decoded); err != nil {
					return fmt.Errorf("OAuth encryption key validation failed: %w", err)
				}

				encryptionKey = decoded
				slog.Info("OAuth token encryption at rest enabled", "algorithm", "AES-256-GCM")
			} else {
				slog.Warn("OAuth encryption key not set - tokens will be stored unencrypted")
			}

			// Warn about insecure configuration options
			if config.OAuth.AllowPublicRegistration {
				slog.Warn("public client registration is enabled - this allows unlimited client registration and may lead to DoS",
					"recommendation", "set --allow-public-registration=false and use --registration-token")
			}
			if config.OAuth.AllowInsecureAuthWithoutState {
				slog.Warn("state parameter is optional - this weakens CSRF protection",
					"recommendation", "set --allow-insecure-auth-without-state=false for production")
			}
			if config.DebugMode {
				slog.Warn("debug logging is enabled - this may log sensitive information",
					"recommendation", "disable debug mode in production")
			}
			if config.OAuth.AllowPrivateURLs {
				slog.Warn("private URL validation disabled - OAuth URLs may resolve to internal IP addresses",
					"note", "this reduces SSRF protection, only use for internal/air-gapped deployments")
			}
			if config.OAuth.CIMDAllowPrivateIPs {
				slog.Warn("CIMD private IP allowlist enabled - CIMD metadata URLs may resolve to internal IP addresses",
					"note", "this reduces SSRF protection, only use for internal/air-gapped deployments")
			}

			return runOAuthHTTPServer(mcpSrv, config.HTTPAddr, shutdownCtx, server.OAuthConfig{
				ServiceVersion:                     rootCmd.Version,
				BaseURL:                            config.OAuth.BaseURL,
				Provider:                           config.OAuth.Provider,
				GoogleClientID:                     config.OAuth.GoogleClientID,
				GoogleClientSecret:                 config.OAuth.GoogleClientSecret,
				DexIssuerURL:                       config.OAuth.DexIssuerURL,
				DexClientID:                        config.OAuth.DexClientID,
				DexClientSecret:                    config.OAuth.DexClientSecret,
				DexConnectorID:                     config.OAuth.DexConnectorID,
				DexCAFile:                          config.OAuth.DexCAFile,
				DexKubernetesAuthenticatorClientID: config.OAuth.DexKubernetesAuthenticatorClientID,
				DisableStreaming:                   config.OAuth.DisableStreaming,
				DebugMode:                          config.DebugMode,
				AllowPublicClientRegistration:      config.OAuth.AllowPublicRegistration,
				RegistrationAccessToken:            config.OAuth.RegistrationToken,
				AllowInsecureAuthWithoutState:      config.OAuth.AllowInsecureAuthWithoutState,
				MaxClientsPerIP:                    config.OAuth.MaxClientsPerIP,
				EncryptionKey:                      encryptionKey,
				EnableHSTS:                         os.Getenv("ENABLE_HSTS") == envValueTrue,
				AllowedOrigins:                     os.Getenv("ALLOWED_ORIGINS"),
				TLSCertFile:                        config.OAuth.TLSCertFile,
				TLSKeyFile:                         config.OAuth.TLSKeyFile,
				InstrumentationProvider:            instrumentationProvider,
				Storage:                            config.OAuth.Storage,
				RedirectURISecurity:                config.OAuth.RedirectURISecurity,
				// Trusted scheme registration for Cursor/VSCode compatibility
				TrustedPublicRegistrationSchemes: config.OAuth.TrustedPublicRegistrationSchemes,
				DisableStrictSchemeMatching:      config.OAuth.DisableStrictSchemeMatching,
				// CIMD (Client ID Metadata Documents) per MCP 2025-11-25
				EnableCIMD:          config.OAuth.EnableCIMD,
				CIMDAllowPrivateIPs: config.OAuth.CIMDAllowPrivateIPs,
				// Trusted audiences for SSO token forwarding from upstream aggregators
				TrustedAudiences:   config.OAuth.TrustedAudiences,
				SSOAllowPrivateIPs: config.OAuth.SSOAllowPrivateIPs,
			}, serverContext, config.Metrics)
		}
		return runStreamableHTTPServer(mcpSrv, config.HTTPAddr, config.HTTPEndpoint, shutdownCtx, config.DebugMode, instrumentationProvider, serverContext, config.Metrics)
	default:
		return fmt.Errorf("unsupported transport type: %s (supported: stdio, sse, streamable-http)", config.Transport)
	}
}

// loadOAuthStorageEnvVars loads OAuth storage configuration from environment variables.
// Environment variables only override flag values when the flag was not explicitly set.
// The cmd parameter is used to check if flags were explicitly set by the user.
func loadOAuthStorageEnvVars(cmd *cobra.Command, config *server.OAuthStorageConfig) {
	// Storage type - env var only applies if flag was not explicitly set
	if !cmd.Flags().Changed("oauth-storage-type") {
		if storageType := os.Getenv("OAUTH_STORAGE_TYPE"); storageType != "" {
			config.Type = server.OAuthStorageType(storageType)
		}
	}

	// Valkey URL - env var only applies if flag was not explicitly set
	if !cmd.Flags().Changed("valkey-url") {
		loadEnvIfEmpty(&config.Valkey.URL, "VALKEY_URL")
	}

	// Valkey Password - env var only applies if flag was not explicitly set
	if !cmd.Flags().Changed("valkey-password") {
		loadEnvIfEmpty(&config.Valkey.Password, "VALKEY_PASSWORD")
	}

	// Valkey Key Prefix - env var only applies if flag was not explicitly set
	if !cmd.Flags().Changed("valkey-key-prefix") {
		loadEnvIfEmpty(&config.Valkey.KeyPrefix, "VALKEY_KEY_PREFIX")
	}

	// Valkey TLS - env var only applies if flag was not explicitly set
	// This properly handles the case where user explicitly sets --valkey-tls=false
	if !cmd.Flags().Changed("valkey-tls") {
		if os.Getenv("VALKEY_TLS_ENABLED") == envValueTrue {
			config.Valkey.TLSEnabled = true
		}
	}

	// Valkey DB - env var only applies if flag was not explicitly set
	// This properly handles the case where user explicitly sets --valkey-db=0
	if !cmd.Flags().Changed("valkey-db") {
		if db, ok := parseIntEnv(os.Getenv("VALKEY_DB"), "VALKEY_DB"); ok {
			config.Valkey.DB = db
		}
	}
}

// loadCAPIModeConfig loads CAPI mode configuration from environment variables.
// This matches the environment variables set by the Helm chart deployment.yaml.
// Invalid values are logged as warnings and ignored.
func loadCAPIModeConfig(config *CAPIModeConfig) {
	// Check if CAPI mode is enabled
	if os.Getenv("CAPI_MODE_ENABLED") == envValueTrue {
		config.Enabled = true
	}

	// Workload cluster authentication mode
	if mode := os.Getenv("WC_AUTH_MODE"); mode != "" {
		config.WorkloadClusterAuth.Mode = mode
	}
	if suffix := os.Getenv("WC_CA_CONFIGMAP_SUFFIX"); suffix != "" {
		config.WorkloadClusterAuth.CAConfigMapSuffix = suffix
	}
	if os.Getenv("WC_DISABLE_CACHING") == envValueTrue {
		config.WorkloadClusterAuth.DisableCaching = true
	}

	// Privileged secret access configuration (split-credential model)
	if os.Getenv("PRIVILEGED_SECRET_ACCESS_STRICT") == envValueTrue {
		config.PrivilegedSecretAccess.Strict = true
	}
	if f, ok := parseFloat64Env(os.Getenv("PRIVILEGED_SECRET_ACCESS_RATE_PER_SECOND"), "PRIVILEGED_SECRET_ACCESS_RATE_PER_SECOND"); ok {
		config.PrivilegedSecretAccess.RateLimitPerSecond = f
	}
	if n, ok := parseIntEnv(os.Getenv("PRIVILEGED_SECRET_ACCESS_RATE_BURST"), "PRIVILEGED_SECRET_ACCESS_RATE_BURST"); ok {
		config.PrivilegedSecretAccess.RateLimitBurst = n
	}

	// Cache configuration - store as strings for later validation
	if ttl := os.Getenv("CLIENT_CACHE_TTL"); ttl != "" {
		config.CacheTTL = ttl
	}
	if n, ok := parseIntEnv(os.Getenv("CLIENT_CACHE_MAX_ENTRIES"), "CLIENT_CACHE_MAX_ENTRIES"); ok {
		config.CacheMaxEntries = n
	}
	if interval := os.Getenv("CLIENT_CACHE_CLEANUP_INTERVAL"); interval != "" {
		config.CacheCleanupInterval = interval
	}

	// OAuth token lifetime for cache TTL validation
	// This helps operators avoid cache TTLs that exceed their token lifetime
	if lifetime := os.Getenv("OAUTH_TOKEN_LIFETIME"); lifetime != "" {
		config.OAuthTokenLifetime = lifetime
	}

	// Connectivity configuration - store as strings for later validation
	if timeout := os.Getenv("CONNECTIVITY_TIMEOUT"); timeout != "" {
		config.ConnectivityTimeout = timeout
	}
	if n, ok := parseIntEnv(os.Getenv("CONNECTIVITY_RETRY_ATTEMPTS"), "CONNECTIVITY_RETRY_ATTEMPTS"); ok {
		config.ConnectivityRetryAttempts = n
	}
	if backoff := os.Getenv("CONNECTIVITY_RETRY_BACKOFF"); backoff != "" {
		config.ConnectivityRetryBackoff = backoff
	}
	if reqTimeout := os.Getenv("CONNECTIVITY_REQUEST_TIMEOUT"); reqTimeout != "" {
		config.ConnectivityRequestTimeout = reqTimeout
	}
	if f, ok := parseFloat32Env(os.Getenv("CONNECTIVITY_QPS"), "CONNECTIVITY_QPS"); ok {
		config.ConnectivityQPS = f
	}
	if n, ok := parseIntEnv(os.Getenv("CONNECTIVITY_BURST"), "CONNECTIVITY_BURST"); ok {
		config.ConnectivityBurst = n
	}
}
