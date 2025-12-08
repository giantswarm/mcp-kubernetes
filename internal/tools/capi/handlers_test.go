package capi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcpoauth "github.com/giantswarm/mcp-oauth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/capi/testdata"
)

// createTestClusters creates a set of test clusters using the testdata package.
func createTestClusters() []federation.ClusterSummary {
	return testdata.CreateTestClusters()
}

// contextWithUserInfo creates a context with user info for testing.
func contextWithUserInfo(email string, groups []string) context.Context {
	userInfo := &oauth.UserInfo{
		Email:  email,
		Groups: groups,
	}
	return mcpoauth.ContextWithUserInfo(context.Background(), userInfo)
}

func TestListClustersWithOptions(t *testing.T) {
	clusters := createTestClusters()

	tests := []struct {
		name          string
		clusters      []federation.ClusterSummary
		opts          *federation.ClusterListOptions
		expectedCount int
		expectedNames []string
	}{
		{
			name:          "no filter returns all clusters",
			clusters:      clusters,
			opts:          nil,
			expectedCount: 3,
			expectedNames: []string{"prod-wc-01", "staging-wc", "dev-cluster"},
		},
		{
			name:     "filter by namespace",
			clusters: clusters,
			opts: &federation.ClusterListOptions{
				Namespace: "org-acme",
			},
			expectedCount: 2,
			expectedNames: []string{"prod-wc-01", "staging-wc"},
		},
		{
			name:     "filter by provider",
			clusters: clusters,
			opts: &federation.ClusterListOptions{
				Provider: "azure",
			},
			expectedCount: 1,
			expectedNames: []string{"dev-cluster"},
		},
		{
			name:     "filter by status",
			clusters: clusters,
			opts: &federation.ClusterListOptions{
				Status: federation.ClusterPhaseProvisioned,
			},
			expectedCount: 2,
			expectedNames: []string{"prod-wc-01", "staging-wc"},
		},
		{
			name:     "filter by ready only",
			clusters: clusters,
			opts: &federation.ClusterListOptions{
				ReadyOnly: true,
			},
			expectedCount: 2,
			expectedNames: []string{"prod-wc-01", "staging-wc"},
		},
		{
			name:     "combined filters",
			clusters: clusters,
			opts: &federation.ClusterListOptions{
				Namespace: "org-acme",
				Provider:  "aws",
				ReadyOnly: true,
			},
			expectedCount: 2,
			expectedNames: []string{"prod-wc-01", "staging-wc"},
		},
		{
			name:     "no matches",
			clusters: clusters,
			opts: &federation.ClusterListOptions{
				Provider: "gcp",
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &testdata.MockFederationManager{Clusters: tt.clusters}
			user := &federation.UserInfo{Email: "test@example.com"}

			result, err := listClustersWithOptions(context.Background(), mock, user, tt.opts)

			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)

			actualNames := make([]string, len(result))
			for i, c := range result {
				actualNames[i] = c.Name
			}
			assert.ElementsMatch(t, tt.expectedNames, actualNames)
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"less than minute", 30 * time.Second, "<1m"},
		{"exactly one minute", 1 * time.Minute, "1m"},
		{"several minutes", 45 * time.Minute, "45m"},
		{"one hour", 1 * time.Hour, "1h"},
		{"several hours", 12 * time.Hour, "12h"},
		{"one day", 24 * time.Hour, "1d"},
		{"several days", 45 * 24 * time.Hour, "45d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAge(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClusterSummaryToListItem(t *testing.T) {
	cluster := federation.ClusterSummary{
		Name:      "test-cluster",
		Namespace: "org-test",
		Provider:  "aws",
		Release:   "20.0.0",
		Status:    "Provisioned",
		Ready:     true,
		NodeCount: 10,
		CreatedAt: time.Now().Add(-24 * time.Hour),
		Labels: map[string]string{
			"giantswarm.io/organization": "test-org",
		},
	}

	item := clusterSummaryToListItem(cluster)

	assert.Equal(t, "test-cluster", item.Name)
	assert.Equal(t, "org-test", item.Namespace)
	assert.Equal(t, "test-org", item.Organization)
	assert.Equal(t, "aws", item.Provider)
	assert.Equal(t, "20.0.0", item.Release)
	assert.Equal(t, "Provisioned", item.Status)
	assert.True(t, item.Ready)
	assert.Equal(t, 10, item.NodeCount)
	assert.Equal(t, "1d", item.Age)
}

func TestClusterSummaryToDetail(t *testing.T) {
	cluster := &federation.ClusterSummary{
		Name:                "test-cluster",
		Namespace:           "org-test",
		Provider:            "aws",
		Release:             "20.0.0",
		KubernetesVersion:   "1.28.5",
		Status:              "Provisioned",
		Ready:               true,
		ControlPlaneReady:   true,
		InfrastructureReady: true,
		NodeCount:           10,
		CreatedAt:           time.Now().Add(-24 * time.Hour),
		Labels: map[string]string{
			"giantswarm.io/organization": "test-org",
			"environment":                "production",
		},
		Annotations: map[string]string{
			"cluster.giantswarm.io/description": "Test cluster",
		},
	}

	detail := clusterSummaryToDetail(cluster)

	assert.Equal(t, "test-cluster", detail.Name)
	assert.Equal(t, "org-test", detail.Namespace)
	assert.Equal(t, "test-org", detail.Metadata.Organization)
	assert.Equal(t, "aws", detail.Metadata.Provider)
	assert.Equal(t, "20.0.0", detail.Metadata.Release)
	assert.Equal(t, "1.28.5", detail.Metadata.KubernetesVersion)
	assert.Equal(t, "Test cluster", detail.Metadata.Description)
	assert.Equal(t, "Provisioned", detail.Status.Phase)
	assert.True(t, detail.Status.Ready)
	assert.True(t, detail.Status.ControlPlaneReady)
	assert.True(t, detail.Status.InfrastructureReady)
	assert.Equal(t, 10, detail.Status.NodeCount)
}

func TestBuildHealthOutput(t *testing.T) {
	tests := []struct {
		name           string
		cluster        *federation.ClusterSummary
		expectedStatus string
	}{
		{
			name: "healthy cluster",
			cluster: &federation.ClusterSummary{
				Name:                "healthy-cluster",
				Status:              "Provisioned",
				Ready:               true,
				ControlPlaneReady:   true,
				InfrastructureReady: true,
				NodeCount:           5,
			},
			expectedStatus: HealthStatusHealthy,
		},
		{
			name: "provisioning cluster",
			cluster: &federation.ClusterSummary{
				Name:                "provisioning-cluster",
				Status:              "Provisioning",
				Ready:               false,
				ControlPlaneReady:   true,
				InfrastructureReady: false,
				NodeCount:           0,
			},
			expectedStatus: HealthStatusDegraded,
		},
		{
			name: "deleting cluster",
			cluster: &federation.ClusterSummary{
				Name:                "deleting-cluster",
				Status:              "Deleting",
				Ready:               false,
				ControlPlaneReady:   true,
				InfrastructureReady: true,
				NodeCount:           5,
			},
			expectedStatus: HealthStatusUnknown,
		},
		{
			name: "unhealthy cluster",
			cluster: &federation.ClusterSummary{
				Name:                "unhealthy-cluster",
				Status:              "Failed",
				Ready:               false,
				ControlPlaneReady:   false,
				InfrastructureReady: false,
				NodeCount:           0,
			},
			expectedStatus: HealthStatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := buildHealthOutput(tt.cluster)

			assert.Equal(t, tt.cluster.Name, output.Name)
			assert.Equal(t, tt.expectedStatus, output.Status)
			assert.NotEmpty(t, output.Message)
			assert.NotEmpty(t, output.Checks)
		})
	}
}

func TestBuildHealthChecks(t *testing.T) {
	cluster := &federation.ClusterSummary{
		Name:                "test-cluster",
		Status:              "Provisioned",
		Ready:               true,
		ControlPlaneReady:   true,
		InfrastructureReady: true,
		NodeCount:           5,
	}

	checks := buildHealthChecks(cluster)

	require.Len(t, checks, 4)

	// Verify control plane check
	cpCheck := checks[0]
	assert.Equal(t, "control-plane-ready", cpCheck.Name)
	assert.Equal(t, CheckStatusPass, cpCheck.Status)

	// Verify infrastructure check
	infraCheck := checks[1]
	assert.Equal(t, "infrastructure-ready", infraCheck.Name)
	assert.Equal(t, CheckStatusPass, infraCheck.Status)

	// Verify phase check
	phaseCheck := checks[2]
	assert.Equal(t, "cluster-phase", phaseCheck.Name)
	assert.Equal(t, CheckStatusPass, phaseCheck.Status)

	// Verify nodes check
	nodeCheck := checks[3]
	assert.Equal(t, "nodes", nodeCheck.Name)
	assert.Equal(t, CheckStatusPass, nodeCheck.Status)
	assert.Contains(t, nodeCheck.Message, "5")
}

func TestHandleFederationError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		operation       string
		expectedMessage string
	}{
		{
			name:            "cluster not found",
			err:             &federation.ClusterNotFoundError{ClusterName: "test", Reason: "not found"},
			operation:       "get cluster",
			expectedMessage: "cluster access denied or unavailable", // Generic message for security
		},
		{
			name:            "discovery error",
			err:             &federation.ClusterDiscoveryError{Reason: "CRD not installed"},
			operation:       "list clusters",
			expectedMessage: "this management cluster does not have CAPI installed",
		},
		{
			name:            "user info required",
			err:             federation.ErrUserInfoRequired,
			operation:       "list clusters",
			expectedMessage: "authentication required for CAPI operations",
		},
		{
			name:            "manager closed",
			err:             federation.ErrManagerClosed,
			operation:       "list clusters",
			expectedMessage: "federation manager is unavailable",
		},
		{
			name:            "generic error",
			err:             errors.New("some internal error"),
			operation:       "list clusters",
			expectedMessage: "failed to list clusters: an unexpected error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleFederationError(tt.err, tt.operation)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Contains(t, getResultText(result), tt.expectedMessage)
		})
	}
}

