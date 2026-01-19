# Security Assessment: mcp-kubernetes

**Assessment Date:** January 19, 2026  
**Assessor:** Claude Opus 4.5 (Anthropic)  
**Repository:** github.com/giantswarm/mcp-kubernetes  
**Scope:** Full codebase security review focused on identity management and authentication

---

## Executive Summary

**mcp-kubernetes** is an MCP (Model Context Protocol) server providing AI agents access to Kubernetes clusters. The project demonstrates **strong security engineering practices** with multiple defense-in-depth layers. The OAuth 2.1 implementation, RBAC enforcement, and non-destructive mode defaults reflect security-conscious design. However, there are areas requiring attention in production deployments.

**Overall Rating: B+ (Good)** - Solid security foundation with comprehensive documentation and thoughtful defaults, with some areas for improvement.

### Key Findings

| Category | Rating | Summary |
|----------|--------|---------|
| Authentication | A | OAuth 2.1 with PKCE, rate limiting, token encryption |
| Authorization | A- | Layered RBAC with non-destructive defaults |
| Input Validation | A | Comprehensive validation preventing injection attacks |
| Secrets Management | B+ | Runtime masking good, storage defaults need work |
| Container Security | A | Distroless image, non-root, minimal privileges |
| Network Security | A | Strong headers, HTTPS enforcement, CORS validation |
| Documentation | A | Excellent security documentation and incident response |

---

## 1. Authentication & Authorization

### 1.1 OAuth 2.1 Implementation

**Strengths:**

- **OAuth 2.1 Compliance**: Uses `mcp-oauth v0.2.40` library with PKCE enforcement (`RequirePKCE: true`, `AllowPKCEPlain: false`)
- **Token Encryption at Rest**: AES-256-GCM encryption available via `OAUTH_ENCRYPTION_KEY`
- **Rate Limiting**: Multi-layer protection:
  - IP-based: 10 req/s, burst 20
  - User-based: 100 req/s, burst 200  
  - Client registration: max 10 clients per IP
- **SSRF Protection**: Validates OAuth URLs to block private IPs by default
- **Audit Logging**: Security audit logging enabled by default via `security.NewAuditor`
- **Client ID Metadata Documents (CIMD)**: Supports MCP 2025-11-25 spec for client authentication
- **TLS 1.2 Minimum**: Enforced for custom CA connections

**Code Reference:**

```go
// internal/server/oauth_http.go
tlsConfig := &tls.Config{
    RootCAs:    caCertPool,
    MinVersion: tls.VersionTLS12,
}
```

**Concerns:**

1. **In-Memory Token Storage Default**: In-memory storage loses tokens on restart; Valkey (Redis) recommended for production but not enforced
2. **Debug Mode Risk**: `--debug=true` is hardcoded in Helm deployment template - could leak sensitive information in production

**Recommendation**: Change Helm template to `--debug={{ .Values.mcpKubernetes.debug | default false }}`

### 1.2 Downstream OAuth (Fail-Closed Model)

**Strong Design**: When `--downstream-oauth` is enabled, the system operates in strict mode:

```go
// internal/server/context.go
if !ok || accessToken == "" {
    if sc.downstreamOAuthStrict {
        // Strict mode: fail closed - do not fall back to service account
        sc.logger.Warn("No access token in context, denying access (strict mode enabled)")
        return nil, ErrOAuthTokenMissing
    }
}
```

This prevents privilege escalation through service account fallback - an excellent security practice.

### 1.3 SSO Token Forwarding

The SSO implementation uses JWKS-based JWT validation with explicit trust model:
- Only explicitly listed `TrustedAudiences` accepted
- Same issuer requirement enforced
- Cryptographic signature verification via IdP's JWKS

---

## 2. Authorization & RBAC

### 2.1 Non-Destructive Mode (Defense in Depth)

**Excellent Default Posture**: Non-destructive mode is enabled by default:

```go
// internal/server/context.go
NonDestructiveMode:   true,
DryRun:               false,
```

Blocked operations in non-destructive mode:
- `create`, `apply`, `delete`, `patch`, `scale`
- `exec` (arbitrary command execution)
- `port-forward` (network tunnel creation)

