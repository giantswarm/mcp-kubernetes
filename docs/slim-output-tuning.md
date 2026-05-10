# Slim-output tuning notes

This page records the methodology behind the default `excludedFields` list
used by `output: slim` (the default) on `kubernetes_get`, `kubernetes_list`,
and `kubernetes_describe`.

## Methodology

The default list was tuned in **five iterative live-cluster bench rounds**
against a real Kubernetes cluster using a build-tagged Go test harness
(`internal/tools/resource/sizebench_test.go` and
`internal/tools/pod/sizebench_test.go`, build tag `clusterbench`). The
inline `// round-N` comments in `internal/tools/output/config.go`,
`internal/tools/output/shape.go`, and
`internal/tools/resource/handlers.go` reference the same numbering.
Each round:

1. drove `kubernetes_get` and `kubernetes_describe` against five
   representative workloads — a deployment, a statefulset, a daemonset, a
   node, and a Secret — across all three values of `output` (`slim`,
   `normal`, `wide`);
2. drove `kubernetes_logs` against five running pods with three `tailLines`
   values across the same three `output` values to confirm `output` is a
   no-op for log content;
3. measured the JSON byte size of the response that would have been returned
   to the MCP caller;
4. dumped the raw responses with `CLUSTERBENCH_DUMP_DIR=...` and inspected
   the largest fields with `jq` to find the next stripping target.

The harness lives in the tree and can be re-run against any cluster:

```bash
KUBECONFIG=$HOME/.kube/config \
  CLUSTERBENCH_CONTEXT=<your-kube-context> \
  CLUSTERBENCH_DUMP_DIR=/tmp/sizebench/dumps \
  CLUSTERBENCH_REPORT=/tmp/sizebench/report.md \
  go test -tags=clusterbench -run TestSizeBench_Resource \
    -v -count=1 -timeout=5m ./internal/tools/resource/...
```

The five workloads it exercises (deployment, statefulset, daemonset, node,
secret) are configurable via constants in the test file — pick names that
exist in the target cluster before running.

## Final defaults and why

The default excluded paths are defined in
`internal/tools/output/config.go:DefaultExcludedFields`. Each entry below
includes the rationale and the rough impact pattern observed in tuning.

| Path | Rationale | Typical saving |
|---|---|---:|
| `metadata.managedFields` | Dominates response size on resources touched by many controllers; not useful for diagnosis. | up to several KB / resource |
| `metadata.annotations.kubectl.kubernetes.io/last-applied-configuration` | Duplicates the entire manifest. | depends on resource size |
| `metadata.annotations.deployment.kubernetes.io/revision` | Internal bookkeeping. Now actually stripped after the dot-key fix. | small |
| `status.conditions[*].lastTransitionTime` / `lastProbeTime` / `lastHeartbeatTime` / `lastUpdateTime` | Timestamp noise that does not help an LLM agent reason about the *current* condition state. `lastUpdateTime` is the deployment-status equivalent of the others. | ~80 B / condition |
| `metadata.ownerReferences` | Discoverable on demand; rarely needed for routine inspection. | small |
| `metadata.finalizers` | Rarely relevant for troubleshooting. | small |
| `metadata.generation`, `metadata.resourceVersion`, `metadata.uid`, `metadata.selfLink` | Internal bookkeeping; the LLM agent treats these as opaque. | ~100 B / resource combined |
| `spec.template.metadata.creationTimestamp` | Always `null` on PodSpec templates. | ~40 B / pod template |
| `status.images` | List of every container image cached on a Node. **Single biggest win**: this list is typically the dominant component of a Node response. | several KB / node |

## Per-event slim list (kubernetes_describe)

`buildDescribeOutput` additionally strips the following from each event in
the response (`internal/tools/resource/handlers.go:eventSlimFields`). On a
controller with many events this is worth several KB:

- `metadata.managedFields`
- `metadata.uid`, `metadata.resourceVersion`, `metadata.creationTimestamp`,
  `metadata.namespace`, `metadata.selfLink`
- `involvedObject.uid`, `involvedObject.resourceVersion`,
  `involvedObject.apiVersion`
- `reportingInstance` (almost always empty)

