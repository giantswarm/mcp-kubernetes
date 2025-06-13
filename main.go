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

	// Create server context with kubernetes client
	serverContext, err := server.NewServerContext(ctx)
	if err != nil {
		log.Fatalf("Failed to create server context: %v", err)
	}
	defer func() {
		if err := serverContext.Shutdown(); err != nil {
			log.Printf("Error during server context shutdown: %v", err)
		}
	}()

	// Create MCP server
	mcpSrv := mcpserver.NewMCPServer("mcp-kubernetes", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	// Register all tool categories
	if err := resource.RegisterResourceTools(mcpSrv, serverContext); err != nil {
		log.Fatalf("Failed to register resource tools: %v", err)
	}

	if err := pod.RegisterPodTools(mcpSrv, serverContext); err != nil {
		log.Fatalf("Failed to register pod tools: %v", err)
	}

	if err := contexttools.RegisterContextTools(mcpSrv, serverContext); err != nil {
		log.Fatalf("Failed to register context tools: %v", err)
	}

	if err := cluster.RegisterClusterTools(mcpSrv, serverContext); err != nil {
		log.Fatalf("Failed to register cluster tools: %v", err)
	}

	if err := helm.RegisterHelmTools(mcpSrv, serverContext); err != nil {
		log.Fatalf("Failed to register helm tools: %v", err)
	}

	fmt.Println("Starting MCP Kubernetes server...")

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
