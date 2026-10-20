package resource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestApplyClientSideFilter(t *testing.T) {
	tests := []struct {
		name     string
		objects  []runtime.Object
		criteria FilterCriteria
		expected int
	}{
		{
			name: "filter by simple field - metadata name",
			objects: []runtime.Object{
				createTestNode("node1", "Ready"),
				createTestNode("node2", "NotReady"),
				createTestNode("node3", "Ready"),
			},
			criteria: FilterCriteria{
				"metadata.name": "node1",
			},
			expected: 1,
		},
		{
			name: "filter by taint key",
			objects: []runtime.Object{
				createTestNodeWithTaint("node1", "node.kubernetes.io/unschedulable", "NoSchedule"),
				createTestNodeWithTaint("node2", "karpenter.sh/unregistered", "NoExecute"),
				createTestNode("node3", "Ready"),
			},
			criteria: FilterCriteria{
				"spec.taints[*].key": "karpenter.sh/unregistered",
			},
			expected: 1,
		},
		{
			name: "filter by multiple taints",
			objects: []runtime.Object{
				createTestNodeWithMultipleTaints("node1", []taint{
					{Key: "karpenter.sh/unregistered", Effect: "NoExecute"},
					{Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule"},
				}),
				createTestNodeWithTaint("node2", "karpenter.sh/unregistered", "NoExecute"),
				createTestNode("node3", "Ready"),
			},
			criteria: FilterCriteria{
				"spec.taints[*].key": "karpenter.sh/unregistered",
			},
			expected: 2,
		},
		{
			name: "filter by label",
			objects: []runtime.Object{
				createTestPodWithLabel("pod1", "app", "nginx"),
				createTestPodWithLabel("pod2", "app", "redis"),
				createTestPodWithLabel("pod3", "app", "nginx"),
			},
			criteria: FilterCriteria{
				"metadata.labels.app": "nginx",
			},
			expected: 2,
		},
		{
			name: "no matching resources",
			objects: []runtime.Object{
				createTestNode("node1", "Ready"),
				createTestNode("node2", "Ready"),
			},
			criteria: FilterCriteria{
				"spec.taints[*].key": "nonexistent",
			},
			expected: 0,
		},
		{
			name: "empty criteria returns all",
			objects: []runtime.Object{
				createTestNode("node1", "Ready"),
				createTestNode("node2", "Ready"),
			},
			criteria: FilterCriteria{},
			expected: 2,
		},
		{
			name: "multiple criteria (AND logic)",
			objects: []runtime.Object{
				createTestNodeWithTaint("node1", "karpenter.sh/unregistered", "NoExecute"),
				createTestNodeWithTaint("node2", "other-taint", "NoExecute"),
			},
			criteria: FilterCriteria{
				"spec.taints[*].key":    "karpenter.sh/unregistered",
				"spec.taints[*].effect": "NoExecute",
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ApplyClientSideFilter(tt.objects, tt.criteria)
			assert.NoError(t, err, "unexpected error during filtering")
			assert.Equal(t, tt.expected, len(result), "unexpected number of filtered resources")
		})
	}
}

func TestMatchesFieldPath(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		path          string
		expectedValue interface{}
		shouldMatch   bool
	}{
		{
			name:          "simple string field match",
			obj:           createTestNode("node1", "Ready"),
			path:          "metadata.name",
			expectedValue: "node1",
			shouldMatch:   true,
		},
		{
			name:          "simple string field no match",
			obj:           createTestNode("node1", "Ready"),
			path:          "metadata.name",
			expectedValue: "node2",
			shouldMatch:   false,
		},
		{
			name:          "nested field match",
			obj:           createTestPodWithLabel("pod1", "app", "nginx"),
			path:          "metadata.labels.app",
			expectedValue: "nginx",
			shouldMatch:   true,
		},
		{
			name:          "array wildcard match",
			obj:           createTestNodeWithTaint("node1", "karpenter.sh/unregistered", "NoExecute"),
			path:          "spec.taints[*].key",
			expectedValue: "karpenter.sh/unregistered",
			shouldMatch:   true,
		},
		{
			name:          "array wildcard no match",
			obj:           createTestNodeWithTaint("node1", "other-taint", "NoExecute"),
			path:          "spec.taints[*].key",
			expectedValue: "karpenter.sh/unregistered",
			shouldMatch:   false,
		},
		{
			name:          "nonexistent field",
			obj:           createTestNode("node1", "Ready"),
			path:          "nonexistent.field",
			expectedValue: "value",
			shouldMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesFieldPath(tt.obj, tt.path, equals(tt.expectedValue))
			assert.Equal(t, tt.shouldMatch, result, "unexpected match result")
		})
	}
}

