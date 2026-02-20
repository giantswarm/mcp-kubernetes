package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// createTestCAPIClusterWithDetails creates a CAPI Cluster resource with full metadata.
// By default it creates a v1beta2-style cluster with conditions in status.conditions[].
func createTestCAPIClusterWithDetails(name, namespace string, opts ...clusterOption) *unstructured.Unstructured {
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"paused": false,
			},
			"status": map[string]interface{}{
				"phase": "Provisioned",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   ConditionControlPlaneAvailable,
						"status": ConditionStatusTrue,
					},
					map[string]interface{}{
						"type":   ConditionInfrastructureReady,
						"status": ConditionStatusTrue,
					},
				},
			},
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(cluster)
	}

	return cluster
}

// clusterOption is a functional option for customizing test clusters.
type clusterOption func(*unstructured.Unstructured)

// withLabels adds labels to the cluster.
func withLabels(labels map[string]string) clusterOption {
	return func(c *unstructured.Unstructured) {
		c.SetLabels(labels)
	}
}

// withAnnotations adds annotations to the cluster.
func withAnnotations(annotations map[string]string) clusterOption {
	return func(c *unstructured.Unstructured) {
		c.SetAnnotations(annotations)
	}
}

// withInfrastructureRef sets the infrastructure reference.
func withInfrastructureRef(kind, name string) clusterOption {
	return func(c *unstructured.Unstructured) {
		_ = unstructured.SetNestedMap(c.Object, map[string]interface{}{
			"kind": kind,
			"name": name,
		}, "spec", "infrastructureRef")
	}
}

// withTopologyVersion sets the topology version (Kubernetes version).
func withTopologyVersion(version string) clusterOption {
	return func(c *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(c.Object, version, "spec", "topology", "version")
	}
}

// withStatus sets the cluster status using v1beta2 conditions.
func withStatus(phase string, cpReady, infraReady bool) clusterOption {
	return func(c *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(c.Object, phase, "status", "phase")
		cpStatus := "False"
		if cpReady {
			cpStatus = ConditionStatusTrue
		}
		infraStatus := "False"
		if infraReady {
			infraStatus = ConditionStatusTrue
		}
		conditions := []interface{}{
			map[string]interface{}{
				"type":   ConditionControlPlaneAvailable,
				"status": cpStatus,
			},
			map[string]interface{}{
				"type":   ConditionInfrastructureReady,
				"status": infraStatus,
			},
		}
		_ = unstructured.SetNestedSlice(c.Object, conditions, "status", "conditions")
	}
}

// withControlPlaneReplicas sets the control plane ready replica count.
func withControlPlaneReplicas(count int) clusterOption {
	return func(c *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(c.Object, int64(count), "status", "controlPlane", "readyReplicas")
	}
}

