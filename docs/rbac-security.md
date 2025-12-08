# RBAC Security for CAPI Mode

This document describes the RBAC (Role-Based Access Control) configuration required for running `mcp-kubernetes` in CAPI Mode as a multi-cluster federation gateway.

## Overview

When operating in CAPI Mode, the MCP server needs specific permissions on the Management Cluster to:

1. **Discover workload clusters** via Cluster API (CAPI) resources
2. **Retrieve kubeconfig secrets** to establish connections to workload clusters
3. **Perform authorization checks** using SubjectAccessReviews
4. **Validate tokens** via TokenReviews (when using OAuth)

## Security Model

### Hub-and-Spoke Identity Propagation

The MCP server implements a **Hub-and-Spoke** identity model that preserves user identity across cluster boundaries:

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
│  1. Validate OAuth token                                            │
│  2. Extract user identity (email, groups)                           │
│  3. Build impersonation headers                                     │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Workload Cluster API Server                      │
│                                                                     │
│  Request Headers:                                                   │
│  - Impersonate-User: user@example.com                               │
│  - Impersonate-Group: team-alpha                                    │
│  - Impersonate-Extra-Agent: mcp-kubernetes                          │
│                                                                     │
│  → RBAC evaluated as if user@example.com made the request           │
└─────────────────────────────────────────────────────────────────────┘
```

**Key Security Properties:**

- The MCP server's ServiceAccount credentials are **only** used to establish TLS connections
- All authorization decisions are delegated to the target cluster's RBAC
- Users can only perform actions they would be able to perform if directly connected
- Audit logs show the actual user identity, not the ServiceAccount

### Principle of Least Privilege

The RBAC configuration follows the principle of least privilege:

| Permission | Scope | Justification |
|------------|-------|---------------|
| CAPI Resources | Cluster-wide (read-only) | Required to discover all workload clusters |
| Kubeconfig Secrets | Namespace-scoped | Minimizes blast radius; only specific org namespaces |
| TokenReviews | Cluster-wide | Required for OAuth token validation |
| SubjectAccessReviews | Cluster-wide | Required for pre-flight authorization checks |

## RBAC Resources

### ClusterRole: CAPI Discovery

This ClusterRole grants read-only access to CAPI resources for cluster discovery:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-kubernetes-capi
rules:
  # CAPI cluster discovery (read-only)
  - apiGroups: ["cluster.x-k8s.io"]
    resources:
      - clusters
      - clusters/status
      - machinepools
      - machinepools/status
      - machinedeployments
      - machinedeployments/status
      - machines
      - machines/status
    verbs: ["get", "list", "watch"]

  # Infrastructure provider resources (read-only)
  # Default uses wildcard to support any provider
  - apiGroups: ["infrastructure.cluster.x-k8s.io"]
    resources: ["*"]
    verbs: ["get", "list"]

  # TokenReviews for validating OAuth tokens
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["create"]

  # SubjectAccessReviews for pre-flight permission checks
  - apiGroups: ["authorization.k8s.io"]
    resources: ["subjectaccessreviews", "selfsubjectaccessreviews"]
    verbs: ["create"]
```

#### Provider-Specific Resource Restrictions

The default CAPI ClusterRole uses a wildcard (`*`) for infrastructure provider resources to support any CAPI provider. For single-provider deployments, you can restrict permissions to specific resources.

**AWS (CAPA):**
```yaml
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources:
    - awsclusters
    - awsclusters/status
    - awsmachines
    - awsmachines/status
    - awsmachinetemplates
  verbs: ["get", "list"]
```

**Azure (CAPZ):**
```yaml
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources:
    - azureclusters
    - azureclusters/status
    - azuremachines
    - azuremachines/status
    - azuremachinetemplates
  verbs: ["get", "list"]
```

**GCP (CAPG):**
```yaml
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources:
    - gcpclusters
    - gcpclusters/status
    - gcpmachines
    - gcpmachines/status
    - gcpmachinetemplates
  verbs: ["get", "list"]
```

**vSphere (CAPV):**
```yaml
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources:
    - vsphereclusters
    - vsphereclusters/status
    - vspheremachines
    - vspheremachines/status
    - vspheremachinetemplates
  verbs: ["get", "list"]
```

To apply provider-specific restrictions, create a custom ClusterRole that overrides the default CAPI role, or modify the Helm chart values to disable RBAC creation and manage it externally.

### Namespace-Scoped Secret Access (Recommended)

For each organization namespace, create a Role and RoleBinding:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mcp-kubernetes-secrets
  namespace: org-acme  # Repeat for each org namespace
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mcp-kubernetes-secrets
  namespace: org-acme
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: mcp-kubernetes-secrets
subjects:
  - kind: ServiceAccount
    name: mcp-kubernetes
    namespace: mcp-system  # Where MCP server is deployed
