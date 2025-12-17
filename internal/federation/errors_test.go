package federation

import (
	"errors"
	"fmt"
	"testing"
	"time"

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
		ErrAccessDenied,
		ErrAccessCheckFailed,
		ErrInvalidAccessCheck,
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

func TestAccessDeniedError(t *testing.T) {
	tests := []struct {
		name          string
		err           *AccessDeniedError
		containsVerb  string
		containsRes   string
		containsNS    string
		containsClust string
		containsName  string
	}{
		{
			name: "namespace-scoped resource",
			err: &AccessDeniedError{
				ClusterName: "prod-cluster",
				UserEmail:   "test@example.com",
				Verb:        "delete",
				Resource:    "pods",
				APIGroup:    "",
				Namespace:   "production",
				Reason:      "RBAC: delete denied",
			},
			containsVerb:  "delete",
			containsRes:   "pods",
			containsNS:    "production",
			containsClust: "prod-cluster",
		},
		{
			name: "cluster-scoped resource",
			err: &AccessDeniedError{
				ClusterName: "prod-cluster",
				UserEmail:   "admin@example.com",
				Verb:        "create",
				Resource:    "namespaces",
				APIGroup:    "",
				Namespace:   "",
				Reason:      "no permission",
			},
			containsVerb:  "create",
			containsRes:   "namespaces",
			containsNS:    "cluster-wide",
			containsClust: "prod-cluster",
		},
		{
			name: "with API group",
			err: &AccessDeniedError{
				ClusterName: "prod-cluster",
				UserEmail:   "dev@example.com",
				Verb:        "patch",
				Resource:    "deployments",
				APIGroup:    "apps",
				Namespace:   "default",
				Reason:      "insufficient permissions",
			},
			containsVerb:  "patch",
			containsRes:   "apps/deployments",
			containsNS:    "default",
			containsClust: "prod-cluster",
		},
		{
			name: "specific resource name",
			err: &AccessDeniedError{
				ClusterName: "prod-cluster",
				UserEmail:   "user@example.com",
				Verb:        "delete",
				Resource:    "pods",
				Namespace:   "default",
				Name:        "my-pod",
				Reason:      "denied",
			},
			containsVerb:  "delete",
			containsRes:   "pods/my-pod",
			containsNS:    "default",
			containsClust: "prod-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			assert.Contains(t, errStr, "access denied")
			assert.Contains(t, errStr, tt.containsVerb)
			assert.Contains(t, errStr, tt.containsRes)
			assert.Contains(t, errStr, tt.containsNS)
			assert.Contains(t, errStr, tt.containsClust)
			// Email should be anonymized (contains "user:")
			assert.Contains(t, errStr, "user:")
			assert.True(t, errors.Is(tt.err, ErrAccessDenied))
			assert.Nil(t, tt.err.Unwrap())
		})
	}
}

func TestAccessDeniedError_UserFacingError(t *testing.T) {
	t.Run("namespace-scoped", func(t *testing.T) {
		err := &AccessDeniedError{
			ClusterName: "secret-cluster",
			UserEmail:   "secret-user@internal.corp",
			Verb:        "delete",
			Resource:    "pods",
			APIGroup:    "",
			Namespace:   "production",
			Reason:      "RBAC: internal policy xyz-123 denied",
		}

		userFacing := err.UserFacingError()

		// Should not contain sensitive internal details
		assert.NotContains(t, userFacing, "secret-cluster")
		assert.NotContains(t, userFacing, "secret-user")
		assert.NotContains(t, userFacing, "internal.corp")
		assert.NotContains(t, userFacing, "xyz-123")

		// Should contain actionable information
		assert.Contains(t, userFacing, "delete")
		assert.Contains(t, userFacing, "pods")
		assert.Contains(t, userFacing, "production")
		assert.Contains(t, userFacing, "administrator")
	})

	t.Run("cluster-scoped", func(t *testing.T) {
		err := &AccessDeniedError{
			ClusterName: "prod",
			UserEmail:   "user@example.com",
			Verb:        "create",
			Resource:    "namespaces",
		}

		userFacing := err.UserFacingError()
		assert.Contains(t, userFacing, "create")
		assert.Contains(t, userFacing, "namespaces")
		// Should not contain "in namespace" phrase - cluster-scoped doesn't have namespace location
		assert.NotContains(t, userFacing, "in namespace")
	})

	t.Run("with API group", func(t *testing.T) {
		err := &AccessDeniedError{
			ClusterName: "prod",
			UserEmail:   "user@example.com",
			Verb:        "patch",
			Resource:    "deployments",
			APIGroup:    "apps",
			Namespace:   "default",
		}

		userFacing := err.UserFacingError()
		assert.Contains(t, userFacing, "patch")
		assert.Contains(t, userFacing, "apps/deployments")
	})
}

