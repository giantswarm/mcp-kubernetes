# Security Assessment: mcp-kubernetes

**Assessment Date:** January 19, 2026  
**Assessor:** GPT-5.2-Codex (OpenAI)  
**Repository:** github.com/giantswarm/mcp-kubernetes  
**Scope:** Static review of repository source and Helm chart defaults  
**Method:** Read-only code and documentation review. No runtime testing or external dependency audit.

---

## Executive Summary

This repository provides an MCP server for Kubernetes with strong safety defaults in code
(non-destructive mode, secret masking, strict downstream OAuth). However, the Helm chart
defaults include a production-unsafe debug flag and an RBAC profile that grants
cluster-wide Secret access by default. Additionally, the non-OAuth HTTP transport has no
authentication in code, so deployments must ensure it is not exposed beyond trusted
networks or must enable OAuth.

## Findings

### High Severity

1) Helm chart always enables debug logging
- Impact: Debug logs can expose sensitive operational details and increase leak risk.
- Evidence: `helm/mcp-kubernetes/templates/deployment.yaml` passes `--debug=true`.
- Recommendation: Make debug configurable with a secure default (`false`).

2) Default Helm RBAC profile grants cluster-wide Secret access
- Impact: ServiceAccount mode grants full Secret access to all users.
- Evidence: `helm/mcp-kubernetes/values.yaml` sets `rbac.profile: "standard"`.
- Recommendation: Default to `readonly` or `minimal` based on deployment mode.

3) Non-OAuth HTTP transport has no authentication
- Impact: If exposed beyond a trusted network, unauthenticated Kubernetes operations are
  possible.
- Evidence: `cmd/serve_http.go` registers the HTTP handler without auth middleware.
- Recommendation: Require OAuth for HTTP transport by default in production deployments
  and document the trust boundary explicitly.

### Medium Severity

4) Token encryption at rest is optional
- Impact: OAuth tokens may be stored unencrypted if key is not provided.
- Evidence: `cmd/serve.go` warns but allows startup without encryption key.
- Recommendation: Enforce encryption key for production OAuth deployments.

## Strengths and Existing Controls

- Non-destructive mode defaults to true, limiting mutating operations.
- Output processing masks Secret data by default to reduce accidental leakage.
- Downstream OAuth strict mode is fail-closed (no service account fallback).
- OAuth HTTP transport adds security headers and CORS validation.
- SSRF protections and URL validation are present for OAuth and CIMD use cases.

## Deployment Guidance (Security Baseline)

- Enable OAuth for any network-accessible HTTP transport.
- Use downstream OAuth for per-user RBAC enforcement.
- Set `rbac.profile: "minimal"` when downstream OAuth is enabled.
- Store secrets in a dedicated secret manager (avoid environment variables in production).
- Provide `OAUTH_ENCRYPTION_KEY` and enable Valkey storage for multi-replica OAuth.
- Disable debug logging in production.
- Keep non-destructive mode enabled; use dry-run for validation.

## Scope Limitations

- No dynamic testing, penetration testing, or threat modeling was performed.
- No dependency vulnerability audit (e.g., govulncheck) executed.
- No environment-specific configuration review (cluster, ingress, or network policy).

## References (Code Paths)

- OAuth server configuration: `internal/server/oauth_http.go`
- OAuth flags and validation: `cmd/serve.go`, `cmd/serve_config.go`
- Non-destructive mode: `internal/tools/safety.go`
- Secret masking: `internal/tools/output/secrets.go`
- Helm defaults: `helm/mcp-kubernetes/values.yaml`, `helm/mcp-kubernetes/templates/deployment.yaml`