```

### Cluster-Wide Secret Access (Not Recommended)

Only use this if namespace-scoped access is impractical:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-kubernetes-secrets
  annotations:
    security.kubernetes.io/warning: >-
      This role grants cluster-wide secret access.
      Use namespace-scoped roles instead.
rules:
  # HIGH PRIVILEGE - grants access to ALL secrets cluster-wide
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
```

**WARNING:** This grants read access to ALL secrets in the cluster, including:
- TLS certificates and private keys
- Database credentials
- API tokens for external services
- Other sensitive configuration

## Blast Radius Analysis

| RBAC Configuration | Blast Radius | Risk Level | Use Case |
|--------------------|--------------|------------|----------|
| Namespace-scoped Roles | Access to secrets in specific org namespaces only | Low | Production (Recommended) |
| ClusterRole + namespace RoleBindings | ClusterRole definition, per-namespace binding | Medium | Large deployments with many orgs |
| ClusterRole + ClusterRoleBinding | Access to ALL secrets in the cluster | High | Development/Testing only |

## Helm Chart Configuration

### Recommended: Namespace-Scoped Access

```yaml
# values.yaml
capiMode:
  enabled: true
  rbac:
    create: true
    allowedNamespaces:
      - org-acme
      - org-beta
      - org-gamma
    clusterWideSecrets: false  # IMPORTANT: Keep this false
```

### Adding New Organization Namespaces

When a new organization is onboarded:

1. Add the namespace to `allowedNamespaces` in your values file
2. Upgrade the Helm release:

```bash
helm upgrade mcp-kubernetes ./helm/mcp-kubernetes \
  --reuse-values \
  --set capiMode.rbac.allowedNamespaces[3]=org-delta
```

Alternatively, use a separate values file for organization configuration:

```yaml
# values-orgs.yaml
capiMode:
  rbac:
    allowedNamespaces:
      - org-acme
      - org-beta
      - org-gamma
      - org-delta  # New org added
```

```bash
helm upgrade mcp-kubernetes ./helm/mcp-kubernetes \
  -f values.yaml \
  -f values-orgs.yaml
```

## Kubernetes Audit Policy

To maintain complete audit trails when using the MCP server, configure your Kubernetes audit policy to log impersonation events.

### Recommended Audit Policy

Add these rules to your Management Cluster's audit policy:

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Log all secret access by the MCP ServiceAccount at Metadata level
  # This captures who accessed which secret, without logging secret contents
  - level: Metadata
    users:
      - "system:serviceaccount:mcp-system:mcp-kubernetes"
    resources:
      - group: ""
        resources: ["secrets"]
    verbs: ["get"]

  # Log all impersonation events at RequestResponse level
  # Critical for tracking which user the MCP server was acting on behalf of
  - level: RequestResponse
    users:
      - "system:serviceaccount:mcp-system:mcp-kubernetes"
    verbs: ["impersonate"]

  # Log all CAPI resource access at Metadata level
  - level: Metadata
    users:
      - "system:serviceaccount:mcp-system:mcp-kubernetes"
    resources:
      - group: "cluster.x-k8s.io"
        resources: ["*"]
    verbs: ["get", "list", "watch"]

  # Log SubjectAccessReview and TokenReview creation
  - level: Metadata
    users:
      - "system:serviceaccount:mcp-system:mcp-kubernetes"
    resources:
      - group: "authorization.k8s.io"
        resources: ["subjectaccessreviews", "selfsubjectaccessreviews"]
      - group: "authentication.k8s.io"
        resources: ["tokenreviews"]
    verbs: ["create"]
```

### Workload Cluster Audit Policy

On workload clusters, the audit log will show impersonated requests:

```yaml
# Recommended audit policy for workload clusters
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Log all impersonated requests with user context
  - level: RequestResponse
    # Match all users being impersonated
    userGroups:
      - "system:authenticated"
    # When impersonation is used, log the full request
    omitStages:
      - "RequestReceived"
```

### Sample Audit Log Entry

When the MCP server performs an action on behalf of a user:

```json
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "RequestResponse",
  "auditID": "abc123-def456",
  "stage": "ResponseComplete",
  "requestURI": "/api/v1/namespaces/production/pods/nginx-abc123",
  "verb": "delete",
  "user": {
    "username": "system:serviceaccount:mcp-system:mcp-kubernetes",
    "groups": ["system:serviceaccounts", "system:authenticated"]
  },
  "impersonatedUser": {
    "username": "jane@giantswarm.io",
    "groups": ["platform-team", "system:authenticated"],
    "extra": {
      "agent": ["mcp-kubernetes"],
      "trace-id": ["abc123def456"]
    }
  },
  "sourceIPs": ["10.0.0.50"],  // MCP server pod IP
  "objectRef": {
    "resource": "pods",
    "namespace": "production",
    "name": "nginx-abc123"
  },
  "responseStatus": {
    "code": 200
  }
}
```

**Key Fields for Investigation:**
- `impersonatedUser.username`: The actual user who initiated the action
- `impersonatedUser.extra.agent`: Identifies MCP server as the proxy
- `impersonatedUser.extra.trace-id`: Correlates with MCP server logs
- `sourceIPs`: The MCP server pod IP (not the user's IP)

## Security Best Practices

### 1. Use Namespace-Scoped RBAC

Always prefer namespace-scoped Roles over ClusterRoles for secret access:

```yaml
# Good - Namespace-scoped
capiMode:
  rbac:
    allowedNamespaces:
      - org-acme
      - org-beta
    clusterWideSecrets: false

