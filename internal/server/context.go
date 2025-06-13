package server

import (
	"context"
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
}

// NewServerContext creates a new ServerContext with default values.
// Use the provided functional options to customize the context.
func NewServerContext(ctx context.Context, opts ...Option) (*ServerContext, error) {
	// Create a cancellable context
	serverCtx, cancel := context.WithCancel(ctx)

	// Initialize with defaults
	sc := &ServerContext{
		ctx:    serverCtx,
		cancel: cancel,
		config: NewDefaultConfig(),
		logger: NewDefaultLogger(),
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

// Shutdown gracefully shuts down the server context.
// This cancels the context and releases any resources.
func (sc *ServerContext) Shutdown() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.shutdown {
		return nil
	}

	sc.logger.Info("Shutting down server context")

	// Cancel the context
	if sc.cancel != nil {
		sc.cancel()
	}

	// Mark as shutdown
	sc.shutdown = true

	sc.logger.Info("Server context shutdown complete")
	return nil
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
