// Package logging provides structured logging utilities for the mcp-kubernetes application.
//
// This package centralizes logging patterns to ensure consistent, structured logging
// throughout the codebase using the standard library's slog package.
//
// # Key Features
//
//   - Structured logging with slog
//   - PII sanitization (email anonymization, credential masking)
//   - Host/URL sanitization for security
//   - Consistent attribute naming across the codebase
//
// # Usage Patterns
//
// Create a logger with standard attributes:
//
//	logger := logging.WithOperation(slog.Default(), "resource.list")
//	logger.Info("listing resources",
//	    logging.Namespace("default"),
//	    logging.ResourceType("pods"))
//
// Sanitize sensitive data before logging:
//
//	logger.Info("user operation",
//	    logging.UserHash(email),
//	    logging.SanitizedHost(apiServer))
//
// # Security Considerations
//
// This package is designed with security in mind:
//   - User emails are hashed to prevent PII leakage while allowing correlation
//   - API server URLs have IP addresses redacted to prevent topology leakage
//   - Credentials and tokens are never logged directly
package logging
