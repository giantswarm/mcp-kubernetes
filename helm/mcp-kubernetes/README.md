# mcp-kubernetes

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

A Helm chart for mcp-kubernetes - Model Context Protocol server for Kubernetes

**Homepage:** <https://github.com/giantswarm/mcp-kubernetes>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Giant Swarm | <team-bumblebee@giantswarm.io> |  |

## Source Code

* <https://github.com/giantswarm/mcp-kubernetes>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| global.domain | string | `""` |  |
| global.identity.issuerUrl | string | `""` |  |
| global.identity.clientId | string | `""` |  |
| global.identity.existingSecret | string | `""` |  |
| global.identity.ca.secretName | string | `""` |  |
| global.identity.ca.key | string | `"ca.crt"` |  |
| replicaCount | int | `1` |  |
| image.registry | string | `"gsoci.azurecr.io"` |  |
| image.repository | string | `"giantswarm/mcp-kubernetes"` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.tag | string | `""` |  |
| imagePullSecrets | list | `[]` |  |
| nameOverride | string | `""` |  |
| fullnameOverride | string | `""` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.name | string | `""` |  |
| rbac.create | bool | `true` |  |
| rbac.profile | string | `"standard"` |  |
| rbac.adminConfirmation | bool | `false` |  |
| rbac.custom.enabled | bool | `false` |  |
| rbac.custom.rules | list | `[]` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
| podSecurityContext.runAsUser | int | `1000` |  |
| podSecurityContext.runAsGroup | int | `1000` |  |
| podSecurityContext.runAsNonRoot | bool | `true` |  |
| podSecurityContext.fsGroup | int | `1000` |  |
| securityContext.readOnlyRootFilesystem | bool | `true` |  |
| securityContext.allowPrivilegeEscalation | bool | `false` |  |
| securityContext.runAsUser | int | `1000` |  |
| securityContext.runAsGroup | int | `1000` |  |
| securityContext.runAsNonRoot | bool | `true` |  |
| securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| service.type | string | `"ClusterIP"` |  |
| service.port | int | `8080` |  |
| ingress.enabled | bool | `false` |  |
| ingress.className | string | `""` |  |
| ingress.annotations | object | `{}` |  |
| ingress.hosts[0].host | string | `"chart-example.local"` |  |
| ingress.hosts[0].paths[0].path | string | `"/"` |  |
| ingress.hosts[0].paths[0].pathType | string | `"Prefix"` |  |
| ingress.tls | list | `[]` |  |
| gatewayAPI.enabled | bool | `false` |  |
| gatewayAPI.httpRoute.parentRefs | list | `[]` |  |
| gatewayAPI.httpRoute.hostnames | list | `[]` |  |
| gatewayAPI.httpRoute.rules | list | `[]` |  |
| gatewayAPI.httpRoute.annotations | object | `{}` |  |
| gatewayAPI.httpRoute.labels | object | `{}` |  |
| gatewayAPI.backendTrafficPolicy.enabled | bool | `false` |  |
| gatewayAPI.backendTrafficPolicy.timeout | string | `"0s"` |  |
| gatewayAPI.backendTrafficPolicy.annotations | object | `{}` |  |
| gatewayAPI.backendTrafficPolicy.labels | object | `{}` |  |
| resources.limits.cpu | string | `"500m"` |  |
| resources.limits.memory | string | `"512Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"128Mi"` |  |
| autoscaling.enabled | bool | `false` |  |
| autoscaling.minReplicas | int | `1` |  |
| autoscaling.maxReplicas | int | `100` |  |
| autoscaling.targetCPUUtilizationPercentage | int | `80` |  |
| volumes | list | `[]` |  |
| volumeMounts | list | `[]` |  |
| nodeSelector | object | `{}` |  |
| tolerations | list | `[]` |  |
| affinity | object | `{}` |  |
| mcpKubernetes.debug | bool | `false` |  |
| mcpKubernetes.kubernetes.inCluster | bool | `true` |  |
| mcpKubernetes.kubernetes.kubeconfig | string | `""` |  |
| mcpKubernetes.oauth.enabled | bool | `false` |  |
| mcpKubernetes.oauth.baseURL | string | `""` |  |
| mcpKubernetes.oauth.provider | string | `"dex"` |  |
| mcpKubernetes.oauth.google.clientID | string | `""` |  |
| mcpKubernetes.oauth.google.clientSecret | string | `""` |  |
| mcpKubernetes.oauth.dex.issuerURL | string | `""` |  |
| mcpKubernetes.oauth.dex.clientID | string | `""` |  |
| mcpKubernetes.oauth.dex.clientSecret | string | `""` |  |
| mcpKubernetes.oauth.dex.connectorID | string | `""` |  |
| mcpKubernetes.oauth.dex.kubernetesAuthenticatorClientID | string | `""` |  |
| mcpKubernetes.oauth.dex.caSecret.name | string | `""` |  |
| mcpKubernetes.oauth.dex.caSecret.key | string | `"ca.crt"` |  |
| mcpKubernetes.oauth.registrationAccessToken | string | `""` |  |
| mcpKubernetes.oauth.allowPublicRegistration | bool | `false` |  |
| mcpKubernetes.oauth.allowInsecureAuthWithoutState | bool | `false` |  |
| mcpKubernetes.oauth.allowPrivateURLs | bool | `false` |  |
| mcpKubernetes.oauth.maxClientsPerIP | int | `10` |  |
| mcpKubernetes.oauth.encryptionKey | bool | `false` |  |
| mcpKubernetes.oauth.disableStreaming | bool | `false` |  |
| mcpKubernetes.oauth.enableDownstreamOAuth | bool | `false` |  |
| mcpKubernetes.oauth.existingSecret | string | `""` |  |
| mcpKubernetes.oauth.storage.type | string | `"memory"` |  |
| mcpKubernetes.oauth.storage.valkey.url | string | `""` |  |
| mcpKubernetes.oauth.storage.valkey.password | string | `""` |  |
| mcpKubernetes.oauth.storage.valkey.tls.enabled | bool | `false` |  |
| mcpKubernetes.oauth.storage.valkey.keyPrefix | string | `"mcp:"` |  |
| mcpKubernetes.oauth.storage.valkey.db | int | `0` |  |
| mcpKubernetes.oauth.storage.valkey.existingSecret | string | `""` |  |
| mcpKubernetes.oauth.storage.valkey.secretKeyPassword | string | `"valkey-password"` |  |
| mcpKubernetes.oauth.redirectURISecurity.disableProductionMode | bool | `false` |  |
| mcpKubernetes.oauth.redirectURISecurity.allowLocalhostRedirectURIs | bool | `false` |  |
| mcpKubernetes.oauth.redirectURISecurity.allowPrivateIPRedirectURIs | bool | `false` |  |
| mcpKubernetes.oauth.redirectURISecurity.allowLinkLocalRedirectURIs | bool | `false` |  |
| mcpKubernetes.oauth.redirectURISecurity.disableDNSValidation | bool | `false` |  |
| mcpKubernetes.oauth.redirectURISecurity.disableDNSValidationStrict | bool | `false` |  |
| mcpKubernetes.oauth.redirectURISecurity.disableAuthorizationTimeValidation | bool | `false` |  |
| mcpKubernetes.oauth.trustedPublicRegistrationSchemes | list | `[]` |  |
| mcpKubernetes.oauth.disableStrictSchemeMatching | bool | `false` |  |
| mcpKubernetes.oauth.enableCIMD | bool | `true` |  |
| mcpKubernetes.oauth.cimd.allowPrivateIPs | bool | `false` |  |
| mcpKubernetes.oauth.trustedAudiences | list | `[]` |  |
| mcpKubernetes.oauth.trustedIssuers | list | `[]` |  |
| mcpKubernetes.oauth.sso.allowPrivateIPs | bool | `false` |  |
| mcpKubernetes.instrumentation.enabled | bool | `true` |  |
| mcpKubernetes.instrumentation.metricsExporter | string | `"prometheus"` |  |
| mcpKubernetes.instrumentation.tracingExporter | string | `"none"` |  |
| mcpKubernetes.instrumentation.otlpEndpoint | string | `""` |  |
| mcpKubernetes.instrumentation.otlpInsecure | bool | `false` |  |
| mcpKubernetes.instrumentation.traceSamplingRate | float | `0.1` |  |
| mcpKubernetes.instrumentation.detailedLabels | bool | `false` |  |
| mcpKubernetes.instrumentation.serviceMonitor.enabled | bool | `false` |  |
| mcpKubernetes.instrumentation.serviceMonitor.labels | object | `{}` |  |
| mcpKubernetes.instrumentation.serviceMonitor.annotations | object | `{}` |  |
| mcpKubernetes.instrumentation.serviceMonitor.interval | string | `""` |  |
| mcpKubernetes.instrumentation.serviceMonitor.scrapeTimeout | string | `""` |  |
| mcpKubernetes.instrumentation.serviceMonitor.relabelings | list | `[]` |  |
| mcpKubernetes.instrumentation.serviceMonitor.metricRelabelings | list | `[]` |  |
| mcpKubernetes.metrics.enabled | bool | `true` |  |
| mcpKubernetes.metrics.port | int | `9090` |  |
| mcpKubernetes.service.port | int | `8080` |  |
| mcpKubernetes.env | list | `[]` |  |
| capiMode | object | `{"cache":{"cleanupInterval":"1m","maxEntries":1000,"ttl":"10m"},"connectivity":{"burst":100,"qps":50,"requestTimeout":"30s","retryAttempts":3,"retryBackoff":"1s","timeout":"5s"},"enabled":false,"output":{"maskSecrets":true,"maxClusters":20,"maxItems":100,"maxResponseBytes":524288,"slimOutput":true},"privilegedAccess":{"enabled":true,"privilegedCAPIDiscovery":true,"rateLimit":{"burst":20,"perSecond":10},"strict":false},"rbac":{"allowedNamespaces":[],"clusterWideSecrets":false,"create":true},"workloadClusterAuth":{"caConfigMapSuffix":"-ca-public","disableCaching":false,"groupMappings":{},"mode":"impersonation"}}` | ------------------------------------ The RBAC resources created by this chart (ClusterRoles, Roles, RoleBindings) are ONLY used when OAuth Downstream is DISABLED (enableDownstreamOAuth: false).  When OAuth Downstream is ENABLED (enableDownstreamOAuth: true):   - The ServiceAccount RBAC is NOT used for Kubernetes API operations   - Users must have their own RBAC permissions on the Management Cluster to:     * List CAPI Cluster resources (cluster.x-k8s.io/clusters)     * Read kubeconfig secrets in organization namespaces   - On Workload Clusters, user identity is propagated via impersonation headers   - Users need appropriate RBAC on each Workload Cluster they access  See docs/rbac-security.md for detailed RBAC requirements per deployment mode. |
| grafanaDashboards.enabled | bool | `false` |  |
| grafanaDashboards.namespace | string | `""` |  |
| grafanaDashboards.labels.grafana_dashboard | string | `"1"` |  |
| grafanaDashboards.annotations | object | `{}` |  |
| grafanaDashboards.folder | string | `"mcp-kubernetes"` |  |
| grafanaDashboards.giantswarm.enabled | bool | `false` |  |
| grafanaDashboards.giantswarm.organization | string | `""` |  |
| grafanaDashboards.dashboards.administrator.enabled | bool | `true` |  |
| grafanaDashboards.dashboards.security.enabled | bool | `true` |  |
| grafanaDashboards.dashboards.clusterOperator.enabled | bool | `true` |  |
| prometheusRules.enabled | bool | `false` |  |
| prometheusRules.labels."observability.giantswarm.io/tenant" | string | `"giantswarm"` |  |
| prometheusRules.annotations | object | `{}` |  |
| prometheusRules.team | string | `"bumblebee"` |  |
| prometheusRules.runbookBaseUrl | string | `"https://github.com/giantswarm/mcp-kubernetes/blob/main/docs/runbooks"` |  |
| prometheusRules.rules.mcpKubernetesHighErrorRate.enabled | bool | `true` |  |
| prometheusRules.rules.mcpKubernetesK8sOperationFailures.enabled | bool | `true` |  |
| prometheusRules.rules.mcpKubernetesOAuthFailures.enabled | bool | `true` |  |
| prometheusRules.rules.mcpKubernetesWorkloadClusterAuthFailures.enabled | bool | `true` |  |
| prometheusRules.rules.mcpKubernetesClusterOperationFailures.enabled | bool | `true` |  |
| ciliumNetworkPolicy.enabled | bool | `true` |  |
| ciliumNetworkPolicy.labels | object | `{}` |  |
| ciliumNetworkPolicy.annotations | object | `{}` |  |
