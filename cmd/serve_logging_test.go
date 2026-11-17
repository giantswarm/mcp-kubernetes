package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureStderr swaps os.Stderr for a pipe while fn runs and returns what
// fn wrote to it.
func captureStderr(t *testing.T, fn func()) []byte {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = original })

	done := make(chan []byte)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		done <- buf.Bytes()
	}()

	fn()

	os.Stderr = original
	require.NoError(t, writer.Close())
	return <-done
}

func stderrLines(t *testing.T, output []byte) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record), "stderr line is not JSON: %s", line)
		records = append(records, record)
	}
	return records
}

func TestInitLoggingMirrorsOTLPRecordsToStderr(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "")

	for _, tc := range []struct {
		name        string
		debug       bool
		wantMessage []string
	}{
		{name: "info", debug: false, wantMessage: []string{"info record"}},
		{name: "debug", debug: true, wantMessage: []string{"debug record", "info record"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureStderr(t, func() {
				logger, shutdown, err := initLogging(t.Context(), tc.debug)
				require.NoError(t, err)
				logger.DebugContext(t.Context(), "debug record")
				logger.InfoContext(t.Context(), "info record")
				// The endpoint is unreachable: a cancelled context stops the
				// flush instead of waiting out the exporter's retries.
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				_ = shutdown(ctx)
			})

			var messages []string
			for _, record := range stderrLines(t, output) {
				messages = append(messages, record["msg"].(string))
			}
			require.Equal(t, tc.wantMessage, messages)
		})
	}
}

func TestInitLoggingWithoutOTLPWritesStderrOnce(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

	output := captureStderr(t, func() {
		logger, shutdown, err := initLogging(t.Context(), true)
		require.NoError(t, err)
		logger.InfoContext(t.Context(), "info record")
		require.NoError(t, shutdown(t.Context()))
	})

	records := stderrLines(t, output)
	require.Len(t, records, 1)
	require.Equal(t, "info record", records[0]["msg"])
}
