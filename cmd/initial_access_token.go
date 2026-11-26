package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

func resolveInitialAccessToken(command *cobra.Command, token, registrationToken string) string {
	if command.Flags().Changed("registration-token") {
		slog.Warn("DEPRECATED: --registration-token is set; use --initial-access-token or OAUTH_INITIAL_ACCESS_TOKEN")
	}
	for _, name := range []string{"REGISTRATION_TOKEN", "OAUTH_REGISTRATION_ACCESS_TOKEN"} {
		if os.Getenv(name) != "" {
			slog.Warn("DEPRECATED: "+name+" is set; use OAUTH_INITIAL_ACCESS_TOKEN", "replacement", "OAUTH_INITIAL_ACCESS_TOKEN")
		}
	}
	if command.Flags().Changed("initial-access-token") {
		return token
	}
	if value := os.Getenv("OAUTH_INITIAL_ACCESS_TOKEN"); value != "" {
		return value
	}
	if command.Flags().Changed("registration-token") {
		return registrationToken
	}
	if value := os.Getenv("OAUTH_REGISTRATION_ACCESS_TOKEN"); value != "" {
		return value
	}
	return os.Getenv("REGISTRATION_TOKEN")
}
