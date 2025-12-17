package instrumentation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric attribute keys - using constants for consistency and DRY
const (
	// Common attributes (reused across metrics)
	attrMethod       = "method"
	attrPath         = "path"
	attrStatus       = "status"
	attrOperation    = "operation"
	attrResourceType = "resource_type"
	attrNamespace    = "namespace"
	attrResult       = "result"
	attrCluster      = "cluster"
	attrReason       = "reason"

	// CAPI/Federation specific attributes (with cardinality controls)
	attrUserDomain  = "user_domain"
	attrClusterType = "cluster_type"
)

// Metrics provides methods for recording observability metrics.
type Metrics struct {
	// HTTP metrics
	httpRequestsTotal   metric.Int64Counter
	httpRequestDuration metric.Float64Histogram
	activeSessions      metric.Int64UpDownCounter

	// Kubernetes operation metrics
	k8sOperationsTotal      metric.Int64Counter
	k8sOperationDuration    metric.Float64Histogram
	k8sPodOperationsTotal   metric.Int64Counter
	k8sPodOperationDuration metric.Float64Histogram

	// OAuth authentication metrics
	oauthDownstreamAuthTotal metric.Int64Counter

	// Client cache metrics
	clientCacheHitsTotal      metric.Int64Counter
	clientCacheMissesTotal    metric.Int64Counter
	clientCacheEvictionsTotal metric.Int64Counter
	clientCacheSize           metric.Int64Gauge

	// CAPI/Federation metrics
	clusterOperationsTotal    metric.Int64Counter
	clusterOperationDuration  metric.Float64Histogram
	impersonationTotal        metric.Int64Counter
	federationClientCreations metric.Int64Counter

	// Configuration
	// detailedLabels controls whether high-cardinality labels (namespace, resource_type)
	// are included in Kubernetes operation metrics
	detailedLabels bool
}

// NewMetrics creates a new Metrics instance with all metrics initialized.
// The detailedLabels parameter controls whether high-cardinality labels are included.
func NewMetrics(meter metric.Meter, detailedLabels bool) (*Metrics, error) {
	m := &Metrics{
		detailedLabels: detailedLabels,
	}

	var err error

	// HTTP Metrics
	m.httpRequestsTotal, err = meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http_requests_total counter: %w", err)
	}

	m.httpRequestDuration, err = meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http_request_duration_seconds histogram: %w", err)
	}

	m.activeSessions, err = meter.Int64UpDownCounter(
		"active_port_forward_sessions",
		metric.WithDescription("Number of active port-forward sessions"),
		metric.WithUnit("{session}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create active_port_forward_sessions gauge: %w", err)
	}

	// Kubernetes Operation Metrics
	m.k8sOperationsTotal, err = meter.Int64Counter(
		"kubernetes_operations_total",
		metric.WithDescription("Total number of Kubernetes operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes_operations_total counter: %w", err)
	}

	m.k8sOperationDuration, err = meter.Float64Histogram(
		"kubernetes_operation_duration_seconds",
		metric.WithDescription("Kubernetes operation duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes_operation_duration_seconds histogram: %w", err)
	}

	// Pod Operation Metrics
	m.k8sPodOperationsTotal, err = meter.Int64Counter(
		"kubernetes_pod_operations_total",
		metric.WithDescription("Total number of Kubernetes pod operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes_pod_operations_total counter: %w", err)
	}

	m.k8sPodOperationDuration, err = meter.Float64Histogram(
		"kubernetes_pod_operation_duration_seconds",
		metric.WithDescription("Kubernetes pod operation duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes_pod_operation_duration_seconds histogram: %w", err)
	}

	// OAuth Metrics
	m.oauthDownstreamAuthTotal, err = meter.Int64Counter(
		"oauth_downstream_auth_total",
		metric.WithDescription("Total number of OAuth downstream authentication attempts"),
		metric.WithUnit("{attempt}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth_downstream_auth_total counter: %w", err)
	}

	// Client Cache Metrics
	//
	// Note on cardinality: The "cluster" label on cache hit/miss metrics may have
	// high cardinality in environments with many clusters. Monitor your metrics
	// backend capacity and consider aggregating by cluster if needed.
	m.clientCacheHitsTotal, err = meter.Int64Counter(
		"mcp_client_cache_hits_total",
		metric.WithDescription("Total number of client cache hits. Label: cluster (may be high cardinality)"),
		metric.WithUnit("{hit}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_client_cache_hits_total counter: %w", err)
	}

	m.clientCacheMissesTotal, err = meter.Int64Counter(
		"mcp_client_cache_misses_total",
		metric.WithDescription("Total number of client cache misses. Label: cluster (may be high cardinality)"),
		metric.WithUnit("{miss}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_client_cache_misses_total counter: %w", err)
	}

	m.clientCacheEvictionsTotal, err = meter.Int64Counter(
		"mcp_client_cache_evictions_total",
		metric.WithDescription("Total number of client cache evictions. Label: reason (expired, lru, manual)"),
		metric.WithUnit("{eviction}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_client_cache_evictions_total counter: %w", err)
	}

	m.clientCacheSize, err = meter.Int64Gauge(
		"mcp_client_cache_entries",
		metric.WithDescription("Current number of entries in the client cache"),
		metric.WithUnit("{entry}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_client_cache_entries gauge: %w", err)
	}

	// CAPI/Federation Metrics
	//
	// Note on cardinality: These metrics use cardinality controls:
	// - cluster_type instead of cluster_name (production/staging/other)
	// - user_domain instead of full email (e.g., "giantswarm.io")
	m.clusterOperationsTotal, err = meter.Int64Counter(
		"mcp_cluster_operations_total",
		metric.WithDescription("Total operations performed on remote clusters. Labels: cluster_type, operation, status"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_cluster_operations_total counter: %w", err)
	}

	m.clusterOperationDuration, err = meter.Float64Histogram(
		"mcp_cluster_operation_duration_seconds",
		metric.WithDescription("Duration of operations on remote clusters. Labels: cluster_type, operation"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_cluster_operation_duration_seconds histogram: %w", err)
	}

	m.impersonationTotal, err = meter.Int64Counter(
		"mcp_impersonation_total",
		metric.WithDescription("Total impersonation requests. Labels: user_domain, cluster_type, result"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_impersonation_total counter: %w", err)
	}

	m.federationClientCreations, err = meter.Int64Counter(
		"mcp_federation_client_creations_total",
		metric.WithDescription("Total federation client creation attempts. Labels: cluster_type, result"),
		metric.WithUnit("{creation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp_federation_client_creations_total counter: %w", err)
	}

	return m, nil
}

// RecordHTTPRequest records an HTTP request with method, path, status code, and duration.
func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	if m.httpRequestsTotal == nil || m.httpRequestDuration == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrMethod, method),
		attribute.String(attrPath, path),
		attribute.String(attrStatus, strconv.Itoa(statusCode)),
	}

	m.httpRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.httpRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordK8sOperation records a Kubernetes operation with operation type, resource type,
