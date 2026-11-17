-- +goose Up

-- ---------------------------------------------------------------- hospitals

CREATE TABLE hospitals (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT        NOT NULL,
    registration_no     TEXT        NOT NULL UNIQUE, -- DGHS licence / registration number
    description         TEXT,
    address             TEXT        NOT NULL,
    district            TEXT        NOT NULL,
    area                TEXT,
    phone               TEXT        NOT NULL,
    email               TEXT,
    website             TEXT,
    departments         TEXT[]      NOT NULL DEFAULT '{}',
    facilities          TEXT[]      NOT NULL DEFAULT '{}',
    visiting_hours      TEXT,
    latitude            NUMERIC(9, 6),
    longitude           NUMERIC(9, 6),
    logo_photo_id       UUID,
    verification_status TEXT        NOT NULL DEFAULT 'pending'
                        CHECK (verification_status IN ('pending', 'approved', 'rejected')),
    verification_note   TEXT,
    verified_at         TIMESTAMPTZ,
    created_by          UUID        NOT NULL REFERENCES users (id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((latitude IS NULL) = (longitude IS NULL))
);

CREATE INDEX hospitals_status_idx ON hospitals (verification_status);

-- Who may manage a hospital. Owners can add and remove staff.
CREATE TABLE hospital_members (
    hospital_id UUID        NOT NULL REFERENCES hospitals (id) ON DELETE CASCADE,
    user_id     UUID        NOT NULL REFERENCES users (id),
    role        TEXT        NOT NULL CHECK (role IN ('owner', 'staff')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (hospital_id, user_id)
);

CREATE INDEX hospital_members_user_idx ON hospital_members (user_id);

CREATE TABLE hospital_photos (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id  UUID        NOT NULL REFERENCES hospitals (id) ON DELETE CASCADE,
    storage_key  TEXT        NOT NULL UNIQUE,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL CHECK (size_bytes > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX hospital_photos_hospital_idx ON hospital_photos (hospital_id, created_at);

ALTER TABLE hospitals
    ADD CONSTRAINT hospitals_logo_fk FOREIGN KEY (logo_photo_id) REFERENCES hospital_photos (id) ON DELETE SET NULL;

-- A doctor joins a hospital by accepting its invitation. The hospital sets only
-- hospital-specific details; the doctor's professional profile stays theirs.
CREATE TABLE hospital_doctors (
    hospital_id UUID        NOT NULL REFERENCES hospitals (id) ON DELETE CASCADE,
    doctor_id   UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    department  TEXT,
    designation TEXT,
    status      TEXT        NOT NULL DEFAULT 'invited'
                CHECK (status IN ('invited', 'active', 'declined', 'removed', 'left')),
    invited_by  UUID        NOT NULL REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (hospital_id, doctor_id)
);

CREATE INDEX hospital_doctors_doctor_idx ON hospital_doctors (doctor_id, status);

-- Hospital chambers are ordinary chambers run by the hospital, so they share the
-- doctor's schedule (no overlaps) and the central e-ticketing system.
ALTER TABLE chambers
    ADD COLUMN hospital_id UUID REFERENCES hospitals (id),
    ADD COLUMN room        TEXT;

CREATE INDEX chambers_hospital_idx ON chambers (hospital_id) WHERE hospital_id IS NOT NULL;

-- ---------------------------------------------------------------- appointments
-- Hospitals can cancel (with a reason) or reschedule bookings. A rescheduled
-- appointment points at its replacement.

ALTER TABLE appointments DROP CONSTRAINT appointments_status_check;
ALTER TABLE appointments
    ADD CONSTRAINT appointments_status_check
        CHECK (status IN ('confirmed', 'cancelled', 'completed', 'no_show', 'rescheduled')),
    ADD COLUMN cancelled_by   TEXT CHECK (cancelled_by IN ('patient', 'hospital', 'doctor')),
    ADD COLUMN cancel_reason  TEXT,
    ADD COLUMN rescheduled_to UUID REFERENCES appointments (id);

DROP INDEX appointments_one_active_per_patient;
CREATE UNIQUE INDEX appointments_one_active_per_patient
    ON appointments (patient_id, schedule_id, appointment_date)
    WHERE status NOT IN ('cancelled', 'rescheduled');

-- +goose Down
DROP INDEX appointments_one_active_per_patient;
UPDATE appointments SET status = 'cancelled' WHERE status = 'rescheduled';
CREATE UNIQUE INDEX appointments_one_active_per_patient
    ON appointments (patient_id, schedule_id, appointment_date)
    WHERE status <> 'cancelled';
ALTER TABLE appointments
    DROP COLUMN rescheduled_to,
    DROP COLUMN cancel_reason,
    DROP COLUMN cancelled_by,
    DROP CONSTRAINT appointments_status_check;
ALTER TABLE appointments
    ADD CONSTRAINT appointments_status_check
        CHECK (status IN ('confirmed', 'cancelled', 'completed', 'no_show'));

UPDATE chambers SET booking_enabled = false WHERE hospital_id IS NOT NULL;
ALTER TABLE chambers DROP COLUMN room, DROP COLUMN hospital_id;
DROP TABLE hospital_doctors;
ALTER TABLE hospitals DROP CONSTRAINT hospitals_logo_fk;
DROP TABLE hospital_photos;
DROP TABLE hospital_members;
DROP TABLE hospitals;
