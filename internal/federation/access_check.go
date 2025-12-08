package federation

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckAccess verifies if the user can perform the specified action on a cluster.
// This performs a SelfSubjectAccessReview to check permissions without actually
// attempting the operation.
//
// # Security Model
//
// The check is performed as the impersonated user, not the admin credentials.
// This means the SelfSubjectAccessReview evaluates the actual permissions the
// user would have when performing the operation.
//
// # Error Handling
//
// - Returns (nil, error) if the check itself failed (e.g., API server error)
// - Returns (*AccessCheckResult, nil) if the check completed successfully
// - The result.Allowed field indicates if the operation would be permitted
//
// # Performance Considerations
//
// SAR checks add ~50-100ms latency. For repeated operations, consider:
// - Caching results for short periods (30s-60s)
// - Making checks optional via configuration
// - Batching checks when possible
func (m *Manager) CheckAccess(ctx context.Context, clusterName string, user *UserInfo, check *AccessCheck) (*AccessCheckResult, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}

	// Validate inputs
	if err := ValidateUserInfo(user); err != nil {
		return nil, err
	}

	if err := ValidateAccessCheck(check); err != nil {
		return nil, err
	}

	// Validate cluster name if provided (empty means local cluster)
	if clusterName != "" {
		if err := ValidateClusterName(clusterName); err != nil {
			return nil, err
		}
	}

	m.logger.Debug("Performing access check",
		"cluster", clusterName,
		UserHashAttr(user.Email),
		"verb", check.Verb,
		"resource", check.Resource,
		"namespace", check.Namespace)

	// Get a client for the target cluster (impersonating the user)
	client, err := m.GetClient(ctx, clusterName, user)
	if err != nil {
		return nil, &AccessCheckError{
			ClusterName: clusterName,
			Check:       check,
			Reason:      "failed to get cluster client",
			Err:         err,
		}
	}

	// Create the SelfSubjectAccessReview request
	// Note: We use SelfSubjectAccessReview (not SubjectAccessReview) because
	// the client is already configured with impersonation headers. This means
	// the API server evaluates the request from the perspective of the
	// impersonated user, giving us accurate permission checks.
	review := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace:   check.Namespace,
				Verb:        check.Verb,
				Group:       check.APIGroup,
				Resource:    check.Resource,
				Name:        check.Name,
				Subresource: check.Subresource,
			},
		},
	}

	// Perform the access review
	result, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(
		ctx,
		review,
		metav1.CreateOptions{},
	)
	if err != nil {
		m.logger.Debug("Access check API call failed",
			"cluster", clusterName,
			UserHashAttr(user.Email),
			"error", err)
		return nil, &AccessCheckError{
			ClusterName: clusterName,
			Check:       check,
			Reason:      "SelfSubjectAccessReview API call failed",
			Err:         err,
		}
	}

	// Build the result
	accessResult := &AccessCheckResult{
		Allowed:         result.Status.Allowed,
		Denied:          result.Status.Denied,
		Reason:          result.Status.Reason,
		EvaluationError: result.Status.EvaluationError,
	}

	// Log the result
	m.logger.Debug("Access check completed",
		"cluster", clusterName,
		UserHashAttr(user.Email),
		"verb", check.Verb,
		"resource", check.Resource,
		"allowed", accessResult.Allowed,
		"denied", accessResult.Denied)

	return accessResult, nil
}

// ValidateAccessCheck validates the AccessCheck parameters.
// Returns ErrInvalidAccessCheck if the check is invalid.
func ValidateAccessCheck(check *AccessCheck) error {
	if check == nil {
		return fmt.Errorf("%w: check is nil", ErrInvalidAccessCheck)
	}

	if check.Verb == "" {
		return fmt.Errorf("%w: verb is required", ErrInvalidAccessCheck)
	}

	if check.Resource == "" {
		return fmt.Errorf("%w: resource is required", ErrInvalidAccessCheck)
	}

	// Validate verb is a known Kubernetes verb
	validVerbs := map[string]bool{
		"get":              true,
		"list":             true,
		"watch":            true,
		"create":           true,
		"update":           true,
		"patch":            true,
		"delete":           true,
		"deletecollection": true,
		"impersonate":      true,
		"bind":             true,
		"escalate":         true,
		"*":                true, // wildcard
	}

	if !validVerbs[check.Verb] {
		return fmt.Errorf("%w: unknown verb %q", ErrInvalidAccessCheck, check.Verb)
	}

	return nil
}

// CheckAccessAllowed is a convenience method that performs an access check
// and returns a clear error if access is denied.
//
// This is useful for pre-flight checks before destructive operations:
//
//	if err := manager.CheckAccessAllowed(ctx, cluster, user, &AccessCheck{
//		Verb:      "delete",
//		Resource:  "pods",
//		Namespace: "production",
//	}); err != nil {
//		return err // Either check failed or access denied
//	}
//	// Proceed with delete...
func (m *Manager) CheckAccessAllowed(ctx context.Context, clusterName string, user *UserInfo, check *AccessCheck) error {
	result, err := m.CheckAccess(ctx, clusterName, user, check)
	if err != nil {
		return err
	}

	if !result.Allowed {
		return &AccessDeniedError{
			ClusterName: clusterName,
			UserEmail:   user.Email,
			Verb:        check.Verb,
			Resource:    check.Resource,
			APIGroup:    check.APIGroup,
			Namespace:   check.Namespace,
			Name:        check.Name,
			Reason:      result.Reason,
		}
	}

	return nil
}
