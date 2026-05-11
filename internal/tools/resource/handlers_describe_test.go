package resource

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// makeEvent builds a corev1.Event whose ObjectMeta carries managedFields, so
// every test exercises the slim path.
func makeEvent(name string, last time.Time) corev1.Event {
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet", Operation: metav1.ManagedFieldsOperationUpdate},
			},
		},
		LastTimestamp: metav1.NewTime(last),
		Reason:        "Synthetic",
		Type:          corev1.EventTypeNormal,
	}
}

// makeEvents returns n events with strictly increasing LastTimestamp so a
// stable sort newest-first yields the input order reversed.
func makeEvents(n int) []corev1.Event {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := make([]corev1.Event, 0, n)
	for i := range n {
		events = append(events, makeEvent("ev-"+strconv.Itoa(i), base.Add(time.Duration(i)*time.Minute)))
	}
	return events
}

func TestBuildDescribeOutput_DefaultsApplied(t *testing.T) {
	events := makeEvents(60)

	out := buildDescribeOutput(nil, nil, nil, events, DefaultEventsLimit)

	assert.Equal(t, 60, out.TotalEvents)
	assert.Equal(t, DefaultEventsLimit, out.ReturnedEvents)
	assert.True(t, out.EventsTruncated)
	assert.Len(t, out.Events, DefaultEventsLimit)
}

func TestBuildDescribeOutput_OverrideLimit(t *testing.T) {
	events := makeEvents(10)

	out := buildDescribeOutput(nil, nil, nil, events, 5)

	assert.Equal(t, 10, out.TotalEvents)
	assert.Equal(t, 5, out.ReturnedEvents)
	assert.True(t, out.EventsTruncated)
}

func TestBuildDescribeOutput_NoTruncation(t *testing.T) {
	events := makeEvents(5)

	out := buildDescribeOutput(nil, nil, nil, events, DefaultEventsLimit)

	assert.Equal(t, 5, out.TotalEvents)
	assert.Equal(t, 5, out.ReturnedEvents)
	assert.False(t, out.EventsTruncated, "eventsTruncated must be false when total <= limit")
}

func TestBuildDescribeOutput_LimitEqualsTotal(t *testing.T) {
	events := makeEvents(10)

	out := buildDescribeOutput(nil, nil, nil, events, 10)

	assert.Equal(t, 10, out.TotalEvents)
	assert.Equal(t, 10, out.ReturnedEvents)
	assert.False(t, out.EventsTruncated, "eventsTruncated must be false when limit == total")
}

func TestBuildDescribeOutput_SortsNewestFirst(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Insert in arbitrary order; expect newest LastTimestamp first.
	events := []corev1.Event{
		makeEvent("middle", base.Add(5*time.Minute)),
		makeEvent("oldest", base),
		makeEvent("newest", base.Add(10*time.Minute)),
	}

	out := buildDescribeOutput(nil, nil, nil, events, 10)

	require.Len(t, out.Events, 3)
	assert.Equal(t, "newest", out.Events[0]["metadata"].(map[string]any)["name"])
	assert.Equal(t, "middle", out.Events[1]["metadata"].(map[string]any)["name"])
	assert.Equal(t, "oldest", out.Events[2]["metadata"].(map[string]any)["name"])
}

func TestBuildDescribeOutput_SortFallsBackToEventTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Both events lack LastTimestamp; sort must fall through to EventTime.
	older := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "older"},
		EventTime:  metav1.NewMicroTime(base),
	}
	newer := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "newer"},
		EventTime:  metav1.NewMicroTime(base.Add(time.Hour)),
	}

	out := buildDescribeOutput(nil, nil, nil, []corev1.Event{older, newer}, 10)

	require.Len(t, out.Events, 2)
	assert.Equal(t, "newer", out.Events[0]["metadata"].(map[string]any)["name"])
	assert.Equal(t, "older", out.Events[1]["metadata"].(map[string]any)["name"])
}

