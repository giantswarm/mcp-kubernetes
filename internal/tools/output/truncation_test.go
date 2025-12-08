package output

import (
	"testing"
)

func TestTruncateResponse(t *testing.T) {
	tests := []struct {
		name        string
		items       []map[string]interface{}
		maxItems    int
		wantLen     int
		wantWarning bool
	}{
		{
			name:        "no truncation needed - empty",
			items:       []map[string]interface{}{},
			maxItems:    100,
			wantLen:     0,
			wantWarning: false,
		},
		{
			name:        "no truncation needed - under limit",
			items:       makeTestMaps(50),
			maxItems:    100,
			wantLen:     50,
			wantWarning: false,
		},
		{
			name:        "no truncation needed - at limit",
			items:       makeTestMaps(100),
			maxItems:    100,
			wantLen:     100,
			wantWarning: false,
		},
		{
			name:        "truncation needed - over limit",
			items:       makeTestMaps(150),
			maxItems:    100,
			wantLen:     100,
			wantWarning: true,
		},
		{
			name:        "uses default when maxItems is 0",
			items:       makeTestMaps(150),
			maxItems:    0,
			wantLen:     DefaultMaxItems,
			wantWarning: true,
		},
		{
			name:        "uses default when maxItems is negative",
			items:       makeTestMaps(150),
			maxItems:    -1,
			wantLen:     DefaultMaxItems,
			wantWarning: true,
		},
		{
			name:        "caps at absolute maximum",
			items:       makeTestMaps(1500),
			maxItems:    2000, // Over AbsoluteMaxItems
			wantLen:     AbsoluteMaxItems,
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, warning := TruncateResponse(tt.items, tt.maxItems)

			if len(result) != tt.wantLen {
				t.Errorf("TruncateResponse() len = %d, want %d", len(result), tt.wantLen)
			}

			if tt.wantWarning && warning == nil {
				t.Error("TruncateResponse() expected warning, got nil")
			}
			if !tt.wantWarning && warning != nil {
				t.Errorf("TruncateResponse() unexpected warning: %v", warning)
			}
		})
	}
}

func TestTruncateResponse_WarningContent(t *testing.T) {
	items := makeTestMaps(200)
	_, warning := TruncateResponse(items, 100)

	if warning == nil {
		t.Fatal("Expected warning for truncated response")
	}

	if warning.Shown != 100 {
		t.Errorf("Warning.Shown = %d, want 100", warning.Shown)
	}
	if warning.Total != 200 {
		t.Errorf("Warning.Total = %d, want 200", warning.Total)
	}
	if warning.Message == "" {
		t.Error("Warning.Message should not be empty")
	}
}

func TestTruncateResponse_SuggestSummary(t *testing.T) {
	// Large result set should suggest summary mode
	items := makeTestMaps(600) // More than DefaultMaxItems * 5
	_, warning := TruncateResponse(items, 100)

	if warning == nil {
		t.Fatal("Expected warning for truncated response")
	}

	if !warning.SuggestSummary {
		t.Error("SuggestSummary should be true for large result sets")
	}
	if len(warning.SuggestFilters) == 0 {
		t.Error("SuggestFilters should have suggestions for large result sets")
	}
}

func TestTruncateGeneric(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	// No truncation needed
	result, warning := TruncateGeneric(items, 10)
	if len(result) != 5 {
		t.Errorf("Expected 5 items, got %d", len(result))
	}
	if warning != nil {
		t.Error("Unexpected warning")
	}

	// Truncation needed
	result, warning = TruncateGeneric(items, 3)
	if len(result) != 3 {
		t.Errorf("Expected 3 items, got %d", len(result))
	}
	if warning == nil {
		t.Fatal("Expected warning") // Use Fatal to stop execution and avoid nil dereference
	}
	if warning.Shown != 3 || warning.Total != 5 {
		t.Errorf("Warning shows %d/%d, want 3/5", warning.Shown, warning.Total)
	}
}

func TestTruncateClusters(t *testing.T) {
	type cluster struct {
		Name string
	}

	clusters := make([]cluster, 30)
	for i := range clusters {
		clusters[i] = cluster{Name: "cluster-" + string(rune('a'+i))}
	}

	// Truncation occurs since we have 30 clusters but limit is 20
	result, warning := TruncateClusters(clusters, DefaultMaxClusters)
	if len(result) != DefaultMaxClusters {
		t.Errorf("Expected %d clusters, got %d", DefaultMaxClusters, len(result))
	}
	if warning == nil {
		t.Fatal("Expected warning when truncating clusters") // Use Fatal to stop execution
	}

	// Verify warning suggests filters
	if len(warning.SuggestFilters) == 0 {
		t.Error("Expected filter suggestions in cluster warning")
	}
}

func TestEffectiveLimit(t *testing.T) {
	tests := []struct {
		name         string
		requestLimit int
		configLimit  int
		want         int
	}{
		{
			name:         "request 0, config 0 -> default",
			requestLimit: 0,
			configLimit:  0,
			want:         DefaultMaxItems,
		},
		{
			name:         "request 0, config set -> config",
			requestLimit: 0,
			configLimit:  50,
			want:         50,
		},
		{
			name:         "request set, config 0 -> request",
			requestLimit: 75,
			configLimit:  0,
			want:         75,
		},
		{
			name:         "both set, request smaller -> request",
			requestLimit: 30,
			configLimit:  50,
			want:         30,
		},
		{
			name:         "both set, config smaller -> config",
			requestLimit: 100,
			configLimit:  50,
			want:         50,
		},
		{
			name:         "request over absolute max -> capped",
			requestLimit: 2000,
			configLimit:  0,
			want:         AbsoluteMaxItems,
		},
		{
			name:         "config over absolute max -> capped",
			requestLimit: 0,
			configLimit:  2000,
			want:         AbsoluteMaxItems,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveLimit(tt.requestLimit, tt.configLimit)
			if got != tt.want {
				t.Errorf("EffectiveLimit(%d, %d) = %d, want %d",
					tt.requestLimit, tt.configLimit, got, tt.want)
			}
		})
	}
}

func TestEffectiveClusterLimit(t *testing.T) {
	tests := []struct {
		name         string
		requestLimit int
		configLimit  int
		want         int
	}{
		{
			name:         "both 0 -> default",
			requestLimit: 0,
			configLimit:  0,
			want:         DefaultMaxClusters,
		},
		{
			name:         "over absolute max -> capped",
			requestLimit: 200,
			configLimit:  0,
			want:         AbsoluteMaxClusters,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveClusterLimit(tt.requestLimit, tt.configLimit)
			if got != tt.want {
				t.Errorf("EffectiveClusterLimit(%d, %d) = %d, want %d",
					tt.requestLimit, tt.configLimit, got, tt.want)
			}
		})
	}
}

// Helper function to create test maps
func makeTestMaps(count int) []map[string]interface{} {
	result := make([]map[string]interface{}, count)
	for i := range result {
		result[i] = map[string]interface{}{
			"name": "item-" + string(rune('a'+i%26)),
			"id":   i,
		}
	}
	return result
}
