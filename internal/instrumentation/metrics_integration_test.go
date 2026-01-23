package instrumentation

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// TestAllMetricsExposedViaPrometheus is an integration test that verifies
// ALL metrics defined in metrics.go are properly recorded and exposed via
// the Prometheus /metrics endpoint.
//
// This test is critical for catching issues where:
// 1. A metric is defined but never recorded
// 2. Middleware is not wired up correctly
// 3. The metric registration failed silently
//
// Unlike the shell-based test, this Go test:
// - Doesn't require a running server or Kubernetes cluster
// - Can call ALL Record* functions including OAuth/CAPI metrics
// - Runs fast and deterministically in CI
func TestAllMetricsExposedViaPrometheus(t *testing.T) {
	// Note: The OTel prometheus exporter registers to the global Prometheus registry
	// so we use promhttp.Handler() which exposes that global registry.
	// This matches how the actual application exposes metrics.

	// Create instrumentation provider with Prometheus exporter
	config := Config{
		ServiceName:     "test-metrics-integration",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "none",
	}

	ctx := context.Background()
	provider, err := NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create instrumentation provider: %v", err)
	}
	defer func() { _ = provider.Shutdown(ctx) }()

	metrics := provider.Metrics()
	if metrics == nil {
		t.Fatal("Metrics should not be nil")
	}

	// Record ALL metrics at least once to ensure they are exposed
	recordAllMetrics(ctx, metrics)

	// Create a test server to scrape metrics
	// We use promhttp.Handler() which exposes the global Prometheus registry
	// that the OTel prometheus exporter registers to
	server := httptest.NewServer(promhttp.Handler())
	defer server.Close()

	// Fetch metrics
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to fetch metrics: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read metrics body: %v", err)
	}
	metricsOutput := string(body)

	// Define all expected metrics
	// NOTE: These MUST match the metric names in metrics.go
	expectedMetrics := []struct {
		name        string
		description string
		isHistogram bool
	}{
		// HTTP metrics
		{"http_requests_total", "Total number of HTTP requests", false},
		{"http_request_duration_seconds", "HTTP request duration", true},
		{"active_port_forward_sessions", "Active port-forward sessions", false},

		// Kubernetes operation metrics
		{"mcp_kubernetes_operations_total", "Total K8s operations", false},
		{"mcp_kubernetes_operation_duration_seconds", "K8s operation duration", true},
		{"kubernetes_pod_operations_total", "Total pod operations", false},
		{"kubernetes_pod_operation_duration_seconds", "Pod operation duration", true},

		// OAuth metrics
		{"oauth_downstream_auth_total", "OAuth downstream auth attempts", false},
		{"oauth_sso_token_injection_total", "SSO token injection events", false},

		// Client cache metrics
		{"mcp_kubernetes_client_cache_hits_total", "Cache hits", false},
		{"mcp_kubernetes_client_cache_misses_total", "Cache misses", false},
		{"mcp_kubernetes_client_cache_evictions_total", "Cache evictions", false},
		{"mcp_kubernetes_client_cache_entries", "Current cache size", false},

		// CAPI/Federation metrics
		{"mcp_kubernetes_impersonation_total", "Impersonation requests", false},
		{"mcp_kubernetes_federation_client_creations_total", "Federation client creations", false},

		// Privileged access metrics
		{"mcp_kubernetes_privileged_secret_access_total", "Privileged secret access", false},

		// Workload cluster auth metrics
		{"mcp_kubernetes_wc_auth_total", "Workload cluster auth attempts", false},
	}

	// Check each metric
	var missing []string
	for _, m := range expectedMetrics {
		found := false

		// For histograms, Prometheus exposes _bucket, _sum, _count suffixes
		if m.isHistogram {
			// Check for histogram suffixes
			suffixes := []string{"_bucket", "_sum", "_count"}
			for _, suffix := range suffixes {
				pattern := m.name + suffix
				if containsMetric(metricsOutput, pattern) {
					found = true
					break
				}
			}
		} else {
			found = containsMetric(metricsOutput, m.name)
		}

		if found {
			t.Logf("PASS: Found metric %s (%s)", m.name, m.description)
		} else {
			missing = append(missing, m.name)
			t.Errorf("FAIL: Missing metric %s (%s)", m.name, m.description)
		}
	}

	if len(missing) > 0 {
		t.Logf("\n\nMissing metrics: %v", missing)
		t.Log("\nThis likely means:")
		t.Log("  1. The metric is defined but Record*() was never called")
		t.Log("  2. The metric registration failed silently")
		t.Log("  3. The OTel prometheus exporter is not properly configured")
		t.Log("\nCheck internal/instrumentation/metrics.go and ensure all")
		t.Log("metrics are properly registered in NewMetrics()")

		// For debugging, print a sample of the metrics output
		t.Log("\n\nSample of metrics output (first 2000 chars):")
		if len(metricsOutput) > 2000 {
			t.Log(metricsOutput[:2000])
		} else {
			t.Log(metricsOutput)
		}
	}

	// Also verify that metrics without explicit registry work
	// (this tests the global prometheus registry integration)
	if !strings.Contains(metricsOutput, "http_requests_total") {
		t.Error("http_requests_total not found in global prometheus registry")
	}
}

