# OAuth 2.1 Authentication for MCP Kubernetes Server

The MCP Kubernetes server supports OAuth 2.1 authentication for HTTP transports (streamable-http). This provides secure, token-based authentication for accessing the Kubernetes MCP tools.

## Features

- **OAuth 2.1 Compliance**: Implements the latest OAuth 2.1 specification with PKCE enforcement
- **Multiple OAuth Providers**:
  - **Dex OIDC Provider** (default): Full OIDC support with connector selection, groups claim, and custom connector ID
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

### 2. Start the Server with OAuth (Google Provider)

```bash
# Using command-line flags
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-provider=google \
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
  --oauth-provider=google \
  --oauth-base-url=https://your-domain.com \
  --registration-token=YOUR_SECURE_TOKEN
```

## Quick Start with Dex Provider

### 1. Prerequisites

You'll need a Dex server configured with at least one connector (GitHub, LDAP, SAML, etc.). Dex acts as a portal to other identity providers.

For testing, you can deploy Dex using Helm:

```bash
helm repo add dex https://charts.dexidp.io
helm install dex dex/dex --set config.issuer=https://dex.example.com
```

### 2. Register OAuth Client in Dex

Add your mcp-kubernetes OAuth client to Dex configuration:

```yaml
# dex-config.yaml
staticClients:
- id: mcp-kubernetes
  name: 'MCP Kubernetes'
  secret: your-dex-client-secret
  redirectURIs:
  - 'https://your-domain.com/oauth/callback'
```

### 3. Start the Server with Dex OAuth

```bash
# Using command-line flags (Dex is the default provider)
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://your-domain.com \
  --dex-issuer-url=https://dex.example.com \
  --dex-client-id=mcp-kubernetes \
  --dex-client-secret=YOUR_DEX_CLIENT_SECRET \
  --dex-connector-id=github \
  --registration-token=YOUR_SECURE_TOKEN

# Using environment variables
export DEX_ISSUER_URL="https://dex.example.com"
export DEX_CLIENT_ID="mcp-kubernetes"
export DEX_CLIENT_SECRET="YOUR_DEX_CLIENT_SECRET"
export DEX_CONNECTOR_ID="github"  # Optional: bypass connector selection

mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://your-domain.com \
  --registration-token=YOUR_SECURE_TOKEN
```

**Note**: The `--dex-connector-id` flag is optional but recommended for better UX. When set, it automatically selects the specified Dex connector (e.g., GitHub, LDAP) instead of showing the connector selection screen.

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

### Security Model (Fail-Closed)

When downstream OAuth is enabled, the server operates in **strict mode** - this is mandatory and cannot be disabled. This design follows the security principle of "fail closed" to protect users from misconfigurations.

#### How Fail-Closed Works

With downstream OAuth enabled:
- Requests **must** have a valid OAuth token to access Kubernetes resources
- If no OAuth token is present, the request fails with an authentication error
- If the OAuth token is invalid or expired, the request fails with an authentication error
- **No fallback to service account occurs** - this prevents privilege escalation

#### Security Benefits

This fail-closed approach ensures:
1. **Audit trail integrity**: All operations are always logged under the actual user identity
2. **RBAC enforcement**: Users can only perform actions their own RBAC permissions allow
3. **Misconfiguration detection**: OIDC misconfigurations fail visibly (immediately) rather than silently granting service account permissions
4. **No privilege escalation**: Users cannot accidentally get elevated permissions through service account fallback

#### Error Messages

When authentication fails, users receive clear error messages:
- "authentication required: please log in to access this resource" - when no OAuth token is present
- "authentication failed: your session may have expired, please log in again" - when the OAuth token is invalid

### Migration Notes

**If upgrading from versions before fail-closed was enforced:**

Previous versions allowed falling back to service account when OAuth tokens were missing. This behavior was a security risk and has been removed. After upgrading:

1. **Ensure OIDC is properly configured** on your Kubernetes cluster before deploying
2. **Test authentication flow** in a non-production environment first
3. **Users must authenticate** - anonymous access via service account fallback is no longer possible
4. **Check monitoring** for authentication failures after deployment (see Monitoring section below)

If your deployment previously relied on service account fallback (not recommended), you will need to ensure all users authenticate properly via OAuth.

## Configuration Options

