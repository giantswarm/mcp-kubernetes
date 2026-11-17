package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/proto"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

func captureStderr(t *testing.T, fn func()) []byte {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, output.Close()) })
	original := os.Stderr
	os.Stderr = output
	defer func() { os.Stderr = original }()

	fn()

	contents, err := os.ReadFile(output.Name())
	require.NoError(t, err)
	return contents
}

func stderrMessages(t *testing.T, output []byte) []string {
	t.Helper()
	var messages []string
	for line := range bytes.SplitSeq(bytes.TrimSpace(output), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record struct {
			Message string `json:"msg"`
		}
		require.NoError(t, json.Unmarshal(line, &record), "stderr line is not JSON: %s", line)
		messages = append(messages, record.Message)
	}
	return messages
}

func newLogCollector(t *testing.T) (string, func() []*logsv1.LogRecord) {
	t.Helper()
	var mutex sync.Mutex
	var records []*logsv1.LogRecord
	collector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/logs" || request.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(writer, "invalid OTLP request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, "unreadable OTLP request", http.StatusBadRequest)
			return
		}
		var export collectorlogs.ExportLogsServiceRequest
		if err := proto.Unmarshal(body, &export); err != nil {
			http.Error(writer, "invalid OTLP payload", http.StatusBadRequest)
			return
		}
		mutex.Lock()
		for _, resource := range export.ResourceLogs {
			for _, scope := range resource.ScopeLogs {
				records = append(records, scope.LogRecords...)
			}
		}
		mutex.Unlock()
		writer.Header().Set("Content-Type", "application/x-protobuf")
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	return collector.URL, func() []*logsv1.LogRecord {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Clone(records)
	}
}

func TestInitLoggingMirrorsExportedRecordsToStderr(t *testing.T) {
	for _, endpointVariable := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"} {
		for _, debug := range []bool{false, true} {
			name := endpointVariable + "/info"
			if debug {
				name = endpointVariable + "/debug"
			}
			t.Run(name, func(t *testing.T) {
				collectorURL, exportedRecords := newLogCollector(t)
				t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
				t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
				t.Setenv("OTEL_LOGS_EXPORTER", "")
				t.Setenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "http/protobuf")
				if endpointVariable == "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT" {
					collectorURL += "/v1/logs"
				}
				t.Setenv(endpointVariable, collectorURL)

				output := captureStderr(t, func() {
					logger, shutdown, err := initLogging(t.Context(), debug)
					require.NoError(t, err)
					defer func() { require.NoError(t, shutdown(t.Context())) }()
					logger.DebugContext(t.Context(), "debug record")
					logger.InfoContext(t.Context(), "info record")
				})

				wantMessages := []string{"info record"}
				if debug {
					wantMessages = []string{"debug record", "info record"}
				}
				require.Equal(t, wantMessages, stderrMessages(t, output))
				var messages []string
				for _, record := range exportedRecords() {
					messages = append(messages, record.Body.GetStringValue())
				}
				require.Equal(t, []string{"debug record", "info record"}, messages)
			})
		}
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

	require.Equal(t, []string{"info record"}, stderrMessages(t, output))
}

func TestInitLoggingWithNoopExporterMirrorsToStderr(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	output := captureStderr(t, func() {
		logger, shutdown, err := initLogging(t.Context(), false)
		require.NoError(t, err)
		logger.InfoContext(t.Context(), "info record")
		require.NoError(t, shutdown(t.Context()))
	})
	require.Equal(t, []string{"info record"}, stderrMessages(t, output))
}

func TestOAuthLogsDoNotExposeInitialAccessToken(t *testing.T) {
	for _, initialAccessToken := range []string{"fixture8", "fixture8-long-initial-access-token"} {
		t.Run(initialAccessToken, func(t *testing.T) {
			collectorURL, exportedRecords := newLogCollector(t)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collectorURL)
			t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
			t.Setenv("OTEL_LOGS_EXPORTER", "")
			t.Setenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "http/protobuf")
			output := captureStderr(t, func() {
				logger, shutdown, err := initLogging(t.Context(), true)
				require.NoError(t, err)
				originalLogger := slog.Default()
				slog.SetDefault(logger)
				defer slog.SetDefault(originalLogger)
				defer func() { require.NoError(t, shutdown(t.Context())) }()
				oauthServer, _, err := server.CreateOAuthServer(server.OAuthConfig{
					BaseURL:                 "https://mcp.example.com",
					Provider:                server.OAuthProviderGoogle,
					GoogleClientID:          "test-client",
					GoogleClientSecret:      "test-secret",
					DebugMode:               true,
					RegistrationAccessToken: initialAccessToken,
				})
				require.NoError(t, err)
				defer func() { require.NoError(t, oauthServer.Shutdown(t.Context())) }()
				require.Same(t, logger, oauthServer.Logger)
				oauthServer.Logger.DebugContext(t.Context(), "oauth debug record")
			})
			require.NotContains(t, string(output), initialAccessToken[:8])
			require.NotContains(t, string(output), "token_prefix")
			require.Equal(t, 1, bytes.Count(output, []byte("oauth debug record")))
			var debugRecordCount int
			for _, record := range exportedRecords() {
				require.NotContains(t, record.String(), initialAccessToken[:8])
				require.NotContains(t, record.String(), "token_prefix")
				if record.Body.GetStringValue() == "oauth debug record" {
					debugRecordCount++
				}
			}
			require.Equal(t, 1, debugRecordCount)
		})
	}
}
