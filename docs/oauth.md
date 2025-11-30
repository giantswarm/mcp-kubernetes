# OAuth 2.1 Authentication for MCP Kubernetes Server

The MCP Kubernetes server supports OAuth 2.1 authentication for HTTP transports (streamable-http). This provides secure, token-based authentication for accessing the Kubernetes MCP tools.

## Features

- **OAuth 2.1 Compliance**: Implements the latest OAuth 2.1 specification with PKCE enforcement
- **Google OAuth Provider**: Supports Google OAuth for authentication with GCP/GKE integration
- **Token Refresh**: Automatic token refresh with refresh token rotation
- **Downstream OAuth Passthrough**: Use users' OAuth tokens for Kubernetes API authentication (RBAC)
- **Security Features**:
  - Rate limiting (per-IP and per-user)
  - Audit logging
  - Token encryption at rest (AES-256-GCM)
  - Client registration rate limiting
  - HTTPS enforcement (except for localhost development)
- **Customizable Branding**: Custom interstitial page for OAuth success flow

## Quick Start

### 1. Setup Google OAuth Credentials

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the required APIs:
   - Google Kubernetes Engine API (for GKE access)
4. Create OAuth 2.0 credentials:
   - Go to "APIs & Services" > "Credentials"
   - Click "Create Credentials" > "OAuth client ID"
   - Select "Web application"
   - Add authorized redirect URIs:
     - For development: `http://localhost:8080/oauth/callback`
     - For production: `https://your-domain.com/oauth/callback`
   - Save the Client ID and Client Secret

### 2. Start the Server with OAuth

```bash
# Using command-line flags
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://your-domain.com \
  --google-client-id=YOUR_CLIENT_ID \
  --google-client-secret=YOUR_CLIENT_SECRET \
  --registration-token=YOUR_SECURE_TOKEN

# Using environment variables
export GOOGLE_CLIENT_ID="YOUR_CLIENT_ID"
export GOOGLE_CLIENT_SECRET="YOUR_CLIENT_SECRET"

mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://your-domain.com \
  --registration-token=YOUR_SECURE_TOKEN
```

### 3. Client Registration

Before a client can authenticate, it must be registered with the OAuth server:

```bash
# Register a new OAuth client
curl -X POST https://your-domain.com/oauth/register \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_REGISTRATION_TOKEN" \
  -d '{
    "client_name": "My MCP Client",
    "redirect_uris": ["cursor://oauth/callback"],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "token_endpoint_auth_method": "none",
    "scope": "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"
  }'
```

Response:
```json
{
  "client_id": "generated-client-id",
  "client_id_issued_at": 1234567890,
  "client_name": "My MCP Client",
  "redirect_uris": ["cursor://oauth/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none",
  "scope": "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"
}
```

### 4. Client Authentication Flow

1. **Authorization Request**: Client redirects user to authorization endpoint:
   ```
   https://your-domain.com/oauth/authorize?
     client_id=CLIENT_ID&
     redirect_uri=cursor://oauth/callback&
     response_type=code&
     scope=https://www.googleapis.com/auth/cloud-platform&
     state=RANDOM_STATE&
     code_challenge=PKCE_CHALLENGE&
     code_challenge_method=S256
   ```

2. **User Authorization**: User is redirected to Google to authorize access

3. **Callback**: After authorization, user is redirected back with authorization code:
   ```
   cursor://oauth/callback?code=AUTH_CODE&state=RANDOM_STATE
   ```

4. **Token Exchange**: Client exchanges authorization code for access token:
   ```bash
   curl -X POST https://your-domain.com/oauth/token \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=authorization_code&\
         code=AUTH_CODE&\
         redirect_uri=cursor://oauth/callback&\
         client_id=CLIENT_ID&\
         code_verifier=PKCE_VERIFIER"
   ```

