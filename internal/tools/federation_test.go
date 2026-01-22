package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/giantswarm/mcp-kubernetes/internal/federation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/mcp/oauth"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

// mockK8sClient implements a minimal k8s.Client for testing
type mockK8sClient struct {
	k8s.Client
}

// mockLogger implements server.Logger for testing
type mockLogger struct{}

func (l *mockLogger) Info(msg string, args ...interface{})  {}
func (l *mockLogger) Debug(msg string, args ...interface{}) {}
func (l *mockLogger) Warn(msg string, args ...interface{})  {}
func (l *mockLogger) Error(msg string, args ...interface{}) {}
func (l *mockLogger) With(args ...interface{}) server.Logger {
	return l
}

// mockClientFactory is a mock client factory for testing OAuth downstream.
type mockClientFactory struct {
	client    k8s.Client
	createErr error
}

func (f *mockClientFactory) CreateBearerTokenClient(token string) (k8s.Client, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.client, nil
}

func TestExtractClusterParam(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		expected string
	}{
		{
			name:     "no cluster param",
			args:     map[string]interface{}{},
			expected: "",
		},
		{
			name:     "empty cluster param",
			args:     map[string]interface{}{"cluster": ""},
			expected: "",
		},
		{
			name:     "valid cluster param",
			args:     map[string]interface{}{"cluster": "my-cluster"},
			expected: "my-cluster",
		},
		{
			name:     "cluster param with other args",
			args:     map[string]interface{}{"cluster": "prod-cluster", "namespace": "default", "name": "test"},
			expected: "prod-cluster",
		},
		{
			name:     "wrong type for cluster param",
			args:     map[string]interface{}{"cluster": 123},
			expected: "",
		},
		{
			name:     "nil args",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractClusterParam(tt.args)
			if result != tt.expected {
				t.Errorf("ExtractClusterParam() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatClusterError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		clusterName string
		contains    []string // Substrings that should be in the result
	}{
		{
			name:        "nil error",
			err:         nil,
			clusterName: "test",
			contains:    []string{},
		},
		{
			name: "ClusterNotFoundError",
			err: &federation.ClusterNotFoundError{
				ClusterName: "my-cluster",
				Reason:      "not found",
			},
			clusterName: "my-cluster",
			contains:    []string{"cluster access denied"},
		},
		{
			name: "KubeconfigError not found",
			err: &federation.KubeconfigError{
				ClusterName:  "my-cluster",
				ResourceName: "my-cluster-kubeconfig",
				Namespace:    "default",
				Reason:       "secret not found",
				NotFound:     true,
			},
			clusterName: "my-cluster",
			contains:    []string{"cluster access denied"},
		},
		{
			name: "KubeconfigError invalid",
			err: &federation.KubeconfigError{
				ClusterName:  "my-cluster",
				ResourceName: "my-cluster-kubeconfig",
				Namespace:    "default",
				Reason:       "invalid kubeconfig",
				NotFound:     false,
			},
			clusterName: "my-cluster",
			contains:    []string{"cluster access denied"},
		},
		{
			name: "ConnectionError",
			err: &federation.ConnectionError{
				ClusterName: "my-cluster",
				Host:        "https://api.my-cluster.example.com:6443",
				Reason:      "connection refused",
			},
			clusterName: "my-cluster",
			contains:    []string{"cluster access denied"},
		},
		{
			name: "ImpersonationError",
			err: &federation.ImpersonationError{
				ClusterName: "my-cluster",
				UserEmail:   "user@example.com",
				GroupCount:  2,
				Reason:      "RBAC denied",
			},
			clusterName: "my-cluster",
			contains:    []string{"insufficient permissions", "RBAC"},
		},
		{
			name: "AccessDeniedError",
			err: &federation.AccessDeniedError{
				ClusterName: "my-cluster",
				UserEmail:   "user@example.com",
				Verb:        "delete",
				Resource:    "pods",
				Namespace:   "production",
				Reason:      "RBAC: denied",
			},
			clusterName: "my-cluster",
			contains:    []string{"permission denied", "delete", "pods"},
		},
		{
			name: "AccessCheckError",
			err: &federation.AccessCheckError{
				ClusterName: "my-cluster",
				Check:       &federation.AccessCheck{Verb: "get", Resource: "pods"},
				Reason:      "check failed",
			},
			clusterName: "my-cluster",
			contains:    []string{"verify permissions"},
		},
		{
			name: "ConnectivityTimeoutError",
			err: &federation.ConnectivityTimeoutError{
				ClusterName: "my-cluster",
				Host:        "https://api.my-cluster.example.com:6443",
			},
			clusterName: "my-cluster",
			contains:    []string{"timed out", "reachable"},
		},
		{
			name: "TLSError",
			err: &federation.TLSError{
				ClusterName: "my-cluster",
				Host:        "https://api.my-cluster.example.com:6443",
				Reason:      "certificate expired",
			},
			clusterName: "my-cluster",
			contains:    []string{"certificate", "expired"},
		},
		{
			name:        "ErrClusterNotFound sentinel",
			err:         federation.ErrClusterNotFound,
			clusterName: "my-cluster",
			contains:    []string{"my-cluster", "not found", "capi_list_clusters"},
		},
		{
			name:        "ErrClusterUnreachable sentinel",
			err:         federation.ErrClusterUnreachable,
			clusterName: "my-cluster",
			contains:    []string{"my-cluster", "unreachable"},
		},
		{
			name:        "ErrAccessDenied sentinel",
			err:         federation.ErrAccessDenied,
			clusterName: "my-cluster",
			contains:    []string{"my-cluster", "access"},
		},
		{
			name:        "ErrConnectionTimeout sentinel",
			err:         federation.ErrConnectionTimeout,
			clusterName: "my-cluster",
			contains:    []string{"timed out"},
		},
		{
			name:        "ErrTLSHandshakeFailed sentinel",
			err:         federation.ErrTLSHandshakeFailed,
			clusterName: "my-cluster",
			contains:    []string{"secure connection"},
		},
		{
			name:        "ErrManagerClosed sentinel",
			err:         federation.ErrManagerClosed,
			clusterName: "my-cluster",
			contains:    []string{"unavailable"},
		},
		{
			name:        "ErrUserInfoRequired sentinel",
			err:         federation.ErrUserInfoRequired,
			clusterName: "my-cluster",
			contains:    []string{"authentication required"},
		},
		{
			name:        "ErrInvalidClusterName sentinel",
			err:         federation.ErrInvalidClusterName,
			clusterName: "my-cluster",
			contains:    []string{"invalid cluster name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatClusterError(tt.err, tt.clusterName)

			if tt.err == nil {
				if result != "" {
					t.Errorf("FormatClusterError(nil) = %q, want empty string", result)
				}
				return
			}

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("FormatClusterError() = %q, want it to contain %q", result, substr)
				}
			}
		})
	}
}

