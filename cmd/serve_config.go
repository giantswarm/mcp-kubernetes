package cmd

import (
	"log"
	"os"
)

// OAuth provider constants
const (
	OAuthProviderDex    = "dex"
	OAuthProviderGoogle = "google"
)

// simpleLogger provides basic logging for the Kubernetes client
type simpleLogger struct{}

func (l *simpleLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[DEBUG] %s %v", msg, args)
}

func (l *simpleLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] %s %v", msg, args)
}

func (l *simpleLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[WARN] %s %v", msg, args)
}

func (l *simpleLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] %s %v", msg, args)
}

// ServeConfig holds all configuration for the serve command.
type ServeConfig struct {
	// Transport settings
	Transport string
	HTTPAddr  string

	// Endpoint paths
	SSEEndpoint     string
	MessageEndpoint string
	HTTPEndpoint    string

	// Kubernetes client settings
	NonDestructiveMode bool
	DryRun             bool
	QPSLimit           float32
	BurstLimit         int
	DebugMode          bool
	InCluster          bool

	// OAuth configuration
	OAuth           OAuthServeConfig
	DownstreamOAuth bool
}

// OAuthServeConfig holds OAuth-specific configuration.
type OAuthServeConfig struct {
	Enabled                       bool
	BaseURL                       string
	Provider                      string // "dex" or "google"
	GoogleClientID                string
	GoogleClientSecret            string
	DexIssuerURL                  string
	DexClientID                   string
	DexClientSecret               string
	DexConnectorID                string // optional: bypasses connector selection screen
	DisableStreaming              bool
	RegistrationToken             string
	AllowPublicRegistration       bool
	AllowInsecureAuthWithoutState bool
	MaxClientsPerIP               int
	EncryptionKey                 string
}

// loadEnvIfEmpty loads an environment variable into a string pointer if it's empty.
func loadEnvIfEmpty(target *string, envKey string) {
	if *target == "" {
		*target = os.Getenv(envKey)
	}
}
