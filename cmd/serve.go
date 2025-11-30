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

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/cluster"
	contexttools "github.com/giantswarm/mcp-kubernetes/internal/tools/context"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/pod"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource"
)

// simpleLogger provides basic logging for the Kubernetes client
type simpleLogger struct{}

func (l *simpleLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[DEBUG] %s %v", msg, args)
}

func (l *simpleLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] %s %v", msg, args)
}

func (l *simpleLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[WARN] %s %v", msg, args)
}

func (l *simpleLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] %s %v", msg, args)
}

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
}

// OAuthServeConfig holds OAuth-specific configuration.
type OAuthServeConfig struct {
	Enabled                       bool
	BaseURL                       string
	GoogleClientID                string
	GoogleClientSecret            string
	DisableStreaming              bool
	RegistrationToken             string
	AllowPublicRegistration       bool
	AllowInsecureAuthWithoutState bool
	MaxClientsPerIP               int
	EncryptionKey                 string
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
		googleClientID                string
		googleClientSecret            string
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
					GoogleClientID:                googleClientID,
					GoogleClientSecret:            googleClientSecret,
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
	cmd.Flags().StringVar(&googleClientID, "google-client-id", "", "Google OAuth Client ID (can also be set via GOOGLE_CLIENT_ID env var)")
	cmd.Flags().StringVar(&googleClientSecret, "google-client-secret", "", "Google OAuth Client Secret (can also be set via GOOGLE_CLIENT_SECRET env var)")
	cmd.Flags().BoolVar(&disableStreaming, "disable-streaming", false, "Disable streaming for streamable-http transport")
	cmd.Flags().StringVar(&registrationToken, "registration-token", "", "OAuth client registration access token (required if public registration is disabled)")
	cmd.Flags().BoolVar(&allowPublicRegistration, "allow-public-registration", false, "Allow unauthenticated OAuth client registration (NOT RECOMMENDED for production)")
	cmd.Flags().BoolVar(&allowInsecureAuthWithoutState, "allow-insecure-auth-without-state", false, "Allow authorization requests without state parameter (for older MCP client compatibility)")
	cmd.Flags().IntVar(&maxClientsPerIP, "max-clients-per-ip", 10, "Maximum number of OAuth clients that can be registered per IP address")
	cmd.Flags().StringVar(&oauthEncryptionKey, "oauth-encryption-key", "", "AES-256 encryption key for token encryption (32 bytes, can also be set via OAUTH_ENCRYPTION_KEY env var)")
	cmd.Flags().BoolVar(&downstreamOAuth, "downstream-oauth", false, "Use OAuth access tokens for downstream Kubernetes API authentication (requires --enable-oauth and --in-cluster)")

	return cmd
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

	// Create server context with kubernetes client and shutdown context
	var serverContextOptions []server.Option
	serverContextOptions = append(serverContextOptions, server.WithK8sClient(k8sClient))

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
			if config.OAuth.GoogleClientID == "" {
				config.OAuth.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
			}
			if config.OAuth.GoogleClientSecret == "" {
				config.OAuth.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
			}
			if config.OAuth.EncryptionKey == "" {
				config.OAuth.EncryptionKey = os.Getenv("OAUTH_ENCRYPTION_KEY")
			}

			// Validate OAuth configuration
			if config.OAuth.BaseURL == "" {
				return fmt.Errorf("--oauth-base-url is required when --enable-oauth is set")
			}
			if config.OAuth.GoogleClientID == "" {
				return fmt.Errorf("Google Client ID is required (use --google-client-id or GOOGLE_CLIENT_ID env var)")
			}
			if config.OAuth.GoogleClientSecret == "" {
				return fmt.Errorf("Google Client Secret is required (use --google-client-secret or GOOGLE_CLIENT_SECRET env var)")
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
				if len(decoded) != 32 {
					return fmt.Errorf("OAuth encryption key must be exactly 32 bytes after base64 decoding, got %d bytes (use: openssl rand -base64 32)", len(decoded))
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
				GoogleClientID:                config.OAuth.GoogleClientID,
				GoogleClientSecret:            config.OAuth.GoogleClientSecret,
				DisableStreaming:              config.OAuth.DisableStreaming,
				DebugMode:                     config.DebugMode,
				AllowPublicClientRegistration: config.OAuth.AllowPublicRegistration,
				RegistrationAccessToken:       config.OAuth.RegistrationToken,
				AllowInsecureAuthWithoutState: config.OAuth.AllowInsecureAuthWithoutState,
				MaxClientsPerIP:               config.OAuth.MaxClientsPerIP,
				EncryptionKey:                 encryptionKey,
				EnableHSTS:                    os.Getenv("ENABLE_HSTS") == "true",
				AllowedOrigins:                os.Getenv("ALLOWED_ORIGINS"),
			})
		}
		return runStreamableHTTPServer(mcpSrv, config.HTTPAddr, config.HTTPEndpoint, shutdownCtx, config.DebugMode)
	default:
		return fmt.Errorf("unsupported transport type: %s (supported: stdio, sse, streamable-http)", config.Transport)
	}
}

