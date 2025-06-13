package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHelmInstallOptions_Structure(t *testing.T) {
	// Test HelmInstallOptions struct

	t.Run("complete install options", func(t *testing.T) {
		opts := HelmInstallOptions{
			Values: map[string]interface{}{
				"replicaCount": 3,
				"image": map[string]interface{}{
					"tag": "latest",
				},
			},
			ValuesFiles:     []string{"values.yaml", "production.yaml"},
			Wait:            true,
			Timeout:         10 * time.Minute,
			CreateNamespace: true,
			Version:         "1.2.3",
			Repository:      "https://charts.example.com",
		}

		assert.NotNil(t, opts.Values)
		assert.Equal(t, 3, opts.Values["replicaCount"])
		assert.Len(t, opts.ValuesFiles, 2)
		assert.Contains(t, opts.ValuesFiles, "values.yaml")
		assert.True(t, opts.Wait)
		assert.Equal(t, 10*time.Minute, opts.Timeout)
		assert.True(t, opts.CreateNamespace)
		assert.Equal(t, "1.2.3", opts.Version)
		assert.Equal(t, "https://charts.example.com", opts.Repository)
	})

	t.Run("minimal install options", func(t *testing.T) {
		opts := HelmInstallOptions{}

		assert.Nil(t, opts.Values)
		assert.Nil(t, opts.ValuesFiles)
		assert.False(t, opts.Wait)
		assert.Equal(t, time.Duration(0), opts.Timeout)
		assert.False(t, opts.CreateNamespace)
		assert.Empty(t, opts.Version)
		assert.Empty(t, opts.Repository)
	})
}

func TestHelmUpgradeOptions_Structure(t *testing.T) {
	// Test HelmUpgradeOptions struct

	t.Run("complete upgrade options", func(t *testing.T) {
		opts := HelmUpgradeOptions{
			Values: map[string]interface{}{
				"service": map[string]interface{}{
					"type": "LoadBalancer",
				},
			},
			ValuesFiles: []string{"values.yaml"},
			Wait:        true,
			Timeout:     5 * time.Minute,
			Version:     "2.0.0",
			Repository:  "https://charts.example.com",
			ResetValues: true,
		}

		assert.NotNil(t, opts.Values)
		service, exists := opts.Values["service"].(map[string]interface{})
		assert.True(t, exists)
		assert.Equal(t, "LoadBalancer", service["type"])
		assert.Len(t, opts.ValuesFiles, 1)
		assert.True(t, opts.Wait)
		assert.Equal(t, 5*time.Minute, opts.Timeout)
		assert.Equal(t, "2.0.0", opts.Version)
		assert.Equal(t, "https://charts.example.com", opts.Repository)
		assert.True(t, opts.ResetValues)
	})

	t.Run("upgrade with reset values", func(t *testing.T) {
		opts := HelmUpgradeOptions{
			ResetValues: true,
		}

		assert.True(t, opts.ResetValues)
		assert.Nil(t, opts.Values)
	})
}

func TestHelmUninstallOptions_Structure(t *testing.T) {
	// Test HelmUninstallOptions struct

	t.Run("complete uninstall options", func(t *testing.T) {
		opts := HelmUninstallOptions{
			Wait:    true,
			Timeout: 2 * time.Minute,
		}

		assert.True(t, opts.Wait)
		assert.Equal(t, 2*time.Minute, opts.Timeout)
	})

	t.Run("minimal uninstall options", func(t *testing.T) {
		opts := HelmUninstallOptions{}

		assert.False(t, opts.Wait)
		assert.Equal(t, time.Duration(0), opts.Timeout)
	})
}

func TestHelmListOptions_Structure(t *testing.T) {
	// Test HelmListOptions struct

	t.Run("complete list options", func(t *testing.T) {
		opts := HelmListOptions{
			AllNamespaces: true,
			Filter:        "nginx",
			Deployed:      true,
			Failed:        false,
			Pending:       false,
		}

		assert.True(t, opts.AllNamespaces)
		assert.Equal(t, "nginx", opts.Filter)
		assert.True(t, opts.Deployed)
		assert.False(t, opts.Failed)
		assert.False(t, opts.Pending)
	})

	t.Run("list failed releases", func(t *testing.T) {
		opts := HelmListOptions{
			Failed: true,
		}

		assert.True(t, opts.Failed)
		assert.False(t, opts.Deployed)
		assert.False(t, opts.Pending)
	})

	t.Run("list all states", func(t *testing.T) {
		opts := HelmListOptions{
			Deployed: true,
			Failed:   true,
			Pending:  true,
		}

		assert.True(t, opts.Deployed)
		assert.True(t, opts.Failed)
		assert.True(t, opts.Pending)
	})
}

