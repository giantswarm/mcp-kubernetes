package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// CanIResponse represents the response from the can_i tool.
type CanIResponse struct {
	// Allowed indicates whether the requested action is permitted.
	Allowed bool `json:"allowed"`

	// Denied indicates whether the action was explicitly denied.
	Denied bool `json:"denied,omitempty"`

	// Reason provides explanation of the decision.
	Reason string `json:"reason,omitempty"`

	// User is the email of the user for whom the check was performed.
	User string `json:"user"`

	// Cluster is the target cluster name.
	Cluster string `json:"cluster,omitempty"`

	// Check contains the access check parameters that were evaluated.
	Check *AccessCheckInfo `json:"check"`
}

// AccessCheckInfo contains the parameters used in the access check.
type AccessCheckInfo struct {
	Verb        string `json:"verb"`
	Resource    string `json:"resource"`
	APIGroup    string `json:"apiGroup,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	Subresource string `json:"subresource,omitempty"`
}

// HandleCanI handles the can_i tool request.
//
// This function performs a SelfSubjectAccessReview to check if the authenticated
// user has permission to perform the specified action.
//
// # Security Model
//
// The check is performed using user impersonation, so the result reflects the
// actual permissions the user would have when performing the operation. This
// requires federation mode to be enabled.
func HandleCanI(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract required parameters
	verb, ok := args["verb"].(string)
	if !ok || verb == "" {
		return mcp.NewToolResultError("verb is required"), nil
	}

	resource, ok := args["resource"].(string)
	if !ok || resource == "" {
		return mcp.NewToolResultError("resource is required"), nil
	}

	// Extract optional parameters
	apiGroup, _ := args["apiGroup"].(string)
	namespace, _ := args["namespace"].(string)
	name, _ := args["name"].(string)
	subresource, _ := args["subresource"].(string)
	clusterName, _ := args["cluster"].(string)

	// Check if federation is enabled
	fedManager := sc.FederationManager()
	if fedManager == nil {
		return mcp.NewToolResultError("permission checks require federation mode to be enabled"), nil
	}

	// Get user info from OAuth context
	userInfo, ok := oauth.UserInfoFromContext(ctx)
	if !ok || userInfo == nil {
		return mcp.NewToolResultError("authentication required: no user info in context"), nil
	}

	// Convert OAuth user info to federation UserInfo using the helper function
	fedUserInfo := oauth.ToFederationUserInfo(userInfo)

	// Build the access check
	check := &federation.AccessCheck{
		Verb:        verb,
		Resource:    resource,
		APIGroup:    apiGroup,
		Namespace:   namespace,
		Name:        name,
		Subresource: subresource,
	}

	// Perform the access check
	result, err := fedManager.CheckAccess(ctx, clusterName, fedUserInfo, check)
	if err != nil {
		// Check if it's a validation error
		if isValidationError(err) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid request: %v", err)), nil
		}
		// For other errors, provide a generic message
		sc.Logger().Error("Access check failed", "error", err)
		return mcp.NewToolResultError("failed to check permissions - please try again"), nil
	}

	// Build the response
	response := &CanIResponse{
		Allowed: result.Allowed,
		Denied:  result.Denied,
		Reason:  result.Reason,
		User:    userInfo.Email,
		Cluster: clusterDisplayName(clusterName),
		Check: &AccessCheckInfo{
			Verb:        verb,
			Resource:    resource,
			APIGroup:    apiGroup,
			Namespace:   namespace,
			Name:        name,
			Subresource: subresource,
		},
	}

	// If there was an evaluation error, include it in the reason
	if result.EvaluationError != "" {
		if response.Reason != "" {
			response.Reason = fmt.Sprintf("%s (evaluation error: %s)", response.Reason, result.EvaluationError)
		} else {
			response.Reason = fmt.Sprintf("evaluation error: %s", result.EvaluationError)
		}
	}

	// Marshal the response to JSON
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// isValidationError checks if the error is a validation error that should be
// shown directly to the user. Only specific validation errors are considered
// safe to display; other errors may contain sensitive information.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	// Check for specific validation error types that are safe to show to users
	return errors.Is(err, federation.ErrInvalidAccessCheck) ||
		errors.Is(err, federation.ErrUserInfoRequired) ||
		errors.Is(err, federation.ErrInvalidClusterName)
}

// clusterDisplayName returns a display name for the cluster.
func clusterDisplayName(clusterName string) string {
	if clusterName == "" {
		return "local"
	}
	return clusterName
}
