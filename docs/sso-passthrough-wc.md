# SSO Token Passthrough to Workload Clusters

This document describes the SSO token passthrough authentication mode for workload clusters, an alternative to the default impersonation-based authentication.

## Overview

mcp-kubernetes supports two authentication modes for connecting to workload clusters in CAPI mode:

1. **Impersonation Mode** (default): Uses admin credentials from kubeconfig secrets with user impersonation headers
2. **SSO Passthrough Mode**: Forwards the user's OAuth/OIDC ID token directly to workload cluster API servers

## Comparison

| Aspect | Impersonation (default) | SSO Passthrough |
|--------|------------------------|-----------------|
| ServiceAccount privileges | High (kubeconfig secrets + impersonate) | **Low (CA ConfigMaps only)** |
| Credential exposure | Admin creds in kubeconfig secrets | **No credentials needed** |
| Secret access | Required (kubeconfig secrets) | **Not required** |
| WC audit logs | Shows impersonated user | Shows direct OIDC user |
| RBAC enforcement | Via impersonation headers | Via WC OIDC validation |
| Requirements | Kubeconfig secrets | WC OIDC configuration + CA ConfigMaps |
| Trust boundary | mcp-kubernetes impersonation | IdP + WC OIDC config |
| Fallback behavior | N/A | **None (fails if token rejected)** |

### ServiceAccount Permission Differences

**Impersonation mode** requires the ServiceAccount to:
- Read kubeconfig secrets (contain admin credentials)
- These secrets grant full admin access to workload clusters
- The ServiceAccount effectively has "impersonate any user" capability

