package instrumentation

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// mockMeterProvider creates a simple meter for testing
func mockMeterProvider() metric.Meter {
	provider := sdkmetric.NewMeterProvider()
	return provider.Meter("test")
}

func TestNewMetrics(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false) // false = no detailed labels
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	if metrics == nil {
		t.Fatal("expected metrics to be non-nil")
	}

	// Verify all metrics are initialized (non-nil)
	if metrics.httpRequestsTotal == nil {
		t.Error("expected httpRequestsTotal to be initialized")
	}
	if metrics.httpRequestDuration == nil {
		t.Error("expected httpRequestDuration to be initialized")
	}
	if metrics.activeSessions == nil {
		t.Error("expected activeSessions to be initialized")
	}
	if metrics.k8sOperationsTotal == nil {
		t.Error("expected k8sOperationsTotal to be initialized")
	}
	if metrics.k8sOperationDuration == nil {
		t.Error("expected k8sOperationDuration to be initialized")
	}
	if metrics.k8sPodOperationsTotal == nil {
		t.Error("expected k8sPodOperationsTotal to be initialized")
	}
	if metrics.k8sPodOperationDuration == nil {
		t.Error("expected k8sPodOperationDuration to be initialized")
	}
	if metrics.oauthDownstreamAuthTotal == nil {
		t.Error("expected oauthDownstreamAuthTotal to be initialized")
	}

	// Verify detailedLabels is set correctly
	if metrics.detailedLabels != false {
		t.Error("expected detailedLabels to be false")
	}
}

func TestNewMetrics_DetailedLabels(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, true) // true = detailed labels
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	if metrics.detailedLabels != true {
		t.Error("expected detailedLabels to be true")
	}
}

func TestMetrics_RecordHTTPRequest(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	metrics.RecordHTTPRequest(ctx, "POST", "/mcp", 200, 100*time.Millisecond)

	// Test with different status codes
	metrics.RecordHTTPRequest(ctx, "GET", "/metrics", 200, 50*time.Millisecond)
	metrics.RecordHTTPRequest(ctx, "POST", "/mcp", 500, 200*time.Millisecond)

	// If we got here without panic, the test passes
	// (metrics are recorded but we don't have easy access to verify the values in this setup)
}

func TestMetrics_RecordHTTPRequest_NilMetrics(t *testing.T) {
	// Test that recording with nil metrics doesn't panic
	metrics := &Metrics{}
	ctx := context.Background()

	// Should not panic
	metrics.RecordHTTPRequest(ctx, "POST", "/mcp", 200, 100*time.Millisecond)
}

func TestMetrics_RecordK8sOperation(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	metrics.RecordK8sOperation(ctx, OperationGet, "pods", "default", StatusSuccess, 50*time.Millisecond)
	metrics.RecordK8sOperation(ctx, OperationList, "deployments", "kube-system", StatusSuccess, 100*time.Millisecond)
	metrics.RecordK8sOperation(ctx, OperationDelete, "pods", "default", StatusError, 75*time.Millisecond)
}

func TestMetrics_RecordK8sOperation_NilMetrics(t *testing.T) {
	metrics := &Metrics{}
	ctx := context.Background()

	// Should not panic
	metrics.RecordK8sOperation(ctx, OperationGet, "pods", "default", StatusSuccess, 50*time.Millisecond)
}

func TestMetrics_RecordPodOperation(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	metrics.RecordPodOperation(ctx, OperationLogs, "default", StatusSuccess, 100*time.Millisecond)
	metrics.RecordPodOperation(ctx, OperationExec, "kube-system", StatusSuccess, 200*time.Millisecond)
}

func TestMetrics_RecordPodOperation_NilMetrics(t *testing.T) {
	metrics := &Metrics{}
	ctx := context.Background()

	// Should not panic
	metrics.RecordPodOperation(ctx, OperationLogs, "default", StatusSuccess, 100*time.Millisecond)
}

func TestMetrics_RecordOAuthDownstreamAuth(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	metrics.RecordOAuthDownstreamAuth(ctx, OAuthResultSuccess)
	metrics.RecordOAuthDownstreamAuth(ctx, OAuthResultFallback)
	metrics.RecordOAuthDownstreamAuth(ctx, OAuthResultFailure)
}

