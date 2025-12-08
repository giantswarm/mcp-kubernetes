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
