package federation

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// createFakeDynamicClient creates a fake dynamic client with CAPI cluster GVR registered.
// This is required because the dynamic fake client needs explicit registration of list kinds.
func createFakeDynamicClient(scheme *runtime.Scheme, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	gvrToListKind := map[schema.GroupVersionResource]string{
		CAPIClusterGVR: "ClusterList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)
}

func TestNewManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	tests := []struct {
		name          string
		localClient   kubernetes.Interface
		localDynamic  dynamic.Interface
		opts          []ManagerOption
		expectedError string
	}{
		{
			name:          "valid inputs",
			localClient:   fakeClient,
			localDynamic:  fakeDynamic,
			opts:          []ManagerOption{WithManagerLogger(logger)},
			expectedError: "",
		},
		{
			name:          "no logger uses default",
			localClient:   fakeClient,
			localDynamic:  fakeDynamic,
			opts:          nil,
			expectedError: "",
		},
		{
			name:          "nil local client",
			localClient:   nil,
			localDynamic:  fakeDynamic,
			opts:          []ManagerOption{WithManagerLogger(logger)},
			expectedError: "local client is required",
		},
		{
			name:          "nil dynamic client",
			localClient:   fakeClient,
			localDynamic:  nil,
			opts:          []ManagerOption{WithManagerLogger(logger)},
			expectedError: "local dynamic client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.localClient, tt.localDynamic, nil, tt.opts...)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, manager)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, manager)
				// Clean up
				defer manager.Close()
			}
		})
	}
}

func TestManager_GetClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, kubernetes.Interface)
	}{
		{
			name:          "nil user returns ErrUserInfoRequired",
			clusterName:   "",
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name:        "empty cluster name with valid user returns local client",
			clusterName: "",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"developers"},
			},
			checkResult: func(t *testing.T, client kubernetes.Interface) {
				assert.NotNil(t, client)
			},
		},
		{
			name:        "valid user with empty email returns local client",
			clusterName: "",
			user: &UserInfo{
				Email:  "",
				Groups: []string{"developers"},
			},
			checkResult: func(t *testing.T, client kubernetes.Interface) {
				assert.NotNil(t, client)
			},
		},
		{
			name:        "remote cluster not implemented",
			clusterName: "workload-cluster",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrClusterNotFound,
		},
		{
			name:        "invalid cluster name returns validation error",
			clusterName: "../etc/passwd",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrInvalidClusterName,
		},
		{
			name:        "invalid email returns validation error",
			clusterName: "",
			user: &UserInfo{
				Email: "not-an-email",
			},
			expectedError: ErrInvalidEmail,
		},
		{
			name:        "closed manager returns error",
			clusterName: "",
			user: &UserInfo{
				Email: "user@example.com",
			},
			setupManager: func(m *Manager) {
				m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
			require.NoError(t, err)
			defer manager.Close()

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			client, err := manager.GetClient(context.Background(), tt.clusterName, tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected %v, got %v", tt.expectedError, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, client)
				}
			}
		})
	}
}

func TestManager_GetDynamicClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, dynamic.Interface)
	}{
		{
			name:          "nil user returns ErrUserInfoRequired",
			clusterName:   "",
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name:        "empty cluster name with valid user returns local dynamic client",
			clusterName: "",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"admins"},
			},
			checkResult: func(t *testing.T, client dynamic.Interface) {
				assert.NotNil(t, client)
			},
		},
		{
			name:        "remote cluster not implemented",
			clusterName: "workload-cluster",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrClusterNotFound,
		},
		{
			name:        "invalid cluster name returns validation error",
			clusterName: "My-Cluster",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrInvalidClusterName,
		},
		{
			name:        "closed manager returns error",
			clusterName: "",
			user: &UserInfo{
				Email: "user@example.com",
			},
			setupManager: func(m *Manager) {
				m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
			require.NoError(t, err)
			defer manager.Close()

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			client, err := manager.GetDynamicClient(context.Background(), tt.clusterName, tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected %v, got %v", tt.expectedError, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, client)
				}
			}
		})
	}
}

func TestManager_ListClusters(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	tests := []struct {
		name          string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, []ClusterSummary)
	}{
		{
			name: "returns empty list (not yet implemented)",
			user: &UserInfo{
				Email: "user@example.com",
			},
			checkResult: func(t *testing.T, clusters []ClusterSummary) {
				assert.Empty(t, clusters)
			},
		},
		{
			name:          "nil user returns ErrUserInfoRequired",
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name: "closed manager returns error",
			user: &UserInfo{Email: "user@example.com"},
			setupManager: func(m *Manager) {
				m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
			require.NoError(t, err)
			defer manager.Close()

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			clusters, err := manager.ListClusters(context.Background(), tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected %v, got %v", tt.expectedError, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, clusters)
				}
			}
		})
	}
}

