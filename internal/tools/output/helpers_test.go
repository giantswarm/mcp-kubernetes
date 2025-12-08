package output

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Test constants to avoid goconst warnings
const (
	testKindPod    = "Pod"
	testKindSecret = "Secret"
)

func TestFromRuntimeObjects(t *testing.T) {
	objects := []runtime.Object{
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": testKindPod,
				"metadata": map[string]interface{}{
					"name": "test-pod",
				},
			},
		},
	}

	result, err := FromRuntimeObjects(objects)
	if err != nil {
		t.Fatalf("FromRuntimeObjects failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result))
	}

	if result[0]["kind"] != testKindPod {
		t.Error("Kind should be Pod")
	}
}

func TestFromRuntimeObjects_Empty(t *testing.T) {
	result, err := FromRuntimeObjects([]runtime.Object{})
	if err != nil {
		t.Fatalf("FromRuntimeObjects failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(result))
	}
}

func TestToRuntimeObjects(t *testing.T) {
	maps := []map[string]interface{}{
		{
			"kind": testKindPod,
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
		},
	}

	result := ToRuntimeObjects(maps)

	if len(result) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result))
	}

	unstructuredObj, ok := result[0].(*unstructured.Unstructured)
	if !ok {
		t.Fatal("Expected unstructured.Unstructured")
	}

	if unstructuredObj.GetKind() != testKindPod {
		t.Error("Kind should be Pod")
	}
}

func TestProcessRuntimeObjects(t *testing.T) {
	cfg := &Config{
		MaxItems:    10,
		SlimOutput:  true,
		MaskSecrets: true,
	}
	processor := NewProcessor(cfg)

	objects := []runtime.Object{
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": testKindSecret,
				"metadata": map[string]interface{}{
					"name":          "test-secret",
					"managedFields": []interface{}{"field"},
				},
				"data": map[string]interface{}{
					"password": "c2VjcmV0",
				},
			},
		},
	}

	processed, result, err := ProcessRuntimeObjects(processor, objects)
	if err != nil {
		t.Fatalf("ProcessRuntimeObjects failed: %v", err)
	}

	if len(processed) != 1 {
		t.Errorf("Expected 1 processed item, got %d", len(processed))
	}

	if result.Metadata.OriginalCount != 1 {
		t.Errorf("OriginalCount = %d, want 1", result.Metadata.OriginalCount)
	}

	// Verify secret masking was applied
	unstructuredObj := processed[0].(*unstructured.Unstructured)
	data, _, _ := unstructured.NestedMap(unstructuredObj.Object, "data")
	if data["password"] != RedactedValue {
		t.Error("Secret data should be masked")
	}
}

func TestProcessRuntimeObjectsWithLimit(t *testing.T) {
	cfg := &Config{
		MaxItems:    100,
		SlimOutput:  false,
		MaskSecrets: false,
	}
	processor := NewProcessor(cfg)

	// Create 10 objects
	objects := make([]runtime.Object, 10)
	for i := range objects {
		objects[i] = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": testKindPod,
				"metadata": map[string]interface{}{
					"name": "pod-" + string(rune('a'+i)),
				},
			},
		}
	}

	// Process with limit of 5
	processed, result, err := ProcessRuntimeObjectsWithLimit(processor, objects, 5)
	if err != nil {
		t.Fatalf("ProcessRuntimeObjectsWithLimit failed: %v", err)
	}

	if len(processed) != 5 {
		t.Errorf("Expected 5 processed items, got %d", len(processed))
	}

	if !result.Metadata.Truncated {
		t.Error("Should be truncated")
	}
}

func TestProcessSingleRuntimeObject(t *testing.T) {
	cfg := &Config{
		SlimOutput:  true,
		MaskSecrets: true,
	}
	processor := NewProcessor(cfg)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": testKindSecret,
			"metadata": map[string]interface{}{
				"name":          "test-secret",
				"managedFields": []interface{}{"field"},
			},
			"data": map[string]interface{}{
				"password": "c2VjcmV0",
			},
		},
	}

	processed, err := ProcessSingleRuntimeObject(processor, obj)
	if err != nil {
		t.Fatalf("ProcessSingleRuntimeObject failed: %v", err)
	}

	unstructuredObj := processed.(*unstructured.Unstructured)

	// Check secret masking
	data, _, _ := unstructured.NestedMap(unstructuredObj.Object, "data")
	if data["password"] != RedactedValue {
		t.Error("Secret data should be masked")
	}

	// Check slim output removed managedFields
	_, found, _ := unstructured.NestedFieldNoCopy(unstructuredObj.Object, "metadata", "managedFields")
	if found {
		t.Error("managedFields should be removed")
	}
}

func TestAppendWarningsToResult(t *testing.T) {
	result := map[string]interface{}{
		"items": []string{"item1", "item2"},
	}

	// No warnings - should return unchanged
	unchanged := AppendWarningsToResult(result, nil)
	if unchanged["_warnings"] != nil {
		t.Error("Should not have warnings for empty warning list")
	}

	// With warnings
	warnings := []TruncationWarning{
		{Message: "Warning 1"},
		{Message: "Warning 2"},
	}
	withWarnings := AppendWarningsToResult(result, warnings)

	if withWarnings["_truncated"] != true {
		t.Error("Should have _truncated marker")
	}

	warningMsgs := withWarnings["_warnings"].([]string)
	if len(warningMsgs) != 2 {
		t.Errorf("Expected 2 warnings, got %d", len(warningMsgs))
	}
}

func TestFormatResultWithMetadata(t *testing.T) {
	items := []string{"item1", "item2"}
	metadata := ProcessingMetadata{
		Truncated:     true,
		OriginalCount: 10,
		FinalCount:    2,
	}
	warnings := []TruncationWarning{
		{Message: "Truncated"},
	}

	result := FormatResultWithMetadata(items, metadata, warnings)

	if result["items"] == nil {
		t.Error("Should have items")
	}
	if result["_truncated"] != true {
		t.Error("Should have _truncated marker")
	}
	if result["_originalCount"] != 10 {
		t.Error("Should have _originalCount")
	}
	if result["_returnedCount"] != 2 {
		t.Error("Should have _returnedCount")
	}
	if result["_warnings"] == nil {
		t.Error("Should have _warnings")
	}
}

func TestFormatResultWithMetadata_NoTruncation(t *testing.T) {
	items := []string{"item1"}
	metadata := ProcessingMetadata{
		Truncated:     false,
		OriginalCount: 1,
		FinalCount:    1,
	}

	result := FormatResultWithMetadata(items, metadata, nil)

	if result["_truncated"] != nil {
		t.Error("Should not have _truncated when not truncated")
	}
	if result["_warnings"] != nil {
		t.Error("Should not have _warnings when no warnings")
	}
}
