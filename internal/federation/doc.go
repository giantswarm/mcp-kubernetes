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
// # User Impersonation
//
// The package implements Kubernetes User Impersonation to propagate authenticated user
// identity to Workload Clusters. Instead of executing operations with admin credentials,
// all API calls include impersonation headers that cause the Kubernetes API server to
// evaluate RBAC policies for the authenticated user.
//
// The impersonation configuration sets the following headers:
//
//	Impersonate-User: <user-email>           (e.g., "jane@giantswarm.io")
//	Impersonate-Group: <group-1>             (e.g., "github:org:giantswarm")
//	Impersonate-Group: <group-2>             (e.g., "platform-team")
//	Impersonate-Extra-agent: mcp-kubernetes  (audit trail identifier)
//	Impersonate-Extra-sub: <subject-id>      (OAuth subject claim)
//
// The "agent: mcp-kubernetes" extra header is automatically added to all impersonated
// requests. This enables audit log correlation to identify operations performed via
// the MCP server, distinguishing them from direct kubectl access.
//
// # Group Mapping Behavior
//
// OAuth groups are passed through to Kubernetes impersonation headers WITHOUT
// transformation. This ensures consistency between MCP-mediated access and direct
// kubectl access with the same identity. Common group formats:
//
//   - GitHub: "github:org:myorg", "github:team:platform"
//   - Azure AD: "azure:group:abc123-def456"
//   - LDAP: "ldap:group:cn=admins,dc=example,dc=com"
//   - System: "system:authenticated", "system:masters"
//
// Administrators should configure Workload Cluster RBAC policies to match the exact
// group strings provided by their identity provider through Dex.
//
// # OAuth Provider Trust Boundary
//
// The OAuth provider (e.g., Dex with GitHub/Azure AD/LDAP connectors) is a critical
// trust boundary in this architecture. The MCP server trusts the OAuth provider to:
//
//   - Accurately identify users (email claim)
//   - Correctly enumerate group memberships (groups claim)
//   - Not return privileged system groups unless the user is actually a member
//
// Security implications:
//
//   - If an OAuth provider is compromised and returns false group claims (e.g.,
//     "system:masters"), users could gain unintended cluster-admin privileges.
//   - This is consistent with direct kubectl access: the same risk exists when
//     users authenticate directly to clusters via OIDC.
//   - Defense: Configure your OAuth provider with appropriate access controls,
//     audit logs, and avoid mapping external groups directly to "system:masters".
//
// The agent header ("Impersonate-Extra-agent: mcp-kubernetes") is immutable and
// cannot be overridden by user-supplied OAuth claims. This ensures the audit trail
// always correctly identifies MCP-mediated access, even if other claims are manipulated.
//
// # RBAC Requirements for Impersonation
//
// For impersonation to work, the admin credentials in the CAPI kubeconfig secret
// must have permission to impersonate users and groups on the Workload Cluster:
//
//	apiVersion: rbac.authorization.k8s.io/v1
//	kind: ClusterRole
//	metadata:
//	  name: impersonate-all
//	rules:
//	  - apiGroups: [""]
//	    resources: ["users", "groups", "serviceaccounts"]
//	    verbs: ["impersonate"]
//	  - apiGroups: ["authentication.k8s.io"]
//	    resources: ["userextras/agent"]
//	    verbs: ["impersonate"]
//
// CAPI-generated admin credentials typically have these permissions by default.
//
// # Error Handling
//
// The package defines specific error types for common failure scenarios:
//   - ErrClusterNotFound: The requested cluster doesn't exist or is inaccessible
//   - ErrKubeconfigSecretNotFound: CAPI kubeconfig secret is missing
//   - ErrKubeconfigInvalid: Secret contains malformed kubeconfig data
//   - ErrConnectionFailed: Network or TLS issues connecting to the cluster
//   - ErrImpersonationFailed: User impersonation could not be configured
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
