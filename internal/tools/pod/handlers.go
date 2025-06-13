package pod

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleGetLogs handles kubectl logs operations
func handleGetLogs(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	podName, ok := args["podName"].(string)
	if !ok || podName == "" {
		return mcp.NewToolResultError("podName is required"), nil
	}

	containerName, _ := args["containerName"].(string)

	follow, _ := args["follow"].(bool)
	previous, _ := args["previous"].(bool)
	timestamps, _ := args["timestamps"].(bool)

	var tailLines *int64
	if tailLinesFloat, ok := args["tailLines"].(float64); ok {
		tailLinesInt := int64(tailLinesFloat)
		tailLines = &tailLinesInt
	}

	opts := k8s.LogOptions{
		Follow:     follow,
		Previous:   previous,
		Timestamps: timestamps,
		TailLines:  tailLines,
	}

	reader, err := sc.K8sClient().GetLogs(ctx, kubeContext, namespace, podName, containerName, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get logs: %v", err)), nil
	}
	defer reader.Close()

	// Read logs into a string
	logsBytes, err := io.ReadAll(reader)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read logs: %v", err)), nil
	}

	return mcp.NewToolResultText(string(logsBytes)), nil
}

// handleExec handles kubectl exec operations
func handleExec(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	podName, ok := args["podName"].(string)
	if !ok || podName == "" {
		return mcp.NewToolResultError("podName is required"), nil
	}

	containerName, _ := args["containerName"].(string)

	commandInterface, ok := args["command"]
	if !ok || commandInterface == nil {
		return mcp.NewToolResultError("command is required"), nil
	}

	// Convert command interface to []string
	var command []string
	if commandSlice, ok := commandInterface.([]interface{}); ok {
		for _, item := range commandSlice {
			if str, ok := item.(string); ok {
				command = append(command, str)
			}
		}
	} else {
		return mcp.NewToolResultError("command must be an array of strings"), nil
	}

	if len(command) == 0 {
		return mcp.NewToolResultError("command cannot be empty"), nil
	}

	tty, _ := args["tty"].(bool)

	opts := k8s.ExecOptions{
		TTY: tty,
	}

	result, err := sc.K8sClient().Exec(ctx, kubeContext, namespace, podName, containerName, command, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to execute command: %v", err)), nil
	}

	// Format the result
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Exit Code: %d\n", result.ExitCode))
	if result.Stdout != "" {
		output.WriteString(fmt.Sprintf("Stdout:\n%s\n", result.Stdout))
	}
	if result.Stderr != "" {
		output.WriteString(fmt.Sprintf("Stderr:\n%s\n", result.Stderr))
	}

	return mcp.NewToolResultText(output.String()), nil
}

// handlePortForward handles kubectl port-forward operations
func handlePortForward(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	kubeContext, _ := args["kubeContext"].(string)

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	// Get resource type (default to "pod" for backward compatibility)
	resourceType, _ := args["resourceType"].(string)
	if resourceType == "" {
		resourceType = "pod"
	}

	// Get resource name (support both old "podName" and new "resourceName" for backward compatibility)
	resourceName, ok := args["resourceName"].(string)
	if !ok || resourceName == "" {
		// Fall back to "podName" for backward compatibility
		if podName, exists := args["podName"].(string); exists && podName != "" {
			resourceName = podName
			resourceType = "pod" // Ensure it's treated as a pod
		} else {
			return mcp.NewToolResultError("resourceName is required"), nil
		}
	}

	portsInterface, ok := args["ports"]
	if !ok || portsInterface == nil {
		return mcp.NewToolResultError("ports is required"), nil
	}

	// Convert ports interface to []string
	var ports []string
	if portsSlice, ok := portsInterface.([]interface{}); ok {
		for _, item := range portsSlice {
			if str, ok := item.(string); ok {
				ports = append(ports, str)
			}
		}
	} else {
		return mcp.NewToolResultError("ports must be an array of strings"), nil
	}

	if len(ports) == 0 {
		return mcp.NewToolResultError("ports cannot be empty"), nil
	}

	opts := k8s.PortForwardOptions{}

	var session *k8s.PortForwardSession
	var err error
	var sessionID string

	// Create a context with a shorter timeout for the initial setup
	setupCtx, setupCancel := context.WithTimeout(ctx, 10*time.Second)
	defer setupCancel()

	// Handle port forwarding based on resource type
	switch resourceType {
	case "pod":
		session, err = sc.K8sClient().PortForward(setupCtx, kubeContext, namespace, resourceName, ports, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to setup port forwarding to pod: %v", err)), nil
		}
		sessionID = fmt.Sprintf("%s/%s:%s", namespace, resourceName, strings.Join(ports, ","))

	case "service":
		session, err = sc.K8sClient().PortForwardToService(setupCtx, kubeContext, namespace, resourceName, ports, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to setup port forwarding to service: %v", err)), nil
		}
		sessionID = fmt.Sprintf("%s/service/%s:%s", namespace, resourceName, strings.Join(ports, ","))

	default:
		return mcp.NewToolResultError(fmt.Sprintf("Invalid resource type: %s. Must be 'pod' or 'service'", resourceType)), nil
	}

	// Register the session for cleanup during shutdown
	sc.RegisterPortForwardSession(sessionID, session)

	// Format the result
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Port forwarding session established to %s %s:\n", resourceType, resourceName))
	for i, localPort := range session.LocalPorts {
		if i < len(session.RemotePorts) {
			output.WriteString(fmt.Sprintf("Local port %d -> Remote port %d\n", localPort, session.RemotePorts[i]))
		}
	}
	output.WriteString(fmt.Sprintf("\nSession ID: %s\n", sessionID))
	output.WriteString("Note: This is a long-running session. Use 'list_port_forward_sessions' to view active sessions and 'stop_port_forward_session' to stop this session.")

	return mcp.NewToolResultText(output.String()), nil
}

// handleListPortForwardSessions handles listing all active port forwarding sessions
func handleListPortForwardSessions(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	sessions := sc.GetActiveSessions()

	if len(sessions) == 0 {
		return mcp.NewToolResultText("No active port forwarding sessions."), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Active port forwarding sessions (%d):\n\n", len(sessions)))

	for sessionID, session := range sessions {
		output.WriteString(fmt.Sprintf("Session ID: %s\n", sessionID))
		output.WriteString("Port mappings:\n")
		for i, localPort := range session.LocalPorts {
			if i < len(session.RemotePorts) {
				output.WriteString(fmt.Sprintf("  Local port %d -> Remote port %d\n", localPort, session.RemotePorts[i]))
			}
		}
		output.WriteString("\n")
	}

	return mcp.NewToolResultText(output.String()), nil
}

// handleStopPortForwardSession handles stopping a specific port forwarding session
func handleStopPortForwardSession(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	sessionID, ok := args["sessionID"].(string)
	if !ok || sessionID == "" {
		return mcp.NewToolResultError("sessionID is required"), nil
	}

	err := sc.StopPortForwardSession(sessionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to stop session: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Port forwarding session %s stopped successfully.", sessionID)), nil
}

// handleStopAllPortForwardSessions handles stopping all active port forwarding sessions
func handleStopAllPortForwardSessions(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	count := sc.StopAllPortForwardSessions()

	if count == 0 {
		return mcp.NewToolResultText("No active port forwarding sessions to stop."), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Stopped %d port forwarding session(s) successfully.", count)), nil
}
