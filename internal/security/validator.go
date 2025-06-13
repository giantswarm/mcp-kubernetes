package security

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Validator provides security validation for various operations and configurations
type Validator struct {
	authorizer      Authorizer
	credentialMgr   *CredentialManager
	validationRules ValidationRules
}

// NewValidator creates a new security validator with the specified components
func NewValidator(authorizer Authorizer, credMgr *CredentialManager, rules ValidationRules) *Validator {
	return &Validator{
		authorizer:      authorizer,
		credentialMgr:   credMgr,
		validationRules: rules,
	}
}

// ValidationRules defines rules for security validation
type ValidationRules struct {
	// MaxNameLength defines the maximum allowed length for resource names
	MaxNameLength int `json:"maxNameLength"`

	// AllowedResourceTypes lists the allowed Kubernetes resource types
	AllowedResourceTypes []string `json:"allowedResourceTypes"`

	// ForbiddenNamePatterns contains regex patterns for forbidden resource names
	ForbiddenNamePatterns []string `json:"forbiddenNamePatterns"`

	// RequireNamespaceForResources specifies if namespaced resources must have a namespace
	RequireNamespaceForResources bool `json:"requireNamespaceForResources"`

	// AllowPrivilegedOperations determines if privileged operations are allowed
	AllowPrivilegedOperations bool `json:"allowPrivilegedOperations"`
}

// DefaultValidationRules returns a set of secure default validation rules
func DefaultValidationRules() ValidationRules {
	return ValidationRules{
		MaxNameLength: 253, // Kubernetes DNS subdomain limit
		AllowedResourceTypes: []string{
			"pods", "services", "deployments", "configmaps", "secrets",
			"namespaces", "nodes", "persistentvolumes", "persistentvolumeclaims",
		},
		ForbiddenNamePatterns: []string{
			"^kube-.*",   // Kubernetes system resources
			".*-admin$",  // Admin resources
			".*-secret$", // Potential secrets
		},
		RequireNamespaceForResources: true,
		AllowPrivilegedOperations:    false,
	}
}

// ValidateOperation performs comprehensive validation of a Kubernetes operation
func (v *Validator) ValidateOperation(ctx context.Context, req *OperationRequest) error {
	if req == nil {
		return ErrInvalidRequest
	}

	// Perform authorization check
	if err := v.authorizer.Authorize(ctx, req); err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	// Validate resource type
	if err := v.validateResourceType(req.Resource); err != nil {
		return fmt.Errorf("resource type validation failed: %w", err)
	}

	// Validate resource name
	if err := v.validateResourceName(req.Name); err != nil {
		return fmt.Errorf("resource name validation failed: %w", err)
	}

	// Validate namespace requirements
	if err := v.validateNamespaceRequirement(req.Resource, req.Namespace); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}

	// Validate privileged operations
	if err := v.validatePrivilegedOperation(req.Operation); err != nil {
		return fmt.Errorf("privileged operation validation failed: %w", err)
	}

	// Validate Kubernetes context if credential manager is available
	if v.credentialMgr != nil {
		if err := v.credentialMgr.ValidateContext(req.Context); err != nil {
			return fmt.Errorf("context validation failed: %w", err)
		}
	}

	return nil
}

// validateResourceType checks if the resource type is allowed
func (v *Validator) validateResourceType(resourceType string) error {
	if resourceType == "" {
		return nil // Empty resource type is allowed for some operations
	}

	// If no allowed types are configured, allow all
	if len(v.validationRules.AllowedResourceTypes) == 0 {
		return nil
	}

	// Normalize to lowercase for comparison
	normalizedType := strings.ToLower(resourceType)

	// Check if resource type is in allowed list
	for _, allowed := range v.validationRules.AllowedResourceTypes {
		if normalizedType == strings.ToLower(allowed) {
			return nil
		}
	}

	return NewForbiddenError("resourceType", resourceType, "resource type not in allowed list")
}

// validateResourceName validates the resource name according to security rules
func (v *Validator) validateResourceName(name string) error {
	if name == "" {
		return nil // Empty name is allowed for list operations
	}

	// Check name length
	if v.validationRules.MaxNameLength > 0 && len(name) > v.validationRules.MaxNameLength {
		return NewValidationError("name", name,
			fmt.Sprintf("name exceeds maximum length of %d characters", v.validationRules.MaxNameLength), nil)
	}

	// Check against forbidden patterns
	for _, pattern := range v.validationRules.ForbiddenNamePatterns {
		matched, err := regexp.MatchString(pattern, name)
		if err != nil {
			continue // Skip invalid regex patterns
		}
		if matched {
			return NewForbiddenError("name", name, fmt.Sprintf("matches forbidden pattern: %s", pattern))
		}
	}

	// Validate Kubernetes naming conventions
	if err := validateKubernetesName(name); err != nil {
		return NewValidationError("name", name, "invalid Kubernetes name format", err)
	}

	return nil
}

