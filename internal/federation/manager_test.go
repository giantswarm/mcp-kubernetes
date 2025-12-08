package federation

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	tests := []struct {
		name          string
		localClient   kubernetes.Interface
		localDynamic  dynamic.Interface
		logger        *slog.Logger
		expectedError string
	}{
		{
			name:          "valid inputs",
			localClient:   fakeClient,
			localDynamic:  fakeDynamic,
			logger:        logger,
			expectedError: "",
		},
		{
			name:          "nil logger uses default",
			localClient:   fakeClient,
			localDynamic:  fakeDynamic,
			logger:        nil,
			expectedError: "",
		},
		{
			name:          "nil local client",
			localClient:   nil,
			localDynamic:  fakeDynamic,
			logger:        logger,
			expectedError: "local client is required",
		},
		{
			name:          "nil dynamic client",
			localClient:   fakeClient,
			localDynamic:  nil,
			logger:        logger,
			expectedError: "local dynamic client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.localClient, tt.localDynamic, tt.logger)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, manager)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, manager)
			}
		})
	}
}

func TestManager_GetClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, kubernetes.Interface)
	}{
		{
			name:        "empty cluster name returns local client",
			clusterName: "",
			user:        nil,
			checkResult: func(t *testing.T, client kubernetes.Interface) {
				assert.NotNil(t, client)
			},
		},
		{
			name:        "empty cluster name with user returns local client",
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
			name:        "remote cluster not implemented",
			clusterName: "workload-cluster",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrClusterNotFound,
		},
		{
			name:        "closed manager returns error",
			clusterName: "",
			user:        nil,
			setupManager: func(m *Manager) {
				m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(fakeClient, fakeDynamic, logger)
			require.NoError(t, err)

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
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, dynamic.Interface)
	}{
		{
			name:        "empty cluster name returns local dynamic client",
			clusterName: "",
			user:        nil,
			checkResult: func(t *testing.T, client dynamic.Interface) {
				assert.NotNil(t, client)
			},
		},
		{
			name:        "empty cluster name with user returns local dynamic client",
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
			name:        "closed manager returns error",
			clusterName: "",
			user:        nil,
			setupManager: func(m *Manager) {
				m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(fakeClient, fakeDynamic, logger)
			require.NoError(t, err)

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
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

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
			name: "nil user returns empty list",
			user: nil,
			checkResult: func(t *testing.T, clusters []ClusterSummary) {
				assert.Empty(t, clusters)
			},
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
			manager, err := NewManager(fakeClient, fakeDynamic, logger)
			require.NoError(t, err)

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
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
	}{
		{
			name:        "empty cluster name returns error",
			clusterName: "",
			user: &UserInfo{
				Email: "user@example.com",
			},
			expectedError: ErrClusterNotFound,
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
			manager, err := NewManager(fakeClient, fakeDynamic, logger)
			require.NoError(t, err)

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
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	t.Run("close succeeds", func(t *testing.T) {
		manager, err := NewManager(fakeClient, fakeDynamic, logger)
		require.NoError(t, err)

		err = manager.Close()
		assert.NoError(t, err)
	})

	t.Run("close is idempotent", func(t *testing.T) {
		manager, err := NewManager(fakeClient, fakeDynamic, logger)
		require.NoError(t, err)

		err = manager.Close()
		assert.NoError(t, err)

		err = manager.Close()
		assert.NoError(t, err)
	})

	t.Run("methods fail after close", func(t *testing.T) {
		manager, err := NewManager(fakeClient, fakeDynamic, logger)
		require.NoError(t, err)

		err = manager.Close()
		require.NoError(t, err)

		_, err = manager.GetClient(context.Background(), "", nil)
		assert.True(t, errors.Is(err, ErrManagerClosed))

		_, err = manager.GetDynamicClient(context.Background(), "", nil)
		assert.True(t, errors.Is(err, ErrManagerClosed))

		_, err = manager.ListClusters(context.Background(), nil)
		assert.True(t, errors.Is(err, ErrManagerClosed))

		_, err = manager.GetClusterSummary(context.Background(), "test", nil)
		assert.True(t, errors.Is(err, ErrManagerClosed))
	})
}

func TestManager_Concurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fakeClient := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	manager, err := NewManager(fakeClient, fakeDynamic, logger)
	require.NoError(t, err)

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
			_, _ = manager.GetClusterSummary(context.Background(), "test", user)
		}(i)
	}

	wg.Wait()

	// Manager should still be usable
	client, err := manager.GetClient(context.Background(), "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestManager_Interface(t *testing.T) {
	// Verify that Manager implements ClusterClientManager
	var _ ClusterClientManager = (*Manager)(nil)
}
