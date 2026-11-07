package api

import (
	"fmt"
	"testing"
)

func TestWrittenReceiptCommittedOnceWithReservation(t *testing.T) {
	e := setup(t)
	e.srv.store.ConfigureWrittenRecords("app-moimroom")
	body := booking(1, 10, 12, "2026-11-03")
	const key = "outbox-successful-write-0001"
	var id int64
	for range 2 {
		status, res := e.call("POST", "/api/reservations", body, "Idempotency-Key", key)
		if status != 201 {
			t.Fatalf("booking: %d %v", status, res)
		}
		id = int64(res["id"].(float64))
	}
	generation, records, err := e.srv.store.WrittenReceipts(t.Context(), 0)
	if err != nil || generation == "" || len(records) != 1 {
		t.Fatalf("receipts %v %v", records, err)
	}
	if records[0].PK != fmt.Sprint(id) || records[0].EventID != fmt.Sprintf("reservation-%d", id) || records[0].At.IsZero() {
		t.Fatal("receipt did not reference the real committed PK")
	}
	// A booking conflict and a retry-key mismatch must never create a receipt.
	if status, _ := e.call("POST", "/api/reservations", body, "Idempotency-Key", "outbox-conflicting-write-0002"); status != 409 {
		t.Fatal(status)
	}
	body["endHour"] = 13
	if status, _ := e.call("POST", "/api/reservations", body, "Idempotency-Key", key); status != 409 {
		t.Fatal(status)
	}
	_, records, err = e.srv.store.WrittenReceipts(t.Context(), 0)
	if err != nil || len(records) != 1 {
		t.Fatal("failed write created a receipt")
	}
}

func TestOutboxFailureRollsBackBookingAndRetry(t *testing.T) {
	e := setup(t)
	e.srv.store.ConfigureWrittenRecords("app-moimroom")
	_, err := e.pool.Exec(t.Context(), `CREATE FUNCTION app.reject_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic outbox failure'; END $$;
 CREATE TRIGGER reject_receipt BEFORE INSERT ON app.written_record_outbox FOR EACH ROW EXECUTE FUNCTION app.reject_receipt()`)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := e.call("POST", "/api/reservations", booking(1, 10, 12, "2026-11-03"), "Idempotency-Key", "outbox-rollback-write-0001"); status != 500 {
		t.Fatal(status)
	}
	var bookings, slots, retries, receipts int
	err = e.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM app.reservations),(SELECT count(*) FROM app.reservation_slots),(SELECT count(*) FROM app.reservation_retries),(SELECT count(*) FROM app.written_record_outbox)`).Scan(&bookings, &slots, &retries, &receipts)
	if err != nil || bookings != 0 || slots != 0 || retries != 0 || receipts != 0 {
		t.Fatalf("partial commit bookings=%d slots=%d retries=%d receipts=%d err=%v", bookings, slots, retries, receipts, err)
	}
}

func TestCollectionDisabledDoesNotEmitReceipts(t *testing.T) {
	e := setup(t)
	if status, _ := e.call("POST", "/api/reservations", booking(1, 10, 12, "2026-11-03")); status != 201 {
		t.Fatal(status)
	}
	var n int
	if err := e.pool.QueryRow(t.Context(), "SELECT count(*) FROM app.written_record_outbox").Scan(&n); err != nil || n != 0 {
		t.Fatalf("disabled collector: %d %v", n, err)
	}
}
