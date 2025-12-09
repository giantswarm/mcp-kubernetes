package server

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// Health status constants for health check responses.
const (
	healthStatusOK           = "ok"
	healthStatusNotReady     = "not ready"
	healthStatusShuttingDown = "shutting down"
)

// HealthChecker provides health check endpoints for Kubernetes probes.
type HealthChecker struct {
	// ready indicates whether the server is ready to receive traffic
	ready atomic.Bool
	// serverContext provides access to dependencies for health checks
	serverContext *ServerContext
	// startTime tracks when the server started
	startTime time.Time
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(sc *ServerContext) *HealthChecker {
	h := &HealthChecker{
		serverContext: sc,
		startTime:     time.Now(),
	}
	// Server starts as ready by default
	h.ready.Store(true)
	return h
}

// SetReady sets the readiness state of the server.
func (h *HealthChecker) SetReady(ready bool) {
	h.ready.Store(ready)
}

// IsReady returns whether the server is ready to receive traffic.
func (h *HealthChecker) IsReady() bool {
	return h.ready.Load()
}

// HealthResponse represents the JSON response for health endpoints.
type HealthResponse struct {
	Status  string            `json:"status"`
	Checks  map[string]string `json:"checks,omitempty"`
	Version string            `json:"version,omitempty"`
}

// DetailedHealthResponse provides comprehensive health information including federation status.
type DetailedHealthResponse struct {
	Status            string                      `json:"status"`
	Mode              string                      `json:"mode"`
	Version           string                      `json:"version,omitempty"`
	Uptime            string                      `json:"uptime"`
	ManagementCluster *ManagementClusterStatus    `json:"management_cluster,omitempty"`
	Federation        *FederationHealthStatus     `json:"federation,omitempty"`
	Instrumentation   *InstrumentationHealthCheck `json:"instrumentation,omitempty"`
}

// ManagementClusterStatus provides health information about the management cluster connection.
type ManagementClusterStatus struct {
	Connected        bool `json:"connected"`
	CAPICRDAvailable bool `json:"capi_crd_available"`
}

// FederationHealthStatus provides health information about federation functionality.
type FederationHealthStatus struct {
	Enabled       bool `json:"enabled"`
	CachedClients int  `json:"cached_clients"`
}

// InstrumentationHealthCheck provides health information about instrumentation.
type InstrumentationHealthCheck struct {
	Enabled         bool   `json:"enabled"`
	MetricsExporter string `json:"metrics_exporter,omitempty"`
	TracingExporter string `json:"tracing_exporter,omitempty"`
}

// LivenessHandler returns an HTTP handler for the /healthz endpoint.
// Liveness probes indicate whether the process should be restarted.
// This should be a simple check that the server process is running.
func (h *HealthChecker) LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple liveness check - if we can respond, we're alive
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := HealthResponse{
			Status: healthStatusOK,
		}

		// Add version if available from server context
		if h.serverContext != nil && h.serverContext.Config() != nil {
			response.Version = h.serverContext.Config().Version
		}

		_ = json.NewEncoder(w).Encode(response)
	})
}

// ReadinessHandler returns an HTTP handler for the /readyz endpoint.
// Readiness probes indicate whether the server is ready to receive traffic.
func (h *HealthChecker) ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		checks := make(map[string]string)
		allOk := true

		// Check if server is marked as ready
		if !h.ready.Load() {
			checks["ready"] = healthStatusNotReady
			allOk = false
		} else {
			checks["ready"] = healthStatusOK
		}

		// Check if server context is not shutdown
		if h.serverContext != nil && h.serverContext.IsShutdown() {
			checks["shutdown"] = healthStatusShuttingDown
			allOk = false
		} else {
			checks["shutdown"] = healthStatusOK
		}

		// Check instrumentation provider if enabled
		if h.serverContext != nil {
			provider := h.serverContext.InstrumentationProvider()
			if provider != nil {
				if provider.Enabled() {
					checks["instrumentation"] = healthStatusOK
				} else {
					checks["instrumentation"] = "disabled"
				}
			}
		}

		response := HealthResponse{
			Checks: checks,
		}

		if allOk {
			response.Status = healthStatusOK
			w.WriteHeader(http.StatusOK)
		} else {
			response.Status = healthStatusNotReady
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(response)
	})
}

// RegisterHealthEndpoints registers health check endpoints on the given mux.
func (h *HealthChecker) RegisterHealthEndpoints(mux *http.ServeMux) {
	mux.Handle("/healthz", h.LivenessHandler())
	mux.Handle("/readyz", h.ReadinessHandler())
	mux.Handle("/healthz/detailed", h.DetailedHealthHandler())
}

// DetailedHealthHandler returns an HTTP handler for the /healthz/detailed endpoint.
// This endpoint provides comprehensive health information including federation status.
func (h *HealthChecker) DetailedHealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := DetailedHealthResponse{
			Status: healthStatusOK,
			Mode:   h.determineMode(),
			Uptime: time.Since(h.startTime).Truncate(time.Second).String(),
		}

		// Add version if available
		if h.serverContext != nil && h.serverContext.Config() != nil {
			response.Version = h.serverContext.Config().Version
		}

		// Check federation status
		if h.serverContext != nil {
			response.Federation = h.getFederationStatus()
			response.ManagementCluster = h.getManagementClusterStatus()
			response.Instrumentation = h.getInstrumentationStatus()
		}

		// Determine overall status
		if !h.ready.Load() {
			response.Status = healthStatusNotReady
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if h.serverContext != nil && h.serverContext.IsShutdown() {
			response.Status = healthStatusShuttingDown
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		_ = json.NewEncoder(w).Encode(response)
	})
}

// determineMode returns the operational mode of the server.
func (h *HealthChecker) determineMode() string {
	if h.serverContext == nil {
		return "unknown"
	}

	if h.serverContext.FederationEnabled() {
		return "capi"
	}

	if h.serverContext.InClusterMode() {
		return "in-cluster"
	}

	return "local"
}

// getFederationStatus returns federation health status.
func (h *HealthChecker) getFederationStatus() *FederationHealthStatus {
	status := &FederationHealthStatus{
		Enabled:       h.serverContext.FederationEnabled(),
		CachedClients: 0,
	}

	// Get cached client count from federation manager stats
	if fedStats := h.serverContext.FederationStats(); fedStats != nil {
		status.CachedClients = fedStats.CacheSize
	}

	return status
}

// getManagementClusterStatus returns management cluster connection status.
func (h *HealthChecker) getManagementClusterStatus() *ManagementClusterStatus {
	if !h.serverContext.FederationEnabled() {
		return nil
	}

	// In CAPI mode, we're connected to the management cluster
	return &ManagementClusterStatus{
		Connected:        true,
		CAPICRDAvailable: true, // Assume true if federation is enabled
	}
}

// getInstrumentationStatus returns instrumentation health status.
func (h *HealthChecker) getInstrumentationStatus() *InstrumentationHealthCheck {
	provider := h.serverContext.InstrumentationProvider()
	if provider == nil {
		return &InstrumentationHealthCheck{
			Enabled: false,
		}
	}

	return &InstrumentationHealthCheck{
		Enabled: provider.Enabled(),
	}
}