func TestAccessCheckError(t *testing.T) {
	tests := []struct {
		name           string
		err            *AccessCheckError
		expectedString string
	}{
		{
			name: "with underlying error",
			err: &AccessCheckError{
				ClusterName: "prod-cluster",
				Check:       &AccessCheck{Verb: "get", Resource: "pods"},
				Reason:      "API server unavailable",
				Err:         fmt.Errorf("connection timeout"),
			},
			expectedString: `access check failed for cluster "prod-cluster" (get pods): API server unavailable: connection timeout`,
		},
		{
			name: "without underlying error",
			err: &AccessCheckError{
				ClusterName: "prod-cluster",
				Check:       &AccessCheck{Verb: "delete", Resource: "secrets"},
				Reason:      "SAR request rejected",
			},
			expectedString: `access check failed for cluster "prod-cluster" (delete secrets): SAR request rejected`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())
			assert.True(t, errors.Is(tt.err, ErrAccessCheckFailed))
		})
	}
}

func TestAccessCheckError_Unwrap(t *testing.T) {
	baseErr := fmt.Errorf("underlying error")
	err := &AccessCheckError{
		ClusterName: "test",
		Check:       &AccessCheck{Verb: "get", Resource: "pods"},
		Reason:      "failed",
		Err:         baseErr,
	}

	assert.Equal(t, baseErr, err.Unwrap())
}

func TestAccessCheckError_UserFacingError(t *testing.T) {
	err := &AccessCheckError{
		ClusterName: "secret-internal-cluster",
		Check:       &AccessCheck{Verb: "get", Resource: "pods"},
		Reason:      "internal server error 500",
		Err:         fmt.Errorf("TLS handshake failed: cert invalid"),
	}

	userFacing := err.UserFacingError()

	// Should not contain internal details
	assert.NotContains(t, userFacing, "secret-internal-cluster")
	assert.NotContains(t, userFacing, "500")
	assert.NotContains(t, userFacing, "TLS")
	assert.NotContains(t, userFacing, "cert")

	// Should contain generic actionable message
	assert.Contains(t, userFacing, "verify permissions")
	assert.Contains(t, userFacing, "administrator")
}

func TestAccessDeniedError_ErrorsAs(t *testing.T) {
	err := fmt.Errorf("operation failed: %w", &AccessDeniedError{
		ClusterName: "test-cluster",
		UserEmail:   "user@example.com",
		Verb:        "delete",
		Resource:    "pods",
		Namespace:   "default",
		Reason:      "no permission",
	})

	var accessErr *AccessDeniedError
	assert.True(t, errors.As(err, &accessErr))
	assert.Equal(t, "test-cluster", accessErr.ClusterName)
	assert.Equal(t, "delete", accessErr.Verb)
	assert.Equal(t, "pods", accessErr.Resource)
}

func TestAccessCheckError_ErrorsAs(t *testing.T) {
	err := fmt.Errorf("operation failed: %w", &AccessCheckError{
		ClusterName: "test-cluster",
		Check:       &AccessCheck{Verb: "get", Resource: "pods"},
		Reason:      "API error",
	})

	var checkErr *AccessCheckError
	assert.True(t, errors.As(err, &checkErr))
	assert.Equal(t, "test-cluster", checkErr.ClusterName)
	assert.Equal(t, "get", checkErr.Check.Verb)
}

