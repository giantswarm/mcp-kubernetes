package output

import (
	"testing"
)

func TestGenerateSummary(t *testing.T) {
	objects := []map[string]interface{}{
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-1",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"phase": "Running",
			},
		},
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-2",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"phase": "Running",
			},
		},
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-3",
				"namespace": "kube-system",
			},
			"status": map[string]interface{}{
				"phase": "Pending",
			},
		},
	}

	summary := GenerateSummary(objects, nil)

	if summary.Total != 3 {
		t.Errorf("Total = %d, want 3", summary.Total)
	}

	// Check status counts
	if summary.ByStatus["Running"] != 2 {
		t.Errorf("ByStatus[Running] = %d, want 2", summary.ByStatus["Running"])
	}
	if summary.ByStatus["Pending"] != 1 {
		t.Errorf("ByStatus[Pending] = %d, want 1", summary.ByStatus["Pending"])
	}

	// Check namespace counts
	if summary.ByNamespace["default"] != 2 {
		t.Errorf("ByNamespace[default] = %d, want 2", summary.ByNamespace["default"])
	}
	if summary.ByNamespace["kube-system"] != 1 {
		t.Errorf("ByNamespace[kube-system] = %d, want 1", summary.ByNamespace["kube-system"])
	}

	// Check sample
	if len(summary.Sample) == 0 {
		t.Error("Sample should not be empty")
	}
}

func TestGenerateSummary_EmptyList(t *testing.T) {
	summary := GenerateSummary([]map[string]interface{}{}, nil)

	if summary.Total != 0 {
		t.Errorf("Total = %d, want 0", summary.Total)
	}
	if summary.HasMore {
		t.Error("HasMore should be false for empty list")
	}
}

func TestGenerateSummary_WithOptions(t *testing.T) {
	objects := makeTestPods(5)

	opts := &SummaryOptions{
		MaxSampleSize:      2,
		IncludeByStatus:    true,
		IncludeByNamespace: false,
		IncludeByCluster:   false,
		IncludeByKind:      true,
	}

	summary := GenerateSummary(objects, opts)

	// Sample should be limited
	if len(summary.Sample) != 2 {
		t.Errorf("Sample length = %d, want 2", len(summary.Sample))
	}

	// HasMore should be true
	if !summary.HasMore {
		t.Error("HasMore should be true when items exceed sample size")
	}

	// ByNamespace should be nil (disabled)
	if summary.ByNamespace != nil {
		t.Error("ByNamespace should be nil when disabled")
	}

	// ByKind should have Pod count
	if summary.ByKind["Pod"] != 5 {
		t.Errorf("ByKind[Pod] = %d, want 5", summary.ByKind["Pod"])
	}
}

func TestGenerateFleetSummary(t *testing.T) {
	objects := []map[string]interface{}{
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "pod-1",
				"labels": map[string]interface{}{
					"cluster": "cluster-a",
				},
			},
		},
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "pod-2",
				"labels": map[string]interface{}{
					"cluster": "cluster-b",
				},
			},
		},
	}

	summary := GenerateFleetSummary(objects, "metadata.labels.cluster")

	if summary.Total != 2 {
		t.Errorf("Total = %d, want 2", summary.Total)
	}

	if summary.ByCluster["cluster-a"] != 1 {
		t.Errorf("ByCluster[cluster-a] = %d, want 1", summary.ByCluster["cluster-a"])
	}
	if summary.ByCluster["cluster-b"] != 1 {
		t.Errorf("ByCluster[cluster-b] = %d, want 1", summary.ByCluster["cluster-b"])
	}
}

func TestShouldUseSummary(t *testing.T) {
	tests := []struct {
		itemCount int
		threshold int
		want      bool
	}{
		{100, 500, false},
		{500, 500, false},
		{501, 500, true},
		{1000, 500, true},
		{100, 0, false}, // Uses default threshold (500)
		{600, 0, true},  // Uses default threshold (500)
	}

	for _, tt := range tests {
		got := ShouldUseSummary(tt.itemCount, tt.threshold)
		if got != tt.want {
			t.Errorf("ShouldUseSummary(%d, %d) = %v, want %v",
				tt.itemCount, tt.threshold, got, tt.want)
		}
	}
}

func TestDefaultSummaryOptions(t *testing.T) {
	opts := DefaultSummaryOptions()

	if opts.MaxSampleSize <= 0 {
		t.Error("MaxSampleSize should be positive")
	}
	if !opts.IncludeByStatus {
		t.Error("IncludeByStatus should be true by default")
	}
	if opts.IncludeByCluster {
		t.Error("IncludeByCluster should be false by default")
	}
	if !opts.IncludeByNamespace {
		t.Error("IncludeByNamespace should be true by default")
	}
}

