package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

// Note: Test helpers (createTestCAPICluster, createTestKubeconfigSecret, setupTestManager)
// and test kubeconfig constants (testValidKubeconfig, testInvalidKubeconfig) are defined
// in testing_helpers_test.go

func TestGetKubeconfigForCluster_Validation(t *testing.T) {
	// These tests verify that GetKubeconfigForCluster validates inputs at the API boundary
	// as a defense-in-depth measure.

	t.Run("returns error when user is nil", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)

		config, err := manager.GetKubeconfigForCluster(context.Background(), "test-cluster", nil)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUserInfoRequired),
			"expected ErrUserInfoRequired, got %v", err)
		assert.Nil(t, config)
	})

	t.Run("returns error when cluster name is invalid", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)
		user := testUser()

		// Test path traversal attempt
		config, err := manager.GetKubeconfigForCluster(context.Background(), "../secret-cluster", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClusterName),
			"expected ErrInvalidClusterName for path traversal, got %v", err)
		assert.Nil(t, config)
	})

	t.Run("returns error when cluster name is empty", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)
		user := testUser()

		config, err := manager.GetKubeconfigForCluster(context.Background(), "", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClusterName),
			"expected ErrInvalidClusterName for empty name, got %v", err)
		assert.Nil(t, config)
	})

	t.Run("returns error when cluster name contains slash", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)
		user := testUser()

		config, err := manager.GetKubeconfigForCluster(context.Background(), "cluster/name", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClusterName),
			"expected ErrInvalidClusterName for slash in name, got %v", err)
		assert.Nil(t, config)
	})
}

func TestGetKubeconfigForCluster(t *testing.T) {
	tests := []struct {
		name          string
		clusterName   string
		clusters      []*unstructured.Unstructured
		secrets       []*corev1.Secret
		expectedError error
		checkResult   func(*testing.T, *rest.Config)
	}{
		{
			name:        "successfully retrieves kubeconfig with 'value' key",
			clusterName: "test-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("test-cluster", "org-acme"),
			},
			secrets: []*corev1.Secret{
				createTestKubeconfigSecret("test-cluster", "org-acme", CAPISecretKey, testValidKubeconfig),
			},
			checkResult: func(t *testing.T, config *rest.Config) {
				assert.NotNil(t, config)
				assert.Equal(t, "https://test-cluster.example.com:6443", config.Host)
			},
		},
		{
			name:        "successfully retrieves kubeconfig with 'kubeconfig' alternate key",
			clusterName: "alt-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("alt-cluster", "org-acme"),
			},
			secrets: []*corev1.Secret{
				createTestKubeconfigSecret("alt-cluster", "org-acme", CAPISecretKeyAlternate, testValidKubeconfig),
			},
			checkResult: func(t *testing.T, config *rest.Config) {
				assert.NotNil(t, config)
				assert.Equal(t, "https://test-cluster.example.com:6443", config.Host)
			},
		},
		{
			name:          "cluster not found returns error",
			clusterName:   "nonexistent",
			clusters:      []*unstructured.Unstructured{},
			secrets:       []*corev1.Secret{},
			expectedError: ErrClusterNotFound,
		},
		{
			name:        "kubeconfig secret not found returns error",
			clusterName: "no-secret-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("no-secret-cluster", "org-acme"),
			},
			secrets:       []*corev1.Secret{}, // No secret created
			expectedError: ErrKubeconfigSecretNotFound,
		},
		{
			name:        "invalid kubeconfig data returns error",
			clusterName: "invalid-kubeconfig",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("invalid-kubeconfig", "org-acme"),
			},
			secrets: []*corev1.Secret{
				createTestKubeconfigSecret("invalid-kubeconfig", "org-acme", CAPISecretKey, testInvalidKubeconfig),
			},
			expectedError: ErrKubeconfigInvalid,
		},
		{
			name:        "secret with missing keys returns error",
			clusterName: "wrong-key-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("wrong-key-cluster", "org-acme"),
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wrong-key-cluster" + CAPISecretSuffix,
						Namespace: "org-acme",
					},
					Data: map[string][]byte{
						"wrong-key": []byte(testValidKubeconfig),
					},
				},
			},
			expectedError: ErrKubeconfigInvalid,
		},
		{
			name:        "secret with empty data returns error",
			clusterName: "empty-secret-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("empty-secret-cluster", "org-acme"),
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "empty-secret-cluster" + CAPISecretSuffix,
						Namespace: "org-acme",
					},
					Data: map[string][]byte{
						CAPISecretKey: []byte(""), // Empty data
					},
				},
			},
			expectedError: ErrKubeconfigInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupTestManager(t, tt.clusters, tt.secrets)

			user := testUser()
			config, err := manager.GetKubeconfigForCluster(context.Background(), tt.clusterName, user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError),
					"expected error %v, got %v", tt.expectedError, err)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, config)
				}
			}
		})
	}
}

