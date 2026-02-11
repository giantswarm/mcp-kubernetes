package federation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

const (
	// MaxMappingCount is the maximum number of group mappings allowed.
	// This limits the size of the mapping table to prevent excessively large
	// configurations that would slow down startup validation and produce verbose
	// warning logs. 100 mappings is generous for any real-world deployment.
	MaxMappingCount = 100
)

// GroupMapper translates OIDC group identifiers to the identifiers expected by
// workload cluster RoleBindings before setting Impersonate-Group headers.
//
// This solves the common problem where the OIDC provider (e.g., Dex, Google, Okta)
// returns group identifiers in a different format than what the workload cluster's
// RBAC expects. For example:
//   - OIDC provider returns Azure AD display names ("customer:GroupA"),
//     but workload cluster RoleBindings use Azure AD group GUIDs.
//   - LDAP-backed provider returns group DNs, but the cluster expects short names.
//   - Federation broker (Dex) normalizes groups differently from the upstream IdP.
//
// # Design
//
// GroupMapper uses a static mapping table (source -> target) that is configured
// at startup. This approach is:
//   - Predictable: mappings are explicit and visible in configuration
//   - Logged: all translations are logged at Info level for operational visibility
//   - Validated: dangerous target groups (e.g., system:masters) are rejected at startup
//   - Fast: O(1) lookup per group, no external dependencies
//
// # Security Considerations
//
// Group mappings can change the effective permissions of users on workload clusters.
// Anyone who can modify the mapping configuration (Helm values or WC_GROUP_MAPPINGS
// env var) can control which Kubernetes groups users are impersonated into. To mitigate
// accidental or intentional privilege escalation:
//
//   - Mapping to dangerous Kubernetes system groups (see deniedTargetGroups) is rejected
//     at validation time and the server will refuse to start.
//   - Mapping to any "system:*" prefixed group triggers a warning log at startup.
//   - All group translations are logged at Info level (not Debug) so they appear in
//     default production log output.
//
// Note that reconstructing a complete audit trail for a mapped impersonation request
// requires correlating mcp-kubernetes application logs with the Kubernetes audit log
// of the target workload cluster. The application logs record which groups were
// translated; the cluster audit log records the resulting API calls.
//
// # Thread Safety
//
// GroupMapper is immutable after construction and safe for concurrent use
// from multiple goroutines. The mapping table is never modified after creation.
//
// # Behavior
//
//   - Mapped groups: translated to their target identifiers
//   - Unmapped groups: passed through unchanged (backward compatible)
//   - Empty mapping: all groups pass through unchanged (no-op)
//   - Nil/empty groups: returned as-is
type GroupMapper struct {
	// mappings is the source-group -> target-group map.
	// This map is never modified after construction (immutable).
	mappings map[string]string

	// logger for logging group translations.
	logger *slog.Logger
}

