package pod

import (
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// RegisterPodTools registers all pod management tools with the MCP server
func RegisterPodTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// Get cluster/context parameters based on server mode
	clusterContextParams := tools.AddClusterContextParams(sc)

	// kubernetes_logs tool
	logsOpts := []mcp.ToolOption{
		mcp.WithDescription("Get logs from a pod container"),
	}
	logsOpts = append(logsOpts, clusterContextParams...)
	logsOpts = append(logsOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the pod is located"),
		),
		mcp.WithString("podName",
			mcp.Required(),
			mcp.Description("Name of the pod to get logs from"),
		),
		mcp.WithString("containerName",
			mcp.Description("Name of the container (optional for single-container pods)"),
		),
		mcp.WithBoolean("follow",
			mcp.Description("Follow log output (default: false)"),
		),
		mcp.WithBoolean("previous",
			mcp.Description("Get logs from previous container instance (default: false)"),
		),
		mcp.WithBoolean("timestamps",
			mcp.Description("Include timestamps in log output (default: false)"),
		),
		mcp.WithNumber("tailLines",
			mcp.Description("Number of lines from the end of logs to show (optional)"),
		),
		mcp.WithNumber("sinceLines",
			mcp.Description("Skip this many lines from the beginning (useful for pagination, optional)"),
		),
		mcp.WithNumber("maxLines",
			mcp.Description("Maximum number of lines to return (useful for pagination, optional)"),
		),
	)
	logsTool := mcp.NewTool("kubernetes_logs", logsOpts...)

	s.AddTool(logsTool, tools.WrapWithAuditLogging("kubernetes_logs", handleGetLogs, sc))

	// kubernetes_exec tool
	execOpts := []mcp.ToolOption{
		mcp.WithDescription("Execute a command inside a pod container"),
	}
	execOpts = append(execOpts, clusterContextParams...)
	execOpts = append(execOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the pod is located"),
		),
		mcp.WithString("podName",
			mcp.Required(),
			mcp.Description("Name of the pod to execute command in"),
		),
		mcp.WithString("containerName",
			mcp.Description("Name of the container (optional for single-container pods)"),
		),
		mcp.WithArray("command",
			mcp.Required(),
			mcp.Description("Command to execute as an array of strings"),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("tty",
			mcp.Description("Allocate a TTY for the exec session (default: false)"),
		),
	)
	execTool := mcp.NewTool("kubernetes_exec", execOpts...)

	s.AddTool(execTool, tools.WrapWithAuditLogging("kubernetes_exec", handleExec, sc))

	// port_forward tool
	portForwardOpts := []mcp.ToolOption{
		mcp.WithDescription("Port-forward to a pod or service"),
	}
	portForwardOpts = append(portForwardOpts, clusterContextParams...)
	portForwardOpts = append(portForwardOpts,
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace where the resource is located"),
		),
		mcp.WithString("resourceType",
			mcp.Description("Type of resource to port-forward to: 'pod' or 'service' (default: 'pod')"),
			mcp.Enum("pod", "service"),
		),
		mcp.WithString("resourceName",
			mcp.Required(),
			mcp.Description("Name of the pod or service to port-forward to"),
		),
		mcp.WithArray("ports",
			mcp.Required(),
			mcp.Description("Port mappings as array of strings (e.g., ['8080:80', '9090:9090'])"),
			mcp.WithStringItems(),
		),
	)
	portForwardTool := mcp.NewTool("port_forward", portForwardOpts...)

	s.AddTool(portForwardTool, tools.WrapWithAuditLogging("port_forward", handlePortForward, sc))

	// list_port_forward_sessions tool
	listSessionsTool := mcp.NewTool("list_port_forward_sessions",
		mcp.WithDescription("List all active port forwarding sessions"),
		mcp.WithInputSchema[tools.EmptyRequest](),
	)

	s.AddTool(listSessionsTool, tools.WrapWithAuditLogging("list_port_forward_sessions", handleListPortForwardSessions, sc))

	// stop_port_forward_session tool
	stopSessionTool := mcp.NewTool("stop_port_forward_session",
		mcp.WithDescription("Stop a specific port forwarding session by ID"),
		mcp.WithString("sessionID",
			mcp.Required(),
			mcp.Description("ID of the port forwarding session to stop"),
		),
	)

	s.AddTool(stopSessionTool, tools.WrapWithAuditLogging("stop_port_forward_session", handleStopPortForwardSession, sc))

	// stop_all_port_forward_sessions tool
	stopAllSessionsTool := mcp.NewTool("stop_all_port_forward_sessions",
		mcp.WithDescription("Stop all active port forwarding sessions"),
		mcp.WithInputSchema[tools.EmptyRequest](),
	)

	s.AddTool(stopAllSessionsTool, tools.WrapWithAuditLogging("stop_all_port_forward_sessions", handleStopAllPortForwardSessions, sc))

	return nil
}
