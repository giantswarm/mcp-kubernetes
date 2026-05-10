# Read-tool argument reference

The four read-only tools (`kubernetes_list`, `kubernetes_get`, `kubernetes_describe`,
`kubernetes_logs`) share most of their argument shape so workflow authors and
LLM agents can move between them without having to relearn the schema.

This page is the authoritative comparison: it lists every argument each tool
accepts, what it does, and when it is required vs optional.

## Cluster / context arguments

These are accepted by all four tools and (depending on server mode) are used to
target a specific cluster or kube-context.

| Argument      | `_list` | `_get` | `_describe` | `_logs` | Notes                                                      |
|---------------|:-------:|:------:|:-----------:|:-------:|------------------------------------------------------------|
| `cluster`     |   yes   |  yes   |     yes     |   yes   | Only present in federated mode.                            |
| `kubeContext` |   yes   |  yes   |     yes     |   yes   | Only present in single-cluster mode.                       |

## Resource selection

| Argument        | `_list`   | `_get`    | `_describe` | `_logs`   | Notes                                                                                       |
|-----------------|:---------:|:---------:|:-----------:|:---------:|---------------------------------------------------------------------------------------------|
| `namespace`     | optional  | optional  |  optional   | required  | Defaults to `default` for namespaced resources; ignored for cluster-scoped.                 |
| `resourceType`  | required  | required  |  required   |    -      | e.g. `pod`, `service`, `deployment`, `clusters`.                                            |
| `apiGroup`      | optional  | optional  |  optional   |    -      | e.g. `apps`, `networking.k8s.io`, or `apps/v1`.                                             |
| `name`          |    -      | required  |  required   |    -      | Name of the single resource to fetch.                                                       |
| `podName`       |    -      |    -      |     -       | required  | Name of the pod to read logs from.                                                          |
| `containerName` |    -      |    -      |     -       | optional  | Required for multi-container pods.                                                          |
| `allNamespaces` | optional  |    -      |     -       |    -      | List namespaced resources across all namespaces.                                            |
| `labelSelector` | optional  |    -      |     -       |    -      | Server-side label selector (`app=nginx,env=prod`).                                          |
| `fieldSelector` | optional  |    -      |     -       |    -      | Server-side field selector (limited fields).                                                |
| `filter`        | optional  |    -      |     -       |    -      | Client-side filter for advanced cases. See [client-side-filtering.md](client-side-filtering.md). |

## Output shaping

| Argument             | `_list`  | `_get`   | `_describe` | `_logs`            | Notes                                                                                                                         |
|----------------------|:--------:|:--------:|:-----------:|:------------------:|-------------------------------------------------------------------------------------------------------------------------------|
| `output`             | optional | optional |  optional   | optional (no-op)   | Enum `slim` (default) / `normal` / `wide` / `full`. `slim` applies blacklist exclusion + Kind-aware shaping; `normal` is blacklist-only (no Kind shaping); `wide` / `full` return the full manifest. On `_logs` it is accepted but currently a no-op. |
| `fullOutput`         | optional |    -     |     -       |        -           | Return full resource manifests instead of compact summary.                                                                    |
| `includeLabels`      | optional |    -     |     -       |        -           | Include labels in compact summary output.                                                                                     |
| `includeAnnotations` | optional |    -     |     -       |        -           | Include annotations in compact summary output.                                                                                |
| `summary`            | optional |    -     |     -       |        -           | Return aggregated counts (by status, namespace) instead of full objects.                                                      |
| `eventsLimit`        |    -     |    -     |  optional   |        -           | Maximum events to include in the describe response (default 50, range 1–1000).                                                |
| `tailLines`          |    -     |    -     |     -       |     optional       | Return the last N lines of log (default 100, max 1000).                                                                       |
| `sinceTime`          |    -     |    -     |     -       |     optional       | RFC3339 timestamp; only return log lines after this time.                                                                     |
| `follow`             |    -     |    -     |     -       |     optional       | Follow log output. Default `false`.                                                                                           |
| `previous`           |    -     |    -     |     -       |     optional       | Return logs from previous container instance.                                                                                 |
| `timestamps`         |    -     |    -     |     -       |     optional       | Prefix each log line with its timestamp.                                                                                      |

## Pagination

| Argument        | `_list`  | `_get` | `_describe` | `_logs` | Notes                                                                  |
|-----------------|:--------:|:------:|:-----------:|:-------:|------------------------------------------------------------------------|
| `limit`         | optional |   -    |     -       |    -    | Maximum number of items per page (default 20, max 1000).               |
| `continue`      | optional |   -    |     -       |    -    | Continue token from a previous paginated response.                     |

## `output` semantics

The `output` argument is intentionally accepted by all four read tools so
callers can use the same key consistently:

- `slim` (default for `_list`; explicit on `_get` / `_describe` / `_logs`):
  apply the server-configured slim processor (generic blacklist exclusion of
  fields such as `metadata.managedFields`,
  `metadata.annotations["kubectl.kubernetes.io/last-applied-configuration"]`,
  status transition timestamps, owner references, `status.images` on Nodes,
  Helm release-coordinate annotations, …) **plus Kind-aware shaping**:
  HelmRelease drops `spec.values` / `status.history` / digest fields,
  Deployment / StatefulSet / DaemonSet collapse long container `env` lists
  to an `envCount` integer and prune well-known probe defaults. The full
  default list and the methodology used to tune it against a real
  installation are in [slim-output-tuning.md](slim-output-tuning.md).
- `normal`: blacklist-only behaviour. The same generic field exclusion as
  `slim`, but **Kind-aware shaping is disabled** so callers can still see a
  typed `env` list, the rendered HelmRelease values map, and other fields
  that `slim` collapses or drops per Kind.
- `wide` (alias: `full`): bypass slim processing entirely and return the
  full manifest. Secret data is still masked.

For `kubernetes_logs` the parameter is currently a no-op (log output is plain
text and not affected by manifest field stripping). Use `tailLines` and
`sinceTime` to shape log volume.

Secret masking is independent of `output` and is always applied when the
server is configured with `MaskSecrets=true` (the default).
