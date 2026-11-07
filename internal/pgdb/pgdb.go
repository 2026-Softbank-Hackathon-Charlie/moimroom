// Package pgdb opens the PostgreSQL pool and applies the
// embedded schema.
package pgdb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckRuntime authenticates a fresh connection instead of trusting a pooled
// session established before a credential rotation. The query changes no data.
func CheckRuntime(ctx context.Context, url, challenge string) error {
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	var got string
	if err := c.QueryRow(ctx, "SELECT $1::text", challenge).Scan(&got); err != nil {
		return err
	}
	if got != challenge {
		return fmt.Errorf("database challenge mismatch")
	}
	return nil
}

// Open connects and waits up to a minute for the database, so a pod that
// starts before its database does not crash-loop.
func Open(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	// Short lifetimes make pooled connections re-dial often, so a DB-blocking
	// NetworkPolicy (which spares established flows on most CNIs) takes effect
	// quickly and DNS changes are picked up.
	cfg.MaxConnLifetime = 30 * time.Second
	cfg.MaxConnLifetimeJitter = 5 * time.Second
	cfg.MaxConnIdleTime = 15 * time.Second
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(time.Minute)
	for {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("database not reachable: %w", err)
		}
		log.Warn("waiting for database", "err", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
		}
	}
}

// Migrate runs the statements in one transaction under an advisory lock, so
// replicas starting together do not race on DDL or seed rows.
func Migrate(ctx context.Context, pool *pgxpool.Pool, lockKey int64, stmts ...string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return tx.Commit(ctx)
}