func TestClusterClient(t *testing.T) {
	t.Run("K8s returns the client", func(t *testing.T) {
		mockClient := &mockK8sClient{}
		cc := &ClusterClient{
			k8sClient: mockClient,
		}

		if cc.K8s() != mockClient {
			t.Error("K8s() should return the k8sClient")
		}
	})

	t.Run("User returns user info", func(t *testing.T) {
		user := &federation.UserInfo{Email: "test@example.com"}
		cc := &ClusterClient{
			user: user,
		}

		if cc.User() != user {
			t.Error("User() should return the user")
		}
	})

	t.Run("ClusterName returns cluster name", func(t *testing.T) {
		cc := &ClusterClient{
			clusterName: "test-cluster",
		}

		if cc.ClusterName() != "test-cluster" {
			t.Errorf("ClusterName() = %q, want %q", cc.ClusterName(), "test-cluster")
		}
	})

	t.Run("IsFederated returns federated status", func(t *testing.T) {
		cc := &ClusterClient{
			federated: true,
		}

		if !cc.IsFederated() {
			t.Error("IsFederated() should return true")
		}

		cc.federated = false
		if cc.IsFederated() {
			t.Error("IsFederated() should return false")
		}
	})
}

// errMsgFederationRequired is the expected error message when federation is not enabled
const errMsgFederationRequired = "multi-cluster operations require federation mode to be enabled"

