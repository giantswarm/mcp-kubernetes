package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Tracing makes every request a server span joined to the caller's
// traceparent, named after the route pattern that served it. The probes are
// left out: the kubelet calls them every few seconds and they carry no caller.
// Without a configured exporter the global tracer provider is a no-op.
func Tracing(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "mcp-kubernetes", otelhttp.WithFilter(traced))
}

func traced(r *http.Request) bool {
	switch r.URL.Path {
	case "/healthz", "/readyz", "/metrics":
		return false
	}
	return true
}
