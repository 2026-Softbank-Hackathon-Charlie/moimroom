package credentialcheck

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/blob"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/config"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/pgdb"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestRuntimeCredentialsIntegration(t *testing.T) {
	dsn := os.Getenv("SECRET_CHECK_TEST_DSN")
	endpoint := os.Getenv("SECRET_CHECK_TEST_S3")
	if dsn == "" || endpoint == "" {
		t.Skip("disposable postgres and minio required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	key, secret := os.Getenv("SECRET_CHECK_TEST_KEY"), os.Getenv("SECRET_CHECK_TEST_PASSWORD")
	cfg := config.S3{Endpoint: endpoint, AccessKey: key, SecretKey: secret, Bucket: "runtime-check", PathStyle: "true"}
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(key, secret, ""), BucketLookup: minio.BucketLookupPath})
	if err != nil {
		t.Fatal("create test client")
	}
	if err := c.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal("create disposable test bucket")
	}
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid credentials", true: "invalid credentials"}[bad], func(t *testing.T) {
			useDSN, useCfg := dsn, cfg
			if bad {
				useDSN = os.Getenv("SECRET_CHECK_TEST_BAD_DSN")
				useCfg.SecretKey = "invalid-password"
			}
			bucket, err := blob.NewS3(useCfg)
			if err != nil {
				t.Fatal("create runtime client")
			}
			h := Handler(func(ctx context.Context, n string) error { return pgdb.CheckRuntime(ctx, useDSN, n) }, func(ctx context.Context, _ string) error { return bucket.CheckBucket(ctx) })
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/?challenge="+strings.Repeat("c", 64), nil))
			want := `"pass"`
			if bad {
				want = `"fail"`
			}
			if strings.Count(w.Body.String(), want) != 2 {
				t.Fatalf("unexpected verification status: %s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), "invalid-password") {
				t.Fatal("credential leaked")
			}
		})
	}
}
