-- +goose Up
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL,
    phone         TEXT        NOT NULL UNIQUE,
    email         TEXT        UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN ('patient', 'doctor', 'hospital_admin', 'pharmacy', 'rider', 'admin')),
    status        TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE doctor_profiles (
    user_id             UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    bmdc_reg_no         TEXT UNIQUE,
    specialization      TEXT,
    experience_years    INT  CHECK (experience_years >= 0),
    bio                 TEXT,
    verification_status TEXT NOT NULL DEFAULT 'pending' CHECK (verification_status IN ('pending', 'approved', 'rejected')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE doctor_profiles;
DROP TABLE users;
