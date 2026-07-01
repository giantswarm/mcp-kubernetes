# Trusted-issuer on-behalf-of (OBO) identity model

mcp-kubernetes accepts tokens minted by an external broker (muster in the Giant
Swarm platform) on the `trustedIssuers` configuration block. The only supported
flow is on-behalf-of (OBO): an agent acting for an authenticated human. A token
with no actor (`act`) claim is rejected.

## Roles in the model

| Component | Role |
|-----------|------|
| **muster** | Mints a short-lived JWT carrying the human subject and an `act` claim identifying the acting agent ServiceAccount. |
| **mcp-kubernetes** | Maps the human `sub` → `Impersonate-User`. Accepts any validated trusted-issuer actor; the impersonated human's Kubernetes RBAC governs access. |
| **rbac-operator / CRB** | Binds the impersonated human to a ClusterRole on the target cluster. All Kubernetes access decisions stay in Kubernetes RBAC. |

mcp-kubernetes never interprets the token's audience or constructs a synthetic
identity from the subject. The human subject arrives from the broker; the actor
is validated to authorize the request but is not projected into an impersonation
header.

## OBO (agent acting for a human)

muster mints a token with:

- `sub` = the human subject (e.g. `alice@giantswarm.io`)
- `act.sub` = the acting agent ServiceAccount (RFC 8693 §4.4 delegation chain)
- `groups` = the human's groups (carried from the upstream IdP)
- `aud` = the backend audience (e.g. `mcp-kubernetes`)
- `typ: at+jwt` (RFC 9068)

mcp-kubernetes, on receiving the token:

1. Validates the JWT signature against the trusted issuer's JWKS.
2. Selects the matching `trustedIssuers` entry by subject pattern
   (`allowedClaims`, keyed on `subjectClaim` when set).
3. Accepts any actor validated against the trusted issuer's JWKS; a token with no
   `act` claim is rejected.
4. Sets `Impersonate-User` (the human) and `Impersonate-Group`
   (`system:authenticated`) on every downstream Kubernetes API call. No
   `Impersonate-Extra-*` headers are sent.

A token with no `act` claim is rejected with 403: there is no delegation to
impersonate, and a bare machine identity is not accepted.

## Configuration

```yaml
mcpKubernetes:
  oauth:
    trustedIssuers:
      - issuer: "https://muster.example.com"
        jwksURL: "https://muster.example.com/.well-known/jwks.json"
        subjectClaim: "email"            # Dex sub is opaque; route/impersonate on email
        allowedClaims:
          email: "*@giantswarm.io"
        allowedTargetClusters: ["cluster-a"]   # optional
```

One entry per issuer URL; a duplicate issuer URL is rejected at startup.

## Required Kubernetes RBAC

The chart provisions a single `*-obo-impersonate` ClusterRole binding the
mcp-kubernetes ServiceAccount to:

- `impersonate users` (unrestricted; any validated actor may act for the human,
  whose own Kubernetes RBAC governs access)
- `impersonate groups` for `system:authenticated`

Only `Impersonate-User` and `Impersonate-Group` are sent, so no
`impersonate userextras/*` grant is required.

Workload-cluster bindings that grant the impersonated human actual permissions
(e.g. a `read-all` ClusterRole) are provisioned per-cluster via shared-configs,
not by this chart.
