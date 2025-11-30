// Package k8s provides tests for bearer token-based Kubernetes client authentication.
// These tests verify the functionality of creating and using Kubernetes clients
// with OAuth bearer tokens for downstream authentication.
package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBearerTokenClientFactory tests the creation of the bearer token client factory.
func TestBearerTokenClientFactory_CreateBearerTokenClient(t *testing.T) {
	// Note: This test cannot fully test the factory since it requires in-cluster config.
	// We test the validation logic and error cases instead.

	t.Run("empty bearer token returns error", func(t *testing.T) {
		// Create a mock factory with test values
		factory := &BearerTokenClientFactory{
			clusterHost:      "https://kubernetes.default.svc",
			caCertFile:       "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
			qpsLimit:         20.0,
			burstLimit:       30,
			timeout:          30 * time.Second,
			builtinResources: initBuiltinResources(),
		}

		client, err := factory.CreateBearerTokenClient("")
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "bearer token is required")
	})

	t.Run("valid bearer token creates client", func(t *testing.T) {
		factory := &BearerTokenClientFactory{
			clusterHost:      "https://kubernetes.default.svc",
			caCertFile:       "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
			qpsLimit:         20.0,
			burstLimit:       30,
			timeout:          30 * time.Second,
			builtinResources: initBuiltinResources(),
		}

		client, err := factory.CreateBearerTokenClient("test-token")
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify the client is of the correct type
		bearerClient, ok := client.(*bearerTokenClient)
		require.True(t, ok)
		assert.Equal(t, "test-token", bearerClient.bearerToken)
		assert.Equal(t, factory.clusterHost, bearerClient.clusterHost)
	})
}

// TestBearerTokenClient_ContextMethods tests the context management methods.
func TestBearerTokenClient_ContextMethods(t *testing.T) {
	client := &bearerTokenClient{
		bearerToken:      "test-token",
		clusterHost:      "https://kubernetes.default.svc",
		caCertFile:       "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		builtinResources: initBuiltinResources(),
	}

	t.Run("ListContexts returns in-cluster context", func(t *testing.T) {
		contexts, err := client.ListContexts(context.Background())
		require.NoError(t, err)
		require.Len(t, contexts, 1)

		ctx := contexts[0]
		assert.Equal(t, "in-cluster", ctx.Name)
		assert.Equal(t, "in-cluster", ctx.Cluster)
		assert.Equal(t, "oauth-user", ctx.User)
		assert.True(t, ctx.Current)
	})

	t.Run("GetCurrentContext returns in-cluster context", func(t *testing.T) {
		ctx, err := client.GetCurrentContext(context.Background())
		require.NoError(t, err)
		require.NotNil(t, ctx)

		assert.Equal(t, "in-cluster", ctx.Name)
		assert.Equal(t, "oauth-user", ctx.User)
		assert.True(t, ctx.Current)
	})

	t.Run("SwitchContext to in-cluster succeeds", func(t *testing.T) {
		err := client.SwitchContext(context.Background(), "in-cluster")
		assert.NoError(t, err)
	})

	t.Run("SwitchContext to other context fails", func(t *testing.T) {
		err := client.SwitchContext(context.Background(), "other-context")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supports in-cluster context")
	})
}

// TestBearerTokenClient_OperationValidation tests the operation validation logic.
func TestBearerTokenClient_OperationValidation(t *testing.T) {
	t.Run("allowed operations check", func(t *testing.T) {
		client := &bearerTokenClient{
			bearerToken:       "test-token",
			allowedOperations: []string{"get", "list"},
		}

		// Allowed operations should pass
		assert.NoError(t, client.isOperationAllowed("get"))
		assert.NoError(t, client.isOperationAllowed("list"))

		// Non-allowed operations should fail
		err := client.isOperationAllowed("delete")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("non-destructive mode blocks destructive operations", func(t *testing.T) {
		client := &bearerTokenClient{
			bearerToken:        "test-token",
			nonDestructiveMode: true,
			dryRun:             false,
		}

		// Destructive operations should fail
		assert.Error(t, client.isOperationAllowed("delete"))
		assert.Error(t, client.isOperationAllowed("create"))
		assert.Error(t, client.isOperationAllowed("patch"))
		assert.Error(t, client.isOperationAllowed("scale"))
		assert.Error(t, client.isOperationAllowed("apply"))
	})

	t.Run("non-destructive mode with dry-run allows operations", func(t *testing.T) {
		client := &bearerTokenClient{
			bearerToken:        "test-token",
			nonDestructiveMode: true,
			dryRun:             true,
		}

		// Operations should pass in dry-run mode
		assert.NoError(t, client.isOperationAllowed("delete"))
		assert.NoError(t, client.isOperationAllowed("create"))
	})
}

// TestBearerTokenClient_NamespaceRestriction tests namespace restriction logic.
func TestBearerTokenClient_NamespaceRestriction(t *testing.T) {
	client := &bearerTokenClient{
		bearerToken:          "test-token",
		restrictedNamespaces: []string{"kube-system", "kube-public"},
	}

	t.Run("restricted namespace is blocked", func(t *testing.T) {
		err := client.isNamespaceRestricted("kube-system")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "restricted")

		err = client.isNamespaceRestricted("kube-public")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "restricted")
	})

	t.Run("non-restricted namespace is allowed", func(t *testing.T) {
		assert.NoError(t, client.isNamespaceRestricted("default"))
		assert.NoError(t, client.isNamespaceRestricted("my-namespace"))
	})
}

// TestInitBuiltinResources verifies the builtin resources mapping is complete.
func TestInitBuiltinResources(t *testing.T) {
	resources := initBuiltinResources()

	// Test common resource types
	expectedResources := []string{
		"pods", "pod",
		"services", "service", "svc",
		"deployments", "deployment", "deploy",
		"configmaps", "configmap", "cm",
		"secrets", "secret",
		"namespaces", "namespace", "ns",
		"nodes", "node",
		"ingresses", "ingress", "ing",
	}

	for _, resourceType := range expectedResources {
		_, exists := resources[resourceType]
		assert.True(t, exists, "Resource type %s should exist in builtin resources", resourceType)
	}

	// Verify pods GVR
	podGVR := resources["pods"]
	assert.Equal(t, "", podGVR.Group)
	assert.Equal(t, "v1", podGVR.Version)
	assert.Equal(t, "pods", podGVR.Resource)

	// Verify deployments GVR
	deployGVR := resources["deployments"]
	assert.Equal(t, "apps", deployGVR.Group)
	assert.Equal(t, "v1", deployGVR.Version)
	assert.Equal(t, "deployments", deployGVR.Resource)
}
