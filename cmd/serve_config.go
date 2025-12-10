package cmd

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// OAuth provider constants - use server package constants for consistency
const (
	OAuthProviderDex    = server.OAuthProviderDex
	OAuthProviderGoogle = server.OAuthProviderGoogle
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

	// CAPI Mode configuration (multi-cluster federation)
	CAPIMode CAPIModeConfig
}

// CAPIModeConfig holds CAPI federation mode configuration.
type CAPIModeConfig struct {
	// Enabled enables CAPI federation mode for multi-cluster operations
	Enabled bool

	// Cache configuration
	CacheTTL             string
	CacheMaxEntries      int
	CacheCleanupInterval string

	// OAuthTokenLifetime is the expected lifetime of OAuth tokens from your provider.
	// If CacheTTL exceeds this value, a warning is logged. This helps prevent
	// authentication failures from using cached clients with expired tokens.
	// Defaults to 1 hour if not specified.
	OAuthTokenLifetime string

	// Connectivity configuration
	ConnectivityTimeout        string
	ConnectivityRetryAttempts  int
	ConnectivityRetryBackoff   string
	ConnectivityRequestTimeout string
	ConnectivityQPS            float32
	ConnectivityBurst          int
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
	AllowPrivateURLs              bool // skip private IP validation for internal deployments
	MaxClientsPerIP               int
	EncryptionKey                 string

	// Storage configuration
	Storage server.OAuthStorageConfig
}

// Type aliases for OAuth storage configuration - use server package types directly
// to avoid duplication and ensure consistency across the codebase.
type (
	// OAuthStorageType represents the type of token storage backend.
	OAuthStorageType = server.OAuthStorageType
	// OAuthStorageConfig holds configuration for OAuth token storage backend.
	OAuthStorageConfig = server.OAuthStorageConfig
	// ValkeyStorageConfig holds configuration for Valkey storage backend.
	ValkeyStorageConfig = server.ValkeyStorageConfig
)

// Storage type constants - re-exported from server package for convenience.
const (
	OAuthStorageTypeMemory = server.OAuthStorageTypeMemory
	OAuthStorageTypeValkey = server.OAuthStorageTypeValkey
)

// loadEnvIfEmpty loads an environment variable into a string pointer if it's empty.
func loadEnvIfEmpty(target *string, envKey string) {
	if *target == "" {
		*target = os.Getenv(envKey)
	}
}

// validateSecureURL validates that a URL uses HTTPS and is not vulnerable to SSRF attacks.
// It checks for:
// - Valid URL format
// - HTTPS scheme (HTTP not allowed)
// - No private/local IP addresses (unless allowPrivate is true)
// - No localhost references
func validateSecureURL(urlStr string, fieldName string, allowPrivate bool) error {
	// Check for empty URL
	if urlStr == "" {
		return fmt.Errorf("%s must be a valid URL: empty URL provided", fieldName)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", fieldName, err)
	}

	// Require HTTPS
	if parsedURL.Scheme != "https" {
		if parsedURL.Scheme == "" {
			return fmt.Errorf("%s must be a valid URL with HTTPS scheme", fieldName)
		}
		return fmt.Errorf("%s must use HTTPS (got: %s)", fieldName, parsedURL.Scheme)
	}

	// Extract hostname for validation
	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("%s must have a valid hostname", fieldName)
	}

	// Check for localhost references
	if strings.ToLower(hostname) == "localhost" {
		return fmt.Errorf("%s cannot use localhost", fieldName)
	}

	// Resolve hostname to IP addresses to check for private IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// DNS lookup failure - this could be transient or the domain doesn't exist yet
		// For development/testing purposes, we'll allow this but log a warning
		log.Printf("[WARN] Could not resolve %s (%s) to validate IP address: %v", fieldName, hostname, err)
		return nil
	}

	// Check if any resolved IP is private or loopback (unless allowPrivate is true)
	if !allowPrivate {
		for _, ip := range ips {
			if isPrivateOrLoopbackIP(ip) {
				return fmt.Errorf("%s resolves to a private or loopback IP address (%s), which could be a security risk", fieldName, ip.String())
			}
		}
	}

	return nil
}

// isPrivateOrLoopbackIP checks if an IP address is private, loopback, or link-local.
func isPrivateOrLoopbackIP(ip net.IP) bool {
	// Check for loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	// 10.0.0.0/8
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges (fc00::/7 - Unique Local Addresses)
	if len(ip) == net.IPv6len && ip[0] == 0xfc || ip[0] == 0xfd {
		return true
	}

	return false
}
