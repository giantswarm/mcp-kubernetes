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
| `image.registry` | Container image registry | `gsoci.azurecr.io` |
| `image.repository` | Container image repository | `giantswarm/mcp-kubernetes` |
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
| `mcpKubernetes.kubernetes.inCluster` | Use in-cluster Kubernetes configuration | `true` |
| `mcpKubernetes.kubernetes.kubeconfig` | Path to kubeconfig file | `""` |
| `mcpKubernetes.env` | Additional environment variables | `[]` |

### OAuth 2.1 Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `mcpKubernetes.oauth.enabled` | Enable OAuth 2.1 authentication | `false` |
| `mcpKubernetes.oauth.baseURL` | OAuth base URL (required if OAuth enabled) | `""` |
| `mcpKubernetes.oauth.googleClientID` | Google OAuth Client ID | `""` |
| `mcpKubernetes.oauth.googleClientSecret` | Google OAuth Client Secret | `""` |
| `mcpKubernetes.oauth.registrationToken` | OAuth client registration access token | `""` |
| `mcpKubernetes.oauth.allowPublicRegistration` | Allow unauthenticated client registration (NOT recommended) | `false` |
| `mcpKubernetes.oauth.disableStreaming` | Disable streaming for HTTP transport | `false` |
| `mcpKubernetes.oauth.existingSecret` | Use existing secret for OAuth credentials | `""` |

**⚠️ SECURITY WARNING:** 

**For production deployments:**
- **MUST** use `existingSecret` - NEVER set credentials in values.yaml
- **MUST** use a secret management solution (see options below)
- **MUST** enable HTTPS with valid TLS certificates
- **MUST** set `allowPublicRegistration: false`

**Recommended Secret Management Solutions:**
- Kubernetes External Secrets Operator (recommended)
- HashiCorp Vault with Vault Secrets Operator
- AWS Secrets Store CSI Driver
- Google Secret Manager CSI Driver
- Azure Key Vault CSI Driver

