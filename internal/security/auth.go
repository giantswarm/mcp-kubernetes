package security

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Authorizer defines the interface for authorization mechanisms
type Authorizer interface {
	// Authorize checks if the given operation is allowed
	Authorize(ctx context.Context, req *OperationRequest) error
}

// OperationRequest represents a request to perform an operation on a Kubernetes resource
type OperationRequest struct {
	// Operation type (e.g., "get", "list", "create", "delete", "apply", "patch")
	Operation string `json:"operation"`

	// Resource type (e.g., "pods", "deployments", "services")
	Resource string `json:"resource"`

	// Namespace where the operation will be performed
	Namespace string `json:"namespace"`

	// Resource name (empty for list operations)
	Name string `json:"name,omitempty"`

	// Kubernetes context
	Context string `json:"context,omitempty"`

	// Additional operation metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PolicyAuthorizer implements authorization based on configurable policies
type PolicyAuthorizer struct {
	// List of allowed operations (e.g., ["get", "list", "describe"])
	allowedOperations []string

	// List of restricted namespaces that cannot be accessed
	restrictedNamespaces []string

	// Whether non-destructive mode is enabled (only allows read operations)
	nonDestructiveMode bool

	// Whether dry-run mode is enabled
	dryRunMode bool
}

// NewPolicyAuthorizer creates a new policy-based authorizer
func NewPolicyAuthorizer(config PolicyConfig) *PolicyAuthorizer {
	return &PolicyAuthorizer{
		allowedOperations:    config.AllowedOperations,
		restrictedNamespaces: config.RestrictedNamespaces,
		nonDestructiveMode:   config.NonDestructiveMode,
		dryRunMode:           config.DryRunMode,
	}
}

// PolicyConfig holds the configuration for policy-based authorization
type PolicyConfig struct {
	AllowedOperations    []string `json:"allowedOperations"`
	RestrictedNamespaces []string `json:"restrictedNamespaces"`
	NonDestructiveMode   bool     `json:"nonDestructiveMode"`
	DryRunMode           bool     `json:"dryRunMode"`
}

// Authorize checks if the operation is allowed based on the configured policies
func (a *PolicyAuthorizer) Authorize(ctx context.Context, req *OperationRequest) error {
	if req == nil {
		return ErrInvalidRequest
	}

	// Validate operation type
	if err := a.validateOperation(req.Operation); err != nil {
		return err
	}

	// Validate namespace access
	if err := a.validateNamespace(req.Namespace); err != nil {
		return err
	}

	// Check non-destructive mode restrictions
	if err := a.validateNonDestructive(req.Operation); err != nil {
		return err
	}

	return nil
}

// validateOperation checks if the operation type is allowed
func (a *PolicyAuthorizer) validateOperation(operation string) error {
	if operation == "" {
		return ErrMissingOperation
	}

	// Normalize operation to lowercase
	op := strings.ToLower(operation)

	// Check if operation is in allowed list
	if len(a.allowedOperations) > 0 && !slices.Contains(a.allowedOperations, op) {
		return NewForbiddenError("operation", operation, "not in allowed operations list")
	}

	return nil
}

// validateNamespace checks if the namespace can be accessed
func (a *PolicyAuthorizer) validateNamespace(namespace string) error {
	if namespace == "" {
		// Empty namespace is allowed (will use default)
		return nil
	}

	// Check if namespace is restricted
	if slices.Contains(a.restrictedNamespaces, namespace) {
		return NewForbiddenError("namespace", namespace, "access to this namespace is restricted")
	}

	return nil
}

// validateNonDestructive checks if the operation is allowed in non-destructive mode
func (a *PolicyAuthorizer) validateNonDestructive(operation string) error {
	if !a.nonDestructiveMode {
		return nil
	}

	op := strings.ToLower(operation)

	// Define read-only operations that are allowed in non-destructive mode
	readOnlyOps := []string{"get", "list", "describe", "logs", "exec"}

	if !slices.Contains(readOnlyOps, op) {
		return NewForbiddenError("operation", operation, "not allowed in non-destructive mode")
	}

	return nil
}

// IsDestructiveOperation returns true if the operation modifies cluster state
func IsDestructiveOperation(operation string) bool {
	op := strings.ToLower(operation)
	destructiveOps := []string{"create", "apply", "patch", "delete", "scale", "helm-install", "helm-upgrade", "helm-uninstall"}
	return slices.Contains(destructiveOps, op)
}

// IsReadOnlyOperation returns true if the operation only reads cluster state
func IsReadOnlyOperation(operation string) bool {
	return !IsDestructiveOperation(operation)
}

// PermissiveAuthorizer allows all operations (for development/testing)
type PermissiveAuthorizer struct{}

// NewPermissiveAuthorizer creates an authorizer that allows all operations
func NewPermissiveAuthorizer() *PermissiveAuthorizer {
	return &PermissiveAuthorizer{}
}

// Authorize always returns nil (allows everything)
func (a *PermissiveAuthorizer) Authorize(ctx context.Context, req *OperationRequest) error {
	return nil
}

// SecurityMiddleware wraps tool handlers with authorization checks
func SecurityMiddleware(authorizer Authorizer) func(next ToolHandler) ToolHandler {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			// Extract operation request from tool arguments
			req, err := ExtractOperationRequest(args)
			if err != nil {
				return nil, fmt.Errorf("failed to extract operation request: %w", err)
			}

			// Perform authorization check
			if err := authorizer.Authorize(ctx, req); err != nil {
				return nil, fmt.Errorf("operation not authorized: %w", err)
			}

			// Call the original handler if authorized
			return next(ctx, args)
		}
	}
}

