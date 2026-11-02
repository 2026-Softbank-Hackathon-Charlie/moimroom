// Package api implements the JSON API under /api.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/blob"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

// Areas are the districts the service covers. The SPA keeps the same list.
var Areas = []string{"강남", "홍대", "성수", "잠실", "여의도", "신촌"}

var categories = map[string]bool{"study": true, "meeting": true, "party": true}

// kst is fixed because Korea has no daylight saving time; it also avoids
// depending on tzdata in the runtime image.
var kst = time.FixedZone("KST", 9*60*60)

const (
	// bookingWindowDays is how far ahead a guest can book.
	bookingWindowDays = 60
	maxBookingHours   = 12
	maxPhotoBytes     = 5 << 20
)

type Server struct {
	store    *store.Store
	blobs    blob.Store
	hostCode string
	now      func() time.Time
}

func New(st *store.Store, blobs blob.Store, hostCode string) *Server {
	return &Server{store: st, blobs: blobs, hostCode: hostCode, now: time.Now}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/spaces", s.listSpaces)
	mux.HandleFunc("GET /api/spaces/{id}", s.getSpace)
	mux.HandleFunc("GET /api/spaces/{id}/availability", s.availability)
	mux.HandleFunc("GET /api/spaces/{id}/reviews", s.listReviews)
	mux.HandleFunc("GET /api/spaces/{id}/photos/{photoId}", s.getPhoto)
	mux.HandleFunc("POST /api/reservations", s.createReservation)
	mux.HandleFunc("POST /api/reservations/lookup", s.lookupReservation)
	mux.HandleFunc("POST /api/reservations/cancel", s.cancelReservation)
	mux.HandleFunc("POST /api/reviews", s.createReview)
	mux.HandleFunc("GET /api/host/spaces", s.host(s.hostSpaces))
	mux.HandleFunc("POST /api/host/spaces", s.host(s.hostCreateSpace))
	mux.HandleFunc("POST /api/host/spaces/{id}/photos", s.host(s.hostUploadPhoto))
	mux.HandleFunc("GET /api/host/reservations", s.host(s.hostReservations))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusNotFound, "요청한 주소를 찾을 수 없습니다.")
	})
}

// today returns the current KST date and hour.
func (s *Server) today() (time.Time, int) {
	n := s.now().In(kst)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, kst), n.Hour()
}

// parseDate accepts YYYY-MM-DD within the booking window.
func (s *Server) parseDate(v string) (time.Time, bool) {
	d, err := time.ParseInLocation(time.DateOnly, v, kst)
	if err != nil {
		return d, false
	}
	today, _ := s.today()
	days := int(d.Sub(today).Hours() / 24)
	return d, !d.Before(today) && days <= bookingWindowDays
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		httpx.Error(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return false
	}
	return true
}

// fail maps store errors to responses and logs the unexpected ones.
func fail(w http.ResponseWriter, r *http.Request, err error, notFound string) {
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, notFound)
		return
	}
	httpx.Logger(r.Context()).Error("request failed", "err", err)
	httpx.Error(w, http.StatusInternalServerError, httpx.MsgUnavailable)
}

func badRequest(w http.ResponseWriter, msg string) { httpx.Error(w, http.StatusBadRequest, msg) }

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// cleanList trims entries, drops empty ones and caps the list.
func cleanList(in []string, maxItems, maxLen int) ([]string, bool) {
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if runeLen(v) > maxLen {
			return nil, false
		}
		out = append(out, v)
	}
	return out, len(out) <= maxItems
}