The rationale for blocking `exec` and `port-forward` is well-documented in `docs/safety-modes.md`:
- **exec**: Allows arbitrary command execution inside pods, bypassing Kubernetes API-level protections
- **port-forward**: Establishes network tunnels that could access internal services

### 2.2 Kubernetes RBAC Profiles

The Helm chart provides security-tiered RBAC profiles:

| Profile | Risk Level | Notes |
|---------|-----------|-------|
| `minimal` | Low | Recommended for downstream OAuth |
| `readonly` | Low | Excludes secrets |
| `standard` | Medium | **Includes cluster-wide secret access** |
| `admin` | Critical | Requires explicit `adminConfirmation: true` |

**Warning in values.yaml:**

```yaml
# WARNING: "standard" profile grants CLUSTER-WIDE SECRET ACCESS.
# Consider using "readonly" if you don't need write operations.
profile: "standard"
```

**Recommendation**: Consider changing default to `readonly` for better security-by-default posture.

### 2.3 User Impersonation (Federation)

For multi-cluster CAPI mode, impersonation headers propagate user identity:

- `Impersonate-User`: User's email from OAuth
- `Impersonate-Group`: User's groups from OAuth
- `Impersonate-Extra-agent`: `mcp-kubernetes` (for audit trail)

Proper impersonation validation in place with comprehensive input sanitization.

---

## 3. Input Validation & Injection Prevention

### 3.1 Federation Input Validation

**Strong Validation Framework** in `internal/federation/validation.go`:

```go
const (
    MaxEmailLength = 254
    MaxGroupNameLength = 256
    MaxGroupCount = 100
    MaxExtraKeyLength = 256
    MaxExtraValueLength = 1024
    MaxExtraCount = 50
    MaxClusterNameLength = 253
)
```

Security controls include:
- Email format validation with regex
- Control character detection via `containsControlCharacters()`
- Path traversal prevention:

```go
if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
    return &ValidationError{
        Field:  "cluster name",
        Reason: "cluster name contains invalid path characters",
    }
}
```

- Kubernetes naming convention enforcement via regex
- HTTP header key validation (alphanumeric, hyphen, underscore only)

### 3.2 PII Protection in Logs

Email anonymization for logging prevents PII leakage:

```go
// internal/federation/validation.go
func AnonymizeEmail(email string) string {
    if email == "" {
        return ""
    }
    hash := sha256.Sum256([]byte(email))
    return "user:" + hex.EncodeToString(hash[:8])
}
```

### 3.3 Access Check Validation

The `ValidateAccessCheck` function validates Kubernetes API verbs against a known whitelist:

```go
var ValidKubernetesVerbs = map[string]bool{
    "get": true, "list": true, "watch": true,
    "create": true, "update": true, "patch": true,
    "delete": true, "deletecollection": true,
    "impersonate": true, "bind": true, "escalate": true,
    "*": true,
}
```

---

## 4. Secrets Management

### 4.1 Runtime Secret Masking

**Automatic Secret Redaction** in `internal/tools/output/secrets.go`:

```go
const RedactedValue = "***REDACTED***"

var secretTypes = map[string]bool{
    "kubernetes.io/service-account-token": true,
    "kubernetes.io/dockercfg":             true,
    "kubernetes.io/dockerconfigjson":      true,
    "kubernetes.io/basic-auth":            true,
    "kubernetes.io/ssh-auth":              true,
    "kubernetes.io/tls":                   true,
    "bootstrap.kubernetes.io/token":       true,
    "helm.sh/release.v1":                  true,
    "Opaque":                              true,
}
```

Features:
- Masks `data` and `stringData` fields in Secrets
- Masks sensitive annotations (service account tokens)
- Detects sensitive ConfigMaps by name pattern (`credentials`, `password`, `secret`, `auth`, `token`, `kubeconfig`)

### 4.2 Production Secret Management Documentation

The `docs/oauth.md` documentation strongly recommends external secret managers with detailed examples:
- HashiCorp Vault
- AWS Secrets Manager
- Google Cloud Secret Manager
- Azure Key Vault
- Kubernetes External Secrets Operator

Environment variables are explicitly marked as **NOT secure for production** with clear explanations.

### 4.3 Encryption Key Rotation

Documented rotation procedure with zero-downtime approach using dual-key support. Recommended rotation schedule: every 90 days.

---

