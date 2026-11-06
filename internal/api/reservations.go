package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

const msgReservationNotFound = "예약 정보를 찾을 수 없습니다. 예약 코드와 휴대폰 번호 뒤 4자리를 확인해 주세요."

type createReservationRequest struct {
	SpaceID   int64  `json:"spaceId"`
	Date      string `json:"date"`
	StartHour int    `json:"startHour"`
	EndHour   int    `json:"endHour"`
	Headcount int    `json:"headcount"`
	GuestName string `json:"guestName"`
	Phone     string `json:"phone"`
}

func (s *Server) createReservation(w http.ResponseWriter, r *http.Request) {
	var req createReservationRequest
	if !decode(w, r, &req) {
		return
	}
	req.GuestName = strings.TrimSpace(req.GuestName)
	if n := runeLen(req.GuestName); n < 1 || n > 30 {
		badRequest(w, "예약자 이름을 30자 이내로 입력해 주세요.")
		return
	}
	phone, ok := normalizePhone(req.Phone)
	if !ok {
		badRequest(w, "휴대폰 번호를 정확히 입력해 주세요.")
		return
	}
	var retry *store.ReservationRetry
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		validKey := len(r.Header.Values("Idempotency-Key")) == 1 && len(key) >= 16 && len(key) <= 128
		for _, char := range key {
			if char < 33 || char > 126 {
				validKey = false
			}
		}
		if !validKey {
			badRequest(w, "Idempotency-Key must be 16 to 128 characters without whitespace")
			return
		}
		req.Phone = phone
		canonical, err := json.Marshal(req)
		if err != nil {
			fail(w, r, err, "")
			return
		}
		keySum := sha256.Sum256([]byte("POST /api/reservations\x00" + key))
		keyHash := hex.EncodeToString(keySum[:])
		matches := func(hash string) bool { return matchesPrivateHash(string(canonical), hash) }
		previous, found, err := s.store.LookupReservationRetry(r.Context(), keyHash, matches)
		if errors.Is(err, store.ErrRetryConflict) {
			httpx.Error(w, http.StatusConflict, "idempotency_key_reused")
			return
		}
		if err != nil {
			fail(w, r, err, "")
			return
		}
		if found {
			httpx.JSON(w, http.StatusCreated, previous)
			return
		}
		requestHash, err := hashPhone(string(canonical))
		if err != nil {
			fail(w, r, err, "")
			return
		}
		retry = &store.ReservationRetry{KeyHash: keyHash, RequestHash: requestHash, Matches: matches}
	}
	d, ok := s.parseDate(req.Date)
	if !ok {
		badRequest(w, "예약할 수 없는 날짜입니다.")
		return
	}
	sp, err := s.store.GetSpace(r.Context(), req.SpaceID)
	if err != nil {
		fail(w, r, err, msgSpaceNotFound)
		return
	}
	hours := req.EndHour - req.StartHour
	if req.StartHour < sp.OpenHour || req.EndHour > sp.CloseHour || hours < 1 || hours > maxBookingHours {
		badRequest(w, "운영 시간 안에서 1시간 이상 선택해 주세요.")
		return
	}
	today, nowHour := s.today()
	if d.Equal(today) && req.StartHour <= nowHour {
		badRequest(w, "이미 지난 시간은 예약할 수 없습니다.")
		return
	}
	if req.Headcount < 1 || req.Headcount > sp.Capacity {
		badRequest(w, "이용 인원이 공간의 최대 인원을 넘었습니다.")
		return
	}
	hash, err := hashPhone(phone)
	if err != nil {
		fail(w, r, err, "")
		return
	}

	res, err := s.store.CreateReservationRetry(r.Context(), store.Reservation{
		SpaceID:    sp.ID,
		Date:       req.Date,
		StartHour:  req.StartHour,
		EndHour:    req.EndHour,
		Headcount:  req.Headcount,
		TotalPrice: hours * sp.PricePerHour,
		GuestName:  req.GuestName,
		PhoneLast4: phone[len(phone)-4:],
		PhoneHash:  hash,
	}, retry)
	if errors.Is(err, store.ErrRetryConflict) {
		httpx.Error(w, http.StatusConflict, "idempotency_key_reused")
		return
	}
	if errors.Is(err, store.ErrSlotTaken) {
		httpx.Error(w, http.StatusConflict, "선택하신 시간에 이미 예약이 있습니다. 다른 시간을 선택해 주세요.")
		return
	}
	if err != nil {
		fail(w, r, err, "")
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

type codeRequest struct {
	Code       string `json:"code"`
	PhoneLast4 string `json:"phoneLast4"`
}

// normalizeCode accepts "k7qm29xa", "K7QM 29XA" or "K7QM-29XA".
func normalizeCode(raw string) string {
	c := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(raw)))
	if len(c) != 8 {
		return c
	}
	return c[:4] + "-" + c[4:]
}

// lookupReservation answers wrong phone digits exactly like a missing code,
// so codes cannot be probed.
func (s *Server) lookupReservation(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.store.ReservationByCode(r.Context(), normalizeCode(req.Code))
	if err == nil && !last4Matches(res, req.PhoneLast4) {
		err = store.ErrNotFound
	}
	if err != nil {
		fail(w, r, err, msgReservationNotFound)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func last4Matches(res store.Reservation, got string) bool {
	return subtle.ConstantTimeCompare([]byte(res.PhoneLast4), []byte(strings.TrimSpace(got))) == 1
}

var (
	errAlreadyCancelled = errors.New("이미 취소된 예약입니다.")
	errStarted          = errors.New("이용 시작 시간이 지난 예약은 취소할 수 없습니다.")
	errReviewed         = errors.New("후기를 작성한 예약은 취소할 수 없습니다.")
)

func (s *Server) cancelReservation(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.store.CancelReservation(r.Context(), normalizeCode(req.Code), func(res store.Reservation) error {
		switch {
		case !last4Matches(res, req.PhoneLast4):
			return store.ErrNotFound
		case res.Status == "cancelled":
			return errAlreadyCancelled
		case res.Reviewed:
			return errReviewed
		case !s.now().Before(startTime(res)):
			return errStarted
		}
		return nil
	})
	switch {
	case errors.Is(err, errAlreadyCancelled), errors.Is(err, errStarted), errors.Is(err, errReviewed):
		httpx.Error(w, http.StatusConflict, err.Error())
	case err != nil:
		fail(w, r, err, msgReservationNotFound)
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}

func startTime(res store.Reservation) time.Time {
	d, _ := time.ParseInLocation(time.DateOnly, res.Date, kst)
	return d.Add(time.Duration(res.StartHour) * time.Hour)
}