`eventTime` is **intentionally preserved** even though it is `null` on
classic core/v1 events. Newer event reporters (kyverno-scan,
controller-runtime) populate `eventTime` instead of
`firstTimestamp`/`lastTimestamp`, so removing it would leave those events
without any timestamp at all. The 30-byte cost per event is worth it.

## Convenience metadata slim (kubernetes_describe)

`kubernetes_describe` returns a convenience top-level `metadata` map that
duplicates `resource.metadata.{labels, annotations, uid, resourceVersion,
creationTimestamp, kind, apiVersion}` for callers that want a quick lookup
without parsing the whole resource. The describe handler now wraps the
convenience map in a `{"metadata": ...}` envelope before applying the slim
processor, so the same paths that get stripped from `resource.metadata` are
also stripped from the duplicate. Without this the duplicate map would
silently re-introduce the same uid / resourceVersion / managedFields bytes
we just stripped.

## Bug found and fixed: dotted annotation/label keys

While tuning built-in resource shapes (round 3), the bench surfaced a
long-standing bug in `internal/tools/output/slim.go:removeField`. The
function split paths on `.` and treated each segment as a separate map key.
Kubernetes label and annotation keys very commonly contain dots
(`kubectl.kubernetes.io/last-applied-configuration`,
`deployment.kubernetes.io/revision`, `app.kubernetes.io/name`, …), so paths
like `metadata.annotations.kubectl.kubernetes.io/last-applied-configuration`
silently no-op'd: the function looked for
`metadata → annotations → kubectl → kubernetes → io/last-applied-configuration`
and gave up at `kubectl`.

The fix is **greedy suffix matching**: at each navigation step the function
now tries the longest joined suffix that exists as a literal key in the
current map before falling back to single-segment lookup. Greedy joins do
not cross array-wildcard boundaries (`[*]`). Pinned by
`TestRemoveField_DotsInKeys` in `internal/tools/output/slim_test.go`.

## Bug found and fixed: typed `map[string]string` annotations / labels

A second silent failure was surfaced by a follow-up bench round against
custom resources (Cluster API objects, Flux `HelmRelease` /
`Kustomization`, cert-manager `Certificate`, Prometheus `ServiceMonitor`,
`PolicyReport`, etc.). The describe handler exposes a convenience top-level
`metadata` map populated from `unstructured.GetAnnotations` and
`unstructured.GetLabels`, both of which return typed `map[string]string` —
not `map[string]interface{}`. The `deepCopyValue` and `removeFieldRecursive`
helpers only knew how to descend into `map[string]interface{}`, so any
nested removal (including the dotted-key paths fixed above) silently no-op'd
on the convenience metadata even though it worked on the resource itself.
The result was that `last-applied-configuration` was correctly stripped from
`resource.metadata.annotations` but silently re-leaked through
`metadata.annotations` next to it.

The fix is to normalise `map[string]string` into `map[string]interface{}`
in `deepCopyValue`. JSON marshaling is identical for both shapes, so callers
see no downstream difference; the recursive remover now has a single map
type to navigate. Pinned by `TestSlimResource_TypedStringMaps` in
`internal/tools/output/slim_test.go`. Measured impact on the
custom-resource describe bench is 3–4 KB reclaimed per response on
resources that have a `last-applied-configuration` annotation.

## CR-shaped bench (second pass)

After the typed-string-map fix, a 10-resource bench was run across the
following shapes to validate that the `slim` defaults still behave well on
custom resources rather than just built-in core/v1 / apps/v1 types:

- `cluster.x-k8s.io/Cluster` (Cluster API top-level)
- `infrastructure.cluster.x-k8s.io/AWSCluster` (large nested `spec.network`)
- `infrastructure.cluster.x-k8s.io/AWSMachineTemplate` (heavy spec, almost no status, frequent events)
- `application.giantswarm.io/App` (custom controller, conditions in status)
- `application.giantswarm.io/AppCatalogEntry` (annotation-heavy)
- `helm.toolkit.fluxcd.io/HelmRelease` (Flux, history in status)
- `kustomize.toolkit.fluxcd.io/Kustomization` (large `status.inventory`)
- `cert-manager.io/Certificate` (conditions + secret reference)
- `monitoring.coreos.com/ServiceMonitor` (small, list-heavy spec)
- `wgpolicyk8s.io/PolicyReport` (large `results` array — fundamentally diagnostic)

