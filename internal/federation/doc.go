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
//   - All operations are executed under the authenticated user's identity
//
// # Core Components
//
// ClusterClientManager is the primary interface for multi-cluster operations:
//
//	// Create a ClientProvider that returns per-user clients
//	clientProvider := &OAuthClientProvider{factory: bearerTokenFactory}
//
//	manager, err := federation.NewManager(clientProvider,
//		federation.WithManagerLogger(logger),
//	)
//	if err != nil {
//		return err
//	}
//	defer manager.Close()
//
//	// Get a client for a specific workload cluster
//	client, err := manager.GetClient(ctx, "my-cluster", userInfo)
//
//	// List all available clusters
//	clusters, err := manager.ListClusters(ctx, userInfo)
//
// # Security Model
//
// The federation package enforces security through defense in depth:
//
//   - ClientProvider: Creates per-user Kubernetes clients for Management Cluster access.
//     When OAuth downstream is enabled, each user's OAuth token is used for authentication.
//
//   - MC RBAC Enforcement: Users must have permission to list CAPI Cluster resources and
//     read kubeconfig secrets on the Management Cluster.
//
//   - WC RBAC Enforcement: Operations on Workload Clusters use impersonation headers,
//     delegating authorization to each cluster's RBAC policies.
//
//   - User Isolation in Caching: Cached clients are keyed by (cluster, user) pairs.
//
// This two-layer security model ensures that users can only access clusters they have
// permission to see on the MC, AND they can only perform operations allowed by their
// identity on each WC.
//
// # Client Caching
//
// The package implements a thread-safe client cache with TTL-based eviction to
// optimize performance. Key security properties:
//
//   - User Isolation: Each cached client is keyed by both cluster name AND user email,
//     ensuring user A can never retrieve a client configured for user B.
//
//   - TTL Expiration: Cached clients expire after a configurable TTL (default: 10 minutes).
//     Set this to be less than or equal to your OAuth token lifetime.
//
//   - Manual Invalidation: Use DeleteByCluster() when cluster credentials are rotated,
//     or implement token revocation callbacks to remove specific user entries.
//
//   - PII Protection: User emails are anonymized in logs using SHA-256 hashing.
//
// # Metrics
//
// The cache exposes the following metrics for monitoring:
//   - mcp_client_cache_hits_total: Cache hit count (by cluster)
//   - mcp_client_cache_misses_total: Cache miss count (by cluster)
//   - mcp_client_cache_evictions_total: Eviction count (by reason: expired, lru, manual)
//   - mcp_client_cache_entries: Current cache size (gauge)
//
// Note: The "cluster" label on hit/miss metrics may have high cardinality in
// environments with many clusters. Monitor your metrics backend capacity.
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
// All user-facing errors return a generic message ("cluster access denied or unavailable")
// to prevent information leakage that could enable cluster enumeration attacks.
//
// # Example Usage
//
//	// Create a ClientProvider for OAuth downstream mode
//	clientProvider := &OAuthClientProvider{
//		factory: bearerTokenFactory,
//	}
//
//	// Initialize the manager with the ClientProvider
//	manager, err := federation.NewManager(clientProvider,
//		federation.WithManagerLogger(logger),
//		federation.WithManagerCacheConfig(federation.CacheConfig{
//			TTL:        10 * time.Minute,
//			MaxEntries: 1000,
//		}),
//	)
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
