package resource

import (
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
)

// Default and maximum values for the kubernetes_describe eventsLimit param.
const (
	// DefaultEventsLimit is the default cap on the number of events returned
	// by kubernetes_describe.
	DefaultEventsLimit = 50

	// MaxEventsLimit is the absolute maximum allowed for eventsLimit.
	MaxEventsLimit = 1000
)

// DescribeOutput is the response shape for kubernetes_describe.
//
// The structure mirrors the original map-based response: Resource, Metadata,
// and Meta come from k8s.ResourceDescription, while Events is the slimmed,
// sorted, and truncated event list. TotalEvents/ReturnedEvents/EventsTruncated
// let callers detect that the event history was clipped.
type DescribeOutput struct {
	// Resource is the slimmed, masked top-level resource object.
	Resource any `json:"resource"`

	// Metadata is the convenience metadata map populated by the k8s layer
	// (kind, apiVersion, labels, annotations, etc.). Omitted when empty.
	Metadata map[string]any `json:"metadata,omitempty"`

	// Meta carries operation transparency info (resource scope, hints).
	Meta *k8s.ResponseMeta `json:"_meta,omitempty"`

	// Events contains the per-event maps after sort+truncate+slim. Always
	// emitted (possibly empty) so callers see a stable shape.
	Events []map[string]any `json:"events"`

	// TotalEvents is the total number of events for the object before
	// truncation.
	TotalEvents int `json:"totalEvents"`

	// ReturnedEvents is the number of events included in this response.
	ReturnedEvents int `json:"returnedEvents"`

	// EventsTruncated is true when the event list was clipped (by eventsLimit
	// or otherwise reduced from TotalEvents). Always emitted so callers can
	// rely on a stable response shape.
	EventsTruncated bool `json:"eventsTruncated"`
}