Slim-vs-wide reductions across this set ranged 7% (PolicyReport, almost
entirely diagnostic content) up to 63% (App). No CR-specific paths were
added to `DefaultExcludedFields`: every additional path considered
(per-result `properties` maps, per-result `timestamp.{seconds,nanos}`,
provider tags on `AWSCluster.spec.network.subnets[*].tags`, artifact-hub
annotations on `AppCatalogEntry`, etc.) was rejected as either too narrow,
too risky to apply globally, or actually diagnostic content the agent
needs.

## Final measured pattern

`output: wide` is the unprocessed payload; `output: slim` is the default.
Across the five tuned workload shapes (deployment, statefulset, daemonset,
node, secret), measured slim-vs-wide reductions ranged from roughly 40% to
over 80%, with the largest reductions on Nodes (the `status.images` strip
dominates) and on `kubernetes_describe` of busy controllers (per-event slim
is the dominant win there). Secret data stays masked across all three
formats.

## Per-Kind shaping (issue #410)

In addition to the path-based blacklist above, `output: slim` runs a
**Kind-aware shaper** on the resource (off by default in `output: normal`).
This handles wasteful patterns the blacklist cannot express:

- `helm.toolkit.fluxcd.io/HelmRelease` — drop `spec.values`, `status.history`,
  `status.lastAppliedConfigDigest`, `status.lastAttemptedConfigDigest`,
  `status.lastAttemptedRevisionDigest`, `status.observedPostRenderersDigest`.
  Conditions, `status.lastAttemptedRevision`, chart references, and
  `spec.valuesFrom` are preserved.
- `apps/Deployment | StatefulSet | DaemonSet`:
  - `spec.progressDeadlineSeconds` and `spec.revisionHistoryLimit` —
    rollout-controller defaults.
  - `spec.template.spec.restartPolicy` and
    `spec.template.spec.terminationGracePeriodSeconds` — PodSpec defaults.
  - For each container: `env` collapses to an `envCount` integer when its
    length exceeds 8 entries (issue #410's literal request).
  - For each container: well-known **probe defaults** are pruned from
    `livenessProbe` / `readinessProbe` / `startupProbe` (`failureThreshold:3`,
    `successThreshold:1`, `periodSeconds:10`, `timeoutSeconds:1`,
    `initialDelaySeconds:0`). The probe action (httpGet / tcpSocket /
    exec / grpc) and any non-default thresholds are preserved.

The shaping was tuned across **five iterative live-cluster bench rounds** against
a sample set of three Flux HelmReleases (backstage, muster, mcp-kubernetes)
and four Deployments (backstage, mcp-kubernetes, karpenter, grafana). Each
round dumped the slim payloads, ranked the largest residual subtrees with
`jq`, picked the most-wasteful field with no diagnostic value, and
re-measured. The cumulative reduction on the largest workload sample
(a Deployment with 39 env entries) was **wide → slim 77.3%**
(31.6 KB → 7.2 KB). Smaller workloads landed in the 60-70% band.

Anything customised with non-default values (e.g. an explicit
`timeoutSeconds: 5`, a non-default `restartPolicy`, a long but
under-threshold env list) is preserved verbatim. Callers that need the
full shape can always pass `output: wide` (or `output: full`) — secret
masking still applies.

## Helm/Flux bookkeeping annotations

`metadata.annotations.meta.helm.sh/release-name` and
`metadata.annotations.meta.helm.sh/release-namespace` are stripped on
every Helm-managed resource (~85 B per response). They duplicate
`metadata.namespace` and the `helm.sh/chart` label and are pure tool
bookkeeping, so they're stripped under both `slim` and `normal`.

The bench runs through `internal/tools/resource/sizebench_test.go` (build
tag `clusterbench`) and is parameterised by environment variables
(`CLUSTERBENCH_CONTEXT`, `CLUSTERBENCH_WORKLOADS`, `CLUSTERBENCH_REPORT`,
`CLUSTERBENCH_DUMP_DIR`), so it can be re-run against any cluster the
caller has access to without baking specific installations into the
repository.

## Secret-masking sanity

The bench specifically pinned that `secret.data.*` is replaced with
`***REDACTED***` for all three values of `output` on both `kubernetes_get`
and `kubernetes_describe`. Secret masking is independent of `output` and
follows the server-level `MaskSecrets` config (default `true`).
