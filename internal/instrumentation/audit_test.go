package instrumentation

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestToolInvocation_NewAndComplete(t *testing.T) {
	ti := NewToolInvocation("kubernetes_get")

	// Verify initial state
	if ti.Tool != "kubernetes_get" {
		t.Errorf("Tool = %q, want %q", ti.Tool, "kubernetes_get")
	}
	if ti.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}

	// Complete the invocation
	time.Sleep(1 * time.Millisecond) // Ensure some duration
	ti.CompleteSuccess()

	if !ti.Success {
		t.Error("Success should be true")
	}
	if ti.Duration == 0 {
		t.Error("Duration should be non-zero")
	}
	if ti.Error != "" {
		t.Errorf("Error should be empty, got %q", ti.Error)
	}
}

func TestToolInvocation_CompleteWithError(t *testing.T) {
	ti := NewToolInvocation("kubernetes_delete")
	err := errors.New("permission denied")

	ti.CompleteWithError(err)

	if ti.Success {
		t.Error("Success should be false")
	}
	if ti.Error != "permission denied" {
		t.Errorf("Error = %q, want %q", ti.Error, "permission denied")
	}
}

func TestToolInvocation_WithUser(t *testing.T) {
	ti := NewToolInvocation("kubernetes_get")
	ti.WithUser("jane@giantswarm.io", []string{"team-a", "admins"})

	if ti.UserEmail != "jane@giantswarm.io" {
		t.Errorf("UserEmail = %q, want %q", ti.UserEmail, "jane@giantswarm.io")
	}
	if len(ti.Groups) != 2 {
		t.Errorf("Groups length = %d, want 2", len(ti.Groups))
	}
}

func TestToolInvocation_WithCluster(t *testing.T) {
	ti := NewToolInvocation("kubernetes_get")
	ti.WithCluster("prod-wc-01")

	if ti.ClusterName != "prod-wc-01" {
		t.Errorf("ClusterName = %q, want %q", ti.ClusterName, "prod-wc-01")
	}
}

func TestToolInvocation_WithResource(t *testing.T) {
	ti := NewToolInvocation("kubernetes_get")
	ti.WithResource("production", "pods", "nginx-abc123")

	if ti.Namespace != "production" {
		t.Errorf("Namespace = %q, want %q", ti.Namespace, "production")
	}
	if ti.ResourceType != "pods" {
		t.Errorf("ResourceType = %q, want %q", ti.ResourceType, "pods")
	}
	if ti.ResourceName != "nginx-abc123" {
		t.Errorf("ResourceName = %q, want %q", ti.ResourceName, "nginx-abc123")
	}
}

func TestToolInvocation_UserDomain(t *testing.T) {
	ti := NewToolInvocation("test")
	ti.UserEmail = "jane@giantswarm.io"

	if domain := ti.UserDomain(); domain != "giantswarm.io" {
		t.Errorf("UserDomain() = %q, want %q", domain, "giantswarm.io")
	}
}

func TestToolInvocation_ClusterType(t *testing.T) {
	tests := []struct {
		clusterName  string
		expectedType string
	}{
		{"", "management"},
		{"prod-wc-01", "production"},
		{"staging-test", "staging"},
		{"dev-cluster", "development"},
		{"my-cluster", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.clusterName, func(t *testing.T) {
			ti := NewToolInvocation("test")
			ti.ClusterName = tt.clusterName

			if ct := ti.ClusterType(); ct != tt.expectedType {
				t.Errorf("ClusterType() = %q, want %q", ct, tt.expectedType)
			}
		})
	}
}

func TestToolInvocation_Status(t *testing.T) {
	ti := NewToolInvocation("test")

	ti.Success = true
	if status := ti.Status(); status != "success" {
		t.Errorf("Status() = %q, want %q", status, "success")
	}

	ti.Success = false
	if status := ti.Status(); status != "error" {
		t.Errorf("Status() = %q, want %q", status, "error")
	}
}

