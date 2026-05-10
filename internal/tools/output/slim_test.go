package output

import (
	"testing"
)

func TestSlimResource(t *testing.T) {
	tests := []struct {
		name           string
		obj            map[string]interface{}
		excludedFields []string
		checkField     string
		shouldExist    bool
	}{
		{
			name:           "nil object",
			obj:            nil,
			excludedFields: nil,
			checkField:     "",
			shouldExist:    false,
		},
		{
			name: "removes managedFields",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":          "test",
					"managedFields": []interface{}{"field1", "field2"},
				},
			},
			excludedFields: []string{"metadata.managedFields"},
			checkField:     "metadata.managedFields",
			shouldExist:    false,
		},
		{
			name: "preserves non-excluded fields",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": "default",
				},
			},
			excludedFields: []string{"metadata.managedFields"},
			checkField:     "metadata.name",
			shouldExist:    true,
		},
		{
			name: "removes nested annotation",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "test",
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/last-applied-configuration": "long config",
						"custom-annotation": "keep this",
					},
				},
			},
			excludedFields: []string{"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration"},
			checkField:     "metadata.annotations.kubectl.kubernetes.io/last-applied-configuration",
			shouldExist:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SlimResource(tt.obj, tt.excludedFields)

			if tt.obj == nil {
				if result != nil {
					t.Error("Expected nil result for nil input")
				}
				return
			}

			// Check if field exists or not
			exists := fieldExists(result, tt.checkField)
			if exists != tt.shouldExist {
				t.Errorf("Field %q exists = %v, want %v", tt.checkField, exists, tt.shouldExist)
			}
		})
	}
}

func TestSlimResource_ArrayWildcard(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":               "Ready",
					"status":             "True",
					"lastTransitionTime": "2024-01-01T00:00:00Z",
				},
				map[string]interface{}{
					"type":               "Scheduled",
					"status":             "True",
					"lastTransitionTime": "2024-01-01T00:00:00Z",
				},
			},
		},
	}

	result := SlimResource(obj, []string{"status.conditions[*].lastTransitionTime"})

	// Verify conditions still exist
	conditions := result["status"].(map[string]interface{})["conditions"].([]interface{})
	if len(conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditions))
	}

	// Verify lastTransitionTime was removed from each condition
	for i, cond := range conditions {
		condMap := cond.(map[string]interface{})
		if _, ok := condMap["lastTransitionTime"]; ok {
			t.Errorf("Condition %d still has lastTransitionTime", i)
		}
		// Verify other fields preserved
		if condMap["type"] == nil || condMap["status"] == nil {
			t.Errorf("Condition %d missing type or status", i)
		}
	}
}

func TestSlimResource_DeepCopy(t *testing.T) {
	original := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":          "test",
			"managedFields": []interface{}{"field1"},
		},
	}

	result := SlimResource(original, []string{"metadata.managedFields"})

	// Verify original is not modified
	if original["metadata"].(map[string]interface{})["managedFields"] == nil {
		t.Error("Original object was modified")
	}

	// Verify result doesn't have managedFields
	if result["metadata"].(map[string]interface{})["managedFields"] != nil {
		t.Error("Result should not have managedFields")
	}
}

func TestSlimResources(t *testing.T) {
	objects := []map[string]interface{}{
		{
			"metadata": map[string]interface{}{
				"name":          "obj1",
				"managedFields": []interface{}{"field1"},
			},
		},
		{
			"metadata": map[string]interface{}{
				"name":          "obj2",
				"managedFields": []interface{}{"field2"},
			},
		},
	}

	result := SlimResources(objects, []string{"metadata.managedFields"})

	if len(result) != 2 {
		t.Errorf("Expected 2 results, got %d", len(result))
	}

	for i, obj := range result {
		if obj["metadata"].(map[string]interface{})["managedFields"] != nil {
			t.Errorf("Object %d still has managedFields", i)
		}
	}
}

