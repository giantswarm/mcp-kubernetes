// Package oauth provides adapters for integrating the github.com/giantswarm/mcp-oauth
// library with the mcp-kubernetes MCP server.
//
// This package bridges the mcp-oauth library with our existing server architecture,
// providing token provider integration and configuration mapping for Kubernetes
// contexts that may require OAuth authentication.
//
// # User Info Integration
//
// This package provides convenience functions for accessing authenticated user
// information from request contexts:
//
//   - [UserInfoFromContext]: Retrieves the full UserInfo from context
//   - [HasUserInfo]: Checks if user info is present in context
//   - [GetUserEmailFromContext]: Extracts just the user's email
//   - [GetUserGroupsFromContext]: Extracts the user's group memberships
//
// For Kubernetes impersonation, use the conversion functions:
//
//   - [ToFederationUserInfo]: Converts OAuth UserInfo to federation.UserInfo
//   - [ToFederationUserInfoWithExtra]: Converts with additional claims
//   - [ValidateUserInfoForImpersonation]: Validates minimum required fields
//
// # Dependency Security Note
//
// This package depends on github.com/giantswarm/mcp-oauth for OAuth 2.1 implementation.
// The library provides: PKCE enforcement, refresh token rotation, rate limiting, and audit logging.
// Security posture: Actively maintained, implements OAuth 2.1 specification.
package oauth
