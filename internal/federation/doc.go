// Package federation provides multi-cluster client management for the MCP Kubernetes server.
//
// This package enables the MCP server to operate across multiple Kubernetes clusters
// in a federated environment, specifically designed for Giant Swarm's Management Cluster
// and Workload Cluster architecture using Cluster API (CAPI).
//
// # Architecture Overview
//
// The federation package implements a "Hub-and-Spoke" model where:
//   - The Management Cluster (MC) acts as the central hub containing CAPI resources
//   - Workload Clusters (WC) are dynamically discovered and accessed through kubeconfig secrets
//   - All operations are executed under the authenticated user's identity via Kubernetes impersonation
//
// # Core Components
//
// ClusterClientManager is the primary interface for multi-cluster operations:
//
//	manager := federation.NewManager(localClient, localDynamic, logger)
//
//	// Get a client for a specific workload cluster
//	client, err := manager.GetClient(ctx, "my-cluster", userInfo)
//
//	// List all available clusters
//	clusters, err := manager.ListClusters(ctx, userInfo)
//
// # Security Model
//
// The federation package enforces security through:
//   - User impersonation: All cluster operations use Impersonate-User headers
//   - RBAC delegation: Authorization is delegated to each cluster's RBAC policies
//   - No credential exposure: Admin credentials are only used for TLS establishment
//
// # Thread Safety
//
// All operations in this package are thread-safe. The ClusterClientManager uses
// internal synchronization to handle concurrent access from multiple tool handlers.
//
// # Error Handling
//
// The package defines specific error types for common failure scenarios:
//   - ErrClusterNotFound: The requested cluster doesn't exist or is inaccessible
//   - ErrKubeconfigSecretNotFound: CAPI kubeconfig secret is missing
//   - ErrKubeconfigInvalid: Secret contains malformed kubeconfig data
//   - ErrConnectionFailed: Network or TLS issues connecting to the cluster
//
// # Example Usage
//
//	// Initialize the manager with the Management Cluster client
//	manager, err := federation.NewManager(localClient, localDynamic, logger)
//	if err != nil {
//		return err
//	}
//	defer manager.Close()
//
//	// Get user info from OAuth token
//	userInfo := &federation.UserInfo{
//		Email:  "user@example.com",
//		Groups: []string{"developers"},
//	}
//
//	// Get a client for a workload cluster with user impersonation
//	client, err := manager.GetClient(ctx, "production-cluster", userInfo)
//	if err != nil {
//		return fmt.Errorf("failed to get cluster client: %w", err)
//	}
//
//	// Use the client for Kubernetes operations
//	pods, err := client.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
//
// # Integration with MCP Server
//
// The federation package is designed to integrate with the ServerContext pattern:
//
//	serverCtx, err := server.NewServerContext(ctx,
//		server.WithK8sClient(k8sClient),
//		server.WithFederationManager(federationManager),
//	)
//
// Tool handlers can then access the federation manager to perform multi-cluster operations.
package federation