// recordAllMetrics calls every Record* function to ensure all metrics
// are recorded at least once. This is the key to testing OAuth/CAPI metrics
// without needing actual OAuth or CAPI infrastructure.
func recordAllMetrics(ctx context.Context, m *Metrics) {
	// HTTP metrics
	m.RecordHTTPRequest(ctx, "GET", "/healthz", 200, 50*time.Millisecond)
	m.RecordHTTPRequest(ctx, "POST", "/mcp", 200, 100*time.Millisecond)
	m.RecordHTTPRequest(ctx, "POST", "/mcp", 500, 200*time.Millisecond)

	// Port-forward session tracking
	m.IncrementActiveSessions(ctx)
	m.DecrementActiveSessions(ctx)

	// Kubernetes operation metrics
	m.RecordK8sOperation(ctx, "", OperationGet, "pods", "default", StatusSuccess, 50*time.Millisecond)
	m.RecordK8sOperation(ctx, "", OperationList, "namespaces", "", StatusSuccess, 100*time.Millisecond)
	m.RecordK8sOperation(ctx, "", OperationCreate, "configmaps", "kube-system", StatusError, 150*time.Millisecond)

	// Pod operation metrics
	m.RecordPodOperation(ctx, OperationLogs, "default", StatusSuccess, 200*time.Millisecond)
	m.RecordPodOperation(ctx, OperationExec, "kube-system", StatusSuccess, 300*time.Millisecond)

	// OAuth downstream authentication metrics
	m.RecordOAuthDownstreamAuth(ctx, OAuthResultSuccess)
	m.RecordOAuthDownstreamAuth(ctx, OAuthResultFallback)
	m.RecordOAuthDownstreamAuth(ctx, OAuthResultFailure)
	m.RecordOAuthDownstreamAuth(ctx, OAuthResultDenied)

	// SSO token injection metrics
	m.RecordSSOTokenInjection(ctx, "success")
	m.RecordSSOTokenInjection(ctx, "oauth_success")
	m.RecordSSOTokenInjection(ctx, "no_token")
	m.RecordSSOTokenInjection(ctx, "no_user")
	m.RecordSSOTokenInjection(ctx, "no_store")
	m.RecordSSOTokenInjection(ctx, "lookup_failed")

	// Client cache metrics
	m.RecordCacheHit(ctx, "prod-cluster-01")
	m.RecordCacheHit(ctx, "staging-cluster")
	m.RecordCacheMiss(ctx, "new-cluster")
	m.RecordCacheEviction(ctx, "expired")
	m.RecordCacheEviction(ctx, "lru")
	m.RecordCacheEviction(ctx, "manual")
	m.SetCacheSize(ctx, 42)

	// CAPI/Federation cluster operation metrics
	m.RecordClusterOperation(ctx, "prod-wc-01", OperationGet, StatusSuccess, 100*time.Millisecond)
	m.RecordClusterOperation(ctx, "staging-cluster", OperationList, StatusSuccess, 150*time.Millisecond)
	m.RecordClusterOperation(ctx, "dev-cluster", OperationCreate, StatusError, 200*time.Millisecond)
	m.RecordClusterOperation(ctx, "", OperationDelete, StatusSuccess, 50*time.Millisecond) // management cluster

	// Impersonation metrics
	m.RecordImpersonation(ctx, "jane@giantswarm.io", "prod-wc-01", ImpersonationResultSuccess)
	m.RecordImpersonation(ctx, "user@example.com", "staging-cluster", ImpersonationResultError)
	m.RecordImpersonation(ctx, "attacker@malicious.io", "dev-cluster", ImpersonationResultDenied)

	// Federation client creation metrics
	m.RecordFederationClientCreation(ctx, "prod-wc-01", FederationClientResultSuccess)
	m.RecordFederationClientCreation(ctx, "staging-cluster", FederationClientResultCached)
	m.RecordFederationClientCreation(ctx, "dev-cluster", FederationClientResultError)

	// Privileged secret access metrics
	m.RecordPrivilegedSecretAccess(ctx, "giantswarm.io", "success")
	m.RecordPrivilegedSecretAccess(ctx, "example.com", "error")
	m.RecordPrivilegedSecretAccess(ctx, "other.org", "rate_limited")
	m.RecordPrivilegedSecretAccess(ctx, "internal.io", "fallback")

	// Workload cluster authentication metrics
	m.RecordWorkloadClusterAuth(ctx, "impersonation", "prod-wc-01", "success")
	m.RecordWorkloadClusterAuth(ctx, "sso-passthrough", "staging-cluster", "success")
	m.RecordWorkloadClusterAuth(ctx, "impersonation", "dev-cluster", "error")
	m.RecordWorkloadClusterAuth(ctx, "sso-passthrough", "new-cluster", "token_missing")
}

