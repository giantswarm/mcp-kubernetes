// Package instrumentation provides comprehensive OpenTelemetry instrumentation
// for the mcp-kubernetes server.
//
// This package enables production-grade observability through:
//   - OpenTelemetry metrics for HTTP requests, Kubernetes operations, and sessions
//   - Distributed tracing for request flows and Kubernetes API calls
//   - Prometheus metrics export via /metrics endpoint
//   - OTLP export support for modern observability platforms
//
// # Metrics
//
// The package exposes the following metric categories:
//
// Server/HTTP Metrics:
//   - http_requests_total: Counter of HTTP requests by method, path, and status
//   - http_request_duration_seconds: Histogram of HTTP request durations
//   - active_sessions: Gauge of active port-forward sessions
//
// Kubernetes Operation Metrics:
//   - kubernetes_operations_total: Counter of K8s operations by operation, resource_type, namespace, status
//   - kubernetes_operation_duration_seconds: Histogram of K8s operation durations
//
// Pod Operation Metrics:
//   - kubernetes_pod_operations_total: Counter of pod operations
//   - kubernetes_pod_operation_duration_seconds: Histogram of pod operation durations
//
// OAuth Authentication Metrics:
//   - oauth_downstream_auth_total: Counter of OAuth authentication events by result
//
// # Cardinality Considerations
//
// IMPORTANT: Some metrics include labels like namespace, resource_type, and pod names
// which can create high cardinality in large Kubernetes clusters. In production
// environments with >1000 namespaces or pods, consider:
//   - Using sampling to reduce metric volume
//   - Aggregating metrics at a higher level (e.g., by operation type only)
//   - Using distributed tracing for detailed per-resource debugging
//   - Monitoring cardinality in your metrics backend (Prometheus, etc.)
//
// High cardinality can lead to:
//   - Increased memory usage in metrics backends
//   - Slower query performance
//   - Higher storage costs
//
// # Tracing
//
// Distributed tracing spans are created for:
//   - HTTP request handling
//   - MCP tool invocations
//   - Kubernetes API calls
//   - OAuth token operations
//   - Port-forward session lifecycle
//
// # Configuration
//
// Instrumentation can be configured via environment variables:
//   - INSTRUMENTATION_ENABLED: Enable/disable instrumentation (default: true)
//   - METRICS_EXPORTER: Metrics exporter type (prometheus, otlp, stdout, default: prometheus)
//   - TRACING_EXPORTER: Tracing exporter type (otlp, stdout, none, default: none)
//   - OTEL_EXPORTER_OTLP_ENDPOINT: OTLP endpoint for traces/metrics
//   - OTEL_TRACES_SAMPLER_ARG: Sampling rate (0.0 to 1.0, default: 0.1)
//   - OTEL_SERVICE_NAME: Service name (default: mcp-kubernetes)
//
// # Example Usage
//
//	// Initialize instrumentation
//	provider, err := instrumentation.NewProvider(ctx, instrumentation.Config{
//		ServiceName:    "mcp-kubernetes",
//		ServiceVersion: "0.1.0",
//		Enabled:        true,
//	})
//	if err != nil {
//		return err
//	}
//	defer provider.Shutdown(ctx)
//
//	// Get metrics recorder
//	recorder := provider.Metrics()
//
//	// Record an HTTP request
//	recorder.RecordHTTPRequest(ctx, "POST", "/mcp", 200, time.Since(start))
//
//	// Record a Kubernetes operation
//	recorder.RecordK8sOperation(ctx, "get", "pods", "default", "success", time.Since(start))
package instrumentation
