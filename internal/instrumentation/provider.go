package instrumentation

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Provider encapsulates OpenTelemetry meter and tracer providers.
type Provider struct {
	config             Config
	meterProvider      *metric.MeterProvider
	tracerProvider     *sdktrace.TracerProvider
	metrics            *Metrics
	prometheusExporter *prometheus.Exporter
	enabled            bool
}

// NewProvider creates a new OpenTelemetry provider with the given configuration.
func NewProvider(ctx context.Context, config Config) (*Provider, error) {
	if !config.Enabled {
		return &Provider{
			config:  config,
			enabled: false,
			metrics: &Metrics{}, // Return a no-op metrics recorder
		}, nil
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	provider := &Provider{
		config:  config,
		enabled: true,
	}

	// Initialize meter provider for metrics
	if err := provider.initMeterProvider(ctx, res); err != nil {
		return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
	}

	// Initialize tracer provider for tracing
	if err := provider.initTracerProvider(ctx, res); err != nil {
		// Clean up meter provider on error
		if shutdownErr := provider.meterProvider.Shutdown(ctx); shutdownErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to shutdown meter provider during cleanup: %w", shutdownErr))
		}
		return nil, fmt.Errorf("failed to initialize tracer provider: %w", err)
	}

	// Set global providers
	otel.SetMeterProvider(provider.meterProvider)
	otel.SetTracerProvider(provider.tracerProvider)

	// Create metrics recorder
	meter := provider.meterProvider.Meter(config.ServiceName)
	provider.metrics, err = NewMetrics(meter)
	if err != nil {
		// Clean up on error
		_ = provider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create metrics recorder: %w", err)
	}

	return provider, nil
}

// initMeterProvider initializes the OpenTelemetry meter provider based on configuration.
func (p *Provider) initMeterProvider(ctx context.Context, res *resource.Resource) error {
	var reader metric.Reader

	switch p.config.MetricsExporter {
	case "prometheus":
		// Create Prometheus exporter - it's a reader, not an exporter
		promExporter, err := prometheus.New()
		if err != nil {
			return fmt.Errorf("failed to create prometheus exporter: %w", err)
		}
		// Store prometheus exporter for HTTP handler
		p.prometheusExporter = promExporter
		reader = promExporter

	case "otlp":
		// OTLP exporter for metrics is not directly available in this version
		// Fall back to stdout for now or use prometheus
		exporter, err := stdoutmetric.New()
		if err != nil {
			return fmt.Errorf("failed to create stdout metrics exporter: %w", err)
		}
		reader = metric.NewPeriodicReader(exporter)

	case "stdout":
		exporter, err := stdoutmetric.New()
		if err != nil {
			return fmt.Errorf("failed to create stdout metrics exporter: %w", err)
		}
		reader = metric.NewPeriodicReader(exporter)

	default:
		return fmt.Errorf("unsupported metrics exporter: %s", p.config.MetricsExporter)
	}

	// Create meter provider
	p.meterProvider = metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(reader),
	)

	return nil
}

// initTracerProvider initializes the OpenTelemetry tracer provider based on configuration.
func (p *Provider) initTracerProvider(ctx context.Context, res *resource.Resource) error {
	if p.config.TracingExporter == "none" {
		// No-op tracer provider
		p.tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.NeverSample()),
		)
		return nil
	}

	var exporter sdktrace.SpanExporter
	var err error

	switch p.config.TracingExporter {
	case "otlp":
		if p.config.OTLPEndpoint == "" {
			return fmt.Errorf("OTLP endpoint is required for OTLP tracing exporter")
		}
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(p.config.OTLPEndpoint),
			otlptracehttp.WithInsecure(), // TODO: Make this configurable
		)
		if err != nil {
			return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
		}

	case "stdout":
		exporter, err = stdouttrace.New()
		if err != nil {
			return fmt.Errorf("failed to create stdout trace exporter: %w", err)
		}

	default:
		return fmt.Errorf("unsupported tracing exporter: %s", p.config.TracingExporter)
	}

	// Create tracer provider with sampling
	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(p.config.TraceSamplingRate),
	)

	p.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
	)

	return nil
}

// Metrics returns the metrics recorder for recording observability metrics.
func (p *Provider) Metrics() *Metrics {
	return p.metrics
}

// Tracer returns a tracer for creating spans.
func (p *Provider) Tracer(name string) trace.Tracer {
	if !p.enabled || p.tracerProvider == nil {
		return trace.NewNoopTracerProvider().Tracer(name)
	}
	return p.tracerProvider.Tracer(name)
}

// PrometheusHandler returns an HTTP handler for the Prometheus metrics endpoint.
// Returns nil if Prometheus exporter is not configured.
func (p *Provider) PrometheusHandler() interface{} {
	if p.prometheusExporter == nil {
		return nil
	}
	return p.prometheusExporter
}

// Shutdown gracefully shuts down the provider, flushing any pending telemetry.
func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.enabled {
		return nil
	}

	var errs []error

	// Shutdown meter provider
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	// Shutdown tracer provider
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// Enabled returns true if instrumentation is enabled.
func (p *Provider) Enabled() bool {
	return p.enabled
}
