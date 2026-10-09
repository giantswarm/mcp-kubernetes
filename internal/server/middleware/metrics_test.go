package middleware

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
)

func TestResponseWriter_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		expectedCode int
	}{
		{
			name:         "captures 200 OK",
			statusCode:   http.StatusOK,
			expectedCode: http.StatusOK,
		},
		{
			name:         "captures 404 Not Found",
			statusCode:   http.StatusNotFound,
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "captures 500 Internal Server Error",
			statusCode:   http.StatusInternalServerError,
			expectedCode: http.StatusInternalServerError,
		},
		{
			name:         "captures 201 Created",
			statusCode:   http.StatusCreated,
			expectedCode: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			rw := newResponseWriter(recorder)

			rw.WriteHeader(tt.statusCode)

			assert.Equal(t, tt.expectedCode, rw.statusCode)
			assert.True(t, rw.written)
		})
	}
}

func TestResponseWriter_DefaultsTo200(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	// Write response body without explicitly setting status
	_, err := rw.Write([]byte("hello"))
	assert.NoError(t, err)

	// Default status should be 200 OK
	assert.Equal(t, http.StatusOK, rw.statusCode)
	assert.True(t, rw.written)
}

func TestResponseWriter_OnlyFirstWriteHeaderCounts(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	rw.WriteHeader(http.StatusAccepted)
	rw.WriteHeader(http.StatusBadRequest) // This should be ignored

	assert.Equal(t, http.StatusAccepted, rw.statusCode)
}

func TestResponseWriter_Flush(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	// Should not panic even if underlying doesn't support Flush
	rw.Flush()
}

func TestResponseWriter_Unwrap(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	assert.Equal(t, recorder, rw.Unwrap())
}

// newPrometheusProvider returns an enabled provider whose metrics the global
// Prometheus registry serves, as the application's metrics server does. Its
// shutdown at the end of the test takes its series out of the registry again.
func newPrometheusProvider(t *testing.T) *instrumentation.Provider {
	t.Helper()

	ctx := context.Background()
	provider, err := instrumentation.NewProvider(ctx, instrumentation.Config{
		ServiceName:     "mcp-kubernetes-middleware-test",
		ServiceVersion:  "test",
		Enabled:         true,
		MetricsExporter: instrumentation.ExporterPrometheus,
		TracingExporter: instrumentation.ExporterNone,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })

	return provider
}

// newRoutedHandler returns the metrics middleware around a ServeMux that
// answers 204 on every one of the given route patterns.
func newRoutedHandler(provider *instrumentation.Provider, patterns ...string) http.Handler {
	mux := http.NewServeMux()
	for _, pattern := range patterns {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}
	return HTTPMetrics(provider)(mux)
}

// serve sends one request with the given raw path through the handler. The
// path is set on the URL directly, as the server's request parser does, so it
// may carry bytes that are not valid UTF-8.
func serve(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = path
	req.URL.RawPath = ""
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// scrape answers GET /metrics from the global Prometheus registry, as the
// application's metrics server does.
func scrape(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec
}

var pathLabelPattern = regexp.MustCompile(`\bpath="([^"]*)"`)

// httpRequestPathLabels returns the distinct path label values of the
// http_requests_total series in a scrape.
func httpRequestPathLabels(metrics string) []string {
	seen := map[string]bool{}
	var labels []string
	for _, line := range strings.Split(metrics, "\n") {
		if !strings.HasPrefix(line, "http_requests_total{") {
			continue
		}
		m := pathLabelPattern.FindStringSubmatch(line)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		labels = append(labels, m[1])
	}
	return labels
}

func TestHTTPMetrics_LabelsByMatchedRoute(t *testing.T) {
	provider := newPrometheusProvider(t)
	handler := newRoutedHandler(provider, "/mcp", "/healthz")

	assert.Equal(t, http.StatusNoContent, serve(handler, "/mcp").Code)
	assert.Equal(t, http.StatusNoContent, serve(handler, "/healthz").Code)
	assert.Equal(t, http.StatusNotFound, serve(handler, "/nope").Code)
	assert.Equal(t, http.StatusNotFound, serve(handler, "/mcp/abc123xyz890def456").Code)

	rec := scrape(t)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.ElementsMatch(t, []string{"/mcp", "/healthz", "unmatched"}, httpRequestPathLabels(rec.Body.String()))
}

func TestHTTPMetrics_InvalidUTF8PathKeepsMetricsServing(t *testing.T) {
	provider := newPrometheusProvider(t)
	handler := newRoutedHandler(provider, "/mcp")

	// A scanner's request path that is not valid UTF-8: the Prometheus
	// registry refuses such a label value and would fail every later scrape.
	assert.Equal(t, http.StatusNotFound, serve(handler, "/\xc0").Code)

	rec := scrape(t)
	require.Equal(t, http.StatusOK, rec.Code, "scrape failed: %s", rec.Body.String())

	assert.ElementsMatch(t, []string{"unmatched"}, httpRequestPathLabels(rec.Body.String()))
}

func TestHTTPMetrics_LabelSetBoundedUnderRandomPaths(t *testing.T) {
	provider := newPrometheusProvider(t)
	routes := []string{"/mcp", "/healthz", "/readyz", "/oauth/token"}
	handler := newRoutedHandler(provider, routes...)

	// Random bytes of every value, so the paths include ones that are neither
	// valid UTF-8 nor clean; the property holds for any bytes, so no seed.
	for i := range 500 {
		raw := make([]byte, 1+i%24)
		_, err := rand.Read(raw)
		require.NoError(t, err)
		serve(handler, "/"+string(raw))
	}
	for _, route := range routes {
		serve(handler, route)
	}

	rec := scrape(t)
	require.Equal(t, http.StatusOK, rec.Code, "scrape failed: %s", rec.Body.String())

	assert.ElementsMatch(t, append(routes, "unmatched"), httpRequestPathLabels(rec.Body.String()))
}

func TestHTTPMetrics_NilProvider(t *testing.T) {
	// When provider is nil, the middleware should just pass through
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
}

func TestHTTPMetrics_MiddlewareChaining(t *testing.T) {
	// Test that the middleware properly chains to the next handler
	callOrder := []string{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callOrder = append(callOrder, "handler")
		w.WriteHeader(http.StatusCreated)
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("POST", "/api/resources", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, callOrder, "handler")
}

func TestHTTPMetrics_PreservesResponseBody(t *testing.T) {
	expectedBody := `{"status":"ok","data":{"id":123}}`

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, expectedBody, rec.Body.String())
}

func TestHTTPMetrics_CapturesErrorStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	middleware := HTTPMetrics(nil)(handler)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
