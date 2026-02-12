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

// runSSEServer runs the server with SSE transport
func runSSEServer(mcpSrv *mcpserver.MCPServer, addr, sseEndpoint, messageEndpoint string, ctx context.Context, debugMode bool, provider *instrumentation.Provider, metricsConfig MetricsServeConfig) error {
	if debugMode {
		slog.Debug("initializing SSE server",
			"address", addr,
			"sse_endpoint", sseEndpoint,
			"message_endpoint", messageEndpoint)
	}

	// Create a custom HTTP server (metrics are now on a separate server)
	mux := http.NewServeMux()

	// Create SSE handler
	sseHandler := mcpserver.NewSSEServer(mcpSrv,
		mcpserver.WithSSEEndpoint(sseEndpoint),
		mcpserver.WithMessageEndpoint(messageEndpoint),
	)

	// Add SSE and message endpoints
	mux.Handle(sseEndpoint, sseHandler)
	mux.Handle(messageEndpoint, sseHandler)

	// Note: Metrics are served on a separate metrics server for security
	// See startMetricsServer() for the dedicated /metrics endpoint

	if debugMode {
		slog.Debug("sse server instance created successfully")
	}

	slog.Info("SSE server starting",
		"addr", addr,
		"sse_endpoint", sseEndpoint,
		"message_endpoint", messageEndpoint)

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
		if debugMode {
			slog.Debug("starting sse server listener", "address", addr)
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if debugMode {
				slog.Debug("sse server start failed", "error", err)
			}
			serverDone <- err
		} else {
			if debugMode {
				slog.Debug("sse server listener stopped cleanly")
			}
		}
	}()

	if debugMode {
		slog.Debug("sse server goroutine started, waiting for shutdown signal or server completion")
	}

	// Wait for either shutdown signal or server completion
	select {
	case <-ctx.Done():
		if debugMode {
			slog.Debug("shutdown signal received, initiating sse server shutdown")
		}
		slog.Info("shutdown signal received, stopping SSE server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), server.DefaultShutdownTimeout)
		defer cancel()
		if debugMode {
			slog.Debug("starting graceful shutdown", "timeout", server.DefaultShutdownTimeout)
		}

		// Shutdown metrics server first
		if metricsServer != nil {
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("error shutting down metrics server", "error", err)
			}
		}

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			if debugMode {
				slog.Debug("error during sse server shutdown", "error", err)
			}
			return fmt.Errorf("error shutting down SSE server: %w", err)
		}
		if debugMode {
			slog.Debug("sse server shutdown completed successfully")
		}
	case err := <-serverDone:
		if err != nil {
			if debugMode {
				slog.Debug("sse server stopped with error", "error", err)
			}
			return fmt.Errorf("SSE server stopped with error: %w", err)
		}
		if debugMode {
			slog.Debug("sse server stopped normally")
		}
		slog.Info("SSE server stopped normally")
	}

	slog.Info("SSE server gracefully stopped")
	if debugMode {
		slog.Debug("sse server shutdown sequence completed")
	}
	return nil
}
