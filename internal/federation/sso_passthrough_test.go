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

	if config.CASecretSuffix != DefaultCASecretSuffix {
		t.Errorf("expected CA secret suffix %q, got %q", DefaultCASecretSuffix, config.CASecretSuffix)
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

func TestUnstructuredHelpers(t *testing.T) {
	t.Run("unstructuredNestedMap", func(t *testing.T) {
		obj := map[string]interface{}{
			"spec": map[string]interface{}{
				"controlPlaneEndpoint": map[string]interface{}{
					"host": "api.example.com",
					"port": float64(6443),
				},
			},
		}

		// Test successful extraction
		spec, found, err := unstructuredNestedMap(obj, "spec")
		if err != nil || !found {
			t.Errorf("expected to find spec, got found=%v, err=%v", found, err)
		}
		if spec == nil {
			t.Error("expected spec to be non-nil")
		}

		// Test nested extraction
		cpEndpoint, found, err := unstructuredNestedMap(spec, "controlPlaneEndpoint")
		if err != nil || !found {
			t.Errorf("expected to find controlPlaneEndpoint, got found=%v, err=%v", found, err)
		}
		if cpEndpoint == nil {
			t.Error("expected controlPlaneEndpoint to be non-nil")
		}

		// Test non-existent key
		_, found, err = unstructuredNestedMap(obj, "nonexistent")
		if found || err != nil {
			t.Errorf("expected not found for nonexistent key, got found=%v, err=%v", found, err)
		}
	})

	t.Run("unstructuredNestedString", func(t *testing.T) {
		obj := map[string]interface{}{
			"host": "api.example.com",
		}

		// Test successful extraction
		host, found, err := unstructuredNestedString(obj, "host")
		if err != nil || !found {
			t.Errorf("expected to find host, got found=%v, err=%v", found, err)
		}
		if host != "api.example.com" {
			t.Errorf("expected host 'api.example.com', got %q", host)
		}

		// Test non-existent key
		_, found, err = unstructuredNestedString(obj, "nonexistent")
		if found || err != nil {
			t.Errorf("expected not found for nonexistent key, got found=%v, err=%v", found, err)
		}
	})

	t.Run("unstructuredNestedInt64", func(t *testing.T) {
		obj := map[string]interface{}{
			"port": float64(6443), // JSON numbers are parsed as float64
		}

		// Test successful extraction
		port, found, err := unstructuredNestedInt64(obj, "port")
		if err != nil || !found {
			t.Errorf("expected to find port, got found=%v, err=%v", found, err)
		}
		if port != 6443 {
			t.Errorf("expected port 6443, got %d", port)
		}

		// Test with int type
		obj["portInt"] = 8080
		port, found, err = unstructuredNestedInt64(obj, "portInt")
		if err != nil || !found {
			t.Errorf("expected to find portInt, got found=%v, err=%v", found, err)
		}
		if port != 8080 {
			t.Errorf("expected portInt 8080, got %d", port)
		}
	})
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

func TestManager_GetCAFromSecret(t *testing.T) {
	tests := []struct {
		name       string
		secret     *corev1.Secret
		wantErr    bool
		wantCAData []byte
	}{
		{
			name: "valid CA secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-ca",
					Namespace: "org-test",
				},
				Data: map[string][]byte{
					"ca.crt": []byte("-----BEGIN CERTIFICATE-----\ntest-ca-data\n-----END CERTIFICATE-----"),
				},
			},
			wantErr:    false,
			wantCAData: []byte("-----BEGIN CERTIFICATE-----\ntest-ca-data\n-----END CERTIFICATE-----"),
		},
		{
			name: "secret missing ca.crt key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-ca",
					Namespace: "org-test",
				},
				Data: map[string][]byte{
					"other-key": []byte("some-data"),
				},
			},
			wantErr: true,
		},
		{
			name: "secret with empty ca.crt",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-ca",
					Namespace: "org-test",
				},
				Data: map[string][]byte{
					"ca.crt": []byte{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewClientset(tt.secret)

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

			caData, err := manager.getCAFromSecret(context.Background(), info, clientset, user)

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
		CASecretSuffix: "-custom-ca",
		TokenExtractor: tokenExtractor,
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

	if manager.ssoPassthroughConfig.CASecretSuffix != "-custom-ca" {
		t.Errorf("expected CA secret suffix '-custom-ca', got %q", manager.ssoPassthroughConfig.CASecretSuffix)
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
				CASecretSuffix: "-ca",
				TokenExtractor: nil, // No token extractor
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
				CASecretSuffix: "-ca",
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

func TestDefaultCASecretSuffix(t *testing.T) {
	if DefaultCASecretSuffix != "-ca" {
		t.Errorf("expected DefaultCASecretSuffix to be '-ca', got %q", DefaultCASecretSuffix)
	}
}

func TestCASecretKey(t *testing.T) {
	if CASecretKey != "ca.crt" {
		t.Errorf("expected CASecretKey to be 'ca.crt', got %q", CASecretKey)
	}
}