func TestFormatJSONResult(t *testing.T) {
	output := ClusterListOutput{
		Clusters: []ClusterListItem{
			{Name: "test-cluster", Status: "Provisioned"},
		},
		TotalCount: 1,
	}

	result, err := formatJSONResult(output)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify it's valid JSON
	text := getResultText(result)
	var parsed ClusterListOutput
	err = json.Unmarshal([]byte(text), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", parsed.Clusters[0].Name)
}

func TestHandleListClustersNoFederation(t *testing.T) {
	// Create a mock ServerContext without federation enabled
	// We can't easily test this without mocking ServerContext,
	// but we can test the error message format
	result, err := handleFederationError(federation.ErrManagerClosed, "list clusters")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestResolveClusterPattern(t *testing.T) {
	clusters := createTestClusters()

	tests := []struct {
		name           string
		pattern        string
		expectResolved bool
		expectCount    int
		expectName     string
	}{
		{
			name:           "exact match",
			pattern:        "prod-wc-01",
			expectResolved: true,
			expectCount:    1,
			expectName:     "prod-wc-01",
		},
		{
			name:           "partial match single",
			pattern:        "staging",
			expectResolved: true,
			expectCount:    1,
			expectName:     "staging-wc",
		},
		{
			name:           "partial match multiple",
			pattern:        "wc",
			expectResolved: false,
			expectCount:    2, // prod-wc-01 and staging-wc
			expectName:     "",
		},
		{
			name:           "case insensitive",
			pattern:        "PROD",
			expectResolved: true,
			expectCount:    1,
			expectName:     "prod-wc-01",
		},
		{
			name:           "no match",
			pattern:        "nonexistent",
			expectResolved: false,
			expectCount:    0,
			expectName:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, matches := resolveClusterPattern(clusters, tt.pattern)

			assert.Len(t, matches, tt.expectCount)

			if tt.expectResolved {
				require.NotNil(t, resolved)
				assert.Equal(t, tt.expectName, resolved.Name)
			} else {
				if tt.expectCount != 1 {
					assert.Nil(t, resolved)
				}
			}
		})
	}
}

func TestDetermineOverallHealth(t *testing.T) {
	tests := []struct {
		name           string
		cluster        *federation.ClusterSummary
		components     ClusterHealthComponents
		expectedStatus string
	}{
		{
			name: "all healthy",
			cluster: &federation.ClusterSummary{
				Status: "Provisioned",
				Ready:  true,
			},
			components: ClusterHealthComponents{
				ControlPlane:   ComponentHealth{Status: ComponentStatusHealthy},
				Infrastructure: ComponentHealth{Status: ComponentStatusHealthy},
			},
			expectedStatus: HealthStatusHealthy,
		},
		{
			name: "control plane unhealthy",
			cluster: &federation.ClusterSummary{
				Status: "Provisioned",
				Ready:  false,
			},
			components: ClusterHealthComponents{
				ControlPlane:   ComponentHealth{Status: ComponentStatusUnhealthy},
				Infrastructure: ComponentHealth{Status: ComponentStatusHealthy},
			},
			expectedStatus: HealthStatusDegraded,
		},
		{
			name: "both unhealthy",
			cluster: &federation.ClusterSummary{
				Status: "Failed",
				Ready:  false,
			},
			components: ClusterHealthComponents{
				ControlPlane:   ComponentHealth{Status: ComponentStatusUnhealthy},
				Infrastructure: ComponentHealth{Status: ComponentStatusUnhealthy},
			},
			expectedStatus: HealthStatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := determineOverallHealth(tt.cluster, tt.components)
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{1000, "1000"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// getResultText extracts text content from a tool result.
func getResultText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// Handler Tests - These test the full handler functions with ServerContext

func TestHandleListClusters_NoFederation(t *testing.T) {
	ctx := context.Background()

	// Create server context WITHOUT federation manager
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleListClusters(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "federation mode is not enabled")
}

func TestHandleListClusters_NoUserInfo(t *testing.T) {
	ctx := context.Background()

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleListClusters(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "authentication required")
}

func TestHandleListClusters_Success(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleListClusters(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Parse the response JSON
	var response ClusterListOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 3, response.TotalCount)
	assert.Len(t, response.Clusters, 3)
}

func TestHandleListClusters_WithFilters(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"organization": "org-acme",
		"provider":     "aws",
		"readyOnly":    true,
	}

	result, err := handleListClusters(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterListOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.True(t, response.FilterApplied)
	assert.Equal(t, 2, response.TotalCount)
}

func TestHandleListClusters_Error(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ListClustersErr: &federation.ClusterDiscoveryError{Reason: "CRD not installed"},
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleListClusters(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "does not have CAPI installed")
}

func TestHandleGetCluster_NoFederation(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "prod-wc-01",
	}

	result, err := handleGetCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "federation mode is not enabled")
}

func TestHandleGetCluster_MissingName(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleGetCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "name parameter is required")
}

func TestHandleGetCluster_Success(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "prod-wc-01",
	}

	result, err := handleGetCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterDetailOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "prod-wc-01", response.Name)
	assert.Equal(t, "org-acme", response.Namespace)
	assert.Equal(t, "aws", response.Metadata.Provider)
	assert.Equal(t, "Provisioned", response.Status.Phase)
	assert.True(t, response.Status.Ready)
}

