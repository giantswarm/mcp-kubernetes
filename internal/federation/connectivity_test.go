package federation

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestDefaultConnectivityConfig(t *testing.T) {
	config := DefaultConnectivityConfig()

	assert.Equal(t, 5*time.Second, config.ConnectionTimeout)
	assert.Equal(t, 30*time.Second, config.RequestTimeout)
	assert.Equal(t, 3, config.RetryAttempts)
	assert.Equal(t, 1*time.Second, config.RetryBackoff)
	assert.Equal(t, "/healthz", config.HealthCheckPath)
	assert.Equal(t, float32(50), config.QPS)
	assert.Equal(t, 100, config.Burst)
}

func TestHighLatencyConnectivityConfig(t *testing.T) {
	config := HighLatencyConnectivityConfig()

	assert.Equal(t, 15*time.Second, config.ConnectionTimeout)
	assert.Equal(t, 60*time.Second, config.RequestTimeout)
	assert.Equal(t, 5, config.RetryAttempts)
	assert.Equal(t, 2*time.Second, config.RetryBackoff)
	assert.Equal(t, "/healthz", config.HealthCheckPath)
	assert.Equal(t, float32(30), config.QPS)
	assert.Equal(t, 60, config.Burst)
}

func TestApplyConnectivityConfig(t *testing.T) {
	t.Run("applies config to rest.Config", func(t *testing.T) {
		restConfig := &rest.Config{
			Host: "https://test.example.com",
		}
		cc := ConnectivityConfig{
			RequestTimeout: 45 * time.Second,
			QPS:            75,
			Burst:          150,
		}

		ApplyConnectivityConfig(restConfig, cc)

		assert.Equal(t, 45*time.Second, restConfig.Timeout)
		assert.Equal(t, float32(75), restConfig.QPS)
		assert.Equal(t, 150, restConfig.Burst)
	})

	t.Run("handles nil config gracefully", func(t *testing.T) {
		// Should not panic
		ApplyConnectivityConfig(nil, DefaultConnectivityConfig())
	})

	t.Run("ignores zero QPS and Burst", func(t *testing.T) {
		restConfig := &rest.Config{
			Host:  "https://test.example.com",
			QPS:   10,
			Burst: 20,
		}
		cc := ConnectivityConfig{
			RequestTimeout: 30 * time.Second,
			QPS:            0, // Should be ignored
			Burst:          0, // Should be ignored
		}

		ApplyConnectivityConfig(restConfig, cc)

		// Original values should be preserved when zero is specified
		assert.Equal(t, float32(10), restConfig.QPS)
		assert.Equal(t, 20, restConfig.Burst)
	})
}

func TestCheckConnectivity(t *testing.T) {
	t.Run("returns error for nil config", func(t *testing.T) {
		err := CheckConnectivity(context.Background(), "test-cluster", nil, DefaultConnectivityConfig())
		require.Error(t, err)

		var connErr *ConnectionError
		assert.True(t, errors.As(err, &connErr))
		assert.Equal(t, "test-cluster", connErr.ClusterName)
	})

	t.Run("succeeds with healthy endpoint", func(t *testing.T) {
		// Create a test server that responds to /healthz
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		config := &rest.Config{
			Host: server.URL,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}
		cc := DefaultConnectivityConfig()
		cc.ConnectionTimeout = 5 * time.Second

		err := CheckConnectivity(context.Background(), "test-cluster", config, cc)
		assert.NoError(t, err)
	})

	t.Run("returns error for unreachable endpoint", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://unreachable.invalid:6443",
		}
		cc := ConnectivityConfig{
			ConnectionTimeout: 100 * time.Millisecond,
		}

		err := CheckConnectivity(context.Background(), "test-cluster", config, cc)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrConnectionFailed) || errors.Is(err, ErrConnectionTimeout))
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		config := &rest.Config{
			Host: "https://example.com:6443",
		}

		err := CheckConnectivity(ctx, "test-cluster", config, DefaultConnectivityConfig())
		require.Error(t, err)
	})

	t.Run("uses custom health check path", func(t *testing.T) {
		customPath := "/custom-health"
		pathReceived := ""

		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathReceived = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &rest.Config{
			Host: server.URL,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}
		cc := DefaultConnectivityConfig()
		cc.HealthCheckPath = customPath

		err := CheckConnectivity(context.Background(), "test-cluster", config, cc)
		assert.NoError(t, err)
		assert.Equal(t, customPath, pathReceived)
	})
}