### Command-Line Flags

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `--enable-oauth` | Enable OAuth 2.1 authentication | `false` | No |
| `--oauth-base-url` | OAuth base URL (e.g., https://mcp.example.com) | - | Yes (if OAuth enabled) |
| `--oauth-provider` | OAuth provider: `dex` or `google` | `dex` | No |
| `--google-client-id` | Google OAuth Client ID | - | Yes (if Google provider) |
| `--google-client-secret` | Google OAuth Client Secret | - | Yes (if Google provider) |
| `--dex-issuer-url` | Dex OIDC issuer URL (e.g., https://dex.example.com) | - | Yes (if Dex provider) |
| `--dex-client-id` | Dex OAuth Client ID | - | Yes (if Dex provider) |
| `--dex-client-secret` | Dex OAuth Client Secret | - | Yes (if Dex provider) |
| `--dex-connector-id` | Dex connector ID (optional, bypasses selection screen) | - | No |
| `--registration-token` | OAuth client registration access token | - | Yes (unless public registration enabled) |
| `--allow-public-registration` | Allow unauthenticated OAuth client registration | `false` | No |
| `--disable-streaming` | Disable streaming for streamable-http transport | `false` | No |
| `--downstream-oauth` | Use OAuth tokens for downstream Kubernetes API auth (fail-closed: no service account fallback) | `false` | No |

### Secret Management (CRITICAL for Production)

**PRODUCTION REQUIREMENT:** For production deployments, you **MUST** use a secret management solution. Environment variables are **NOT secure** for production use.

**Recommended Secret Managers:**
- HashiCorp Vault
- AWS Secrets Manager
- Google Cloud Secret Manager
- Azure Key Vault
- Kubernetes External Secrets Operator

See the [Production Secret Management](#production-secret-management) section below for detailed implementation examples.

### Environment Variables (Development Only)

**⚠️ WARNING:** Environment variables are **NOT secure for production**. Use them only for local development and testing.

**Why environment variables are insecure:**
- Visible in process listings (`ps aux`, `docker inspect`)
- Leaked in logs, error messages, and stack traces
- No audit trail or rotation support
- No encryption at rest
- Vulnerable to memory dumps

| Variable | Description | Production Alternative |
|----------|-------------|----------------------|
| `GOOGLE_CLIENT_ID` | Google OAuth Client ID (Google provider) | Use secret manager |
| `GOOGLE_CLIENT_SECRET` | Google OAuth Client Secret (Google provider) | Use secret manager |
| `DEX_ISSUER_URL` | Dex OIDC issuer URL (Dex provider) | ConfigMap or secret manager |
| `DEX_CLIENT_ID` | Dex OAuth Client ID (Dex provider) | ConfigMap or secret manager |
| `DEX_CLIENT_SECRET` | Dex OAuth Client Secret (Dex provider) | Use secret manager |
| `DEX_CONNECTOR_ID` | Dex connector ID (optional) | ConfigMap |
| `OAUTH_ENCRYPTION_KEY` | OAuth encryption key (32 bytes, base64) | Use secret manager |
| `ALLOWED_ORIGINS` | Comma-separated list of allowed CORS origins | ConfigMap or secret manager |

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

OAuth 2.1 **requires HTTPS** for all production deployments. The server enforces HTTPS for all OAuth URLs with multiple security validations:

**Automatic Validation:**
- OAuth base URL must use HTTPS
- Dex issuer URL must use HTTPS (for Dex provider)
- Localhost URLs are blocked in production (SSRF protection)
- Private IP addresses are blocked (SSRF protection)

**SSRF Protection:**
The server validates all OAuth URLs to prevent Server-Side Request Forgery (SSRF) attacks:
- Blocks localhost and 127.0.0.1
- Blocks private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
- Blocks link-local addresses (169.254.0.0/16, fe80::/10)
- Blocks loopback addresses (::1)

For development only, you can use:
- `http://localhost:8080`
- `http://127.0.0.1:8080`

**Note**: Production deployments attempting to use HTTP or private IPs will fail validation with a clear error message.

### OAuth Provider Security Considerations

#### Dex OIDC Provider

When using the Dex provider, the server requests the following scopes:
- `openid`: OpenID Connect authentication
- `profile`: User profile information
- `email`: User email address
- **`groups`**: User group memberships
- `offline_access`: Refresh tokens

**Groups Scope Privacy Implications:**

The `groups` scope exposes user group memberships to the MCP server. This is necessary for:
- Kubernetes RBAC integration (mapping groups to cluster roles)
- Fine-grained access control
- Audit logging with group context

**Security considerations:**
- Group memberships may contain sensitive organizational information
- Ensure your Dex connectors (GitHub, LDAP, etc.) are configured to return only necessary groups
- Consider using group filtering in Dex configuration if you need to limit exposure
- Document which groups are being passed to users

**Connector ID Security:**

The optional `--dex-connector-id` flag bypasses the Dex connector selection screen for better UX. Security notes:
- Connector IDs are not secrets but reveal your authentication backend (e.g., "github", "ldap", "saml")
- This information could help attackers understand your infrastructure
- Acceptable for internal use but avoid publishing in public documentation
- Consider the trade-off between UX convenience and information disclosure

#### Google OAuth Provider

When using the Google provider, the server requests:
- `https://www.googleapis.com/auth/cloud-platform`: Full GCP access
- `https://www.googleapis.com/auth/userinfo.email`: User email
- `https://www.googleapis.com/auth/userinfo.profile`: User profile

**Security note**: The `cloud-platform` scope grants broad access to GCP resources. Ensure your OAuth consent screen clearly communicates this to users.

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

```bash
# OAuth authentication metrics are available via OpenTelemetry instrumentation
# Enable instrumentation to track OAuth downstream authentication:
# - oauth_downstream_auth_total{result="success"}: Successful per-user K8s authentications
# - oauth_downstream_auth_total{result="denied"}: Requests blocked due to missing/invalid OAuth tokens
# - oauth_downstream_auth_total{result="failure"}: Failed bearer token client creations

# Configure instrumentation via environment variables:
# INSTRUMENTATION_ENABLED=true
# METRICS_EXPORTER=prometheus
```

**Recommended Prometheus alerts**:

```yaml
groups:
- name: mcp_kubernetes_oauth
  rules:
  # Alert when authentication denial rate is high
  # This could indicate OIDC misconfiguration or users not logged in
  - alert: HighOAuthDenialRate
    expr: |
      rate(oauth_downstream_auth_total{result="denied"}[5m]) 
      / rate(oauth_downstream_auth_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High OAuth denial rate detected"
      description: "More than 10% of requests are being denied due to missing/invalid OAuth tokens. Check OIDC configuration and user sessions."

  # Alert when there are any bearer token client creation failures
  # This indicates issues with token format or Kubernetes API connectivity
  - alert: OAuthClientCreationFailures
    expr: rate(oauth_downstream_auth_total{result="failure"}[5m]) > 0
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "OAuth client creation failures"
      description: "Bearer token clients are failing to be created. Check Kubernetes API server OIDC configuration."

  # Alert when no successful authentications in 5 minutes
  # Could indicate complete authentication failure
  - alert: NoSuccessfulOAuthAuth
    expr: |
      absent(rate(oauth_downstream_auth_total{result="success"}[5m]) > 0)
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "No successful OAuth authentications"
      description: "No users have successfully authenticated in the last 5 minutes. Check OAuth provider and OIDC configuration."
```

**Post-deployment monitoring checklist**:

After enabling downstream OAuth, monitor the following for at least 24 hours:

1. **Authentication success rate**: Should be >95% for active users
2. **Denial rate**: High denial rates indicate OIDC misconfiguration or session issues
3. **Error logs**: Check for "authentication required" or "authentication failed" messages
4. **User feedback**: Ensure users are being prompted to log in and can complete authentication

**Grafana dashboard queries**:

```promql
# Success rate over time
sum(rate(oauth_downstream_auth_total{result="success"}[5m])) 
/ sum(rate(oauth_downstream_auth_total[5m])) * 100

# Denials by reason
sum by (result) (rate(oauth_downstream_auth_total{result=~"denied|failure"}[5m]))

# Total authentication attempts
sum(increase(oauth_downstream_auth_total[1h]))
```

### Dependency Security

The project includes automated vulnerability scanning:

```bash
# Run locally
make govulncheck

# Runs automatically in CI/CD on every pull request
```

**Dependencies**:
- Primary OAuth library: `github.com/giantswarm/mcp-oauth v0.2.7`
- Ensure regular updates for security patches
- Review dependency updates before merging

## Production Secret Management

### Why Secret Managers Are Required

Environment variables are **NOT secure** for production because they:
- Are visible in process listings and container metadata
- Get leaked in logs, error messages, and crash dumps
- Have no built-in audit trail or rotation
- Lack encryption at rest
- Cannot be securely deleted from memory

**Production deployments MUST use secret management solutions.**

### Recommended Secret Management Solutions

#### 1. HashiCorp Vault

**Setup:**
```bash
# Install Vault Agent Injector in Kubernetes
helm repo add hashicorp https://helm.releases.hashicorp.com
helm install vault hashicorp/vault \
  --set "injector.enabled=true" \
  --set "server.enabled=false"
```

**Store secrets:**
```bash
# Write secrets to Vault
vault kv put secret/mcp-kubernetes/oauth \
  google-client-id="your-client-id" \
  google-client-secret="your-client-secret" \
  oauth-encryption-key="$(openssl rand -base64 32)" \
  registration-token="$(openssl rand -hex 32)"
```

**Deployment with Vault annotations:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-kubernetes
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/role: "mcp-kubernetes"
        vault.hashicorp.com/agent-inject-secret-oauth: "secret/data/mcp-kubernetes/oauth"
        vault.hashicorp.com/agent-inject-template-oauth: |
          {{- with secret "secret/data/mcp-kubernetes/oauth" -}}
          export GOOGLE_CLIENT_ID="{{ .Data.data.google-client-id }}"
          export GOOGLE_CLIENT_SECRET="{{ .Data.data.google-client-secret }}"
          export OAUTH_ENCRYPTION_KEY="{{ .Data.data.oauth-encryption-key }}"
          export REGISTRATION_TOKEN="{{ .Data.data.registration-token }}"
          {{- end }}
    spec:
      containers:
      - name: mcp-kubernetes
        command: ["/bin/sh", "-c"]
        args:
          - source /vault/secrets/oauth && exec /app/mcp-kubernetes serve --enable-oauth ...
```

#### 2. AWS Secrets Manager

**Setup:**
```bash
# Create secret
aws secretsmanager create-secret \
  --name mcp-kubernetes/oauth \
  --secret-string '{
    "google-client-id": "your-client-id",
    "google-client-secret": "your-client-secret",
    "oauth-encryption-key": "'"$(openssl rand -base64 32)"'",
    "registration-token": "'"$(openssl rand -hex 32)"'"
  }'
```

**Go code to fetch secrets:**
```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type OAuthSecrets struct {
    GoogleClientID     string `json:"google-client-id"`
    GoogleClientSecret string `json:"google-client-secret"`
    OAuthEncryptionKey string `json:"oauth-encryption-key"`
    RegistrationToken  string `json:"registration-token"`
}

func getSecrets(ctx context.Context) (*OAuthSecrets, error) {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }

    client := secretsmanager.NewFromConfig(cfg)
    
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
        SecretId: aws.String("mcp-kubernetes/oauth"),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get secret: %w", err)
    }

    if result.SecretString == nil {
        return nil, fmt.Errorf("secret value is nil")
    }

    var secrets OAuthSecrets
    if err := json.Unmarshal([]byte(*result.SecretString), &secrets); err != nil {
        return nil, fmt.Errorf("failed to parse secret: %w", err)
    }

    return &secrets, nil
}
```

#### 3. Google Cloud Secret Manager

**Setup:**
```bash
# Create secrets
echo -n "your-client-id" | gcloud secrets create mcp-k8s-google-client-id --data-file=-
echo -n "your-client-secret" | gcloud secrets create mcp-k8s-google-client-secret --data-file=-
openssl rand -base64 32 | gcloud secrets create mcp-k8s-oauth-encryption-key --data-file=-
openssl rand -hex 32 | gcloud secrets create mcp-k8s-registration-token --data-file=-

