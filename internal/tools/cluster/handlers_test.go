package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// healthMock wraps testdata.MockK8sClient and returns a configurable
// ClusterHealth from GetClusterHealth so tests can drive the handler.
type healthMock struct {
	*testdata.MockK8sClient
	health *k8s.ClusterHealth
}

func (m *healthMock) GetClusterHealth(_ context.Context, _ string) (*k8s.ClusterHealth, error) {
	return m.health, nil
}

func newClusterHealthTestServer(t *testing.T, health *k8s.ClusterHealth) *server.ServerContext {
	t.Helper()
	mock := &healthMock{
		MockK8sClient: &testdata.MockK8sClient{},
		health:        health,
	}
	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(mock),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)
	return sc
}

func makeNodes(n int, readyEvery int) []k8s.NodeHealth {
	out := make([]k8s.NodeHealth, 0, n)
	for i := range n {
		ready := readyEvery > 0 && i%readyEvery == 0
		status := corev1.ConditionFalse
		if ready {
			status = corev1.ConditionTrue
		}
		out = append(out, k8s.NodeHealth{
			Name:  fmt.Sprintf("node-%02d", i),
			Ready: ready,
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: status},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
			},
		})
	}
	return out
}

func unmarshalHealth(t *testing.T, result *mcp.CallToolResult) ClusterHealthOutput {
	t.Helper()
	require.False(t, result.IsError, "expected success result")
	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	var out ClusterHealthOutput
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	return out
}

func TestClusterHealth_DefaultsOmitConditionsAndCapAt20(t *testing.T) {
	health := &k8s.ClusterHealth{
		Status:     "Healthy",
		Components: []k8s.ComponentHealth{{Name: "API Server", Status: "Healthy"}},
		Nodes:      makeNodes(30, 1), // all 30 ready
	}
	sc := newClusterHealthTestServer(t, health)

	result, err := handleGetClusterHealth(context.Background(), mcp.CallToolRequest{}, sc)
	require.NoError(t, err)

	out := unmarshalHealth(t, result)
	assert.Equal(t, 30, out.TotalNodes)
	assert.Equal(t, 20, out.ReturnedNodes)
	assert.Equal(t, 30, out.ReadyNodes, "ReadyNodes must reflect the full set, not the truncated slice")
	assert.True(t, out.NodesTruncated)
	assert.Len(t, out.Nodes, 20)
	for _, n := range out.Nodes {
		assert.Nil(t, n.Conditions, "conditions must be omitted by default")
	}
}

func TestClusterHealth_NodesLimitOverride(t *testing.T) {
	health := &k8s.ClusterHealth{
		Status: "Healthy",
		Nodes:  makeNodes(10, 2), // 5 ready, 5 not ready
	}
	sc := newClusterHealthTestServer(t, health)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"nodesLimit": float64(5)}

	result, err := handleGetClusterHealth(context.Background(), req, sc)
	require.NoError(t, err)

	out := unmarshalHealth(t, result)
	assert.Equal(t, 10, out.TotalNodes)
	assert.Equal(t, 5, out.ReturnedNodes)
	assert.Equal(t, 5, out.ReadyNodes, "ReadyNodes counts the full list, regardless of truncation")
	assert.True(t, out.NodesTruncated)
}

func TestClusterHealth_NoTruncationWhenWithinLimit(t *testing.T) {
	health := &k8s.ClusterHealth{
		Status: "Healthy",
		Nodes:  makeNodes(3, 1),
	}
	sc := newClusterHealthTestServer(t, health)

	result, err := handleGetClusterHealth(context.Background(), mcp.CallToolRequest{}, sc)
	require.NoError(t, err)

	out := unmarshalHealth(t, result)
	assert.Equal(t, 3, out.TotalNodes)
	assert.Equal(t, 3, out.ReturnedNodes)
	assert.False(t, out.NodesTruncated, "nodesTruncated must be false when total <= nodesLimit")
}

func TestClusterHealth_NoTruncationWhenLimitEqualsTotal(t *testing.T) {
	health := &k8s.ClusterHealth{
		Status: "Healthy",
		Nodes:  makeNodes(20, 1),
	}
	sc := newClusterHealthTestServer(t, health)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"nodesLimit": float64(20)}

	result, err := handleGetClusterHealth(context.Background(), req, sc)
	require.NoError(t, err)

	out := unmarshalHealth(t, result)
	assert.Equal(t, 20, out.TotalNodes)
	assert.Equal(t, 20, out.ReturnedNodes)
	assert.False(t, out.NodesTruncated, "nodesTruncated must be false when limit == total")
}

func TestClusterHealth_IncludeNodeConditions(t *testing.T) {
	health := &k8s.ClusterHealth{
		Status: "Healthy",
		Nodes:  makeNodes(2, 1),
	}
	sc := newClusterHealthTestServer(t, health)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"includeNodeConditions": true}

	result, err := handleGetClusterHealth(context.Background(), req, sc)
	require.NoError(t, err)

	out := unmarshalHealth(t, result)
	require.Len(t, out.Nodes, 2)
	for _, n := range out.Nodes {
		assert.Len(t, n.Conditions, 2, "conditions should be present when requested")
	}
}

func TestClusterHealth_NodesLimitOutOfRangeReturnsError(t *testing.T) {
	health := &k8s.ClusterHealth{Status: "Healthy"}
	sc := newClusterHealthTestServer(t, health)

	for _, val := range []float64{0, -1, float64(MaxNodesLimit + 1)} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{"nodesLimit": val}

		result, err := handleGetClusterHealth(context.Background(), req, sc)
		require.NoError(t, err)
		assert.True(t, result.IsError, "nodesLimit=%v must be rejected", val)
	}
}
