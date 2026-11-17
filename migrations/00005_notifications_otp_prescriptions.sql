-- +goose Up

-- ---------------------------------------------------------------- notifications
-- In-app notifications, which double as the SMS outbox: rows with
-- sms_status = 'pending' are picked up and sent by a background worker. Rows are
-- written in the same transaction as the event they describe, so a notification
-- exists if and only if the event happened.

CREATE TABLE notifications (
    id           BIGSERIAL PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users (id),
    kind         TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    body         TEXT        NOT NULL,
    sms_status   TEXT CHECK (sms_status IN ('pending', 'sent', 'failed')),
    sms_attempts INT         NOT NULL DEFAULT 0,
    sms_error    TEXT,
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at      TIMESTAMPTZ
);

CREATE INDEX notifications_user_idx ON notifications (user_id, id DESC);
CREATE INDEX notifications_sms_pending_idx ON notifications (id) WHERE sms_status = 'pending';

-- ---------------------------------------------------------------- one-time passwords
-- Only an HMAC of the code is stored.

CREATE TABLE otp_codes (
    id          BIGSERIAL PRIMARY KEY,
    phone       TEXT        NOT NULL,
    purpose     TEXT        NOT NULL CHECK (purpose IN ('register', 'reset_password')),
    code_hash   TEXT        NOT NULL,
    attempts    INT         NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX otp_codes_phone_idx ON otp_codes (phone, purpose, created_at DESC);

-- ---------------------------------------------------------------- prescriptions
-- Doctor and patient details are copied in at issue time, so a prescription
-- reads the same forever even if a profile changes later. A prescription is never
-- edited: a mistake is fixed by revoking it and issuing a new one.

CREATE TABLE prescriptions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                  TEXT        NOT NULL UNIQUE,
    doctor_id             UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    patient_id            UUID        NOT NULL REFERENCES users (id),
    consultation_id       UUID REFERENCES consultations (id),
    appointment_id        UUID REFERENCES appointments (id),

    doctor_name           TEXT        NOT NULL,
    doctor_degrees        TEXT,
    doctor_specialization TEXT,
    doctor_bmdc           TEXT        NOT NULL,
    patient_name          TEXT        NOT NULL,
    patient_age           TEXT,
    patient_gender        TEXT CHECK (patient_gender IN ('male', 'female', 'other')),

    chief_complaints      TEXT,
    diagnosis             TEXT,
    medicines             JSONB       NOT NULL,
    investigations        JSONB       NOT NULL DEFAULT '[]',
    advice                TEXT,
    follow_up_date        DATE,

    status                TEXT        NOT NULL DEFAULT 'issued' CHECK (status IN ('issued', 'revoked')),
    revoked_reason        TEXT,
    issued_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at            TIMESTAMPTZ,

    CHECK ((consultation_id IS NOT NULL) <> (appointment_id IS NOT NULL)),
    CHECK (jsonb_typeof(medicines) = 'array' AND jsonb_typeof(investigations) = 'array')
);

CREATE INDEX prescriptions_patient_idx ON prescriptions (patient_id, issued_at DESC);
CREATE INDEX prescriptions_doctor_idx ON prescriptions (doctor_id, issued_at DESC);
CREATE INDEX prescriptions_consultation_idx ON prescriptions (consultation_id);
CREATE INDEX prescriptions_appointment_idx ON prescriptions (appointment_id);

-- +goose Down
DROP TABLE prescriptions;
DROP TABLE otp_codes;
DROP TABLE notifications;
