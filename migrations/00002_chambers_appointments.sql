-- +goose Up
ALTER TABLE doctor_profiles
    ADD COLUMN degrees           TEXT,
    ADD COLUMN verification_note TEXT,
    ADD COLUMN verified_at       TIMESTAMPTZ;

CREATE INDEX doctor_profiles_status_idx ON doctor_profiles (verification_status);

CREATE TABLE chambers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id       UUID        NOT NULL REFERENCES doctor_profiles (user_id) ON DELETE CASCADE,
    name            TEXT        NOT NULL,
    address         TEXT        NOT NULL,
    district        TEXT        NOT NULL,
    area            TEXT,
    phone           TEXT,
    fee_paisa       BIGINT      NOT NULL CHECK (fee_paisa >= 0),
    booking_enabled BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX chambers_doctor_idx ON chambers (doctor_id);

-- Weekly recurring sessions. weekday: 0 = Sunday ... 6 = Saturday.
-- Rows are deactivated, never deleted, because appointments reference them.
CREATE TABLE chamber_schedules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chamber_id   UUID        NOT NULL REFERENCES chambers (id) ON DELETE CASCADE,
    weekday      SMALLINT    NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time   TIME        NOT NULL,
    end_time     TIME        NOT NULL,
    max_patients INT         NOT NULL CHECK (max_patients BETWEEN 1 AND 500),
    active       BOOLEAN     NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_time > start_time)
);

CREATE INDEX chamber_schedules_chamber_idx ON chamber_schedules (chamber_id);

CREATE TABLE appointments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_code      TEXT        NOT NULL UNIQUE,
    schedule_id      UUID        NOT NULL REFERENCES chamber_schedules (id),
    chamber_id       UUID        NOT NULL REFERENCES chambers (id),
    doctor_id        UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    patient_id       UUID        NOT NULL REFERENCES users (id),
    appointment_date DATE        NOT NULL,
    serial_no        INT         NOT NULL CHECK (serial_no > 0),
    fee_paisa        BIGINT      NOT NULL CHECK (fee_paisa >= 0),
    status           TEXT        NOT NULL DEFAULT 'confirmed'
                     CHECK (status IN ('confirmed', 'cancelled', 'completed', 'no_show')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, appointment_date, serial_no)
);

-- A patient may hold only one live booking per session.
CREATE UNIQUE INDEX appointments_one_active_per_patient
    ON appointments (patient_id, schedule_id, appointment_date)
    WHERE status <> 'cancelled';

CREATE INDEX appointments_doctor_date_idx ON appointments (doctor_id, appointment_date);
CREATE INDEX appointments_patient_idx ON appointments (patient_id, appointment_date DESC);

-- +goose Down
DROP TABLE appointments;
DROP TABLE chamber_schedules;
DROP TABLE chambers;
DROP INDEX doctor_profiles_status_idx;
ALTER TABLE doctor_profiles
    DROP COLUMN degrees,
    DROP COLUMN verification_note,
    DROP COLUMN verified_at;
