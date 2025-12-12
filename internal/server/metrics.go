package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
)

const (
	// DefaultMetricsAddr is the default address for the metrics server.
	DefaultMetricsAddr = ":9090"

	// DefaultMetricsReadTimeout is the default read timeout for the metrics server.
	DefaultMetricsReadTimeout = 10 * time.Second

	// DefaultMetricsWriteTimeout is the default write timeout for the metrics server.
	DefaultMetricsWriteTimeout = 10 * time.Second

	// DefaultMetricsIdleTimeout is the default idle timeout for the metrics server.
	DefaultMetricsIdleTimeout = 60 * time.Second
)

// MetricsServerConfig holds configuration for the metrics server.
type MetricsServerConfig struct {
	// Addr is the address to bind the metrics server to (e.g., ":9090").
	Addr string

	// Enabled determines whether the metrics server should be started.
	Enabled bool

	// InstrumentationProvider provides the Prometheus metrics handler.
	InstrumentationProvider *instrumentation.Provider
}

// MetricsServer serves Prometheus metrics on a dedicated port.
// This isolates metrics from the main application traffic for security,
// preventing unauthorized access to operational metrics.
type MetricsServer struct {
	httpServer              *http.Server
	instrumentationProvider *instrumentation.Provider
	addr                    string
}

// NewMetricsServer creates a new metrics server with the given configuration.
// The server exposes a /metrics endpoint for Prometheus scraping.
func NewMetricsServer(config MetricsServerConfig) (*MetricsServer, error) {
	if config.Addr == "" {
		config.Addr = DefaultMetricsAddr
	}

	if config.InstrumentationProvider == nil {
		return nil, fmt.Errorf("instrumentation provider is required for metrics server")
	}

	if !config.InstrumentationProvider.Enabled() {
		return nil, fmt.Errorf("instrumentation provider is not enabled")
	}

	return &MetricsServer{
		instrumentationProvider: config.InstrumentationProvider,
		addr:                    config.Addr,
	}, nil
}

// Start starts the metrics server in a blocking manner.
// Call this in a goroutine if you need non-blocking operation.
func (s *MetricsServer) Start() error {
	mux := http.NewServeMux()

	// Register /metrics endpoint using promhttp.Handler()
	// The OpenTelemetry prometheus exporter registers metrics to the global
	// Prometheus registry, which promhttp.Handler() exposes.
	mux.Handle("/metrics", promhttp.Handler())

	// Add a basic health check for the metrics server itself
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	s.httpServer = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: DefaultMetricsReadTimeout,
		WriteTimeout:      DefaultMetricsWriteTimeout,
		IdleTimeout:       DefaultMetricsIdleTimeout,
	}

	slog.Info("starting metrics server", "addr", s.addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the metrics server.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		slog.Info("shutting down metrics server")
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Addr returns the configured address for the metrics server.
func (s *MetricsServer) Addr() string {
	return s.addr
}
