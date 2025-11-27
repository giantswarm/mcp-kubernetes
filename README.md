# mcp-kubernetes

A Model Context Protocol (MCP) server that provides tools for interacting with Kubernetes clusters.

## Features

- **Resource Management**: Get, list, describe, create, apply, delete, and patch Kubernetes resources
- **Pod Operations**: Get logs, execute commands, and set up port forwarding
- **Context Management**: List, get, and switch between Kubernetes contexts
- **Cluster Information**: Get API resources and cluster health status
- **Multiple Authentication Modes**: Support for kubeconfig, in-cluster, and OAuth 2.1 authentication
- **Multiple Transport Types**: Support for stdio, SSE, and streamable HTTP
- **Safety Features**: Non-destructive mode, dry-run capability, and operation restrictions
- **OAuth 2.1 Support**: Secure token-based authentication with Google OAuth provider

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

The server supports two authentication modes:

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
Uses OAuth 2.1 with Google OAuth provider for secure, token-based authentication (available for HTTP transports only).

```bash
# Start with OAuth authentication
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://mcp.example.com \
  --google-client-id=YOUR_CLIENT_ID \
  --google-client-secret=YOUR_CLIENT_SECRET \
  --registration-token=YOUR_SECURE_TOKEN
```

**See [docs/oauth.md](docs/oauth.md) for detailed OAuth setup and configuration.**

### Transport Types

#### Standard I/O (Default)
```bash
mcp-kubernetes serve --transport stdio
```

#### Server-Sent Events (SSE)
```bash
mcp-kubernetes serve --transport sse --http-addr :8080
```

#### Streamable HTTP
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
--transport string            # Transport type: stdio, sse, or streamable-http
--http-addr :8080            # HTTP server address (for sse and streamable-http)
--sse-endpoint /sse          # SSE endpoint path
--message-endpoint /message  # Message endpoint path
--http-endpoint /mcp         # HTTP endpoint path
--disable-streaming          # Disable streaming for streamable-http transport
```

## Running in Kubernetes

To run mcp-kubernetes as a pod in your Kubernetes cluster:

### 1. Create RBAC Resources

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mcp-kubernetes
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-kubernetes
rules:
- apiGroups: [""]
  resources: ["*"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: ["apps"]
  resources: ["*"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: ["batch"]
  resources: ["*"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
# Add more API groups as needed
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mcp-kubernetes
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: mcp-kubernetes
subjects:
- kind: ServiceAccount
  name: mcp-kubernetes
  namespace: default
```

### 2. Create Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-kubernetes
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mcp-kubernetes
  template:
    metadata:
      labels:
        app: mcp-kubernetes
    spec:
      serviceAccountName: mcp-kubernetes
      containers:
      - name: mcp-kubernetes
        image: ghcr.io/giantswarm/mcp-kubernetes:latest
        args:
        - "serve"
        - "--in-cluster"
        - "--transport=sse"
        - "--http-addr=:8080"
        ports:
        - containerPort: 8080
          name: http
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
```

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

- The server runs in non-destructive mode by default
- Supports dry-run mode for safe operation testing
- Allows restriction of operations and namespaces
- Follows Kubernetes RBAC when using in-cluster authentication

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
