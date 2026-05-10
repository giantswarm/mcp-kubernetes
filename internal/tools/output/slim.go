package output

import (
	"strings"
)

// SlimResource removes verbose fields from a resource map.
// This reduces response size by removing fields that rarely help AI agents.
func SlimResource(obj map[string]interface{}, excludedFields []string) map[string]interface{} {
	if obj == nil {
		return nil
	}

	if len(excludedFields) == 0 {
		excludedFields = DefaultExcludedFields()
	}

	// Create a deep copy to avoid modifying the original
	result := deepCopyMap(obj)

	// Remove each excluded field
	for _, field := range excludedFields {
		removeField(result, field)
	}

	return result
}

// SlimResources applies SlimResource to a slice of resources.
func SlimResources(objects []map[string]interface{}, excludedFields []string) []map[string]interface{} {
	if len(objects) == 0 {
		return objects
	}

	result := make([]map[string]interface{}, len(objects))
	for i, obj := range objects {
		result[i] = SlimResource(obj, excludedFields)
	}

	return result
}

// removeField removes a field at the specified path from a map.
// Supports dot notation for nested fields and [*] for array wildcards.
// Examples:
//   - "metadata.managedFields" -> removes obj["metadata"]["managedFields"]
//   - "status.conditions[*].lastTransitionTime" -> removes field from all array elements
//
// Keys with literal dots are also supported via greedy matching: a path like
// "metadata.annotations.kubectl.kubernetes.io/last-applied-configuration"
// will, after navigating into metadata.annotations, try the longest joined
// suffix that exists as a literal key. This is required because Kubernetes
// label and annotation keys very commonly contain dots
// (e.g. "kubernetes.io/foo", "deployment.kubernetes.io/revision").
//
// Precedence: when both a literal dotted key (e.g. "a.b") and a nested
// traversal (obj["a"]["b"]) would resolve, the longer literal-key match
// wins. Real Kubernetes resource shapes never make this ambiguous —
// annotations / labels are flat string maps and never contain nested
// objects — but the rule is load-bearing for the dot-key fix and is
// pinned by TestRemoveField_DotKeyPrecedence in slim_test.go.
func removeField(obj map[string]interface{}, path string) {
	if obj == nil || path == "" {
		return
	}

	parts := strings.Split(path, ".")
	removeFieldRecursive(obj, parts)
}

// removeFieldRecursive handles the recursive field removal.
func removeFieldRecursive(obj map[string]interface{}, parts []string) {
	if len(parts) == 0 || obj == nil {
		return
	}

	current := parts[0]
	remaining := parts[1:]

	// Check for array wildcard
	if strings.HasSuffix(current, "[*]") {
		fieldName := strings.TrimSuffix(current, "[*]")
		arrayVal, ok := obj[fieldName]
		if !ok {
			return
		}

		array, ok := arrayVal.([]interface{})
		if !ok {
			return
		}

		// Apply to each array element
		for _, elem := range array {
			if elemMap, ok := elem.(map[string]interface{}); ok {
				if len(remaining) == 0 {
					// Can't remove array element itself via this API
					continue
				}
				removeFieldRecursive(elemMap, remaining)
			}
		}
		return
	}

	// Check if this is the last part
	if len(remaining) == 0 {
		delete(obj, current)
		return
	}

	// Greedy match: try the longest joined suffix that exists as a literal
	// key in the current map before falling back to single-segment lookup.
	// This lets paths like "...annotations.kubernetes.io/foo" find an
	// annotation literally named "kubernetes.io/foo". We never join across
	// an array wildcard because that segment has its own semantics.
	wildcardIdx := indexOfArrayWildcard(parts)
	maxJoin := len(parts)
	if wildcardIdx >= 0 {
		maxJoin = wildcardIdx
	}
	for end := maxJoin; end > 1; end-- {
		joined := strings.Join(parts[:end], ".")
		val, ok := obj[joined]
		if !ok {
			continue
		}
		if end == len(parts) {
			delete(obj, joined)
			return
		}
		// Joined key matched but path continues — recurse into its value.
		if subMap, ok := val.(map[string]interface{}); ok {
			removeFieldRecursive(subMap, parts[end:])
			return
		}
	}

	// Fall back to navigating one segment at a time.
	nextVal, ok := obj[current]
	if !ok {
		return
	}

	nextMap, ok := nextVal.(map[string]interface{})
	if !ok {
		return
	}

	removeFieldRecursive(nextMap, remaining)
}

// indexOfArrayWildcard returns the index of the first part ending in "[*]",
// or -1 if none is present. Greedy joining must not cross array boundaries.
func indexOfArrayWildcard(parts []string) int {
	for i, p := range parts {
		if strings.HasSuffix(p, "[*]") {
			return i
		}
	}
	return -1
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}

	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}

	return result
}

// deepCopyValue creates a deep copy of a value.
//
// map[string]string is normalised into map[string]interface{} so the
// recursive remove/slim logic (which only navigates map[string]interface{})
// can also strip nested keys from typed string maps. This matters in
// practice because unstructured.GetAnnotations / GetLabels return
// map[string]string, and the describe handler exposes those via the
// convenience metadata map. Without this normalisation, paths like
// metadata.annotations.kubectl.kubernetes.io/last-applied-configuration
// silently no-op on the convenience map even though they work on the
// resource's own metadata. JSON marshaling treats both map shapes
// identically, so callers see no difference downstream.
func deepCopyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(val)
	case map[string]string:
		result := make(map[string]interface{}, len(val))
		for k, s := range val {
			result[k] = s
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = deepCopyValue(item)
		}
		return result
	default:
		// Primitives (string, int, bool, etc.) are copied by value
		return v
	}
}

// EstimateFieldSize estimates the JSON size reduction from removing a field.
// This is useful for logging and metrics.
func EstimateFieldSize(obj map[string]interface{}, path string) int64 {
	value := getFieldValue(obj, path)
	if value == nil {
		return 0
	}

	// Rough estimate based on value type
	return estimateValueSize(value)
}

// getFieldValue retrieves a field value at the given path.
func getFieldValue(obj map[string]interface{}, path string) interface{} {
	if obj == nil || path == "" {
		return nil
	}

	parts := strings.Split(path, ".")
	current := interface{}(obj)

	for _, part := range parts {
		// Handle array wildcard - just return the first match for estimation
		if strings.HasSuffix(part, "[*]") {
			fieldName := strings.TrimSuffix(part, "[*]")
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			current = m[fieldName]
			continue
		}

		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}

		current = m[part]
		if current == nil {
			return nil
		}
	}

	return current
}

// estimateValueSize estimates the JSON size of a value.
func estimateValueSize(v interface{}) int64 {
	switch val := v.(type) {
	case nil:
		return 4 // "null"
	case bool:
		return 5 // "true" or "false"
	case string:
		return int64(len(val) + 2) // quotes
	case float64, int64, int:
		return 10 // rough average for numbers
	case map[string]interface{}:
		var size int64 = 2 // braces
		for k, subVal := range val {
			size += int64(len(k)+3) + estimateValueSize(subVal) // key + quotes + colon
		}
		return size
	case []interface{}:
		var size int64 = 2 // brackets
		for _, item := range val {
			size += estimateValueSize(item) + 1 // comma
		}
		return size
	default:
		return 10 // rough default
	}
}
