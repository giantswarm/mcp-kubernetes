package cmd

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCIMDPreRegisteredClientMetadata(t *testing.T) {
	documents, err := parseCIMDPreRegisteredClientMetadata(`[{"client_id":"https://muster.example.com/.well-known/oauth-client.json","client_name":"muster","redirect_uris":["https://muster.example.com/oauth/callback"]}]`)
	require.NoError(t, err)
	require.Len(t, documents, 1)
	require.Equal(t, "https://muster.example.com/.well-known/oauth-client.json", documents[0].ClientID)
	require.Equal(t, []string{"https://muster.example.com/oauth/callback"}, documents[0].RedirectURIs)

	documents, err = parseCIMDPreRegisteredClientMetadata("")
	require.NoError(t, err)
	require.Empty(t, documents)

	for _, raw := range []string{
		`[{"client_id":"https://a.example/c.json","redirect_uri":["https://a.example/cb"]}]`,
		`{"client_id":"https://a.example/c.json"}`,
		`[] []`,
		`[]]`,
		`[]}`,
		`[]]garbage`,
		`[] invalid`,
		`[{"client_id":"https://a.example/c.json","client_name":"client","redirect_uris":["https://a.example/cb"]}]]`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := parseCIMDPreRegisteredClientMetadata(raw)
			require.ErrorContains(t, err, cimdPreRegisteredClientMetadataEnv)
		})
	}

	documents, err = parseCIMDPreRegisteredClientMetadata(" [] \n\t")
	require.NoError(t, err)
	require.Empty(t, documents)
}

func TestLogDeprecatedPrivateIPSettings(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))

	logDeprecatedPrivateIPSettings(logger, OAuthServeConfig{
		CIMDPrivateIPHosts:  []string{"muster.internal"},
		RedirectURISecurity: RedirectURISecurityConfig{PrivateIPRedirectURIHosts: []string{"callback.internal"}},
	})
	require.Contains(t, output.String(), "--cimd-private-ip-hosts")
	require.NotContains(t, output.String(), "--cimd-allow-private-ips")
	require.NotContains(t, output.String(), "redirect")

	output.Reset()
	logDeprecatedPrivateIPSettings(logger, OAuthServeConfig{
		CIMDAllowPrivateIPs: true,
		RedirectURISecurity: RedirectURISecurityConfig{AllowPrivateIPRedirectURIs: true},
	})
	require.Contains(t, output.String(), "level=WARN")
	require.Contains(t, output.String(), "CIMD_ALLOW_PRIVATE_IPS")
	require.Contains(t, output.String(), "mcpKubernetes.oauth.cimd.preRegisteredClientMetadata")
	require.Contains(t, output.String(), "mcpKubernetes.oauth.redirectURISecurity.privateIPHosts")

	output.Reset()
	logDeprecatedPrivateIPSettings(logger, OAuthServeConfig{})
	require.Empty(t, output.String())
}
