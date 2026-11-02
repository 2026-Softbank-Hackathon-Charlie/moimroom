package httpx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestMiddlewareHeaders(t *testing.T) {
	var seenID string
	h := Middleware(quietLog(), "pod-a", "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestID(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/spaces", nil))
	if got := rec.Header().Get(HeaderServedBy); got != "pod-a" {
		t.Errorf("X-Served-By = %q", got)
	}
	if _, ok := rec.Header()[HeaderRegion]; ok {
		t.Error("X-Region must be absent when no label is set")
	}
	if id := rec.Header().Get(HeaderRequestID); id == "" || id != seenID {
		t.Errorf("request id header %q, context %q", id, seenID)
	}

	h = Middleware(quietLog(), "pod-a", "seoul-1")(http.NotFoundHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, "abc123")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(HeaderRegion); got != "seoul-1" {
		t.Errorf("X-Region = %q", got)
	}
	if got := rec.Header().Get(HeaderRequestID); got != "abc123" {
		t.Errorf("incoming request id not kept: %q", got)
	}
}

func TestMiddlewareRecoversPanic(t *testing.T) {
	h := Middleware(quietLog(), "pod-a", "")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestProbes(t *testing.T) {
	var checkErr error
	p := NewProbes(func(context.Context) error { return checkErr })
	mux := http.NewServeMux()
	p.Register(mux)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}

	if rec := get("/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("ready: %d", rec.Code)
	}
	checkErr = errors.New("dial tcp db.internal:5432: refused")
	rec := get("/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db.internal") {
		t.Fatal("readiness body must not leak dependency details")
	}
	if rec := get("/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("liveness must ignore dependencies: %d", rec.Code)
	}
	checkErr = nil
	p.draining.Store(true)
	if rec := get("/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("draining: %d", rec.Code)
	}
}
