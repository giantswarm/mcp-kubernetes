package federation

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterNotFoundError(t *testing.T) {
	tests := []struct {
		name           string
		err            *ClusterNotFoundError
		expectedString string
	}{
		{
			name: "with namespace",
			err: &ClusterNotFoundError{
				ClusterName: "my-cluster",
				Namespace:   "org-acme",
				Reason:      "cluster does not exist",
			},
			expectedString: `cluster "my-cluster" not found in namespace "org-acme": cluster does not exist`,
		},
		{
			name: "without namespace",
			err: &ClusterNotFoundError{
				ClusterName: "my-cluster",
				Reason:      "user does not have access",
			},
			expectedString: `cluster "my-cluster" not found: user does not have access`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())
			assert.True(t, errors.Is(tt.err, ErrClusterNotFound))
		})
	}
}

func TestClusterNotFoundError_Unwrap(t *testing.T) {
	err := &ClusterNotFoundError{
		ClusterName: "test",
		Reason:      "not found",
	}

	assert.True(t, errors.Is(err, ErrClusterNotFound))
}

func TestKubeconfigError(t *testing.T) {
	tests := []struct {
		name           string
		err            *KubeconfigError
		expectedString string
		unwrapsTo      error
	}{
		{
			name: "with underlying error",
			err: &KubeconfigError{
				ClusterName: "my-cluster",
				SecretName:  "my-cluster-kubeconfig",
				Namespace:   "org-acme",
				Reason:      "failed to parse",
				Err:         fmt.Errorf("invalid YAML"),
			},
			expectedString: `kubeconfig error for cluster "my-cluster" (secret org-acme/my-cluster-kubeconfig): failed to parse: invalid YAML`,
			unwrapsTo:      fmt.Errorf("invalid YAML"),
		},
		{
			name: "secret not found",
			err: &KubeconfigError{
				ClusterName: "my-cluster",
				SecretName:  "my-cluster-kubeconfig",
				Namespace:   "org-acme",
				Reason:      "secret not found",
				NotFound:    true,
			},
			expectedString: `kubeconfig error for cluster "my-cluster" (secret org-acme/my-cluster-kubeconfig): secret not found`,
			unwrapsTo:      ErrKubeconfigSecretNotFound,
		},
		{
			name: "invalid kubeconfig",
			err: &KubeconfigError{
				ClusterName: "my-cluster",
				SecretName:  "my-cluster-kubeconfig",
				Namespace:   "org-acme",
				Reason:      "invalid data",
			},
			expectedString: `kubeconfig error for cluster "my-cluster" (secret org-acme/my-cluster-kubeconfig): invalid data`,
			unwrapsTo:      ErrKubeconfigInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())

			unwrapped := tt.err.Unwrap()
			if tt.err.Err != nil {
				assert.Equal(t, tt.err.Err, unwrapped)
			} else {
				assert.Equal(t, tt.unwrapsTo, unwrapped)
			}
		})
	}
}

func TestKubeconfigError_ErrorsIs(t *testing.T) {
	baseErr := fmt.Errorf("underlying error")
	err := &KubeconfigError{
		ClusterName: "test",
		SecretName:  "test-kubeconfig",
		Namespace:   "default",
		Reason:      "failed",
		Err:         baseErr,
	}

	// Should unwrap to the underlying error
	assert.Equal(t, baseErr, err.Unwrap())
}

func TestKubeconfigError_SecretNotFound(t *testing.T) {
	err := &KubeconfigError{
		ClusterName: "test",
		SecretName:  "test-kubeconfig",
		Namespace:   "default",
		Reason:      "secret not found",
		NotFound:    true,
	}

	assert.True(t, errors.Is(err, ErrKubeconfigSecretNotFound))
}

func TestConnectionError(t *testing.T) {
	tests := []struct {
		name           string
		err            *ConnectionError
		expectedString string
	}{
		{
			name: "with underlying error",
			err: &ConnectionError{
				ClusterName: "my-cluster",
				Host:        "https://api.my-cluster.example.com:6443",
				Reason:      "TLS handshake failed",
				Err:         fmt.Errorf("certificate has expired"),
			},
			expectedString: `connection to cluster "my-cluster" (https://api.my-cluster.example.com:6443) failed: TLS handshake failed: certificate has expired`,
		},
		{
			name: "without underlying error",
			err: &ConnectionError{
				ClusterName: "my-cluster",
				Host:        "https://api.my-cluster.example.com:6443",
				Reason:      "connection refused",
			},
			expectedString: `connection to cluster "my-cluster" (https://api.my-cluster.example.com:6443) failed: connection refused`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())
		})
	}
}