func TestClusterSummaryFromUnstructured(t *testing.T) {
	tests := []struct {
		name     string
		cluster  *unstructured.Unstructured
		expected ClusterSummary
	}{
		{
			name: "basic cluster",
			cluster: createTestCAPIClusterWithDetails("my-cluster", "org-acme",
				withStatus("Provisioned", true, true),
			),
			expected: ClusterSummary{
				Name:                "my-cluster",
				Namespace:           "org-acme",
				Provider:            ProviderUnknown,
				Status:              "Provisioned",
				Ready:               true,
				ControlPlaneReady:   true,
				InfrastructureReady: true,
			},
		},
		{
			name: "AWS cluster with full metadata",
			cluster: createTestCAPIClusterWithDetails("prod-cluster", "org-acme",
				withLabels(map[string]string{
					LabelGiantSwarmCluster:      "prod-cluster",
					LabelGiantSwarmOrganization: "acme-corp",
					LabelGiantSwarmRelease:      "20.1.0",
				}),
				withAnnotations(map[string]string{
					AnnotationClusterDescription: "Production cluster",
				}),
				withInfrastructureRef("AWSCluster", "prod-cluster"),
				withTopologyVersion("v1.28.5"),
				withStatus("Provisioned", true, true),
				withControlPlaneReplicas(3),
			),
			expected: ClusterSummary{
				Name:                "prod-cluster",
				Namespace:           "org-acme",
				Provider:            ProviderAWS,
				Release:             "20.1.0",
				KubernetesVersion:   "v1.28.5",
				Status:              "Provisioned",
				Ready:               true,
				ControlPlaneReady:   true,
				InfrastructureReady: true,
				NodeCount:           3,
			},
		},
		{
			name: "Azure cluster",
			cluster: createTestCAPIClusterWithDetails("azure-cluster", "org-azure",
				withInfrastructureRef("AzureCluster", "azure-cluster"),
				withStatus("Provisioned", true, true),
			),
			expected: ClusterSummary{
				Name:                "azure-cluster",
				Namespace:           "org-azure",
				Provider:            ProviderAzure,
				Status:              "Provisioned",
				Ready:               true,
				ControlPlaneReady:   true,
				InfrastructureReady: true,
			},
		},
		{
			name: "vSphere cluster",
			cluster: createTestCAPIClusterWithDetails("vsphere-cluster", "org-vsphere",
				withInfrastructureRef("VSphereCluster", "vsphere-cluster"),
				withStatus("Provisioned", true, true),
			),
			expected: ClusterSummary{
				Name:                "vsphere-cluster",
				Namespace:           "org-vsphere",
				Provider:            ProviderVSphere,
				Status:              "Provisioned",
				Ready:               true,
				ControlPlaneReady:   true,
				InfrastructureReady: true,
			},
		},
		{
			name: "cluster in provisioning state",
			cluster: createTestCAPIClusterWithDetails("new-cluster", "org-test",
				withStatus("Provisioning", false, true),
			),
			expected: ClusterSummary{
				Name:                "new-cluster",
				Namespace:           "org-test",
				Provider:            ProviderUnknown,
				Status:              "Provisioning",
				Ready:               false,
				ControlPlaneReady:   false,
				InfrastructureReady: true,
			},
		},
		{
			name: "cluster with control plane not ready",
			cluster: createTestCAPIClusterWithDetails("cp-cluster", "org-test",
				withStatus("Provisioned", false, true),
			),
			expected: ClusterSummary{
				Name:                "cp-cluster",
				Namespace:           "org-test",
				Provider:            ProviderUnknown,
				Status:              "Provisioned",
				Ready:               false, // Not ready because CP is not ready
				ControlPlaneReady:   false,
				InfrastructureReady: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clusterSummaryFromUnstructured(tt.cluster)

			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Namespace, result.Namespace)
			assert.Equal(t, tt.expected.Provider, result.Provider)
			assert.Equal(t, tt.expected.Release, result.Release)
			assert.Equal(t, tt.expected.KubernetesVersion, result.KubernetesVersion)
			assert.Equal(t, tt.expected.Status, result.Status)
			assert.Equal(t, tt.expected.Ready, result.Ready)
			assert.Equal(t, tt.expected.ControlPlaneReady, result.ControlPlaneReady)
			assert.Equal(t, tt.expected.InfrastructureReady, result.InfrastructureReady)
			assert.Equal(t, tt.expected.NodeCount, result.NodeCount)
		})
	}
}

