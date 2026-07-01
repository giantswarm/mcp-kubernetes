package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateImpersonationClient_UserAndGroupsOnly(t *testing.T) {
	factory := &InClusterImpersonationFactory{
		clusterHost: "https://k8s.example.com",
		logger:      &testLogger{},
	}

	t.Run("sends only Impersonate-User and Impersonate-Group, no extras", func(t *testing.T) {
		identity := ImpersonationIdentity{
			UserName: "human@example.com",
			Groups:   []string{"system:authenticated"},
		}

		client, err := factory.CreateImpersonationClient(identity)
		require.NoError(t, err)
		require.NotNil(t, client)

		ic, ok := client.(*impersonationClient)
		require.True(t, ok)
		cfg := ic.restConfig

		require.Equal(t, "human@example.com", cfg.Impersonate.UserName)
		require.Equal(t, []string{"system:authenticated"}, cfg.Impersonate.Groups)
		require.Empty(t, cfg.Impersonate.Extra, "no Impersonate-Extra-* headers must be sent")
	})

	t.Run("empty UserName returns error", func(t *testing.T) {
		_, err := factory.CreateImpersonationClient(ImpersonationIdentity{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "non-empty UserName")
	})
}
