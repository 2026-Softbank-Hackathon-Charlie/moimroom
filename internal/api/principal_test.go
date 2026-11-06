package api

import (
	"context"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestActualApplicationDatabasePrincipal(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='moimroom_sandbox_app') THEN
 CREATE ROLE moimroom_sandbox_app LOGIN PASSWORD 'sandbox-app-test-only';
 END IF; END $$;
 GRANT USAGE ON SCHEMA app TO moimroom_sandbox_app;
 GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA app TO moimroom_sandbox_app;
 GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA app TO moimroom_sandbox_app;`)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("MOIMROOM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "moimroom_sandbox_app"
	cfg.ConnConfig.Password = "sandbox-app-test-only"
	appPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer appPool.Close()
	var principal string
	if err := appPool.QueryRow(ctx, "SELECT current_user").Scan(&principal); err != nil || principal != "moimroom_sandbox_app" {
		t.Fatal("app principal not established")
	}
	if _, err := appPool.Exec(ctx, "SELECT * FROM pg_authid"); err == nil {
		t.Fatal("app principal can read protected credential catalog")
	}
	e.srv.store = store.New(appPool)
	if status, _ := e.call("POST", "/api/reservations", booking(1, 10, 12, "2026-11-03"), "Idempotency-Key", "sandbox-principal-retry-0001"); status != 201 {
		t.Fatal(status)
	}
	if status, _ := e.call("GET", "/api/spaces", nil); status != 200 {
		t.Fatal(status)
	}
	t.Log("real app DB principal: read/write allowed; protected credential catalog denied; credential value omitted")
}
