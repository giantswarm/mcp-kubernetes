package logging

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSlogAdapter(t *testing.T) {
	t.Run("with nil logger uses default", func(t *testing.T) {
		adapter := NewSlogAdapter(nil)
		assert.NotNil(t, adapter)
		assert.NotNil(t, adapter.Logger())
	})

	t.Run("with custom logger", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := slog.New(handler)

		adapter := NewSlogAdapter(logger)
		assert.NotNil(t, adapter)
		assert.Equal(t, logger, adapter.Logger())
	})
}

func TestSlogAdapterLogging(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)
	adapter := NewSlogAdapter(logger)

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		// Note: Debug level is typically not logged unless configured
		// This test just ensures no panic
		adapter.Debug("debug message", "key", "value")
	})

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		adapter.Info("info message", "key", "value")
		output := buf.String()
		assert.Contains(t, output, "info message")
		assert.Contains(t, output, "key")
		assert.Contains(t, output, "value")
	})

	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		adapter.Warn("warn message", "key", "value")
		output := buf.String()
		assert.Contains(t, output, "warn message")
		assert.Contains(t, output, "WARN")
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		adapter.Error("error message", "key", "value")
		output := buf.String()
		assert.Contains(t, output, "error message")
		assert.Contains(t, output, "ERROR")
	})
}

func TestDefaultLogger(t *testing.T) {
	adapter := DefaultLogger()
	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.Logger())
}

func TestSlogAdapterImplementsLogger(t *testing.T) {
	// Verify that SlogAdapter implements the Logger interface
	var _ Logger = (*SlogAdapter)(nil)
}
