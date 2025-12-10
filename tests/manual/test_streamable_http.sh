#!/bin/bash
# Test script for mcp-kubernetes streamable-http transport
# This script helps diagnose issues with the MCP server

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=== MCP Kubernetes Streamable HTTP Test ==="
echo ""

# Check if server is running
check_server() {
    local url="${1:-http://localhost:8080}"
    echo -n "Checking server at $url... "
    if curl -s -o /dev/null -w "%{http_code}" "$url/healthz" | grep -q "200"; then
        echo -e "${GREEN}OK${NC}"
        return 0
    else
        echo -e "${RED}FAILED${NC}"
        return 1
    fi
}

# Test MCP endpoint without auth (should return 401 or error)
test_mcp_no_auth() {
    local url="${1:-http://localhost:8080}"
    echo ""
    echo "=== Test 1: MCP endpoint without auth ==="
    echo "POST $url/mcp"
    
    response=$(curl -s -X POST "$url/mcp" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}')
    
    echo "Response: $response"
    
    if echo "$response" | grep -q "invalid_token\|Missing Authorization"; then
        echo -e "${GREEN}PASS: Auth required as expected${NC}"
    else
        echo -e "${YELLOW}WARN: Unexpected response (might be OK if OAuth disabled)${NC}"
    fi
}

# Test MCP initialize (stdio mode - no auth required)
test_mcp_initialize_stdio() {
    local url="${1:-http://localhost:8080}"
    echo ""
    echo "=== Test 2: MCP initialize (streamable-http, simulated) ==="
    
    # For streamable-http, we need a session
    # First, let's see if the endpoint responds at all
    echo "Testing basic connectivity..."
    
    response=$(curl -s -X POST "$url/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        --max-time 5 \
        -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}' 2>&1)
    
    echo "Response: $response"
}

# Test with a valid Bearer token (for OAuth-enabled server)
test_mcp_with_token() {
    local url="${1:-http://localhost:8080}"
    local token="${2:-}"
    
    if [ -z "$token" ]; then
        echo ""
        echo "=== Test 3: Skipped (no token provided) ==="
        echo "To test with OAuth, provide a Bearer token as second argument"
        return 0
    fi
    
    echo ""
    echo "=== Test 3: MCP initialize with Bearer token ==="
    echo "POST $url/mcp"
    
    response=$(curl -s -X POST "$url/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -H "Authorization: Bearer $token" \
        --max-time 10 \
        -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}')
    
    echo "Response: $response"
    
    if echo "$response" | grep -q '"result"'; then
        echo -e "${GREEN}PASS: Initialize succeeded${NC}"
        return 0
    else
        echo -e "${RED}FAIL: Initialize failed${NC}"
        return 1
    fi
}

# Test tools/list
test_tools_list() {
    local url="${1:-http://localhost:8080}"
    local token="${2:-}"
    local session_id="${3:-}"
    
    echo ""
    echo "=== Test 4: List tools ==="
    
    local auth_header=""
    if [ -n "$token" ]; then
        auth_header="-H \"Authorization: Bearer $token\""
    fi
    
    local session_header=""
    if [ -n "$session_id" ]; then
        session_header="-H \"Mcp-Session-Id: $session_id\""
    fi
    
    response=$(curl -s -X POST "$url/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        ${token:+-H "Authorization: Bearer $token"} \
        ${session_id:+-H "Mcp-Session-Id: $session_id"} \
        --max-time 10 \
        -d '{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}')
    
    echo "Response (first 500 chars): ${response:0:500}"
    
    if echo "$response" | grep -q '"tools"'; then
        echo -e "${GREEN}PASS: Tools list succeeded${NC}"
    else
        echo -e "${RED}FAIL: Tools list failed${NC}"
    fi
}

# Test call tool (kubernetes_list)
test_call_tool() {
    local url="${1:-http://localhost:8080}"
    local token="${2:-}"
    local session_id="${3:-}"
    
    echo ""
    echo "=== Test 5: Call kubernetes_list tool ==="
    
    response=$(curl -s -X POST "$url/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        ${token:+-H "Authorization: Bearer $token"} \
        ${session_id:+-H "Mcp-Session-Id: $session_id"} \
        --max-time 30 \
        -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"kubernetes_list","arguments":{"namespace":"default","resourceType":"pods"}},"id":3}')
    
    echo "Response (first 1000 chars): ${response:0:1000}"
    
    if echo "$response" | grep -q '"result"\|"content"'; then
        echo -e "${GREEN}PASS: Tool call succeeded${NC}"
    elif echo "$response" | grep -q '"error"'; then
        echo -e "${YELLOW}WARN: Tool returned error (might be expected)${NC}"
    else
        echo -e "${RED}FAIL: Tool call failed or hung${NC}"
    fi
}

# Main
main() {
    local url="${1:-http://localhost:8080}"
    local token="${2:-}"
    
    echo "Server URL: $url"
    echo "Token: ${token:+<provided>}"
    echo ""
    
    if ! check_server "$url"; then
        echo ""
        echo "Server not running. Start it with:"
        echo "  cd $PROJECT_ROOT"
        echo "  go run . serve --transport=streamable-http --debug"
        echo ""
        echo "Or with OAuth (for testing OAuth flow):"
        echo "  go run . serve --transport=streamable-http --debug --enable-oauth --oauth-base-url=http://localhost:8080 ..."
        exit 1
    fi
    
    test_mcp_no_auth "$url"
    test_mcp_initialize_stdio "$url"
    test_mcp_with_token "$url" "$token"
    test_tools_list "$url" "$token"
    test_call_tool "$url" "$token"
    
    echo ""
    echo "=== Tests complete ==="
}

main "$@"

