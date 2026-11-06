package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReservationCommitThenNetworkResponseLoss(t *testing.T) {
	e := setup(t)
	var drop atomic.Bool
	drop.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := httptest.NewRecorder()
		e.h.ServeHTTP(recorder, r)
		if recorder.Code == 201 && drop.Swap(false) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	}))
	t.Cleanup(server.Close)
	body, _ := json.Marshal(booking(1, 10, 12, "2026-11-03"))
	call := func() (*http.Response, error) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/reservations", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "sandbox-network-loss-0001")
		return server.Client().Do(req)
	}
	response, err := call()
	if err == nil {
		response.Body.Close()
		t.Fatal("expected lost response after commit")
	}
	response, err = call()
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal(response.StatusCode)
	}
	var count int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM app.reservations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReservationIdempotencyConcurrentAndResponseLost(t *testing.T) {
	e := setup(t)
	body := booking(1, 10, 12, "2026-11-03")
	const key = "sandbox-retry-key-0001"
	var wg sync.WaitGroup
	codes := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			status, res := e.call("POST", "/api/reservations", body, "Idempotency-Key", key)
			if status != 201 {
				t.Errorf("status=%d body=%v", status, res)
				return
			}
			codes <- res["code"].(string)
		})
	}
	wg.Wait()
	close(codes)
	first := ""
	for code := range codes {
		if first == "" {
			first = code
		}
		if code != first {
			t.Fatal("different reservation for same key")
		}
	}
	var reservations, receipts int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM app.reservations").Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM app.reservation_retries").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if reservations != 1 || receipts != 1 {
		t.Fatalf("reservations=%d receipts=%d", reservations, receipts)
	}
	var keyHash, requestHash, responseText string
	if err := e.pool.QueryRow(context.Background(), "SELECT key_hash,request_hash,response::text FROM app.reservation_retries").Scan(&keyHash, &requestHash, &responseText); err != nil {
		t.Fatal(err)
	}
	if keyHash == key || !strings.HasPrefix(requestHash, "pbkdf2-sha256$100000$") || strings.Contains(responseText, "01012345678") || strings.Contains(responseText, "010-1234-5678") {
		t.Fatal("retry receipt contains raw key/phone or an unprotected request digest")
	}
	// A lost response is retried with the same key; even after the date's
	// validation window has passed, the committed receipt is returned.
	e.srv.now = func() time.Time { return fixedNow.Add(90 * 24 * time.Hour) }
	status, res := e.call("POST", "/api/reservations", body, "Idempotency-Key", key)
	if status != 201 || res["code"] != first {
		t.Fatalf("retry=%d %v", status, res)
	}
	body["headcount"] = 3
	if status, _ := e.call("POST", "/api/reservations", body, "Idempotency-Key", key); status != 409 {
		t.Fatalf("changed body status=%d", status)
	}
}

func TestReservationFailedAttemptDoesNotReserveRetryKey(t *testing.T) {
	e := setup(t)
	body := booking(1, 10, 12, "2026-11-03")
	if status, _ := e.call("POST", "/api/reservations", body); status != 201 {
		t.Fatal(status)
	}
	if status, _ := e.call("POST", "/api/reservations", body, "Idempotency-Key", "sandbox-conflict-key-1"); status != 409 {
		t.Fatal(status)
	}
	var count int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM app.reservation_retries").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed write left a receipt: %d %v", count, err)
	}
}
