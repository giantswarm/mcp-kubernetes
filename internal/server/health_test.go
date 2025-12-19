// Package server provides tests for health check functionality.
// These tests verify the /healthz, /readyz, and /healthz/detailed endpoints.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
)

// mockFederationManager implements federation.ClusterClientManager for testing.
type mockFederationManager struct {
	stats federation.ManagerStats
}

func (m *mockFederationManager) GetClient(ctx context.Context, clusterName string, user *federation.UserInfo) (kubernetes.Interface, error) {
	return nil, nil
}

func (m *mockFederationManager) GetDynamicClient(ctx context.Context, clusterName string, user *federation.UserInfo) (dynamic.Interface, error) {
	return nil, nil
}

func (m *mockFederationManager) GetRestConfig(ctx context.Context, clusterName string, user *federation.UserInfo) (*rest.Config, error) {
	return &rest.Config{Host: "https://mock-cluster:6443"}, nil
}

func (m *mockFederationManager) ListClusters(ctx context.Context, user *federation.UserInfo) ([]federation.ClusterSummary, error) {
	return nil, nil
}

func (m *mockFederationManager) GetClusterSummary(ctx context.Context, clusterName string, user *federation.UserInfo) (*federation.ClusterSummary, error) {
	return nil, nil
}

func (m *mockFederationManager) CheckAccess(ctx context.Context, clusterName string, user *federation.UserInfo, check *federation.AccessCheck) (*federation.AccessCheckResult, error) {
	return nil, nil
}

func (m *mockFederationManager) Close() error {
	return nil
}

func (m *mockFederationManager) Stats() federation.ManagerStats {
	return m.stats
}

func TestNewHealthChecker(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}

	h := NewHealthChecker(sc)

	require.NotNil(t, h)
	assert.True(t, h.IsReady(), "HealthChecker should start ready")
	assert.NotNil(t, h.serverContext)
	assert.False(t, h.startTime.IsZero(), "startTime should be set")
}

func TestHealthChecker_SetReady(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	assert.True(t, h.IsReady())

	h.SetReady(false)
	assert.False(t, h.IsReady())

	h.SetReady(true)
	assert.True(t, h.IsReady())
}

func TestLivenessHandler(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.LivenessHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
}

func TestReadinessHandler_Ready(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.ReadinessHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "ok", response.Checks["ready"])
	assert.Equal(t, "ok", response.Checks["shutdown"])
}

func TestReadinessHandler_NotReady(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)
	h.SetReady(false)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.ReadinessHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var response HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
	assert.Equal(t, "not ready", response.Checks["ready"])
}

func TestReadinessHandler_ShuttingDown(t *testing.T) {
	sc := &ServerContext{
		config:   NewDefaultConfig(),
		shutdown: true,
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.ReadinessHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var response HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
	assert.Equal(t, "shutting down", response.Checks["shutdown"])
}

func TestDetailedHealthHandler_LocalMode(t *testing.T) {
	sc := &ServerContext{
		config:    NewDefaultConfig(),
		inCluster: false,
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/healthz/detailed", nil)
	rec := httptest.NewRecorder()

	h.DetailedHealthHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response DetailedHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "local", response.Mode)
	assert.NotEmpty(t, response.Uptime)
	assert.Nil(t, response.ManagementCluster, "ManagementCluster should be nil in local mode")
}

func TestDetailedHealthHandler_InClusterMode(t *testing.T) {
	sc := &ServerContext{
		config:    NewDefaultConfig(),
		inCluster: true,
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/healthz/detailed", nil)
	rec := httptest.NewRecorder()

	h.DetailedHealthHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response DetailedHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "in-cluster", response.Mode)
}

func TestDetailedHealthHandler_CAPIMode(t *testing.T) {
	mockManager := &mockFederationManager{
		stats: federation.ManagerStats{
			CacheSize:       42,
			CacheMaxEntries: 1000,
			CacheTTL:        10 * time.Minute,
			Closed:          false,
		},
	}

	sc := &ServerContext{
		config:            NewDefaultConfig(),
		federationManager: mockManager,
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/healthz/detailed", nil)
	rec := httptest.NewRecorder()

	h.DetailedHealthHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response DetailedHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "capi", response.Mode)

	require.NotNil(t, response.Federation)
	assert.True(t, response.Federation.Enabled)
	assert.Equal(t, 42, response.Federation.CachedClients)

	require.NotNil(t, response.ManagementCluster)
	assert.True(t, response.ManagementCluster.Connected)
	assert.True(t, response.ManagementCluster.CAPICRDAvailable)
}

func TestDetailedHealthHandler_NotReady(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)
	h.SetReady(false)

	req := httptest.NewRequest(http.MethodGet, "/healthz/detailed", nil)
	rec := httptest.NewRecorder()

	h.DetailedHealthHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var response DetailedHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
}

