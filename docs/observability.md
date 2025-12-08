# Observability Guide for mcp-kubernetes

This document provides a comprehensive guide to the observability features of `mcp-kubernetes`, including metrics, tracing, and example queries for monitoring production deployments.

## Overview

`mcp-kubernetes` uses OpenTelemetry for comprehensive instrumentation, providing:

- **Metrics**: Prometheus-compatible metrics for HTTP requests, Kubernetes operations, and sessions
- **Distributed Tracing**: OpenTelemetry traces for request flows and Kubernetes API calls
- **Prometheus Integration**: `/metrics` endpoint for Prometheus scraping
- **Health Checks**: `/healthz` (liveness) and `/readyz` (readiness) endpoints for Kubernetes probes
- **ServiceMonitor**: Optional Prometheus Operator ServiceMonitor CRD

## Configuration

Instrumentation is configured via environment variables:

```bash
# Enable/disable instrumentation (default: true)
INSTRUMENTATION_ENABLED=true

# Metrics exporter type (default: prometheus)
# Options: prometheus, otlp, stdout
METRICS_EXPORTER=prometheus

# Tracing exporter type (default: none)
# Options: otlp, stdout, none
TRACING_EXPORTER=otlp

# OTLP endpoint for traces/metrics (required for otlp exporters)
# Format: hostname:port (without protocol prefix)
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318

# Use insecure (HTTP) transport for OTLP (default: false, uses HTTPS)
# WARNING: Only enable for local development/testing
OTEL_EXPORTER_OTLP_INSECURE=false

# Sampling rate (0.0 to 1.0, default: 0.1)
OTEL_TRACES_SAMPLER_ARG=0.1

# Service name (default: mcp-kubernetes)
OTEL_SERVICE_NAME=mcp-kubernetes

# Enable detailed labels (namespace, resource_type) in K8s metrics
# WARNING: Can cause high cardinality in large clusters (>1000 namespaces)
METRICS_DETAILED_LABELS=false

# Kubernetes metadata (automatically set by Helm chart)
K8S_NAMESPACE=default
K8S_POD_NAME=mcp-kubernetes-abc123
```

## Available Metrics

### HTTP Server Metrics

#### `http_requests_total`
Counter of HTTP requests.

**Labels:**
- `method`: HTTP method (GET, POST, etc.)
- `path`: Request path (/mcp, /metrics, etc.)
- `status`: HTTP status code

**Example:**
```promql
# Total requests to /mcp endpoint
http_requests_total{path="/mcp"}

# Error rate (5xx responses)
rate(http_requests_total{status=~"5.."}[5m])
```

#### `http_request_duration_seconds`
Histogram of HTTP request durations.

**Labels:**
- `method`: HTTP method
- `path`: Request path
- `status`: HTTP status code

**Buckets:** 0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0 seconds

**Example:**
```promql
# 95th percentile request duration
histogram_quantile(0.95, 
  rate(http_request_duration_seconds_bucket[5m])
)

# Average request duration by path
rate(http_request_duration_seconds_sum{path="/mcp"}[5m])
/ rate(http_request_duration_seconds_count{path="/mcp"}[5m])
```

### Kubernetes Operation Metrics

#### `kubernetes_operations_total`
Counter of Kubernetes operations.

**Labels:**
- `operation`: Operation type (get, list, create, apply, delete, patch)
- `resource_type`: Kubernetes resource type (pods, deployments, etc.)
- `namespace`: Kubernetes namespace
- `status`: Operation result (success, error)

**Example:**
```promql
# Total pod operations in default namespace
kubernetes_operations_total{resource_type="pods",namespace="default"}

# Error rate for deployments
rate(kubernetes_operations_total{
  resource_type="deployments",
  status="error"
}[5m])
```

#### `kubernetes_operation_duration_seconds`
Histogram of Kubernetes operation durations.

**Labels:**
- `operation`: Operation type
- `resource_type`: Kubernetes resource type

**Buckets:** 0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0 seconds