func TestToolInvocation_LogAttrs(t *testing.T) {
	ti := NewToolInvocation("kubernetes_delete")
	ti.WithUser("jane@giantswarm.io", []string{"team-a"}).
		WithCluster("prod-wc-01").
		WithResource("production", "pods", "nginx-abc123").
		CompleteSuccess()
	ti.TraceID = "abc123def456"

	attrs := ti.LogAttrs()

	// Verify we have the expected attributes
	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// Check required attributes
	requiredKeys := []string{"tool", "user_domain", "group_count", "cluster_type", "duration", "success"}
	for _, key := range requiredKeys {
		if _, ok := attrMap[key]; !ok {
			t.Errorf("Missing required attribute: %s", key)
		}
	}

	// Check cardinality-controlled values
	if domain := attrMap["user_domain"].Value.String(); domain != "giantswarm.io" {
		t.Errorf("user_domain = %q, want %q", domain, "giantswarm.io")
	}
	if ct := attrMap["cluster_type"].Value.String(); ct != "production" {
		t.Errorf("cluster_type = %q, want %q", ct, "production")
	}
}

func TestToolInvocation_LogAuditAttrs(t *testing.T) {
	ti := NewToolInvocation("kubernetes_delete")
	ti.WithUser("jane@giantswarm.io", []string{"team-a"}).
		WithCluster("prod-wc-01").
		WithResource("production", "pods", "nginx-abc123").
		CompleteSuccess()
	ti.TraceID = "abc123def456"
	ti.SpanID = "span789"

	attrs := ti.LogAuditAttrs()

	// Verify we have the expected attributes
	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// Check that full values are present (not cardinality-controlled)
	if user := attrMap["user"].Value.String(); user != "jane@giantswarm.io" {
		t.Errorf("user = %q, want %q", user, "jane@giantswarm.io")
	}
	if cluster := attrMap["cluster"].Value.String(); cluster != "prod-wc-01" {
		t.Errorf("cluster = %q, want %q", cluster, "prod-wc-01")
	}

	// Check trace context
	if traceID := attrMap["trace_id"].Value.String(); traceID != "abc123def456" {
		t.Errorf("trace_id = %q, want %q", traceID, "abc123def456")
	}
	if spanID := attrMap["span_id"].Value.String(); spanID != "span789" {
		t.Errorf("span_id = %q, want %q", spanID, "span789")
	}
}

func TestToolInvocation_MethodChaining(t *testing.T) {
	ti := NewToolInvocation("kubernetes_list").
		WithUser("user@example.com", []string{"group"}).
		WithCluster("staging-cluster").
		WithResource("default", "deployments", "").
		CompleteSuccess()

	if ti.Tool != "kubernetes_list" {
		t.Errorf("Tool = %q, want %q", ti.Tool, "kubernetes_list")
	}
	if ti.UserEmail != "user@example.com" {
		t.Errorf("UserEmail = %q, want %q", ti.UserEmail, "user@example.com")
	}
	if ti.ClusterName != "staging-cluster" {
		t.Errorf("ClusterName = %q, want %q", ti.ClusterName, "staging-cluster")
	}
	if !ti.Success {
		t.Error("Success should be true")
	}
}

func TestAuditLogger_New(t *testing.T) {
	// Test with nil logger (should use default)
	al := NewAuditLogger(nil)
	if al.logger == nil {
		t.Error("logger should not be nil when created with nil")
	}

	// Test with custom logger
	logger := slog.Default()
	al = NewAuditLogger(logger)
	if al.logger != logger {
		t.Error("logger should be the provided logger")
	}
}

func TestTraceIDFromContext_NoSpan(t *testing.T) {
	ctx := context.Background()
	traceID := TraceIDFromContext(ctx)

	if traceID != "" {
		t.Errorf("TraceIDFromContext with no span = %q, want empty string", traceID)
	}
}

func TestToolInvocation_WithSpanContext_NoSpan(t *testing.T) {
	ctx := context.Background()
	ti := NewToolInvocation("test").WithSpanContext(ctx)

	if ti.TraceID != "" {
		t.Errorf("TraceID = %q, want empty string", ti.TraceID)
	}
	if ti.SpanID != "" {
		t.Errorf("SpanID = %q, want empty string", ti.SpanID)
	}
}

func TestToolInvocation_Complete_NilError(t *testing.T) {
	ti := NewToolInvocation("test")
	ti.Complete(true, nil)

	if ti.Error != "" {
		t.Errorf("Error = %q, want empty string", ti.Error)
	}
}