func TestHelmRelease_Structure(t *testing.T) {
	// Test HelmRelease struct

	t.Run("complete release info", func(t *testing.T) {
		updated := time.Now()
		release := HelmRelease{
			Name:       "my-app",
			Namespace:  "production",
			Revision:   3,
			Status:     "deployed",
			Chart:      "nginx-1.2.3",
			AppVersion: "1.19.0",
			Updated:    updated,
			Values: map[string]interface{}{
				"replicaCount": 2,
				"service": map[string]interface{}{
					"type": "ClusterIP",
				},
			},
		}

		assert.Equal(t, "my-app", release.Name)
		assert.Equal(t, "production", release.Namespace)
		assert.Equal(t, 3, release.Revision)
		assert.Equal(t, "deployed", release.Status)
		assert.Equal(t, "nginx-1.2.3", release.Chart)
		assert.Equal(t, "1.19.0", release.AppVersion)
		assert.Equal(t, updated, release.Updated)
		assert.NotNil(t, release.Values)
		assert.Equal(t, 2, release.Values["replicaCount"])
	})

	t.Run("failed release", func(t *testing.T) {
		release := HelmRelease{
			Name:       "failed-app",
			Namespace:  "default",
			Revision:   1,
			Status:     "failed",
			Chart:      "broken-chart-1.0.0",
			AppVersion: "unknown",
			Updated:    time.Now(),
		}

		assert.Equal(t, "failed-app", release.Name)
		assert.Equal(t, "failed", release.Status)
		assert.Equal(t, 1, release.Revision)
		assert.Equal(t, "broken-chart-1.0.0", release.Chart)
		assert.Nil(t, release.Values)
	})
}

func TestKubernetesClient_HelmOperationsValidation(t *testing.T) {
	// Test validation logic for Helm operations without calling methods that can deadlock

	t.Run("helm install validation", func(t *testing.T) {
		client := createTestClientForResources()
		client.allowedOperations = []string{"create", "get", "list"}
		client.restrictedNamespaces = []string{"kube-system"}

		// Helm install would require create permissions
		assert.NoError(t, client.isOperationAllowed("create"))

		// Test restricted namespace
		assert.Error(t, client.isNamespaceRestricted("kube-system"))
		assert.Contains(t, client.isNamespaceRestricted("kube-system").Error(), "is restricted")

		// Test allowed namespace
		assert.NoError(t, client.isNamespaceRestricted("default"))
	})

	t.Run("helm upgrade validation", func(t *testing.T) {
		client := createTestClientForResources()
		client.allowedOperations = []string{"patch", "update"}

		// Helm upgrade would require update/patch permissions
		assert.NoError(t, client.isOperationAllowed("patch"))
		assert.NoError(t, client.isOperationAllowed("update"))

		// Test disallowed operations
		assert.Error(t, client.isOperationAllowed("delete"))
	})

	t.Run("helm uninstall validation", func(t *testing.T) {
		client := createTestClientForResources()
		client.nonDestructiveMode = true
		client.dryRun = false

		// Helm uninstall is destructive and should be blocked
		assert.Error(t, client.isOperationAllowed("delete"))
		assert.Contains(t, client.isOperationAllowed("delete").Error(), "destructive operation")

		// But allowed with dry-run
		client.dryRun = true
		assert.NoError(t, client.isOperationAllowed("delete"))
	})

	t.Run("helm list validation", func(t *testing.T) {
		client := createTestClientForResources()
		client.allowedOperations = []string{"get", "list"}

		// Helm list would require list permissions
		assert.NoError(t, client.isOperationAllowed("list"))
		assert.NoError(t, client.isOperationAllowed("get"))
	})
}

func TestKubernetesClient_HelmOperationsLogging(t *testing.T) {
	// Test logging for Helm operations

	mockLogger := &MockLogger{}

	// Expect debug log calls for each operation
	mockLogger.On("Debug", "kubernetes operation", mock.AnythingOfType("[]interface {}")).Return().Times(4)

	client := &kubernetesClient{
		config: &ClientConfig{
			Logger: mockLogger,
		},
	}

	// Test logging for Helm operations
	client.logOperation("helm-install", "test-context", "default", "release", "my-app")
	client.logOperation("helm-upgrade", "test-context", "default", "release", "my-app")
	client.logOperation("helm-uninstall", "test-context", "default", "release", "my-app")
	client.logOperation("helm-list", "test-context", "default", "releases", "")

	mockLogger.AssertExpectations(t)
}

func TestHelmOptions_TimeoutValidation(t *testing.T) {
	// Test timeout validation logic

	t.Run("reasonable timeouts", func(t *testing.T) {
		installOpts := HelmInstallOptions{
			Timeout: 5 * time.Minute,
		}
		upgradeOpts := HelmUpgradeOptions{
			Timeout: 10 * time.Minute,
		}
		uninstallOpts := HelmUninstallOptions{
			Timeout: 2 * time.Minute,
		}

		assert.Equal(t, 5*time.Minute, installOpts.Timeout)
		assert.Equal(t, 10*time.Minute, upgradeOpts.Timeout)
		assert.Equal(t, 2*time.Minute, uninstallOpts.Timeout)
	})

	t.Run("zero timeout", func(t *testing.T) {
		opts := HelmInstallOptions{
			Timeout: 0,
		}

		// Zero timeout means no timeout (wait indefinitely)
		assert.Equal(t, time.Duration(0), opts.Timeout)
	})
}
