package store

import (
	"context"
	"time"
)

// ConfigureWrittenRecords is startup-only. Only successful reservation INSERT
// transactions emit receipts; client response delivery and other CRUD are not covered.
func (st *Store) ConfigureWrittenRecords(stream string) { st.writtenStream = stream }

type WrittenReceipt struct {
	Seq     int64
	EventID string
	PK      string
	At      time.Time
}

func (st *Store) WrittenReceipts(ctx context.Context, after int64) (string, []WrittenReceipt, error) {
	generation, rows, _, err := st.WrittenReceiptPage(ctx, after, 0)
	return generation, rows, err
}

// WrittenReceiptPage fixes an upper sequence for one bounded sweep. A new sweep
// starts with cutoff zero. Sequence allocation is not commit ordering, so every
// completed sweep must start again at zero, including during continuous writes.
func (st *Store) WrittenReceiptPage(ctx context.Context, after, cutoff int64) (string, []WrittenReceipt, int64, error) {
	var recovering bool
	if err := st.pool.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&recovering); err != nil {
		return "", nil, cutoff, err
	}
	if recovering {
		return "", nil, 0, nil
	}
	var generation string
	if err := st.pool.QueryRow(ctx, `SELECT generation::text FROM app.write_collection_identity WHERE id=true`).Scan(&generation); err != nil {
		return "", nil, cutoff, err
	}
	if cutoff == 0 {
		if err := st.pool.QueryRow(ctx, `SELECT coalesce(max(seq),0) FROM app.written_record_outbox WHERE stream=$1`, st.writtenStream).Scan(&cutoff); err != nil {
			return "", nil, 0, err
		}
	}
	rows, err := st.pool.Query(ctx, `SELECT seq,event_id,pk,recorded_at FROM app.written_record_outbox WHERE stream=$1 AND seq>$2 AND seq<=$3 ORDER BY seq LIMIT 100`, st.writtenStream, after, cutoff)
	if err != nil {
		return "", nil, cutoff, err
	}
	defer rows.Close()
	out := []WrittenReceipt{}
	for rows.Next() {
		var r WrittenReceipt
		if err := rows.Scan(&r.Seq, &r.EventID, &r.PK, &r.At); err != nil {
			return "", nil, cutoff, err
		}
		out = append(out, r)
	}
	return generation, out, cutoff, rows.Err()
}
