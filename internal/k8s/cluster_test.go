package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAPIResourceInfo_Structure(t *testing.T) {
	// Test APIResourceInfo struct

	t.Run("complete resource info", func(t *testing.T) {
		resource := APIResourceInfo{
			Name:         "pods",
			SingularName: "pod",
			Namespaced:   true,
			Kind:         "Pod",
			Verbs:        []string{"get", "list", "create", "update", "delete"},
			Group:        "",
			Version:      "v1",
		}

		assert.Equal(t, "pods", resource.Name)
		assert.Equal(t, "pod", resource.SingularName)
		assert.True(t, resource.Namespaced)
		assert.Equal(t, "Pod", resource.Kind)
		assert.Len(t, resource.Verbs, 5)
		assert.Contains(t, resource.Verbs, "get")
		assert.Contains(t, resource.Verbs, "delete")
		assert.Empty(t, resource.Group) // Core group
		assert.Equal(t, "v1", resource.Version)
	})

	t.Run("cluster-scoped resource", func(t *testing.T) {
		resource := APIResourceInfo{
			Name:         "nodes",
			SingularName: "node",
			Namespaced:   false,
			Kind:         "Node",
			Verbs:        []string{"get", "list"},
			Group:        "",
			Version:      "v1",
		}

		assert.Equal(t, "nodes", resource.Name)
		assert.False(t, resource.Namespaced)
		assert.Equal(t, "Node", resource.Kind)
	})

	t.Run("custom resource", func(t *testing.T) {
		resource := APIResourceInfo{
			Name:         "customresources",
			SingularName: "customresource",
			Namespaced:   true,
			Kind:         "CustomResource",
			Verbs:        []string{"get", "list", "create", "update", "delete"},
			Group:        "example.com",
			Version:      "v1beta1",
		}

		assert.Equal(t, "customresources", resource.Name)
		assert.Equal(t, "example.com", resource.Group)
		assert.Equal(t, "v1beta1", resource.Version)
	})
}

func TestClusterHealth_Structure(t *testing.T) {
	// Test ClusterHealth struct

	t.Run("healthy cluster", func(t *testing.T) {
		health := ClusterHealth{
			Status: "Healthy",
			Components: []ComponentHealth{
				{
					Name:    "kube-apiserver",
					Status:  "Healthy",
					Message: "",
				},
				{
					Name:    "etcd",
					Status:  "Healthy",
					Message: "",
				},
			},
			Nodes: []NodeHealth{
				{
					Name:  "node-1",
					Ready: true,
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		}

		assert.Equal(t, "Healthy", health.Status)
		assert.Len(t, health.Components, 2)
		assert.Len(t, health.Nodes, 1)

		// Check component health
		apiserver := health.Components[0]
		assert.Equal(t, "kube-apiserver", apiserver.Name)
		assert.Equal(t, "Healthy", apiserver.Status)
		assert.Empty(t, apiserver.Message)

		// Check node health
		node := health.Nodes[0]
		assert.Equal(t, "node-1", node.Name)
		assert.True(t, node.Ready)
		assert.Len(t, node.Conditions, 1)
		assert.Equal(t, corev1.NodeReady, node.Conditions[0].Type)
		assert.Equal(t, corev1.ConditionTrue, node.Conditions[0].Status)
	})

	t.Run("unhealthy cluster", func(t *testing.T) {
		health := ClusterHealth{
			Status: "Unhealthy",
			Components: []ComponentHealth{
				{
					Name:    "kube-apiserver",
					Status:  "Healthy",
					Message: "",
				},
				{
					Name:    "etcd",
					Status:  "Unhealthy",
					Message: "Connection timeout",
				},
			},
			Nodes: []NodeHealth{
				{
					Name:  "node-1",
					Ready: false,
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
							Reason: "NetworkUnavailable",
						},
					},
				},
			},
		}

		assert.Equal(t, "Unhealthy", health.Status)

		// Check unhealthy component
		etcd := health.Components[1]
		assert.Equal(t, "etcd", etcd.Name)
		assert.Equal(t, "Unhealthy", etcd.Status)
		assert.Equal(t, "Connection timeout", etcd.Message)

		// Check unhealthy node
		node := health.Nodes[0]
		assert.Equal(t, "node-1", node.Name)
		assert.False(t, node.Ready)
		assert.Equal(t, corev1.ConditionFalse, node.Conditions[0].Status)
		assert.Equal(t, "NetworkUnavailable", node.Conditions[0].Reason)
	})
}

