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
		name            string
		err             *KubeconfigError
		expectedString  string
		unwrapsTo       error // The underlying error that Unwrap() returns
		matchesSentinel error // The sentinel error that Is() matches
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
			expectedString:  `kubeconfig error for cluster "my-cluster" (secret org-acme/my-cluster-kubeconfig): failed to parse: invalid YAML`,
			matchesSentinel: ErrKubeconfigInvalid, // NotFound is false by default
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
			expectedString:  `kubeconfig error for cluster "my-cluster" (secret org-acme/my-cluster-kubeconfig): secret not found`,
			matchesSentinel: ErrKubeconfigSecretNotFound,
		},
		{
			name: "invalid kubeconfig",
			err: &KubeconfigError{
				ClusterName: "my-cluster",
				SecretName:  "my-cluster-kubeconfig",
				Namespace:   "org-acme",
				Reason:      "invalid data",
			},
			expectedString:  `kubeconfig error for cluster "my-cluster" (secret org-acme/my-cluster-kubeconfig): invalid data`,
			matchesSentinel: ErrKubeconfigInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())

			// Unwrap returns the underlying error (may be nil)
			unwrapped := tt.err.Unwrap()
			assert.Equal(t, tt.err.Err, unwrapped)

			// Is() matches the sentinel error
			assert.True(t, errors.Is(tt.err, tt.matchesSentinel))
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

func TestUserFacingErrors(t *testing.T) {
	t.Run("ClusterNotFoundError user facing error hides details", func(t *testing.T) {
		err := &ClusterNotFoundError{
			ClusterName: "secret-production-cluster",
			Namespace:   "org-internal-acme",
			Reason:      "RBAC denied access due to missing role binding",
		}

		userFacing := err.UserFacingError()

		// Should not contain internal details
		assert.NotContains(t, userFacing, "secret-production-cluster")
		assert.NotContains(t, userFacing, "org-internal-acme")
		assert.NotContains(t, userFacing, "RBAC")
		assert.NotContains(t, userFacing, "role binding")

		// Should contain generic message (unified across all cluster errors)
		assert.Equal(t, "cluster access denied or unavailable", userFacing)
	})

	t.Run("KubeconfigError user facing error hides secret details", func(t *testing.T) {
		err := &KubeconfigError{
			ClusterName: "production-cluster",
			SecretName:  "production-cluster-kubeconfig",
			Namespace:   "org-internal",
			Reason:      "secret data corrupted",
			NotFound:    false,
		}

		userFacing := err.UserFacingError()

		// Should not contain internal details
		assert.NotContains(t, userFacing, "production-cluster")
		assert.NotContains(t, userFacing, "kubeconfig")
		assert.NotContains(t, userFacing, "org-internal")
		assert.NotContains(t, userFacing, "corrupted")

		// Should contain generic message (unified across all cluster errors)
		assert.Equal(t, "cluster access denied or unavailable", userFacing)
	})

	t.Run("KubeconfigError not found user facing error is identical to other errors", func(t *testing.T) {
		// Security: Both NotFound=true and NotFound=false should return the same
		// user-facing message to prevent cluster existence leakage
		errNotFound := &KubeconfigError{
			ClusterName: "production-cluster",
			SecretName:  "production-cluster-kubeconfig",
			Namespace:   "org-internal",
			Reason:      "secret not found in namespace",
			NotFound:    true,
		}

		errInvalid := &KubeconfigError{
			ClusterName: "production-cluster",
			SecretName:  "production-cluster-kubeconfig",
			Namespace:   "org-internal",
			Reason:      "invalid kubeconfig data",
			NotFound:    false,
		}

		// Both should return the same message
		assert.Equal(t, errNotFound.UserFacingError(), errInvalid.UserFacingError())
		assert.Equal(t, "cluster access denied or unavailable", errNotFound.UserFacingError())
	})

	t.Run("ConnectionError user facing error hides host", func(t *testing.T) {
		err := &ConnectionError{
			ClusterName: "internal-cluster",
			Host:        "https://api.internal.192-168-1-100.nip.io:6443",
			Reason:      "TLS certificate signed by unknown authority",
			Err:         fmt.Errorf("x509: certificate signed by unknown authority"),
		}

		userFacing := err.UserFacingError()

		// Should not contain internal details
		assert.NotContains(t, userFacing, "internal-cluster")
		assert.NotContains(t, userFacing, "192-168-1-100")
		assert.NotContains(t, userFacing, "api.internal")
		assert.NotContains(t, userFacing, "x509")
		assert.NotContains(t, userFacing, "certificate")

		// Should contain generic message (unified across all cluster errors)
		assert.Equal(t, "cluster access denied or unavailable", userFacing)
	})

	t.Run("All cluster errors return same user facing message", func(t *testing.T) {
		// Security: All cluster-related errors should return the same user-facing
		// message to prevent error response differentiation attacks
		clusterNotFound := &ClusterNotFoundError{ClusterName: "test"}
		kubeconfigNotFound := &KubeconfigError{ClusterName: "test", NotFound: true}
		kubeconfigInvalid := &KubeconfigError{ClusterName: "test", NotFound: false}
		connectionError := &ConnectionError{ClusterName: "test"}

		expectedMsg := "cluster access denied or unavailable"
		assert.Equal(t, expectedMsg, clusterNotFound.UserFacingError())
		assert.Equal(t, expectedMsg, kubeconfigNotFound.UserFacingError())
		assert.Equal(t, expectedMsg, kubeconfigInvalid.UserFacingError())
		assert.Equal(t, expectedMsg, connectionError.UserFacingError())
	})
}
