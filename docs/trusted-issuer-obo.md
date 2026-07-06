# Trusted-issuer on-behalf-of (OBO) identity model

mcp-kubernetes accepts tokens minted by an external broker (muster in the Giant
Swarm platform) on the `trustedIssuers` configuration block. The invariant is a
validated human subject, not a present actor: the token must resolve a human
subject (an `email` claim, via `subjectClaim`, matching `allowedClaims.email`),
and it is then impersonated. An `act` claim is validated if present but is not
required.

Two caller shapes are accepted:

- **On-behalf-of (OBO)**: an agent acting for a human. The token carries the
  human subject plus an `act` claim identifying the acting agent ServiceAccount.
- **Human-direct**: a human reaching kubernetes tools through muster (Backstage,
  Claude Code). The token carries the human subject and no `act` claim.

A token with no resolvable human subject (a bare machine or ServiceAccount token,
which has no `email` claim) is rejected.

## Roles in the model

| Component | Role |
|-----------|------|
| **muster** | Mints a short-lived JWT carrying the human subject, and (for OBO) an `act` claim identifying the acting agent ServiceAccount. |
| **mcp-kubernetes** | Maps the human subject → `Impersonate-User`. Requires a validated human subject; validates the `act` claim if present but does not require it. The impersonated human's Kubernetes RBAC governs access. |
| **rbac-operator / CRB** | Binds the impersonated human to a ClusterRole on the target cluster. All Kubernetes access decisions stay in Kubernetes RBAC. |

mcp-kubernetes never interprets the token's audience or constructs a synthetic
identity from the subject. The human subject arrives from the broker; the actor,
when present, is logged but is not projected into an impersonation header.

## Token shape and processing

muster mints a token with:

- `sub` = the human subject; with `subjectClaim: email` the impersonated value is
  the `email` claim (e.g. `alice@giantswarm.io`)
- `act.sub` = the acting agent ServiceAccount (RFC 8693 §4.4 delegation chain),
  present on the OBO path and absent on the human-direct path
- `groups` = the human's groups (carried from the upstream IdP)
- `aud` = muster's resource identifier (forwarded unchanged to the backend)
- `typ: at+jwt` (RFC 9068)

mcp-kubernetes, on receiving the token:

1. Validates the JWT signature against the trusted issuer's JWKS.
2. Selects the matching `trustedIssuers` entry by subject pattern
   (`allowedClaims`, keyed on `subjectClaim` when set).
3. Requires a validated human subject (an `email` claim). A token with no human
   subject is rejected. If an `act` claim is present it is validated (muster
   validates the actor at exchange time); its absence is the human-direct path,
   not an error.
4. Sets `Impersonate-User` (the human) and `Impersonate-Group`
   (`system:authenticated`) on every downstream Kubernetes API call. No
   `Impersonate-Extra-*` headers are sent.

A token with no resolvable human subject is rejected with 403: a bare machine or
ServiceAccount identity (no `email` claim) has no human to impersonate.

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