## 5. Container & Deployment Security

### 5.1 Dockerfile

**Minimal Attack Surface**:

```dockerfile
FROM gsoci.azurecr.io/giantswarm/alpine:3.20.3-giantswarm
FROM scratch

COPY --from=0 /etc/passwd /etc/passwd
COPY --from=0 /etc/group /etc/group
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ADD mcp-kubernetes /
USER giantswarm

ENTRYPOINT ["/mcp-kubernetes"]
```

Security benefits:
- Distroless `scratch` base image (no shell, no package manager)
- Non-root user execution
- Only essential CA certificates copied
- Static binary with no runtime dependencies

### 5.2 Pod Security

**Strong Security Context** in Helm values:

```yaml
podSecurityContext:
  runAsUser: 1000
  runAsGroup: 1000
  runAsNonRoot: true
  fsGroup: 1000

securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  runAsUser: 1000
  runAsGroup: 1000
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault
  capabilities:
    drop:
    - ALL
```

This aligns with Kubernetes Pod Security Standards (Restricted profile).

### 5.3 Service Account Token Security

**Short-Lived Projected Tokens**:

```yaml
- name: sa-token
  projected:
    sources:
      - serviceAccountToken:
          expirationSeconds: 3600  # 1-hour auto-rotation
```

Default ServiceAccount token automounting disabled (`automountServiceAccountToken: false`), with explicit projected volume for controlled access.

---

## 6. Network Security

### 6.1 Security Headers

Comprehensive HTTP security headers in `internal/server/middleware/security.go`:

| Header | Value | Protection |
|--------|-------|------------|
| `X-Content-Type-Options` | `nosniff` | MIME sniffing |
| `X-Frame-Options` | `DENY` | Clickjacking |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains; preload` | HTTPS enforcement |
| `X-XSS-Protection` | `1; mode=block` | XSS attacks |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Referrer leakage |
| `Content-Security-Policy` | `default-src 'self'; frame-ancestors 'none'` | Content injection |
| `Permissions-Policy` | Disables geolocation, camera, etc. | Feature abuse |
| `Cross-Origin-Opener-Policy` | `same-origin` | Cross-origin isolation |
| `Cross-Origin-Embedder-Policy` | `require-corp` | Cross-origin resources |
| `Cross-Origin-Resource-Policy` | `same-origin` | Resource sharing |

### 6.2 CORS Validation

Strict origin validation in middleware:

```go
if u.Scheme != "http" && u.Scheme != "https" {
    return nil, fmt.Errorf("origin %q must use http or https scheme", origin)
}
if u.Path != "" && u.Path != "/" {
    return nil, fmt.Errorf("origin %q should not include path", origin)
}
```

### 6.3 HTTPS Enforcement

OAuth 2.1 HTTPS requirement enforced except for localhost development:

```go
func validateHTTPSRequirement(baseURL string) error {
    if u.Scheme == "http" {
        host := u.Hostname()
        if host != "localhost" && host != "127.0.0.1" && host != "::1" {
            return fmt.Errorf("OAuth 2.1 requires HTTPS for production (got: %s)", baseURL)
        }
    }
}
```

### 6.4 HTTP Server Timeouts

Appropriate timeouts configured to prevent slowloris and similar attacks:

```go
const (
    DefaultReadHeaderTimeout = 10 * time.Second
    DefaultWriteTimeout      = 120 * time.Second  // Increased for long MCP operations
    DefaultIdleTimeout       = 120 * time.Second
)
```

---

## 7. Identified Vulnerabilities & Recommendations

### 7.1 High Priority

| ID | Issue | Risk | Recommendation |
|----|-------|------|----------------|
| SEC-001 | Debug mode hardcoded in Helm | Information Disclosure | Make configurable with default `false` |
| SEC-002 | Standard RBAC profile default | Excessive Privileges | Consider defaulting to `readonly` |
| SEC-003 | In-memory token storage default | Session Loss/Scalability | Document Valkey requirement more prominently, consider validation |

### 7.2 Medium Priority

| ID | Issue | Risk | Recommendation |
|----|-------|------|----------------|
| SEC-004 | No request size limits visible | DoS | Add `--max-request-size` configuration |
| SEC-005 | Token cache timing | Stale Tokens | Add validation that cache TTL <= token lifetime |
| SEC-006 | Limited command validation in exec | Command Injection | Consider command allowlisting option for high-security environments |

### 7.3 Low Priority / Hardening

| ID | Issue | Recommendation |
|----|-------|----------------|
| SEC-007 | gosec annotations | Review all `#nosec` comments periodically |
| SEC-008 | Dependency updates | Automate security scanning in CI (Renovate already configured) |
| SEC-009 | Network policies | Add default CiliumNetworkPolicy in Helm chart |
| SEC-010 | Metrics endpoint exposure | Ensure `/metrics` is not exposed publicly (currently handled correctly) |

