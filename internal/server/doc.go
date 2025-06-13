// Package server provides the ServerContext pattern and related infrastructure
// for the MCP Kubernetes server.
//
// This package implements the core server architecture patterns including:
//
//   - ServerContext: Encapsulates all server dependencies and lifecycle management
//   - Functional Options: Clean dependency injection and configuration
//   - Logger Interface: Abstraction for logging operations
//   - Configuration Management: Centralized server configuration
//
// The ServerContext Pattern:
//
// The ServerContext struct follows the context pattern commonly used in Go
// applications to encapsulate dependencies and provide clean separation of
// concerns. It includes:
//
//   - Kubernetes client interface
//   - Logger interface
//   - Configuration settings
//   - Context for cancellation and timeouts
//   - Lifecycle management (shutdown, cleanup)
//
// All dependencies are injected using functional options, making the code
// highly testable and modular. The pattern enables:
//
//   - Easy mocking for unit tests
//   - Runtime configuration flexibility
//   - Clean dependency management
//   - Graceful shutdown handling
//
// Example usage:
//
//	// Create a server context with custom configuration
//	ctx := context.Background()
//	serverCtx, err := NewServerContext(ctx,
//		WithK8sClient(k8sClient),
//		WithLogger(customLogger),
//		WithNonDestructiveMode(true),
//		WithDefaultNamespace("production"),
//		WithLogLevel("debug"),
//	)
//	if err != nil {
//		return err
//	}
//	defer serverCtx.Shutdown()
//
//	// Use the context in MCP tools
//	client := serverCtx.K8sClient()
//	logger := serverCtx.Logger()
//	config := serverCtx.Config()
//
//	// Check if server is shutting down
//	if serverCtx.IsShutdown() {
//		return ErrServerShutdown
//	}
//
// Configuration Management:
//
// The Config struct provides centralized configuration with sensible defaults
// and support for:
//
//   - Server identity (name, version)
//   - Kubernetes settings (default namespace, context, kubeconfig path)
//   - Non-destructive mode and dry-run settings
//   - Logging configuration (level, format)
//   - Security settings (authentication, allowed operations, restricted namespaces)
//
// The configuration supports deep cloning to prevent accidental mutations
// and follows immutable patterns where possible.
//
// Functional Options Pattern:
//
// The package uses functional options for flexible and extensible configuration:
//
//   - WithK8sClient: Inject Kubernetes client
//   - WithLogger: Inject custom logger
//   - WithConfig: Provide complete configuration
//   - WithServerName: Set server name
//   - WithDefaultNamespace: Set default Kubernetes namespace
//   - WithNonDestructiveMode: Enable/disable non-destructive mode
//   - WithDryRun: Enable/disable dry-run mode
//   - WithLogLevel: Set logging level
//   - WithAuth: Configure authentication and authorization
//   - WithRestrictedNamespaces: Set namespace restrictions
//
// This pattern allows for clean composition and makes the API forward-compatible
// as new options can be added without breaking existing code.
package server
