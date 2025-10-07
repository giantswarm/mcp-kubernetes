package cmd

import (
	"context"
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

// silentLogger discards all log output for stdio mode
type silentLogger struct{}

func (l *silentLogger) Debug(msg string, args ...interface{}) {
	// Discard output
}

func (l *silentLogger) Info(msg string, args ...interface{}) {
	// Discard output
}

func (l *silentLogger) Warn(msg string, args ...interface{}) {
	// Discard output
}

func (l *silentLogger) Error(msg string, args ...interface{}) {
	// Discard output
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
  - In-cluster: Uses service account token when running inside a Kubernetes pod`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(transport, nonDestructiveMode, dryRun, qpsLimit, burstLimit, debugMode, inCluster,
				httpAddr, sseEndpoint, messageEndpoint, httpEndpoint)
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

	return cmd
}

// runServe contains the main server logic with support for multiple transports
func runServe(transport string, nonDestructiveMode, dryRun bool, qpsLimit float32, burstLimit int, debugMode, inCluster bool,
	httpAddr, sseEndpoint, messageEndpoint, httpEndpoint string) error {

	// Create Kubernetes client configuration
	// Use a silent logger for stdio mode to avoid output interference
	var k8sLogger k8s.Logger
	if transport == "stdio" {
		k8sLogger = &silentLogger{}
	} else {
		k8sLogger = &simpleLogger{}
	}

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

	// Setup graceful shutdown - listen for both SIGINT and SIGTERM
	shutdownCtx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create server context with kubernetes client and shutdown context
	// Use silent logger for stdio mode to avoid any output interference
	var serverContextOptions []server.Option
	serverContextOptions = append(serverContextOptions, server.WithK8sClient(k8sClient))

	if transport == "stdio" {
		serverContextOptions = append(serverContextOptions, server.WithLogger(server.NewSilentLogger()))
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
