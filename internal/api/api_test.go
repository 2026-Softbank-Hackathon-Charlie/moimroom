package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/blob"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

const testHostCode = "host-code-for-tests"

// fakeBlobs is an in-memory blob.Store.
type fakeBlobs struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
}

func (f *fakeBlobs) Put(_ context.Context, key string, body io.Reader, _ int64, ctype string) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key], f.types[key] = b, ctype
	return nil
}

func (f *fakeBlobs) Get(_ context.Context, key string) (blob.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return blob.Object{}, blob.ErrNotFound
	}
	return blob.Object{Body: io.NopCloser(bytes.NewReader(b)), Size: int64(len(b)), ContentType: f.types[key]}, nil
}

type env struct {
	t     *testing.T
	h     http.Handler
	srv   *Server
	pool  *pgxpool.Pool
	blobs *fakeBlobs
}

// fixedNow is a Monday morning in Seoul.
var fixedNow = time.Date(2026, 11, 2, 10, 30, 0, 0, kst)

// setup gives each test a fresh schema with the seed catalog.
func setup(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("MOIMROOM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOIMROOM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE"); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, pool, true); err != nil {
		t.Fatal(err)
	}
	fb := &fakeBlobs{objects: map[string][]byte{}, types: map[string]string{}}
	srv := New(store.New(pool), fb, testHostCode)
	srv.now = func() time.Time { return fixedNow }
	mux := http.NewServeMux()
	srv.Register(mux)
	return &env{t: t, h: mux, srv: srv, pool: pool, blobs: fb}
}

func (e *env) call(method, path string, body any, hdr ...string) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func items(m map[string]any) []map[string]any {
	var out []map[string]any
	for _, v := range m["items"].([]any) {
		out = append(out, v.(map[string]any))
	}
	return out
}

func booking(spaceID, start, end int, date string) map[string]any {
	return map[string]any{
		"spaceId": spaceID, "date": date, "startHour": start, "endHour": end,
		"headcount": 2, "guestName": "김민수", "phone": "010-1234-5678",
	}
}

func TestListAndDetail(t *testing.T) {
	e := setup(t)
	code, body := e.call("GET", "/api/spaces?limit=100", nil)
	if code != 200 || len(items(body)) != 16 {
		t.Fatalf("list: %d, %d items", code, len(items(body)))
	}
	_, body = e.call("GET", "/api/spaces?area=성수&limit=100", nil)
	for _, s := range items(body) {
		if s["area"] != "성수" {
			t.Errorf("area filter leaked %v", s["area"])
		}
	}
	_, body = e.call("GET", "/api/spaces?minCapacity=20&sort=price_asc", nil)
	last := 0.0
	for _, s := range items(body) {
		if s["capacity"].(float64) < 20 || s["pricePerHour"].(float64) < last {
			t.Errorf("filter or sort broken: %v", s)
		}
		last = s["pricePerHour"].(float64)
	}
	if code, _ := e.call("GET", "/api/spaces?sort=random", nil); code != 400 {
		t.Errorf("bad sort: %d", code)
	}
	if code, _ := e.call("GET", "/api/spaces?area=부산", nil); code != 400 {
		t.Errorf("bad area: %d", code)
	}

	code, sp := e.call("GET", "/api/spaces/1", nil)
	if code != 200 || sp["description"] == "" || len(sp["rules"].([]any)) == 0 {
		t.Fatalf("detail: %d %v", code, sp)
	}
	if code, _ := e.call("GET", "/api/spaces/999", nil); code != 404 {
		t.Errorf("missing space: %d", code)
	}
}

