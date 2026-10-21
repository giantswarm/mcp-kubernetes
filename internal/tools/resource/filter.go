package resource

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/giantswarm/mcp-kubernetes/internal/tools/output"
)

const (
	// arrayWildcard is the token used to match any element in an array path
	arrayWildcard = "[*]"

	// imagesPath is the virtual filter path over every container,
	// initContainer and ephemeralContainer image of a pod or of a workload's
	// pod template — the images the list summary reports.
	imagesPath = "images"

	// Security limits to prevent resource exhaustion attacks
	maxFilterCriteria  = 50   // Maximum number of filter criteria allowed
	maxPathDepth       = 20   // Maximum depth of nested paths (number of dots)
	maxFilterValueSize = 1024 // Maximum size of filter values in bytes

	// maxRegexInstructions caps the compiled program of a regex operator.
	// RE2 matching is linear in the input but also in the program size, and
	// counted repeats such as (?:[a-z]?){1000} compile to tens of thousands
	// of instructions within maxFilterValueSize. An image or name pattern
	// needs a few dozen.
	maxRegexInstructions = 1000
)

// String operators of an operator criterion, {"op": "prefix", "value": "registry.example/"}.
const (
	opPrefix   = "prefix"
	opSuffix   = "suffix"
	opContains = "contains"
	opRegex    = "regex"
)

// stringOperators are the ops an operator criterion may name.
var stringOperators = map[string]bool{opPrefix: true, opSuffix: true, opContains: true, opRegex: true}

// FilterCriteria represents client-side filtering criteria for resources.
// The map keys are JSON paths (e.g., "spec.taints", "metadata.labels.app")
// and the values are the expected values or conditions to match.
type FilterCriteria map[string]interface{}

// valueMatcher reports whether a value found at a filter path satisfies a
// criterion.
type valueMatcher func(actual interface{}) bool

// criterion is a validated filter entry: a path and the matcher its value
// must satisfy.
type criterion struct {
	path  string
	match valueMatcher
}

// clientFilter is a compiled FilterCriteria: every criterion validated once
// per call, all of which an object must satisfy (AND).
type clientFilter []criterion

// ApplyClientSideFilter filters a list of resources based on the provided criteria.
// It supports:
// - Simple field matching: {"status.phase": "Running"}
// - Array element matching: {"spec.taints[*].key": "node.kubernetes.io/unschedulable"}
// - Nested map matching: {"metadata.labels.app": "nginx"}
// - String operators: {"spec.containers[*].image": {"op": "prefix", "value": "registry.example/"}}
// - The virtual "images" path over every container image of pods and workloads
// - Multiple criteria (AND logic): all criteria must match for a resource to pass
//
// Returns an error if filter criteria exceed security limits to prevent resource exhaustion,
// or hold an invalid operator criterion or regular expression.
func ApplyClientSideFilter(objects []runtime.Object, criteria FilterCriteria) ([]runtime.Object, error) {
	filter, err := compileFilter(criteria)
	if err != nil {
		return nil, err
	}
	return filter.apply(objects), nil
}

// compileFilter validates criteria against the security limits and builds
// each criterion's matcher. Empty criteria compile to a filter that keeps
// every object.
func compileFilter(criteria FilterCriteria) (clientFilter, error) {
	if len(criteria) == 0 {
		return nil, nil
	}

	// Validate number of filter criteria to prevent DoS
	if len(criteria) > maxFilterCriteria {
		return nil, fmt.Errorf("too many filter criteria: %d (maximum allowed: %d)", len(criteria), maxFilterCriteria)
	}

	// Validate each filter criterion
	compiled := make([]criterion, 0, len(criteria))
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
		if strings.Count(path, arrayWildcard) > 1 {
			return nil, fmt.Errorf("filter path %q has more than one %s: only one array wildcard per path is supported", path, arrayWildcard)
		}

		match, err := newValueMatcher(value)
		if err != nil {
			return nil, fmt.Errorf("filter %q: %w", path, err)
		}
		compiled = append(compiled, criterion{path: path, match: match})
	}

	return compiled, nil
}

