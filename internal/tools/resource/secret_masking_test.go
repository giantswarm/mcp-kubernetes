package resource

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/resource/testdata"
)

// Values that must never appear in a tool result. The last-applied
// configuration repeats them, as kubectl apply writes it.
const (
	secretPlain  = "hunter2-plain"
	secretBase64 = "aHVudGVyMi1iYXNlNjQ=" //nolint:gosec // G101: test fixture, not a real credential
	cmPassword   = "cm-db-password"
)

func sensitiveObjects() []*unstructured.Unstructured {
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      "db",
			"namespace": "default",
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"` + secretBase64 + `"},"stringData":{"token":"` + secretPlain + `"}}`,
			},
		},
		"type":       "Opaque",
		"data":       map[string]interface{}{"password": secretBase64},
		"stringData": map[string]interface{}{"token": secretPlain},
	}}
	configMap := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "db-credentials",
			"namespace": "default",
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"` + cmPassword + `"}}`,
			},
		},
		"data": map[string]interface{}{"password": cmPassword},
	}}
	return []*unstructured.Unstructured{secret, configMap}
}

// sensitiveK8sClient answers every read and write with a sensitive object.
type sensitiveK8sClient struct {
	testdata.MockK8sClient
	obj *unstructured.Unstructured
}

func (m *sensitiveK8sClient) Get(context.Context, string, string, string, string, string) (*k8s.GetResponse, error) {
	return &k8s.GetResponse{Resource: m.obj.DeepCopy()}, nil
}

func (m *sensitiveK8sClient) List(context.Context, string, string, string, string, k8s.ListOptions) (*k8s.PaginatedListResponse, error) {
	return &k8s.PaginatedListResponse{Items: []runtime.Object{m.obj.DeepCopy()}, TotalItems: 1}, nil
}

func (m *sensitiveK8sClient) Describe(context.Context, string, string, string, string, string) (*k8s.ResourceDescription, error) {
	obj := m.obj.DeepCopy()
	return &k8s.ResourceDescription{
		Resource: obj,
		Metadata: map[string]interface{}{
			"kind":        obj.GetKind(),
			"annotations": obj.GetAnnotations(),
		},
	}, nil
}

func (m *sensitiveK8sClient) Create(context.Context, string, string, runtime.Object) (runtime.Object, error) {
	return m.obj.DeepCopy(), nil
}

func (m *sensitiveK8sClient) Apply(context.Context, string, string, runtime.Object) (runtime.Object, error) {
	return m.obj.DeepCopy(), nil
}

func (m *sensitiveK8sClient) Patch(context.Context, string, string, string, string, string, types.PatchType, []byte) (*k8s.PatchResponse, error) {
	return &k8s.PatchResponse{Resource: m.obj.DeepCopy()}, nil
}

// TestToolsNeverReturnSecretData runs every tool that returns a resource,
// in every output format, against a Secret and a sensitive ConfigMap.
func TestToolsNeverReturnSecretData(t *testing.T) {
	type handler func(context.Context, mcp.CallToolRequest, *server.ServerContext) (*mcp.CallToolResult, error)

	manifest := map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]interface{}{"name": "db"}}
	calls := []struct {
		name    string
		handler handler
		args    map[string]interface{}
	}{
		{"get", handleGetResource, map[string]interface{}{"resourceType": "secrets", "name": "db"}},
		{"list", handleListResources, map[string]interface{}{"resourceType": "secrets", "includeAnnotations": true}},
		{"list full", handleListResources, map[string]interface{}{"resourceType": "secrets", "fullOutput": true}},
		{"list summary", handleListResources, map[string]interface{}{"resourceType": "secrets", "summary": true}},
		{"describe", handleDescribeResource, map[string]interface{}{"resourceType": "secrets", "name": "db"}},
		{"create", handleCreateResource, map[string]interface{}{"namespace": "default", "manifest": manifest}},
		{"apply", handleApplyResource, map[string]interface{}{"namespace": "default", "manifest": manifest}},
		{"patch", handlePatchResource, map[string]interface{}{"resourceType": "secrets", "name": "db", "patchType": "merge", "patch": map[string]interface{}{}}},
	}

	for _, obj := range sensitiveObjects() {
		sc, err := server.NewServerContext(context.Background(),
			server.WithK8sClient(&sensitiveK8sClient{obj: obj}),
			server.WithLogger(&testdata.MockLogger{}),
			server.WithNonDestructiveMode(false),
		)
		require.NoError(t, err)

		for _, call := range calls {
			for _, format := range []string{"", "slim", "normal", "wide", "full"} {
				t.Run(obj.GetKind()+"/"+call.name+"/"+format, func(t *testing.T) {
					args := map[string]interface{}{"output": format}
					for k, v := range call.args {
						args[k] = v
					}
					request := mcp.CallToolRequest{}
					request.Params.Arguments = args

					result, err := call.handler(context.Background(), request, sc)
					require.NoError(t, err)
					require.False(t, result.IsError, "unexpected error: %v", result.Content)

					text := result.Content[0].(mcp.TextContent).Text
					for _, value := range []string{secretPlain, secretBase64, cmPassword} {
						assert.NotContains(t, text, value)
					}
				})
			}
		}
	}
}
