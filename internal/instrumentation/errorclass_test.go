package instrumentation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"syscall"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// errTestOperation is a failed operation without a recognisable cause.
var errTestOperation = errors.New("test operation failed")

func TestErrorClass(t *testing.T) {
	pods := schema.GroupResource{Resource: "pods"}
	refused := &url.Error{Op: "Get", URL: "https://10.0.0.1:6443/api", Err: &net.OpError{
		Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED),
	}}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"forbidden", apierrors.NewForbidden(pods, "", errors.New("RBAC")), ErrorClassForbidden},
		{"wrapped forbidden", fmt.Errorf("failed to list pods: %w", apierrors.NewForbidden(pods, "", errors.New("RBAC"))), ErrorClassForbidden},
		{"unauthorized", apierrors.NewUnauthorized("token expired"), ErrorClassUnauthorized},
		{"not found", apierrors.NewNotFound(pods, "web-0"), ErrorClassNotFound},
		{"expired", apierrors.NewResourceExpired("continue token expired"), ErrorClassNotFound},
		{"unknown resource type", &meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{Resource: "widgets"}}, ErrorClassNotFound},
		{"conflict", apierrors.NewConflict(pods, "web-0", errors.New("stale")), ErrorClassConflict},
		{"already exists", apierrors.NewAlreadyExists(pods, "web-0"), ErrorClassConflict},
		{"invalid", apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "web-0", nil), ErrorClassInvalid},
		{"bad request", apierrors.NewBadRequest("bad selector"), ErrorClassInvalid},
		{"method not supported", apierrors.NewMethodNotSupported(pods, "patch"), ErrorClassInvalid},
		{"canceled", fmt.Errorf("list: %w", context.Canceled), ErrorClassCanceled},
		{"api timeout", apierrors.NewTimeoutError("took too long", 1), ErrorClassTimeout},
		{"server timeout", apierrors.NewServerTimeout(pods, "list", 1), ErrorClassTimeout},
		{"deadline exceeded", fmt.Errorf("discovery: %w", context.DeadlineExceeded), ErrorClassTimeout},
		{"service unavailable", apierrors.NewServiceUnavailable("etcd down"), ErrorClassUnavailable},
		{"too many requests", apierrors.NewTooManyRequests("throttled", 1), ErrorClassUnavailable},
		{"connection refused", refused, ErrorClassUnavailable},
		{"dns", &net.DNSError{Err: "no such host", Name: "api.example"}, ErrorClassUnavailable},
		{"internal error", apierrors.NewInternalError(errors.New("boom")), ErrorClassServerError},
		{"other 5xx", apierrors.NewGenericServerResponse(502, "get", pods, "web-0", "bad gateway", 0, false), ErrorClassServerError},
		{"plain error", errors.New("something odd"), ErrorClassOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorClass(tt.err); got != tt.want {
				t.Errorf("ErrorClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestErrorClass_Bounded pins the label's value set: any error, whatever its
// text, maps to one of ErrorClasses.
func TestErrorClass_Bounded(t *testing.T) {
	if len(ErrorClasses) != 10 {
		t.Errorf("ErrorClasses has %d values, want 10: %v", len(ErrorClasses), ErrorClasses)
	}
	for _, err := range []error{
		errors.New("pods is forbidden: User \"jane@example.com\" cannot list resource \"pods\""),
		fmt.Errorf("unique %d", 42),
		apierrors.NewGenericServerResponse(418, "get", schema.GroupResource{}, "", "teapot", 0, false),
	} {
		if class := ErrorClass(err); !slices.Contains(ErrorClasses, class) {
			t.Errorf("ErrorClass(%v) = %q, not in ErrorClasses", err, class)
		}
	}
}

// TestRecordK8sOperation_ErrorClassLabel pins what the failure-rate alerts
// select on: a denied call carries status="error" and error_class="forbidden",
// a successful call status="success" and no error_class at all.
func TestRecordK8sOperation_ErrorClassLabel(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	metrics, err := NewMetrics(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), false)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	ctx := context.Background()
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("RBAC"))
	metrics.RecordK8sOperation(ctx, "", OperationList, "pods", "default", denied, 0)
	metrics.RecordK8sOperation(ctx, "", OperationList, "pods", "default", nil, 0)
	metrics.RecordClusterOperation(ctx, "prod-wc-01", OperationGet, apierrors.NewServiceUnavailable("down"), 0)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	got := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "mcp_kubernetes_operations_total" {
				continue
			}
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				status, _ := dp.Attributes.Value(attribute.Key(attrStatus))
				class, hasClass := dp.Attributes.Value(attribute.Key(attrErrorClass))
				key := status.AsString()
				if hasClass {
					key += "/" + class.AsString()
				}
				got[key] = true
			}
		}
	}

	for _, want := range []string{"error/forbidden", "success", "error/unavailable"} {
		if !got[want] {
			t.Errorf("mcp_kubernetes_operations_total has no series %q; got %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("want exactly 3 series, got %v", got)
	}
}
