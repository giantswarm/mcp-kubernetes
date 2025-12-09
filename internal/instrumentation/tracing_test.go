package instrumentation

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Test constants for tracing tests
const (
	tracingTestEmail      = "jane@giantswarm.io"
	tracingTestDomain     = "giantswarm.io"
	tracingTestCluster    = "prod-wc-01"
	tracingTestNamespace  = "production"
	tracingTestToolGet    = "kubernetes_get"
	tracingTestToolDelete = "kubernetes_delete"
)

func TestSpanAttributeBuilder(t *testing.T) {
	t.Run("empty builder", func(t *testing.T) {
		builder := NewSpanAttributeBuilder()
		attrs := builder.Build()
		if len(attrs) != 0 {
			t.Errorf("Empty builder should return 0 attributes, got %d", len(attrs))
		}
	})

	t.Run("with tool", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithTool(tracingTestToolGet)
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Key != SpanAttrTool {
			t.Errorf("Expected key %q, got %q", SpanAttrTool, attrs[0].Key)
		}
		if attrs[0].Value.AsString() != tracingTestToolGet {
			t.Errorf("Expected value %q, got %q", tracingTestToolGet, attrs[0].Value.AsString())
		}
	})

	t.Run("with cluster", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithCluster(tracingTestCluster)
		attrs := builder.Build()

		if len(attrs) != 2 {
			t.Fatalf("Expected 2 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if attrMap[SpanAttrCluster].AsString() != tracingTestCluster {
			t.Errorf("Expected cluster %q, got %q", tracingTestCluster, attrMap[SpanAttrCluster].AsString())
		}
		if attrMap[SpanAttrClusterType].AsString() != "production" {
			t.Errorf("Expected cluster_type %q, got %q", "production", attrMap[SpanAttrClusterType].AsString())
		}
	})

	t.Run("with cluster type only", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithClusterType("staging-test")
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Key != SpanAttrClusterType {
			t.Errorf("Expected key %q, got %q", SpanAttrClusterType, attrs[0].Key)
		}
		if attrs[0].Value.AsString() != "staging" {
			t.Errorf("Expected value %q, got %q", "staging", attrs[0].Value.AsString())
		}
	})

	t.Run("with user including email", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithUser(tracingTestEmail, []string{"team-a", "admins"}, true)
		attrs := builder.Build()

		if len(attrs) != 3 {
			t.Fatalf("Expected 3 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if attrMap[SpanAttrUserEmail].AsString() != tracingTestEmail {
			t.Errorf("Expected email %q, got %q", tracingTestEmail, attrMap[SpanAttrUserEmail].AsString())
		}
		if attrMap[SpanAttrUserDomain].AsString() != tracingTestDomain {
			t.Errorf("Expected domain %q, got %q", tracingTestDomain, attrMap[SpanAttrUserDomain].AsString())
		}
		if attrMap[SpanAttrGroupCount].AsInt64() != 2 {
			t.Errorf("Expected group_count 2, got %d", attrMap[SpanAttrGroupCount].AsInt64())
		}
	})

	t.Run("with user excluding email", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithUser(tracingTestEmail, []string{"team-a"}, false)
		attrs := builder.Build()

		if len(attrs) != 2 {
			t.Fatalf("Expected 2 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if _, ok := attrMap[SpanAttrUserEmail]; ok {
			t.Error("Should not include email when includeEmail is false")
		}
		if attrMap[SpanAttrUserDomain].AsString() != tracingTestDomain {
			t.Errorf("Expected domain %q, got %q", tracingTestDomain, attrMap[SpanAttrUserDomain].AsString())
		}
	})

	t.Run("with namespace", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithNamespace(tracingTestNamespace)
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Value.AsString() != tracingTestNamespace {
			t.Errorf("Expected namespace %q, got %q", tracingTestNamespace, attrs[0].Value.AsString())
		}
	})

	t.Run("with empty namespace", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithNamespace("")
		attrs := builder.Build()

		if len(attrs) != 0 {
			t.Errorf("Expected 0 attributes for empty namespace, got %d", len(attrs))
		}
	})

	t.Run("with resource", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithResource("pods", "nginx-abc123")
		attrs := builder.Build()

		if len(attrs) != 2 {
			t.Fatalf("Expected 2 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if attrMap[SpanAttrResourceType].AsString() != "pods" {
			t.Errorf("Expected resource_type %q, got %q", "pods", attrMap[SpanAttrResourceType].AsString())
		}
		if attrMap[SpanAttrResourceName].AsString() != "nginx-abc123" {
			t.Errorf("Expected resource_name %q, got %q", "nginx-abc123", attrMap[SpanAttrResourceName].AsString())
		}
	})

	t.Run("with empty resource type", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithResource("", "nginx-abc123")
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		attrMap := attrsToMap(attrs)
		if _, ok := attrMap[SpanAttrResourceType]; ok {
			t.Error("Should not include resource_type when empty")
		}
	})

	t.Run("with empty resource name", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithResource("pods", "")
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		attrMap := attrsToMap(attrs)
		if _, ok := attrMap[SpanAttrResourceName]; ok {
			t.Error("Should not include resource_name when empty")
		}
	})

	t.Run("with operation", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithOperation("delete")
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Value.AsString() != "delete" {
			t.Errorf("Expected operation %q, got %q", "delete", attrs[0].Value.AsString())
		}
	})

	t.Run("with cache hit", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithCacheHit(true)
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Value.AsBool() != true {
			t.Errorf("Expected cache_hit true, got %v", attrs[0].Value.AsBool())
		}
	})

	t.Run("with impersonated", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithImpersonated(true)
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Value.AsBool() != true {
			t.Errorf("Expected impersonated true, got %v", attrs[0].Value.AsBool())
		}
	})

	t.Run("with federated", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithFederated(true)
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Value.AsBool() != true {
			t.Errorf("Expected federated true, got %v", attrs[0].Value.AsBool())
		}
	})

	t.Run("method chaining", func(t *testing.T) {
		attrs := NewSpanAttributeBuilder().
			WithTool(tracingTestToolDelete).
			WithCluster(tracingTestCluster).
			WithUser(tracingTestEmail, []string{"admin"}, false).
			WithNamespace(tracingTestNamespace).
			WithResource("pods", "nginx").
			WithOperation("delete").
			WithCacheHit(false).
			WithImpersonated(true).
			WithFederated(true).
			Build()

		// 1 tool + 2 cluster + 2 user + 1 namespace + 2 resource + 1 operation + 1 cache + 1 impersonated + 1 federated = 12
		if len(attrs) != 12 {
			t.Errorf("Expected 12 attributes, got %d", len(attrs))
		}
	})
}

