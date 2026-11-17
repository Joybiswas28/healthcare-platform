-- +goose Up

-- ---------------------------------------------------------------- public medical concerns
-- A patient posts a concern; verified doctors send offers; the patient accepts
-- one offer, which immediately starts a paid consultation. Doctors never see the
-- patient's identity on the public feed.

CREATE TABLE concerns (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id      UUID        NOT NULL REFERENCES users (id),
    title           TEXT        NOT NULL,
    description     TEXT        NOT NULL,
    specialization  TEXT,
    status          TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed', 'converted')),
    consultation_id UUID REFERENCES consultations (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at       TIMESTAMPTZ
);

CREATE INDEX concerns_open_idx ON concerns (created_at DESC) WHERE status = 'open';
CREATE INDEX concerns_patient_idx ON concerns (patient_id, created_at DESC);

CREATE TABLE concern_offers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    concern_id     UUID        NOT NULL REFERENCES concerns (id),
    doctor_id      UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    message        TEXT        NOT NULL,
    fee_tokens     INT         NOT NULL CHECK (fee_tokens > 0),
    commission_bps INT         NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
    status         TEXT        NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'accepted', 'declined', 'withdrawn')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (concern_id, doctor_id)
);

CREATE INDEX concern_offers_doctor_idx ON concern_offers (doctor_id, created_at DESC);

-- ---------------------------------------------------------------- doctor withdrawals
-- A withdrawal request moves money from the doctor's earnings into payouts_pending.
-- When an admin pays it, it leaves the platform (back through the gateway account);
-- when rejected, it returns to the doctor's earnings.

ALTER TABLE ledger_accounts
    DROP CONSTRAINT ledger_accounts_kind_check,
    DROP CONSTRAINT ledger_accounts_check,
    DROP CONSTRAINT ledger_accounts_non_negative;

ALTER TABLE ledger_accounts
    ADD CONSTRAINT ledger_accounts_kind_check
        CHECK (kind IN ('gateway', 'escrow', 'platform_revenue', 'payouts_pending', 'patient_tokens', 'doctor_earnings')),
    ADD CONSTRAINT ledger_accounts_owner_check
        CHECK ((owner_id IS NULL) = (kind IN ('gateway', 'escrow', 'platform_revenue', 'payouts_pending'))),
    ADD CONSTRAINT ledger_accounts_non_negative
        CHECK (kind IN ('gateway', 'platform_revenue') OR balance_paisa >= 0);

INSERT INTO ledger_accounts (kind) VALUES ('payouts_pending');

CREATE TABLE withdrawals (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id       UUID        NOT NULL REFERENCES doctor_profiles (user_id),
    amount_paisa    BIGINT      NOT NULL CHECK (amount_paisa > 0),
    method          TEXT        NOT NULL CHECK (method IN ('bkash', 'nagad', 'bank')),
    account_details TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'paid', 'rejected')),
    admin_note      TEXT,
    payout_ref      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX withdrawals_one_pending ON withdrawals (doctor_id) WHERE status = 'requested';
CREATE INDEX withdrawals_status_idx ON withdrawals (status, created_at);
CREATE INDEX withdrawals_doctor_idx ON withdrawals (doctor_id, created_at DESC);

-- ---------------------------------------------------------------- realtime chat
-- Every new message is announced on the consultation_messages channel as
-- "<consultation id>:<message id>". The notification is delivered on commit,
-- so API servers listening on it only ever see saved messages.

-- +goose StatementBegin
CREATE FUNCTION notify_consultation_message() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('consultation_messages', NEW.consultation_id::text || ':' || NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER consultation_messages_notify
    AFTER INSERT ON consultation_messages
    FOR EACH ROW EXECUTE FUNCTION notify_consultation_message();

-- +goose Down
DROP TRIGGER consultation_messages_notify ON consultation_messages;
DROP FUNCTION notify_consultation_message();

DROP TABLE withdrawals;
DELETE FROM ledger_accounts WHERE kind = 'payouts_pending';
ALTER TABLE ledger_accounts
    DROP CONSTRAINT ledger_accounts_kind_check,
    DROP CONSTRAINT ledger_accounts_owner_check,
    DROP CONSTRAINT ledger_accounts_non_negative;
ALTER TABLE ledger_accounts
    ADD CONSTRAINT ledger_accounts_kind_check
        CHECK (kind IN ('gateway', 'escrow', 'platform_revenue', 'patient_tokens', 'doctor_earnings')),
    ADD CONSTRAINT ledger_accounts_check
        CHECK ((owner_id IS NULL) = (kind IN ('gateway', 'escrow', 'platform_revenue'))),
    ADD CONSTRAINT ledger_accounts_non_negative
        CHECK (kind IN ('gateway', 'platform_revenue') OR balance_paisa >= 0);

DROP TABLE concern_offers;
DROP TABLE concerns;