// containsMetric checks if the metrics output contains a metric line
// that starts with the given metric name (accounting for labels).
func containsMetric(metricsOutput, metricName string) bool {
	// Prometheus metrics format: metric_name{labels} value
	// We need to check for:
	// 1. metric_name{ - metric with labels
	// 2. metric_name  - metric with space before value (no labels)
	// 3. # TYPE metric_name - type declaration
	// 4. # HELP metric_name - help declaration
	lines := strings.Split(metricsOutput, "\n")
	for _, line := range lines {
		// Skip empty lines and comments (except TYPE/HELP)
		if line == "" {
			continue
		}

		// Check for TYPE or HELP declarations
		if strings.HasPrefix(line, "# TYPE "+metricName+" ") ||
			strings.HasPrefix(line, "# HELP "+metricName+" ") {
			return true
		}

		// Check for metric data lines
		// Format: metric_name{labels} value or metric_name value
		if strings.HasPrefix(line, metricName+"{") || strings.HasPrefix(line, metricName+" ") {
			return true
		}
	}
	return false
}

// TestMetricLabelsAreRecorded verifies that metric labels are properly recorded
// with the expected values (cardinality controls, etc.).
func TestMetricLabelsAreRecorded(t *testing.T) {
	config := Config{
		ServiceName:     "test-metrics-labels",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "none",
	}

	ctx := context.Background()
	provider, err := NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create instrumentation provider: %v", err)
	}
	defer func() { _ = provider.Shutdown(ctx) }()

	metrics := provider.Metrics()

	// Record some metrics with specific labels
	metrics.RecordHTTPRequest(ctx, "POST", "/mcp", 201, 50*time.Millisecond)
	metrics.RecordK8sOperation(ctx, "", OperationGet, "pods", "production", StatusSuccess, 100*time.Millisecond)
	metrics.RecordImpersonation(ctx, "jane@giantswarm.io", "prod-wc-01", ImpersonationResultSuccess)
	metrics.RecordPrivilegedSecretAccess(ctx, "giantswarm.io", "success")

	// Fetch metrics
	server := httptest.NewServer(promhttp.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to fetch metrics: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read metrics body: %v", err)
	}
	metricsOutput := string(body)

	// Verify specific label values
	labelTests := []struct {
		description string
		expected    string
	}{
		{"HTTP method label", `method="POST"`},
		{"HTTP path label", `path="/mcp"`},
		{"HTTP status label", `status="201"`},
		{"K8s operation label", `operation="get"`},
		{"K8s status label", `status="success"`},
		{"K8s cluster scope label", `cluster_scope="management"`},
		{"K8s discovery mode label", `discovery_mode="single"`},
		{"K8s cluster type label", `cluster_type="management"`},
		// Impersonation uses domain extraction
		{"User domain label (cardinality control)", `user_domain="giantswarm.io"`},
		// Cluster type classification
		{"Cluster type label (cardinality control)", `cluster="production"`},
	}

	for _, tc := range labelTests {
		if strings.Contains(metricsOutput, tc.expected) {
			t.Logf("PASS: Found label %s (%s)", tc.expected, tc.description)
		} else {
			t.Errorf("FAIL: Missing label %s (%s)", tc.expected, tc.description)
		}
	}
}

// TestMetricsAreThreadSafe runs concurrent metric recordings to verify
// thread safety (already covered in metrics_test.go but good to have here
// with real Prometheus export).
func TestMetricsAreThreadSafe(t *testing.T) {
	config := Config{
		ServiceName:     "test-metrics-threadsafe",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "none",
	}

	ctx := context.Background()
	provider, err := NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create instrumentation provider: %v", err)
	}
	defer func() { _ = provider.Shutdown(ctx) }()

	metrics := provider.Metrics()

	// Run concurrent recordings
	const goroutines = 50
	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()

			// Record various metrics concurrently
			for j := 0; j < 10; j++ {
				metrics.RecordHTTPRequest(ctx, "GET", "/test", 200, time.Duration(id)*time.Millisecond)
				metrics.RecordK8sOperation(ctx, "", OperationList, "pods", "default", StatusSuccess, 50*time.Millisecond)
				metrics.RecordCacheHit(ctx, "cluster-1")
				metrics.RecordImpersonation(ctx, "user@test.io", "cluster", ImpersonationResultSuccess)
				metrics.IncrementActiveSessions(ctx)
				metrics.DecrementActiveSessions(ctx)
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// If we got here without panic or deadlock, the test passes
	// Verify we can still fetch metrics
	server := httptest.NewServer(promhttp.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to fetch metrics after concurrent recording: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}
}
