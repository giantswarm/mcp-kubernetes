# RBAC Security for mcp-kubernetes

This document describes the RBAC (Role-Based Access Control) configuration and security model for `mcp-kubernetes`, including both single-cluster and multi-cluster (CAPI) federation modes.

## TL;DR - Which RBAC Do I Need?

| Deployment Mode | Who Needs RBAC | ServiceAccount RBAC? | Profile Behavior |
|-----------------|----------------|---------------------|------------------|
| **Service Account Mode** (`enableDownstreamOAuth: false`) | ServiceAccount | **Yes** - SA permissions apply to all users | Uses configured profile |
| **OAuth Downstream Mode** (`enableDownstreamOAuth: true`) | Each User + ServiceAccount for secrets | **Minimal enforced** - SA has no API access | **Automatically set to `minimal`** |

**Key Insight**: In OAuth Downstream Mode with CAPI, the security model uses split credentials:
- **ServiceAccount handles CAPI cluster discovery and kubeconfig secret access** - users do NOT need cluster-scoped CAPI permissions or secret access
- **Users authenticate via OAuth** for workload cluster operations (with impersonation)
- This prevents users from extracting admin credentials and bypassing impersonation

### Quick Reference

**Service Account Mode** (simpler, less secure):
- Helm chart RBAC = what all users can do
- All users share the same permissions
- Good for: development, testing, trusted environments
- Use: `rbac.profile: "standard"` or `rbac.profile: "readonly"`

**OAuth Downstream Mode** (recommended for production):
- **Base RBAC profile is automatically enforced to `minimal`** (no Kubernetes API permissions)
- CAPI RBAC is created when both `rbac.create: true` and `capiMode.rbac.create: true`
- Each user has individual permissions via their OAuth identity
- ServiceAccount only reads secrets/configmaps for workload cluster access
- Good for: production, multi-tenant, compliance requirements

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

> **Note:** When `enableDownstreamOAuth: true`, the profile is **automatically enforced to `minimal`** regardless of the configured value. This is a security feature to ensure the ServiceAccount cannot bypass user RBAC.

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

> **Security Warning:** The `standard` profile grants **cluster-wide secret access**. This includes read/write permissions to all secrets in all namespaces. If you don't need write operations or secret access, consider using the `readonly` profile instead.

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

> **DANGER:** The `admin` profile grants **extremely dangerous permissions** that can lead to:
> - **Privilege escalation** - RBAC management allows creating cluster-admin bindings
> - **Container escape** - Pod exec allows arbitrary command execution in containers
> - **API interception** - Webhook configuration can intercept/modify all API requests
>
> **You must explicitly confirm you understand these risks** by setting `adminConfirmation: true`.

**Use when:**
- Administrative tooling with strict access controls
- Fully trusted environments only
- Emergency break-glass scenarios

**Configuration:**
```yaml
rbac:
  create: true
  profile: "admin"
  adminConfirmation: true  # REQUIRED - confirms you understand the risks
```

**Security controls:** The Helm chart will **fail to render** unless you explicitly set `adminConfirmation: true`. A ClusterRole annotation also warns about the elevated permissions.

### Custom RBAC Rules

For fine-grained control, you can define custom rules that override the profile:

> **Security Warning:** Custom rules **bypass all profile safety controls**. When `custom.enabled: true`:
> - No validation is performed on your custom rules
> - The profile setting is completely ignored
> - You can accidentally grant excessive permissions (including admin-level access)
>
> **Review custom rules carefully before deploying.**

```yaml
rbac:
  create: true
  profile: "readonly"  # IGNORED when custom.enabled is true
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

Only use custom rules when the predefined profiles don't meet your specific requirements.

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

### RBAC Control Hierarchy

The chart provides fine-grained control over RBAC resource creation:

| `rbac.create` | `capiMode.rbac.create` | Result |
|---------------|------------------------|--------|
| `false` | (ignored) | No RBAC resources at all |
| `true` | `false` | Only base profile RBAC (ClusterRole/ClusterRoleBinding) |
| `true` | `true` | Base RBAC + CAPI mode RBAC (including namespace-scoped Roles) |

- **`rbac.create`**: Master switch for ALL RBAC resources
- **`capiMode.rbac.create`**: Controls CAPI-specific RBAC (subordinate to master switch)

Use `rbac.create: false` when you manage all RBAC externally. Use `capiMode.rbac.create: false` when you only want base profile RBAC without CAPI-specific resources.

### Operator-Managed Namespace RBAC

For dynamic environments where organization namespaces are created/deleted frequently, you can configure the chart to create only the base CAPI ClusterRole while an external operator manages namespace-scoped Roles:

```yaml
capiMode:
  enabled: true
  rbac:
    create: true
    allowedNamespaces: []      # No static namespace Roles
    clusterWideSecrets: false  # No cluster-wide secret access
