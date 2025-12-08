package server

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// HealthChecker provides health check endpoints for Kubernetes probes.
type HealthChecker struct {
	// ready indicates whether the server is ready to receive traffic
	ready atomic.Bool
	// serverContext provides access to dependencies for health checks
	serverContext *ServerContext
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(sc *ServerContext) *HealthChecker {
	h := &HealthChecker{
		serverContext: sc,
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

// LivenessHandler returns an HTTP handler for the /healthz endpoint.
// Liveness probes indicate whether the process should be restarted.
// This should be a simple check that the server process is running.
func (h *HealthChecker) LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple liveness check - if we can respond, we're alive
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := HealthResponse{
			Status: "ok",
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
			checks["ready"] = "not ready"
			allOk = false
		} else {
			checks["ready"] = "ok"
		}

		// Check if server context is not shutdown
		if h.serverContext != nil && h.serverContext.IsShutdown() {
			checks["shutdown"] = "shutting down"
			allOk = false
		} else {
			checks["shutdown"] = "ok"
		}

		// Check instrumentation provider if enabled
		if h.serverContext != nil {
			provider := h.serverContext.InstrumentationProvider()
			if provider != nil {
				if provider.Enabled() {
					checks["instrumentation"] = "ok"
				} else {
					checks["instrumentation"] = "disabled"
				}
			}
		}

		response := HealthResponse{
			Checks: checks,
		}

		if allOk {
			response.Status = "ok"
			w.WriteHeader(http.StatusOK)
		} else {
			response.Status = "not ready"
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(response)
	})
}

// RegisterHealthEndpoints registers health check endpoints on the given mux.
func (h *HealthChecker) RegisterHealthEndpoints(mux *http.ServeMux) {
	mux.Handle("/healthz", h.LivenessHandler())
	mux.Handle("/readyz", h.ReadinessHandler())
}
