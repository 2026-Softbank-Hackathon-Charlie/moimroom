-- Idempotent schema. Every table has a primary key.
CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE IF NOT EXISTS app.spaces (
    id             bigserial PRIMARY KEY,
    name           text        NOT NULL,
    category       text        NOT NULL CHECK (category IN ('study', 'meeting', 'party')),
    area           text        NOT NULL,
    address        text        NOT NULL,
    summary        text        NOT NULL DEFAULT '',
    description    text        NOT NULL DEFAULT '',
    capacity       int         NOT NULL CHECK (capacity > 0),
    price_per_hour int         NOT NULL CHECK (price_per_hour >= 0),
    open_hour      int         NOT NULL CHECK (open_hour BETWEEN 0 AND 23),
    close_hour     int         NOT NULL CHECK (close_hour BETWEEN 1 AND 24),
    amenities      text[]      NOT NULL DEFAULT '{}',
    rules          text[]      NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (close_hour > open_hour)
);

CREATE TABLE IF NOT EXISTS app.space_photos (
    id         bigserial PRIMARY KEY,
    space_id   bigint      NOT NULL REFERENCES app.spaces (id),
    object_key text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS space_photos_space_idx ON app.space_photos (space_id, id);

CREATE TABLE IF NOT EXISTS app.reservations (
    id          bigserial PRIMARY KEY,
    code        text        NOT NULL UNIQUE,
    space_id    bigint      NOT NULL REFERENCES app.spaces (id),
    date        date        NOT NULL,
    start_hour  int         NOT NULL CHECK (start_hour BETWEEN 0 AND 23),
    end_hour    int         NOT NULL CHECK (end_hour BETWEEN 1 AND 24),
    headcount   int         NOT NULL CHECK (headcount > 0),
    total_price int         NOT NULL CHECK (total_price >= 0),
    guest_name  text        NOT NULL,
    phone_last4 text        NOT NULL,
    phone_hash  text        NOT NULL,
    status      text        NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed', 'cancelled')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (end_hour > start_hour)
);
CREATE INDEX IF NOT EXISTS reservations_space_date_idx ON app.reservations (space_id, date);

-- One row per booked hour. The primary key is what rejects double booking.
CREATE TABLE IF NOT EXISTS app.reservation_slots (
    space_id       bigint NOT NULL,
    date           date   NOT NULL,
    hour           int    NOT NULL CHECK (hour BETWEEN 0 AND 23),
    reservation_id bigint NOT NULL REFERENCES app.reservations (id),
    PRIMARY KEY (space_id, date, hour)
);
CREATE INDEX IF NOT EXISTS reservation_slots_reservation_idx ON app.reservation_slots (reservation_id);

CREATE TABLE IF NOT EXISTS app.reviews (
    id             bigserial PRIMARY KEY,
    space_id       bigint      NOT NULL REFERENCES app.spaces (id),
    reservation_id bigint      NOT NULL UNIQUE REFERENCES app.reservations (id),
    rating         int         NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body           text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS reviews_space_idx ON app.reviews (space_id, id DESC);
