// Package store is the PostgreSQL data layer. All tables live in schema app.
package store

import (
	"context"
	"crypto/rand"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/pgdb"
)

//go:embed schema.sql
var schemaSQL string

//go:embed seed.sql
var seedSQL string

// migrateLockKey serializes schema setup across replicas.
const migrateLockKey = 7_316_001

var (
	ErrNotFound        = errors.New("not found")
	ErrSlotTaken       = errors.New("slot already booked")
	ErrAlreadyReviewed = errors.New("already reviewed")
)

// Migrate creates the schema and, when seed is true, the first-start spaces.
// On a standby (a replica that a migration will promote later) it does
// nothing: the schema arrives through replication, and a read-only server
// rejects even CREATE ... IF NOT EXISTS. It reports whether it ran.
func Migrate(ctx context.Context, pool *pgxpool.Pool, seed bool) (bool, error) {
	var standby bool
	if err := pool.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&standby); err != nil {
		return false, err
	}
	if standby {
		return false, nil
	}
	return true, migrate(ctx, pool, seed)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, seed bool) error {
	stmts := []string{schemaSQL}
	if seed {
		stmts = append(stmts, seedSQL)
	}
	return pgdb.Migrate(ctx, pool, migrateLockKey, stmts...)
}

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Space struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	Area         string    `json:"area"`
	Address      string    `json:"address"`
	Summary      string    `json:"summary"`
	Description  string    `json:"description,omitempty"`
	Capacity     int       `json:"capacity"`
	PricePerHour int       `json:"pricePerHour"`
	OpenHour     int       `json:"openHour"`
	CloseHour    int       `json:"closeHour"`
	Amenities    []string  `json:"amenities"`
	Rules        []string  `json:"rules,omitempty"`
	Rating       float64   `json:"rating"`
	ReviewCount  int       `json:"reviewCount"`
	CoverPhotoID *int64    `json:"coverPhotoId"`
	PhotoIDs     []int64   `json:"photoIds,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type SpaceFilter struct {
	Area        string
	Category    string
	MinCapacity int
	MaxPrice    int
	Sort        string
	Limit       int
}

// sortOrders maps API sort names to ORDER BY clauses; unknown names are
// rejected by the caller, so no user text reaches the SQL.
var sortOrders = map[string]string{
	"recommended": "coalesce(r.cnt, 0) DESC, coalesce(r.avg, 0) DESC, s.id",
	"rating":      "coalesce(r.avg, 0) DESC, coalesce(r.cnt, 0) DESC, s.id",
	"price_asc":   "s.price_per_hour, s.id",
	"price_desc":  "s.price_per_hour DESC, s.id",
	"capacity":    "s.capacity DESC, s.id",
	"newest":      "s.id DESC",
}

// ValidSort reports whether name is a supported sort.
func ValidSort(name string) bool {
	_, ok := sortOrders[name]
	return ok
}

const spaceCols = `s.id, s.name, s.category, s.area, s.address, s.summary, s.description,
	s.capacity, s.price_per_hour, s.open_hour, s.close_hour, s.amenities, s.rules, s.created_at,
	coalesce(r.avg, 0)::float8, coalesce(r.cnt, 0), p.id`

const spaceJoins = `FROM app.spaces s
	LEFT JOIN LATERAL (SELECT avg(rating) AS avg, count(*) AS cnt FROM app.reviews WHERE space_id = s.id) r ON true
	LEFT JOIN LATERAL (SELECT id FROM app.space_photos WHERE space_id = s.id ORDER BY id LIMIT 1) p ON true`

func scanSpace(row pgx.Row) (Space, error) {
	var s Space
	err := row.Scan(&s.ID, &s.Name, &s.Category, &s.Area, &s.Address, &s.Summary, &s.Description,
		&s.Capacity, &s.PricePerHour, &s.OpenHour, &s.CloseHour, &s.Amenities, &s.Rules, &s.CreatedAt,
		&s.Rating, &s.ReviewCount, &s.CoverPhotoID)
	return s, err
}

func (st *Store) ListSpaces(ctx context.Context, f SpaceFilter) ([]Space, error) {
	order, ok := sortOrders[f.Sort]
	if !ok {
		order = sortOrders["recommended"]
	}
	q := `SELECT ` + spaceCols + ` ` + spaceJoins + `
		WHERE ($1::text = '' OR s.area = $1)
		  AND ($2::text = '' OR s.category = $2)
		  AND s.capacity >= $3
		  AND ($4::int = 0 OR s.price_per_hour <= $4)
		ORDER BY ` + order + ` LIMIT $5`
	rows, err := st.pool.Query(ctx, q, f.Area, f.Category, f.MinCapacity, f.MaxPrice, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Space{}
	for rows.Next() {
		s, err := scanSpace(rows)
		if err != nil {
			return nil, err
		}
		// Lists stay light; the detail endpoint carries the long text.
		s.Description, s.Rules = "", nil
		out = append(out, s)
	}
	return out, rows.Err()
}

func (st *Store) GetSpace(ctx context.Context, id int64) (Space, error) {
	s, err := scanSpace(st.pool.QueryRow(ctx, `SELECT `+spaceCols+` `+spaceJoins+` WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	if err != nil {
		return s, err
	}
	rows, err := st.pool.Query(ctx, `SELECT id FROM app.space_photos WHERE space_id = $1 ORDER BY id`, id)
	if err != nil {
		return s, err
	}
	s.PhotoIDs, err = pgx.CollectRows(rows, pgx.RowTo[int64])
	return s, err
}

