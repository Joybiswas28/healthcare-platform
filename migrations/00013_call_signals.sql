-- +goose Up
-- Video calls in a consultation run browser to browser (WebRTC). The server
-- only relays the short setup messages between the two people: the offer, the
-- answer, network candidates, and hang-up. They are useful for a minute or two
-- and are cleaned up as new ones arrive.
CREATE TABLE call_signals (
    id              BIGSERIAL PRIMARY KEY,
    consultation_id UUID        NOT NULL REFERENCES consultations (id),
    sender_id       UUID        NOT NULL REFERENCES users (id),
    kind            TEXT        NOT NULL CHECK (kind IN ('offer', 'answer', 'candidate', 'hangup', 'decline')),
    payload         TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX call_signals_consultation_idx ON call_signals (consultation_id, id);

-- Wake the consultation's listeners the same way new chat messages do.
-- +goose StatementBegin
CREATE FUNCTION notify_call_signal() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('consultation_messages', NEW.consultation_id::text || ':signal:' || NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER call_signals_notify
    AFTER INSERT ON call_signals
    FOR EACH ROW EXECUTE FUNCTION notify_call_signal();

-- +goose Down
DROP TRIGGER call_signals_notify ON call_signals;
DROP FUNCTION notify_call_signal();
DROP TABLE call_signals;
