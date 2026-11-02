// Package config reads settings from environment variables only, so the same
// image runs unchanged everywhere.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port        string
	DatabaseURL string
	S3          S3
	HostCode    string
	AdminToken  string
	RegionLabel string
	Instance    string
	// SeedSpaces controls the first-start catalog. Turn it off for a database
	// that will receive its rows from elsewhere (restore, replication).
	SeedSpaces bool
}

// S3 describes an S3-compatible bucket (AWS S3, GCS interoperability, MinIO).
type S3 struct {
	Endpoint  string // host[:port], scheme stripped
	Secure    bool
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle string // "true", "false" or "" for auto
}

func Load() (Config, error) {
	c := Config{
		Port:        get("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HostCode:    os.Getenv("HOST_CODE"),
		AdminToken:  os.Getenv("ADMIN_TOKEN"),
		RegionLabel: os.Getenv("SITE_REGION_LABEL"),
		SeedSpaces:  true,
	}
	if v := os.Getenv("SEED_SPACES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("SEED_SPACES: %w", err)
		}
		c.SeedSpaces = b
	}
	if h, err := os.Hostname(); err == nil {
		c.Instance = h
	} else {
		c.Instance = "unknown"
	}

	endpoint, secure, err := ParseEndpoint(os.Getenv("S3_ENDPOINT"))
	if err != nil {
		return c, err
	}
	c.S3 = S3{
		Endpoint:  endpoint,
		Secure:    secure,
		Region:    os.Getenv("S3_REGION"),
		Bucket:    os.Getenv("S3_BUCKET"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
		PathStyle: strings.ToLower(os.Getenv("S3_PATH_STYLE")),
	}
	switch c.S3.PathStyle {
	case "", "true", "false":
	default:
		return c, fmt.Errorf("S3_PATH_STYLE must be true, false or empty, got %q", c.S3.PathStyle)
	}

	var missing []string
	for _, kv := range [][2]string{
		{"DATABASE_URL", c.DatabaseURL}, {"S3_ENDPOINT", c.S3.Endpoint}, {"S3_BUCKET", c.S3.Bucket},
	} {
		if kv[1] == "" {
			missing = append(missing, kv[0])
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

// ParseEndpoint accepts "https://host", "http://host:9000" or a bare
// "host:port". minio-go wants the host alone plus a TLS flag; a bare host
// defaults to TLS because every cloud endpoint uses it.
func ParseEndpoint(raw string) (host string, secure bool, err error) {
	if raw == "" {
		return "", true, nil
	}
	if !strings.Contains(raw, "://") {
		return strings.TrimRight(raw, "/"), true, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("S3_ENDPOINT: %w", err)
	}
	switch u.Scheme {
	case "http":
		return u.Host, false, nil
	case "https":
		return u.Host, true, nil
	default:
		return "", false, fmt.Errorf("S3_ENDPOINT: unsupported scheme %q", u.Scheme)
	}
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
