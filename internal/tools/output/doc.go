// Package output provides fleet-scale output filtering and truncation for MCP tool responses.
//
// When operating across a fleet of clusters, queries can return massive amounts of data
// that overwhelm LLM context windows (typically 128K-200K tokens). This package implements
// intelligent output filtering, truncation, and summarization to maintain usable response sizes.
//
// # Key Features
//
// Response Truncation: Limits the number of resources returned per query with clear warning
// messages when truncation occurs. Configurable per-query limits with absolute maximums.
//
// Field Exclusion (Slim Output): Automatically removes verbose fields that rarely help AI agents,
// such as managedFields, last-applied-configuration annotations, and timestamps.
//
// Secret Masking: Never returns secret data in responses - all secret values are replaced
// with "***REDACTED***" to prevent accidental exposure.
//
// Summary Mode: For large queries, offers summary counts by status and cluster instead of
// raw data, dramatically reducing response size while maintaining usefulness.
//
// # Configuration
//
// Output behavior is controlled via [Config]:
//
//	cfg := output.DefaultConfig()
//	cfg.MaxItems = 50     // Limit items per response
//	cfg.SlimOutput = true // Enable field exclusion
//	cfg.MaskSecrets = true // Redact secret data
//
// # Security Considerations
//
// This package implements several security controls:
//   - Secret masking prevents credential leakage
//   - Response size limits prevent DoS via context exhaustion
//   - Configurable limits allow operators to tune for their environment
//
// # Usage Example
//
//	// Apply all transformations to a response
//	processor := output.NewProcessor(cfg)
//	result, warnings := processor.Process(items)
//
//	// Or use individual functions
//	slimmed := output.SlimResource(obj)
//	masked := output.MaskSecrets(obj)
//	truncated, warning := output.TruncateResponse(items, 100)
package output
