package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
)

// Resource type constant for default resource type in port-forward operations.
const defaultResourceTypePod = "pod"

// PortForwardResponse represents the structured response for port forwarding operations
type PortForwardResponse struct {
	Success      bool          `json:"success"`
	Message      string        `json:"message"`
	SessionID    string        `json:"sessionId"`
	ResourceType string        `json:"resourceType"`
	ResourceName string        `json:"resourceName"`
	Namespace    string        `json:"namespace"`
	PortMappings []PortMapping `json:"portMappings"`
	Instructions string        `json:"instructions"`
}

// PortMapping represents a single port mapping
type PortMapping struct {
	LocalPort  int `json:"localPort"`
	RemotePort int `json:"remotePort"`
}

// recordPodOperation records metrics for a pod operation.
// Delegates to ServerContext which handles nil checks internally.
func recordPodOperation(ctx context.Context, sc *server.ServerContext, operation, namespace, status string, duration time.Duration) {
	sc.RecordPodOperation(ctx, operation, namespace, status, duration)
}

// incrementActiveSessions increments the active port-forward sessions counter.
// Delegates to ServerContext which handles nil checks internally.
func incrementActiveSessions(ctx context.Context, sc *server.ServerContext) {
	sc.IncrementActiveSessions(ctx)
}

// decrementActiveSessions decrements the active port-forward sessions counter.
// Delegates to ServerContext which handles nil checks internally.
func decrementActiveSessions(ctx context.Context, sc *server.ServerContext) {
	sc.DecrementActiveSessions(ctx)
}

// handleGetLogs handles kubectl logs operations
func handleGetLogs(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

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

	var tailLines, sinceLines, maxLines *int64

	if tailLinesVal, ok := args["tailLines"]; ok {
		if tailLinesFloat, ok := tailLinesVal.(float64); ok {
			val := int64(tailLinesFloat)
			tailLines = &val
		}
	}

	if sinceLinesVal, ok := args["sinceLines"]; ok {
		if sinceLinesFloat, ok := sinceLinesVal.(float64); ok {
			val := int64(sinceLinesFloat)
			sinceLines = &val
		}
	}

	if maxLinesVal, ok := args["maxLines"]; ok {
		if maxLinesFloat, ok := maxLinesVal.(float64); ok {
			val := int64(maxLinesFloat)
			maxLines = &val
		}
	}

	opts := k8s.LogOptions{
		Follow:     follow,
		Previous:   previous,
		Timestamps: timestamps,
		TailLines:  tailLines,
		SinceLines: sinceLines,
		MaxLines:   maxLines,
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	logs, err := k8sClient.GetLogs(ctx, kubeContext, namespace, podName, containerName, opts)
	duration := time.Since(start)

	if err != nil {
		recordPodOperation(ctx, sc, instrumentation.OperationLogs, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get logs: %v", err)), nil
	}
	defer func() { _ = logs.Close() }()

	recordPodOperation(ctx, sc, instrumentation.OperationLogs, namespace, instrumentation.StatusSuccess, duration)

	// Read logs and apply pagination if needed
	logData, err := io.ReadAll(logs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read logs: %v", err)), nil
	}

	logText := string(logData)

	// Apply pagination if sinceLines or maxLines are specified
	if sinceLines != nil || maxLines != nil {
		lines := strings.Split(logText, "\n")

		startIdx := 0
		if sinceLines != nil && *sinceLines > 0 {
			startIdx = int(*sinceLines)
			if startIdx >= len(lines) {
				// If sinceLines is beyond available lines, return empty
				return mcp.NewToolResultText(""), nil
			}
		}

		endIdx := len(lines)
		if maxLines != nil && *maxLines > 0 {
			endIdx = startIdx + int(*maxLines)
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
		}

		if startIdx < endIdx {
			paginatedLines := lines[startIdx:endIdx]
			logText = strings.Join(paginatedLines, "\n")
		} else {
			logText = ""
		}
	}

	return mcp.NewToolResultText(logText), nil
}

// handleExec handles kubectl exec operations
func handleExec(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

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

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	result, err := k8sClient.Exec(ctx, kubeContext, namespace, podName, containerName, command, opts)
	duration := time.Since(start)

	if err != nil {
		recordPodOperation(ctx, sc, instrumentation.OperationExec, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to execute command: %v", err)), nil
	}

	recordPodOperation(ctx, sc, instrumentation.OperationExec, namespace, instrumentation.StatusSuccess, duration)

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

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	// Get resource type (default to "pod" for backward compatibility)
	resourceType, _ := args["resourceType"].(string)
	if resourceType == "" {
		resourceType = defaultResourceTypePod
	}

	// Get resource name (support both old "podName" and new "resourceName" for backward compatibility)
	resourceName, ok := args["resourceName"].(string)
	if !ok || resourceName == "" {
		// Fall back to "podName" for backward compatibility
		if podName, exists := args["podName"].(string); exists && podName != "" {
			resourceName = podName
			resourceType = defaultResourceTypePod // Ensure it's treated as a pod
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

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	// Handle port forwarding based on resource type
	switch resourceType {
	case defaultResourceTypePod:
		session, err = k8sClient.PortForward(setupCtx, kubeContext, namespace, resourceName, ports, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to setup port forwarding to pod: %v", err)), nil
		}
		sessionID = fmt.Sprintf("%s/%s:%s", namespace, resourceName, strings.Join(ports, ","))

	case "service":
		session, err = k8sClient.PortForwardToService(setupCtx, kubeContext, namespace, resourceName, ports, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to setup port forwarding to service: %v", err)), nil
		}
		sessionID = fmt.Sprintf("%s/service/%s:%s", namespace, resourceName, strings.Join(ports, ","))

	default:
		return mcp.NewToolResultError(fmt.Sprintf("Invalid resource type: %s. Must be 'pod' or 'service'", resourceType)), nil
	}

	// Register the session for cleanup during shutdown
	sc.RegisterPortForwardSession(sessionID, session)

	// Increment active sessions metric
	incrementActiveSessions(ctx, sc)

	// Create port mappings
	var portMappings []PortMapping
	for i, localPort := range session.LocalPorts {
		if i < len(session.RemotePorts) {
			portMappings = append(portMappings, PortMapping{
				LocalPort:  localPort,
				RemotePort: session.RemotePorts[i],
			})
		}
	}

	// Create structured response
	response := PortForwardResponse{
		Success:      true,
		Message:      fmt.Sprintf("Port forwarding session established to %s %s", resourceType, resourceName),
		SessionID:    sessionID,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Namespace:    namespace,
		PortMappings: portMappings,
		Instructions: "This is a long-running session. Use 'list_port_forward_sessions' to view active sessions and 'stop_port_forward_session' to stop this session.",
	}

	// Marshal response to JSON
	jsonResponse, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonResponse)), nil
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

	// Decrement active sessions metric
	decrementActiveSessions(ctx, sc)

	return mcp.NewToolResultText(fmt.Sprintf("Port forwarding session %s stopped successfully.", sessionID)), nil
}

// handleStopAllPortForwardSessions handles stopping all active port forwarding sessions
func handleStopAllPortForwardSessions(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	count := sc.StopAllPortForwardSessions()

	if count == 0 {
		return mcp.NewToolResultText("No active port forwarding sessions to stop."), nil
	}

	// Decrement active sessions metric for all stopped sessions
	for i := 0; i < count; i++ {
		decrementActiveSessions(ctx, sc)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Stopped %d port forwarding session(s) successfully.", count)), nil
}
