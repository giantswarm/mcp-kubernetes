//go:build clusterbench

// Package pod bench harness driving handleGetLogs against a live cluster
// to measure how many bytes each output format produces. The output arg is
// expected to be a no-op for logs; the bench pins that and gives us a single
// place to measure tailLines / sinceTime impact alongside the other read tools.
//
// Run with:
//
//	KUBECONFIG=$HOME/.kube/config \
//	  CLUSTERBENCH_CONTEXT=<your-kube-context> \
//	  CLUSTERBENCH_LOG_PODS="<namespace>/<pod-name>,<namespace>/<pod-name>" \
//	  CLUSTERBENCH_REPORT=/tmp/sizebench-pod.md \
//	  go test -tags=clusterbench -run TestSizeBench_Pod \
//	    -v -count=1 -timeout=5m ./internal/tools/pod/...
package pod

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

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

// TestSizeBench_Pod walks five pods and runs the logs tool with output =
// slim/normal/wide and a few tailLines values. The output arg should not
// affect log size; tailLines should.
func TestSizeBench_Pod(t *testing.T) {
	sc := newBenchServerContext(t)

	podsEnv := os.Getenv("CLUSTERBENCH_LOG_PODS")
	if podsEnv == "" {
		t.Skip("CLUSTERBENCH_LOG_PODS not set; skipping log bench")
	}

	type podRef struct{ ns, name string }
	var pods []podRef
	for _, p := range strings.Split(podsEnv, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		parts := strings.SplitN(p, "/", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed pod ref %q (expected ns/name)", p)
		}
		pods = append(pods, podRef{ns: parts[0], name: parts[1]})
	}

	formats := []string{"slim", "normal", "wide"}
	tailLines := []int{50, 100, 500}

	type row struct {
		Pod       string
		Format    string
		TailLines int
		Bytes     int
		IsError   bool
		Note      string
	}

	var rows []row

	for _, p := range pods {
		for _, f := range formats {
			for _, tl := range tailLines {
				args := map[string]interface{}{
					"namespace": p.ns,
					"podName":   p.name,
					"output":    f,
					"tailLines": float64(tl),
				}
				req := mcp.CallToolRequest{}
				req.Params.Arguments = args

				result, err := handleGetLogs(context.Background(), req, sc)
				require.NoError(t, err)
				r := row{
					Pod:       fmt.Sprintf("%s/%s", p.ns, p.name),
					Format:    f,
					TailLines: tl,
					Bytes:     responseSize(t, result),
					IsError:   result.IsError,
				}
				if result.IsError && len(result.Content) > 0 {
					if tc, ok := result.Content[0].(mcp.TextContent); ok {
						r.Note = strings.ReplaceAll(tc.Text, "\n", " ")
						if len(r.Note) > 120 {
							r.Note = r.Note[:120] + "..."
						}
					}
				}
				rows = append(rows, r)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintln(&b, "# Pod logs size bench")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Pod | Format | tailLines | Bytes | Error | Note |")
	fmt.Fprintln(&b, "|---|---|---:|---:|:---:|---|")
	for _, r := range rows {
		errMark := ""
		if r.IsError {
			errMark = "x"
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %s | %s |\n",
			r.Pod, r.Format, r.TailLines, r.Bytes, errMark, r.Note)
	}

	// Per-pod: format should not affect bytes for a fixed tailLines.
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Format-vs-bytes (sanity: should be identical per (pod, tailLines))")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Pod | tailLines | slim | normal | wide |")
	fmt.Fprintln(&b, "|---|---:|---:|---:|---:|")
	type key struct {
		pod string
		tl  int
	}
	bytesBy := map[key]map[string]int{}
	for _, r := range rows {
		k := key{r.Pod, r.TailLines}
		if bytesBy[k] == nil {
			bytesBy[k] = map[string]int{}
		}
		bytesBy[k][r.Format] = r.Bytes
	}
	for k, m := range bytesBy {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d |\n",
			k.pod, k.tl, m["slim"], m["normal"], m["wide"])
	}

	report := b.String()
	t.Log("\n" + report)

	if path := os.Getenv("CLUSTERBENCH_REPORT"); path != "" {
		require.NoError(t, os.WriteFile(path, []byte(report), 0o600))
		t.Logf("report written to %s", path)
	}
}