func TestBuildDescribeOutput_SortFallsBackToFirstTimestamp(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Both events lack LastTimestamp and EventTime; sort must fall through
	// to FirstTimestamp (the most common timestamp on classic core/v1 events).
	older := corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "older"},
		FirstTimestamp: metav1.NewTime(base),
	}
	newer := corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "newer"},
		FirstTimestamp: metav1.NewTime(base.Add(time.Hour)),
	}

	out := buildDescribeOutput(nil, nil, nil, []corev1.Event{older, newer}, 10)

	require.Len(t, out.Events, 2)
	assert.Equal(t, "newer", out.Events[0]["metadata"].(map[string]any)["name"])
	assert.Equal(t, "older", out.Events[1]["metadata"].(map[string]any)["name"])
}

func TestBuildDescribeOutput_StripsManagedFields(t *testing.T) {
	events := makeEvents(1)
	require.NotEmpty(t, events[0].ManagedFields, "precondition: input event must carry managedFields")

	out := buildDescribeOutput(nil, nil, nil, events, 10)

	require.Len(t, out.Events, 1)
	meta, ok := out.Events[0]["metadata"].(map[string]any)
	require.True(t, ok)
	_, hasManagedFields := meta["managedFields"]
	assert.False(t, hasManagedFields, "metadata.managedFields must be stripped from each event")
}

// TestBuildDescribeOutput_StripsPerEventBookkeeping pins the round-1 / round-3
// optimisations: per-event uid, resourceVersion, creationTimestamp, namespace,
// involvedObject.{uid,resourceVersion,apiVersion} and reportingInstance must
// be stripped to save ~340 bytes per event on busy controllers like cilium.
// eventTime is intentionally NOT stripped — kyverno-style events use it as
// their only timestamp.
func TestBuildDescribeOutput_StripsPerEventBookkeeping(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ev := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ev-1",
			Namespace:       "default",
			UID:             "ev-uid",
			ResourceVersion: "12345",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet", Operation: metav1.ManagedFieldsOperationUpdate},
			},
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion:      "apps/v1",
			Kind:            "DaemonSet",
			Name:            "cilium",
			Namespace:       "kube-system",
			UID:             "obj-uid",
			ResourceVersion: "67890",
		},
		EventTime:         metav1.NewMicroTime(now),
		ReportingInstance: "",
		Reason:            "PolicyViolation",
		Type:              corev1.EventTypeWarning,
	}

	out := buildDescribeOutput(nil, nil, nil, []corev1.Event{ev}, 10)
	require.Len(t, out.Events, 1)
	got := out.Events[0]

	meta, _ := got["metadata"].(map[string]any)
	for _, k := range []string{"uid", "resourceVersion", "creationTimestamp", "namespace", "selfLink", "managedFields"} {
		_, has := meta[k]
		assert.Falsef(t, has, "metadata.%s must be stripped from event", k)
	}
	assert.Equal(t, "ev-1", meta["name"], "event metadata.name must be preserved as the unique event id")

	involved, _ := got["involvedObject"].(map[string]any)
	for _, k := range []string{"uid", "resourceVersion", "apiVersion"} {
		_, has := involved[k]
		assert.Falsef(t, has, "involvedObject.%s must be stripped from event", k)
	}
	assert.Equal(t, "DaemonSet", involved["kind"])
	assert.Equal(t, "cilium", involved["name"])
	assert.Equal(t, "kube-system", involved["namespace"])

	_, hasReportingInstance := got["reportingInstance"]
	assert.False(t, hasReportingInstance, "reportingInstance must be stripped (typically empty)")

	_, hasEventTime := got["eventTime"]
	assert.True(t, hasEventTime, "eventTime must be preserved — kyverno-style events have no other timestamp")
}

