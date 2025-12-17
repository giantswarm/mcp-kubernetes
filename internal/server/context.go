package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
)

// ServerContext encapsulates all dependencies needed by the MCP server
// and provides a clean abstraction for dependency injection and lifecycle management.
type ServerContext struct {
	// Core dependencies
	k8sClient k8s.Client
	logger    Logger
	config    *Config

	// OAuth downstream authentication support
	// When clientFactory is set and downstreamOAuth is true, the server will
	// create per-user Kubernetes clients using the user's OAuth token.
	clientFactory         k8s.ClientFactory
	downstreamOAuth       bool
	downstreamOAuthStrict bool

	// inCluster indicates whether the server is running inside a Kubernetes cluster
	// using in-cluster authentication (service account token).
	// When true, kubeconfig-based context switching is not available.
	inCluster bool

	// Federation manager for multi-cluster support
	// When set, enables operations across multiple Kubernetes clusters via CAPI.
	federationManager federation.ClusterClientManager

	// OpenTelemetry instrumentation provider
	instrumentationProvider *instrumentation.Provider

	// Context management
	ctx    context.Context
	cancel context.CancelFunc

	// Lifecycle management
	mu       sync.RWMutex
	shutdown bool

	// Active session tracking for cleanup during shutdown
	activeSessions map[string]*k8s.PortForwardSession
	sessionsMu     sync.RWMutex
}

// NewServerContext creates a new ServerContext with default values.
// Use the provided functional options to customize the context.
func NewServerContext(ctx context.Context, opts ...Option) (*ServerContext, error) {
	// Create a cancellable context
	serverCtx, cancel := context.WithCancel(ctx)

	// Initialize with defaults
	sc := &ServerContext{
		ctx:            serverCtx,
		cancel:         cancel,
		config:         NewDefaultConfig(),
		logger:         NewDefaultLogger(),
		activeSessions: make(map[string]*k8s.PortForwardSession),
	}

	// Apply functional options
	for _, opt := range opts {
		if err := opt(sc); err != nil {
			cancel()
			return nil, err
		}
	}

	// Validate required dependencies
	if err := sc.validate(); err != nil {
		cancel()
		return nil, err
	}

	return sc, nil
}

// Context returns the server context for cancellation and deadlines.
func (sc *ServerContext) Context() context.Context {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.ctx
}

// K8sClient returns the Kubernetes client interface.
// Note: For OAuth downstream mode, consider using K8sClientForContext instead.
func (sc *ServerContext) K8sClient() k8s.Client {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.k8sClient
}

