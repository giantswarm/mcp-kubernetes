# RBAC Security for mcp-kubernetes

This document describes the RBAC (Role-Based Access Control) configuration and security model for `mcp-kubernetes`, including both single-cluster and multi-cluster (CAPI) federation modes.

## Authentication Modes

`mcp-kubernetes` supports two distinct authentication modes that fundamentally change how RBAC permissions are evaluated:

### 1. Service Account Mode (Default)

When OAuth is disabled or `enableDownstreamOAuth` is false:

- The server runs with its own **ServiceAccount credentials**
- All Kubernetes API calls use the ServiceAccount's identity
- **ServiceAccount RBAC is required** and defines the permissions available to all users
- Users effectively share the same Kubernetes permissions

This mode is suitable for:
- Development and testing environments
- Deployments where all users should have the same permissions
- Scenarios where OAuth/OIDC is not available

### 2. OAuth Downstream Mode (Recommended for Production)

When `enableDownstreamOAuth: true` is configured:

- Users authenticate to mcp-kubernetes via OAuth (Dex, Google, etc.)
- **User's OAuth token is used for ALL Kubernetes API calls**
- User's own RBAC permissions apply on both Management Cluster and Workload Clusters
- **ServiceAccount RBAC is NOT used for Kubernetes API operations**
- The ServiceAccount is only used for pod lifecycle (mounting tokens, network access)

This mode provides:
- True RBAC isolation between users
- Audit logs show actual user identities
- Compliance with principle of least privilege

```
┌─────────────────────────────────────────────────────────────────────┐
│                      Authentication Mode Comparison                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Service Account Mode          OAuth Downstream Mode                │
│  ─────────────────────         ────────────────────────             │
│                                                                     │
│  User → mcp-kubernetes         User → mcp-kubernetes                │
│           │                              │                          │
│           ▼                              ▼                          │
│    ServiceAccount Token         User's OAuth Token                  │
│           │                              │                          │
│           ▼                              ▼                          │
│    K8s API (SA RBAC)            K8s API (User's RBAC)               │
│                                                                     │
│  All users share SA perms      Each user has own permissions        │
└─────────────────────────────────────────────────────────────────────┘
```

## Helm Chart RBAC Configuration

The Helm chart includes RBAC resources that are used **only in Service Account Mode**. When OAuth Downstream Mode is enabled, these permissions are not utilized for API operations.

### Base ClusterRole (Service Account Mode Only)

The base ClusterRole grants broad permissions for Kubernetes operations:

```yaml
# Only used when enableDownstreamOAuth is FALSE
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-kubernetes
rules:
  - apiGroups: [""]
    resources: ["pods", "services", "endpoints", "nodes", ...]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  # ... additional resource permissions
```

This RBAC is necessary for non-OAuth deployments where all users share the ServiceAccount's permissions.

### CAPI Mode RBAC (Service Account Mode Only)

When CAPI mode is enabled without OAuth downstream, additional RBAC is created for:
- CAPI cluster discovery
- Kubeconfig secret access

In OAuth Downstream Mode, users need these permissions in their own RBAC configuration.

## CAPI Federation Mode with OAuth Downstream

When operating in CAPI Mode with OAuth Downstream enabled (`enableDownstreamOAuth: true`), the security model is:

### User-Centric RBAC

All Kubernetes API operations use the user's OAuth token:

1. **Management Cluster Operations**: User's OAuth token authenticates directly
2. **Workload Cluster Operations**: User identity is impersonated via headers

```
┌─────────────────────────────────────────────────────────────────────┐
│                         MCP Client (AI Agent)                       │
│                    (OAuth Token: user@example.com)                  │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     mcp-kubernetes Server                           │
│                                                                     │
│  Management Cluster: Uses user's OAuth token directly               │
│  Workload Clusters:  Impersonates user via headers                  │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    ▼                           ▼
┌────────────────────────────┐   ┌────────────────────────────┐
│   Management Cluster API   │   │    Workload Cluster API    │
│                            │   │                            │
│  User's OAuth Token        │   │  Impersonate-User: user@   │
│  → User's RBAC applies     │   │  → User's RBAC applies     │
└────────────────────────────┘   └────────────────────────────┘
```

### Required User Permissions

When OAuth Downstream is enabled, **users** (not the ServiceAccount) need RBAC permissions to:

1. **Discover CAPI clusters** (on Management Cluster):
   ```yaml
   apiGroups: ["cluster.x-k8s.io"]
   resources: ["clusters", "clusters/status", ...]
   verbs: ["get", "list"]
   ```

