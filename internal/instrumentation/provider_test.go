package instrumentation

import (
	"context"
	"testing"
	"time"
)

func TestNewProvider_Disabled(t *testing.T) {
	config := Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Enabled:        false,
	}

	provider, err := NewProvider(context.Background(), config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if provider == nil {
		t.Fatal("expected provider to be non-nil")
	}

	if provider.Enabled() {
		t.Error("expected provider to be disabled")
	}

	if provider.Metrics() == nil {
		t.Error("expected metrics to be non-nil even when disabled")
	}

	// Shutdown should not error for disabled provider
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Errorf("expected no error on shutdown, got %v", err)
	}
}

func TestNewProvider_PrometheusExporter(t *testing.T) {
	config := Config{
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "none",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() { _ = provider.Shutdown(ctx) }()

	if !provider.Enabled() {
		t.Error("expected provider to be enabled")
	}

	if provider.Metrics() == nil {
		t.Error("expected metrics to be non-nil")
	}

	if provider.PrometheusHandler() == nil {
		t.Error("expected PrometheusHandler to be non-nil for prometheus exporter")
	}

	// Test tracer
	tracer := provider.Tracer("test")
	if tracer == nil {
		t.Error("expected tracer to be non-nil")
	}
}

func TestNewProvider_StdoutExporter(t *testing.T) {
	config := Config{
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "stdout",
		TracingExporter: "stdout",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() { _ = provider.Shutdown(ctx) }()

	if !provider.Enabled() {
		t.Error("expected provider to be enabled")
	}

	if provider.PrometheusHandler() != nil {
		t.Error("expected PrometheusHandler to be nil for stdout exporter")
	}
}

func TestNewProvider_InvalidMetricsExporter(t *testing.T) {
	config := Config{
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "invalid",
		TracingExporter: "none",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewProvider(ctx, config)
	if err == nil {
		t.Error("expected error for invalid metrics exporter")
	}
}

func TestNewProvider_InvalidTracingExporter(t *testing.T) {
	config := Config{
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "invalid",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewProvider(ctx, config)
	if err == nil {
		t.Error("expected error for invalid tracing exporter")
	}
}

func TestNewProvider_OTLPTracingWithoutEndpoint(t *testing.T) {
	config := Config{
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "otlp",
		OTLPEndpoint:    "", // Missing endpoint
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewProvider(ctx, config)
	if err == nil {
		t.Error("expected error for OTLP tracing without endpoint")
	}
}

func TestProvider_Shutdown(t *testing.T) {
	config := Config{
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "none",
	}

	ctx := context.Background()
	provider, err := NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Shutdown should not error
	if err := provider.Shutdown(ctx); err != nil {
		t.Errorf("expected no error on shutdown, got %v", err)
	}
}

func TestProvider_Tracer_Disabled(t *testing.T) {
	config := Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Enabled:        false,
	}

	provider, err := NewProvider(context.Background(), config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	tracer := provider.Tracer("test")
	if tracer == nil {
		t.Error("expected tracer to be non-nil (no-op)")
	}
}
