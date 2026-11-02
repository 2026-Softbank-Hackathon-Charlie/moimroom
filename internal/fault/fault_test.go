package fault

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const token = "s3cret"

func newServer(t *testing.T, tok string) (*Injector, http.Handler, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	inj := New()
	inj.now = func() time.Time { return now }
	inj.roll = func() float64 { return 0.5 }
	mux := http.NewServeMux()
	inj.Register(mux, tok)
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("/api/spaces", ok)
	mux.HandleFunc("/healthz", ok)
	mux.HandleFunc("/", ok)
	return inj, inj.Middleware(mux), &now
}

func do(h http.Handler, method, path, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminDisabledWithoutToken(t *testing.T) {
	mux := http.NewServeMux()
	New().Register(mux, "")
	rec := do(mux, "POST", "/admin/fault", "", `{"errorRate":1,"durationSeconds":10}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminAuthAndValidation(t *testing.T) {
	_, h, _ := newServer(t, token)
	if rec := do(h, "POST", "/admin/fault", "", `{}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := do(h, "POST", "/admin/fault", "wrong", `{}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", rec.Code)
	}
	for _, body := range []string{
		`not json`,
		`{"errorRate":1.5,"durationSeconds":10}`,
		`{"errorRate":-0.1,"durationSeconds":10}`,
		`{"extraLatencyMs":20000,"durationSeconds":10}`,
		`{"errorRate":0.5,"durationSeconds":0}`,
	} {
		if rec := do(h, "POST", "/admin/fault", token, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, rec.Code)
		}
	}
}

func TestErrorFaultScopeAndExpiry(t *testing.T) {
	inj, h, now := newServer(t, token)
	rec := do(h, "POST", "/admin/fault", token, `{"errorRate":1,"durationSeconds":30}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/spaces", "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("api under fault: %d", rec.Code)
	}
	// Probes, static files and admin stay healthy.
	for _, p := range []string{"/healthz", "/", "/spaces/1"} {
		if rec := do(h, "GET", p, "", ""); rec.Code != http.StatusOK {
			t.Errorf("%s under fault: %d", p, rec.Code)
		}
	}
	*now = now.Add(31 * time.Second)
	if _, ok := inj.Active(); ok {
		t.Fatal("fault should have expired")
	}
	if rec := do(h, "GET", "/api/spaces", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("after expiry: %d", rec.Code)
	}
}

func TestErrorRateUsesRoll(t *testing.T) {
	inj, h, _ := newServer(t, token)
	do(h, "POST", "/admin/fault", token, `{"errorRate":0.4,"durationSeconds":30}`)
	// roll is fixed at 0.5, so a 0.4 rate lets the request through.
	if rec := do(h, "GET", "/api/spaces", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("rate 0.4 with roll 0.5: %d", rec.Code)
	}
	inj.roll = func() float64 { return 0.1 }
	if rec := do(h, "GET", "/api/spaces", "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("rate 0.4 with roll 0.1: %d", rec.Code)
	}
}

func TestLatencyAndClear(t *testing.T) {
	_, h, _ := newServer(t, token)
	do(h, "POST", "/admin/fault", token, `{"extraLatencyMs":60,"durationSeconds":30}`)
	start := time.Now()
	if rec := do(h, "GET", "/api/spaces", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("latency only should succeed: %d", rec.Code)
	}
	if d := time.Since(start); d < 60*time.Millisecond {
		t.Fatalf("latency %v, want >= 60ms", d)
	}
	if rec := do(h, "DELETE", "/admin/fault", token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("clear: %d", rec.Code)
	}
	start = time.Now()
	do(h, "GET", "/api/spaces", "", "")
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("latency after clear %v", d)
	}
}
