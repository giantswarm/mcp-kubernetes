package federation

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// defaultHealthCheckPath is the standard Kubernetes health endpoint.
const defaultHealthCheckPath = "/healthz"

// ConnectivityConfig holds configuration options for cluster connectivity.
// These settings control how the federation manager establishes and validates
// connections to workload clusters.
//
// # Default Values
//
// If not specified, the following defaults are used:
//   - ConnectionTimeout: 5 seconds
//   - RequestTimeout: 30 seconds
//   - RetryAttempts: 3
//   - RetryBackoff: 1 second (exponential backoff with factor 2)
//
// # Network Topology Considerations
//
// Giant Swarm deployments often span multiple VPCs and use various connectivity
// methods (VPC peering, Transit Gateway, konnectivity). Tune these values based
// on your network topology:
//   - For high-latency networks (cross-region): increase ConnectionTimeout
//   - For konnectivity proxies: increase RequestTimeout
//   - For unstable networks: increase RetryAttempts
type ConnectivityConfig struct {
	// ConnectionTimeout is the maximum time to wait for the initial TCP connection
	// to the cluster API server. This applies to the TCP dial phase only.
	//
	// Default: 5 seconds.
	ConnectionTimeout time.Duration

	// RequestTimeout is the maximum time to wait for individual API requests
	// to complete. This includes TLS handshake, sending the request, and receiving
	// the response.
	//
	// Default: 30 seconds.
	RequestTimeout time.Duration

	// RetryAttempts is the number of times to retry a failed connection before
	// giving up. This helps with transient network issues.
	//
	// Default: 3.
	RetryAttempts int

	// RetryBackoff is the initial backoff duration between retry attempts.
	// Subsequent retries use exponential backoff (backoff * 2^attempt).
	//
	// Default: 1 second.
	RetryBackoff time.Duration

	// HealthCheckPath is the API path used for health checks.
	// Default: "/healthz" (standard Kubernetes health endpoint).
	HealthCheckPath string

	// QPS is the queries per second limit for the Kubernetes client.
	// This controls client-side rate limiting.
	//
	// Default: 50 (reasonable for AI workloads).
	QPS float32

	// Burst is the maximum burst for throttled requests.
	// Allows short bursts of requests above QPS.
	//
	// Default: 100.
	Burst int
}

// DefaultConnectivityConfig returns a ConnectivityConfig with sensible defaults.
// These defaults are suitable for typical VPC-peered deployments.
func DefaultConnectivityConfig() ConnectivityConfig {
	return ConnectivityConfig{
		ConnectionTimeout: 5 * time.Second,
		RequestTimeout:    30 * time.Second,
		RetryAttempts:     3,
		RetryBackoff:      1 * time.Second,
		HealthCheckPath:   defaultHealthCheckPath,
		QPS:               50,
		Burst:             100,
	}
}

// HighLatencyConnectivityConfig returns a ConnectivityConfig optimized for
// high-latency networks such as cross-region deployments or konnectivity proxies.
func HighLatencyConnectivityConfig() ConnectivityConfig {
	return ConnectivityConfig{
		ConnectionTimeout: 15 * time.Second,
		RequestTimeout:    60 * time.Second,
		RetryAttempts:     5,
		RetryBackoff:      2 * time.Second,
		HealthCheckPath:   defaultHealthCheckPath,
		QPS:               30,
		Burst:             60,
	}
}

// ApplyConnectivityConfig applies the connectivity configuration to a rest.Config.
// This modifies the config in place to use the specified timeouts and rate limits.
//
// Note: This function modifies the provided config. If you need to preserve the
// original config, use rest.CopyConfig() first.
func ApplyConnectivityConfig(config *rest.Config, cc ConnectivityConfig) {
	if config == nil {
		return
	}

	// Apply timeout settings
	config.Timeout = cc.RequestTimeout

	// Apply rate limiting
	if cc.QPS > 0 {
		config.QPS = cc.QPS
	}
	if cc.Burst > 0 {
		config.Burst = cc.Burst
	}

	// Note: ConnectionTimeout is used during dial, not directly settable on rest.Config.
	// It's used during CheckConnectivity via custom dialer.
}