// K8sClientForContext returns a Kubernetes client appropriate for the request context.
// If downstream OAuth is enabled and an access token is present in the context,
// it returns a per-user client using the bearer token.
//
// When downstream OAuth strict mode is enabled (the default via CLI):
//   - If no access token is available, returns ErrOAuthTokenMissing
//   - If the bearer token client cannot be created, returns ErrOAuthClientFailed
//
// When strict mode is disabled (NOT recommended for production):
//   - Falls back to the shared service account client if authentication fails
//
// Returns (client, nil) on success, or (nil, error) when strict mode denies access.
func (sc *ServerContext) K8sClientForContext(ctx context.Context) (k8s.Client, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// If downstream OAuth is not enabled, use the shared client
	if !sc.downstreamOAuth || sc.clientFactory == nil {
		return sc.k8sClient, nil
	}

	// Try to get the access token from context
	accessToken, ok := oauth.GetAccessTokenFromContext(ctx)
	if !ok || accessToken == "" {
		// No access token in context
		if sc.downstreamOAuthStrict {
			// Strict mode: fail closed - do not fall back to service account
			sc.logger.Warn("No access token in context, denying access (strict mode enabled)")
			if sc.instrumentationProvider != nil && sc.instrumentationProvider.Enabled() {
				sc.instrumentationProvider.Metrics().RecordOAuthDownstreamAuth(ctx, instrumentation.OAuthResultDenied)
			}
			return nil, ErrOAuthTokenMissing
		}
		// Non-strict mode: fall back to shared client (legacy behavior)
		sc.logger.Debug("No access token in context, using shared client")
		if sc.instrumentationProvider != nil && sc.instrumentationProvider.Enabled() {
			sc.instrumentationProvider.Metrics().RecordOAuthDownstreamAuth(ctx, instrumentation.OAuthResultFallback)
		}
		return sc.k8sClient, nil
	}

	// Create a per-user client with the bearer token
	client, err := sc.clientFactory.CreateBearerTokenClient(accessToken)
	if err != nil {
		if sc.downstreamOAuthStrict {
			// Strict mode: fail closed - do not fall back to service account
			sc.logger.Warn("Failed to create bearer token client, denying access (strict mode enabled)", "error", err)
			if sc.instrumentationProvider != nil && sc.instrumentationProvider.Enabled() {
				sc.instrumentationProvider.Metrics().RecordOAuthDownstreamAuth(ctx, instrumentation.OAuthResultDenied)
			}
			return nil, fmt.Errorf("%w: %v", ErrOAuthClientFailed, err)
		}
		// Non-strict mode: fall back to shared client (legacy behavior)
		sc.logger.Warn("Failed to create bearer token client, using shared client", "error", err)
		if sc.instrumentationProvider != nil && sc.instrumentationProvider.Enabled() {
			sc.instrumentationProvider.Metrics().RecordOAuthDownstreamAuth(ctx, instrumentation.OAuthResultFailure)
		}
		return sc.k8sClient, nil
	}

	sc.logger.Debug("Created bearer token client for user request")
	if sc.instrumentationProvider != nil && sc.instrumentationProvider.Enabled() {
		sc.instrumentationProvider.Metrics().RecordOAuthDownstreamAuth(ctx, instrumentation.OAuthResultSuccess)
	}
	return client, nil
}

// DownstreamOAuthEnabled returns true if downstream OAuth authentication is enabled.
func (sc *ServerContext) DownstreamOAuthEnabled() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.downstreamOAuth
}

// DownstreamOAuthStrictEnabled returns true if downstream OAuth strict mode is enabled.
// When strict mode is enabled, requests without valid OAuth tokens will fail with an
// authentication error instead of falling back to the service account.
func (sc *ServerContext) DownstreamOAuthStrictEnabled() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.downstreamOAuthStrict
}

// InClusterMode returns true if the server is running inside a Kubernetes cluster.
// When true, kubeconfig-based context switching is not available.
func (sc *ServerContext) InClusterMode() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.inCluster
}

// ClientFactory returns the client factory for creating per-user clients.
func (sc *ServerContext) ClientFactory() k8s.ClientFactory {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.clientFactory
}

// InstrumentationProvider returns the OpenTelemetry instrumentation provider.
func (sc *ServerContext) InstrumentationProvider() *instrumentation.Provider {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.instrumentationProvider
}

// FederationManager returns the multi-cluster federation manager.
// Returns nil if federation is not enabled.
func (sc *ServerContext) FederationManager() federation.ClusterClientManager {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.federationManager
}

// FederationEnabled returns true if multi-cluster federation is enabled.
func (sc *ServerContext) FederationEnabled() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.federationManager != nil
}

// FederationStats returns statistics about the federation manager.
// Returns nil if federation is not enabled.
func (sc *ServerContext) FederationStats() *federation.ManagerStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.federationManager == nil {
		return nil
	}

	stats := sc.federationManager.Stats()
	return &stats
}

// OutputConfig returns the output processing configuration.
// Returns default config if not explicitly set.
func (sc *ServerContext) OutputConfig() *OutputConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.config != nil && sc.config.Output != nil {
		return sc.config.Output
	}
	return NewDefaultOutputConfig()
}

