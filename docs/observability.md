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

### CAPI/Federation Metrics

These metrics are specific to multi-cluster federation mode and use cardinality controls to prevent metric explosion.

#### `mcp_cluster_operations_total`
Counter of operations performed on remote clusters.

**Labels:**
- `cluster`: Classified cluster type (production, staging, development, management, other)
- `operation`: Operation type (get, list, create, delete, etc.)
- `status`: Operation result (success, error)

**Note:** Cluster names are automatically classified into types to prevent high cardinality.

**Example:**
```promql
# Total operations on production clusters
mcp_cluster_operations_total{cluster="production"}

# Error rate on remote clusters
rate(mcp_cluster_operations_total{status="error"}[5m])
/ rate(mcp_cluster_operations_total[5m])
```

#### `mcp_cluster_operation_duration_seconds`
Histogram of remote cluster operation durations.

**Labels:**
- `cluster`: Classified cluster type
- `operation`: Operation type

**Buckets:** 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0 seconds

**Example:**
```promql
# P95 duration for production cluster operations
histogram_quantile(0.95,
  rate(mcp_cluster_operation_duration_seconds_bucket{cluster="production"}[5m])
)
```

#### `mcp_impersonation_total`
Counter of impersonation requests.

**Labels:**
- `user_domain`: Email domain of the user (e.g., "giantswarm.io")
- `cluster`: Classified cluster type
- `result`: Result (success, error, denied)

**Note:** User emails are reduced to domains to prevent high cardinality and protect PII.

**Example:**
```promql
# Total impersonation by domain
sum by (user_domain) (mcp_impersonation_total)

# Impersonation denial rate
rate(mcp_impersonation_total{result="denied"}[5m])
/ rate(mcp_impersonation_total[5m])
```

#### `mcp_federation_client_creations_total`
Counter of federation client creation attempts.

**Labels:**
- `cluster`: Classified cluster type
- `result`: Result (success, error, cached)

**Example:**
```promql
# Cache hit ratio
mcp_federation_client_creations_total{result="cached"}
/ sum(mcp_federation_client_creations_total)
```

#### `mcp_wc_auth_total`
Counter of workload cluster authentication attempts. Distinguishes between authentication modes.

**Labels:**
- `auth_mode`: Authentication mode (`impersonation` or `sso-passthrough`)
- `cluster_type`: Classified cluster type (production, staging, development, other)
- `result`: Result (`success`, `error`, `token_missing`, `token_expired`)

**Use Cases:**
- Monitor adoption of SSO passthrough mode
- Track authentication failures by mode
- Detect clusters with auth issues
- Compare success rates between auth modes

**Example:**
```promql
# Total auth by mode
sum by (auth_mode) (mcp_wc_auth_total)

# SSO passthrough success rate
sum(rate(mcp_wc_auth_total{auth_mode="sso-passthrough", result="success"}[5m]))
/ sum(rate(mcp_wc_auth_total{auth_mode="sso-passthrough"}[5m]))

# Token-related failures in SSO passthrough mode
rate(mcp_wc_auth_total{auth_mode="sso-passthrough", result=~"token.*"}[5m])
```

#### `mcp_client_cache_hits_total`
Counter of client cache hits.

**Labels:**
- `cluster`: Cluster name (may have high cardinality)

#### `mcp_client_cache_misses_total`
Counter of client cache misses.

**Labels:**
- `cluster`: Cluster name (may have high cardinality)

#### `mcp_client_cache_evictions_total`
Counter of client cache evictions.

**Labels:**
- `reason`: Eviction reason (expired, lru, manual)

**Example:**
```promql
# Cache eviction rate by reason
sum by (reason) (rate(mcp_client_cache_evictions_total[5m]))
```

#### `mcp_client_cache_entries`
Gauge of current entries in the client cache.

