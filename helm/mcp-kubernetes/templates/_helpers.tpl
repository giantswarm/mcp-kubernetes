{{/*
Expand the name of the chart.
*/}}
{{- define "mcp-kubernetes.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "mcp-kubernetes.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label. A label value is at
most 63 characters and begins and ends alphanumeric: the cut of a long version
(a branch build's <version>-dev.<branch>.<date>.<time>.<sha>, or the
<version>+<digest> helm-controller installs) can land on any run of ".", "_"
(from "+") and "-", so the whole run is trimmed.
*/}}
{{- define "mcp-kubernetes.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimAll "-._" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "mcp-kubernetes.labels" -}}
helm.sh/chart: {{ include "mcp-kubernetes.chart" . }}
{{ include "mcp-kubernetes.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
application.giantswarm.io/team: {{ index .Chart.Annotations "io.giantswarm.application.team" | quote }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "mcp-kubernetes.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mcp-kubernetes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "mcp-kubernetes.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "mcp-kubernetes.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Platform identity contract (global.identity) fallbacks for the OAuth settings.

Helm forwards `global.*` to every sub-chart, so a parent chart such as the
agent-platform meta chart can describe the platform's single identity provider
once (global.identity.issuerUrl / clientId / existingSecret / ca, global.domain)
and have this chart pick it up. Each helper returns the explicit local value
(mcpKubernetes.oauth.*) when it is set and falls back to global.identity only
when the local value is empty. Without global.identity the helpers reduce to the
local values, so a chart installed on its own behaves exactly as before.
*/}}

{{/*
The global.identity block, or an empty dict when the parent set none.
*/}}
{{- define "mcp-kubernetes.oauth.globalIdentity" -}}
{{- dig "identity" (dict) (.Values.global | default dict) | default (dict) | toJson -}}
{{- end }}

{{/*
Dex issuer URL: mcpKubernetes.oauth.dex.issuerURL, else global.identity.issuerUrl.
*/}}
{{- define "mcp-kubernetes.oauth.dexIssuerURL" -}}
{{- $identity := include "mcp-kubernetes.oauth.globalIdentity" . | fromJson -}}
{{- .Values.mcpKubernetes.oauth.dex.issuerURL | default (dig "issuerUrl" "" $identity) -}}
{{- end }}

{{/*
Dex client ID: mcpKubernetes.oauth.dex.clientID, else global.identity.clientId.
*/}}
{{- define "mcp-kubernetes.oauth.dexClientID" -}}
{{- $identity := include "mcp-kubernetes.oauth.globalIdentity" . | fromJson -}}
{{- .Values.mcpKubernetes.oauth.dex.clientID | default (dig "clientId" "" $identity) -}}
{{- end }}

{{/*
Existing OAuth credentials Secret: mcpKubernetes.oauth.existingSecret, else
global.identity.existingSecret. Empty when neither is set, in which case the
chart creates its own Secret (templates/oauth-secret.yaml). The platform Secret
carries the same keys this chart reads: dex-client-secret, registration-token,
oauth-encryption-key.
*/}}
{{- define "mcp-kubernetes.oauth.existingSecret" -}}
{{- $identity := include "mcp-kubernetes.oauth.globalIdentity" . | fromJson -}}
{{- .Values.mcpKubernetes.oauth.existingSecret | default (dig "existingSecret" "" $identity) -}}
{{- end }}

{{/*
Name of the Secret the Deployment reads OAuth credentials from: the existing
Secret when one is configured, else the chart-managed <fullname>-oauth.
*/}}
{{- define "mcp-kubernetes.oauth.secretName" -}}
{{- include "mcp-kubernetes.oauth.existingSecret" . | default (printf "%s-oauth" (include "mcp-kubernetes.fullname" .)) -}}
{{- end }}

{{/*
Secret holding the Dex CA certificate: mcpKubernetes.oauth.dex.caSecret.name,
else global.identity.ca.secretName. Empty means Dex is verified against the
system trust store.
*/}}
{{- define "mcp-kubernetes.oauth.caSecretName" -}}
{{- $identity := include "mcp-kubernetes.oauth.globalIdentity" . | fromJson -}}
{{- .Values.mcpKubernetes.oauth.dex.caSecret.name | default (dig "ca" "secretName" "" $identity) -}}
{{- end }}

{{/*
Key of the CA certificate inside that Secret. The key follows the source of
the Secret name: mcpKubernetes.oauth.dex.caSecret.key when the local Secret
name is set, global.identity.ca.key when the name comes from global.identity;
both default to ca.crt.
*/}}
{{- define "mcp-kubernetes.oauth.caSecretKey" -}}
{{- $identity := include "mcp-kubernetes.oauth.globalIdentity" . | fromJson -}}
{{- if .Values.mcpKubernetes.oauth.dex.caSecret.name -}}
{{- .Values.mcpKubernetes.oauth.dex.caSecret.key | default "ca.crt" -}}
{{- else -}}
{{- dig "ca" "key" "" $identity | default "ca.crt" -}}
{{- end -}}
{{- end }}

{{/*
Trusted audiences for SSO token forwarding, comma-separated as the
OAUTH_TRUSTED_AUDIENCES env expects: mcpKubernetes.oauth.trustedAudiences, else
the platform client global.identity.clientId (the audience of the IdP id_token
muster forwards). Empty when neither is set.
*/}}
{{- define "mcp-kubernetes.oauth.trustedAudiences" -}}
{{- $identity := include "mcp-kubernetes.oauth.globalIdentity" . | fromJson -}}
{{- if .Values.mcpKubernetes.oauth.trustedAudiences -}}
{{- .Values.mcpKubernetes.oauth.trustedAudiences | join "," -}}
{{- else -}}
{{- dig "clientId" "" $identity -}}
{{- end -}}
{{- end }}

{{/*
OAuth base URL: mcpKubernetes.oauth.baseURL, else https://<fullname>.<global.domain>
following the platform's hostname convention. Empty when neither is set.
*/}}
{{- define "mcp-kubernetes.oauth.baseURL" -}}
{{- $domain := dig "domain" "" (.Values.global | default dict) -}}
{{- if .Values.mcpKubernetes.oauth.baseURL -}}
{{- .Values.mcpKubernetes.oauth.baseURL -}}
{{- else if $domain -}}
{{- printf "https://%s.%s" (include "mcp-kubernetes.fullname" .) $domain -}}
{{- end -}}
{{- end }}

{{/*
Data of the OAuth credentials Secret the chart renders (templates/oauth-secret.yaml),
key by key with the required-value checks. Its SHA-256 is the pod template's
checksum/oauth-secret annotation, so the server rolls when a credential changes.
*/}}
{{- define "mcp-kubernetes.oauthSecretData" -}}
{{- $oauth := .Values.mcpKubernetes.oauth -}}
{{- $data := dict -}}
{{- if eq ($oauth.provider | default "dex") "google" -}}
{{- if $oauth.google.clientID -}}
{{- $_ := set $data "google-client-id" ($oauth.google.clientID | b64enc) -}}
{{- else -}}
{{- fail "mcpKubernetes.oauth.google.clientID is required when Google provider is used and existingSecret is not set" -}}
{{- end -}}
{{- if $oauth.google.clientSecret -}}
{{- $_ := set $data "google-client-secret" ($oauth.google.clientSecret | b64enc) -}}
{{- else -}}
{{- fail "mcpKubernetes.oauth.google.clientSecret is required when Google provider is used and existingSecret is not set" -}}
{{- end -}}
{{- else if eq ($oauth.provider | default "dex") "dex" -}}
{{- if $oauth.dex.clientSecret -}}
{{- $_ := set $data "dex-client-secret" ($oauth.dex.clientSecret | b64enc) -}}
{{- else -}}
{{- fail "mcpKubernetes.oauth.dex.clientSecret is required when Dex provider is used and existingSecret is not set" -}}
{{- end -}}
{{- end -}}
{{- /* Registration token is required unless:
      1. allowPublicRegistration is true (anyone can register), OR
      2. trustedPublicRegistrationSchemes is configured (Cursor/VSCode can register without token)
*/ -}}
{{- if not $oauth.allowPublicRegistration -}}
{{- if $oauth.registrationAccessToken -}}
{{- $_ := set $data "registration-token" ($oauth.registrationAccessToken | b64enc) -}}
{{- else if not $oauth.trustedPublicRegistrationSchemes -}}
{{- fail "mcpKubernetes.oauth.registrationAccessToken is required when OAuth is enabled, allowPublicRegistration is false, trustedPublicRegistrationSchemes is empty, and existingSecret is not set. Either set a registrationAccessToken, enable allowPublicRegistration, or configure trustedPublicRegistrationSchemes for Cursor/VSCode compatibility." -}}
{{- end -}}
{{- end -}}
{{- if $oauth.encryptionKey -}}
{{- if $oauth.encryptionKeyValue -}}
{{- $_ := set $data "oauth-encryption-key" ($oauth.encryptionKeyValue | b64enc) -}}
{{- else -}}
{{- fail "mcpKubernetes.oauth.encryptionKeyValue is required when encryptionKey is true and existingSecret is not set" -}}
{{- end -}}
{{- end -}}
{{- /* Valkey password - only when Valkey storage is configured and no existing secret holds it */ -}}
{{- if and (eq $oauth.storage.type "valkey") (not $oauth.storage.valkey.existingSecret) $oauth.storage.valkey.password -}}
{{- $_ := set $data "valkey-password" ($oauth.storage.valkey.password | b64enc) -}}
{{- end -}}
{{- toYaml $data -}}
{{- end }}

{{/*
Value of the pod template's checksum/oauth-secret annotation: the SHA-256 of
the chart-rendered Secret's data, or mcpKubernetes.oauth.existingSecretChecksum
verbatim when the credentials come from an existing Secret the chart cannot
read. Empty while OAuth is off, or while nothing marks the existing Secret's
revision.
*/}}
{{- define "mcp-kubernetes.oauthSecretChecksum" -}}
{{- if .Values.mcpKubernetes.oauth.enabled -}}
{{- if include "mcp-kubernetes.oauth.existingSecret" . -}}
{{- .Values.mcpKubernetes.oauth.existingSecretChecksum -}}
{{- else -}}
{{- include "mcp-kubernetes.oauthSecretData" . | sha256sum -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Value of the pod template's checksum/valkey-secret annotation:
mcpKubernetes.oauth.storage.valkey.existingSecretChecksum verbatim while the
Valkey password comes from its own existing Secret. Empty otherwise: a password
in the OAuth Secret is covered by checksum/oauth-secret.
*/}}
{{- define "mcp-kubernetes.valkeySecretChecksum" -}}
{{- $valkey := .Values.mcpKubernetes.oauth.storage.valkey -}}
{{- if and .Values.mcpKubernetes.oauth.enabled (eq .Values.mcpKubernetes.oauth.storage.type "valkey") $valkey.existingSecret -}}
{{- $valkey.existingSecretChecksum -}}
{{- end -}}
{{- end }}
