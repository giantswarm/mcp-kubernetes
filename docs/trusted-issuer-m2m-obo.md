# Trusted-issuer M2M and OBO identity model

mcp-kubernetes supports two identity flows for tokens minted by an external broker
(muster in the GiantSwarm platform): M2M (machine-to-machine, headless agents) and
OBO (on-behalf-of, agents acting for an authenticated human). Both flows use the
`trustedIssuers` configuration block.

## Roles in the model

| Component | Role |
|-----------|------|
| **muster** | Single authority for the Kubernetes identity an agent acts as. Validates the agent's SA token via `WithTrustedIssuers`, mints a short-lived JWT carrying the downstream K8s user and groups. |
| **mcp-kubernetes** | Pure projector. Maps the minted `sub` → `Impersonate-User` and minted `groups` → `Impersonate-Group`. Decides nothing about authorization; enforces exact allow-lists (`impersonateUser`, `impersonateGroups`). |
| **rbac-operator / CRB** | Binds the minted group to a ClusterRole (e.g. `read-all`) on the target cluster. All K8s access decisions stay in Kubernetes RBAC. |

mcp-kubernetes never interprets the token's audience or constructs a synthetic
namespace-based SA identity from the subject. The minted user and groups arrive
verbatim from the broker.

## M2M (machine identity, headless agents)

muster mints a token with:

- `sub` = the impersonated user (e.g. `agent:sre`), set by the `WorkloadGroupGrant`
  `granted.subject` field
- `groups` = the impersonated groups (e.g. `["agent:sre"]`), set by `granted.groups`
- no `act` claim (pure machine identity, no delegation chain)
- `aud` = the backend audience (e.g. `mcp-kubernetes`)
- `typ: at+jwt` (RFC 9068)

mcp-kubernetes, on receiving the token:

1. Validates the JWT signature against the trusted issuer's JWKS.
2. Checks `allowedClaims.sub` pattern against the remapped subject.
3. Asserts the minted subject equals `impersonateUser` (exact match; no wildcard).
4. Intersects the minted `groups` with `impersonateGroups`; rejects if the
   intersection is empty (403).
5. Sets `Impersonate-User` and `Impersonate-Group` headers on every downstream
   Kubernetes API call.

The intersection at step 4 is the MCP-level allow-list. Kubernetes RBAC provides
the second gate: the group must be bound to the required ClusterRole on the target
cluster.

### Required K8s RBAC

The minted group must be bound to a ClusterRole on each target cluster:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: agent-sre-read-all
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: read-all          # GiantSwarm standard read-only ClusterRole
subjects:
  - kind: Group
    name: agent:sre
    apiGroup: rbac.authorization.k8s.io
```

At GiantSwarm this binding is owned by rbac-operator; do not hand-write it.

### Configuration (trustedIssuers)

```yaml
mcpKubernetes:
  oauth:
    trustedIssuers:
      - issuer: "https://muster.example.io"
        jwksURL: "https://muster.example.io/.well-known/jwks.json"
        impersonateUser: agent:sre
        impersonateGroups:
          - agent:sre
        allowedClaims:
          sub: agent:sre          # exact match; use globs for multiple agents
        acceptedTypHeaders:
          - "at+jwt"
