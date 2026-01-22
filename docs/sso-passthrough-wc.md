# SSO Token Passthrough to Workload Clusters

This document describes the SSO token passthrough authentication mode for workload clusters, an alternative to the default impersonation-based authentication.

## Overview

mcp-kubernetes supports two authentication modes for connecting to workload clusters in CAPI mode:

1. **Impersonation Mode** (default): Uses admin credentials from kubeconfig secrets with user impersonation headers
2. **SSO Passthrough Mode**: Forwards the user's OAuth/OIDC ID token directly to workload cluster API servers

## Comparison

| Aspect | Impersonation (default) | SSO Passthrough |
|--------|------------------------|-----------------|
| ServiceAccount privileges | High (secret read, impersonate) | Low (CA secret read only) |
| Credential exposure | Admin creds in kubeconfig secrets | No admin credentials needed |
| WC audit logs | Shows impersonated user | Shows direct OIDC user |
| RBAC enforcement | Via impersonation headers | Via WC OIDC validation |
| Requirements | Kubeconfig secrets | WC OIDC configuration |
| Trust boundary | mcp-kubernetes impersonation | IdP + WC OIDC config |

## When to Use SSO Passthrough

Use SSO passthrough when:
- Workload cluster API servers are configured with OIDC authentication
- The same Identity Provider (e.g., Dex) is trusted by all clusters
- You want to reduce ServiceAccount privilege requirements
- You want direct user authentication at the workload cluster level
- Your organization manages RBAC via groups (not per-user bindings)

## When to Use Impersonation

Use impersonation (default) when:
- Workload clusters don't have OIDC configured
- Different Identity Providers are used for management and workload clusters
- You need compatibility with existing deployments
- You cannot modify workload cluster API server configurations

## Configuration

### Helm Values

```yaml
capiMode:
  enabled: true
  workloadClusterAuth:
    # Authentication mode: "impersonation" (default) or "sso-passthrough"
    mode: "sso-passthrough"
    # Secret suffix for CA-only secrets (used in sso-passthrough mode)
    # The full secret name is: ${CLUSTER_NAME}${caSecretSuffix}
    caSecretSuffix: "-ca"
    # Disable client caching for high-security deployments (optional)
    disableCaching: false
```

### High-Security Configuration

For deployments with strict security requirements, you can disable client caching to ensure:
- Tokens are never cached beyond their natural lifetime
- Token revocation takes effect immediately
- No risk of reusing expired tokens from cache

```yaml
capiMode:
  enabled: true
  workloadClusterAuth:
    mode: "sso-passthrough"
    caSecretSuffix: "-ca"
    # Enable for high-security deployments
    disableCaching: true
```

**Trade-off:** Disabling caching increases latency per request since each request creates a fresh client connection. For most deployments, keeping caching enabled with a conservative TTL (e.g., 10 minutes) provides a good balance.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WC_AUTH_MODE` | Authentication mode: `impersonation` or `sso-passthrough` | `impersonation` |
| `WC_CA_SECRET_SUFFIX` | Suffix for CA-only secrets | `-ca` |
| `WC_DISABLE_CACHING` | Disable client caching in SSO passthrough mode | `false` |

## Requirements

### Workload Cluster API Server Configuration

For SSO passthrough to work, workload cluster API servers must be configured with OIDC authentication:

```yaml
# kube-apiserver configuration
--oidc-issuer-url=https://dex.example.com
--oidc-client-id=muster-client  # Must match the token's audience
--oidc-username-claim=email
--oidc-groups-claim=groups
```

**Important**: The `--oidc-client-id` must match the audience claim in the SSO token. When using SSO token forwarding from an aggregator like muster, this is typically the aggregator's client ID.

For Kubernetes 1.29+, you can accept multiple audiences:

```yaml
--oidc-issuer-url=https://dex.example.com
--oidc-client-id=kubernetes
--oidc-username-claim=email
--oidc-groups-claim=groups
--api-audiences=kubernetes,muster-client,mcp-kubernetes-client
```

### CA-Only Secrets

In SSO passthrough mode, mcp-kubernetes only needs the cluster's CA certificate, not admin credentials. Create CA-only secrets using a policy engine like Kyverno.

**Important:** Extracting the CA from a kubeconfig YAML within Kyverno requires custom logic because:
1. The kubeconfig is stored as base64-encoded YAML in the secret
2. The CA is nested within the kubeconfig structure

**Recommended Approach:** Use the [fine-grained-certs-poc](https://github.com/giantswarm/fine-grained-certs-poc) repository which provides a complete, tested implementation including:
- Kyverno policy with proper YAML parsing
- Helper functions for CA extraction
- Example manifests and test cases

**Alternative: Manual Secret Creation**

If you prefer to create CA secrets manually or via automation:

```yaml
# Example CA-only secret structure
apiVersion: v1
kind: Secret
metadata:
  name: my-cluster-ca
  namespace: org-giantswarm
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
type: Opaque
data:
  # Base64-encoded CA certificate (same as certificate-authority-data from kubeconfig)
  ca.crt: LS0tLS1CRUdJTi...
