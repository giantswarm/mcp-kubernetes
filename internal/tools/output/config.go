package output

import "time"

// Default limits for output processing.
// These are tuned for typical LLM context windows and API response sizes.
const (
	// DefaultMaxItems is the default maximum number of resources returned per query.
	DefaultMaxItems = 100

	// DefaultMaxClusters is the default maximum number of clusters in fleet-wide queries.
	DefaultMaxClusters = 20

	// DefaultMaxResponseBytes is the default hard limit on response size (512KB).
	DefaultMaxResponseBytes = 512 * 1024

	// AbsoluteMaxItems is the absolute maximum items that can be requested.
	// This prevents DoS via unbounded result sets even when users request higher limits.
	AbsoluteMaxItems = 1000

	// AbsoluteMaxClusters is the absolute maximum clusters for fleet operations.
	AbsoluteMaxClusters = 100

	// AbsoluteMaxResponseBytes is the absolute maximum response size (2MB).
	AbsoluteMaxResponseBytes = 2 * 1024 * 1024
)

// Config holds configuration for output processing.
// All limits have sensible defaults that can be overridden via Helm values.
type Config struct {
	// MaxItems limits the number of resources returned per query.
	// Default: 100, Absolute max: 1000
	MaxItems int `json:"maxItems" yaml:"maxItems"`

	// MaxClusters limits clusters in fleet-wide queries.
	// Default: 20, Absolute max: 100
	MaxClusters int `json:"maxClusters" yaml:"maxClusters"`

	// MaxResponseBytes is a hard limit on response size in bytes.
	// Default: 512KB, Absolute max: 2MB
	MaxResponseBytes int `json:"maxResponseBytes" yaml:"maxResponseBytes"`

	// SlimOutput enables removal of verbose fields that rarely help AI agents.
	// Default: true
	SlimOutput bool `json:"slimOutput" yaml:"slimOutput"`

	// MaskSecrets replaces secret data with "***REDACTED***".
	// Default: true (security critical - should rarely be disabled)
	MaskSecrets bool `json:"maskSecrets" yaml:"maskSecrets"`

	// SummaryThreshold is the item count above which summary mode is suggested.
	// Default: 500
	SummaryThreshold int `json:"summaryThreshold" yaml:"summaryThreshold"`

	// ExcludedFields lists JSON paths of fields to exclude in slim mode.
	// Default: common verbose fields (managedFields, last-applied-configuration, etc.)
	ExcludedFields []string `json:"excludedFields,omitempty" yaml:"excludedFields,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults for fleet-scale operations.
func DefaultConfig() *Config {
	return &Config{
		MaxItems:         DefaultMaxItems,
		MaxClusters:      DefaultMaxClusters,
		MaxResponseBytes: DefaultMaxResponseBytes,
		SlimOutput:       true,
		MaskSecrets:      true,
		SummaryThreshold: 500,
		ExcludedFields:   DefaultExcludedFields(),
	}
}

// DefaultExcludedFields returns the default list of fields to exclude in slim mode.
// These are verbose fields that typically don't help AI agents understand resources.
func DefaultExcludedFields() []string {
	return []string{
		// Managed fields are verbose and rarely useful for troubleshooting
		"metadata.managedFields",
		// Last-applied-configuration duplicates the entire manifest
		"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration",
		// Revision annotations are internal bookkeeping
		"metadata.annotations.deployment.kubernetes.io/revision",
		// Transition times add noise without helping diagnosis
		"status.conditions[*].lastTransitionTime",
		"status.conditions[*].lastProbeTime",
		"status.conditions[*].lastHeartbeatTime",
		// Owner references can be looked up if needed
		"metadata.ownerReferences",
		// Finalizers are rarely relevant for troubleshooting
		"metadata.finalizers",
		// Generation is usually not needed
		"metadata.generation",
		// Resource version changes constantly
		"metadata.resourceVersion",
		// UID is rarely needed for troubleshooting
		"metadata.uid",
		// Self link is deprecated
		"metadata.selfLink",
	}
}

// Validate validates the configuration and applies absolute limits.
// It returns a validated copy with any out-of-range values capped.
func (c *Config) Validate() *Config {
	validated := *c

	// Apply minimum bounds
	if validated.MaxItems <= 0 {
		validated.MaxItems = DefaultMaxItems
	}
	if validated.MaxClusters <= 0 {
		validated.MaxClusters = DefaultMaxClusters
	}
	if validated.MaxResponseBytes <= 0 {
		validated.MaxResponseBytes = DefaultMaxResponseBytes
	}

	// Apply absolute maximum bounds (security critical)
	if validated.MaxItems > AbsoluteMaxItems {
		validated.MaxItems = AbsoluteMaxItems
	}
	if validated.MaxClusters > AbsoluteMaxClusters {
		validated.MaxClusters = AbsoluteMaxClusters
	}
	if validated.MaxResponseBytes > AbsoluteMaxResponseBytes {
		validated.MaxResponseBytes = AbsoluteMaxResponseBytes
	}

	// Ensure excluded fields has a default if empty and slim mode is enabled
	if validated.SlimOutput && len(validated.ExcludedFields) == 0 {
		validated.ExcludedFields = DefaultExcludedFields()
	}

	return &validated
}

// Clone creates a deep copy of the configuration.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}

	clone := *c

	// Deep copy slices
	if c.ExcludedFields != nil {
		clone.ExcludedFields = make([]string, len(c.ExcludedFields))
		copy(clone.ExcludedFields, c.ExcludedFields)
	}

	return &clone
}

// TruncationWarning contains information about response truncation.
type TruncationWarning struct {
	// Shown is the number of items returned
	Shown int `json:"shown"`

	// Total is the total number of items before truncation
	Total int `json:"total"`

	// Message is a human-readable warning message
	Message string `json:"message"`

	// SuggestSummary indicates if summary mode is recommended
	SuggestSummary bool `json:"suggestSummary,omitempty"`

	// SuggestFilters suggests filter options to reduce results
	SuggestFilters []string `json:"suggestFilters,omitempty"`
}

// ProcessingResult contains the result of output processing.
type ProcessingResult struct {
	// Items contains the processed items
	Items []map[string]interface{} `json:"items"`

	// Warnings contains any warnings generated during processing
	Warnings []TruncationWarning `json:"warnings,omitempty"`

	// Metadata contains additional processing metadata
	Metadata ProcessingMetadata `json:"metadata"`
}

// ProcessingMetadata contains metadata about the processing operation.
type ProcessingMetadata struct {
	// ProcessedAt is when processing occurred
	ProcessedAt time.Time `json:"processedAt"`

	// OriginalCount is the item count before processing
	OriginalCount int `json:"originalCount"`

	// FinalCount is the item count after processing
	FinalCount int `json:"finalCount"`

	// Truncated indicates if truncation occurred
	Truncated bool `json:"truncated"`

	// SlimApplied indicates if slim output was applied
	SlimApplied bool `json:"slimApplied"`

	// SecretsMasked indicates if secrets were masked
	SecretsMasked bool `json:"secretsMasked"`

	// BytesReduced is the estimated bytes saved by processing
	BytesReduced int64 `json:"bytesReduced,omitempty"`
}
