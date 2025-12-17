package capi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// Generic error messages that don't reveal internal architecture details.
// Security: Using generic messages prevents information disclosure about
// server configuration and internal systems.
const (
	// errOperationNotAvailable is returned when federation mode is not enabled.
	// Intentionally generic to avoid revealing server configuration details.
	errOperationNotAvailable = "this operation is not available"

	// errServiceUnavailable is returned when the federation manager is closed or unavailable.
	errServiceUnavailable = "service temporarily unavailable"

	// errAuthRequired is returned when authentication is missing or invalid.
	errAuthRequired = "authentication required"

	// DefaultMaxResults is the default maximum number of results returned by list operations.
	// This prevents DoS attacks via unbounded result sets.
	DefaultMaxResults = 100

	// MaxResultsLimit is the absolute maximum number of results that can be requested.
	MaxResultsLimit = 500
)

// handleListClusters handles the capi_list_clusters tool request.
// It lists all CAPI clusters the user has access to, with optional filtering.
func handleListClusters(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Get federation manager
	fedManager := sc.FederationManager()
	if fedManager == nil {
		return mcp.NewToolResultError(errOperationNotAvailable), nil
	}

	// Get authenticated user
	user, err := getUserFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(errAuthRequired), nil
	}

	// Extract filter parameters
	args := request.GetArguments()
	organization, _ := args["organization"].(string)
	provider, _ := args["provider"].(string)
	status, _ := args["status"].(string)
	readyOnly, _ := args["readyOnly"].(bool)
	labelSelector, _ := args["labelSelector"].(string)

	// Extract pagination parameter with security limits
	limit := DefaultMaxResults
	if limitArg, ok := args["limit"].(float64); ok && limitArg > 0 {
		limit = int(limitArg)
		if limit > MaxResultsLimit {
			limit = MaxResultsLimit
		}
	}

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

	// Apply pagination limit to prevent DoS via large result sets
	totalCount := len(clusters)
	truncated := false
	if len(clusters) > limit {
		clusters = clusters[:limit]
		truncated = true
	}

	// Convert to output format
	output := ClusterListOutput{
		Clusters:      make([]ClusterListItem, 0, len(clusters)),
		TotalCount:    totalCount,
		ReturnedCount: len(clusters),
		Truncated:     truncated,
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
		return mcp.NewToolResultError(errOperationNotAvailable), nil
	}

	// Get authenticated user
	user, err := getUserFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(errAuthRequired), nil
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
		return mcp.NewToolResultError(errOperationNotAvailable), nil
	}

	// Get authenticated user
	user, err := getUserFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(errAuthRequired), nil
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
// An exact match takes priority over partial matches.
func resolveClusterPattern(clusters []federation.ClusterSummary, pattern string) (*federation.ClusterSummary, []federation.ClusterSummary) {
	patternLower := strings.ToLower(pattern)
	var matches []federation.ClusterSummary

	for i := range clusters {
		// Exact match takes priority - return immediately
		if clusters[i].Name == pattern {
			return &clusters[i], []federation.ClusterSummary{clusters[i]}
		}
		// Collect partial matches (case-insensitive)
		if strings.Contains(strings.ToLower(clusters[i].Name), patternLower) {
			matches = append(matches, clusters[i])
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
		return mcp.NewToolResultError(errOperationNotAvailable), nil
	}

	// Get authenticated user
	user, err := getUserFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(errAuthRequired), nil
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
// Returns the federation.UserInfo on success, or an error on failure.
func getUserFromContext(ctx context.Context) (*federation.UserInfo, error) {
	oauthUser, ok := oauth.UserInfoFromContext(ctx)
	if !ok || oauthUser == nil {
		return nil, errors.New("authentication required: no user info in context")
	}

	// Validate user info for impersonation
	if err := oauth.ValidateUserInfoForImpersonation(oauthUser); err != nil {
		return nil, fmt.Errorf("authentication error: %w", err)
	}

	// Convert to federation user info
	user := oauth.ToFederationUserInfo(oauthUser)
	if user == nil {
		return nil, errors.New("failed to convert user info for federation")
	}

	return user, nil
}

// listClustersWithOptions lists clusters with optional filtering.
// This is a helper that wraps the federation manager's ListClusters method
// with support for the ClusterListOptions filtering. The Manager has filtering
// support internally, but the ClusterClientManager interface only exposes
// ListClusters, so we apply additional filters client-side.
func listClustersWithOptions(ctx context.Context, fedManager federation.ClusterClientManager, user *federation.UserInfo, opts *federation.ClusterListOptions) ([]federation.ClusterSummary, error) {
	clusters, err := fedManager.ListClusters(ctx, user)
	if err != nil {
		return nil, err
	}

	// If no options, return all clusters
	if opts == nil {
		return clusters, nil
	}

	// Parse label selector if provided
	var labelSel labels.Selector
	if opts.LabelSelector != "" {
		var parseErr error
		labelSel, parseErr = labels.Parse(opts.LabelSelector)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid label selector: %w", parseErr)
		}
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

		// Filter by label selector
		if labelSel != nil && !labelSel.Matches(labels.Set(cluster.Labels)) {
			continue
		}

		filtered = append(filtered, cluster)
	}

	return filtered, nil
}

// handleFederationError converts federation errors to user-friendly tool results.
// This function should only be called with non-nil errors. Passing nil is a
// programming error and will cause a panic to catch bugs early.
func handleFederationError(err error, operation string) (*mcp.CallToolResult, error) {
	if err == nil {
		panic("handleFederationError called with nil error")
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

	// Handle sentinel errors with generic messages to prevent information disclosure.
	// Security: These messages intentionally don't reveal internal system details.
	switch {
	case errors.Is(err, federation.ErrUserInfoRequired):
		return mcp.NewToolResultError(errAuthRequired), nil
	case errors.Is(err, federation.ErrManagerClosed):
		return mcp.NewToolResultError(errServiceUnavailable), nil
	case errors.Is(err, federation.ErrCAPICRDNotInstalled):
		// Intentionally generic - don't reveal whether CAPI is installed
		return mcp.NewToolResultError(errOperationNotAvailable), nil
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
