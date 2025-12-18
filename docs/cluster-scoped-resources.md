# Cluster-Scoped Resources

This document describes how `mcp-kubernetes` handles cluster-scoped vs namespaced resources.

## Overview

Kubernetes resources are either **namespaced** (exist within a namespace) or **cluster-scoped** (exist at the cluster level). When using the `kubernetes_list` tool, the `namespace` parameter behavior differs based on the resource type:

- **Known namespaced resources** (pods, deployments, services, etc.): Require a `namespace` parameter or `allNamespaces=true`
- **Known cluster-scoped resources** (nodes, persistentvolumes, namespaces, etc.): Do not require a `namespace` parameter
- **Unknown resources** (CRDs, custom resources): No early validation - the K8s API determines scope via discovery

## Supported Cluster-Scoped Resources

The following resource types are recognized as cluster-scoped. Both plural and singular forms are supported, along with common short aliases. Resource type matching is case-insensitive.

| Category | Resources | Short Aliases |
|----------|-----------|---------------|
| **Core** | nodes, persistentvolumes, namespaces, componentstatuses | pv, ns, cs |
| **RBAC** | clusterroles, clusterrolebindings | - |
| **Storage** | storageclasses, volumeattachments, csidrivers, csinodes, csistoragecapacities | sc |
| **Networking** | ingressclasses | - |
| **Scheduling** | priorityclasses, runtimeclasses | pc |
| **Policy** | podsecuritypolicies (deprecated) | psp |
| **Admission** | mutatingwebhookconfigurations, validatingwebhookconfigurations | - |
| **API Extensions** | customresourcedefinitions, apiservices | crd, crds |
| **Certificates** | certificatesigningrequests | csr |

## Usage Examples

### Listing Cluster-Scoped Resources

```json
// List all nodes (no namespace needed)
{
  "resourceType": "nodes"
}

// List all persistent volumes using short alias
{
  "resourceType": "pv"
}

// List all namespaces
{
  "resourceType": "ns"
}

// List all CRDs
{
  "resourceType": "crd"
}
```

### Listing Namespaced Resources

```json
// List pods in a specific namespace (namespace required)
{
  "resourceType": "pods",
  "namespace": "default"
}

// List pods across all namespaces
{
  "resourceType": "pods",
  "allNamespaces": true
}
```

## Error Messages

If you attempt to list a namespaced resource without providing a namespace:

```
namespace is required for namespaced resources. Omit namespace for cluster-scoped resources (nodes, persistentvolumes, namespaces, clusterroles, etc.) or use allNamespaces=true
```

## Custom Resources (CRDs)

Custom Resource Definitions (CRDs) themselves are cluster-scoped. However, the **instances** of custom resources may be either namespaced or cluster-scoped depending on how the CRD is defined.

### Hybrid Validation Approach

The `mcp-kubernetes` server uses a **hybrid approach** for namespace validation:

1. **Known built-in resources**: Early validation provides helpful error messages
   - Known cluster-scoped (nodes, pv, etc.): Namespace not required
   - Known namespaced (pods, deployments, etc.): Namespace required (unless `allNamespaces=true`)

2. **Unknown resources (CRDs)**: No early validation
   - The K8s API discovery determines the actual scope at runtime
   - This allows CRDs like CAPI `Cluster`, `Machine`, Flux `HelmRelease`, etc. to work correctly
   - If a namespace is needed but not provided, the K8s API returns an appropriate error

### Examples of CRDs

| CRD | Scope | Example |
|-----|-------|---------|
| CAPI `Cluster` | Cluster-scoped | `clusters.cluster.x-k8s.io` |
| CAPI `Machine` | Namespaced | `machines.cluster.x-k8s.io` |
| Flux `HelmRelease` | Namespaced | `helmreleases.helm.toolkit.fluxcd.io` |
| ArgoCD `Application` | Namespaced | `applications.argoproj.io` |
| Prometheus | Namespaced | `prometheuses.monitoring.coreos.com` |

## Implementation Details

The resource scope detection is implemented in `internal/k8s/constants.go` with three functions:

- `IsClusterScoped(resourceType)`: Returns true for known cluster-scoped resources
- `IsKnownNamespaced(resourceType)`: Returns true for known namespaced resources  
- `IsKnownResource(resourceType)`: Returns true if resource is in either known list

For unknown resources, the actual scope is determined via the Kubernetes API discovery mechanism at runtime.