// RecordK8sOperation records a Kubernetes operation metric if instrumentation is enabled.
// This is a convenience method that handles nil checks internally.
func (sc *ServerContext) RecordK8sOperation(ctx context.Context, operation, resourceType, namespace, status string, duration time.Duration) {
	sc.mu.RLock()
	provider := sc.instrumentationProvider
	sc.mu.RUnlock()

	if provider != nil && provider.Enabled() {
		provider.Metrics().RecordK8sOperation(ctx, operation, resourceType, namespace, status, duration)
	}
}

// RecordPodOperation records a pod operation metric if instrumentation is enabled.
// This is a convenience method that handles nil checks internally.
func (sc *ServerContext) RecordPodOperation(ctx context.Context, operation, namespace, status string, duration time.Duration) {
	sc.mu.RLock()
	provider := sc.instrumentationProvider
	sc.mu.RUnlock()

	if provider != nil && provider.Enabled() {
		provider.Metrics().RecordPodOperation(ctx, operation, namespace, status, duration)
	}
}

// IncrementActiveSessions increments the active port-forward sessions metric.
func (sc *ServerContext) IncrementActiveSessions(ctx context.Context) {
	sc.mu.RLock()
	provider := sc.instrumentationProvider
	sc.mu.RUnlock()

	if provider != nil && provider.Enabled() {
		provider.Metrics().IncrementActiveSessions(ctx)
	}
}

// DecrementActiveSessions decrements the active port-forward sessions metric.
func (sc *ServerContext) DecrementActiveSessions(ctx context.Context) {
	sc.mu.RLock()
	provider := sc.instrumentationProvider
	sc.mu.RUnlock()

	if provider != nil && provider.Enabled() {
		provider.Metrics().DecrementActiveSessions(ctx)
	}
}

// Logger returns the logger interface.
func (sc *ServerContext) Logger() Logger {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.logger
}

// Config returns the server configuration.
func (sc *ServerContext) Config() *Config {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.config
}

// RegisterPortForwardSession registers an active port forwarding session for cleanup tracking.
func (sc *ServerContext) RegisterPortForwardSession(sessionID string, session *k8s.PortForwardSession) {
	sc.sessionsMu.Lock()
	defer sc.sessionsMu.Unlock()

	if sc.activeSessions != nil {
		sc.activeSessions[sessionID] = session
		sc.logger.Debug("Registered port forward session", "sessionID", sessionID)
	}
}

// UnregisterPortForwardSession removes a port forwarding session from tracking.
func (sc *ServerContext) UnregisterPortForwardSession(sessionID string) {
	sc.sessionsMu.Lock()
	defer sc.sessionsMu.Unlock()

	if sc.activeSessions != nil {
		delete(sc.activeSessions, sessionID)
		sc.logger.Debug("Unregistered port forward session", "sessionID", sessionID)
	}
}

// GetActiveSessionCount returns the number of active port forwarding sessions.
func (sc *ServerContext) GetActiveSessionCount() int {
	sc.sessionsMu.RLock()
	defer sc.sessionsMu.RUnlock()
	return len(sc.activeSessions)
}

// GetActiveSessions returns a copy of all active port forwarding sessions.
func (sc *ServerContext) GetActiveSessions() map[string]*k8s.PortForwardSession {
	sc.sessionsMu.RLock()
	defer sc.sessionsMu.RUnlock()

	// Return a copy to avoid race conditions
	sessions := make(map[string]*k8s.PortForwardSession)
	for id, session := range sc.activeSessions {
		sessions[id] = session
	}
	return sessions
}

// StopPortForwardSession stops a specific port forwarding session by ID.
func (sc *ServerContext) StopPortForwardSession(sessionID string) error {
	sc.sessionsMu.Lock()
	defer sc.sessionsMu.Unlock()

	session, exists := sc.activeSessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session != nil && session.StopChan != nil {
		sc.logger.Debug("Stopping port forward session", "sessionID", sessionID)
		select {
		case session.StopChan <- struct{}{}:
			// Signal sent successfully
		default:
			// Channel was already closed or full, that's ok
		}
	}

	// Remove from active sessions
	delete(sc.activeSessions, sessionID)
	sc.logger.Info("Port forward session stopped", "sessionID", sessionID)
	return nil
}