```

**Extracting CA from existing kubeconfig secret:**

```bash
# Extract CA from a kubeconfig secret
kubectl get secret my-cluster-kubeconfig -n org-giantswarm -o jsonpath='{.data.value}' | \
  base64 -d | yq '.clusters[0].cluster.certificate-authority-data' | \
  base64 -d > ca.crt

# Create the CA-only secret
kubectl create secret generic my-cluster-ca \
  --from-file=ca.crt=ca.crt \
  -n org-giantswarm
```

**Kyverno Policy Skeleton:**

```yaml
# NOTE: This is a simplified example. See fine-grained-certs-poc for production use.
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: generate-ca-secrets
  annotations:
    policies.kyverno.io/description: |
      Generates CA-only secrets from kubeconfig secrets for SSO passthrough mode.
      See: https://github.com/giantswarm/fine-grained-certs-poc
spec:
  rules:
    - name: copy-ca-from-kubeconfig
      match:
        resources:
          kinds:
            - Secret
          names:
            - "*-kubeconfig"
      generate:
        apiVersion: v1
        kind: Secret
        name: "{{request.object.metadata.name | replace('-kubeconfig', '-ca')}}"
        namespace: "{{request.object.metadata.namespace}}"
        synchronize: true
        data:
          kind: Secret
          type: Opaque
          # The CA extraction requires custom JMESPath - see fine-grained-certs-poc
          # for a complete implementation with proper YAML parsing
```

### Secret Naming Convention

The CA secret name follows this convention:
```
${CLUSTER_NAME}${caSecretSuffix}
```

For example, with the default suffix `-ca`:
- Cluster name: `my-cluster`
- CA secret name: `my-cluster-ca`

The secret should contain the CA certificate in the `ca.crt` key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-cluster-ca
  namespace: org-giantswarm
type: Opaque
data:
  ca.crt: <base64-encoded-ca-certificate>
```

## Security Model

### SSO Passthrough Flow

```
User ─── authenticates ───> Aggregator (muster)
                               │
                          forwards ID token
                               │
                               ▼
                          mcp-kubernetes (validates via TrustedAudiences)
                               │
                          forwards same ID token
                               │
                               ▼
                          Workload Cluster API Server
                               │
                          validates via OIDC config
                               │
                          User's own RBAC applies
```

### Benefits

1. **Reduced privilege requirements**: ServiceAccount only needs access to CA secrets, not full kubeconfig secrets containing admin credentials
2. **Simpler trust model**: mcp-kubernetes only needs the WC endpoint and CA certificate, not admin credentials
3. **Direct authentication**: User identity is verified by the WC API server itself, not via impersonation
4. **Better audit trail**: Kubernetes audit logs show direct OIDC authentication, not impersonated requests
5. **Aligns with enterprise requirements**: Organizations that manage RBAC via groups prefer direct token authentication

### Limitations

1. Requires OIDC configuration on all workload cluster API servers
2. All clusters must trust the same Identity Provider (or compatible chain)
3. API servers must accept tokens with the upstream aggregator's audience
4. More complex initial setup compared to impersonation mode

### Audience Configuration

**Important:** The SSO token forwarded to workload clusters contains an audience claim (`aud`) that identifies who the token was issued for. The WC API server must be configured to accept this audience.

**The Challenge:**

When users authenticate through an aggregator (like muster), their token's audience is typically the aggregator's client ID (e.g., `muster-client`). But the WC API server may be configured with a different `--oidc-client-id` (e.g., `kubernetes` or `dex-k8s-authenticator`).

**Solutions:**

