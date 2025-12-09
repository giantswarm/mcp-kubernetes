package instrumentation

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
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
		builder := NewSpanAttributeBuilder().WithTool("kubernetes_get")
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Key != SpanAttrTool {
			t.Errorf("Expected key %q, got %q", SpanAttrTool, attrs[0].Key)
		}
		if attrs[0].Value.AsString() != "kubernetes_get" {
			t.Errorf("Expected value %q, got %q", "kubernetes_get", attrs[0].Value.AsString())
		}
	})

	t.Run("with cluster", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithCluster("prod-wc-01")
		attrs := builder.Build()

		if len(attrs) != 2 {
			t.Fatalf("Expected 2 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if attrMap[SpanAttrCluster].AsString() != "prod-wc-01" {
			t.Errorf("Expected cluster %q, got %q", "prod-wc-01", attrMap[SpanAttrCluster].AsString())
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
		builder := NewSpanAttributeBuilder().WithUser("jane@giantswarm.io", []string{"team-a", "admins"}, true)
		attrs := builder.Build()

		if len(attrs) != 3 {
			t.Fatalf("Expected 3 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if attrMap[SpanAttrUserEmail].AsString() != "jane@giantswarm.io" {
			t.Errorf("Expected email %q, got %q", "jane@giantswarm.io", attrMap[SpanAttrUserEmail].AsString())
		}
		if attrMap[SpanAttrUserDomain].AsString() != "giantswarm.io" {
			t.Errorf("Expected domain %q, got %q", "giantswarm.io", attrMap[SpanAttrUserDomain].AsString())
		}
		if attrMap[SpanAttrGroupCount].AsInt64() != 2 {
			t.Errorf("Expected group_count 2, got %d", attrMap[SpanAttrGroupCount].AsInt64())
		}
	})

	t.Run("with user excluding email", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithUser("jane@giantswarm.io", []string{"team-a"}, false)
		attrs := builder.Build()

		if len(attrs) != 2 {
			t.Fatalf("Expected 2 attributes, got %d", len(attrs))
		}

		attrMap := attrsToMap(attrs)
		if _, ok := attrMap[SpanAttrUserEmail]; ok {
			t.Error("Should not include email when includeEmail is false")
		}
		if attrMap[SpanAttrUserDomain].AsString() != "giantswarm.io" {
			t.Errorf("Expected domain %q, got %q", "giantswarm.io", attrMap[SpanAttrUserDomain].AsString())
		}
	})

	t.Run("with namespace", func(t *testing.T) {
		builder := NewSpanAttributeBuilder().WithNamespace("production")
		attrs := builder.Build()

		if len(attrs) != 1 {
			t.Fatalf("Expected 1 attribute, got %d", len(attrs))
		}
		if attrs[0].Value.AsString() != "production" {
			t.Errorf("Expected namespace %q, got %q", "production", attrs[0].Value.AsString())
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
			WithTool("kubernetes_delete").
			WithCluster("prod-wc-01").
			WithUser("jane@giantswarm.io", []string{"admin"}, false).
			WithNamespace("production").
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

// Helper function to convert attributes slice to map for easier testing
func attrsToMap(attrs []attribute.KeyValue) map[attribute.Key]attribute.Value {
	m := make(map[attribute.Key]attribute.Value)
	for _, attr := range attrs {
		m[attr.Key] = attr.Value
	}
	return m
}
