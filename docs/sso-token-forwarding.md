# Single sign-on and resource access tokens

MCP requests use access tokens issued for mcp-kubernetes. An OIDC ID token proves a sign-in to its OIDC client; mcp-kubernetes rejects it as a resource bearer, including when its signature and audience are valid. Adding an aggregator client ID to `trustedAudiences` does not change that requirement.

With muster, register the server for per-server OAuth:

```yaml
apiVersion: muster.giantswarm.io/v1alpha1
kind: MCPServer
metadata:
  name: mcp-kubernetes
spec:
  type: streamable-http
  url: https://mcp-kubernetes.example.com/mcp
  auth:
    type: oauth
    forwardToken: false
```

Muster discovers the protected resource and authorization server metadata, uses its Client ID Metadata Document, and starts an authorization-code flow with S256 PKCE. The person approves client consent before the identity provider is contacted. An existing identity-provider browser session can avoid another password prompt. Muster exchanges the code for mcp-kubernetes's access token and uses it only with that server.

For Kubernetes user RBAC, `enableDownstreamOAuth` uses the separately validated upstream OIDC identity for the kube-apiserver. Configure the Kubernetes audience and the identity provider's client trust accordingly. The identity token is not the bearer on the MCP endpoint.

OAuth state must be shared across replicas. Use encrypted Valkey storage with an existing OAuth Secret; the chart rejects multi-replica OAuth with memory storage. Consent uses that store automatically without another key or an enable flag.

For an internal deployment, `--cimd-private-ip-hosts` and `--private-ip-redirect-uri-hosts` name exact hosts allowed to resolve to private addresses. Their Helm values are `mcpKubernetes.oauth.cimd.privateIPHosts` and `mcpKubernetes.oauth.redirectURISecurity.privateIPHosts`, both empty by default. They scope network access, not client trust. `DEX_CA_FILE` also supplies the CIMD fetcher's trust roots. TLS verification stays enabled.

These boundaries follow the [MCP authorization specification](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization) and its [token passthrough protections](https://modelcontextprotocol.io/docs/2026-07-28/tutorials/security/security_best_practices#token-passthrough).