func TestManager_GetClusterSummary(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
	}{
		{
			name:          "nil user returns ErrUserInfoRequired",
			clusterName:   "my-cluster",
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name:        "empty cluster name returns validation error",
			clusterName: "",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrInvalidClusterName,
		},
		{
			name:        "invalid cluster name returns validation error",
			clusterName: "INVALID_CLUSTER",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrInvalidClusterName,
		},
		{
			name:        "cluster not found (not yet implemented)",
			clusterName: "my-cluster",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrClusterNotFound,
		},
		{
			name:        "closed manager returns error",
			clusterName: "my-cluster",
			user:        &UserInfo{Email: "user@example.com"},
			setupManager: func(m *Manager) {
				m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
			require.NoError(t, err)
			defer manager.Close()

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			summary, err := manager.GetClusterSummary(context.Background(), tt.clusterName, tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected %v, got %v", tt.expectedError, err)
				assert.Nil(t, summary)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, summary)
			}
		})
	}
}

func TestManager_Close(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	t.Run("close succeeds", func(t *testing.T) {
		manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
		require.NoError(t, err)

		err = manager.Close()
		assert.NoError(t, err)
	})

	t.Run("close is idempotent", func(t *testing.T) {
		manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
		require.NoError(t, err)

		err = manager.Close()
		assert.NoError(t, err)

		err = manager.Close()
		assert.NoError(t, err)
	})

	t.Run("methods fail after close", func(t *testing.T) {
		manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
		require.NoError(t, err)

		err = manager.Close()
		require.NoError(t, err)

		user := &UserInfo{Email: "user@example.com"}

		_, err = manager.GetClient(context.Background(), "", user)
		assert.True(t, errors.Is(err, ErrManagerClosed))

		_, err = manager.GetDynamicClient(context.Background(), "", user)
		assert.True(t, errors.Is(err, ErrManagerClosed))

		_, err = manager.ListClusters(context.Background(), user)
		assert.True(t, errors.Is(err, ErrManagerClosed))

		_, err = manager.GetClusterSummary(context.Background(), "test", user)
		assert.True(t, errors.Is(err, ErrManagerClosed))
	})
}

func TestManager_Concurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)

	manager, err := NewManager(fakeClient, fakeDynamic, nil, WithManagerLogger(logger))
	require.NoError(t, err)
	defer manager.Close()

	// Test concurrent access
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			user := &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"group1"},
			}

			// Call various methods concurrently
			_, _ = manager.GetClient(context.Background(), "", user)
			_, _ = manager.GetDynamicClient(context.Background(), "", user)
			_, _ = manager.ListClusters(context.Background(), user)
			_, _ = manager.GetClusterSummary(context.Background(), "test-cluster", user)
		}(i)
	}

	wg.Wait()

	// Manager should still be usable with valid user
	user := &UserInfo{Email: "user@example.com"}
	client, err := manager.GetClient(context.Background(), "", user)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestManager_Interface(t *testing.T) {
	// Verify that Manager implements ClusterClientManager
	var _ ClusterClientManager = (*Manager)(nil)
}

func TestManager_OptionsComposition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := createFakeDynamicClient(scheme)
	metrics := newMockMetricsRecorder()

	// Test that WithManagerCacheConfig and WithManagerCacheMetrics can be combined
	t.Run("cache config and metrics can be combined", func(t *testing.T) {
		config := CacheConfig{
			TTL:             5 * time.Minute,
			MaxEntries:      500,
			CleanupInterval: 30 * time.Second,
		}

		manager, err := NewManager(fakeClient, fakeDynamic, nil,
			WithManagerLogger(logger),
			WithManagerCacheConfig(config),
			WithManagerCacheMetrics(metrics),
		)
		require.NoError(t, err)
		defer manager.Close()

		// Verify cache was created with the custom config
		assert.Equal(t, 5*time.Minute, manager.cache.config.TTL)
		assert.Equal(t, 500, manager.cache.config.MaxEntries)
		assert.Equal(t, 30*time.Second, manager.cache.config.CleanupInterval)

		// Verify metrics recorder was set
		ctx := context.Background()
		user := &UserInfo{Email: "user@example.com"}
		_, err = manager.GetClient(ctx, "", user)
		require.NoError(t, err)

		// Should have recorded cache metrics
		assert.True(t, metrics.getMisses() > 0 || metrics.getHits() > 0, "expected cache metrics to be recorded")
	})

	// Test that order doesn't matter
	t.Run("option order does not matter", func(t *testing.T) {
		metrics2 := newMockMetricsRecorder()
		config := CacheConfig{
			TTL:        3 * time.Minute,
			MaxEntries: 100,
		}

		// Apply metrics before config
		manager, err := NewManager(fakeClient, fakeDynamic, nil,
			WithManagerCacheMetrics(metrics2),
			WithManagerCacheConfig(config),
			WithManagerLogger(logger),
		)
		require.NoError(t, err)
		defer manager.Close()

		// Both should be applied
		assert.Equal(t, 3*time.Minute, manager.cache.config.TTL)
		assert.Equal(t, 100, manager.cache.config.MaxEntries)
	})
}