# Grant access to service account
gcloud secrets add-iam-policy-binding mcp-k8s-google-client-id \
  --member="serviceAccount:mcp-kubernetes@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

**Workload Identity configuration:**
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mcp-kubernetes
  annotations:
    iam.gke.io/gcp-service-account: mcp-kubernetes@PROJECT_ID.iam.gserviceaccount.com
```

#### 4. Azure Key Vault

**Setup:**
```bash
# Create Key Vault
az keyvault create --name mcp-k8s-vault --resource-group mcp-rg --location eastus

# Store secrets
az keyvault secret set --vault-name mcp-k8s-vault --name google-client-id --value "your-client-id"
az keyvault secret set --vault-name mcp-k8s-vault --name google-client-secret --value "your-client-secret"
az keyvault secret set --vault-name mcp-k8s-vault --name oauth-encryption-key --value "$(openssl rand -base64 32)"
az keyvault secret set --vault-name mcp-k8s-vault --name registration-token --value "$(openssl rand -hex 32)"

# Grant access
az keyvault set-policy --name mcp-k8s-vault \
  --spn <service-principal-id> \
  --secret-permissions get list
```

#### 5. Kubernetes External Secrets Operator (Recommended)

**Installation:**
```bash
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets \
  external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace
```

**SecretStore configuration (AWS example):**
```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secretsmanager
  namespace: default
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: mcp-kubernetes
```

**ExternalSecret:**
```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: mcp-oauth-credentials
  namespace: default
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore
  target:
    name: mcp-oauth-credentials
    creationPolicy: Owner
  data:
  - secretKey: google-client-id
    remoteRef:
      key: mcp-kubernetes/oauth
      property: google-client-id
  - secretKey: google-client-secret
    remoteRef:
      key: mcp-kubernetes/oauth
      property: google-client-secret
  - secretKey: oauth-encryption-key
    remoteRef:
      key: mcp-kubernetes/oauth
      property: oauth-encryption-key
  - secretKey: registration-token
    remoteRef:
      key: mcp-kubernetes/oauth
      property: registration-token
