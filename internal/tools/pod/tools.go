package pod

import (
	"context"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterPodTools registers all pod management tools with the MCP server
func RegisterPodTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// kubernetes_logs tool
	logsTool := mcp.NewTool("kubernetes_logs",
		mcp.WithDescription("Get logs from a pod container"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
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
	)

	s.AddTool(logsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetLogs(ctx, request, sc)
	})

	// kubernetes_exec tool
	execTool := mcp.NewTool("kubernetes_exec",
		mcp.WithDescription("Execute a command inside a pod container"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
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
		),
		mcp.WithBoolean("tty",
			mcp.Description("Allocate a TTY for the exec session (default: false)"),
		),
	)

	s.AddTool(execTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleExec(ctx, request, sc)
	})

	// port_forward tool
	portForwardTool := mcp.NewTool("port_forward",
		mcp.WithDescription("Port-forward to a pod or service"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
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
		),
	)

	s.AddTool(portForwardTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handlePortForward(ctx, request, sc)
	})

	// list_port_forward_sessions tool
	listSessionsTool := mcp.NewTool("list_port_forward_sessions",
		mcp.WithDescription("List all active port forwarding sessions"),
	)

	s.AddTool(listSessionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListPortForwardSessions(ctx, request, sc)
	})

	// stop_port_forward_session tool
	stopSessionTool := mcp.NewTool("stop_port_forward_session",
		mcp.WithDescription("Stop a specific port forwarding session by ID"),
		mcp.WithString("sessionID",
			mcp.Required(),
			mcp.Description("ID of the port forwarding session to stop"),
		),
	)

	s.AddTool(stopSessionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleStopPortForwardSession(ctx, request, sc)
	})

	// stop_all_port_forward_sessions tool
	stopAllSessionsTool := mcp.NewTool("stop_all_port_forward_sessions",
		mcp.WithDescription("Stop all active port forwarding sessions"),
	)

	s.AddTool(stopAllSessionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleStopAllPortForwardSessions(ctx, request, sc)
	})

	return nil
}
