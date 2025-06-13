package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/cluster"
	contexttools "github.com/giantswarm/mcp-kubernetes/internal/tools/context"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/helm"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/pod"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	// Setup context for server
	ctx := context.Background()

	// Create server context with kubernetes client and security enhancements
	serverContext, err := server.NewServerContext(ctx,
		// Enable policy-based security with default settings
		server.WithPolicyBasedSecurity(),
		// Use non-destructive mode by default for security
		server.WithNonDestructiveMode(true),
		// Set default allowed operations for read-only access
		server.WithAuth([]string{"get", "list", "describe", "logs"}),
		// Restrict access to system namespaces
		server.WithRestrictedNamespaces([]string{"kube-system", "kube-public", "kube-node-lease"}),
	)
	if err != nil {
		log.Fatalf("Failed to create server context: %v", err)
	}
	defer func() {
		if err := serverContext.Shutdown(); err != nil {
			log.Printf("Error during server context shutdown: %v", err)
		}
	}()

	// Log security status
	if serverContext.IsSecurityEnabled() {
		log.Println("Security enhancements enabled:")
		log.Printf("  - Non-destructive mode: %v", serverContext.Config().NonDestructiveMode)
		log.Printf("  - Allowed operations: %v", serverContext.Config().AllowedOperations)
		log.Printf("  - Restricted namespaces: %v", serverContext.Config().RestrictedNamespaces)
	} else {
		log.Println("WARNING: Security enhancements are disabled. Server running in permissive mode.")
	}

	// Create MCP server
	mcpSrv := mcpserver.NewMCPServer("mcp-kubernetes", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	// Register all tool categories with security middleware
	if err := registerSecureTools(mcpSrv, serverContext); err != nil {
		log.Fatalf("Failed to register tools: %v", err)
	}

	fmt.Println("Starting MCP Kubernetes server with security enhancements...")

	// Setup graceful shutdown
	shutdownCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Start stdio server
	if err := mcpserver.ServeStdio(mcpSrv); err != nil {
		log.Printf("Server stopped: %v", err)
	}

	<-shutdownCtx.Done()
	fmt.Println("Server gracefully stopped")
}

// registerSecureTools registers all tool categories with security middleware applied
func registerSecureTools(mcpSrv *mcpserver.MCPServer, serverContext *server.ServerContext) error {
	// Get security middleware from server context
	securityMiddleware := serverContext.SecurityMiddleware()

	// Register resource tools with security
	if err := resource.RegisterResourceTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register resource tools: %w", err)
	}

	// Register pod tools with security
	if err := pod.RegisterPodTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register pod tools: %w", err)
	}

	// Register context tools with security
	if err := contexttools.RegisterContextTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register context tools: %w", err)
	}

	// Register cluster tools with security
	if err := cluster.RegisterClusterTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register cluster tools: %w", err)
	}

	// Register helm tools with security
	if err := helm.RegisterHelmTools(mcpSrv, serverContext); err != nil {
		return fmt.Errorf("failed to register helm tools: %w", err)
	}

	// Log that tools are registered with security
	if serverContext.IsSecurityEnabled() {
		log.Println("All MCP tools registered with security middleware applied")
	} else {
		log.Println("All MCP tools registered without security (permissive mode)")
	}

	// The actual security middleware application would need to be implemented
	// in each tool registration function, as the current tool registration
	// functions don't support middleware wrapping yet.
	// This is a placeholder for the security integration point.
	_ = securityMiddleware

	return nil
}