```

**Monitoring External Secrets:**
```bash
# Check ExternalSecret status
kubectl get externalsecret mcp-oauth-credentials -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'

# Check for sync errors
kubectl describe externalsecret mcp-oauth-credentials

# Set up alerts for failed syncs
kubectl get externalsecret -A -o json | jq '.items[] | select(.status.conditions[].status=="False") | .metadata.name'
```

**Security Best Practices for External Secrets:**
- Use RBAC to restrict access to SecretStore resources
- Enable audit logging for secret access
- Set appropriate `refreshInterval` (1h recommended)
- Monitor sync failures and set up alerts
- Use separate SecretStores per environment/team
- Regularly rotate IAM credentials used by SecretStore

### Environment Variables (Development Only)

**⚠️ WARNING:** Use environment variables **ONLY for local development**. Never use them in production.

**Secure generation for development (avoiding shell history):**
```bash
# Generate secrets in a temporary file
cat > /tmp/secrets.env << 'EOF'
export GOOGLE_CLIENT_ID="your-dev-client-id"
export GOOGLE_CLIENT_SECRET="your-dev-client-secret"
export OAUTH_ENCRYPTION_KEY=$(openssl rand -base64 32)
export REGISTRATION_TOKEN=$(openssl rand -hex 32)
EOF