```

When the muster JWKS endpoint resolves to a private IP (in-cluster load balancer),
add `allowPrivateIPJWKS: true` on that issuer entry. This is an SSRF downgrade
scoped to the pinned issuer; prefer host-scoped JWKS allowlist when available.

### Why group impersonation, not SA namespace impersonation

The earlier `kagent-glean` model impersonated
`system:serviceaccount:<alias-namespace>:<agent-name>` and required a hand-written
Role + RoleBinding per alias namespace. That approach:

- required bespoke RBAC per agent alias rather than using the standard rbac-operator
  bindings that govern human access;
- coupled the agent's K8s identity to the MCP deployment namespace, creating
  per-install drift;
- did not compose with GiantSwarm's group-based RBAC model.

Group impersonation via a dedicated agent group (`agent:sre`) resolves to the same
string on every cluster regardless of connector prefix (both `giantswarm-ad` and
`giantswarm-github` connectors exist on every MC). rbac-operator binds that group
to `read-all` using the same mechanism it uses for human groups.

## OBO (on-behalf-of, human-triggered via A2A)

muster mints a delegated token with:

- `sub` = the human's email (remapped from the Dex ID token via `subjectClaim: email`)
- `act.sub` = the agent SA (`system:serviceaccount:kagent:sre-agent`)
- `aud` = the backend audience
- `typ: at+jwt`

mcp-kubernetes, on receiving an OBO token (`act` claim present):

1. Validates the JWT and the actor against `allowedActors[].sub`. The check walks
   the full RFC 8693 chain (`act.act.sub`, etc.) so multi-hop A2A is honored.
2. If `matchedActor.allowedSubjects` is set, checks the human subject (`sub`) matches
   one of the patterns.
3. Sets `Impersonate-User` to the human's email. No groups are carried in the current
   implementation (OBO group-carrying is a follow-up; human K8s RBAC at GiantSwarm
   uses per-user subject bindings for now).

### Configuration (trustedIssuers, OBO entry)

The OBO entry requires a distinct `alias` so the chart generates a separate RBAC
grant covering the unrestricted `impersonate users` verb:

```yaml
mcpKubernetes:
  oauth:
    trustedIssuers:
      # M2M entry above ...
      - issuer: "https://muster.example.io"
        jwksURL: "https://muster.example.io/.well-known/jwks.json"
        alias: "muster-obo"
        allowedClaims:
          sub: "*@example.io"
        acceptedTypHeaders:
          - "at+jwt"
        allowedActors:
          - sub: "system:serviceaccount:kagent:sre-agent"
            allowedSubjects:
              - "*@example.io"
```

The `alias` field causes the chart to emit a `ClusterRole`/`ClusterRoleBinding`
granting `impersonate users` (unrestricted) for this issuer. Per-actor subject
enforcement runs in-process; Kubernetes RBAC cannot couple impersonation verbs to
extras.

## mcp-kubernetes SA RBAC (own pod)

The mcp-kubernetes `ServiceAccount` needs `impersonate` rights on both `users` and
`groups` resources scoped to the identities it projects. The chart emits these grants
automatically from the `trustedIssuers` entries; do not manage them manually.

For M2M, the chart creates a `ClusterRole` allowing impersonation of the configured
`impersonateUser` and each `impersonateGroups` entry via `resourceNames`. For OBO
(`alias` present), it grants unrestricted `impersonate users`.

## Authority model and cloud-agnosticism

muster owns the identity assertion: it validates the agent's K8s SA token via
`WithTrustedIssuers` (per-cluster OIDC issuer; only the issuer URL differs between
Azure Workload Identity and IRSA) and emits from a `workloadGroupGrant` keyed on
explicit issuer + subject. The grant is the auditable, deprovisionable declaration
that this agent serves this tenant; no mint-time external call means no fail-open path.

The agent group (`agent:sre`) is a single deterministic string on every cluster,
independent of the Dex connector prefix (both `giantswarm-ad` and `giantswarm-github`
connectors exist on every management cluster). Human OBO groups still arrive
connector-prefixed from Dex; only the autonomous M2M agent uses the connector-free group.

This model was chosen over Azure AD group-gating and Keycloak client-credentials:
both require per-cloud adapter layers that add complexity without removing the need
for backend group bindings, and neither applies to the majority of the fleet running
on AWS/IRSA. See `subplans/14-external-workload-identity.md` for the full evaluation.

SPIFFE/JWT-SVIDs are the intended successor for the SA token source when SPIRE is
available fleet-wide: mcp-oauth's `JWTSVIDValidator` accepts SVIDs as `subject_token`,
the grant becomes `spiffe://…/sa/sre-agent -> group`, and everything downstream
(mck8s projector, rbac-operator binding) is unchanged.

## Non-regression: SSO + IRSA paths

The trusted-issuer path activates only when the Bearer token's issuer matches a
`trustedIssuers` entry (`userInfo.IsExternalIssuer()`). Tokens validated via
`trustedAudiences` (Dex SSO forwarded by muster) or IRSA/SA-direct paths are
unaffected; they continue through the existing SSO forwarding or in-cluster auth
paths.