**Example:**
```promql
# 99th percentile operation duration for pods
histogram_quantile(0.99, 
  rate(kubernetes_operation_duration_seconds_bucket{
    resource_type="pods"
  }[5m])
)

# Slow operations (>1s)
histogram_quantile(0.5, 
  rate(kubernetes_operation_duration_seconds_bucket[5m])
) > 1
```

### Pod Operation Metrics

#### `kubernetes_pod_operations_total`
Counter of pod-specific operations (logs, exec, port-forward).

**Labels:**
- `operation`: Operation type (logs, exec, watch)
- `namespace`: Kubernetes namespace
- `status`: Operation result (success, error)

**Example:**
```promql
# Total log operations
kubernetes_pod_operations_total{operation="logs"}

# Failed exec operations
rate(kubernetes_pod_operations_total{
  operation="exec",
  status="error"
}[5m])
```

#### `kubernetes_pod_operation_duration_seconds`
Histogram of pod operation durations.

**Labels:**
- `operation`: Operation type
- `namespace`: Kubernetes namespace
- `status`: Operation result

**Buckets:** 0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0 seconds

### OAuth Authentication Metrics

#### `oauth_downstream_auth_total`
Counter of OAuth downstream authentication attempts.

**Labels:**
- `result`: Authentication result (success, fallback, failure)

**Example:**
```promql
# Total successful OAuth authentications
oauth_downstream_auth_total{result="success"}

# OAuth fallback rate
rate(oauth_downstream_auth_total{result="fallback"}[5m])
/ rate(oauth_downstream_auth_total[5m])
```

### Session Metrics

#### `active_port_forward_sessions`
Gauge of active port-forward sessions.

**Example:**
```promql
# Current active sessions
active_port_forward_sessions

# Average active sessions over time
avg_over_time(active_port_forward_sessions[1h])
```

## Example Prometheus Queries

### Service Health

```promql
# Request success rate (non-5xx responses)
sum(rate(http_requests_total{status!~"5.."}[5m]))
/ sum(rate(http_requests_total[5m]))

# Kubernetes API error rate
sum(rate(kubernetes_operations_total{status="error"}[5m]))
/ sum(rate(kubernetes_operations_total[5m]))
```

### Performance Monitoring

```promql
# P95 HTTP request duration
histogram_quantile(0.95, 
  sum by (le) (rate(http_request_duration_seconds_bucket[5m]))
)

# P95 Kubernetes operation duration
histogram_quantile(0.95, 
  sum by (le, operation) (
    rate(kubernetes_operation_duration_seconds_bucket[5m])
  )
)
```

### Resource Usage

```promql
# Requests per second
rate(http_requests_total[1m])

# Kubernetes operations per second by type
sum by (operation) (rate(kubernetes_operations_total[1m]))
```

## Prometheus Scraping Configuration

Add the following to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'mcp-kubernetes'
    static_configs:
      - targets: ['mcp-kubernetes:8080']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

## Kubernetes ServiceMonitor

For Prometheus Operator, use a ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: mcp-kubernetes
  namespace: default
spec:
  selector:
    matchLabels:
      app: mcp-kubernetes
  endpoints:
    - port: http
      path: /metrics
      interval: 15s
```

## Example Alerts

### High Error Rate

```yaml
groups:
  - name: mcp-kubernetes
    rules:
      - alert: HighErrorRate
        expr: |
          (
            sum(rate(http_requests_total{status=~"5.."}[5m]))
            / sum(rate(http_requests_total[5m]))
          ) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in mcp-kubernetes"
          description: "Error rate is {{ $value | humanizePercentage }}"
```

### Slow Operations

```yaml
      - alert: SlowKubernetesOperations
        expr: |
          histogram_quantile(0.95, 
            sum by (le, operation) (
              rate(kubernetes_operation_duration_seconds_bucket[5m])
            )
          ) > 2
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Slow Kubernetes operations detected"
          description: "P95 duration for {{ $labels.operation }} is {{ $value }}s"
