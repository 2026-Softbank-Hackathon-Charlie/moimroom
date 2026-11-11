package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

const guideText = "요약: 조용한 모임 공간입니다.\n시설: 화이트보드가 있습니다.\n주의: 실내에서는 조용히 이용해 주세요.\n[SPACE-1]"

func goodSpace(context.Context, int64) (store.Space, error) {
	return store.Space{ID: 1, Description: "조용한 모임 공간입니다.", Amenities: []string{"화이트보드"}, Rules: []string{"실내에서는 조용히 이용해 주세요."}}, nil
}

func aiTestHandler(t *testing.T, upstream http.HandlerFunc, getSpace func(context.Context, int64) (store.Space, error)) (*aiBriefHandler, http.Handler) {
	t.Helper()
	ts := httptest.NewTLSServer(upstream)
	t.Cleanup(ts.Close)
	h, err := newAIBriefHandler(getSpace, ts.URL+"/api/chat", "test-secret", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/spaces/{id}/ai-brief", h)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return h, httpx.Middleware(log, "test", "")(mux)
}

func aiCall(handler http.Handler, path, body string, ctx context.Context) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set(httpx.HeaderRequestID, "ai-demo-001")
	req.Header.Set("X-Hop-Backend", "client-forged")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestAIBriefSuccessAndFixedPayload(t *testing.T) {
	_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" || r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get(httpx.HeaderRequestID) != "ai-demo-001" || r.Header.Get("X-Hop-Backend") != "" {
			t.Error("request contract or credential/provenance boundary failed")
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			KeepAlive string `json:"keep_alive"`
			Stream    bool   `json:"stream"`
			Options   struct {
				Context     int `json:"num_ctx"`
				Predict     int `json:"num_predict"`
				Temperature int `json:"temperature"`
				Seed        int `json:"seed"`
			} `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Model != aiModel || payload.Stream || payload.Options.Context != 2048 || payload.Options.Predict != 128 || payload.Options.Temperature != 0 || payload.Options.Seed != 42 || payload.KeepAlive != "60s" || len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[0].Content != aiSystem || payload.Messages[1].Role != "user" || !strings.HasPrefix(payload.Messages[1].Content, "[SPACE-1]\n") {
			t.Error("model, limits, or document header changed")
		}
		w.Header().Set("X-Hop-Backend", "gcp")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": guideText}, "done": true})
	}, goodSpace)
	w := aiCall(handler, "/api/spaces/1/ai-brief", "", context.Background())
	var out aiBrief
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || out.Text != guideText || out.RequestID != "ai-demo-001" || out.Backend != "gcp" || out.DurationMS < 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response: %d %+v", w.Code, out)
	}
}

func TestAIPromptPublicFieldsAndBounds(t *testing.T) {
	prompt, err := aiPrompt(store.Space{ID: 1, Description: "설명", Name: "private-marker", Capacity: 12345, PricePerHour: 98765})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-marker", "12345", "98765", "guestName", "phone", "reservation"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("unneeded field leaked: %s", forbidden)
		}
	}
	if !strings.Contains(prompt, `"amenities":[],"rules":[]`) || !strings.Contains(aiSystem, "확인된 정보 없음") {
		t.Fatal("unknown-field instruction or empty evidence missing")
	}
	if _, err := aiPrompt(store.Space{ID: 1, Description: " ", Amenities: []string{" "}}); err == nil {
		t.Fatal("empty fields must fail")
	}
	if _, err := aiPrompt(store.Space{ID: 1, Description: strings.Repeat("가", 2001)}); err == nil {
		t.Fatal("Unicode length limit must fail")
	}
	if _, err := aiPrompt(store.Space{ID: 1, Description: strings.Repeat("가", 1200)}); err != nil {
		t.Fatal("Unicode count must not use UTF-8 byte length")
	}
}

func TestAIBriefValidation(t *testing.T) {
	cases := []struct {
		name, path, body string
		get              func(context.Context, int64) (store.Space, error)
		status           int
	}{
		{"invalid", "/api/spaces/nope/ai-brief", "", goodSpace, 404},
		{"overflow", "/api/spaces/9223372036854775808/ai-brief", "", goodSpace, 404},
		{"body", "/api/spaces/1/ai-brief", "{}", goodSpace, 400},
		{"query", "/api/spaces/1/ai-brief?model=other", "", goodSpace, 400},
		{"stale", "/api/spaces/9223372036854775807/ai-brief", "", func(context.Context, int64) (store.Space, error) { return store.Space{}, store.ErrNotFound }, 404},
		{"empty", "/api/spaces/1/ai-brief", "", func(context.Context, int64) (store.Space, error) { return store.Space{ID: 1}, nil }, 422},
		{"length", "/api/spaces/1/ai-brief", "", func(context.Context, int64) (store.Space, error) {
			return store.Space{ID: 1, Description: strings.Repeat("가", 2001)}, nil
		}, 422},
		{"store-error", "/api/spaces/1/ai-brief", "", func(context.Context, int64) (store.Space, error) {
			return store.Space{}, errors.New("store unavailable")
		}, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, handler := aiTestHandler(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid input reached inference") }, tc.get)
			w := aiCall(handler, tc.path, tc.body, context.Background())
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != tc.status || len(body) != 1 || body["error"] == "" || w.Header().Get(httpx.HeaderRequestID) != "ai-demo-001" {
				t.Fatalf("error contract failed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAIBriefUpstreamFailures(t *testing.T) {
	cases := []struct {
		name, backend, body string
		upstream, status    int
	}{
		{"failure", "gcp", "unavailable", 503, 502},
		{"busy", "gcp", "busy", 429, 429},
		{"redirect", "gcp", "", 302, 502},
		{"missing-provenance", "", `{"message":{"content":"ok"},"done":true}`, 200, 502},
		{"wrong-provenance", "current-route", `{"message":{"content":"ok"},"done":true}`, 200, 502},
		{"invalid-json", "gcp", "{", 200, 502},
		{"not-done", "gcp", `{"message":{"content":"ok"},"done":false}`, 200, 502},
		{"empty-output", "gcp", `{"message":{"content":""},"done":true}`, 200, 502},
		{"missing-citation", "gcp", `{"message":{"content":"요약: 안녕하세요"},"done":true}`, 200, 502},
		{"wrong-citation", "gcp", `{"message":{"content":"요약\n[SPACE-2]"},"done":true}`, 200, 502},
		{"long-output", "gcp", `{"message":{"content":"` + strings.Repeat("가", 601) + `\n[SPACE-1]"},"done":true}`, 200, 502},
		{"large-response", "gcp", strings.Repeat("x", 16385), 200, 502},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.backend != "" {
					w.Header().Set("X-Hop-Backend", tc.backend)
				}
				if tc.upstream == 302 {
					w.Header().Set("Location", "https://invalid.example/api/chat")
				}
				w.WriteHeader(tc.upstream)
				_, _ = io.WriteString(w, tc.body)
			}, goodSpace)
			w := aiCall(handler, "/api/spaces/1/ai-brief", "", context.Background())
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
}

func TestAIBriefDeadlineAndAdmission(t *testing.T) {
	_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); <-r.Context().Done() }, goodSpace)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if w := aiCall(handler, "/api/spaces/1/ai-brief", "", ctx); w.Code != 504 || time.Since(start) > time.Second {
		t.Fatal("inherited deadline must bound request and return 504")
	}
	h, busy := aiTestHandler(t, func(http.ResponseWriter, *http.Request) { t.Error("full app admission reached inference") }, goodSpace)
	h.slots <- struct{}{}
	h.slots <- struct{}{}
	if w := aiCall(busy, "/api/spaces/1/ai-brief", "", context.Background()); w.Code != 429 {
		t.Fatal("app must cap aggregate to two")
	}
	<-h.slots
	<-h.slots
}

func TestAIBriefDisabledAndConfiguration(t *testing.T) {
	for _, pair := range [][2]string{{"", ""}, {"http://unprotected/api/chat", "token"}, {"https://example/api/chat", ""}, {"https://example/api/chat?model=x", "token"}, {"https://example/api/chat", "bad\r\ntoken"}} {
		mux := http.NewServeMux()
		err := RegisterAIBrief(mux, goodSpace, pair[0], pair[1])
		if (pair[0] != "" || pair[1] != "") && err == nil {
			t.Fatal("invalid configuration accepted")
		}
		w := aiCall(mux, "/api/spaces/1/ai-brief", "", context.Background())
		if w.Code != 503 {
			t.Fatalf("disabled AI must return 503, got %d", w.Code)
		}
	}
}

func TestAIBriefOwnBudget(t *testing.T) {
	get := func(ctx context.Context, id int64) (store.Space, error) {
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining > aiBudget || remaining < aiBudget-time.Second {
			t.Fatal("AI handler must install a 12s child deadline")
		}
		return goodSpace(ctx, id)
	}
	_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Hop-Backend", "laptop")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": guideText}, "done": true})
	}, get)
	w := aiCall(handler, "/api/spaces/1/ai-brief", "", context.Background())
	var out aiBrief
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Backend != "laptop" {
		t.Fatal("laptop provenance or deadline failed")
	}
}

func TestAIBriefFixtureMatchesRequestAndDynamicCitation(t *testing.T) {
	fixture, err := os.ReadFile("../../docs/ai/warmup-request.json")
	if err != nil {
		t.Fatal(err)
	}
	_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		var got, want any
		if json.NewDecoder(r.Body).Decode(&got) != nil || json.Unmarshal(fixture, &want) != nil {
			t.Error("invalid fixture/request")
		}
		g, _ := json.Marshal(got)
		v, _ := json.Marshal(want)
		if !bytes.Equal(g, v) {
			t.Error("warmup differs from actual app request")
		}
		w.Header().Set("X-Hop-Backend", "gcp")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": guideText}, "done": true})
	}, goodSpace)
	if w := aiCall(handler, "/api/spaces/1/ai-brief", "", context.Background()); w.Code != 200 {
		t.Fatal(w.Code)
	}
	_, dynamic := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		var payload struct{ Messages []struct{ Content string } }
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.Messages) != 2 || !strings.HasPrefix(payload.Messages[1].Content, "[SPACE-9223372036854775807]\n") {
			t.Error("citation must preserve int64 ID")
		}
		w.Header().Set("X-Hop-Backend", "laptop")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "요약: 공간\n시설: 확인된 정보 없음\n주의: 확인된 정보 없음\n[SPACE-9223372036854775807]"}, "done": true})
	}, func(_ context.Context, id int64) (store.Space, error) {
		return store.Space{ID: id, Description: "공간"}, nil
	})
	if w := aiCall(dynamic, "/api/spaces/9223372036854775807/ai-brief", "", context.Background()); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestAIBriefRejectsAmbiguousProvenanceAndNeverRetries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		status int
		body   string
	}{
		{"duplicate", []string{"gcp", "gcp"}, 200, `{"message":{"content":"ok\n[SPACE-1]"},"done":true}`},
		{"combined", []string{"gcp, laptop"}, 200, `{"message":{"content":"ok\n[SPACE-1]"},"done":true}`},
		{"untrusted-busy", nil, 429, "busy"},
		{"invalid-utf8", []string{"gcp"}, 200, "{\"message\":{\"content\":\"" + string([]byte{0xff}) + "\\n[SPACE-1]\"},\"done\":true}"},
		{"trailing-json", []string{"gcp"}, 200, `{"message":{"content":"ok\n[SPACE-1]"},"done":true}{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				for _, v := range tc.values {
					w.Header().Add("X-Hop-Backend", v)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}, goodSpace)
			w := aiCall(handler, "/api/spaces/1/ai-brief", "", context.Background())
			if w.Code != 502 || calls.Load() != 1 {
				t.Fatalf("status=%d attempts=%d", w.Code, calls.Load())
			}
		})
	}
	for _, status := range []int{302, 307, 308, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("X-Hop-Backend", "gcp")
				w.Header().Set("Location", "/redirect-target")
				w.WriteHeader(status)
			}, goodSpace)
			if w := aiCall(handler, "/api/spaces/1/ai-brief", "", context.Background()); w.Code != 502 || calls.Load() != 1 {
				t.Fatalf("redirect/retry: status=%d attempts=%d", w.Code, calls.Load())
			}
		})
	}
}

