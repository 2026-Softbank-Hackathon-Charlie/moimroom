package api

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/blob"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

const msgSpaceNotFound = "공간을 찾을 수 없습니다."

func (s *Server) listSpaces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.SpaceFilter{
		Area:     q.Get("area"),
		Category: q.Get("category"),
		Sort:     q.Get("sort"),
		Limit:    24,
	}
	if f.Area != "" && !slices.Contains(Areas, f.Area) {
		badRequest(w, "지원하지 않는 지역입니다.")
		return
	}
	if f.Category != "" && !categories[f.Category] {
		badRequest(w, "지원하지 않는 공간 유형입니다.")
		return
	}
	if f.Sort == "" {
		f.Sort = "recommended"
	}
	if !store.ValidSort(f.Sort) {
		badRequest(w, "지원하지 않는 정렬 방식입니다.")
		return
	}
	for name, dst := range map[string]*int{"minCapacity": &f.MinCapacity, "maxPrice": &f.MaxPrice, "limit": &f.Limit} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				badRequest(w, "검색 조건이 올바르지 않습니다.")
				return
			}
			*dst = n
		}
	}
	f.Limit = min(max(f.Limit, 1), 100)

	spaces, err := s.store.ListSpaces(r.Context(), f)
	if err != nil {
		fail(w, r, err, "")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": spaces})
}

func (s *Server) getSpace(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, msgSpaceNotFound)
		return
	}
	sp, err := s.store.GetSpace(r.Context(), id)
	if err != nil {
		fail(w, r, err, msgSpaceNotFound)
		return
	}
	httpx.JSON(w, http.StatusOK, sp)
}

type hourStatus struct {
	Hour   int    `json:"hour"`
	Status string `json:"status"` // free, booked, past
}

func (s *Server) availability(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, msgSpaceNotFound)
		return
	}
	date := r.URL.Query().Get("date")
	d, ok := s.parseDate(date)
	if !ok {
		badRequest(w, "예약할 수 없는 날짜입니다.")
		return
	}
	sp, err := s.store.GetSpace(r.Context(), id)
	if err != nil {
		fail(w, r, err, msgSpaceNotFound)
		return
	}
	booked, err := s.store.BookedHours(r.Context(), id, date)
	if err != nil {
		fail(w, r, err, "")
		return
	}
	today, nowHour := s.today()
	hours := make([]hourStatus, 0, sp.CloseHour-sp.OpenHour)
	for h := sp.OpenHour; h < sp.CloseHour; h++ {
		st := "free"
		switch {
		case slices.Contains(booked, h):
			st = "booked"
		case d.Equal(today) && h <= nowHour:
			st = "past"
		}
		hours = append(hours, hourStatus{Hour: h, Status: st})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"date": date, "openHour": sp.OpenHour, "closeHour": sp.CloseHour, "hours": hours,
	})
}

func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, msgSpaceNotFound)
		return
	}
	reviews, err := s.store.ListReviews(r.Context(), id, 50)
	if err != nil {
		fail(w, r, err, "")
		return
	}
	for i := range reviews {
		reviews[i].GuestName = maskName(reviews[i].GuestName)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": reviews})
}

func (s *Server) getPhoto(w http.ResponseWriter, r *http.Request) {
	spaceID, ok1 := pathID(r, "id")
	photoID, ok2 := pathID(r, "photoId")
	if !ok1 || !ok2 {
		http.NotFound(w, r)
		return
	}
	p, err := s.store.GetPhoto(r.Context(), spaceID, photoID)
	if err != nil {
		fail(w, r, err, "사진을 찾을 수 없습니다.")
		return
	}
	obj, err := s.blobs.Get(r.Context(), p.ObjectKey)
	if errors.Is(err, blob.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "사진을 찾을 수 없습니다.")
		return
	}
	if err != nil {
		httpx.Logger(r.Context()).Error("photo fetch failed", "err", err, "photo_id", photoID)
		httpx.Error(w, http.StatusBadGateway, httpx.MsgUnavailable)
		return
	}
	defer obj.Body.Close()
	h := w.Header()
	h.Set("Content-Type", obj.ContentType)
	h.Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	// A photo id never points at different bytes, so caches may keep it.
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, obj.Body); err != nil {
		httpx.Logger(r.Context()).Warn("photo stream interrupted", "err", err)
	}
}

// maskName keeps the first and last characters: "김민수" becomes "김*수".
func maskName(name string) string {
	rs := []rune(name)
	switch len(rs) {
	case 0:
		return ""
	case 1:
		return "*"
	case 2:
		return string(rs[0]) + "*"
	}
	out := []rune{rs[0]}
	for range rs[1 : len(rs)-1] {
		out = append(out, '*')
	}
	return string(append(out, rs[len(rs)-1]))
}
