# Security Assessment: mcp-kubernetes

**Assessment Date:** January 19, 2026  
**Assessor:** Gemini 3 Pro (Google)  
**Repository:** github.com/giantswarm/mcp-kubernetes  
**Scope:** Security architecture review with focus on identity propagation and multi-cluster federation

---

## Executive Summary

**Overall Security Posture: Strong**

The `mcp-kubernetes` library demonstrates a mature security architecture with a "secure by default" philosophy. It implements robust defense-in-depth mechanisms, particularly for multi-cluster federation and identity propagation. The codebase shows clear evidence of security-conscious engineering, including automatic secret redaction, strict middleware policies, and immutable audit trails.

## Detailed Findings

### 1. Identity & Access Management (IAM)

*   **Federation & Impersonation (High Security):**
    *   **Architecture**: The library uses a "defense in depth" approach for accessing Workload Clusters (WC) from a Management Cluster (MC).
    *   **Mechanism**: Accessing a WC requires two distinct permissions:
        1.  **MC RBAC**: The user must have permission on the Management Cluster to read the `Secret` containing the WC's kubeconfig.
        2.  **WC RBAC**: The library configures the client to use Kubernetes Impersonation (`Impersonate-User`, `Impersonate-Group`). This means the operation on the WC is performed under the *user's* identity, not the admin credentials found in the kubeconfig.
    *   **Verification**: The code explicitly panics if user info is missing during client creation, preventing accidental privilege escalation to `system:admin`.

*   **Audit Trails**:
    *   **Immutability**: The library injects an `Impersonate-Extra-agent: mcp-kubernetes` header into requests. This header is added *after* user-supplied headers, making it immutable. This ensures that all actions performed via this tool are clearly traceable in Kubernetes audit logs, and users cannot spoof this origin.

*   **OAuth 2.1**:
    *   The implementation enforces strict checking by default (`WithDownstreamOAuthStrict`). If an OAuth token is missing, the request fails closed rather than silently falling back to a service account.

### 2. Operational Security

*   **Non-Destructive Mode**:
    *   By default, the server runs in `NonDestructiveMode`. Mutating operations (create, patch, delete, exec, port-forward) are blocked unless explicitly allowed or if `DryRun` is enabled.
    *   **Recommendation**: In production, rely on this default and only whitelist specific operations (e.g., `k8s_create_resource`) if absolutely necessary.

*   **Pod Execution (`k8s_exec_pod`)**:
    *   **Risk**: This tool allows arbitrary command execution within pods.
    *   **Control**: It is treated as a mutating operation and blocked by `NonDestructiveMode`.
    *   **Observation**: While the library records internal metrics for these operations, the actual *command executed* is not logged by the MCP server itself (it relies on Kubernetes audit logs).
    *   **Recommendation**: Ensure Kubernetes audit logging is enabled at the cluster level to capture the actual commands executed via this tool.

*   **Secret Redaction**:
    *   The library includes a robust `MaskSecrets` function that recursively scans output. It replaces `data` and `stringData` in Secrets, as well as sensitive annotations, with `***REDACTED***`. It also proactively masks ConfigMaps that match sensitive naming patterns (e.g., "password", "token").

### 3. Network Security

*   **Middleware**:
    *   The server applies a comprehensive suite of security headers: `Content-Security-Policy` (CSP), `HSTS`, `X-Frame-Options`, and `Permissions-Policy`. This protects against common web vulnerabilities (XSS, Clickjacking).

*   **SSRF Protection**:
    *   URL validation logic (`validateSecureURL`) blocks access to private IP ranges (RFC 1918) and loopback addresses by default.
    *   **Risk**: Configuration options like `SSOAllowPrivateIPs` and `CIMDAllowPrivateIPs` exist to disable these checks.
    *   **Recommendation**: strictly avoid enabling `*AllowPrivateIPs` flags in production environments exposed to the internet.

### 4. Supply Chain & Code Quality

*   **CI/CD**: The repository includes automated workflows for `gitleaks` (secret scanning), `ossf-scorecard`, and vulnerability fixing, indicating a proactive approach to supply chain security.
*   **Dependencies**: The project relies on standard, well-maintained libraries (`k8s.io/client-go`, `mark3labs/mcp-go`).

## Conclusion & Recommendations

The `mcp-kubernetes` library is well-designed for security-critical environments. Its handling of identity propagation via impersonation is a standout feature that allows it to integrate safely into existing RBAC structures without creating a "super-admin" bottleneck.

**Actionable Recommendations for Deployment:**
1.  **Keep Non-Destructive Mode Enabled**: Only disable it for specific, audited use cases.
2.  **Enable Kubernetes Audit Logs**: Rely on K8s audit logs to track the specific commands executed via `k8s_exec_pod` and `Impersonate-User` headers.
3.  **Avoid Private IP Allowlisting**: Do not enable `SSOAllowPrivateIPs` or `CIMDAllowPrivateIPs` unless the deployment is strictly internal and air-gapped.
4.  **Monitor "Exec" Usage**: Treat usage of the `k8s_exec_pod` tool as a high-risk event in your SIEM/monitoring systems.