# Source it
source /tmp/secrets.env

# Immediately delete it
shred -u /tmp/secrets.env

# Start server
mcp-kubernetes serve --enable-oauth --oauth-base-url=http://localhost:8080
```

### Kubernetes Secrets (Minimum Production Standard)

**⚠️ WARNING:** Basic Kubernetes Secrets are better than environment variables but still not ideal for production. They are:
- Not encrypted at rest by default (unless you enable encryption at rest)
- Accessible to anyone with namespace access
- No built-in rotation or audit trail

**If you must use Kubernetes Secrets**, enable encryption at rest and use RBAC strictly:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mcp-oauth-credentials
  namespace: default
type: Opaque
stringData:
  google-client-id: "your-client-id"
  google-client-secret: "your-client-secret"
  oauth-encryption-key: "base64-encoded-32-bytes"
  registration-token: "your-registration-token"
```

**Deployment using the secret:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-kubernetes
spec:
  template:
    spec:
      containers:
      - name: mcp-kubernetes
        envFrom:
        - secretRef:
            name: mcp-oauth-credentials
```

**Enable encryption at rest** (requires cluster admin access):
```yaml
# /etc/kubernetes/encryption-config.yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
    - secrets
    providers:
    - aescbc:
        keys:
        - name: key1
          secret: <base64-encoded-32-byte-key>
    - identity: {}
```

### Encryption Key Rotation Procedure

**Rotation Schedule:**
- Regular rotation: Every 90 days
- After security incident: Immediately
- Staff changes: Within 24 hours
- Suspected compromise: Immediately

**Zero-Downtime Rotation Procedure:**

1. **Generate new encryption key:**
   ```bash
   NEW_KEY=$(openssl rand -base64 32)
   ```

2. **Deploy with dual-key support** (if supported by your secret manager):
   ```bash
   # Add new key while keeping old key
   kubectl patch secret mcp-oauth-credentials \
     --type=merge \
     -p '{"data":{"oauth-encryption-key-new":"'$(echo -n "$NEW_KEY" | base64)'"}}'
   ```

3. **Update application configuration** to use new key for encryption while keeping old key for decryption:
   ```yaml
   env:
   - name: OAUTH_ENCRYPTION_KEY
     valueFrom:
       secretKeyRef:
         name: mcp-oauth-credentials
         key: oauth-encryption-key-new
   - name: OAUTH_ENCRYPTION_KEY_OLD
     valueFrom:
       secretKeyRef:
         name: mcp-oauth-credentials
         key: oauth-encryption-key
   ```

4. **Wait for token expiration** (or force re-authentication):
   ```bash
   # Check token expiration times
   # Wait at least 1 hour (typical token lifetime)
   ```

5. **Remove old encryption key:**
   ```bash
   kubectl patch secret mcp-oauth-credentials \
     --type=json \
     -p '[{"op":"remove","path":"/data/oauth-encryption-key"}]'
   
   # Rename new key to primary
   kubectl patch secret mcp-oauth-credentials \
     --type=json \
     -p '[
       {"op":"copy","from":"/data/oauth-encryption-key-new","path":"/data/oauth-encryption-key"},
       {"op":"remove","path":"/data/oauth-encryption-key-new"}
     ]'
   ```

**Verification:**
```bash
# Verify new key is in use
kubectl exec -it deploy/mcp-kubernetes -- env | grep OAUTH_ENCRYPTION_KEY