func TestGetTraceID_NoSpan(t *testing.T) {
	ctx := context.Background()
	traceID := GetTraceID(ctx)

	if traceID != "" {
		t.Errorf("GetTraceID with no span = %q, want empty string", traceID)
	}
}

func TestGetSpanID_NoSpan(t *testing.T) {
	ctx := context.Background()
	spanID := GetSpanID(ctx)

	if spanID != "" {
		t.Errorf("GetSpanID with no span = %q, want empty string", spanID)
	}
}

func TestSpanContextString_NoSpan(t *testing.T) {
	ctx := context.Background()
	result := SpanContextString(ctx)

	if result != "" {
		t.Errorf("SpanContextString with no span = %q, want empty string", result)
	}
}

func TestSpanAttributeConstants(t *testing.T) {
	// Verify constants are defined with expected values
	expectedValues := map[string]string{
		"SpanAttrCluster":      "mcp.cluster",
		"SpanAttrClusterType":  "mcp.cluster_type",
		"SpanAttrUserEmail":    "mcp.user.email",
		"SpanAttrUserDomain":   "mcp.user.domain",
		"SpanAttrGroupCount":   "mcp.user.group_count",
		"SpanAttrTool":         "mcp.tool",
		"SpanAttrNamespace":    "k8s.namespace",
		"SpanAttrResourceType": "k8s.resource_type",
		"SpanAttrResourceName": "k8s.resource_name",
		"SpanAttrOperation":    "k8s.operation",
		"SpanAttrCacheHit":     "mcp.cache_hit",
		"SpanAttrImpersonated": "mcp.impersonated",
		"SpanAttrFederated":    "mcp.federated",
	}

	actualValues := map[string]string{
		"SpanAttrCluster":      SpanAttrCluster,
		"SpanAttrClusterType":  SpanAttrClusterType,
		"SpanAttrUserEmail":    SpanAttrUserEmail,
		"SpanAttrUserDomain":   SpanAttrUserDomain,
		"SpanAttrGroupCount":   SpanAttrGroupCount,
		"SpanAttrTool":         SpanAttrTool,
		"SpanAttrNamespace":    SpanAttrNamespace,
		"SpanAttrResourceType": SpanAttrResourceType,
		"SpanAttrResourceName": SpanAttrResourceName,
		"SpanAttrOperation":    SpanAttrOperation,
		"SpanAttrCacheHit":     SpanAttrCacheHit,
		"SpanAttrImpersonated": SpanAttrImpersonated,
		"SpanAttrFederated":    SpanAttrFederated,
	}

	for name, expected := range expectedValues {
		if actual := actualValues[name]; actual != expected {
			t.Errorf("%s = %q, want %q", name, actual, expected)
		}
	}
}

func TestTracerNameConstant(t *testing.T) {
	if TracerName != "github.com/giantswarm/mcp-kubernetes" {
		t.Errorf("TracerName = %q, want %q", TracerName, "github.com/giantswarm/mcp-kubernetes")
	}
}

