package instrumentation

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type traceReceiver struct {
	collectortrace.UnimplementedTraceServiceServer
	requests chan metadata.MD
}

func (r *traceReceiver) Export(ctx context.Context, _ *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.requests <- md
	return &collectortrace.ExportTraceServiceResponse{}, nil
}

func startTraceReceiver(t *testing.T) (string, *traceReceiver) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	recv := &traceReceiver{requests: make(chan metadata.MD, 1)}
	srv := grpc.NewServer()
	collectortrace.RegisterTraceServiceServer(srv, recv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), recv
}

func TestNewProvider_OTLPGRPCExportsWithHeaders(t *testing.T) {
	endpoint, recv := startTraceReceiver(t)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "X-Scope-OrgID=giantswarm")

	provider, err := NewProvider(t.Context(), Config{
		ServiceName:       "mcp-kubernetes",
		Enabled:           true,
		MetricsExporter:   ExporterPrometheus,
		TracingExporter:   ExporterOTLP,
		OTLPEndpoint:      endpoint,
		OTLPProtocol:      ProtocolGRPC,
		OTLPInsecure:      true,
		TraceSamplingRate: 1,
	})
	require.NoError(t, err)

	_, span := provider.Tracer("test").Start(t.Context(), "tools/call")
	span.End()
	require.NoError(t, provider.Shutdown(t.Context()))

	select {
	case md := <-recv.requests:
		require.Equal(t, []string{"giantswarm"}, md.Get("x-scope-orgid"))
	default:
		t.Fatal("the gRPC receiver got no export")
	}
}

func TestConfigValidate_OTLPProtocol(t *testing.T) {
	for _, protocol := range []string{"", ProtocolHTTPProtobuf, ProtocolGRPC} {
		c := Config{OTLPProtocol: protocol}
		require.NoError(t, c.Validate(), protocol)
	}
	c := Config{OTLPProtocol: "http/json"}
	require.ErrorContains(t, c.Validate(), "invalid OTLP protocol")
}

func TestDefaultConfig_OTLPProtocol(t *testing.T) {
	require.Equal(t, ProtocolHTTPProtobuf, DefaultConfig().OTLPProtocol)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", ProtocolGRPC)
	require.Equal(t, ProtocolGRPC, DefaultConfig().OTLPProtocol)
}
