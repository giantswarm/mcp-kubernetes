package federation

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
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

func TestManager_GetClusterEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		cluster     *unstructured.Unstructured
		wantHost    string
		wantErr     bool
	}{
		{
			name:        "cluster with host and port",
			clusterName: "test-cluster",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta1",
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
			wantHost: "https://api.test-cluster.example.com:6443",
			wantErr:  false,
		},
		{
			name:        "cluster with host only (default port)",
			clusterName: "test-cluster",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta1",
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
			wantHost: "https://api.test-cluster.example.com:6443",
			wantErr:  false,
		},
		{
			name:        "cluster not found",
			clusterName: "nonexistent",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta1",
					"kind":       "Cluster",
					"metadata": map[string]interface{}{
						"name":      "other-cluster",
						"namespace": "org-test",
					},
				},
			},
			wantErr: true,
		},
		{
			name:        "cluster without controlPlaneEndpoint",
			clusterName: "test-cluster",
			cluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.x-k8s.io/v1beta1",
					"kind":       "Cluster",
					"metadata": map[string]interface{}{
						"name":      "test-cluster",
						"namespace": "org-test",
					},
					"spec": map[string]interface{}{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake dynamic client with the cluster
			scheme := runtime.NewScheme()
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				scheme,
				map[schema.GroupVersionResource]string{
					CAPIClusterGVR: "ClusterList",
				},
				tt.cluster,
			)

			manager, err := NewManager(&StaticClientProvider{
				DynamicClient: dynamicClient,
			})
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}

			host, err := manager.getClusterEndpoint(context.Background(), tt.clusterName, dynamicClient)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if host != tt.wantHost {
					t.Errorf("expected host %q, got %q", tt.wantHost, host)
				}
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
