# Client-Side Filtering

The `kubernetes_list` tool supports client-side filtering to enable advanced filtering scenarios that aren't supported by Kubernetes native field selectors.

## Overview

Kubernetes native field selectors are limited to a small set of fields (e.g., `metadata.name`, `metadata.namespace`, `spec.nodeName`, `status.phase`). For fields like `spec.taints` on nodes, you cannot use field selectors.

Client-side filtering solves this by filtering resources after they are retrieved from the API server, providing a convenient layer for complex filtering operations.

## Usage

Add a `filter` parameter to your `kubernetes_list` call with a JSON object specifying the filter criteria:

```json
{
  "resourceType": "nodes",
  "namespace": "",
  "allNamespaces": true,
  "filter": {
    "spec.taints[*].key": "karpenter.sh/unregistered"
  }
}
```

## Filter Syntax

### Simple Field Matching

Filter by a direct field value:

```json
{
  "metadata.name": "my-pod"
}
```

### Nested Map Matching

Filter by nested fields using dot notation:

```json
{
  "metadata.labels.app": "nginx"
}
```

### Array Element Matching

Use `[*]` wildcard to match any element in an array:

```json
{
  "spec.taints[*].key": "node.kubernetes.io/unschedulable"
}
```

This will match any node that has a taint with the specified key, regardless of where it appears in the taints array.

### Multiple Criteria (AND Logic)

All criteria must match for a resource to be included:

```json
{
  "spec.taints[*].key": "karpenter.sh/unregistered",
  "spec.taints[*].effect": "NoExecute"
}
```

## Examples

### Example 1: Find Nodes with Specific Taint

Find all nodes that have the `karpenter.sh/unregistered` taint:

```json
{
  "resourceType": "nodes",
  "namespace": "",
  "allNamespaces": true,
  "filter": {
    "spec.taints[*].key": "karpenter.sh/unregistered"
  }
}
```

### Example 2: Find Pods by Label

Find all pods with label `app=nginx`:

```json
{
  "resourceType": "pods",
  "namespace": "default",
  "filter": {
    "metadata.labels.app": "nginx"
  }
}
```

### Example 3: Combine with Label Selectors

You can combine client-side filtering with label selectors for efficient filtering:

```json
{
  "resourceType": "pods",
  "namespace": "production",
  "labelSelector": "app=nginx",
  "filter": {
    "status.phase": "Running"
  }
}
```

The label selector runs server-side (more efficient), and the filter runs client-side on the results.

## Performance Considerations

- **Client-side filtering runs after API retrieval**: Resources are fetched from the API server first, then filtered locally.
- **Combine with server-side selectors**: Use `labelSelector` and `fieldSelector` when possible to reduce the amount of data transferred from the API server.
- **Pagination works before filtering**: The `limit` parameter affects how many items are retrieved from the API, not how many filtered results you get.

## Use Cases

### Incident Investigation

During an incident with Karpenter nodes:

```json
{
  "resourceType": "nodes",
  "namespace": "",
  "allNamespaces": true,
  "filter": {
    "spec.taints[*].key": "karpenter.sh/unregistered"
  }
}
```

### Finding Resources with Specific Configurations

Find deployments with a specific number of replicas:

```json
{
  "resourceType": "deployments",
  "namespace": "production",
  "filter": {
    "spec.replicas": 3
  }
}
```

### Complex Label Matching

Find pods with multiple specific labels:

```json
{
  "resourceType": "pods",
  "namespace": "production",
  "filter": {
    "metadata.labels.app": "api",
    "metadata.labels.version": "v2"
  }
}
```

## Limitations

- **Performance**: Client-side filtering requires fetching all resources first, which can be slow for large result sets. Use server-side selectors when possible.
- **Pagination**: Filtering happens after pagination, so you may get fewer results than the `limit` parameter specifies.
- **Complex matching**: The filter currently supports exact matches and array element matching, but not regex or range queries.

## Troubleshooting

### No Results Returned

If your filter returns no results:

1. **Check the field path**: Ensure the path is correct (e.g., `spec.taints[*].key`, not `spec.taint[*].key`)
2. **Check the value**: Make sure the value matches exactly (case-sensitive)
3. **Test without filter**: Try listing without the filter to see what data is available

### Unexpected Results

If you get unexpected results:

1. **Check with `fullOutput: true`**: See the full resource manifests to understand the data structure
2. **Simplify the filter**: Start with a single criterion and add more to narrow down the issue
3. **Verify array paths**: Ensure you're using `[*]` for array fields

## Security Considerations

Client-side filtering includes built-in protections against resource exhaustion attacks:

- **RBAC Enforcement**: Client-side filtering respects Kubernetes RBAC. You can only filter resources you have permission to view. Filtering happens after authentication and authorization, so it cannot be used to access unauthorized resources.

- **Resource Limits**: To prevent denial-of-service attacks, the following limits are enforced:
  - Maximum 50 filter criteria per request
  - Maximum path depth of 20 levels (e.g., `a.b.c.d...`)
  - Maximum filter value size of 1024 bytes for string values

- **Performance Monitoring**: The system logs a warning when filtered results exceed 1000 items, as this may indicate inefficient filtering. Consider using server-side selectors (`labelSelector`, `fieldSelector`) when possible to reduce data transfer and memory usage.

- **Path Validation**: Filter paths are validated to prevent malicious patterns:
  - Empty paths are rejected
  - Paths containing `..` are rejected
  - Excessive nesting is prevented

### Best Practices for Security

1. **Prefer Server-Side Filtering**: Use `labelSelector` and `fieldSelector` when possible, as they filter at the API server level and are more efficient.

2. **Limit Result Sets**: Combine filters with appropriate `limit` parameters to avoid processing large datasets.

3. **Monitor Usage**: In multi-tenant environments, monitor logs for warnings about large result sets or rejected filter criteria.

4. **Trusted Users Only**: While client-side filtering includes DoS protections, it's designed for trusted users within your organization. Additional rate limiting may be needed for untrusted or public-facing deployments.

## Related Issues

- [Issue #88: Field selector doesn't support filtering nodes by taints](https://github.com/giantswarm/mcp-kubernetes/issues/88)

