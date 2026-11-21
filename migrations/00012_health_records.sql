-- +goose Up
-- A patient's own health records: lab reports, imaging, old prescriptions and
-- the like, for themself or a family member. Files live in the same storage as
-- chat attachments. A record is private until the patient shares it with the
-- doctor of one of their chamber appointments (record_shares) or sends a copy
-- into a consultation chat.
CREATE TABLE health_records (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id         UUID        NOT NULL REFERENCES users (id),
    family_member_id UUID REFERENCES family_members (id),
    title            TEXT        NOT NULL,
    kind             TEXT        NOT NULL
                     CHECK (kind IN ('lab_report', 'imaging', 'prescription', 'discharge', 'vaccination', 'other')),
    record_date      DATE,
    note             TEXT,
    file_name        TEXT        NOT NULL,
    content_type     TEXT        NOT NULL,
    size_bytes       BIGINT      NOT NULL CHECK (size_bytes > 0),
    storage_key      TEXT        NOT NULL UNIQUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX health_records_owner_idx ON health_records (owner_id, record_date DESC NULLS LAST, created_at DESC);

CREATE TABLE record_shares (
    record_id      UUID        NOT NULL REFERENCES health_records (id) ON DELETE CASCADE,
    appointment_id UUID        NOT NULL REFERENCES appointments (id),
    shared_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (record_id, appointment_id)
);

CREATE INDEX record_shares_appointment_idx ON record_shares (appointment_id);

-- +goose Down
DROP TABLE record_shares;
DROP TABLE health_records;
