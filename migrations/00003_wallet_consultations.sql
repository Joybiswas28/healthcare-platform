-- +goose Up

-- Online consultation settings per doctor. commission_bps is the platform's share
-- in basis points (5000 = 50%), set by admins.
ALTER TABLE doctor_profiles
    ADD COLUMN consultation_enabled    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN consultation_fee_tokens INT     NOT NULL DEFAULT 1 CHECK (consultation_fee_tokens BETWEEN 1 AND 20),
    ADD COLUMN commission_bps          INT     NOT NULL DEFAULT 5000 CHECK (commission_bps BETWEEN 0 AND 10000);

-- ---------------------------------------------------------------- tokens & payments

CREATE TABLE token_packages (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL,
    price_paisa   BIGINT      NOT NULL CHECK (price_paisa > 0),
    tokens        INT         NOT NULL CHECK (tokens > 0),
    validity_days INT         NOT NULL CHECK (validity_days > 0),
    active        BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO token_packages (name, price_paisa, tokens, validity_days)
VALUES ('Standard: 10 tokens', 100000, 10, 365);

CREATE TABLE payments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users (id),
    package_id    UUID        NOT NULL REFERENCES token_packages (id),
    amount_paisa  BIGINT      NOT NULL CHECK (amount_paisa > 0),
    tokens        INT         NOT NULL CHECK (tokens > 0),
    validity_days INT         NOT NULL CHECK (validity_days > 0),
    provider      TEXT        NOT NULL,
    provider_ref  TEXT,
    status        TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at       TIMESTAMPTZ
);

CREATE INDEX payments_user_idx ON payments (user_id, created_at DESC);

-- Each purchase creates a lot. Tokens are spent from the lot that expires first.
CREATE TABLE token_lots (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id       UUID        NOT NULL REFERENCES users (id),
    payment_id       UUID        REFERENCES payments (id),
    tokens_total     INT         NOT NULL CHECK (tokens_total > 0),
    tokens_remaining INT         NOT NULL CHECK (tokens_remaining BETWEEN 0 AND tokens_total),
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX token_lots_payment_idx ON token_lots (payment_id);
CREATE INDEX token_lots_patient_idx ON token_lots (patient_id, expires_at) WHERE tokens_remaining > 0;

-- ---------------------------------------------------------------- double-entry ledger
-- Every money movement is a transaction whose entries sum to zero.
-- System accounts (owner_id NULL): gateway (cash received, goes negative),
-- escrow (tokens held for active consultations), platform_revenue.

CREATE TABLE ledger_accounts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      UUID REFERENCES users (id),
    kind          TEXT        NOT NULL CHECK (kind IN ('gateway', 'escrow', 'platform_revenue', 'patient_tokens', 'doctor_earnings')),
    balance_paisa BIGINT      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledger_accounts_non_negative
        CHECK (kind IN ('gateway', 'platform_revenue') OR balance_paisa >= 0),
    CHECK ((owner_id IS NULL) = (kind IN ('gateway', 'escrow', 'platform_revenue')))
);

CREATE UNIQUE INDEX ledger_accounts_user_idx ON ledger_accounts (owner_id, kind) WHERE owner_id IS NOT NULL;
CREATE UNIQUE INDEX ledger_accounts_system_idx ON ledger_accounts (kind) WHERE owner_id IS NULL;

INSERT INTO ledger_accounts (kind) VALUES ('gateway'), ('escrow'), ('platform_revenue');

CREATE TABLE ledger_transactions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         TEXT        NOT NULL,
    reference_id UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ledger_transactions_ref_idx ON ledger_transactions (reference_id);

CREATE TABLE ledger_entries (
    id             BIGSERIAL PRIMARY KEY,
    transaction_id UUID        NOT NULL REFERENCES ledger_transactions (id),
    account_id     UUID        NOT NULL REFERENCES ledger_accounts (id),
    amount_paisa   BIGINT      NOT NULL CHECK (amount_paisa <> 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ledger_entries_account_idx ON ledger_entries (account_id, id DESC);

-- ---------------------------------------------------------------- consultations

CREATE TABLE consultations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id     UUID        NOT NULL REFERENCES users (id),
    doctor_id      UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    subject        TEXT        NOT NULL,
    description    TEXT        NOT NULL,
    fee_tokens     INT         NOT NULL CHECK (fee_tokens > 0),
    commission_bps INT         NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
    status         TEXT        NOT NULL DEFAULT 'requested'
                   CHECK (status IN ('requested', 'accepted', 'rejected', 'active', 'completed', 'cancelled')),
    reject_reason  TEXT,
    requested_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at   TIMESTAMPTZ,
    confirmed_at   TIMESTAMPTZ,
    closed_at      TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX consultations_patient_idx ON consultations (patient_id, requested_at DESC);
CREATE INDEX consultations_doctor_idx ON consultations (doctor_id, requested_at DESC);

-- At most one open consultation between the same patient and doctor.
CREATE UNIQUE INDEX consultations_one_open_per_pair
    ON consultations (patient_id, doctor_id)
    WHERE status IN ('requested', 'accepted', 'active');

-- Which token lots paid for a consultation, so a refund can restore them exactly.
CREATE TABLE consultation_token_usage (
    consultation_id UUID NOT NULL REFERENCES consultations (id),
    lot_id          UUID NOT NULL REFERENCES token_lots (id),
    tokens          INT  NOT NULL CHECK (tokens > 0),
    PRIMARY KEY (consultation_id, lot_id)
);

CREATE TABLE attachments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consultation_id UUID        NOT NULL REFERENCES consultations (id),
    uploader_id     UUID        NOT NULL REFERENCES users (id),
    file_name       TEXT        NOT NULL,
    content_type    TEXT        NOT NULL,
    size_bytes      BIGINT      NOT NULL CHECK (size_bytes > 0),
    storage_key     TEXT        NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE consultation_messages (
    id              BIGSERIAL PRIMARY KEY,
    consultation_id UUID        NOT NULL REFERENCES consultations (id),
    sender_id       UUID        NOT NULL REFERENCES users (id),
    body            TEXT,
    attachment_id   UUID REFERENCES attachments (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (body IS NOT NULL OR attachment_id IS NOT NULL)
);

CREATE INDEX consultation_messages_idx ON consultation_messages (consultation_id, id);

-- +goose Down
DROP TABLE consultation_messages;
DROP TABLE attachments;
DROP TABLE consultation_token_usage;
DROP TABLE consultations;
DROP TABLE ledger_entries;
DROP TABLE ledger_transactions;
DROP TABLE ledger_accounts;
DROP TABLE token_lots;
DROP TABLE payments;
DROP TABLE token_packages;
ALTER TABLE doctor_profiles
    DROP COLUMN consultation_enabled,
    DROP COLUMN consultation_fee_tokens,
    DROP COLUMN commission_bps;