func TestBookingLifecycle(t *testing.T) {
	e := setup(t)
	date := "2026-11-03"

	code, res := e.call("POST", "/api/reservations", booking(1, 10, 12, date))
	if code != 201 {
		t.Fatalf("book: %d %v", code, res)
	}
	if !regexp.MustCompile(`^[A-Z2-9]{4}-[A-Z2-9]{4}$`).MatchString(res["code"].(string)) {
		t.Errorf("code format %q", res["code"])
	}
	if res["totalPrice"].(float64) != 2*22000 || res["phoneLast4"] != "5678" {
		t.Errorf("unexpected reservation %v", res)
	}
	resCode := res["code"].(string)

	// The phone number itself is never stored.
	var hash string
	if err := e.pool.QueryRow(context.Background(), "SELECT phone_hash FROM app.reservations WHERE code = $1", resCode).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "01012345678") || !strings.HasPrefix(hash, "pbkdf2-sha256$") {
		t.Errorf("phone_hash %q", hash)
	}

	_, av := e.call("GET", "/api/spaces/1/availability?date="+date, nil)
	for _, h := range av["hours"].([]any) {
		hs := h.(map[string]any)
		booked := hs["hour"].(float64) == 10 || hs["hour"].(float64) == 11
		if (hs["status"] == "booked") != booked {
			t.Errorf("hour %v status %v", hs["hour"], hs["status"])
		}
	}

	if code, _ := e.call("POST", "/api/reservations", booking(1, 11, 13, date)); code != 409 {
		t.Errorf("overlap: %d, want 409", code)
	}
	if code, _ := e.call("POST", "/api/reservations", booking(1, 12, 13, date)); code != 201 {
		t.Errorf("adjacent booking: %d, want 201", code)
	}

	lookup := map[string]any{"code": strings.ToLower(strings.ReplaceAll(resCode, "-", "")), "phoneLast4": "5678"}
	if code, got := e.call("POST", "/api/reservations/lookup", lookup); code != 200 || got["code"] != resCode {
		t.Errorf("lookup: %d %v", code, got)
	}
	if code, _ := e.call("POST", "/api/reservations/lookup", map[string]any{"code": resCode, "phoneLast4": "0000"}); code != 404 {
		t.Errorf("wrong digits: %d, want 404", code)
	}

	cancel := map[string]any{"code": resCode, "phoneLast4": "5678"}
	if code, got := e.call("POST", "/api/reservations/cancel", cancel); code != 200 || got["status"] != "cancelled" {
		t.Fatalf("cancel: %d %v", code, got)
	}
	if code, _ := e.call("POST", "/api/reservations/cancel", cancel); code != 409 {
		t.Errorf("second cancel: %d, want 409", code)
	}
	// Cancelling freed the hours.
	if code, _ := e.call("POST", "/api/reservations", booking(1, 10, 12, date)); code != 201 {
		t.Errorf("rebook after cancel: %d", code)
	}
}

func TestBookingValidation(t *testing.T) {
	e := setup(t)
	today := fixedNow.Format(time.DateOnly)
	cases := map[string]map[string]any{
		"past hour today":    booking(1, 9, 10, today),
		"current hour":       booking(1, 10, 11, today),
		"yesterday":          booking(1, 12, 13, "2026-11-01"),
		"beyond window":      booking(1, 12, 13, "2027-02-01"),
		"before opening":     booking(1, 6, 9, "2026-11-03"),
		"after closing":      booking(1, 22, 24, "2026-11-03"),
		"empty range":        booking(1, 12, 12, "2026-11-03"),
		"unknown space":      booking(999, 12, 13, "2026-11-03"),
		"bad phone":          func() map[string]any { b := booking(1, 12, 13, "2026-11-03"); b["phone"] = "02-123-4567"; return b }(),
		"no name":            func() map[string]any { b := booking(1, 12, 13, "2026-11-03"); b["guestName"] = " "; return b }(),
		"too many guests":    func() map[string]any { b := booking(1, 12, 13, "2026-11-03"); b["headcount"] = 9; return b }(),
		"malformed date str": booking(1, 12, 13, "11/03/2026"),
	}
	for name, body := range cases {
		code, _ := e.call("POST", "/api/reservations", body)
		want := 400
		if name == "unknown space" {
			want = 404
		}
		if code != want {
			t.Errorf("%s: %d, want %d", name, code, want)
		}
	}
	if code, _ := e.call("POST", "/api/reservations", booking(1, 11, 12, today)); code != 201 {
		t.Errorf("next hour today should be bookable: %d", code)
	}
}