5. **Access MCP Tools**: Use the access token to call MCP endpoints:
   ```bash
   curl -X POST https://your-domain.com/mcp \
     -H "Authorization: Bearer ACCESS_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "jsonrpc": "2.0",
       "method": "tools/call",
       "params": {
         "name": "kubernetes_list",
         "arguments": {
           "namespace": "default",
           "resourceType": "pods"
         }
       },
       "id": 1
     }'
   ```

## Downstream OAuth Authentication (RBAC Passthrough)

When running `mcp-kubernetes` inside a Kubernetes cluster, you can enable **downstream OAuth authentication** to ensure that users only have their configured Kubernetes RBAC permissions.

### How It Works

1. User authenticates to `mcp-kubernetes` via Google OAuth
2. `mcp-kubernetes` stores the user's OAuth access token
3. When the user makes MCP tool calls, their OAuth token is used to authenticate with the Kubernetes API
4. The Kubernetes cluster validates the token via its OIDC configuration and applies RBAC rules

This ensures that users can only perform actions they're authorized for in the Kubernetes cluster, rather than having the full permissions of the `mcp-kubernetes` service account.

### Requirements

- The Kubernetes cluster must be configured for OIDC authentication with Google as the identity provider
- `mcp-kubernetes` must be running in-cluster (`--in-cluster` flag)
- OAuth must be enabled (`--enable-oauth` flag)

### GKE Configuration

For Google Kubernetes Engine (GKE), OIDC with Google is typically enabled by default. You may need to configure:

1. **RBAC bindings**: Create `RoleBinding` or `ClusterRoleBinding` resources to grant permissions to Google identities:

```yaml
# Example: Give user read access to a namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: user-reader
  namespace: my-app
subjects:
- kind: User
  name: user@example.com  # Google account email
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: view
  apiGroup: rbac.authorization.k8s.io
```

### Non-GKE Kubernetes Clusters

For other Kubernetes distributions, configure the API server with OIDC flags:

```yaml
# API server configuration
--oidc-issuer-url=https://accounts.google.com
--oidc-client-id=YOUR_GOOGLE_CLIENT_ID
--oidc-username-claim=email
--oidc-groups-claim=groups  # Optional
```

### Enabling Downstream OAuth

Start the server with the `--downstream-oauth` flag:

```bash
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --in-cluster \
  --downstream-oauth \
  --oauth-base-url=https://mcp.example.com \
  --google-client-id=YOUR_CLIENT_ID \
  --google-client-secret=YOUR_CLIENT_SECRET \
  --registration-token=YOUR_SECURE_TOKEN
```

### Fallback Behavior

If a user's OAuth token is unavailable (e.g., expired or not present), `mcp-kubernetes` falls back to using its service account token. This ensures the server remains functional while logging warnings about the fallback.

## Configuration Options

