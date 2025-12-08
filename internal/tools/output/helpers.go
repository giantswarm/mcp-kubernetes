package output

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// FromRuntimeObjects converts a slice of runtime.Object to maps for processing.
func FromRuntimeObjects(objects []runtime.Object) ([]map[string]interface{}, error) {
	result := make([]map[string]interface{}, 0, len(objects))

	for _, obj := range objects {
		// Try to convert to unstructured first (most efficient)
		if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
			result = append(result, unstructuredObj.Object)
			continue
		}

		// Fall back to JSON marshal/unmarshal for typed objects
		jsonData, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal object: %w", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(jsonData, &m); err != nil {
			return nil, fmt.Errorf("failed to unmarshal object: %w", err)
		}

		result = append(result, m)
	}

	return result, nil
}

// ToRuntimeObjects converts a slice of maps back to runtime.Objects.
// Note: This returns unstructured objects, not typed objects.
func ToRuntimeObjects(maps []map[string]interface{}) []runtime.Object {
	result := make([]runtime.Object, 0, len(maps))

	for _, m := range maps {
		obj := &unstructured.Unstructured{Object: m}
		result = append(result, obj)
	}

	return result
}

// ProcessRuntimeObjects applies output processing to runtime.Objects.
// This is a convenience function for handlers that work with runtime.Object slices.
func ProcessRuntimeObjects(processor *Processor, objects []runtime.Object) ([]runtime.Object, *ProcessingResult, error) {
	// Convert to maps
	maps, err := FromRuntimeObjects(objects)
	if err != nil {
		return nil, nil, err
	}

	// Process
	result := processor.Process(maps)

	// Convert back to runtime.Objects
	processed := ToRuntimeObjects(result.Items)

	return processed, result, nil
}

// ProcessRuntimeObjectsWithLimit applies output processing with a custom limit.
func ProcessRuntimeObjectsWithLimit(processor *Processor, objects []runtime.Object, limit int) ([]runtime.Object, *ProcessingResult, error) {
	// Convert to maps
	maps, err := FromRuntimeObjects(objects)
	if err != nil {
		return nil, nil, err
	}

	// Process with limit
	result := processor.ProcessWithLimit(maps, limit)

	// Convert back to runtime.Objects
	processed := ToRuntimeObjects(result.Items)

	return processed, result, nil
}

// ProcessSingleRuntimeObject applies output processing to a single runtime.Object.
func ProcessSingleRuntimeObject(processor *Processor, obj runtime.Object) (runtime.Object, error) {
	// Try to convert to unstructured first
	if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
		processed := processor.ProcessSingle(unstructuredObj.Object)
		return &unstructured.Unstructured{Object: processed}, nil
	}

	// Fall back to JSON marshal/unmarshal
	jsonData, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(jsonData, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %w", err)
	}

	processed := processor.ProcessSingle(m)
	return &unstructured.Unstructured{Object: processed}, nil
}

// AppendWarningsToResult appends truncation warnings to a JSON result.
// The warnings are added to a "warnings" field if any exist.
func AppendWarningsToResult(result map[string]interface{}, warnings []TruncationWarning) map[string]interface{} {
	if len(warnings) == 0 {
		return result
	}

	// Add warnings to result
	warningMsgs := make([]string, 0, len(warnings))
	for _, w := range warnings {
		warningMsgs = append(warningMsgs, w.Message)
	}

	result["_warnings"] = warningMsgs
	result["_truncated"] = true

	return result
}

// FormatResultWithMetadata formats a result map with processing metadata.
func FormatResultWithMetadata(items interface{}, metadata ProcessingMetadata, warnings []TruncationWarning) map[string]interface{} {
	result := map[string]interface{}{
		"items": items,
	}

	if metadata.Truncated {
		result["_truncated"] = true
		result["_originalCount"] = metadata.OriginalCount
		result["_returnedCount"] = metadata.FinalCount
	}

	if len(warnings) > 0 {
		warningMsgs := make([]string, 0, len(warnings))
		for _, w := range warnings {
			warningMsgs = append(warningMsgs, w.Message)
		}
		result["_warnings"] = warningMsgs
	}

	return result
}
