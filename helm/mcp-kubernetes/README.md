# mcp-kubernetes Helm Chart

This Helm chart deploys the [mcp-kubernetes](https://github.com/giantswarm/mcp-kubernetes) server, a Model Context Protocol (MCP) server that provides Kubernetes cluster management capabilities.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installation

### Add the repository (if published)

```bash
helm repo add giantswarm https://giantswarm.github.io/helm-charts
helm repo update
```

### Install from local directory

```bash
# Clone the repository
git clone https://github.com/giantswarm/mcp-kubernetes.git
cd mcp-kubernetes

# Install the chart
helm install mcp-kubernetes ./helm/mcp-kubernetes
```

### Install with custom values

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes -f custom-values.yaml
```

## Configuration

The following table lists the configurable parameters of the mcp-kubernetes chart and their default values.

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Container image repository | `mcp-kubernetes` |
| `image.tag` | Container image tag | `""` (uses Chart.appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `nameOverride` | Override the name of the chart | `""` |
| `fullnameOverride` | Override the full name of the chart | `""` |

### Service Account

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create a service account | `true` |
| `serviceAccount.automount` | Automount service account token | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |

### Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `8080` |

### Ingress Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.className` | Ingress class name | `""` |
| `ingress.annotations` | Ingress annotations | `{}` |
| `ingress.hosts` | Ingress hosts configuration | See values.yaml |
| `ingress.tls` | Ingress TLS configuration | `[]` |

### Resource Management

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources` | CPU/Memory resource requests/limits | `{}` |
| `nodeSelector` | Node labels for pod assignment | `{}` |
| `tolerations` | Tolerations for pod assignment | `[]` |
| `affinity` | Affinity rules for pod assignment | `{}` |

### Autoscaling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable horizontal pod autoscaler | `false` |
| `autoscaling.minReplicas` | Minimum number of replicas | `1` |
| `autoscaling.maxReplicas` | Maximum number of replicas | `100` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization | `80` |
| `autoscaling.targetMemoryUtilizationPercentage` | Target memory utilization | `""` |

### MCP Kubernetes Specific Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `mcpKubernetes.port` | MCP server port | `8080` |
| `mcpKubernetes.logLevel` | Log level (debug, info, warn, error) | `info` |
| `mcpKubernetes.kubernetes.inCluster` | Use in-cluster Kubernetes configuration | `true` |
| `mcpKubernetes.kubernetes.kubeconfig` | Path to kubeconfig file | `""` |
| `mcpKubernetes.env` | Additional environment variables | `[]` |

## RBAC Permissions

The chart automatically creates a ClusterRole and ClusterRoleBinding that grants the following permissions:

- **Core resources**: pods, services, endpoints, nodes, namespaces, configmaps, secrets, persistentvolumes, persistentvolumeclaims, events
- **Apps resources**: deployments, replicasets, statefulsets, daemonsets
- **Networking resources**: ingresses, networkpolicies
- **Storage resources**: storageclasses, volumeattachments (read-only)
- **Metrics resources**: pods, nodes (read-only)
- **Custom resources**: customresourcedefinitions (read-only)
- **RBAC resources**: roles, rolebindings, clusterroles, clusterrolebindings
- **Batch resources**: jobs, cronjobs
- **Autoscaling resources**: horizontalpodautoscalers
- **Policy resources**: poddisruptionbudgets

## Usage Examples

### Basic Installation

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes
```

### Installation with Custom Image

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set image.repository=ghcr.io/giantswarm/mcp-kubernetes \
  --set image.tag=v1.0.0
```

### Installation with Ingress

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=mcp-kubernetes.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

### Installation with Resource Limits

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set resources.limits.cpu=500m \
  --set resources.limits.memory=512Mi \
  --set resources.requests.cpu=250m \
  --set resources.requests.memory=256Mi
```

### Installation with Autoscaling

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10 \
  --set autoscaling.targetCPUUtilizationPercentage=70
```

## Connecting to the MCP Server

Once deployed, you can connect to the MCP server using:

### Port Forward (for testing)

```bash
kubectl port-forward service/mcp-kubernetes 8080:8080
```

Then connect your MCP client to `http://localhost:8080`.

### In-Cluster Access

Use the service DNS name: `<release-name>-mcp-kubernetes.<namespace>.svc.cluster.local:8080`

### External Access (with Ingress)

Configure the ingress with your desired hostname and access via `https://your-domain.com`.

## Monitoring and Health Checks

The deployment includes:

- **Liveness Probe**: HTTP GET to `/health` on port 8080
- **Readiness Probe**: HTTP GET to `/health` on port 8080

## Troubleshooting

### Common Issues

1. **RBAC Permissions**: Ensure the service account has the necessary permissions. The chart creates a ClusterRole with broad permissions by default.

2. **Image Pull Issues**: Make sure the image exists and is accessible. Set `imagePullSecrets` if using private registries.

3. **Health Check Failures**: Verify that the `/health` endpoint is implemented in your mcp-kubernetes binary.

### Debugging

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=mcp-kubernetes

# Check pod logs
kubectl logs -l app.kubernetes.io/name=mcp-kubernetes

# Check service
kubectl get svc -l app.kubernetes.io/name=mcp-kubernetes

# Test connectivity
kubectl port-forward service/mcp-kubernetes 8080:8080
curl http://localhost:8080/health
```

## Contributing

Please read the [contributing guidelines](https://github.com/giantswarm/mcp-kubernetes/blob/main/CONTRIBUTING.md) before submitting pull requests.

## License

This chart is licensed under the Apache License 2.0. See the [LICENSE](https://github.com/giantswarm/mcp-kubernetes/blob/main/LICENSE) file for details.