// namespace, status, and duration.
//
// CARDINALITY NOTE: When detailedLabels is false (default), only operation and status
// labels are recorded to avoid cardinality explosion in large clusters.
// When detailedLabels is true, namespace and resource_type are also included.
// For large clusters with >1000 namespaces, keep detailedLabels disabled and use
// traces for per-namespace/resource debugging instead.
func (m *Metrics) RecordK8sOperation(ctx context.Context, operation, resourceType, namespace, status string, duration time.Duration) {
	if m.k8sOperationsTotal == nil || m.k8sOperationDuration == nil {
		return // Instrumentation not initialized
	}

	// Always include operation and status (low cardinality)
	attrs := []attribute.KeyValue{
		attribute.String(attrOperation, operation),
		attribute.String(attrStatus, status),
	}

	// Only add high-cardinality labels if explicitly enabled
	if m.detailedLabels {
		attrs = append(attrs,
			attribute.String(attrResourceType, resourceType),
			attribute.String(attrNamespace, namespace),
		)
	}

	m.k8sOperationsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.k8sOperationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordPodOperation records a Kubernetes pod operation with operation type, namespace,
// status, and duration.
//
// CARDINALITY NOTE: When detailedLabels is false (default), only operation and status
// labels are recorded to avoid cardinality explosion in large clusters.
// When detailedLabels is true, namespace is also included.
func (m *Metrics) RecordPodOperation(ctx context.Context, operation, namespace, status string, duration time.Duration) {
	if m.k8sPodOperationsTotal == nil || m.k8sPodOperationDuration == nil {
		return // Instrumentation not initialized
	}

	// Always include operation and status (low cardinality)
	attrs := []attribute.KeyValue{
		attribute.String(attrOperation, operation),
		attribute.String(attrStatus, status),
	}

	// Only add high-cardinality labels if explicitly enabled
	if m.detailedLabels {
		attrs = append(attrs, attribute.String(attrNamespace, namespace))
	}

	m.k8sPodOperationsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.k8sPodOperationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordOAuthDownstreamAuth records an OAuth downstream authentication attempt with result.
// Result should be one of: "success", "fallback", "failure"
func (m *Metrics) RecordOAuthDownstreamAuth(ctx context.Context, result string) {
	if m.oauthDownstreamAuthTotal == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrResult, result),
	}

	m.oauthDownstreamAuthTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// IncrementActiveSessions increments the active port-forward sessions counter.
func (m *Metrics) IncrementActiveSessions(ctx context.Context) {
	if m.activeSessions == nil {
		return // Instrumentation not initialized
	}

	m.activeSessions.Add(ctx, 1)
}

// DecrementActiveSessions decrements the active port-forward sessions counter.
func (m *Metrics) DecrementActiveSessions(ctx context.Context) {
	if m.activeSessions == nil {
		return // Instrumentation not initialized
	}

	m.activeSessions.Add(ctx, -1)
}

// RecordCacheHit records a cache hit event with optional cluster name.
//
// Note: The cluster label may have high cardinality in environments with many
// clusters. Monitor your metrics backend capacity and consider aggregating
// by cluster if needed.
func (m *Metrics) RecordCacheHit(ctx context.Context, clusterName string) {
	if m.clientCacheHitsTotal == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{}
	if clusterName != "" {
		attrs = append(attrs, attribute.String(attrCluster, clusterName))
	}

	m.clientCacheHitsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordCacheMiss records a cache miss event with optional cluster name.
//
// Note: The cluster label may have high cardinality in environments with many
// clusters. Monitor your metrics backend capacity and consider aggregating
// by cluster if needed.
func (m *Metrics) RecordCacheMiss(ctx context.Context, clusterName string) {
	if m.clientCacheMissesTotal == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{}
	if clusterName != "" {
		attrs = append(attrs, attribute.String(attrCluster, clusterName))
	}

	m.clientCacheMissesTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordCacheEviction records a cache eviction event with the reason.
// Common reasons: "expired", "lru", "manual"
func (m *Metrics) RecordCacheEviction(ctx context.Context, reason string) {
	if m.clientCacheEvictionsTotal == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrReason, reason),
	}

	m.clientCacheEvictionsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// SetCacheSize sets the current cache size gauge.
func (m *Metrics) SetCacheSize(ctx context.Context, size int) {
	if m.clientCacheSize == nil {
		return // Instrumentation not initialized
	}

	m.clientCacheSize.Record(ctx, int64(size))
}

// RecordClusterOperation records a remote cluster operation with cardinality controls.
// The clusterName is automatically classified into cluster types (production/staging/other)
// to prevent cardinality explosion.
//
// Parameters:
//   - clusterName: Original cluster name (will be classified)
//   - operation: The operation type (get, list, create, delete, etc.)
//   - status: Result status ("success" or "error")
//   - duration: Time taken for the operation
func (m *Metrics) RecordClusterOperation(ctx context.Context, clusterName, operation, status string, duration time.Duration) {
	if m.clusterOperationsTotal == nil || m.clusterOperationDuration == nil {
		return // Instrumentation not initialized
	}

	clusterType := ClassifyClusterName(clusterName)

	attrs := []attribute.KeyValue{
		attribute.String(attrCluster, clusterType),
		attribute.String(attrOperation, operation),
		attribute.String(attrStatus, status),
	}

	m.clusterOperationsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	// Duration metrics don't need status label
	durationAttrs := []attribute.KeyValue{
		attribute.String(attrCluster, clusterType),
		attribute.String(attrOperation, operation),
	}
	m.clusterOperationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(durationAttrs...))
}

// RecordImpersonation records an impersonation request with cardinality controls.
// The userEmail is reduced to domain only to prevent cardinality explosion.
//
// Parameters:
//   - userEmail: User's email (will be reduced to domain)
//   - clusterName: Target cluster (will be classified)
//   - result: Result of impersonation ("success", "error", "denied")
func (m *Metrics) RecordImpersonation(ctx context.Context, userEmail, clusterName, result string) {
	if m.impersonationTotal == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrUserDomain, ExtractUserDomain(userEmail)),
		attribute.String(attrCluster, ClassifyClusterName(clusterName)),
		attribute.String(attrResult, result),
	}

	m.impersonationTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordFederationClientCreation records a federation client creation attempt.
//
// Parameters:
//   - clusterName: Target cluster (will be classified)
//   - result: Result of creation ("success", "error", "cached")
func (m *Metrics) RecordFederationClientCreation(ctx context.Context, clusterName, result string) {
	if m.federationClientCreations == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrCluster, ClassifyClusterName(clusterName)),
		attribute.String(attrResult, result),
	}

	m.federationClientCreations.Add(ctx, 1, metric.WithAttributes(attrs...))
}
