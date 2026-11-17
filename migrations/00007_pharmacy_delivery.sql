-- +goose Up

-- Great-circle distance in kilometres. Good enough for "nearby" within a city;
-- no PostGIS needed at this scale.
-- +goose StatementBegin
CREATE FUNCTION distance_km(lat1 float8, lon1 float8, lat2 float8, lon2 float8) RETURNS float8
LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT 6371 * 2 * asin(sqrt(
        power(sin(radians(lat2 - lat1) / 2), 2) +
        cos(radians(lat1)) * cos(radians(lat2)) * power(sin(radians(lon2 - lon1) / 2), 2)))
$$;
-- +goose StatementEnd

-- ---------------------------------------------------------------- pharmacies

CREATE TABLE pharmacies (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID          NOT NULL REFERENCES users (id),
    name                TEXT          NOT NULL,
    drug_license_no     TEXT          NOT NULL UNIQUE, -- DGDA drug licence
    address             TEXT          NOT NULL,
    district            TEXT          NOT NULL,
    area                TEXT,
    phone               TEXT          NOT NULL,
    latitude            NUMERIC(9, 6) NOT NULL,
    longitude           NUMERIC(9, 6) NOT NULL,
    opening_hours       TEXT,
    offers_delivery     BOOLEAN       NOT NULL DEFAULT true,
    delivery_radius_km  NUMERIC(4, 1) NOT NULL DEFAULT 5 CHECK (delivery_radius_km BETWEEN 0.5 AND 20),
    accepting_orders    BOOLEAN       NOT NULL DEFAULT true,
    verification_status TEXT          NOT NULL DEFAULT 'pending'
                        CHECK (verification_status IN ('pending', 'approved', 'rejected')),
    verification_note   TEXT,
    verified_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX pharmacies_owner_idx ON pharmacies (owner_id);
CREATE INDEX pharmacies_live_idx ON pharmacies (latitude, longitude) WHERE verification_status = 'approved';

-- ---------------------------------------------------------------- riders

CREATE TABLE riders (
    user_id             UUID PRIMARY KEY REFERENCES users (id),
    nid_number          TEXT        NOT NULL UNIQUE,
    vehicle             TEXT        NOT NULL CHECK (vehicle IN ('bicycle', 'motorcycle', 'car', 'on_foot')),
    verification_status TEXT        NOT NULL DEFAULT 'pending'
                        CHECK (verification_status IN ('pending', 'approved', 'rejected')),
    verification_note   TEXT,
    available           BOOLEAN     NOT NULL DEFAULT false,
    latitude            NUMERIC(9, 6),
    longitude           NUMERIC(9, 6),
    location_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------- requests and quotes
-- A patient's request goes to nearby pharmacies, which answer with quotes. The
-- patient accepts one quote, which becomes an order.

CREATE TABLE medicine_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id      UUID          NOT NULL REFERENCES users (id),
    prescription_id UUID REFERENCES prescriptions (id),
    attachment_key  TEXT UNIQUE,  -- uploaded prescription (when not issued on the platform)
    attachment_type TEXT,
    items           JSONB         NOT NULL DEFAULT '[]',
    note            TEXT,
    fulfilment      TEXT          NOT NULL CHECK (fulfilment IN ('delivery', 'pickup')),
    address         TEXT,
    latitude        NUMERIC(9, 6) NOT NULL,
    longitude       NUMERIC(9, 6) NOT NULL,
    contact_phone   TEXT          NOT NULL,
    status          TEXT          NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'ordered', 'cancelled')),
    expires_at      TIMESTAMPTZ   NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CHECK (prescription_id IS NOT NULL OR attachment_key IS NOT NULL),
    CHECK (fulfilment = 'pickup' OR address IS NOT NULL)
);

CREATE INDEX medicine_requests_open_idx ON medicine_requests (expires_at) WHERE status = 'open';
CREATE INDEX medicine_requests_patient_idx ON medicine_requests (patient_id, created_at DESC);

