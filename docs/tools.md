# The tools

Generated from the registered tools (`make tools-doc`); through muster every tool is `x_<server>_<name>`. This page lists every tool; a deployment serves a subset by its mode: the context tools and port forwarding only outside the cluster, create, apply, delete, patch, scale, exec and port forwarding only where mutations are allowed, the CAPI tools only with federation. Parameters vary the same way: `cluster` only with federation, `kubeContext` only outside the cluster.

| Tool | Kind |
|---|---|
| `api_resources` | read-only |
| `apply` | write |
| `capi_cluster_health` | read-only |
| `capi_get_cluster` | read-only |
| `capi_list_clusters` | read-only |
| `capi_resolve_cluster` | read-only |
| `cluster_health` | read-only |
| `context_get_current` | read-only |
| `context_list` | read-only |
| `context_use` | write |
| `create` | write |
| `delete` | write, destructive |
| `describe` | read-only |
| `exec` | write, destructive |
| `get` | read-only |
| `list` | read-only |
| `list_port_forward_sessions` | read-only |
| `logs` | read-only |
| `patch` | write, destructive |
| `port_forward` | write |
| `scale` | write, destructive |
| `stop_all_port_forward_sessions` | write, destructive |
| `stop_port_forward_session` | write, destructive |

## `api_resources`

```text
List available API resources in the cluster
```

```json
{
  "additionalProperties": false,
  "properties": {
    "apiGroup": {
      "description": "Filter by API group (optional)",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "limit": {
      "description": "Maximum number of items to return per page (optional, default: 20, 0 = no limit)",
      "type": "number"
    },
    "namespaced": {
      "description": "Filter by namespaced resources only (optional)",
      "type": "boolean"
    },
    "offset": {
      "description": "Number of items to skip (optional, for simple offset-based pagination)",
      "type": "number"
    },
    "verbs": {
      "description": "Filter by supported verbs (e.g., 'get,list,create') (optional)",
      "type": "string"
    }
  },
  "required": [],
  "type": "object"
}
```

## `apply`

```text
Apply a Kubernetes manifest (create or update)
```

```json
{
  "additionalProperties": false,
  "properties": {
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "manifest": {
      "description": "Kubernetes manifest as JSON object",
      "properties": {},
      "type": "object"
    },
    "namespace": {
      "description": "Namespace where the resource should be applied",
      "type": "string"
    }
  },
  "required": [
    "namespace",
    "manifest"
  ],
  "type": "object"
}
```

## `capi_cluster_health`

```text
Check the health status of a CAPI cluster. Returns overall health, component status, and individual health checks.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "name": {
      "description": "The name of the cluster to check health for",
      "type": "string"
    }
  },
  "required": [
    "name"
  ],
  "type": "object"
}
```

## `capi_get_cluster`

```text
Get detailed information about a specific CAPI cluster including metadata, status, labels, and annotations.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "name": {
      "description": "The name of the cluster to get details for",
      "type": "string"
    }
  },
  "required": [
    "name"
  ],
  "type": "object"
}
```

## `capi_list_clusters`

```text
List the workload clusters managed by CAPI that you can access: name, organization, provider, release, status, age.

Results are limited by default; narrow with the filters or raise limit.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "labelSelector": {
      "description": "Filter clusters by Kubernetes label selector (e.g., 'environment=production')",
      "type": "string"
    },
    "limit": {
      "description": "Maximum number of clusters to return (default: 100, max: 500). Use filters to narrow results if truncated.",
      "type": "number"
    },
    "organization": {
      "description": "Filter by organization namespace (e.g., 'org-acme')",
      "type": "string"
    },
    "provider": {
      "description": "Filter by infrastructure provider (aws, azure, gcp, vsphere)",
      "type": "string"
    },
    "readyOnly": {
      "description": "Only show clusters that are fully ready (default: false)",
      "type": "boolean"
    },
    "status": {
      "description": "Filter by cluster status (Provisioned, Provisioning, Deleting, Failed)",
      "type": "string"
    }
  },
  "required": [],
  "type": "object"
}
```

## `capi_resolve_cluster`

```text
Resolve a partial cluster name pattern to its full identifier. Useful when you only know part of a cluster name.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "pattern": {
      "description": "Partial cluster name or pattern to search for (e.g., 'prod' to find 'prod-wc-01')",
      "type": "string"
    }
  },
  "required": [
    "pattern"
  ],
  "type": "object"
}
```

