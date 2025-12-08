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

	// Navigate deeper
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
func deepCopyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(val)
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
