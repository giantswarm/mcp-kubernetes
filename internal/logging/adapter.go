package logging

import (
	"log/slog"
)

// Logger is the canonical interface for structured logging throughout the application.
// It provides a simple, level-based logging API compatible with slog.
// The k8s package imports this interface via a type alias for backward compatibility.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// SlogAdapter adapts an slog.Logger to the Logger interface.
// This allows slog to be used with code that expects the simpler Logger interface.
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter creates a new SlogAdapter wrapping the given slog.Logger.
// If logger is nil, slog.Default() is used.
func NewSlogAdapter(logger *slog.Logger) *SlogAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &SlogAdapter{logger: logger}
}

// Debug logs a debug message with key-value pairs.
// Arguments should be provided as alternating key-value pairs: key1, value1, key2, value2, ...
func (a *SlogAdapter) Debug(msg string, args ...interface{}) {
	a.logger.Debug(msg, args...)
}

// Info logs an info message with key-value pairs.
// Arguments should be provided as alternating key-value pairs: key1, value1, key2, value2, ...
func (a *SlogAdapter) Info(msg string, args ...interface{}) {
	a.logger.Info(msg, args...)
}

// Warn logs a warning message with key-value pairs.
// Arguments should be provided as alternating key-value pairs: key1, value1, key2, value2, ...
func (a *SlogAdapter) Warn(msg string, args ...interface{}) {
	a.logger.Warn(msg, args...)
}

// Error logs an error message with key-value pairs.
// Arguments should be provided as alternating key-value pairs: key1, value1, key2, value2, ...
func (a *SlogAdapter) Error(msg string, args ...interface{}) {
	a.logger.Error(msg, args...)
}

// Logger returns the underlying slog.Logger for direct access when needed.
func (a *SlogAdapter) Logger() *slog.Logger {
	return a.logger
}

// DefaultLogger returns a Logger using the default slog.Logger.
func DefaultLogger() *SlogAdapter {
	return NewSlogAdapter(slog.Default())
}
