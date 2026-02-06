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
//   - active_port_forward_sessions: Gauge of active port-forward sessions
//
// Kubernetes Operation Metrics:
//   - mcp_kubernetes_operations_total: Counter of K8s operations by cluster_scope, discovery_mode, cluster_type, operation, status
//   - mcp_kubernetes_operation_duration_seconds: Histogram of K8s operation durations by cluster_scope, discovery_mode, cluster_type, operation, status
//
// Pod Operation Metrics:
//   - kubernetes_pod_operations_total: Counter of pod operations
//   - kubernetes_pod_operation_duration_seconds: Histogram of pod operation durations
//
// OAuth Authentication Metrics:
//   - oauth_downstream_auth_total: Counter of OAuth authentication events by result
//   - oauth_sso_token_injection_total: Counter of SSO token injections for downstream K8s API auth (by result)
//
// OAuth CIMD (Client ID Metadata Documents) Metrics:
// Note: CIMD metrics are provided by the mcp-oauth library when instrumentation is enabled.
// See the mcp-oauth documentation for available metrics: oauth.cimd.fetch.total,
// oauth.cimd.fetch.duration, oauth.cimd.cache.total
//
// CAPI/Federation Metrics (with cardinality controls):
//   - mcp_kubernetes_operations_total: Covers remote cluster operations with cluster_scope=workload and discovery_mode=capi
//   - mcp_kubernetes_operation_duration_seconds: Histogram of operation durations for management/workload scopes
//   - mcp_kubernetes_impersonation_total: Counter of impersonation requests (by user_domain, cluster_type, result)
//   - mcp_kubernetes_federation_client_creations_total: Counter of federation client creation attempts
//   - mcp_kubernetes_privileged_access_total: Counter of privileged access attempts (secret access + CAPI discovery)
//   - mcp_kubernetes_wc_auth_total: Counter of workload cluster authentication attempts
//   - mcp_kubernetes_client_cache_hits_total: Counter of client cache hits
//   - mcp_kubernetes_client_cache_misses_total: Counter of client cache misses
//   - mcp_kubernetes_client_cache_evictions_total: Counter of client cache evictions
//   - mcp_kubernetes_client_cache_entries: Gauge of current cache entries
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
//	recorder.RecordK8sOperation(ctx, "", "get", "pods", "default", "success", time.Since(start))
//
//	// Record a CAPI cluster operation with cardinality control
//	recorder.RecordClusterOperation(ctx, "prod-wc-01", "list", "success", time.Since(start))
//
//	// Record impersonation with cardinality control
//	recorder.RecordImpersonation(ctx, "jane@giantswarm.io", "prod-wc-01", "success")
package instrumentation
