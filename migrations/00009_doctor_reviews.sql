-- +goose Up
-- A patient rates a doctor once per finished visit: a completed chamber
-- appointment or a completed online consultation. Reviews are shown without the
-- patient's name, and the doctor can answer each one publicly.
CREATE TABLE doctor_reviews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id       UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    patient_id      UUID        NOT NULL REFERENCES users (id),
    appointment_id  UUID UNIQUE REFERENCES appointments (id),
    consultation_id UUID UNIQUE REFERENCES consultations (id),
    rating          INT         NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment         TEXT,
    reply           TEXT,
    replied_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(appointment_id, consultation_id) = 1)
);

CREATE INDEX doctor_reviews_doctor_idx ON doctor_reviews (doctor_id, created_at DESC);
CREATE INDEX doctor_reviews_patient_idx ON doctor_reviews (patient_id);

-- +goose Down
DROP TABLE doctor_reviews;
