package api

import (
	"context"
	"encoding/json"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"os"
	"testing"
	"time"
)

// The sandbox runner performs a real pg_dump/pg_restore between these phases.
// The oracle is outside PostgreSQL and contains only a synthetic booking code.
func TestSandboxBackupRestoreRetry(t *testing.T) {
	stage := os.Getenv("MOIMROOM_RESTORE_STAGE")
	if stage == "" {
		t.Skip("run sandbox backup/restore harness")
	}
	oracle := os.Getenv("MOIMROOM_RESTORE_ORACLE")
	if oracle == "" {
		t.Fatal("independent oracle path required")
	}
	body := booking(1, 10, 12, "2026-11-03")
	body["guestName"] = "sandbox"
	const key = "sandbox-backup-restore-0001"
	if stage == "prepare" {
		e := setup(t)
		status, res := e.call("POST", "/api/reservations", body, "Idempotency-Key", key)
		if status != 201 {
			t.Fatal(status)
		}
		b, _ := json.Marshal(res["code"])
		if err := os.WriteFile(oracle, b, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if stage != "replay" {
		t.Fatal("invalid restore stage")
	}
	pool, err := pgxpool.New(context.Background(), os.Getenv("MOIMROOM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	srv := New(store.New(pool), nil, testHostCode)
	srv.now = func() time.Time { return fixedNow }
	mux := http.NewServeMux()
	srv.Register(mux)
	e := &env{t: t, h: mux, srv: srv, pool: pool}
	b, err := os.ReadFile(oracle)
	if err != nil {
		t.Fatal(err)
	}
	var want string
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	status, res := e.call("POST", "/api/reservations", body, "Idempotency-Key", key)
	if status != 201 || res["code"] != want {
		t.Fatalf("restored retry status=%d does not match independent oracle", status)
	}
	var reservations, receipts int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM app.reservations").Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM app.reservation_retries").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if reservations != 1 || receipts != 1 {
		t.Fatalf("reservations=%d receipts=%d", reservations, receipts)
	}
	t.Log("restored acknowledged reservation preserved; same-key retry did not duplicate the booking")
}