func (st *Store) CreateSpace(ctx context.Context, s Space) (Space, error) {
	var id int64
	err := st.pool.QueryRow(ctx, `INSERT INTO app.spaces
		(name, category, area, address, summary, description, capacity, price_per_hour, open_hour, close_hour, amenities, rules)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		s.Name, s.Category, s.Area, s.Address, s.Summary, s.Description, s.Capacity, s.PricePerHour,
		s.OpenHour, s.CloseHour, s.Amenities, s.Rules).Scan(&id)
	if err != nil {
		return Space{}, err
	}
	return st.GetSpace(ctx, id)
}

type Photo struct {
	ID        int64     `json:"id"`
	SpaceID   int64     `json:"spaceId"`
	ObjectKey string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

func (st *Store) AddPhoto(ctx context.Context, spaceID int64, key string) (Photo, error) {
	p := Photo{SpaceID: spaceID, ObjectKey: key}
	err := st.pool.QueryRow(ctx, `INSERT INTO app.space_photos (space_id, object_key) VALUES ($1, $2)
		RETURNING id, created_at`, spaceID, key).Scan(&p.ID, &p.CreatedAt)
	return p, err
}

func (st *Store) GetPhoto(ctx context.Context, spaceID, photoID int64) (Photo, error) {
	p := Photo{ID: photoID, SpaceID: spaceID}
	err := st.pool.QueryRow(ctx, `SELECT object_key, created_at FROM app.space_photos
		WHERE id = $1 AND space_id = $2`, photoID, spaceID).Scan(&p.ObjectKey, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// BookedHours returns the booked hours of a space on date (YYYY-MM-DD).
func (st *Store) BookedHours(ctx context.Context, spaceID int64, date string) ([]int, error) {
	rows, err := st.pool.Query(ctx, `SELECT hour FROM app.reservation_slots
		WHERE space_id = $1 AND date = $2::date ORDER BY hour`, spaceID, date)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int])
}

type Reservation struct {
	ID         int64     `json:"id"`
	Code       string    `json:"code"`
	SpaceID    int64     `json:"spaceId"`
	SpaceName  string    `json:"spaceName"`
	Date       string    `json:"date"`
	StartHour  int       `json:"startHour"`
	EndHour    int       `json:"endHour"`
	Headcount  int       `json:"headcount"`
	TotalPrice int       `json:"totalPrice"`
	GuestName  string    `json:"guestName"`
	PhoneLast4 string    `json:"phoneLast4"`
	PhoneHash  string    `json:"-"`
	Status     string    `json:"status"`
	Reviewed   bool      `json:"reviewed"`
	CreatedAt  time.Time `json:"createdAt"`
}

const reservationCols = `r.id, r.code, r.space_id, s.name, to_char(r.date, 'YYYY-MM-DD'), r.start_hour, r.end_hour,
	r.headcount, r.total_price, r.guest_name, r.phone_last4, r.phone_hash, r.status, r.created_at,
	EXISTS (SELECT 1 FROM app.reviews v WHERE v.reservation_id = r.id)`

func scanReservation(row pgx.Row) (Reservation, error) {
	var r Reservation
	err := row.Scan(&r.ID, &r.Code, &r.SpaceID, &r.SpaceName, &r.Date, &r.StartHour, &r.EndHour,
		&r.Headcount, &r.TotalPrice, &r.GuestName, &r.PhoneLast4, &r.PhoneHash, &r.Status, &r.CreatedAt, &r.Reviewed)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// CreateReservation inserts the reservation and one slot row per hour in a
// single transaction. A slot primary key violation means someone else holds
// one of the hours, and nothing is written.
func (st *Store) CreateReservation(ctx context.Context, r Reservation) (Reservation, error) {
	for attempt := 0; ; attempt++ {
		r.Code = newCode()
		out, err := st.createReservation(ctx, r)
		if isUnique(err, "reservations_code_key") && attempt < 3 {
			continue // code collision, astronomically rare
		}
		return out, err
	}
}

func (st *Store) createReservation(ctx context.Context, r Reservation) (Reservation, error) {
	tx, err := st.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `INSERT INTO app.reservations
		(code, space_id, date, start_hour, end_hour, headcount, total_price, guest_name, phone_last4, phone_hash)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		r.Code, r.SpaceID, r.Date, r.StartHour, r.EndHour, r.Headcount, r.TotalPrice,
		r.GuestName, r.PhoneLast4, r.PhoneHash).Scan(&r.ID)
	if err != nil {
		return r, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.reservation_slots (space_id, date, hour, reservation_id)
		SELECT $1, $2::date, h, $3 FROM generate_series($4::int, $5::int - 1) AS h`,
		r.SpaceID, r.Date, r.ID, r.StartHour, r.EndHour)
	if isUnique(err, "reservation_slots_pkey") {
		return r, ErrSlotTaken
	}
	if err != nil {
		return r, err
	}
	out, err := scanReservation(tx.QueryRow(ctx, `SELECT `+reservationCols+`
		FROM app.reservations r JOIN app.spaces s ON s.id = r.space_id WHERE r.id = $1`, r.ID))
	if err != nil {
		return r, err
	}
	return out, tx.Commit(ctx)
}

func (st *Store) ReservationByCode(ctx context.Context, code string) (Reservation, error) {
	return scanReservation(st.pool.QueryRow(ctx, `SELECT `+reservationCols+`
		FROM app.reservations r JOIN app.spaces s ON s.id = r.space_id WHERE r.code = $1`, code))
}

// CancelReservation locks the reservation, lets check veto the cancel, then
// frees its slots and marks it cancelled, all in one transaction.
func (st *Store) CancelReservation(ctx context.Context, code string, check func(Reservation) error) (Reservation, error) {
	tx, err := st.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	r, err := scanReservation(tx.QueryRow(ctx, `SELECT `+reservationCols+`
		FROM app.reservations r JOIN app.spaces s ON s.id = r.space_id WHERE r.code = $1 FOR UPDATE OF r`, code))
	if err != nil {
		return r, err
	}
	if err := check(r); err != nil {
		return r, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM app.reservation_slots WHERE reservation_id = $1`, r.ID); err != nil {
		return r, err
	}
	if _, err := tx.Exec(ctx, `UPDATE app.reservations SET status = 'cancelled' WHERE id = $1`, r.ID); err != nil {
		return r, err
	}
	r.Status = "cancelled"
	return r, tx.Commit(ctx)
}