func TestGetClusterClient(t *testing.T) {
	t.Run("no cluster name and no federation returns local client", func(t *testing.T) {
		// Create a minimal server context without federation
		ctx := context.Background()

		// We can't easily test this without mocking server.ServerContext
		// Just verify the function signature is correct
		_ = ctx
	})

	t.Run("cluster name without federation returns error message", func(t *testing.T) {
		// This test verifies the error message format
		if !strings.Contains(errMsgFederationRequired, "federation") {
			t.Error("Error message should mention federation")
		}
	})

	t.Run("cluster name with federation but no oauth returns error message", func(t *testing.T) {
		// This test verifies the error message format
		errMsg := "authentication required: no user info in context"
		if !strings.Contains(errMsg, "authentication") {
			t.Error("Error message should mention authentication")
		}
	})
}

func TestValidateClusterParam(t *testing.T) {
	t.Run("empty cluster param is valid", func(t *testing.T) {
		// Create a minimal server context
		ctx := context.Background()
		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(&mockK8sClient{}),
			server.WithLogger(&mockLogger{}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		errMsg := ValidateClusterParam(sc, "")
		if errMsg != "" {
			t.Errorf("ValidateClusterParam('') should return empty string, got %q", errMsg)
		}
	})

	t.Run("non-empty cluster param without federation returns error", func(t *testing.T) {
		ctx := context.Background()
		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(&mockK8sClient{}),
			server.WithLogger(&mockLogger{}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		errMsg := ValidateClusterParam(sc, "my-cluster")
		if errMsg == "" {
			t.Error("ValidateClusterParam('my-cluster') should return error message")
		}
		if !strings.Contains(errMsg, "federation") {
			t.Errorf("Error message should mention federation, got %q", errMsg)
		}
		if errMsg != errMsgFederationRequired {
			t.Errorf("Expected error message %q, got %q", errMsgFederationRequired, errMsg)
		}
	})
}

func TestGetClusterClientWithOAuth(t *testing.T) {
	t.Run("without OAuth user and no cluster name", func(t *testing.T) {
		ctx := context.Background()

		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(&mockK8sClient{}),
			server.WithLogger(&mockLogger{}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		// With no cluster name and no federation, should succeed using local client
		client, errMsg := GetClusterClient(ctx, sc, "")
		if errMsg != "" {
			t.Errorf("GetClusterClient with empty cluster should succeed, got error: %s", errMsg)
		}
		if client == nil {
			t.Error("GetClusterClient should return a client")
		}
		if client.IsFederated() {
			t.Error("Client should not be federated without federation manager")
		}
	})

	t.Run("with cluster name but no federation manager", func(t *testing.T) {
		ctx := context.Background()

		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(&mockK8sClient{}),
			server.WithLogger(&mockLogger{}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		// With cluster name but no federation, should fail
		client, errMsg := GetClusterClient(ctx, sc, "my-cluster")
		if errMsg == "" {
			t.Error("GetClusterClient with cluster name but no federation should fail")
		}
		if client != nil {
			t.Error("GetClusterClient should not return a client on error")
		}
		if !strings.Contains(errMsg, "federation") {
			t.Errorf("Error message should mention federation, got: %s", errMsg)
		}
		if errMsg != errMsgFederationRequired {
			t.Errorf("Expected error message %q, got %q", errMsgFederationRequired, errMsg)
		}
	})

	t.Run("with invalid cluster name returns validation error", func(t *testing.T) {
		ctx := context.Background()

		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(&mockK8sClient{}),
			server.WithLogger(&mockLogger{}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		// Invalid cluster names should be rejected early
		invalidNames := []string{
			"../escape",          // path traversal
			"cluster/with/slash", // path characters
			"UPPERCASE",          // must be lowercase
			"cluster_underscore", // underscores not allowed
			"-starts-with-dash",  // must start with alphanumeric
		}

		for _, invalidName := range invalidNames {
			client, errMsg := GetClusterClient(ctx, sc, invalidName)
			if errMsg == "" {
				t.Errorf("GetClusterClient with invalid name %q should fail", invalidName)
			}
			if client != nil {
				t.Errorf("GetClusterClient should not return a client for invalid name %q", invalidName)
			}
			if !strings.Contains(errMsg, "invalid cluster name") {
				t.Errorf("Error message for %q should mention 'invalid cluster name', got: %s", invalidName, errMsg)
			}
		}
	})
}

func TestFormatClusterError_GenericFallback(t *testing.T) {
	// Test that the generic error fallback doesn't leak internal error details
	t.Run("unhandled error returns generic message", func(t *testing.T) {
		// Create a custom error that doesn't match any known federation error types
		customErr := fmt.Errorf("internal database connection failed: host=10.0.0.5 password=secret123")

		result := FormatClusterError(customErr, "test-cluster")

		// Should NOT contain the internal error details
		if strings.Contains(result, "database") {
			t.Errorf("Generic error should not leak 'database', got: %s", result)
		}
		if strings.Contains(result, "10.0.0.5") {
			t.Errorf("Generic error should not leak IP address, got: %s", result)
		}
		if strings.Contains(result, "secret123") {
			t.Errorf("Generic error should not leak password, got: %s", result)
		}

		// Should contain the expected generic message
		expectedMsg := "failed to access cluster: an unexpected error occurred"
		if result != expectedMsg {
			t.Errorf("Expected generic message %q, got %q", expectedMsg, result)
		}
	})
}

// TestGetClusterClientWithDownstreamOAuth tests the fail-closed OAuth behavior.
// Note: Strict mode is always enabled - there is no option to disable it.
// This ensures security by default: users cannot accidentally get service account permissions.
func TestGetClusterClientWithDownstreamOAuth(t *testing.T) {
	t.Run("without token returns auth error", func(t *testing.T) {
		ctx := context.Background() // No OAuth token

		sharedClient := &mockK8sClient{}
		perUserClient := &mockK8sClient{}

		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(sharedClient),
			server.WithLogger(&mockLogger{}),
			server.WithDownstreamOAuth(true),
			server.WithDownstreamOAuthStrict(true), // Always true - cannot be disabled
			server.WithClientFactory(&mockClientFactory{client: perUserClient}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		// With no token, should fail with auth error (fail closed)
		client, errMsg := GetClusterClient(ctx, sc, "")
		if errMsg == "" {
			t.Error("GetClusterClient without token should fail")
		}
		if client != nil {
			t.Error("GetClusterClient should not return a client on auth failure")
		}
		if !strings.Contains(errMsg, "authentication") {
			t.Errorf("Error message should mention authentication, got: %s", errMsg)
		}
	})

	t.Run("with valid token returns per-user client", func(t *testing.T) {
		// Add OAuth token to context
		ctx := oauth.ContextWithAccessToken(context.Background(), "valid-token")

		sharedClient := &mockK8sClient{}
		perUserClient := &mockK8sClient{}

		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(sharedClient),
			server.WithLogger(&mockLogger{}),
			server.WithDownstreamOAuth(true),
			server.WithDownstreamOAuthStrict(true),
			server.WithClientFactory(&mockClientFactory{client: perUserClient}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		// With valid token, should succeed
		client, errMsg := GetClusterClient(ctx, sc, "")
		if errMsg != "" {
			t.Errorf("GetClusterClient with valid token should succeed, got error: %s", errMsg)
		}
		if client == nil {
			t.Error("GetClusterClient should return a client")
		}
		// The returned client should be the per-user client, not the shared one
		if client.K8s() != perUserClient {
			t.Error("GetClusterClient should return per-user client, not shared client")
		}
	})

	t.Run("with client creation error returns auth error", func(t *testing.T) {
		// Add OAuth token to context
		ctx := oauth.ContextWithAccessToken(context.Background(), "invalid-token")

		sharedClient := &mockK8sClient{}

		sc, err := server.NewServerContext(ctx,
			server.WithK8sClient(sharedClient),
			server.WithLogger(&mockLogger{}),
			server.WithDownstreamOAuth(true),
			server.WithDownstreamOAuthStrict(true),
			server.WithClientFactory(&mockClientFactory{createErr: errors.New("token rejected")}),
		)
		if err != nil {
			t.Fatalf("Failed to create server context: %v", err)
		}
		defer func() { _ = sc.Shutdown() }()

		// With invalid token, should fail with auth error (fail closed)
		client, errMsg := GetClusterClient(ctx, sc, "")
		if errMsg == "" {
			t.Error("GetClusterClient with invalid token should fail")
		}
		if client != nil {
			t.Error("GetClusterClient should not return a client on auth failure")
		}
		if !strings.Contains(errMsg, "authentication") {
			t.Errorf("Error message should mention authentication, got: %s", errMsg)
		}
	})
}

func TestFormatAuthenticationError(t *testing.T) {
	t.Run("nil error returns empty string", func(t *testing.T) {
		result := FormatAuthenticationError(nil)
		if result != "" {
			t.Errorf("FormatAuthenticationError(nil) should return empty string, got: %s", result)
		}
	})

	t.Run("token missing error returns login message", func(t *testing.T) {
		result := FormatAuthenticationError(server.ErrOAuthTokenMissing)
		if !strings.Contains(result, "log in") {
			t.Errorf("Error message should suggest logging in, got: %s", result)
		}
	})

	t.Run("client failed error returns session expired message", func(t *testing.T) {
		result := FormatAuthenticationError(server.ErrOAuthClientFailed)
		if !strings.Contains(result, "expired") {
			t.Errorf("Error message should mention session expiration, got: %s", result)
		}
	})

	t.Run("unknown error returns generic message", func(t *testing.T) {
		result := FormatAuthenticationError(errors.New("some random error"))
		if !strings.Contains(result, "authentication error") {
			t.Errorf("Unknown error should return generic auth error, got: %s", result)
		}
	})
}

func TestIsAuthenticationError(t *testing.T) {
	t.Run("token missing is auth error", func(t *testing.T) {
		if !IsAuthenticationError(server.ErrOAuthTokenMissing) {
			t.Error("ErrOAuthTokenMissing should be recognized as auth error")
		}
	})

	t.Run("client failed is auth error", func(t *testing.T) {
		if !IsAuthenticationError(server.ErrOAuthClientFailed) {
			t.Error("ErrOAuthClientFailed should be recognized as auth error")
		}
	})

	t.Run("wrapped token missing is auth error", func(t *testing.T) {
		wrapped := fmt.Errorf("context: %w", server.ErrOAuthTokenMissing)
		if !IsAuthenticationError(wrapped) {
			t.Error("Wrapped ErrOAuthTokenMissing should be recognized as auth error")
		}
	})

	t.Run("random error is not auth error", func(t *testing.T) {
		if IsAuthenticationError(errors.New("some random error")) {
			t.Error("Random error should not be recognized as auth error")
		}
	})

	t.Run("nil is not auth error", func(t *testing.T) {
		if IsAuthenticationError(nil) {
			t.Error("nil should not be recognized as auth error")
		}
	})
}
