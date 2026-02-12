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
	"github.com/giantswarm/mcp-kubernetes/internal/server/middleware"
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

	slog.Info("streamable HTTP server starting",
		"addr", addr,
		"endpoint", endpoint,
		"health_endpoints", []string{"/healthz", "/readyz"})

	// Apply HTTP metrics middleware to record request metrics
	var handler http.Handler = mux
	handler = middleware.HTTPMetrics(provider)(handler)

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
		Handler:           handler,
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
		slog.Info("shutdown signal received, stopping HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), server.DefaultShutdownTimeout)
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
		}
		slog.Info("HTTP server stopped normally")
	}

	slog.Info("HTTP server gracefully stopped")
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

	slog.Info("OAuth-enabled HTTP server starting",
		"addr", addr,
		"base_url", config.BaseURL,
		"mcp_endpoint", "/mcp",
		"health_endpoints", []string{"/healthz", "/readyz"},
		"oauth_endpoints", []string{
			"/.well-known/oauth-authorization-server",
			"/.well-known/oauth-protected-resource",
			"/oauth/register",
			"/oauth/authorize",
			"/oauth/token",
			"/oauth/callback",
			"/oauth/revoke",
			"/oauth/introspect",
		})

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
		slog.Info("shutdown signal received, stopping OAuth HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), server.DefaultShutdownTimeout)
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
		}
		slog.Info("OAuth HTTP server stopped normally")
	}

	slog.Info("OAuth HTTP server gracefully stopped")
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

	slog.Info("metrics server started", "addr", config.Addr, "endpoint", "/metrics")
	return metricsServer, nil
}