func TestSlimResources_Empty(t *testing.T) {
	result := SlimResources([]map[string]interface{}{}, nil)
	if len(result) != 0 {
		t.Error("Expected empty result for empty input")
	}

	result = SlimResources(nil, nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestRemoveField_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		path string
	}{
		{
			name: "empty path",
			obj:  map[string]interface{}{"key": "value"},
			path: "",
		},
		{
			name: "non-existent path",
			obj:  map[string]interface{}{"key": "value"},
			path: "nonexistent.path",
		},
		{
			name: "path to non-map value",
			obj:  map[string]interface{}{"key": "value"},
			path: "key.subkey",
		},
		{
			name: "array wildcard on non-array",
			obj: map[string]interface{}{
				"data": "not-an-array",
			},
			path: "data[*].field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			removeField(tt.obj, tt.path)
		})
	}
}

// TestRemoveField_DotsInKeys pins the round-3 fix for dotted Kubernetes
// label / annotation keys. The original implementation split the path on
// every "." and so silently failed to strip
// "metadata.annotations.kubectl.kubernetes.io/last-applied-configuration"
// even though that exact string was in the default excluded fields list.
// The greedy-join logic should now find and remove it.
func TestRemoveField_DotsInKeys(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		obj        map[string]interface{}
		mustBeGone []string
		mustRemain []string
	}{
		{
			name: "annotation with dots and slash",
			path: "metadata.annotations.kubectl.kubernetes.io/last-applied-configuration",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/last-applied-configuration": "{...}",
						"keep.me": "yes",
					},
				},
			},
			mustBeGone: []string{"kubectl.kubernetes.io/last-applied-configuration"},
			mustRemain: []string{"keep.me"},
		},
		{
			name: "deployment.kubernetes.io/revision annotation",
			path: "metadata.annotations.deployment.kubernetes.io/revision",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"deployment.kubernetes.io/revision": "42",
						"meta.helm.sh/release-name":         "mychart",
					},
				},
			},
			mustBeGone: []string{"deployment.kubernetes.io/revision"},
			mustRemain: []string{"meta.helm.sh/release-name"},
		},
		{
			name: "label with dots",
			path: "metadata.labels.app.kubernetes.io/name",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app.kubernetes.io/name": "demo",
						"keep":                   "x",
					},
				},
			},
			mustBeGone: []string{"app.kubernetes.io/name"},
			mustRemain: []string{"keep"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeField(tt.obj, tt.path)

			leaf := tt.obj["metadata"].(map[string]interface{})
			// The leaf could be either annotations or labels - figure out from path.
			var sub map[string]interface{}
			switch {
			case leaf["annotations"] != nil:
				sub = leaf["annotations"].(map[string]interface{})
			case leaf["labels"] != nil:
				sub = leaf["labels"].(map[string]interface{})
			}

			for _, k := range tt.mustBeGone {
				if _, ok := sub[k]; ok {
					t.Errorf("expected key %q to be removed, but it is still present", k)
				}
			}
			for _, k := range tt.mustRemain {
				if _, ok := sub[k]; !ok {
					t.Errorf("expected key %q to remain, but it was removed", k)
				}
			}
		})
	}
}

// TestRemoveField_DotKeyPrecedence pins the precedence rule documented on
// removeField: when both a literal dotted key (e.g. "a.b") and a nested
// traversal (obj["a"]["b"]) would resolve a path, the longer literal match
// wins. Real Kubernetes data never makes this ambiguous (annotations /
// labels are flat string maps), but the rule is load-bearing for the
// dot-key fix and worth pinning so a future refactor can't quietly swap to
// shortest-match without a failing test.
func TestRemoveField_DotKeyPrecedence(t *testing.T) {
	obj := map[string]interface{}{
		"foo.bar": "literal-wins",
		"foo": map[string]interface{}{
			"bar": "nested-loses",
		},
	}

	removeField(obj, "foo.bar")

	if _, ok := obj["foo.bar"]; ok {
		t.Errorf("literal dotted key %q must be removed by greedy match", "foo.bar")
	}
	nested, ok := obj["foo"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected obj[\"foo\"] to remain a map, got %T", obj["foo"])
	}
	if got, ok := nested["bar"]; !ok || got != "nested-loses" {
		t.Errorf("nested obj[\"foo\"][\"bar\"] must be untouched, got ok=%v val=%v", ok, got)
	}
}

