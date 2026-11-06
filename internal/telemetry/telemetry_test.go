package telemetry_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/telemetry"
	collect "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestHTTPTraceContextLogCorrelationAndFlush(t *testing.T) {
	exported := make(chan *collect.ExportTraceServiceRequest, 4)
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected exporter path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		req := new(collect.ExportTraceServiceRequest)
		if err := proto.Unmarshal(body, req); err != nil {
			t.Errorf("decode OTLP: %v", err)
			w.WriteHeader(400)
			return
		}
		exported <- req
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer sink.Close()
	cert := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sink.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", sink.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", cert)
	t.Setenv("OTEL_SERVICE_NAME", "test-application-web")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=synthetic,k8s.namespace.name=never-registered")
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://must-not-be-used.invalid")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE", "true")
	tracing, err := telemetry.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := tracing.Middleware(httpx.Middleware(logger, "test", "test-region")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })))
	req := httptest.NewRequest("GET", "https://example.test/private/user?secret=must-not-enter-span", nil)
	req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
	req.Header.Set("Authorization", "Bearer must-not-enter-span")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	// Health probes are intentionally not additional spans.
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := tracing.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-exported:
		if len(data.ResourceSpans) != 1 || len(data.ResourceSpans[0].ScopeSpans) != 1 || len(data.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Fatalf("expected one request span: %v", data)
		}
		span := data.ResourceSpans[0].ScopeSpans[0].Spans[0]
		if hex.EncodeToString(span.TraceId) != "11111111111111111111111111111111" || hex.EncodeToString(span.ParentSpanId) != "2222222222222222" {
			t.Fatal("incoming context not propagated")
		}
		if strings.Contains(data.String(), "must-not-enter-span") || strings.Contains(data.String(), "/private/user") {
			t.Fatal("sensitive request data in trace")
		}
		var record map[string]any
		if err := json.Unmarshal(bytes.Split(logs.Bytes(), []byte("\n"))[0], &record); err != nil {
			t.Fatal(err)
		}
		if record["trace_id"] != hex.EncodeToString(span.TraceId) || record["span_id"] != hex.EncodeToString(span.SpanId) {
			t.Fatalf("request log not correlated: %v", record)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not flush span")
	}
}

func TestNoEndpointPreservesHandlerAndShutdown(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	tracing, err := telemetry.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	response := httptest.NewRecorder()
	tracing.Middleware(handler).ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRejectInsecureOrCredentialEndpointsWithoutLeakingThem(t *testing.T) {
	for _, endpoint := range []string{"http://collector.invalid:443", "https://user:must-not-print@collector.invalid:443", "https://collector.invalid:443/?secret=must-not-print"} {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)
		t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
		_, err := telemetry.New(context.Background())
		if err == nil {
			t.Fatalf("accepted invalid endpoint")
		}
		if strings.Contains(err.Error(), "must-not-print") {
			t.Fatal("credential leaked")
		}
	}
}