## `cluster_health`

```text
Check the health of the cluster's components and nodes: overall status, component health and a node list.

The node list is capped by nodesLimit; per-node conditions only with includeNodeConditions: true.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "includeNodeConditions": {
      "description": "Include the full per-node conditions array in the response (default: false). The Ready field on each node already conveys overall readiness.",
      "type": "boolean"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "nodesLimit": {
      "description": "Most nodes to include (default 20, at most 1000); readyNodes and totalNodes count them all.",
      "maximum": 1000,
      "minimum": 1,
      "type": "number"
    }
  },
  "required": [],
  "type": "object"
}
```

## `context_get_current`

```text
Get the current Kubernetes context
```

```json
{
  "additionalProperties": false,
  "properties": {},
  "required": [],
  "type": "object"
}
```

## `context_list`

```text
List all available Kubernetes contexts
```

```json
{
  "additionalProperties": false,
  "properties": {},
  "required": [],
  "type": "object"
}
```

## `context_use`

```text
Switch to a different Kubernetes context
```

```json
{
  "additionalProperties": false,
  "properties": {
    "contextName": {
      "description": "Name of the Kubernetes context to switch to",
      "type": "string"
    }
  },
  "required": [
    "contextName"
  ],
  "type": "object"
}
```

## `create`

```text
Create a new Kubernetes resource from a manifest
```

```json
{
  "additionalProperties": false,
  "properties": {
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "manifest": {
      "description": "Kubernetes manifest as JSON object",
      "properties": {},
      "type": "object"
    },
    "namespace": {
      "description": "Namespace where the resource should be created",
      "type": "string"
    }
  },
  "required": [
    "namespace",
    "manifest"
  ],
  "type": "object"
}
```

## `delete`

```text
Delete a Kubernetes resource.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "apiGroup": {
      "description": "Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "name": {
      "description": "Name of the resource to delete",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace of a namespaced resource; default 'default'. Ignored for cluster-scoped resources (nodes, namespaces, PVs, clusterroles).",
      "type": "string"
    },
    "resourceType": {
      "description": "Resource type (pod, service, deployment, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io).",
      "type": "string"
    }
  },
  "required": [
    "resourceType",
    "name"
  ],
  "type": "object"
}
```

## `describe`

```text
Get detailed information about a Kubernetes resource, including its events.

Secret data is always masked.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "apiGroup": {
      "description": "Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "eventsLimit": {
      "description": "Most events to return, newest first (default 50, at most 1000); totalEvents and eventsTruncated tell when clipped. output does not shape them.",
      "maximum": 1000,
      "minimum": 1,
      "type": "number"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "name": {
      "description": "Name of the resource to describe",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace of a namespaced resource; default 'default'. Ignored for cluster-scoped resources (nodes, namespaces, PVs, clusterroles).",
      "type": "string"
    },
    "output": {
      "description": "normal strips managedFields, ownerReferences, finalizers; slim (default) also HelmRelease values/history, long env lists; wide/full keep all.",
      "enum": [
        "slim",
        "normal",
        "wide",
        "full"
      ],
      "type": "string"
    },
    "resourceType": {
      "description": "Resource type (pod, service, deployment, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io).",
      "type": "string"
    }
  },
  "required": [
    "resourceType",
    "name"
  ],
  "type": "object"
}
```

## `exec`

```text
Execute a command inside a pod container
```

```json
{
  "additionalProperties": false,
  "properties": {
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "command": {
      "description": "Command to execute as an array of strings",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "containerName": {
      "description": "Name of the container (optional for single-container pods)",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace where the pod is located",
      "type": "string"
    },
    "podName": {
      "description": "Name of the pod to execute command in",
      "type": "string"
    },
    "tty": {
      "description": "Allocate a TTY for the exec session (default: false)",
      "type": "boolean"
    }
  },
  "required": [
    "namespace",
    "podName",
    "command"
  ],
  "type": "object"
}
```

## `get`

