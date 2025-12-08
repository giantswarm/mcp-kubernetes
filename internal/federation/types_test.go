package federation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserInfo(t *testing.T) {
	t.Run("basic user info", func(t *testing.T) {
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"developers", "team-alpha"},
			Extra: map[string][]string{
				"organization": {"acme-corp"},
				"tenant":       {"tenant-123"},
			},
		}

		assert.Equal(t, "user@example.com", user.Email)
		assert.Len(t, user.Groups, 2)
		assert.Contains(t, user.Groups, "developers")
		assert.Contains(t, user.Groups, "team-alpha")
		assert.Len(t, user.Extra, 2)
		assert.Equal(t, []string{"acme-corp"}, user.Extra["organization"])
	})

	t.Run("empty user info", func(t *testing.T) {
		user := &UserInfo{}

		assert.Empty(t, user.Email)
		assert.Nil(t, user.Groups)
		assert.Nil(t, user.Extra)
	})
}

func TestClusterSummary(t *testing.T) {
	t.Run("full cluster summary", func(t *testing.T) {
		createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		summary := ClusterSummary{
			Name:                "production-cluster",
			Namespace:           "org-acme",
			Provider:            "aws",
			Release:             "19.3.0",
			KubernetesVersion:   "1.28.5",
			Status:              "Provisioned",
			Ready:               true,
			ControlPlaneReady:   true,
			InfrastructureReady: true,
			NodeCount:           5,
			CreatedAt:           createdAt,
			Labels: map[string]string{
				"environment": "production",
				"team":        "platform",
			},
			Annotations: map[string]string{
				"description": "Production workload cluster",
			},
		}

		assert.Equal(t, "production-cluster", summary.Name)
		assert.Equal(t, "org-acme", summary.Namespace)
		assert.Equal(t, "aws", summary.Provider)
		assert.Equal(t, "19.3.0", summary.Release)
		assert.Equal(t, "1.28.5", summary.KubernetesVersion)
		assert.Equal(t, "Provisioned", summary.Status)
		assert.True(t, summary.Ready)
		assert.True(t, summary.ControlPlaneReady)
		assert.True(t, summary.InfrastructureReady)
		assert.Equal(t, 5, summary.NodeCount)
		assert.Equal(t, createdAt, summary.CreatedAt)
		assert.Equal(t, "production", summary.Labels["environment"])
		assert.Equal(t, "Production workload cluster", summary.Annotations["description"])
	})

	t.Run("minimal cluster summary", func(t *testing.T) {
		summary := ClusterSummary{
			Name:      "test-cluster",
			Namespace: "org-test",
			Status:    "Provisioning",
		}

		assert.Equal(t, "test-cluster", summary.Name)
		assert.Equal(t, "org-test", summary.Namespace)
		assert.Equal(t, "Provisioning", summary.Status)
		assert.False(t, summary.Ready)
		assert.Empty(t, summary.Provider)
		assert.Zero(t, summary.NodeCount)
	})

	t.Run("JSON serialization", func(t *testing.T) {
		createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		summary := ClusterSummary{
			Name:              "test-cluster",
			Namespace:         "org-test",
			Provider:          "aws",
			Status:            "Provisioned",
			Ready:             true,
			KubernetesVersion: "1.28.5",
			NodeCount:         3,
			CreatedAt:         createdAt,
		}

		data, err := json.Marshal(summary)
		require.NoError(t, err)

		var decoded ClusterSummary
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, summary.Name, decoded.Name)
		assert.Equal(t, summary.Namespace, decoded.Namespace)
		assert.Equal(t, summary.Provider, decoded.Provider)
		assert.Equal(t, summary.Status, decoded.Status)
		assert.Equal(t, summary.Ready, decoded.Ready)
		assert.Equal(t, summary.KubernetesVersion, decoded.KubernetesVersion)
		assert.Equal(t, summary.NodeCount, decoded.NodeCount)
		assert.True(t, summary.CreatedAt.Equal(decoded.CreatedAt))
	})

	t.Run("JSON omitempty", func(t *testing.T) {
		summary := ClusterSummary{
			Name:      "test-cluster",
			Namespace: "org-test",
			Status:    "Provisioned",
		}

		data, err := json.Marshal(summary)
		require.NoError(t, err)

		// Check that empty/zero fields are omitted
		jsonStr := string(data)
		assert.NotContains(t, jsonStr, "provider")
		assert.NotContains(t, jsonStr, "release")
		assert.NotContains(t, jsonStr, "kubernetesVersion")
		assert.NotContains(t, jsonStr, "nodeCount")
		assert.NotContains(t, jsonStr, "labels")
		assert.NotContains(t, jsonStr, "annotations")
	})
}

func TestClusterPhase(t *testing.T) {
	t.Run("cluster phases are distinct", func(t *testing.T) {
		phases := []ClusterPhase{
			ClusterPhasePending,
			ClusterPhaseProvisioning,
			ClusterPhaseProvisioned,
			ClusterPhaseDeleting,
			ClusterPhaseFailed,
			ClusterPhaseUnknown,
		}

		// Verify all phases are distinct
		seen := make(map[ClusterPhase]bool)
		for _, phase := range phases {
			assert.False(t, seen[phase], "duplicate phase: %s", phase)
			seen[phase] = true
		}
	})

	t.Run("cluster phase values", func(t *testing.T) {
		assert.Equal(t, ClusterPhase("Pending"), ClusterPhasePending)
		assert.Equal(t, ClusterPhase("Provisioning"), ClusterPhaseProvisioning)
		assert.Equal(t, ClusterPhase("Provisioned"), ClusterPhaseProvisioned)
		assert.Equal(t, ClusterPhase("Deleting"), ClusterPhaseDeleting)
		assert.Equal(t, ClusterPhase("Failed"), ClusterPhaseFailed)
		assert.Equal(t, ClusterPhase("Unknown"), ClusterPhaseUnknown)
	})
}

func TestConstants(t *testing.T) {
	t.Run("CAPI secret suffix", func(t *testing.T) {
		assert.Equal(t, "-kubeconfig", CAPISecretSuffix)

		// Verify naming convention
		clusterName := "my-cluster"
		expectedSecretName := clusterName + CAPISecretSuffix
		assert.Equal(t, "my-cluster-kubeconfig", expectedSecretName)
	})

	t.Run("CAPI secret key", func(t *testing.T) {
		assert.Equal(t, "value", CAPISecretKey)
	})

	t.Run("impersonation headers", func(t *testing.T) {
		assert.Equal(t, "Impersonate-User", ImpersonateUserHeader)
		assert.Equal(t, "Impersonate-Group", ImpersonateGroupHeader)
		assert.Equal(t, "Impersonate-Extra-", ImpersonateExtraHeaderPrefix)
	})
}