func TestGetKubeconfigForClusterValidated(t *testing.T) {
	t.Run("returns error when connection validation fails", func(t *testing.T) {
		clusters := []*unstructured.Unstructured{
			createTestCAPICluster("unreachable-cluster", "org-acme"),
		}
		secrets := []*corev1.Secret{
			createTestKubeconfigSecret("unreachable-cluster", "org-acme", CAPISecretKey, testValidKubeconfig),
		}
		manager := setupTestManager(t, clusters, secrets)

		user := testUser()
		// GetKubeconfigForClusterValidated will try to connect to the cluster
		// which will fail since we're using a test URL
		config, err := manager.GetKubeconfigForClusterValidated(context.Background(), "unreachable-cluster", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrConnectionFailed),
			"expected connection error, got %v", err)
		assert.Nil(t, config)
	})
}

func TestFindClusterInfo(t *testing.T) {
	tests := []struct {
		name          string
		clusterName   string
		clusters      []*unstructured.Unstructured
		expectedNS    string
		expectedError error
	}{
		{
			name:        "finds cluster in correct namespace",
			clusterName: "prod-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("prod-cluster", "org-production"),
			},
			expectedNS: "org-production",
		},
		{
			name:        "finds cluster among multiple clusters",
			clusterName: "staging-cluster",
			clusters: []*unstructured.Unstructured{
				createTestCAPICluster("prod-cluster", "org-production"),
				createTestCAPICluster("staging-cluster", "org-staging"),
				createTestCAPICluster("dev-cluster", "org-dev"),
			},
			// Note: The fake dynamic client may return clusters in any order
			// so we just verify the cluster is found with the correct name
			expectedNS: "org-staging",
		},
		{
			name:          "returns error when cluster not found",
			clusterName:   "nonexistent",
			clusters:      []*unstructured.Unstructured{},
			expectedError: ErrClusterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake dynamic client for this test
			testScheme := runtime.NewScheme()
			var objects []runtime.Object
			for _, cluster := range tt.clusters {
				objects = append(objects, cluster)
			}
			fakeDynamic := createTestFakeDynamicClient(testScheme, objects...)

			manager := setupTestManager(t, tt.clusters, nil)

			user := testUser()
			info, err := manager.findClusterInfo(context.Background(), tt.clusterName, fakeDynamic, user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError))
				assert.Nil(t, info)
			} else {
				require.NoError(t, err)
				require.NotNil(t, info)
				assert.Equal(t, tt.clusterName, info.Name)
				assert.Equal(t, tt.expectedNS, info.Namespace)
			}
		})
	}
}

