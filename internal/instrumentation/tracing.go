package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracerName is the default tracer name for the mcp-kubernetes package.
const TracerName = "github.com/giantswarm/mcp-kubernetes"

// Span attribute keys for CAPI/Federation operations.
const (
	// SpanAttrCluster is the cluster name attribute.
	SpanAttrCluster = "mcp.cluster"

	// SpanAttrClusterType is the classified cluster type attribute.
	SpanAttrClusterType = "mcp.cluster_type"

	// SpanAttrUserEmail is the user's email attribute (PII - use with care).
	SpanAttrUserEmail = "mcp.user.email"

	// SpanAttrUserDomain is the user's email domain (lower cardinality).
	SpanAttrUserDomain = "mcp.user.domain"

	// SpanAttrGroupCount is the number of groups the user belongs to.
	SpanAttrGroupCount = "mcp.user.group_count"

	// SpanAttrTool is the MCP tool name.
	SpanAttrTool = "mcp.tool"

	// SpanAttrNamespace is the Kubernetes namespace.
	SpanAttrNamespace = "k8s.namespace"

	// SpanAttrResourceType is the Kubernetes resource type.
	SpanAttrResourceType = "k8s.resource_type"

	// SpanAttrResourceName is the Kubernetes resource name.
	SpanAttrResourceName = "k8s.resource_name"

	// SpanAttrOperation is the operation type (get, list, create, delete, etc.).
	SpanAttrOperation = "k8s.operation"

	// SpanAttrCacheHit indicates whether a cache hit occurred.
	SpanAttrCacheHit = "mcp.cache_hit"

	// SpanAttrImpersonated indicates whether impersonation was used.
	SpanAttrImpersonated = "mcp.impersonated"

	// SpanAttrFederated indicates whether federation was used.
	SpanAttrFederated = "mcp.federated"
)

// SpanAttributeBuilder helps construct OpenTelemetry span attributes
// with consistent naming and cardinality controls.
type SpanAttributeBuilder struct {
	attrs []attribute.KeyValue
}

// NewSpanAttributeBuilder creates a new SpanAttributeBuilder.
func NewSpanAttributeBuilder() *SpanAttributeBuilder {
	return &SpanAttributeBuilder{
		attrs: make([]attribute.KeyValue, 0, 10),
	}
}

// WithTool adds the MCP tool name attribute.
func (b *SpanAttributeBuilder) WithTool(tool string) *SpanAttributeBuilder {
	b.attrs = append(b.attrs, attribute.String(SpanAttrTool, tool))
	return b
}

// WithCluster adds cluster attributes with cardinality control.
// Adds both the full cluster name and classified type.
func (b *SpanAttributeBuilder) WithCluster(clusterName string) *SpanAttributeBuilder {
	b.attrs = append(b.attrs,
		attribute.String(SpanAttrCluster, clusterName),
		attribute.String(SpanAttrClusterType, ClassifyClusterName(clusterName)),
	)
	return b
}

// WithClusterType adds only the classified cluster type (for lower cardinality).
func (b *SpanAttributeBuilder) WithClusterType(clusterName string) *SpanAttributeBuilder {
	b.attrs = append(b.attrs,
		attribute.String(SpanAttrClusterType, ClassifyClusterName(clusterName)),
	)
	return b
}

// WithUser adds user attributes with optional cardinality control.
// If includeEmail is true, includes the full email; otherwise only the domain.
func (b *SpanAttributeBuilder) WithUser(email string, groups []string, includeEmail bool) *SpanAttributeBuilder {
	if includeEmail {
		b.attrs = append(b.attrs, attribute.String(SpanAttrUserEmail, email))
	}
	b.attrs = append(b.attrs,
		attribute.String(SpanAttrUserDomain, ExtractUserDomain(email)),
		attribute.Int(SpanAttrGroupCount, len(groups)),
	)
	return b
}

// WithNamespace adds the Kubernetes namespace attribute.
func (b *SpanAttributeBuilder) WithNamespace(namespace string) *SpanAttributeBuilder {
	if namespace != "" {
		b.attrs = append(b.attrs, attribute.String(SpanAttrNamespace, namespace))
	}
	return b
}

