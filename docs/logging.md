# Logging Guidelines

This document describes the structured logging patterns used throughout `mcp-kubernetes`.

## Overview

The application uses Go's standard library `log/slog` package for structured logging. This provides:

- Consistent, machine-parseable log output
- Structured key-value attributes for easy filtering and querying
- Built-in support for log levels
- JSON output format suitable for log aggregation systems

## Logging Levels

Use the following guidelines when choosing log levels:

| Level   | When to Use                                                                          | Examples                                          |
|---------|--------------------------------------------------------------------------------------|---------------------------------------------------|
| `Debug` | Detailed diagnostic information useful during development/troubleshooting            | Client cache hits, REST config creation, context switching |
| `Info`  | Normal operational events that indicate progress or state changes                    | Server started, OAuth enabled, cluster connected  |
| `Warn`  | Potentially problematic situations that don't prevent operation                      | Cache TTL exceeds token lifetime, DNS lookup failure, large result sets |
| `Error` | Failures that require attention but don't crash the application                      | Shutdown errors, authentication failures          |

## Security Considerations

### PII Sanitization

Never log Personally Identifiable Information (PII) directly. Use sanitization functions:

```go
import "github.com/giantswarm/mcp-kubernetes/internal/logging"

// Bad - logs full email
slog.Info("user operation", "email", user.Email)

// Good - logs anonymized hash
slog.Info("user operation", logging.UserHash(user.Email))

// Good - logs only domain for lower cardinality
slog.Info("user operation", logging.Domain(user.Email))
```

### Host/URL Sanitization

Redact IP addresses to prevent network topology leakage:

```go
// Bad - exposes internal IPs
slog.Info("connecting", "host", config.Host)

// Good - redacts IP addresses
slog.Info("connecting", logging.Host(config.Host))
```

### Token/Credential Sanitization

Never log tokens or credentials, even partially:

```go
// Bad - exposes token prefix
slog.Debug("authenticating", "token", token[:10])

// Good - use sanitized version
slog.Debug("authenticating", "token", logging.SanitizeToken(token))
```

## Attribute Naming

Use consistent attribute names throughout the codebase. The `logging` package provides constants and helper functions:

| Attribute Key    | Helper Function      | Description                     |
|------------------|---------------------|---------------------------------|
| `operation`      | `logging.Operation()` | Operation name (list, get, etc) |
| `namespace`      | `logging.Namespace()` | Kubernetes namespace            |
| `resource_type`  | `logging.ResourceType()` | Resource type (pods, services) |
| `resource_name`  | `logging.ResourceName()` | Resource name                  |
| `cluster`        | `logging.Cluster()` | Cluster name                    |
| `user_hash`      | `logging.UserHash()` | Anonymized user identifier     |
| `status`         | `logging.Status()` | Operation status                |
| `error`          | `logging.Err()` | Error message                   |
| `host`           | `logging.Host()` | Sanitized host URL              |

## Usage Examples

### Basic Structured Logging

```go
import (
    "log/slog"
    "github.com/giantswarm/mcp-kubernetes/internal/logging"
)

// With helper functions (preferred)
slog.Info("listing resources",
    logging.ResourceType("pods"),
    logging.Namespace("default"),
    slog.Int("count", 42))

// With slog directly
slog.Warn("cache TTL exceeds token lifetime",
    slog.Duration("cache_ttl", cacheTTL),
    slog.Duration("token_lifetime", tokenLifetime))
```

### Creating Context-Enriched Loggers

```go
// Add context attributes to a logger for reuse
logger := logging.WithOperation(slog.Default(), "resource.list")
logger = logging.WithCluster(logger, "prod-cluster")

// All subsequent logs include these attributes
logger.Info("starting operation")
logger.Debug("processing item", slog.Int("index", 0))
```

### Logger Adapter for k8s Package

The `internal/k8s` package uses a `Logger` interface. Use the adapter:

```go
import "github.com/giantswarm/mcp-kubernetes/internal/logging"

// Create adapter from slog.Logger
k8sLogger := logging.NewSlogAdapter(slog.Default())

// Or use the default
k8sLogger := logging.DefaultLogger()
```

## Configuration

### Log Level

Set the log level via environment variable or handler configuration:

```bash
# Via environment (if supported by handler)
export LOG_LEVEL=debug
```

Or programmatically:

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})
slog.SetDefault(slog.New(handler))
```

### Output Format

For production, use JSON output for log aggregation:

```go
handler := slog.NewJSONHandler(os.Stdout, nil)
slog.SetDefault(slog.New(handler))
```

For development, use text output for readability:

```go
handler := slog.NewTextHandler(os.Stdout, nil)
slog.SetDefault(slog.New(handler))
```

## Best Practices

1. **Be Consistent**: Use the `logging` package helpers for attribute names
2. **Be Secure**: Always sanitize PII, tokens, and internal network details
3. **Be Specific**: Include relevant context (namespace, resource type, cluster)
4. **Be Appropriate**: Choose the right log level for the situation
5. **Be Concise**: Log messages should be actionable and informative
6. **Avoid Noise**: Don't log expected events at high levels (e.g., don't log every successful request at Info)

