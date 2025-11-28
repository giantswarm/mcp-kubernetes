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
			return runServe(transport, nonDestructiveMode, dryRun, qpsLimit, burstLimit, debugMode, inCluster,
				httpAddr, sseEndpoint, messageEndpoint, httpEndpoint,
				enableOAuth, oauthBaseURL, googleClientID, googleClientSecret, disableStreaming,
				registrationToken, allowPublicRegistration, allowInsecureAuthWithoutState, maxClientsPerIP, oauthEncryptionKey,
				downstreamOAuth)
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
func runServe(transport string, nonDestructiveMode, dryRun bool, qpsLimit float32, burstLimit int, debugMode, inCluster bool,
	httpAddr, sseEndpoint, messageEndpoint, httpEndpoint string,
	enableOAuth bool, oauthBaseURL, googleClientID, googleClientSecret string, disableStreaming bool,
	registrationToken string, allowPublicRegistration, allowInsecureAuthWithoutState bool, maxClientsPerIP int, oauthEncryptionKey string,
	downstreamOAuth bool) error {

	// Create Kubernetes client configuration
	var k8sLogger = &simpleLogger{}

	k8sConfig := &k8s.ClientConfig{
		NonDestructiveMode: nonDestructiveMode,
		DryRun:             dryRun,
		QPSLimit:           qpsLimit,
		BurstLimit:         burstLimit,
		Timeout:            30 * time.Second,
		DebugMode:          debugMode,
		InCluster:          inCluster,
		Logger:             k8sLogger,
	}

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient(k8sConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Validate downstream OAuth configuration
	if downstreamOAuth {
		if !enableOAuth {
			return fmt.Errorf("--downstream-oauth requires --enable-oauth to be set")
		}
		if !inCluster {
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
	if downstreamOAuth {
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
			if transport != "stdio" {
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
	switch transport {
	case "stdio":
		// Don't print startup message for stdio mode as it interferes with MCP communication
		return runStdioServer(mcpSrv)
	case "sse":
		fmt.Printf("Starting MCP Kubernetes server with %s transport...\n", transport)
		return runSSEServer(mcpSrv, httpAddr, sseEndpoint, messageEndpoint, shutdownCtx, debugMode)
	case "streamable-http":
		fmt.Printf("Starting MCP Kubernetes server with %s transport...\n", transport)
		if enableOAuth {
			// Get OAuth credentials from env vars if not provided via flags
			if googleClientID == "" {
				googleClientID = os.Getenv("GOOGLE_CLIENT_ID")
			}
			if googleClientSecret == "" {
				googleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
			}
			if oauthEncryptionKey == "" {
				oauthEncryptionKey = os.Getenv("OAUTH_ENCRYPTION_KEY")
			}

			// Validate OAuth configuration
			if oauthBaseURL == "" {
				return fmt.Errorf("--oauth-base-url is required when --enable-oauth is set")
			}
			if googleClientID == "" {
				return fmt.Errorf("Google Client ID is required (use --google-client-id or GOOGLE_CLIENT_ID env var)")
			}
			if googleClientSecret == "" {
				return fmt.Errorf("Google Client Secret is required (use --google-client-secret or GOOGLE_CLIENT_SECRET env var)")
			}
			if !allowPublicRegistration && registrationToken == "" {
				return fmt.Errorf("--registration-token is required when public registration is disabled")
			}

			// Prepare encryption key if provided
			var encryptionKey []byte
			if oauthEncryptionKey != "" {
				// Try to decode from base64 first
				decoded, err := base64.StdEncoding.DecodeString(oauthEncryptionKey)
				if err == nil && len(decoded) == 32 {
					encryptionKey = decoded
				} else {
					// Use as raw bytes if not base64 or wrong length after decoding
					encryptionKey = []byte(oauthEncryptionKey)
				}
				if len(encryptionKey) != 32 {
					return fmt.Errorf("OAuth encryption key must be exactly 32 bytes, got %d bytes (key might need to be base64 decoded)", len(encryptionKey))
				}
			}

			return runOAuthHTTPServer(mcpSrv, httpAddr, shutdownCtx, server.OAuthConfig{
				BaseURL:                       oauthBaseURL,
				GoogleClientID:                googleClientID,
				GoogleClientSecret:            googleClientSecret,
				DisableStreaming:              disableStreaming,
				DebugMode:                     debugMode,
				AllowPublicClientRegistration: allowPublicRegistration,
				RegistrationAccessToken:       registrationToken,
				AllowInsecureAuthWithoutState: allowInsecureAuthWithoutState,
				MaxClientsPerIP:               maxClientsPerIP,
				EncryptionKey:                 encryptionKey,
			})
		}
		return runStreamableHTTPServer(mcpSrv, httpAddr, httpEndpoint, shutdownCtx, debugMode)
	default:
		return fmt.Errorf("unsupported transport type: %s (supported: stdio, sse, streamable-http)", transport)
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
		if err := oauthServer.Start(addr); err != nil {
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