// StopAllPortForwardSessions stops all active port forwarding sessions.
func (sc *ServerContext) StopAllPortForwardSessions() int {
	sc.sessionsMu.Lock()
	defer sc.sessionsMu.Unlock()

	count := len(sc.activeSessions)
	if count == 0 {
		return 0
	}

	sc.logger.Info("Stopping all port forwarding sessions", "count", count)

	for sessionID, session := range sc.activeSessions {
		if session != nil && session.StopChan != nil {
			sc.logger.Debug("Stopping port forward session", "sessionID", sessionID)
			select {
			case session.StopChan <- struct{}{}:
				// Signal sent successfully
			default:
				// Channel was already closed or full, that's ok
			}
		}
	}

	// Clear all sessions
	sc.activeSessions = make(map[string]*k8s.PortForwardSession)
	sc.logger.Info("All port forwarding sessions stopped", "count", count)
	return count
}

// Shutdown gracefully shuts down the server context.
// This cancels the context and releases any resources.
func (sc *ServerContext) Shutdown() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.shutdown {
		return nil
	}

	sc.logger.Info("Shutting down server context")

	// Clean up active port forwarding sessions
	sc.cleanupPortForwardSessions()

	// Shutdown federation manager
	if sc.federationManager != nil {
		if err := sc.federationManager.Close(); err != nil {
			sc.logger.Error("Failed to close federation manager", "error", err)
		}
	}

	// Shutdown instrumentation provider
	if sc.instrumentationProvider != nil {
		shutdownCtx := context.Background()
		if err := sc.instrumentationProvider.Shutdown(shutdownCtx); err != nil {
			sc.logger.Error("Failed to shutdown instrumentation provider", "error", err)
		}
	}

	// Cancel the context
	if sc.cancel != nil {
		sc.cancel()
	}

	// Mark as shutdown
	sc.shutdown = true

	sc.logger.Info("Server context shutdown complete")
	return nil
}

// cleanupPortForwardSessions stops all active port forwarding sessions.
func (sc *ServerContext) cleanupPortForwardSessions() {
	sc.sessionsMu.Lock()
	defer sc.sessionsMu.Unlock()

	if len(sc.activeSessions) == 0 {
		return
	}

	sc.logger.Info("Cleaning up active port forwarding sessions", "count", len(sc.activeSessions))

	for sessionID, session := range sc.activeSessions {
		if session != nil && session.StopChan != nil {
			sc.logger.Debug("Stopping port forward session", "sessionID", sessionID)
			select {
			case session.StopChan <- struct{}{}:
				// Signal sent successfully
			default:
				// Channel was already closed or full, that's ok
			}
		}
	}

	// Clear the sessions map
	sc.activeSessions = make(map[string]*k8s.PortForwardSession)
}

// IsShutdown returns true if the server context has been shutdown.
func (sc *ServerContext) IsShutdown() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.shutdown
}

// validate ensures all required dependencies are set.
func (sc *ServerContext) validate() error {
	if sc.k8sClient == nil {
		return ErrMissingK8sClient
	}
	if sc.logger == nil {
		return ErrMissingLogger
	}
	if sc.config == nil {
		return ErrMissingConfig
	}
	return nil
}

// Logger defines the interface for logging operations.
type Logger interface {
	// Info logs an informational message.
	Info(msg string, args ...interface{})

	// Debug logs a debug message.
	Debug(msg string, args ...interface{})

	// Warn logs a warning message.
	Warn(msg string, args ...interface{})

	// Error logs an error message.
	Error(msg string, args ...interface{})

	// With returns a new logger with additional context fields.
	With(args ...interface{}) Logger
}