// runStdioServer runs the server with STDIO transport
func runStdioServer(mcpSrv *mcpserver.MCPServer) error {
	// Start the server in a goroutine so we can handle shutdown signals
	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if err := mcpserver.ServeStdio(mcpSrv); err != nil {
			serverDone <- err
		}
	}()

	// Wait for server completion
	err := <-serverDone
	if err != nil {
		return fmt.Errorf("server stopped with error: %w", err)
	}

	// Don't print to stdout in stdio mode as it interferes with MCP communication
	return nil
}

// runSSEServer runs the server with SSE transport
func runSSEServer(mcpSrv *mcpserver.MCPServer, addr, sseEndpoint, messageEndpoint string, ctx context.Context, debugMode bool) error {
	if debugMode {
		log.Printf("[DEBUG] Initializing SSE server with configuration:")
		log.Printf("[DEBUG]   Address: %s", addr)
		log.Printf("[DEBUG]   SSE Endpoint: %s", sseEndpoint)
		log.Printf("[DEBUG]   Message Endpoint: %s", messageEndpoint)
	}

	// Create SSE server with custom endpoints
	sseServer := mcpserver.NewSSEServer(mcpSrv,
		mcpserver.WithSSEEndpoint(sseEndpoint),
		mcpserver.WithMessageEndpoint(messageEndpoint),
	)

	if debugMode {
		log.Printf("[DEBUG] SSE server instance created successfully")
	}

	fmt.Printf("SSE server starting on %s\n", addr)
	fmt.Printf("  SSE endpoint: %s\n", sseEndpoint)
	fmt.Printf("  Message endpoint: %s\n", messageEndpoint)

	// Start server in goroutine
	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if debugMode {
			log.Printf("[DEBUG] Starting SSE server listener on %s", addr)
		}
		if err := sseServer.Start(addr); err != nil {
			if debugMode {
				log.Printf("[DEBUG] SSE server start failed: %v", err)
			}
			serverDone <- err
		} else {
			if debugMode {
				log.Printf("[DEBUG] SSE server listener stopped cleanly")
			}
		}
	}()

	if debugMode {
		log.Printf("[DEBUG] SSE server goroutine started, waiting for shutdown signal or server completion")
	}

	// Wait for either shutdown signal or server completion
	select {
	case <-ctx.Done():
		if debugMode {
			log.Printf("[DEBUG] Shutdown signal received, initiating SSE server shutdown")
		}
		fmt.Println("Shutdown signal received, stopping SSE server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30)
		defer cancel()
		if debugMode {
			log.Printf("[DEBUG] Starting graceful shutdown with 30s timeout")
		}
		if err := sseServer.Shutdown(shutdownCtx); err != nil {
			if debugMode {
				log.Printf("[DEBUG] Error during SSE server shutdown: %v", err)
			}
			return fmt.Errorf("error shutting down SSE server: %w", err)
		}
		if debugMode {
			log.Printf("[DEBUG] SSE server shutdown completed successfully")
		}
	case err := <-serverDone:
		if err != nil {
			if debugMode {
				log.Printf("[DEBUG] SSE server stopped with error: %v", err)
			}
			return fmt.Errorf("SSE server stopped with error: %w", err)
		} else {
			if debugMode {
				log.Printf("[DEBUG] SSE server stopped normally")
			}
			fmt.Println("SSE server stopped normally")
		}
	}

	fmt.Println("SSE server gracefully stopped")
	if debugMode {
		log.Printf("[DEBUG] SSE server shutdown sequence completed")
	}
	return nil
}

