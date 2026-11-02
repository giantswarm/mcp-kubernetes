package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
)

// Tracing makes every request a server span joined to the caller's
// traceparent, named after the route pattern that served it. The probes are
// left out: the kubelet calls them every few seconds and they carry no caller.
// Without a configured exporter the global tracer provider is a no-op.
//
// The W3C propagator is set here rather than relied on as a global, because
// nothing installs a global one and the default drops the traceparent. With
// it, the request context carries the caller's trace ID even without an
// exporter, so an error reference matches the ID the caller logged.
func Tracing(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "mcp-kubernetes",
		otelhttp.WithFilter(traced),
		otelhttp.WithPropagators(propagation.TraceContext{}))
}

func traced(r *http.Request) bool {
	switch r.URL.Path {
	case "/healthz", "/readyz", "/metrics":
		return false
	}
	return true
}
