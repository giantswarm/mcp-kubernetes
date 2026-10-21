package resource

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestTruncateRunes pins the rune-aware truncation contract: byte-length
// based slicing would happily cut a multi-byte rune in half and produce
// invalid UTF-8 in the JSON response. truncateRunes must always return
// well-formed UTF-8 plus a flag indicating whether truncation happened.
func TestTruncateRunes(t *testing.T) {
	t.Run("ascii under cap is unchanged", func(t *testing.T) {
		got, truncated := truncateRunes("hello", 10)
		assert.Equal(t, "hello", got)
		assert.False(t, truncated)
	})

	t.Run("ascii at cap is unchanged", func(t *testing.T) {
		got, truncated := truncateRunes("hello", 5)
		assert.Equal(t, "hello", got)
		assert.False(t, truncated)
	})

	t.Run("ascii over cap is truncated with ellipsis", func(t *testing.T) {
		got, truncated := truncateRunes("hello world", 5)
		assert.Equal(t, "hello…", got)
		assert.True(t, truncated)
	})

	t.Run("multi-byte runes produce valid utf8", func(t *testing.T) {
		// Each "→" is 3 bytes, but one rune.
		s := "policy → fail → validation"
		got, truncated := truncateRunes(s, 8)
		assert.True(t, truncated)
		assert.True(t, utf8.ValidString(got), "truncated output must be valid UTF-8")
		// 8 runes from "policy → fail → validation" = "policy →"
		assert.Equal(t, "policy →…", got)
	})

	t.Run("zero cap on non-empty string truncates to ellipsis", func(t *testing.T) {
		got, truncated := truncateRunes("anything", 0)
		assert.Equal(t, "…", got)
		assert.True(t, truncated)
	})
}

// TestExtractEventInfo_MessageTruncation verifies that extractEventInfo
// honours the rune-aware cap on event.message and only sets
// messageTruncated when the message actually got cut. The kyverno-style
// arrow characters in the input ensure a byte-based truncation would be
// observable as invalid UTF-8.
func TestExtractEventInfo_MessageTruncation(t *testing.T) {
	long := ""
	for utf8.RuneCountInString(long) <= eventMessageMaxRunes {
		long += "policy → fail "
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"type":    "Warning",
		"reason":  "PolicyViolation",
		"message": long,
	}}
	summary := ResourceSummary{Extra: map[string]interface{}{}}

	extractEventInfo(obj, &summary)

	msg, ok := summary.Extra["message"].(string)
	require.True(t, ok, "message must be a string")
	assert.True(t, utf8.ValidString(msg), "truncated message must be valid UTF-8")
	assert.LessOrEqual(t, utf8.RuneCountInString(msg), eventMessageMaxRunes+1, "expected at most maxRunes + ellipsis")
	assert.Equal(t, true, summary.Extra["messageTruncated"], "messageTruncated must be set when truncated")

	short := "Pulled image"
	obj2 := &unstructured.Unstructured{Object: map[string]any{
		"type":    "Normal",
		"message": short,
	}}
	summary2 := ResourceSummary{Extra: map[string]interface{}{}}
	extractEventInfo(obj2, &summary2)
	assert.Equal(t, short, summary2.Extra["message"])
	_, hasTruncatedFlag := summary2.Extra["messageTruncated"]
	assert.False(t, hasTruncatedFlag, "messageTruncated must be absent when message fits")
}

// podSummary builds the compact summary for a pod object the way the list
// handler does, so the tests below exercise the real extraction path rather
// than extractPodInfo in isolation.
func podSummary(t *testing.T, status map[string]any, meta map[string]any) ResourceSummary {
	t.Helper()
	if meta == nil {
		meta = map[string]any{}
	}
	meta["name"] = "test-pod"
	meta["namespace"] = "kube-system"
	obj := &unstructured.Unstructured{Object: map[string]any{
		"kind":       "Pod",
		"apiVersion": "v1",
		"metadata":   meta,
		"status":     status,
	}}
	return summarizeResource(obj, false, false)
}

// TestExtractPodInfo_HuskDiscrimination pins the fields that let a caller
// tell a pod that is failing *right now* from a husk left behind by an
// eviction or a graceful delete. Both report phase Failed / 0-of-N ready, so
// without status.reason and metadata.deletionTimestamp the compact summary
// makes an already-replaced pod look like an active outage.
func TestExtractPodInfo_HuskDiscrimination(t *testing.T) {
	t.Run("evicted pod surfaces its reason", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase":  "Failed",
			"reason": "Evicted",
		}, nil)

		assert.Equal(t, "Failed", summary.Status)
		assert.Equal(t, "Evicted", summary.Extra["reason"])
	})

	t.Run("terminating pod is flagged", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase": "Running",
		}, map[string]any{"deletionTimestamp": "2026-07-29T10:00:00Z"})

		assert.Equal(t, true, summary.Extra["terminating"])
	})

	t.Run("healthy pod gains no husk keys", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{"ready": true, "restartCount": int64(0)},
			},
		}, nil)

		assert.Equal(t, "1/1", summary.Ready)
		for _, key := range []string{"reason", "terminating", "waitingReasons", "lastTerminationReason"} {
			_, present := summary.Extra[key]
			assert.False(t, present, "%s must be absent for a healthy pod", key)
		}
	})
}

