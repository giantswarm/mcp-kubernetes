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
//	// Create an OAuthClientProvider for OAuth downstream mode
//	oauthProvider, err := federation.NewOAuthClientProviderFromInCluster()
//	if err != nil {
//		return err
//	}
//
//	// Configure the token extractor to get OAuth tokens from context
//	oauthProvider.SetTokenExtractor(oauth.GetAccessTokenFromContext)
//
//	manager, err := federation.NewManager(oauthProvider,
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
// # Group Handling and Mapping
//
// By default, OAuth groups are passed through to Kubernetes impersonation headers
// WITHOUT transformation. This ensures consistency between MCP-mediated access and
// direct kubectl access with the same identity. Common group formats:
//
//   - GitHub: "github:org:myorg", "github:team:platform"
//   - Azure AD: "azure:group:abc123-def456"
//   - LDAP: "ldap:group:cn=admins,dc=example,dc=com"
//   - System: "system:authenticated", "system:masters"
//
// However, the OIDC provider may return group identifiers in a different format than
// what the workload cluster RoleBindings expect. This is especially common when a
// federation broker like Dex sits between mcp-kubernetes and the upstream IdP. For
// example, Dex may return Azure AD group display names while workload clusters use
// Azure AD group GUIDs for RoleBindings.
//
// To handle this, mcp-kubernetes supports configurable group mapping via the
// GroupMapper type. When configured (via WithGroupMapper option or the
// WC_GROUP_MAPPINGS environment variable), group identifiers are translated before
// setting Impersonate-Group headers:
//
//   - Mapped groups: translated to their target identifiers
//   - Unmapped groups: passed through unchanged (backward compatible)
//   - Each translation is logged at Info level for operational visibility
//   - Mapping to dangerous system groups (e.g., system:masters) is rejected at startup
//
// Group mapping is only applied in impersonation mode. In SSO passthrough mode,
// the workload cluster's own OIDC configuration handles group resolution.
//
// Security note: group mappings can change the effective permissions of users on
// workload clusters. Whoever controls the mapping configuration (Helm values or env
// var) controls which Kubernetes groups users are impersonated into. Mapping to
// dangerous system groups (system:masters, system:nodes, system:kube-controller-manager,
// system:kube-scheduler, system:kube-proxy) is blocked at startup. Other system:*
// targets produce a warning. Malformed WC_GROUP_MAPPINGS JSON will fail startup
// (fail-closed) rather than silently starting without mappings.
// Reconstructing a full audit trail for mapped requests requires correlating
// mcp-kubernetes application logs (which record translations) with the Kubernetes
// audit log of the target workload cluster (which records the resulting API calls).
//
// Example Helm values configuration:
//
//	capiMode:
//	  workloadClusterAuth:
//	    groupMappings: '{"customer:Platform Engineers": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"}'
//
// Administrators should configure Workload Cluster RBAC policies to match the exact
// group strings provided by their identity provider through Dex, or use group mapping
// when the formats differ.
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
// # Pre-flight Access Checks
//
// The package provides SubjectAccessReview (SAR) pre-flight checks to verify user
// permissions before attempting operations:
//
//	// Check if user can delete pods before attempting
//	result, err := manager.CheckAccess(ctx, "prod-cluster", user, &AccessCheck{
//		Verb:      "delete",
//		Resource:  "pods",
//		Namespace: "production",
//	})
//	if err != nil {
//		return err // Check failed (API error, etc.)
//	}
//	if !result.Allowed {
//		return fmt.Errorf("permission denied: %s", result.Reason)
//	}
//	// Proceed with delete...
//
// Or use the convenience method that returns an error if denied:
//
//	if err := manager.CheckAccessAllowed(ctx, cluster, user, check); err != nil {
//		return err // Either check failed or access denied
//	}
//
// The "can_i" tool (in internal/tools/access) exposes this functionality to AI agents,
// allowing them to query permissions before attempting operations.
//
// # Error Handling
//
// The package defines specific error types for common failure scenarios:
//   - ErrClusterNotFound: The requested cluster doesn't exist or is inaccessible
//   - ErrKubeconfigSecretNotFound: CAPI kubeconfig secret is missing
//   - ErrKubeconfigInvalid: Secret contains malformed kubeconfig data
//   - ErrConnectionFailed: Network or TLS issues connecting to the cluster
//   - ErrImpersonationFailed: User impersonation could not be configured
//   - ErrAccessDenied: User lacks RBAC permissions for the operation
//   - ErrAccessCheckFailed: The SAR check itself failed (not denied, but error)
//   - ErrInvalidAccessCheck: Invalid AccessCheck parameters
//
// All user-facing errors return a generic message ("cluster access denied or unavailable")
// to prevent information leakage that could enable cluster enumeration attacks.
//
// # Example Usage
//
//	// Create an OAuthClientProvider for OAuth downstream mode
//	oauthProvider, err := federation.NewOAuthClientProviderFromInCluster()
//	if err != nil {
//		return err
//	}
//
//	// Configure the token extractor (uses OAuth middleware's context storage)
//	oauthProvider.SetTokenExtractor(oauth.GetAccessTokenFromContext)
//
//	// Initialize the manager with the OAuth provider
//	manager, err := federation.NewManager(oauthProvider,
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
//	// Get user info from OAuth token (set by OAuth middleware)
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
