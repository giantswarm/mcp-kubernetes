# mcp-kubernetes

A Model Context Protocol (MCP) server that provides tools for interacting with Kubernetes clusters.

## Features

- **Resource Management**: Get, list, describe, create, apply, delete, and patch Kubernetes resources
- **Pod Operations**: Get logs, execute commands, and set up port forwarding
- **Context Management**: List, get, and switch between Kubernetes contexts
- **Cluster Information**: Get API resources and cluster health status
- **Multiple Authentication Modes**: Support for kubeconfig, in-cluster, OAuth 2.1, and federated multi-cluster authentication
- **Multi-Cluster Support**: Federation with Cluster API (CAPI) for managing workload clusters from a management cluster
- **Multiple Transport Types**: Support for stdio and streamable HTTP
- **Safety Features**: Non-destructive mode, dry-run capability, and operation restrictions
- **OAuth 2.1 Support**: Secure token-based authentication with Dex OIDC (default) and Google OAuth providers
- **Production-Grade Observability**: OpenTelemetry instrumentation with Prometheus metrics and distributed tracing

## Installation

```bash
go install github.com/giantswarm/mcp-kubernetes@latest
```

## Usage

### Basic Usage

```bash
# Start the MCP server with default settings (stdio transport, kubeconfig authentication)
mcp-kubernetes serve

# Start with debug logging
mcp-kubernetes serve --debug

# Start with in-cluster authentication (when running as a pod in Kubernetes)
mcp-kubernetes serve --in-cluster
```

### Authentication Modes

The server supports multiple authentication modes:

#### Kubeconfig Authentication (Default)
Uses standard kubeconfig file authentication. The server will look for kubeconfig in the default locations (`~/.kube/config`) or use the `KUBECONFIG` environment variable.

```bash
# Use default kubeconfig
mcp-kubernetes serve

# Use specific kubeconfig (via environment variable)
KUBECONFIG=/path/to/kubeconfig mcp-kubernetes serve
```

#### In-Cluster Authentication
Uses service account token when running inside a Kubernetes pod. This mode automatically uses the mounted service account credentials.

```bash
# Enable in-cluster authentication
mcp-kubernetes serve --in-cluster
```

**Requirements for in-cluster mode:**
- Must be running inside a Kubernetes pod
- Service account token must be mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`
- CA certificate must be available at `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`
- Namespace must be available at `/var/run/secrets/kubernetes.io/serviceaccount/namespace`

#### OAuth 2.1 Authentication
Uses OAuth 2.1 with Dex OIDC (default) or Google OAuth provider for secure, token-based authentication (available for HTTP transports only).

```bash
# Start with Dex OAuth authentication (default provider)
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://mcp.example.com \
  --dex-issuer-url=https://dex.example.com \
  --dex-client-id=mcp-kubernetes \
  --dex-client-secret=YOUR_DEX_SECRET \
  --dex-connector-id=github \
  --registration-token=YOUR_SECURE_TOKEN

# Or use Google OAuth provider
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-provider=google \
  --oauth-base-url=https://mcp.example.com \
  --google-client-id=YOUR_CLIENT_ID \
  --google-client-secret=YOUR_CLIENT_SECRET \
  --registration-token=YOUR_SECURE_TOKEN
```

**See [docs/oauth.md](docs/oauth.md) for detailed OAuth setup and configuration.**

#### Federation (Multi-Cluster with CAPI)
Enables access to multiple workload clusters through a management cluster using Cluster API (CAPI). The server discovers workload clusters dynamically and uses user impersonation for RBAC enforcement on each cluster.

```bash
# Enable federation mode with OAuth
mcp-kubernetes serve \
  --transport=streamable-http \
  --in-cluster \
  --enable-oauth \
  --downstream-oauth \
  --enable-capi \
  --oauth-base-url=https://mcp.example.com \
  --dex-issuer-url=https://dex.example.com \
  --dex-client-id=mcp-kubernetes \
  --dex-client-secret=YOUR_DEX_SECRET