// TestExtractPodInfo_WaitingReasons pins the live crashloop signal. A
// crashlooping pod reports phase Running, so status.phase alone cannot
// reveal a restart loop; callers previously had to fall back to BackOff
// events, which outlive the pod they describe and therefore report loops
// that have already been resolved by a replacement pod.
func TestExtractPodInfo_WaitingReasons(t *testing.T) {
	t.Run("crashloop is visible despite phase Running", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{
					"ready":        false,
					"restartCount": int64(7),
					"state": map[string]any{
						"waiting": map[string]any{"reason": "CrashLoopBackOff"},
					},
					"lastState": map[string]any{
						"terminated": map[string]any{"reason": "Error"},
					},
				},
			},
		}, nil)

		assert.Equal(t, "Running", summary.Status)
		assert.Equal(t, "0/1", summary.Ready)
		assert.Equal(t, int64(7), summary.Extra["restarts"])
		assert.Equal(t, []string{"CrashLoopBackOff"}, summary.Extra["waitingReasons"])
		assert.Equal(t, []string{"Error"}, summary.Extra["lastTerminationReason"])
	})

	t.Run("oomkilled root cause survives a restart", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{
					"ready":        true,
					"restartCount": int64(3),
					"lastState": map[string]any{
						"terminated": map[string]any{"reason": "OOMKilled"},
					},
				},
			},
		}, nil)

		assert.Equal(t, []string{"OOMKilled"}, summary.Extra["lastTerminationReason"])
		_, present := summary.Extra["waitingReasons"]
		assert.False(t, present, "a running container has no waiting reason")
	})

	t.Run("reasons are de-duplicated and sorted across containers", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase": "Pending",
			"containerStatuses": []any{
				map[string]any{"ready": false, "state": map[string]any{
					"waiting": map[string]any{"reason": "ImagePullBackOff"},
				}},
				map[string]any{"ready": false, "state": map[string]any{
					"waiting": map[string]any{"reason": "CrashLoopBackOff"},
				}},
				map[string]any{"ready": false, "state": map[string]any{
					"waiting": map[string]any{"reason": "ImagePullBackOff"},
				}},
			},
		}, nil)

		assert.Equal(t, []string{"CrashLoopBackOff", "ImagePullBackOff"}, summary.Extra["waitingReasons"])
		assert.Equal(t, "0/3", summary.Ready)
	})

	t.Run("restarts and ready count are unaffected by the new fields", func(t *testing.T) {
		summary := podSummary(t, map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{"ready": true, "restartCount": int64(2)},
				map[string]any{"ready": false, "restartCount": int64(5)},
			},
		}, nil)

		assert.Equal(t, "1/2", summary.Ready)
		assert.Equal(t, int64(7), summary.Extra["restarts"])
	})
}

// TestSummarizeResource_OwnerAndImages: the compact summary names the
// controller and lists the images of every container field, so a fleet
// scan needs no full manifests.
func TestSummarizeResource_OwnerAndImages(t *testing.T) {
	t.Run("pod names its controller and every image once", func(t *testing.T) {
		pod := createTestWorkload("Pod", "web-5d4f8-abcde", map[string][]string{
			"containers":     {"registry.example/org/web:1.0", "registry.example/org/proxy:2"},
			"initContainers": {"registry.example/org/web:1.0"},
		})
		require.NoError(t, unstructured.SetNestedSlice(pod.Object, []any{
			map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "name": "settings", "uid": "1"},
			map[string]any{"apiVersion": "apps/v1", "kind": "ReplicaSet", "name": "web-5d4f8", "uid": "2", "controller": true},
		}, "metadata", "ownerReferences"))

		summary := summarizeResource(pod, false, false)

		assert.Equal(t, "ReplicaSet/web-5d4f8", summary.Owner)
		assert.Equal(t, []string{"registry.example/org/proxy:2", "registry.example/org/web:1.0"}, summary.Extra["images"])
	})

	t.Run("an owner that does not control is no owner", func(t *testing.T) {
		pod := createTestWorkload("Pod", "standalone", map[string][]string{"containers": {"nginx"}})
		require.NoError(t, unstructured.SetNestedSlice(pod.Object, []any{
			map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "name": "settings", "uid": "1"},
		}, "metadata", "ownerReferences"))

		assert.Empty(t, summarizeResource(pod, false, false).Owner)
	})

	t.Run("cronjob images come from its job template", func(t *testing.T) {
		cron := createTestWorkload("CronJob", "backup", map[string][]string{"containers": {"registry.example/org/backup:4"}})

		assert.Equal(t, []string{"registry.example/org/backup:4"}, summarizeResource(cron, false, false).Extra["images"])
	})

	t.Run("a resource without a PodSpec has no images", func(t *testing.T) {
		svc := createTestWorkload("Service", "web", map[string][]string{"containers": {"nginx"}})

		summary := summarizeResource(svc, false, false)

		_, present := summary.Extra["images"]
		assert.False(t, present)
		assert.Empty(t, summary.Owner)
	})
}
