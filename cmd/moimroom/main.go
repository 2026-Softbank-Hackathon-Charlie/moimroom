// Command moimroom serves the space booking site: the embedded SPA plus the
// JSON API under /api.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/api"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/blob"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/config"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/credentialcheck"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/fault"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/pgdb"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/spa"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

func main() {
	cfg, err := config.Load()
	log := httpx.NewLogger(cfg.Instance, cfg.RegionLabel)
	if err == nil {
		err = run(cfg, log)
	}
	if err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgdb.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	ran, err := store.Migrate(ctx, pool, cfg.SeedSpaces)
	if err != nil {
		return err
	}
	if !ran {
		log.Info("database is a standby; schema migration skipped (it replicates from the primary)")
	}
	st := store.New(pool)

	photos, err := blob.NewS3(cfg.S3)
	if err != nil {
		return err
	}
	if err := photos.CheckBucket(ctx); err != nil {
		// Not fatal: readiness keeps the pod out of rotation until it works.
		log.Warn("object storage check failed", "err", err)
	}

	probes := httpx.NewProbes(func(ctx context.Context) error {
		if err := st.Ping(ctx); err != nil {
			return fmt.Errorf("database: %w", err)
		}
		if err := photos.CheckBucket(ctx); err != nil {
			return fmt.Errorf("object storage: %w", err)
		}
		return nil
	})
	inj := fault.New()

	mux := http.NewServeMux()
	probes.Register(mux)
	// This read-only check uses fresh DB authentication and the application's
	// configured signed bucket client. It reports no credentials/provider errors.
	mux.Handle("GET /internal/secret-usage", credentialcheck.Handler(func(ctx context.Context, challenge string) error {
		return pgdb.CheckRuntime(ctx, cfg.DatabaseURL, challenge)
	}, func(ctx context.Context, _ string) error {
		if cfg.S3.AccessKey == "" || cfg.S3.SecretKey == "" {
			return credentialcheck.ErrNotConfigured
		}
		return photos.CheckBucket(ctx)
	}))
	inj.Register(mux, cfg.AdminToken)
	notFound := func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusNotFound, "요청한 주소를 찾을 수 없습니다.")
	}
	mux.HandleFunc("/admin", notFound)
	mux.HandleFunc("/admin/", notFound)
	api.New(st, photos, cfg.HostCode).Register(mux)
	if err := api.RegisterAIBrief(mux, st.GetSpace, cfg.AIEndpoint, cfg.AIToken); err != nil {
		log.Warn("AI guide disabled", "reason", "invalid AI configuration")
	}
	mux.Handle("/", spa.Handler(spa.Dist()))

	handler := httpx.Middleware(log, cfg.Instance, cfg.RegionLabel)(inj.Middleware(mux))
	return httpx.Serve(ctx, log, httpx.NewServer(":"+cfg.Port, handler), probes)
}