func TestExtractKubeconfigData(t *testing.T) {
	logger := newTestLogger()

	// Create minimal manager just for testing extractKubeconfigData
	fakeClient := fake.NewSimpleClientset()
	testScheme := runtime.NewScheme()
	fakeDynamic := createTestFakeDynamicClient(testScheme)
	clientProvider := &StaticClientProvider{
		Clientset:     fakeClient,
		DynamicClient: fakeDynamic,
	}
	manager, err := NewManager(clientProvider, WithManagerLogger(logger))
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	info := &ClusterInfo{Name: "test", Namespace: "ns"}

	tests := []struct {
		name          string
		data          map[string][]byte
		expectedError bool
		expectedKey   string // which key the data should be extracted from
	}{
		{
			name: "extracts from 'value' key",
			data: map[string][]byte{
				CAPISecretKey: []byte("kubeconfig-data"),
			},
			expectedKey: CAPISecretKey,
		},
		{
			name: "extracts from 'kubeconfig' key when 'value' is missing",
			data: map[string][]byte{
				CAPISecretKeyAlternate: []byte("kubeconfig-data"),
			},
			expectedKey: CAPISecretKeyAlternate,
		},
		{
			name: "prefers 'value' key over 'kubeconfig' key",
			data: map[string][]byte{
				CAPISecretKey:          []byte("primary-data"),
				CAPISecretKeyAlternate: []byte("alternate-data"),
			},
			expectedKey: CAPISecretKey,
		},
		{
			name: "returns error when both keys missing",
			data: map[string][]byte{
				"other-key": []byte("some-data"),
			},
			expectedError: true,
		},
		{
			name: "returns error when data is empty",
			data: map[string][]byte{
				CAPISecretKey: []byte(""),
			},
			expectedError: true,
		},
		{
			name:          "returns error when data map is nil",
			data:          nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.extractKubeconfigData(tt.data, info, "secret-name")

			if tt.expectedError {
				require.Error(t, err)
				var kubeconfigErr *KubeconfigError
				assert.True(t, errors.As(err, &kubeconfigErr))
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.data[tt.expectedKey], result)
			}
		})
	}
}

