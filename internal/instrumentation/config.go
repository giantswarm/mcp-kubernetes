package instrumentation

import (
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for OpenTelemetry instrumentation.
type Config struct {
	// ServiceName is the name of the service (default: mcp-kubernetes)
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Enabled determines if instrumentation is active (default: false for zero overhead)
	// Set to true via INSTRUMENTATION_ENABLED=true to enable metrics and tracing
	Enabled bool

	// MetricsExporter specifies the metrics exporter type
	// Options: "prometheus", "otlp", "stdout" (default: "prometheus")
	MetricsExporter string

	// TracingExporter specifies the tracing exporter type
	// Options: "otlp", "stdout", "none" (default: "none")
	TracingExporter string

	// OTLPEndpoint is the OTLP collector endpoint
	// Example: "http://localhost:4318"
	OTLPEndpoint string

	// OTLPInsecure controls whether to use insecure HTTP for OTLP export
	// When false (default), uses TLS for secure transport
	// Set to true only for local development or testing with unencrypted endpoints
	// WARNING: Never use insecure transport in production - traces may contain
	// sensitive metadata and should be encrypted in transit
	OTLPInsecure bool

	// TraceSamplingRate is the sampling rate for traces (0.0 to 1.0, default: 0.1)
	TraceSamplingRate float64

	// PrometheusEndpoint is the path for the Prometheus metrics endpoint (default: "/metrics")
	PrometheusEndpoint string
}

// DefaultConfig returns a Config with sensible defaults based on environment variables.
func DefaultConfig() Config {
	config := Config{
		ServiceName:        getEnvOrDefault("OTEL_SERVICE_NAME", "mcp-kubernetes"),
		ServiceVersion:     "unknown",
		Enabled:            getEnvBoolOrDefault("INSTRUMENTATION_ENABLED", false),
		MetricsExporter:    getEnvOrDefault("METRICS_EXPORTER", "prometheus"),
		TracingExporter:    getEnvOrDefault("TRACING_EXPORTER", "none"),
		OTLPEndpoint:       getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTLPInsecure:       getEnvBoolOrDefault("OTEL_EXPORTER_OTLP_INSECURE", false),
		TraceSamplingRate:  getEnvFloatOrDefault("OTEL_TRACES_SAMPLER_ARG", 0.1),
		PrometheusEndpoint: getEnvOrDefault("PROMETHEUS_ENDPOINT", "/metrics"),
	}

	return config
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	// Validation is lenient - we can work with most configurations
	// The provider will handle invalid configurations gracefully
	return nil
}

// getEnvOrDefault returns the value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBoolOrDefault returns the boolean value of an environment variable or a default value.
func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return parsed
	}
	return defaultValue
}

// getEnvFloatOrDefault returns the float64 value of an environment variable or a default value.
func getEnvFloatOrDefault(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return defaultValue
		}
		return parsed
	}
	return defaultValue
}

// MetricLabels contains common labels for metrics.
type MetricLabels struct {
	// HTTP labels
	Method     string
	Path       string
	StatusCode string

	// Kubernetes labels
	Operation    string
	ResourceType string
	Namespace    string
	Status       string

	// OAuth labels
	Result string

	// Port-forward labels
	PodName string
	PodPort string
}

// Constants for metric label values.
const (
	// Status values
	StatusSuccess = "success"
	StatusError   = "error"
	StatusUnknown = "unknown"

	// OAuth result values
	OAuthResultSuccess  = "success"
	OAuthResultFallback = "fallback"
	OAuthResultFailure  = "failure"

	// Operation types
	OperationGet    = "get"
	OperationList   = "list"
	OperationCreate = "create"
	OperationApply  = "apply"
	OperationDelete = "delete"
	OperationPatch  = "patch"
	OperationLogs   = "logs"
	OperationExec   = "exec"
	OperationWatch  = "watch"

	// Metric recording intervals
	DefaultMetricInterval = 10 * time.Second
)