1. **Configure multiple audiences (Kubernetes 1.29+):**
   ```yaml
   # kube-apiserver configuration
   --oidc-issuer-url=https://dex.example.com
   --oidc-client-id=kubernetes
   --api-audiences=kubernetes,muster-client,mcp-kubernetes-client
   ```

2. **Use a shared client ID across the ecosystem:**
   Configure both the aggregator and WC API servers to use the same OIDC client ID.

3. **Structured Authentication (Kubernetes 1.34+):**
   Kubernetes 1.34+ supports multiple OIDC providers via structured authentication config, allowing different audiences per provider.

**Current Limitation:**

mcp-kubernetes does not currently support per-cluster audience configuration. The token is forwarded as-is with its original audience. If your WC API servers require different audiences, you'll need to:
- Configure the WC API servers to accept the upstream token's audience (recommended)
- Use impersonation mode instead (fallback)

**Future Enhancement:**

Per-cluster audience configuration or token exchange may be added in a future release. See [issue #240](https://github.com/giantswarm/mcp-kubernetes/issues/240) for updates.

### Token Lifetime and Cache TTL

**Important:** In SSO passthrough mode, the user's OAuth token is embedded in the Kubernetes client configuration and cached. If the cache TTL exceeds the token lifetime, operations will fail with authentication errors when using cached clients with expired tokens.

**Configuration:**

```yaml
# Helm values
capiMode:
  cache:
    # MUST be <= your OAuth token lifetime
    ttl: "10m"  # Default: 10 minutes
```

**Recommendations:**

1. **Set cache TTL <= token lifetime:** If your IdP issues tokens with 1-hour lifetime, set cache TTL to 55 minutes or less.

2. **Check your IdP configuration:** Most OIDC providers (Dex, Google, etc.) issue tokens with 1-hour lifetime by default.

3. **Monitor the warning:** mcp-kubernetes logs a warning at startup if the configured cache TTL exceeds the expected token lifetime:
   ```
   WARN cache TTL exceeds OAuth token lifetime cache_ttl=2h token_lifetime=1h
   ```

4. **Consider shorter TTLs for dynamic environments:** Shorter TTLs (5-15 minutes) improve security by limiting the window of token reuse.

**Current Limitation:**

mcp-kubernetes does not proactively refresh tokens for cached clients. When a cached client's token expires:
- The next operation using that client will fail with an authentication error
- The client will be evicted from cache on the next cleanup cycle
- A new client will be created with a fresh token on the next request

**Mitigation Options:**

For production deployments, choose one of these approaches:

1. **Conservative cache TTL (recommended for most deployments):**
   - Set cache TTL values shorter than token lifetime (default 10m is usually safe)
   - Good balance between security and performance

2. **Disable caching entirely (high-security deployments):**
   ```yaml
   capiMode:
     workloadClusterAuth:
       mode: "sso-passthrough"
       disableCaching: true
   ```
   - Each request uses a fresh client with the current token
   - Token revocation takes effect immediately
   - Trade-off: Higher latency per request

3. **Monitor and alert:**
   - Monitor `mcp_wc_auth_total{result="token_expired"}` metric (if enabled)
   - Implement retry logic in calling applications

## Troubleshooting

### Token Not Accepted by Workload Cluster

1. **Check audience claim**: Ensure the WC API server's `--oidc-client-id` matches the token's `aud` claim
2. **Check issuer**: Ensure the WC API server's `--oidc-issuer-url` matches the token's `iss` claim
3. **Verify OIDC configuration**: Confirm the WC API server can reach the IdP's JWKS endpoint

### CA Secret Not Found

1. **Check secret exists**: `kubectl get secret ${CLUSTER_NAME}-ca -n ${NAMESPACE}`
2. **Check secret key**: The secret must contain a `ca.crt` key
3. **Check RBAC**: The mcp-kubernetes ServiceAccount must have permission to read CA secrets

### Connection Timeout

1. **Check network connectivity**: Ensure mcp-kubernetes can reach the WC API server endpoint
2. **Check endpoint**: The cluster's `spec.controlPlaneEndpoint` must be resolvable and reachable
3. **Check firewall rules**: Ensure the WC API server port (usually 6443) is accessible

## Related Documentation

- [SSO Token Forwarding](sso-token-forwarding.md) - How SSO tokens are forwarded from aggregators
- [RBAC Security](rbac-security.md) - RBAC requirements for different deployment modes
- [OAuth Configuration](oauth.md) - OAuth/OIDC setup guide
