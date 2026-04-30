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