```text
Get a specific Kubernetes resource by name.

Secret data is always masked.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "apiGroup": {
      "description": "Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "name": {
      "description": "Name of the resource to get",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace of a namespaced resource; default 'default'. Ignored for cluster-scoped resources (nodes, namespaces, PVs, clusterroles).",
      "type": "string"
    },
    "output": {
      "description": "normal strips managedFields, ownerReferences, finalizers; slim (default) also HelmRelease values/history, long env lists; wide/full keep all.",
      "enum": [
        "slim",
        "normal",
        "wide",
        "full"
      ],
      "type": "string"
    },
    "resourceType": {
      "description": "Resource type (pod, service, deployment, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io).",
      "type": "string"
    }
  },
  "required": [
    "resourceType",
    "name"
  ],
  "type": "object"
}
```

## `list`

```text
List Kubernetes resources of a type, narrowed by namespace, label or field selectors, or a client-side filter.

limit takes events newest first: an event query scans at most 5000 matches (the answer says when it hit that: narrow it) and returns no continue token; passing one gives unsorted API order. Every other type comes in API order (namespace/name), so limit takes an alphabetical prefix, not the newest or the worst. Nothing selects by age: filter matches exact values. Examples: {"resourceType": "pods", "allNamespaces": true}, {"resourceType": "clusters", "apiGroup": "cluster.x-k8s.io"}.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "allNamespaces": {
      "description": "List a namespaced resource across all namespaces; overrides namespace. Ignored for cluster-scoped resources.",
      "type": "boolean"
    },
    "apiGroup": {
      "description": "Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "continue": {
      "description": "Continue token from previous paginated request (optional)",
      "type": "string"
    },
    "fieldSelector": {
      "description": "Indexed fields only, per Kind: metadata.name, .namespace on all; e.g. pods spec.nodeName, status.phase; events reason, type, involvedObject.*.",
      "type": "string"
    },
    "filter": {
      "description": "Client-side filter for what selectors cannot do: dot paths, [*] over arrays, e.g. {\"spec.taints[*].key\": \"karpenter.sh/unregistered\"}.",
      "properties": {},
      "type": "object"
    },
    "fullOutput": {
      "description": "Return full manifests instead of the compact per-Kind summary (default false); output applies only then.",
      "type": "boolean"
    },
    "includeAnnotations": {
      "description": "Include resource annotations in summary output (default: false)",
      "type": "boolean"
    },
    "includeLabels": {
      "description": "Include resource labels in summary output (default: false)",
      "type": "boolean"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "labelSelector": {
      "description": "Server-side label selector for efficient filtering (e.g., 'app=nginx,env=prod'). Use this when possible for better performance.",
      "type": "string"
    },
    "limit": {
      "description": "Maximum number of items to return per page (optional, default: 20, max: 1000)",
      "type": "number"
    },
    "namespace": {
      "description": "Namespace of a namespaced resource; default 'default', allNamespaces for every one. Ignored for cluster-scoped resources (nodes, PVs, …).",
      "type": "string"
    },
    "output": {
      "description": "With fullOutput: normal strips managedFields, ownerReferences, finalizers; slim (default) also per-Kind fields; wide/full keep all. Secrets masked.",
      "enum": [
        "slim",
        "normal",
        "wide",
        "full"
      ],
      "type": "string"
    },
    "resourceType": {
      "description": "Resource type to list (pods, services, nodes, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io).",
      "type": "string"
    },
    "summary": {
      "description": "Return aggregated counts (by status, namespace) instead of full objects. Useful for fleet-scale operations with many results. Default: false",
      "type": "boolean"
    }
  },
  "required": [
    "resourceType"
  ],
  "type": "object"
}
```

## `list_port_forward_sessions`

```text
List all active port forwarding sessions
```

```json
{
  "additionalProperties": false,
  "properties": {},
  "required": [],
  "type": "object"
}
```

## `logs`

```text
Get logs from a pod container
```