// TestSlimResource_TypedStringMaps pins the bug fix for typed string-keyed
// maps. unstructured.GetAnnotations / GetLabels return map[string]string,
// not map[string]interface{}. Before the fix, deepCopyValue and the
// recursive remover both bailed out on map[string]string, so paths like
// "metadata.annotations.kubectl.kubernetes.io/last-applied-configuration"
// silently no-op'd whenever the convenience metadata exposed by the
// describe handler was processed.
func TestSlimResource_TypedStringMaps(t *testing.T) {
	in := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{...}",
				"meta.helm.sh/release-name":                        "demo",
			},
			"labels": map[string]string{
				"app.kubernetes.io/name": "demo",
				"keep":                   "x",
			},
		},
	}

	out := SlimResource(in, []string{
		"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration",
		"metadata.labels.app.kubernetes.io/name",
	})

	md := out["metadata"].(map[string]interface{})
	ann := md["annotations"].(map[string]interface{})
	if _, ok := ann["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Errorf("expected last-applied-configuration to be stripped from typed string annotations")
	}
	if _, ok := ann["meta.helm.sh/release-name"]; !ok {
		t.Errorf("expected unrelated annotation to be preserved")
	}
	lbl := md["labels"].(map[string]interface{})
	if _, ok := lbl["app.kubernetes.io/name"]; ok {
		t.Errorf("expected app.kubernetes.io/name to be stripped from typed string labels")
	}
	if _, ok := lbl["keep"]; !ok {
		t.Errorf("expected unrelated label to be preserved")
	}

	origMd := in["metadata"].(map[string]interface{})
	origAnn := origMd["annotations"].(map[string]string)
	if _, ok := origAnn["kubectl.kubernetes.io/last-applied-configuration"]; !ok {
		t.Errorf("original input must not be mutated")
	}
}

func TestDeepCopyMap(t *testing.T) {
	original := map[string]interface{}{
		"string": "value",
		"int":    42,
		"bool":   true,
		"nested": map[string]interface{}{
			"inner": "data",
		},
		"array": []interface{}{1, 2, 3},
	}

	copy := deepCopyMap(original)

	// Verify values are equal
	if copy["string"] != original["string"] {
		t.Error("String value mismatch")
	}
	if copy["int"] != original["int"] {
		t.Error("Int value mismatch")
	}
	if copy["bool"] != original["bool"] {
		t.Error("Bool value mismatch")
	}

	// Modify copy and verify original is unchanged
	copy["string"] = "modified"
	copy["nested"].(map[string]interface{})["inner"] = "modified"
	copy["array"].([]interface{})[0] = 999

	if original["string"] != "value" {
		t.Error("Original string was modified")
	}
	if original["nested"].(map[string]interface{})["inner"] != "data" {
		t.Error("Original nested value was modified")
	}
	if original["array"].([]interface{})[0] != 1 {
		t.Error("Original array was modified")
	}
}

func TestDeepCopyMap_Nil(t *testing.T) {
	result := deepCopyMap(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestEstimateFieldSize(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"managedFields": []interface{}{
				map[string]interface{}{"field": "value"},
			},
		},
	}

	size := EstimateFieldSize(obj, "metadata.managedFields")
	if size <= 0 {
		t.Error("Expected positive size estimate")
	}

	// Non-existent field should return 0
	size = EstimateFieldSize(obj, "nonexistent.path")
	if size != 0 {
		t.Errorf("Expected 0 for non-existent field, got %d", size)
	}
}

// Helper to check if a field exists at a path
func fieldExists(obj map[string]interface{}, path string) bool {
	if path == "" || obj == nil {
		return false
	}

	value := getFieldValue(obj, path)
	return value != nil
}