func TestMapInfraKindToProvider(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"AWSCluster", ProviderAWS},
		{"AWSManagedCluster", ProviderAWS},
		{"AzureCluster", ProviderAzure},
		{"AzureManagedCluster", ProviderAzure},
		{"VSphereCluster", ProviderVSphere},
		{"GCPCluster", ProviderGCP},
		{"GoogleCluster", ProviderGCP},
		{"DockerCluster", "docker"},
		{"KindCluster", "kind"},
		{"UnknownType", "unknowntype"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			result := mapInfraKindToProvider(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractClusterStatus(t *testing.T) {
	tests := []struct {
		name               string
		cluster            *unstructured.Unstructured
		expectedPhase      string
		expectedReady      bool
		expectedCPReady    bool
		expectedInfraReady bool
	}{
		{
			name: "fully ready cluster",
			cluster: createTestCAPIClusterWithDetails("ready", "ns",
				withStatus("Provisioned", true, true),
			),
			expectedPhase:      "Provisioned",
			expectedReady:      true,
			expectedCPReady:    true,
			expectedInfraReady: true,
		},
		{
			name: "provisioning cluster",
			cluster: createTestCAPIClusterWithDetails("provisioning", "ns",
				withStatus("Provisioning", false, false),
			),
			expectedPhase:      "Provisioning",
			expectedReady:      false,
			expectedCPReady:    false,
			expectedInfraReady: false,
		},
		{
			name: "deleting cluster",
			cluster: createTestCAPIClusterWithDetails("deleting", "ns",
				withStatus("Deleting", true, true),
			),
			expectedPhase:      "Deleting",
			expectedReady:      false,
			expectedCPReady:    true,
			expectedInfraReady: true,
		},
		{
			name:               "cluster with missing conditions",
			cluster:            createTestCAPICluster("minimal", "ns"),
			expectedPhase:      "Provisioned",
			expectedReady:      false,
			expectedCPReady:    false,
			expectedInfraReady: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase, ready, cpReady, infraReady := extractClusterStatus(tt.cluster)

			assert.Equal(t, tt.expectedPhase, phase)
			assert.Equal(t, tt.expectedReady, ready)
			assert.Equal(t, tt.expectedCPReady, cpReady)
			assert.Equal(t, tt.expectedInfraReady, infraReady)
		})
	}
}

func TestFindConditionStatus(t *testing.T) {
	tests := []struct {
		name          string
		obj           map[string]interface{}
		conditionType string
		path          []string
		expectedVal   bool
		expectedFound bool
	}{
		{
			name: "finds True condition",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready", "status": ConditionStatusTrue},
					},
				},
			},
			conditionType: "Ready",
			path:          []string{"status", "conditions"},
			expectedVal:   true,
			expectedFound: true,
		},
		{
			name: "finds False condition",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready", "status": "False"},
					},
				},
			},
			conditionType: "Ready",
			path:          []string{"status", "conditions"},
			expectedVal:   false,
			expectedFound: true,
		},
		{
			name: "condition not found",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Other", "status": ConditionStatusTrue},
					},
				},
			},
			conditionType: "Ready",
			path:          []string{"status", "conditions"},
			expectedVal:   false,
			expectedFound: false,
		},
		{
			name:          "missing conditions path",
			obj:           map[string]interface{}{},
			conditionType: "Ready",
			path:          []string{"status", "conditions"},
			expectedVal:   false,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := findConditionStatus(tt.obj, tt.conditionType, tt.path...)
			assert.Equal(t, tt.expectedVal, val)
			assert.Equal(t, tt.expectedFound, found)
		})
	}
}

