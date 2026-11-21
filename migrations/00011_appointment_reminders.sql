-- +goose Up
-- Patients get two reminders for a confirmed appointment: the evening before
-- and shortly before the session starts. These record that each was sent, so
-- a restart or a second server never sends one twice.
ALTER TABLE appointments
    ADD COLUMN reminded_day_before_at TIMESTAMPTZ,
    ADD COLUMN reminded_soon_at       TIMESTAMPTZ;

-- The reminder worker looks at confirmed appointments of today and tomorrow.
CREATE INDEX appointments_confirmed_date_idx ON appointments (appointment_date) WHERE status = 'confirmed';

-- +goose Down
DROP INDEX appointments_confirmed_date_idx;
ALTER TABLE appointments
    DROP COLUMN reminded_soon_at,
    DROP COLUMN reminded_day_before_at;
