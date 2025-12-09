package instrumentation

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// Test constants to reduce string repetition and satisfy goconst
const (
	testEmail       = "jane@giantswarm.io"
	testDomain      = "giantswarm.io"
	testCluster     = "prod-wc-01"
	testTraceID     = "abc123def456"
	testSpanID      = "span789"
	testNamespace   = "production"
	testToolGet     = "kubernetes_get"
	testToolDelete  = "kubernetes_delete"
	testToolList    = "kubernetes_list"
	testResourcePod = "pods"
)

func TestToolInvocation_NewAndComplete(t *testing.T) {
	ti := NewToolInvocation(testToolGet)

	// Verify initial state
	if ti.Tool != testToolGet {
		t.Errorf("Tool = %q, want %q", ti.Tool, testToolGet)
	}
	if ti.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}

	// Complete the invocation - duration should be calculated from StartTime
	ti.CompleteSuccess()

	if !ti.Success {
		t.Error("Success should be true")
	}
	// Duration is calculated from StartTime, so it should be >= 0
	// We don't check for > 0 as the test may complete instantly
	if ti.Duration < 0 {
		t.Error("Duration should not be negative")
	}
	if ti.Error != "" {
		t.Errorf("Error should be empty, got %q", ti.Error)
	}
}

func TestToolInvocation_CompleteWithError(t *testing.T) {
	ti := NewToolInvocation(testToolDelete)
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
	ti := NewToolInvocation(testToolGet)
	ti.WithUser(testEmail, []string{"team-a", "admins"})

	if ti.UserEmail != testEmail {
		t.Errorf("UserEmail = %q, want %q", ti.UserEmail, testEmail)
	}
	if len(ti.Groups) != 2 {
		t.Errorf("Groups length = %d, want 2", len(ti.Groups))
	}
}

func TestToolInvocation_WithCluster(t *testing.T) {
	ti := NewToolInvocation(testToolGet)
	ti.WithCluster(testCluster)

	if ti.ClusterName != testCluster {
		t.Errorf("ClusterName = %q, want %q", ti.ClusterName, testCluster)
	}
}

func TestToolInvocation_WithResource(t *testing.T) {
	ti := NewToolInvocation(testToolGet)
	ti.WithResource(testNamespace, testResourcePod, "nginx-abc123")

	if ti.Namespace != testNamespace {
		t.Errorf("Namespace = %q, want %q", ti.Namespace, testNamespace)
	}
	if ti.ResourceType != testResourcePod {
		t.Errorf("ResourceType = %q, want %q", ti.ResourceType, testResourcePod)
	}
	if ti.ResourceName != "nginx-abc123" {
		t.Errorf("ResourceName = %q, want %q", ti.ResourceName, "nginx-abc123")
	}
}

func TestToolInvocation_UserDomain(t *testing.T) {
	ti := NewToolInvocation("test")
	ti.UserEmail = testEmail

	if domain := ti.UserDomain(); domain != testDomain {
		t.Errorf("UserDomain() = %q, want %q", domain, testDomain)
	}
}

