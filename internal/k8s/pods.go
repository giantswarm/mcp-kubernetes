package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

// PodManager implementation

// GetLogs retrieves logs from a pod container.
func (c *kubernetesClient) GetLogs(ctx context.Context, kubeContext, namespace, podName, containerName string, opts LogOptions) (io.ReadCloser, error) {
	// Validate operation
	if err := c.isOperationAllowed("logs"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	c.logOperation("get-logs", kubeContext, namespace, "pod", podName)

	// Get clientset for the context
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return nil, err
	}

	// Build log options
	logOpts := &corev1.PodLogOptions{
		Container:  containerName,
		Follow:     opts.Follow,
		Previous:   opts.Previous,
		Timestamps: opts.Timestamps,
	}

	if opts.SinceTime != nil {
		logOpts.SinceTime = &metav1.Time{Time: *opts.SinceTime}
	}

	if opts.TailLines != nil {
		logOpts.TailLines = opts.TailLines
	}

	// Get logs request
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, logOpts)

	// Execute the request
	logs, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for pod %s/%s: %w", namespace, podName, err)
	}

	return logs, nil
}

// Exec executes a command inside a pod container.
func (c *kubernetesClient) Exec(ctx context.Context, kubeContext, namespace, podName, containerName string, command []string, opts ExecOptions) (*ExecResult, error) {
	// Validate operation
	if err := c.isOperationAllowed("exec"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	c.logOperation("exec", kubeContext, namespace, "pod", podName)

	// Get clientset and rest config for the context
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return nil, err
	}

	restConfig, err := c.getRestConfig(kubeContext)
	if err != nil {
		return nil, err
	}

	// Build exec request
	execReq := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)

	// Create SPDY executor
	exec, err := remotecommand.NewSPDYExecutor(restConfig, http.MethodPost, execReq.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	// Create streams
	streamOpts := remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Tty:    opts.TTY,
	}

	// Execute the command
	err = exec.StreamWithContext(ctx, streamOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to execute command in pod %s/%s: %w", namespace, podName, err)
	}

	// Create result (note: exit code extraction would require more complex implementation)
	result := &ExecResult{
		ExitCode: 0, // TODO: Extract actual exit code
	}

	return result, nil
}

// PortForward sets up port forwarding to a pod.
func (c *kubernetesClient) PortForward(ctx context.Context, kubeContext, namespace, podName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	// Validate operation
	if err := c.isOperationAllowed("port-forward"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	c.logOperation("port-forward", kubeContext, namespace, "pod", podName)

	// Get rest config for the context
	restConfig, err := c.getRestConfig(kubeContext)
	if err != nil {
		return nil, err
	}

	// Parse ports
	localPorts := make([]int, len(ports))
	remotePorts := make([]int, len(ports))

	for i, port := range ports {
		if strings.Contains(port, ":") {
			// Format: localPort:remotePort
			parts := strings.Split(port, ":")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port format: %s", port)
			}

			localPort, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid local port: %s", parts[0])
			}

			remotePort, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid remote port: %s", parts[1])
			}

			localPorts[i] = localPort
			remotePorts[i] = remotePort
		} else {
			// Format: port (same for local and remote)
			port, err := strconv.Atoi(port)
			if err != nil {
				return nil, fmt.Errorf("invalid port: %s", ports[i])
			}

			localPorts[i] = port
			remotePorts[i] = port
		}
	}

	// Build port forward request
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	hostIP := strings.TrimLeft(restConfig.Host, "https://")

	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}

	// Create SPDY roundtripper
	roundTripper, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}

	// Create dialer
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

	// Create channels for control
	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)

	// Setup streams
	stdout := opts.Stdout
	stderr := opts.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	// Create port forwarder
	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	// Start port forwarding in background
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			if c.config.Logger != nil {
				c.config.Logger.Error("port forwarding error", "error", err)
			}
		}
	}()

	// Wait for ready signal or context cancellation
	select {
	case <-readyChan:
		// Port forwarding is ready
	case <-ctx.Done():
		close(stopChan)
		return nil, fmt.Errorf("port forwarding cancelled: %w", ctx.Err())
	}

	// Get the actual forwarded ports
	forwardedPorts, err := forwarder.GetPorts()
	if err != nil {
		close(stopChan)
		return nil, fmt.Errorf("failed to get forwarded ports: %w", err)
	}

	// Extract local and remote ports from forwarded ports
	actualLocalPorts := make([]int, len(forwardedPorts))
	actualRemotePorts := make([]int, len(forwardedPorts))

	for i, port := range forwardedPorts {
		actualLocalPorts[i] = int(port.Local)
		actualRemotePorts[i] = int(port.Remote)
	}

	// Create session
	session := &PortForwardSession{
		LocalPorts:  actualLocalPorts,
		RemotePorts: actualRemotePorts,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		Forwarder:   forwarder,
	}

	return session, nil
}

// Helper methods for pod operations

// validatePodExists checks if a pod exists in the specified namespace.
func (c *kubernetesClient) validatePodExists(ctx context.Context, kubeContext, namespace, podName string) error {
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("pod %s/%s not found: %w", namespace, podName, err)
	}

	return nil
}

// validateContainerExists checks if a container exists in the specified pod.
func (c *kubernetesClient) validateContainerExists(ctx context.Context, kubeContext, namespace, podName, containerName string) error {
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return err
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("pod %s/%s not found: %w", namespace, podName, err)
	}

	// Check if container exists
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return nil
		}
	}

	// Check init containers as well
	for _, container := range pod.Spec.InitContainers {
		if container.Name == containerName {
			return nil
		}
	}

	return fmt.Errorf("container %q not found in pod %s/%s", containerName, namespace, podName)
}
