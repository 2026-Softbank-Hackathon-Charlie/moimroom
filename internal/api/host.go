package api

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/httpx"
	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/store"
)

// HeaderHostCode carries the shared host code. There are no host accounts;
// the code is checked on every request, so no server-side session exists.
const HeaderHostCode = "X-Host-Code"

// photoTypes maps sniffed content types to file extensions.
var photoTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",
}

func (s *Server) host(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.hostCode == "" {
			httpx.Error(w, http.StatusNotFound, "요청한 주소를 찾을 수 없습니다.")
			return
		}
		got := r.Header.Get(HeaderHostCode)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.hostCode)) != 1 {
			httpx.Error(w, http.StatusUnauthorized, "호스트 코드가 올바르지 않습니다.")
			return
		}
		h(w, r)
	}
}

func (s *Server) hostSpaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := s.store.ListSpaces(r.Context(), store.SpaceFilter{Sort: "newest", Limit: 100})
	if err != nil {
		fail(w, r, err, "")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": spaces})
}

type createSpaceRequest struct {
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Area         string   `json:"area"`
	Address      string   `json:"address"`
	Summary      string   `json:"summary"`
	Description  string   `json:"description"`
	Capacity     int      `json:"capacity"`
	PricePerHour int      `json:"pricePerHour"`
	OpenHour     int      `json:"openHour"`
	CloseHour    int      `json:"closeHour"`
	Amenities    []string `json:"amenities"`
	Rules        []string `json:"rules"`
}

func (req *createSpaceRequest) validate() (store.Space, string) {
	t := strings.TrimSpace
	sp := store.Space{
		Name: t(req.Name), Category: req.Category, Area: req.Area, Address: t(req.Address),
		Summary: t(req.Summary), Description: t(req.Description), Capacity: req.Capacity,
		PricePerHour: req.PricePerHour, OpenHour: req.OpenHour, CloseHour: req.CloseHour,
	}
	var ok1, ok2 bool
	sp.Amenities, ok1 = cleanList(req.Amenities, 20, 20)
	sp.Rules, ok2 = cleanList(req.Rules, 10, 100)
	switch {
	case runeLen(sp.Name) < 2 || runeLen(sp.Name) > 60:
		return sp, "공간 이름을 2자 이상 60자 이하로 입력해 주세요."
	case !categories[sp.Category]:
		return sp, "공간 유형을 선택해 주세요."
	case !slices.Contains(Areas, sp.Area):
		return sp, "지역을 선택해 주세요."
	case runeLen(sp.Address) < 2 || runeLen(sp.Address) > 100:
		return sp, "주소를 입력해 주세요."
	case runeLen(sp.Summary) > 80:
		return sp, "한 줄 소개는 80자 이하로 입력해 주세요."
	case runeLen(sp.Description) > 2000:
		return sp, "상세 설명은 2000자 이하로 입력해 주세요."
	case sp.Capacity < 1 || sp.Capacity > 100:
		return sp, "최대 인원은 1명부터 100명까지 입력할 수 있습니다."
	case sp.PricePerHour < 0 || sp.PricePerHour > 1_000_000:
		return sp, "시간당 요금을 확인해 주세요."
	case sp.OpenHour < 0 || sp.OpenHour > 23 || sp.CloseHour <= sp.OpenHour || sp.CloseHour > 24:
		return sp, "운영 시간을 확인해 주세요."
	case !ok1:
		return sp, "편의시설은 20자 이내로 20개까지 입력할 수 있습니다."
	case !ok2:
		return sp, "이용 규칙은 100자 이내로 10개까지 입력할 수 있습니다."
	}
	return sp, ""
}

func (s *Server) hostCreateSpace(w http.ResponseWriter, r *http.Request) {
	var req createSpaceRequest
	if !decode(w, r, &req) {
		return
	}
	sp, msg := req.validate()
	if msg != "" {
		badRequest(w, msg)
		return
	}
	created, err := s.store.CreateSpace(r.Context(), sp)
	if err != nil {
		fail(w, r, err, "")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// hostUploadPhoto reads the multipart field "photo" into memory (at most
// 5 MiB, so no temp files on a read-only filesystem), checks the real
// content type, stores the object and records only its key.
func (s *Server) hostUploadPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, msgSpaceNotFound)
		return
	}
	if _, err := s.store.GetSpace(r.Context(), id); err != nil {
		fail(w, r, err, msgSpaceNotFound)
		return
	}
	data, msg, status := readPhoto(w, r)
	if msg != "" {
		httpx.Error(w, status, msg)
		return
	}
	ctype := http.DetectContentType(data)
	ext, ok := photoTypes[ctype]
	if !ok {
		httpx.Error(w, http.StatusUnsupportedMediaType, "JPG, PNG, WEBP, GIF 이미지만 올릴 수 있습니다.")
		return
	}
	key := fmt.Sprintf("spaces/%d/%s.%s", id, uuid.NewString(), ext)
	if err := s.blobs.Put(r.Context(), key, bytes.NewReader(data), int64(len(data)), ctype); err != nil {
		httpx.Logger(r.Context()).Error("photo upload failed", "err", err)
		httpx.Error(w, http.StatusBadGateway, httpx.MsgUnavailable)
		return
	}
	p, err := s.store.AddPhoto(r.Context(), id, key)
	if err != nil {
		fail(w, r, err, "")
		return
	}
	httpx.JSON(w, http.StatusCreated, p)
}

func readPhoto(w http.ResponseWriter, r *http.Request) ([]byte, string, int) {
	const tooLarge = "사진은 5MB 이하만 올릴 수 있습니다."
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, "사진 파일을 첨부해 주세요.", http.StatusBadRequest
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, "사진 파일을 첨부해 주세요.", http.StatusBadRequest
		}
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, tooLarge, http.StatusRequestEntityTooLarge
		}
		if err != nil {
			return nil, "사진 파일을 첨부해 주세요.", http.StatusBadRequest
		}
		if part.FormName() != "photo" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, maxPhotoBytes+1))
		if errors.As(err, &mbe) || len(data) > maxPhotoBytes {
			return nil, tooLarge, http.StatusRequestEntityTooLarge
		}
		if err != nil || len(data) == 0 {
			return nil, "사진 파일을 첨부해 주세요.", http.StatusBadRequest
		}
		return data, "", 0
	}
}

func (s *Server) hostReservations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var spaceID int64
	if v := q.Get("spaceId"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			badRequest(w, "공간 번호가 올바르지 않습니다.")
			return
		}
		spaceID = n
	}
	from := q.Get("from")
	if from == "" {
		today, _ := s.today()
		from = today.Format(time.DateOnly)
	} else if _, err := time.ParseInLocation(time.DateOnly, from, kst); err != nil {
		badRequest(w, "날짜 형식이 올바르지 않습니다.")
		return
	}
	list, err := s.store.HostReservations(r.Context(), spaceID, from)
	if err != nil {
		fail(w, r, err, "")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}
