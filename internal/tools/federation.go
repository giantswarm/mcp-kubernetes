// Package tools provides shared utilities and types for MCP tool implementations.
package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// Error message constants for consistent user-facing error messages.
const (
	errMsgInvalidClusterName = "invalid cluster name provided"
)

// ClusterClient provides access to Kubernetes operations with multi-cluster support.
// It wraps either the local k8s.Client or federation-based clients.
//
// # Usage Pattern
//
// Tool handlers should use GetClusterClient to get the appropriate client:
//
//	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
//	if errMsg != "" {
//	    return mcp.NewToolResultError(errMsg), nil
//	}
//	// Use client.K8s() for standard operations, or client.User() for user info
type ClusterClient struct {
	// k8sClient is the local k8s.Client (used when cluster is empty or federation disabled)
	k8sClient k8s.Client

	// user is the authenticated user info (nil when using local client without federation)
	user *federation.UserInfo

	// clusterName is the target cluster name (empty for local cluster)
	clusterName string

	// federated indicates whether this client uses federation (vs local fallback)
	federated bool
}

// K8s returns the underlying Kubernetes client.
// This client is configured for the target cluster.
func (cc *ClusterClient) K8s() k8s.Client {
	return cc.k8sClient
}

// User returns the authenticated user info.
// Returns nil if federation is not enabled or if using local fallback.
func (cc *ClusterClient) User() *federation.UserInfo {
	return cc.user
}

// ClusterName returns the target cluster name (empty for local cluster).
func (cc *ClusterClient) ClusterName() string {
	return cc.clusterName
}

// IsFederated returns true if this client uses federation.
func (cc *ClusterClient) IsFederated() bool {
	return cc.federated
}

// GetClusterClient returns a ClusterClient for the specified cluster.
// If clusterName is empty, returns a client for the local cluster.
//
// # Federation Behavior
//
// When federation is enabled AND a cluster name is specified:
//   - Creates a FederatedClient using the federation manager
//   - The client is configured with user impersonation for security
//   - All operations are performed under the authenticated user's identity
//
// When federation is NOT enabled or cluster is empty:
//   - Returns the standard k8s client from ServerContext
//   - Does not require OAuth authentication
//
// # Return Values
//
// Returns (ClusterClient, "") on success or (nil, errorMessage) on failure.
// The error message is suitable for direct use in MCP tool responses.
func GetClusterClient(ctx context.Context, sc *server.ServerContext, clusterName string) (*ClusterClient, string) {
	slog.Debug("GetClusterClient called", slog.String("cluster", clusterName))
	fedManager := sc.FederationManager()

	// If a cluster is specified, validate it early to provide fast feedback
	// and prevent unnecessary processing with invalid input
	if clusterName != "" {
		if err := federation.ValidateClusterName(clusterName); err != nil {
			return nil, errMsgInvalidClusterName
		}
	}

	// If a cluster is specified but federation isn't enabled, return an error
	if clusterName != "" && fedManager == nil {
		return nil, "multi-cluster operations require federation mode to be enabled"
	}

	// If a cluster is specified, we need federation support
	if clusterName != "" {
		// Extract user info from OAuth context
		oauthUser, ok := oauth.UserInfoFromContext(ctx)
		if !ok || oauthUser == nil {
			return nil, "authentication required: no user info in context"
		}

		// Convert OAuth user info to federation UserInfo
		user := oauth.ToFederationUserInfo(oauthUser)
		if user == nil {
			return nil, "failed to convert user info for federation"
		}

		// Get clients from federation manager for the target cluster
		clientset, err := fedManager.GetClient(ctx, clusterName, user)
		if err != nil {
			slog.Warn("failed to get federated clientset",
				slog.String("cluster", clusterName),
				slog.Any("error", err))
			return nil, FormatClusterError(err, clusterName)
		}

		dynamicClient, err := fedManager.GetDynamicClient(ctx, clusterName, user)
		if err != nil {
			slog.Warn("failed to get federated dynamic client",
				slog.String("cluster", clusterName),
				slog.Any("error", err))
			return nil, FormatClusterError(err, clusterName)
		}

		restConfig, err := fedManager.GetRestConfig(ctx, clusterName, user)
		if err != nil {
			slog.Warn("failed to get federated rest config",
				slog.String("cluster", clusterName),
				slog.Any("error", err))
			return nil, FormatClusterError(err, clusterName)
		}

		// Create federated k8s.Client wrapper
		federatedClient, err := k8s.NewFederatedClient(&k8s.FederatedClientConfig{
			ClusterName:   clusterName,
			Clientset:     clientset,
			DynamicClient: dynamicClient,
			RestConfig:    restConfig,
		})
		if err != nil {
			slog.Error("failed to create federated client",
				slog.String("cluster", clusterName),
				slog.Any("error", err))
			return nil, "failed to initialize cluster client"
		}

		slog.Debug("created federated client",
			slog.String("cluster", clusterName))

		return &ClusterClient{
			k8sClient:   federatedClient,
			user:        user,
			clusterName: clusterName,
			federated:   true,
		}, ""
	}

	// No cluster specified - use local client
	k8sClient, err := sc.K8sClientForContext(ctx)
	if err != nil {
		// Authentication failed in strict mode
		return nil, FormatAuthenticationError(err)
	}
	return &ClusterClient{
		k8sClient:   k8sClient,
		clusterName: "",
		federated:   false,
	}, ""
}