func TestConfigWithImpersonation(t *testing.T) {
	t.Run("configures impersonation correctly", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://test.example.com",
		}
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"dev", "ops"},
			Extra: map[string][]string{
				"department": {"engineering"},
			},
		}

		result := ConfigWithImpersonation(config, user)

		assert.Equal(t, "user@example.com", result.Impersonate.UserName)
		assert.Equal(t, []string{"dev", "ops"}, result.Impersonate.Groups)
		assert.Equal(t, map[string][]string{"department": {"engineering"}}, result.Impersonate.Extra)

		// Verify original config wasn't modified
		assert.Empty(t, config.Impersonate.UserName, "original config should not be modified")
	})

	t.Run("panics when user is nil with non-nil config (security)", func(t *testing.T) {
		// Security: ConfigWithImpersonation must panic if user is nil but config is non-nil.
		// Silently returning a non-impersonated config would be a privilege escalation bug.
		config := &rest.Config{
			Host: "https://test.example.com",
		}

		assert.PanicsWithValue(t,
			"federation: ConfigWithImpersonation called with nil user - this is a programming error; validate user with ValidateUserInfo before calling",
			func() {
				ConfigWithImpersonation(config, nil)
			},
			"should panic when user is nil to prevent privilege escalation")
	})

	t.Run("returns nil when config is nil", func(t *testing.T) {
		user := &UserInfo{
			Email: "user@example.com",
		}

		result := ConfigWithImpersonation(nil, user)

		assert.Nil(t, result)
	})

	t.Run("returns nil when both config and user are nil", func(t *testing.T) {
		result := ConfigWithImpersonation(nil, nil)
		assert.Nil(t, result)
	})

	t.Run("does not modify original config", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://original.example.com",
		}
		user := &UserInfo{
			Email: "user@example.com",
		}

		result := ConfigWithImpersonation(config, user)

		// Result should have impersonation
		assert.Equal(t, "user@example.com", result.Impersonate.UserName)
		// Original should be unchanged
		assert.Equal(t, "https://original.example.com", config.Host)
		assert.Empty(t, config.Impersonate.UserName, "original config should not be modified")
	})
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error returns false",
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "empty string returns <empty>",
			host:     "",
			expected: "<empty>",
		},
		{
			name:     "valid host is returned as-is",
			host:     "https://api.example.com:6443",
			expected: "https://api.example.com:6443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHost(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSecretKeys(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string][]byte
		expected []string
	}{
		{
			name:     "nil data returns empty slice",
			data:     nil,
			expected: []string{},
		},
		{
			name:     "empty data returns empty slice",
			data:     map[string][]byte{},
			expected: []string{},
		},
		{
			name: "returns all keys",
			data: map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
			},
			expected: []string{"key1", "key2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSecretKeys(tt.data)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestKubeconfigErrorWrapping(t *testing.T) {
	t.Run("wraps not found error correctly", func(t *testing.T) {
		err := &KubeconfigError{
			ClusterName: "test",
			SecretName:  "test-kubeconfig",
			Namespace:   "ns",
			Reason:      "not found",
			NotFound:    true,
		}

		assert.True(t, errors.Is(err, ErrKubeconfigSecretNotFound))
		assert.False(t, errors.Is(err, ErrKubeconfigInvalid))
	})

	t.Run("wraps invalid error correctly", func(t *testing.T) {
		err := &KubeconfigError{
			ClusterName: "test",
			SecretName:  "test-kubeconfig",
			Namespace:   "ns",
			Reason:      "invalid data",
			NotFound:    false,
		}

		assert.True(t, errors.Is(err, ErrKubeconfigInvalid))
		assert.False(t, errors.Is(err, ErrKubeconfigSecretNotFound))
	})

	t.Run("wraps underlying error", func(t *testing.T) {
		underlyingErr := errors.New("underlying error")
		err := &KubeconfigError{
			ClusterName: "test",
			SecretName:  "test-kubeconfig",
			Namespace:   "ns",
			Reason:      "some reason",
			Err:         underlyingErr,
		}

		assert.True(t, errors.Is(err, underlyingErr))
	})
}

func TestRemoteClientWithKubeconfig(t *testing.T) {
	t.Run("GetClient retrieves remote cluster client", func(t *testing.T) {
		clusters := []*unstructured.Unstructured{
			createTestCAPICluster("remote-cluster", "org-acme"),
		}
		secrets := []*corev1.Secret{
			createTestKubeconfigSecret("remote-cluster", "org-acme", CAPISecretKey, testValidKubeconfig),
		}
		manager := setupTestManager(t, clusters, secrets)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		// Note: GetClient will work but the underlying client won't be able to
		// actually connect to the fake server URL in the kubeconfig
		client, err := manager.GetClient(context.Background(), "remote-cluster", user)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("GetDynamicClient retrieves remote cluster dynamic client", func(t *testing.T) {
		clusters := []*unstructured.Unstructured{
			createTestCAPICluster("remote-cluster", "org-acme"),
		}
		secrets := []*corev1.Secret{
			createTestKubeconfigSecret("remote-cluster", "org-acme", CAPISecretKey, testValidKubeconfig),
		}
		manager := setupTestManager(t, clusters, secrets)

		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers"},
		}

		dynClient, err := manager.GetDynamicClient(context.Background(), "remote-cluster", user)
		require.NoError(t, err)
		assert.NotNil(t, dynClient)
	})

	t.Run("returns error when cluster not found", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)

		user := &UserInfo{Email: "user@example.com"}

		_, err := manager.GetClient(context.Background(), "nonexistent", user)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClusterNotFound))
	})

	t.Run("returns error when kubeconfig secret missing", func(t *testing.T) {
		clusters := []*unstructured.Unstructured{
			createTestCAPICluster("no-secret", "org-acme"),
		}
		manager := setupTestManager(t, clusters, nil)

		user := &UserInfo{Email: "user@example.com"}

		_, err := manager.GetClient(context.Background(), "no-secret", user)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrKubeconfigSecretNotFound))
	})
}

func TestCAPIClusterGVR(t *testing.T) {
	assert.Equal(t, "cluster.x-k8s.io", CAPIClusterGVR.Group)
	assert.Equal(t, "v1beta1", CAPIClusterGVR.Version)
	assert.Equal(t, "clusters", CAPIClusterGVR.Resource)
}

func TestCAPISecretConstants(t *testing.T) {
	assert.Equal(t, "-kubeconfig", CAPISecretSuffix)
	assert.Equal(t, "value", CAPISecretKey)
	assert.Equal(t, "kubeconfig", CAPISecretKeyAlternate)
}

func TestFindClusterInfoWithDynamicClientError(t *testing.T) {
	logger := newTestLogger()

	fakeClient := fake.NewSimpleClientset()
	testScheme := runtime.NewScheme()
	fakeDynamic := createTestFakeDynamicClient(testScheme)

	// Add reactor to simulate API error
	fakeDynamic.PrependReactor("list", "clusters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server unavailable")
	})

	clientProvider := &StaticClientProvider{
		Clientset:     fakeClient,
		DynamicClient: fakeDynamic,
	}
	manager, err := NewManager(clientProvider, WithManagerLogger(logger))
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	user := testUser()
	_, err = manager.findClusterInfo(context.Background(), "any-cluster", fakeDynamic, user)
	require.Error(t, err)

	var clusterErr *ClusterNotFoundError
	assert.True(t, errors.As(err, &clusterErr))
}

