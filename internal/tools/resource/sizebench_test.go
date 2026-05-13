//go:build clusterbench

// Package resource bench harness driving the real handlers against a live
// cluster to measure how many bytes each output format produces.
//
// Usage:
//
//	KUBECONFIG=$HOME/.kube/config \
//	  CLUSTERBENCH_CONTEXT=<your-kube-context> \
//	  CLUSTERBENCH_WORKLOADS="<ns>/<resourceType>/<name>[/<apiGroup>],..." \
//	  CLUSTERBENCH_REPORT=/tmp/sizebench-resource.md \
//	  go test -tags=clusterbench -run TestSizeBench_Resource \
//	    -v -count=1 -timeout=5m ./internal/tools/resource/...
//
// CLUSTERBENCH_WORKLOADS is a comma-separated list of workloads to exercise.
// Each entry is "<namespace>/<resourceType>/<name>" with an optional
// "/<apiGroup>" suffix. For cluster-scoped resources, leave the namespace
// empty (e.g. "/node/<node-name>").
//
// Pick five representative shapes (one deployment, one statefulset, one
// daemonset, one cluster-scoped resource, one Secret to validate masking)
// to mirror the methodology documented in docs/slim-output-tuning.md.
package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

type benchWorkload struct {
	name         string
	namespace    string
	resourceType string
	apiGroup     string
}

func parseBenchWorkloads(t *testing.T, raw string) []benchWorkload {
	t.Helper()
	if raw == "" {
		t.Skip("CLUSTERBENCH_WORKLOADS not set; skipping live cluster bench")
	}

	var out []benchWorkload
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "/")
		if len(parts) < 3 {
			t.Fatalf("malformed workload %q (expected ns/resourceType/name[/apiGroup])", entry)
		}
		w := benchWorkload{
			namespace:    parts[0],
			resourceType: parts[1],
			name:         parts[2],
		}
		if len(parts) >= 4 {
			w.apiGroup = strings.Join(parts[3:], "/")
		}
		out = append(out, w)
	}
	if len(out) == 0 {
		t.Skip("CLUSTERBENCH_WORKLOADS contained no usable entries; skipping")
	}
	return out
}

func newBenchServerContext(t *testing.T) *server.ServerContext {
	t.Helper()

	kctx := os.Getenv("CLUSTERBENCH_CONTEXT")
	if kctx == "" {
		t.Skip("CLUSTERBENCH_CONTEXT not set; skipping live cluster bench")
	}

	cfg := &k8s.ClientConfig{
		KubeconfigPath:     os.Getenv("KUBECONFIG"),
		Context:            kctx,
		NonDestructiveMode: true,
	}
	client, err := k8s.NewClient(cfg)
	require.NoError(t, err, "create real k8s client")

	sc, err := server.NewServerContext(context.Background(),
		server.WithK8sClient(client),
		server.WithLogger(server.NewDefaultLogger()),
		server.WithNonDestructiveMode(true),
	)
	require.NoError(t, err)
	return sc
}

func responseSize(t *testing.T, result *mcp.CallToolResult) int {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		return 0
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return 0
	}
	return len(tc.Text)
}

