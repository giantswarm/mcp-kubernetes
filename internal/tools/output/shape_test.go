package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShapeResource_HelmRelease pins the per-Kind shaping for Flux
// HelmReleases: the verbose blobs (spec.values, status.history,
// digest fields) must go, while the diagnostic surface (status.conditions,
// status.lastAttemptedRevision, spec.chartRef, spec.valuesFrom) must stay.
func TestShapeResource_HelmRelease(t *testing.T) {
	in := map[string]interface{}{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]interface{}{"name": "backstage", "namespace": "flux-giantswarm"},
		"spec": map[string]interface{}{
			"chartRef":   map[string]interface{}{"kind": "OCIRepository", "name": "backstage"},
			"valuesFrom": []interface{}{map[string]interface{}{"kind": "ConfigMap", "name": "backstage-values"}},
			"values":     map[string]interface{}{"image": map[string]interface{}{"tag": "v1.2.3"}, "replicas": 3, "env": []interface{}{"FOO=bar"}},
			"interval":   "10m",
			"timeout":    "5m",
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True"}},
			"history": []interface{}{
				map[string]interface{}{"revision": "v292", "status": "Succeeded"},
				map[string]interface{}{"revision": "v291", "status": "Succeeded"},
			},
			"lastAttemptedRevision":       "0.128.0+631280c48495",
			"lastAppliedConfigDigest":     "sha256:abc",
			"lastAttemptedConfigDigest":   "sha256:def",
			"observedPostRenderersDigest": "sha256:ghi",
			"observedGeneration":          int64(7),
		},
	}

	got := ShapeResource(in)

	spec := got["spec"].(map[string]interface{})
	_, hasValues := spec["values"]
	assert.False(t, hasValues, "spec.values must be dropped on slim shaping")
	assert.NotNil(t, spec["chartRef"], "spec.chartRef must be preserved (used for diagnosis)")
	assert.NotNil(t, spec["valuesFrom"], "spec.valuesFrom references must be preserved")
	assert.Equal(t, "10m", spec["interval"], "scalar spec fields must be preserved")

	status := got["status"].(map[string]interface{})
	_, hasHistory := status["history"]
	assert.False(t, hasHistory, "status.history must be dropped on slim shaping")
	for _, k := range []string{"lastAppliedConfigDigest", "lastAttemptedConfigDigest", "observedPostRenderersDigest"} {
		_, has := status[k]
		assert.Falsef(t, has, "status.%s digest must be dropped", k)
	}
	assert.NotNil(t, status["conditions"], "status.conditions must be preserved (Ready/Stalled diagnosis)")
	assert.Equal(t, "0.128.0+631280c48495", status["lastAttemptedRevision"], "lastAttemptedRevision must be preserved (version diagnosis)")
}

// TestShapeResource_DeploymentEnvCollapse pins the workload-template shaper:
// long container env walls (>envCollapseThreshold) collapse to a name-only
// summary plus envCount. Short env lists pass through unchanged.
func TestShapeResource_DeploymentEnvCollapse(t *testing.T) {
	makeEnv := func(n int) []interface{} {
		out := make([]interface{}, n)
		for i := 0; i < n; i++ {
			out[i] = map[string]interface{}{
				"name":  "VAR_" + string(rune('A'+i%26)),
				"value": "some-bulky-value-the-llm-rarely-needs",
			}
		}
		return out
	}

	tests := []struct {
		name      string
		envCount  int
		wantCount int  // expected envCount key, 0 means absent
		wantEnv   bool // true: env array kept; false: env field deleted
	}{
		{name: "short env stays intact", envCount: 4, wantCount: 0, wantEnv: true},
		{name: "exactly threshold stays intact", envCount: envCollapseThreshold, wantCount: 0, wantEnv: true},
		{name: "above threshold collapses to count only", envCount: envCollapseThreshold + 1, wantCount: envCollapseThreshold + 1, wantEnv: false},
		{name: "very long env collapses to count only", envCount: 39, wantCount: 39, wantEnv: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "app",
									"image": "ghcr.io/giantswarm/foo:1.0",
									"env":   makeEnv(tt.envCount),
								},
							},
						},
					},
				},
			}

			got := ShapeResource(in)
			containers := got["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
			c := containers[0].(map[string]interface{})

			if tt.wantCount == 0 {
				_, hasCount := c["envCount"]
				assert.False(t, hasCount, "envCount must not be added when env is below threshold")
			} else {
				assert.Equal(t, tt.wantCount, c["envCount"], "envCount must record the original env length")
			}

			if tt.wantEnv {
				env, ok := c["env"].([]interface{})
				require.True(t, ok, "below threshold env must remain present")
				require.Len(t, env, tt.envCount)
				first, ok := env[0].(map[string]interface{})
				require.True(t, ok, "below threshold env entries must remain typed maps")
				assert.NotEmpty(t, first["name"])
				_, hasValue := first["value"]
				assert.True(t, hasValue, "below threshold env values are preserved")
			} else {
				_, hasEnv := c["env"]
				assert.False(t, hasEnv, "above threshold env must be removed entirely; only envCount remains")
			}

			assert.Equal(t, "ghcr.io/giantswarm/foo:1.0", c["image"], "image must always be preserved")
		})
	}
}

