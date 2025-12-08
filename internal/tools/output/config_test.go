package output

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Verify defaults
	if cfg.MaxItems != DefaultMaxItems {
		t.Errorf("MaxItems = %d, want %d", cfg.MaxItems, DefaultMaxItems)
	}
	if cfg.MaxClusters != DefaultMaxClusters {
		t.Errorf("MaxClusters = %d, want %d", cfg.MaxClusters, DefaultMaxClusters)
	}
	if cfg.MaxResponseBytes != DefaultMaxResponseBytes {
		t.Errorf("MaxResponseBytes = %d, want %d", cfg.MaxResponseBytes, DefaultMaxResponseBytes)
	}
	if !cfg.SlimOutput {
		t.Error("SlimOutput should be true by default")
	}
	if !cfg.MaskSecrets {
		t.Error("MaskSecrets should be true by default")
	}
	if cfg.SummaryThreshold != 500 {
		t.Errorf("SummaryThreshold = %d, want 500", cfg.SummaryThreshold)
	}
	if len(cfg.ExcludedFields) == 0 {
		t.Error("ExcludedFields should have default values")
	}
}

func TestDefaultExcludedFields(t *testing.T) {
	fields := DefaultExcludedFields()

	if len(fields) == 0 {
		t.Fatal("DefaultExcludedFields returned empty slice")
	}

	// Verify expected fields are present
	expectedFields := []string{
		"metadata.managedFields",
		"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration",
	}

	for _, expected := range expectedFields {
		found := false
		for _, field := range fields {
			if field == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected field %q not in DefaultExcludedFields", expected)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		wantMax  int
		wantClus int
	}{
		{
			name:     "nil values use defaults",
			config:   &Config{},
			wantMax:  DefaultMaxItems,
			wantClus: DefaultMaxClusters,
		},
		{
			name:     "values under max are preserved",
			config:   &Config{MaxItems: 50, MaxClusters: 10},
			wantMax:  50,
			wantClus: 10,
		},
		{
			name:     "values over absolute max are capped",
			config:   &Config{MaxItems: 2000, MaxClusters: 200},
			wantMax:  AbsoluteMaxItems,
			wantClus: AbsoluteMaxClusters,
		},
		{
			name:     "negative values use defaults",
			config:   &Config{MaxItems: -1, MaxClusters: -1},
			wantMax:  DefaultMaxItems,
			wantClus: DefaultMaxClusters,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validated := tt.config.Validate()

			if validated.MaxItems != tt.wantMax {
				t.Errorf("MaxItems = %d, want %d", validated.MaxItems, tt.wantMax)
			}
			if validated.MaxClusters != tt.wantClus {
				t.Errorf("MaxClusters = %d, want %d", validated.MaxClusters, tt.wantClus)
			}
		})
	}
}

func TestConfigValidate_SlimOutputDefaults(t *testing.T) {
	cfg := &Config{
		SlimOutput:     true,
		ExcludedFields: nil, // Empty
	}

	validated := cfg.Validate()

	if len(validated.ExcludedFields) == 0 {
		t.Error("Validate should add default excluded fields when SlimOutput is true")
	}
}

func TestConfigClone(t *testing.T) {
	original := &Config{
		MaxItems:         50,
		MaxClusters:      10,
		MaxResponseBytes: 1024,
		SlimOutput:       true,
		MaskSecrets:      true,
		SummaryThreshold: 100,
		ExcludedFields:   []string{"field1", "field2"},
	}

	clone := original.Clone()

	// Verify values are copied
	if clone.MaxItems != original.MaxItems {
		t.Error("Clone MaxItems mismatch")
	}
	if clone.MaxClusters != original.MaxClusters {
		t.Error("Clone MaxClusters mismatch")
	}
	if len(clone.ExcludedFields) != len(original.ExcludedFields) {
		t.Error("Clone ExcludedFields length mismatch")
	}

	// Verify it's a deep copy - modifying clone shouldn't affect original
	clone.MaxItems = 999
	clone.ExcludedFields[0] = "modified"

	if original.MaxItems == 999 {
		t.Error("Modifying clone affected original MaxItems")
	}
	if original.ExcludedFields[0] == "modified" {
		t.Error("Modifying clone affected original ExcludedFields")
	}
}

func TestConfigClone_Nil(t *testing.T) {
	var cfg *Config
	clone := cfg.Clone()

	if clone != nil {
		t.Error("Clone of nil should return nil")
	}
}

func TestTruncationWarning(t *testing.T) {
	warning := &TruncationWarning{
		Shown:          100,
		Total:          500,
		Message:        "Output truncated",
		SuggestSummary: true,
		SuggestFilters: []string{"filter1", "filter2"},
	}

	if warning.Shown != 100 {
		t.Error("Shown mismatch")
	}
	if warning.Total != 500 {
		t.Error("Total mismatch")
	}
	if !warning.SuggestSummary {
		t.Error("SuggestSummary should be true")
	}
	if len(warning.SuggestFilters) != 2 {
		t.Error("SuggestFilters length mismatch")
	}
}

func TestProcessingMetadata(t *testing.T) {
	meta := ProcessingMetadata{
		OriginalCount: 100,
		FinalCount:    50,
		Truncated:     true,
		SlimApplied:   true,
		SecretsMasked: true,
	}

	if meta.OriginalCount != 100 {
		t.Error("OriginalCount mismatch")
	}
	if meta.FinalCount != 50 {
		t.Error("FinalCount mismatch")
	}
	if !meta.Truncated {
		t.Error("Truncated should be true")
	}
	if !meta.SlimApplied {
		t.Error("SlimApplied should be true")
	}
	if !meta.SecretsMasked {
		t.Error("SecretsMasked should be true")
	}
}