// CheckConnectivity verifies that the MCP server can reach the Workload Cluster API server.
// This should be called before caching a client to detect network issues early.
//
// The check performs a GET request to the health endpoint (default: /healthz) which
// doesn't require authentication, making it suitable for connectivity validation.
//
// # Network Topology
//
// This check validates the complete network path from the MCP server to the target
// cluster, including:
//   - DNS resolution
//   - TCP connectivity (VPC peering, Transit Gateway, konnectivity)
//   - TLS handshake (certificate validation)
//   - HTTP/2 connection establishment
//
// # Error Types
//
// Returns different error types based on the failure:
//   - ErrConnectionTimeout: TCP connection timed out
//   - ErrTLSHandshakeFailed: TLS/certificate issues
//   - ErrClusterUnreachable: General connectivity failure
func CheckConnectivity(ctx context.Context, clusterName string, config *rest.Config, cc ConnectivityConfig) error {
	if config == nil {
		return &ConnectionError{
			ClusterName: clusterName,
			Host:        "<nil config>",
			Reason:      "config is nil",
		}
	}

	// Create a validation context with timeout
	timeout := cc.ConnectionTimeout
	if timeout <= 0 {
		timeout = DefaultConnectivityConfig().ConnectionTimeout
	}
	validationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prepare the config for REST client creation
	configCopy := rest.CopyConfig(config)
	configCopy.APIPath = "/api"
	configCopy.GroupVersion = &schema.GroupVersion{Version: "v1"}
	configCopy.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	configCopy.Timeout = timeout

	// Create a minimal REST client for validation
	restClient, err := rest.RESTClientFor(configCopy)
	if err != nil {
		return wrapConnectivityError(clusterName, config.Host, "failed to create REST client", err)
	}

	// Determine health check path
	healthPath := cc.HealthCheckPath
	if healthPath == "" {
		healthPath = defaultHealthCheckPath
	}

	// Perform the health check
	result := restClient.Get().AbsPath(healthPath).Do(validationCtx)
	if err := result.Error(); err != nil {
		return wrapConnectivityError(clusterName, config.Host, "health check failed", err)
	}

	return nil
}

// CheckConnectivityWithRetry performs connectivity check with retry logic.
// This is useful for handling transient network issues during cluster discovery.
//
// The function uses exponential backoff between retries:
//
//	attempt 1: immediate
//	attempt 2: wait RetryBackoff
//	attempt 3: wait RetryBackoff * 2
//	attempt 4: wait RetryBackoff * 4
//	...
//
// Returns the last error if all retry attempts fail.
func CheckConnectivityWithRetry(ctx context.Context, clusterName string, config *rest.Config, cc ConnectivityConfig) error {
	// Early nil check to prevent panics
	if config == nil {
		return &ConnectionError{
			ClusterName: clusterName,
			Host:        "<nil config>",
			Reason:      "config is nil",
		}
	}

	attempts := cc.RetryAttempts
	if attempts <= 0 {
		attempts = 1
	}

	backoff := cc.RetryBackoff
	if backoff <= 0 {
		backoff = DefaultConnectivityConfig().RetryBackoff
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		// Check if context is already cancelled
		if ctx.Err() != nil {
			return wrapConnectivityError(clusterName, config.Host, "context cancelled during retry", ctx.Err())
		}

		// Wait before retry (except for first attempt)
		if attempt > 0 {
			// Exponential backoff: backoff * 2^(attempt-1)
			waitDuration := backoff * time.Duration(1<<(attempt-1))

			select {
			case <-ctx.Done():
				return wrapConnectivityError(clusterName, config.Host, "context cancelled during backoff", ctx.Err())
			case <-time.After(waitDuration):
				// Continue to retry
			}
		}

		// Attempt connectivity check
		err := CheckConnectivity(ctx, clusterName, config, cc)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Don't retry for certain error types
		if !isRetryableError(err) {
			return err
		}
	}

	return lastErr
}

// isRetryableError determines if an error is worth retrying.
// Some errors (like TLS certificate issues) won't be fixed by retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific non-retryable error types
	var tlsErr *TLSError
	if errors.As(err, &tlsErr) {
		return false
	}

	// Timeout and connection errors are generally retryable
	return true
}

// wrapConnectivityError creates an appropriate error type based on the underlying error.
// This provides user-friendly error messages while preserving the original error for debugging.
func wrapConnectivityError(clusterName, host, reason string, err error) error {
	if err == nil {
		return &ConnectionError{
			ClusterName: clusterName,
			Host:        sanitizeHost(host),
			Reason:      reason,
		}
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return &ConnectivityTimeoutError{
			ClusterName: clusterName,
			Host:        sanitizeHost(host),
			Timeout:     0, // Unknown at this point
			Err:         err,
		}
	}

	// Check for context cancelled
	if errors.Is(err, context.Canceled) {
		return &ConnectionError{
			ClusterName: clusterName,
			Host:        sanitizeHost(host),
			Reason:      "request cancelled",
			Err:         err,
		}
	}

	// Check for TLS errors
	if isTLSError(err) {
		return &TLSError{
			ClusterName: clusterName,
			Host:        sanitizeHost(host),
			Reason:      extractTLSReason(err),
			Err:         err,
		}
	}

	// Check for network timeout errors
	if isTimeoutError(err) {
		return &ConnectivityTimeoutError{
			ClusterName: clusterName,
			Host:        sanitizeHost(host),
			Err:         err,
		}
	}

	// General connection error
	return &ConnectionError{
		ClusterName: clusterName,
		Host:        sanitizeHost(host),
		Reason:      reason,
		Err:         err,
	}
}