func TestAIPromptExactScalarBudget(t *testing.T) {
	base, err := aiPrompt(store.Space{ID: 1, Description: "가"})
	if err != nil {
		t.Fatal(err)
	}
	available := 2000 - utf8.RuneCountInString(aiSystem) - utf8.RuneCountInString(base) + 1
	if _, err := aiPrompt(store.Space{ID: 1, Description: strings.Repeat("😀", available)}); err != nil {
		t.Fatal("exact scalar budget rejected", err)
	}
	if _, err := aiPrompt(store.Space{ID: 1, Description: strings.Repeat("😀", available+1)}); err == nil {
		t.Fatal("excess scalar accepted")
	}
}

func TestAIBriefAdmissionIncludesStoreAndReleasesOnCancel(t *testing.T) {
	entered := make(chan struct{}, 2)
	get := func(ctx context.Context, id int64) (store.Space, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return store.Space{}, ctx.Err()
	}
	h, handler := aiTestHandler(t, func(http.ResponseWriter, *http.Request) { t.Error("cancelled read reached inference") }, get)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- aiCall(handler, "/api/spaces/1/ai-brief", "", ctx).Code }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("request did not enter store")
		}
	}
	thirdCtx, thirdCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer thirdCancel()
	if w := aiCall(handler, "/api/spaces/1/ai-brief", "", thirdCtx); w.Code != 429 {
		t.Fatalf("two active handlers must reject third before store: %d", w.Code)
	}
	cancel()
	for i := 0; i < 2; i++ {
		select {
		case code := <-results:
			if code != 504 {
				t.Fatal(code)
			}
		case <-time.After(time.Second):
			t.Fatal("cancel not propagated")
		}
	}
	if len(h.slots) != 0 {
		t.Fatal("admission leaked on cancellation")
	}
}

