package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
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
		log.Printf("Warning: invalid duration for %s=%q: %v", envName, value, err)
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
		log.Printf("Warning: invalid integer for %s=%q: %v", envName, value, err)
		return 0, false
	}
	return n, true
}

// parseFloat32Env parses a float32 from an environment variable value.
// Returns the parsed float and true if successful, or zero and false if parsing fails.
// Logs a warning if the value is present but invalid.
func parseFloat32Env(value, envName string) (float32, bool) {
	if value == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(value, 32)
	if err != nil {
		log.Printf("Warning: invalid float for %s=%q: %v", envName, value, err)
		return 0, false
	}
	return float32(f), true
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

		// OAuth options
		enableOAuth                   bool
		oauthBaseURL                  string
		oauthProvider                 string
		googleClientID                string
		googleClientSecret            string
		dexIssuerURL                  string
		dexClientID                   string
		dexClientSecret               string
		dexConnectorID                string
		disableStreaming              bool
		registrationToken             string
		allowPublicRegistration       bool
		allowInsecureAuthWithoutState bool
		allowPrivateOAuthURLs         bool
		maxClientsPerIP               int
		oauthEncryptionKey            string
		downstreamOAuth               bool

		// OAuth storage options
		oauthStorageType string
		valkeyURL        string
		valkeyPassword   string
		valkeyTLS        bool
		valkeyKeyPrefix  string
		valkeyDB         int
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

			// Security warning: CLI password flags may be visible in process listings
			if cmd.Flags().Changed("valkey-password") {
				log.Printf("WARNING: Valkey password provided via CLI flag - password may be visible in process listings (ps aux)")
				log.Printf("         For better security, use the VALKEY_PASSWORD environment variable instead")
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
					Enabled:                       enableOAuth,
					BaseURL:                       oauthBaseURL,
					Provider:                      oauthProvider,
					GoogleClientID:                googleClientID,
					GoogleClientSecret:            googleClientSecret,
					DexIssuerURL:                  dexIssuerURL,
					DexClientID:                   dexClientID,
					DexClientSecret:               dexClientSecret,
					DexConnectorID:                dexConnectorID,
					DisableStreaming:              disableStreaming,
					RegistrationToken:             registrationToken,
					AllowPublicRegistration:       allowPublicRegistration,
					AllowInsecureAuthWithoutState: allowInsecureAuthWithoutState,
					AllowPrivateURLs:              allowPrivateOAuthURLs,
					MaxClientsPerIP:               maxClientsPerIP,
					EncryptionKey:                 oauthEncryptionKey,
					Storage:                       storageConfig,
				},
				DownstreamOAuth: downstreamOAuth,
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
	cmd.Flags().BoolVar(&disableStreaming, "disable-streaming", false, "Disable streaming for streamable-http transport")
	cmd.Flags().StringVar(&registrationToken, "registration-token", "", "OAuth client registration access token (required if public registration is disabled)")
	cmd.Flags().BoolVar(&allowPublicRegistration, "allow-public-registration", false, "Allow unauthenticated OAuth client registration (NOT RECOMMENDED for production)")
	cmd.Flags().BoolVar(&allowInsecureAuthWithoutState, "allow-insecure-auth-without-state", false, "Allow authorization requests without state parameter (for older MCP client compatibility)")
	cmd.Flags().BoolVar(&allowPrivateOAuthURLs, "allow-private-oauth-urls", false, "Allow OAuth URLs that resolve to private/internal IP addresses (for internal deployments)")
	cmd.Flags().IntVar(&maxClientsPerIP, "max-clients-per-ip", 10, "Maximum number of OAuth clients that can be registered per IP address")
	cmd.Flags().StringVar(&oauthEncryptionKey, "oauth-encryption-key", "", "AES-256 encryption key for token encryption (32 bytes, can also be set via OAUTH_ENCRYPTION_KEY env var)")
	cmd.Flags().BoolVar(&downstreamOAuth, "downstream-oauth", false, "Use OAuth access tokens for downstream Kubernetes API authentication (requires --enable-oauth and --in-cluster)")

	// OAuth storage flags
	cmd.Flags().StringVar(&oauthStorageType, "oauth-storage-type", "memory", "OAuth token storage type: memory or valkey (can also be set via OAUTH_STORAGE_TYPE env var)")
	cmd.Flags().StringVar(&valkeyURL, "valkey-url", "", "Valkey server address (e.g., valkey.namespace.svc:6379, can also be set via VALKEY_URL env var)")
	cmd.Flags().StringVar(&valkeyPassword, "valkey-password", "", "Valkey authentication password (can also be set via VALKEY_PASSWORD env var)")
	cmd.Flags().BoolVar(&valkeyTLS, "valkey-tls", false, "Enable TLS for Valkey connections (can also be set via VALKEY_TLS_ENABLED env var)")
	cmd.Flags().StringVar(&valkeyKeyPrefix, "valkey-key-prefix", "mcp:", "Prefix for all Valkey keys (can also be set via VALKEY_KEY_PREFIX env var)")
	cmd.Flags().IntVar(&valkeyDB, "valkey-db", 0, "Valkey database number (can also be set via VALKEY_DB env var)")

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
	// Create Kubernetes client configuration
	var k8sLogger = &simpleLogger{}

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
				log.Printf("Error during instrumentation shutdown: %v", shutdownErr)
			}
		}
	}()

	if instrumentationProvider.Enabled() {
		log.Printf("OpenTelemetry instrumentation enabled (metrics: %s, tracing: %s)",
			instrumentationConfig.MetricsExporter, instrumentationConfig.TracingExporter)
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

		log.Printf("Downstream OAuth enabled: requests without valid OAuth tokens will fail with authentication error")
	}

	// Load CAPI mode configuration from environment variables
	loadCAPIModeConfig(&config.CAPIMode)

	// Create federation manager if CAPI mode is enabled
	var fedManager federation.ClusterClientManager
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

		// Build federation manager options
		var managerOpts []federation.ManagerOption

		// Configure cache
		if config.CAPIMode.CacheTTL != "" {
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
					log.Printf("Warning: invalid OAUTH_TOKEN_LIFETIME=%q, using default %v: %v",
						config.CAPIMode.OAuthTokenLifetime, defaultOAuthTokenLifetime, err)
				}
			}

			// Security warning: Cache TTL exceeding OAuth token lifetime
			// could lead to using expired tokens for cached clients.
			if ttl > tokenLifetime {
				log.Printf("Warning: Cache TTL (%v) exceeds OAuth token lifetime (%v). "+
					"This may cause authentication failures when cached clients use expired tokens. "+
					"Consider setting CLIENT_CACHE_TTL <= %v or configuring longer token lifetimes in your OAuth provider "+
					"(set OAUTH_TOKEN_LIFETIME to customize this threshold).",
					ttl, tokenLifetime, tokenLifetime)
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
		}

		// Create federation manager
		fedManager, err = federation.NewManager(oauthProvider, managerOpts...)
		if err != nil {
			return fmt.Errorf("failed to create federation manager: %w", err)
		}

		serverContextOptions = append(serverContextOptions, server.WithFederationManager(fedManager))

		log.Printf("CAPI federation mode enabled: multi-cluster operations available")
	}

	serverContext, err := server.NewServerContext(shutdownCtx, serverContextOptions...)
	if err != nil {
		return fmt.Errorf("failed to create server context: %w", err)
	}
	defer func() {
		if err := serverContext.Shutdown(); err != nil {
			// Only log shutdown errors for non-stdio transports to avoid output interference
			if config.Transport != transportStdio {
				log.Printf("Error during server context shutdown: %v", err)
			}
		}
		// Close federation manager if it was created
		if fedManager != nil {
			if err := fedManager.Close(); err != nil {
				if config.Transport != transportStdio {
					log.Printf("Error during federation manager shutdown: %v", err)
				}
			}
		}
	}()

	// Create MCP server
	mcpSrv := mcpserver.NewMCPServer("mcp-kubernetes", rootCmd.Version,
		mcpserver.WithToolCapabilities(true),
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
		fmt.Printf("Starting MCP Kubernetes server with %s transport...\n", config.Transport)
		return runSSEServer(mcpSrv, config.HTTPAddr, config.SSEEndpoint, config.MessageEndpoint, shutdownCtx, config.DebugMode, instrumentationProvider)
	case transportStreamableHTTP:
		fmt.Printf("Starting MCP Kubernetes server with %s transport...\n", config.Transport)
		if config.OAuth.Enabled {
			// Get OAuth credentials from env vars if not provided via flags
			loadEnvIfEmpty(&config.OAuth.GoogleClientID, "GOOGLE_CLIENT_ID")
			loadEnvIfEmpty(&config.OAuth.GoogleClientSecret, "GOOGLE_CLIENT_SECRET")
			loadEnvIfEmpty(&config.OAuth.DexIssuerURL, "DEX_ISSUER_URL")
			loadEnvIfEmpty(&config.OAuth.DexClientID, "DEX_CLIENT_ID")
			loadEnvIfEmpty(&config.OAuth.DexClientSecret, "DEX_CLIENT_SECRET")
			loadEnvIfEmpty(&config.OAuth.DexConnectorID, "DEX_CONNECTOR_ID")
			loadEnvIfEmpty(&config.OAuth.EncryptionKey, "OAUTH_ENCRYPTION_KEY")
			// Note: Valkey storage env vars are loaded in RunE closure where cmd is available

			// Validate OAuth configuration
			if config.OAuth.BaseURL == "" {
				return fmt.Errorf("--oauth-base-url is required when --enable-oauth is set")
			}
			// Validate OAuth base URL is HTTPS and not vulnerable to SSRF
			if err := validateSecureURL(config.OAuth.BaseURL, "OAuth base URL", config.OAuth.AllowPrivateURLs); err != nil {
				return err
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

			if !config.OAuth.AllowPublicRegistration && config.OAuth.RegistrationToken == "" {
				return fmt.Errorf("--registration-token is required when public registration is disabled")
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
				fmt.Println("OAuth: Token encryption at rest enabled (AES-256-GCM)")
			} else {
				fmt.Println("WARNING: OAuth encryption key not set - tokens will be stored unencrypted")
			}

			// Warn about insecure configuration options
			if config.OAuth.AllowPublicRegistration {
				fmt.Println("WARNING: Public client registration is enabled - this allows unlimited client registration and may lead to DoS")
				fmt.Println("         Recommended: Set --allow-public-registration=false and use --registration-token")
			}
			if config.OAuth.AllowInsecureAuthWithoutState {
				fmt.Println("WARNING: State parameter is optional - this weakens CSRF protection")
				fmt.Println("         Recommended: Set --allow-insecure-auth-without-state=false for production")
			}
			if config.DebugMode {
				fmt.Println("WARNING: Debug logging is enabled - this may log sensitive information")
				fmt.Println("         Recommended: Disable debug mode in production")
			}

			return runOAuthHTTPServer(mcpSrv, config.HTTPAddr, shutdownCtx, server.OAuthConfig{
				BaseURL:                       config.OAuth.BaseURL,
				Provider:                      config.OAuth.Provider,
				GoogleClientID:                config.OAuth.GoogleClientID,
				GoogleClientSecret:            config.OAuth.GoogleClientSecret,
				DexIssuerURL:                  config.OAuth.DexIssuerURL,
				DexClientID:                   config.OAuth.DexClientID,
				DexClientSecret:               config.OAuth.DexClientSecret,
				DexConnectorID:                config.OAuth.DexConnectorID,
				DisableStreaming:              config.OAuth.DisableStreaming,
				DebugMode:                     config.DebugMode,
				AllowPublicClientRegistration: config.OAuth.AllowPublicRegistration,
				RegistrationAccessToken:       config.OAuth.RegistrationToken,
				AllowInsecureAuthWithoutState: config.OAuth.AllowInsecureAuthWithoutState,
				MaxClientsPerIP:               config.OAuth.MaxClientsPerIP,
				EncryptionKey:                 encryptionKey,
				EnableHSTS:                    os.Getenv("ENABLE_HSTS") == envValueTrue,
				AllowedOrigins:                os.Getenv("ALLOWED_ORIGINS"),
				InstrumentationProvider:       instrumentationProvider,
				Storage:                       config.OAuth.Storage, // Same type, no conversion needed
			}, serverContext)
		}
		return runStreamableHTTPServer(mcpSrv, config.HTTPAddr, config.HTTPEndpoint, shutdownCtx, config.DebugMode, instrumentationProvider, serverContext)
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