func TestMatchesArrayPath(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		path          string
		expectedValue interface{}
		shouldMatch   bool
	}{
		{
			name: "taint key match",
			obj: createTestNodeWithMultipleTaints("node1", []taint{
				{Key: "taint1", Effect: "NoSchedule"},
				{Key: "karpenter.sh/unregistered", Effect: "NoExecute"},
			}),
			path:          "spec.taints[*].key",
			expectedValue: "karpenter.sh/unregistered",
			shouldMatch:   true,
		},
		{
			name: "taint effect match",
			obj: createTestNodeWithMultipleTaints("node1", []taint{
				{Key: "taint1", Effect: "NoSchedule"},
				{Key: "taint2", Effect: "NoExecute"},
			}),
			path:          "spec.taints[*].effect",
			expectedValue: "NoExecute",
			shouldMatch:   true,
		},
		{
			name:          "empty array",
			obj:           createTestNode("node1", "Ready"),
			path:          "spec.taints[*].key",
			expectedValue: "anything",
			shouldMatch:   false,
		},
		{
			name:          "no matching element",
			obj:           createTestNodeWithTaint("node1", "other-taint", "NoSchedule"),
			path:          "spec.taints[*].key",
			expectedValue: "karpenter.sh/unregistered",
			shouldMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesArrayPath(tt.obj, tt.path, equals(tt.expectedValue))
			assert.Equal(t, tt.shouldMatch, result, "unexpected array path match result")
		})
	}
}

func TestValuesMatch(t *testing.T) {
	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
		match    bool
	}{
		{
			name:     "string match",
			actual:   "hello",
			expected: "hello",
			match:    true,
		},
		{
			name:     "string no match",
			actual:   "hello",
			expected: "world",
			match:    false,
		},
		{
			name:     "int match",
			actual:   42,
			expected: 42,
			match:    true,
		},
		{
			name:     "float match",
			actual:   3.14,
			expected: 3.14,
			match:    true,
		},
		{
			name:     "bool match",
			actual:   true,
			expected: true,
			match:    true,
		},
		{
			name:     "bool no match",
			actual:   true,
			expected: false,
			match:    false,
		},
		{
			name:     "nil match",
			actual:   nil,
			expected: nil,
			match:    true,
		},
		{
			name:     "one nil",
			actual:   "value",
			expected: nil,
			match:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := valuesMatch(tt.actual, tt.expected)
			assert.Equal(t, tt.match, result, "unexpected value match result")
		})
	}
}

func TestMapsMatch(t *testing.T) {
	tests := []struct {
		name     string
		actual   map[string]interface{}
		expected map[string]interface{}
		match    bool
	}{
		{
			name:     "exact match",
			actual:   map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
			match:    true,
		},
		{
			name:     "actual has extra keys",
			actual:   map[string]interface{}{"key": "value", "extra": "ignored"},
			expected: map[string]interface{}{"key": "value"},
			match:    true,
		},
		{
			name:     "value mismatch",
			actual:   map[string]interface{}{"key": "wrong"},
			expected: map[string]interface{}{"key": "value"},
			match:    false,
		},
		{
			name:     "missing key in actual",
			actual:   map[string]interface{}{},
			expected: map[string]interface{}{"key": "value"},
			match:    false,
		},
		{
			name:     "empty maps",
			actual:   map[string]interface{}{},
			expected: map[string]interface{}{},
			match:    true,
		},
		{
			name: "nested matching",
			actual: map[string]interface{}{
				"outer": "value1",
				"inner": "value2",
			},
			expected: map[string]interface{}{
				"outer": "value1",
			},
			match: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapsMatch(tt.actual, tt.expected)
			assert.Equal(t, tt.match, result, "unexpected map match result")
		})
	}
}

// Helper functions to create test objects

type taint struct {
	Key    string
	Effect string
}

func createTestNode(name, status string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   status,
						"status": "True",
					},
				},
			},
		},
	}
}

func createTestNodeWithTaint(name, taintKey, taintEffect string) *unstructured.Unstructured {
	return createTestNodeWithMultipleTaints(name, []taint{{Key: taintKey, Effect: taintEffect}})
}

func createTestNodeWithMultipleTaints(name string, taints []taint) *unstructured.Unstructured {
	node := createTestNode(name, "Ready")

	taintList := make([]interface{}, len(taints))
	for i, t := range taints {
		taintList[i] = map[string]interface{}{
			"key":    t.Key,
			"effect": t.Effect,
		}
	}

	spec := map[string]interface{}{
		"taints": taintList,
	}
	node.Object["spec"] = spec

	return node
}

