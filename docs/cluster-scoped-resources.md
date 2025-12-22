# Namespace Handling

This document describes how `mcp-kubernetes` handles namespaces for Kubernetes resources.

## Design Principles

1. **Discovery-based**: Resource scope (namespaced vs cluster-scoped) is determined via the Kubernetes API discovery, not static lists
2. **No static lists**: We do not maintain lists of cluster-scoped resources that could become outdated
3. **kubectl-compatible**: Follows kubectl's behavior for namespace defaults
4. **Transparency**: Responses include metadata explaining what happened

## kubectl-Compatible Behavior

The `mcp-kubernetes` server follows kubectl's namespace behavior:

- **No namespace provided**: Uses the `default` namespace for namespaced resources
- **For cluster-scoped resources**: The Kubernetes API automatically ignores any namespace parameter
- **With `allNamespaces=true`**: Lists namespaced resources across all namespaces

## Tool Examples

### kubernetes_list

```json
// Lists pods in "default" namespace
{"resourceType": "pods"}

// Lists pods in kube-system namespace
{"resourceType": "pods", "namespace": "kube-system"}

// Lists pods across all namespaces
{"resourceType": "pods", "allNamespaces": true}

// Lists nodes (cluster-scoped - namespace is automatically ignored)
{"resourceType": "nodes"}

// Lists CAPI Clusters (CRD - K8s API determines scope via discovery)
{"resourceType": "clusters", "apiGroup": "cluster.x-k8s.io"}
```

### kubernetes_get

```json
// Get a pod in default namespace
{"resourceType": "pods", "name": "my-pod"}

// Get a pod in kube-system namespace
{"resourceType": "pods", "namespace": "kube-system", "name": "coredns-abc123"}

// Get a node (cluster-scoped - namespace is ignored)
{"resourceType": "nodes", "name": "worker-1"}

// Get a ClusterRole (cluster-scoped)
{"resourceType": "clusterroles", "name": "admin"}
```

### kubernetes_describe

```json
// Describe a deployment
{"resourceType": "deployments", "namespace": "production", "name": "my-app"}

// Describe a node
{"resourceType": "nodes", "name": "control-plane-1"}
```

### kubernetes_delete

```json
// Delete a pod
{"resourceType": "pods", "namespace": "default", "name": "my-pod"}

// Delete a PersistentVolume (cluster-scoped)
{"resourceType": "persistentvolumes", "name": "pv-001"}
```

### kubernetes_patch

```json
// Patch a deployment
{"resourceType": "deployments", "namespace": "production", "name": "my-app", "patchType": "merge", "patch": {"spec": {"replicas": 3}}}

// Patch a node (cluster-scoped)
{"resourceType": "nodes", "name": "worker-1", "patchType": "merge", "patch": {"metadata": {"labels": {"zone": "us-west-1"}}}}
```

## Response Metadata

All resource operations include a `_meta` field that provides transparency about how the request was interpreted:

```json
{
  "resource": {...},
  "_meta": {
    "resourceScope": "cluster",
    "requestedNamespace": "kube-system",
    "effectiveNamespace": "",
    "hint": "nodes is cluster-scoped; namespace parameter was ignored"
  }
}
```

This metadata is included in responses from:
- `kubernetes_get` - wrapped response with resource and `_meta`
- `kubernetes_list` - paginated response includes `_meta`
- `kubernetes_describe` - description includes `_meta`
- `kubernetes_delete` - response with message and `_meta`
- `kubernetes_patch` - response with patched resource and `_meta`
- `kubernetes_scale` - response with message, replicas count, and `_meta`

This helps agents understand:
- Whether the resource is namespaced or cluster-scoped
- What namespace was requested vs. what was actually used
- Explanatory hints when behavior might be unexpected

### Benefits

1. **Transparency for agents**: Agents learn when parameters are being interpreted differently than expected
2. **Reduced debugging roundtrips**: Hints explain unexpected behavior upfront
3. **Consistent interface**: Same metadata structure across all resource tools
4. **Self-correcting agents**: Agents can adjust future calls based on feedback

## How Discovery Works

1. **Tool receives request**: The handler receives a request with `resourceType` and optional `namespace`
2. **Discovery resolution**: The Kubernetes API discovery is queried to determine the resource's GVR and scope
3. **Scope determination**: The discovery response includes `Namespaced: true/false` for each resource
4. **API call construction**: Based on the scope, the API path is correctly constructed
5. **Metadata enrichment**: Response includes `_meta` explaining what happened

```
Request: {resourceType: "nodes", namespace: "kube-system"}
    |
    v
Discovery: nodes -> Namespaced: false (cluster-scoped)
    |
    v
API Call: GET /api/v1/nodes (namespace ignored)
    |
    v
Response: {items: [...], _meta: {resourceScope: "cluster", hint: "..."}}
```

## CRDs and Custom Resources

Custom Resource Definitions (CRDs) can define either namespaced or cluster-scoped resources. Since `mcp-kubernetes` uses Kubernetes API discovery, all CRDs work correctly without any static configuration:

| CRD | API Group | Scope | How It Works |
|-----|-----------|-------|--------------|
| CAPI `Cluster` | cluster.x-k8s.io | Namespaced | Uses provided or default namespace |
| CAPI `ClusterClass` | cluster.x-k8s.io | Cluster | Namespace ignored |
| CAPI `Machine` | cluster.x-k8s.io | Namespaced | Uses provided or default namespace |
| Flux `HelmRelease` | helm.toolkit.fluxcd.io | Namespaced | Uses provided or default namespace |
| Flux `HelmRepository` | source.toolkit.fluxcd.io | Namespaced | Uses provided or default namespace |
| ArgoCD `Application` | argoproj.io | Namespaced | Uses provided or default namespace |
| Crossplane `Provider` | pkg.crossplane.io | Cluster | Namespace ignored |

## Performance

Discovery results are cached by the Kubernetes discovery client, so repeated operations on the same resource type do not incur additional API calls. Additionally, the resource scope cache maintains mappings to avoid redundant resolution.

## Implementation Details

The implementation follows these key principles:

1. **Always use discovery**: Resource scope is never determined from static lists
2. **Cache discovery results**: The discovery client caches `ServerPreferredResources()` results
3. **Default to "default"**: If no namespace provided for namespaced resources, use `default`
4. **Ignore namespace for cluster-scoped**: The Kubernetes API naturally ignores namespace for cluster-scoped resources
5. **Provide transparency**: Include `_meta` in responses to explain behavior

```go
// In handlers: default namespace if not provided
if namespace == "" {
    namespace = k8s.DefaultNamespace  // "default"
}

// In k8s client: discovery determines scope
gvr, namespaced, err := resolveResourceTypeShared(resourceType, apiGroup, discoveryClient)
if namespaced && namespace != "" {
    resourceInterface = dynamicClient.Resource(gvr).Namespace(namespace)
} else {
    resourceInterface = dynamicClient.Resource(gvr)  // cluster-scoped or all namespaces
}
```

This design ensures correctness across all Kubernetes versions and custom resources without maintaining any static lists that could become outdated.