func TestConnectionError_Unwrap(t *testing.T) {
	t.Run("with underlying error", func(t *testing.T) {
		baseErr := fmt.Errorf("network unreachable")
		err := &ConnectionError{
			ClusterName: "test",
			Host:        "https://api.test.example.com:6443",
			Reason:      "failed",
			Err:         baseErr,
		}

		assert.Equal(t, baseErr, err.Unwrap())
	})

	t.Run("without underlying error", func(t *testing.T) {
		err := &ConnectionError{
			ClusterName: "test",
			Host:        "https://api.test.example.com:6443",
			Reason:      "connection refused",
		}

		assert.True(t, errors.Is(err, ErrConnectionFailed))
	})
}

func TestSentinelErrors(t *testing.T) {
	// Verify all sentinel errors are distinct
	sentinels := []error{
		ErrClusterNotFound,
		ErrKubeconfigSecretNotFound,
		ErrKubeconfigInvalid,
		ErrConnectionFailed,
		ErrImpersonationFailed,
		ErrManagerClosed,
	}

	for i, err1 := range sentinels {
		for j, err2 := range sentinels {
			if i == j {
				assert.True(t, errors.Is(err1, err2), "error should equal itself")
			} else {
				assert.False(t, errors.Is(err1, err2), "different errors should not be equal: %v vs %v", err1, err2)
			}
		}
	}
}

func TestErrorWrapping(t *testing.T) {
	// Test that errors.Is works correctly with wrapped errors
	t.Run("ClusterNotFoundError", func(t *testing.T) {
		err := fmt.Errorf("operation failed: %w", &ClusterNotFoundError{
			ClusterName: "test",
			Reason:      "not found",
		})
		assert.True(t, errors.Is(err, ErrClusterNotFound))
	})

	t.Run("KubeconfigError with secret not found", func(t *testing.T) {
		err := fmt.Errorf("operation failed: %w", &KubeconfigError{
			ClusterName: "test",
			SecretName:  "test-kubeconfig",
			Namespace:   "default",
			Reason:      "secret not found",
			NotFound:    true,
		})
		assert.True(t, errors.Is(err, ErrKubeconfigSecretNotFound))
	})

	t.Run("ConnectionError", func(t *testing.T) {
		err := fmt.Errorf("operation failed: %w", &ConnectionError{
			ClusterName: "test",
			Host:        "https://test:6443",
			Reason:      "timeout",
		})
		assert.True(t, errors.Is(err, ErrConnectionFailed))
	})
}

func TestErrorsAs(t *testing.T) {
	t.Run("ClusterNotFoundError", func(t *testing.T) {
		err := fmt.Errorf("operation failed: %w", &ClusterNotFoundError{
			ClusterName: "test-cluster",
			Namespace:   "org-test",
			Reason:      "not found",
		})

		var clusterErr *ClusterNotFoundError
		assert.True(t, errors.As(err, &clusterErr))
		assert.Equal(t, "test-cluster", clusterErr.ClusterName)
		assert.Equal(t, "org-test", clusterErr.Namespace)
	})

	t.Run("KubeconfigError", func(t *testing.T) {
		err := fmt.Errorf("operation failed: %w", &KubeconfigError{
			ClusterName: "test-cluster",
			SecretName:  "test-cluster-kubeconfig",
			Namespace:   "org-test",
			Reason:      "invalid",
		})

		var kubeconfigErr *KubeconfigError
		assert.True(t, errors.As(err, &kubeconfigErr))
		assert.Equal(t, "test-cluster", kubeconfigErr.ClusterName)
		assert.Equal(t, "test-cluster-kubeconfig", kubeconfigErr.SecretName)
	})

	t.Run("ConnectionError", func(t *testing.T) {
		err := fmt.Errorf("operation failed: %w", &ConnectionError{
			ClusterName: "test-cluster",
			Host:        "https://api.test:6443",
			Reason:      "timeout",
		})

		var connErr *ConnectionError
		assert.True(t, errors.As(err, &connErr))
		assert.Equal(t, "test-cluster", connErr.ClusterName)
		assert.Equal(t, "https://api.test:6443", connErr.Host)
	})
}