// runStreamableHTTPServer runs the server with Streamable HTTP transport
func runStreamableHTTPServer(mcpSrv *mcpserver.MCPServer, addr, endpoint string, ctx context.Context, debugMode bool) error {
	// Create Streamable HTTP server with custom endpoint
	httpServer := mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithEndpointPath(endpoint),
	)

	fmt.Printf("Streamable HTTP server starting on %s\n", addr)
	fmt.Printf("  HTTP endpoint: %s\n", endpoint)

	// Start server in goroutine
	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if err := httpServer.Start(addr); err != nil {
			serverDone <- err
		}
	}()

	// Wait for either shutdown signal or server completion
	select {
	case <-ctx.Done():
		fmt.Println("Shutdown signal received, stopping HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error shutting down HTTP server: %w", err)
		}
	case err := <-serverDone:
		if err != nil {
			return fmt.Errorf("HTTP server stopped with error: %w", err)
		} else {
			fmt.Println("HTTP server stopped normally")
		}
	}

	fmt.Println("HTTP server gracefully stopped")
	return nil
}

// runOAuthHTTPServer runs the server with OAuth 2.1 authentication
func runOAuthHTTPServer(mcpSrv *mcpserver.MCPServer, addr string, ctx context.Context, config server.OAuthConfig) error {
	// Create OAuth HTTP server
	oauthServer, err := server.NewOAuthHTTPServer(mcpSrv, "streamable-http", config)
	if err != nil {
		return fmt.Errorf("failed to create OAuth HTTP server: %w", err)
	}

	fmt.Printf("OAuth-enabled HTTP server starting on %s\n", addr)
	fmt.Printf("  Base URL: %s\n", config.BaseURL)
	fmt.Printf("  MCP endpoint: /mcp (requires OAuth Bearer token)\n")
	fmt.Printf("  OAuth endpoints:\n")
	fmt.Printf("    - Authorization Server Metadata: /.well-known/oauth-authorization-server\n")
	fmt.Printf("    - Protected Resource Metadata: /.well-known/oauth-protected-resource\n")
	fmt.Printf("    - Client Registration: /oauth/register\n")
	fmt.Printf("    - Authorization: /oauth/authorize\n")
	fmt.Printf("    - Token: /oauth/token\n")
	fmt.Printf("    - Callback: /oauth/callback\n")
	fmt.Printf("    - Revoke: /oauth/revoke\n")
	fmt.Printf("    - Introspect: /oauth/introspect\n")

	// Start server in goroutine
	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if err := oauthServer.Start(addr, config); err != nil {
			serverDone <- err
		}
	}()

	// Wait for either shutdown signal or server completion
	select {
	case <-ctx.Done():
		fmt.Println("Shutdown signal received, stopping OAuth HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := oauthServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error shutting down OAuth HTTP server: %w", err)
		}
	case err := <-serverDone:
		if err != nil {
			return fmt.Errorf("OAuth HTTP server stopped with error: %w", err)
		} else {
			fmt.Println("OAuth HTTP server stopped normally")
		}
	}

	fmt.Println("OAuth HTTP server gracefully stopped")
	return nil
}
