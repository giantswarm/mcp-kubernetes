package output

import (
	"testing"
)

func TestNewProcessor(t *testing.T) {
	// With nil config
	p := NewProcessor(nil)
	if p == nil {
		t.Fatal("NewProcessor(nil) returned nil")
	}
	if p.config == nil {
		t.Error("Processor config should not be nil")
	}

	// With custom config
	cfg := &Config{
		MaxItems:    50,
		MaskSecrets: false,
	}
	p = NewProcessor(cfg)
	if p.config.MaxItems != 50 {
		t.Errorf("MaxItems = %d, want 50", p.config.MaxItems)
	}
}

func TestProcessor_Process(t *testing.T) {
	// Create processor with slim output and secret masking
	cfg := &Config{
		MaxItems:    10,
		SlimOutput:  true,
		MaskSecrets: true,
	}
	p := NewProcessor(cfg)

	items := []map[string]interface{}{
		{
			"kind": "Secret",
			"metadata": map[string]interface{}{
				"name":          "test-secret",
				"managedFields": []interface{}{"field"},
			},
			"data": map[string]interface{}{
				"password": "c2VjcmV0",
			},
		},
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name":          "test-pod",
				"managedFields": []interface{}{"field"},
			},
		},
	}

	result := p.Process(items)

	// Check metadata
	if result.Metadata.OriginalCount != 2 {
		t.Errorf("OriginalCount = %d, want 2", result.Metadata.OriginalCount)
	}
	if result.Metadata.FinalCount != 2 {
		t.Errorf("FinalCount = %d, want 2", result.Metadata.FinalCount)
	}
	if !result.Metadata.SecretsMasked {
		t.Error("SecretsMasked should be true")
	}
	if !result.Metadata.SlimApplied {
		t.Error("SlimApplied should be true")
	}

	// Check secret masking was applied
	secret := result.Items[0]
	if secret["data"].(map[string]interface{})["password"] != RedactedValue {
		t.Error("Secret data should be masked")
	}

	// Check slim output removed managedFields
	if secret["metadata"].(map[string]interface{})["managedFields"] != nil {
		t.Error("managedFields should be removed by slim output")
	}
}

func TestProcessor_Process_EmptyList(t *testing.T) {
	p := NewProcessor(nil)
	result := p.Process([]map[string]interface{}{})

	if result.Metadata.FinalCount != 0 {
		t.Errorf("FinalCount = %d, want 0", result.Metadata.FinalCount)
	}
	if len(result.Items) != 0 {
		t.Error("Items should be empty")
	}
}

func TestProcessor_Process_Truncation(t *testing.T) {
	cfg := &Config{
		MaxItems:    5,
		SlimOutput:  false,
		MaskSecrets: false,
	}
	p := NewProcessor(cfg)

	items := makeTestMaps(10)
	result := p.Process(items)

	if result.Metadata.FinalCount != 5 {
		t.Errorf("FinalCount = %d, want 5", result.Metadata.FinalCount)
	}
	if !result.Metadata.Truncated {
		t.Error("Truncated should be true")
	}
	if len(result.Warnings) == 0 {
		t.Error("Should have truncation warning")
	}
}

func TestProcessor_ProcessWithLimit(t *testing.T) {
	cfg := &Config{
		MaxItems:    100,
		SlimOutput:  false,
		MaskSecrets: false,
	}
	p := NewProcessor(cfg)

	items := makeTestMaps(50)

	// Custom limit lower than config
	result := p.ProcessWithLimit(items, 20)

	if result.Metadata.FinalCount != 20 {
		t.Errorf("FinalCount = %d, want 20", result.Metadata.FinalCount)
	}
	if !result.Metadata.Truncated {
		t.Error("Truncated should be true")
	}
}

func TestProcessor_ProcessWithLimit_Zero(t *testing.T) {
	cfg := &Config{
		MaxItems:    10,
		SlimOutput:  false,
		MaskSecrets: false,
	}
	p := NewProcessor(cfg)

	items := makeTestMaps(5)

	// Zero limit should use config limit
	result := p.ProcessWithLimit(items, 0)

	if result.Metadata.FinalCount != 5 {
		t.Errorf("FinalCount = %d, want 5", result.Metadata.FinalCount)
	}
}

