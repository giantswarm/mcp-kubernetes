package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
)

// getAccessTokenFromContext retrieves the OAuth access token from the context.
// This is a thin wrapper around the oauth package function.
func getAccessTokenFromContext(ctx context.Context) (string, bool) {
	return oauth.GetAccessTokenFromContext(ctx)
}

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
	clientFactory   k8s.ClientFactory
	downstreamOAuth bool

	// Metrics tracking
	metrics *Metrics

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

// Metrics tracks operational metrics for monitoring
type Metrics struct {
	// OAuth downstream authentication metrics
	PerUserAuthSuccess   int64 // Successful per-user authentications
	PerUserAuthFallback  int64 // Fallbacks to service account
	BearerClientFailures int64 // Failed bearer client creations

	mu sync.RWMutex
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// IncrementPerUserAuthSuccess increments the per-user auth success counter
func (m *Metrics) IncrementPerUserAuthSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PerUserAuthSuccess++
}

// IncrementPerUserAuthFallback increments the fallback counter
func (m *Metrics) IncrementPerUserAuthFallback() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PerUserAuthFallback++
}

// IncrementBearerClientFailures increments the bearer client failure counter
func (m *Metrics) IncrementBearerClientFailures() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BearerClientFailures++
}

// GetMetrics returns a snapshot of current metrics
func (m *Metrics) GetMetrics() (success, fallback, failures int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.PerUserAuthSuccess, m.PerUserAuthFallback, m.BearerClientFailures
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
		metrics:        NewMetrics(),
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
// it returns a per-user client using the bearer token. Otherwise, it returns the
// shared service account client.
func (sc *ServerContext) K8sClientForContext(ctx context.Context) k8s.Client {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// If downstream OAuth is not enabled, use the shared client
	if !sc.downstreamOAuth || sc.clientFactory == nil {
		return sc.k8sClient
	}

	// Try to get the access token from context
	// The import for oauth package is added at the top
	accessToken, ok := getAccessTokenFromContext(ctx)
	if !ok || accessToken == "" {
		// No access token in context, fall back to shared client
		sc.logger.Debug("No access token in context, using shared client")
		sc.metrics.IncrementPerUserAuthFallback()
		return sc.k8sClient
	}

	// Create a per-user client with the bearer token
	client, err := sc.clientFactory.CreateBearerTokenClient(accessToken)
	if err != nil {
		sc.logger.Warn("Failed to create bearer token client, using shared client", "error", err)
		sc.metrics.IncrementBearerClientFailures()
		sc.metrics.IncrementPerUserAuthFallback()
		return sc.k8sClient
	}

	sc.logger.Debug("Created bearer token client for user request")
	sc.metrics.IncrementPerUserAuthSuccess()
	return client
}

// DownstreamOAuthEnabled returns true if downstream OAuth authentication is enabled.
func (sc *ServerContext) DownstreamOAuthEnabled() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.downstreamOAuth
}

// ClientFactory returns the client factory for creating per-user clients.
func (sc *ServerContext) ClientFactory() k8s.ClientFactory {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.clientFactory
}

// Metrics returns the metrics tracker.
func (sc *ServerContext) Metrics() *Metrics {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.metrics
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

	return &clone
}