func createTestPodWithLabel(name, labelKey, labelValue string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					labelKey: labelValue,
				},
			},
		},
	}
}

// TestSecurityLimits tests the security validations for filter criteria
func TestSecurityLimits(t *testing.T) {
	tests := []struct {
		name          string
		criteria      FilterCriteria
		expectError   bool
		errorContains string
	}{
		{
			name: "too many filter criteria",
			criteria: func() FilterCriteria {
				c := make(FilterCriteria)
				for i := 0; i < maxFilterCriteria+1; i++ {
					c[fmt.Sprintf("field%d", i)] = "value"
				}
				return c
			}(),
			expectError:   true,
			errorContains: "too many filter criteria",
		},
		{
			name: "path too deep",
			criteria: FilterCriteria{
				"a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z": "value",
			},
			expectError:   true,
			errorContains: "path too deep",
		},
		{
			name: "empty path",
			criteria: FilterCriteria{
				"": "value",
			},
			expectError:   true,
			errorContains: "path cannot be empty",
		},
		{
			name: "path with double dots",
			criteria: FilterCriteria{
				"metadata..name": "value",
			},
			expectError:   true,
			errorContains: "invalid pattern '..'",
		},
		{
			name: "filter value too large",
			criteria: FilterCriteria{
				"metadata.name": func() string {
					// Create a string larger than maxFilterValueSize
					largeValue := make([]byte, maxFilterValueSize+1)
					for i := range largeValue {
						largeValue[i] = 'a'
					}
					return string(largeValue)
				}(),
			},
			expectError:   true,
			errorContains: "value too large",
		},
		{
			name: "valid criteria at limits",
			criteria: FilterCriteria{
				"metadata.name": "test",
				"spec.replicas": 3,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{createTestNode("node1", "Ready")}
			_, err := ApplyClientSideFilter(objects, tt.criteria)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "error message doesn't contain expected text")
				}
			} else {
				assert.NoError(t, err, "unexpected error")
			}
		})
	}
}

// testPodSpecPaths is where each test Kind keeps its PodSpec, spelled out
// rather than taken from podSpecFields so a wrong path there fails here.
var testPodSpecPaths = map[string][]string{
	"Pod":        {"spec"},
	"Deployment": {"spec", "template", "spec"},
	"CronJob":    {"spec", "jobTemplate", "spec", "template", "spec"},
	"Service":    {"spec"},
}

// createTestWorkload builds an object of kind whose PodSpec holds a
// container per image under each PodSpec field (containers,
// initContainers, ephemeralContainers).
func createTestWorkload(kind, name string, images map[string][]string) *unstructured.Unstructured {
	podSpec := map[string]interface{}{}
	for field, fieldImages := range images {
		containers := make([]interface{}, 0, len(fieldImages))
		for i, image := range fieldImages {
			containers = append(containers, map[string]interface{}{"name": fmt.Sprintf("c%d", i), "image": image})
		}
		podSpec[field] = containers
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": "default"},
	}}
	if err := unstructured.SetNestedField(obj.Object, podSpec, testPodSpecPaths[kind]...); err != nil {
		panic(err)
	}
	return obj
}

// filteredNames returns the names of the objects the criteria keep.
func filteredNames(t *testing.T, objects []runtime.Object, criteria FilterCriteria) []string {
	t.Helper()
	result, err := ApplyClientSideFilter(objects, criteria)
	assert.NoError(t, err)
	names := []string{}
	for _, obj := range result {
		names = append(names, obj.(*unstructured.Unstructured).GetName())
	}
	return names
}

