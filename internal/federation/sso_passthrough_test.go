package federation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestSSOPassthroughConfig_Default(t *testing.T) {
	config := DefaultSSOPassthroughConfig()

	if config == nil {
		t.Fatal("DefaultSSOPassthroughConfig returned nil")
	}

	if config.CAConfigMapSuffix != DefaultCAConfigMapSuffix {
		t.Errorf("expected CA ConfigMap suffix %q, got %q", DefaultCAConfigMapSuffix, config.CAConfigMapSuffix)
	}

	if config.TokenExtractor != nil {
		t.Error("expected TokenExtractor to be nil by default")
	}
}

func TestWorkloadClusterAuthMode_Constants(t *testing.T) {
	// Verify the constants have expected values
	if WorkloadClusterAuthModeImpersonation != "impersonation" {
		t.Errorf("expected impersonation mode to be 'impersonation', got %q", WorkloadClusterAuthModeImpersonation)
	}

	if WorkloadClusterAuthModeSSOPassthrough != "sso-passthrough" {
		t.Errorf("expected sso-passthrough mode to be 'sso-passthrough', got %q", WorkloadClusterAuthModeSSOPassthrough)
	}
}

func TestExtractClusterEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		cluster      *unstructured.Unstructured
		wantEndpoint string
	}{
		{
			name: "cluster with host and port",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta2",
					"kind":       "Cluster",
					"metadata": map[string]interface{}{
						"name":      "test-cluster",
						"namespace": "org-test",
					},
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "api.test-cluster.example.com",
							"port": float64(6443),
						},
					},
				},
			},
			wantEndpoint: "https://api.test-cluster.example.com:6443",
		},
		{
			name: "cluster with host only (default port 6443)",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta2",
					"kind":       "Cluster",
					"metadata": map[string]interface{}{
						"name":      "test-cluster",
						"namespace": "org-test",
					},
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "api.test-cluster.example.com",
						},
					},
				},
			},
			wantEndpoint: "https://api.test-cluster.example.com:6443",
		},
		{
			name: "cluster without controlPlaneEndpoint returns empty",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta2",
					"kind":       "Cluster",
					"metadata": map[string]interface{}{
						"name":      "test-cluster",
						"namespace": "org-test",
					},
					"spec": map[string]interface{}{},
				},
			},
			wantEndpoint: "",
		},
		{
			name: "cluster with custom port",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta2",
					"kind":       "Cluster",
					"metadata": map[string]interface{}{
						"name":      "test-cluster",
						"namespace": "org-test",
					},
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "api.test-cluster.example.com",
							"port": float64(8443),
						},
					},
				},
			},
			wantEndpoint: "https://api.test-cluster.example.com:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := extractClusterEndpoint(tt.cluster)
			if endpoint != tt.wantEndpoint {
				t.Errorf("expected endpoint %q, got %q", tt.wantEndpoint, endpoint)
			}
		})
	}
}

func TestManager_GetCAFromConfigMap(t *testing.T) {
	tests := []struct {
		name       string
		configMap  *corev1.ConfigMap
		wantErr    bool
		wantCAData []byte
	}{
		{
			name: "valid CA ConfigMap",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-ca-public",
					Namespace: "org-test",
				},
				Data: map[string]string{
					"ca.crt": "-----BEGIN CERTIFICATE-----\ntest-ca-data\n-----END CERTIFICATE-----",
				},
			},
			wantErr:    false,
			wantCAData: []byte("-----BEGIN CERTIFICATE-----\ntest-ca-data\n-----END CERTIFICATE-----"),
		},
		{
			name: "ConfigMap missing ca.crt key",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-ca-public",
					Namespace: "org-test",
				},
				Data: map[string]string{
					"other-key": "some-data",
				},
			},
			wantErr: true,
		},
		{
			name: "ConfigMap with empty ca.crt",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-ca-public",
					Namespace: "org-test",
				},
				Data: map[string]string{
					"ca.crt": "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewClientset(tt.configMap)

			manager, err := NewManager(&StaticClientProvider{
				Clientset: clientset,
			})
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}

			info := &ClusterInfo{
				Name:      "test-cluster",
				Namespace: "org-test",
			}

			user := &UserInfo{Email: "test@example.com"}

			caData, err := manager.getCAFromConfigMap(context.Background(), info, clientset, user)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if string(caData) != string(tt.wantCAData) {
					t.Errorf("expected CA data %q, got %q", string(tt.wantCAData), string(caData))
				}
			}
		})
	}
}

func TestManager_WithWorkloadClusterAuthMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     WorkloadClusterAuthMode
		wantMode WorkloadClusterAuthMode
	}{
		{
			name:     "impersonation mode",
			mode:     WorkloadClusterAuthModeImpersonation,
			wantMode: WorkloadClusterAuthModeImpersonation,
		},
		{
			name:     "sso-passthrough mode",
			mode:     WorkloadClusterAuthModeSSOPassthrough,
			wantMode: WorkloadClusterAuthModeSSOPassthrough,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(
				&StaticClientProvider{},
				WithWorkloadClusterAuthMode(tt.mode),
			)
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}

			if manager.workloadClusterAuthMode != tt.wantMode {
				t.Errorf("expected mode %q, got %q", tt.wantMode, manager.workloadClusterAuthMode)
			}
		})
	}
}

func TestManager_WithSSOPassthroughConfig(t *testing.T) {
	tokenExtractor := func(ctx context.Context) (string, bool) {
		return testToken, true
	}

	config := &SSOPassthroughConfig{
		CAConfigMapSuffix: "-custom-ca-public",
		TokenExtractor:    tokenExtractor,
	}

	manager, err := NewManager(
		&StaticClientProvider{},
		WithSSOPassthroughConfig(config),
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if manager.ssoPassthroughConfig == nil {
		t.Fatal("expected ssoPassthroughConfig to be set")
	}

	if manager.ssoPassthroughConfig.CAConfigMapSuffix != "-custom-ca-public" {
		t.Errorf("expected CA ConfigMap suffix '-custom-ca-public', got %q", manager.ssoPassthroughConfig.CAConfigMapSuffix)
	}

	if manager.ssoPassthroughConfig.TokenExtractor == nil {
		t.Error("expected TokenExtractor to be set")
	}

	// Test that the token extractor works
	token, ok := manager.ssoPassthroughConfig.TokenExtractor(context.Background())
	if !ok || token != testToken {
		t.Errorf("expected token %q, got %q (ok=%v)", testToken, token, ok)
	}
}

func TestManager_CreateSSOPassthroughClient_Errors(t *testing.T) {
	user := &UserInfo{Email: "test@example.com"}

	t.Run("no SSO passthrough config", func(t *testing.T) {
		manager, err := NewManager(&StaticClientProvider{})
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		_, _, _, err = manager.CreateSSOPassthroughClient(context.Background(), "test-cluster", user)
		if err == nil {
			t.Error("expected error when SSO passthrough not configured")
		}
	})

	t.Run("no token extractor", func(t *testing.T) {
		manager, err := NewManager(
			&StaticClientProvider{},
			WithSSOPassthroughConfig(&SSOPassthroughConfig{
				CAConfigMapSuffix: "-ca-public",
				TokenExtractor:    nil, // No token extractor
			}),
		)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		_, _, _, err = manager.CreateSSOPassthroughClient(context.Background(), "test-cluster", user)
		if err == nil {
			t.Error("expected error when token extractor not configured")
		}
	})

	t.Run("token not available", func(t *testing.T) {
		manager, err := NewManager(
			&StaticClientProvider{},
			WithSSOPassthroughConfig(&SSOPassthroughConfig{
				CAConfigMapSuffix: "-ca-public",
				TokenExtractor: func(ctx context.Context) (string, bool) {
					return "", false // No token available
				},
			}),
		)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		_, _, _, err = manager.CreateSSOPassthroughClient(context.Background(), "test-cluster", user)
		if err == nil {
			t.Error("expected error when token not available")
		}
	})
}

func TestDefaultCAConfigMapSuffix(t *testing.T) {
	if DefaultCAConfigMapSuffix != "-ca-public" {
		t.Errorf("expected DefaultCAConfigMapSuffix to be '-ca-public', got %q", DefaultCAConfigMapSuffix)
	}
}

func TestCAConfigMapKey(t *testing.T) {
	if CAConfigMapKey != "ca.crt" {
		t.Errorf("expected CAConfigMapKey to be 'ca.crt', got %q", CAConfigMapKey)
	}
}

func TestSSOPassthroughDefaults(t *testing.T) {
	// Verify default QPS and Burst values are reasonable
	if DefaultSSOPassthroughQPS != 50 {
		t.Errorf("expected DefaultSSOPassthroughQPS to be 50, got %v", DefaultSSOPassthroughQPS)
	}
	if DefaultSSOPassthroughBurst != 100 {
		t.Errorf("expected DefaultSSOPassthroughBurst to be 100, got %d", DefaultSSOPassthroughBurst)
	}
}

// TestGetCAForCluster_CredentialModels tests the three deployment credential
// configurations for CA certificate retrieval via SSO passthrough:
//
//  1. No privileged access (StaticClientProvider) - user RBAC for both discovery and ConfigMap
//  2. Privileged access + privileged CAPI discovery - ServiceAccount for discovery, user for ConfigMap
//  3. Privileged access + NO privileged CAPI discovery - user RBAC for both
//  4. Strict mode - rejects fallback on runtime failure
func TestGetCAForCluster_CredentialModels(t *testing.T) {
	const (
		clusterName = "wc-cluster"
		namespace   = "org-acme"
		host        = "api.wc-cluster.example.com"
		port        = int64(6443)
	)

	// --- Scenario 1: No privileged access at all ---
	// No WithPrivilegedAccess option → CredentialModeUser
	// => User RBAC is used for both CAPI discovery AND ConfigMap access

	t.Run("no privileged access: user RBAC for discovery and ConfigMap", func(t *testing.T) {
		cluster := createTestCAPIClusterWithEndpoint(clusterName, namespace, host, port)
		caConfigMap := createTestCAConfigMap(clusterName, namespace, DefaultCAConfigMapSuffix)

		// Both discovery and ConfigMap data are accessible via the user's client
		userDynamic := createTestFakeDynamicClient(runtime.NewScheme(), cluster)
		userClientset := fake.NewClientset(caConfigMap)

		clientProvider := &StaticClientProvider{
			Clientset:     userClientset,
			DynamicClient: userDynamic,
		}

		manager, err := NewManager(clientProvider, WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		// Verify the credential mode was resolved correctly
		assert.Equal(t, CredentialModeUser, manager.credentialMode)

		caData, endpoint, err := manager.GetCAForCluster(context.Background(), clusterName, testUser())

		require.NoError(t, err)
		assert.NotEmpty(t, caData)
		assert.Contains(t, string(caData), "BEGIN CERTIFICATE")
		assert.Equal(t, "https://api.wc-cluster.example.com:6443", endpoint)
	})

	t.Run("no privileged access: fails when user lacks CAPI list RBAC", func(t *testing.T) {
		// User cannot list CAPI clusters and no privileged fallback exists
		userDynamic := createTestFakeDynamicClient(runtime.NewScheme())
		userDynamic.PrependReactor("list", "clusters", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("clusters.cluster.x-k8s.io is forbidden")
		})

		clientProvider := &StaticClientProvider{
			Clientset:     fake.NewClientset(),
			DynamicClient: userDynamic,
		}

		manager, err := NewManager(clientProvider, WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		_, _, err = manager.GetCAForCluster(context.Background(), clusterName, testUser())

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClusterNotFound))
	})

	// --- Scenario 2: Full privileged access ---
	// WithPrivilegedAccess + PrivilegedCAPIDiscovery: true → CredentialModeFullPrivileged
	// => ServiceAccount for CAPI discovery, user credentials for ConfigMap access

	t.Run("full privileged: ServiceAccount for discovery, user for ConfigMap", func(t *testing.T) {
		cluster := createTestCAPIClusterWithEndpoint(clusterName, namespace, host, port)
		caConfigMap := createTestCAConfigMap(clusterName, namespace, DefaultCAConfigMapSuffix)

		// Privileged dynamic client has the CAPI clusters
		privDynamic := createTestFakeDynamicClient(runtime.NewScheme(), cluster)

		// User clients have the ConfigMap (CA cert is public, user credentials for ConfigMap)
		// but NO CAPI cluster data (proving privileged path is used for discovery)
		userDynamic := createTestFakeDynamicClient(runtime.NewScheme())
		userDynamic.PrependReactor("list", "clusters", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("clusters.cluster.x-k8s.io is forbidden")
		})

		provider := &mockPrivilegedStaticProvider{
			userClientset:           fake.NewClientset(caConfigMap),
			userDynamicClient:       userDynamic,
			privilegedClientset:     fake.NewClientset(),
			privilegedDynamicClient: privDynamic,
			privilegedCAPIDiscovery: true,
		}

		manager, err := NewManager(provider, WithPrivilegedAccess(provider), WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		// Verify the credential mode was resolved correctly
		assert.Equal(t, CredentialModeFullPrivileged, manager.credentialMode)

		caData, endpoint, err := manager.GetCAForCluster(context.Background(), clusterName, testUser())

		require.NoError(t, err)
		assert.NotEmpty(t, caData)
		assert.Contains(t, string(caData), "BEGIN CERTIFICATE")
		assert.Equal(t, "https://api.wc-cluster.example.com:6443", endpoint)

		// Verify privileged dynamic was used for CAPI discovery
		assert.True(t, provider.privilegedDynamicCalls > 0,
			"privileged dynamic client should be used for CAPI discovery")
		// Verify user credentials were used for ConfigMap access
		assert.True(t, provider.userClientsForUserCalls > 0,
			"user clients should be called for ConfigMap access (CA certs are public)")
		// Verify privileged clientset was NOT used for ConfigMap (no secret access needed)
		assert.False(t, provider.privilegedSecretsCalls > 0,
			"privileged secret access should not be used for CA ConfigMap retrieval")
	})

	// --- Scenario 3: Privileged secrets only ---
	// WithPrivilegedAccess + PrivilegedCAPIDiscovery: false → CredentialModePrivilegedSecrets
	// => User RBAC for CAPI discovery, user credentials for ConfigMap access

	t.Run("privileged secrets only: user RBAC for discovery and ConfigMap", func(t *testing.T) {
		cluster := createTestCAPIClusterWithEndpoint(clusterName, namespace, host, port)
		caConfigMap := createTestCAConfigMap(clusterName, namespace, DefaultCAConfigMapSuffix)

		// User dynamic client has the CAPI clusters (user has RBAC to list them)
		userDynamic := createTestFakeDynamicClient(runtime.NewScheme(), cluster)

		provider := &mockPrivilegedStaticProvider{
			userClientset:           fake.NewClientset(caConfigMap),
			userDynamicClient:       userDynamic,
			privilegedClientset:     fake.NewClientset(),
			privilegedDynamicClient: nil, // Not used in this mode
			privilegedCAPIDiscovery: false,
		}

		manager, err := NewManager(provider, WithPrivilegedAccess(provider), WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		// Verify the credential mode was resolved correctly
		assert.Equal(t, CredentialModePrivilegedSecrets, manager.credentialMode)

		caData, endpoint, err := manager.GetCAForCluster(context.Background(), clusterName, testUser())

		require.NoError(t, err)
		assert.NotEmpty(t, caData)
		assert.Contains(t, string(caData), "BEGIN CERTIFICATE")
		assert.Equal(t, "https://api.wc-cluster.example.com:6443", endpoint)

		// Verify correct client selection:
		// - Privileged dynamic was NOT called (mode is CredentialModePrivilegedSecrets,
		//   so discovery goes directly to user credentials)
		assert.False(t, provider.privilegedDynamicCalls > 0,
			"privileged dynamic client should not be called in CredentialModePrivilegedSecrets")
		// - User credentials were used for both CAPI discovery and ConfigMap access
		assert.True(t, provider.userClientsForUserCalls > 0,
			"user clients should be called for CAPI discovery and ConfigMap access")
	})

	// --- Scenario 4: Strict mode rejects fallback on runtime failure ---

	t.Run("strict mode: CAPI discovery failure returns error instead of fallback", func(t *testing.T) {
		provider := &mockPrivilegedStaticProvider{
			userClientset:           fake.NewClientset(),
			userDynamicClient:       createTestFakeDynamicClient(runtime.NewScheme()),
			privilegedDynamicErr:    errors.New("ServiceAccount client init failed"),
			privilegedCAPIDiscovery: true,
			strictMode:              true,
		}

		manager, err := NewManager(provider, WithPrivilegedAccess(provider), WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		assert.Equal(t, CredentialModeFullPrivileged, manager.credentialMode)

		_, _, err = manager.GetCAForCluster(context.Background(), clusterName, testUser())

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStrictPrivilegedAccessRequired),
			"strict mode should prevent fallback to user credentials")
		// User credentials should NOT have been called
		assert.False(t, provider.userClientsForUserCalls > 0,
			"user clients must not be called when strict mode rejects fallback")
	})

	t.Run("full privileged: runtime fallback when ServiceAccount fails (non-strict)", func(t *testing.T) {
		cluster := createTestCAPIClusterWithEndpoint(clusterName, namespace, host, port)
		caConfigMap := createTestCAConfigMap(clusterName, namespace, DefaultCAConfigMapSuffix)

		// Privileged dynamic client fails at runtime
		// User clients have both CAPI data and ConfigMap as fallback
		userDynamic := createTestFakeDynamicClient(runtime.NewScheme(), cluster)

		provider := &mockPrivilegedStaticProvider{
			userClientset:           fake.NewClientset(caConfigMap),
			userDynamicClient:       userDynamic,
			privilegedDynamicErr:    errors.New("ServiceAccount client init failed"),
			privilegedCAPIDiscovery: true,
			strictMode:              false, // non-strict: allows fallback
		}

		manager, err := NewManager(provider, WithPrivilegedAccess(provider), WithManagerLogger(newTestLogger()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Close() })

		caData, endpoint, err := manager.GetCAForCluster(context.Background(), clusterName, testUser())

		require.NoError(t, err)
		assert.NotEmpty(t, caData)
		assert.Equal(t, "https://api.wc-cluster.example.com:6443", endpoint)

		// Privileged dynamic was called but failed
		assert.True(t, provider.privilegedDynamicCalls > 0,
			"privileged dynamic client should have been attempted")
		// Fallback to user credentials should have been used
		assert.True(t, provider.userClientsForUserCalls > 0,
			"user clients should be called as fallback when privileged access fails")
	})
}

// TestGetCAForCluster_Validation tests that GetCAForCluster validates inputs.
func TestGetCAForCluster_Validation(t *testing.T) {
	t.Run("returns error when user is nil", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)

		_, _, err := manager.GetCAForCluster(context.Background(), "test-cluster", nil)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUserInfoRequired),
			"expected ErrUserInfoRequired, got %v", err)
	})

	t.Run("returns error when cluster name is invalid", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)
		user := testUser()

		_, _, err := manager.GetCAForCluster(context.Background(), "../secret-cluster", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClusterName),
			"expected ErrInvalidClusterName for path traversal, got %v", err)
	})

	t.Run("returns error when cluster name is empty", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)
		user := testUser()

		_, _, err := manager.GetCAForCluster(context.Background(), "", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClusterName),
			"expected ErrInvalidClusterName for empty name, got %v", err)
	})

	t.Run("returns error when context is cancelled", func(t *testing.T) {
		manager := setupTestManager(t, nil, nil)
		user := testUser()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := manager.GetCAForCluster(ctx, "test-cluster", user)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClusterNotFound))
	})
}
