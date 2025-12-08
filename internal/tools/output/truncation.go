package output

import (
	"fmt"
)

// TruncateResponse truncates a slice of items to the configured maximum.
// Returns the truncated slice and a warning if truncation occurred.
func TruncateResponse(items []map[string]interface{}, maxItems int) ([]map[string]interface{}, *TruncationWarning) {
	if maxItems <= 0 {
		maxItems = DefaultMaxItems
	}

	// Cap at absolute maximum
	if maxItems > AbsoluteMaxItems {
		maxItems = AbsoluteMaxItems
	}

	total := len(items)
	if total <= maxItems {
		return items, nil
	}

	truncated := items[:maxItems]
	warning := &TruncationWarning{
		Shown:   maxItems,
		Total:   total,
		Message: fmt.Sprintf("Output truncated. Showing %d of %d items. Refine your query with namespace, label, or field filters for complete results.", maxItems, total),
	}

	// Suggest summary mode for very large result sets
	if total > DefaultMaxItems*5 {
		warning.SuggestSummary = true
		warning.SuggestFilters = []string{
			"Use labelSelector to filter by labels (e.g., app=nginx)",
			"Use namespace to limit to a specific namespace",
			"Use summary=true to get counts instead of full objects",
		}
	}

	return truncated, warning
}

// TruncateGeneric truncates a generic slice of items.
// This is useful for typed slices that need to be truncated.
func TruncateGeneric[T any](items []T, maxItems int) ([]T, *TruncationWarning) {
	if maxItems <= 0 {
		maxItems = DefaultMaxItems
	}

	// Cap at absolute maximum
	if maxItems > AbsoluteMaxItems {
		maxItems = AbsoluteMaxItems
	}

	total := len(items)
	if total <= maxItems {
		return items, nil
	}

	truncated := items[:maxItems]
	warning := &TruncationWarning{
		Shown:   maxItems,
		Total:   total,
		Message: fmt.Sprintf("Output truncated. Showing %d of %d items. Refine your query with filters for complete results.", maxItems, total),
	}

	// Suggest summary mode for very large result sets
	if total > DefaultMaxItems*5 {
		warning.SuggestSummary = true
		warning.SuggestFilters = []string{
			"Use more specific filters to narrow results",
			"Use summary=true for counts instead of full objects",
		}
	}

	return truncated, warning
}

// TruncateClusters truncates a slice of clusters for fleet-wide operations.
// Returns the truncated slice and a warning if truncation occurred.
func TruncateClusters[T any](clusters []T, maxClusters int) ([]T, *TruncationWarning) {
	if maxClusters <= 0 {
		maxClusters = DefaultMaxClusters
	}

	// Cap at absolute maximum
	if maxClusters > AbsoluteMaxClusters {
		maxClusters = AbsoluteMaxClusters
	}

	total := len(clusters)
	if total <= maxClusters {
		return clusters, nil
	}

	truncated := clusters[:maxClusters]
	warning := &TruncationWarning{
		Shown:   maxClusters,
		Total:   total,
		Message: fmt.Sprintf("Cluster results truncated. Showing %d of %d clusters. Use organization or provider filters to narrow results.", maxClusters, total),
		SuggestFilters: []string{
			"Use organization to filter by namespace/org",
			"Use provider to filter by infrastructure provider",
			"Use status to filter by cluster phase",
		},
	}

	return truncated, warning
}

// EffectiveLimit calculates the effective limit considering request and config limits.
// It applies absolute bounds to prevent DoS attacks.
func EffectiveLimit(requestLimit, configLimit int) int {
	// If no request limit specified, use config limit
	if requestLimit <= 0 {
		if configLimit <= 0 {
			return DefaultMaxItems
		}
		return min(configLimit, AbsoluteMaxItems)
	}

	// Take the minimum of request and config limits
	effective := requestLimit
	if configLimit > 0 && configLimit < effective {
		effective = configLimit
	}

	// Apply absolute maximum
	return min(effective, AbsoluteMaxItems)
}

// EffectiveClusterLimit calculates the effective cluster limit.
func EffectiveClusterLimit(requestLimit, configLimit int) int {
	// If no request limit specified, use config limit
	if requestLimit <= 0 {
		if configLimit <= 0 {
			return DefaultMaxClusters
		}
		return min(configLimit, AbsoluteMaxClusters)
	}

	// Take the minimum of request and config limits
	effective := requestLimit
	if configLimit > 0 && configLimit < effective {
		effective = configLimit
	}

	// Apply absolute maximum
	return min(effective, AbsoluteMaxClusters)
}
