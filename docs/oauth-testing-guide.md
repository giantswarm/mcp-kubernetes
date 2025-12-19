# OAuth Testing Guide

This guide provides step-by-step instructions for testing the OAuth 2.1 implementation in mcp-kubernetes using Dex OIDC provider.

## Table of Contents

1. [Quick Start: Local Testing with Dex OAuth](#quick-start-local-testing-with-dex-oauth)
2. [Testing with curl](#testing-with-curl)
3. [Testing Downstream OAuth (RBAC Passthrough)](#testing-downstream-oauth-rbac-passthrough)
4. [Troubleshooting](#troubleshooting)

---

## Quick Start: Local Testing with Dex OAuth

### Prerequisites

1. **Dex OIDC Server**:
   - You need a running Dex server with at least one connector configured
   - For testing, you can deploy Dex using Helm or Docker
   - Configure a test connector (GitHub, LDAP, mockCallback, etc.)

2. **Deploy Dex for Testing** (Optional - if you don't have Dex):

```bash
# Using Helm
helm repo add dex https://charts.dexidp.io
helm install dex dex/dex \
  --set config.issuer=http://localhost:5556/dex \
  --set config.staticClients[0].id=mcp-kubernetes \
  --set config.staticClients[0].name='MCP Kubernetes' \
  --set config.staticClients[0].secret=test-secret \
  --set config.staticClients[0].redirectURIs[0]=http://localhost:8080/oauth/callback

# Or using Docker with example config
docker run -d --name dex-local \
  -p 5556:5556 --entrypoint /usr/local/bin/dex \
  -v $(pwd)/dex-config.yaml:/etc/dex/config.yaml \
  ghcr.io/dexidp/dex:latest serve /etc/dex/config.yaml

```

Generate an HTTPS endpoint using ngrok and capture the URL:

```bash
# Start ngrok in the background and capture the HTTPS URL
ngrok http 5556 --log=stdout > /tmp/ngrok.log 2>&1 &
sleep 3  # Wait for ngrok to initialize
NGROK_URL=$(curl -s http://localhost:4040/api/tunnels | jq -r '.tunnels[] | select(.proto=="https") | .public_url')
echo "Ngrok HTTPS URL: $NGROK_URL"
```

3. **Example Dex Configuration** (`dex-config.yaml`):

**Note:** Update the `issuer` URL with your ngrok HTTPS URL once generated.

```yaml
issuer: https://<NGROK_URL>/dex  # Replace with your actual ngrok URL

storage:
  type: sqlite3
  config:
    file: /tmp/dex.db

web:
  http: 0.0.0.0:5556

staticClients:
- id: mcp-kubernetes
  name: 'MCP Kubernetes'
  secret: test-secret
  redirectURIs:
  - 'http://localhost:8080/oauth/callback'   # For local testing (HTTP)
  - 'https://localhost:8080/oauth/callback' # For local testing (HTTPS) - REQUIRED if using --oauth-base-url=https://localhost:8080
  - 'https://<MCP_NGROK_URL>/oauth/callback' # If using ngrok for mcp-kubernetes too

connectors:
- type: mockCallback
  id: mock
  name: Example Connector

oauth2:
  skipApprovalScreen: true

enablePasswordDB: true
staticPasswords:
- email: "admin@example.com"
  hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"  # password: password
  username: "admin"
  userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"
```

4. **Generate Test Tokens**:

```bash
# Generate registration token
export REGISTRATION_TOKEN=$(openssl rand -hex 32)
echo "Registration Token: $REGISTRATION_TOKEN"

# Generate encryption key (32 bytes, base64)
export OAUTH_ENCRYPTION_KEY=$(openssl rand -base64 32)
echo "Encryption Key: $OAUTH_ENCRYPTION_KEY"
```

### Step 1: Start the MCP Server with OAuth

```bash
# Set environment variables for Dex
export DEX_ISSUER_URL="https://<NGROK_URL>/dex"
export DEX_CLIENT_ID="mcp-kubernetes"
export DEX_CLIENT_SECRET="test-secret"
export DEX_CONNECTOR_ID="mock"  # Optional: bypasses connector selection
export REGISTRATION_TOKEN=$(openssl rand -hex 32)  # Generate a secure token
export OAUTH_ENCRYPTION_KEY=$(openssl rand -base64 32)

# IMPORTANT: Save the registration token - you'll need it for client registration
echo "Registration Token: $REGISTRATION_TOKEN"
echo "Save this token securely!"

# Start the server with Dex provider (default)
# NOTE: Use http:// for local testing (simpler) or https:// with TLS certificates
./mcp-kubernetes serve \
  --transport=streamable-http \
  --enable-oauth \
  --oauth-base-url=http://localhost:8080 \
  --registration-token=$REGISTRATION_TOKEN \
  --debug

# OR if you need HTTPS for local testing, generate self-signed certificates first:
# openssl req -x509 -newkey rsa:4096 -keyout /tmp/mcp-key.pem -out /tmp/mcp-cert.pem -days 365 -nodes -subj "/CN=localhost"
# Then use:
# ./mcp-kubernetes serve \
#   --transport=streamable-http \
#   --enable-oauth \
#   --oauth-base-url=https://localhost:8080 \
#   --tls-cert-file=/tmp/mcp-cert.pem \
#   --tls-key-file=/tmp/mcp-key.pem \
#   --registration-token=$REGISTRATION_TOKEN \
#   --debug
```

**⚠️ Important Notes:**

- The `--registration-token` flag **must** be set when starting the server
- The token in the `Authorization: Bearer` header must match this registration token
- Without this flag, all client registration requests will be rejected
- For testing only, you can use `--allow-public-registration=true` to skip token validation (NOT recommended)

The server should start and log:

```text
Using Dex OIDC provider
issuer: http://localhost:5556/dex
Token encryption at rest enabled (AES-256-GCM)
Security audit logging enabled
IP-based rate limiting enabled
User-based rate limiting enabled
Server listening on :8080
```

### Step 2: Verify OAuth Metadata Endpoints

Test that the OAuth server is properly configured:

```bash
# Check Authorization Server Metadata (RFC 8414)
curl -s http://localhost:8080/.well-known/oauth-authorization-server | jq

# Expected output includes:
# - issuer: "http://localhost:8080"
# - authorization_endpoint
# - token_endpoint
# - registration_endpoint
# - supported grant_types, response_types, etc.

# Check Protected Resource Metadata (RFC 9728)
curl -s http://localhost:8080/.well-known/oauth-protected-resource | jq
```

---

## Testing with curl

### Step 1: Register an OAuth Client

First, register a client that will use the OAuth flow:

**⚠️ Common Issue**: If you get an error like `"Public client registration is not enabled"`, make sure:

1. You started the server with `--registration-token=$REGISTRATION_TOKEN`
2. The `$REGISTRATION_TOKEN` variable is set and matches what you used to start the server
3. You're using the correct token in the `Authorization: Bearer` header

```bash
# Verify the registration token is set
echo "Using registration token: ${REGISTRATION_TOKEN:0:10}..."

# Register a new client
# NOTE: If you use "token_endpoint_auth_method": "none", you're registering a PUBLIC client
# which requires AllowPublicClientRegistration=true. For testing, omit this field to register
# a confidential client instead.
CLIENT_RESPONSE=$(curl -s -X POST http://localhost:8080/oauth/register \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $REGISTRATION_TOKEN" \
  -d '{
    "client_name": "Test MCP Client",
    "redirect_uris": ["http://localhost:3000/callback"],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "scope": "openid profile email groups offline_access"
  }')

echo "$CLIENT_RESPONSE" | jq

# Extract client_id
CLIENT_ID=$(echo "$CLIENT_RESPONSE" | jq -r '.client_id')
echo "Client ID: $CLIENT_ID"
```

**Expected output**:

```json
{
  "client_id": "generated-uuid",
  "client_id_issued_at": 1234567890,
  "client_name": "Test MCP Client",
  "redirect_uris": ["http://localhost:3000/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "client_secret_basic"
}
```

### Step 2: Generate PKCE Challenge

OAuth 2.1 requires PKCE (Proof Key for Code Exchange):

```bash
# Generate code verifier (random 43-128 character string)
export CODE_VERIFIER=$(openssl rand -base64 32 | tr -d '=+/' | cut -c1-43)
echo "Code Verifier: $CODE_VERIFIER"

# Generate code challenge (SHA256 hash of verifier, base64url encoded)
export CODE_CHALLENGE=$(echo -n "$CODE_VERIFIER" | openssl dgst -sha256 -binary | base64 | tr -d '=' | tr '+/' '-_')
echo "Code Challenge: $CODE_CHALLENGE"

# Generate state for CSRF protection
export STATE=$(openssl rand -hex 16)
echo "State: $STATE"
```

### Step 3: Authorization Request (Manual Browser Step)

Build the authorization URL and open it in a browser:

```bash
# Build authorization URL with Dex scopes
export AUTH_URL="http://localhost:8080/oauth/authorize?client_id=$CLIENT_ID&redirect_uri=http://localhost:3000/callback&response_type=code&scope=openid%20profile%20email%20groups%20offline_access&state=$STATE&code_challenge=$CODE_CHALLENGE&code_challenge_method=S256"

echo "Open this URL in your browser:"
echo "$AUTH_URL"
```

**What happens**:

1. Browser opens the URL
2. User is redirected to Dex OIDC authentication page
3. If `DEX_CONNECTOR_ID` is set, automatically uses that connector
4. Otherwise, shows connector selection screen
5. User authenticates (e.g., with mock connector: admin@example.com / password)
6. Dex redirects back to: `http://localhost:3000/callback?code=AUTH_CODE&state=$STATE`
7. Copy the `AUTH_CODE` from the redirect URL

### Step 4: Exchange Authorization Code for Tokens

```bash
# Set the authorization code from the callback
AUTH_CODE="paste-auth-code-here"

# Exchange code for access token
TOKEN_RESPONSE=$(curl -s -X POST http://localhost:8080/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=$AUTH_CODE&redirect_uri=http://localhost:3000/callback&client_id=$CLIENT_ID&code_verifier=$CODE_VERIFIER")

echo "$TOKEN_RESPONSE" | jq

# Extract tokens
ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
REFRESH_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.refresh_token')
ID_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.id_token')

echo "Access Token: $ACCESS_TOKEN"
echo "Refresh Token: $REFRESH_TOKEN"
echo "ID Token: $ID_TOKEN"
```

**Expected output**:

```json
{
  "access_token": "eyJhbGc...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "refresh_token_string",
  "id_token": "eyJhbGc...",
  "scope": "openid profile email groups offline_access"
}
```

### Step 5: Call MCP Tools with Access Token

Now use the access token to call MCP endpoints:

```bash
# List pods in default namespace
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
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
  }' | jq
```

**Expected output**: JSON-RPC response with pod list

### Step 6: Token Introspection

Verify token is valid:

```bash
curl -X POST http://localhost:8080/oauth/introspect \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=$ACCESS_TOKEN" | jq
```

**Expected output**:

```json
{
  "active": true,
  "scope": "openid profile email groups offline_access",
  "client_id": "your-client-id",
  "exp": 1234567890,
  "iat": 1234567890,
  "sub": "admin@example.com",
  "email": "admin@example.com",
  "groups": ["admins"]
}
```

### Step 7: Token Refresh

Use refresh token to get a new access token:

```bash
NEW_TOKEN_RESPONSE=$(curl -s -X POST http://localhost:8080/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token&refresh_token=$REFRESH_TOKEN&client_id=$CLIENT_ID")

echo "$NEW_TOKEN_RESPONSE" | jq

# Extract new tokens (refresh token may rotate)
NEW_ACCESS_TOKEN=$(echo "$NEW_TOKEN_RESPONSE" | jq -r '.access_token')
NEW_REFRESH_TOKEN=$(echo "$NEW_TOKEN_RESPONSE" | jq -r '.refresh_token')
```

### Step 8: Token Revocation

Revoke the refresh token:

```bash
curl -X POST http://localhost:8080/oauth/revoke \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=$REFRESH_TOKEN&client_id=$CLIENT_ID"

# Response: HTTP 200 OK with empty body
```

---

## Troubleshooting

### Issue 1: "Unregistered redirect_uri" Error from Dex

**Error Message**:
```
msg="failed to parse authorization request" err="Unregistered redirect_uri (\"https://localhost:8080/oauth/callback\")."
```

**Root Cause**: Dex doesn't have the redirect URI registered for the mcp-kubernetes client. The redirect URI is automatically constructed from your `--oauth-base-url` + `/oauth/callback`.

**Solution**:

1. **Check your `--oauth-base-url` flag**:
   ```bash
   # If you're using HTTPS:
   --oauth-base-url=https://localhost:8080
   
   # Then Dex MUST have this redirect URI:
   redirectURIs:
   - 'https://localhost:8080/oauth/callback'
   
   # If you're using HTTP:
   --oauth-base-url=http://localhost:8080
   
   # Then Dex MUST have this redirect URI:
   redirectURIs:
   - 'http://localhost:8080/oauth/callback'
   ```

2. **Update Dex configuration** (`/tmp/dex-config.yaml`):
   ```yaml
   staticClients:
   - id: mcp-kubernetes
     name: 'MCP Kubernetes'
     secret: test-secret
     redirectURIs:
     - 'http://localhost:8080/oauth/callback'   # For HTTP
     - 'https://localhost:8080/oauth/callback'  # For HTTPS (if using --oauth-base-url=https://...)
   ```

3. **Restart Dex container** to pick up the new config:
   ```bash
   docker restart dex-local
   ```

4. **Verify the config was loaded**:
   ```bash
   docker exec dex-local cat /etc/dex/config.yaml | grep -A 5 "staticClients:"
   ```

5. **Check mcp-kubernetes is using the correct Dex client ID**:
   ```bash
   # Your mcp-kubernetes should be started with:
   --dex-client-id=mcp-kubernetes
   
   # This must match the client ID in Dex config:
   staticClients:
   - id: mcp-kubernetes  # <-- Must match
   ```

6. **Verify the redirect URI matches exactly** (including scheme, host, port, and path):
   ```bash
   # Check what mcp-kubernetes is using:
   ps aux | grep "mcp-kubernetes serve" | grep "oauth-base-url"
   
   # The redirect URI will be: <oauth-base-url>/oauth/callback
   # Example: https://localhost:8080/oauth/callback
   ```

**Common Mistakes**:
- ❌ Using `--oauth-base-url=https://localhost:8080` but Dex only has `http://localhost:8080/oauth/callback` registered
- ❌ Using `--dex-client-id=something-else` but Dex config has `id: mcp-kubernetes`
- ❌ Forgetting to restart Dex after updating the config file
- ❌ Config file not mounted correctly in Docker container (check with `docker inspect dex-local`)
- ❌ Redirect URI has trailing slash or different port than configured

**Quick Fix**: Add both HTTP and HTTPS redirect URIs to Dex config to cover both scenarios:
```yaml
redirectURIs:
- 'http://localhost:8080/oauth/callback'
- 'https://localhost:8080/oauth/callback'
```

### Issue 2: SSL_ERROR_RX_RECORD_TOO_LONG Error

**Error Message**:
```
An error occurred during a connection to localhost:8080. SSL received a record that exceeded the maximum permissible length.
Error code: SSL_ERROR_RX_RECORD_TOO_LONG
```

**Root Cause**: You're using `--oauth-base-url=https://localhost:8080` but the server is running without TLS certificates (HTTP only). The browser tries to connect via HTTPS, but the server is only listening on HTTP.

**Solution**:

You have two options:

**Option A: Use HTTP for Local Testing (Simplest)**

1. **Change `--oauth-base-url` to use HTTP**:
   ```bash
   ./mcp-kubernetes serve \
     --transport=streamable-http \
     --enable-oauth \
     --oauth-base-url=http://localhost:8080 \  # Changed to http://
     --registration-token=$REGISTRATION_TOKEN \
     --debug
   ```

2. **Update Dex redirect URI to match**:
   ```yaml
   redirectURIs:
   - 'http://localhost:8080/oauth/callback'  # Use HTTP
   ```

3. **Restart both services**:
   ```bash
   docker restart dex-local
   # Restart mcp-kubernetes with the new --oauth-base-url
   ```

**Option B: Enable HTTPS with Self-Signed Certificates**

1. **Generate self-signed certificates**:
   ```bash
   openssl req -x509 -newkey rsa:4096 \
     -keyout /tmp/mcp-key.pem \
     -out /tmp/mcp-cert.pem \
     -days 365 -nodes \
     -subj "/CN=localhost"
   ```

2. **Start server with TLS certificates**:
   ```bash
   ./mcp-kubernetes serve \
     --transport=streamable-http \
     --enable-oauth \
     --oauth-base-url=https://localhost:8080 \
     --tls-cert-file=/tmp/mcp-cert.pem \
     --tls-key-file=/tmp/mcp-key.pem \
     --registration-token=$REGISTRATION_TOKEN \
     --debug
   ```

3. **Accept the self-signed certificate in your browser**:
   - When you first visit `https://localhost:8080`, your browser will warn about the self-signed certificate
   - Click "Advanced" → "Accept the Risk and Continue" (or similar)
   - This is safe for local testing only

4. **Update Dex redirect URI to use HTTPS**:
   ```yaml
   redirectURIs:
   - 'https://localhost:8080/oauth/callback'
   ```

**Common Mistakes**:
- ❌ Using `--oauth-base-url=https://localhost:8080` without `--tls-cert-file` and `--tls-key-file`
- ❌ Forgetting to restart the server after changing `--oauth-base-url`
- ❌ Not updating Dex redirect URIs to match the scheme (http vs https)
- ❌ Browser caching old redirect URLs (clear browser cache or use incognito mode)

**Quick Check**: Verify your server is actually listening on HTTPS:
```bash
# If using HTTPS, you should see TLS in the connection:
curl -k https://localhost:8080/.well-known/oauth-authorization-server

# If using HTTP, this should work:
curl http://localhost:8080/.well-known/oauth-authorization-server
```