func TestToolInvocation_ClusterType(t *testing.T) {
	tests := []struct {
		clusterName  string
		expectedType string
	}{
		{"", "management"},
		{testCluster, "production"},
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
	if status := ti.Status(); status != StatusSuccess {
		t.Errorf("Status() = %q, want %q", status, StatusSuccess)
	}

	ti.Success = false
	if status := ti.Status(); status != StatusError {
		t.Errorf("Status() = %q, want %q", status, StatusError)
	}
}

func TestToolInvocation_LogAttrs(t *testing.T) {
	ti := NewToolInvocation(testToolDelete)
	ti.WithUser(testEmail, []string{"team-a"}).
		WithCluster(testCluster).
		WithResource(testNamespace, testResourcePod, "nginx-abc123").
		CompleteSuccess()
	ti.TraceID = testTraceID

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
	if domain := attrMap["user_domain"].Value.String(); domain != testDomain {
		t.Errorf("user_domain = %q, want %q", domain, testDomain)
	}
	if ct := attrMap["cluster_type"].Value.String(); ct != "production" {
		t.Errorf("cluster_type = %q, want %q", ct, "production")
	}
}

func TestToolInvocation_LogAttrs_WithError(t *testing.T) {
	ti := NewToolInvocation(testToolDelete)
	ti.WithUser(testEmail, []string{"team-a"}).
		WithCluster(testCluster).
		CompleteWithError(errors.New("test error"))

	attrs := ti.LogAttrs()

	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// Check error attribute is present
	if _, ok := attrMap["error"]; !ok {
		t.Error("Missing error attribute")
	}
	if errVal := attrMap["error"].Value.String(); errVal != "test error" {
		t.Errorf("error = %q, want %q", errVal, "test error")
	}
}

func TestToolInvocation_LogAttrs_MinimalFields(t *testing.T) {
	ti := NewToolInvocation(testToolGet)
	ti.CompleteSuccess()

	attrs := ti.LogAttrs()

	// Verify minimal attributes are present
	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// These should NOT be present when not set
	if _, ok := attrMap["namespace"]; ok {
		t.Error("namespace should not be present when empty")
	}
	if _, ok := attrMap["resource_type"]; ok {
		t.Error("resource_type should not be present when empty")
	}
	if _, ok := attrMap["trace_id"]; ok {
		t.Error("trace_id should not be present when empty")
	}
}

func TestToolInvocation_LogAuditAttrs(t *testing.T) {
	ti := NewToolInvocation(testToolDelete)
	ti.WithUser(testEmail, []string{"team-a"}).
		WithCluster(testCluster).
		WithResource(testNamespace, testResourcePod, "nginx-abc123").
		CompleteSuccess()
	ti.TraceID = testTraceID
	ti.SpanID = testSpanID

	attrs := ti.LogAuditAttrs()

	// Verify we have the expected attributes
	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// Check that full values are present (not cardinality-controlled)
	if user := attrMap["user"].Value.String(); user != testEmail {
		t.Errorf("user = %q, want %q", user, testEmail)
	}
	if cluster := attrMap["cluster"].Value.String(); cluster != testCluster {
		t.Errorf("cluster = %q, want %q", cluster, testCluster)
	}

	// Check trace context
	if traceID := attrMap["trace_id"].Value.String(); traceID != testTraceID {
		t.Errorf("trace_id = %q, want %q", traceID, testTraceID)
	}
	if spanID := attrMap["span_id"].Value.String(); spanID != testSpanID {
		t.Errorf("span_id = %q, want %q", spanID, testSpanID)
	}
}

func TestToolInvocation_LogAuditAttrs_WithError(t *testing.T) {
	ti := NewToolInvocation(testToolDelete)
	ti.WithUser(testEmail, []string{"team-a"}).
		WithCluster(testCluster).
		CompleteWithError(errors.New("audit error"))

	attrs := ti.LogAuditAttrs()

	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// Check error attribute is present
	if _, ok := attrMap["error"]; !ok {
		t.Error("Missing error attribute")
	}
}

func TestToolInvocation_LogAuditAttrs_MinimalFields(t *testing.T) {
	ti := NewToolInvocation(testToolGet)
	ti.CompleteSuccess()

	attrs := ti.LogAuditAttrs()

	attrMap := make(map[string]slog.Attr)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr
	}

	// These should NOT be present when not set
	if _, ok := attrMap["namespace"]; ok {
		t.Error("namespace should not be present when empty")
	}
	if _, ok := attrMap["resource_name"]; ok {
		t.Error("resource_name should not be present when empty")
	}
}

func TestToolInvocation_MethodChaining(t *testing.T) {
	ti := NewToolInvocation(testToolList).
		WithUser("user@example.com", []string{"group"}).
		WithCluster("staging-cluster").
		WithResource("default", "deployments", "").
		CompleteSuccess()

	if ti.Tool != testToolList {
		t.Errorf("Tool = %q, want %q", ti.Tool, testToolList)
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

func TestAuditLogger_LogToolInvocation_Success(t *testing.T) {
	// This test verifies the method runs without panic
	al := NewAuditLogger(slog.Default())
	ti := NewToolInvocation(testToolGet).
		WithUser(testEmail, []string{"group"}).
		WithCluster(testCluster).
		CompleteSuccess()

	// Should not panic
	al.LogToolInvocation(ti)
}

func TestAuditLogger_LogToolInvocation_Failure(t *testing.T) {
	// This test verifies the method runs without panic for failures
	al := NewAuditLogger(slog.Default())
	ti := NewToolInvocation(testToolDelete).
		WithUser(testEmail, []string{"group"}).
		WithCluster(testCluster).
		CompleteWithError(errors.New("test error"))

	// Should not panic
	al.LogToolInvocation(ti)
}

func TestAuditLogger_LogToolAudit(t *testing.T) {
	// This test verifies the method runs without panic
	al := NewAuditLogger(slog.Default())
	ti := NewToolInvocation(testToolDelete).
		WithUser(testEmail, []string{"group"}).
		WithCluster(testCluster).
		WithResource(testNamespace, testResourcePod, "nginx-abc123").
		CompleteSuccess()
	ti.TraceID = testTraceID

	// Should not panic
	al.LogToolAudit(ti)
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

func TestToolInvocation_Complete_WithError(t *testing.T) {
	ti := NewToolInvocation("test")
	ti.Complete(false, errors.New("some error"))

	if ti.Success {
		t.Error("Success should be false")
	}
	if ti.Error != "some error" {
		t.Errorf("Error = %q, want %q", ti.Error, "some error")
	}
}
