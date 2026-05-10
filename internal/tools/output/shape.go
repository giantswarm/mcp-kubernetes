package output

import (
	"strings"
)

// Shaper is a Kind-aware transformation that runs on top of generic slim
// processing. It receives the resource map (already deep-copied by
// SlimResource) and returns a possibly-trimmed map.
//
// Shapers must be conservative: drop large blobs the LLM does not need to
// reason about (rendered Helm values, rollout history, opaque digests, very
// long env lists), but always preserve the diagnostic surface — kind /
// metadata.name / status.conditions / replica counts / Ready signals.
type Shaper func(map[string]interface{}) map[string]interface{}

// shaperKey is the (group, kind) lookup key used by the shaper registry.
// Group is the api-group portion of apiVersion (e.g. "apps" for "apps/v1");
// the empty string matches core/v1 resources.
type shaperKey struct {
	group string
	kind  string
}

// shapers is the registry of per-Kind shapers consulted by ShapeResource.
// Lookup is by exact (group, kind); apiVersion is parsed at call time.
var shapers = map[shaperKey]Shaper{
	{group: "helm.toolkit.fluxcd.io", kind: "HelmRelease"}: shapeHelmRelease,
	{group: "apps", kind: "Deployment"}:                    shapeWorkloadTemplate,
	{group: "apps", kind: "StatefulSet"}:                   shapeWorkloadTemplate,
	{group: "apps", kind: "DaemonSet"}:                     shapeWorkloadTemplate,
}

// envCollapseThreshold is the number of container env entries above which
// shapeWorkloadTemplate replaces the full env array with a count summary.
// 8 is small enough to keep typical "12-factor" configs readable while
// preventing 30+-entry env walls (mcp-kubernetes, karpenter, kserve, etc.)
// from dominating the response.
const envCollapseThreshold = 8

// ShapeResource applies the registered Kind-aware shaper to obj and returns
// the transformed map. obj is mutated in place; callers that need to
// preserve the original should deep-copy first (SlimResource already does).
//
// If no shaper is registered for the resource's apiVersion / kind, obj is
// returned unchanged. Shapers always run AFTER generic field exclusion so
// they can rely on bookkeeping fields already being gone.
func ShapeResource(obj map[string]interface{}) map[string]interface{} {
	if obj == nil {
		return nil
	}
	key, ok := lookupKey(obj)
	if !ok {
		return obj
	}
	shaper, ok := shapers[key]
	if !ok {
		return obj
	}
	return shaper(obj)
}

// ShapeResources applies ShapeResource to every item in objects, returning
// the transformed slice. The input slice is reused; callers that need the
// original list intact should clone first.
func ShapeResources(objects []map[string]interface{}) []map[string]interface{} {
	if len(objects) == 0 {
		return objects
	}
	for i, o := range objects {
		objects[i] = ShapeResource(o)
	}
	return objects
}

// lookupKey extracts the (group, kind) shaper key from a resource map. It
// returns ok=false when either field is missing or when kind is empty —
// callers should treat that as "no shaping" rather than failing.
func lookupKey(obj map[string]interface{}) (shaperKey, bool) {
	kind, _ := obj["kind"].(string)
	if kind == "" {
		return shaperKey{}, false
	}
	apiVersion, _ := obj["apiVersion"].(string)
	group := ""
	if i := strings.Index(apiVersion, "/"); i > 0 {
		group = apiVersion[:i]
	}
	return shaperKey{group: group, kind: kind}, true
}

// shapeHelmRelease drops the verbose blobs of a Flux HelmRelease that an
// LLM agent does not need to reason about current state. Specifically:
//
//   - spec.values: the rendered values map. On clusters that inline values
//     (rather than using valuesFrom) this can be the dominant component of
//     the response. The chart source already documents the schema; an LLM
//     diagnosing a HelmRelease almost always wants the conditions, not the
//     values.
//   - status.history: the rollout log. Useful for "why did v3 fail" forensics
//     but noisy for "is this Ready right now". Issue #410 calls it out by
//     name as a target.
//   - status.lastAppliedConfigDigest, lastAttemptedConfigDigest,
//     lastAttemptedRevisionDigest, observedPostRenderersDigest: opaque
//     hashes. lastAttemptedRevisionDigest was added in round 2 of the
//     live-cluster bench — it dominates the residual status bytes on a slim
//     HelmRelease (~70 B) without offering anything an LLM can reason about.
//
// status.conditions, status.lastAttemptedRevision, spec.chart{,Ref},
// spec.valuesFrom (the references), and metadata are all preserved so
// callers can still answer "is this Ready?", "what version?", and "where
// do its values come from?".
func shapeHelmRelease(obj map[string]interface{}) map[string]interface{} {
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		delete(spec, "values")
	}
	if status, ok := obj["status"].(map[string]interface{}); ok {
		delete(status, "history")
		delete(status, "lastAppliedConfigDigest")
		delete(status, "lastAttemptedConfigDigest")
		delete(status, "lastAttemptedRevisionDigest")
		delete(status, "observedPostRenderersDigest")
	}
	return obj
}

