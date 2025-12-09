package instrumentation

import (
	"fmt"
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

	// ServiceInstanceID is the unique instance identifier (default: hostname)
	// In Kubernetes, this is typically the pod name
	ServiceInstanceID string

	// K8sNamespace is the Kubernetes namespace where the service is running
	K8sNamespace string

	// K8sPodName is the Kubernetes pod name
	K8sPodName string

	// Enabled determines if instrumentation is active (default: true)
	// Set to false via INSTRUMENTATION_ENABLED=false to disable metrics and tracing
	Enabled bool

	// MetricsExporter specifies the metrics exporter type
	// Options: "prometheus", "otlp", "stdout" (default: "prometheus")
	MetricsExporter string

	// TracingExporter specifies the tracing exporter type
	// Options: "otlp", "stdout", "none" (default: "none")
	TracingExporter string

	// OTLPEndpoint is the OTLP collector endpoint
	// Example: "localhost:4318" (without protocol prefix)
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

	// DetailedLabels controls whether high-cardinality labels (namespace, resource_type)
	// are included in Kubernetes operation metrics.
	// When false (default), only operation and status labels are included.
	// When true, namespace and resource_type labels are added (can cause cardinality issues in large clusters).
	// For large clusters with >1000 namespaces, consider keeping this false and using traces for detailed debugging.
	DetailedLabels bool
}

// DefaultConfig returns a Config with sensible defaults based on environment variables.
func DefaultConfig() Config {
	config := Config{
		ServiceName:        getEnvOrDefault("OTEL_SERVICE_NAME", "mcp-kubernetes"),
		ServiceVersion:     "unknown",
		ServiceInstanceID:  getEnvOrDefault("OTEL_SERVICE_INSTANCE_ID", ""),
		K8sNamespace:       getEnvOrDefault("K8S_NAMESPACE", getEnvOrDefault("POD_NAMESPACE", "")),
		K8sPodName:         getEnvOrDefault("K8S_POD_NAME", getEnvOrDefault("HOSTNAME", "")),
		Enabled:            getEnvBoolOrDefault("INSTRUMENTATION_ENABLED", true),
		MetricsExporter:    getEnvOrDefault("METRICS_EXPORTER", ExporterPrometheus),
		TracingExporter:    getEnvOrDefault("TRACING_EXPORTER", ExporterNone),
		OTLPEndpoint:       getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTLPInsecure:       getEnvBoolOrDefault("OTEL_EXPORTER_OTLP_INSECURE", false),
		TraceSamplingRate:  getEnvFloatOrDefault("OTEL_TRACES_SAMPLER_ARG", 0.1),
		PrometheusEndpoint: getEnvOrDefault("PROMETHEUS_ENDPOINT", "/metrics"),
		DetailedLabels:     getEnvBoolOrDefault("METRICS_DETAILED_LABELS", false),
	}

	return config
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	// Validate sampling rate is within bounds
	if c.TraceSamplingRate < 0 || c.TraceSamplingRate > 1 {
		return fmt.Errorf("trace sampling rate must be between 0.0 and 1.0, got %f", c.TraceSamplingRate)
	}

	// Validate metrics exporter
	validMetricsExporters := map[string]bool{ExporterPrometheus: true, ExporterOTLP: true, ExporterStdout: true}
	if c.MetricsExporter != "" && !validMetricsExporters[c.MetricsExporter] {
		return fmt.Errorf("invalid metrics exporter %q, must be one of: prometheus, otlp, stdout", c.MetricsExporter)
	}

	// Validate tracing exporter
	validTracingExporters := map[string]bool{ExporterOTLP: true, ExporterStdout: true, ExporterNone: true}
	if c.TracingExporter != "" && !validTracingExporters[c.TracingExporter] {
		return fmt.Errorf("invalid tracing exporter %q, must be one of: otlp, stdout, none", c.TracingExporter)
	}

	// OTLP endpoint required when using OTLP exporters
	if c.TracingExporter == ExporterOTLP && c.OTLPEndpoint == "" {
		return fmt.Errorf("OTLP endpoint is required when using OTLP tracing exporter")
	}
	if c.MetricsExporter == ExporterOTLP && c.OTLPEndpoint == "" {
		return fmt.Errorf("OTLP endpoint is required when using OTLP metrics exporter")
	}

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
	OperationScale  = "scale"
	OperationLogs   = "logs"
	OperationExec   = "exec"
	OperationWatch  = "watch"

	// Exporter types
	ExporterPrometheus = "prometheus"
	ExporterOTLP       = "otlp"
	ExporterStdout     = "stdout"
	ExporterNone       = "none"

	// Metric recording intervals
	DefaultMetricInterval = 10 * time.Second
)