```

This configuration:
- Creates the CAPI ClusterRole for cluster discovery (clusters, machines, etc.)
- Does NOT create namespace-scoped Roles/RoleBindings
- Allows an operator to dynamically create Roles in `org-*` namespaces as they are created

The operator would create Roles matching this pattern per namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mcp-kubernetes-secrets
  namespace: org-<name>
rules:
  - apiGroups: [""]
    resources: ["secrets"]  # or "configmaps" for sso-passthrough mode
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mcp-kubernetes-secrets
  namespace: org-<name>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: mcp-kubernetes-secrets
subjects:
  - kind: ServiceAccount
    name: mcp-kubernetes
    namespace: <mcp-kubernetes-namespace>
```

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

### CAPI Mode RBAC

When `capiMode.enabled: true` and `capiMode.rbac.create: true`, CAPI-specific RBAC is created. This includes:

1. **CAPI ClusterRole** (`mcp-kubernetes-capi`): Permissions to list CAPI clusters, machines, etc.
2. **Namespace-scoped Roles**: Access to kubeconfig secrets (impersonation mode) or CA configmaps (SSO passthrough mode)

**Why CAPI RBAC is always needed:**
- In **Service Account Mode**: The ServiceAccount uses these permissions directly
- In **OAuth Downstream Mode**: The ServiceAccount still needs to read kubeconfig secrets/CA configmaps (split credential model - see below)

The type of namespace-scoped access depends on `capiMode.workloadClusterAuth.mode`:
- `impersonation` (default): Creates Roles for **Secret** access (kubeconfig credentials)
- `sso-passthrough`: Creates Roles for **ConfigMap** access (CA certificates only - no secrets needed)

## CAPI Federation Mode with OAuth Downstream

When operating in CAPI Mode with OAuth Downstream enabled (`enableDownstreamOAuth: true`), the security model provides true per-user isolation with enhanced credential protection.

### Security Model: Split Credentials

The architecture uses split credentials to prevent credential leakage:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          mcp-kubernetes (CAPI Mode)                          │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Split Client Provider                           │   │
│  │                                                                      │   │
│  │  ┌─────────────────────────┐   ┌──────────────────────────────────┐ │   │
│  │  │   User OAuth Client     │   │   ServiceAccount Client          │ │   │
│  │  │                         │   │                                  │ │   │
│  │  │  • Perform WC operations│   │  • Discover CAPI clusters       │ │   │
│  │  │    (with impersonation) │   │  • Read kubeconfig secrets      │ │   │
│  │  │                         │   │                                  │ │   │
│  │  └─────────────────────────┘   └──────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### How It Works: Management Cluster vs Workload Clusters

**Management Cluster (MC)**: Split credential model
- ServiceAccount discovers CAPI clusters (privileged access, configurable)
- ServiceAccount reads kubeconfig secrets (privileged access)
- **Users do NOT need cluster-scoped CAPI permissions or secret read permissions**
- This ensures users cannot extract admin kubeconfigs via kubectl

**Workload Clusters (WC)**: User identity is propagated via impersonation
- The kubeconfig secret contains admin credentials (created by CAPI)
- mcp-kubernetes uses these credentials BUT adds impersonation headers
- The WC API server evaluates RBAC as if the user made the request directly
- The `Impersonate-Extra-agent: mcp-kubernetes` header provides audit trail

**Security Guarantee**: Users can only access workload clusters where their impersonated identity has RBAC permissions. They cannot bypass impersonation by reading kubeconfig secrets directly.

This means: **Users can only do what their own RBAC allows on workload clusters.**

### User-Centric RBAC

