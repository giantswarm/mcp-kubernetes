package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
)

// ServerContext encapsulates all dependencies needed by the MCP server
// and provides a clean abstraction for dependency injection and lifecycle management.
type ServerContext struct {
	// Core dependencies
	k8sClient k8s.Client
	logger    Logger
	config    *Config

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
func (sc *ServerContext) K8sClient() k8s.Client {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.k8sClient
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