// TestShapeResource_AppliesToInitAndEphemeralContainers pins that the
// workload shaper walks initContainers and ephemeralContainers in addition
// to the primary containers slice. Sidecar-heavy apps commonly run init
// containers with the same long env walls as the main container, and
// ephemeral debug containers can also carry a non-default env list.
func TestShapeResource_AppliesToInitAndEphemeralContainers(t *testing.T) {
	makeBigEnv := func(prefix string) []interface{} {
		out := make([]interface{}, envCollapseThreshold+5)
		for i := range out {
			out[i] = map[string]interface{}{
				"name":  prefix + "_VAR",
				"value": "bulk-value",
			}
		}
		return out
	}

	in := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "app", "env": makeBigEnv("APP")},
					},
					"initContainers": []interface{}{
						map[string]interface{}{"name": "init", "env": makeBigEnv("INIT")},
					},
					"ephemeralContainers": []interface{}{
						map[string]interface{}{"name": "debug", "env": makeBigEnv("DEBUG")},
					},
				},
			},
		},
	}

	got := ShapeResource(in)
	podSpec := got["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})

	for _, key := range []string{"containers", "initContainers", "ephemeralContainers"} {
		slice, ok := podSpec[key].([]interface{})
		require.True(t, ok, "%s slice must remain present", key)
		require.Len(t, slice, 1)
		c := slice[0].(map[string]interface{})

		_, hasEnv := c["env"]
		assert.False(t, hasEnv, "%s[0].env must be collapsed when over threshold", key)
		assert.Equal(t, envCollapseThreshold+5, c["envCount"], "%s[0].envCount must record original length", key)
	}
}

// TestShapeResource_DeploymentDefaultsStripped pins the round-2 strips on
// the workload shaper: spec.progressDeadlineSeconds, spec.revisionHistoryLimit,
// spec.template.spec.restartPolicy, and spec.template.spec.terminationGracePeriodSeconds
// are noisy defaults that get dropped under output: slim. They must stay
// present under output: normal (covered by TestProcessor_KindShapingFollowsSlim).
func TestShapeResource_DeploymentDefaultsStripped(t *testing.T) {
	in := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"spec": map[string]interface{}{
			"progressDeadlineSeconds": int64(600),
			"revisionHistoryLimit":    int64(10),
			"replicas":                int64(3),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"restartPolicy":                 "Always",
					"terminationGracePeriodSeconds": int64(30),
					"serviceAccountName":            "app",
					"containers": []interface{}{
						map[string]interface{}{"name": "app", "image": "x:1"},
					},
				},
			},
		},
	}

	got := ShapeResource(in)
	spec := got["spec"].(map[string]interface{})
	for _, k := range []string{"progressDeadlineSeconds", "revisionHistoryLimit"} {
		_, has := spec[k]
		assert.Falsef(t, has, "spec.%s must be dropped on slim shaping", k)
	}
	assert.Equal(t, int64(3), spec["replicas"], "spec.replicas must be preserved")

	podSpec := spec["template"].(map[string]interface{})["spec"].(map[string]interface{})
	for _, k := range []string{"restartPolicy", "terminationGracePeriodSeconds"} {
		_, has := podSpec[k]
		assert.Falsef(t, has, "spec.template.spec.%s must be dropped on slim shaping", k)
	}
	assert.Equal(t, "app", podSpec["serviceAccountName"], "serviceAccountName must be preserved")
}