func TestHandleGetCluster_NotFound(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "nonexistent-cluster",
	}

	result, err := handleGetCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	// Generic message for security
	assert.Contains(t, getResultText(result), "cluster access denied or unavailable")
}

func TestHandleResolveCluster_NoFederation(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"pattern": "prod",
	}

	result, err := handleResolveCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "federation mode is not enabled")
}

func TestHandleResolveCluster_MissingPattern(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleResolveCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "pattern parameter is required")
}

func TestHandleResolveCluster_ExactMatch(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"pattern": "prod-wc-01",
	}

	result, err := handleResolveCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterResolveOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.True(t, response.Resolved)
	require.NotNil(t, response.Cluster)
	assert.Equal(t, "prod-wc-01", response.Cluster.Name)
	assert.Contains(t, response.Message, "resolved to cluster")
}

func TestHandleResolveCluster_PartialMatch(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"pattern": "staging",
	}

	result, err := handleResolveCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterResolveOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.True(t, response.Resolved)
	require.NotNil(t, response.Cluster)
	assert.Equal(t, "staging-wc", response.Cluster.Name)
}

func TestHandleResolveCluster_MultipleMatches(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"pattern": "wc", // Matches "prod-wc-01" and "staging-wc"
	}

	result, err := handleResolveCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterResolveOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.False(t, response.Resolved)
	assert.Nil(t, response.Cluster)
	assert.Len(t, response.Matches, 2)
	assert.Contains(t, response.Message, "Multiple clusters match")
}

