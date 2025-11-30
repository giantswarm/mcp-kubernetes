package cmd

import (
	"context"
	"fmt"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

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
