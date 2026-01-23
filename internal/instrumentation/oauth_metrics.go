package instrumentation

// ExpectedMCPOAuthMetrics returns the list of metric names that the mcp-oauth
// library exposes when instrumentation is enabled.
//
// These metrics are expected by the Grafana dashboards in:
//   - helm/mcp-kubernetes/dashboards/security-dashboard.json
//   - helm/mcp-kubernetes/dashboards/administrator-dashboard.json
//
// When mcp-kubernetes creates an OAuth server with instrumentation enabled
// (see internal/server/oauth_http.go), mcp-oauth registers these metrics
// to the global Prometheus registry alongside mcp-kubernetes metrics.
//
// # Metric Categories
//
// Security/Rate Limiting:
//   - oauth_rate_limit_exceeded_total: Rate limit violations by limiter_type
//   - oauth_redirect_uri_security_rejected_total: Redirect URI rejections by category
//
// Replay Attack Detection:
//   - oauth_code_reuse_detected_total: Authorization code reuse detection
//   - oauth_token_reuse_detected_total: Token reuse detection
//
// PKCE (Proof Key for Code Exchange):
//   - oauth_pkce_validation_failed_total: PKCE validation failures by method
//
// Client Management:
//   - oauth_client_registered_total: Client registrations by client_type
//   - storage_clients_count: Current count of registered clients
//
// CIMD (Client ID Metadata Documents):
//   - oauth_cimd_fetch_total: CIMD fetch operations by result
//
// # Usage
//
// This function is primarily used in integration tests to verify that
// mcp-oauth metrics are correctly exposed when OAuth is enabled:
//
//	expectedMetrics := instrumentation.ExpectedMCPOAuthMetrics()
//	for _, metric := range expectedMetrics {
//	    if !containsMetric(output, metric) {
//	        t.Errorf("Missing mcp-oauth metric: %s", metric)
//	    }
//	}
func ExpectedMCPOAuthMetrics() []string {
	return []string{
		// Rate limiting and security
		"oauth_rate_limit_exceeded_total",
		"oauth_redirect_uri_security_rejected_total",

		// Replay attack detection
		"oauth_code_reuse_detected_total",
		"oauth_token_reuse_detected_total",

		// PKCE validation
		"oauth_pkce_validation_failed_total",

		// Client registration
		"oauth_client_registered_total",

		// CIMD (Client ID Metadata Documents)
		"oauth_cimd_fetch_total",

		// Storage metrics
		"storage_clients_count",
		"storage_tokens_count",
	}
}
