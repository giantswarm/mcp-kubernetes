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
	
	podName, ok := args["podName"].(string)
	if !ok || podName == "" {
		return mcp.NewToolResultError("podName is required"), nil
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

	session, err := sc.K8sClient().PortForward(ctx, kubeContext, namespace, podName, ports, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to setup port forwarding: %v", err)), nil
	}

	// Wait a moment for the port forward to be ready
	select {
	case <-session.ReadyChan:
		// Port forward is ready
	case <-time.After(5 * time.Second):
		return mcp.NewToolResultError("Port forward setup timed out"), nil
	}

	// Format the result
	var output strings.Builder
	output.WriteString("Port forwarding session established:\n")
	for i, localPort := range session.LocalPorts {
		if i < len(session.RemotePorts) {
			output.WriteString(fmt.Sprintf("Local port %d -> Remote port %d\n", localPort, session.RemotePorts[i]))
		}
	}
	output.WriteString("\nNote: This is a long-running session. Use Ctrl+C or close the connection to stop port forwarding.")

	return mcp.NewToolResultText(output.String()), nil
} 