# Test OAuth flow works
curl -k https://YOUR_DOMAIN/.well-known/oauth-authorization-server
```

### Incident Response Procedures

#### 1. Secrets Leaked in Logs/Monitoring

**Impact:** HIGH - Immediate rotation required

**Response Timeline:** Within 1 hour

**Actions:**
```bash
# 1. Rotate ALL affected secrets immediately
# (Google Client Secret, OAuth Encryption Key, Registration Token)

# 2. Revoke all active tokens
kubectl exec -it deploy/mcp-kubernetes -- /app/mcp-kubernetes admin revoke-all-tokens

# 3. Clean up logs (if possible)
# Contact your logging provider to purge sensitive data

# 4. Review and fix logging configuration
# Ensure secrets are masked in application logs
```

#### 2. Secrets in Git History

**Impact:** CRITICAL - Permanent exposure

**Response Timeline:** Within 1 hour

**Actions:**
```bash
# 1. Rotate ALL secrets immediately (assume compromised)

# 2. Clean Git history using git-filter-repo
pip3 install git-filter-repo

# Create a file with patterns to remove
cat > /tmp/secrets-to-remove.txt << 'EOF'
google-client-secret
GOOGLE_CLIENT_SECRET
oauth-encryption-key
OAUTH_ENCRYPTION_KEY
registration-token
REGISTRATION_TOKEN
EOF

# Rewrite history to remove secrets
git filter-repo --replace-text /tmp/secrets-to-remove.txt --force

# 3. Force push (coordinate with team!)
git push --force --all
git push --force --tags

# 4. Notify team to re-clone repository
echo "ALERT: All team members must delete local copies and re-clone"

# 5. Consider repository as permanently compromised
# Create new Google OAuth credentials with new client ID/secret
```

#### 3. Secrets in Container Images

**Impact:** HIGH - Public exposure if image is public

**Response Timeline:** Within 1 hour

**Actions:**
```bash
# 1. Rotate all secrets immediately

# 2. Delete compromised container images
docker rmi your-image:compromised-tag
kubectl delete pod -l app=mcp-kubernetes  # Force re-pull

# 3. Re-build images without secrets
# Ensure Dockerfile does NOT copy .env files or secrets

# 4. Scan images for secrets
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy image your-image:latest --severity HIGH,CRITICAL

# 5. If image was pushed to public registry, consider it permanently compromised
# Create new Google OAuth credentials
```

#### 4. Secrets in CI/CD Logs

**Impact:** HIGH - Logs may be accessible to many users

**Response Timeline:** Within 1 hour

**Actions:**
```bash
# 1. Rotate all exposed secrets immediately

# 2. Delete/purge CI/CD run logs (if possible)
# GitHub Actions: Cannot delete, must rotate secrets
# GitLab CI: Can delete pipeline logs
# CircleCI: Contact support to purge logs

# 3. Add secret masking to CI/CD
# GitHub Actions: Use ::add-mask::
echo "::add-mask::$GOOGLE_CLIENT_SECRET"

# 4. Review CI/CD configuration
# Ensure secrets are injected as environment variables, not echoed
```

### Prevention Measures

**1. Pre-commit Hooks:**
```bash
# Install gitleaks
brew install gitleaks  # macOS
# or
wget https://github.com/gitleaks/gitleaks/releases/download/v8.18.0/gitleaks_8.18.0_linux_x64.tar.gz

# Add to .git/hooks/pre-commit
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
gitleaks protect --verbose --redact --staged
EOF