```

**Security model:**
- **Management Cluster RBAC**: Users must have permission to list CAPI Cluster resources and read kubeconfig secrets
- **Workload Cluster RBAC**: Operations use impersonation headers, delegating authorization to each cluster's RBAC policies
- **User Isolation**: Cached clients are keyed by (cluster, user) pairs to prevent cross-user access

**Impersonation headers added to workload cluster requests:**
- `Impersonate-User`: User's email from OAuth
- `Impersonate-Group`: User's groups from OAuth
- `Impersonate-Extra-agent`: `mcp-kubernetes` (for audit trail)

**SSO Token Passthrough Mode (Alternative):**

For organizations where workload cluster API servers are configured with OIDC authentication, an alternative authentication mode forwards the user's SSO token directly to workload clusters instead of using impersonation:

```yaml
# Helm values for SSO passthrough mode
capiMode:
  enabled: true
  workloadClusterAuth:
    mode: "sso-passthrough"  # Instead of default "impersonation"
    caSecretSuffix: "-ca"    # CA-only secrets (no admin credentials needed)
```

| Aspect | Impersonation (default) | SSO Passthrough |
|--------|------------------------|-----------------|
| ServiceAccount privileges | High (secret read, impersonate) | Low (CA secret read only) |
| WC API server requirements | None | OIDC configuration required |
| Audit trail | Shows impersonated user | Shows direct OIDC user |

**See [docs/sso-passthrough-wc.md](docs/sso-passthrough-wc.md) for detailed configuration and requirements.**

### Observability

The server includes comprehensive OpenTelemetry instrumentation for production monitoring:

- **Metrics**: Prometheus-compatible metrics for HTTP requests, Kubernetes operations, and sessions
- **Distributed Tracing**: OpenTelemetry traces for request flows and K8s API calls
- **Metrics Endpoint**: `/metrics` endpoint for Prometheus scraping

```bash
# Enable instrumentation (enabled by default)
INSTRUMENTATION_ENABLED=true

# Configure metrics exporter (prometheus, otlp, stdout)
METRICS_EXPORTER=prometheus

# Configure tracing exporter (otlp, stdout, none)
TRACING_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# Set trace sampling rate (0.0 to 1.0)
OTEL_TRACES_SAMPLER_ARG=0.1
```

**Available Metrics:**
- `http_requests_total` - HTTP request counter
- `http_request_duration_seconds` - HTTP request duration histogram
- `kubernetes_operations_total` - Kubernetes operation counter
- `kubernetes_operation_duration_seconds` - K8s operation duration histogram
- `kubernetes_pod_operations_total` - Pod operation counter
- `oauth_downstream_auth_total` - OAuth authentication counter
- `active_port_forward_sessions` - Active port-forward sessions gauge

**See [docs/observability.md](docs/observability.md) for detailed metrics documentation, Prometheus queries, and alerting examples.**

### Transport Types

#### Standard I/O (Default)
For local development and direct integration with MCP clients:
```bash
mcp-kubernetes serve --transport stdio
```

#### Streamable HTTP (Recommended for Production)
For network-accessible deployments with OAuth support:
```bash
mcp-kubernetes serve --transport streamable-http --http-addr :8080
```

### Configuration Options

```bash
# Safety and operation modes
--non-destructive     # Enable non-destructive mode (default: true)
--dry-run            # Enable dry run mode (default: false)

# Performance tuning
--qps-limit 20.0     # QPS limit for Kubernetes API calls
--burst-limit 30     # Burst limit for Kubernetes API calls

# Authentication
--in-cluster                   # Use in-cluster authentication instead of kubeconfig
--enable-oauth                 # Enable OAuth 2.1 authentication (for HTTP transports)
--oauth-base-url string        # OAuth base URL (e.g., https://mcp.example.com)
--google-client-id string      # Google OAuth Client ID
--google-client-secret string  # Google OAuth Client Secret
--registration-token string    # OAuth client registration access token
--allow-public-registration    # Allow unauthenticated OAuth client registration

# Debugging
--debug              # Enable debug logging

# Transport-specific options
--transport string            # Transport type: stdio or streamable-http
--http-addr :8080            # HTTP server address (for streamable-http)
--http-endpoint /mcp         # HTTP endpoint path (default: /mcp)
--disable-streaming          # Disable streaming for streamable-http transport
```

## Running in Kubernetes

The recommended way to deploy mcp-kubernetes in a Kubernetes cluster is using the Helm chart, which handles RBAC, Ingress, TLS, and OAuth configuration.

### Using Helm (Recommended)

```bash
# Add the Giant Swarm app catalog
helm repo add giantswarm https://giantswarm.github.io/giantswarm-catalog/

# Install with default configuration
helm install mcp-kubernetes giantswarm/mcp-kubernetes

