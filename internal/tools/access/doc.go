// Package access provides tools for checking user permissions on Kubernetes clusters.
//
// This package implements the "can_i" tool, which allows agents to query whether
// the authenticated user has permission to perform specific actions before attempting
// them. This provides better user experience by failing fast with clear error messages
// and reduces noise in Kubernetes audit logs from failed requests.
//
// # Security Model
//
// The can_i tool uses Kubernetes SelfSubjectAccessReview to check permissions.
// Because the MCP server uses user impersonation, the access check is performed
// as the authenticated user, not with elevated admin credentials.
//
// # Usage Examples
//
// Check if user can delete pods in a namespace:
//
//	{
//	  "verb": "delete",
//	  "resource": "pods",
//	  "namespace": "production"
//	}
//
// Check if user can create deployments cluster-wide:
//
//	{
//	  "verb": "create",
//	  "resource": "deployments",
//	  "apiGroup": "apps"
//	}
//
// Check access on a specific workload cluster:
//
//	{
//	  "verb": "list",
//	  "resource": "secrets",
//	  "namespace": "kube-system",
//	  "cluster": "prod-cluster-01"
//	}
package access