// isTLSError checks if the error is related to TLS/certificate issues.
func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	// Check for tls.RecordHeaderError or similar typed errors first
	var tlsRecordErr tls.RecordHeaderError
	if errors.As(err, &tlsRecordErr) {
		return true
	}

	errStr := err.Error()

	// Check for specific TLS/x509 error patterns
	// These patterns are more specific to avoid false positives
	tlsPatterns := []string{
		"tls:",
		"x509:",
		"certificate signed by",
		"certificate has expired",
		"certificate is not valid",
		"certificate is valid for",
		"handshake failure",
		"unknown authority",
		"bad certificate",
		"unsupported protocol",
	}

	for _, pattern := range tlsPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isTimeoutError checks if the error is a timeout error.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check error message for timeout patterns
	errStr := err.Error()
	timeoutPatterns := []string{
		"timeout",
		"timed out",
		"deadline exceeded",
		"i/o timeout",
	}

	for _, pattern := range timeoutPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// extractTLSReason extracts a human-readable reason from a TLS error.
func extractTLSReason(err error) string {
	if err == nil {
		return "unknown TLS error"
	}

	errStr := err.Error()

	// Map common TLS errors to user-friendly messages
	switch {
	case strings.Contains(errStr, "unknown authority"):
		return "certificate signed by unknown authority"
	case strings.Contains(errStr, "has expired"):
		return "certificate has expired"
	case strings.Contains(errStr, "not valid yet"):
		return "certificate is not yet valid"
	case strings.Contains(errStr, "doesn't contain any IP SANs"):
		return "certificate doesn't match server IP"
	case strings.Contains(errStr, "doesn't match"):
		return "certificate hostname mismatch"
	case strings.Contains(errStr, "handshake failure"):
		return "TLS handshake failed"
	case strings.Contains(errStr, "protocol version"):
		return "TLS protocol version mismatch"
	default:
		return "TLS error"
	}
}

// GetEndpointType attempts to classify the endpoint type based on the host URL.
// This is informational and used for logging/debugging purposes.
//
// Returns one of:
//   - "private" - Private IP address (VPC peering required)
//   - "public" - Public DNS/IP address
//   - "konnectivity" - Konnectivity proxy endpoint
//   - "unknown" - Cannot determine endpoint type
func GetEndpointType(host string) string {
	if host == "" {
		return "unknown"
	}

	// Check for konnectivity proxy pattern
	if strings.Contains(host, "konnectivity") {
		return "konnectivity"
	}

	// Extract host/IP from URL
	hostPart := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")

	// Use net.SplitHostPort for proper handling of IPv4 and IPv6 addresses
	// This correctly handles cases like "[::1]:6443" and "10.0.0.1:6443"
	if h, _, err := net.SplitHostPort(hostPart); err == nil {
		hostPart = h
	}

	// Try to parse as IP
	ip := net.ParseIP(hostPart)
	if ip != nil {
		if isPrivateIP(ip) {
			return "private"
		}
		return "public"
	}

	// It's a hostname - check for common private DNS patterns
	// Note: We only check DNS-based patterns here since IP parsing failed
	privateHostnamePatterns := []string{
		".internal",
		".local",
		".svc",
		".cluster.local",
	}

	for _, pattern := range privateHostnamePatterns {
		if strings.Contains(hostPart, pattern) {
			return "private"
		}
	}

	return "public"
}

// isPrivateIP checks if an IP address is in a private range.
// Uses Go's built-in IsPrivate() for RFC 1918 and RFC 4193 (IPv6) addresses,
// plus checks for link-local addresses.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// Go 1.17+ provides IsPrivate() which handles:
	// - RFC 1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	// - RFC 4193: fc00::/7 (IPv6 unique local addresses)
	if ip.IsPrivate() {
		return true
	}

	// Also check for link-local addresses (169.254.0.0/16 for IPv4, fe80::/10 for IPv6)
	// These are used in some network configurations
	return ip.IsLinkLocalUnicast()
}
