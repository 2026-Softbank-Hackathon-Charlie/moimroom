package writtenrecords

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

type sourceFunc func(context.Context, int64) (string, []store.WrittenReceipt, error)

func (f sourceFunc) WrittenReceiptPage(c context.Context, n, cutoff int64) (string, []store.WrittenReceipt, int64, error) {
	generation, rows, err := f(c, n)
	return generation, rows, 99, err
}

func TestReporterRetriesExactCommittedPKAndRequiresFullAck(t *testing.T) {
	var bodies []string
	var status = 503
	ack := `{"accepted":1}`
	token := strings.Repeat("s", 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/written-records/app-shop" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("wrong collector target/auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		b, _ := json.Marshal(body)
		bodies = append(bodies, string(b))
		rec := body["records"].([]any)[0].(map[string]any)
		if rec["table"] != "app.reservations" || rec["pk"] != "711" || rec["eventId"] != "reservation-711" || body["scope"] != "application_commits" || body["generation"] != "db-1" {
			t.Errorf("not actual receipt: %s", b)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(ack))
	}))
	defer srv.Close()
	reporter := Reporter{Config: Config{URL: srv.URL, Stream: "app-shop", Token: token}, Source: sourceFunc(func(_ context.Context, after int64) (string, []store.WrittenReceipt, error) {
		if after != 0 {
			t.Fatal("advanced before acknowledgement")
		}
		return "db-1", []store.WrittenReceipt{{Seq: 9, EventID: "reservation-711", PK: "711", At: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)}}, nil
	})}
	if next, err := reporter.Flush(t.Context(), 0); err == nil || next != 0 {
		t.Fatal("advanced after failure")
	}
	status = 202
	ack = `{"accepted":0}`
	if next, err := reporter.Flush(t.Context(), 0); err == nil || next != 0 {
		t.Fatal("advanced after partial acknowledgement")
	}
	ack = `{"accepted":1}`
	if next, err := reporter.Flush(t.Context(), 0); err != nil || next != 9 {
		t.Fatalf("full acknowledgement: %d %v", next, err)
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatal("retry receipt changed")
	}
}

func TestReporterDoesNotForwardTokenOnRedirect(t *testing.T) {
	var reached bool
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer dest.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dest.URL, 307) }))
	defer srv.Close()
	r := Reporter{Config: Config{URL: srv.URL, Stream: "app-shop", Token: strings.Repeat("s", 32)}, Source: sourceFunc(func(context.Context, int64) (string, []store.WrittenReceipt, error) {
		return "db-1", []store.WrittenReceipt{{Seq: 1, PK: "1", EventID: "reservation-1"}}, nil
	})}
	if next, err := r.Flush(t.Context(), 0); err == nil || next != 0 || reached {
		t.Fatal("redirect escaped collector boundary")
	}
}

func TestCompletedSweepRestartsToCatchLateCommits(t *testing.T) {
	r := Reporter{Source: sourceFunc(func(context.Context, int64) (string, []store.WrittenReceipt, error) { return "db-1", nil, nil })}
	if next, err := r.Flush(t.Context(), 99); err != nil || next != 0 {
		t.Fatal("misses lower sequence committed after a higher sequence")
	}
}

func TestConfigIsAllOrNone(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("s", 32)
	for _, c := range []Config{{URL: "https://collector", Stream: "app-shop"}, {URL: "https://user:secret@collector", Stream: "app-shop", Token: token}, {URL: "https://collector?token=secret", Stream: "app-shop", Token: token}} {
		if err := c.Validate(); err == nil || strings.Contains(err.Error(), token) {
			t.Fatal("invalid or leaked collector configuration")
		}
	}
}

func TestReporterBoundsSourceReadAndPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	r := Reporter{Source: sourceFunc(func(ctx context.Context, _ int64) (string, []store.WrittenReceipt, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 3*time.Second {
			t.Fatal("source read has no bounded attempt deadline")
		}
		<-ctx.Done()
		return "", nil, ctx.Err()
	})}
	if after, err := r.Flush(ctx, 7); err == nil || after != 7 {
		t.Fatalf("source cancellation: %d %v", after, err)
	}
}