func TestAIBriefBodyReadDeadlineAndNoSensitiveLogs(t *testing.T) {
	_, handler := aiTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Hop-Backend", "gcp")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}, goodSpace)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if w := aiCall(handler, "/api/spaces/1/ai-brief", "", ctx); w.Code != 504 {
		t.Fatal("body read deadline", w.Code)
	}
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Hop-Backend", "gcp")
		_, _ = io.WriteString(w, "sensitive-upstream-marker")
	}))
	defer ts.Close()
	h, err := newAIBriefHandler(func(context.Context, int64) (store.Space, error) {
		return store.Space{ID: 1, Description: "sensitive-input-marker"}, nil
	}, ts.URL+"/api/chat", "sensitive-token-marker", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/spaces/{id}/ai-brief", h)
	w := aiCall(httpx.Middleware(log, "test", "")(mux), "/api/spaces/1/ai-brief", "", context.Background())
	if w.Code != 502 {
		t.Fatal(w.Code)
	}
	for _, secret := range []string{"sensitive-upstream-marker", "sensitive-input-marker", "sensitive-token-marker", ts.URL} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("sensitive log value", secret)
		}
	}
	if !strings.Contains(logs.String(), `"completed":false`) || !strings.Contains(logs.String(), `"backend":"gcp"`) {
		t.Fatal("safe metadata missing")
	}
}
