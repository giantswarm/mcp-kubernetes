// Package capi provides MCP tools for CAPI (Cluster API) cluster discovery
// and navigation in multi-cluster environments.
//
// These tools allow AI agents to:
//   - List all workload clusters the user has access to
//   - Get detailed information about specific clusters
//   - Resolve fuzzy cluster name patterns
//   - Check cluster health status
//
// # Security Model
//
// All tools in this package respect the authenticated user's RBAC permissions
// via Kubernetes user impersonation. Operations are performed using the user's
// identity, ensuring that users can only see and interact with clusters they
// have permission to access.
//
// # Requirements
//
// These tools require:
//   - Federation mode to be enabled on the MCP server
//   - OAuth authentication configured for user identification
//   - CAPI CRDs installed on the Management Cluster
//
// # Example Usage
//
// List all clusters:
//
//	capi_list_clusters {}
//
// List clusters with filtering:
//
//	capi_list_clusters { "organization": "org-acme", "provider": "aws" }
//
// Get cluster details:
//
//	capi_get_cluster { "name": "prod-wc-01" }
//
// Resolve a fuzzy cluster name:
//
//	capi_resolve_cluster { "pattern": "prod" }
//
// Check cluster health:
//
//	capi_cluster_health { "name": "prod-wc-01" }
package capi
