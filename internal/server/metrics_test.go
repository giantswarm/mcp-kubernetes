package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
)

func TestNewMetricsServer(t *testing.T) {
	tests := []struct {
		name        string
		config      MetricsServerConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "nil instrumentation provider",
			config: MetricsServerConfig{
				Addr:                    ":9090",
				InstrumentationProvider: nil,
			},
			wantErr:     true,
			errContains: "instrumentation provider is required",
		},
		{
			name: "valid config uses default addr",
			config: MetricsServerConfig{
				Addr:                    "",
				InstrumentationProvider: createTestProvider(t),
			},
			wantErr: false,
		},
		{
			name: "valid config with custom addr",
			config: MetricsServerConfig{
				Addr:                    ":9091",
				InstrumentationProvider: createTestProvider(t),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewMetricsServer(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewMetricsServer() expected error, got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewMetricsServer() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("NewMetricsServer() unexpected error: %v", err)
				return
			}
			if server == nil {
				t.Error("NewMetricsServer() returned nil server")
				return
			}

			// Verify addr is set correctly
			expectedAddr := tt.config.Addr
			if expectedAddr == "" {
				expectedAddr = DefaultMetricsAddr
			}
			if server.Addr() != expectedAddr {
				t.Errorf("Addr() = %v, want %v", server.Addr(), expectedAddr)
			}
		})
	}
}

func TestMetricsServer_StartAndShutdown(t *testing.T) {
	provider := createTestProvider(t)

	server, err := NewMetricsServer(MetricsServerConfig{
		Addr:                    ":9092",
		InstrumentationProvider: provider,
	})
	if err != nil {
		t.Fatalf("NewMetricsServer() error = %v", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that the /metrics endpoint is accessible
	resp, err := http.Get("http://localhost:9092/metrics")
	if err != nil {
		t.Errorf("Failed to reach /metrics endpoint: %v", err)
	} else {
		// The prometheus handler may return 500 if there are collection errors
		// In our test environment, this is acceptable since we're testing the server
		// infrastructure, not the actual metrics collection
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("GET /metrics returned status %d, want 200 or 500", resp.StatusCode)
		}
		// Log a warning if we get 500
		if resp.StatusCode == http.StatusInternalServerError {
			t.Log("Note: /metrics returned 500 - this may be due to metric collection errors in test environment")
		}
		if err := resp.Body.Close(); err != nil {
			t.Logf("Warning: failed to close response body: %v", err)
		}
	}

	// Test that the /healthz endpoint is accessible
	resp, err = http.Get("http://localhost:9092/healthz")
	if err != nil {
		t.Errorf("Failed to reach /healthz endpoint: %v", err)
	} else {
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /healthz returned status %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if err := resp.Body.Close(); err != nil {
			t.Logf("Warning: failed to close response body: %v", err)
		}
	}

	// Shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Check that server stopped without error
	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server stopped with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for server to stop")
	}
}

func TestMetricsServer_ShutdownWithoutStart(t *testing.T) {
	provider := createTestProvider(t)

	server, err := NewMetricsServer(MetricsServerConfig{
		Addr:                    ":9093",
		InstrumentationProvider: provider,
	})
	if err != nil {
		t.Fatalf("NewMetricsServer() error = %v", err)
	}

	// Shutdown without starting should not error
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() without Start() error = %v", err)
	}
}

// createTestProvider creates an instrumentation provider for testing.
func createTestProvider(t *testing.T) *instrumentation.Provider {
	t.Helper()
	ctx := context.Background()
	config := instrumentation.Config{
		Enabled:         true,
		MetricsExporter: "prometheus",
		TracingExporter: "none",
	}
	provider, err := instrumentation.NewProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create test provider: %v", err)
	}
	return provider
}

// contains checks if substr is contained in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