// TestSizeBench_Resource drives the get and describe tools on
// the workloads from CLUSTERBENCH_WORKLOADS with output=slim/normal/wide
// and writes a markdown report.
func TestSizeBench_Resource(t *testing.T) {
	sc := newBenchServerContext(t)
	workloads := parseBenchWorkloads(t, os.Getenv("CLUSTERBENCH_WORKLOADS"))

	formats := []string{"slim", "normal", "wide"}

	type row struct {
		Tool       string
		Workload   string
		Format     string
		Bytes      int
		IsError    bool
		ErrSnippet string
	}

	var rows []row

	tools := []struct {
		name    string
		handler func(ctx context.Context, req mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error)
	}{
		{name: "get", handler: handleGetResource},
		{name: "describe", handler: handleDescribeResource},
	}

	for _, w := range workloads {
		for _, tool := range tools {
			for _, f := range formats {
				args := map[string]interface{}{
					"namespace":    w.namespace,
					"resourceType": w.resourceType,
					"name":         w.name,
					"output":       f,
				}
				if w.apiGroup != "" {
					args["apiGroup"] = w.apiGroup
				}
				req := mcp.CallToolRequest{}
				req.Params.Arguments = args

				result, err := tool.handler(context.Background(), req, sc)
				require.NoError(t, err)

				if dir := os.Getenv("CLUSTERBENCH_DUMP_DIR"); dir != "" && !result.IsError && len(result.Content) > 0 {
					if tc, ok := result.Content[0].(mcp.TextContent); ok {
						_ = os.MkdirAll(dir, 0o700)
						fname := fmt.Sprintf("%s/%s_%s_%s_%s.json",
							dir,
							sanitize(tool.name),
							sanitize(w.resourceType),
							sanitize(w.name),
							f)
						_ = os.WriteFile(fname, []byte(tc.Text), 0o600)
					}
				}

				r := row{
					Tool:     tool.name,
					Workload: fmt.Sprintf("%s/%s", w.resourceType, w.name),
					Format:   f,
					Bytes:    responseSize(t, result),
					IsError:  result.IsError,
				}
				if result.IsError && len(result.Content) > 0 {
					if tc, ok := result.Content[0].(mcp.TextContent); ok {
						r.ErrSnippet = truncate(tc.Text, 120)
					}
				}
				rows = append(rows, r)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintln(&b, "# Resource handler size bench")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "All sizes are bytes of the JSON response body returned to the MCP caller.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Tool | Workload | Format | Bytes | Error | Note |")
	fmt.Fprintln(&b, "|---|---|---|---:|:---:|---|")
	for _, r := range rows {
		errMark := ""
		if r.IsError {
			errMark = "x"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s |\n",
			r.Tool, r.Workload, r.Format, r.Bytes, errMark, r.ErrSnippet)
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Per-format reduction")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Three columns matter:")
	fmt.Fprintln(&b, "- `wide vs slim` — total response shrinkage from full manifest to the LLM-friendly default.")
	fmt.Fprintln(&b, "- `wide vs normal` — what the generic blacklist (managedFields, last-applied-configuration, ...) buys on its own.")
	fmt.Fprintln(&b, "- `normal vs slim` — the *per-Kind shaping* delta. For HelmRelease this is `spec.values` + `status.history`; for workload templates it is the env-collapse threshold.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Tool | Workload | wide | normal | slim | wide→slim | wide→normal | normal→slim |")
	fmt.Fprintln(&b, "|---|---|---:|---:|---:|---:|---:|---:|")
	type key struct{ tool, workload string }
	bytesByFormat := map[key]map[string]int{}
	for _, r := range rows {
		k := key{r.Tool, r.Workload}
		if bytesByFormat[k] == nil {
			bytesByFormat[k] = map[string]int{}
		}
		bytesByFormat[k][r.Format] = r.Bytes
	}
	pct := func(from, to int) string {
		if from <= 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.1f%%", (float64(from-to)/float64(from))*100)
	}
	for k, m := range bytesByFormat {
		wide := m["wide"]
		normal := m["normal"]
		slim := m["slim"]
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %s | %s | %s |\n",
			k.tool, k.workload, wide, normal, slim,
			pct(wide, slim), pct(wide, normal), pct(normal, slim))
	}

	report := b.String()
	t.Log("\n" + report)

	if path := os.Getenv("CLUSTERBENCH_REPORT"); path != "" {
		require.NoError(t, os.WriteFile(path, []byte(report), 0o600))
		t.Logf("report written to %s", path)
	}

	if path := os.Getenv("CLUSTERBENCH_JSON"); path != "" {
		raw, err := json.MarshalIndent(rows, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, raw, 0o600))
	}
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", " ", "_", ".", "_")
	return r.Replace(s)
}