**Example:**
```promql
# Current cache size
mcp_client_cache_entries

# Cache capacity utilization (if max entries is 1000)
mcp_client_cache_entries / 1000
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

### Federation/CAPI Alerts

```yaml
      - alert: HighClusterOperationErrorRate
        expr: |
          (
            sum(rate(mcp_cluster_operations_total{status="error"}[5m]))
            / sum(rate(mcp_cluster_operations_total[5m]))
          ) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in remote cluster operations"
          description: "Error rate is {{ $value | humanizePercentage }}"

      - alert: ImpersonationDenials
        expr: |
          rate(mcp_impersonation_total{result="denied"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Impersonation requests being denied"
          description: "Denial rate: {{ $value | humanize }} per second"

      - alert: HighClientCacheEvictions
        expr: |
          rate(mcp_client_cache_evictions_total{reason="lru"}[5m]) > 1
        for: 10m
        labels:
          severity: info
        annotations:
          summary: "High LRU cache evictions - consider increasing cache size"
          description: "Eviction rate: {{ $value | humanize }} per second"

      - alert: SlowRemoteClusterOperations
        expr: |
          histogram_quantile(0.95,
            sum by (le, cluster) (
              rate(mcp_cluster_operation_duration_seconds_bucket[5m])
            )
          ) > 5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Slow remote cluster operations"
          description: "P95 duration for {{ $labels.cluster }} clusters is {{ $value }}s"
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

Traces include the following standard attributes:

- `http.method`: HTTP method
- `http.route`: Request route
- `http.status_code`: HTTP status code
- `k8s.namespace`: Kubernetes namespace
- `k8s.resource_type`: Resource type
- `k8s.resource_name`: Resource name
- `k8s.operation`: Operation type (get, list, create, delete, etc.)

### CAPI/Federation Trace Attributes

For multi-cluster operations, additional attributes are included:

- `mcp.tool`: MCP tool name being executed
- `mcp.cluster`: Target cluster name
- `mcp.cluster_type`: Classified cluster type (production, staging, development, management, other)
- `mcp.user.email`: User's email (optional, for audit)
- `mcp.user.domain`: User's email domain (always included)
- `mcp.user.group_count`: Number of groups the user belongs to
- `mcp.cache_hit`: Whether the operation used a cached client
- `mcp.impersonated`: Whether user impersonation was used
- `mcp.federated`: Whether federation was used for the operation

### Span Naming Convention

Spans follow a consistent naming convention:

- `tool.<tool_name>`: MCP tool invocations (e.g., `tool.kubernetes_get`)
- `k8s.<operation>`: Kubernetes API calls (e.g., `k8s.get`, `k8s.list`)
- `federation.<operation>`: Federation operations (e.g., `federation.GetClient`)

### Trace ID Propagation

Trace IDs are propagated to Kubernetes audit logs via impersonation headers. This bridges the "audit gap" when the MCP server acts as a proxy:

```
Impersonate-Extra-trace-id: abc123def456...
```

Kubernetes audit logs will show:
```json
{
  "user": {
    "username": "jane@giantswarm.io",
    "extra": {
      "agent": ["mcp-kubernetes"],
      "trace-id": ["abc123def456..."]
    }
  }
}
```

This allows correlation between:
1. MCP server traces in your observability backend (Jaeger, Tempo, etc.)
2. Kubernetes audit logs on workload clusters

### Example Trace Queries

**Jaeger:**
```
service:mcp-kubernetes operation:tool.kubernetes_get
service:mcp-kubernetes mcp.cluster_type=production
service:mcp-kubernetes mcp.user.domain=giantswarm.io
```

**Grafana Tempo:**
```
{ span.mcp.cluster_type = "production" && span.mcp.tool = "kubernetes_delete" }
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

### Detailed Health (`/healthz/detailed`)

Returns comprehensive health information including CAPI/federation status:

```json
{
  "status": "ok",
  "mode": "capi",
  "version": "1.0.0",
  "uptime": "4h32m",
  "management_cluster": {
    "connected": true,
    "capi_crd_available": true
  },
  "federation": {
    "enabled": true,
    "cached_clients": 42
  },
  "instrumentation": {
    "enabled": true
  }
}
```

**Mode values:**
- `capi`: Federation mode with CAPI cluster management
- `in-cluster`: Running inside a Kubernetes cluster with service account
- `local`: Running locally with kubeconfig

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

## Structured Audit Logging

`mcp-kubernetes` provides structured JSON logging for tool invocations to support security auditing and compliance.

### Log Format

Every tool invocation produces a structured log entry:

```json
{
  "level": "info",
  "msg": "tool_executed",
  "tool": "kubernetes_delete",
  "user_domain": "giantswarm.io",
  "group_count": 3,
  "cluster_type": "production",
  "namespace": "production",
  "resource_type": "pods",
  "duration": "0.523s",
  "success": true,
  "trace_id": "abc123def456..."
}
```

### Audit Log Fields

**Standard fields (cardinality-controlled):**
- `tool`: MCP tool name
- `user_domain`: User's email domain (not full email)
- `group_count`: Number of groups
- `cluster_type`: Classified cluster type
- `duration`: Execution duration
- `success`: Boolean success indicator
- `trace_id`: OpenTelemetry trace ID for correlation

**Optional fields:**
- `namespace`: Kubernetes namespace (when applicable)
- `resource_type`: Resource type (when applicable)
- `error`: Error message (when failed)

### Full Audit Logs

For compliance/audit purposes, a separate log stream can include full details:

```json
{
  "level": "info",
  "msg": "tool_audit",
  "tool": "kubernetes_delete",
  "user": "jane@giantswarm.io",
  "groups": ["github:org:giantswarm", "platform-team"],
  "cluster": "prod-wc-01",
  "namespace": "production",
  "resource_type": "pods",
  "resource_name": "nginx-abc123",
  "duration": "0.523s",
  "success": true,
  "trace_id": "abc123def456...",
  "span_id": "789xyz..."
}
```

**Warning:** Full audit logs contain PII (user emails) and sensitive infrastructure details. Ensure:
- Audit logs are stored securely
- Access controls are in place
- Retention policies comply with data protection regulations

### Loki/Grafana Log Queries

```logql
# All tool executions by a specific domain
{app="mcp-kubernetes"} |= "tool_executed" | json | user_domain="giantswarm.io"

# Failed operations on production clusters
{app="mcp-kubernetes"} |= "tool_failed" | json | cluster_type="production"

# Delete operations (security audit)
{app="mcp-kubernetes"} | json | tool="kubernetes_delete"

# Correlate logs with trace ID
{app="mcp-kubernetes"} | json | trace_id="abc123def456"
```

## Best Practices

1. **Sampling**: Set `OTEL_TRACES_SAMPLER_ARG` to an appropriate value (e.g., 0.1 for 10% sampling)
2. **Cardinality**: Keep `METRICS_DETAILED_LABELS=false` in large clusters
3. **Retention**: Configure appropriate retention policies for metrics and traces
4. **Alerting**: Set up alerts for critical metrics (error rates, latency)
5. **Dashboards**: Create Grafana dashboards for key metrics
6. **Monitoring**: Monitor the instrumentation overhead itself
7. **Health Checks**: Use `/healthz` and `/readyz` for Kubernetes probes
8. **Audit Logs**: Separate audit logs from operational logs for compliance
9. **Trace Correlation**: Use trace IDs to correlate MCP server logs with Kubernetes audit logs

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

