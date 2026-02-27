package pod

import (
	"context"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

var portForwardTools = []string{
	"port_forward",
	"list_port_forward_sessions",
	"stop_port_forward_session",
	"stop_all_port_forward_sessions",
}

var alwaysRegisteredTools = []string{
	"kubernetes_logs",
	"kubernetes_exec",
}

func TestRegisterPodTools_LocalMode(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	mcpSrv := mcpserver.NewMCPServer("test", "0.0.1",
		mcpserver.WithToolCapabilities(true),
	)

	err = RegisterPodTools(mcpSrv, sc)
	require.NoError(t, err)

	tools := mcpSrv.ListTools()

	for _, name := range alwaysRegisteredTools {
		assert.Contains(t, tools, name, "tool %s should be registered in local mode", name)
	}

	for _, name := range portForwardTools {
		assert.Contains(t, tools, name, "tool %s should be registered in local mode", name)
	}
}

func TestRegisterPodTools_InClusterMode(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
		server.WithInCluster(true),
	)
	require.NoError(t, err)

	mcpSrv := mcpserver.NewMCPServer("test", "0.0.1",
		mcpserver.WithToolCapabilities(true),
	)

	err = RegisterPodTools(mcpSrv, sc)
	require.NoError(t, err)

	tools := mcpSrv.ListTools()

	for _, name := range alwaysRegisteredTools {
		assert.Contains(t, tools, name, "tool %s should still be registered in in-cluster mode", name)
	}

	for _, name := range portForwardTools {
		assert.NotContains(t, tools, name, "tool %s should NOT be registered in in-cluster mode", name)
	}
}