// ExtractClusterParam extracts the cluster parameter from request arguments.
// Returns an empty string if not provided.
func ExtractClusterParam(args map[string]interface{}) string {
	if cluster, ok := args["cluster"].(string); ok {
		return cluster
	}
	return ""
}

// FormatClusterError formats a federation error into a user-friendly message.
// This function handles the various error types from the federation package
// and returns appropriate messages for MCP tool responses.
//
// # Security
//
// This function uses UserFacingError() methods from federation error types
// to ensure no internal details (cluster names, network topology) are leaked.
func FormatClusterError(err error, clusterName string) string {
	if err == nil {
		return ""
	}

	// Handle specific error types with custom user-facing messages
	var clusterNotFound *federation.ClusterNotFoundError
	if errors.As(err, &clusterNotFound) {
		return clusterNotFound.UserFacingError()
	}

	var kubeconfigErr *federation.KubeconfigError
	if errors.As(err, &kubeconfigErr) {
		return kubeconfigErr.UserFacingError()
	}

	var connectionErr *federation.ConnectionError
	if errors.As(err, &connectionErr) {
		return connectionErr.UserFacingError()
	}

	var impersonationErr *federation.ImpersonationError
	if errors.As(err, &impersonationErr) {
		return impersonationErr.UserFacingError()
	}

	var accessDeniedErr *federation.AccessDeniedError
	if errors.As(err, &accessDeniedErr) {
		return accessDeniedErr.UserFacingError()
	}

	var accessCheckErr *federation.AccessCheckError
	if errors.As(err, &accessCheckErr) {
		return accessCheckErr.UserFacingError()
	}

	var timeoutErr *federation.ConnectivityTimeoutError
	if errors.As(err, &timeoutErr) {
		return timeoutErr.UserFacingError()
	}

	var tlsErr *federation.TLSError
	if errors.As(err, &tlsErr) {
		return tlsErr.UserFacingError()
	}

	// Handle sentinel errors
	switch {
	case errors.Is(err, federation.ErrClusterNotFound):
		if clusterName != "" {
			return fmt.Sprintf("cluster '%s' not found - use 'capi_list_clusters' to see available clusters", clusterName)
		}
		return "cluster not found"
	case errors.Is(err, federation.ErrClusterUnreachable):
		return fmt.Sprintf("cluster '%s' is unreachable - check network connectivity", clusterName)
	case errors.Is(err, federation.ErrAccessDenied):
		return fmt.Sprintf("you don't have access to cluster '%s'", clusterName)
	case errors.Is(err, federation.ErrConnectionTimeout):
		return "connection to cluster timed out"
	case errors.Is(err, federation.ErrTLSHandshakeFailed):
		return "secure connection to cluster failed"
	case errors.Is(err, federation.ErrManagerClosed):
		return "federation manager is unavailable"
	case errors.Is(err, federation.ErrUserInfoRequired):
		return "authentication required for multi-cluster operations"
	case errors.Is(err, federation.ErrInvalidClusterName):
		return errMsgInvalidClusterName
	}

	// For unhandled errors, return a generic message that doesn't leak internal details.
	// The actual error should be logged server-side for debugging purposes.
	return "failed to access cluster: an unexpected error occurred"
}

// ValidateClusterParam validates that the cluster parameter can be used.
// Returns an error message if the cluster parameter is specified but
// federation is not enabled.
//
// This is a convenience function for handlers that don't yet support
// multi-cluster operations but want to provide clear error messages.
func ValidateClusterParam(sc *server.ServerContext, clusterName string) string {
	if clusterName == "" {
		return "" // No cluster specified, all good
	}

	fedManager := sc.FederationManager()
	if fedManager == nil {
		return "multi-cluster operations require federation mode to be enabled"
	}

	// Validate cluster name format
	if err := federation.ValidateClusterName(clusterName); err != nil {
		return errMsgInvalidClusterName
	}

	// Federation is enabled and cluster name is valid
	return ""
}

// FormatK8sError formats a Kubernetes operation error with optional impersonation context.
// When user info is available (federation mode), the impersonated user and groups are
// appended to help diagnose RBAC permission issues.
//
// Example output without impersonation:
//
//	"Failed to list resources: pods is forbidden: ..."
//
// Example output with impersonation:
//
//	"Failed to list resources: pods is forbidden: ... (impersonating user=fernando@example.com, groups=[org:giantswarm, team-platform])"
func FormatK8sError(prefix string, err error, user *federation.UserInfo) string {
	msg := fmt.Sprintf("%s: %v", prefix, err)
	if user != nil && user.Email != "" {
		msg += fmt.Sprintf(" (impersonating user=%s", user.Email)
		if len(user.Groups) > 0 {
			msg += fmt.Sprintf(", groups=[%s]", strings.Join(user.Groups, ", "))
		}
		msg += ")"
	}
	return msg
}