CREATE TABLE pharmacy_quotes (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id            UUID        NOT NULL REFERENCES medicine_requests (id),
    pharmacy_id           UUID        NOT NULL REFERENCES pharmacies (id),
    items                 JSONB       NOT NULL,
    medicines_total_paisa BIGINT      NOT NULL CHECK (medicines_total_paisa >= 0),
    delivery_charge_paisa BIGINT      NOT NULL DEFAULT 0 CHECK (delivery_charge_paisa >= 0),
    eta_minutes           INT         NOT NULL CHECK (eta_minutes BETWEEN 5 AND 1440),
    note                  TEXT,
    status                TEXT        NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'accepted', 'declined', 'withdrawn')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id, pharmacy_id)
);

CREATE INDEX pharmacy_quotes_pharmacy_idx ON pharmacy_quotes (pharmacy_id, created_at DESC);

-- ---------------------------------------------------------------- orders

CREATE TABLE medicine_orders (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id             UUID        NOT NULL UNIQUE REFERENCES medicine_requests (id),
    quote_id               UUID        NOT NULL UNIQUE REFERENCES pharmacy_quotes (id),
    patient_id             UUID        NOT NULL REFERENCES users (id),
    pharmacy_id            UUID        NOT NULL REFERENCES pharmacies (id),
    items                  JSONB       NOT NULL,
    medicines_total_paisa  BIGINT      NOT NULL,
    delivery_charge_paisa  BIGINT      NOT NULL,
    total_paisa            BIGINT      NOT NULL,
    commission_bps         INT         NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
    commission_paisa       BIGINT,     -- set on delivery
    fulfilment             TEXT        NOT NULL CHECK (fulfilment IN ('delivery', 'pickup')),
    delivery_mode          TEXT CHECK (delivery_mode IN ('pharmacy', 'rider')),
    rider_id               UUID REFERENCES riders (user_id),
    delivery_code          TEXT        NOT NULL,
    delivery_code_attempts INT         NOT NULL DEFAULT 0,
    payment_method         TEXT        NOT NULL DEFAULT 'cash_on_delivery',
    status                 TEXT        NOT NULL DEFAULT 'confirmed'
                           CHECK (status IN ('confirmed', 'preparing', 'ready', 'rider_assigned',
                                             'out_for_delivery', 'delivered', 'cancelled')),
    cancelled_by           TEXT CHECK (cancelled_by IN ('patient', 'pharmacy')),
    cancel_reason          TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at           TIMESTAMPTZ
);

CREATE INDEX medicine_orders_patient_idx ON medicine_orders (patient_id, created_at DESC);
CREATE INDEX medicine_orders_pharmacy_idx ON medicine_orders (pharmacy_id, status, created_at DESC);
CREATE INDEX medicine_orders_jobs_idx ON medicine_orders (pharmacy_id)
    WHERE status = 'ready' AND delivery_mode = 'rider' AND rider_id IS NULL;
-- A rider carries one order at a time.
CREATE UNIQUE INDEX medicine_orders_one_job_per_rider ON medicine_orders (rider_id)
    WHERE status IN ('rider_assigned', 'out_for_delivery');

CREATE TABLE order_events (
    id         BIGSERIAL PRIMARY KEY,
    order_id   UUID        NOT NULL REFERENCES medicine_orders (id),
    status     TEXT        NOT NULL,
    note       TEXT,
    actor_id   UUID REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX order_events_order_idx ON order_events (order_id, id);

CREATE TABLE pharmacy_reviews (
    order_id    UUID PRIMARY KEY REFERENCES medicine_orders (id),
    pharmacy_id UUID        NOT NULL REFERENCES pharmacies (id),
    patient_id  UUID        NOT NULL REFERENCES users (id),
    rating      INT         NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX pharmacy_reviews_pharmacy_idx ON pharmacy_reviews (pharmacy_id, created_at DESC);

-- +goose Down
DROP TABLE pharmacy_reviews;
DROP TABLE order_events;
DROP TABLE medicine_orders;
DROP TABLE pharmacy_quotes;
DROP TABLE medicine_requests;
DROP TABLE riders;
DROP TABLE pharmacies;
DROP FUNCTION distance_km(float8, float8, float8, float8);