func TestProcessor_ProcessSingle(t *testing.T) {
	cfg := &Config{
		SlimOutput:  true,
		MaskSecrets: true,
	}
	p := NewProcessor(cfg)

	item := map[string]interface{}{
		"kind": "Secret",
		"metadata": map[string]interface{}{
			"name":          "test",
			"managedFields": []interface{}{"field"},
		},
		"data": map[string]interface{}{
			"key": "value",
		},
	}

	result := p.ProcessSingle(item)

	// Check secret masking
	if result["data"].(map[string]interface{})["key"] != RedactedValue {
		t.Error("Secret data should be masked")
	}

	// Check slim output
	if result["metadata"].(map[string]interface{})["managedFields"] != nil {
		t.Error("managedFields should be removed")
	}
}

func TestProcessor_ProcessSingle_Nil(t *testing.T) {
	p := NewProcessor(nil)
	result := p.ProcessSingle(nil)

	if result != nil {
		t.Error("ProcessSingle(nil) should return nil")
	}
}

func TestProcessor_GenerateSummary(t *testing.T) {
	p := NewProcessor(nil)

	items := makeTestPods(10)
	summary := p.GenerateSummary(items, nil)

	if summary.Total != 10 {
		t.Errorf("Total = %d, want 10", summary.Total)
	}
}

func TestProcessor_ShouldSuggestSummary(t *testing.T) {
	cfg := &Config{
		SummaryThreshold: 100,
	}
	p := NewProcessor(cfg)

	if p.ShouldSuggestSummary(50) {
		t.Error("Should not suggest summary for 50 items with threshold 100")
	}
	if !p.ShouldSuggestSummary(150) {
		t.Error("Should suggest summary for 150 items with threshold 100")
	}
}

func TestProcessor_Config(t *testing.T) {
	cfg := &Config{
		MaxItems:    42,
		SlimOutput:  true,
		MaskSecrets: false,
	}
	p := NewProcessor(cfg)

	if p.Config().MaxItems != 42 {
		t.Error("Config() should return processor config")
	}
}

func TestProcessWithStats(t *testing.T) {
	cfg := &Config{
		MaxItems:    5,
		SlimOutput:  true,
		MaskSecrets: true,
	}
	p := NewProcessor(cfg)

	items := []map[string]interface{}{
		{"kind": "Secret", "data": map[string]interface{}{"key": "val"}},
		{"kind": "Pod"},
		{"kind": "Secret", "data": map[string]interface{}{"key": "val"}},
	}
	items = append(items, makeTestMaps(7)...) // Total 10 items

	result, stats := p.ProcessWithStats(items)

	if stats.ItemsProcessed != 10 {
		t.Errorf("ItemsProcessed = %d, want 10", stats.ItemsProcessed)
	}
	if stats.ItemsTruncated != 5 {
		t.Errorf("ItemsTruncated = %d, want 5", stats.ItemsTruncated)
	}
	if stats.SecretsRedacted != 2 {
		t.Errorf("SecretsRedacted = %d, want 2", stats.SecretsRedacted)
	}
	if stats.FieldsRemoved == 0 {
		t.Error("FieldsRemoved should be positive when slim output is enabled")
	}
	if stats.ProcessingTimeMs < 0 {
		t.Error("ProcessingTimeMs should not be negative")
	}

	// Verify result is also returned
	if result.Metadata.FinalCount != 5 {
		t.Errorf("Result FinalCount = %d, want 5", result.Metadata.FinalCount)
	}
}

func TestQuickProcess(t *testing.T) {
	items := []map[string]interface{}{
		{
			"kind": "Secret",
			"data": map[string]interface{}{
				"password": "secret",
			},
		},
	}

	result := QuickProcess(items)

	// Should apply defaults (mask secrets, slim output)
	if result.Items[0]["data"].(map[string]interface{})["password"] != RedactedValue {
		t.Error("QuickProcess should mask secrets by default")
	}
}

func TestQuickProcessSingle(t *testing.T) {
	item := map[string]interface{}{
		"kind": "Secret",
		"data": map[string]interface{}{
			"password": "secret",
		},
	}

	result := QuickProcessSingle(item)

	// Should apply defaults (mask secrets, slim output)
	if result["data"].(map[string]interface{})["password"] != RedactedValue {
		t.Error("QuickProcessSingle should mask secrets by default")
	}
}