func TestHandleResolveCluster_NoMatch(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		Clusters: testdata.CreateTestClusters(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"pattern": "nonexistent",
	}

	result, err := handleResolveCluster(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterResolveOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.False(t, response.Resolved)
	assert.Contains(t, response.Message, "No clusters match")
}

func TestHandleClusterHealth_NoFederation(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "prod-wc-01",
	}

	result, err := handleClusterHealth(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "federation mode is not enabled")
}

func TestHandleClusterHealth_MissingName(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handleClusterHealth(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "name parameter is required")
}

func TestHandleClusterHealth_Success(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "prod-wc-01",
	}

	result, err := handleClusterHealth(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterHealthOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "prod-wc-01", response.Name)
	assert.Equal(t, HealthStatusHealthy, response.Status)
	assert.Equal(t, ComponentStatusHealthy, response.Components.ControlPlane.Status)
	assert.Equal(t, ComponentStatusHealthy, response.Components.Infrastructure.Status)
	assert.NotEmpty(t, response.Checks)
}

func TestHandleClusterHealth_UnhealthyCluster(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "dev-cluster", // This is provisioning/not ready
	}

	result, err := handleClusterHealth(ctx, request, sc)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response ClusterHealthOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "dev-cluster", response.Name)
	assert.Equal(t, HealthStatusDegraded, response.Status)
	assert.Equal(t, ComponentStatusUnhealthy, response.Components.Infrastructure.Status)
}

func TestHandleClusterHealth_NotFound(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	mockManager := &testdata.MockFederationManager{
		ClusterDetails: testdata.CreateTestClusterDetailsMap(),
	}

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithFederationManager(mockManager),
	)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"name": "nonexistent-cluster",
	}

	result, err := handleClusterHealth(ctx, request, sc)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "cluster access denied or unavailable")
}

func TestGetUserFromContext_NoUser(t *testing.T) {
	ctx := context.Background()

	user, errMsg := getUserFromContext(ctx)
	assert.Nil(t, user)
	assert.Contains(t, errMsg, "authentication required")
}

func TestGetUserFromContext_WithUser(t *testing.T) {
	ctx := contextWithUserInfo("test@example.com", []string{"developers"})

	user, errMsg := getUserFromContext(ctx)
	require.Empty(t, errMsg)
	require.NotNil(t, user)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Contains(t, user.Groups, "developers")
}
