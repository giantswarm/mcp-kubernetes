package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	oauthserver "github.com/giantswarm/mcp-oauth/server"
)

// cimdPreRegisteredClientMetadataEnv holds a JSON list of Client ID Metadata
// Documents, each in the document's own snake_case format.
const cimdPreRegisteredClientMetadataEnv = "CIMD_PRE_REGISTERED_CLIENT_METADATA"

// parseCIMDPreRegisteredClientMetadata decodes the pinned documents. Unknown
// fields are refused so a misspelled field cannot drop a redirect URI silently;
// mcp-oauth validates the documents themselves at startup.
func parseCIMDPreRegisteredClientMetadata(raw string) ([]oauthserver.ClientMetadata, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var documents []oauthserver.ClientMetadata
	if err := decoder.Decode(&documents); err != nil {
		return nil, fmt.Errorf("invalid %s: %w (expected a JSON list of Client ID Metadata Documents)", cimdPreRegisteredClientMetadataEnv, err)
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, fmt.Errorf("invalid %s: trailing data after the JSON list", cimdPreRegisteredClientMetadataEnv)
	}
	return documents, nil
}

// logDeprecatedPrivateIPSettings names the deployment setting behind each
// deprecated mcp-oauth private-address field. mcp-oauth logs its own warning
// for the field.
func logDeprecatedPrivateIPSettings(logger *slog.Logger, config OAuthServeConfig) {
	const pinCIMD = "pin the clients in mcpKubernetes.oauth.cimd.preRegisteredClientMetadata (" + cimdPreRegisteredClientMetadataEnv + ") instead"
	if config.CIMDAllowPrivateIPs {
		logger.Warn("--cimd-allow-private-ips (CIMD_ALLOW_PRIVATE_IPS, Helm mcpKubernetes.oauth.cimd.allowPrivateIPs) is deprecated and will be removed; " + pinCIMD)
	}
	if len(config.CIMDPrivateIPHosts) > 0 {
		logger.Warn("--cimd-private-ip-hosts (Helm mcpKubernetes.oauth.cimd.privateIPHosts) is deprecated and will be removed; " + pinCIMD)
	}
	if config.RedirectURISecurity.AllowPrivateIPRedirectURIs {
		logger.Warn("--allow-private-ip-redirect-uris (Helm mcpKubernetes.oauth.redirectURISecurity.allowPrivateIPRedirectURIs) is deprecated and will be removed; list the hosts in mcpKubernetes.oauth.redirectURISecurity.privateIPHosts (--private-ip-redirect-uri-hosts) instead")
	}
}
