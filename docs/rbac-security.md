# RBAC Security for mcp-kubernetes

This document describes the RBAC (Role-Based Access Control) configuration and security model for `mcp-kubernetes`, including both single-cluster and multi-cluster (CAPI) federation modes.

## TL;DR - Which RBAC Do I Need?

| Deployment Mode | Who Needs RBAC | Helm Chart RBAC Used? | Recommended Profile |
|-----------------|----------------|----------------------|---------------------|
| **Service Account Mode** (`enableDownstreamOAuth: false`) | ServiceAccount | **Yes** - SA permissions apply to all users | `standard` or `readonly` |
| **OAuth Downstream Mode** (`enableDownstreamOAuth: true`) | Each User | **No** - Users need their own RBAC bindings | `minimal` |

**Key Insight**: The Helm chart creates RBAC for the ServiceAccount. This RBAC is **only used when OAuth Downstream is disabled**. When OAuth Downstream is enabled, users authenticate with their own OAuth tokens and need their own RBAC permissions.

### Quick Reference

**Service Account Mode** (simpler, less secure):
- Helm chart RBAC = what all users can do
- All users share the same permissions
- Good for: development, testing, trusted environments
- Use: `rbac.profile: "standard"` or `rbac.profile: "readonly"`

**OAuth Downstream Mode** (recommended for production):
- Helm chart RBAC = not used for API operations
- Each user has individual permissions via their OAuth identity
- Good for: production, multi-tenant, compliance requirements
- Use: `rbac.profile: "minimal"` to follow least-privilege principles

---

## RBAC Profiles

The Helm chart provides four RBAC profiles to control ServiceAccount permissions. Choose the appropriate profile based on your deployment mode and security requirements.

### Profile Overview

| Profile | Description | Use Case |
|---------|-------------|----------|
| `minimal` | No Kubernetes API permissions | OAuth downstream mode (production) |
| `readonly` | Read-only access to common resources | Monitoring, debugging, audit |
| `standard` | Read + limited write operations | Development, testing (default) |
| `admin` | Full access to all resources | Trusted environments, admin tools |

### Profile: `minimal`

**Permissions:** None (empty rules)

**Use when:**
- `enableDownstreamOAuth: true` (user tokens used for API)
- You want to minimize attack surface
- Production deployments following least-privilege

**Configuration:**
```yaml
rbac:
  create: true
  profile: "minimal"
```

**Security benefit:** Even if the ServiceAccount token is compromised, it cannot access any Kubernetes resources.

### Profile: `readonly`

**Permissions:**
- Read access to: pods, services, deployments, configmaps, namespaces, etc.
- **Secrets excluded** for security
- No create/update/delete permissions

**Use when:**
- Read-only monitoring or debugging access
- Audit and compliance reporting
- Limited-access development environments

**Configuration:**
```yaml
rbac:
  create: true
  profile: "readonly"
```

### Profile: `standard` (Default)

**Permissions:**
- Read/write access to: pods, services, deployments, configmaps, secrets, jobs, etc.
- Read-only access to: storage classes, CRDs
- **RBAC resources are read-only** (cannot escalate privileges)

**Use when:**
- Development and testing environments
- Trusted single-tenant deployments
- When OAuth downstream is disabled

**Configuration:**
```yaml
rbac:
  create: true
  profile: "standard"  # This is the default
```

### Profile: `admin`

**Permissions:**
- Full CRUD access to all common resources
- Pod exec, logs, port-forward access
- RBAC management (create/update/delete roles)
- Admission webhook management

**Use when:**
- Administrative tooling
- Fully trusted environments only
- Emergency break-glass scenarios

**Configuration:**
```yaml
rbac:
  create: true
  profile: "admin"
```

**Warning:** The `admin` profile grants extensive permissions. A ClusterRole annotation warns about this.

### Custom RBAC Rules

For fine-grained control, you can define custom rules that override the profile:

```yaml
rbac:
  create: true
  profile: "readonly"  # Ignored when custom.enabled is true
  custom:
    enabled: true
    rules:
      - apiGroups: [""]
        resources: ["pods", "services"]
        verbs: ["get", "list", "watch"]
      - apiGroups: ["apps"]
        resources: ["deployments"]
        verbs: ["get", "list"]
```

---

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

When operating in CAPI Mode with OAuth Downstream enabled (`enableDownstreamOAuth: true`), the security model provides true per-user isolation.

### How It Works: Management Cluster vs Workload Clusters

**Management Cluster (MC)**: User's OAuth token authenticates directly
- The Kubernetes API server validates the OAuth token via OIDC
- User's RBAC bindings on the MC determine what they can access
- Users need permission to list CAPI clusters and read kubeconfig secrets

**Workload Clusters (WC)**: User identity is propagated via impersonation
- The kubeconfig secret contains admin credentials (created by CAPI)
- mcp-kubernetes uses these credentials BUT adds impersonation headers
- The WC API server evaluates RBAC as if the user made the request directly
- The `Impersonate-Extra-agent: mcp-kubernetes` header provides audit trail

This means: **Users can only do what their own RBAC allows, on both MC and WCs.**

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

1. **Use the minimal RBAC profile**: When user tokens are used for API operations, the ServiceAccount doesn't need permissions:
   ```yaml
   rbac:
     profile: "minimal"
   mcpKubernetes:
     oauth:
       enableDownstreamOAuth: true
   ```

2. **Configure OIDC on your Kubernetes clusters**: Ensure the Management Cluster accepts your OAuth provider's tokens

3. **Grant users appropriate RBAC**: Users need permissions for the operations they should perform

4. **Use group-based RBAC**: Map OAuth groups to Kubernetes RBAC for easier management:
   ```yaml
   subjects:
     - kind: Group
       name: platform-engineers  # OAuth group
       apiGroup: rbac.authorization.k8s.io
   ```

5. **Match cache TTL to token lifetime**:
   ```yaml
   capiMode:
     cache:
       ttl: "10m"  # Should be <= OAuth token lifetime
   ```

### For Service Account Mode

1. **Choose the appropriate RBAC profile**: Match the profile to your security requirements:
   ```yaml
   rbac:
     # Use "readonly" for monitoring/audit scenarios
     # Use "standard" for development environments
     # Avoid "admin" unless absolutely necessary
     profile: "readonly"
   ```

2. **Use namespace-scoped RBAC** for secrets:
   ```yaml
   capiMode:
     rbac:
       allowedNamespaces:
         - org-acme
         - org-beta
       clusterWideSecrets: false
   ```

3. **Review SA permissions regularly**: All users inherit SA permissions

4. **Consider network policies**: Restrict which endpoints the MCP server can reach

5. **Never use the admin profile in production**: The `admin` profile grants RBAC management permissions which could be used for privilege escalation

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