chmod +x .git/hooks/pre-commit

# Or use git-secrets
git secrets --install
git secrets --register-aws
git secrets --add 'google-client-secret'
git secrets --add 'oauth-encryption-key'
```

**2. GitHub Secret Scanning:**
```yaml
# .github/secret_scanning.yml
# GitHub automatically scans for known patterns
# Add custom patterns:
patterns:
  - pattern: "GOOGLE_CLIENT_SECRET=[a-zA-Z0-9_-]{24,}"
    description: "Google OAuth Client Secret"
  - pattern: "OAUTH_ENCRYPTION_KEY=[A-Za-z0-9+/]{43}="
    description: "OAuth Encryption Key (Base64)"
```

**3. Development Best Practices:**
```bash
# NEVER do this:
export GOOGLE_CLIENT_SECRET="actual-secret"  # Stored in shell history!

# ALWAYS do this:
read -s GOOGLE_CLIENT_SECRET
# (type secret, press Enter)
export GOOGLE_CLIENT_SECRET

# Or use temporary files:
echo "export GOOGLE_CLIENT_SECRET=secret" > /tmp/secrets.env
source /tmp/secrets.env
shred -u /tmp/secrets.env
```

**4. Monitoring and Alerting:**
```bash
# Set up alerts for:
# - Failed authentication attempts (>10/min)
# - OAuth token access without valid session
# - Unusual geographic access patterns
# - Mass token generation (potential leak)

# Example Prometheus alert:
groups:
- name: oauth_security
  rules:
  - alert: HighFailedAuthRate
    expr: rate(oauth_failed_auth_total[5m]) > 0.1
    for: 5m
    annotations:
      description: "High rate of failed OAuth authentications"
```

### Security Checklist for Production (Comprehensive)

**Authentication & Authorization:**
- [ ] HTTPS is enforced (not localhost)
- [ ] `--allow-public-registration=false` (production)
- [ ] `--allow-insecure-auth-without-state=false` (production)
- [ ] Registration token is cryptographically random (32+ bytes)
- [ ] OAuth encryption key is set (exactly 32 bytes, base64 encoded)
- [ ] OAuth encryption key is rotated every 90 days
- [ ] RBAC is configured for downstream Kubernetes authentication
- [ ] Token expiration is set appropriately (default: 1 hour)
- [ ] Refresh token rotation is enabled

**Secret Management:**
- [ ] Secrets are stored in secret manager (Vault/AWS/GCP/Azure)
- [ ] Secrets are NOT stored as environment variables (production)
- [ ] Secrets are NOT committed to Git
- [ ] Secrets are NOT in container images
- [ ] Kubernetes encryption at rest is enabled
- [ ] Secret access is audited and monitored
- [ ] Incident response plan for compromised secrets exists
- [ ] Team knows how to rotate secrets

**Network Security:**
- [ ] TLS certificates are valid and from trusted CA
- [ ] TLS minimum version is 1.2 or higher
- [ ] HSTS header is enabled (`ENABLE_HSTS=true`)
- [ ] CORS origins are validated and minimal
- [ ] Network policies restrict unnecessary traffic
- [ ] Ingress uses appropriate ingress class
- [ ] Rate limiting is enabled (default: 10 req/sec per IP)

**Application Security:**
- [ ] Debug logging is disabled (`--debug=false`)
- [ ] Non-root container user is configured
- [ ] Security context is properly set (no privileged mode)
- [ ] Resource limits are set appropriately
- [ ] Health checks are configured
- [ ] Graceful shutdown is implemented

**Monitoring & Logging:**
- [ ] Audit logging is enabled and reviewed regularly
- [ ] Metrics are exported and monitored
- [ ] Alerts are configured for security events
- [ ] Failed authentication attempts are tracked
- [ ] Token usage patterns are monitored
- [ ] Secrets are masked in all logs

**Supply Chain Security:**
- [ ] Dependency scanning is enabled (Dependabot/Renovate)
- [ ] Container scanning is enabled (Trivy/Grype/Snyk)
- [ ] SBOM is generated for releases
- [ ] Base images are from trusted sources
- [ ] Images are signed and verified
- [ ] Vulnerability alerts are acted upon promptly

**Operational Security:**
- [ ] Regular security updates are applied
- [ ] Security patches are deployed within SLA
- [ ] Backup and disaster recovery plan exists
- [ ] Access to secrets is logged and reviewed
- [ ] Principle of least privilege is enforced
- [ ] Regular security audits are performed
- [ ] Penetration testing is conducted annually

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

## Valkey Token Storage (Production)

By default, mcp-kubernetes uses in-memory token storage which is **NOT recommended for production** because:
- Tokens are lost on pod restart
- Multiple replicas cannot share session state
- Rolling updates cause user session loss

For production deployments, use Valkey (Redis-compatible) storage.

### Benefits of Valkey Storage

1. **High Availability** - Multiple replicas can serve users with shared session state
2. **Rolling Updates** - Deployments do not disrupt user sessions
3. **Scalability** - Can scale mcp-kubernetes instances independently
4. **Session Persistence** - Tokens survive pod restarts

### Valkey Configuration

#### Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--oauth-storage-type` | Storage backend: `memory` or `valkey` | `memory` |
| `--valkey-url` | Valkey server address (e.g., `valkey.namespace.svc:6379`) | - |
| `--valkey-password` | Valkey authentication password | - |
| `--valkey-tls` | Enable TLS for Valkey connections | `false` |
| `--valkey-key-prefix` | Prefix for all Valkey keys | `mcp:` |
| `--valkey-db` | Valkey database number (0-15) | `0` |