func TestExtractStatus_Pod(t *testing.T) {
	pod := map[string]interface{}{
		"kind": "Pod",
		"status": map[string]interface{}{
			"phase": "Running",
		},
	}

	status := extractStatus(pod)
	if status != "Running" {
		t.Errorf("extractStatus(Pod) = %q, want %q", status, "Running")
	}
}

func TestExtractStatus_Deployment(t *testing.T) {
	deployment := map[string]interface{}{
		"kind": "Deployment",
		"spec": map[string]interface{}{
			"replicas": 3,
		},
		"status": map[string]interface{}{
			"readyReplicas":     3,
			"availableReplicas": 3,
		},
	}

	status := extractStatus(deployment)
	if status != "Ready" {
		t.Errorf("extractStatus(Deployment) = %q, want %q", status, "Ready")
	}

	// Test partial ready
	deployment["status"].(map[string]interface{})["readyReplicas"] = 1
	status = extractStatus(deployment)
	if status != "Partially Ready" {
		t.Errorf("extractStatus(Deployment partial) = %q, want %q", status, "Partially Ready")
	}
}

func TestExtractStatus_Node(t *testing.T) {
	node := map[string]interface{}{
		"kind": "Node",
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	}

	status := extractStatus(node)
	if status != "Ready" {
		t.Errorf("extractStatus(Node) = %q, want %q", status, "Ready")
	}

	// Test NotReady
	node["status"].(map[string]interface{})["conditions"].([]interface{})[0].(map[string]interface{})["status"] = "False"
	status = extractStatus(node)
	if status != "NotReady" {
		t.Errorf("extractStatus(Node not ready) = %q, want %q", status, "NotReady")
	}
}

func TestExtractStatus_Job(t *testing.T) {
	job := map[string]interface{}{
		"kind": "Job",
		"status": map[string]interface{}{
			"succeeded": 1,
		},
	}

	status := extractStatus(job)
	if status != "Succeeded" {
		t.Errorf("extractStatus(Job succeeded) = %q, want %q", status, "Succeeded")
	}

	// Test failed
	job["status"] = map[string]interface{}{
		"failed": 1,
	}
	status = extractStatus(job)
	if status != "Failed" {
		t.Errorf("extractStatus(Job failed) = %q, want %q", status, "Failed")
	}

	// Test running
	job["status"] = map[string]interface{}{}
	status = extractStatus(job)
	if status != "Running" {
		t.Errorf("extractStatus(Job running) = %q, want %q", status, "Running")
	}
}

func TestTopCounts(t *testing.T) {
	counts := map[string]int{
		"a": 10,
		"b": 30,
		"c": 20,
		"d": 5,
	}

	// Get top 2
	top := TopCounts(counts, 2)
	if len(top) != 2 {
		t.Errorf("TopCounts returned %d items, want 2", len(top))
	}
	if top[0].Key != "b" || top[0].Count != 30 {
		t.Errorf("First entry = %v, want {b, 30}", top[0])
	}
	if top[1].Key != "c" || top[1].Count != 20 {
		t.Errorf("Second entry = %v, want {c, 20}", top[1])
	}

	// Get all
	all := TopCounts(counts, 0)
	if len(all) != 4 {
		t.Errorf("TopCounts(0) returned %d items, want 4", len(all))
	}

	// Empty map
	empty := TopCounts(map[string]int{}, 5)
	if empty != nil {
		t.Error("TopCounts of empty map should return nil")
	}
}

func TestGetNestedInt(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": 3,
		},
		"status": map[string]interface{}{
			"readyReplicas": float64(2), // JSON numbers are float64
		},
	}

	// Test int value
	val, ok := getNestedInt(obj, "spec", "replicas")
	if !ok || val != 3 {
		t.Errorf("getNestedInt(spec.replicas) = %d, %v, want 3, true", val, ok)
	}

	// Test float64 value (JSON numbers)
	val, ok = getNestedInt(obj, "status", "readyReplicas")
	if !ok || val != 2 {
		t.Errorf("getNestedInt(status.readyReplicas) = %d, %v, want 2, true", val, ok)
	}

	// Test missing path
	val, ok = getNestedInt(obj, "missing", "path")
	if ok {
		t.Error("getNestedInt(missing.path) should return false")
	}
}

// Helper to create test pods
func makeTestPods(count int) []map[string]interface{} {
	pods := make([]map[string]interface{}, count)
	for i := range pods {
		pods[i] = map[string]interface{}{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-" + string(rune('a'+i)),
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"phase": "Running",
			},
		}
	}
	return pods
}