```

### OAuth Authentication Issues

```yaml
      - alert: OAuthAuthenticationFailures
        expr: |
          rate(oauth_downstream_auth_total{result="failure"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "OAuth authentication failures detected"
          description: "Failure rate: {{ $value | humanize }} per second"
```

## Grafana Dashboards

### Key Panels

1. **Request Rate**: `rate(http_requests_total[1m])`
2. **Error Rate**: `rate(http_requests_total{status=~"5.."}[1m])`
3. **Request Duration (P50, P95, P99)**: 
   ```promql
   histogram_quantile(0.50, rate(http_request_duration_seconds_bucket[5m]))
   histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))
   histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))
   ```
4. **Active Sessions**: `active_port_forward_sessions`
5. **Kubernetes Operations by Type**: 
   ```promql
   sum by (operation) (rate(kubernetes_operations_total[1m]))
   ```

## Tracing

When `TRACING_EXPORTER=otlp` is set, distributed traces are exported to an OTLP collector.

### Trace Attributes

Traces include the following attributes:

- `http.method`: HTTP method
- `http.route`: Request route
- `http.status_code`: HTTP status code
- `k8s.namespace`: Kubernetes namespace
- `k8s.resource_type`: Resource type
- `k8s.operation`: Operation type

### Example Trace Query (Jaeger)

```
service:mcp-kubernetes operation:kubernetes_get
```

## Health Endpoints

`mcp-kubernetes` exposes standard Kubernetes health check endpoints:

### Liveness Probe (`/healthz`)

Returns `200 OK` if the server process is running. Use for Kubernetes liveness probes.

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

### Readiness Probe (`/readyz`)

Returns `200 OK` if the server is ready to receive traffic. Checks:
- Server is not shutting down
- Required components are initialized

```yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Cardinality Management

### Detailed Labels

By default, Kubernetes operation metrics only include low-cardinality labels (`operation`, `status`) to prevent cardinality explosion in large clusters.

To enable detailed labels (`namespace`, `resource_type`):

```bash
METRICS_DETAILED_LABELS=true
```

**Warning**: In clusters with >1000 namespaces, detailed labels can cause:
- Prometheus memory issues
- Slow queries
- Storage bloat

For large clusters, use traces instead of detailed metrics for per-namespace debugging.

### Recommended Prometheus Settings for Large Clusters

```yaml
# prometheus.yml
global:
  # Limit samples per scrape
  sample_limit: 10000

scrape_configs:
  - job_name: 'mcp-kubernetes'
    scrape_interval: 30s  # Increase interval
    metric_relabel_configs:
      # Drop high-cardinality labels if needed
      - source_labels: [namespace]
        regex: '.*'
        action: labeldrop
```

## Best Practices

1. **Sampling**: Set `OTEL_TRACES_SAMPLER_ARG` to an appropriate value (e.g., 0.1 for 10% sampling)
2. **Cardinality**: Keep `METRICS_DETAILED_LABELS=false` in large clusters
3. **Retention**: Configure appropriate retention policies for metrics and traces
4. **Alerting**: Set up alerts for critical metrics (error rates, latency)
5. **Dashboards**: Create Grafana dashboards for key metrics
6. **Monitoring**: Monitor the instrumentation overhead itself
7. **Health Checks**: Use `/healthz` and `/readyz` for Kubernetes probes

## Troubleshooting

### Metrics Not Appearing

1. Check that `INSTRUMENTATION_ENABLED=true`
2. Verify Prometheus is scraping the `/metrics` endpoint
3. Check logs for instrumentation initialization errors
4. Ensure the metrics exporter is correctly configured

### High Cardinality

If you see high cardinality warnings:

1. Review label values (avoid using unique IDs as labels)
2. Consider using recording rules to pre-aggregate metrics
3. Adjust Prometheus storage settings

### Performance Impact

If instrumentation impacts performance:

1. Reduce trace sampling rate (`OTEL_TRACES_SAMPLER_ARG`)
2. Use `prometheus` exporter instead of `otlp` for lower overhead
3. Consider disabling tracing (`TRACING_EXPORTER=none`)

