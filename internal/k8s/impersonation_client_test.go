package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateImpersonationClient_ActorInjectedIntoExtra(t *testing.T) {
	factory := &InClusterImpersonationFactory{
		clusterHost: "https://k8s.example.com",
		logger:      &testLogger{},
	}

	t.Run("Actor non-empty: injected as Extra[actor], original map not mutated", func(t *testing.T) {
		original := map[string][]string{
			"issuer": {"https://oidc.example.com"},
			"agent":  {"mcp-kubernetes"},
		}
		identity := ImpersonationIdentity{
			UserName: "human@example.com",
			Extra:    original,
			Actor:    "system:serviceaccount:agent-ns:agent-sa",
		}

		client, err := factory.CreateImpersonationClient(identity)
		require.NoError(t, err)
		require.NotNil(t, client)

		ic, ok := client.(*impersonationClient)
		require.True(t, ok)
		cfg := ic.restConfig

		require.Equal(t, "human@example.com", cfg.Impersonate.UserName)
		require.Equal(t, []string{"system:serviceaccount:agent-ns:agent-sa"}, cfg.Impersonate.Extra["actor"])
		require.Equal(t, []string{"https://oidc.example.com"}, cfg.Impersonate.Extra["issuer"])

		// original map must not be mutated
		require.Nil(t, original["actor"], "CreateImpersonationClient must not mutate the caller's Extra map")
	})

	t.Run("Actor empty: Extra[actor] absent, original map preserved", func(t *testing.T) {
		original := map[string][]string{
			"issuer": {"https://oidc.example.com"},
			"agent":  {"mcp-kubernetes"},
		}
		identity := ImpersonationIdentity{
			UserName: "system:serviceaccount:glean:bot",
			Extra:    original,
			// Actor is zero value
		}

		client, err := factory.CreateImpersonationClient(identity)
		require.NoError(t, err)

		ic, ok := client.(*impersonationClient)
		require.True(t, ok)

		require.Nil(t, ic.restConfig.Impersonate.Extra["actor"])
		require.Equal(t, original, ic.restConfig.Impersonate.Extra)
	})

	t.Run("empty UserName returns error", func(t *testing.T) {
		_, err := factory.CreateImpersonationClient(ImpersonationIdentity{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "non-empty UserName")
	})
}