func TestConnectivityTimeoutError(t *testing.T) {
	tests := []struct {
		name           string
		err            *ConnectivityTimeoutError
		expectedString string
	}{
		{
			name: "with timeout duration",
			err: &ConnectivityTimeoutError{
				ClusterName: "prod-cluster",
				Host:        "https://api.prod.example.com:6443",
				Timeout:     5 * time.Second,
			},
			expectedString: `connection to cluster "prod-cluster" (https://api.prod.example.com:6443) timed out after 5s`,
		},
		{
			name: "with underlying error",
			err: &ConnectivityTimeoutError{
				ClusterName: "prod-cluster",
				Host:        "https://api.prod.example.com:6443",
				Err:         fmt.Errorf("i/o timeout"),
			},
			expectedString: `connection to cluster "prod-cluster" (https://api.prod.example.com:6443) timed out: i/o timeout`,
		},
		{
			name: "minimal info",
			err: &ConnectivityTimeoutError{
				ClusterName: "prod-cluster",
				Host:        "https://api.prod.example.com:6443",
			},
			expectedString: `connection to cluster "prod-cluster" (https://api.prod.example.com:6443) timed out`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())
		})
	}
}

func TestConnectivityTimeoutError_Is(t *testing.T) {
	err := &ConnectivityTimeoutError{
		ClusterName: "test",
		Host:        "https://test:6443",
	}

	// Should match all relevant sentinel errors
	assert.True(t, errors.Is(err, ErrConnectionTimeout))
	assert.True(t, errors.Is(err, ErrClusterUnreachable))
	assert.True(t, errors.Is(err, ErrConnectionFailed))

	// Should not match unrelated errors
	assert.False(t, errors.Is(err, ErrTLSHandshakeFailed))
	assert.False(t, errors.Is(err, ErrClusterNotFound))
}

func TestConnectivityTimeoutError_Unwrap(t *testing.T) {
	baseErr := fmt.Errorf("underlying timeout")
	err := &ConnectivityTimeoutError{
		ClusterName: "test",
		Host:        "https://test:6443",
		Err:         baseErr,
	}

	assert.Equal(t, baseErr, err.Unwrap())
}

func TestConnectivityTimeoutError_UserFacingError(t *testing.T) {
	err := &ConnectivityTimeoutError{
		ClusterName: "secret-internal-cluster",
		Host:        "https://10.0.1.50:6443",
		Timeout:     5 * time.Second,
	}

	userFacing := err.UserFacingError()

	// Should not contain internal details
	assert.NotContains(t, userFacing, "secret-internal-cluster")
	assert.NotContains(t, userFacing, "10.0.1.50")
	assert.NotContains(t, userFacing, "5s")

	// Should contain actionable message
	assert.Contains(t, userFacing, "timed out")
	assert.Contains(t, userFacing, "verify")
}

func TestConnectivityTimeoutError_ErrorsAs(t *testing.T) {
	err := fmt.Errorf("operation failed: %w", &ConnectivityTimeoutError{
		ClusterName: "test-cluster",
		Host:        "https://test:6443",
		Timeout:     10 * time.Second,
	})

	var timeoutErr *ConnectivityTimeoutError
	assert.True(t, errors.As(err, &timeoutErr))
	assert.Equal(t, "test-cluster", timeoutErr.ClusterName)
	assert.Equal(t, 10*time.Second, timeoutErr.Timeout)
}

func TestTLSError(t *testing.T) {
	tests := []struct {
		name           string
		err            *TLSError
		expectedString string
	}{
		{
			name: "with underlying error",
			err: &TLSError{
				ClusterName: "prod-cluster",
				Host:        "https://api.prod.example.com:6443",
				Reason:      "certificate has expired",
				Err:         fmt.Errorf("x509: certificate has expired"),
			},
			expectedString: `TLS handshake with cluster "prod-cluster" (https://api.prod.example.com:6443) failed: certificate has expired: x509: certificate has expired`,
		},
		{
			name: "without underlying error",
			err: &TLSError{
				ClusterName: "prod-cluster",
				Host:        "https://api.prod.example.com:6443",
				Reason:      "certificate signed by unknown authority",
			},
			expectedString: `TLS handshake with cluster "prod-cluster" (https://api.prod.example.com:6443) failed: certificate signed by unknown authority`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.err.Error())
		})
	}
}

