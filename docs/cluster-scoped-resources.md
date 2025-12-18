# Namespace Handling

This document describes how `mcp-kubernetes` handles namespaces, following kubectl conventions.

## kubectl-Compatible Behavior

The `mcp-kubernetes` server follows kubectl's namespace behavior:

- **No namespace provided**: Uses the `default` namespace
- **For cluster-scoped resources**: The Kubernetes API ignores the namespace parameter
- **With `allNamespaces=true`**: Lists resources across all namespaces

This approach:
- Requires no static resource lists that could become outdated
- Works with any CRD without prior configuration
- Matches user expectations from kubectl

## Usage Examples

### Listing Resources Without Namespace

```json
// Lists pods in "default" namespace
{"resourceType": "pods"}

// Lists nodes (cluster-scoped - namespace is ignored)
{"resourceType": "nodes"}

// Lists CAPI Clusters (CRD - K8s API determines scope)
{"resourceType": "clusters", "apiGroup": "cluster.x-k8s.io"}
```

### Listing Resources With Explicit Namespace

```json
// Lists pods in kube-system namespace
{"resourceType": "pods", "namespace": "kube-system"}

// Lists deployments in production namespace
{"resourceType": "deployments", "namespace": "production"}
```

### Listing Resources Across All Namespaces

```json
// Lists pods across all namespaces
{"resourceType": "pods", "allNamespaces": true}

// Lists all deployments in the cluster
{"resourceType": "deployments", "allNamespaces": true}
```

## How It Works

1. **Namespace resolution**: If no namespace is provided and `allNamespaces` is false, the handler uses `default`
2. **API discovery**: The Kubernetes API discovery determines whether a resource is namespaced or cluster-scoped
3. **Cluster-scoped handling**: For cluster-scoped resources (nodes, namespaces, PVs, etc.), the K8s API ignores any namespace parameter

## CRDs and Custom Resources

Custom Resource Definitions (CRDs) can define either namespaced or cluster-scoped resources. Since `mcp-kubernetes` uses the Kubernetes API discovery, all CRDs work correctly without any static configuration:

| CRD | Scope | How It Works |
|-----|-------|--------------|
| CAPI `Cluster` | Cluster-scoped | API ignores namespace |
| CAPI `Machine` | Namespaced | Uses provided or default namespace |
| Flux `HelmRelease` | Namespaced | Uses provided or default namespace |
| ArgoCD `Application` | Namespaced | Uses provided or default namespace |

## Implementation Details

The namespace handling is implemented simply:

```go
// Follow kubectl behavior: if no namespace specified, use "default".
// For cluster-scoped resources, the Kubernetes API ignores the namespace.
if !allNamespaces && namespace == "" {
    namespace = k8s.DefaultNamespace  // "default"
}
```

This eliminates the need for maintaining static lists of cluster-scoped or namespaced resources, which would be error-prone and require updates as Kubernetes evolves.
