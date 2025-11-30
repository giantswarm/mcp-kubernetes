package resource

import (
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
			result := ApplyClientSideFilter(tt.objects, tt.criteria)
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
			result := matchesFieldPath(tt.obj, tt.path, tt.expectedValue)
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
			result := matchesArrayPath(tt.obj, tt.path, tt.expectedValue)
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