---

## 8. Static Analysis Coverage

The codebase uses:
- `gosec` via golangci-lint (configured in `Makefile.gen.go.mk`)
- `gitleaks` for secret scanning (pre-commit hook)
- Pre-commit hooks configured in `.pre-commit-config.yaml`

Legitimate security suppressions are documented:

```go
// #nosec G304 -- caFile is a configuration value from operator, not user input
caCert, err := os.ReadFile(caFile)
```

All `#nosec` annotations reviewed appear to be legitimate with appropriate justification comments.

---

## 9. Dependency Analysis

### 9.1 Key Security Dependencies

| Dependency | Version | Purpose | Notes |
|------------|---------|---------|-------|
| `mcp-oauth` | v0.2.40 | OAuth 2.1 server | Giant Swarm maintained |
| `k8s.io/client-go` | v0.35.0 | Kubernetes client | Well-maintained |
| `golang.org/x/oauth2` | v0.34.0 | OAuth client | Google maintained |
| `golang-jwt/jwt/v5` | v5.3.0 | JWT handling | Indirect dependency |

### 9.2 Recommended Actions

1. Enable Dependabot or Renovate security alerts (Renovate appears configured)
2. Run `govulncheck` in CI pipeline
3. Consider SBOM generation for releases

---

## 10. Compliance Considerations

### 10.1 OAuth Standards Compliance

| Standard | Status |
|----------|--------|
| OAuth 2.1 with PKCE (RFC 6749, RFC 7636) | Compliant |
| Token Introspection (RFC 7662) | Compliant |
| Token Revocation (RFC 7009) | Compliant |
| Authorization Server Metadata (RFC 8414) | Compliant |
| Protected Resource Metadata (RFC 9728) | Compliant |
| Dynamic Client Registration (RFC 7591) | Compliant |

### 10.2 Kubernetes Security Standards

| Standard | Status |
|----------|--------|
| Kubernetes RBAC | Properly implemented |
| Pod Security Standards (Restricted) | Aligned |
| Service Account Token Projection | Implemented |
| User Impersonation | Properly implemented |

---

## 11. Incident Response Documentation

The `docs/oauth.md` includes comprehensive incident response procedures for:

1. **Secrets Leaked in Logs/Monitoring** - 1-hour response timeline
2. **Secrets in Git History** - 1-hour response with git-filter-repo instructions
3. **Secrets in Container Images** - Image purging and scanning procedures
4. **Secrets in CI/CD Logs** - Platform-specific guidance

This level of incident response documentation is commendable and exceeds typical open-source project standards.

---

## 12. Conclusion

**mcp-kubernetes demonstrates mature security engineering** with:

1. **Strong authentication** via OAuth 2.1 with PKCE enforcement
2. **Layered authorization** combining OAuth, RBAC profiles, and non-destructive mode
3. **Comprehensive input validation** preventing injection and traversal attacks
4. **Production-grade container security** with minimal attack surface
5. **Excellent documentation** of security considerations and incident response

### Immediate Actions Required

1. Fix hardcoded debug mode in Helm template (SEC-001)
2. Add runtime validation warning for in-memory token storage in production (SEC-003)

### Recommended Improvements

1. Consider more restrictive RBAC default (SEC-002)
2. Add request size limits (SEC-004)
3. Add default network policies to Helm chart (SEC-009)

### Overall Assessment

The project is **suitable for production use in security-conscious environments** when deployed with the documented best practices. The security posture reflects thoughtful design decisions and a defense-in-depth approach that is rare in open-source MCP implementations.

---

*Assessment performed by Claude Opus 4.5 (Anthropic) via automated code review.*
