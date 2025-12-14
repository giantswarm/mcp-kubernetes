// Package instrumentation provides comprehensive OpenTelemetry instrumentation
// for the mcp-kubernetes server.
//
// This package enables production-grade observability through:
//   - OpenTelemetry metrics for HTTP requests, Kubernetes operations, and sessions
//   - Distributed tracing for request flows and Kubernetes API calls
//   - Prometheus metrics export via /metrics endpoint
//   - OTLP export support for modern observability platforms
//   - Structured audit logging for tool invocations
//   - CAPI/Federation-specific metrics and tracing
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
// OAuth CIMD (Client ID Metadata Documents) Metrics:
// (Registered and ready for use, pending mcp-oauth library instrumentation callbacks)
//   - oauth_cimd_fetch_total: Counter of CIMD metadata fetch attempts (by result: success, error, blocked)
//   - oauth_cimd_fetch_duration_seconds: Histogram of CIMD metadata fetch durations
//   - oauth_cimd_cache_total: Counter of CIMD cache operations (by operation: hit, miss, negative_hit)
//
// CAPI/Federation Metrics (with cardinality controls):
//   - mcp_cluster_operations_total: Counter of remote cluster operations (by cluster_type, operation, status)
//   - mcp_cluster_operation_duration_seconds: Histogram of remote cluster operation durations
//   - mcp_impersonation_total: Counter of impersonation requests (by user_domain, cluster_type, result)
//   - mcp_federation_client_creations_total: Counter of federation client creation attempts
//   - mcp_client_cache_hits_total: Counter of client cache hits
//   - mcp_client_cache_misses_total: Counter of client cache misses
//   - mcp_client_cache_evictions_total: Counter of client cache evictions
//   - mcp_client_cache_entries: Gauge of current cache entries
//
// # Cardinality Management
//
// IMPORTANT: High cardinality in metrics can cause memory issues in production.
// This package provides automatic cardinality controls:
//
//   - User emails are reduced to domains (e.g., "giantswarm.io" instead of "jane@giantswarm.io")
//   - Cluster names are classified into types (production, staging, development, management, other)
//   - Use ClassifyClusterName() and ExtractUserDomain() for consistent cardinality control
//
// For large clusters with >1000 namespaces, keep METRICS_DETAILED_LABELS=false (default)
// and use traces for per-namespace/resource debugging.
//
// # Tracing
//
// Distributed tracing spans are created for:
//   - HTTP request handling
//   - MCP tool invocations (tool.<name>)
//   - Kubernetes API calls (k8s.<operation>)
//   - Federation operations (federation.<operation>)
//   - OAuth token operations
//   - Port-forward session lifecycle
//
// CAPI-specific span attributes include:
//   - mcp.cluster: Target cluster name
//   - mcp.cluster_type: Classified cluster type
//   - mcp.user.email: User email (optional, configurable)
//   - mcp.user.domain: User's email domain (always included)
//   - mcp.user.group_count: Number of groups
//   - mcp.tool: MCP tool name
//   - mcp.cache_hit: Whether a cache hit occurred
//   - mcp.impersonated: Whether impersonation was used
//   - mcp.federated: Whether federation was used
//
// # Audit Logging
//
// The ToolInvocation type provides structured audit logging for MCP tool calls:
//
//	ti := instrumentation.NewToolInvocation("kubernetes_delete").
//		WithUser(email, groups).
//		WithCluster(clusterName).
//		WithResource(namespace, resourceType, resourceName).
//		WithSpanContext(ctx)
//	defer func() {
//		ti.Complete(success, err)
//		auditLogger.LogToolInvocation(ti)
//	}()
//
// Audit logs include:
//   - Tool name, user identity, cluster, resource details
//   - Execution duration, success/failure status
//   - Trace ID for correlation with distributed traces
//   - Cardinality-controlled variants for metrics-compatible logging
//
// # Trace ID Propagation
//
// Trace IDs can be propagated to Kubernetes audit logs via impersonation extra headers.
// This bridges the "audit gap" when the MCP server acts as a proxy:
//
//	config := federation.ConfigWithImpersonationAndTraceID(baseConfig, user, traceID)
//	// Kubernetes audit log will show: {"extra": {"trace-id": ["abc123..."]}}
//
// # Configuration
//
// Instrumentation can be configured via environment variables:
//   - INSTRUMENTATION_ENABLED: Enable/disable instrumentation (default: true)
//   - METRICS_EXPORTER: Metrics exporter type (prometheus, otlp, stdout, default: prometheus)
//   - METRICS_DETAILED_LABELS: Include high-cardinality labels (default: false)
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
//
//	// Record a CAPI cluster operation with cardinality control
//	recorder.RecordClusterOperation(ctx, "prod-wc-01", "list", "success", time.Since(start))
//
//	// Record impersonation with cardinality control
//	recorder.RecordImpersonation(ctx, "jane@giantswarm.io", "prod-wc-01", "success")
package instrumentation
