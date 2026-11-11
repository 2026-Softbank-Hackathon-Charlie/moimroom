package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

func TestAIRehearsalHost(t *testing.T) {
	if os.Getenv("AI_REHEARSAL_HOST") != "1" {
		t.Skip("explicit local rehearsal only")
	}
	root, err := base64.StdEncoding.DecodeString(os.Getenv("AI_REHEARSAL_ROOT"))
	if err != nil {
		t.Fatal("invalid root")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(root) {
		t.Fatal("private test root required")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	defer client.CloseIdleConnections()
	fixture := func(_ context.Context, id int64) (store.Space, error) {
		if id != 1 {
			return store.Space{}, store.ErrNotFound
		}
		return store.Space{ID: 1, Description: "조용한 모임 공간입니다.", Amenities: []string{"화이트보드"}, Rules: []string{"실내에서는 조용히 이용해 주세요."}}, nil
	}
	h, err := newAIBriefHandler(fixture, os.Getenv("AI_ENDPOINT"), os.Getenv("AI_GATEWAY_TOKEN"), client)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("POST /api/spaces/{id}/ai-brief", h)
	mux.HandleFunc("POST /__rehearsal_stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+os.Getenv("GATEWAY_ADMIN_TOKEN") {
			w.WriteHeader(401)
			return
		}
		select {
		case stop <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	server := httptest.NewServer(httpx.Middleware(slog.New(slog.NewJSONHandler(io.Discard, nil)), "rehearsal", "")(mux))
	defer server.Close()
	ready, _ := json.Marshal(map[string]any{"url": server.URL, "pid": os.Getpid(), "store": "synthetic public SPACE-1; no DB/booking integration", "tls": "private test root; verification enabled"})
	fmt.Println("AI_HOST " + string(ready))
	select {
	case <-stop:
	case <-time.After(180 * time.Second):
		t.Fatal("rehearsal host deadline")
	}
}
