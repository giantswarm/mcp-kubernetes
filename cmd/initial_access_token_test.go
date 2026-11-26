package cmd

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitialAccessTokenOptions(t *testing.T) {
	for _, test := range []struct {
		name        string
		flags       []string
		environment map[string]string
		want        string
		deprecated  bool
	}{
		{name: "new flag", flags: []string{"--initial-access-token=current-secret"}, want: "current-secret"},
		{name: "old flag", flags: []string{"--registration-token=legacy-secret"}, want: "legacy-secret", deprecated: true},
		{name: "new flag wins", flags: []string{"--initial-access-token=current-secret", "--registration-token=legacy-secret"}, want: "current-secret", deprecated: true},
		{name: "new env", environment: map[string]string{"OAUTH_INITIAL_ACCESS_TOKEN": "current-secret"}, want: "current-secret"},
		{name: "old chart env", environment: map[string]string{"REGISTRATION_TOKEN": "legacy-secret"}, want: "legacy-secret", deprecated: true},
		{name: "old oauth env", environment: map[string]string{"OAUTH_REGISTRATION_ACCESS_TOKEN": "legacy-secret"}, want: "legacy-secret", deprecated: true},
		{name: "new env wins", flags: []string{"--registration-token=legacy-secret"}, environment: map[string]string{"OAUTH_INITIAL_ACCESS_TOKEN": "current-secret"}, want: "current-secret", deprecated: true},
		{name: "new flag wins over old env", flags: []string{"--initial-access-token=current-secret"}, environment: map[string]string{"REGISTRATION_TOKEN": "legacy-secret"}, want: "current-secret", deprecated: true},
		{name: "empty new flag wins", flags: []string{"--initial-access-token=", "--registration-token=legacy-secret"}, deprecated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range []string{"OAUTH_INITIAL_ACCESS_TOKEN", "OAUTH_REGISTRATION_ACCESS_TOKEN", "REGISTRATION_TOKEN"} {
				t.Setenv(name, test.environment[name])
			}
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			command := newServeCmd()
			require.NoError(t, command.ParseFlags(test.flags))
			token, err := command.Flags().GetString("initial-access-token")
			require.NoError(t, err)
			legacy, err := command.Flags().GetString("registration-token")
			require.NoError(t, err)
			require.Equal(t, test.want, resolveInitialAccessToken(command, token, legacy))
			if test.deprecated {
				require.Contains(t, output.String(), "DEPRECATED:")
				require.Contains(t, output.String(), "OAUTH_INITIAL_ACCESS_TOKEN")
			} else {
				require.Empty(t, output.String())
			}
			require.NotContains(t, output.String(), "current-secret")
			require.NotContains(t, output.String(), "legacy-secret")
		})
	}
}