// Helper function to create a test span and context
func createTestSpanContext() (context.Context, trace.Span, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	tracer := tp.Tracer(TracerName)
	ctx, span := tracer.Start(context.Background(), "test-span")

	return ctx, span, exporter
}

func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	spanCtx, span := StartSpan(ctx, "test-operation", attribute.String("key", "value"))
	defer span.End()

	if spanCtx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}
}

func TestStartToolSpan(t *testing.T) {
	ctx := context.Background()
	spanCtx, span := StartToolSpan(ctx, tracingTestToolGet, attribute.String("extra", "attr"))
	defer span.End()

	if spanCtx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}
}

func TestStartFederationSpan(t *testing.T) {
	ctx := context.Background()
	spanCtx, span := StartFederationSpan(ctx, "GetClient", tracingTestCluster)
	defer span.End()

	if spanCtx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}
}

func TestStartK8sSpan(t *testing.T) {
	ctx := context.Background()
	spanCtx, span := StartK8sSpan(ctx, "list", "pods", tracingTestNamespace)
	defer span.End()

	if spanCtx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}
}

func TestStartK8sSpan_EmptyOptionalFields(t *testing.T) {
	ctx := context.Background()
	spanCtx, span := StartK8sSpan(ctx, "list", "", "")
	defer span.End()

	if spanCtx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}
}

func TestSetSpanError(t *testing.T) {
	ctx, span, _ := createTestSpanContext()
	defer span.End()

	testErr := errors.New("test error")
	SetSpanError(span, testErr)

	// Verify the span has error status
	// We can't easily check the status from the span interface,
	// but we can verify the function doesn't panic
	_ = ctx
}

func TestSetSpanError_NilError(t *testing.T) {
	_, span, _ := createTestSpanContext()
	defer span.End()

	// Should not panic with nil error
	SetSpanError(span, nil)
}

func TestSetSpanSuccess(t *testing.T) {
	_, span, _ := createTestSpanContext()
	defer span.End()

	// Should not panic
	SetSpanSuccess(span)
}

func TestAddSpanEvent(t *testing.T) {
	_, span, _ := createTestSpanContext()
	defer span.End()

	// Should not panic
	AddSpanEvent(span, "test-event", attribute.String("key", "value"))
}

func TestAddSpanEvent_NoAttrs(t *testing.T) {
	_, span, _ := createTestSpanContext()
	defer span.End()

	// Should not panic
	AddSpanEvent(span, "test-event")
}

func TestGetTraceID_WithSpan(t *testing.T) {
	ctx, span, _ := createTestSpanContext()
	defer span.End()

	traceID := GetTraceID(ctx)

	if traceID == "" {
		t.Error("TraceID should not be empty when span is present")
	}
	// Verify it's a valid hex string (32 chars for trace ID)
	if len(traceID) != 32 {
		t.Errorf("TraceID should be 32 chars, got %d", len(traceID))
	}
}

func TestGetSpanID_WithSpan(t *testing.T) {
	ctx, span, _ := createTestSpanContext()
	defer span.End()

	spanID := GetSpanID(ctx)

	if spanID == "" {
		t.Error("SpanID should not be empty when span is present")
	}
	// Verify it's a valid hex string (16 chars for span ID)
	if len(spanID) != 16 {
		t.Errorf("SpanID should be 16 chars, got %d", len(spanID))
	}
}

func TestSpanContextString_WithSpan(t *testing.T) {
	ctx, span, _ := createTestSpanContext()
	defer span.End()

	result := SpanContextString(ctx)

	if result == "" {
		t.Error("SpanContextString should not be empty when span is present")
	}

	// Should contain both trace_id and span_id
	if len(result) < 50 { // "trace_id=" + 32 + " span_id=" + 16 = 59 chars minimum
		t.Errorf("SpanContextString too short: %q", result)
	}
}

// Helper function to convert attributes slice to map for easier testing
func attrsToMap(attrs []attribute.KeyValue) map[attribute.Key]attribute.Value {
	m := make(map[attribute.Key]attribute.Value)
	for _, attr := range attrs {
		m[attr.Key] = attr.Value
	}
	return m
}

// Test that SetSpanError correctly sets error status
func TestSetSpanError_SetsErrorCode(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	tracer := tp.Tracer(TracerName)

	_, span := tracer.Start(context.Background(), "test-span")
	testErr := errors.New("test error")
	SetSpanError(span, testErr)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("Expected error status code, got %v", spans[0].Status.Code)
	}
}

// Test that SetSpanSuccess correctly sets OK status
func TestSetSpanSuccess_SetsOKCode(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	tracer := tp.Tracer(TracerName)

	_, span := tracer.Start(context.Background(), "test-span")
	SetSpanSuccess(span)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Ok {
		t.Errorf("Expected OK status code, got %v", spans[0].Status.Code)
	}
}
