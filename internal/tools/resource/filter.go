package resource

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// arrayWildcard is the token used to match any element in an array path
	arrayWildcard = "[*]"

	// Security limits to prevent resource exhaustion attacks
	maxFilterCriteria  = 50   // Maximum number of filter criteria allowed
	maxPathDepth       = 20   // Maximum depth of nested paths (number of dots)
	maxFilterValueSize = 1024 // Maximum size of filter values in bytes
)

// FilterCriteria represents client-side filtering criteria for resources.
// The map keys are JSON paths (e.g., "spec.taints", "metadata.labels.app")
// and the values are the expected values or conditions to match.
type FilterCriteria map[string]interface{}

// ApplyClientSideFilter filters a list of resources based on the provided criteria.
// It supports:
// - Simple field matching: {"status.phase": "Running"}
// - Array element matching: {"spec.taints[*].key": "node.kubernetes.io/unschedulable"}
// - Nested map matching: {"metadata.labels.app": "nginx"}
// - Multiple criteria (AND logic): all criteria must match for a resource to pass
//
// Returns an error if filter criteria exceed security limits to prevent resource exhaustion.
func ApplyClientSideFilter(objects []runtime.Object, criteria FilterCriteria) ([]runtime.Object, error) {
	if len(criteria) == 0 {
		return objects, nil
	}

	// Validate number of filter criteria to prevent DoS
	if len(criteria) > maxFilterCriteria {
		return nil, fmt.Errorf("too many filter criteria: %d (maximum allowed: %d)", len(criteria), maxFilterCriteria)
	}

	// Validate each filter criterion
	for path, value := range criteria {
		// Validate path depth to prevent deep recursion attacks
		pathDepth := strings.Count(path, ".")
		if pathDepth > maxPathDepth {
			return nil, fmt.Errorf("filter path too deep: %q has depth %d (maximum allowed: %d)", path, pathDepth, maxPathDepth)
		}

		// Validate path is not empty or contains suspicious patterns
		if path == "" {
			return nil, fmt.Errorf("filter path cannot be empty")
		}
		if strings.Contains(path, "..") {
			return nil, fmt.Errorf("filter path contains invalid pattern '..': %q", path)
		}

		// Validate filter value size for string values
		if strValue, ok := value.(string); ok {
			if len(strValue) > maxFilterValueSize {
				return nil, fmt.Errorf("filter value too large: %d bytes (maximum allowed: %d)", len(strValue), maxFilterValueSize)
			}
		}
	}

	filtered := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		if matchesFilter(obj, criteria) {
			filtered = append(filtered, obj)
		}
	}

	return filtered, nil
}

// matchesFilter checks if a single object matches all filter criteria
func matchesFilter(obj runtime.Object, criteria FilterCriteria) bool {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return false
	}

	// All criteria must match (AND logic)
	for path, expectedValue := range criteria {
		if !matchesFieldPath(unstructuredObj, path, expectedValue) {
			return false
		}
	}

	return true
}

// matchesFieldPath checks if a field path in the object matches the expected value.
// Supports:
// - Simple paths: "status.phase"
// - Array wildcard paths: "spec.taints[*].key"
// - Nested maps: "metadata.labels.app"
func matchesFieldPath(obj *unstructured.Unstructured, path string, expectedValue interface{}) bool {
	// Check if this is an array wildcard path (contains [*])
	if strings.Contains(path, arrayWildcard) {
		return matchesArrayPath(obj, path, expectedValue)
	}

	// Regular path traversal
	parts := strings.Split(path, ".")
	value, found := getNestedValue(obj.Object, parts)
	if !found {
		return false
	}

	return valuesMatch(value, expectedValue)
}

// matchesArrayPath handles paths with array wildcards like "spec.taints[*].key"
func matchesArrayPath(obj *unstructured.Unstructured, path string, expectedValue interface{}) bool {
	// Split path into parts: ["spec", "taints[*]", "key"]
	parts := strings.Split(path, ".")

	// Find the array part
	var (
		arrayIndex     int
		arrayFieldName string
		remainingPath  []string
	)

	for i, part := range parts {
		if strings.Contains(part, arrayWildcard) {
			arrayIndex = i
			arrayFieldName = strings.TrimSuffix(part, arrayWildcard)
			if i+1 < len(parts) {
				remainingPath = parts[i+1:]
			}
			break
		}
	}

	// Get the path up to the array field
	pathToArray := parts[:arrayIndex]
	if arrayFieldName != "" {
		pathToArray = append(pathToArray, arrayFieldName)
	}

	// Get the array value
	arrayValue, found := getNestedValue(obj.Object, pathToArray)
	if !found {
		return false
	}

	// Check if it's actually an array
	arraySlice, ok := arrayValue.([]interface{})
	if !ok {
		return false
	}

	// Check if any element in the array matches
	for _, elem := range arraySlice {
		if len(remainingPath) == 0 {
			// No remaining path, compare the array element directly
			if valuesMatch(elem, expectedValue) {
				return true
			}
		} else {
			// Traverse into the array element
			elemMap, ok := elem.(map[string]interface{})
			if !ok {
				continue
			}

			elemValue, found := getNestedValue(elemMap, remainingPath)
			if found && valuesMatch(elemValue, expectedValue) {
				return true
			}
		}
	}

	return false
}

// getNestedValue retrieves a nested value from a map using a path
func getNestedValue(obj map[string]interface{}, path []string) (interface{}, bool) {
	if len(path) == 0 {
		return nil, false
	}

	current := obj
	for i, key := range path {
		value, found := current[key]
		if !found {
			return nil, false
		}

		// If this is the last key, return the value
		if i == len(path)-1 {
			return value, true
		}

		// Otherwise, continue traversing
		nextMap, ok := value.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = nextMap
	}

	return nil, false
}

// valuesMatch compares two values for equality.
// Supports different types and flexible matching.
func valuesMatch(actual, expected interface{}) bool {
	// Handle nil cases
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}

	// Handle string comparison (most common case)
	actualStr, actualIsStr := actual.(string)
	expectedStr, expectedIsStr := expected.(string)
	if actualIsStr && expectedIsStr {
		return actualStr == expectedStr
	}

	// Handle bool comparison
	actualBool, actualIsBool := actual.(bool)
	expectedBool, expectedIsBool := expected.(bool)
	if actualIsBool && expectedIsBool {
		return actualBool == expectedBool
	}

	// Handle map matching (for nested object filters)
	expectedMap, expectedIsMap := expected.(map[string]interface{})
	if expectedIsMap {
		actualMap, actualIsMap := actual.(map[string]interface{})
		if !actualIsMap {
			return false
		}
		return mapsMatch(actualMap, expectedMap)
	}

	// Fallback to string representation comparison for numbers and other types
	// This handles int, float, and other types in a simple, type-safe way
	return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
}

// mapsMatch checks if all key-value pairs in expected map exist in actual map
func mapsMatch(actual, expected map[string]interface{}) bool {
	for key, expectedVal := range expected {
		actualVal, found := actual[key]
		if !found {
			return false
		}
		if !valuesMatch(actualVal, expectedVal) {
			return false
		}
	}
	return true
}