// TestConcurrentBookingConflict races overlapping bookings; the slot primary
// key inside the transaction must let exactly one through.
func TestConcurrentBookingConflict(t *testing.T) {
	e := setup(t)
	const n = 12
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			start := 14 + i%2 // [14,17) and [15,18) overlap on 15 and 16
			code, _ := e.call("POST", "/api/reservations", booking(3, start, start+3, "2026-11-05"))
			codes <- code
		})
	}
	wg.Wait()
	close(codes)
	created, conflicts := 0, 0
	for c := range codes {
		switch c {
		case 201:
			created++
		case 409:
			conflicts++
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if created != 1 || conflicts != n-1 {
		t.Fatalf("created %d, conflicts %d", created, conflicts)
	}
	var slots, reservations int
	ctx := context.Background()
	_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM app.reservation_slots WHERE space_id = 3").Scan(&slots)
	_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM app.reservations WHERE space_id = 3").Scan(&reservations)
	if slots != 3 || reservations != 1 {
		t.Fatalf("slots %d, reservations %d: a failed booking left rows behind", slots, reservations)
	}
}

func TestReviewRules(t *testing.T) {
	e := setup(t)
	_, res := e.call("POST", "/api/reservations", booking(2, 10, 12, "2026-11-04"))
	resCode := res["code"].(string)

	review := func(code string, rating int, body string) (int, map[string]any) {
		return e.call("POST", "/api/reviews", map[string]any{"code": code, "rating": rating, "body": body})
	}
	if c, _ := review("ZZZZ-ZZZZ", 5, "아주 좋았습니다"); c != 404 {
		t.Errorf("unknown code: %d", c)
	}
	if c, _ := review(resCode, 0, "아주 좋았습니다"); c != 400 {
		t.Errorf("rating 0: %d", c)
	}
	if c, _ := review(resCode, 6, "아주 좋았습니다"); c != 400 {
		t.Errorf("rating 6: %d", c)
	}
	if c, _ := review(resCode, 5, "짧음"); c != 400 {
		t.Errorf("short body: %d", c)
	}
	c, rv := review(resCode, 4, "창이 넓고 조용해서 집중이 잘 됐어요.")
	if c != 201 || rv["guestName"] != "김*수" {
		t.Fatalf("review: %d %v", c, rv)
	}
	if c, _ := review(resCode, 5, "한 번 더 남기고 싶어요."); c != 409 {
		t.Errorf("duplicate review: %d", c)
	}

	_, list := e.call("GET", "/api/spaces/2/reviews", nil)
	if got := items(list); len(got) != 1 || got[0]["rating"].(float64) != 4 {
		t.Errorf("review list %v", got)
	}
	_, sp := e.call("GET", "/api/spaces/2", nil)
	if sp["rating"].(float64) != 4 || sp["reviewCount"].(float64) != 1 {
		t.Errorf("space rating %v count %v", sp["rating"], sp["reviewCount"])
	}
	if c, _ := e.call("POST", "/api/reservations/cancel", map[string]any{"code": resCode, "phoneLast4": "5678"}); c != 409 {
		t.Errorf("cancel after review: %d, want 409", c)
	}

	_, res2 := e.call("POST", "/api/reservations", booking(2, 14, 15, "2026-11-04"))
	e.call("POST", "/api/reservations/cancel", map[string]any{"code": res2["code"], "phoneLast4": "5678"})
	if c, _ := review(res2["code"].(string), 5, "취소한 예약 후기입니다."); c != 409 {
		t.Errorf("review on cancelled: %d, want 409", c)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (e *env) upload(spaceID int, data []byte, field, hostCode string) (int, map[string]any) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile(field, "room.png")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/host/spaces/%d/photos", spaceID), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(HeaderHostCode, hostCode)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHostArea(t *testing.T) {
	e := setup(t)
	if c, _ := e.call("GET", "/api/host/spaces", nil); c != 401 {
		t.Errorf("no code: %d", c)
	}
	if c, _ := e.call("GET", "/api/host/spaces", nil, HeaderHostCode, "nope"); c != 401 {
		t.Errorf("wrong code: %d", c)
	}
	c, list := e.call("GET", "/api/host/spaces", nil, HeaderHostCode, testHostCode)
	if c != 200 || len(items(list)) != 16 {
		t.Fatalf("host spaces: %d", c)
	}

	newSpace := map[string]any{
		"name": "망원 한강 스터디룸", "category": "study", "area": "홍대", "address": "서울 마포구 망원동",
		"summary": "한강공원 가까운 4인실", "description": "조용한 4인 스터디룸입니다.",
		"capacity": 4, "pricePerHour": 8000, "openHour": 9, "closeHour": 22,
		"amenities": []string{"와이파이", " ", "화이트보드"}, "rules": []string{"음식물 반입 금지"},
	}
	c, sp := e.call("POST", "/api/host/spaces", newSpace, HeaderHostCode, testHostCode)
	if c != 201 || len(sp["amenities"].([]any)) != 2 {
		t.Fatalf("create space: %d %v", c, sp)
	}
	bad := map[string]any{"name": "x", "category": "study", "area": "홍대", "address": "서울", "capacity": 4, "openHour": 9, "closeHour": 22}
	if c, _ := e.call("POST", "/api/host/spaces", bad, HeaderHostCode, testHostCode); c != 400 {
		t.Errorf("invalid space: %d", c)
	}
	id := int(sp["id"].(float64))

	img := pngBytes(t)
	c, photo := e.upload(id, img, "photo", testHostCode)
	if c != 201 {
		t.Fatalf("upload: %d %v", c, photo)
	}
	var key string
	_ = e.pool.QueryRow(context.Background(), "SELECT object_key FROM app.space_photos WHERE id = $1", photo["id"]).Scan(&key)
	if !regexp.MustCompile(fmt.Sprintf(`^spaces/%d/[0-9a-f-]{36}\.png$`, id)).MatchString(key) {
		t.Errorf("object key %q", key)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/spaces/%d/photos/%v", id, photo["id"]), nil)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), img) {
		t.Errorf("photo stream: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if c, _ := e.call("GET", fmt.Sprintf("/api/spaces/1/photos/%v", photo["id"]), nil); c != 404 {
		t.Errorf("photo of another space: %d", c)
	}
	_, detail := e.call("GET", fmt.Sprintf("/api/spaces/%d", id), nil)
	if detail["coverPhotoId"] != photo["id"] {
		t.Errorf("cover photo %v", detail["coverPhotoId"])
	}

	if c, _ := e.upload(id, []byte("plain text, not an image"), "photo", testHostCode); c != 415 {
		t.Errorf("non-image: %d", c)
	}
	big := append(pngBytes(t), make([]byte, maxPhotoBytes)...)
	if c, _ := e.upload(id, big, "photo", testHostCode); c != 413 {
		t.Errorf("oversize: %d", c)
	}
	if c, _ := e.upload(id, img, "file", testHostCode); c != 400 {
		t.Errorf("wrong field: %d", c)
	}
	if c, _ := e.upload(id, img, "photo", "nope"); c != 401 {
		t.Errorf("upload without code: %d", c)
	}

	e.call("POST", "/api/reservations", booking(id, 10, 12, "2026-11-06"))
	c, rl := e.call("GET", fmt.Sprintf("/api/host/reservations?spaceId=%d", id), nil, HeaderHostCode, testHostCode)
	if c != 200 || len(items(rl)) != 1 || items(rl)[0]["guestName"] != "김민수" {
		t.Errorf("host reservations: %d %v", c, rl)
	}
}

func TestHostDisabledWithoutCode(t *testing.T) {
	e := setup(t)
	e.srv.hostCode = ""
	if c, _ := e.call("GET", "/api/host/spaces", nil, HeaderHostCode, ""); c != 404 {
		t.Errorf("disabled host area: %d, want 404", c)
	}
}