Workload cluster operations use the user's OAuth identity via impersonation:

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
│  CAPI Discovery:   ServiceAccount credentials (privileged)          │
│  Secret Access:    ServiceAccount credentials (privileged)          │
│  Workload Clusters: Impersonates user via headers                   │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    ▼                           ▼
┌────────────────────────────┐   ┌────────────────────────────┐
│   Management Cluster API   │   │    Workload Cluster API    │
│                            │   │                            │
│  ServiceAccount Token      │   │  Impersonate-User: user@   │
│  → SA RBAC for CAPI/secrets│   │  → User's RBAC applies     │
└────────────────────────────┘   └────────────────────────────┘
```

### Required User Permissions

When OAuth Downstream is enabled in CAPI mode, the permission requirements are split:

**Users need RBAC permissions for:**

1. **Perform operations on workload clusters**:
   - Users need appropriate RBAC on each workload cluster
   - Impersonation headers carry user identity

**Users do NOT need RBAC for:**

1. **CAPI cluster discovery** - ServiceAccount handles this (when `privilegedCAPIDiscovery: true`, the default)
2. **Kubeconfig secret access** - ServiceAccount handles this

> **Note**: When `privilegedCAPIDiscovery: true` (default), all authenticated users can see
> all CAPI clusters (names, namespaces, providers, status). This is intentional -- granting
> every user cluster-scoped CAPI permissions is impractical in multi-tenant environments.
> Access to workload cluster **operations** is still governed by the user's own RBAC via impersonation.
>
> To restrict cluster visibility to what each user's RBAC allows, set
> `capiMode.privilegedAccess.privilegedCAPIDiscovery: false`. Users will then need
> their own ClusterRoleBinding for `clusters.cluster.x-k8s.io` to discover clusters.

**ServiceAccount needs RBAC permissions for:**

1. **Discover CAPI clusters** (on Management Cluster, when `privilegedCAPIDiscovery: true`):
   ```yaml
   apiGroups: ["cluster.x-k8s.io"]
   resources: ["clusters", "clusters/status"]
   verbs: ["get", "list"]
   ```

2. **Read kubeconfig secrets** (on Management Cluster):
   ```yaml
   apiGroups: [""]
   resources: ["secrets"]
   verbs: ["get"]
   # In namespaces where their clusters are defined
   ```

   This is configured via `capiMode.rbac.allowedNamespaces` in the Helm chart.

**Security Note**: Users do NOT need secret read permissions or CAPI cluster-scoped permissions. This is intentional - it prevents users from extracting admin kubeconfig credentials via kubectl and bypassing impersonation enforcement.

## Service Account Requirements by Mode

| Deployment Mode | ServiceAccount RBAC Required? | Notes |
|-----------------|-------------------------------|-------|
| Single-cluster, no OAuth | Yes | SA RBAC defines all user permissions |
| Single-cluster, OAuth downstream | No | User's OIDC token used directly |
| CAPI mode, no OAuth | Yes | SA needs CAPI + secret access |
| CAPI mode, OAuth downstream | **Yes for CAPI discovery + secrets** | SA discovers clusters and reads kubeconfigs; users need neither |

**Key Security Change**: In CAPI mode with OAuth downstream, the ServiceAccount handles both CAPI cluster discovery and kubeconfig secret access. Users do not need cluster-scoped CAPI permissions or secret access. This is a security feature that simplifies user RBAC while preventing credential leakage.

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
| "forbidden" on secrets | ServiceAccount lacks secret access | Add namespace to `capiMode.rbac.allowedNamespaces` |
| Cannot access workload cluster | User lacks RBAC on workload cluster | Grant user RBAC on the specific WC |

### Service Account Mode Issues

| Symptom | Possible Cause | Solution |
|---------|----------------|----------|
| "clusters.cluster.x-k8s.io is forbidden" | CAPI ClusterRole not bound | Ensure `rbac.create: true` and `capiMode.rbac.create: true` |
| "secrets is forbidden" | Namespace not in allowedNamespaces | Add namespace to the list |
| No RBAC resources created | Master switch disabled | Set `rbac.create: true` |

### Check Effective Permissions

```bash
# For Service Account Mode
kubectl auth can-i list pods \
  --as=system:serviceaccount:mcp-system:mcp-kubernetes

# For OAuth Downstream Mode - test as your user
kubectl auth can-i list clusters.cluster.x-k8s.io
```

## Security: Why ServiceAccount Reads Secrets in OAuth Mode

This section explains the security rationale for the split-credential model.

### The Problem with User-Based Secret Access

If users had RBAC permission to read kubeconfig secrets, they could:

1. Use `kubectl get secret my-cluster-kubeconfig -o yaml` to read the secret directly
2. Extract the kubeconfig data (containing admin credentials)
3. Use these admin credentials directly, **bypassing impersonation entirely**

```
Attack Vector:
User (via kubectl) → MC API → Secret (admin kubeconfig) → WC API (as admin!)
                                    ↑
                         No impersonation here!
