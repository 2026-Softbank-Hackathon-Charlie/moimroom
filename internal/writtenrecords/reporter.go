// Package writtenrecords reports real committed reservation primary keys from
// a durable, append-only transactional outbox. It never exposes tokens to clients.
package writtenrecords

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

type Config struct{ URL, Stream, Token string }

func (c Config) Enabled() bool { return c.URL != "" }
func (c Config) Validate() error {
	if c.URL == "" && c.Stream == "" && c.Token == "" {
		return nil
	}
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(c.Stream) || len(c.Token) < 32 {
		return errors.New("written record collection requires a server URL, valid stream and a dedicated stream token of at least 32 characters")
	}
	return nil
}

type Source interface {
	WrittenReceiptPage(context.Context, int64, int64) (string, []store.WrittenReceipt, int64, error)
}
type Reporter struct {
	Config Config
	Source Source
	HTTP   *http.Client
	cutoff int64 // fixed upper sequence of the current bounded sweep
}

func (r *Reporter) Flush(ctx context.Context, after int64) (int64, error) {
	// Bound pool acquisition, source SELECTs and collector HTTP as one attempt.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	generation, rows, cutoff, err := r.Source.WrittenReceiptPage(ctx, after, r.cutoff)
	if err != nil {
		return after, err
	}
	if len(rows) == 0 {
		// Sequences allocate before commit: a lower sequence can become visible
		// after a higher one. Rescan each completed sweep, with server deduplication.
		r.cutoff = 0
		return 0, nil
	}
	r.cutoff = cutoff
	type record struct {
		Table   string    `json:"table"`
		PK      string    `json:"pk"`
		EventID string    `json:"eventId"`
		At      time.Time `json:"at"`
	}
	batch := struct {
		Records    []record `json:"records"`
		Scope      string   `json:"scope"`
		Coverage   string   `json:"coverage"`
		Generation string   `json:"generation"`
	}{Scope: "application_commits", Coverage: "reservation_inserts", Generation: generation}
	for _, row := range rows {
		batch.Records = append(batch.Records, record{Table: "app.reservations", PK: row.PK, EventID: row.EventID, At: row.At})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return after, errors.New("encode write receipts")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(r.Config.URL, "/")+"/v1/written-records/"+url.PathEscape(r.Config.Stream), bytes.NewReader(body))
	if err != nil {
		return after, errors.New("create write receipt request")
	}
	req.Header.Set("Authorization", "Bearer "+r.Config.Token)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 3 * time.Second}
	if r.HTTP != nil {
		client = *r.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return after, errors.New("write receipt collector unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return after, errors.New("write receipt collector rejected batch")
	}
	var ack struct {
		Accepted int `json:"accepted"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&ack) != nil || ack.Accepted != len(rows) {
		return after, errors.New("write receipt collector did not acknowledge the batch")
	}
	return rows[len(rows)-1].Seq, nil
}
func (r *Reporter) Run(ctx context.Context, log *slog.Logger) {
	var after int64
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		next, err := r.Flush(ctx, after)
		if err != nil && ctx.Err() == nil {
			log.Warn("write receipt collection retry", "stream", r.Config.Stream)
		} else {
			after = next
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
