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
| `exec` | Execute commands in pods | `kubectl exec` |
| `port-forward` | Forward ports to pods/services | `kubectl port-forward` |

#### Why exec and port-forward are blocked

These operations are blocked because they can bypass the read-only guarantees:

- **exec**: Allows arbitrary command execution inside pods, which can modify files, delete data, read secrets, or perform any action the pod's service account allows. This completely bypasses Kubernetes API-level protections.

- **port-forward**: Establishes network tunnels to internal services, which could be used to access databases, admin interfaces, or other services that allow modifications through their own APIs.

### Allowed Operations

The following operations are always allowed (read-only):

| Operation | Description | CLI Tool Equivalent |
|-----------|-------------|---------------------|
| `get` | Retrieve a single resource | `kubectl get -o yaml` |
| `list` | List multiple resources | `kubectl get` |
| `describe` | Get detailed information | `kubectl describe` |
| `logs` | View pod logs | `kubectl logs` |

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
| `true` | `true` | Resource mutations are dry-run only (validated, not applied). Note: `exec` and `port-forward` are allowed but cannot be dry-run validated by Kubernetes. |
| `false` | `false` | **DANGEROUS**: All operations allowed and applied. |
| `false` | `true` | All operations allowed, but resource mutations are dry-run only. |

**Important**: The `exec` and `port-forward` operations cannot be validated via Kubernetes dry-run because they don't create or modify Kubernetes resources - they establish direct connections to running workloads.

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

All mutating operation handlers use the shared `CheckMutatingOperation` function:

```go
// CheckMutatingOperation verifies if a mutating operation is allowed.
// Returns an error result if blocked, nil if allowed.
func CheckMutatingOperation(sc *server.ServerContext, operation string) *mcp.CallToolResult {
    config := sc.Config()
    if !config.NonDestructiveMode || config.DryRun {
        return nil
    }

    for _, op := range config.AllowedOperations {
        if op == operation {
            return nil
        }
    }

    return mcp.NewToolResultError(fmt.Sprintf(
        "%s operations are not allowed in non-destructive mode",
        operation,
    ))
}
```

This centralized function is used by all handlers that perform potentially dangerous operations:
- Resource handlers: `create`, `apply`, `delete`, `patch`, `scale`
- Pod handlers: `exec`, `port-forward`

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