# Bad - Cluster-wide
capiMode:
  rbac:
    allowedNamespaces: []
    clusterWideSecrets: true
```

### 2. Match Cache TTL to Token Lifetime

Configure the client cache TTL to be less than or equal to your OAuth token lifetime:

```yaml
capiMode:
  cache:
    # If OAuth tokens expire in 15 minutes, set TTL <= 15m
    ttl: "10m"
```

This ensures cached clients are invalidated before the user's authorization expires.

### 3. Enable Secret Masking

Always mask secret data in tool responses:

```yaml
capiMode:
  output:
    maskSecrets: true
```

### 4. Implement Network Policies

Restrict MCP server network access to only required endpoints:

```yaml
# CiliumNetworkPolicy example
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: mcp-kubernetes
spec:
  endpointSelector:
    matchLabels:
      app.kubernetes.io/name: mcp-kubernetes
  egress:
    # Allow kube-apiserver access
    - toEntities:
        - kube-apiserver
    # Allow workload cluster API servers (via ingress/load balancer)
    - toFQDNs:
        - matchPattern: "*.g8s.example.com"
      toPorts:
        - ports:
            - port: "443"
              protocol: TCP
```

### 5. Regular RBAC Audits

Periodically review RBAC configuration:

```bash
# List all RoleBindings for the MCP ServiceAccount
kubectl get rolebindings -A -o json | jq '.items[] | select(.subjects[]?.name == "mcp-kubernetes")'

# List ClusterRoleBindings
kubectl get clusterrolebindings -o json | jq '.items[] | select(.subjects[]?.name == "mcp-kubernetes")'

# Check effective permissions
kubectl auth can-i --list --as=system:serviceaccount:mcp-system:mcp-kubernetes
```

### 6. ServiceAccount Token Security

The Helm chart configures secure ServiceAccount token handling:

- Default ServiceAccount token automounting is **disabled** (`automountServiceAccountToken: false`)
- Tokens are **projected** with a 1-hour expiration (automatically rotated by kubelet)
- CA certificate and namespace are mounted separately for defense in depth
- Tokens are mounted read-only at the standard path

```yaml
# From deployment.yaml
spec:
  automountServiceAccountToken: false  # Disable default mounting
  volumes:
    - name: sa-token
      projected:
        sources:
          - serviceAccountToken:
              expirationSeconds: 3600  # 1 hour, auto-rotated
              path: token
          - configMap:
              name: kube-root-ca.crt
              items:
                - key: ca.crt
                  path: ca.crt
          - downwardAPI:
              items:
                - path: namespace
                  fieldRef:
                    fieldPath: metadata.namespace
```

**Security Benefits:**

- **Automatic rotation**: kubelet rotates the token before expiration
- **Reduced blast radius**: Short-lived tokens limit exposure window if compromised
- **Explicit mounting**: Disabling automount ensures tokens are only available where explicitly configured

## Troubleshooting

### Check ServiceAccount Permissions

```bash
# Test if the SA can list clusters
kubectl auth can-i list clusters.cluster.x-k8s.io \
  --as=system:serviceaccount:mcp-system:mcp-kubernetes

# Test if the SA can get secrets in a specific namespace
kubectl auth can-i get secrets \
  --as=system:serviceaccount:mcp-system:mcp-kubernetes \
  -n org-acme

# Test if the SA can get secrets cluster-wide (should be "no" in production)
kubectl auth can-i get secrets \
  --as=system:serviceaccount:mcp-system:mcp-kubernetes \
  --all-namespaces
```

### Common RBAC Issues

| Symptom | Possible Cause | Solution |
|---------|----------------|----------|
| "clusters.cluster.x-k8s.io is forbidden" | CAPI ClusterRole not bound | Ensure `capiMode.rbac.create: true` |
| "secrets is forbidden" for specific namespace | Namespace not in `allowedNamespaces` | Add namespace to the list |
| "cannot impersonate" on workload cluster | WC RBAC missing impersonation permission | Configure WC RBAC for MCP SA |
| TokenReview failures | TokenReview ClusterRole not bound | Check ClusterRoleBinding exists |

### View RBAC Resources Created by Helm

```bash
# List all RBAC resources created by the Helm release
helm get manifest mcp-kubernetes | grep -A 50 "kind: ClusterRole\|kind: Role\|kind: ClusterRoleBinding\|kind: RoleBinding"
```

## References

- [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Kubernetes User Impersonation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation)
- [Kubernetes Audit Logging](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- [Cluster API Security Best Practices](https://cluster-api.sigs.k8s.io/security/)

