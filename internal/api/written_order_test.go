package api

import "testing"

func TestWrittenReceiptsRescanOutOfOrderCommit(t *testing.T) {
	e := setup(t)
	e.srv.store.ConfigureWrittenRecords("app-moimroom")
	ctx := t.Context()
	slow, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Rollback(ctx)
	var lower, higher int64
	if err := slow.QueryRow(ctx, `INSERT INTO app.written_record_outbox(stream,event_id,pk) VALUES('app-moimroom','reservation-1','1') RETURNING seq`).Scan(&lower); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `INSERT INTO app.written_record_outbox(stream,event_id,pk) VALUES('app-moimroom','reservation-2','2') RETURNING seq`).Scan(&higher); err != nil {
		t.Fatal(err)
	}
	_, rows, cutoff, err := e.srv.store.WrittenReceiptPage(ctx, 0, 0)
	if err != nil || len(rows) != 1 || rows[0].Seq != higher || higher <= lower {
		t.Fatalf("initial visible receipt %+v %v", rows, err)
	}
	if err := slow.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// More traffic arrives while this sweep is active. It cannot prevent the
	// bounded sweep from completing, or starve the lower late-committing row.
	if _, err := e.pool.Exec(ctx, `INSERT INTO app.written_record_outbox(stream,event_id,pk) SELECT 'app-moimroom','reservation-'||n,n::text FROM generate_series(3,202) n`); err != nil {
		t.Fatal(err)
	}
	_, rows, _, err = e.srv.store.WrittenReceiptPage(ctx, higher, cutoff)
	if err != nil || len(rows) != 0 {
		t.Fatal("expected sweep completion")
	}
	// Reporter resets its read-only cursor to zero on this empty page; stable
	// event IDs let the collector deduplicate the already reported higher row.
	_, rows, err = e.srv.store.WrittenReceipts(ctx, 0)
	if err != nil || len(rows) != 100 || rows[0].Seq != lower {
		t.Fatalf("late commit lost on rescan: %+v %v", rows, err)
	}
}
