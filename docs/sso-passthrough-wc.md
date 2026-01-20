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
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WC_AUTH_MODE` | Authentication mode: `impersonation` or `sso-passthrough` | `impersonation` |
| `WC_CA_SECRET_SUFFIX` | Suffix for CA-only secrets | `-ca` |

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

In SSO passthrough mode, mcp-kubernetes only needs the cluster's CA certificate, not admin credentials. Create CA-only secrets using a policy engine like Kyverno:

```yaml
# Example Kyverno ClusterPolicy to create CA-only secrets
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: generate-ca-secrets
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
          type: Opaque
          data:
            ca.crt: |
              # Extract CA from kubeconfig - requires custom logic
              {{request.object.data.value | base64_decode | parseYaml | 
                index 'clusters' 0 | index 'cluster' | index 'certificate-authority-data'}}
```

See the [fine-grained-certs-poc](https://github.com/giantswarm/fine-grained-certs-poc) repository for a complete example implementation.

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
