// Package httpx holds the HTTP plumbing: request ids, identity headers, JSON
// access logs, probes and graceful shutdown.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"
)

const (
	HeaderRequestID = "X-Request-Id"
	HeaderServedBy  = "X-Served-By"
	HeaderRegion    = "X-Region"

	// RequestTimeout bounds every request so a blocked database turns into
	// a quick 5xx instead of a hung client.
	RequestTimeout = 15 * time.Second
	// DrainDelay keeps serving after SIGTERM while Kubernetes removes the pod
	// from Service endpoints; without it new connections get refused.
	DrainDelay = 5 * time.Second
	// ShutdownTimeout is how long in-flight requests get to finish.
	ShutdownTimeout = 20 * time.Second
)

// MsgUnavailable is the user-facing text for any temporary outage.
const MsgUnavailable = "일시적으로 서비스를 이용할 수 없습니다. 잠시 후 다시 시도해 주세요."

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyLogger
)

// NewLogger returns a JSON-lines logger tagged with the instance.
func NewLogger(instance, region string) *slog.Logger {
	l := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("instance", instance)
	if region != "" {
		l = l.With("region", region)
	}
	return l
}

// RequestID returns the id the middleware attached to ctx.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(keyRequestID).(string)
	return id
}

// Logger returns the request-scoped logger, falling back to the default.
func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(keyLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// Middleware assigns a request id, stamps X-Served-By (and X-Region when
// set) on every response, recovers panics and writes one access log line.
func Middleware(log *slog.Logger, servedBy, region string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := r.Header.Get(HeaderRequestID)
			if id == "" || len(id) > 128 {
				id = newID()
			}
			reqLog := log.With("request_id", id)
			if span := trace.SpanContextFromContext(r.Context()); span.IsValid() {
				reqLog = reqLog.With("trace_id", span.TraceID().String(), "span_id", span.SpanID().String())
			}
			ctx := context.WithValue(r.Context(), keyRequestID, id)
			ctx = context.WithValue(ctx, keyLogger, reqLog)
			ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
			defer cancel()

			h := w.Header()
			h.Set(HeaderRequestID, id)
			h.Set(HeaderServedBy, servedBy)
			if region != "" {
				h.Set(HeaderRegion, region)
			}

			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if p := recover(); p != nil {
					if p == http.ErrAbortHandler {
						panic(p)
					}
					reqLog.Error("panic", "panic", p)
					if !rec.wrote {
						Error(rec, http.StatusInternalServerError, MsgUnavailable)
					}
				}
				level := slog.LevelInfo
				if isProbe(r.URL.Path) {
					level = slog.LevelDebug // probes would drown the log
				}
				reqLog.Log(ctx, level, "request",
					"method", r.Method, "path", r.URL.Path, "status", rec.status,
					"bytes", rec.bytes, "duration_ms", time.Since(start).Milliseconds())
			}()
			next.ServeHTTP(rec, r.WithContext(ctx))
		})
	}
}

func isProbe(path string) bool { return path == "/healthz" || path == "/readyz" }

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController (used by ReverseProxy) reach Flush.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes {"error": msg}.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Probes serves /healthz (process is alive) and /readyz (dependencies are
// reachable and the server is not draining).
type Probes struct {
	check    func(context.Context) error
	draining atomic.Bool
}

// NewProbes uses check for readiness; nil means always ready.
func NewProbes(check func(context.Context) error) *Probes {
	return &Probes{check: check}
}

func (p *Probes) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if p.draining.Load() {
			Error(w, http.StatusServiceUnavailable, "draining")
			return
		}
		if p.check != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := p.check(ctx); err != nil {
				// Details go to the log only; /readyz is publicly reachable.
				Logger(r.Context()).Warn("not ready", "err", err)
				Error(w, http.StatusServiceUnavailable, "not ready")
				return
			}
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}

// NewServer returns a server with timeouts suitable for this app.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Serve runs srv until ctx is cancelled (SIGTERM), then fails readiness,
// waits DrainDelay, and lets in-flight requests finish.
func Serve(ctx context.Context, log *slog.Logger, srv *http.Server, p *Probes) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", srv.Addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutdown: draining", "delay", DrainDelay.String())
	p.draining.Store(true)
	// Connection: close on remaining responses moves keep-alive clients
	// (load balancers, proxies) to pods that are staying.
	srv.SetKeepAlivesEnabled(false)
	time.Sleep(DrainDelay)

	sctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("shutdown: done")
	return nil
}

// BearerToken extracts the token from an Authorization header.
func BearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(v, "Bearer "); ok {
		return strings.TrimSpace(t)
	}
	return ""
}