func TestApplyClientSideFilter_Operators(t *testing.T) {
	objects := []runtime.Object{
		createTestWorkload("Pod", "web", map[string][]string{
			"containers": {"registry.example/org/web:1.0"},
		}),
		createTestWorkload("Pod", "sidecar", map[string][]string{
			"containers":     {"docker.io/library/nginx:1.25"},
			"initContainers": {"registry.example/org/init:2"},
		}),
		createTestWorkload("Pod", "other", map[string][]string{
			"containers": {"quay.io/other/app:3"},
		}),
		createTestNode("node1", "Ready"),
	}
	op := func(op, value string) map[string]interface{} {
		return map[string]interface{}{"op": op, "value": value}
	}

	tests := []struct {
		name     string
		criteria FilterCriteria
		expected []string
	}{
		{
			name:     "prefix on an array path",
			criteria: FilterCriteria{"spec.containers[*].image": op("prefix", "registry.example/org/")},
			expected: []string{"web"},
		},
		{
			name:     "prefix on images covers initContainers",
			criteria: FilterCriteria{"images": op("prefix", "registry.example/org/")},
			expected: []string{"web", "sidecar"},
		},
		{
			name:     "suffix",
			criteria: FilterCriteria{"images": op("suffix", ":1.25")},
			expected: []string{"sidecar"},
		},
		{
			name:     "contains",
			criteria: FilterCriteria{"images": op("contains", "/other/")},
			expected: []string{"other"},
		},
		{
			name:     "regex",
			criteria: FilterCriteria{"images": op("regex", `^(quay|docker)\.io/`)},
			expected: []string{"sidecar", "other"},
		},
		{
			name:     "exact value on images",
			criteria: FilterCriteria{"images": "quay.io/other/app:3"},
			expected: []string{"other"},
		},
		{
			name:     "exact value on an array path keeps working",
			criteria: FilterCriteria{"spec.containers[*].image": "docker.io/library/nginx:1.25"},
			expected: []string{"sidecar"},
		},
		{
			name:     "prefix on a scalar path",
			criteria: FilterCriteria{"metadata.name": op("prefix", "we")},
			expected: []string{"web"},
		},
		{
			name: "operators AND with other criteria",
			criteria: FilterCriteria{
				"images":        op("prefix", "registry.example/"),
				"metadata.name": op("contains", "car"),
			},
			expected: []string{"sidecar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, filteredNames(t, objects, tt.criteria))
		})
	}
}

// TestApplyClientSideFilter_ImagesAcrossKinds: the images path reads the
// PodSpec wherever a Kind keeps it, and every container field in it.
func TestApplyClientSideFilter_ImagesAcrossKinds(t *testing.T) {
	objects := []runtime.Object{
		createTestWorkload("Deployment", "deploy-init", map[string][]string{
			"containers":     {"quay.io/app:1"},
			"initContainers": {"registry.example/org/migrate:1"},
		}),
		createTestWorkload("Deployment", "deploy-elsewhere", map[string][]string{
			"containers": {"quay.io/app:1"},
		}),
		createTestWorkload("CronJob", "cron", map[string][]string{
			"containers": {"registry.example/org/backup:4"},
		}),
		createTestWorkload("Pod", "debugged", map[string][]string{
			"containers":          {"quay.io/app:1"},
			"ephemeralContainers": {"registry.example/org/debug:1"},
		}),
		createTestWorkload("Service", "svc", map[string][]string{
			"containers": {"registry.example/org/not-a-podspec:1"},
		}),
	}

	criteria := FilterCriteria{"images": map[string]interface{}{"op": "prefix", "value": "registry.example/org/"}}
	assert.Equal(t, []string{"deploy-init", "cron", "debugged"}, filteredNames(t, objects, criteria))
}

func TestApplyClientSideFilter_OperatorErrors(t *testing.T) {
	tests := []struct {
		name          string
		value         interface{}
		errorContains string
	}{
		{"unknown op", map[string]interface{}{"op": "glob", "value": "x*"}, `unknown op "glob"`},
		{"non-string value", map[string]interface{}{"op": "prefix", "value": 3}, "needs a string value"},
		{"invalid regex", map[string]interface{}{"op": "regex", "value": "("}, "invalid regular expression"},
		{"value too large", map[string]interface{}{"op": "contains", "value": strings.Repeat("a", maxFilterValueSize+1)}, "value too large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{createTestNode("node1", "Ready")}
			_, err := ApplyClientSideFilter(objects, FilterCriteria{"metadata.name": tt.value})
			assert.ErrorContains(t, err, tt.errorContains)
			assert.ErrorContains(t, err, `filter "metadata.name"`)
		})
	}
}

// TestNewValueMatcher_ObjectsStayNestedMatches: only an object of exactly
// "op" (a string) and "value" is an operator; any other object keeps its
// nested-map meaning.
func TestNewValueMatcher_ObjectsStayNestedMatches(t *testing.T) {
	actual := map[string]interface{}{"op": "add", "value": "x", "path": "/a"}

	match, err := newValueMatcher(map[string]interface{}{"op": "add", "value": "x", "path": "/a"})
	assert.NoError(t, err)
	assert.True(t, match(actual), "an object with more keys than op and value is a nested match")

	match, err = newValueMatcher(map[string]interface{}{"op": 1, "value": "x"})
	assert.NoError(t, err)
	assert.True(t, match(map[string]interface{}{"op": 1, "value": "x"}), "a non-string op is a nested match")
}

func TestNewValueMatcher_StringOperatorsNeedStrings(t *testing.T) {
	match, err := newValueMatcher(map[string]interface{}{"op": "prefix", "value": "3"})
	assert.NoError(t, err)
	assert.True(t, match("3x"))
	assert.False(t, match(int64(3)), "a number is not a string")
	assert.False(t, match(nil))
}
