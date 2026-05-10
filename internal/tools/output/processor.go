package output

import (
	"time"
)

// Processor applies output transformations based on configuration.
type Processor struct {
	config *Config
}

// NewProcessor creates a new output processor with the given configuration.
func NewProcessor(config *Config) *Processor {
	if config == nil {
		config = DefaultConfig()
	}
	return &Processor{
		config: config.Validate(),
	}
}

// Process applies all configured transformations to a list of resources.
// It returns the processed items and a result containing metadata and warnings.
func (p *Processor) Process(items []map[string]interface{}) *ProcessingResult {
	return p.processInternal(items, p.config.MaxItems)
}

// ProcessWithLimit applies transformations with a custom limit.
// Useful when the limit is specified per-request.
func (p *Processor) ProcessWithLimit(items []map[string]interface{}, limit int) *ProcessingResult {
	return p.processInternal(items, EffectiveLimit(limit, p.config.MaxItems))
}

// processInternal contains the shared processing logic for Process and ProcessWithLimit.
func (p *Processor) processInternal(items []map[string]interface{}, limit int) *ProcessingResult {
	start := time.Now()
	originalCount := len(items)

	result := &ProcessingResult{
		Items:    items,
		Warnings: make([]TruncationWarning, 0),
		Metadata: ProcessingMetadata{
			ProcessedAt:   start,
			OriginalCount: originalCount,
		},
	}

	if len(items) == 0 {
		result.Metadata.FinalCount = 0
		return result
	}

	processed := items

	// Apply secret masking first (security critical)
	if p.config.MaskSecrets {
		processed = MaskSecretsInList(processed)
		result.Metadata.SecretsMasked = true
	}

	// Apply slim output (remove verbose fields)
	if p.config.SlimOutput {
		processed = SlimResources(processed, p.config.ExcludedFields)
		result.Metadata.SlimApplied = true
	}

	// Apply Kind-aware shaping after generic slim. Shapers rely on
	// bookkeeping fields already being gone and only run when SlimOutput
	// is on (KindShaping by itself with SlimOutput=false would be a "wide"
	// output with surprise per-Kind drops, which the contract forbids).
	if p.config.SlimOutput && p.config.KindShaping {
		processed = ShapeResources(processed)
	}

	// Apply truncation last (so warnings reflect final count)
	truncated, warning := TruncateResponse(processed, limit)
	if warning != nil {
		result.Warnings = append(result.Warnings, *warning)
		result.Metadata.Truncated = true
	}

	result.Items = truncated
	result.Metadata.FinalCount = len(truncated)

	return result
}

// GenerateSummary creates a summary for large result sets.
func (p *Processor) GenerateSummary(items []map[string]interface{}, opts *SummaryOptions) *ResourceSummary {
	return GenerateSummary(items, opts)
}

// ShouldSuggestSummary returns true if the item count exceeds the summary threshold.
func (p *Processor) ShouldSuggestSummary(itemCount int) bool {
	return itemCount > p.config.SummaryThreshold
}

// Config returns the processor's configuration.
func (p *Processor) Config() *Config {
	return p.config
}

// ProcessSingle applies transformations to a single resource.
func (p *Processor) ProcessSingle(item map[string]interface{}) map[string]interface{} {
	if item == nil {
		return nil
	}

	processed := item

	// Apply secret masking first (security critical)
	if p.config.MaskSecrets {
		processed = MaskSecrets(processed)
	}

	// Apply slim output (remove verbose fields)
	if p.config.SlimOutput {
		processed = SlimResource(processed, p.config.ExcludedFields)
	}

	// Apply Kind-aware shaping (HelmRelease drops spec.values /
	// status.history; workload templates collapse long env lists). Only
	// runs alongside SlimOutput so callers asking for the full manifest
	// (output: wide) still get every field.
	if p.config.SlimOutput && p.config.KindShaping {
		processed = ShapeResource(processed)
	}

	return processed
}

// ProcessingStats contains statistics about output processing.
type ProcessingStats struct {
	// ItemsProcessed is the number of items processed
	ItemsProcessed int `json:"itemsProcessed"`

	// ItemsTruncated is the number of items removed by truncation
	ItemsTruncated int `json:"itemsTruncated"`

	// SecretsRedacted is the number of secrets that had data redacted
	SecretsRedacted int `json:"secretsRedacted"`

	// FieldsRemoved is the estimated count of fields removed by slim mode
	FieldsRemoved int `json:"fieldsRemoved"`

	// BytesSaved is the estimated bytes saved by processing
	BytesSaved int64 `json:"bytesSaved"`

	// ProcessingTimeMs is the processing duration in milliseconds
	ProcessingTimeMs int64 `json:"processingTimeMs"`
}

// ProcessWithStats applies transformations and returns detailed statistics.
// This is useful for observability and debugging.
func (p *Processor) ProcessWithStats(items []map[string]interface{}) (*ProcessingResult, *ProcessingStats) {
	start := time.Now()
	originalCount := len(items)

	stats := &ProcessingStats{
		ItemsProcessed: originalCount,
	}

	result := p.Process(items)

	// Calculate statistics
	stats.ItemsTruncated = originalCount - result.Metadata.FinalCount

	// Count secrets
	for _, item := range items {
		if IsSecretResource(item) {
			stats.SecretsRedacted++
		}
	}

	// Estimate fields removed (rough calculation)
	if p.config.SlimOutput {
		stats.FieldsRemoved = originalCount * len(p.config.ExcludedFields)
	}

	stats.ProcessingTimeMs = time.Since(start).Milliseconds()

	return result, stats
}

// QuickProcess is a convenience function for processing with default config.
func QuickProcess(items []map[string]interface{}) *ProcessingResult {
	return NewProcessor(nil).Process(items)
}

// QuickProcessSingle is a convenience function for processing a single item.
func QuickProcessSingle(item map[string]interface{}) map[string]interface{} {
	return NewProcessor(nil).ProcessSingle(item)
}