// ToolHandler represents a function that handles MCP tool calls
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// ExtractOperationRequest extracts operation details from MCP tool arguments
func ExtractOperationRequest(args map[string]interface{}) (*OperationRequest, error) {
	req := &OperationRequest{
		Metadata: make(map[string]interface{}),
	}

	// Extract common fields from tool arguments
	if operation, ok := args["operation"].(string); ok {
		req.Operation = operation
	}

	if resource, ok := args["resource"].(string); ok {
		req.Resource = resource
	} else if resourceType, ok := args["resourceType"].(string); ok {
		req.Resource = resourceType
	}

	if namespace, ok := args["namespace"].(string); ok {
		req.Namespace = namespace
	}

	if name, ok := args["name"].(string); ok {
		req.Name = name
	}

	if context, ok := args["kubeContext"].(string); ok {
		req.Context = context
	} else if context, ok := args["context"].(string); ok {
		req.Context = context
	}

	// Infer operation from tool arguments if not explicitly set
	if req.Operation == "" {
		req.Operation = inferOperationFromArgs(args)
	}

	// Store original arguments as metadata
	req.Metadata["originalArgs"] = args

	return req, nil
}

// inferOperationFromArgs attempts to determine the operation type from tool arguments
func inferOperationFromArgs(args map[string]interface{}) string {
	// Check for explicit action indicators
	if _, ok := args["delete"]; ok {
		return "delete"
	}

	if _, ok := args["create"]; ok {
		return "create"
	}

	if _, ok := args["apply"]; ok {
		return "apply"
	}

	if _, ok := args["patch"]; ok {
		return "patch"
	}

	if _, ok := args["scale"]; ok {
		return "scale"
	}

	// Check for Helm operations
	if helmAction, ok := args["helmAction"].(string); ok {
		return "helm-" + strings.ToLower(helmAction)
	}

	// Check for log operations
	if _, ok := args["logs"]; ok {
		return "logs"
	}

	if _, ok := args["follow"]; ok {
		return "logs"
	}

	// Check for exec operations
	if _, ok := args["command"]; ok {
		return "exec"
	}

	// Check if name is provided (likely a get operation)
	if name, ok := args["name"].(string); ok && name != "" {
		return "get"
	}

	// Default to list operation
	return "list"
}