#### Environment Variables

| Variable | Description |
|----------|-------------|
| `OAUTH_STORAGE_TYPE` | Storage backend: `memory` or `valkey` |
| `VALKEY_URL` | Valkey server address |
| `VALKEY_PASSWORD` | Valkey authentication password |
| `VALKEY_TLS_ENABLED` | Enable TLS (`true`/`false`) |
| `VALKEY_KEY_PREFIX` | Prefix for all Valkey keys |
| `VALKEY_DB` | Valkey database number |

### Example: Production Deployment with Valkey

```bash
# Deploy Valkey (example using Bitnami Helm chart)
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install valkey bitnami/valkey \
  --set auth.enabled=true \
  --set auth.password=your-secure-password \
  --set tls.enabled=true

# Start mcp-kubernetes with Valkey storage
mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=https://mcp.example.com \
  --oauth-provider=dex \
  --dex-issuer-url=https://dex.example.com \
  --dex-client-id=mcp-kubernetes \
  --dex-client-secret=$DEX_CLIENT_SECRET \
  --registration-token=$REGISTRATION_TOKEN \
  --oauth-encryption-key=$ENCRYPTION_KEY \
  --oauth-storage-type=valkey \
  --valkey-url=valkey.default.svc:6379 \
  --valkey-password=$VALKEY_PASSWORD \
  --valkey-tls
```

### Helm Chart Configuration

```yaml
mcpKubernetes:
  oauth:
    enabled: true
    baseURL: "https://mcp.example.com"
    provider: "dex"
    dex:
      issuerURL: "https://dex.example.com"
      clientID: "mcp-kubernetes"
    encryptionKey: true
    existingSecret: "mcp-kubernetes-oauth"
    
    storage:
      type: "valkey"
      valkey:
        url: "valkey.default.svc:6379"
        tls:
          enabled: true
        keyPrefix: "mcp:"
        # Password loaded from existingSecret
```

See `helm/mcp-kubernetes/values-oauth-valkey-example.yaml` for a complete production example.

### Security Considerations for Valkey

1. **Enable TLS** - Always use TLS in production to encrypt traffic between mcp-kubernetes and Valkey
2. **Use Authentication** - Configure Valkey password and store it in a Kubernetes Secret
3. **Enable Token Encryption** - Use `--oauth-encryption-key` to encrypt tokens at rest in Valkey
4. **Network Policies** - Restrict access to Valkey to only mcp-kubernetes pods
5. **Key Prefix** - Use unique key prefixes in multi-tenant environments
6. **Avoid CLI Password Flags** - Use environment variables (`VALKEY_PASSWORD`) instead of `--valkey-password` flag, as command-line arguments may be visible in process listings (`ps aux`)

### Valkey vs Redis

Valkey is a community-driven fork of Redis that maintains full compatibility while being fully open source. It provides:
- Redis protocol compatibility
- Active community development
- No licensing concerns
- Kubernetes-native deployment via Helm charts

## Architecture

The OAuth implementation is based on the [mcp-oauth](https://github.com/giantswarm/mcp-oauth) library (v0.2.14), which provides:

- OAuth 2.1 server implementation
- Multiple provider support:
  - Dex OIDC provider (with connector selection and groups claim)
  - Google OAuth provider
- Token storage backends:
  - In-memory storage (development)
  - Valkey storage (production, Redis-compatible)
- Security features (rate limiting, audit logging, encryption at rest)
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


