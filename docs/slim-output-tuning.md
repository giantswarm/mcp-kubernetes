# Slim-output tuning notes

This page records the methodology behind the default `excludedFields` list
used by `output: slim` (the default) on `kubernetes_get`, `kubernetes_list`,
and `kubernetes_describe`.

## Methodology

The default list was tuned in three rounds against a real Kubernetes cluster
using a build-tagged Go test harness (`internal/tools/resource/sizebench_test.go`
and `internal/tools/pod/sizebench_test.go`, build tag `clusterbench`). Each
round:

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

While round-3 tuning, the bench surfaced a long-standing bug in
`internal/tools/output/slim.go:removeField`. The function split paths on `.`
and treated each segment as a separate map key. Kubernetes label and
annotation keys very commonly contain dots
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

## Final measured pattern

`output: wide` is the unprocessed payload; `output: slim` is the default.
Across the five tuned workload shapes (deployment, statefulset, daemonset,
node, secret), measured slim-vs-wide reductions ranged from roughly 40% to
over 80%, with the largest reductions on Nodes (the `status.images` strip
dominates) and on `kubernetes_describe` of busy controllers (per-event slim
is the dominant win there). Secret data stays masked across all three
formats.

## Secret-masking sanity

The bench specifically pinned that `secret.data.*` is replaced with
`***REDACTED***` for all three values of `output` on both `kubernetes_get`
and `kubernetes_describe`. Secret masking is independent of `output` and
follows the server-level `MaskSecrets` config (default `true`).
