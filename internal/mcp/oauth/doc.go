// Package oauth provides adapters for integrating the github.com/giantswarm/mcp-oauth
// library with the mcp-kubernetes MCP server.
//
// This package bridges the mcp-oauth library with our existing server architecture,
// providing token provider integration and configuration mapping for Kubernetes
// contexts that may require OAuth authentication.
//
// Dependency Security Note:
// This package depends on github.com/giantswarm/mcp-oauth for OAuth 2.1 implementation.
// The library provides: PKCE enforcement, refresh token rotation, rate limiting, and audit logging.
// Security posture: Actively maintained, implements OAuth 2.1 specification.
package oauth