```

### The Solution: ServiceAccount for Privileged Access

By having the ServiceAccount handle CAPI discovery and secret access:

1. Users can discover clusters without needing cluster-scoped CAPI permissions
2. Users cannot read kubeconfig secrets via kubectl
3. mcp-kubernetes discovers clusters and reads secrets using ServiceAccount credentials
4. mcp-kubernetes always applies impersonation when accessing workload clusters
5. Users can only perform actions allowed by their RBAC on each workload cluster

```
Secure Flow:
User → mcp-kubernetes → SA discovers clusters → SA reads secret → WC API (with impersonation!)
                                                                        ↑
                                                            User's identity enforced
```

### Security Properties

| Property | Guarantee |
|----------|-----------|
| Cluster discovery | ServiceAccount discovers all clusters (configurable via `privilegedCAPIDiscovery`) |
| Cluster visibility | All authenticated users see all clusters (names, namespaces, status) |
| Secret access | ServiceAccount only (users cannot extract admin credentials) |
| Workload cluster access | Impersonation always enforced (user's identity) |
| Audit trail | Logs show user identity on all operations |
| Credential leakage | Prevented (users cannot access raw kubeconfigs) |

## Advanced Security Configuration

### Privileged CAPI Discovery

By default, CAPI cluster discovery uses ServiceAccount credentials so that users do not need cluster-scoped CAPI permissions. This means all authenticated users can see all CAPI clusters.

To restrict cluster visibility to what each user's RBAC allows, disable privileged CAPI discovery:

```yaml
capiMode:
  privilegedAccess:
    privilegedCAPIDiscovery: false  # Users need their own CAPI RBAC
```

When disabled, users must have a `ClusterRoleBinding` granting `get` and `list` on `clusters.cluster.x-k8s.io`. This provides tenant-level isolation of cluster discovery at the cost of additional RBAC management.

| Setting | Cluster Visibility | User RBAC Required | Use Case |
|---------|-------------------|-------------------|----------|
| `true` (default) | All clusters visible to all users | None for discovery | Most deployments |
| `false` | Only clusters user can list | `clusters.cluster.x-k8s.io` get/list | Strict multi-tenant isolation |

### Strict Mode

When strict mode is enabled, mcp-kubernetes will fail instead of falling back to user credentials when ServiceAccount access is unavailable. This enforces the split-credential security model.

```yaml
capiMode:
  privilegedAccess:
    strict: true  # Fail instead of fallback (recommended for production)
```

**Benefits of Strict Mode:**
- Enforces the split-credential security model
- Prevents accidental exposure if ServiceAccount RBAC is misconfigured
- Ensures consistent security behavior across all deployments

**When to Enable:**
- Production environments
- Environments with strict compliance requirements
- Any deployment where you want to ensure users cannot access kubeconfig secrets

### Rate Limiting

Privileged secret access is rate-limited per user to prevent abuse:

```yaml
capiMode:
  privilegedAccess:
    rateLimit:
      perSecond: 10.0  # Requests per second per user
      burst: 20        # Burst size per user
```

**Default Values:**
- Rate: 10 requests/second per user
- Burst: 20 requests per user

Rate limiting protects against:
- Denial-of-service attacks via excessive kubeconfig retrieval
- Automated scripts that might overwhelm the system
- Unintentional abuse from misconfigured clients

### Metrics

Privileged access is instrumented with Prometheus metrics:

```
# Metric: mcp_kubernetes_privileged_access_total
# Labels: user_domain, operation, result
# Operation values: secret_access, capi_discovery
# Result values: success, error, rate_limited, fallback

# Example queries:

# Rate of all privileged access attempts
rate(mcp_kubernetes_privileged_access_total[5m])

# Rate of CAPI discovery vs secret access
rate(mcp_kubernetes_privileged_access_total{operation="capi_discovery"}[5m])
rate(mcp_kubernetes_privileged_access_total{operation="secret_access"}[5m])

# Rate-limited requests (potential abuse)
rate(mcp_kubernetes_privileged_access_total{result="rate_limited"}[5m])

# Fallback to user credentials (weaker security, should be 0 in strict mode)
rate(mcp_kubernetes_privileged_access_total{result="fallback"}[5m])
```

## References

- [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Kubernetes OIDC Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#openid-connect-tokens)
- [Kubernetes User Impersonation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation)
- [Kubernetes Audit Logging](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