2. **Read kubeconfig secrets** (on Management Cluster):
   ```yaml
   apiGroups: [""]
   resources: ["secrets"]
   verbs: ["get"]
   # In namespaces where their clusters are defined
   ```

3. **Perform operations on workload clusters**:
   - Users need appropriate RBAC on each workload cluster
   - Impersonation headers carry user identity

## Service Account Requirements by Mode

| Deployment Mode | ServiceAccount RBAC Required? | Notes |
|-----------------|-------------------------------|-------|
| Single-cluster, no OAuth | Yes | SA RBAC defines all user permissions |
| Single-cluster, OAuth downstream | No | User's OIDC token used directly |
| CAPI mode, no OAuth | Yes | SA needs CAPI + secret access |
| CAPI mode, OAuth downstream | No | User's token for MC, impersonation for WC |

## Security Best Practices

### For OAuth Downstream Mode (Recommended)

1. **Configure OIDC on your Kubernetes clusters**: Ensure the Management Cluster accepts your OAuth provider's tokens

2. **Grant users appropriate RBAC**: Users need permissions for the operations they should perform

3. **Use group-based RBAC**: Map OAuth groups to Kubernetes RBAC for easier management:
   ```yaml
   subjects:
     - kind: Group
       name: platform-engineers  # OAuth group
       apiGroup: rbac.authorization.k8s.io
   ```

4. **Match cache TTL to token lifetime**:
   ```yaml
   capiMode:
     cache:
       ttl: "10m"  # Should be <= OAuth token lifetime
   ```

### For Service Account Mode

1. **Use namespace-scoped RBAC** for secrets:
   ```yaml
   capiMode:
     rbac:
       allowedNamespaces:
         - org-acme
         - org-beta
       clusterWideSecrets: false
   ```

2. **Review SA permissions regularly**: All users inherit SA permissions

3. **Consider network policies**: Restrict which endpoints the MCP server can reach

## Kubernetes Audit Policy

For either mode, configure audit logging to track who performed what actions.

### OAuth Downstream Mode Audit

Audit logs will show the actual user identity from the OAuth token:

```json
{
  "user": {
    "username": "jane@giantswarm.io",
    "groups": ["platform-team", "system:authenticated"]
  },
  "objectRef": {
    "resource": "pods",
    "namespace": "production"
  }
}
```

### Service Account Mode Audit

When using impersonation on workload clusters:

```json
{
  "user": {
    "username": "system:serviceaccount:mcp-system:mcp-kubernetes"
  },
  "impersonatedUser": {
    "username": "jane@giantswarm.io",
    "extra": {
      "agent": ["mcp-kubernetes"]
    }
  }
}
```

## ServiceAccount Token Security

The Helm chart configures secure ServiceAccount token handling regardless of mode:

- Default ServiceAccount token automounting is **disabled**
- Tokens are **projected** with a 1-hour expiration (automatically rotated)
- CA certificate and namespace are mounted separately

```yaml
spec:
  automountServiceAccountToken: false
  volumes:
    - name: sa-token
      projected:
        sources:
          - serviceAccountToken:
              expirationSeconds: 3600  # 1 hour, auto-rotated
```

This provides defense in depth even in OAuth Downstream Mode.

## Troubleshooting

### OAuth Downstream Mode Issues

| Symptom | Possible Cause | Solution |
|---------|----------------|----------|
| "Unauthorized" on MC operations | User's OAuth token not accepted | Verify OIDC config on MC |
| "forbidden" on CAPI resources | User lacks RBAC for clusters | Grant user CAPI read permissions |
| "forbidden" on secrets | User lacks secret read access | Grant user secret read in namespace |

### Service Account Mode Issues

| Symptom | Possible Cause | Solution |
|---------|----------------|----------|
| "clusters.cluster.x-k8s.io is forbidden" | CAPI ClusterRole not bound | Set `capiMode.rbac.create: true` |
| "secrets is forbidden" | Namespace not in allowedNamespaces | Add namespace to the list |

### Check Effective Permissions

```bash
# For Service Account Mode
kubectl auth can-i list pods \
  --as=system:serviceaccount:mcp-system:mcp-kubernetes

# For OAuth Downstream Mode - test as your user
kubectl auth can-i list clusters.cluster.x-k8s.io
```

## References

- [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Kubernetes OIDC Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#openid-connect-tokens)
- [Kubernetes User Impersonation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation)
- [Kubernetes Audit Logging](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
