-- +goose Up
-- Live serial status for today's chamber sessions: the doctor or hospital
-- staff record which serial is being seen, and patients follow it from their
-- phone instead of waiting at the chamber for hours.
CREATE TABLE session_queue (
    schedule_id  UUID        NOT NULL REFERENCES chamber_schedules (id) ON DELETE CASCADE,
    session_date DATE        NOT NULL,
    now_serving  INT         NOT NULL CHECK (now_serving > 0),
    -- How many times the serial moved forward; with started_at it gives the
    -- session's real pace, for waiting-time estimates.
    advances     INT         NOT NULL DEFAULT 0,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, session_date)
);

-- When the patient was told their turn is near (sent once per booking).
ALTER TABLE appointments ADD COLUMN queue_alerted_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE appointments DROP COLUMN queue_alerted_at;
DROP TABLE session_queue;