```json
{
  "additionalProperties": false,
  "properties": {
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "containerName": {
      "description": "Name of the container (optional for single-container pods)",
      "type": "string"
    },
    "follow": {
      "description": "Follow log output (default: false)",
      "type": "boolean"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace where the pod is located",
      "type": "string"
    },
    "output": {
      "description": "Accepted for symmetry with list / get / describe; currently a no-op for log content. Use tailLines and sinceTime to shape log volume.",
      "enum": [
        "slim",
        "normal",
        "wide",
        "full"
      ],
      "type": "string"
    },
    "podName": {
      "description": "Name of the pod to get logs from",
      "type": "string"
    },
    "previous": {
      "description": "Get logs from previous container instance (default: false)",
      "type": "boolean"
    },
    "sinceTime": {
      "description": "Only return log lines after this RFC3339 timestamp (e.g. 2026-04-29T10:00:00Z). Optional.",
      "type": "string"
    },
    "tailLines": {
      "description": "Return the last N lines of the log. Default: 100. Maximum: 1000.",
      "maximum": 1000,
      "minimum": 1,
      "type": "number"
    },
    "timestamps": {
      "description": "Include timestamps in log output (default: false)",
      "type": "boolean"
    }
  },
  "required": [
    "namespace",
    "podName"
  ],
  "type": "object"
}
```

## `patch`

```text
Patch a Kubernetes resource with specific changes.
```

```json
{
  "additionalProperties": false,
  "properties": {
    "apiGroup": {
      "description": "Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "name": {
      "description": "Name of the resource to patch",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace of a namespaced resource; default 'default'. Ignored for cluster-scoped resources (nodes, namespaces, PVs, clusterroles).",
      "type": "string"
    },
    "patch": {
      "description": "Patch data as JSON object",
      "properties": {},
      "type": "object"
    },
    "patchType": {
      "description": "Type of patch (strategic, merge, json)",
      "enum": [
        "strategic",
        "merge",
        "json"
      ],
      "type": "string"
    },
    "resourceType": {
      "description": "Resource type (pod, service, deployment, …); a custom resource may name its API group as kubectl does (clusters.cluster.x-k8s.io).",
      "type": "string"
    }
  },
  "required": [
    "resourceType",
    "name",
    "patchType",
    "patch"
  ],
  "type": "object"
}
```

## `port_forward`

```text
Port-forward to a pod or service
```

```json
{
  "additionalProperties": false,
  "properties": {
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace where the resource is located",
      "type": "string"
    },
    "podName": {
      "description": "Deprecated: use resourceName. Used as the pod name when resourceName is empty.",
      "type": "string"
    },
    "ports": {
      "description": "Port mappings as array of strings (e.g., ['8080:80', '9090:9090'])",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "resourceName": {
      "description": "Name of the pod or service to port-forward to",
      "type": "string"
    },
    "resourceType": {
      "description": "Type of resource to port-forward to: 'pod' or 'service' (default: 'pod')",
      "enum": [
        "pod",
        "service"
      ],
      "type": "string"
    }
  },
  "required": [
    "namespace",
    "resourceName",
    "ports"
  ],
  "type": "object"
}
```

## `scale`

```text
Scale a Kubernetes resource (deployment, replicaset, etc.)
```

```json
{
  "additionalProperties": false,
  "properties": {
    "apiGroup": {
      "description": "Optional API group for the resource (e.g., 'apps', 'networking.k8s.io', or 'apps/v1')",
      "type": "string"
    },
    "cluster": {
      "description": "Target cluster; empty for the local one.",
      "type": "string"
    },
    "kubeContext": {
      "description": "Kubernetes context; default: the current one.",
      "type": "string"
    },
    "name": {
      "description": "Name of the resource to scale",
      "type": "string"
    },
    "namespace": {
      "description": "Namespace where the resource is located",
      "type": "string"
    },
    "replicas": {
      "description": "Number of replicas to scale to",
      "type": "number"
    },
    "resourceType": {
      "description": "Scalable resource type (deployment, replicaset, statefulset), optionally with its API group as kubectl does (deployments.apps).",
      "type": "string"
    }
  },
  "required": [
    "namespace",
    "resourceType",
    "name",
    "replicas"
  ],
  "type": "object"
}
```

## `stop_all_port_forward_sessions`

```text
Stop all active port forwarding sessions
```

```json
{
  "additionalProperties": false,
  "properties": {},
  "required": [],
  "type": "object"
}
```

## `stop_port_forward_session`

```text
Stop a specific port forwarding session by ID
```

```json
{
  "additionalProperties": false,
  "properties": {
    "sessionID": {
      "description": "ID of the port forwarding session to stop",
      "type": "string"
    }
  },
  "required": [
    "sessionID"
  ],
  "type": "object"
}
```