func TestTLSError_Is(t *testing.T) {
	err := &TLSError{
		ClusterName: "test",
		Host:        "https://test:6443",
		Reason:      "cert expired",
	}

	// Should match TLS sentinel error
	assert.True(t, errors.Is(err, ErrTLSHandshakeFailed))
	assert.True(t, errors.Is(err, ErrConnectionFailed))

	// Should not match unrelated errors
	assert.False(t, errors.Is(err, ErrConnectionTimeout))
	assert.False(t, errors.Is(err, ErrClusterNotFound))
}

func TestTLSError_Unwrap(t *testing.T) {
	baseErr := fmt.Errorf("x509: certificate has expired")
	err := &TLSError{
		ClusterName: "test",
		Host:        "https://test:6443",
		Reason:      "expired",
		Err:         baseErr,
	}

	assert.Equal(t, baseErr, err.Unwrap())
}

func TestTLSError_UserFacingError(t *testing.T) {
	tests := []struct {
		name            string
		reason          string
		expectedContain string
		hasAdmin        bool // whether it should contain "administrator"
	}{
		{
			name:            "expired certificate",
			reason:          "certificate has expired",
			expectedContain: "expired",
			hasAdmin:        true,
		},
		{
			name:            "unknown authority",
			reason:          "certificate signed by unknown authority",
			expectedContain: "not trusted",
			hasAdmin:        false, // This message says "verify the kubeconfig" instead
		},
		{
			name:            "hostname mismatch",
			reason:          "certificate hostname mismatch",
			expectedContain: "doesn't match",
			hasAdmin:        true,
		},
		{
			name:            "generic TLS error",
			reason:          "some other TLS error",
			expectedContain: "secure connection",
			hasAdmin:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &TLSError{
				ClusterName: "secret-cluster",
				Host:        "https://internal.example.com:6443",
				Reason:      tt.reason,
			}

			userFacing := err.UserFacingError()

			// Should not contain internal details
			assert.NotContains(t, userFacing, "secret-cluster")
			assert.NotContains(t, userFacing, "internal.example.com")

			// Should contain expected message
			assert.Contains(t, userFacing, tt.expectedContain)
			if tt.hasAdmin {
				assert.Contains(t, userFacing, "administrator")
			}
		})
	}
}

func TestTLSError_ErrorsAs(t *testing.T) {
	err := fmt.Errorf("operation failed: %w", &TLSError{
		ClusterName: "test-cluster",
		Host:        "https://test:6443",
		Reason:      "cert expired",
	})

	var tlsErr *TLSError
	assert.True(t, errors.As(err, &tlsErr))
	assert.Equal(t, "test-cluster", tlsErr.ClusterName)
	assert.Equal(t, "cert expired", tlsErr.Reason)
}

func TestNewSentinelErrors(t *testing.T) {
	// Verify the new sentinel errors are distinct from existing ones
	newSentinels := []error{
		ErrClusterUnreachable,
		ErrTLSHandshakeFailed,
		ErrConnectionTimeout,
	}

	existingSentinels := []error{
		ErrClusterNotFound,
		ErrKubeconfigSecretNotFound,
		ErrKubeconfigInvalid,
		ErrConnectionFailed,
		ErrImpersonationFailed,
		ErrManagerClosed,
	}

	// New sentinels should be distinct from each other
	for i, err1 := range newSentinels {
		for j, err2 := range newSentinels {
			if i == j {
				assert.True(t, errors.Is(err1, err2), "error should equal itself")
			} else {
				assert.False(t, errors.Is(err1, err2), "different new errors should not be equal: %v vs %v", err1, err2)
			}
		}
	}

	// New sentinels should be distinct from existing ones
	for _, newErr := range newSentinels {
		for _, existingErr := range existingSentinels {
			assert.False(t, errors.Is(newErr, existingErr), "new error %v should not match existing error %v", newErr, existingErr)
		}
	}
}
