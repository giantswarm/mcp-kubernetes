package security

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

// Default security configuration values
const (
	DefaultTimeout = 30 * time.Second
	DefaultQPS     = 100
	DefaultBurst   = 100
)

// CredentialManager handles secure management of Kubernetes credentials
type CredentialManager struct {
	// allowedKubeconfigPaths contains paths that are allowed for kubeconfig files
	allowedKubeconfigPaths []string

	// allowedContexts contains contexts that are allowed to be used
	allowedContexts []string

	// restrictSensitiveData determines if sensitive data should be redacted in logs
	restrictSensitiveData bool
}

// NewCredentialManager creates a new credential manager with security policies
func NewCredentialManager(config CredentialConfig) *CredentialManager {
	return &CredentialManager{
		allowedKubeconfigPaths: config.AllowedKubeconfigPaths,
		allowedContexts:        config.AllowedContexts,
		restrictSensitiveData:  config.RestrictSensitiveData,
	}
}

// CredentialConfig holds the configuration for credential management
type CredentialConfig struct {
	// AllowedKubeconfigPaths lists paths that are allowed for kubeconfig files
	AllowedKubeconfigPaths []string `json:"allowedKubeconfigPaths"`

	// AllowedContexts lists contexts that are allowed to be used
	AllowedContexts []string `json:"allowedContexts"`

	// RestrictSensitiveData determines if sensitive data should be redacted
	RestrictSensitiveData bool `json:"restrictSensitiveData"`
}

// ValidateKubeconfigPath checks if the kubeconfig path is allowed
func (cm *CredentialManager) ValidateKubeconfigPath(path string) error {
	if path == "" {
		// Empty path is allowed (will use default)
		return nil
	}

	// Resolve the absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return NewValidationError("kubeconfig", path, "failed to resolve absolute path", err)
	}

	// Check if the file exists
	if _, err := os.Stat(absPath); err != nil {
		return NewValidationError("kubeconfig", path, "kubeconfig file not accessible", err)
	}

	// If no allowed paths are configured, allow any readable file
	if len(cm.allowedKubeconfigPaths) == 0 {
		return nil
	}

	// Check if the path is in the allowed list
	for _, allowedPath := range cm.allowedKubeconfigPaths {
		allowedAbs, err := filepath.Abs(allowedPath)
		if err != nil {
			continue
		}

		// Check for exact match or if the file is within an allowed directory
		if absPath == allowedAbs || strings.HasPrefix(absPath, allowedAbs+string(filepath.Separator)) {
			return nil
		}
	}

	return NewForbiddenError("kubeconfig", path, "path not in allowed kubeconfig paths")
}

// ValidateContext checks if the Kubernetes context is allowed
func (cm *CredentialManager) ValidateContext(context string) error {
	if context == "" {
		// Empty context is allowed (will use default)
		return nil
	}

	// If no allowed contexts are configured, allow any context
	if len(cm.allowedContexts) == 0 {
		return nil
	}

	// Check if the context is in the allowed list
	for _, allowedContext := range cm.allowedContexts {
		if context == allowedContext {
			return nil
		}
	}

	return NewForbiddenError("context", context, "context not in allowed contexts list")
}

// SecureRestConfig wraps a Kubernetes REST config with security enhancements
func (cm *CredentialManager) SecureRestConfig(config *rest.Config) *rest.Config {
	if config == nil {
		return nil
	}

	// Create a copy to avoid modifying the original
	secureConfig := rest.CopyConfig(config)

	// Wrap the transport to redact sensitive information in logs
	if cm.restrictSensitiveData {
		secureConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			return &secureTransport{
				base:         rt,
				restrictData: cm.restrictSensitiveData,
			}
		}
	}

	// Set reasonable security defaults
	if secureConfig.Timeout == 0 {
		secureConfig.Timeout = DefaultTimeout
	}

	if secureConfig.QPS == 0 {
		secureConfig.QPS = DefaultQPS
	}

	if secureConfig.Burst == 0 {
		secureConfig.Burst = DefaultBurst
	}

	return secureConfig
}

