# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Enable JSON-Schema input validation for every tool (per [SEP-1303](https://modelcontextprotocol.io/seps/1303-input-validation-errors-as-tool-execution-errors)). Calls with unknown property names, wrong types, or missing required fields now return a structured tool execution error instead of being silently dropped — for example, sending `cursor` instead of `continue` to `list` is rejected with a message the model can self-correct from ([#36458](https://github.com/giantswarm/giantswarm/issues/36458)). The `port_forward` tool retains a `podName` parameter as a deprecated alias for `resourceName`.
- `get`, `describe`, and `logs` now accept the same `output` argument (`slim` / `normal` / `wide`) as `list`, so workflow authors and LLM agents can use a single, symmetric argument shape across all four read tools. On `get` and `describe`, `output: wide` returns the full manifest (skipping slim-output field stripping); on `logs` it is accepted as a no-op for symmetry. Calls that previously failed with the opaque `input schema validation failed: <root>: &{[output]}` error now succeed. New `docs/read-tools-arguments.md` documents the full argument matrix for the four read tools ([#409](https://github.com/giantswarm/mcp-kubernetes/issues/409)).

### Changed




- Bump `giantswarm/architect` orb to `8.2.2` and re-enable cosign keyless chart signing (`sign: false` removed from every `push-to-app-catalog*` invocation). v8.2.2 ships [architect-orb#772](https://github.com/giantswarm/architect-orb/pull/772) which upgrades the `app-build-suite` executor image from `1.8.0-circleci` to `1.8.1-circleci` -- the new image includes the `cosign` binary that v8.2.0's chart signing defaults require. Closes [architect-orb#769](https://github.com/giantswarm/architect-orb/issues/769).
- Disable cosign keyless chart signing on the `push-to-app-catalog*` jobs (`sign: false`). The architect orb's `push-to-app-catalog` defaults `sign` to `true` since v8.2.0 and shells out to `cosign`, but this repo uses `executor: app-build-suite` (so the `app_build_suite` Python CLI is available to package the chart with metadata) and the `app-build-suite` image doesn't ship `cosign`. Without this opt-out, every chart push fails on the `Mint Sigstore OIDC token` step with `cosign: command not found`. To be removed once architect-orb makes `cosign-prepare` resilient to a missing binary (or ships cosign in the `app-build-suite` executor).
- Bump `giantswarm/architect` orb to `8.2.1` to pick up [architect-orb#767](https://github.com/giantswarm/architect-orb/pull/767): `image-login-to-registries` is now POSIX-portable, unblocking `architect/sync-china-registry` (the gsoci -> Aliyun mirror via the in-China `giantswarm/galaxy-runner`). The v8.1.0 refactor accidentally introduced bash-only `${!var}` indirect expansion in the shared login command, which BusyBox `/bin/sh` (used by the regctl executor) rejected with `bad substitution` -- so no Aliyun mirror has been happening since the migration to `split-china-push: true`. v8.2.x also enables cosign keyless signing, SLSA provenance, and SBOM attestations by default for public images and charts.
- Renamed 15 MCP tools to drop the `kubernetes_` prefix: `get`, `list`, `describe`, `create`, `apply`, `delete`, `patch`, `scale`, `logs`, `exec`, `api_resources`, `cluster_health`, `context_list`, `context_get_current`, `context_use`. They now match the existing `port_forward` family. The previous `kubernetes_<name>` names remain fully invokable as deprecated aliases so existing clients keep working without changes, but they are hidden from `tools/list` so new clients can't discover the deprecated names — the aliases act as a silent backward-compat shim. Audit logs continue to record the alias name actually invoked, so residual usage stays observable. The aliases will be removed in a future release; please migrate at your convenience. Prometheus metric names (`mcp_kubernetes_*`, `kubernetes_pod_operations_total`, `kubernetes_pod_operation_duration_seconds`) are unaffected.
- Replace the `push-to-gsoci-release` + `push-to-all-registries-release` workaround pair with a single `push-to-registries-release` job using `split-china-push: true` and a companion `sync-china-registry` job. The cross-Pacific `docker buildx` push to the Aliyun mirror is gone; the in-China `giantswarm/galaxy-runner` self-hosted CircleCI runner does `regctl image copy` (gsoci -> Aliyun) via the Singapore geo-replica. Chart catalog publish still does not gate on Aliyun.
- Bump `giantswarm/architect` orb to `8.1.0` and migrate all three image pushes from the deprecated `push-to-registries-multiarch` job to `push-to-registries` with `multiarch: true`. Picks up the v8.1.0 QEMU/binfmt auto-registration, hardened buildx bootstrap, and standard OCI image labels.
- Tuned the default `slim` field-exclusion list against a live cluster covering deployment, statefulset, daemonset, node, and Secret workloads (methodology in `docs/slim-output-tuning.md`). New defaults strip `status.images` on Nodes (the single largest unhelpful field on a typical node), `status.conditions[*].lastUpdateTime`, and `spec.template.metadata.creationTimestamp`. `describe` additionally slims per-event bookkeeping (`metadata.uid` / `resourceVersion` / `creationTimestamp` / `namespace`, `involvedObject.uid` / `resourceVersion` / `apiVersion`, empty `reportingInstance`) and the convenience top-level `metadata` map. Measured slim-vs-wide reductions across the five tuned workloads ranged 40–82%. `eventTime` is intentionally preserved so kyverno- and controller-runtime-style events keep their only timestamp.
- `output: slim` now applies **Kind-aware shaping** on top of the generic blacklist (controlled by the new `KindShaping` config flag, on by default; `output: normal` keeps the blacklist-only behaviour). HelmRelease drops `spec.values`, `status.history`, and the `lastApplied` / `lastAttempted` / `lastAttemptedRevision` / `observedPostRenderers` digests. Deployment / StatefulSet / DaemonSet collapse long container `env` lists (>8 entries) to an `envCount` integer, prune well-known probe defaults (`failureThreshold:3`, `successThreshold:1`, `periodSeconds:10`, `timeoutSeconds:1`, `initialDelaySeconds:0`) on `livenessProbe` / `readinessProbe` / `startupProbe`, and drop rollout-controller defaults (`spec.progressDeadlineSeconds`, `spec.revisionHistoryLimit`) and PodSpec defaults (`restartPolicy`, `terminationGracePeriodSeconds`). Tuned in five iterative live-cluster bench rounds; measured cumulative reduction on the largest sample (a Deployment with 39 env vars) is **wide → slim 77.3%** (31.6 KB → 7.2 KB). Resolves [#410](https://github.com/giantswarm/mcp-kubernetes/issues/410).
- `output: full` is now accepted as an alias for `output: wide` on every read tool. `output: normal` is now distinct from `output: slim` — it applies generic field exclusion (managedFields, last-applied-configuration, transition timestamps, the new PodSpec / Helm-bookkeeping strips) but skips Kind-aware shaping, so callers that want a typed `env` list or the full HelmRelease values map can ask for it.
- Default field-exclusion list grew to cover PodSpec defaults (`spec.template.spec.dnsPolicy`, `schedulerName`, deprecated `serviceAccount` alias), per-container `terminationMessagePath` / `terminationMessagePolicy`, and the Helm release-coordinate annotations `meta.helm.sh/release-name` / `release-namespace`. These were tuned in rounds 2 and 5 of the live-cluster bench and apply to both `slim` and `normal`.
- Mutating tools (`create`, `apply`, `delete`, `patch`, `scale`, `exec`, `port_forward` and the related session-management tools) are no longer registered with the MCP server when they cannot be invoked under the current configuration. This shrinks the tool list seen by clients in non-destructive mode and prevents models from attempting destructive operations that would always be rejected. Mutating tools become visible again when `--non-destructive=false`, when `--dry-run` is set, or when the operation verb is in `AllowedOperations`. Resolves [#4296](https://github.com/giantswarm/roadmap/issues/4296).

### Fixed

- `output.SlimResource` now correctly removes annotation and label keys that contain literal dots. Previously paths like `metadata.annotations.kubectl.kubernetes.io/last-applied-configuration` and `metadata.annotations.deployment.kubernetes.io/revision` were silently no-ops because the path resolver split on `.` without considering that Kubernetes label / annotation keys very commonly contain dots themselves. The resolver now uses greedy suffix matching at each navigation step (without crossing `[*]` array-wildcard boundaries), so the existing default excluded paths actually do what they claim to.
- `output.SlimResource` now also slims typed string-keyed maps (`map[string]string`) instead of bailing out of them. This was a second silent failure surfaced by benching `describe` against custom resources: the convenience top-level `metadata` map exposes annotations and labels as `map[string]string` (the type returned by `unstructured.GetAnnotations` / `GetLabels`), and the deep-copy / remove logic only knew how to descend into `map[string]interface{}`. As a result, dotted-key paths like `metadata.annotations.kubectl.kubernetes.io/last-applied-configuration` were stripped from the resource itself but silently re-leaked through the convenience metadata. Typed string maps are now normalised to `map[string]interface{}` on copy (JSON output is unchanged), and the same path rules apply to both. Measured impact on a CR-heavy describe is 3–4 KB reclaimed per response.
- `list` with `output: wide` now also runs results through secret masking and the `MaxItems` safety cap, matching the contract documented for `get` / `describe` and `docs/read-tools-arguments.md`. Previously `output: wide` on the list handler bypassed the output processor entirely, which silently disabled `MaskSecrets` for that one format. Field stripping is still skipped for `wide` — only masking and truncation are now uniform across every read tool.
- Event `message` truncation in `list` summary mode is now rune-aware. Previously the 240-character cap sliced on byte boundaries, which could cut multi-byte runes (kyverno-style validation errors commonly contain `→` and accented characters) and emit invalid UTF-8 in the JSON response.
- Kind-aware workload shaping (`output: slim` on `Deployment` / `StatefulSet` / `DaemonSet`) now also applies to `initContainers` and `ephemeralContainers`, not just the primary `containers` slice. Sidecar and debug containers no longer escape env-collapse and probe-default pruning.
- The `--non-destructive` and `--dry-run` CLI flags now correctly propagate to the server context. Previously, the values were only applied to the Kubernetes client layer, so the runtime safety check in `CheckMutatingOperation` always read the default values regardless of CLI input.

## [0.1.73] - 2026-05-05

### Changed

- Update module github.com/mark3labs/mcp-go to v0.52.0 ([#393](https://github.com/giantswarm/mcp-kubernetes/pull/393))

## [0.1.72] - 2026-05-04

### Changed

- Update module github.com/mark3labs/mcp-go to v0.51.0 ([#390](https://github.com/giantswarm/mcp-kubernetes/pull/390))

## [0.1.71] - 2026-05-02

### Changed

- Update module github.com/mark3labs/mcp-go to v0.50.0 ([#389](https://github.com/giantswarm/mcp-kubernetes/pull/389))

## [0.1.70] - 2026-04-30

### Changed

- `kubernetes_describe` now caps the embedded event list and returns events newest-first. New parameter `eventsLimit` (default 50, range 1–1000) controls the cap; the response gains `totalEvents`, `returnedEvents`, and `eventsTruncated` fields so callers can detect that the event history was clipped. `metadata.managedFields` is now stripped from each returned event ([#388](https://github.com/giantswarm/mcp-kubernetes/pull/388)).


## [0.1.69] - 2026-04-30

### Changed

- Limit kubernetes_cluster_health output ([#387](https://github.com/giantswarm/mcp-kubernetes/pull/387))

## [0.1.68] - 2026-04-29

### Changed

- Align files ([#386](https://github.com/giantswarm/mcp-kubernetes/pull/386))

## [0.1.67] - 2026-04-29

### Changed

- **Breaking:** `kubernetes_logs` parameter surface simplified. `tailLines` now defaults to `100`, is bounded to `[1, 1000]`, and is enforced server-side via `corev1.PodLogOptions.TailLines`, bounding both response size and gateway memory. The `sinceLines` and `maxLines` parameters have been removed; for time-based filtering, the new `sinceTime` parameter accepts an RFC3339 timestamp and is also applied server-side ([#383](https://github.com/giantswarm/mcp-kubernetes/pull/383)).


## [0.1.66] - 2026-04-29

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.117 ([#384](https://github.com/giantswarm/mcp-kubernetes/pull/384))

## [0.1.65] - 2026-04-28

### Changed

- chore(deps): update dependency architect to v7.1.0 ([#385](https://github.com/giantswarm/mcp-kubernetes/pull/385))

## [0.1.64] - 2026-04-27

### Changed

- Change owning team ([#382](https://github.com/giantswarm/mcp-kubernetes/pull/382))

## [0.1.63] - 2026-04-24

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.104 ([#377](https://github.com/giantswarm/mcp-kubernetes/pull/377))
- Align files ([#381](https://github.com/giantswarm/mcp-kubernetes/pull/381))

## [0.1.62] - 2026-04-24

### Changed

- chore(deps): update dependency architect to v7 ([#379](https://github.com/giantswarm/mcp-kubernetes/pull/379))

## [0.1.61] - 2026-04-24

### Changed

- Align files ([#380](https://github.com/giantswarm/mcp-kubernetes/pull/380))

## [0.1.60] - 2026-04-24

### Changed

- ci: finish pre-commit cleanup (golangci-lint gosec fixes) ([#378](https://github.com/giantswarm/mcp-kubernetes/pull/378))

## [0.1.59] - 2026-04-23

### Changed

- fix(deps): update k8s modules to v0.36.0 ([#374](https://github.com/giantswarm/mcp-kubernetes/pull/374))

## [0.1.58] - 2026-04-23

### Changed

- ci: fix pre-commit auto-formatter violations ([#376](https://github.com/giantswarm/mcp-kubernetes/pull/376))

## [0.1.57] - 2026-04-23

### Changed

- Align files ([#375](https://github.com/giantswarm/mcp-kubernetes/pull/375))

## [0.1.56] - 2026-04-21

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.49.0 ([#373](https://github.com/giantswarm/mcp-kubernetes/pull/373))

## [0.1.55] - 2026-04-21

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.102 ([#372](https://github.com/giantswarm/mcp-kubernetes/pull/372))

## [0.1.54] - 2026-04-21

### Added

- Added MCP tool annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) to all tools to help clients and users assess tool behavior ([#371](https://github.com/giantswarm/mcp-kubernetes/pull/371)).


## [0.1.53] - 2026-04-20

### Changed

- Align files ([#370](https://github.com/giantswarm/mcp-kubernetes/pull/370))

## [0.1.52] - 2026-04-20

### Fixed

- Fix helm-schema rename in pre-commit workflow ([#369](https://github.com/giantswarm/mcp-kubernetes/pull/369))

## [0.1.51] - 2026-04-17

### Changed

- Align files ([#368](https://github.com/giantswarm/mcp-kubernetes/pull/368))

## [0.1.50] - 2026-04-17

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.101 ([#367](https://github.com/giantswarm/mcp-kubernetes/pull/367))

## [0.1.49] - 2026-04-16

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.100 - autoclosed ([#366](https://github.com/giantswarm/mcp-kubernetes/pull/366))

## [0.1.48] - 2026-04-16

### Changed

- fix(deps): update k8s modules to v0.35.4 ([#365](https://github.com/giantswarm/mcp-kubernetes/pull/365))

## [0.1.47] - 2026-04-16

### Changed

- Align files ([#364](https://github.com/giantswarm/mcp-kubernetes/pull/364))

## [0.1.46] - 2026-04-15

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.99 ([#363](https://github.com/giantswarm/mcp-kubernetes/pull/363))

## [0.1.45] - 2026-04-15

### Changed

- Align files ([#362](https://github.com/giantswarm/mcp-kubernetes/pull/362))

## [0.1.44] - 2026-04-15

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.48.0 ([#361](https://github.com/giantswarm/mcp-kubernetes/pull/361))

## [0.1.43] - 2026-04-12

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.98 ([#360](https://github.com/giantswarm/mcp-kubernetes/pull/360))

## [0.1.42] - 2026-04-09

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.97 ([#359](https://github.com/giantswarm/mcp-kubernetes/pull/359))

## [0.1.41] - 2026-04-09

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.96 ([#357](https://github.com/giantswarm/mcp-kubernetes/pull/357))

## [0.1.40] - 2026-04-09

### Changed

- chore(deps): update dependency go to v1.26.2 ([#353](https://github.com/giantswarm/mcp-kubernetes/pull/353))
- fix(deps): update module golang.org/x/text to v0.36.0 ([#358](https://github.com/giantswarm/mcp-kubernetes/pull/358))

## [0.1.39] - 2026-04-09

### Changed

- chore(deps): update golang docker tag to v1.26.2 ([#354](https://github.com/giantswarm/mcp-kubernetes/pull/354))

## [0.1.38] - 2026-04-09

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.47.1 ([#355](https://github.com/giantswarm/mcp-kubernetes/pull/355))

## [0.1.37] - 2026-04-09

### Changed

- Align files ([#356](https://github.com/giantswarm/mcp-kubernetes/pull/356))

## [0.1.36] - 2026-04-06

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.47.0 ([#351](https://github.com/giantswarm/mcp-kubernetes/pull/351))
- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.94 ([#352](https://github.com/giantswarm/mcp-kubernetes/pull/352))

## [0.1.35] - 2026-04-05

### Changed

- fix(deps): update opentelemetry-go monorepo ([#350](https://github.com/giantswarm/mcp-kubernetes/pull/350))

## [0.1.34] - 2026-03-30

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.93 ([#349](https://github.com/giantswarm/mcp-kubernetes/pull/349))

## [0.1.33] - 2026-03-26

### Changed

- chore(deps): update dependency requests to ~=2.33.0 ([#347](https://github.com/giantswarm/mcp-kubernetes/pull/347))

## [0.1.32] - 2026-03-26

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.46.0 ([#348](https://github.com/giantswarm/mcp-kubernetes/pull/348))

## [0.1.31] - 2026-03-25

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.92 ([#345](https://github.com/giantswarm/mcp-kubernetes/pull/345))

## [0.1.30] - 2026-03-24

### Changed

- Upgrade mcp-oauth to v0.2.91 ([#346](https://github.com/giantswarm/mcp-kubernetes/pull/346))

## [0.1.29] - 2026-03-24

### Changed

- fix(deps): update k8s modules to v0.35.3 ([#343](https://github.com/giantswarm/mcp-kubernetes/pull/343))
- chore(deps): update azure/setup-helm action to v5 ([#344](https://github.com/giantswarm/mcp-kubernetes/pull/344))

## [0.1.28] - 2026-03-19

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.90 ([#342](https://github.com/giantswarm/mcp-kubernetes/pull/342))

## [0.1.27] - 2026-03-19

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.89 ([#341](https://github.com/giantswarm/mcp-kubernetes/pull/341))

## [0.1.26] - 2026-03-18

### Changed

- Disable BackendTrafficPolicy timeout for SSE connections ([#340](https://github.com/giantswarm/mcp-kubernetes/pull/340))

## [0.1.25] - 2026-03-18

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.88 ([#339](https://github.com/giantswarm/mcp-kubernetes/pull/339))

## [0.1.24] - 2026-03-17

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.87 ([#338](https://github.com/giantswarm/mcp-kubernetes/pull/338))

## [0.1.23] - 2026-03-17

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.85 ([#337](https://github.com/giantswarm/mcp-kubernetes/pull/337))

## [0.1.22] - 2026-03-17

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.83 ([#336](https://github.com/giantswarm/mcp-kubernetes/pull/336))

## [0.1.21] - 2026-03-16

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.82 ([#334](https://github.com/giantswarm/mcp-kubernetes/pull/334))

## [0.1.20] - 2026-03-13

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.81 ([#333](https://github.com/giantswarm/mcp-kubernetes/pull/333))

## [0.1.19] - 2026-03-12

### Changed

- chore(deps): update dependency architect to v6.15.0 ([#332](https://github.com/giantswarm/mcp-kubernetes/pull/332))

## [0.1.18] - 2026-03-12

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.80 ([#331](https://github.com/giantswarm/mcp-kubernetes/pull/331))

## [0.1.17] - 2026-03-11

### Changed

- fix(deps): update module golang.org/x/text to v0.35.0 ([#330](https://github.com/giantswarm/mcp-kubernetes/pull/330))

## [0.1.16] - 2026-03-10

### Fixed

- fix(federation): truncate excessive groups instead of rejecting ([#327](https://github.com/giantswarm/mcp-kubernetes/pull/327))

## [0.1.15] - 2026-03-10

### Changed

- Chore: auto-commit changes from agent session ([#328](https://github.com/giantswarm/mcp-kubernetes/pull/328))

## [0.1.14] - 2026-03-09

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.78 ([#324](https://github.com/giantswarm/mcp-kubernetes/pull/324))

## [0.1.13] - 2026-03-08

### Changed

- fix(deps): update module golang.org/x/time to v0.15.0 ([#323](https://github.com/giantswarm/mcp-kubernetes/pull/323))
- fix(deps): update module golang.org/x/oauth2 to v0.36.0 ([#320](https://github.com/giantswarm/mcp-kubernetes/pull/320))

## [0.1.12] - 2026-03-08

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.77 ([#322](https://github.com/giantswarm/mcp-kubernetes/pull/322))

## [0.1.11] - 2026-03-07

### Changed

- fix(deps): update opentelemetry-go monorepo ([#318](https://github.com/giantswarm/mcp-kubernetes/pull/318))
- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.76 ([#319](https://github.com/giantswarm/mcp-kubernetes/pull/319))

## [0.1.10] - 2026-03-06

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.45.0 ([#317](https://github.com/giantswarm/mcp-kubernetes/pull/317))

## [0.1.9] - 2026-03-06

### Fixed

- fix(dashboards): fix namespace variable and no-data handling in Grafana dashboards ([#316](https://github.com/giantswarm/mcp-kubernetes/pull/316))

## [0.1.8] - 2026-03-06

### Fixed

- fix(dashboards): remove broken links from Grafana dashboards ([#314](https://github.com/giantswarm/mcp-kubernetes/pull/314))

## [0.1.7] - 2026-03-06

### Changed

- chore(deps): update mcp-oauth to v0.2.75 ([#315](https://github.com/giantswarm/mcp-kubernetes/pull/315))

## [0.1.6] - 2026-03-06

### Changed

- chore(deps): update golang docker tag to v1.26.1 ([#311](https://github.com/giantswarm/mcp-kubernetes/pull/311))
- chore(deps): update dependency go to v1.26.1 ([#309](https://github.com/giantswarm/mcp-kubernetes/pull/309))
- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.74 ([#312](https://github.com/giantswarm/mcp-kubernetes/pull/312))

## [0.1.5] - 2026-03-06

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.73 ([#310](https://github.com/giantswarm/mcp-kubernetes/pull/310))

## [0.1.4] - 2026-03-05

### Changed

- fix(deps): update mcp-oauth to v0.2.72 for VS Code state parameter compatibility ([#308](https://github.com/giantswarm/mcp-kubernetes/pull/308))

## [0.1.3] - 2026-03-05

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.71 ([#307](https://github.com/giantswarm/mcp-kubernetes/pull/307))

## [0.1.2] - 2026-03-03

### Changed

- fix(deps): update opentelemetry-go monorepo ([#305](https://github.com/giantswarm/mcp-kubernetes/pull/305))

## [0.1.1] - 2026-03-03

### Fixed

- fix(ci): prevent auto-release from conflicting with manual releases ([#306](https://github.com/giantswarm/mcp-kubernetes/pull/306))

## [0.1.0] - 2026-02-27

### Changed

- Updated CAPI Cluster discovery to use `cluster.x-k8s.io/v1beta2` API ([#253](https://github.com/giantswarm/mcp-kubernetes/issues/253))
- Switch CI to `push-to-registries-multiarch` with amd64-only on branches
  and full multi-arch on release tags for faster PR builds.
- Run chart tests before pushing to the app catalog.
- Update Dockerfile with multi-stage Go build for buildx multi-arch support.
- Update `mcp-oauth` to v0.2.69.

### Fixed

- Fixed `capi_cluster_health` reporting healthy clusters as UNHEALTHY due to CAPI v1beta2 schema changes ([#287](https://github.com/giantswarm/mcp-kubernetes/issues/287))

## [0.0.177] - 2026-03-01

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.70 ([#301](https://github.com/giantswarm/mcp-kubernetes/pull/301))

## [0.0.176] - 2026-02-27

### Changed

- chore(deps): update goreleaser/goreleaser-action action to v7 ([#304](https://github.com/giantswarm/mcp-kubernetes/pull/304))

## [0.0.175] - 2026-02-27

### Changed

- fix(deps): update k8s modules to v0.35.2 ([#300](https://github.com/giantswarm/mcp-kubernetes/pull/300))

## [0.0.174] - 2026-02-27

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.44.1 ([#303](https://github.com/giantswarm/mcp-kubernetes/pull/303))

## [0.0.173] - 2026-02-27

### Fixed

- Fix: Split release image push so China mirror failures don't block chart publication ([#302](https://github.com/giantswarm/mcp-kubernetes/pull/302))

## [0.0.172] - 2026-02-27

### Changed

- chore(deps): update mcp-oauth to v0.2.69 ([#299](https://github.com/giantswarm/mcp-kubernetes/pull/299))

## [0.0.171] - 2026-02-27

### Changed

- Release v0.1.0 ([#298](https://github.com/giantswarm/mcp-kubernetes/pull/298))

## [0.0.170] - 2026-02-27

### Fixed

- fix(tools): disable port forwarding tools in in-cluster mode ([#297](https://github.com/giantswarm/mcp-kubernetes/pull/297))

## [0.0.169] - 2026-02-24

### Changed

- chore(deps): update mcp-oauth to v0.2.68 ([#295](https://github.com/giantswarm/mcp-kubernetes/pull/295))

## [0.0.168] - 2026-02-21

### Fixed

- fix: multiarch Dockerfile and faster CI with correct job ordering ([#294](https://github.com/giantswarm/mcp-kubernetes/pull/294))

## [0.0.167] - 2026-02-20

### Added

- Add extra audit to show user/group info ([#293](https://github.com/giantswarm/mcp-kubernetes/pull/293))

## [0.0.166] - 2026-02-20

### Added

- Add logging for tools call and initilize ([#283](https://github.com/giantswarm/mcp-kubernetes/pull/283))

## [0.0.165] - 2026-02-19

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.67 ([#291](https://github.com/giantswarm/mcp-kubernetes/pull/291))

## [0.0.164] - 2026-02-19

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.62 ([#289](https://github.com/giantswarm/mcp-kubernetes/pull/289))

## [0.0.163] - 2026-02-18

### Fixed

- Fix capi_cluster_health reporting healthy clusters as unhealthy ([#288](https://github.com/giantswarm/mcp-kubernetes/pull/288))

## [0.0.162] - 2026-02-18

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.44.0 ([#285](https://github.com/giantswarm/mcp-kubernetes/pull/285))

## [0.0.161] - 2026-02-18

### Changed

- chore(deps): update dependency architect to v6.13.0 ([#286](https://github.com/giantswarm/mcp-kubernetes/pull/286))

## [0.0.160] - 2026-02-12

### Changed

- chore(deps): update dependency go to v1.26.0 ([#280](https://github.com/giantswarm/mcp-kubernetes/pull/280))

## [0.0.159] - 2026-02-12

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.59 ([#277](https://github.com/giantswarm/mcp-kubernetes/pull/277))

## [0.0.158] - 2026-02-12

### Changed

- fix(deps): update module golang.org/x/text to v0.34.0 ([#278](https://github.com/giantswarm/mcp-kubernetes/pull/278))

## [0.0.157] - 2026-02-12

### Changed

- fix(deps): update k8s modules to v0.35.1 ([#281](https://github.com/giantswarm/mcp-kubernetes/pull/281))

## [0.0.156] - 2026-02-11

### Added

- feat(federation): add group mapping for workload cluster impersonation ([#279](https://github.com/giantswarm/mcp-kubernetes/pull/279))

## [0.0.155] - 2026-02-09

### Changed

- fix(deps): update module golang.org/x/oauth2 to v0.35.0 ([#276](https://github.com/giantswarm/mcp-kubernetes/pull/276))

## [0.0.154] - 2026-02-06

### Changed

- fix(deps): update opentelemetry-go monorepo ([#265](https://github.com/giantswarm/mcp-kubernetes/pull/265))

## [0.0.153] - 2026-02-06

### Fixed

- fix(federation): use privileged CAPI discovery in kubeconfig retrieval ([#272](https://github.com/giantswarm/mcp-kubernetes/pull/272))

## [0.0.152] - 2026-02-05

### Added

- feat(federation): use privileged CAPI discovery with split-credential model ([#269](https://github.com/giantswarm/mcp-kubernetes/pull/269))

## [0.0.151] - 2026-02-05

### Changed

- chore(deps): update dependency go to v1.25.7 ([#267](https://github.com/giantswarm/mcp-kubernetes/pull/267))

## [0.0.150] - 2026-02-05

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.58 ([#268](https://github.com/giantswarm/mcp-kubernetes/pull/268))

## [0.0.149] - 2026-02-03

### Changed

- Align files ([#266](https://github.com/giantswarm/mcp-kubernetes/pull/266))

## [0.0.148] - 2026-02-03

### Added

- feat(rbac): enforce minimal profile and validate custom RBAC with OAuth downstream ([#264](https://github.com/giantswarm/mcp-kubernetes/pull/264))

## [0.0.147] - 2026-01-30

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.56 ([#263](https://github.com/giantswarm/mcp-kubernetes/pull/263))

## [0.0.146] - 2026-01-23

### Fixed

- fix(observability): improve cluster classification patterns and dashboard labels ([#261](https://github.com/giantswarm/mcp-kubernetes/pull/261))

## [0.0.145] - 2026-01-23

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.52 ([#257](https://github.com/giantswarm/mcp-kubernetes/pull/257))

## [0.0.144] - 2026-01-23

### Fixed

- fix(metrics): wire up federation metrics and rename to mcp_kubernetes_ prefix ([#258](https://github.com/giantswarm/mcp-kubernetes/pull/258))

## [0.0.143] - 2026-01-23

### Added

- feat(federation): wire up HybridOAuthClientProvider for privileged secret access ([#259](https://github.com/giantswarm/mcp-kubernetes/pull/259))

## [0.0.142] - 2026-01-23

### Fixed

- fix(metrics): unify Kubernetes ops metric labels ([#255](https://github.com/giantswarm/mcp-kubernetes/pull/255))

## [0.0.141] - 2026-01-23

### Fixed

- fix(federation): use CAPI v1beta2 for cluster discovery ([#254](https://github.com/giantswarm/mcp-kubernetes/pull/254))

## [0.0.140] - 2026-01-23

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.50 ([#251](https://github.com/giantswarm/mcp-kubernetes/pull/251))

## [0.0.139] - 2026-01-23

### Fixed

- fix(dashboards): correct metric names and update mcp-oauth dependency ([#250](https://github.com/giantswarm/mcp-kubernetes/pull/250))

## [0.0.138] - 2026-01-22

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.47 ([#245](https://github.com/giantswarm/mcp-kubernetes/pull/245))

## [0.0.137] - 2026-01-22

### Added

- feat(helm): add Grafana dashboards for observability ([#248](https://github.com/giantswarm/mcp-kubernetes/pull/248))

## [0.0.136] - 2026-01-22

### Added

- feat(oauth): add silent token renewal support ([#247](https://github.com/giantswarm/mcp-kubernetes/pull/247))

## [0.0.135] - 2026-01-22

### Added

- feat(capi): Add SSO token passthrough to workload clusters ([#243](https://github.com/giantswarm/mcp-kubernetes/pull/243))

## [0.0.134] - 2026-01-20

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.45 ([#242](https://github.com/giantswarm/mcp-kubernetes/pull/242))

## [0.0.133] - 2026-01-20

### Changed

- chore(deps): update dependency architect to v6.12.0 ([#241](https://github.com/giantswarm/mcp-kubernetes/pull/241))

## [0.0.132] - 2026-01-20

### Fixed

- fix(oauth): Inject SSO-forwarded tokens for downstream K8s API auth ([#239](https://github.com/giantswarm/mcp-kubernetes/pull/239))

## [0.0.131] - 2026-01-20

### Added

- feat(federation): Add ServiceAccount for kubeconfig secret access ([#222](https://github.com/giantswarm/mcp-kubernetes/pull/222))

## [0.0.130] - 2026-01-20

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.42 ([#218](https://github.com/giantswarm/mcp-kubernetes/pull/218))

## [0.0.129] - 2026-01-19

### Fixed

- fix(helm): make debug flag configurable with secure default ([#236](https://github.com/giantswarm/mcp-kubernetes/pull/236))

## [0.0.128] - 2026-01-19

### Changed

- docs: add security assessment by LLMs ([#226](https://github.com/giantswarm/mcp-kubernetes/pull/226))

## [0.0.127] - 2026-01-18

### Added

- feat(oauth): Wire SSOAllowPrivateIPs to mcp-oauth v0.2.40 AllowPrivateIPJWKS ([#225](https://github.com/giantswarm/mcp-kubernetes/pull/225))

## [0.0.126] - 2026-01-18

### Added

- feat(helm): add Gateway API support to Helm chart ([#224](https://github.com/giantswarm/mcp-kubernetes/pull/224))

## [0.0.125] - 2026-01-17

### Added

- feat(oauth): add TrustedAudiences for SSO token forwarding ([#221](https://github.com/giantswarm/mcp-kubernetes/pull/221))

## [0.0.124] - 2026-01-16

### Fixed

- fix(oauth): update mcp-oauth v0.2.35 and enable proactive token refresh ([#217](https://github.com/giantswarm/mcp-kubernetes/pull/217))

## [0.0.123] - 2026-01-16

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.34 ([#214](https://github.com/giantswarm/mcp-kubernetes/pull/214))

## [0.0.122] - 2026-01-16

### Changed

- chore(deps): update dependency go to v1.25.6 ([#216](https://github.com/giantswarm/mcp-kubernetes/pull/216))

## [0.0.121] - 2026-01-15

### Added

- feat(oauth): Add CIMD private IP allowlist option for internal deployments ([#215](https://github.com/giantswarm/mcp-kubernetes/pull/215))

## [0.0.120] - 2026-01-14

### Fixed

- fix(oauth): allow CIMD-only mode without DCR configuration ([#211](https://github.com/giantswarm/mcp-kubernetes/pull/211))

## [0.0.119] - 2026-01-14

### Added

- feat(helm): migrate chart metadata to OCI-compliant format ([#165](https://github.com/giantswarm/mcp-kubernetes/pull/165))

## [0.0.118] - 2026-01-14

### Fixed

- fix(helm): use correct allowPublicRegistration key in deployment template ([#209](https://github.com/giantswarm/mcp-kubernetes/pull/209))

## [0.0.117] - 2026-01-12

### Changed

- fix(deps): update k8s modules to v0.35.0 ([#186](https://github.com/giantswarm/mcp-kubernetes/pull/186))

## [0.0.116] - 2026-01-12

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.31 ([#208](https://github.com/giantswarm/mcp-kubernetes/pull/208))

## [0.0.115] - 2026-01-12

### Changed

- fix(deps): update module golang.org/x/text to v0.33.0 ([#207](https://github.com/giantswarm/mcp-kubernetes/pull/207))

## [0.0.114] - 2026-01-09

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.30 ([#206](https://github.com/giantswarm/mcp-kubernetes/pull/206))

## [0.0.113] - 2026-01-09

### Changed

- Align files ([#205](https://github.com/giantswarm/mcp-kubernetes/pull/205))

## [0.0.112] - 2025-12-24

### Fixed

- fix: propagate access token to mcp-go tool execution context ([#204](https://github.com/giantswarm/mcp-kubernetes/pull/204))

## [0.0.111] - 2025-12-23

### Fixed

- fix: update mcp-oauth to v0.2.29 to fix Valkey token serialization ([#203](https://github.com/giantswarm/mcp-kubernetes/pull/203))

## [0.0.110] - 2025-12-23

### Changed

- Update mcp-oauth to v0.2.27 ([#201](https://github.com/giantswarm/mcp-kubernetes/pull/201))

## [0.0.109] - 2025-12-22

### Changed

- ci: Add helm lint and unit tests to CI workflow ([#198](https://github.com/giantswarm/mcp-kubernetes/pull/198))

## [0.0.108] - 2025-12-22

### Fixed

- fix: apply output processing to Patch and Describe handlers ([#199](https://github.com/giantswarm/mcp-kubernetes/pull/199))

## [0.0.107] - 2025-12-22

### Added

- feat(resource): Add _meta response metadata to all resource tool calls ([#195](https://github.com/giantswarm/mcp-kubernetes/pull/195))

## [0.0.106] - 2025-12-22

### Changed

- fix(deps): update module github.com/creativeprojects/go-selfupdate to v1.5.2 ([#197](https://github.com/giantswarm/mcp-kubernetes/pull/197))

## [0.0.105] - 2025-12-19

### Changed

- docs(readme): add federation and multi-cluster support documentation ([#194](https://github.com/giantswarm/mcp-kubernetes/pull/194))

## [0.0.104] - 2025-12-19

### Fixed

- fix(resource): make namespace optional for cluster-scoped resources ([#191](https://github.com/giantswarm/mcp-kubernetes/pull/191))

## [0.0.103] - 2025-12-19

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.26 ([#192](https://github.com/giantswarm/mcp-kubernetes/pull/192))

## [0.0.102] - 2025-12-19

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.24 ([#178](https://github.com/giantswarm/mcp-kubernetes/pull/178))

## [0.0.101] - 2025-12-19

### Added

- Add TLS support and a guide to test it local ([#183](https://github.com/giantswarm/mcp-kubernetes/pull/183))

## [0.0.100] - 2025-12-18

### Added

- feat(helm): Add allowPrivateURLs option to values.yaml ([#190](https://github.com/giantswarm/mcp-kubernetes/pull/190))

## [0.0.99] - 2025-12-18

### Fixed

- fix(security): make version disclosure configurable in health endpoints ([#189](https://github.com/giantswarm/mcp-kubernetes/pull/189))

## [0.0.98] - 2025-12-14

### Changed

- chore(deps): upgrade mcp-oauth to v0.2.23 ([#182](https://github.com/giantswarm/mcp-kubernetes/pull/182))

## [0.0.97] - 2025-12-14

### Fixed

- fix(helm): make registration-token optional when using trusted schemes ([#181](https://github.com/giantswarm/mcp-kubernetes/pull/181))

## [0.0.96] - 2025-12-14

### Added

- feat(oauth): enable Client ID Metadata Documents (CIMD) support ([#180](https://github.com/giantswarm/mcp-kubernetes/pull/180))

## [0.0.95] - 2025-12-13

### Added

- feat(security): Add redirect URI validation and trusted scheme registration ([#179](https://github.com/giantswarm/mcp-kubernetes/pull/179))

## [0.0.94] - 2025-12-12

### Added

- feat(security): expose metrics on separate port ([#175](https://github.com/giantswarm/mcp-kubernetes/pull/175))

## [0.0.93] - 2025-12-12

### Changed

- ci: optimize release dry-run to build only linux/amd64 ([#174](https://github.com/giantswarm/mcp-kubernetes/pull/174))

## [0.0.92] - 2025-12-12

### Fixed

- fix(security): pass maxClientsPerIP to client registration rate limiter ([#171](https://github.com/giantswarm/mcp-kubernetes/pull/171))

## [0.0.91] - 2025-12-12

### Changed

- ci: enable Go build caching to speed up CI ([#173](https://github.com/giantswarm/mcp-kubernetes/pull/173))

## [0.0.90] - 2025-12-12

### Added

- feat(federation): implement federated k8s.Client for multi-cluster operations ([#167](https://github.com/giantswarm/mcp-kubernetes/pull/167))

## [0.0.89] - 2025-12-12

### Added

- feat(oauth): add configurable Kubernetes authenticator client ID for Dex ([#164](https://github.com/giantswarm/mcp-kubernetes/pull/164))

## [0.0.88] - 2025-12-11

### Added

- feat(logging): add detailed timing logs to handleListResources ([#161](https://github.com/giantswarm/mcp-kubernetes/pull/161))

## [0.0.87] - 2025-12-11

### Changed

- chore(deps): update dependency architect to v6.11.0 ([#163](https://github.com/giantswarm/mcp-kubernetes/pull/163))

## [0.0.86] - 2025-12-11

### Fixed

- fix(logging): configure default slog logger for debug mode ([#160](https://github.com/giantswarm/mcp-kubernetes/pull/160))

## [0.0.85] - 2025-12-10

### Fixed

- fix(oauth): add detailed debug logging to AccessTokenInjector ([#159](https://github.com/giantswarm/mcp-kubernetes/pull/159))

## [0.0.84] - 2025-12-10

### Added

- feat(tools): add debug logging and integration tests for streamable-http ([#158](https://github.com/giantswarm/mcp-kubernetes/pull/158))

## [0.0.83] - 2025-12-10

### Added

- feat(oauth): add --dex-ca-file flag for private CA support ([#157](https://github.com/giantswarm/mcp-kubernetes/pull/157))

## [0.0.82] - 2025-12-10

### Fixed

- fix(oauth): add --allow-private-oauth-urls flag for internal deployments ([#156](https://github.com/giantswarm/mcp-kubernetes/pull/156))

## [0.0.81] - 2025-12-10

### Added

- feat(logging): implement structured logging with slog ([#155](https://github.com/giantswarm/mcp-kubernetes/pull/155))

## [0.0.80] - 2025-12-10

### Changed

- chore(deps): upgrade mcp-oauth to v0.2.15 ([#153](https://github.com/giantswarm/mcp-kubernetes/pull/153))

## [0.0.79] - 2025-12-10

### Changed

- fix(deps): update k8s modules to v0.34.3 ([#152](https://github.com/giantswarm/mcp-kubernetes/pull/152))

## [0.0.78] - 2025-12-09

### Added

- feat(oauth): Add Valkey storage backend for production deployments ([#151](https://github.com/giantswarm/mcp-kubernetes/pull/151))

## [0.0.77] - 2025-12-09

### Fixed

- fix(safety): Fix dry-run mode integration with non-destructive mode ([#150](https://github.com/giantswarm/mcp-kubernetes/pull/150))

## [0.0.76] - 2025-12-09

### Fixed

- fix(helm): remove trailing spaces from values.yaml ([#149](https://github.com/giantswarm/mcp-kubernetes/pull/149))

## [0.0.75] - 2025-12-09

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.14 ([#148](https://github.com/giantswarm/mcp-kubernetes/pull/148))

## [0.0.74] - 2025-12-09

### Added

- feat(helm): Add minimal-privilege RBAC profiles for production ([#147](https://github.com/giantswarm/mcp-kubernetes/pull/147))

## [0.0.73] - 2025-12-09

### Fixed

- fix(oauth): implement fail-closed behavior for downstream OAuth ([#145](https://github.com/giantswarm/mcp-kubernetes/pull/145))

## [0.0.72] - 2025-12-09

### Changed

- fix(deps): update module github.com/giantswarm/mcp-oauth to v0.2.13 ([#142](https://github.com/giantswarm/mcp-kubernetes/pull/142))

## [0.0.71] - 2025-12-09

### Changed

- chore(deps): update dependency architect to v6.10.0 ([#141](https://github.com/giantswarm/mcp-kubernetes/pull/141))

## [0.0.70] - 2025-12-09

### Changed

- docs: add ingress connection documentation ([#140](https://github.com/giantswarm/mcp-kubernetes/pull/140))

## [0.0.69] - 2025-12-09

### Changed

- docs: Clarify RBAC usage in Service Account vs OAuth Downstream modes ([#139](https://github.com/giantswarm/mcp-kubernetes/pull/139))

## [0.0.68] - 2025-12-09

### Added

- feat(federation): wire up federation manager with OAuth downstream support ([#137](https://github.com/giantswarm/mcp-kubernetes/pull/137))

## [0.0.67] - 2025-12-09

### Added

- feat(observability): Add CAPI mode observability and audit logging ([#135](https://github.com/giantswarm/mcp-kubernetes/pull/135))

## [0.0.66] - 2025-12-09

### Added

- feat(rbac): Configure RBAC for Management Cluster Access ([#134](https://github.com/giantswarm/mcp-kubernetes/pull/134))

## [0.0.65] - 2025-12-08

### Added

- feat(helm): Add CAPI Mode configuration to Helm chart ([#133](https://github.com/giantswarm/mcp-kubernetes/pull/133))

## [0.0.64] - 2025-12-08

### Added

- feat(tools): Implement Fleet-Scale Output Filtering and Truncation ([#132](https://github.com/giantswarm/mcp-kubernetes/pull/132))

## [0.0.63] - 2025-12-08

### Added

- feat(tools): Implement CAPI discovery tools ([#131](https://github.com/giantswarm/mcp-kubernetes/pull/131))

## [0.0.62] - 2025-12-08

### Changed

- fix(deps): update opentelemetry-go monorepo ([#128](https://github.com/giantswarm/mcp-kubernetes/pull/128))

## [0.0.61] - 2025-12-08

### Added

- feat(tools): add multi-cluster support with cluster parameter ([#130](https://github.com/giantswarm/mcp-kubernetes/pull/130))

## [0.0.60] - 2025-12-08

### Added

- feat(federation): Add network topology and connectivity handling ([#129](https://github.com/giantswarm/mcp-kubernetes/pull/129))

## [0.0.59] - 2025-12-08

### Added

- feat(federation): implement CAPI cluster discovery ([#127](https://github.com/giantswarm/mcp-kubernetes/pull/127))

## [0.0.58] - 2025-12-08

### Changed

- fix(deps): update opentelemetry-go monorepo to v1.39.0 ([#125](https://github.com/giantswarm/mcp-kubernetes/pull/125))

## [0.0.57] - 2025-12-08

### Added

- feat(security): Add SubjectAccessReview pre-flight checks ([#126](https://github.com/giantswarm/mcp-kubernetes/pull/126))

## [0.0.56] - 2025-12-08

### Added

- feat(security): Implement Kubernetes User Impersonation for Identity Propagation ([#124](https://github.com/giantswarm/mcp-kubernetes/pull/124))

## [0.0.55] - 2025-12-08

### Added

- feat(oauth): Integrate mcp-oauth for OAuth 2.1 Authentication ([#123](https://github.com/giantswarm/mcp-kubernetes/pull/123))

## [0.0.54] - 2025-12-08

### Added

- feat(federation): Implement Kubeconfig Secret Retrieval for CAPI Clusters ([#122](https://github.com/giantswarm/mcp-kubernetes/pull/122))

## [0.0.53] - 2025-12-08

### Added

- feat(federation): Implement thread-safe client caching with TTL eviction ([#121](https://github.com/giantswarm/mcp-kubernetes/pull/121))

## [0.0.52] - 2025-12-08

### Added

- feat(federation): create federation package for multi-cluster support ([#120](https://github.com/giantswarm/mcp-kubernetes/pull/120))

## [0.0.51] - 2025-12-08

### Added

- feat: Add client-side filtering support for kubernetes_list ([#99](https://github.com/giantswarm/mcp-kubernetes/pull/99))

## [0.0.50] - 2025-12-08

### Changed

- chore(deps): update dependency architect to v6.9.0 ([#92](https://github.com/giantswarm/mcp-kubernetes/pull/92))

## [0.0.49] - 2025-12-08

### Changed

- fix(deps): update module github.com/spf13/cobra to v1.10.2 ([#102](https://github.com/giantswarm/mcp-kubernetes/pull/102))

## [0.0.48] - 2025-12-08

### Changed

- fix(deps): update module golang.org/x/oauth2 to v0.34.0 ([#119](https://github.com/giantswarm/mcp-kubernetes/pull/119))

## [0.0.47] - 2025-12-08

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.43.2 ([#103](https://github.com/giantswarm/mcp-kubernetes/pull/103))

## [0.0.46] - 2025-12-08

### Added

- feat: Add comprehensive OpenTelemetry instrumentation ([#101](https://github.com/giantswarm/mcp-kubernetes/pull/101))

## [0.0.45] - 2025-12-02

### Added

- feat(k8s): enhance ResourceManager interface to include apiGroup ([#95](https://github.com/giantswarm/mcp-kubernetes/pull/95))

## [0.0.44] - 2025-11-30

### Added

- feat: Add Dex OIDC provider support and update mcp-oauth to v0.2.7 ([#98](https://github.com/giantswarm/mcp-kubernetes/pull/98))

## [0.0.43] - 2025-11-30

### Added

- feat(oauth): Add OAuth 2.1 authentication support ([#94](https://github.com/giantswarm/mcp-kubernetes/pull/94))

## [0.0.42] - 2025-11-24

### Changed

- chore(deps): update dependency go to v1.25.1 ([#81](https://github.com/giantswarm/mcp-kubernetes/pull/81))

## [0.0.41] - 2025-11-24

### Changed

- chore(deps): update dependency architect to v6.6.1 ([#82](https://github.com/giantswarm/mcp-kubernetes/pull/82))

## [0.0.40] - 2025-11-24

### Changed

- chore(deps): update actions/checkout action to v6 ([#91](https://github.com/giantswarm/mcp-kubernetes/pull/91))

## [0.0.39] - 2025-11-24

### Changed

- fix(deps): update k8s modules to v0.34.2 ([#90](https://github.com/giantswarm/mcp-kubernetes/pull/90))

## [0.0.38] - 2025-11-24

### Changed

- Align files ([#89](https://github.com/giantswarm/mcp-kubernetes/pull/89))

## [0.0.37] - 2025-11-24

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.43.1 ([#85](https://github.com/giantswarm/mcp-kubernetes/pull/85))

## [0.0.36] - 2025-10-08

### Added

- feat(logging): implement silent logger for stdio mode to suppress output ([#86](https://github.com/giantswarm/mcp-kubernetes/pull/86))

## [0.0.35] - 2025-09-29

### Changed

- fix(deps): update k8s modules to v0.34.1 ([#83](https://github.com/giantswarm/mcp-kubernetes/pull/83))

## [0.0.34] - 2025-09-29

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.41.0 ([#84](https://github.com/giantswarm/mcp-kubernetes/pull/84))

## [0.0.33] - 2025-09-04

### Changed

- fix(deps): update module github.com/spf13/cobra to v1.10.1 ([#77](https://github.com/giantswarm/mcp-kubernetes/pull/77))

## [0.0.32] - 2025-09-04

### Changed

- chore(deps): update dependency architect to v6.5.0 ([#78](https://github.com/giantswarm/mcp-kubernetes/pull/78))

## [0.0.31] - 2025-09-04

### Changed

- chore(deps): update actions/setup-python action to v6 ([#80](https://github.com/giantswarm/mcp-kubernetes/pull/80))

## [0.0.30] - 2025-09-04

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.39.1 ([#79](https://github.com/giantswarm/mcp-kubernetes/pull/79))

## [0.0.29] - 2025-09-01

### Changed

- fix(deps): update k8s modules to v0.34.0 ([#72](https://github.com/giantswarm/mcp-kubernetes/pull/72))

## [0.0.28] - 2025-09-01

### Changed

- Align files ([#73](https://github.com/giantswarm/mcp-kubernetes/pull/73))

## [0.0.27] - 2025-09-01

### Changed

- chore(deps): update dependency pytest-rerunfailures to v16 ([#76](https://github.com/giantswarm/mcp-kubernetes/pull/76))

## [0.0.26] - 2025-09-01

### Changed

- fix(deps): update module github.com/creativeprojects/go-selfupdate to v1.5.1 ([#75](https://github.com/giantswarm/mcp-kubernetes/pull/75))

## [0.0.25] - 2025-09-01

### Changed

- fix(deps): update module github.com/stretchr/testify to v1.11.1 ([#74](https://github.com/giantswarm/mcp-kubernetes/pull/74))

## [0.0.24] - 2025-09-01

### Changed

- chore(deps): update dependency go to 1.25 ([#71](https://github.com/giantswarm/mcp-kubernetes/pull/71))

## [0.0.23] - 2025-09-01

### Changed

- chore(deps): update actions/checkout action to v5 ([#70](https://github.com/giantswarm/mcp-kubernetes/pull/70))

## [0.0.22] - 2025-09-01

### Changed

- fix(deps): update module github.com/mark3labs/mcp-go to v0.38.0 ([#67](https://github.com/giantswarm/mcp-kubernetes/pull/67))

## [0.0.21] - 2025-09-01

### Changed

- chore(deps): update dependency architect to v6.4.0 ([#69](https://github.com/giantswarm/mcp-kubernetes/pull/69))

## [0.0.20] - 2025-07-30

### Changed

- Re-enable autorelease for each commit to main ([#68](https://github.com/giantswarm/mcp-kubernetes/pull/68))

## [0.0.19] - 2025-07-24

### Changed

- Align renovate config

## [0.0.18] - 2025-07-24

### Added

- Helm chart for Kubernetes deployment ([#54](https://github.com/giantswarm/mcp-kubernetes/pull/54))
- In-cluster authentication support with `--in-cluster` flag ([#56](https://github.com/giantswarm/mcp-kubernetes/pull/56))
- Support for reading kubeconfig from environment variable ([#58](https://github.com/giantswarm/mcp-kubernetes/pull/58))

### Fixed

- Fixed Go imports formatting ([#51](https://github.com/giantswarm/mcp-kubernetes/pull/51))
- Removed expectation for kubeconfig or context to exist at startup ([#57](https://github.com/giantswarm/mcp-kubernetes/pull/57))

## [0.0.17] - 2025-07-20

### Added

- Comprehensive pagination for MCP tools ([#43](https://github.com/giantswarm/mcp-kubernetes/pull/43))

## [0.0.16] - 2025-07-17

### Fixed

- Updated module github.com/mark3labs/mcp-go to v0.34.0 ([#41](https://github.com/giantswarm/mcp-kubernetes/pull/41))

## [0.0.15] - 2025-07-17

### Fixed

- Updated kubernetes packages to v0.33.3 ([#42](https://github.com/giantswarm/mcp-kubernetes/pull/42))

## [0.0.14] - 2025-07-10

### Fixed

- Relaxed namespace requirement for listing namespaces ([#39](https://github.com/giantswarm/mcp-kubernetes/pull/39))

## [0.0.13] - 2025-07-09

### Fixed

- Updated module github.com/mark3labs/mcp-go to v0.33.0 ([#40](https://github.com/giantswarm/mcp-kubernetes/pull/40))

## [0.0.12] - 2025-07-05

### Added

- Structured response for port forwarding sessions ([#38](https://github.com/giantswarm/mcp-kubernetes/pull/38))

## [0.0.11] - 2025-06-27

### Changed

- Reduced output for resource listing ([#31](https://github.com/giantswarm/mcp-kubernetes/pull/31))

## [0.0.10] - 2025-06-27

### Fixed

- Updated kubernetes packages to v0.33.2 ([#32](https://github.com/giantswarm/mcp-kubernetes/pull/32))

## [0.0.9] - 2025-06-27

### Changed

- Phase 1: Foundation Standardization - Confirm Alignment ([#37](https://github.com/giantswarm/mcp-kubernetes/pull/37))

## [0.0.8] - 2025-06-26

### Removed

- Task-Master ([#30](https://github.com/giantswarm/mcp-kubernetes/pull/30))

## [0.0.7] - 2025-06-26

### Changed

- refactor: remove Helm-related functionality and update documentation by @teemow in #8

## [0.0.6] - 2025-06-26

### Changed

- feat: add version command and improve context tool naming by @teemow in #7

## [0.0.5] - 2025-06-26

### Changed

- Moved main to root folder by @teemow in #6

## [0.0.4] - 2025-06-26

### Changed

- Configure Renovate

## [0.0.3] - 2025-06-26

Initial release

### Added

- Kubernetes client package implementation ([#3](https://github.com/giantswarm/mcp-kubernetes/pull/3))

[Unreleased]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.73...HEAD
[0.1.73]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.72...v0.1.73
[0.1.72]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.71...v0.1.72
[0.1.71]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.70...v0.1.71
[0.1.70]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.69...v0.1.70
[0.1.69]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.68...v0.1.69
[0.1.68]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.67...v0.1.68
[0.1.67]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.66...v0.1.67
[0.1.66]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.65...v0.1.66
[0.1.65]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.64...v0.1.65
[0.1.64]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.63...v0.1.64
[0.1.63]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.62...v0.1.63
[0.1.62]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.61...v0.1.62
[0.1.61]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.60...v0.1.61
[0.1.60]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.59...v0.1.60
[0.1.59]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.58...v0.1.59
[0.1.58]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.57...v0.1.58
[0.1.57]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.56...v0.1.57
[0.1.56]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.55...v0.1.56
[0.1.55]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.54...v0.1.55
[0.1.54]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.53...v0.1.54
[0.1.53]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.52...v0.1.53
[0.1.52]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.51...v0.1.52
[0.1.51]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.50...v0.1.51
[0.1.50]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.49...v0.1.50
[0.1.49]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.48...v0.1.49
[0.1.48]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.47...v0.1.48
[0.1.47]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.46...v0.1.47
[0.1.46]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.45...v0.1.46
[0.1.45]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.44...v0.1.45
[0.1.44]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.43...v0.1.44
[0.1.43]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.42...v0.1.43
[0.1.42]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.41...v0.1.42
[0.1.41]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.40...v0.1.41
[0.1.40]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.39...v0.1.40
[0.1.39]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.38...v0.1.39
[0.1.38]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.37...v0.1.38
[0.1.37]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.36...v0.1.37
[0.1.36]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.35...v0.1.36
[0.1.35]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.34...v0.1.35
[0.1.34]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.33...v0.1.34
[0.1.33]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.32...v0.1.33
[0.1.32]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.31...v0.1.32
[0.1.31]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.30...v0.1.31
[0.1.30]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.29...v0.1.30
[0.1.29]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.28...v0.1.29
[0.1.28]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.27...v0.1.28
[0.1.27]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.26...v0.1.27
[0.1.26]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.25...v0.1.26
[0.1.25]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.24...v0.1.25
[0.1.24]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.23...v0.1.24
[0.1.23]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.22...v0.1.23
[0.1.22]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.21...v0.1.22
[0.1.21]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.20...v0.1.21
[0.1.20]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.19...v0.1.20
[0.1.19]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.18...v0.1.19
[0.1.18]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.17...v0.1.18
[0.1.17]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.16...v0.1.17
[0.1.16]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.15...v0.1.16
[0.1.15]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.14...v0.1.15
[0.1.14]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.13...v0.1.14
[0.1.13]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.12...v0.1.13
[0.1.12]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.11...v0.1.12
[0.1.11]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.10...v0.1.11
[0.1.10]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.9...v0.1.10
[0.1.9]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.19...v0.1.0
[0.0.177]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.176...v0.0.177
[0.0.176]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.175...v0.0.176
[0.0.175]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.174...v0.0.175
[0.0.174]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.173...v0.0.174
[0.0.173]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.172...v0.0.173
[0.0.172]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.171...v0.0.172
[0.0.171]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.170...v0.0.171
[0.0.170]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.169...v0.0.170
[0.0.169]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.168...v0.0.169
[0.0.168]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.167...v0.0.168
[0.0.167]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.166...v0.0.167
[0.0.166]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.165...v0.0.166
[0.0.165]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.164...v0.0.165
[0.0.164]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.163...v0.0.164
[0.0.163]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.162...v0.0.163
[0.0.162]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.161...v0.0.162
[0.0.161]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.160...v0.0.161
[0.0.160]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.159...v0.0.160
[0.0.159]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.158...v0.0.159
[0.0.158]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.157...v0.0.158
[0.0.157]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.156...v0.0.157
[0.0.156]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.155...v0.0.156
[0.0.155]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.154...v0.0.155
[0.0.154]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.153...v0.0.154
[0.0.153]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.152...v0.0.153
[0.0.152]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.151...v0.0.152
[0.0.151]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.150...v0.0.151
[0.0.150]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.149...v0.0.150
[0.0.149]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.148...v0.0.149
[0.0.148]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.147...v0.0.148
[0.0.147]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.146...v0.0.147
[0.0.146]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.145...v0.0.146
[0.0.145]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.144...v0.0.145
[0.0.144]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.143...v0.0.144
[0.0.143]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.142...v0.0.143
[0.0.142]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.141...v0.0.142
[0.0.141]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.140...v0.0.141
[0.0.140]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.139...v0.0.140
[0.0.139]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.138...v0.0.139
[0.0.138]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.137...v0.0.138
[0.0.137]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.136...v0.0.137
[0.0.136]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.135...v0.0.136
[0.0.135]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.134...v0.0.135
[0.0.134]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.133...v0.0.134
[0.0.133]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.132...v0.0.133
[0.0.132]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.131...v0.0.132
[0.0.131]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.130...v0.0.131
[0.0.130]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.129...v0.0.130
[0.0.129]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.128...v0.0.129
[0.0.128]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.127...v0.0.128
[0.0.127]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.126...v0.0.127
[0.0.126]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.125...v0.0.126
[0.0.125]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.124...v0.0.125
[0.0.124]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.123...v0.0.124
[0.0.123]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.122...v0.0.123
[0.0.122]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.121...v0.0.122
[0.0.121]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.120...v0.0.121
[0.0.120]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.119...v0.0.120
[0.0.119]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.118...v0.0.119
[0.0.118]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.117...v0.0.118
[0.0.117]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.116...v0.0.117
[0.0.116]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.115...v0.0.116
[0.0.115]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.114...v0.0.115
[0.0.114]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.113...v0.0.114
[0.0.113]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.112...v0.0.113
[0.0.112]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.111...v0.0.112
[0.0.111]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.110...v0.0.111
[0.0.110]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.109...v0.0.110
[0.0.109]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.108...v0.0.109
[0.0.108]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.107...v0.0.108
[0.0.107]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.106...v0.0.107
[0.0.106]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.105...v0.0.106
[0.0.105]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.104...v0.0.105
[0.0.104]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.103...v0.0.104
[0.0.103]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.102...v0.0.103
[0.0.102]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.101...v0.0.102
[0.0.101]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.100...v0.0.101
[0.0.100]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.99...v0.0.100
[0.0.99]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.98...v0.0.99
[0.0.98]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.97...v0.0.98
[0.0.97]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.96...v0.0.97
[0.0.96]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.95...v0.0.96
[0.0.95]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.94...v0.0.95
[0.0.94]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.93...v0.0.94
[0.0.93]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.92...v0.0.93
[0.0.92]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.91...v0.0.92
[0.0.91]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.90...v0.0.91
[0.0.90]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.89...v0.0.90
[0.0.89]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.88...v0.0.89
[0.0.88]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.87...v0.0.88
[0.0.87]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.86...v0.0.87
[0.0.86]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.85...v0.0.86
[0.0.85]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.84...v0.0.85
[0.0.84]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.83...v0.0.84
[0.0.83]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.82...v0.0.83
[0.0.82]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.81...v0.0.82
[0.0.81]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.80...v0.0.81
[0.0.80]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.79...v0.0.80
[0.0.79]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.78...v0.0.79
[0.0.78]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.77...v0.0.78
[0.0.77]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.76...v0.0.77
[0.0.76]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.75...v0.0.76
[0.0.75]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.74...v0.0.75
[0.0.74]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.73...v0.0.74
[0.0.73]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.72...v0.0.73
[0.0.72]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.71...v0.0.72
[0.0.71]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.70...v0.0.71
[0.0.70]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.69...v0.0.70
[0.0.69]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.68...v0.0.69
[0.0.68]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.67...v0.0.68
[0.0.67]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.66...v0.0.67
[0.0.66]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.65...v0.0.66
[0.0.65]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.64...v0.0.65
[0.0.64]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.63...v0.0.64
[0.0.63]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.62...v0.0.63
[0.0.62]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.61...v0.0.62
[0.0.61]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.60...v0.0.61
[0.0.60]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.59...v0.0.60
[0.0.59]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.58...v0.0.59
[0.0.58]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.57...v0.0.58
[0.0.57]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.56...v0.0.57
[0.0.56]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.55...v0.0.56
[0.0.55]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.54...v0.0.55
[0.0.54]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.53...v0.0.54
[0.0.53]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.52...v0.0.53
[0.0.52]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.51...v0.0.52
[0.0.51]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.50...v0.0.51
[0.0.50]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.49...v0.0.50
[0.0.49]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.48...v0.0.49
[0.0.48]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.47...v0.0.48
[0.0.47]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.46...v0.0.47
[0.0.46]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.45...v0.0.46
[0.0.45]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.44...v0.0.45
[0.0.44]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.43...v0.0.44
[0.0.43]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.42...v0.0.43
[0.0.42]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.41...v0.0.42
[0.0.41]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.40...v0.0.41
[0.0.40]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.39...v0.0.40
[0.0.39]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.38...v0.0.39
[0.0.38]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.37...v0.0.38
[0.0.37]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.36...v0.0.37
[0.0.36]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.35...v0.0.36
[0.0.35]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.34...v0.0.35
[0.0.34]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.33...v0.0.34
[0.0.33]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.32...v0.0.33
[0.0.32]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.31...v0.0.32
[0.0.31]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.30...v0.0.31
[0.0.30]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.29...v0.0.30
[0.0.29]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.28...v0.0.29
[0.0.28]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.27...v0.0.28
[0.0.27]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.26...v0.0.27
[0.0.26]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.25...v0.0.26
[0.0.25]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.24...v0.0.25
[0.0.24]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.23...v0.0.24
[0.0.23]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.22...v0.0.23
[0.0.22]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.21...v0.0.22
[0.0.21]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.20...v0.0.21
[0.0.20]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.19...v0.0.20
[0.0.19]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.18...v0.0.19
[0.0.18]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.17...v0.0.18
[0.0.17]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.16...v0.0.17
[0.0.16]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.15...v0.0.16
[0.0.15]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.14...v0.0.15
[0.0.14]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.13...v0.0.14
[0.0.13]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.12...v0.0.13
[0.0.12]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.11...v0.0.12
[0.0.11]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.8...v0.0.9
[0.0.8]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.7...v0.0.8
[0.0.7]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/giantswarm/mcp-kubernetes/releases/tag/v0.0.3