### Command-Line Flags

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `--enable-oauth` | Enable OAuth 2.1 authentication | `false` | No |
| `--oauth-base-url` | OAuth base URL (e.g., https://mcp.example.com) | - | Yes (if OAuth enabled) |
| `--google-client-id` | Google OAuth Client ID | - | Yes (if OAuth enabled) |
| `--google-client-secret` | Google OAuth Client Secret | - | Yes (if OAuth enabled) |
| `--registration-token` | OAuth client registration access token | - | Yes (unless public registration enabled) |
| `--allow-public-registration` | Allow unauthenticated OAuth client registration | `false` | No |
| `--disable-streaming` | Disable streaming for streamable-http transport | `false` | No |
| `--downstream-oauth` | Use OAuth tokens for downstream Kubernetes API auth | `false` | No |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `GOOGLE_CLIENT_ID` | Google OAuth Client ID (alternative to flag) |
| `GOOGLE_CLIENT_SECRET` | Google OAuth Client Secret (alternative to flag) |
| `ALLOWED_ORIGINS` | Comma-separated list of allowed CORS origins |

## OAuth Endpoints

The server exposes the following OAuth 2.1 endpoints:

| Endpoint | Description | RFC |
|----------|-------------|-----|
| `/.well-known/oauth-authorization-server` | Authorization Server Metadata | RFC 8414 |
| `/.well-known/oauth-protected-resource` | Protected Resource Metadata | RFC 9728 |
| `/oauth/register` | Dynamic Client Registration | RFC 7591 |
| `/oauth/authorize` | OAuth Authorization | RFC 6749 |
| `/oauth/token` | Token Endpoint | RFC 6749 |
| `/oauth/callback` | OAuth Callback (from Google) | RFC 6749 |
| `/oauth/revoke` | Token Revocation | RFC 7009 |
| `/oauth/introspect` | Token Introspection | RFC 7662 |

## Security Considerations

### HTTPS Requirement

OAuth 2.1 **requires HTTPS** for all production deployments. The server will reject HTTP connections except for loopback addresses (localhost, 127.0.0.1, ::1).

For development, you can use:
- `http://localhost:8080`
- `http://127.0.0.1:8080`

### Security Best Practices

#### Production Configuration Checklist

**CRITICAL - Never deploy to production with these settings:**

```bash
# ❌ INSECURE - Do not use in production
--allow-public-registration=true          # Allows unlimited client registration (DoS risk)
--allow-insecure-auth-without-state=true  # Weakens CSRF protection
--debug=true                              # May log sensitive information
```

**✅ RECOMMENDED - Production configuration:**

```bash
# Secure production settings
--allow-public-registration=false          # Require registration token
--registration-token=STRONG_RANDOM_TOKEN  # Use cryptographically random token
--allow-insecure-auth-without-state=false # Enforce state parameter for CSRF protection
--oauth-encryption-key=$(openssl rand -base64 32)  # Enable token encryption
```

#### Client Registration Token

By default, client registration requires a bearer token for security. You can:

1. **Use a registration token** (✅ RECOMMENDED for production):
   ```bash
   # Generate a secure random token
   REGISTRATION_TOKEN=$(openssl rand -hex 32)
   --registration-token=$REGISTRATION_TOKEN
   ```

2. **Allow public registration** (❌ NOT RECOMMENDED for production):
   ```bash
   # Only for development/testing
   --allow-public-registration=true
   ```
   **Warning**: This allows unlimited client registration and may lead to denial-of-service attacks.

#### Token Encryption at Rest

**REQUIRED for production**: Encrypt tokens at rest using AES-256-GCM:

```bash
# Generate a 32-byte encryption key (base64 encoded)
OAUTH_ENCRYPTION_KEY=$(openssl rand -base64 32)

# Pass to server
--oauth-encryption-key=$OAUTH_ENCRYPTION_KEY

# Or via environment variable
export OAUTH_ENCRYPTION_KEY=$(openssl rand -base64 32)
```

**Important**: Store the encryption key securely (e.g., in Kubernetes Secret, HashiCorp Vault, AWS Secrets Manager). The key must be:
- Exactly 32 bytes (after base64 decoding)
- Base64 encoded
- Stored securely and never committed to version control

#### CORS Configuration

When configuring CORS origins, validate all URLs:

```bash
# Valid CORS origins (must include scheme and host)
export ALLOWED_ORIGINS="https://app.example.com,https://admin.example.com"

# ❌ INVALID - will be rejected
export ALLOWED_ORIGINS="example.com"  # Missing scheme
export ALLOWED_ORIGINS="https://example.com/path"  # Paths not allowed
```

The server validates:
- All origins must use `http` or `https` scheme
- Must include host (with optional port)
- No path, query, or fragment components
- Normalizes to `scheme://host:port` format

#### Security Headers

The server automatically adds comprehensive security headers:

- **HSTS**: Enabled for HTTPS connections (or via `ENABLE_HSTS=true` for reverse proxies)
- **Content Security Policy**: Restricts resource loading
- **Permissions Policy**: Disables dangerous browser features
- **Cross-Origin Policies**: Isolation from other origins
- **X-Frame-Options**: Prevents clickjacking
- **X-Content-Type-Options**: Prevents MIME sniffing

For reverse proxy scenarios (e.g., behind ingress):
```bash
export ENABLE_HSTS=true  # Force HSTS header even without TLS termination
```

### Rate Limiting

The server implements multi-layered rate limiting to prevent abuse:
- **IP-based**: 10 req/sec per IP (burst: 20)
- **User-based**: 100 req/sec per authenticated user (burst: 200)
- **Client registration**: Prevents mass registration attacks (max 10 clients per IP by default)

Configure limits:
```bash
--max-clients-per-ip=10  # Limit clients registered per IP address
```

### Audit Logging

Security audit logging is **enabled by default** and logs:
- Authentication events (success/failure)
- Token operations (issue, refresh, revoke)
- Security violations (rate limits, invalid tokens)
- Client registration attempts

Review logs regularly for:
- Unusual authentication patterns
- High rate of failed authentications
- Unexpected client registrations
- Token abuse patterns

### Monitoring and Alerts

The server tracks metrics for downstream OAuth operations:

```go
// Available metrics (accessible via sc.Metrics())
metrics := sc.Metrics()
success, fallback, failures := metrics.GetMetrics()

// Track:
// - PerUserAuthSuccess: Successful per-user K8s authentications
// - PerUserAuthFallback: Fallbacks to service account
// - BearerClientFailures: Failed bearer token client creations
```

**Recommended alerts**:
- High fallback rate (>10% of requests)
- Increasing bearer client failures
- Unusual authentication patterns

### Dependency Security

The project includes automated vulnerability scanning:

```bash
# Run locally
make govulncheck

# Runs automatically in CI/CD on every pull request
```

**Dependencies**:
- Primary OAuth library: `github.com/giantswarm/mcp-oauth v0.2.1`
- Ensure regular updates for security patches
- Review dependency updates before merging

### Environment Variables

**Never commit secrets to version control**. Use environment variables or secrets management:

```bash
# ✅ GOOD - Use environment variables
export GOOGLE_CLIENT_SECRET="your-secret"
export OAUTH_ENCRYPTION_KEY=$(openssl rand -base64 32)
export REGISTRATION_TOKEN=$(openssl rand -hex 32)

# ❌ BAD - Never hardcode secrets
--google-client-secret="hardcoded-secret"  # Don't do this!
```

For Kubernetes deployments, use Secrets:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mcp-oauth-credentials
type: Opaque
data:
  google-client-id: <base64-encoded>
  google-client-secret: <base64-encoded>
  oauth-encryption-key: <base64-encoded-32-bytes>
  registration-token: <base64-encoded>
```

### Security Checklist for Production

Before deploying to production, verify:

- [ ] HTTPS is enforced (not localhost)
- [ ] `--allow-public-registration=false`
- [ ] `--allow-insecure-auth-without-state=false`
- [ ] `--debug=false` (debug logging disabled)
- [ ] OAuth encryption key is set (32 bytes, base64 encoded)
- [ ] Registration token is configured and secure
- [ ] CORS origins are validated and minimal
- [ ] Secrets are stored in secure secret management (not in code)
- [ ] Rate limiting is enabled (default settings)
- [ ] Audit logging is reviewed regularly
- [ ] Dependency scanning is enabled in CI/CD
- [ ] Container runs as non-root with minimal privileges
- [ ] Resource limits are set appropriately
- [ ] Network policies restrict unnecessary traffic
- [ ] Regular security updates are applied

## Customizing the OAuth Success Page

You can customize the OAuth success interstitial page:

```go
// In code, when creating the OAuth config
config := server.OAuthConfig{
    // ... other config ...
    Interstitial: &oauth.InterstitialConfig{
        Title:              "Connected to Kubernetes MCP",
        Message:            "You have successfully authenticated with {{.AppName}}",
        ButtonText:         "Open {{.AppName}}",
        PrimaryColor:       "#4f46e5",
        BackgroundGradient: "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
    },
}
```

## Troubleshooting

### Common Issues

1. **"OAuth 2.1 requires HTTPS"**: Ensure you're using HTTPS in production or localhost for development

2. **"registration-token is required"**: Provide a registration token via `--registration-token` flag or enable public registration (not recommended)

3. **"Google OAuth credentials not found"**: Verify that `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` are set

4. **CORS errors**: Add your client's origin to the `ALLOWED_ORIGINS` environment variable

### Debug Mode

Enable debug logging to troubleshoot OAuth issues:

```bash
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --debug \
  ...other flags...
```

## Example: Development Setup

For local development:

```bash
# Set up environment variables
export GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export GOOGLE_CLIENT_SECRET="your-client-secret"
export REGISTRATION_TOKEN="dev-registration-token-123"

# Start server with OAuth
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=http://localhost:8080 \
  --registration-token=$REGISTRATION_TOKEN \
  --debug

# In another terminal, register a client
curl -X POST http://localhost:8080/oauth/register \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dev-registration-token-123" \
  -d '{
    "client_name": "Dev Client",
    "redirect_uris": ["http://localhost:3000/callback"],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "token_endpoint_auth_method": "none"
  }'
