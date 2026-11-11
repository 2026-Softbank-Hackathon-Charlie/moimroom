package credentialcheck

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperationsAreFreshAndErrorsAreRedacted(t *testing.T) {
	n := strings.Repeat("a", 64)
	calls := 0
	h := Handler(func(ctx context.Context, challenge string) error {
		calls++
		if challenge != n {
			t.Fatal("wrong challenge")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded check")
		}
		return nil
	}, func(context.Context, string) error { return errors.New("password=do-not-print") })
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/internal/secret-usage?challenge="+n, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"database-query":"pass"`) || !strings.Contains(w.Body.String(), `"bucket-access":"fail"`) || strings.Contains(w.Body.String(), "do-not-print") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if calls != 2 {
		t.Fatal("cached a runtime check")
	}
}

func TestUnavailableAndInvalidChallengeNeverPass(t *testing.T) {
	h := Handler(nil, func(context.Context, string) error { return ErrNotConfigured })
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/?challenge="+strings.Repeat("b", 64), nil))
	if strings.Contains(w.Body.String(), `"pass"`) || !strings.Contains(w.Body.String(), `"unknown"`) {
		t.Fatal(w.Body)
	}
	for _, q := range []string{"", "?challenge=x", "?challenge=" + strings.Repeat("a", 64) + "&challenge=" + strings.Repeat("a", 64)} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/"+q, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
}
