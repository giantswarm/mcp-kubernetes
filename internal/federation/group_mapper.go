package federation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
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
//   - Auditable: all translations are logged with original and mapped values
//   - Secure: only configured mappings are applied, no dynamic resolution
//   - Fast: O(1) lookup per group, no external dependencies
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

	// logger for audit logging of group translations.
	logger *slog.Logger
}

// GroupMapperConfig holds the configuration for constructing a GroupMapper.
type GroupMapperConfig struct {
	// Mappings is a map of source-group -> target-group.
	// When a user's OIDC group matches a source-group key, it is
	// translated to the corresponding target-group value before
	// being set as an Impersonate-Group header.
	//
	// Groups not present in this map pass through unchanged.
	Mappings map[string]string
}

// NewGroupMapper creates a new GroupMapper with the given configuration.
// Returns nil if the configuration has no mappings (no-op optimization).
//
// The mappings are defensively copied to prevent external mutation.
// After construction, the GroupMapper is immutable and safe for concurrent use.
//
// Returns an error if the configuration is invalid (e.g., empty keys or values,
// multiple source groups mapping to the same target).
func NewGroupMapper(config GroupMapperConfig, logger *slog.Logger) (*GroupMapper, error) {
	if len(config.Mappings) == 0 {
		return nil, nil
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Validate mappings
	if err := validateGroupMappings(config.Mappings); err != nil {
		return nil, fmt.Errorf("invalid group mappings: %w", err)
	}

	// Defensive copy to prevent external mutation
	mappings := make(map[string]string, len(config.Mappings))
	for k, v := range config.Mappings {
		mappings[k] = v
	}

	logger.Info("Group mapper initialized",
		"mapping_count", len(mappings))

	return &GroupMapper{
		mappings: mappings,
		logger:   logger,
	}, nil
}

// MapGroups translates a slice of OIDC group identifiers using the configured mappings.
// Groups that have a mapping are translated; groups without a mapping pass through unchanged.
//
// The original slice is never modified. A new slice is always returned when any
// mapping is applied.
//
// All translations are logged at Debug level for audit purposes, including
// the original and mapped group values.
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
	for i, g := range groups {
		if target, ok := gm.mappings[g]; ok {
			mapped[i] = target
			gm.logger.Debug("Group mapped for impersonation",
				"original_group", g,
				"mapped_group", target,
				UserHashAttr(userEmail))
		} else {
			mapped[i] = g
		}
	}

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

// validateGroupMappings validates the group mapping configuration.
// It ensures:
//   - No empty keys or values
//   - No duplicate target groups (multiple sources mapping to the same target
//     would make audit trails ambiguous)
//   - Keys and values don't contain control characters
func validateGroupMappings(mappings map[string]string) error {
	if len(mappings) == 0 {
		return nil
	}

	// Track target groups to detect duplicates
	targetToSource := make(map[string]string, len(mappings))

	for source, target := range mappings {
		// Validate source group
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("source group must not be empty")
		}
		if containsControlChars(source) {
			return fmt.Errorf("source group %q contains control characters", source)
		}

		// Validate target group
		if strings.TrimSpace(target) == "" {
			return fmt.Errorf("target group for source %q must not be empty", source)
		}
		if containsControlChars(target) {
			return fmt.Errorf("target group %q for source %q contains control characters", target, source)
		}

		// Check for duplicate targets (ambiguous audit trail)
		if existingSource, ok := targetToSource[target]; ok {
			return fmt.Errorf("duplicate target group %q: both %q and %q map to it", target, existingSource, source)
		}
		targetToSource[target] = source
	}

	return nil
}

// containsControlChars checks if a string contains ASCII control characters.
func containsControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// ParseGroupMappingsJSON parses a JSON string into a group mappings map.
// The expected format is a JSON object: {"source1": "target1", "source2": "target2"}.
//
// This is the primary format used by the WC_GROUP_MAPPINGS environment variable.
// JSON is used instead of a simple key=value format because group names may
// contain characters like '=' and ',' that would be ambiguous in simpler formats.
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

// FormatGroupMappingsForLog returns a safe representation of group mappings for logging.
// It shows the number of mappings and the source group names (but not target values,
// which could contain sensitive identifiers like GUIDs).
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