func TestBuildDescribeOutput_NoEvents(t *testing.T) {
	out := buildDescribeOutput(nil, nil, nil, nil, DefaultEventsLimit)

	assert.Equal(t, 0, out.TotalEvents)
	assert.Equal(t, 0, out.ReturnedEvents)
	assert.False(t, out.EventsTruncated)
	require.NotNil(t, out.Events, "events should be a non-nil empty slice for stable shape")
	assert.Empty(t, out.Events)
}

func TestBuildDescribeOutput_AlwaysEmitsCountFields(t *testing.T) {
	// totalEvents, returnedEvents, and eventsTruncated must always be present
	// in the marshaled output, even at zero/false values, so callers can rely
	// on a stable wire shape.
	out := buildDescribeOutput(nil, nil, nil, nil, DefaultEventsLimit)

	raw, err := json.Marshal(out)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, field := range []string{"totalEvents", "returnedEvents", "eventsTruncated"} {
		_, present := decoded[field]
		assert.True(t, present, "%q must be present in the marshaled output", field)
	}
	assert.Equal(t, false, decoded["eventsTruncated"])
}

// TestDescribeResource_ConvenienceMetadataIsSlimmed pins the round-2
// optimisation: the convenience metadata map duplicates resource.metadata.X
// fields and previously was returned untouched. We now slim it (uid /
// resourceVersion / managedFields / etc) to keep the duplicate from
// re-introducing the bytes we just stripped from resource.metadata.
func TestDescribeResource_ConvenienceMetadataIsSlimmed(t *testing.T) {
	mf := []map[string]any{{"manager": "kubectl"}}
	annotations := map[string]any{
		"kubectl.kubernetes.io/last-applied-configuration": "{...long blob...}",
		"keep.me": "yes",
	}

	in := map[string]any{
		"uid":             "abc-123",
		"resourceVersion": "99999",
		"managedFields":   mf,
		"annotations":     annotations,
		"labels":          map[string]any{"app": "demo"},
		"kind":            "Deployment",
		"apiVersion":      "apps/v1",
	}
	processor := getOutputProcessorForFormat(serverContextWithSlim(t), "slim")
	cfg := processor.Config()
	require.True(t, cfg.SlimOutput, "precondition: SlimOutput must be on")

	// Exercise the same helper handleDescribeResource uses on the
	// convenience metadata map.
	got := slimMetadataMap(in, cfg.ExcludedFields)
	require.NotNil(t, got)

	for _, stripped := range []string{"uid", "resourceVersion", "managedFields"} {
		_, has := got[stripped]
		assert.Falsef(t, has, "convenience metadata.%s must be slimmed", stripped)
	}

	// Annotations are kept but the last-applied-configuration entry is dropped.
	keptAnn, _ := got["annotations"].(map[string]any)
	require.NotNil(t, keptAnn)
	_, hasLAC := keptAnn["kubectl.kubernetes.io/last-applied-configuration"]
	assert.False(t, hasLAC, "last-applied-configuration must be slimmed from convenience metadata.annotations")
	assert.Equal(t, "yes", keptAnn["keep.me"], "non-bookkeeping annotations must be preserved")

	// Labels and kind/apiVersion remain — they're useful diagnostic fields.
	assert.Equal(t, "Deployment", got["kind"])
	assert.NotNil(t, got["labels"])
}

func serverContextWithSlim(t *testing.T) *server.ServerContext {
	t.Helper()
	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)
	return sc
}

func TestDescribeResource_EventsLimitOutOfRangeRejected(t *testing.T) {
	ctx := context.Background()

	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(&testdata.MockK8sClient{}),
		server.WithLogger(&testdata.MockLogger{}),
	)
	require.NoError(t, err)

	for _, val := range []float64{0, -1, float64(MaxEventsLimit + 1)} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"resourceType": "pods",
			"name":         "my-pod",
			"eventsLimit":  val,
		}

		result, err := handleDescribeResource(ctx, req, sc)
		require.NoError(t, err)
		require.True(t, result.IsError, "eventsLimit=%v must be rejected", val)
		assert.Contains(t, getErrorText(t, result), "eventsLimit must be between")
	}
}
