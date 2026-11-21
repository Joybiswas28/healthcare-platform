-- +goose Up
-- One patient account can book appointments and consultations for family
-- members (children, parents, a spouse) who have no account of their own.
-- Removing a member only hides it: past bookings keep pointing at it.
CREATE TABLE family_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   UUID        NOT NULL REFERENCES users (id),
    name       TEXT        NOT NULL,
    relation   TEXT        NOT NULL CHECK (relation IN ('spouse', 'child', 'parent', 'sibling', 'other')),
    gender     TEXT CHECK (gender IN ('male', 'female', 'other')),
    birth_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at TIMESTAMPTZ
);

CREATE INDEX family_members_owner_idx ON family_members (owner_id) WHERE removed_at IS NULL;

ALTER TABLE appointments ADD COLUMN family_member_id UUID REFERENCES family_members (id);
ALTER TABLE consultations ADD COLUMN family_member_id UUID REFERENCES family_members (id);

-- The one-booking-per-session and one-open-consultation-per-doctor rules now
-- apply per person, so a parent can book themself and a child together.
DROP INDEX appointments_one_active_per_patient;
CREATE UNIQUE INDEX appointments_one_active_per_patient
    ON appointments (patient_id, COALESCE(family_member_id, '00000000-0000-0000-0000-000000000000'), schedule_id, appointment_date)
    WHERE status NOT IN ('cancelled', 'rescheduled');

DROP INDEX consultations_one_open_per_pair;
CREATE UNIQUE INDEX consultations_one_open_per_pair
    ON consultations (patient_id, COALESCE(family_member_id, '00000000-0000-0000-0000-000000000000'), doctor_id)
    WHERE status IN ('requested', 'accepted', 'active');

-- +goose Down
-- Fails, and changes nothing, while one account holds two open bookings for the
-- same session or doctor; settle those by hand first.
DROP INDEX consultations_one_open_per_pair;
CREATE UNIQUE INDEX consultations_one_open_per_pair
    ON consultations (patient_id, doctor_id)
    WHERE status IN ('requested', 'accepted', 'active');

DROP INDEX appointments_one_active_per_patient;
CREATE UNIQUE INDEX appointments_one_active_per_patient
    ON appointments (patient_id, schedule_id, appointment_date)
    WHERE status NOT IN ('cancelled', 'rescheduled');

ALTER TABLE consultations DROP COLUMN family_member_id;
ALTER TABLE appointments DROP COLUMN family_member_id;
DROP TABLE family_members;
