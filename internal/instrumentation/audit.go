package instrumentation

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// ToolInvocation captures all information about a tool invocation for audit logging.
// This provides a comprehensive audit trail for all MCP tool calls.
//
// # Privacy Considerations
//
// The UserEmail field contains PII. When logging, consider:
//   - Using UserDomain() to get only the domain for metrics/general logs
//   - Only logging full email in audit-specific log streams
//   - Ensuring audit logs have appropriate access controls
type ToolInvocation struct {
	// Tool name
	Tool string

	// User identity (from OAuth)
	UserEmail string
	Groups    []string

	// Target information
	ClusterName  string
	Namespace    string
	ResourceType string
	ResourceName string

	// Execution details
	StartTime time.Time
	Duration  time.Duration
	Success   bool
	Error     string

	// Tracing context
	TraceID string
	SpanID  string
}

// UserDomain returns the domain portion of the user's email for lower-cardinality logging.
func (ti *ToolInvocation) UserDomain() string {
	return ExtractUserDomain(ti.UserEmail)
}

// ClusterType returns the classified cluster type for lower-cardinality logging.
func (ti *ToolInvocation) ClusterType() string {
	return ClassifyClusterName(ti.ClusterName)
}

// Status returns "success" or "error" based on the Success field.
func (ti *ToolInvocation) Status() string {
	if ti.Success {
		return StatusSuccess
	}
	return StatusError
}

// LogAttrs returns slog attributes for structured logging.
// This provides a consistent set of fields for all tool invocation logs.
//
// # Cardinality
//
// This function uses cardinality-controlled values (user_domain, cluster_type)
// for metrics-compatible logging. For full audit logging, use LogAuditAttrs.
func (ti *ToolInvocation) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("tool", ti.Tool),
		slog.String("user_domain", ti.UserDomain()),
		slog.Int("group_count", len(ti.Groups)),
		slog.String("cluster_type", ti.ClusterType()),
		slog.Duration("duration", ti.Duration),
		slog.Bool("success", ti.Success),
	}

	// Add optional fields only if present
	if ti.Namespace != "" {
		attrs = append(attrs, slog.String("namespace", ti.Namespace))
	}
	if ti.ResourceType != "" {
		attrs = append(attrs, slog.String("resource_type", ti.ResourceType))
	}
	if ti.TraceID != "" {
		attrs = append(attrs, slog.String("trace_id", ti.TraceID))
	}
	if ti.Error != "" {
		attrs = append(attrs, slog.String("error", ti.Error))
	}

	return attrs
}

// LogAuditAttrs returns slog attributes for full audit logging.
// This includes the full user email and cluster name for compliance/audit purposes.
//
// # Security Warning
//
// This method includes PII (full email) and sensitive infrastructure details
// (cluster names). Ensure audit logs are:
//   - Stored securely with appropriate access controls
//   - Not exposed to general monitoring dashboards
//   - Retained according to compliance requirements
func (ti *ToolInvocation) LogAuditAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("tool", ti.Tool),
		slog.String("user", ti.UserEmail),
		slog.Any("groups", ti.Groups),
		slog.String("cluster", ti.ClusterName),
		slog.Duration("duration", ti.Duration),
		slog.Bool("success", ti.Success),
	}

	// Add all optional fields
	if ti.Namespace != "" {
		attrs = append(attrs, slog.String("namespace", ti.Namespace))
	}
	if ti.ResourceType != "" {
		attrs = append(attrs, slog.String("resource_type", ti.ResourceType))
	}
	if ti.ResourceName != "" {
		attrs = append(attrs, slog.String("resource_name", ti.ResourceName))
	}
	if ti.TraceID != "" {
		attrs = append(attrs, slog.String("trace_id", ti.TraceID))
	}
	if ti.SpanID != "" {
		attrs = append(attrs, slog.String("span_id", ti.SpanID))
	}
	if ti.Error != "" {
		attrs = append(attrs, slog.String("error", ti.Error))
	}

	return attrs
}

// NewToolInvocation creates a new ToolInvocation with timing started.
// Call Complete() when the tool operation finishes.
func NewToolInvocation(tool string) *ToolInvocation {
	return &ToolInvocation{
		Tool:      tool,
		StartTime: time.Now(),
	}
}

// WithUser sets the user identity information.
func (ti *ToolInvocation) WithUser(email string, groups []string) *ToolInvocation {
	ti.UserEmail = email
	ti.Groups = groups
	return ti
}

// WithCluster sets the target cluster name.
func (ti *ToolInvocation) WithCluster(clusterName string) *ToolInvocation {
	ti.ClusterName = clusterName
	return ti
}

// WithResource sets the resource information.
func (ti *ToolInvocation) WithResource(namespace, resourceType, resourceName string) *ToolInvocation {
	ti.Namespace = namespace
	ti.ResourceType = resourceType
	ti.ResourceName = resourceName
	return ti
}

// WithSpanContext extracts trace context from the current span.
func (ti *ToolInvocation) WithSpanContext(ctx context.Context) *ToolInvocation {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		ti.TraceID = span.SpanContext().TraceID().String()
		ti.SpanID = span.SpanContext().SpanID().String()
	}
	return ti
}

// Complete marks the invocation as completed and calculates duration.
// Returns the same ToolInvocation for method chaining.
func (ti *ToolInvocation) Complete(success bool, err error) *ToolInvocation {
	ti.Duration = time.Since(ti.StartTime)
	ti.Success = success
	if err != nil {
		ti.Error = err.Error()
	}
	return ti
}

// CompleteWithError marks the invocation as failed with the given error.
func (ti *ToolInvocation) CompleteWithError(err error) *ToolInvocation {
	return ti.Complete(false, err)
}

// CompleteSuccess marks the invocation as successful.
func (ti *ToolInvocation) CompleteSuccess() *ToolInvocation {
	return ti.Complete(true, nil)
}

// AuditLogger provides structured audit logging for tool invocations.
// It wraps slog.Logger with convenience methods for logging tool operations.
type AuditLogger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new AuditLogger with the given slog.Logger.
func NewAuditLogger(logger *slog.Logger) *AuditLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditLogger{logger: logger}
}

// LogToolInvocation logs a tool invocation using the standard log attributes.
// This is suitable for general operational logging with cardinality controls.
func (al *AuditLogger) LogToolInvocation(ti *ToolInvocation) {
	attrs := ti.LogAttrs()
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}

	if ti.Success {
		al.logger.Info("tool_executed", args...)
	} else {
		al.logger.Warn("tool_failed", args...)
	}
}

// LogToolAudit logs a tool invocation with full audit details.
// This includes PII and should be sent to secure audit streams.
func (al *AuditLogger) LogToolAudit(ti *ToolInvocation) {
	attrs := ti.LogAuditAttrs()
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}

	al.logger.Info("tool_audit", args...)
}

// TraceIDFromContext extracts the trace ID from the current span in context.
// Returns empty string if no valid span is present.
//
// Deprecated: Use GetTraceID instead. This function is kept for backwards compatibility
// and will be removed in v2.0.
func TraceIDFromContext(ctx context.Context) string {
	return GetTraceID(ctx)
}