// secureTransport is a custom HTTP transport that prevents logging of sensitive information
type secureTransport struct {
	base         http.RoundTripper
	restrictData bool
}

// RoundTrip implements the http.RoundTripper interface with security enhancements
func (t *secureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.restrictData {
		// Create a copy of the request for potential logging
		// The original request maintains all headers for the actual call
		if shouldRedactRequest(req) {
			// Log that a request was made but redact sensitive details
			logSecureRequest(req)
		}
	}

	// Perform the actual request with the original, unmodified request
	return t.base.RoundTrip(req)
}

// shouldRedactRequest determines if a request contains sensitive information
func shouldRedactRequest(req *http.Request) bool {
	// Check for authorization headers
	if req.Header.Get("Authorization") != "" {
		return true
	}

	// Check for client certificate data in TLS config
	if req.TLS != nil && len(req.TLS.PeerCertificates) > 0 {
		return true
	}

	// Check for other sensitive headers
	sensitiveHeaders := []string{
		"X-Auth-Token",
		"X-API-Key",
		"Cookie",
		"Proxy-Authorization",
	}

	for _, header := range sensitiveHeaders {
		if req.Header.Get(header) != "" {
			return true
		}
	}

	return false
}

// logSecureRequest logs a request while redacting sensitive information
func logSecureRequest(req *http.Request) {
	// Create a sanitized version for logging
	sanitizedURL := req.URL.String()

	// Redact query parameters that might contain sensitive data
	if req.URL.RawQuery != "" {
		sanitizedURL = strings.Split(sanitizedURL, "?")[0] + "?[REDACTED]"
	}

	// Log the sanitized request information
	fmt.Printf("[SECURITY] HTTP %s %s (sensitive headers redacted)\n", req.Method, sanitizedURL)
}

// GetDefaultKubeconfigPath returns the default kubeconfig path with validation
func GetDefaultKubeconfigPath() (string, error) {
	// Try KUBECONFIG environment variable first
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig, nil
	}

	// Fall back to default location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	defaultPath := filepath.Join(homeDir, ".kube", "config")

	// Check if the default path exists
	if _, err := os.Stat(defaultPath); err != nil {
		return "", fmt.Errorf("default kubeconfig not found at %s: %w", defaultPath, err)
	}

	return defaultPath, nil
}

// RedactSensitiveConfig creates a copy of config with sensitive data redacted for logging
func RedactSensitiveConfig(config *rest.Config) *rest.Config {
	if config == nil {
		return nil
	}

	redacted := &rest.Config{
		Host:            config.Host,
		APIPath:         config.APIPath,
		ContentConfig:   config.ContentConfig,
		Username:        "[REDACTED]",
		Password:        "[REDACTED]",
		BearerToken:     "[REDACTED]",
		BearerTokenFile: config.BearerTokenFile,
		Impersonate:     config.Impersonate,
		UserAgent:       config.UserAgent,
		Transport:       config.Transport,
		WrapTransport:   config.WrapTransport,
		QPS:             config.QPS,
		Burst:           config.Burst,
		Timeout:         config.Timeout,
	}

	// Redact TLS config sensitive data
	if config.TLSClientConfig.CertData != nil {
		redacted.TLSClientConfig.CertData = []byte("[REDACTED]")
	}
	if config.TLSClientConfig.KeyData != nil {
		redacted.TLSClientConfig.KeyData = []byte("[REDACTED]")
	}
	if config.TLSClientConfig.CAData != nil {
		redacted.TLSClientConfig.CAData = []byte("[REDACTED]")
	}

	// Keep non-sensitive TLS config
	redacted.TLSClientConfig.ServerName = config.TLSClientConfig.ServerName
	redacted.TLSClientConfig.Insecure = config.TLSClientConfig.Insecure
	redacted.TLSClientConfig.CertFile = config.TLSClientConfig.CertFile
	redacted.TLSClientConfig.KeyFile = config.TLSClientConfig.KeyFile
	redacted.TLSClientConfig.CAFile = config.TLSClientConfig.CAFile

	return redacted
}
