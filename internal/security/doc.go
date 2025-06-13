// Package security provides security enhancements for the MCP Kubernetes server.
//
// This package implements operation-level authorization, secure credential handling,
// and security validation for Kubernetes operations performed through the MCP server.
//
// ## Core Components
//
// ### Authorization
//
// The authorization system provides fine-grained control over which operations
// can be performed on Kubernetes resources:
//
//	// Create a policy-based authorizer
//	authorizer := security.NewPolicyAuthorizer(security.PolicyConfig{
//		AllowedOperations:    []string{"get", "list", "describe"},
//		RestrictedNamespaces: []string{"kube-system", "kube-public"},
//		NonDestructiveMode:   true,
//	})
//
//	// Use in middleware
//	middleware := security.SecurityMiddleware(authorizer)
//	secureHandler := middleware(originalHandler)
//
// ### Credential Management
//
// The credential manager handles secure access to Kubernetes credentials:
//
//	// Create a credential manager
//	credMgr := security.NewCredentialManager(security.CredentialConfig{
//		AllowedKubeconfigPaths: []string{"/home/user/.kube/config"},
//		AllowedContexts:        []string{"prod", "staging"},
//		RestrictSensitiveData:  true,
//	})
//
//	// Validate and secure configuration
//	err := credMgr.ValidateKubeconfigPath("/path/to/kubeconfig")
//	secureConfig := credMgr.SecureRestConfig(restConfig)
//
// ### Error Handling
//
// Security errors provide detailed context while protecting sensitive information:
//
//	// Check error types
//	if security.IsForbiddenError(err) {
//		// Handle forbidden access
//	}
//
//	// Sanitize errors before logging
//	sanitized := security.SanitizeError(err)
//
// ## Security Features
//
// - **Operation Authorization**: Control which Kubernetes operations are allowed
// - **Namespace Restrictions**: Limit access to specific namespaces
// - **Non-Destructive Mode**: Allow only read-only operations
// - **Credential Validation**: Validate kubeconfig paths and contexts
// - **Sensitive Data Protection**: Redact credentials in logs and error messages
// - **Secure Transport**: Wrap HTTP transport to prevent credential leakage
//
// ## Integration with MCP Server
//
// The security package integrates with the MCP server through middleware that
// wraps tool handlers with authorization checks. This ensures that every
// operation is validated before execution:
//
//	// In tool registration
//	secureHandler := security.SecurityMiddleware(authorizer)(toolHandler)
//	mcpServer.RegisterTool("kubectl-get", secureHandler)
//
// ## Configuration
//
// Security policies are typically configured through the server configuration:
//
//	type ServerConfig struct {
//		// Security settings
//		EnableAuth           bool     `json:"enableAuth"`
//		AllowedOperations    []string `json:"allowedOperations"`
//		RestrictedNamespaces []string `json:"restrictedNamespaces"`
//		NonDestructiveMode   bool     `json:"nonDestructiveMode"`
//	}
//
// The security components automatically integrate with these configuration
// settings through the server context and dependency injection.
package security
