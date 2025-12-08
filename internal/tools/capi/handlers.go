package capi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// handleListClusters handles the capi_list_clusters tool request.
// It lists all CAPI clusters the user has access to, with optional filtering.
func handleListClusters(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Get federation manager
	fedManager := sc.FederationManager()
	if fedManager == nil {
		return mcp.NewToolResultError("federation mode is not enabled"), nil
	}

	// Get authenticated user
	user, errMsg := getUserFromContext(ctx)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}

	// Extract filter parameters
	args := request.GetArguments()
	organization, _ := args["organization"].(string)
	provider, _ := args["provider"].(string)
	status, _ := args["status"].(string)
	readyOnly, _ := args["readyOnly"].(bool)
	labelSelector, _ := args["labelSelector"].(string)

	// Build list options from filter parameters
	var opts *federation.ClusterListOptions
	if organization != "" || provider != "" || status != "" || readyOnly || labelSelector != "" {
		opts = &federation.ClusterListOptions{
			Namespace:     organization,
			Provider:      strings.ToLower(provider),
			LabelSelector: labelSelector,
			ReadyOnly:     readyOnly,
		}
		if status != "" {
			opts.Status = federation.ClusterPhase(status)
		}
	}

	// List clusters
	clusters, err := listClustersWithOptions(ctx, fedManager, user, opts)
	if err != nil {
		return handleFederationError(err, "list clusters")
	}

	// Convert to output format
	output := ClusterListOutput{
		Clusters:      make([]ClusterListItem, 0, len(clusters)),
		TotalCount:    len(clusters),
		FilterApplied: opts != nil,
	}

	for _, cluster := range clusters {
		output.Clusters = append(output.Clusters, clusterSummaryToListItem(cluster))
	}

	return formatJSONResult(output)
}

// handleGetCluster handles the capi_get_cluster tool request.
// It returns detailed information about a specific cluster.
func handleGetCluster(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Get federation manager
	fedManager := sc.FederationManager()
	if fedManager == nil {
		return mcp.NewToolResultError("federation mode is not enabled"), nil
	}

	// Get authenticated user
	user, errMsg := getUserFromContext(ctx)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}

	// Extract required name parameter
	args := request.GetArguments()
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}

	// Get cluster summary
	cluster, err := fedManager.GetClusterSummary(ctx, name, user)
	if err != nil {
		return handleFederationError(err, "get cluster")
	}

	// Convert to detailed output format
	output := clusterSummaryToDetail(cluster)

	return formatJSONResult(output)
}

// handleResolveCluster handles the capi_resolve_cluster tool request.
// It resolves a partial cluster name pattern to its full identifier.
func handleResolveCluster(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Get federation manager
	fedManager := sc.FederationManager()
	if fedManager == nil {
		return mcp.NewToolResultError("federation mode is not enabled"), nil
	}

	// Get authenticated user
	user, errMsg := getUserFromContext(ctx)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}

	// Extract required pattern parameter
	args := request.GetArguments()
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return mcp.NewToolResultError("pattern parameter is required"), nil
	}

	// Get all clusters and resolve the pattern
	clusters, err := fedManager.ListClusters(ctx, user)
	if err != nil {
		return handleFederationError(err, "resolve cluster")
	}

	// Try to resolve the pattern
	resolved, matches := resolveClusterPattern(clusters, pattern)

	switch len(matches) {
	case 0:
		output := ClusterResolveOutput{
			Resolved: false,
			Message:  fmt.Sprintf("No clusters match pattern '%s'. Use capi_list_clusters to see available clusters.", pattern),
		}
		return formatJSONResult(output)
	case 1:
		item := clusterSummaryToListItem(matches[0])
		output := ClusterResolveOutput{
			Resolved: true,
			Cluster:  &item,
			Message:  fmt.Sprintf("Pattern '%s' resolved to cluster '%s' in namespace '%s'.", pattern, resolved.Name, resolved.Namespace),
		}
		return formatJSONResult(output)
	default:
		// Multiple matches found
		output := ClusterResolveOutput{
			Resolved: false,
			Matches:  make([]ClusterListItem, 0, len(matches)),
			Message:  fmt.Sprintf("Multiple clusters match pattern '%s'. Please use a more specific name.", pattern),
		}
		for _, match := range matches {
			output.Matches = append(output.Matches, clusterSummaryToListItem(match))
		}
		return formatJSONResult(output)
	}
}

// resolveClusterPattern resolves a pattern to cluster(s).
// Returns the exact match (if found) and all matching clusters.
func resolveClusterPattern(clusters []federation.ClusterSummary, pattern string) (*federation.ClusterSummary, []federation.ClusterSummary) {
	// Try exact match first
	for i := range clusters {
		if clusters[i].Name == pattern {
			return &clusters[i], []federation.ClusterSummary{clusters[i]}
		}
	}

	// Try partial match (case-insensitive contains)
	patternLower := strings.ToLower(pattern)
	var matches []federation.ClusterSummary
	for _, cluster := range clusters {
		if strings.Contains(strings.ToLower(cluster.Name), patternLower) {
			matches = append(matches, cluster)
		}
	}

	if len(matches) == 1 {
		return &matches[0], matches
	}
	return nil, matches
}