func TestExtractNodeCount(t *testing.T) {
	tests := []struct {
		name     string
		cluster  *unstructured.Unstructured
		expected int
	}{
		{
			name: "control plane ready replicas",
			cluster: createTestCAPIClusterWithDetails("test", "ns",
				withControlPlaneReplicas(3),
			),
			expected: 3,
		},
		{
			name:     "no replica info returns 0",
			cluster:  createTestCAPICluster("test", "ns"),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractNodeCount(tt.cluster)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindClusterByName(t *testing.T) {
	clusters := []ClusterSummary{
		{Name: "cluster-a", Namespace: "ns1"},
		{Name: "cluster-b", Namespace: "ns2"},
		{Name: "prod-cluster", Namespace: "ns3"},
	}

	tests := []struct {
		name        string
		searchName  string
		expectFound bool
		expectName  string
	}{
		{"exact match first", "cluster-a", true, "cluster-a"},
		{"exact match last", "prod-cluster", true, "prod-cluster"},
		{"not found", "missing", false, ""},
		{"partial match doesn't work", "cluster", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findClusterByName(clusters, tt.searchName)

			if tt.expectFound {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectName, result.Name)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestFindClustersByPattern(t *testing.T) {
	clusters := []ClusterSummary{
		{Name: "prod-cluster-01", Namespace: "ns1"},
		{Name: "prod-cluster-02", Namespace: "ns2"},
		{Name: "staging-cluster", Namespace: "ns3"},
		{Name: "dev-app", Namespace: "ns4"},
	}

	tests := []struct {
		name        string
		pattern     string
		expectCount int
		expectNames []string
	}{
		{"matches prefix", "prod", 2, []string{"prod-cluster-01", "prod-cluster-02"}},
		{"matches suffix", "cluster", 3, []string{"prod-cluster-01", "prod-cluster-02", "staging-cluster"}},
		{"matches middle", "-cluster-", 2, []string{"prod-cluster-01", "prod-cluster-02"}},
		{"exact match", "dev-app", 1, []string{"dev-app"}},
		{"case insensitive", "PROD", 2, []string{"prod-cluster-01", "prod-cluster-02"}},
		{"no matches", "missing", 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findClustersByPattern(clusters, tt.pattern)

			assert.Equal(t, tt.expectCount, len(result))
			if tt.expectNames != nil {
				names := make([]string, len(result))
				for i, c := range result {
					names[i] = c.Name
				}
				assert.ElementsMatch(t, tt.expectNames, names)
			}
		})
	}
}

func TestManager_ListClusters_Discovery(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		clusters      []*unstructured.Unstructured
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, []ClusterSummary)
	}{
		{
			name: "lists all clusters",
			clusters: []*unstructured.Unstructured{
				createTestCAPIClusterWithDetails("cluster-1", "org-a",
					withLabels(map[string]string{LabelGiantSwarmRelease: "20.0.0"}),
				),
				createTestCAPIClusterWithDetails("cluster-2", "org-b"),
			},
			user: testUser(),
			checkResult: func(t *testing.T, clusters []ClusterSummary) {
				require.Len(t, clusters, 2)
				names := []string{clusters[0].Name, clusters[1].Name}
				assert.ElementsMatch(t, []string{"cluster-1", "cluster-2"}, names)
			},
		},
		{
			name:     "returns empty list when no clusters",
			clusters: nil,
			user:     testUser(),
			checkResult: func(t *testing.T, clusters []ClusterSummary) {
				assert.Empty(t, clusters)
			},
		},
		{
			name:          "nil user returns error",
			clusters:      nil,
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name:     "closed manager returns error",
			clusters: nil,
			user:     testUser(),
			setupManager: func(m *Manager) {
				_ = m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupTestManager(t, tt.clusters, nil)

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			result, err := manager.ListClusters(ctx, tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected %v, got %v", tt.expectedError, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestManager_ListClustersWithMetadata(t *testing.T) {
	ctx := context.Background()

	// Create a cluster with full metadata
	cluster := createTestCAPIClusterWithDetails("prod-cluster", "org-acme",
		withLabels(map[string]string{
			LabelGiantSwarmCluster:      "prod-cluster",
			LabelGiantSwarmOrganization: "acme-corp",
			LabelGiantSwarmRelease:      "20.1.0",
		}),
		withAnnotations(map[string]string{
			AnnotationClusterDescription: "Production cluster for ACME",
		}),
		withInfrastructureRef("AWSCluster", "prod-cluster"),
		withTopologyVersion("v1.28.5"),
		withStatus("Provisioned", true, true),
		withControlPlaneReplicas(3),
	)

	manager := setupTestManager(t, []*unstructured.Unstructured{cluster}, nil)

	result, err := manager.ListClusters(ctx, testUser())
	require.NoError(t, err)
	require.Len(t, result, 1)

	c := result[0]
	assert.Equal(t, "prod-cluster", c.Name)
	assert.Equal(t, "org-acme", c.Namespace)
	assert.Equal(t, ProviderAWS, c.Provider)
	assert.Equal(t, "20.1.0", c.Release)
	assert.Equal(t, "v1.28.5", c.KubernetesVersion)
	assert.Equal(t, "Provisioned", c.Status)
	assert.True(t, c.Ready)
	assert.True(t, c.ControlPlaneReady)
	assert.True(t, c.InfrastructureReady)
	assert.Equal(t, 3, c.NodeCount)

	// Check helper methods
	assert.True(t, c.IsGiantSwarmCluster())
	assert.Equal(t, "acme-corp", c.Organization())
	assert.Equal(t, "Production cluster for ACME", c.Description())
}

func TestManager_GetClusterSummary_Discovery(t *testing.T) {
	ctx := context.Background()

	clusters := []*unstructured.Unstructured{
		createTestCAPIClusterWithDetails("prod-cluster", "org-acme",
			withLabels(map[string]string{LabelGiantSwarmRelease: "20.0.0"}),
			withInfrastructureRef("AWSCluster", "prod-cluster"),
		),
		createTestCAPIClusterWithDetails("staging-cluster", "org-acme"),
	}

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError error
		checkResult   func(*testing.T, *ClusterSummary)
	}{
		{
			name:        "finds existing cluster",
			clusterName: "prod-cluster",
			user:        testUser(),
			checkResult: func(t *testing.T, summary *ClusterSummary) {
				assert.Equal(t, "prod-cluster", summary.Name)
				assert.Equal(t, "org-acme", summary.Namespace)
				assert.Equal(t, ProviderAWS, summary.Provider)
				assert.Equal(t, "20.0.0", summary.Release)
			},
		},
		{
			name:        "finds second cluster",
			clusterName: "staging-cluster",
			user:        testUser(),
			checkResult: func(t *testing.T, summary *ClusterSummary) {
				assert.Equal(t, "staging-cluster", summary.Name)
				assert.Equal(t, "org-acme", summary.Namespace)
			},
		},
		{
			name:          "cluster not found",
			clusterName:   "nonexistent",
			user:          testUser(),
			expectedError: ErrClusterNotFound,
		},
		{
			name:          "empty cluster name",
			clusterName:   "",
			user:          testUser(),
			expectedError: ErrInvalidClusterName,
		},
		{
			name:          "invalid cluster name",
			clusterName:   "../etc/passwd",
			user:          testUser(),
			expectedError: ErrInvalidClusterName,
		},
		{
			name:          "nil user",
			clusterName:   "prod-cluster",
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name:        "closed manager",
			clusterName: "prod-cluster",
			user:        testUser(),
			setupManager: func(m *Manager) {
				_ = m.Close()
			},
			expectedError: ErrManagerClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupTestManager(t, clusters, nil)

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			result, err := manager.GetClusterSummary(ctx, tt.clusterName, tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected %v, got %v", tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestManager_ResolveCluster(t *testing.T) {
	ctx := context.Background()

	clusters := []*unstructured.Unstructured{
		createTestCAPIClusterWithDetails("prod-cluster-01", "org-acme"),
		createTestCAPIClusterWithDetails("prod-cluster-02", "org-acme"),
		createTestCAPIClusterWithDetails("staging-cluster", "org-acme"),
		createTestCAPIClusterWithDetails("dev-app", "org-dev"),
	}

	tests := []struct {
		name          string
		pattern       string
		user          *UserInfo
		setupManager  func(*Manager)
		expectedError string
		checkResult   func(*testing.T, *ClusterSummary)
	}{
		{
			name:    "exact match",
			pattern: "dev-app",
			user:    testUser(),
			checkResult: func(t *testing.T, summary *ClusterSummary) {
				assert.Equal(t, "dev-app", summary.Name)
			},
		},
		{
			name:    "unique pattern match",
			pattern: "staging",
			user:    testUser(),
			checkResult: func(t *testing.T, summary *ClusterSummary) {
				assert.Equal(t, "staging-cluster", summary.Name)
			},
		},
		{
			name:          "ambiguous pattern",
			pattern:       "prod",
			user:          testUser(),
			expectedError: "ambiguous cluster name",
		},
		{
			name:          "no match",
			pattern:       "nonexistent",
			user:          testUser(),
			expectedError: "not found",
		},
		{
			name:          "empty pattern",
			pattern:       "",
			user:          testUser(),
			expectedError: "pattern cannot be empty",
		},
		{
			name:          "nil user",
			pattern:       "dev-app",
			user:          nil,
			expectedError: "user information is required",
		},
		{
			name:    "closed manager",
			pattern: "dev-app",
			user:    testUser(),
			setupManager: func(m *Manager) {
				_ = m.Close()
			},
			expectedError: "manager is closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupTestManager(t, clusters, nil)

			if tt.setupManager != nil {
				tt.setupManager(manager)
			}

			result, err := manager.ResolveCluster(ctx, tt.pattern, tt.user)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestManager_ListClustersWithOptions(t *testing.T) {
	ctx := context.Background()

	clusters := []*unstructured.Unstructured{
		createTestCAPIClusterWithDetails("aws-prod", "org-acme",
			withInfrastructureRef("AWSCluster", "aws-prod"),
			withStatus("Provisioned", true, true),
		),
		createTestCAPIClusterWithDetails("aws-staging", "org-acme",
			withInfrastructureRef("AWSCluster", "aws-staging"),
			withStatus("Provisioning", false, true),
		),
		createTestCAPIClusterWithDetails("azure-prod", "org-azure",
			withInfrastructureRef("AzureCluster", "azure-prod"),
			withStatus("Provisioned", true, true),
		),
	}

	tests := []struct {
		name        string
		opts        *ClusterListOptions
		expectCount int
		expectNames []string
	}{
		{
			name:        "no options returns all",
			opts:        nil,
			expectCount: 3,
		},
		{
			name: "filter by provider",
			opts: &ClusterListOptions{
				Provider: ProviderAWS,
			},
			expectCount: 2,
			expectNames: []string{"aws-prod", "aws-staging"},
		},
		{
			name: "filter by ready only",
			opts: &ClusterListOptions{
				ReadyOnly: true,
			},
			expectCount: 2,
			expectNames: []string{"aws-prod", "azure-prod"},
		},
		{
			name: "filter by provider and ready",
			opts: &ClusterListOptions{
				Provider:  ProviderAWS,
				ReadyOnly: true,
			},
			expectCount: 1,
			expectNames: []string{"aws-prod"},
		},
		{
			name: "filter by status",
			opts: &ClusterListOptions{
				Status: ClusterPhaseProvisioning,
			},
			expectCount: 1,
			expectNames: []string{"aws-staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupTestManager(t, clusters, nil)

			result, err := manager.listClustersWithOptions(ctx, testUser(), tt.opts)
			require.NoError(t, err)

			assert.Equal(t, tt.expectCount, len(result))
			if tt.expectNames != nil {
				names := make([]string, len(result))
				for i, c := range result {
					names[i] = c.Name
				}
				assert.ElementsMatch(t, tt.expectNames, names)
			}
		})
	}
}

func TestClusterSummaryHelperMethods(t *testing.T) {
	t.Run("IsGiantSwarmCluster", func(t *testing.T) {
		tests := []struct {
			name     string
			summary  ClusterSummary
			expected bool
		}{
			{
				name: "with gs cluster label",
				summary: ClusterSummary{
					Labels: map[string]string{LabelGiantSwarmCluster: "my-cluster"},
				},
				expected: true,
			},
			{
				name: "with gs org label",
				summary: ClusterSummary{
					Labels: map[string]string{LabelGiantSwarmOrganization: "acme"},
				},
				expected: true,
			},
			{
				name: "with both labels",
				summary: ClusterSummary{
					Labels: map[string]string{
						LabelGiantSwarmCluster:      "my-cluster",
						LabelGiantSwarmOrganization: "acme",
					},
				},
				expected: true,
			},
			{
				name: "without gs labels",
				summary: ClusterSummary{
					Labels: map[string]string{"other": "value"},
				},
				expected: false,
			},
			{
				name:     "nil labels",
				summary:  ClusterSummary{},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.expected, tt.summary.IsGiantSwarmCluster())
			})
		}
	})

	t.Run("Organization", func(t *testing.T) {
		tests := []struct {
			name     string
			summary  ClusterSummary
			expected string
		}{
			{
				name: "with org label",
				summary: ClusterSummary{
					Labels: map[string]string{LabelGiantSwarmOrganization: "acme-corp"},
				},
				expected: "acme-corp",
			},
			{
				name: "without org label",
				summary: ClusterSummary{
					Labels: map[string]string{"other": "value"},
				},
				expected: "",
			},
			{
				name:     "nil labels",
				summary:  ClusterSummary{},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.expected, tt.summary.Organization())
			})
		}
	})

	t.Run("Description", func(t *testing.T) {
		tests := []struct {
			name     string
			summary  ClusterSummary
			expected string
		}{
			{
				name: "with description",
				summary: ClusterSummary{
					Annotations: map[string]string{AnnotationClusterDescription: "Production cluster"},
				},
				expected: "Production cluster",
			},
			{
				name: "without description",
				summary: ClusterSummary{
					Annotations: map[string]string{"other": "value"},
				},
				expected: "",
			},
			{
				name:     "nil annotations",
				summary:  ClusterSummary{},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.expected, tt.summary.Description())
			})
		}
	})

	t.Run("ClusterAge", func(t *testing.T) {
		createdAt := time.Now().Add(-24 * time.Hour)
		summary := ClusterSummary{CreatedAt: createdAt}

		age := summary.ClusterAge()
		// Allow some tolerance for test execution time
		assert.True(t, age >= 23*time.Hour && age <= 25*time.Hour)
	})
}

func TestClusterDiscoveryError(t *testing.T) {
	t.Run("Error message", func(t *testing.T) {
		err := &ClusterDiscoveryError{
			Reason: "test reason",
			Err:    errors.New("underlying error"),
		}
		assert.Contains(t, err.Error(), "test reason")
		assert.Contains(t, err.Error(), "underlying error")
	})

	t.Run("Unwrap", func(t *testing.T) {
		underlying := errors.New("underlying")
		err := &ClusterDiscoveryError{Reason: "test", Err: underlying}
		assert.Equal(t, underlying, err.Unwrap())
	})

	t.Run("UserFacingError for CRD not installed", func(t *testing.T) {
		err := &ClusterDiscoveryError{
			Reason: "CAPI Cluster CRD not installed",
		}
		assert.Contains(t, err.UserFacingError(), "does not have CAPI installed")
	})

	t.Run("UserFacingError for other errors", func(t *testing.T) {
		err := &ClusterDiscoveryError{
			Reason: "some other error",
		}
		assert.Contains(t, err.UserFacingError(), "unable to discover clusters")
	})
}

func TestAmbiguousClusterError(t *testing.T) {
	err := &AmbiguousClusterError{
		Pattern: "prod",
		Matches: []ClusterSummary{
			{Name: "prod-01", Namespace: "ns1"},
			{Name: "prod-02", Namespace: "ns2"},
		},
	}

	t.Run("Error message", func(t *testing.T) {
		assert.Contains(t, err.Error(), "ambiguous cluster name")
		assert.Contains(t, err.Error(), "2 clusters")
		assert.Contains(t, err.Error(), "prod")
	})

	t.Run("UserFacingError", func(t *testing.T) {
		msg := err.UserFacingError()
		assert.Contains(t, msg, "multiple clusters match")
		assert.Contains(t, msg, "prod-01")
		assert.Contains(t, msg, "prod-02")
	})
}

func TestManager_DiscoveryConcurrency(t *testing.T) {
	ctx := context.Background()

	// Create test clusters
	var clusters []*unstructured.Unstructured
	for i := 0; i < 10; i++ {
		clusters = append(clusters, createTestCAPIClusterWithDetails(
			"cluster-"+string(rune('a'+i)),
			"org-test",
		))
	}

	manager := setupTestManager(t, clusters, nil)

	// Run concurrent discovery operations
	const numGoroutines = 50
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			user := &UserInfo{Email: "user@example.com"}

			// Mix of operations
			switch id % 3 {
			case 0:
				_, err := manager.ListClusters(ctx, user)
				results <- err
			case 1:
				_, err := manager.GetClusterSummary(ctx, "cluster-a", user)
				results <- err
			case 2:
				_, err := manager.ResolveCluster(ctx, "cluster-b", user)
				results <- err
			}
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(t, err)
	}
}

func TestExtractKubernetesVersion(t *testing.T) {
	tests := []struct {
		name     string
		cluster  *unstructured.Unstructured
		expected string
	}{
		{
			name: "from topology version",
			cluster: createTestCAPIClusterWithDetails("test", "ns",
				withTopologyVersion("v1.28.5"),
			),
			expected: "v1.28.5",
		},
		{
			name: "from status version",
			cluster: func() *unstructured.Unstructured {
				c := createTestCAPIClusterWithDetails("test", "ns")
				_ = unstructured.SetNestedField(c.Object, "v1.27.0", "status", "version")
				return c
			}(),
			expected: "v1.27.0",
		},
		{
			name:     "missing version",
			cluster:  createTestCAPIClusterWithDetails("test", "ns"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKubernetesVersion(tt.cluster)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWrapCAPIListError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		result := wrapCAPIListError(nil)
		assert.Nil(t, result)
	})

	t.Run("generic error wraps as failed to list", func(t *testing.T) {
		err := errors.New("connection refused")
		result := wrapCAPIListError(err)

		require.NotNil(t, result)
		discoveryErr, ok := result.(*ClusterDiscoveryError)
		require.True(t, ok)
		assert.Equal(t, "failed to list CAPI clusters", discoveryErr.Reason)
		assert.Equal(t, err, discoveryErr.Err)
	})
}

func TestManager_GetClusterByName(t *testing.T) {
	ctx := context.Background()

	clusters := []*unstructured.Unstructured{
		createTestCAPIClusterWithDetails("prod-cluster", "org-acme",
			withLabels(map[string]string{LabelGiantSwarmRelease: "20.0.0"}),
			withInfrastructureRef("AWSCluster", "prod-cluster"),
		),
		createTestCAPIClusterWithDetails("staging-cluster", "org-acme"),
	}

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		expectFound   bool
		expectName    string
		expectedError error
	}{
		{
			name:        "finds existing cluster",
			clusterName: "prod-cluster",
			user:        testUser(),
			expectFound: true,
			expectName:  "prod-cluster",
		},
		{
			name:        "finds second cluster",
			clusterName: "staging-cluster",
			user:        testUser(),
			expectFound: true,
			expectName:  "staging-cluster",
		},
		{
			name:        "cluster not found returns nil",
			clusterName: "nonexistent",
			user:        testUser(),
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupTestManager(t, clusters, nil)
			dynamicClient, err := manager.GetDynamicClient(ctx, "", tt.user)
			require.NoError(t, err)

			result, err := manager.getClusterByName(ctx, dynamicClient, tt.clusterName, tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError))
			} else {
				require.NoError(t, err)
				if tt.expectFound {
					require.NotNil(t, result)
					assert.Equal(t, tt.expectName, result.Name)
				} else {
					assert.Nil(t, result)
				}
			}
		})
	}
}