// apply returns the objects that satisfy every criterion of f.
func (f clientFilter) apply(objects []runtime.Object) []runtime.Object {
	if len(f) == 0 {
		return objects
	}
	filtered := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		if matchesFilter(obj, f) {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}

// newValueMatcher builds the matcher for a criterion's value: a string
// operator for an operator criterion, equality (valuesMatch) for anything
// else. String values, the operator's included, are held to
// maxFilterValueSize.
func newValueMatcher(expected interface{}) (valueMatcher, error) {
	op, operand, isOperator := operatorCriterion(expected)
	if !isOperator {
		if s, ok := expected.(string); ok && len(s) > maxFilterValueSize {
			return nil, fmt.Errorf("filter value too large: %d bytes (maximum allowed: %d)", len(s), maxFilterValueSize)
		}
		return equals(expected), nil
	}

	value, ok := operand.(string)
	if !ok {
		return nil, fmt.Errorf("op %q needs a string value, got %T", op, operand)
	}
	if len(value) > maxFilterValueSize {
		return nil, fmt.Errorf("filter value too large: %d bytes (maximum allowed: %d)", len(value), maxFilterValueSize)
	}

	var test func(string) bool
	switch op {
	case opPrefix:
		test = func(s string) bool { return strings.HasPrefix(s, value) }
	case opSuffix:
		test = func(s string) bool { return strings.HasSuffix(s, value) }
	case opContains:
		test = func(s string) bool { return strings.Contains(s, value) }
	case opRegex:
		re, err := compileRegex(value)
		if err != nil {
			return nil, fmt.Errorf("op %q: %w", op, err)
		}
		test = re.MatchString
	}

	return func(actual interface{}) bool {
		s, ok := actual.(string)
		return ok && test(s)
	}, nil
}

// compileRegex compiles a regex operator's pattern as Go RE2, whose matching
// cannot backtrack, and refuses a pattern whose compiled program exceeds
// maxRegexInstructions, since matching time grows with the program too.
func compileRegex(pattern string) (*regexp.Regexp, error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	if len(prog.Inst) > maxRegexInstructions {
		return nil, fmt.Errorf("regular expression too complex: %d instructions (maximum allowed: %d)", len(prog.Inst), maxRegexInstructions)
	}
	return regexp.Compile(pattern)
}

// operatorCriterion reports whether expected is an operator criterion — an
// object with exactly the keys "op", naming one of the string operators, and
// "value" — and returns its parts. Any other object, one whose "op" names
// anything else included, stays a nested map match, so existing filters on
// objects keep their meaning.
func operatorCriterion(expected interface{}) (op string, value interface{}, ok bool) {
	m, isMap := expected.(map[string]interface{})
	if !isMap || len(m) != 2 {
		return "", nil, false
	}
	op, _ = m["op"].(string)
	value, hasValue := m["value"]
	if !stringOperators[op] || !hasValue {
		return "", nil, false
	}
	return op, value, true
}

// equals is the matcher of a plain criterion value: equality as valuesMatch
// defines it.
func equals(expected interface{}) valueMatcher {
	return func(actual interface{}) bool { return valuesMatch(actual, expected) }
}

// matchesFilter checks if a single object matches all filter criteria.
//
// The criteria see the object as the tools show it (output.MaskedView):
// matched against the real data of a Secret, a filter would answer whether a
// value masking hides starts with a guess, and so give it away one prefix at
// a time.
func matchesFilter(obj runtime.Object, criteria clientFilter) bool {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return false
	}
	view := &unstructured.Unstructured{Object: output.MaskedView(unstructuredObj.Object)}

	// All criteria must match (AND logic)
	for _, c := range criteria {
		if !matchesFieldPath(view, c.path, c.match) {
			return false
		}
	}

	return true
}

// matchesFieldPath checks if a field path in the object satisfies match.
// Supports:
//   - Simple paths: "status.phase"
//   - Array wildcard paths: "spec.taints[*].key"
//   - Nested maps: "metadata.labels.app"
//   - The virtual "images" path: any container image of a pod or workload; on
//     a Kind without a PodSpec, "images" is an ordinary field path
func matchesFieldPath(obj *unstructured.Unstructured, path string, match valueMatcher) bool {
	if path == imagesPath {
		if spec := podSpec(obj); spec != nil {
			for _, image := range containerImages(spec) {
				if match(image) {
					return true
				}
			}
			return false
		}
	}

	// Check if this is an array wildcard path (contains [*])
	if strings.Contains(path, arrayWildcard) {
		return matchesArrayPath(obj, path, match)
	}

	// Regular path traversal
	parts := strings.Split(path, ".")
	value, found := getNestedValue(obj.Object, parts)
	if !found {
		return false
	}

	return match(value)
}

// matchesArrayPath handles paths with array wildcards like "spec.taints[*].key"
func matchesArrayPath(obj *unstructured.Unstructured, path string, match valueMatcher) bool {
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
			if match(elem) {
				return true
			}
		} else {
			// Traverse into the array element
			elemMap, ok := elem.(map[string]interface{})
			if !ok {
				continue
			}

			elemValue, found := getNestedValue(elemMap, remainingPath)
			if found && match(elemValue) {
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