// handleClusterHealth handles the capi_cluster_health tool request.
// It returns health information about a specific cluster.
func handleClusterHealth(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Get federation manager
	fedManager := sc.FederationManager()
	if fedManager == nil {
		return mcp.NewToolResultError("federation mode is not enabled"), nil
	}

	// Get authenticated user
	user, errMsg := getUserFromContext(ctx)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}

	// Extract required name parameter
	args := request.GetArguments()
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}

	// Get cluster summary (contains status information)
	cluster, err := fedManager.GetClusterSummary(ctx, name, user)
	if err != nil {
		return handleFederationError(err, "check cluster health")
	}

	// Build health output from cluster status
	output := buildHealthOutput(cluster)

	return formatJSONResult(output)
}

// getUserFromContext extracts the authenticated user from the context.
// Returns the federation.UserInfo and an empty string on success,
// or nil and an error message on failure.
func getUserFromContext(ctx context.Context) (*federation.UserInfo, string) {
	oauthUser, ok := oauth.UserInfoFromContext(ctx)
	if !ok || oauthUser == nil {
		return nil, "authentication required: no user info in context"
	}

	// Validate user info for impersonation
	if err := oauth.ValidateUserInfoForImpersonation(oauthUser); err != nil {
		return nil, fmt.Sprintf("authentication error: %v", err)
	}

	// Convert to federation user info
	user := oauth.ToFederationUserInfo(oauthUser)
	if user == nil {
		return nil, "failed to convert user info for federation"
	}

	return user, ""
}

// listClustersWithOptions lists clusters with optional filtering.
// This is a helper that wraps the federation manager's ListClusters method
// with support for the ClusterListOptions filtering.
func listClustersWithOptions(ctx context.Context, fedManager federation.ClusterClientManager, user *federation.UserInfo, opts *federation.ClusterListOptions) ([]federation.ClusterSummary, error) {
	// Use the federation manager's ListClusters method
	// Note: The federation manager already supports filtering via ListClustersWithOptions
	// but that method is internal. We use ListClusters and apply additional filters client-side.
	clusters, err := fedManager.ListClusters(ctx, user)
	if err != nil {
		return nil, err
	}

	// If no options, return all clusters
	if opts == nil {
		return clusters, nil
	}

	// Apply client-side filters
	filtered := make([]federation.ClusterSummary, 0, len(clusters))
	for _, cluster := range clusters {
		// Filter by namespace/organization
		if opts.Namespace != "" && cluster.Namespace != opts.Namespace {
			continue
		}

		// Filter by provider
		if opts.Provider != "" && strings.ToLower(cluster.Provider) != opts.Provider {
			continue
		}

		// Filter by status
		if opts.Status != "" && cluster.Status != string(opts.Status) {
			continue
		}

		// Filter by ready status
		if opts.ReadyOnly && !cluster.Ready {
			continue
		}

		filtered = append(filtered, cluster)
	}

	return filtered, nil
}

// handleFederationError converts federation errors to user-friendly tool results.
func handleFederationError(err error, operation string) (*mcp.CallToolResult, error) {
	if err == nil {
		return nil, nil
	}

	// Handle specific federation error types
	var clusterNotFound *federation.ClusterNotFoundError
	if errors.As(err, &clusterNotFound) {
		return mcp.NewToolResultError(clusterNotFound.UserFacingError()), nil
	}

	var discoveryErr *federation.ClusterDiscoveryError
	if errors.As(err, &discoveryErr) {
		return mcp.NewToolResultError(discoveryErr.UserFacingError()), nil
	}

	var accessDeniedErr *federation.AccessDeniedError
	if errors.As(err, &accessDeniedErr) {
		return mcp.NewToolResultError(accessDeniedErr.UserFacingError()), nil
	}

	// Handle sentinel errors
	switch {
	case errors.Is(err, federation.ErrUserInfoRequired):
		return mcp.NewToolResultError("authentication required for CAPI operations"), nil
	case errors.Is(err, federation.ErrManagerClosed):
		return mcp.NewToolResultError("federation manager is unavailable"), nil
	case errors.Is(err, federation.ErrCAPICRDNotInstalled):
		return mcp.NewToolResultError("CAPI is not installed on this management cluster"), nil
	}

	// Generic error message that doesn't leak internal details
	return mcp.NewToolResultError(fmt.Sprintf("failed to %s: an unexpected error occurred", operation)), nil
}

// formatJSONResult marshals the output to JSON and returns a tool result.
func formatJSONResult(output interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to format output: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonData)), nil
}
