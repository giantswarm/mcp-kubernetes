package server

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
)

// Option is a functional option for configuring ServerContext.
type Option func(*ServerContext) error

// WithK8sClient sets the Kubernetes client for the ServerContext.
func WithK8sClient(client k8s.Client) Option {
	return func(sc *ServerContext) error {
		if client == nil {
			return ErrMissingK8sClient
		}
		sc.k8sClient = client
		return nil
	}
}

// WithLogger sets the logger for the ServerContext.
func WithLogger(logger Logger) Option {
	return func(sc *ServerContext) error {
		if logger == nil {
			return ErrMissingLogger
		}
		sc.logger = logger
		return nil
	}
}

// WithConfig sets the configuration for the ServerContext.
func WithConfig(config *Config) Option {
	return func(sc *ServerContext) error {
		if config == nil {
			return ErrMissingConfig
		}
		sc.config = config.Clone()
		return nil
	}
}

// WithServerName sets the server name in the configuration.
func WithServerName(name string) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		sc.config.ServerName = name
		return nil
	}
}

// WithDefaultNamespace sets the default namespace for Kubernetes operations.
func WithDefaultNamespace(namespace string) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		sc.config.DefaultNamespace = namespace
		return nil
	}
}

// WithNonDestructiveMode enables or disables non-destructive mode.
func WithNonDestructiveMode(enabled bool) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		sc.config.NonDestructiveMode = enabled
		return nil
	}
}

// WithDryRun enables or disables dry-run mode.
func WithDryRun(enabled bool) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		sc.config.DryRun = enabled
		return nil
	}
}

// WithLogLevel sets the logging level.
func WithLogLevel(level string) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		sc.config.LogLevel = level
		return nil
	}
}

// WithAuth enables authentication with the specified allowed operations.
func WithAuth(allowedOperations []string) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		sc.config.EnableAuth = true
		if allowedOperations != nil {
			sc.config.AllowedOperations = make([]string, len(allowedOperations))
			copy(sc.config.AllowedOperations, allowedOperations)
		}
		return nil
	}
}

// WithRestrictedNamespaces sets the list of restricted namespaces.
func WithRestrictedNamespaces(namespaces []string) Option {
	return func(sc *ServerContext) error {
		if sc.config == nil {
			sc.config = NewDefaultConfig()
		}
		if namespaces != nil {
			sc.config.RestrictedNamespaces = make([]string, len(namespaces))
			copy(sc.config.RestrictedNamespaces, namespaces)
		}
		return nil
	}
}

// WithClientFactory sets the client factory for creating per-user Kubernetes clients.
// This is used for OAuth downstream authentication where each user's OAuth token
// is used to authenticate with Kubernetes.
func WithClientFactory(factory k8s.ClientFactory) Option {
	return func(sc *ServerContext) error {
		sc.clientFactory = factory
		return nil
	}
}

// WithDownstreamOAuth enables downstream OAuth authentication.
// When enabled and a client factory is set, the server will create per-user
// Kubernetes clients using the user's OAuth token for authentication.
// This requires the Kubernetes cluster to be configured to accept the OAuth
// provider's tokens (e.g., Google OIDC for GKE).
func WithDownstreamOAuth(enabled bool) Option {
	return func(sc *ServerContext) error {
		sc.downstreamOAuth = enabled
		return nil
	}
}

// WithInstrumentationProvider sets the OpenTelemetry instrumentation provider.
// This enables production-grade observability including metrics and tracing.
func WithInstrumentationProvider(provider *instrumentation.Provider) Option {
	return func(sc *ServerContext) error {
		sc.instrumentationProvider = provider
		return nil
	}
}

// WithFederationManager sets the multi-cluster federation manager.
// This enables operations across multiple Kubernetes clusters via CAPI.
// When set, the server can perform operations on both the Management Cluster
// and Workload Clusters discovered through Cluster API resources.
func WithFederationManager(manager federation.ClusterClientManager) Option {
	return func(sc *ServerContext) error {
		sc.federationManager = manager
		return nil
	}
}

// Error definitions for ServerContext validation and operations.
var (
	ErrMissingK8sClient = errors.New("kubernetes client is required")
	ErrMissingLogger    = errors.New("logger is required")
	ErrMissingConfig    = errors.New("configuration is required")
	ErrServerShutdown   = errors.New("server context has been shutdown")
)

// DefaultLogger is a simple logger implementation that wraps the standard library logger.
type DefaultLogger struct {
	logger *log.Logger
	level  string
}

// NewDefaultLogger creates a new default logger with standard error output.
func NewDefaultLogger() Logger {
	return &DefaultLogger{
		logger: log.New(os.Stderr, "[mcp-kubernetes] ", log.LstdFlags|log.Lshortfile),
		level:  "info",
	}
}

// Info logs an informational message.
func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	l.logger.Printf("[INFO] "+msg, args...)
}

// Debug logs a debug message.
func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	if l.level == "debug" {
		l.logger.Printf("[DEBUG] "+msg, args...)
	}
}

// Warn logs a warning message.
func (l *DefaultLogger) Warn(msg string, args ...interface{}) {
	l.logger.Printf("[WARN] "+msg, args...)
}

// Error logs an error message.
func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	l.logger.Printf("[ERROR] "+msg, args...)
}

// With returns a new logger with additional context fields.
func (l *DefaultLogger) With(args ...interface{}) Logger {
	// For the default logger, we'll just add the context to the prefix
	if len(args) > 0 {
		prefix := fmt.Sprintf("[mcp-kubernetes] %v ", args)
		return &DefaultLogger{
			logger: log.New(os.Stderr, prefix, log.LstdFlags|log.Lshortfile),
			level:  l.level,
		}
	}
	return l
}
