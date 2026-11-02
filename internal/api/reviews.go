package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

type createReviewRequest struct {
	Code   string `json:"code"`
	Rating int    `json:"rating"`
	Body   string `json:"body"`
}

// createReview accepts one review per confirmed reservation; the booking
// code proves the author made the reservation.
func (s *Server) createReview(w http.ResponseWriter, r *http.Request) {
	var req createReviewRequest
	if !decode(w, r, &req) {
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Rating < 1 || req.Rating > 5 {
		badRequest(w, "별점은 1점부터 5점까지 선택할 수 있습니다.")
		return
	}
	if n := runeLen(req.Body); n < 5 || n > 1000 {
		badRequest(w, "후기는 5자 이상 1000자 이하로 작성해 주세요.")
		return
	}
	res, err := s.store.ReservationByCode(r.Context(), normalizeCode(req.Code))
	if err != nil {
		fail(w, r, err, "예약 코드를 찾을 수 없습니다.")
		return
	}
	if res.Status != "confirmed" {
		httpx.Error(w, http.StatusConflict, "취소된 예약에는 후기를 남길 수 없습니다.")
		return
	}
	rv, err := s.store.CreateReview(r.Context(), res, req.Rating, req.Body)
	if errors.Is(err, store.ErrAlreadyReviewed) {
		httpx.Error(w, http.StatusConflict, "이미 후기를 작성한 예약입니다.")
		return
	}
	if err != nil {
		fail(w, r, err, "")
		return
	}
	rv.GuestName = maskName(rv.GuestName)
	httpx.JSON(w, http.StatusCreated, rv)
}
