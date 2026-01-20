# Single Sign-On (SSO) via Token Forwarding

This document describes how to configure mcp-kubernetes to accept tokens forwarded from trusted upstream aggregators, enabling Single Sign-On (SSO) across the MCP ecosystem.

## Background

When users connect to mcp-kubernetes through an aggregator like [muster](https://github.com/giantswarm/muster), they typically need to authenticate separately:

1. First, to the aggregator (e.g., muster)
2. Then, to each downstream MCP server (e.g., mcp-kubernetes)

This creates friction because both services often use the same Identity Provider (e.g., Dex). The user has already proven their identity to the aggregator - downstream servers should be able to accept that proof.

## How Token Forwarding Works (mcp-oauth v0.2.39)

The SSO token forwarding implementation uses JWKS-based JWT validation, which correctly handles ID token forwarding scenarios where the previous userinfo-based validation failed.

```
User ─── authenticates ───> Aggregator (muster)
                               │
                          forwards ID token
                               │
                               ▼
                          mcp-kubernetes
                               │
                               │ validates token via JWKS:
                               │ 1. Detect JWT format (3 dot-separated parts)
                               │ 2. Fetch JWKS from IdP (Google/Dex)
                               │ 3. Validate signature using public key
                               │ 4. Validate issuer matches provider
                               │ 5. Check audience in TrustedAudiences
                               │ 6. Extract user info from JWT claims
                               ▼
                          Access granted
```

**Key Improvement (v0.2.39):** The previous implementation called the IdP's userinfo endpoint, which fails for ID tokens (userinfo expects access tokens). The new implementation validates ID tokens directly using JWKS signature verification.

## Configuration

### CLI Flags

```bash
mcp-kubernetes serve \
  --enable-oauth \
  --oauth-provider dex \
  --dex-issuer-url https://dex.example.com \
  --dex-client-id mcp-kubernetes-client \
  --dex-client-secret <secret> \
  --oauth-trusted-audiences "muster-client,another-aggregator"
```

### Environment Variables

```bash
# Accept tokens issued to these client IDs (comma-separated)
export OAUTH_TRUSTED_AUDIENCES="muster-client,another-aggregator"
```

### Helm Values

```yaml
mcpKubernetes:
  oauth:
    enabled: true
    provider: dex
    dex:
      issuerUrl: "https://dex.example.com"
      clientId: "mcp-kubernetes-client"
      clientSecret: "<secret>"
    # Trust tokens forwarded from these upstream services
    trustedAudiences:
      - "muster-client"
      - "another-aggregator"
```

## Security Considerations

### Explicit Trust Model

- **No implicit trust**: Only client IDs explicitly listed in `trustedAudiences` are accepted
- **Same issuer requirement**: Tokens must be issued by the configured OAuth provider (Dex/Google)
- **Cryptographic verification**: The IdP's signature proves token authenticity

### What This Does NOT Do

- Does NOT bypass token signature validation
- Does NOT allow tokens from different issuers
- Does NOT grant any additional permissions beyond what RBAC allows

### Audit Trail

When mcp-kubernetes accepts a cross-client token, it logs an audit event:

```
level=INFO msg="SSO: accepting cross-client token" 
  audience="muster-client" 
  user_email_hash="abc123..." 
  issuer="https://dex.example.com"
```

### Recommended Practices

1. **Minimize trusted audiences**: Only add client IDs you explicitly trust
2. **Use the same issuer**: Ensure aggregators use the same Dex/Google instance
3. **Monitor audit logs**: Watch for unexpected cross-client token usage
4. **Enable downstream OAuth**: Use `--downstream-oauth` to ensure users only get their own RBAC permissions

## Example: muster Integration

When deploying mcp-kubernetes as a downstream server behind muster:

```yaml
# muster configuration
aggregatedServers:
  - name: mcp-kubernetes
    transport: streamable-http
    url: https://mcp-kubernetes.internal.example.com/mcp
    forwardToken: true  # Forward user's ID token

# mcp-kubernetes configuration
mcpKubernetes:
  oauth:
    enabled: true
    provider: dex
    dex:
      issuerUrl: "https://dex.example.com"
      clientId: "mcp-kubernetes-client"
    trustedAudiences:
      - "muster-client"  # Accept tokens from muster
    enableDownstreamOAuth: true  # Use tokens for K8s API auth
```

## Downstream OAuth with SSO Token Forwarding

When `enableDownstreamOAuth: true` is configured alongside `trustedAudiences`, mcp-kubernetes automatically uses SSO-forwarded tokens for Kubernetes API authentication:

1. Muster forwards the user's ID token to mcp-kubernetes
2. mcp-kubernetes validates the token via JWKS (TrustedAudiences)
3. mcp-kubernetes uses the forwarded ID token directly for Kubernetes API calls
4. The user gets their own RBAC permissions in the cluster

This works because for SSO-forwarded tokens, the Bearer token **is** the ID token. mcp-kubernetes detects this via the `TokenSource` metadata set during token validation (requires mcp-oauth v0.2.43+).

### Kubernetes API Server OIDC Configuration

**Important:** When using SSO-forwarded tokens for Kubernetes API authentication, the token's `aud` (audience) claim contains the upstream service's client ID (e.g., `muster-client`), not the Kubernetes API server's expected audience.

You must configure the Kubernetes API server to accept tokens with the upstream service's audience:

```yaml
# kube-apiserver configuration
--oidc-issuer-url=https://dex.example.com
--oidc-client-id=muster-client  # Use the upstream aggregator's client ID
--oidc-username-claim=email
--oidc-groups-claim=groups
```

Alternatively, if your Kubernetes cluster supports multiple audiences (Kubernetes 1.29+), you can use:

```yaml
--oidc-issuer-url=https://dex.example.com
--oidc-client-id=kubernetes  # Primary audience
--oidc-username-claim=email
--oidc-groups-claim=groups
# Additional audiences accepted for authentication
--api-audiences=kubernetes,muster-client,another-aggregator
```

**Note:** If you're using Dex with `DexKubernetesAuthenticatorClientID` configured for normal OAuth flow, ensure the same client ID is used consistently across your SSO token forwarding setup.

## Troubleshooting

### Token Not Accepted

If tokens from an aggregator are not being accepted:

1. **Check the audience**: Verify the token's `aud` claim matches an entry in `trustedAudiences`
2. **Check the issuer**: Ensure both services use the same Dex/Google issuer URL
3. **Enable debug logging**: Use `--debug` to see detailed token validation logs
4. **Check token expiry**: Ensure the forwarded token hasn't expired

### JWKS Fetching Fails (SSRF Error)

If you see errors like "JWKS URI must not point to private IP ranges" when using TrustedAudiences:

1. **Check your IdP location**: If Dex/your IdP is on a private network (10.x, 172.16.x, 192.168.x), you need to enable private IP allowance
2. **Enable SSO private IPs**: Set `--sso-allow-private-ips=true` or `SSO_ALLOW_PRIVATE_IPS=true` (or `sso.allowPrivateIPs: true` in Helm values)
3. **Security note**: Only enable this for internal/VPN deployments not exposed to the public internet

```bash
# CLI flag
mcp-kubernetes serve \
  --enable-oauth \
  --oauth-trusted-audiences "muster-client" \
  --sso-allow-private-ips
```

```yaml
# Helm values
mcpKubernetes:
  oauth:
    trustedAudiences:
      - "muster-client"
    sso:
      # Enable for private Dex on internal networks (10.x, 172.16.x, 192.168.x)
      allowPrivateIPs: true
```

**Warning:** Enabling `sso.allowPrivateIPs` reduces SSRF protection. Only use for:
- Home lab deployments
- Air-gapped environments  
- Internal enterprise networks

**Note:** For Google OAuth, this setting has no effect because Google's JWKS endpoint (`https://www.googleapis.com/oauth2/v3/certs`) is always publicly accessible. This option is primarily for private Dex deployments.

**Availability:** This feature requires mcp-oauth v0.2.40 or later.

### Debug Logging

```bash
mcp-kubernetes serve --debug \
  --oauth-trusted-audiences "muster-client"
```

Look for log entries like:
```
level=DEBUG msg="Validating token audience" 
  token_audience="muster-client" 
  trusted_audiences=["muster-client"]
```

## Related Documentation

- [OAuth Configuration](oauth.md) - Full OAuth setup guide
- [Downstream OAuth](oauth.md#downstream-oauth) - Using OAuth tokens for Kubernetes API auth
- muster documentation - ADR-009: Single Sign-On via Token Forwarding
