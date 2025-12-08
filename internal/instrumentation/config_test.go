package instrumentation

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	// Clear any environment variables
	os.Clearenv()

	config := DefaultConfig()

	if config.ServiceName != "mcp-kubernetes" {
		t.Errorf("expected ServiceName to be 'mcp-kubernetes', got %s", config.ServiceName)
	}

	if config.Enabled {
		t.Error("expected Enabled to be false by default for zero overhead")
	}

	if config.MetricsExporter != "prometheus" {
		t.Errorf("expected MetricsExporter to be 'prometheus', got %s", config.MetricsExporter)
	}

	if config.TracingExporter != "none" {
		t.Errorf("expected TracingExporter to be 'none', got %s", config.TracingExporter)
	}

	if config.TraceSamplingRate != 0.1 {
		t.Errorf("expected TraceSamplingRate to be 0.1, got %f", config.TraceSamplingRate)
	}
}

func TestDefaultConfigWithEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	os.Setenv("INSTRUMENTATION_ENABLED", "false")
	os.Setenv("METRICS_EXPORTER", "stdout")
	os.Setenv("TRACING_EXPORTER", "otlp")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	os.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")
	defer os.Clearenv()

	config := DefaultConfig()

	if config.ServiceName != "test-service" {
		t.Errorf("expected ServiceName to be 'test-service', got %s", config.ServiceName)
	}

	if config.Enabled {
		t.Error("expected Enabled to be false")
	}

	if config.MetricsExporter != "stdout" {
		t.Errorf("expected MetricsExporter to be 'stdout', got %s", config.MetricsExporter)
	}

	if config.TracingExporter != "otlp" {
		t.Errorf("expected TracingExporter to be 'otlp', got %s", config.TracingExporter)
	}

	if config.OTLPEndpoint != "http://localhost:4318" {
		t.Errorf("expected OTLPEndpoint to be 'http://localhost:4318', got %s", config.OTLPEndpoint)
	}

	if config.TraceSamplingRate != 0.5 {
		t.Errorf("expected TraceSamplingRate to be 0.5, got %f", config.TraceSamplingRate)
	}
}

func TestConfigValidate(t *testing.T) {
	// Test valid default config
	config := DefaultConfig()
	if err := config.Validate(); err != nil {
		t.Errorf("expected Validate to return nil for default config, got %v", err)
	}

	// Test invalid sampling rate (too high)
	config.TraceSamplingRate = 1.5
	if err := config.Validate(); err == nil {
		t.Error("expected error for sampling rate > 1.0")
	}

	// Test invalid sampling rate (negative)
	config.TraceSamplingRate = -0.1
	if err := config.Validate(); err == nil {
		t.Error("expected error for negative sampling rate")
	}

	// Reset to valid sampling rate
	config.TraceSamplingRate = 0.5

	// Test invalid metrics exporter
	config.MetricsExporter = "invalid"
	if err := config.Validate(); err == nil {
		t.Error("expected error for invalid metrics exporter")
	}

	// Reset to valid metrics exporter
	config.MetricsExporter = "prometheus"

	// Test invalid tracing exporter
	config.TracingExporter = "invalid"
	if err := config.Validate(); err == nil {
		t.Error("expected error for invalid tracing exporter")
	}

	// Test OTLP tracing without endpoint
	config.TracingExporter = "otlp"
	config.OTLPEndpoint = ""
	if err := config.Validate(); err == nil {
		t.Error("expected error for OTLP tracing without endpoint")
	}

	// Test OTLP tracing with endpoint (valid)
	config.OTLPEndpoint = "http://localhost:4318"
	if err := config.Validate(); err != nil {
		t.Errorf("expected no error for valid OTLP config, got %v", err)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	os.Clearenv()

	// Test with no env var set
	result := getEnvOrDefault("TEST_VAR", "default")
	if result != "default" {
		t.Errorf("expected 'default', got %s", result)
	}

	// Test with env var set
	os.Setenv("TEST_VAR", "custom")
	defer os.Unsetenv("TEST_VAR")

	result = getEnvOrDefault("TEST_VAR", "default")
	if result != "custom" {
		t.Errorf("expected 'custom', got %s", result)
	}
}

func TestGetEnvBoolOrDefault(t *testing.T) {
	os.Clearenv()

	// Test with no env var set
	result := getEnvBoolOrDefault("TEST_BOOL", true)
	if !result {
		t.Error("expected true")
	}

	// Test with valid bool env var
	os.Setenv("TEST_BOOL", "false")
	defer os.Unsetenv("TEST_BOOL")

	result = getEnvBoolOrDefault("TEST_BOOL", true)
	if result {
		t.Error("expected false")
	}

	// Test with invalid bool env var - should return default
	os.Setenv("TEST_BOOL", "invalid")
	result = getEnvBoolOrDefault("TEST_BOOL", true)
	if !result {
		t.Error("expected default true for invalid value")
	}
}

func TestGetEnvFloatOrDefault(t *testing.T) {
	os.Clearenv()

	// Test with no env var set
	result := getEnvFloatOrDefault("TEST_FLOAT", 0.5)
	if result != 0.5 {
		t.Errorf("expected 0.5, got %f", result)
	}

	// Test with valid float env var
	os.Setenv("TEST_FLOAT", "0.8")
	defer os.Unsetenv("TEST_FLOAT")

	result = getEnvFloatOrDefault("TEST_FLOAT", 0.5)
	if result != 0.8 {
		t.Errorf("expected 0.8, got %f", result)
	}

	// Test with invalid float env var - should return default
	os.Setenv("TEST_FLOAT", "invalid")
	result = getEnvFloatOrDefault("TEST_FLOAT", 0.5)
	if result != 0.5 {
		t.Errorf("expected default 0.5 for invalid value, got %f", result)
	}
}
