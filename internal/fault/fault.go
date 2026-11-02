// Package fault lets operators inject errors and latency into the API of a
// running instance, to exercise alerts, retries and rollback procedures.
// State is per process and expires on its own.
package fault

import (
	"crypto/subtle"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
)

const (
	maxLatencyMs = 10_000
	maxDuration  = 3600
)

// Fault is the active fault. A zero ExpiresAt never happens: Set requires
// a positive duration.
type Fault struct {
	ErrorRate      float64   `json:"errorRate"`
	ExtraLatencyMs int       `json:"extraLatencyMs"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// Injector holds the current fault.
type Injector struct {
	cur  atomic.Pointer[Fault]
	now  func() time.Time
	roll func() float64
}

func New() *Injector {
	return &Injector{now: time.Now, roll: rand.Float64}
}

// Active returns the current fault if it has not expired.
func (i *Injector) Active() (Fault, bool) {
	f := i.cur.Load()
	if f == nil || !i.now().Before(f.ExpiresAt) {
		return Fault{}, false
	}
	return *f, true
}

func (i *Injector) set(f Fault) { i.cur.Store(&f) }
func (i *Injector) clear()      { i.cur.Store(nil) }

// applies limits faults to the JSON API. Probes, static files and the admin
// API stay healthy, so Kubernetes keeps the pod and the fault can be cleared.
func applies(path string) bool {
	return strings.HasPrefix(path, "/api/")
}

// Middleware applies the active fault to matching requests.
func (i *Injector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := i.Active()
		if !ok || !applies(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if f.ExtraLatencyMs > 0 {
			t := time.NewTimer(time.Duration(f.ExtraLatencyMs) * time.Millisecond)
			select {
			case <-t.C:
			case <-r.Context().Done():
				t.Stop()
				return
			}
		}
		if f.ErrorRate > 0 && i.roll() < f.ErrorRate {
			// Same body as a real outage, so nothing unusual reaches the UI.
			httpx.Error(w, http.StatusServiceUnavailable, httpx.MsgUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type setRequest struct {
	ErrorRate       float64 `json:"errorRate"`
	ExtraLatencyMs  int     `json:"extraLatencyMs"`
	DurationSeconds int     `json:"durationSeconds"`
}

// Register adds POST and DELETE /admin/fault. With an empty token nothing
// is registered, so the routes answer 404.
func (i *Injector) Register(mux *http.ServeMux, token string) {
	if token == "" {
		return
	}
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			got := httpx.BearerToken(r)
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				httpx.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("POST /admin/fault", auth(func(w http.ResponseWriter, r *http.Request) {
		var req setRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		switch {
		case req.ErrorRate < 0 || req.ErrorRate > 1:
			httpx.Error(w, http.StatusBadRequest, "errorRate must be between 0 and 1")
			return
		case req.ExtraLatencyMs < 0 || req.ExtraLatencyMs > maxLatencyMs:
			httpx.Error(w, http.StatusBadRequest, "extraLatencyMs must be between 0 and 10000")
			return
		case req.DurationSeconds < 1 || req.DurationSeconds > maxDuration:
			httpx.Error(w, http.StatusBadRequest, "durationSeconds must be between 1 and 3600")
			return
		}
		f := Fault{
			ErrorRate:      req.ErrorRate,
			ExtraLatencyMs: req.ExtraLatencyMs,
			ExpiresAt:      i.now().Add(time.Duration(req.DurationSeconds) * time.Second).UTC(),
		}
		i.set(f)
		httpx.Logger(r.Context()).Warn("fault injected", "error_rate", f.ErrorRate,
			"extra_latency_ms", f.ExtraLatencyMs, "expires_at", f.ExpiresAt)
		httpx.JSON(w, http.StatusOK, f)
	}))
	mux.HandleFunc("DELETE /admin/fault", auth(func(w http.ResponseWriter, r *http.Request) {
		i.clear()
		httpx.Logger(r.Context()).Warn("fault cleared")
		w.WriteHeader(http.StatusNoContent)
	}))
}