func TestDetailedHealthHandler_ShuttingDown(t *testing.T) {
	sc := &ServerContext{
		config:   NewDefaultConfig(),
		shutdown: true,
	}
	h := NewHealthChecker(sc)

	req := httptest.NewRequest(http.MethodGet, "/healthz/detailed", nil)
	rec := httptest.NewRecorder()

	h.DetailedHealthHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var response DetailedHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "shutting down", response.Status)
}

func TestDetailedHealthHandler_NilServerContext(t *testing.T) {
	h := NewHealthChecker(nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz/detailed", nil)
	rec := httptest.NewRecorder()

	h.DetailedHealthHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response DetailedHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "unknown", response.Mode)
}

func TestDetermineMode(t *testing.T) {
	tests := []struct {
		name     string
		sc       *ServerContext
		wantMode string
	}{
		{
			name:     "nil server context",
			sc:       nil,
			wantMode: "unknown",
		},
		{
			name: "federation enabled (CAPI mode)",
			sc: &ServerContext{
				federationManager: &mockFederationManager{},
			},
			wantMode: "capi",
		},
		{
			name: "in-cluster mode",
			sc: &ServerContext{
				inCluster: true,
			},
			wantMode: "in-cluster",
		},
		{
			name: "local mode (default)",
			sc: &ServerContext{
				inCluster: false,
			},
			wantMode: "local",
		},
		{
			name: "federation takes precedence over in-cluster",
			sc: &ServerContext{
				federationManager: &mockFederationManager{},
				inCluster:         true,
			},
			wantMode: "capi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &HealthChecker{serverContext: tt.sc}
			assert.Equal(t, tt.wantMode, h.determineMode())
		})
	}
}

func TestRegisterHealthEndpoints(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	mux := http.NewServeMux()
	h.RegisterHealthEndpoints(mux)

	// Test that all endpoints are registered
	endpoints := []string{"/healthz", "/readyz", "/healthz/detailed"}
	for _, endpoint := range endpoints {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusNotFound, rec.Code, "Endpoint %s should be registered", endpoint)
	}
}

func TestGetFederationStatus_Disabled(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	status := h.getFederationStatus()

	require.NotNil(t, status)
	assert.False(t, status.Enabled)
	assert.Equal(t, 0, status.CachedClients)
}

func TestGetFederationStatus_Enabled(t *testing.T) {
	mockManager := &mockFederationManager{
		stats: federation.ManagerStats{
			CacheSize: 25,
		},
	}

	sc := &ServerContext{
		config:            NewDefaultConfig(),
		federationManager: mockManager,
	}
	h := NewHealthChecker(sc)

	status := h.getFederationStatus()

	require.NotNil(t, status)
	assert.True(t, status.Enabled)
	assert.Equal(t, 25, status.CachedClients)
}

func TestGetManagementClusterStatus_FederationDisabled(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	status := h.getManagementClusterStatus()

	assert.Nil(t, status, "Should return nil when federation is disabled")
}

func TestGetManagementClusterStatus_FederationEnabled(t *testing.T) {
	sc := &ServerContext{
		config:            NewDefaultConfig(),
		federationManager: &mockFederationManager{},
	}
	h := NewHealthChecker(sc)

	status := h.getManagementClusterStatus()

	require.NotNil(t, status)
	assert.True(t, status.Connected)
	assert.True(t, status.CAPICRDAvailable)
}

func TestGetInstrumentationStatus_Disabled(t *testing.T) {
	sc := &ServerContext{
		config: NewDefaultConfig(),
	}
	h := NewHealthChecker(sc)

	status := h.getInstrumentationStatus()

	require.NotNil(t, status)
	assert.False(t, status.Enabled)
}
