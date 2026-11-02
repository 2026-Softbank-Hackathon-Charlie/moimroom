package spa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/index-abc.js":  {Data: []byte("console.log(1)")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		"assets/index-abc.css": {Data: []byte("body{}")},
	}
	h := Handler(fsys)
	cases := []struct {
		method, path string
		status       int
		bodyHas      string
		cache        string
	}{
		{"GET", "/", 200, "id=root", "no-cache"},
		{"GET", "/spaces/3", 200, "id=root", "no-cache"},
		{"GET", "/host", 200, "id=root", "no-cache"},
		{"GET", "/assets/index-abc.js", 200, "console.log", "public, max-age=31536000, immutable"},
		{"GET", "/favicon.svg", 200, "<svg", "no-cache"},
		{"GET", "/assets/missing.js", 404, "", ""},
		{"POST", "/", 405, "", ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.status {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, rec.Code, c.status)
			continue
		}
		if c.bodyHas != "" && !strings.Contains(rec.Body.String(), c.bodyHas) {
			t.Errorf("%s: body %q", c.path, rec.Body.String())
		}
		if c.cache != "" && rec.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s: Cache-Control %q", c.path, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestHandlerWithoutBuild(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}