// shapeWorkloadTemplate trims the PodTemplate of a Deployment / StatefulSet
// / DaemonSet. Three transformations:
//
//  1. Long container env lists (>envCollapseThreshold) collapse to a flat
//     list of names plus an envCount marker. Names alone are diagnostic
//     ("which env keys exist"); values/valueFrom are bulk and rarely useful.
//  2. PodSpec defaults that are universally diagnostic-noise are dropped:
//     restartPolicy (Always / OnFailure are the only options and the
//     default for these Kinds is Always), terminationGracePeriodSeconds
//     (almost always 30).
//  3. Workload-spec defaults are dropped: progressDeadlineSeconds (always
//     600 unless someone is debugging a stalled rollout — and that
//     scenario is far better served by status.conditions[Progressing]),
//     revisionHistoryLimit (rollout-controller bookkeeping).
//
// Other parts of the template (image, ports, volumeMounts, resources,
// probes, args, command, securityContext, volumes) are kept intact — they
// are routinely needed for diagnosis.
func shapeWorkloadTemplate(obj map[string]interface{}) map[string]interface{} {
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		delete(spec, "progressDeadlineSeconds")
		delete(spec, "revisionHistoryLimit")
		if template, ok := spec["template"].(map[string]interface{}); ok {
			if podSpec, ok := template["spec"].(map[string]interface{}); ok {
				delete(podSpec, "restartPolicy")
				delete(podSpec, "terminationGracePeriodSeconds")
			}
		}
	}
	containers := getContainerSlice(obj)
	if len(containers) == 0 {
		return obj
	}
	for _, raw := range containers {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		collapseContainerEnv(c)
		pruneProbeDefaults(c)
	}
	return obj
}

// getContainerSlice fetches spec.template.spec.containers, or nil if the
// path doesn't resolve to a slice (which is the case for non-template
// kinds and for templates carrying an empty / malformed spec).
func getContainerSlice(obj map[string]interface{}) []interface{} {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	templateSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	containers, _ := templateSpec["containers"].([]interface{})
	return containers
}

// probeDefaults are the API-server-assigned defaults for Probe scalars.
// Dropping these scalars when they match the default removes ~80 B per
// configured probe (round-4 live-cluster bench finding) without losing any
// signal: an explicit `failureThreshold: 3` tells an LLM nothing the
// platform doesn't already imply. Non-default values (e.g. an explicit
// `timeoutSeconds: 5`) are preserved.
//
// initialDelaySeconds defaults to 0 but it's almost always set explicitly
// for HTTP probes; we still prune it when it equals 0 because the JSON
// will print "initialDelaySeconds": 0 unhelpfully.
var probeDefaults = map[string]float64{
	"failureThreshold":              3,
	"successThreshold":              1,
	"periodSeconds":                 10,
	"timeoutSeconds":                1,
	"initialDelaySeconds":           0,
	"terminationGracePeriodSeconds": 30,
}

// pruneProbeDefaults removes well-known default scalars from livenessProbe,
// readinessProbe, and startupProbe on a container. The probe action
// (httpGet / tcpSocket / exec / grpc) and any non-default thresholds are
// preserved — those are the bytes that actually answer "what does this
// probe do?".
func pruneProbeDefaults(c map[string]interface{}) {
	for _, key := range []string{"livenessProbe", "readinessProbe", "startupProbe"} {
		probe, ok := c[key].(map[string]interface{})
		if !ok {
			continue
		}
		for field, def := range probeDefaults {
			if v, ok := probe[field]; ok && asNumber(v) == def {
				delete(probe, field)
			}
		}
	}
}

// asNumber returns v as a float64 when it is a JSON-decoded number, or
// NaN-equivalent (-1) otherwise. Used by pruneProbeDefaults to compare
// any numeric type to the API default without panicking on non-numbers.
func asNumber(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return -1
}

// collapseContainerEnv replaces a container's env with just an envCount
// marker when the list exceeds envCollapseThreshold. Names, values, and
// valueFrom references are dropped because the bytes-per-signal ratio is
// poor: an LLM diagnosing a workload almost always wants to know either
// "is env big?" (count answers it) or "what is variable X set to?" (the
// caller can re-ask with output: normal to see the typed list).
//
// The 0-keep policy was tuned in round 3 of the live-cluster bench: round 2
// kept names as a flat string list, but on a 39-entry Deployment env block
// the names still cost ~900 B of slim response — second only to args.
// Issue #410 calls for "drop spec.template's container env list down to
// a count"; this matches that contract literally.
//
// Below the threshold, env passes through unchanged.
func collapseContainerEnv(c map[string]interface{}) {
	env, ok := c["env"].([]interface{})
	if !ok || len(env) <= envCollapseThreshold {
		return
	}
	delete(c, "env")
	c["envCount"] = len(env)
}