```

## Architecture

The OAuth implementation is based on the [mcp-oauth](https://github.com/giantswarm/mcp-oauth) library, which provides:

- OAuth 2.1 server implementation
- Google OAuth provider integration
- In-memory token storage
- Security features (rate limiting, audit logging, encryption)
- RFC-compliant endpoints

The integration is organized as follows:

```
internal/mcp/oauth/          # OAuth integration layer
  ├── doc.go                 # Package documentation
  ├── handler.go             # OAuth handler wrapper
  └── token_provider.go      # Token provider (access token context handling)

internal/server/
  ├── context.go             # ServerContext with per-user client support
  ├── options.go             # Configuration options (WithDownstreamOAuth)
  └── oauth_http.go          # OAuth HTTP server integration

internal/k8s/
  ├── client.go              # Client interface with ClientFactory
  ├── bearer_client.go       # Bearer token client for OAuth passthrough
  └── resource_ops.go        # Shared resource operations

internal/tools/
  └── types.go               # GetK8sClient helper for tool handlers

cmd/
  └── serve.go               # Command-line interface (--downstream-oauth)
```

### Downstream OAuth Data Flow

When `--downstream-oauth` is enabled:

1. **Authentication**: User authenticates via Google OAuth
2. **Token Storage**: Access token is stored in `mcp-oauth` token store
3. **Request Handling**: On each MCP tool call:
   - `ValidateToken` middleware validates the MCP access token
   - Access token injector middleware retrieves user's Google OAuth token
   - Token is stored in request context
4. **Tool Execution**: Tool handler calls `tools.GetK8sClient(ctx, sc)`
   - If downstream OAuth enabled, creates `bearerTokenClient` with user's token
   - Otherwise, uses shared service account client
5. **Kubernetes API Call**: Bearer token client uses user's token for K8s API auth
6. **RBAC Enforcement**: Kubernetes validates token and applies user's RBAC permissions

## References

- [OAuth 2.1 Specification](https://oauth.net/2.1/)
- [RFC 6749: OAuth 2.0](https://datatracker.ietf.org/doc/html/rfc6749)
- [RFC 7591: Dynamic Client Registration](https://datatracker.ietf.org/doc/html/rfc7591)
- [RFC 7636: PKCE](https://datatracker.ietf.org/doc/html/rfc7636)
- [RFC 8414: Authorization Server Metadata](https://datatracker.ietf.org/doc/html/rfc8414)
- [RFC 9728: Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728)
- [mcp-oauth Library](https://github.com/giantswarm/mcp-oauth)