// HostReservations lists reservations from date onward; spaceID 0 means all.
func (st *Store) HostReservations(ctx context.Context, spaceID int64, from string) ([]Reservation, error) {
	rows, err := st.pool.Query(ctx, `SELECT `+reservationCols+`
		FROM app.reservations r JOIN app.spaces s ON s.id = r.space_id
		WHERE ($1::bigint = 0 OR r.space_id = $1) AND r.date >= $2::date
		ORDER BY r.date, r.start_hour, r.id LIMIT 500`, spaceID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reservation{}
	for rows.Next() {
		r, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type Review struct {
	ID            int64     `json:"id"`
	SpaceID       int64     `json:"spaceId"`
	ReservationID int64     `json:"-"`
	Rating        int       `json:"rating"`
	Body          string    `json:"body"`
	GuestName     string    `json:"guestName"`
	CreatedAt     time.Time `json:"createdAt"`
}

// CreateReview stores one review per reservation.
func (st *Store) CreateReview(ctx context.Context, r Reservation, rating int, body string) (Review, error) {
	rv := Review{SpaceID: r.SpaceID, ReservationID: r.ID, Rating: rating, Body: body, GuestName: r.GuestName}
	err := st.pool.QueryRow(ctx, `INSERT INTO app.reviews (space_id, reservation_id, rating, body)
		VALUES ($1, $2, $3, $4) ON CONFLICT (reservation_id) DO NOTHING RETURNING id, created_at`,
		r.SpaceID, r.ID, rating, body).Scan(&rv.ID, &rv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return rv, ErrAlreadyReviewed
	}
	return rv, err
}

func (st *Store) ListReviews(ctx context.Context, spaceID int64, limit int) ([]Review, error) {
	rows, err := st.pool.Query(ctx, `SELECT v.id, v.space_id, v.reservation_id, v.rating, v.body, r.guest_name, v.created_at
		FROM app.reviews v JOIN app.reservations r ON r.id = v.reservation_id
		WHERE v.space_id = $1 ORDER BY v.id DESC LIMIT $2`, spaceID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Review, error) {
		var v Review
		err := row.Scan(&v.ID, &v.SpaceID, &v.ReservationID, &v.Rating, &v.Body, &v.GuestName, &v.CreatedAt)
		return v, err
	})
}

// Ping checks the database for readiness.
func (st *Store) Ping(ctx context.Context) error { return st.pool.Ping(ctx) }

// codeAlphabet skips look-alike characters (0/O, 1/I/L) so codes survive
// being read over the phone.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// newCode returns a booking code like "K7QM-29XA" (about 39 bits).
func newCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 0, 9)
	for i, v := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, codeAlphabet[int(v)%len(codeAlphabet)])
	}
	return string(out)
}

func isUnique(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == constraint
}
