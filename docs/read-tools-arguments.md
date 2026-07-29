# Read-tool argument reference

The four read-only tools (`list`, `get`, `describe`, `logs`) share most of
their argument shape so workflow authors and LLM agents can move between them
without having to relearn the schema.

This page is the authoritative comparison: it lists every argument each tool
accepts, what it does, and when it is required vs optional.

## Cluster / context arguments

These are accepted by all four tools and (depending on server mode) are used to
target a specific cluster or kube-context.

| Argument      | `list` | `get` | `describe` | `logs` | Notes                                                      |
|---------------|:-------:|:------:|:-----------:|:-------:|------------------------------------------------------------|
| `cluster`     |   yes   |  yes   |     yes     |   yes   | Only present in federated mode.                            |
| `kubeContext` |   yes   |  yes   |     yes     |   yes   | Only present in single-cluster mode.                       |

## Resource selection

| Argument        | `list`   | `get`    | `describe` | `logs`   | Notes                                                                                       |
|-----------------|:---------:|:---------:|:-----------:|:---------:|---------------------------------------------------------------------------------------------|
| `namespace`     | optional  | optional  |  optional   | required  | Defaults to `default` for namespaced resources; ignored for cluster-scoped.                 |
| `resourceType`  | required  | required  |  required   |    -      | e.g. `pod`, `service`, `deployment`, `clusters`.                                            |
| `apiGroup`      | optional  | optional  |  optional   |    -      | e.g. `apps`, `networking.k8s.io`, or `apps/v1`.                                             |
| `name`          |    -      | required  |  required   |    -      | Name of the single resource to fetch.                                                       |
| `podName`       |    -      |    -      |     -       | required  | Name of the pod to read logs from.                                                          |
| `containerName` |    -      |    -      |     -       | optional  | Required for multi-container pods.                                                          |
| `allNamespaces` | optional  |    -      |     -       |    -      | List namespaced resources across all namespaces.                                            |
| `labelSelector` | optional  |    -      |     -       |    -      | Server-side label selector (`app=nginx,env=prod`).                                          |
| `fieldSelector` | optional  |    -      |     -       |    -      | Server-side field selector; selectable fields vary per Kind, and no Kind exposes a timestamp. See [selectable fields](#server-side-field-selectors). |
| `filter`        | optional  |    -      |     -       |    -      | Client-side filter for advanced cases. See [client-side-filtering.md](client-side-filtering.md). |

## Output shaping

| Argument             | `list`  | `get`   | `describe` | `logs`            | Notes                                                                                                                         |
|----------------------|:--------:|:--------:|:-----------:|:------------------:|-------------------------------------------------------------------------------------------------------------------------------|
| `output`             | optional | optional |  optional   | optional (no-op)   | Enum `slim` (default) / `normal` / `wide` / `full`. `slim` applies blacklist exclusion + Kind-aware shaping; `normal` is blacklist-only (no Kind shaping); `wide` / `full` return the full manifest. On `logs` it is accepted but currently a no-op. |
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

| Argument        | `list`  | `get` | `describe` | `logs` | Notes                                                                  |
|-----------------|:--------:|:------:|:-----------:|:-------:|------------------------------------------------------------------------|
| `limit`         | optional |   -    |     -       |    -    | Maximum number of items per page (default 20, max 1000).               |
| `continue`      | optional |   -    |     -       |    -    | Continue token from a previous paginated response.                     |

## Result ordering

`list` applies one Kind-specific ordering rule, and otherwise preserves the API
server's own order. This matters because `limit` keeps a **prefix** of the
result, so ordering decides which items a bounded call actually returns.

- **Events** are sorted **newest-first**, by `lastTimestamp`, then `eventTime`,
  then `firstTimestamp` — the same precedence reported as `lastSeen` in compact
  output, and the same ordering `describe` applies to its embedded events. So
  `{"resourceType": "events", "limit": 15}` returns the 15 most recent events.
  Events with no parseable timestamp sort last.
- **Every other Kind** is returned in the API server's order, which is
  namespace/name ascending. A `limit` therefore yields an alphabetical prefix —
  *not* the newest, largest or unhealthiest items. To bound a result set
  meaningfully, narrow it with `labelSelector` / `fieldSelector` / `filter`
  rather than relying on `limit`.

Note that client-side `filter` is applied **after** the server-side `limit`, so
a filtered call needs a `limit` large enough to cover the candidate set before
filtering (and then pages via `continue`).

## Server-side field selectors

Only fields the API server indexes are selectable, and the set is per-Kind.
Passing an unindexed field makes the API server reject the request.

| Kind    | Selectable fields                                                                                                                                    |
|---------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| Pods    | `metadata.name`, `metadata.namespace`, `spec.nodeName`, `status.phase`                                                                               |
| Events  | `metadata.name`, `metadata.namespace`, `reason`, `type`, `involvedObject.kind`, `involvedObject.name`, `involvedObject.namespace`, `involvedObject.fieldPath` |
| Others  | `metadata.name`, `metadata.namespace`                                                                                                                |

**No Kind exposes a timestamp field**, so "events from the last 10 minutes"
cannot be expressed server-side, and `filter` is equality-only so it cannot
express it either. Rely on the newest-first event ordering above and compare
`lastSeen` on the returned items.

## `output` semantics

The `output` argument is intentionally accepted by all four read tools so
callers can use the same key consistently:

- `slim` (default for `list`; explicit on `get` / `describe` / `logs`):
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

For `logs` the parameter is currently a no-op (log output is plain
text and not affected by manifest field stripping). Use `tailLines` and
`sinceTime` to shape log volume.

Secret masking is independent of `output` and is always applied when the
server is configured with `MaskSecrets=true` (the default).