// TestShapeResource_UnknownKindIsPassthrough makes sure a Kind without a
// registered shaper is returned unchanged — the registry must be opt-in,
// not a stealth allowlist.
func TestShapeResource_UnknownKindIsPassthrough(t *testing.T) {
	in := map[string]interface{}{
		"apiVersion": "example.com/v1",
		"kind":       "MadeUpKind",
		"spec":       map[string]interface{}{"values": map[string]interface{}{"keep": "me"}},
		"status":     map[string]interface{}{"history": []interface{}{"do-not-drop"}},
	}

	got := ShapeResource(in)

	assert.NotNil(t, got["spec"].(map[string]interface{})["values"], "unknown Kinds must not have spec.values touched")
	assert.NotNil(t, got["status"].(map[string]interface{})["history"], "unknown Kinds must not have status.history touched")
}

// TestShapeResource_NilAndMissingKind covers the early-return paths so a
// shaper bug can never crash the read tools on malformed input.
func TestShapeResource_NilAndMissingKind(t *testing.T) {
	assert.Nil(t, ShapeResource(nil), "nil in -> nil out")

	missingKind := map[string]interface{}{"apiVersion": "apps/v1", "spec": map[string]interface{}{}}
	got := ShapeResource(missingKind)
	assert.Equal(t, missingKind, got, "missing kind -> passthrough")
}

// TestProcessor_KindShapingFollowsSlim pins the contract that KindShaping
// only fires when SlimOutput is on. A "wide" output (SlimOutput=false)
// must never silently drop spec.values via per-Kind shaping.
func TestProcessor_KindShapingFollowsSlim(t *testing.T) {
	hr := func() map[string]interface{} {
		return map[string]interface{}{
			"apiVersion": "helm.toolkit.fluxcd.io/v2",
			"kind":       "HelmRelease",
			"spec":       map[string]interface{}{"values": map[string]interface{}{"big": "blob"}},
			"status":     map[string]interface{}{"history": []interface{}{"v1", "v2"}},
		}
	}

	t.Run("wide preserves everything", func(t *testing.T) {
		p := NewProcessor(&Config{SlimOutput: false, KindShaping: true, MaskSecrets: false})
		out := p.ProcessSingle(hr())
		assert.NotNil(t, out["spec"].(map[string]interface{})["values"], "spec.values must survive output: wide even with KindShaping=true")
		assert.NotNil(t, out["status"].(map[string]interface{})["history"], "status.history must survive output: wide")
	})

	t.Run("normal preserves Kind-specific blobs", func(t *testing.T) {
		p := NewProcessor(&Config{SlimOutput: true, KindShaping: false, MaskSecrets: false})
		out := p.ProcessSingle(hr())
		assert.NotNil(t, out["spec"].(map[string]interface{})["values"], "spec.values must survive output: normal (no Kind shaping)")
		assert.NotNil(t, out["status"].(map[string]interface{})["history"], "status.history must survive output: normal")
	})

	t.Run("slim drops Kind-specific blobs", func(t *testing.T) {
		p := NewProcessor(&Config{SlimOutput: true, KindShaping: true, MaskSecrets: false})
		out := p.ProcessSingle(hr())
		_, hasValues := out["spec"].(map[string]interface{})["values"]
		assert.False(t, hasValues, "spec.values must be dropped on output: slim")
		_, hasHistory := out["status"].(map[string]interface{})["history"]
		assert.False(t, hasHistory, "status.history must be dropped on output: slim")
	})
}
