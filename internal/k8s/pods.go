package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

	// Validate that the pod exists and is running
	if err := c.validatePodRunning(ctx, kubeContext, namespace, podName); err != nil {
		return nil, err
	}

	// Get rest config for the context
	restConfig, err := c.getRestConfig(kubeContext)
	if err != nil {
		return nil, err
	}

	// Get clientset for the context
	clientset, err := c.getClientset(kubeContext)
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

	// Build port forward request using RESTClient (like exec does)
	portForwardReq := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("portforward")

	if c.config.DebugMode && c.config.Logger != nil {
		c.config.Logger.Debug("port forward URL", "url", portForwardReq.URL().String())
	}

	// Create SPDY roundtripper
	roundTripper, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}

	// Create dialer
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, portForwardReq.URL())

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

	// Start port forwarding in background with error handling
	errChan := make(chan error, 1)
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			if c.config.Logger != nil {
				c.config.Logger.Error("port forwarding error", "error", err)
			}
			errChan <- err
		}
	}()

	// Wait for ready signal, error, or context cancellation with timeout
	select {
	case <-readyChan:
		// Port forwarding is ready
		c.config.Logger.Debug("Port forward ready", "namespace", namespace, "pod", podName)
	case err := <-errChan:
		// Port forwarding failed
		close(stopChan)
		return nil, fmt.Errorf("port forwarding failed: %w", err)
	case <-ctx.Done():
		// Context cancelled
		close(stopChan)
		return nil, fmt.Errorf("port forwarding cancelled: %w", ctx.Err())
	}

	// Create session with requested ports (avoid blocking GetPorts() call)
	session := &PortForwardSession{
		LocalPorts:  localPorts,
		RemotePorts: remotePorts,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		Forwarder:   forwarder,
	}

	return session, nil
}

// Helper methods for pod operations

// resolveServiceToPods resolves a service to its target pods.
func (c *kubernetesClient) resolveServiceToPods(ctx context.Context, kubeContext, namespace, serviceName string) ([]string, error) {
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return nil, err
	}

	// Get the service
	service, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("service %s/%s not found: %w", namespace, serviceName, err)
	}

	// If service has no selector, it doesn't target pods directly
	if len(service.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s/%s has no selector - cannot resolve to pods", namespace, serviceName)
	}

	// Build label selector from service selector
	labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: service.Spec.Selector,
	})

	// List pods matching the service selector
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for service %s/%s: %w", namespace, serviceName, err)
	}

	// Filter for running pods
	var podNames []string
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podNames = append(podNames, pod.Name)
		}
	}

	if len(podNames) == 0 {
		return nil, fmt.Errorf("no running pods found for service %s/%s", namespace, serviceName)
	}

	return podNames, nil
}

// PortForwardToService sets up port forwarding to the first available pod behind a service.
func (c *kubernetesClient) PortForwardToService(ctx context.Context, kubeContext, namespace, serviceName string, ports []string, opts PortForwardOptions) (*PortForwardSession, error) {
	// Validate operation
	if err := c.isOperationAllowed("port-forward"); err != nil {
		return nil, err
	}

	// Validate namespace access
	if err := c.isNamespaceRestricted(namespace); err != nil {
		return nil, err
	}

	c.logOperation("port-forward", kubeContext, namespace, "service", serviceName)

	// Resolve service to pods
	podNames, err := c.resolveServiceToPods(ctx, kubeContext, namespace, serviceName)
	if err != nil {
		return nil, err
	}

	// Use the first available pod
	targetPod := podNames[0]
	c.config.Logger.Info("Resolved service to pod", "service", serviceName, "pod", targetPod, "totalPods", len(podNames))

	// Forward to the resolved pod
	return c.PortForward(ctx, kubeContext, namespace, targetPod, ports, opts)
}

// validatePodRunning checks if a pod is running in the specified namespace.
func (c *kubernetesClient) validatePodRunning(ctx context.Context, kubeContext, namespace, podName string) error {
	clientset, err := c.getClientset(kubeContext)
	if err != nil {
		return err
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("pod %s/%s not found: %w", namespace, podName, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("pod %s/%s is not running", namespace, podName)
	}

	return nil
}