func TestComponentHealth_Structure(t *testing.T) {
	// Test ComponentHealth struct

	t.Run("healthy component", func(t *testing.T) {
		component := ComponentHealth{
			Name:    "kube-scheduler",
			Status:  "Healthy",
			Message: "",
		}

		assert.Equal(t, "kube-scheduler", component.Name)
		assert.Equal(t, "Healthy", component.Status)
		assert.Empty(t, component.Message)
	})

	t.Run("unhealthy component with message", func(t *testing.T) {
		component := ComponentHealth{
			Name:    "kube-controller-manager",
			Status:  "Unhealthy",
			Message: "Failed to connect to etcd",
		}

		assert.Equal(t, "kube-controller-manager", component.Name)
		assert.Equal(t, "Unhealthy", component.Status)
		assert.Equal(t, "Failed to connect to etcd", component.Message)
	})
}

func TestNodeHealth_Structure(t *testing.T) {
	// Test NodeHealth struct

	t.Run("ready node with multiple conditions", func(t *testing.T) {
		now := metav1.Now()
		node := NodeHealth{
			Name:  "master-node",
			Ready: true,
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  now,
					LastTransitionTime: now,
					Reason:             "KubeletReady",
					Message:            "kubelet is posting ready status",
				},
				{
					Type:               corev1.NodeMemoryPressure,
					Status:             corev1.ConditionFalse,
					LastHeartbeatTime:  now,
					LastTransitionTime: now,
					Reason:             "KubeletHasSufficientMemory",
					Message:            "kubelet has sufficient memory available",
				},
				{
					Type:               corev1.NodeDiskPressure,
					Status:             corev1.ConditionFalse,
					LastHeartbeatTime:  now,
					LastTransitionTime: now,
					Reason:             "KubeletHasNoDiskPressure",
					Message:            "kubelet has no disk pressure",
				},
			},
		}

		assert.Equal(t, "master-node", node.Name)
		assert.True(t, node.Ready)
		assert.Len(t, node.Conditions, 3)

		// Check Ready condition
		readyCondition := node.Conditions[0]
		assert.Equal(t, corev1.NodeReady, readyCondition.Type)
		assert.Equal(t, corev1.ConditionTrue, readyCondition.Status)
		assert.Equal(t, "KubeletReady", readyCondition.Reason)

		// Check MemoryPressure condition (should be False for healthy)
		memoryCondition := node.Conditions[1]
		assert.Equal(t, corev1.NodeMemoryPressure, memoryCondition.Type)
		assert.Equal(t, corev1.ConditionFalse, memoryCondition.Status)

		// Check DiskPressure condition (should be False for healthy)
		diskCondition := node.Conditions[2]
		assert.Equal(t, corev1.NodeDiskPressure, diskCondition.Type)
		assert.Equal(t, corev1.ConditionFalse, diskCondition.Status)
	})

	t.Run("not ready node", func(t *testing.T) {
		node := NodeHealth{
			Name:  "worker-node",
			Ready: false,
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet stopped posting node status",
				},
			},
		}

		assert.Equal(t, "worker-node", node.Name)
		assert.False(t, node.Ready)
		assert.Len(t, node.Conditions, 1)

		condition := node.Conditions[0]
		assert.Equal(t, corev1.NodeReady, condition.Type)
		assert.Equal(t, corev1.ConditionFalse, condition.Status)
		assert.Equal(t, "KubeletNotReady", condition.Reason)
	})
}

func TestKubernetesClient_ClusterOperationsValidation(t *testing.T) {
	// Test validation logic for cluster operations without calling methods that can deadlock

	t.Run("API resources operation validation", func(t *testing.T) {
		client := createTestClientForResources()
		client.allowedOperations = []string{"get", "list"}

		// API resources operation would typically be covered by "get" or "list"
		assert.NoError(t, client.isOperationAllowed("get"))
		assert.NoError(t, client.isOperationAllowed("list"))

		// Test disallowed operations
		client.allowedOperations = []string{"delete"}
		assert.Error(t, client.isOperationAllowed("get"))
		assert.Error(t, client.isOperationAllowed("list"))
	})

	t.Run("cluster health operation validation", func(t *testing.T) {
		client := createTestClientForResources()
		client.nonDestructiveMode = true
		client.dryRun = false

		// Cluster health checks should be allowed in non-destructive mode
		assert.NoError(t, client.isOperationAllowed("get"))
		assert.NoError(t, client.isOperationAllowed("list"))
		assert.NoError(t, client.isOperationAllowed("describe"))
	})
}

func TestKubernetesClient_LogClusterOperations(t *testing.T) {
	testLog := &testLogger{}

	client := &kubernetesClient{
		config: &ClientConfig{
			Logger: testLog,
		},
	}

	// Log cluster operations
	client.logOperation("get-api-resources", "test-context", "", "", "")
	client.logOperation("get-cluster-health", "test-context", "", "", "")

	// Verify that logging occurred
	assert.NotEmpty(t, testLog.messages)
}
