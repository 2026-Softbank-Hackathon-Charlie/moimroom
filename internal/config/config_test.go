package config

import "testing"

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		secure bool
		err    bool
	}{
		{"http://minio:9000", "minio:9000", false, false},
		{"https://storage.googleapis.com", "storage.googleapis.com", true, false},
		{"s3.ap-northeast-2.amazonaws.com", "s3.ap-northeast-2.amazonaws.com", true, false},
		{"ftp://x", "", false, true},
	}
	for _, c := range cases {
		host, secure, err := ParseEndpoint(c.in)
		if (err != nil) != c.err || host != c.host || secure != c.secure {
			t.Errorf("%q: got (%q, %v, %v)", c.in, host, secure, err)
		}
	}
}

func TestLoadRequiresCoreSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_BUCKET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing env error")
	}
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("S3_ENDPOINT", "http://minio:9000")
	t.Setenv("S3_BUCKET", "photos")
	t.Setenv("SEED_SPACES", "false")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || c.SeedSpaces || c.S3.Secure {
		t.Fatalf("unexpected config %+v", c)
	}
}

func TestLoadAISettingsAreOptionalAndServerSide(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("S3_ENDPOINT", "http://minio:9000")
	t.Setenv("S3_BUCKET", "photos")
	t.Setenv("AI_ENDPOINT", "https://gateway.example/api/chat")
	t.Setenv("AI_TOKEN", "server-secret")
	c, err := Load()
	if err != nil || c.AIEndpoint != "https://gateway.example/api/chat" || c.AIToken != "server-secret" {
		t.Fatal("AI environment settings not loaded")
	}
	t.Setenv("AI_ENDPOINT", "")
	t.Setenv("AI_TOKEN", "")
	if _, err := Load(); err != nil {
		t.Fatal("disabled AI must not prevent app startup", err)
	}
}