// NewGroupMapper creates a new GroupMapper with the given mappings.
// Returns nil if the mappings are empty (no-op optimization).
//
// The mappings are defensively copied before validation to prevent external
// mutation. After construction, the GroupMapper is immutable and safe for
// concurrent use.
//
// Returns an error if the mappings are invalid (e.g., empty keys or values,
// multiple source groups mapping to the same target, dangerous target groups).
func NewGroupMapper(mappings map[string]string, logger *slog.Logger) (*GroupMapper, error) {
	if len(mappings) == 0 {
		return nil, nil
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Defensive copy first, then validate the copy (not the original).
	// This ensures we validate exactly what we store.
	copied := make(map[string]string, len(mappings))
	for k, v := range mappings {
		copied[k] = v
	}

	if err := validateGroupMappings(copied); err != nil {
		return nil, fmt.Errorf("invalid group mappings: %w", err)
	}

	// Warn about any system:* target groups that passed validation
	// (they aren't on the denylist but may still be unexpected)
	for source, target := range copied {
		if strings.HasPrefix(target, "system:") {
			logger.Warn("Group mapping targets a Kubernetes system group",
				"source_group", source,
				"target_group", target,
				"hint", "Ensure this is intentional; system groups carry special privileges")
		}
	}

	return &GroupMapper{
		mappings: copied,
		logger:   logger,
	}, nil
}

// MapGroups translates a slice of OIDC group identifiers using the configured mappings.
// Groups that have a mapping are translated; groups without a mapping pass through unchanged.
//
// The original slice is never modified. A new slice is always returned when any
// mapping is applied.
//
// A summary of all translations is logged at Info level for operational visibility.
// Note that the user email is hashed in logs (via UserHashAttr) for privacy; correlating
// a specific translation with a user identity requires matching the hash across log entries
// or consulting the Kubernetes audit log of the target workload cluster.
//
// Returns nil if groups is nil, or an empty slice if groups is empty.
func (gm *GroupMapper) MapGroups(groups []string, userEmail string) []string {
	if gm == nil || len(gm.mappings) == 0 {
		return groups
	}

	if groups == nil {
		return nil
	}

	if len(groups) == 0 {
		return groups
	}

	// Check if any mapping applies before allocating a new slice
	anyMapped := false
	for _, g := range groups {
		if _, ok := gm.mappings[g]; ok {
			anyMapped = true
			break
		}
	}

	if !anyMapped {
		return groups
	}

	// At least one group needs mapping: create a new slice
	mapped := make([]string, len(groups))
	var translations []string
	for i, g := range groups {
		if target, ok := gm.mappings[g]; ok {
			mapped[i] = target
			translations = append(translations, fmt.Sprintf("%s->%s", g, target))
		} else {
			mapped[i] = g
		}
	}

	gm.logger.Info("Groups mapped for impersonation",
		"mapped_count", len(translations),
		"total_groups", len(groups),
		"translations", strings.Join(translations, ", "),
		UserHashAttr(userEmail))

	return mapped
}

// MappingCount returns the number of configured group mappings.
func (gm *GroupMapper) MappingCount() int {
	if gm == nil {
		return 0
	}
	return len(gm.mappings)
}

// String returns a human-readable summary of the group mapper for logging.
// It does not expose the actual mapping values for security.
func (gm *GroupMapper) String() string {
	if gm == nil {
		return "GroupMapper{disabled}"
	}
	return fmt.Sprintf("GroupMapper{mappings=%d}", len(gm.mappings))
}

// deniedTargetGroups contains Kubernetes groups that must never be used as mapping
// targets. These groups grant dangerous privileges that bypass normal RBAC:
//
//   - system:masters: Bypasses ALL RBAC checks, equivalent to cluster-admin.
//   - system:nodes: Grants node-level access via the Node authorizer, enabling
//     potential privilege escalation through node identity impersonation.
//   - system:kube-controller-manager: Grants controller-manager privileges,
//     which include creating/modifying most cluster resources.
//   - system:kube-scheduler: Grants scheduler privileges including pod placement
//     decisions and access to scheduling-related resources.
//   - system:kube-proxy: Can modify network rules (iptables/ipvs) on nodes,
//     enabling traffic interception or redirection.
//
// This is a hard denylist: the server will refuse to start if any mapping targets
// one of these groups. This prevents both accidental misconfiguration and
// intentional privilege escalation via the mapping configuration.
//
// Any other "system:*" prefixed target group that is NOT on this denylist will
// still trigger a warning log at startup (see NewGroupMapper).
var deniedTargetGroups = map[string]struct{}{
	"system:masters":                 {},
	"system:nodes":                   {},
	"system:kube-controller-manager": {},
	"system:kube-scheduler":          {},
	"system:kube-proxy":              {},
}

// validateGroupMappings validates the group mapping configuration.
// It ensures:
//   - No empty keys or values
//   - No mapping to dangerous target groups (see deniedTargetGroups)
//   - No duplicate target groups (multiple sources mapping to the same target
//     would make log correlation ambiguous)
//   - Keys and values don't contain control characters
func validateGroupMappings(mappings map[string]string) error {
	if len(mappings) == 0 {
		return nil
	}

	if len(mappings) > MaxMappingCount {
		return fmt.Errorf("too many group mappings (%d): maximum is %d", len(mappings), MaxMappingCount)
	}

	// Track target groups to detect duplicates
	targetToSource := make(map[string]string, len(mappings))

	for source, target := range mappings {
		// Validate source group
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("source group must not be empty")
		}
		if containsControlCharacters(source) {
			return fmt.Errorf("source group %q contains control characters", source)
		}

		// Validate target group
		if strings.TrimSpace(target) == "" {
			return fmt.Errorf("target group for source %q must not be empty", source)
		}
		if containsControlCharacters(target) {
			return fmt.Errorf("target group %q for source %q contains control characters", target, source)
		}

		// Reject dangerous target groups that would enable privilege escalation
		if _, denied := deniedTargetGroups[target]; denied {
			return fmt.Errorf(
				"target group %q for source %q is denied: mapping to this group "+
					"would enable privilege escalation (this group bypasses RBAC)",
				target, source)
		}

		// Check for duplicate targets (ambiguous log correlation)
		if existingSource, ok := targetToSource[target]; ok {
			return fmt.Errorf("duplicate target group %q: both %q and %q map to it", target, existingSource, source)
		}
		targetToSource[target] = source
	}

	return nil
}

// ParseGroupMappingsJSON parses a JSON string into a group mappings map.
// The expected format is a JSON object: {"source1": "target1", "source2": "target2"}.
//
// This is the primary format used by the WC_GROUP_MAPPINGS environment variable.
// JSON is used instead of a simple key=value format because group names may
// contain characters like '=' and ',' that would be ambiguous in simpler formats.
//
// This function only handles JSON parsing. Semantic validation (denylist, duplicates,
// control characters) is performed by NewGroupMapper to avoid double validation.
func ParseGroupMappingsJSON(jsonStr string) (map[string]string, error) {
	if jsonStr == "" {
		return nil, nil
	}

	var mappings map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &mappings); err != nil {
		return nil, fmt.Errorf("failed to parse group mappings JSON: %w", err)
	}

	return mappings, nil
}

// FormatGroupMappingsForLog returns a human-readable representation of group mappings
// for operator logs. It includes the mapping count and source group names (sorted
// alphabetically) to help operators verify the configuration. Note that source
// group names may contain organizational structure (e.g., team names, AD group
// display names); ensure log aggregation systems treat these as sensitive data.
func FormatGroupMappingsForLog(mappings map[string]string) string {
	if len(mappings) == 0 {
		return "none"
	}

	sources := make([]string, 0, len(mappings))
	for source := range mappings {
		sources = append(sources, source)
	}
	sort.Strings(sources)

	return fmt.Sprintf("%d mappings (sources: %s)", len(mappings), strings.Join(sources, ", "))
}