# Install with Ingress and TLS enabled
helm install mcp-kubernetes giantswarm/mcp-kubernetes \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=mcp.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix \
  --set ingress.tls[0].secretName=mcp-kubernetes-tls \
  --set ingress.tls[0].hosts[0]=mcp.example.com

# Install with OAuth enabled
helm install mcp-kubernetes giantswarm/mcp-kubernetes \
  --set mcpKubernetes.oauth.enabled=true \
  --set mcpKubernetes.oauth.baseURL=https://mcp.example.com \
  --set mcpKubernetes.oauth.dex.issuerURL=https://dex.example.com \
  --set mcpKubernetes.oauth.dex.clientID=mcp-kubernetes \
  --set mcpKubernetes.oauth.existingSecret=mcp-kubernetes-oauth
```

See [helm/mcp-kubernetes/values.yaml](helm/mcp-kubernetes/values.yaml) for all configuration options, including:
- **Ingress**: TLS termination, custom annotations, multiple hosts
- **OAuth 2.1**: Dex or Google providers, client registration, downstream OAuth
- **CAPI Mode**: Multi-cluster federation via Cluster API
- **Observability**: Prometheus ServiceMonitor, OpenTelemetry tracing

### Connecting to the MCP Server

Once deployed with Ingress enabled, MCP clients connect to the `/mcp` endpoint:

```json
{
  "mcpServers": {
    "kubernetes": {
      "url": "https://mcp.example.com/mcp"
    }
  }
}
```

The endpoint path is configurable via `mcpKubernetes.httpEndpoint` in the Helm values (default: `/mcp`).

## Available Tools

The MCP server provides the following tools:

### Resource Management
- `k8s_get_resource` - Get a specific resource
- `k8s_list_resources` - List resources with pagination
- `k8s_describe_resource` - Get detailed resource information
- `k8s_create_resource` - Create a new resource
- `k8s_apply_resource` - Apply resource configuration
- `k8s_delete_resource` - Delete a resource
- `k8s_patch_resource` - Patch a resource
- `k8s_scale_resource` - Scale deployments, replicasets, statefulsets

### Pod Operations
- `k8s_get_pod_logs` - Get logs from pod containers
- `k8s_exec_pod` - Execute commands in pod containers
- `k8s_port_forward_pod` - Set up port forwarding to pods
- `k8s_port_forward_service` - Set up port forwarding to services

### Context Management
- `k8s_list_contexts` - List available Kubernetes contexts
- `k8s_get_current_context` - Get the current context
- `k8s_switch_context` - Switch to a different context

### Cluster Information
- `k8s_get_api_resources` - Get available API resources
- `k8s_get_cluster_health` - Get cluster health information

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Linting

```bash
make lint
```

## Security

### Authentication & Authorization

- The server runs in non-destructive mode by default
- Supports dry-run mode for safe operation testing
- Allows restriction of operations and namespaces
- Follows Kubernetes RBAC when using in-cluster authentication
- OAuth 2.1 support with PKCE enforcement for HTTP transports
- Downstream OAuth authentication for per-user RBAC enforcement

### Secret Management (CRITICAL for Production)

**⚠️ PRODUCTION REQUIREMENT:** You **MUST** use a secret management solution for production deployments.

**Recommended Solutions:**
- HashiCorp Vault
- AWS Secrets Manager
- Google Cloud Secret Manager
- Azure Key Vault
- Kubernetes External Secrets Operator

**NEVER use environment variables for secrets in production** because they:
- Are visible in process listings (`ps aux`, `docker inspect`)
- Get leaked in logs and error messages
- Have no audit trail or rotation support
- Are not encrypted at rest
- Cannot be securely deleted

**For detailed setup and examples**, see:
- [OAuth Authentication Guide](docs/oauth.md#production-secret-management)
- [Production Security Checklist](docs/oauth.md#security-checklist-for-production-comprehensive)
- [Incident Response Procedures](docs/oauth.md#incident-response-procedures)

### Security Best Practices

**Development:**
- Use environment variables **only** for local development
- Never commit secrets to Git
- Use pre-commit hooks (gitleaks, git-secrets)
- Generate secrets securely (avoid shell history)

**Production:**
- Store secrets in a secret manager (required)
- Enable encryption at rest for Kubernetes Secrets
- Rotate encryption keys every 90 days
- Enable audit logging and monitoring
- Use HTTPS with valid TLS certificates
- Follow the [comprehensive production checklist](docs/oauth.md#security-checklist-for-production-comprehensive)

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