func TestGetKubeconfigFromSecretWithClientError(t *testing.T) {
	logger := newTestLogger()

	fakeClient := fake.NewSimpleClientset()

	// Add reactor to simulate API error
	fakeClient.PrependReactor("get", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("permission denied")
	})

	testScheme := runtime.NewScheme()
	fakeDynamic := createTestFakeDynamicClient(testScheme)

	clientProvider := &StaticClientProvider{
		Clientset:     fakeClient,
		DynamicClient: fakeDynamic,
	}
	manager, err := NewManager(clientProvider, WithManagerLogger(logger))
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	user := testUser()
	info := &ClusterInfo{Name: "test-cluster", Namespace: "org-acme"}
	_, err = manager.getKubeconfigFromSecret(context.Background(), info, fakeClient, user)
	require.Error(t, err)

	var kubeconfigErr *KubeconfigError
	assert.True(t, errors.As(err, &kubeconfigErr))
	assert.Equal(t, "test-cluster", kubeconfigErr.ClusterName)
}

func TestClusterInfoStruct(t *testing.T) {
	info := &ClusterInfo{
		Name:      "my-cluster",
		Namespace: "my-namespace",
	}

	assert.Equal(t, "my-cluster", info.Name)
	assert.Equal(t, "my-namespace", info.Namespace)
}

func TestValidateClusterConnectionError(t *testing.T) {
	logger := newTestLogger()

	fakeClient := fake.NewSimpleClientset()
	testScheme := runtime.NewScheme()
	fakeDynamic := createTestFakeDynamicClient(testScheme)

	clientProvider := &StaticClientProvider{
		Clientset:     fakeClient,
		DynamicClient: fakeDynamic,
	}
	manager, err := NewManager(clientProvider, WithManagerLogger(logger))
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	// Create config that will fail validation (unreachable host)
	config := &rest.Config{
		Host: "https://unreachable.example.com:6443",
	}

	err = manager.validateClusterConnection(context.Background(), "test-cluster", config)
	require.Error(t, err)

	var connErr *ConnectionError
	assert.True(t, errors.As(err, &connErr))
	assert.Equal(t, "test-cluster", connErr.ClusterName)
}

func TestConnectionValidationTimeoutConfigurable(t *testing.T) {
	logger := newTestLogger()

	fakeClient := fake.NewSimpleClientset()
	testScheme := runtime.NewScheme()
	fakeDynamic := createTestFakeDynamicClient(testScheme)

	clientProvider := &StaticClientProvider{
		Clientset:     fakeClient,
		DynamicClient: fakeDynamic,
	}

	t.Run("default timeout is used when not configured", func(t *testing.T) {
		manager, err := NewManager(clientProvider, WithManagerLogger(logger))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		assert.Equal(t, DefaultConnectionValidationTimeout, manager.connectionValidationTimeout)
	})

	t.Run("custom timeout is applied via option", func(t *testing.T) {
		customTimeout := 30 * time.Second
		manager, err := NewManager(clientProvider,
			WithManagerLogger(logger),
			WithManagerConnectionValidationTimeout(customTimeout),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		assert.Equal(t, customTimeout, manager.connectionValidationTimeout)
	})

	t.Run("short timeout causes faster failure", func(t *testing.T) {
		// Use a very short timeout to ensure the test runs quickly
		shortTimeout := 100 * time.Millisecond
		manager, err := NewManager(clientProvider,
			WithManagerLogger(logger),
			WithManagerConnectionValidationTimeout(shortTimeout),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		config := &rest.Config{
			Host: "https://unreachable.example.com:6443",
		}

		start := time.Now()
		err = manager.validateClusterConnection(context.Background(), "test-cluster", config)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Should fail within approximately the timeout duration (with some margin)
		assert.Less(t, elapsed, 2*time.Second, "validation should fail quickly with short timeout")
	})
}