// Config holds the server configuration.
type Config struct {
	// Server settings
	ServerName string `json:"serverName"`
	Version    string `json:"version"`

	// Kubernetes settings
	DefaultNamespace string `json:"defaultNamespace"`
	KubeConfigPath   string `json:"kubeConfigPath"`
	DefaultContext   string `json:"defaultContext"`

	// Non-destructive mode settings
	NonDestructiveMode bool `json:"nonDestructiveMode"`
	DryRun             bool `json:"dryRun"`

	// Logging settings
	LogLevel  string `json:"logLevel"`
	LogFormat string `json:"logFormat"`

	// Security settings
	EnableAuth           bool     `json:"enableAuth"`
	AllowedOperations    []string `json:"allowedOperations"`
	RestrictedNamespaces []string `json:"restrictedNamespaces"`

	// Output processing settings for fleet-scale operations
	Output *OutputConfig `json:"output,omitempty"`
}

// OutputConfig holds configuration for output processing.
// This controls how large responses are handled to prevent context overflow.
type OutputConfig struct {
	// MaxItems limits the number of resources returned per query.
	// Default: 100, Absolute max: 1000
	MaxItems int `json:"maxItems" yaml:"maxItems"`

	// MaxClusters limits clusters in fleet-wide queries.
	// Default: 20, Absolute max: 100
	MaxClusters int `json:"maxClusters" yaml:"maxClusters"`

	// MaxResponseBytes is a hard limit on response size in bytes.
	// Default: 512KB, Absolute max: 2MB
	MaxResponseBytes int `json:"maxResponseBytes" yaml:"maxResponseBytes"`

	// SlimOutput enables removal of verbose fields that rarely help AI agents.
	// Default: true
	SlimOutput bool `json:"slimOutput" yaml:"slimOutput"`

	// MaskSecrets replaces secret data with "***REDACTED***".
	// Default: true (security critical - should rarely be disabled)
	MaskSecrets bool `json:"maskSecrets" yaml:"maskSecrets"`

	// SummaryThreshold is the item count above which summary mode is suggested.
	// Default: 500
	SummaryThreshold int `json:"summaryThreshold" yaml:"summaryThreshold"`
}

// NewDefaultConfig creates a configuration with sensible defaults.
func NewDefaultConfig() *Config {
	return &Config{
		ServerName:           "mcp-kubernetes",
		Version:              "0.1.0",
		DefaultNamespace:     "default",
		NonDestructiveMode:   true,
		DryRun:               false,
		LogLevel:             "info",
		LogFormat:            "json",
		EnableAuth:           false,
		AllowedOperations:    []string{"get", "list", "describe"},
		RestrictedNamespaces: []string{"kube-system", "kube-public"},
		Output:               NewDefaultOutputConfig(),
	}
}

// NewDefaultOutputConfig creates default output processing configuration.
func NewDefaultOutputConfig() *OutputConfig {
	return &OutputConfig{
		MaxItems:         100,
		MaxClusters:      20,
		MaxResponseBytes: 512 * 1024, // 512KB
		SlimOutput:       true,
		MaskSecrets:      true,
		SummaryThreshold: 500,
	}
}

// Clone creates a deep copy of the configuration.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}

	clone := *c

	// Deep copy slices
	if c.AllowedOperations != nil {
		clone.AllowedOperations = make([]string, len(c.AllowedOperations))
		copy(clone.AllowedOperations, c.AllowedOperations)
	}

	if c.RestrictedNamespaces != nil {
		clone.RestrictedNamespaces = make([]string, len(c.RestrictedNamespaces))
		copy(clone.RestrictedNamespaces, c.RestrictedNamespaces)
	}

	// Deep copy output config
	if c.Output != nil {
		outputCopy := *c.Output
		clone.Output = &outputCopy
	}

	return &clone
}