// WithResource adds Kubernetes resource attributes.
func (b *SpanAttributeBuilder) WithResource(resourceType, resourceName string) *SpanAttributeBuilder {
	if resourceType != "" {
		b.attrs = append(b.attrs, attribute.String(SpanAttrResourceType, resourceType))
	}
	if resourceName != "" {
		b.attrs = append(b.attrs, attribute.String(SpanAttrResourceName, resourceName))
	}
	return b
}

// WithOperation adds the operation type attribute.
func (b *SpanAttributeBuilder) WithOperation(operation string) *SpanAttributeBuilder {
	b.attrs = append(b.attrs, attribute.String(SpanAttrOperation, operation))
	return b
}

// WithCacheHit adds the cache hit indicator attribute.
func (b *SpanAttributeBuilder) WithCacheHit(hit bool) *SpanAttributeBuilder {
	b.attrs = append(b.attrs, attribute.Bool(SpanAttrCacheHit, hit))
	return b
}

// WithImpersonated adds the impersonation indicator attribute.
func (b *SpanAttributeBuilder) WithImpersonated(impersonated bool) *SpanAttributeBuilder {
	b.attrs = append(b.attrs, attribute.Bool(SpanAttrImpersonated, impersonated))
	return b
}

// WithFederated adds the federation indicator attribute.
func (b *SpanAttributeBuilder) WithFederated(federated bool) *SpanAttributeBuilder {
	b.attrs = append(b.attrs, attribute.Bool(SpanAttrFederated, federated))
	return b
}

// Build returns the constructed attributes.
func (b *SpanAttributeBuilder) Build() []attribute.KeyValue {
	return b.attrs
}

// StartSpan starts a new span with the given name and attributes.
// Returns the context with the span and the span itself.
// The caller is responsible for ending the span with defer span.End().
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.GetTracerProvider().Tracer(TracerName)
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartToolSpan starts a span for an MCP tool invocation.
// Automatically adds tool name and sets appropriate span kind.
func StartToolSpan(ctx context.Context, toolName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+1)
	allAttrs = append(allAttrs, attribute.String(SpanAttrTool, toolName))
	allAttrs = append(allAttrs, attrs...)

	tracer := otel.GetTracerProvider().Tracer(TracerName)
	return tracer.Start(ctx, "tool."+toolName,
		trace.WithAttributes(allAttrs...),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartFederationSpan starts a span for federation operations.
// Includes cluster attributes and sets appropriate span kind.
func StartFederationSpan(ctx context.Context, operation, clusterName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+3)
	allAttrs = append(allAttrs,
		attribute.String(SpanAttrOperation, operation),
		attribute.String(SpanAttrCluster, clusterName),
		attribute.String(SpanAttrClusterType, ClassifyClusterName(clusterName)),
	)
	allAttrs = append(allAttrs, attrs...)

	tracer := otel.GetTracerProvider().Tracer(TracerName)
	return tracer.Start(ctx, "federation."+operation,
		trace.WithAttributes(allAttrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartK8sSpan starts a span for Kubernetes API operations.
// Includes operation and resource attributes.
func StartK8sSpan(ctx context.Context, operation, resourceType, namespace string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+3)
	allAttrs = append(allAttrs, attribute.String(SpanAttrOperation, operation))
	if resourceType != "" {
		allAttrs = append(allAttrs, attribute.String(SpanAttrResourceType, resourceType))
	}
	if namespace != "" {
		allAttrs = append(allAttrs, attribute.String(SpanAttrNamespace, namespace))
	}
	allAttrs = append(allAttrs, attrs...)

	tracer := otel.GetTracerProvider().Tracer(TracerName)
	return tracer.Start(ctx, "k8s."+operation,
		trace.WithAttributes(allAttrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// SetSpanError records an error on the span and sets the status to error.
func SetSpanError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanSuccess sets the span status to OK.
func SetSpanSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "")
}

// AddSpanEvent adds an event to the span with optional attributes.
func AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// GetTraceID returns the trace ID from the current span in context.
// Returns empty string if no valid span is present.
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID returns the span ID from the current span in context.
// Returns empty string if no valid span is present.
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// SpanContextString returns a human-readable trace context string.
// Format: "trace_id=X span_id=Y" or empty string if no valid context.
func SpanContextString(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return "trace_id=" + span.SpanContext().TraceID().String() +
		" span_id=" + span.SpanContext().SpanID().String()
}
