package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
)

// runSSEServer runs the server with SSE transport
func runSSEServer(mcpSrv *mcpserver.MCPServer, addr, sseEndpoint, messageEndpoint string, ctx context.Context, debugMode bool, provider *instrumentation.Provider) error {
	if debugMode {
		log.Printf("[DEBUG] Initializing SSE server with configuration:")
		log.Printf("[DEBUG]   Address: %s", addr)
		log.Printf("[DEBUG]   SSE Endpoint: %s", sseEndpoint)
		log.Printf("[DEBUG]   Message Endpoint: %s", messageEndpoint)
	}

	// Create a custom HTTP server with metrics endpoint
	mux := http.NewServeMux()

	// Create SSE handler
	sseHandler := mcpserver.NewSSEServer(mcpSrv,
		mcpserver.WithSSEEndpoint(sseEndpoint),
		mcpserver.WithMessageEndpoint(messageEndpoint),
	)

	// Add SSE and message endpoints
	mux.Handle(sseEndpoint, sseHandler)
	mux.Handle(messageEndpoint, sseHandler)

	// Add metrics endpoint if instrumentation is enabled
	if provider != nil && provider.Enabled() {
		mux.Handle("/metrics", promhttp.Handler())
		fmt.Printf("  Metrics endpoint: /metrics\n")
	}

	if debugMode {
		log.Printf("[DEBUG] SSE server instance created successfully")
	}

	fmt.Printf("SSE server starting on %s\n", addr)
	fmt.Printf("  SSE endpoint: %s\n", sseEndpoint)
	fmt.Printf("  Message endpoint: %s\n", messageEndpoint)

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
		if debugMode {
			log.Printf("[DEBUG] Starting SSE server listener on %s", addr)
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if debugMode {
			log.Printf("[DEBUG] Starting graceful shutdown with 30s timeout")
		}
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
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
