// Package k8s provides interfaces and types for Kubernetes operations.
//
// This package defines the core Client interface that abstracts all Kubernetes
// operations needed by the MCP tools. The interface is designed to support:
//
//   - Multi-cluster operations through context parameters
//   - All kubectl operations (get, list, describe, create, apply, delete, patch, scale)
//   - Pod-specific operations (logs, exec, port-forward)
//   - Cluster management (API resources, health checks)
//
// The interfaces are broken down into focused concerns:
//
//   - ContextManager: Kubernetes context operations
//   - ResourceManager: General resource CRUD operations
//   - PodManager: Pod-specific operations
//   - ClusterManager: Cluster-level operations
//
// All operations support multi-cluster scenarios by accepting kubeContext
// parameters, enabling the MCP server to work with multiple Kubernetes
// clusters simultaneously.
//
// Example usage:
//
//	// Get a pod from a specific cluster and namespace
//	pod, err := client.Get(ctx, "production", "default", "pod", "my-pod")
//	if err != nil {
//		return err
//	}
//
//	// List all deployments across all namespaces in a cluster
//	deployments, err := client.List(ctx, "staging", "", "deployment",
//		ListOptions{AllNamespaces: true})
//	if err != nil {
//		return err
//	}
//
//	// Get logs from a pod container
//	logs, err := client.GetLogs(ctx, "production", "default", "my-pod", "app",
//		LogOptions{Follow: true, TailLines: &tailLines})
//	if err != nil {
//		return err
//	}
//
// The package focuses on interface definitions and types, with concrete
// implementations provided in separate packages to maintain clean separation
// of concerns and enable easy testing through dependency injection.
package k8s