**SSO Passthrough mode** requires the ServiceAccount to:
- Read CA ConfigMaps (contain only the cluster's CA certificate - public key)
- Read CAPI Cluster resources (to get the API endpoint)
- **No access to secrets at all**

This is a significant security improvement for organizations that want to minimize privileged access. Since CA certificates are public keys (used for TLS verification), they are stored in ConfigMaps rather than Secrets.

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
    # ConfigMap suffix for CA certificates (used in sso-passthrough mode)
    # The full ConfigMap name is: ${CLUSTER_NAME}${caConfigMapSuffix}
    caConfigMapSuffix: "-ca-public"
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
    caConfigMapSuffix: "-ca-public"
    # Enable for high-security deployments
    disableCaching: true
```

**Trade-off:** Disabling caching increases latency per request since each request creates a fresh client connection. For most deployments, keeping caching enabled with a conservative TTL (e.g., 10 minutes) provides a good balance.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WC_AUTH_MODE` | Authentication mode: `impersonation` or `sso-passthrough` | `impersonation` |
| `WC_CA_CONFIGMAP_SUFFIX` | Suffix for CA ConfigMaps | `-ca-public` |
| `WC_DISABLE_CACHING` | Disable client caching in SSO passthrough mode | `false` |

## Requirements

### What the ServiceAccount Still Needs

Even though SSO passthrough eliminates the need for kubeconfig secrets (admin credentials), the ServiceAccount still requires access to:

1. **CA Certificates** - To establish TLS connections to workload cluster API servers
   - Stored in ConfigMaps (e.g., `my-cluster-ca-public`)
   - Contains only the cluster's public CA certificate
   - No sensitive data (CA certificates are public keys)

2. **Cluster Endpoints** - To know where to connect
   - Read from CAPI Cluster resources (`spec.controlPlaneEndpoint`)
   - Requires read access to `clusters.cluster.x-k8s.io`

The RBAC configuration automatically adjusts based on the auth mode:
- **Impersonation mode**: Creates Roles for secret access
- **SSO Passthrough mode**: Creates Roles for ConfigMap access (no secret access)

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

### CA ConfigMaps

In SSO passthrough mode, mcp-kubernetes only needs the cluster's CA certificate, not admin credentials. Since CA certificates are public keys (used for TLS verification), they are stored in ConfigMaps rather than Secrets.

**ConfigMap Structure:**

An operator should create ConfigMaps with the following structure:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-cluster-ca-public
  namespace: org-giantswarm
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
    giantswarm.io/cluster: my-cluster
data:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    MIICxjCCAa6gAwIBAgIBADANBg...
    -----END CERTIFICATE-----
```

**Operator Requirements:**

An operator is needed to:
1. Watch for CAPI Cluster resources or the `${CLUSTER_NAME}-ca` secrets
2. Extract the CA certificate (`tls.crt`) from the CAPI-generated `${CLUSTER_NAME}-ca` secret
3. Create a ConfigMap named `${CLUSTER_NAME}-ca-public` with the CA certificate

**Note:** The CAPI-generated `${CLUSTER_NAME}-ca` secret contains both:
- `tls.crt` - The CA certificate (public key) - this is what we need
- `tls.key` - The CA private key (extremely sensitive) - we do NOT need this

**Manual ConfigMap Creation (for testing):**

```bash
# Extract CA certificate from the CAPI-generated secret
kubectl get secret my-cluster-ca -n org-giantswarm -o jsonpath='{.data.tls\.crt}' | \
  base64 -d > ca.crt

# Create the CA ConfigMap
kubectl create configmap my-cluster-ca-public \
  --from-file=ca.crt=ca.crt \
  -n org-giantswarm

# Add the cluster label
kubectl label configmap my-cluster-ca-public \
  cluster.x-k8s.io/cluster-name=my-cluster \
  -n org-giantswarm
```

### ConfigMap Naming Convention

The CA ConfigMap name follows this convention:
```
${CLUSTER_NAME}${caConfigMapSuffix}
```

For example, with the default suffix `-ca-public`:
- Cluster name: `my-cluster`
- CA ConfigMap name: `my-cluster-ca-public`

The ConfigMap should contain the CA certificate in the `ca.crt` key:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-cluster-ca-public
  namespace: org-giantswarm
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
data:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
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

1. **No secret access required**: ServiceAccount only reads CA ConfigMaps (public keys), not any secrets
2. **Eliminated privilege escalation risk**: Cannot extract admin kubeconfig even with ServiceAccount compromise
3. **Simpler trust model**: mcp-kubernetes only needs the WC endpoint and CA certificate
4. **Direct authentication**: User identity is verified by the WC API server itself, not via impersonation
5. **Better audit trail**: Kubernetes audit logs show direct OIDC authentication, not impersonated requests
6. **Aligns with enterprise requirements**: Organizations that manage RBAC via groups prefer direct token authentication

### Limitations

1. Requires OIDC configuration on all workload cluster API servers
2. All clusters must trust the same Identity Provider (or compatible chain)
3. API servers must accept tokens with the upstream aggregator's audience
4. More complex initial setup compared to impersonation mode

### No Graceful Fallback (By Design)

**Important:** There is no automatic fallback from SSO passthrough to impersonation mode. This is an intentional security design decision.

If SSO passthrough authentication fails (e.g., token rejected by the WC API server, OIDC misconfiguration), the operation will fail. The user cannot access the workload cluster until the issue is resolved.

**Why no fallback?**

- Automatic fallback would require the ServiceAccount to maintain access to kubeconfig secrets with admin credentials
- This would defeat the purpose of SSO passthrough (reduced privilege requirements)
- Mixing auth modes per-request creates unpredictable security behavior
- Clear failure modes are easier to debug than silent fallback

**Operators must choose one mode per deployment:**

| Mode | ServiceAccount Needs | Failure Behavior |
|------|---------------------|------------------|
| `impersonation` | Kubeconfig secrets (admin creds) | Falls back to user RBAC via impersonation |
| `sso-passthrough` | CA ConfigMaps only (no secrets) | Fails if SSO token not accepted |

If you're unsure which mode works for your environment, test SSO passthrough in a non-production cluster first.

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

### CA ConfigMap Not Found

1. **Check ConfigMap exists**: `kubectl get configmap ${CLUSTER_NAME}-ca-public -n ${NAMESPACE}`
2. **Check ConfigMap key**: The ConfigMap must contain a `ca.crt` key
3. **Check RBAC**: The mcp-kubernetes ServiceAccount must have permission to read ConfigMaps
4. **Check operator**: Ensure the CA ConfigMap operator is running and creating ConfigMaps

### Connection Timeout

1. **Check network connectivity**: Ensure mcp-kubernetes can reach the WC API server endpoint
2. **Check endpoint**: The cluster's `spec.controlPlaneEndpoint` must be resolvable and reachable
3. **Check firewall rules**: Ensure the WC API server port (usually 6443) is accessible

## Related Documentation

- [SSO Token Forwarding](sso-token-forwarding.md) - How SSO tokens are forwarded from aggregators
- [RBAC Security](rbac-security.md) - RBAC requirements for different deployment modes
- [OAuth Configuration](oauth.md) - OAuth/OIDC setup guide
