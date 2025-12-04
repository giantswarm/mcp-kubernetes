package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/cluster"
	contexttools "github.com/giantswarm/mcp-kubernetes/internal/tools/context"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/pod"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource"
)

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
		maxClientsPerIP               int
		oauthEncryptionKey            string
		downstreamOAuth               bool
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
					MaxClientsPerIP:               maxClientsPerIP,
					EncryptionKey:                 oauthEncryptionKey,
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
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport type: stdio, sse, or streamable-http")
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
	cmd.Flags().IntVar(&maxClientsPerIP, "max-clients-per-ip", 10, "Maximum number of OAuth clients that can be registered per IP address")
	cmd.Flags().StringVar(&oauthEncryptionKey, "oauth-encryption-key", "", "AES-256 encryption key for token encryption (32 bytes, can also be set via OAUTH_ENCRYPTION_KEY env var)")
	cmd.Flags().BoolVar(&downstreamOAuth, "downstream-oauth", false, "Use OAuth access tokens for downstream Kubernetes API authentication (requires --enable-oauth and --in-cluster)")

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
			if config.Transport != "stdio" {
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

	// Create client factory for downstream OAuth if enabled
	if config.DownstreamOAuth {
		clientFactory, err := k8s.NewBearerTokenClientFactory(k8sConfig)
		if err != nil {
			return fmt.Errorf("failed to create bearer token client factory: %w", err)
		}
		serverContextOptions = append(serverContextOptions, server.WithClientFactory(clientFactory))
		serverContextOptions = append(serverContextOptions, server.WithDownstreamOAuth(true))
		log.Printf("Downstream OAuth enabled: user OAuth tokens will be used for Kubernetes API authentication")
	}

	serverContext, err := server.NewServerContext(shutdownCtx, serverContextOptions...)
	if err != nil {
		return fmt.Errorf("failed to create server context: %w", err)
	}
	defer func() {
		if err := serverContext.Shutdown(); err != nil {
			// Only log shutdown errors for non-stdio transports to avoid output interference
			if config.Transport != "stdio" {
				log.Printf("Error during server context shutdown: %v", err)
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

	// Start the appropriate server based on transport type
	switch config.Transport {
	case "stdio":
		// Don't print startup message for stdio mode as it interferes with MCP communication
		return runStdioServer(mcpSrv)
	case "sse":
		fmt.Printf("Starting MCP Kubernetes server with %s transport...\n", config.Transport)
		return runSSEServer(mcpSrv, config.HTTPAddr, config.SSEEndpoint, config.MessageEndpoint, shutdownCtx, config.DebugMode)
	case "streamable-http":
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

			// Validate OAuth configuration
			if config.OAuth.BaseURL == "" {
				return fmt.Errorf("--oauth-base-url is required when --enable-oauth is set")
			}
			// Validate OAuth base URL is HTTPS and not vulnerable to SSRF
			if err := validateSecureURL(config.OAuth.BaseURL, "OAuth base URL"); err != nil {
				return err
			}

			// Provider-specific validation
			switch config.OAuth.Provider {
			case OAuthProviderDex:
				if config.OAuth.DexIssuerURL == "" {
					return fmt.Errorf("dex issuer URL is required when using Dex provider (--dex-issuer-url or DEX_ISSUER_URL)")
				}
				// Validate Dex issuer URL is HTTPS and not vulnerable to SSRF
				if err := validateSecureURL(config.OAuth.DexIssuerURL, "Dex issuer URL"); err != nil {
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
				EnableHSTS:                    os.Getenv("ENABLE_HSTS") == "true",
				AllowedOrigins:                os.Getenv("ALLOWED_ORIGINS"),
				InstrumentationProvider:       instrumentationProvider,
			})
		}
		return runStreamableHTTPServer(mcpSrv, config.HTTPAddr, config.HTTPEndpoint, shutdownCtx, config.DebugMode)
	default:
		return fmt.Errorf("unsupported transport type: %s (supported: stdio, sse, streamable-http)", config.Transport)
	}
}
