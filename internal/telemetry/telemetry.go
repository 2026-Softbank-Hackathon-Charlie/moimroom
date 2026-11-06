// Package telemetry connects an SDK-enabled service to the platform OTLP profile.
// An absent endpoint keeps the application's previous behavior.
package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct{ provider *sdktrace.TracerProvider }

func New(ctx context.Context) (*Telemetry, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" || os.Getenv("OTEL_TRACES_EXPORTER") == "none" {
		return &Telemetry{}, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true" {
		return nil, fmt.Errorf("telemetry requires an HTTPS OTLP endpoint without credentials, path, query or fragment")
	}
	// Configure the validated endpoint and trust explicitly so per-signal environment
	// overrides cannot silently disable TLS or redirect this platform exporter.
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("telemetry trust initialization failed")
	}
	if cert := os.Getenv("OTEL_EXPORTER_OTLP_CERTIFICATE"); cert != "" {
		pem, err := os.ReadFile(cert)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("telemetry CA initialization failed")
		}
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithURLPath("/v1/traces"),
		otlptracehttp.WithTLSClientConfig(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}),
		otlptracehttp.WithTimeout(3*time.Second))
	if err != nil {
		return nil, fmt.Errorf("telemetry exporter initialization failed")
	}
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithTelemetrySDK())
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("telemetry resource initialization failed")
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(exporter, sdktrace.WithMaxQueueSize(1024), sdktrace.WithBatchTimeout(time.Second), sdktrace.WithExportTimeout(3*time.Second)))
	return &Telemetry{provider: provider}, nil
}

// Shutdown uses the caller's bounded context; it never delays request handling.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t.provider == nil {
		return nil
	}
	return t.provider.Shutdown(ctx)
}

func (t *Telemetry) Middleware(next http.Handler) http.Handler {
	if t.provider == nil {
		return next
	}
	tracer := t.provider.Tracer("moimroom/http")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := (propagation.TraceContext{}).Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		// Method only: IDs, URL queries, credentials and request bodies stay out of spans.
		method := r.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			method = "_OTHER"
		}
		ctx, span := tracer.Start(ctx, "HTTP "+method, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", method)))
		defer span.End()
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))
		span.SetAttributes(attribute.Int("http.response.status_code", rec.status))
		if rec.status >= 500 {
			span.SetStatus(codes.Error, "")
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.wrote = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
