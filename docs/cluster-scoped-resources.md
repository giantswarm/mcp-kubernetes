# Cluster-Scoped Resources

This document describes how `mcp-kubernetes` handles cluster-scoped vs namespaced resources.

## Overview

Kubernetes resources are either **namespaced** (exist within a namespace) or **cluster-scoped** (exist at the cluster level). When using the `kubernetes_list` tool, the `namespace` parameter behavior differs based on the resource type:

- **Namespaced resources** (pods, deployments, services, etc.): Require a `namespace` parameter or `allNamespaces=true`
- **Cluster-scoped resources** (nodes, persistentvolumes, namespaces, etc.): Do not require a `namespace` parameter

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

Custom Resource Definitions (CRDs) themselves are cluster-scoped. However, the **instances** of custom resources may be either namespaced or cluster-scoped depending on how the CRD is defined. The `mcp-kubernetes` server uses API discovery to determine the scope of custom resource instances at runtime.

## Implementation Details

The cluster-scoped resource detection is implemented in `internal/k8s/constants.go` as a single source of truth. This ensures consistent behavior across all tools that need to distinguish between namespaced and cluster-scoped resources.

