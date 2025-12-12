package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// runStreamableHTTPServer runs the server with Streamable HTTP transport
func runStreamableHTTPServer(mcpSrv *mcpserver.MCPServer, addr, endpoint string, ctx context.Context, debugMode bool, provider *instrumentation.Provider, sc *server.ServerContext, metricsConfig MetricsServeConfig) error {
	// Create a custom HTTP server (metrics are now on a separate server)
	mux := http.NewServeMux()

	// Create Streamable HTTP handler
	mcpHandler := mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithEndpointPath(endpoint),
	)

	// Add MCP endpoint
	mux.Handle(endpoint, mcpHandler)

	// Note: Metrics are served on a separate metrics server for security
	// See startMetricsServer() for the dedicated /metrics endpoint

	// Add health check endpoints
	healthChecker := server.NewHealthChecker(sc)
	healthChecker.RegisterHealthEndpoints(mux)
	fmt.Printf("  Health endpoints: /healthz, /readyz\n")

	fmt.Printf("Streamable HTTP server starting on %s\n", addr)
	fmt.Printf("  HTTP endpoint: %s\n", endpoint)

	// Start metrics server if enabled
	var metricsServer *server.MetricsServer
	if metricsConfig.Enabled && provider != nil && provider.Enabled() {
		var err error
		metricsServer, err = startMetricsServer(metricsConfig, provider)
		if err != nil {
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
	}

	// Create HTTP server with security timeouts
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start server in goroutine
	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverDone <- err
		}
	}()

	// Wait for either shutdown signal or server completion
	select {
	case <-ctx.Done():
		fmt.Println("Shutdown signal received, stopping HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown metrics server first
		if metricsServer != nil {
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("error shutting down metrics server", "error", err)
			}
		}

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
func runOAuthHTTPServer(mcpSrv *mcpserver.MCPServer, addr string, ctx context.Context, config server.OAuthConfig, sc *server.ServerContext, metricsConfig MetricsServeConfig) error {
	// Create OAuth HTTP server
	oauthServer, err := server.NewOAuthHTTPServer(mcpSrv, "streamable-http", config)
	if err != nil {
		return fmt.Errorf("failed to create OAuth HTTP server: %w", err)
	}

	// Set up health checker
	healthChecker := server.NewHealthChecker(sc)
	oauthServer.SetHealthChecker(healthChecker)

	fmt.Printf("OAuth-enabled HTTP server starting on %s\n", addr)
	fmt.Printf("  Base URL: %s\n", config.BaseURL)
	fmt.Printf("  MCP endpoint: /mcp (requires OAuth Bearer token)\n")
	fmt.Printf("  Health endpoints: /healthz, /readyz\n")
	fmt.Printf("  OAuth endpoints:\n")
	fmt.Printf("    - Authorization Server Metadata: /.well-known/oauth-authorization-server\n")
	fmt.Printf("    - Protected Resource Metadata: /.well-known/oauth-protected-resource\n")
	fmt.Printf("    - Client Registration: /oauth/register\n")
	fmt.Printf("    - Authorization: /oauth/authorize\n")
	fmt.Printf("    - Token: /oauth/token\n")
	fmt.Printf("    - Callback: /oauth/callback\n")
	fmt.Printf("    - Revoke: /oauth/revoke\n")
	fmt.Printf("    - Introspect: /oauth/introspect\n")

	// Start metrics server if enabled (separate from main server for security)
	var metricsServer *server.MetricsServer
	if metricsConfig.Enabled && config.InstrumentationProvider != nil && config.InstrumentationProvider.Enabled() {
		metricsServer, err = startMetricsServer(metricsConfig, config.InstrumentationProvider)
		if err != nil {
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
	}

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

		// Shutdown metrics server first
		if metricsServer != nil {
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("error shutting down metrics server", "error", err)
			}
		}

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

// startMetricsServer starts the dedicated metrics server on a separate port.
// This isolates Prometheus metrics from the main application traffic for security.
func startMetricsServer(config MetricsServeConfig, provider *instrumentation.Provider) (*server.MetricsServer, error) {
	metricsServer, err := server.NewMetricsServer(server.MetricsServerConfig{
		Addr:                    config.Addr,
		Enabled:                 config.Enabled,
		InstrumentationProvider: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics server: %w", err)
	}

	// Start metrics server in background
	go func() {
		if err := metricsServer.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	fmt.Printf("  Metrics server: %s/metrics (dedicated port)\n", config.Addr)
	return metricsServer, nil
}