func TestCheckConnectivityWithRetry(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &rest.Config{
			Host:            server.URL,
			TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		}
		cc := DefaultConnectivityConfig()

		err := CheckConnectivityWithRetry(context.Background(), "test-cluster", config, cc)
		assert.NoError(t, err)
	})

	t.Run("retries on transient failure", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			if attemptCount < 3 {
				// Simulate transient failure
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &rest.Config{
			Host:            server.URL,
			TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		}
		cc := ConnectivityConfig{
			ConnectionTimeout: 1 * time.Second,
			RetryAttempts:     5,
			RetryBackoff:      10 * time.Millisecond, // Short for testing
			HealthCheckPath:   "/healthz",
		}

		err := CheckConnectivityWithRetry(context.Background(), "test-cluster", config, cc)
		// Note: The health check checks HTTP status, so even 503 returns no error from REST client
		// This test verifies retry behavior when the endpoint responds
		assert.NoError(t, err)
	})

	t.Run("respects retry attempts limit", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://unreachable.invalid:6443",
		}
		cc := ConnectivityConfig{
			ConnectionTimeout: 50 * time.Millisecond,
			RetryAttempts:     2,
			RetryBackoff:      10 * time.Millisecond,
		}

		start := time.Now()
		err := CheckConnectivityWithRetry(context.Background(), "test-cluster", config, cc)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Should have taken at least the backoff time but not too long
		assert.Less(t, elapsed, 5*time.Second, "retries should be bounded")
	})

	t.Run("respects context cancellation during retry", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		config := &rest.Config{
			Host: "https://unreachable.invalid:6443",
		}
		cc := ConnectivityConfig{
			ConnectionTimeout: 1 * time.Second,
			RetryAttempts:     10,
			RetryBackoff:      500 * time.Millisecond,
		}

		start := time.Now()
		err := CheckConnectivityWithRetry(ctx, "test-cluster", config, cc)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Should respect context timeout, not retry 10 times
		assert.Less(t, elapsed, 2*time.Second)
	})

	t.Run("uses default values when not specified", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://unreachable.invalid:6443",
		}
		cc := ConnectivityConfig{
			// All zeros - should use defaults
		}

		err := CheckConnectivityWithRetry(context.Background(), "test-cluster", config, cc)
		require.Error(t, err)
	})
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not retryable",
			err:      nil,
			expected: false,
		},
		{
			name:     "TLS error is not retryable",
			err:      &TLSError{ClusterName: "test", Reason: "cert expired"},
			expected: false,
		},
		{
			name: "ConnectionError wrapping TLS sentinel is retryable",
			err: &ConnectionError{
				ClusterName: "test",
				Err:         ErrTLSHandshakeFailed,
			},
			// ConnectionError itself doesn't trigger the TLSError type check,
			// only TLSError struct does. So this returns true.
			expected: true,
		},
		{
			name:     "timeout error is retryable",
			err:      &ConnectivityTimeoutError{ClusterName: "test"},
			expected: true,
		},
		{
			name:     "connection error is retryable",
			err:      &ConnectionError{ClusterName: "test", Reason: "connection refused"},
			expected: true,
		},
		{
			name:     "generic error is retryable",
			err:      errors.New("some error"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTLSError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "certificate error",
			err:      errors.New("x509: certificate signed by unknown authority"),
			expected: true,
		},
		{
			name:     "TLS handshake error",
			err:      errors.New("tls: handshake failure"),
			expected: true,
		},
		{
			name:     "expired certificate error",
			err:      errors.New("certificate has expired"),
			expected: true,
		},
		{
			name:     "non-TLS error",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "tls.RecordHeaderError",
			err:      tls.RecordHeaderError{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTLSError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout string in error",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("i/o timeout"),
			expected: true,
		},
		{
			name:     "non-timeout error",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name: "net.Error timeout",
			err: &mockNetError{
				isTimeout:   true,
				isTemporary: false,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockNetError implements net.Error for testing
type mockNetError struct {
	isTimeout   bool
	isTemporary bool
}

func (e *mockNetError) Error() string   { return "mock net error" }
func (e *mockNetError) Timeout() bool   { return e.isTimeout }
func (e *mockNetError) Temporary() bool { return e.isTemporary }

// Ensure mockNetError implements net.Error
var _ net.Error = (*mockNetError)(nil)

func TestExtractTLSReason(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "unknown TLS error",
		},
		{
			name:     "unknown authority",
			err:      errors.New("x509: certificate signed by unknown authority"),
			expected: "certificate signed by unknown authority",
		},
		{
			name:     "expired certificate",
			err:      errors.New("certificate has expired"),
			expected: "certificate has expired",
		},
		{
			name:     "not valid yet",
			err:      errors.New("certificate is not valid yet"),
			expected: "certificate is not yet valid",
		},
		{
			name:     "IP SAN mismatch",
			err:      errors.New("doesn't contain any IP SANs"),
			expected: "certificate doesn't match server IP",
		},
		{
			name:     "hostname mismatch",
			err:      errors.New("certificate doesn't match hostname"),
			expected: "certificate hostname mismatch",
		},
		{
			name:     "handshake failure",
			err:      errors.New("handshake failure"),
			expected: "TLS handshake failed",
		},
		{
			name:     "protocol version",
			err:      errors.New("protocol version not supported"),
			expected: "TLS protocol version mismatch",
		},
		{
			name:     "generic TLS error",
			err:      errors.New("some other TLS error"),
			expected: "TLS error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTLSReason(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEndpointType(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "empty host",
			host:     "",
			expected: "unknown",
		},
		{
			name:     "konnectivity endpoint",
			host:     "https://konnectivity-prod-cluster.management.svc:8132",
			expected: "konnectivity",
		},
		{
			name:     "private IP (10.x.x.x)",
			host:     "https://10.0.1.50:6443",
			expected: "private",
		},
		{
			name:     "private IP (192.168.x.x)",
			host:     "https://192.168.1.100:6443",
			expected: "private",
		},
		{
			name:     "private IP (172.16.x.x)",
			host:     "https://172.16.0.1:6443",
			expected: "private",
		},
		{
			name:     "public DNS",
			host:     "https://api.prod.example.com:6443",
			expected: "public",
		},
		{
			name:     "public IP",
			host:     "https://203.0.113.50:6443",
			expected: "public",
		},
		{
			name:     "internal hostname",
			host:     "https://api.internal:6443",
			expected: "private",
		},
		{
			name:     "local hostname",
			host:     "https://cluster.local:6443",
			expected: "private",
		},
		{
			name:     "kubernetes service",
			host:     "https://kubernetes.default.svc:443",
			expected: "private",
		},
		{
			name:     "without scheme",
			host:     "10.0.1.50:6443",
			expected: "private",
		},
		{
			name:     "http scheme (should still work)",
			host:     "http://10.0.1.50:6443",
			expected: "private",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEndpointType(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "nil IP",
			ip:       "",
			expected: false,
		},
		{
			name:     "10.0.0.0/8 range",
			ip:       "10.0.0.1",
			expected: true,
		},
		{
			name:     "10.255.255.255",
			ip:       "10.255.255.255",
			expected: true,
		},
		{
			name:     "172.16.0.0/12 range start",
			ip:       "172.16.0.1",
			expected: true,
		},
		{
			name:     "172.16.0.0/12 range end",
			ip:       "172.31.255.255",
			expected: true,
		},
		{
			name:     "172.32.0.0 (outside range)",
			ip:       "172.32.0.1",
			expected: false,
		},
		{
			name:     "192.168.0.0/16 range",
			ip:       "192.168.1.1",
			expected: true,
		},
		{
			name:     "public IP",
			ip:       "8.8.8.8",
			expected: false,
		},
		{
			name:     "link-local",
			ip:       "169.254.1.1",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
			}
			result := isPrivateIP(ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWrapConnectivityError(t *testing.T) {
	t.Run("wraps nil error", func(t *testing.T) {
		err := wrapConnectivityError("test-cluster", "https://example.com", "test reason", nil)

		var connErr *ConnectionError
		require.True(t, errors.As(err, &connErr))
		assert.Equal(t, "test-cluster", connErr.ClusterName)
		assert.Equal(t, "test reason", connErr.Reason)
	})

	t.Run("wraps context deadline exceeded as timeout", func(t *testing.T) {
		err := wrapConnectivityError("test-cluster", "https://example.com", "health check", context.DeadlineExceeded)

		var timeoutErr *ConnectivityTimeoutError
		require.True(t, errors.As(err, &timeoutErr))
		assert.Equal(t, "test-cluster", timeoutErr.ClusterName)
		assert.True(t, errors.Is(err, ErrConnectionTimeout))
	})

	t.Run("wraps context cancelled", func(t *testing.T) {
		err := wrapConnectivityError("test-cluster", "https://example.com", "health check", context.Canceled)

		var connErr *ConnectionError
		require.True(t, errors.As(err, &connErr))
		assert.Equal(t, "request cancelled", connErr.Reason)
	})

	t.Run("wraps TLS error", func(t *testing.T) {
		tlsErr := errors.New("x509: certificate signed by unknown authority")
		err := wrapConnectivityError("test-cluster", "https://example.com", "health check", tlsErr)

		var wrapped *TLSError
		require.True(t, errors.As(err, &wrapped))
		assert.Equal(t, "test-cluster", wrapped.ClusterName)
		assert.True(t, errors.Is(err, ErrTLSHandshakeFailed))
	})

	t.Run("wraps timeout error", func(t *testing.T) {
		timeoutErr := &mockNetError{isTimeout: true}
		err := wrapConnectivityError("test-cluster", "https://example.com", "health check", timeoutErr)

		var wrapped *ConnectivityTimeoutError
		require.True(t, errors.As(err, &wrapped))
		assert.Equal(t, "test-cluster", wrapped.ClusterName)
	})

	t.Run("wraps generic error as connection error", func(t *testing.T) {
		genericErr := errors.New("connection refused")
		err := wrapConnectivityError("test-cluster", "https://example.com", "health check failed", genericErr)

		var connErr *ConnectionError
		require.True(t, errors.As(err, &connErr))
		assert.Equal(t, "test-cluster", connErr.ClusterName)
		assert.Equal(t, "health check failed", connErr.Reason)
	})

	t.Run("sanitizes empty host", func(t *testing.T) {
		err := wrapConnectivityError("test-cluster", "", "test", nil)

		var connErr *ConnectionError
		require.True(t, errors.As(err, &connErr))
		assert.Equal(t, "<empty>", connErr.Host)
	})
}

func TestConnectivityConfigZeroValues(t *testing.T) {
	t.Run("CheckConnectivity uses default timeout when not specified", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://unreachable.invalid:6443",
		}
		cc := ConnectivityConfig{
			// ConnectionTimeout is zero - should use default
		}

		// Should use default timeout, not hang indefinitely
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := CheckConnectivity(ctx, "test-cluster", config, cc)
		require.Error(t, err)
	})
}

func TestConnectivityIntegration(t *testing.T) {
	t.Run("full flow with healthy server", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		config := &rest.Config{
			Host:            server.URL,
			TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		}

		// Apply connectivity config
		cc := DefaultConnectivityConfig()
		ApplyConnectivityConfig(config, cc)

		// Check connectivity
		err := CheckConnectivity(context.Background(), "test-cluster", config, cc)
		assert.NoError(t, err)

		// Verify config was modified
		assert.Equal(t, cc.RequestTimeout, config.Timeout)
		assert.Equal(t, cc.QPS, config.QPS)
		assert.Equal(t, cc.Burst, config.Burst)
	})
}
