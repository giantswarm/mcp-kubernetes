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
}

// NewMetrics creates a new Metrics instance with all metrics initialized.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{}

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
// CARDINALITY WARNING: Recording namespace and resource_type as labels can create
// high cardinality in large clusters with thousands of namespaces and resource types.
// In production with >1000 namespaces, consider:
// 1. Using sampling for detailed labels
// 2. Aggregating by operation and status only
// 3. Using traces for per-namespace/resource debugging instead of metrics
func (m *Metrics) RecordK8sOperation(ctx context.Context, operation, resourceType, namespace, status string, duration time.Duration) {
	if m.k8sOperationsTotal == nil || m.k8sOperationDuration == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrOperation, operation),
		attribute.String(attrResourceType, resourceType),
		attribute.String(attrNamespace, namespace),
		attribute.String(attrStatus, status),
	}

	m.k8sOperationsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.k8sOperationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordPodOperation records a Kubernetes pod operation with operation type, namespace,
// status, and duration.
//
// CARDINALITY WARNING: Recording namespace as a label can create high cardinality
// in large clusters with thousands of namespaces.
func (m *Metrics) RecordPodOperation(ctx context.Context, operation, namespace, status string, duration time.Duration) {
	if m.k8sPodOperationsTotal == nil || m.k8sPodOperationDuration == nil {
		return // Instrumentation not initialized
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrOperation, operation),
		attribute.String(attrNamespace, namespace),
		attribute.String(attrStatus, status),
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
