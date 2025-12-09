# Safety Modes

This document describes the safety modes available in mcp-kubernetes to control which operations are allowed. These modes help prevent accidental changes to Kubernetes clusters when using AI agents.

## Overview

mcp-kubernetes provides two safety modes that work together:

1. **Non-Destructive Mode** (default: enabled) - Blocks mutating operations
2. **Dry-Run Mode** (default: disabled) - Validates operations without applying them

## Non-Destructive Mode

When `--non-destructive` mode is enabled (the default), the server blocks all mutating operations unless they are explicitly allowed. This provides safety-by-default for AI agent interactions with Kubernetes clusters.

### Blocked Operations

The following operations are blocked by default:

| Operation | Description | CLI Tool Equivalent |
|-----------|-------------|---------------------|
| `create` | Create new resources | `kubectl create` |
| `apply` | Create or update resources | `kubectl apply` |
| `delete` | Remove resources | `kubectl delete` |
| `patch` | Update specific fields | `kubectl patch` |
| `scale` | Change replica counts | `kubectl scale` |

### Allowed Operations

The following operations are always allowed:

| Operation | Description | CLI Tool Equivalent |
|-----------|-------------|---------------------|
| `get` | Retrieve a single resource | `kubectl get -o yaml` |
| `list` | List multiple resources | `kubectl get` |
| `describe` | Get detailed information | `kubectl describe` |

### Configuration

```bash
# Enable non-destructive mode (default)
mcp-kubernetes serve --non-destructive=true

# Disable non-destructive mode (DANGEROUS: allows all operations)
mcp-kubernetes serve --non-destructive=false
```

## Dry-Run Mode

Dry-run mode allows mutating operations to be validated by the Kubernetes API server without actually applying them. This is useful for:

- Validating manifests before deployment
- Testing RBAC permissions
- Previewing changes

### Behavior

When `--dry-run` is enabled:

1. Mutating operations bypass the non-destructive mode block
2. Operations are sent to the Kubernetes API with `dryRun=All`
3. The API server validates the request and returns what would happen
4. **No actual changes are made to the cluster**

### Configuration

```bash
# Enable dry-run mode (allows validation without applying changes)
mcp-kubernetes serve --non-destructive=true --dry-run=true

# This combination validates operations without applying them
```

## Mode Combinations

| Non-Destructive | Dry-Run | Behavior |
|-----------------|---------|----------|
| `true` (default) | `false` (default) | Only read operations allowed. Mutating operations blocked. |
| `true` | `true` | All operations allowed, but mutations are dry-run only (validated, not applied). |
| `false` | `false` | **DANGEROUS**: All operations allowed and applied. |
| `false` | `true` | All operations allowed, but mutations are dry-run only. |

## AllowedOperations

For more granular control, you can configure specific operations to be allowed even in non-destructive mode:

```go
// In code, configure allowed operations
config := server.NewDefaultConfig()
config.NonDestructiveMode = true
config.AllowedOperations = []string{"get", "list", "describe", "create"} // Allow create
```

The default `AllowedOperations` are: `["get", "list", "describe"]`

## Security Recommendations

### Production Deployments

For production AI agent deployments, we recommend:

1. **Keep non-destructive mode enabled** (`--non-destructive=true`)
2. **Use dry-run for validation** (`--dry-run=true` when validation is needed)
3. **Never disable non-destructive mode** without understanding the risks

### Development and Testing

For development environments:

1. You may enable dry-run mode for testing manifests
2. Consider using restricted namespaces to limit scope
3. Use RBAC to further restrict what the service account can do

## Error Messages

When an operation is blocked, the error message includes a hint about using dry-run mode:

```
Create operations are not allowed in non-destructive mode (use --dry-run to validate without applying)
```

This helps users understand their options when they encounter a blocked operation.

## Implementation Details

### Handler-Level Checks

Each mutating operation handler checks the safety modes:

```go
config := sc.Config()
if config.NonDestructiveMode && !config.DryRun {
    // Check if operation is in AllowedOperations
    allowed := false
    for _, op := range config.AllowedOperations {
        if op == "<operation>" {
            allowed = true
            break
        }
    }
    if !allowed {
        return error("Operation not allowed in non-destructive mode")
    }
}
```

### Kubernetes API Dry-Run

When dry-run is enabled, the k8s client sends requests with the `dryRun=All` parameter:

```go
createOpts := metav1.CreateOptions{}
if c.dryRun {
    createOpts.DryRun = []string{metav1.DryRunAll}
}
```

This tells the Kubernetes API server to validate the request without persisting changes.

## Related Documentation

- [RBAC Security](rbac-security.md) - Configure Kubernetes RBAC for mcp-kubernetes
- [OAuth](oauth.md) - Authentication configuration
- [Observability](observability.md) - Monitoring and metrics