See the [Production Secret Management](#production-secret-management) section below for detailed examples.

### Cilium Network Policy

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ciliumNetworkPolicy.enabled` | Create a CiliumNetworkPolicy | `true` |
| `ciliumNetworkPolicy.labels` | Additional labels for the CiliumNetworkPolicy | `{}` |
| `ciliumNetworkPolicy.annotations` | Additional annotations for the CiliumNetworkPolicy | `{}` |

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

## Network Security

The chart creates a CliumNetworkPolicy to control network traffic for the mcp-kubernetes pods. The policy provides:

- **Egress to kube-apiserver**: Required for Kubernetes API communication
- **Default deny**: All other traffic is blocked by default

The policy uses the standard chart selector labels (`app.kubernetes.io/name` and `app.kubernetes.io/instance`) to identify the target pods.

**Note**: CiliumNetworkPolicy requires Cilium CNI to be installed in your cluster.

## Usage Examples

### Basic Installation

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes
```

### Installation with Custom Image

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set image.registry=ghcr.io \
  --set image.repository=giantswarm/mcp-kubernetes \
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

### Installation without Network Policy

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set ciliumNetworkPolicy.enabled=false
```

### Installation with OAuth 2.1 Authentication

**⚠️ DEVELOPMENT ONLY:** The following example uses a manually created Kubernetes Secret, which is acceptable for development but **NOT recommended for production**.

**For production**, use a secret management solution (see [Production Secret Management](#production-secret-management)).

#### Development Example

```bash
# Development only - use secret manager in production!
kubectl create secret generic mcp-k8s-oauth-credentials \
  --from-literal=google-client-id=YOUR_CLIENT_ID \
  --from-literal=google-client-secret=YOUR_CLIENT_SECRET \
  --from-literal=registration-token=$(openssl rand -hex 32) \
  --from-literal=oauth-encryption-key=$(openssl rand -base64 32)
```

Then install with OAuth enabled:

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set mcpKubernetes.oauth.enabled=true \
  --set mcpKubernetes.oauth.baseURL=https://mcp-k8s.example.com \
  --set mcpKubernetes.oauth.existingSecret=mcp-k8s-oauth-credentials \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=mcp-k8s.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix \
  --set ingress.tls[0].secretName=mcp-k8s-tls \
  --set ingress.tls[0].hosts[0]=mcp-k8s.example.com
```

Or use the example values file:

```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  -f ./helm/mcp-kubernetes/values-oauth-example.yaml
```

**Important**: OAuth requires HTTPS in production. Make sure to configure TLS for your ingress.

## Production Secret Management

**⚠️ CRITICAL:** Production deployments **MUST** use a secret management solution. Basic Kubernetes Secrets are the **minimum acceptable standard** but still not ideal.

### Why Secret Managers Are Required

Environment variables and manually created Kubernetes Secrets are **NOT secure** for production because they:
- Are visible in process listings (`ps aux`, `kubectl describe pod`)
- Get leaked in logs, error messages, and crash dumps
- Have no built-in audit trail or rotation capabilities
- Lack encryption at rest (unless explicitly enabled)
- Cannot be securely deleted from memory
- No centralized access control

### Recommended Solutions

#### 1. External Secrets Operator (Recommended for Kubernetes)

The External Secrets Operator syncs secrets from external secret managers into Kubernetes Secrets.

**Installation:**
```bash
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace \
  --set installCRDs=true
```

**AWS Secrets Manager Example:**
```yaml
# SecretStore - Configure connection to AWS Secrets Manager
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secretsmanager
  namespace: default
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: mcp-kubernetes

---
# ExternalSecret - Define which secrets to sync
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: mcp-oauth-credentials
  namespace: default
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore
  target:
    name: mcp-oauth-credentials
    creationPolicy: Owner
  data:
  - secretKey: google-client-id
    remoteRef:
      key: mcp-kubernetes/oauth
      property: google-client-id
  - secretKey: google-client-secret
    remoteRef:
      key: mcp-kubernetes/oauth
      property: google-client-secret
  - secretKey: oauth-encryption-key
    remoteRef:
      key: mcp-kubernetes/oauth
      property: oauth-encryption-key
  - secretKey: registration-token
    remoteRef:
      key: mcp-kubernetes/oauth
      property: registration-token
```

**Then deploy mcp-kubernetes:**
```bash
helm install mcp-kubernetes ./helm/mcp-kubernetes \
  --set mcpKubernetes.oauth.enabled=true \
  --set mcpKubernetes.oauth.baseURL=https://mcp-k8s.example.com \
  --set mcpKubernetes.oauth.existingSecret=mcp-oauth-credentials \
  --set ingress.enabled=true \
  --set ingress.tls[0].secretName=mcp-k8s-tls \
  --set ingress.tls[0].hosts[0]=mcp-k8s.example.com
```

#### 2. HashiCorp Vault

**Installation:**
```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
helm install vault hashicorp/vault \
  --set "injector.enabled=true" \
  --set "server.dev.enabled=true"  # Dev mode for testing only!
```

**Store secrets in Vault:**
```bash
vault kv put secret/mcp-kubernetes/oauth \
  google-client-id="YOUR_CLIENT_ID" \
  google-client-secret="YOUR_CLIENT_SECRET" \
  oauth-encryption-key="$(openssl rand -base64 32)" \
  registration-token="$(openssl rand -hex 32)"
```

**Deploy with Vault annotations:**
```yaml
# values-vault.yaml
mcpKubernetes:
  oauth:
    enabled: true
    baseURL: https://mcp-k8s.example.com

podAnnotations:
  vault.hashicorp.com/agent-inject: "true"
  vault.hashicorp.com/role: "mcp-kubernetes"
  vault.hashicorp.com/agent-inject-secret-oauth: "secret/data/mcp-kubernetes/oauth"
  vault.hashicorp.com/agent-inject-template-oauth: |
    {{- with secret "secret/data/mcp-kubernetes/oauth" -}}
    export GOOGLE_CLIENT_ID="{{ .Data.data.google-client-id }}"
    export GOOGLE_CLIENT_SECRET="{{ .Data.data.google-client-secret }}"
    export OAUTH_ENCRYPTION_KEY="{{ .Data.data.oauth-encryption-key }}"
    export REGISTRATION_TOKEN="{{ .Data.data.registration-token }}"
    {{- end }}

# Modify container command to source secrets
containers:
  - name: mcp-kubernetes
    command: ["/bin/sh", "-c"]
    args:
      - source /vault/secrets/oauth && exec /app/mcp-kubernetes serve --enable-oauth ...
```

#### 3. Cloud Provider Secret Managers

**AWS Secrets Store CSI Driver:**
```yaml
# Install the driver
helm repo add secrets-store-csi-driver https://kubernetes-sigs.github.io/secrets-store-csi-driver/charts
helm install csi-secrets-store secrets-store-csi-driver/secrets-store-csi-driver \
  --namespace kube-system

# Install AWS provider
kubectl apply -f https://raw.githubusercontent.com/aws/secrets-store-csi-driver-provider-aws/main/deployment/aws-provider-installer.yaml

# SecretProviderClass
apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: mcp-oauth-secrets
spec:
  provider: aws
  parameters:
    objects: |
      - objectName: "mcp-kubernetes/oauth"
        objectType: "secretsmanager"
        jmesPath:
          - path: google-client-id
            objectAlias: google-client-id
          - path: google-client-secret
            objectAlias: google-client-secret
          - path: oauth-encryption-key
            objectAlias: oauth-encryption-key
          - path: registration-token
            objectAlias: registration-token
  secretObjects:
  - secretName: mcp-oauth-credentials
    type: Opaque
    data:
    - objectName: google-client-id
      key: google-client-id
    - objectName: google-client-secret
      key: google-client-secret
    - objectName: oauth-encryption-key
      key: oauth-encryption-key
    - objectName: registration-token
      key: registration-token

# Update values.yaml to mount the CSI volume
volumes:
- name: secrets-store
  csi:
    driver: secrets-store.csi.k8s.io
    readOnly: true
    volumeAttributes:
      secretProviderClass: mcp-oauth-secrets

volumeMounts:
- name: secrets-store
  mountPath: "/mnt/secrets-store"
  readOnly: true
```

**Google Secret Manager CSI Driver:**
```bash
# Install the driver
kubectl apply -f https://github.com/GoogleCloudPlatform/secrets-store-csi-driver-provider-gcp/releases/latest/download/provider-gcp-installer.yaml

# Similar configuration to AWS, using GCP-specific parameters
```

**Azure Key Vault CSI Driver:**
```bash
# Install the driver
helm repo add csi-secrets-store-provider-azure https://azure.github.io/secrets-store-csi-driver-provider-azure/charts
helm install csi-secrets-store-provider-azure csi-secrets-store-provider-azure/csi-secrets-store-provider-azure
```

### Production Security Checklist

Before deploying to production, verify:

**Secret Management:**
- [ ] Using External Secrets Operator or equivalent secret manager
- [ ] Secrets are NOT hardcoded in values.yaml
- [ ] Secrets are NOT stored as plain environment variables
- [ ] Secret rotation procedure is documented and tested
- [ ] Access to secrets is logged and monitored
- [ ] Encryption at rest is enabled for Kubernetes Secrets

**Network Security:**
- [ ] HTTPS is enforced (ingress.tls is configured)
- [ ] TLS certificates are from a trusted CA (Let's Encrypt, commercial CA)
- [ ] `allowPublicRegistration: false` is set
- [ ] CORS origins are validated and minimal
- [ ] Network policies are configured and tested

**Application Security:**
- [ ] OAuth encryption key is exactly 32 bytes
- [ ] Registration token is cryptographically random
- [ ] Resource limits are set appropriately
- [ ] Security context prevents privilege escalation
- [ ] Container runs as non-root user

**Monitoring & Operations:**
- [ ] Audit logging is enabled
- [ ] Metrics are collected and monitored
- [ ] Alerts are configured for security events
- [ ] Incident response plan exists
- [ ] Backup and disaster recovery tested

**Supply Chain Security:**
- [ ] Container images are scanned for vulnerabilities
- [ ] Images are from trusted registries
- [ ] Image pull secrets are configured
- [ ] Dependency updates are monitored
- [ ] SBOM is available for deployed version

### Encryption Key Rotation

**Recommended Schedule:**
- Regular rotation: Every 90 days
- After incidents: Immediately
- Staff changes: Within 24 hours

**Rotation Procedure:**
1. Generate new encryption key: `openssl rand -base64 32`
2. Update secret in your secret manager
3. Wait for External Secrets Operator to sync (check `refreshInterval`)
4. Restart pods to pick up new key: `kubectl rollout restart deployment/mcp-kubernetes`
5. Wait for token expiration (typically 1 hour)
6. Verify new tokens are being issued successfully

### Monitoring Secret Sync

**Check External Secrets status:**
```bash
# Verify ExternalSecret is synced
kubectl get externalsecret mcp-oauth-credentials -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'

# Check for errors
kubectl describe externalsecret mcp-oauth-credentials

# View last sync time
kubectl get externalsecret mcp-oauth-credentials -o jsonpath='{.status.syncedResourceVersion}'
```

**Set up alerts:**
```yaml
# Prometheus alert for failed secret sync
- alert: ExternalSecretSyncFailed
  expr: |
    external_secrets_sync_calls_error{name="mcp-oauth-credentials"} > 0
  for: 5m
  annotations:
    summary: "External Secret sync failing for mcp-oauth-credentials"
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