func TestMetrics_RecordOAuthDownstreamAuth_NilMetrics(t *testing.T) {
	metrics := &Metrics{}
	ctx := context.Background()

	// Should not panic
	metrics.RecordOAuthDownstreamAuth(ctx, OAuthResultSuccess)
}

func TestMetrics_ActiveSessions(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()

	// Increment sessions
	metrics.IncrementActiveSessions(ctx)
	metrics.IncrementActiveSessions(ctx)
	metrics.IncrementActiveSessions(ctx)

	// Decrement sessions
	metrics.DecrementActiveSessions(ctx)
	metrics.DecrementActiveSessions(ctx)

	// Final count should be 1, but we can't easily verify this in unit tests
	// The important thing is that it doesn't panic
}

func TestMetrics_ActiveSessions_NilMetrics(t *testing.T) {
	metrics := &Metrics{}
	ctx := context.Background()

	// Should not panic
	metrics.IncrementActiveSessions(ctx)
	metrics.DecrementActiveSessions(ctx)
}

func TestMetricConstants(t *testing.T) {
	// Test that metric constants are defined
	if StatusSuccess == "" {
		t.Error("StatusSuccess should not be empty")
	}
	if StatusError == "" {
		t.Error("StatusError should not be empty")
	}
	if OAuthResultSuccess == "" {
		t.Error("OAuthResultSuccess should not be empty")
	}
	if OAuthResultFallback == "" {
		t.Error("OAuthResultFallback should not be empty")
	}
	if OAuthResultFailure == "" {
		t.Error("OAuthResultFailure should not be empty")
	}

	// Verify operation constants
	operations := []string{
		OperationGet,
		OperationList,
		OperationCreate,
		OperationApply,
		OperationDelete,
		OperationPatch,
		OperationLogs,
		OperationExec,
		OperationWatch,
	}

	for _, op := range operations {
		if op == "" {
			t.Errorf("operation constant should not be empty")
		}
	}
}

func TestMetrics_ConcurrentHTTPRecording(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			method := "GET"
			if id%2 == 0 {
				method = "POST"
			}
			statusCode := 200
			if id%3 == 0 {
				statusCode = 500
			}
			metrics.RecordHTTPRequest(ctx, method, "/test", statusCode, 10*time.Millisecond)
		}(i)
	}

	wg.Wait()
	// If we got here without panic or race conditions, the test passes
}

func TestMetrics_ConcurrentK8sOperationRecording(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			operation := OperationGet
			if id%2 == 0 {
				operation = OperationList
			}
			namespace := "default"
			if id%3 == 0 {
				namespace = "kube-system"
			}
			metrics.RecordK8sOperation(ctx, operation, "pods", namespace, StatusSuccess, 50*time.Millisecond)
		}(i)
	}

	wg.Wait()
}

func TestMetrics_ConcurrentPodOperationRecording(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			operation := OperationLogs
			if id%2 == 0 {
				operation = OperationExec
			}
			metrics.RecordPodOperation(ctx, operation, "default", StatusSuccess, 100*time.Millisecond)
		}(i)
	}

	wg.Wait()
}

func TestMetrics_ConcurrentOAuthRecording(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			result := OAuthResultSuccess
			if id%3 == 0 {
				result = OAuthResultFallback
			} else if id%3 == 1 {
				result = OAuthResultFailure
			}
			metrics.RecordOAuthDownstreamAuth(ctx, result)
		}(i)
	}

	wg.Wait()
}

func TestMetrics_ConcurrentSessionTracking(t *testing.T) {
	meter := mockMeterProvider()
	metrics, err := NewMetrics(meter, false)
	if err != nil {
		t.Fatalf("expected no error creating metrics, got %v", err)
	}

	ctx := context.Background()
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Half incrementing, half decrementing
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			metrics.IncrementActiveSessions(ctx)
		}()
		go func() {
			defer wg.Done()
			metrics.DecrementActiveSessions(ctx)
		}()
	}

	wg.Wait()
	// Final count should be around 0, but we can't easily verify this
	// The important thing is no race conditions or panics
}