// validateNamespaceRequirement checks if namespace is required and provided
func (v *Validator) validateNamespaceRequirement(resourceType, namespace string) error {
	if !v.validationRules.RequireNamespaceForResources {
		return nil
	}

	// List of resources that are typically namespaced
	namespacedResources := []string{
		"pods", "services", "deployments", "replicasets", "configmaps",
		"secrets", "persistentvolumeclaims", "jobs", "cronjobs",
	}

	// Check if this resource type typically requires a namespace
	normalizedType := strings.ToLower(resourceType)
	for _, nsResource := range namespacedResources {
		if normalizedType == nsResource && namespace == "" {
			return NewValidationError("namespace", "",
				fmt.Sprintf("namespace is required for resource type '%s'", resourceType), nil)
		}
	}

	return nil
}

// validatePrivilegedOperation checks if privileged operations are allowed
func (v *Validator) validatePrivilegedOperation(operation string) error {
	if v.validationRules.AllowPrivilegedOperations {
		return nil
	}

	// Define operations that are considered privileged
	privilegedOps := []string{
		"create", "apply", "patch", "delete", "scale",
		"helm-install", "helm-upgrade", "helm-uninstall",
	}

	normalizedOp := strings.ToLower(operation)
	for _, privOp := range privilegedOps {
		if normalizedOp == privOp {
			return NewForbiddenError("operation", operation, "privileged operations are not allowed")
		}
	}

	return nil
}

// validateKubernetesName validates that a name conforms to Kubernetes naming conventions
func validateKubernetesName(name string) error {
	// Kubernetes names must be lowercase alphanumeric with dashes and dots
	// Must start and end with alphanumeric
	nameRegex := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`

	matched, err := regexp.MatchString(nameRegex, name)
	if err != nil {
		return fmt.Errorf("regex compilation error: %w", err)
	}

	if !matched {
		return fmt.Errorf("name must consist of lowercase alphanumeric characters, dashes, or dots, and must start and end with an alphanumeric character")
	}

	return nil
}

// ValidateConfiguration validates server security configuration
func (v *Validator) ValidateConfiguration(config interface{}) error {
	// This could be extended to validate server configuration
	// For now, return nil as basic validation
	return nil
}

// SecurityContext holds security validation context for operations
type SecurityContext struct {
	UserID     string                 `json:"userId,omitempty"`
	SessionID  string                 `json:"sessionId,omitempty"`
	RemoteAddr string                 `json:"remoteAddr,omitempty"`
	UserAgent  string                 `json:"userAgent,omitempty"`
	RequestID  string                 `json:"requestId,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// NewSecurityContext creates a new security context for tracking operations
func NewSecurityContext(userID, sessionID string) *SecurityContext {
	return &SecurityContext{
		UserID:    userID,
		SessionID: sessionID,
		Metadata:  make(map[string]interface{}),
	}
}

// WithRequestID adds a request ID to the security context
func (sc *SecurityContext) WithRequestID(requestID string) *SecurityContext {
	sc.RequestID = requestID
	return sc
}

// WithMetadata adds metadata to the security context
func (sc *SecurityContext) WithMetadata(key string, value interface{}) *SecurityContext {
	if sc.Metadata == nil {
		sc.Metadata = make(map[string]interface{})
	}
	sc.Metadata[key] = value
	return sc
}

// AuditLog represents an audit log entry for security events
type AuditLog struct {
	Timestamp string                 `json:"timestamp"`
	EventType string                 `json:"eventType"`
	Operation *OperationRequest      `json:"operation,omitempty"`
	Context   *SecurityContext       `json:"context,omitempty"`
	Result    string                 `json:"result"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// LogSecurityEvent logs a security event for auditing purposes
func LogSecurityEvent(eventType string, operation *OperationRequest, context *SecurityContext, result string, err error) {
	// In a real implementation, this would write to a secure audit log
	// For now, we'll just structure the audit log entry

	auditEntry := &AuditLog{
		Timestamp: fmt.Sprintf("%d", context.Metadata["timestamp"]),
		EventType: eventType,
		Operation: operation,
		Context:   context,
		Result:    result,
	}

	if err != nil {
		// Sanitize the error before logging
		sanitized := SanitizeError(err)
		auditEntry.Error = sanitized.Error()
	}

	// TODO: Implement actual audit logging to secure storage
	// This could be extended to write to files, databases, or external systems
}
